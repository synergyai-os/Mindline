package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteCorpusAcceptanceLabelingPacket(outDir string, packet CorpusAcceptanceLabelingPacket, template CorpusAcceptanceAnswerKey) error {
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
	expected := map[string]bool{"labeling-packet.json": true, "answer-key-template.json": true, "labeling-report.md": true}
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
	return nil
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
