package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteCorpusAcceptanceLabelingPacket(outDir string, packet CorpusAcceptanceLabelingPacket, template CorpusAcceptanceAnswerKey) error {
	return WriteCorpusAcceptanceLabelingPacketWithSeed(outDir, packet, template, nil, nil)
}

func WriteCorpusAcceptanceLabelingPacketWithSeed(outDir string, packet CorpusAcceptanceLabelingPacket, template CorpusAcceptanceAnswerKey, seed *CorpusAcceptanceLabelSeedSummary, privateMap *CorpusAcceptanceLabelSeedPrivateMap) error {
	if strings.TrimSpace(outDir) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing required --out")}
	}
	if err := ValidateCorpusAcceptanceLabelingPacket(packet); err != nil {
		return ArtifactWriteError{Err: err}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	root, err := filepath.Abs(filepath.Join(outDir, corpusAcceptanceLabelingDirName))
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectIfSymlink(root); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ArtifactWriteError{Err: err}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if seed != nil {
		if err := rejectCorpusAcceptanceLabelSeedDurableOutput(realRoot); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	expected := map[string]bool{"labeling-packet.json": true, "answer-key-template.json": true, "labeling-report.md": true}
	if seed != nil {
		expected["seed-summary.json"] = true
		expected["seed-report.md"] = true
		expected["seed-private-map.json"] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "labeling-packet.json", packet); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "answer-key-template.json", template); err != nil {
		return ArtifactWriteError{Err: err}
	}
	report := corpusAcceptanceLabelingMarkdown(packet)
	if containsUnsafeMarker(report) {
		return ArtifactWriteError{Err: fmt.Errorf("corpus acceptance labeling report contains unsafe marker")}
	}
	if containsGovernanceID(report) {
		return ArtifactWriteError{Err: fmt.Errorf("corpus acceptance labeling report contains governance marker")}
	}
	if containsPrivateReportMarker(report) {
		return ArtifactWriteError{Err: fmt.Errorf("corpus acceptance labeling report contains private marker")}
	}
	if err := writeFile(realRoot, "labeling-report.md", []byte(report)); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if seed != nil {
		if privateMap == nil {
			return ArtifactWriteError{Err: fmt.Errorf("missing seed private map")}
		}
		if err := ValidateCorpusAcceptanceLabelSeedSummary(*seed); err != nil {
			return ArtifactWriteError{Err: err}
		}
		if err := ValidateCorpusAcceptanceLabelSeedPrivateMap(*privateMap, packet); err != nil {
			return ArtifactWriteError{Err: err}
		}
		seedReport := corpusAcceptanceLabelSeedMarkdown(*seed)
		if containsUnsafeMarker(seedReport) || containsGovernanceID(seedReport) || containsPrivateReportMarker(seedReport) {
			return ArtifactWriteError{Err: fmt.Errorf("corpus acceptance label seed report contains private marker")}
		}
		if err := writeJSON(realRoot, "seed-summary.json", seed); err != nil {
			return ArtifactWriteError{Err: err}
		}
		if err := writeFile(realRoot, "seed-report.md", []byte(seedReport)); err != nil {
			return ArtifactWriteError{Err: err}
		}
		if err := writeJSON(realRoot, "seed-private-map.json", privateMap); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	return nil
}

func rejectCorpusAcceptanceLabelSeedDurableOutput(realRoot string) error {
	clean := filepath.Clean(realRoot)
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == ".productbrain" {
			return fmt.Errorf("seed private map output must not be under durable Product Brain artifacts")
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	absWD, err := filepath.Abs(wd)
	if err != nil {
		return err
	}
	realWD, err := filepath.EvalSymlinks(absWD)
	if err != nil {
		return err
	}
	if sameOrChildPath(realWD, clean) {
		return fmt.Errorf("seed private map output must be outside the current workspace")
	}
	return nil
}

func sameOrChildPath(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func ValidateCorpusAcceptanceLabelingPacket(packet CorpusAcceptanceLabelingPacket) error {
	if packet.SchemaVersion != CorpusAcceptanceLabelingPacketSchemaVersion {
		return fmt.Errorf("unsupported corpus acceptance labeling packet schema version: %s", packet.SchemaVersion)
	}
	if strings.TrimSpace(packet.PacketID) == "" || sanitizeID(packet.PacketID) != packet.PacketID {
		return fmt.Errorf("unsafe packet id")
	}
	if packet.LabelingStatus != corpusAcceptanceLabelingRequired || packet.HeldOutReady {
		return fmt.Errorf("labeling packet must require labeling and remain held-out not ready")
	}
	if packet.SourceCount != len(packet.Sources) {
		return fmt.Errorf("source count mismatch")
	}
	body := strings.Join([]string{
		packet.PacketID,
		packet.CorpusID,
		packet.CorpusFingerprint,
		packet.CommandConfigFingerprint,
		packet.PressureReplayFingerprint,
		packet.LabelingStatus,
		packet.LabelingPacketPath,
		packet.AnswerKeyTemplatePath,
		packet.ReportPath,
		strings.Join(packet.ClaimBoundaries, "\n"),
		strings.Join(packet.Instructions, "\n"),
	}, "\n")
	for _, source := range packet.Sources {
		if strings.TrimSpace(source.CaseID) == "" || sanitizeID(source.CaseID) != source.CaseID {
			return fmt.Errorf("unsafe case id")
		}
		if strings.TrimSpace(source.SourceID) == "" || sanitizeID(source.SourceID) != source.SourceID {
			return fmt.Errorf("unsafe source id")
		}
		if strings.TrimSpace(source.SourcePath) != "" && unsafeRelativeArtifactPath(source.SourcePath) {
			return fmt.Errorf("unsafe source path")
		}
		if strings.TrimSpace(source.SemanticRunDir) != "" && unsafeRelativeArtifactPath(source.SemanticRunDir) {
			return fmt.Errorf("unsafe semantic run dir")
		}
		body += "\n" + source.CaseID + "\n" + source.SourceID + "\n" + source.SourceContentHash + "\n" + source.SourcePath + "\n" + source.SemanticRunDir
		for _, candidate := range source.Candidates {
			body += "\n" + candidate.CandidateID + "\n" + candidate.SourceDocumentID
			body += "\n" + strings.Join(candidate.EvidenceNodes, "\n")
		}
	}
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("corpus acceptance labeling packet contains private marker")
	}
	return nil
}

func ValidateCorpusAcceptanceLabelSeedSummary(summary CorpusAcceptanceLabelSeedSummary) error {
	if summary.SchemaVersion != CorpusAcceptanceLabelSeedSummarySchemaVersion {
		return fmt.Errorf("unsupported corpus acceptance label seed summary schema version: %s", summary.SchemaVersion)
	}
	if summary.SelectionVersion != corpusAcceptanceLabelSeedSelectionVersion || !summary.SeedMode {
		return fmt.Errorf("invalid corpus acceptance label seed summary")
	}
	if summary.MaxCases <= 0 || summary.SelectedCaseCount != len(summary.SelectedCases) {
		return fmt.Errorf("seed summary count mismatch")
	}
	body := strings.Join([]string{
		summary.SelectionVersion,
		summary.CorpusFingerprint,
		summary.CommandConfigFingerprint,
		summary.PressureReplayFingerprint,
		summary.LabelingPacketPath,
		summary.PrivateMapPath,
		summary.ReportPath,
		strings.Join(summary.UnmetCoverage, "\n"),
		strings.Join(summary.ClaimBoundaries, "\n"),
	}, "\n")
	for _, selected := range summary.SelectedCases {
		if strings.TrimSpace(selected.CaseRef) == "" || sanitizeID(selected.CaseRef) != selected.CaseRef {
			return fmt.Errorf("unsafe seed case ref")
		}
		if strings.TrimSpace(selected.SourceGroupRef) == "" || sanitizeID(selected.SourceGroupRef) != selected.SourceGroupRef {
			return fmt.Errorf("unsafe seed source group ref")
		}
		body += "\n" + selected.CaseRef + "\n" + selected.CandidateRef + "\n" + selected.SourceGroupRef + "\n" + strings.Join(selected.RationaleBuckets, "\n")
	}
	if containsUnsafeMarker(body) || containsGovernanceID(body) || containsPrivateReportMarker(body) {
		return fmt.Errorf("corpus acceptance label seed summary contains private marker")
	}
	return nil
}

func ValidateCorpusAcceptanceLabelSeedPrivateMap(privateMap CorpusAcceptanceLabelSeedPrivateMap, packet CorpusAcceptanceLabelingPacket) error {
	if privateMap.SchemaVersion != CorpusAcceptanceLabelSeedPrivateMapSchemaVersion {
		return fmt.Errorf("unsupported corpus acceptance label seed private map schema version: %s", privateMap.SchemaVersion)
	}
	if privateMap.PacketID != packet.PacketID || privateMap.CorpusFingerprint != packet.CorpusFingerprint || privateMap.CommandConfigFingerprint != packet.CommandConfigFingerprint {
		return fmt.Errorf("seed private map fingerprint mismatch")
	}
	if len(privateMap.Cases) != len(packet.Sources) {
		return fmt.Errorf("seed private map case count mismatch")
	}
	for idx, mapCase := range privateMap.Cases {
		source := packet.Sources[idx]
		if mapCase.CaseRef != source.CaseID {
			return fmt.Errorf("seed private map case ref mismatch")
		}
		if strings.TrimSpace(mapCase.OriginalSourceID) == "" {
			return fmt.Errorf("seed private map missing original source id")
		}
		if len(mapCase.Candidates) != len(source.Candidates) {
			return fmt.Errorf("seed private map candidate count mismatch")
		}
		for candidateIdx, mapCandidate := range mapCase.Candidates {
			candidate := source.Candidates[candidateIdx]
			if mapCandidate.CandidateRef != candidate.CandidateID {
				return fmt.Errorf("seed private map candidate ref mismatch")
			}
			if strings.TrimSpace(mapCandidate.OriginalCandidateID) == "" {
				return fmt.Errorf("seed private map missing original candidate id")
			}
		}
	}
	return nil
}

func corpusAcceptanceLabelingMarkdown(packet CorpusAcceptanceLabelingPacket) string {
	var b strings.Builder
	b.WriteString("# Corpus acceptance labeling packet\n\n")
	b.WriteString("This packet is ready for human labeling. It is not held-out proof, formal autonomy-threshold proof, generalization proof, or destination-write readiness.\n\n")
	b.WriteString(fmt.Sprintf("- Corpus fingerprint: %s\n", packet.CorpusFingerprint))
	b.WriteString(fmt.Sprintf("- Command config fingerprint: %s\n", packet.CommandConfigFingerprint))
	b.WriteString(fmt.Sprintf("- Pressure replay fingerprint: %s\n", packet.PressureReplayFingerprint))
	b.WriteString(fmt.Sprintf("- Sources needing labels: %d\n", packet.SourceCount))
	b.WriteString(fmt.Sprintf("- Candidate references: %d\n", packet.CandidateCount))
	b.WriteString(fmt.Sprintf("- Relation candidates for labeling coverage: %d\n", packet.RelationCoverage.RelationCount))
	b.WriteString(fmt.Sprintf("- Held-out ready: %t\n\n", packet.HeldOutReady))
	b.WriteString("## Guardrails\n\n")
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", packet.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", packet.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n", packet.Guardrails.TolariaWrites))
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", packet.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n", packet.Guardrails.HostedTelemetryExports))
	b.WriteString(fmt.Sprintf("- Auto accepts: %d\n", packet.Guardrails.AutoAccepts))
	b.WriteString(fmt.Sprintf("- No-human claims: %d\n\n", packet.Guardrails.NoHumanClaims))
	b.WriteString("## Cases\n\n")
	for _, source := range packet.Sources {
		b.WriteString(fmt.Sprintf("- `%s`: state=%s candidates=%d", source.CaseID, source.SourceState, source.CandidateCount))
		if len(source.CandidateKinds) > 0 {
			b.WriteString(" kinds=" + formatCandidateKindCounts(source.CandidateKinds))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Claim boundaries\n\n")
	for _, boundary := range packet.ClaimBoundaries {
		b.WriteString("- " + boundary + "\n")
	}
	return b.String()
}

func corpusAcceptanceLabelSeedMarkdown(summary CorpusAcceptanceLabelSeedSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus acceptance label seed\n\n")
	b.WriteString("This seed is a bounded human labeling queue. It is not an answer key, held-out accuracy proof, generalization proof, formal autonomy-threshold proof, destination-write readiness, or no-human operation.\n\n")
	b.WriteString(fmt.Sprintf("- Selection version: %s\n", summary.SelectionVersion))
	b.WriteString(fmt.Sprintf("- Max cases: %d\n", summary.MaxCases))
	b.WriteString(fmt.Sprintf("- Selected cases: %d\n", summary.SelectedCaseCount))
	b.WriteString(fmt.Sprintf("- Candidate cases: %d\n", summary.SelectedCandidateCount))
	b.WriteString(fmt.Sprintf("- Eligible cases: %d\n", summary.EligibleCaseCount))
	b.WriteString(fmt.Sprintf("- Unselected cases: %d\n\n", summary.UnselectedCaseCount))
	b.WriteString("## Coverage\n\n")
	b.WriteString(fmt.Sprintf("- Source groups: %d\n", len(summary.Coverage.SourceGroupCounts)))
	b.WriteString(fmt.Sprintf("- Candidate kinds: %s\n", formatCandidateKindCounts(summary.Coverage.CandidateKindCounts)))
	b.WriteString(fmt.Sprintf("- Confidence bands: %s\n", formatSeedStringCounts(summary.Coverage.ConfidenceCounts)))
	b.WriteString(fmt.Sprintf("- Review states: %s\n", formatReviewStatusCounts(summary.Coverage.ReviewStatusCounts)))
	b.WriteString(fmt.Sprintf("- Rationale buckets: %s\n\n", formatSeedStringCounts(summary.Coverage.RationaleCounts)))
	if len(summary.UnmetCoverage) > 0 {
		b.WriteString("## Unmet Coverage\n\n")
		for _, blocker := range summary.UnmetCoverage {
			b.WriteString("- " + blocker + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Selected Cases\n\n")
	for _, selected := range summary.SelectedCases {
		b.WriteString(fmt.Sprintf("- `%s`", selected.CaseRef))
		if selected.CandidateRef != "" {
			b.WriteString(fmt.Sprintf(" `%s`", selected.CandidateRef))
		}
		b.WriteString(": " + strings.Join(selected.RationaleBuckets, ",") + "\n")
	}
	b.WriteString("\n## Guardrails\n\n")
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", summary.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", summary.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n", summary.Guardrails.TolariaWrites))
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", summary.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n", summary.Guardrails.HostedTelemetryExports))
	b.WriteString(fmt.Sprintf("- Auto accepts: %d\n", summary.Guardrails.AutoAccepts))
	b.WriteString(fmt.Sprintf("- No-human claims: %d\n\n", summary.Guardrails.NoHumanClaims))
	b.WriteString("## Claim boundaries\n\n")
	for _, boundary := range summary.ClaimBoundaries {
		b.WriteString("- " + boundary + "\n")
	}
	return b.String()
}

func formatSeedStringCounts(values map[string]int) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
	}
	return strings.Join(parts, ",")
}

func formatReviewStatusCounts(values map[ReviewStatus]int) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]ReviewStatus, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
	}
	return strings.Join(parts, ",")
}

func formatCandidateKindCounts(values map[SemanticCandidateKind]int) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]SemanticCandidateKind, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
	}
	return strings.Join(parts, ",")
}

func unsafeRelativeArtifactPath(value string) bool {
	if strings.TrimSpace(value) == "" || filepath.IsAbs(value) {
		return true
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	return clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func containsPrivateReportMarker(value string) bool {
	body := strings.ToLower(value)
	return strings.Contains(body, "slack-") ||
		strings.Contains(body, "slack.com") ||
		strings.Contains(body, "/private/") ||
		strings.Contains(body, "/users/")
}
