package evalproof

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/evalreadback"
)

func Build(inputRoot, outRoot string, options Options) (Packet, error) {
	claim := strings.TrimSpace(options.Claim)
	if !validClaim(claim) {
		return Packet{}, fmt.Errorf("unsupported proof claim: %s", claim)
	}
	if strings.TrimSpace(inputRoot) == "" {
		return Packet{}, errors.New("missing input root")
	}
	if strings.TrimSpace(outRoot) == "" {
		return Packet{}, errors.New("missing output root")
	}
	root, err := filepath.Abs(outRoot)
	if err != nil {
		return Packet{}, err
	}
	proofDir := filepath.Join(root, DirName)
	if err := evalreadback.ValidateOutputPath(root, proofDir, options.ProtectedRoots); err != nil {
		return Packet{}, err
	}
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return Packet{}, err
	}

	summary, readbackRef, err := readbackFor(inputRoot, proofDir, options)
	if err != nil {
		if !strings.Contains(err.Error(), "no supported eval/trace artifacts") {
			return Packet{}, err
		}
		summary = missingProofSummary(inputRoot)
		readbackRef = ""
	}
	packet := Evaluate(summary, claim, readbackRef, options)
	if err := writePacket(root, packet, options.ProtectedRoots); err != nil {
		return Packet{}, err
	}
	return packet, nil
}

func Evaluate(summary evalreadback.Summary, claim string, readbackRef string, options Options) Packet {
	packet := Packet{
		SchemaVersion:        PacketSchemaVersion,
		RunID:                stableID("proof", []string{summary.RunID, claim, summary.ImprovementStatus, summary.GeneralizationStatus}),
		Claim:                claim,
		Verdict:              VerdictPass,
		ExitCode:             0,
		InputRootLabel:       summary.InputRootLabel,
		BaselineRootLabel:    summary.BaselineRootLabel,
		ReadbackSummaryRef:   readbackRef,
		EvalProjection:       projectionFor(summary, claim),
		GeneralizationLimit:  generalizationLimit(summary),
		TopImprovementTarget: summary.TopImprovementTarget,
		RerunInstructions:    summary.RerunInstructions,
		SafeArtifactRefs:     append([]string{}, summary.SafeArtifactRefs...),
	}
	packet.SafeArtifactRefs = append(packet.SafeArtifactRefs, summary.BaselineArtifactRefs...)
	sort.Strings(packet.SafeArtifactRefs)

	packet.MandatoryGates = mandatoryGates(summary, claim, options)
	packet.PermittedClaims, packet.BlockedClaims, packet.FailedClaims = classifyClaims(summary)
	packet.Verdict = verdictFor(packet.MandatoryGates)
	if packet.Verdict != VerdictPass {
		packet.ExitCode = 2
	}
	return packet
}

func readbackFor(inputRoot, proofDir string, options Options) (evalreadback.Summary, string, error) {
	if summaryPath := existingReadbackSummaryPath(inputRoot); summaryPath != "" {
		summary, err := evalreadback.LoadSummary(summaryPath)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		return summary, safeExistingReadbackRef(summaryPath), nil
	}
	readbackOut := filepath.Join(proofDir, "readback")
	summary, err := evalreadback.Build(inputRoot, readbackOut, evalreadback.Options{
		BaselineRoot:   options.BaselineRoot,
		ProtectedRoots: options.ProtectedRoots,
	})
	if err != nil {
		return evalreadback.Summary{}, "", err
	}
	return summary, filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)), nil
}

