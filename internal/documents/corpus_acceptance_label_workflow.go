package documents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	corpusAcceptanceLabelNextDirName = "corpus-acceptance-label-next"
	corpusAcceptanceLabelQueueReady  = "label_ready"
	corpusAcceptanceLabelQueueEmpty  = "label_queue_empty"
)

func BuildCorpusAcceptanceLabelNext(labelingPath, recordsPath, outDir string) (CorpusAcceptanceLabelNextSummary, CorpusAcceptanceLabelRecords, error) {
	packet, err := readCorpusAcceptanceLabelingPacket(labelingPath)
	if err != nil {
		return CorpusAcceptanceLabelNextSummary{}, CorpusAcceptanceLabelRecords{}, err
	}
	records, created, err := readOrBootstrapCorpusAcceptanceLabelRecords(recordsPath, packet)
	if err != nil {
		return CorpusAcceptanceLabelNextSummary{}, CorpusAcceptanceLabelRecords{}, err
	}
	if created {
		if err := writeCorpusAcceptanceLabelRecords(recordsPath, records); err != nil {
			return CorpusAcceptanceLabelNextSummary{}, CorpusAcceptanceLabelRecords{}, err
		}
	}
	summary, labelMap := buildCorpusAcceptanceLabelNext(packet, records)
	if err := WriteCorpusAcceptanceLabelNext(outDir, summary, labelMap); err != nil {
		return CorpusAcceptanceLabelNextSummary{}, CorpusAcceptanceLabelRecords{}, err
	}
	return summary, records, nil
}

func RecordCorpusAcceptanceLabel(labelingPath, recordsPath, mapPath string, input CorpusAcceptanceLabelRecordInput) (CorpusAcceptanceLabelRecords, error) {
	packet, err := readCorpusAcceptanceLabelingPacket(labelingPath)
	if err != nil {
		return CorpusAcceptanceLabelRecords{}, err
	}
	records, created, err := readOrBootstrapCorpusAcceptanceLabelRecords(recordsPath, packet)
	if err != nil {
		return CorpusAcceptanceLabelRecords{}, err
	}
	if records.SchemaVersion != CorpusAcceptanceLabelRecordsSchemaVersion {
		return CorpusAcceptanceLabelRecords{}, fmt.Errorf("invalid corpus acceptance label records: unsupported_records_schema")
	}
	labelMap, err := readCorpusAcceptanceLabelNextMap(mapPath)
	if err != nil {
		return CorpusAcceptanceLabelRecords{}, err
	}
	source, candidate, hasCandidate, evidenceNodes, err := resolveCorpusAcceptanceLabelRecordRefs(packet, labelMap, input)
	if err != nil {
		return CorpusAcceptanceLabelRecords{}, err
	}
	if strings.TrimSpace(input.Labeler) != "" {
		records.Provenance.Labeler = input.Labeler
	}
	if input.IndependenceAttestation != "" {
		if input.IndependenceAttestation != corpusAcceptanceIndependentProvenance {
			return CorpusAcceptanceLabelRecords{}, fmt.Errorf("unsupported independence attestation")
		}
		if records.Provenance.Independence != corpusAcceptanceIndependentProvenance && len(records.Records) > 0 {
			return CorpusAcceptanceLabelRecords{}, fmt.Errorf("cannot upgrade generated label records to independent provenance")
		}
		records.Provenance.Independence = input.IndependenceAttestation
	}
	recordID := deterministicCorpusAcceptanceLabelRecordID(input.CaseRef, input.CandidateRef, input.Decision)
	expectedOutcomeID := input.ExpectedOutcomeID
	if expectedOutcomeID == "" && (input.Decision == CorpusAcceptanceLabelExpectedPresent || input.Decision == CorpusAcceptanceLabelExpectedAbsent) {
		expectedOutcomeID = strings.TrimPrefix(recordID, "rec-")
	}
	if err := validateCorpusAcceptanceLabelRecordInput(input, records.Provenance.Labeler, expectedOutcomeID, hasCandidate); err != nil {
		return CorpusAcceptanceLabelRecords{}, err
	}
	record := CorpusAcceptanceLabelRecordItem{
		RecordID:               recordID,
		CaseID:                 source.CaseID,
		Decision:               input.Decision,
		CandidateID:            candidateIDForLabelRecord(candidate, hasCandidate),
		ExpectedOutcomeID:      expectedOutcomeID,
		ExpectedKind:           input.ExpectedKind,
		SourceID:               source.SourceID,
		SourceDocumentID:       sourceDocumentIDForLabel(candidate, hasCandidate),
		RequiredEvidence:       evidenceNodes,
		MinimumConfidenceFloor: input.MinimumConfidenceFloor,
		Notes:                  input.Notes,
	}
	if record.ExpectedKind == "" && hasCandidate {
		record.ExpectedKind = candidate.CandidateKind
	}
	records.Records = upsertCorpusAcceptanceLabelRecord(records.Records, record)
	index := corpusAcceptanceLabelingIndex(packet)
	if blockers := validateCorpusAcceptanceLabelRecords(records, index); hasFatalCorpusAcceptanceLabelRecordBlocker(blockers) {
		return CorpusAcceptanceLabelRecords{}, fmt.Errorf("invalid corpus acceptance label records: %s", strings.Join(blockers, ", "))
	}
	if created || len(records.Records) > 0 {
		if err := writeCorpusAcceptanceLabelRecords(recordsPath, records); err != nil {
			return CorpusAcceptanceLabelRecords{}, err
		}
	}
	return records, nil
}

