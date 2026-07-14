package evalloopdecision

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/evalproof"
	"github.com/synergyai-os/Mindline/internal/evalreadback"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

func Build(inputRoot, outRoot string, options Options) (Packet, error) {
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
	if err := evalreadback.ValidateOutputPath(root, root, options.ProtectedRoots); err != nil {
		return Packet{}, err
	}
	if err := privateio.PrepareDir(root); err != nil {
		return Packet{}, err
	}
	decisionDir := filepath.Join(root, DirName)
	if err := evalreadback.ValidateOutputPath(root, decisionDir, options.ProtectedRoots); err != nil {
		return Packet{}, err
	}
	if err := privateio.PrepareDir(decisionDir); err != nil {
		return Packet{}, err
	}

	summary, readbackRef, err := readbackFor(inputRoot, decisionDir, options)
	if err != nil {
		return Packet{}, err
	}
	packet := Decide(summary, readbackRef, options)
	if err := Write(root, packet, options.ProtectedRoots); err != nil {
		return Packet{}, err
	}
	return packet, nil
}

func Decide(summary evalreadback.Summary, readbackRef string, options Options) Packet {
	target := topTarget(summary, options)
	if strings.TrimSpace(target.Code) == "" {
		target = evalreadback.ImprovementTarget{
			Code:      "inspect_readback",
			Rationale: "Readback did not name a target; inspect the canonical local eval artifacts before changing product behavior.",
		}
	}
	packet := Packet{
		SchemaVersion:        PacketSchemaVersion,
		RunID:                stableID("loop-decision", []string{summary.RunID, summary.ImprovementStatus, target.Code}),
		DecisionKind:         "read_only_improvement_decision",
		CurrentRootLabel:     summary.InputRootLabel,
		BaselineRootLabel:    summary.BaselineRootLabel,
		ImprovementState:     improvementState(summary, options),
		ClaimStatuses:        claimStatuses(summary, options),
		TopImprovementTarget: target,
		ProductGeneralTarget: productGeneralTarget(summary),
		RerunInstruction:     rerunInstruction(summary),
		Guardrails:           summary.Guardrails,
		SafeArtifactRefs:     safeRefs(summary),
		ReadbackSummaryRef:   readbackRef,
		DecisionLimits:       decisionLimits(summary, options),
	}
	if summary.Comparison != nil {
		comparison := *summary.Comparison
		packet.Comparison = &comparison
	}
	return packet
}

func topTarget(summary evalreadback.Summary, options Options) evalreadback.ImprovementTarget {
	if summary.TopImprovementTarget.Code == "missing_proof" {
		return summary.TopImprovementTarget
	}
	if strings.TrimSpace(options.BaselineRoot) == "" && summary.Comparison == nil {
		return evalreadback.ImprovementTarget{
			Code:         "establish_comparable_baseline",
			Rationale:    "The decision loop cannot prove improvement until a comparable baseline/current pair exists.",
			EvidenceRefs: safeRefs(summary),
		}
	}
	if summary.ImprovementStatus == "not_comparable" {
		return evalreadback.ImprovementTarget{
			Code:         "repair_comparability",
			Rationale:    "Baseline and current artifacts are not comparable, so improvement cannot be trusted.",
			EvidenceRefs: safeRefs(summary),
		}
	}
	return summary.TopImprovementTarget
}