func existingReadbackSummaryPath(input string) string {
	info, err := os.Stat(input)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		if filepath.Base(input) == evalreadback.ReadbackSummaryFile {
			return input
		}
		return ""
	}
	for _, candidate := range []string{
		filepath.Join(input, evalreadback.ReadbackSummaryFile),
		filepath.Join(input, evalreadback.DirName, evalreadback.ReadbackSummaryFile),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func safeExistingReadbackRef(path string) string {
	if filepath.Base(path) == evalreadback.ReadbackSummaryFile {
		return "input/" + evalreadback.ReadbackSummaryFile
	}
	return "input/readback-summary"
}

func mandatoryGates(summary evalreadback.Summary, claim string, options Options) []GateResult {
	gates := []GateResult{
		readbackStatusGate(summary, "artifact_presence", []string{"pass"}, "required local eval/readback evidence exists"),
		statusGate("schema_supported", schemaStatus(summary), []string{"pass"}, "unsupported artifact schemas block machine proof"),
		readbackStatusGate(summary, "privacy_safe_readback", []string{"pass"}, "proof output must be safe for PR and Chain capture"),
		readbackStatusGate(summary, "side_effect_claim", []string{"pass"}, "proof requires zero prohibited side-effect counters"),
	}
	switch claim {
	case ClaimImprovement:
		if strings.TrimSpace(options.BaselineRoot) == "" && summary.Comparison == nil {
			gates = append(gates, GateResult{
				Gate:             "improvement_claim",
				RequiredStatuses: []string{"pass"},
				ActualStatus:     "blocked",
				Verdict:          VerdictBlocked,
				ReasonCodes:      []string{"missing_baseline"},
				ClaimImpact:      "blocks improvement claim until comparable baseline/current evidence is supplied",
			})
		} else {
			status, reasons := readbackGate(summary, "improvement_claim")
			gates = append(gates, statusGateWithReasons("improvement_claim", status, []string{"pass"}, reasons, "improvement claims require comparable positive baseline/current deltas"))
		}
	case ClaimGeneralization:
		gates = append(gates, readbackStatusGate(summary, "generalization_claim", []string{"pass"}, "generalization claims require reusable or held-out evidence"))
	case ClaimDEC64:
		gates = append(gates, readbackStatusGate(summary, "dec64_no_human_claim", []string{"pass"}, "no-human claims require held-out threshold proof and safety counters"))
	}
	return gates
}

func readbackStatusGate(summary evalreadback.Summary, name string, required []string, impact string) GateResult {
	status, reasons := readbackGate(summary, name)
	return statusGateWithReasons(name, status, required, reasons, impact)
}

func statusGate(name, actual string, required []string, impact string) GateResult {
	return statusGateWithReasons(name, actual, required, nil, impact)
}

func statusGateWithReasons(name, actual string, required []string, reasons []string, impact string) GateResult {
	result := GateResult{
		Gate:             name,
		RequiredStatuses: required,
		ActualStatus:     actual,
		Verdict:          VerdictPass,
		ClaimImpact:      impact,
	}
	for _, status := range required {
		if actual == status {
			return result
		}
	}
	result.Verdict = VerdictFail
	if actual == "blocked" || actual == "not_evaluated" {
		result.Verdict = VerdictBlocked
	}
	if len(reasons) > 0 && name != "privacy_safe_readback" && name != "generalization_claim" && name != "side_effect_claim" {
		result.ReasonCodes = append([]string{}, reasons...)
	} else {
		result.ReasonCodes = reasonCodesFor(name, actual)
	}
	return result
}

func reasonCodesFor(gate, actual string) []string {
	switch gate {
	case "schema_supported":
		return []string{"unsupported_artifact"}
	case "privacy_safe_readback":
		return []string{"unsafe_output"}
	case "side_effect_claim":
		if actual == "blocked" {
			return []string{"missing_side_effect_evidence"}
		}
		return []string{"guardrail_failed"}
	case "improvement_claim":
		switch actual {
		case "blocked", "not_evaluated":
			return []string{"missing_baseline"}
		case "not_comparable":
			return []string{"not_comparable"}
		default:
			return []string{"kr_failed"}
		}
	case "generalization_claim":
		return []string{"non_generalizable"}
	case "dec64_no_human_claim":
		return []string{"held_out_threshold_not_proven"}
	default:
		if actual == "" {
			return []string{"missing_proof"}
		}
		return []string{actual}
	}
}

func readbackGateStatus(summary evalreadback.Summary, gate string) string {
	status, _ := readbackGate(summary, gate)
	return status
}

func readbackGate(summary evalreadback.Summary, gate string) (string, []string) {
	for _, claimGate := range summary.ClaimGates {
		if claimGate.Gate == gate {
			return claimGate.Status, claimGate.ReasonCodes
		}
	}
	switch gate {
	case "improvement_claim":
		if summary.ImprovementStatus == "not_evaluated" {
			return "blocked", []string{"missing_baseline"}
		}
		return summary.ImprovementStatus, nil
	default:
		return "", nil
	}
}

func schemaStatus(summary evalreadback.Summary) string {
	for _, artifact := range append(append([]evalreadback.ArtifactEvidence{}, summary.Artifacts...), summary.BaselineArtifacts...) {
		if artifact.Status == "unsupported_schema" {
			return "fail"
		}
	}
	return "pass"
}

func verdictFor(gates []GateResult) string {
	verdict := VerdictPass
	for _, gate := range gates {
		switch gate.Verdict {
		case VerdictFail:
			return VerdictFail
		case VerdictBlocked:
			verdict = VerdictBlocked
		}
	}
	return verdict
}

func classifyClaims(summary evalreadback.Summary) (permitted []ClaimResult, blocked []ClaimResult, failed []ClaimResult) {
	for _, gate := range summary.ClaimGates {
		result := ClaimResult{Claim: gate.Gate, Status: gate.Status, ReasonCodes: append([]string{}, gate.ReasonCodes...)}
		switch gate.Status {
		case "pass":
			permitted = append(permitted, result)
		case "fail":
			failed = append(failed, result)
		case "blocked":
			blocked = append(blocked, result)
		}
	}
	return permitted, blocked, failed
}

func projectionFor(summary evalreadback.Summary, claim string) EvalProjection {
	return EvalProjection{
		IntendedUsers:              "Mindline operators, agents, and reviewers validating a named claim before PR or Chain proof",
		InputSourceTypes:           sortedKeys(summary.ArtifactTypeCounts),
		OutputDestinationSurfaces:  []string{"local proof packet", "local proof report", "safe Chain capture draft"},
		WorkspaceAssumptions:       "source-neutral, destination-neutral, provider-neutral local artifact proof",
		ProviderModelAssumptions:   "no provider or hosted model is required by the proof gate",
		PrivacyBoundary:            "metadata-only local proof; no raw private source content, prompts, completions, permalinks, or absolute private paths",
		SampleStatus:               summary.SampleStatus,
		HeldOutGeneralizationClaim: claimGeneralizationClaim(summary, claim),
		KRThresholds:               thresholdsFor(claim),
		Guardrails:                 "zero prohibited side-effect counters required for safety-dependent claims",
	}
}

func claimGeneralizationClaim(summary evalreadback.Summary, claim string) string {
	if claim == ClaimGeneralization || claim == ClaimDEC64 {
		return summary.GeneralizationStatus
	}
	if summary.GeneralizationStatus != "generalizable" {
		return "not claimed; broad generalization remains blocked"
	}
	return "generalizable evidence detected but not required by selected claim"
}

func thresholdsFor(claim string) []string {
	switch claim {
	case ClaimDEC64:
		return []string{"held-out threshold >= 0.98", "no-human eligible", "zero destination-write/autonomy side effects"}
	case ClaimImprovement:
		return []string{"comparable baseline/current evidence", "at least one supported positive metric delta", "no guardrail regression"}
	case ClaimGeneralization:
		return []string{"generalization_claim=pass", "no private/temp/non-held-out sample-bound proof"}
	default:
		return []string{"artifact presence", "supported schemas", "privacy-safe readback", "side-effect safety"}
	}
}

func generalizationLimit(summary evalreadback.Summary) string {
	if summary.GeneralizationStatus == "generalizable" {
		return "bounded generalization is supported by readback evidence"
	}
	return "broad product, destination-readiness, and no-human claims remain blocked"
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validClaim(claim string) bool {
	switch claim {
	case ClaimSafety, ClaimImprovement, ClaimGeneralization, ClaimDEC64:
		return true
	default:
		return false
	}
}

func missingProofSummary(inputRoot string) evalreadback.Summary {
	label := safeRootLabel(inputRoot)
	return evalreadback.Summary{
		SchemaVersion:        evalreadback.SummarySchemaVersion,
		RunID:                stableID("missing-proof", []string{label}),
		InputRootLabel:       label,
		ArtifactCount:        0,
		ArtifactTypeCounts:   map[string]int{},
		SampleStatus:         sampleStatusFor(inputRoot),
		GeneralizationStatus: "non_generalizable",
		ImprovementStatus:    "not_evaluated",
		ClaimGates: []evalreadback.ClaimGate{
			{Gate: "artifact_presence", Status: "fail", ReasonCodes: []string{"missing_proof"}, ClaimImpact: "blocks proof until local eval/readback artifacts exist"},
			{Gate: "privacy_safe_readback", Status: "pass", ClaimImpact: "no unsafe supported artifacts were available to inspect"},
			{Gate: "generalization_claim", Status: "blocked", ReasonCodes: []string{"missing_proof"}, ClaimImpact: "blocks broad product, DEC-64, or no-human claims"},
			{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"missing_baseline"}, ClaimImpact: "blocks improvement claim until comparable baseline/current evidence is supplied"},
			{Gate: "dec64_no_human_claim", Status: "blocked", ReasonCodes: []string{"held_out_threshold_not_proven"}, ClaimImpact: "blocks no-human autonomy readiness claim"},
			{Gate: "side_effect_claim", Status: "blocked", ReasonCodes: []string{"missing_side_effect_evidence"}, ClaimImpact: "blocks safety claim until artifacts expose guardrail counters"},
		},
		TopImprovementTarget: evalreadback.ImprovementTarget{Code: "missing_proof", Rationale: "No supported eval/readback artifacts were found."},
		RerunInstructions:    []string{"run the relevant Mindline eval command to produce local trace/eval artifacts, then rerun eval proof-gate"},
	}
}

func safeRootLabel(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return stableID("root", []string{filepath.ToSlash(abs)})
}

func sampleStatusFor(root string) string {
	clean := filepath.ToSlash(root)
	switch {
	case strings.Contains(clean, "/private/tmp/"):
		return "private_runtime"
	case strings.Contains(clean, "/temp/") || strings.HasSuffix(clean, "/temp"):
		return "temp_runtime"
	case strings.Contains(clean, "/testdata/"):
		return "fixture"
	default:
		return "unknown"
	}
}

func stableID(prefix string, parts []string) string {
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(prefix + ":" + strings.Join(parts, "|")))
	return strings.Trim(prefix, "-_ ") + "-" + hex.EncodeToString(sum[:])[:12]
}