func buildCorpusAcceptanceLabelNext(packet CorpusAcceptanceLabelingPacket, records CorpusAcceptanceLabelRecords) (CorpusAcceptanceLabelNextSummary, CorpusAcceptanceLabelNextMap) {
	labelMap := CorpusAcceptanceLabelNextMap{
		SchemaVersion:            CorpusAcceptanceLabelNextMapSchemaVersion,
		PacketID:                 packet.PacketID,
		CorpusFingerprint:        packet.CorpusFingerprint,
		CommandConfigFingerprint: packet.CommandConfigFingerprint,
	}
	items := []CorpusAcceptanceLabelNextQueueItem{}
	recorded := map[string]bool{}
	blockers := validateCorpusAcceptanceLabelRecords(records, corpusAcceptanceLabelingIndex(packet))
	if records.SchemaVersion != CorpusAcceptanceLabelRecordsSchemaVersion {
		blockers = append(blockers, "unsupported_records_schema")
	}
	if len(blockers) == 0 {
		recorded = recordedCorpusAcceptanceLabelTargets(packet, records)
	}
	for _, source := range packet.Sources {
		mapCase := CorpusAcceptanceLabelNextMapCase{
			CaseRef:  source.CaseID,
			CaseID:   source.CaseID,
			SourceID: source.SourceID,
		}
		if len(source.Candidates) == 0 {
			items = append(items, CorpusAcceptanceLabelNextQueueItem{
				CaseRef:     source.CaseID,
				SourceState: source.SourceState,
				SourceID:    source.SourceID,
			})
		}
		for idx, candidate := range source.Candidates {
			candidateRef := fmt.Sprintf("candidate-%03d", idx+1)
			evidenceRefs := make([]string, 0, len(candidate.EvidenceNodes))
			for evidenceIdx := range candidate.EvidenceNodes {
				evidenceRefs = append(evidenceRefs, fmt.Sprintf("evidence-%03d", evidenceIdx+1))
			}
			items = append(items, CorpusAcceptanceLabelNextQueueItem{
				CaseRef:            source.CaseID,
				CandidateRef:       candidateRef,
				EvidenceRefs:       evidenceRefs,
				CandidateKind:      candidate.CandidateKind,
				ReviewStatus:       candidate.ReviewStatus,
				ConfidenceBucket:   string(candidate.Confidence),
				SourceState:        source.SourceState,
				CandidateOrdinal:   idx + 1,
				EvidenceOrdinalMax: len(candidate.EvidenceNodes),
				SourceID:           source.SourceID,
				CandidateID:        candidate.CandidateID,
				EvidenceNodeIDs:    append([]string{}, candidate.EvidenceNodes...),
			})
			mapCase.Candidates = append(mapCase.Candidates, CorpusAcceptanceLabelNextMapCandidate{
				CandidateRef:     candidateRef,
				CandidateID:      candidate.CandidateID,
				SourceDocumentID: candidate.SourceDocumentID,
				EvidenceRefs:     evidenceRefs,
				EvidenceNodeIDs:  append([]string{}, candidate.EvidenceNodes...),
			})
		}
		labelMap.Cases = append(labelMap.Cases, mapCase)
	}
	var next *CorpusAcceptanceLabelNextQueueItem
	remaining := 0
	for _, item := range items {
		key := corpusAcceptanceLabelTargetKey(item.CaseRef, item.CandidateRef)
		if recorded[key] {
			continue
		}
		remaining++
		if next == nil {
			copyItem := redactedCorpusAcceptanceLabelNextItem(item)
			next = &copyItem
		}
	}
	queueState := corpusAcceptanceLabelQueueReady
	if remaining == 0 {
		queueState = corpusAcceptanceLabelQueueEmpty
	}
	return CorpusAcceptanceLabelNextSummary{
		SchemaVersion:   CorpusAcceptanceLabelNextSummarySchemaVersion,
		QueueState:      queueState,
		SuiteID:         records.SuiteID,
		SuiteKind:       records.SuiteKind,
		Independence:    records.Provenance.Independence,
		SourceCount:     packet.SourceCount,
		CandidateCount:  packet.CandidateCount,
		RecordedCount:   len(recorded),
		RemainingCount:  remaining,
		NextItem:        next,
		Blockers:        uniqueStrings(blockers),
		MapPath:         filepath.ToSlash(filepath.Join(corpusAcceptanceLabelNextDirName, "label-next-map.json")),
		ReportPath:      filepath.ToSlash(filepath.Join(corpusAcceptanceLabelNextDirName, "label-next-report.md")),
		ClaimBoundaries: corpusAcceptanceLabelNextClaimBoundaries(),
	}, labelMap
}