func readbackFor(inputRoot, decisionDir string, options Options) (evalreadback.Summary, string, error) {
	if summaryPath := existingReadbackSummaryPath(inputRoot); summaryPath != "" {
		summary, err := evalreadback.LoadSummary(summaryPath)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		summary, err = applyBaseline(summary, options.BaselineRoot)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		readbackRef, err := persistReadbackSummary(decisionDir, summary, options.ProtectedRoots)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		return summary, readbackRef, nil
	}
	if proofPath := existingProofPacketPath(inputRoot); proofPath != "" {
		summaryPath := proofReadbackSummaryPath(inputRoot, proofPath)
		if summaryPath == "" {
			summary, err := summaryFromProofPacket(inputRoot, proofPath)
			if err != nil {
				return evalreadback.Summary{}, "", err
			}
			if strings.TrimSpace(options.BaselineRoot) != "" {
				summary, err = applyBaseline(summary, options.BaselineRoot)
				if err != nil {
					return evalreadback.Summary{}, "", err
				}
			}
			readbackRef, err := persistReadbackSummary(decisionDir, summary, options.ProtectedRoots)
			if err != nil {
				return evalreadback.Summary{}, "", err
			}
			return summary, readbackRef, nil
		}
		summary, err := evalreadback.LoadSummary(summaryPath)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		summary, err = applyBaseline(summary, options.BaselineRoot)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		readbackRef, err := persistReadbackSummary(decisionDir, summary, options.ProtectedRoots)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		return summary, readbackRef, nil
	}
	readbackOut := filepath.Join(decisionDir, "readback")
	summary, err := evalreadback.Build(inputRoot, readbackOut, evalreadback.Options{
		ProtectedRoots: options.ProtectedRoots,
	})
	if err != nil {
		return evalreadback.Summary{}, "", err
	}
	summary, err = applyBaseline(summary, options.BaselineRoot)
	if err != nil {
		return evalreadback.Summary{}, "", err
	}
	if strings.TrimSpace(options.BaselineRoot) != "" {
		if err := evalreadback.Write(readbackOut, summary, options.ProtectedRoots); err != nil {
			return evalreadback.Summary{}, "", err
		}
	}
	return summary, filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)), nil
}

func persistReadbackSummary(decisionDir string, summary evalreadback.Summary, protectedRoots []string) (string, error) {
	readbackOut := filepath.Join(decisionDir, "readback")
	if err := evalreadback.Write(readbackOut, summary, protectedRoots); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)), nil
}

func applyBaseline(summary evalreadback.Summary, baselineRoot string) (evalreadback.Summary, error) {
	if strings.TrimSpace(baselineRoot) == "" {
		return evalreadback.ApplyBaseline(summary, "")
	}
	if baselineSummary, _, err := evalreadback.LoadSummaryFromRoot(baselineRoot); err == nil {
		return evalreadback.ApplyBaselineSummary(summary, baselineSummary), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return evalreadback.Summary{}, err
	}
	if proofPath := existingProofPacketPath(baselineRoot); proofPath != "" {
		baselineSummaryPath := proofReadbackSummaryPath(baselineRoot, proofPath)
		if baselineSummaryPath == "" {
			return evalreadback.Summary{}, errors.New("baseline proof packet input missing readback summary")
		}
		baselineSummary, err := evalreadback.LoadSummary(baselineSummaryPath)
		if err != nil {
			return evalreadback.Summary{}, err
		}
		return evalreadback.ApplyBaselineSummary(summary, baselineSummary), nil
	}
	return evalreadback.ApplyBaseline(summary, baselineRoot)
}

