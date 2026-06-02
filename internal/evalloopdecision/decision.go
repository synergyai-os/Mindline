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
	decisionDir := filepath.Join(root, DirName)
	if err := evalreadback.ValidateOutputPath(root, decisionDir, options.ProtectedRoots); err != nil {
		return Packet{}, err
	}
	if err := os.MkdirAll(decisionDir, 0o755); err != nil {
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
		return summary, safeExistingReadbackRef(inputRoot, summaryPath), nil
	}
	if proofPath := existingProofPacketPath(inputRoot); proofPath != "" {
		summaryPath := proofReadbackSummaryPath(inputRoot, proofPath)
		if summaryPath == "" {
			return evalreadback.Summary{}, "", errors.New("proof packet input missing readback summary")
		}
		summary, err := evalreadback.LoadSummary(summaryPath)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		summary, err = applyBaseline(summary, options.BaselineRoot)
		if err != nil {
			return evalreadback.Summary{}, "", err
		}
		return summary, safeExistingReadbackRef(inputRoot, summaryPath), nil
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

func applyBaseline(summary evalreadback.Summary, baselineRoot string) (evalreadback.Summary, error) {
	if strings.TrimSpace(baselineRoot) == "" {
		return evalreadback.ApplyBaseline(summary, "")
	}
	if baselineSummaryPath := existingReadbackSummaryPath(baselineRoot); baselineSummaryPath != "" {
		baselineSummary, err := evalreadback.LoadSummary(baselineSummaryPath)
		if err != nil {
			return evalreadback.Summary{}, err
		}
		return evalreadback.ApplyBaselineSummary(summary, baselineSummary), nil
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

func safeExistingReadbackRef(inputRoot, summaryPath string) string {
	info, err := os.Stat(inputRoot)
	if err != nil || !info.IsDir() {
		return "input/" + evalreadback.ReadbackSummaryFile
	}
	rel, err := filepath.Rel(inputRoot, summaryPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "input/" + evalreadback.ReadbackSummaryFile
	}
	return filepath.ToSlash(filepath.Join("input", rel))
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
	statuses := ClaimStatuses{
		Safety:         safetyStatus,
		Improvement:    improvementState(summary, options),
		Generalization: gateStatus(summary, "generalization_claim"),
		DEC64:          gateStatus(summary, "dec64_no_human_claim"),
	}
	switch {
	case privacyStatus == "fail" || safetyStatus == "fail":
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
	return statuses
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