func WriteCorpusAcceptanceLabelNext(outDir string, summary CorpusAcceptanceLabelNextSummary, labelMap CorpusAcceptanceLabelNextMap) error {
	if strings.TrimSpace(outDir) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing required --out")}
	}
	report := corpusAcceptanceLabelNextMarkdown(summary)
	if containsUnsafeMarker(report) || containsGovernanceID(report) || containsPrivateReportMarker(report) {
		return ArtifactWriteError{Err: fmt.Errorf("corpus acceptance label next report contains private marker")}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	root, err := filepath.Abs(filepath.Join(outDir, corpusAcceptanceLabelNextDirName))
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
	expected := map[string]bool{"label-next-summary.json": true, "label-next-map.json": true, "label-next-report.md": true}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "label-next-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "label-next-map.json", labelMap); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "label-next-report.md", []byte(report)); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func CorpusAcceptanceLabelNextOutputFor(summary CorpusAcceptanceLabelNextSummary) CorpusAcceptanceLabelNextOutputSummary {
	return CorpusAcceptanceLabelNextOutputSummary{
		SchemaVersion:   summary.SchemaVersion,
		QueueState:      summary.QueueState,
		RecordedCount:   summary.RecordedCount,
		RemainingCount:  summary.RemainingCount,
		NextItem:        summary.NextItem,
		Blockers:        append([]string{}, summary.Blockers...),
		MapPath:         summary.MapPath,
		ReportPath:      summary.ReportPath,
		ClaimBoundaries: append([]string{}, summary.ClaimBoundaries...),
	}
}

func readOrBootstrapCorpusAcceptanceLabelRecords(recordsPath string, packet CorpusAcceptanceLabelingPacket) (CorpusAcceptanceLabelRecords, bool, error) {
	if strings.TrimSpace(recordsPath) == "" {
		return CorpusAcceptanceLabelRecords{}, false, fmt.Errorf("missing label records path")
	}
	data, err := os.ReadFile(recordsPath)
	if err == nil {
		var records CorpusAcceptanceLabelRecords
		if err := json.Unmarshal(data, &records); err != nil {
			return CorpusAcceptanceLabelRecords{}, false, fmt.Errorf("decode corpus acceptance label records: %w", err)
		}
		return records, false, nil
	}
	if !os.IsNotExist(err) {
		return CorpusAcceptanceLabelRecords{}, false, fmt.Errorf("read corpus acceptance label records: %w", err)
	}
	return CorpusAcceptanceLabelRecords{
		SchemaVersion: CorpusAcceptanceLabelRecordsSchemaVersion,
		SuiteID:       sanitizeID(packet.PacketID + "-guided-records"),
		SuiteKind:     CorpusAcceptanceSuiteHeldOut,
		Provenance: CorpusAcceptanceProvenance{
			Labeler:      "human_labeler_required",
			Independence: corpusAcceptanceGeneratedProvenance,
		},
		MinEvalCount: CorpusAcceptanceDEC64MinEvalCount,
		CoverageRequirements: CorpusAcceptanceCoverage{
			MinSourceCount: packet.SourceCount,
		},
		Records: []CorpusAcceptanceLabelRecordItem{},
	}, true, nil
}