func existingProofPacketPath(input string) string {
	info, err := os.Stat(input)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		if filepath.Base(input) == "proof-packet.json" {
			return input
		}
		return ""
	}
	for _, candidate := range []string{
		filepath.Join(input, "proof-packet.json"),
		filepath.Join(input, evalproof.DirName, "proof-packet.json"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func proofReadbackSummaryPath(inputRoot, proofPath string) string {
	var packet evalproof.Packet
	data, err := os.ReadFile(proofPath)
	if err != nil {
		return ""
	}
	if err := json.Unmarshal(data, &packet); err != nil {
		return ""
	}
	if strings.TrimSpace(packet.ReadbackSummaryRef) != "" {
		if path := resolveProofReadbackRef(inputRoot, proofPath, packet.ReadbackSummaryRef); path != "" {
			return path
		}
	}
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(proofPath), "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile),
		filepath.Join(filepath.Dir(filepath.Dir(proofPath)), "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func summaryFromProofPacket(inputRoot, proofPath string) (evalreadback.Summary, error) {
	data, err := os.ReadFile(proofPath)
	if err != nil {
		return evalreadback.Summary{}, err
	}
	var packet evalproof.Packet
	if err := json.Unmarshal(data, &packet); err != nil {
		return evalreadback.Summary{}, err
	}
	if packet.SchemaVersion != evalproof.PacketSchemaVersion {
		return evalreadback.Summary{}, errors.New("unsupported proof packet schema")
	}
	typeCounts := map[string]int{}
	for _, ref := range packet.SafeArtifactRefs {
		if artifactType := artifactTypeFromProofRef(ref); artifactType != "" {
			typeCounts[artifactType]++
		}
	}
	summary := evalreadback.Summary{
		SchemaVersion:        evalreadback.SummarySchemaVersion,
		RunID:                stableID("proof-packet-summary", []string{packet.RunID, proofPath}),
		InputRootLabel:       firstNonEmpty(packet.InputRootLabel, safeRootLabel(inputRoot)),
		BaselineRootLabel:    packet.BaselineRootLabel,
		ArtifactCount:        len(packet.SafeArtifactRefs),
		ArtifactTypeCounts:   typeCounts,
		SampleStatus:         firstNonEmpty(packet.EvalProjection.SampleStatus, sampleStatusFor(inputRoot)),
		GeneralizationStatus: proofGeneralizationStatus(packet),
		ImprovementStatus:    proofImprovementStatus(packet),
		ClaimGates:           proofClaimGates(packet),
		TopImprovementTarget: packet.TopImprovementTarget,
		RerunInstructions:    append([]string{}, packet.RerunInstructions...),
		SafeArtifactRefs:     append([]string{}, packet.SafeArtifactRefs...),
	}
	if strings.TrimSpace(summary.TopImprovementTarget.Code) == "" {
		summary.TopImprovementTarget = evalreadback.ImprovementTarget{
			Code:      "missing_proof",
			Rationale: "Proof packet did not include readback-backed artifacts to inspect.",
		}
	}
	if len(summary.RerunInstructions) == 0 {
		summary.RerunInstructions = []string{"run the relevant Mindline eval command to produce local trace/eval artifacts, then rerun eval loop-decision"}
	}
	if summary.GeneralizationStatus == "" {
		summary.GeneralizationStatus = "non_generalizable"
	}
	if summary.ImprovementStatus == "" {
		summary.ImprovementStatus = "not_evaluated"
	}
	return summary, nil
}

func proofClaimGates(packet evalproof.Packet) []evalreadback.ClaimGate {
	gates := []evalreadback.ClaimGate{}
	seen := map[string]bool{}
	for _, gate := range packet.MandatoryGates {
		status := gate.ActualStatus
		if status == "" && gate.Verdict == evalproof.VerdictPass {
			status = "pass"
		}
		if status == "" {
			status = "blocked"
		}
		gates = append(gates, evalreadback.ClaimGate{
			Gate:        gate.Gate,
			Status:      status,
			ReasonCodes: append([]string{}, gate.ReasonCodes...),
			ClaimImpact: gate.ClaimImpact,
		})
		seen[gate.Gate] = true
	}
	for _, claim := range append(append([]evalproof.ClaimResult{}, packet.PermittedClaims...), append(packet.BlockedClaims, packet.FailedClaims...)...) {
		if seen[claim.Claim] {
			continue
		}
		gates = append(gates, evalreadback.ClaimGate{
			Gate:        claim.Claim,
			Status:      claim.Status,
			ReasonCodes: append([]string{}, claim.ReasonCodes...),
			ClaimImpact: "carried from proof packet because no readback summary ref was available",
		})
		seen[claim.Claim] = true
	}
	return gates
}

func proofGeneralizationStatus(packet evalproof.Packet) string {
	for _, claim := range packet.PermittedClaims {
		if claim.Claim == "generalization_claim" || claim.Claim == evalproof.ClaimGeneralization {
			return "generalizable"
		}
	}
	if strings.Contains(packet.GeneralizationLimit, "supported") {
		return "generalizable"
	}
	return "non_generalizable"
}

func proofImprovementStatus(packet evalproof.Packet) string {
	for _, claim := range packet.PermittedClaims {
		if claim.Claim == "improvement_claim" || claim.Claim == evalproof.ClaimImprovement {
			return "improved"
		}
	}
	for _, claim := range packet.FailedClaims {
		if claim.Claim == "improvement_claim" || claim.Claim == evalproof.ClaimImprovement {
			return "regressed"
		}
	}
	for _, claim := range packet.BlockedClaims {
		if claim.Claim == "improvement_claim" || claim.Claim == evalproof.ClaimImprovement {
			return "not_evaluated"
		}
	}
	return "not_evaluated"
}

func artifactTypeFromProofRef(ref string) string {
	clean := strings.TrimPrefix(ref, "baseline/")
	clean = strings.TrimPrefix(clean, "current/")
	clean = strings.TrimPrefix(clean, "input/")
	switch {
	case strings.Contains(clean, "trace/trace-summary.json"):
		return "generic_trace_summary"
	case strings.Contains(clean, "corpus-pressure/pressure-summary.json"):
		return "corpus_pressure_summary"
	case strings.Contains(clean, "corpus-pressure/eval-input.json"):
		return "corpus_pressure_eval_input"
	case strings.Contains(clean, "corpus-pressure/trace-summary.json"):
		return "corpus_pressure_trace_summary"
	case strings.Contains(clean, "corpus-pressure-loop/loop-summary.json"):
		return "corpus_pressure_loop_summary"
	case strings.Contains(clean, "corpus-acceptance/benchmark-summary.json"):
		return "corpus_acceptance_benchmark"
	case strings.Contains(clean, "autonomy-readiness/readiness-report.json"):
		return "autonomy_readiness_report"
	case strings.Contains(clean, "link-enrichment/loop-summary.json"):
		return "link_enrichment_loop_summary"
	case strings.Contains(clean, "link-enrichment/comparison/comparison-summary.json"):
		return "link_enrichment_comparison_summary"
	case strings.Contains(clean, "link-enrichment/requests/link-artifact-requests.json"):
		return "link_artifact_requests"
	case strings.Contains(clean, "link-enrichment/posthog/eval-projection.json"):
		return "link_enrichment_eval_projection"
	case strings.Contains(clean, "value-proof/value-summary.json") || strings.HasSuffix(clean, "value-summary.json"):
		return "value_proof_summary"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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

func resolveProofReadbackRef(inputRoot, proofPath, ref string) string {
	cleanRef := filepath.Clean(filepath.FromSlash(ref))
	candidates := []string{
		filepath.Join(filepath.Dir(proofPath), cleanRef),
		filepath.Join(filepath.Dir(filepath.Dir(proofPath)), cleanRef),
	}
	info, err := os.Stat(inputRoot)
	if err == nil && info.IsDir() {
		candidates = append(candidates, filepath.Join(inputRoot, cleanRef))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
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

func improvementState(summary evalreadback.Summary, options Options) string {
	if strings.TrimSpace(options.BaselineRoot) == "" && summary.Comparison == nil {
		return ImprovementBlockedMissingBaseline
	}
	claimStatus := gateStatus(summary, "improvement_claim")
	if claimStatus == "blocked" {
		if summary.ImprovementStatus == "not_comparable" {
			return ImprovementNotComparable
		}
		return ImprovementInconclusive
	}
	if claimStatus == "fail" {
		return ImprovementNotImproved
	}
	switch summary.ImprovementStatus {
	case "improved":
		return ImprovementImproved
	case "regressed", "unchanged":
		return ImprovementNotImproved
	case "not_comparable":
		return ImprovementNotComparable
	case "not_evaluated", "", "blocked":
		return ImprovementBlockedMissingBaseline
	default:
		return ImprovementInconclusive
	}
}

func claimStatuses(summary evalreadback.Summary, options Options) ClaimStatuses {
	safetyStatus := gateStatus(summary, "side_effect_claim")
	privacyStatus := gateStatus(summary, "privacy_safe_readback")
	schemaStatus := gateStatus(summary, "schema_supported")
	statuses := ClaimStatuses{
		Safety:         safetyStatus,
		Improvement:    improvementState(summary, options),
		Generalization: gateStatus(summary, "generalization_claim"),
		DEC64:          gateStatus(summary, "dec64_no_human_claim"),
	}
	switch {
	case privacyStatus == "fail" || safetyStatus == "fail" || schemaStatus == "fail" || hasUnsupportedArtifact(summary):
		statuses.Safety = "fail"
	case privacyStatus != "pass" || safetyStatus != "pass":
		statuses.Safety = SafetyBlocked
	}
	if statuses.Safety == "" {
		statuses.Safety = SafetyBlocked
	}
	if statuses.Generalization == "" {
		statuses.Generalization = GeneralizationBlocked
	}
	if statuses.DEC64 == "" {
		statuses.DEC64 = DEC64Blocked
	}
	if comparisonHasReason(summary, "replay_baseline_blocked") {
		statuses.Generalization = GeneralizationBlocked
		statuses.DEC64 = DEC64Blocked
	}
	return statuses
}

func comparisonHasReason(summary evalreadback.Summary, reason string) bool {
	if summary.Comparison == nil {
		return false
	}
	for _, actual := range summary.Comparison.ReasonCodes {
		if actual == reason {
			return true
		}
	}
	return false
}

func hasUnsupportedArtifact(summary evalreadback.Summary) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			return true
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsupported_schema" {
			return true
		}
	}
	return false
}

func gateStatus(summary evalreadback.Summary, gate string) string {
	for _, claimGate := range summary.ClaimGates {
		if claimGate.Gate == gate {
			return claimGate.Status
		}
	}
	return ""
}

func productGeneralTarget(summary evalreadback.Summary) string {
	if summary.GeneralizationStatus == "generalizable" {
		return "candidate_product_general"
	}
	return "blocked_sample_bound"
}

func rerunInstruction(summary evalreadback.Summary) string {
	if summary.TopImprovementTarget.Code == "missing_proof" && len(summary.RerunInstructions) > 0 && strings.TrimSpace(summary.RerunInstructions[0]) != "" {
		return summary.RerunInstructions[0]
	}
	if summary.Comparison == nil {
		return "rerun the same source command as the next comparable current run, then run mindline eval loop-decision CURRENT --baseline BASELINE --out OUT"
	}
	if summary.ImprovementStatus == "not_comparable" {
		return "rerun baseline and current with the same corpus and command configuration, then rerun mindline eval loop-decision with --baseline"
	}
	if len(summary.RerunInstructions) > 0 && strings.TrimSpace(summary.RerunInstructions[0]) != "" {
		return summary.RerunInstructions[0]
	}
	return "rerun the source command, then run mindline eval loop-decision with --baseline pointing to the previous comparable run"
}

func decisionLimits(summary evalreadback.Summary, options Options) []string {
	limits := []string{
		"decision support only; no automatic implementation or destination write",
		"local artifacts are canonical; hosted telemetry is not required or queried",
	}
	if strings.TrimSpace(options.BaselineRoot) == "" && summary.Comparison == nil {
		limits = append(limits, "improvement claim blocked until comparable baseline/current evidence is supplied")
	}
	if summary.GeneralizationStatus != "generalizable" {
		limits = append(limits, "generalization remains blocked by sample status or missing held-out evidence")
	}
	if gateStatus(summary, "dec64_no_human_claim") != "pass" {
		limits = append(limits, "DEC-64/no-human readiness remains blocked without held-out >=98% proof")
	}
	return limits
}

func safeRefs(summary evalreadback.Summary) []string {
	refs := append([]string{}, summary.SafeArtifactRefs...)
	refs = append(refs, summary.BaselineArtifactRefs...)
	sort.Strings(refs)
	return refs
}

func stableID(prefix string, parts []string) string {
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(prefix + ":" + strings.Join(parts, "|")))
	return strings.Trim(prefix, "-_ ") + "-" + hex.EncodeToString(sum[:])[:12]
}