func writeCorpusAcceptanceLabelRecords(recordsPath string, records CorpusAcceptanceLabelRecords) error {
	if strings.TrimSpace(recordsPath) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing label records path")}
	}
	root, err := filepath.Abs(filepath.Dir(recordsPath))
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ArtifactWriteError{Err: err}
	}
	target, err := filepath.Abs(recordsPath)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if !isInside(root, target) || root == target {
		return ArtifactWriteError{Err: fmt.Errorf("records path escaped output directory")}
	}
	if err := rejectIfSymlink(target); err != nil {
		return ArtifactWriteError{Err: err}
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(root, ".label-records-*.tmp")
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return ArtifactWriteError{Err: err}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return ArtifactWriteError{Err: err}
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func readCorpusAcceptanceLabelNextMap(path string) (CorpusAcceptanceLabelNextMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CorpusAcceptanceLabelNextMap{}, fmt.Errorf("read corpus acceptance label next map: %w", err)
	}
	var labelMap CorpusAcceptanceLabelNextMap
	if err := json.Unmarshal(data, &labelMap); err != nil {
		return CorpusAcceptanceLabelNextMap{}, fmt.Errorf("decode corpus acceptance label next map: %w", err)
	}
	if labelMap.SchemaVersion != CorpusAcceptanceLabelNextMapSchemaVersion {
		return CorpusAcceptanceLabelNextMap{}, fmt.Errorf("unsupported label next map schema")
	}
	return labelMap, nil
}

func resolveCorpusAcceptanceLabelRecordRefs(packet CorpusAcceptanceLabelingPacket, labelMap CorpusAcceptanceLabelNextMap, input CorpusAcceptanceLabelRecordInput) (*CorpusAcceptanceLabelingSource, *CorpusAcceptanceLabelingCandidateReference, bool, []string, error) {
	if labelMap.PacketID != packet.PacketID || labelMap.CorpusFingerprint != packet.CorpusFingerprint || labelMap.CommandConfigFingerprint != packet.CommandConfigFingerprint {
		return nil, nil, false, nil, fmt.Errorf("label next map does not match current packet")
	}
	index := corpusAcceptanceLabelingIndex(packet)
	for _, mapCase := range labelMap.Cases {
		if mapCase.CaseRef != input.CaseRef {
			continue
		}
		source := index.sourcesByCase[mapCase.CaseID]
		if source == nil || source.SourceID != mapCase.SourceID {
			return nil, nil, false, nil, fmt.Errorf("label next map case does not match current packet")
		}
		if input.CandidateRef == "" {
			return source, nil, false, nil, nil
		}
		for _, mapCandidate := range mapCase.Candidates {
			if mapCandidate.CandidateRef != input.CandidateRef {
				continue
			}
			candidate, evidenceRefs, ok := corpusAcceptanceLabelCandidateForRef(source, input.CandidateRef)
			if !ok ||
				candidate.CandidateID != mapCandidate.CandidateID ||
				candidate.SourceDocumentID != mapCandidate.SourceDocumentID ||
				!sameStrings(evidenceRefs, mapCandidate.EvidenceRefs) ||
				!sameStrings(candidate.EvidenceNodes, mapCandidate.EvidenceNodeIDs) {
				return nil, nil, false, nil, fmt.Errorf("label next map candidate does not match current packet")
			}
			evidenceNodes, err := resolveCorpusAcceptanceLabelEvidenceRefs(evidenceRefs, candidate.EvidenceNodes, input.RequiredEvidenceRefs)
			if err != nil {
				return nil, nil, false, nil, err
			}
			return source, candidate, true, evidenceNodes, nil
		}
		return nil, nil, false, nil, fmt.Errorf("unknown candidate ref")
	}
	return nil, nil, false, nil, fmt.Errorf("unknown case ref")
}

func corpusAcceptanceLabelCandidateForRef(source *CorpusAcceptanceLabelingSource, candidateRef string) (*CorpusAcceptanceLabelingCandidateReference, []string, bool) {
	for idx := range source.Candidates {
		if fmt.Sprintf("candidate-%03d", idx+1) != candidateRef {
			continue
		}
		candidate := &source.Candidates[idx]
		evidenceRefs := make([]string, 0, len(candidate.EvidenceNodes))
		for evidenceIdx := range candidate.EvidenceNodes {
			evidenceRefs = append(evidenceRefs, fmt.Sprintf("evidence-%03d", evidenceIdx+1))
		}
		return candidate, evidenceRefs, true
	}
	return nil, nil, false
}

func resolveCorpusAcceptanceLabelEvidenceRefs(evidenceRefs, evidenceNodeIDs, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	byRef := map[string]string{}
	for idx, ref := range evidenceRefs {
		if idx < len(evidenceNodeIDs) {
			byRef[ref] = evidenceNodeIDs[idx]
		}
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		node := byRef[ref]
		if node == "" {
			return nil, fmt.Errorf("unknown evidence ref")
		}
		out = append(out, node)
	}
	return out, nil
}

func validateCorpusAcceptanceLabelRecordInput(input CorpusAcceptanceLabelRecordInput, effectiveLabeler, expectedOutcomeID string, hasCandidate bool) error {
	if strings.TrimSpace(input.CaseRef) == "" {
		return fmt.Errorf("missing case ref")
	}
	if !validCorpusAcceptanceLabelDecision(input.Decision) {
		return fmt.Errorf("unsupported decision")
	}
	if strings.TrimSpace(effectiveLabeler) == "" || sanitizeID(effectiveLabeler) != effectiveLabeler || containsUnsafeMarker(effectiveLabeler) || containsGovernanceID(effectiveLabeler) {
		return fmt.Errorf("unsafe labeler")
	}
	if (input.Decision == CorpusAcceptanceLabelExpectedPresent || input.Decision == CorpusAcceptanceLabelExpectedAbsent) && strings.TrimSpace(expectedOutcomeID) == "" {
		return fmt.Errorf("missing expected outcome id")
	}
	if expectedOutcomeID != "" && sanitizeID(expectedOutcomeID) != expectedOutcomeID {
		return fmt.Errorf("unsafe expected outcome id")
	}
	if input.Decision == CorpusAcceptanceLabelExpectedPresent && !hasCandidate {
		return fmt.Errorf("missing candidate")
	}
	if input.MinimumConfidenceFloor != "" && !validConfidence(input.MinimumConfidenceFloor) {
		return fmt.Errorf("unsupported minimum confidence floor")
	}
	if input.Decision == CorpusAcceptanceLabelExpectedAbsent && !hasCandidate {
		if input.ExpectedKind == "" {
			return fmt.Errorf("missing expected kind")
		}
		if input.MinimumConfidenceFloor == "" {
			return fmt.Errorf("missing minimum confidence floor")
		}
	}
	if containsUnsafeMarker(input.Notes) || containsGovernanceID(input.Notes) || containsPrivateReportMarker(input.Notes) {
		return fmt.Errorf("unsafe note")
	}
	return nil
}

func upsertCorpusAcceptanceLabelRecord(records []CorpusAcceptanceLabelRecordItem, record CorpusAcceptanceLabelRecordItem) []CorpusAcceptanceLabelRecordItem {
	out := append([]CorpusAcceptanceLabelRecordItem{}, records...)
	for idx := range out {
		if out[idx].RecordID == record.RecordID {
			out[idx] = record
			return out
		}
	}
	out = append(out, record)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].RecordID < out[j].RecordID
	})
	return out
}

func recordedCorpusAcceptanceLabelTargets(packet CorpusAcceptanceLabelingPacket, records CorpusAcceptanceLabelRecords) map[string]bool {
	out := map[string]bool{}
	candidateRefs := map[string]string{}
	for _, source := range packet.Sources {
		for idx, candidate := range source.Candidates {
			candidateRefs[source.CaseID+"\x00"+candidate.CandidateID] = fmt.Sprintf("candidate-%03d", idx+1)
		}
	}
	for _, record := range records.Records {
		if record.Decision == CorpusAcceptanceLabelExpectedPresent ||
			record.Decision == CorpusAcceptanceLabelExpectedAbsent ||
			record.Decision == CorpusAcceptanceLabelUncertain ||
			record.Decision == CorpusAcceptanceLabelAbstain {
			candidateRef := ""
			if strings.TrimSpace(record.CandidateID) != "" {
				candidateRef = candidateRefs[record.CaseID+"\x00"+record.CandidateID]
			}
			out[corpusAcceptanceLabelTargetKey(record.CaseID, candidateRef)] = true
		}
	}
	return out
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func deterministicCorpusAcceptanceLabelRecordID(caseRef, candidateRef string, decision CorpusAcceptanceLabelDecision) string {
	parts := []string{"rec", sanitizeID(caseRef)}
	if strings.TrimSpace(candidateRef) != "" {
		parts = append(parts, sanitizeID(candidateRef))
	}
	parts = append(parts, strings.ReplaceAll(sanitizeID(string(decision)), "_", "-"))
	return strings.Join(parts, "-")
}

func corpusAcceptanceLabelTargetKey(caseRef, candidateRef string) string {
	return caseRef + "\x00" + candidateRef
}

func redactedCorpusAcceptanceLabelNextItem(item CorpusAcceptanceLabelNextQueueItem) CorpusAcceptanceLabelNextQueueItem {
	item.SourceID = ""
	item.CandidateID = ""
	item.EvidenceNodeIDs = nil
	return item
}

func candidateIDForLabelRecord(candidate *CorpusAcceptanceLabelingCandidateReference, hasCandidate bool) string {
	if hasCandidate {
		return candidate.CandidateID
	}
	return ""
}

func corpusAcceptanceLabelNextClaimBoundaries() []string {
	return []string{
		"label_next_is_not_held_out_accuracy_proof",
		"generated_records_are_not_independent",
		"generalization_not_claimed",
		"formal_autonomy_threshold_not_claimed",
		"destination_write_readiness_not_claimed",
		"no_human_operation_not_claimed",
	}
}

func corpusAcceptanceLabelNextMarkdown(summary CorpusAcceptanceLabelNextSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus acceptance label next\n\n")
	b.WriteString("This local report guides human label recording. It is not held-out accuracy proof, generalization proof, formal autonomy-threshold proof, destination-write readiness, or no-human operation.\n\n")
	b.WriteString(fmt.Sprintf("- Queue state: %s\n", summary.QueueState))
	b.WriteString(fmt.Sprintf("- Recorded labels: %d\n", summary.RecordedCount))
	b.WriteString(fmt.Sprintf("- Remaining labels: %d\n", summary.RemainingCount))
	b.WriteString(fmt.Sprintf("- Independence: %s\n\n", summary.Independence))
	if summary.NextItem != nil {
		b.WriteString("## Next item\n\n")
		b.WriteString(fmt.Sprintf("- Case ref: %s\n", summary.NextItem.CaseRef))
		if summary.NextItem.CandidateRef != "" {
			b.WriteString(fmt.Sprintf("- Candidate ref: %s\n", summary.NextItem.CandidateRef))
			b.WriteString(fmt.Sprintf("- Candidate kind: %s\n", summary.NextItem.CandidateKind))
			b.WriteString(fmt.Sprintf("- Review status: %s\n", summary.NextItem.ReviewStatus))
			b.WriteString(fmt.Sprintf("- Confidence bucket: %s\n", summary.NextItem.ConfidenceBucket))
			if len(summary.NextItem.EvidenceRefs) > 0 {
				b.WriteString("- Evidence refs: " + strings.Join(summary.NextItem.EvidenceRefs, ", ") + "\n")
			}
		}
		b.WriteString("\n")
	}
	if len(summary.Blockers) > 0 {
		b.WriteString("## Blockers\n\n")
		for _, blocker := range summary.Blockers {
			b.WriteString("- " + blocker + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Claim boundaries\n\n")
	for _, boundary := range summary.ClaimBoundaries {
		b.WriteString("- " + boundary + "\n")
	}
	return b.String()
}
