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
	corpusAcceptanceLabelRecordingDirName = "corpus-acceptance-label-recording"
	corpusAcceptanceLabelsRecorded        = "labels_recorded"
)

func BuildCorpusAcceptanceLabelRecording(labelingPath, recordsPath, outDir string) (CorpusAcceptanceLabelRecordingSummary, CorpusAcceptanceAnswerKey, error) {
	packet, err := readCorpusAcceptanceLabelingPacket(labelingPath)
	if err != nil {
		return CorpusAcceptanceLabelRecordingSummary{}, CorpusAcceptanceAnswerKey{}, err
	}
	records, err := readCorpusAcceptanceLabelRecords(recordsPath)
	if err != nil {
		return CorpusAcceptanceLabelRecordingSummary{}, CorpusAcceptanceAnswerKey{}, err
	}
	summary, answerKey, err := buildCorpusAcceptanceLabelRecording(packet, records)
	if err != nil {
		return CorpusAcceptanceLabelRecordingSummary{}, CorpusAcceptanceAnswerKey{}, err
	}
	if err := WriteCorpusAcceptanceLabelRecording(outDir, summary, answerKey); err != nil {
		return CorpusAcceptanceLabelRecordingSummary{}, CorpusAcceptanceAnswerKey{}, err
	}
	return summary, answerKey, nil
}

func readCorpusAcceptanceLabelingPacket(path string) (CorpusAcceptanceLabelingPacket, error) {
	if strings.TrimSpace(path) == "" {
		return CorpusAcceptanceLabelingPacket{}, fmt.Errorf("missing corpus acceptance labeling path")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return CorpusAcceptanceLabelingPacket{}, err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return CorpusAcceptanceLabelingPacket{}, err
	}
	packetPath := filepath.Join(root, corpusAcceptanceLabelingDirName, "labeling-packet.json")
	if filepath.Base(root) == corpusAcceptanceLabelingDirName {
		packetPath = filepath.Join(root, "labeling-packet.json")
	}
	if err := rejectIfSymlink(packetPath); err != nil {
		return CorpusAcceptanceLabelingPacket{}, err
	}
	data, err := os.ReadFile(packetPath)
	if err != nil {
		return CorpusAcceptanceLabelingPacket{}, fmt.Errorf("read corpus acceptance labeling packet: %w", err)
	}
	var packet CorpusAcceptanceLabelingPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return CorpusAcceptanceLabelingPacket{}, fmt.Errorf("decode corpus acceptance labeling packet: %w", err)
	}
	if err := ValidateCorpusAcceptanceLabelingPacket(packet); err != nil {
		return CorpusAcceptanceLabelingPacket{}, err
	}
	return packet, nil
}

func readCorpusAcceptanceLabelRecords(path string) (CorpusAcceptanceLabelRecords, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CorpusAcceptanceLabelRecords{}, fmt.Errorf("read corpus acceptance label records: %w", err)
	}
	var records CorpusAcceptanceLabelRecords
	if err := json.Unmarshal(data, &records); err != nil {
		return CorpusAcceptanceLabelRecords{}, fmt.Errorf("decode corpus acceptance label records: %w", err)
	}
	return records, nil
}

func buildCorpusAcceptanceLabelRecording(packet CorpusAcceptanceLabelingPacket, records CorpusAcceptanceLabelRecords) (CorpusAcceptanceLabelRecordingSummary, CorpusAcceptanceAnswerKey, error) {
	index := corpusAcceptanceLabelingIndex(packet)
	answerKey := CorpusAcceptanceAnswerKey{
		SchemaVersion:            CorpusAcceptanceAnswerKeySchemaVersion,
		SuiteID:                  records.SuiteID,
		SuiteKind:                records.SuiteKind,
		Provenance:               records.Provenance,
		CorpusID:                 packet.CorpusID,
		CorpusFingerprint:        packet.CorpusFingerprint,
		CommandConfigFingerprint: packet.CommandConfigFingerprint,
		MinEvalCount:             records.MinEvalCount,
		CoverageRequirements:     sanitizedCorpusAcceptanceCoverage(records.CoverageRequirements),
		Sources:                  []CorpusAcceptanceAnswerKeySource{},
	}
	sourceKeys := map[string]*CorpusAcceptanceAnswerKeySource{}
	summary := CorpusAcceptanceLabelRecordingSummary{
		SchemaVersion:            CorpusAcceptanceLabelRecordingSummarySchemaVersion,
		SuiteID:                  records.SuiteID,
		SuiteKind:                records.SuiteKind,
		LabelingStatus:           corpusAcceptanceLabelsRecorded,
		CorpusID:                 packet.CorpusID,
		CorpusFingerprint:        packet.CorpusFingerprint,
		CommandConfigFingerprint: packet.CommandConfigFingerprint,
		RecordCount:              len(records.Records),
		SourceCount:              packet.SourceCount,
		Independence:             records.Provenance.Independence,
		Guardrails:               packet.Guardrails,
		AnswerKeyPath:            filepath.ToSlash(filepath.Join(corpusAcceptanceLabelRecordingDirName, "answer-key.json")),
		ReportPath:               filepath.ToSlash(filepath.Join(corpusAcceptanceLabelRecordingDirName, "label-recording-report.md")),
		ClaimBoundaries: []string{
			"label_records_do_not_certify_true_independence",
			"held_out_accuracy_not_claimed",
			"generalization_not_claimed",
			"formal_autonomy_threshold_not_claimed",
			"destination_write_readiness_not_claimed",
			"no_human_operation_not_claimed",
		},
	}
	blockers := validateCorpusAcceptanceLabelRecords(records, index)
	if records.SchemaVersion != CorpusAcceptanceLabelRecordsSchemaVersion {
		blockers = append(blockers, "unsupported_records_schema")
	}
	if hasFatalCorpusAcceptanceLabelRecordBlocker(blockers) {
		return CorpusAcceptanceLabelRecordingSummary{}, CorpusAcceptanceAnswerKey{}, fmt.Errorf("invalid corpus acceptance label records: %s", strings.Join(uniqueStrings(blockers), ", "))
	}
	seenCasesWithOutcomes := map[string]bool{}
	for _, record := range records.Records {
		source, candidate, hasCandidate := index.lookup(record.CaseID, record.CandidateID)
		switch record.Decision {
		case CorpusAcceptanceLabelUncertain:
			summary.UncertainCount++
			continue
		case CorpusAcceptanceLabelAbstain:
			summary.AbstainCount++
			continue
		case CorpusAcceptanceLabelExpectedPresent, CorpusAcceptanceLabelExpectedAbsent:
		default:
			continue
		}
		if source == nil {
			continue
		}
		sourceKey := sourceKeys[source.SourceID]
		if sourceKey == nil {
			sourceKey = &CorpusAcceptanceAnswerKeySource{
				SourceID:         source.SourceID,
				SourceDocumentID: firstNonBlankCorpusString(record.SourceDocumentID, sourceDocumentIDForLabel(candidate, hasCandidate)),
				ExpectedOutcomes: []SemanticExpectedOutcome{},
			}
			sourceKeys[source.SourceID] = sourceKey
		}
		outcome := labelRecordOutcome(record, candidate, hasCandidate)
		sourceKey.ExpectedOutcomes = append(sourceKey.ExpectedOutcomes, outcome)
		seenCasesWithOutcomes[source.CaseID] = true
		summary.EvalCount++
		if outcome.ExpectedState == ExpectedOutcomePresent {
			summary.ExpectedPresentCount++
		}
		if outcome.ExpectedState == ExpectedOutcomeAbsent {
			summary.ExpectedAbsentCount++
		}
	}
	for sourceID, sourceKey := range sourceKeys {
		sort.SliceStable(sourceKey.ExpectedOutcomes, func(i, j int) bool {
			return sourceKey.ExpectedOutcomes[i].ExpectedOutcomeID < sourceKey.ExpectedOutcomes[j].ExpectedOutcomeID
		})
		sourceKeys[sourceID] = sourceKey
		answerKey.Sources = append(answerKey.Sources, *sourceKey)
	}
	sort.SliceStable(answerKey.Sources, func(i, j int) bool {
		return answerKey.Sources[i].SourceID < answerKey.Sources[j].SourceID
	})
	if records.Provenance.Independence != corpusAcceptanceIndependentProvenance {
		blockers = append(blockers, "label_records_not_independent")
	}
	if records.MinEvalCount <= 0 {
		blockers = append(blockers, "missing_min_eval_count")
	}
	if records.SuiteKind == CorpusAcceptanceSuiteHeldOut && records.MinEvalCount < CorpusAcceptanceDEC64MinEvalCount {
		blockers = append(blockers, "below_dec64_min_eval_count")
	}
	if summary.EvalCount < records.MinEvalCount {
		blockers = append(blockers, "below_min_eval_count")
	}
	if len(seenCasesWithOutcomes) < records.CoverageRequirements.MinSourceCount {
		blockers = append(blockers, "below_min_source_count")
	}
	blockers = append(blockers, corpusAcceptanceLabelRecordingGuardrailBlockers(packet.Guardrails)...)
	blockers = append(blockers, corpusAcceptanceLabelRecordingCoverageBlockers(answerKey, records.CoverageRequirements)...)
	summary.Blockers = uniqueStrings(blockers)
	summary.BenchmarkReady = len(summary.Blockers) == 0
	summary.HeldOutReady = summary.BenchmarkReady &&
		records.SuiteKind == CorpusAcceptanceSuiteHeldOut &&
		records.Provenance.Independence == corpusAcceptanceIndependentProvenance &&
		summary.EvalCount >= CorpusAcceptanceDEC64MinEvalCount &&
		records.MinEvalCount >= CorpusAcceptanceDEC64MinEvalCount
	return summary, answerKey, nil
}

type corpusAcceptanceLabelingPacketIndex struct {
	sourcesByCase map[string]*CorpusAcceptanceLabelingSource
	candidates    map[string]map[string]*CorpusAcceptanceLabelingCandidateReference
}

func corpusAcceptanceLabelingIndex(packet CorpusAcceptanceLabelingPacket) corpusAcceptanceLabelingPacketIndex {
	index := corpusAcceptanceLabelingPacketIndex{
		sourcesByCase: map[string]*CorpusAcceptanceLabelingSource{},
		candidates:    map[string]map[string]*CorpusAcceptanceLabelingCandidateReference{},
	}
	for i := range packet.Sources {
		source := &packet.Sources[i]
		index.sourcesByCase[source.CaseID] = source
		index.candidates[source.CaseID] = map[string]*CorpusAcceptanceLabelingCandidateReference{}
		for j := range source.Candidates {
			candidate := &source.Candidates[j]
			index.candidates[source.CaseID][candidate.CandidateID] = candidate
		}
	}
	return index
}

func (index corpusAcceptanceLabelingPacketIndex) lookup(caseID, candidateID string) (*CorpusAcceptanceLabelingSource, *CorpusAcceptanceLabelingCandidateReference, bool) {
	source := index.sourcesByCase[caseID]
	if source == nil || strings.TrimSpace(candidateID) == "" {
		return source, nil, false
	}
	candidate := index.candidates[caseID][candidateID]
	return source, candidate, candidate != nil
}

func validateCorpusAcceptanceLabelRecords(records CorpusAcceptanceLabelRecords, index corpusAcceptanceLabelingPacketIndex) []string {
	var blockers []string
	if strings.TrimSpace(records.SuiteID) == "" || sanitizeID(records.SuiteID) != records.SuiteID {
		blockers = append(blockers, "unsafe_suite_id")
	}
	if records.SuiteKind != CorpusAcceptanceSuiteHeldOut && records.SuiteKind != CorpusAcceptanceSuiteCalibration {
		blockers = append(blockers, "unsupported_suite_kind")
	}
	if strings.TrimSpace(records.Provenance.Labeler) == "" || containsUnsafeMarker(records.Provenance.Labeler) || containsGovernanceID(records.Provenance.Labeler) {
		blockers = append(blockers, "unsafe_labeler")
	}
	if strings.TrimSpace(records.Provenance.Independence) == "" || containsUnsafeMarker(records.Provenance.Independence) || containsGovernanceID(records.Provenance.Independence) {
		blockers = append(blockers, "unsafe_independence")
	}
	seenRecords := map[string]bool{}
	seenOutcomes := map[string]bool{}
	seenCandidateLabelRefs := map[string]bool{}
	for _, record := range records.Records {
		recordRef := safeLabelRecordBlockerID(record.RecordID)
		if strings.TrimSpace(record.RecordID) == "" || sanitizeID(record.RecordID) != record.RecordID || containsUnsafeMarker(record.RecordID) || containsGovernanceID(record.RecordID) {
			blockers = append(blockers, "unsafe_record_id")
		}
		if seenRecords[record.RecordID] {
			blockers = append(blockers, "duplicate_record_id")
		}
		seenRecords[record.RecordID] = true
		source, candidate, hasCandidate := index.lookup(record.CaseID, record.CandidateID)
		if source == nil {
			blockers = append(blockers, "unknown_case_id:"+recordRef)
			continue
		}
		if !validCorpusAcceptanceLabelDecision(record.Decision) {
			blockers = append(blockers, "unsupported_decision:"+recordRef)
		}
		outcomeRecord := record.Decision == CorpusAcceptanceLabelExpectedPresent || record.Decision == CorpusAcceptanceLabelExpectedAbsent
		if outcomeRecord && strings.TrimSpace(record.SourceID) == "" {
			blockers = append(blockers, "missing_source_id:"+recordRef)
		}
		if strings.TrimSpace(record.SourceID) != "" && record.SourceID != source.SourceID {
			blockers = append(blockers, "source_id_mismatch:"+recordRef)
		}
		if record.Decision == CorpusAcceptanceLabelExpectedPresent && !hasCandidate {
			blockers = append(blockers, "missing_candidate:"+recordRef)
		}
		if strings.TrimSpace(record.CandidateID) != "" && !hasCandidate {
			blockers = append(blockers, "unknown_candidate_id:"+recordRef)
		}
		if hasCandidate {
			if record.SourceDocumentID != "" && record.SourceDocumentID != candidate.SourceDocumentID {
				blockers = append(blockers, "source_document_id_mismatch:"+recordRef)
			}
		} else if record.SourceDocumentID != "" && !sourceHasDocumentID(source, record.SourceDocumentID) {
			blockers = append(blockers, "source_document_id_mismatch:"+recordRef)
		}
		if outcomeRecord {
			for _, refKey := range labelRecordDuplicateRefKeys(record, source, candidate, hasCandidate) {
				if seenCandidateLabelRefs[refKey] {
					blockers = append(blockers, "duplicate_label_ref:"+recordRef)
				}
				seenCandidateLabelRefs[refKey] = true
			}
		}
		if record.Decision == CorpusAcceptanceLabelExpectedAbsent && hasCandidate && !hasLabelRecordCandidateConstraints(record) && len(candidate.EvidenceNodes) == 0 {
			blockers = append(blockers, "missing_candidate_constraints:"+recordRef)
		}
		for _, evidence := range labelRecordEvidenceRefs(record) {
			if hasCandidate {
				if !corpusLabelStringListContains(candidate.EvidenceNodes, evidence) {
					blockers = append(blockers, "unknown_evidence_ref:"+recordRef)
				}
				continue
			}
			if !sourceHasEvidenceNode(source, evidence) {
				blockers = append(blockers, "unknown_evidence_ref:"+recordRef)
			}
		}
		if record.Decision == CorpusAcceptanceLabelExpectedPresent || record.Decision == CorpusAcceptanceLabelExpectedAbsent {
			if strings.TrimSpace(record.ExpectedOutcomeID) == "" || sanitizeID(record.ExpectedOutcomeID) != record.ExpectedOutcomeID {
				blockers = append(blockers, "unsafe_expected_outcome_id:"+recordRef)
			}
			if seenOutcomes[record.ExpectedOutcomeID] {
				blockers = append(blockers, "duplicate_expected_outcome_id")
			}
			seenOutcomes[record.ExpectedOutcomeID] = true
			outcome := labelRecordOutcome(record, candidate, hasCandidate)
			sourceAnswerKey := SemanticAcceptanceAnswerKey{
				SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
				AnswerKeyID:      "label-record-" + record.RecordID,
				SourceDocumentID: firstNonBlankCorpusString(record.SourceDocumentID, sourceDocumentIDForLabel(candidate, hasCandidate), source.SourceID),
				ExpectedOutcomes: []SemanticExpectedOutcome{outcome},
			}
			if err := ValidateSemanticAcceptanceAnswerKey(sourceAnswerKey); err != nil {
				blockers = append(blockers, "invalid_expected_outcome:"+recordRef)
			}
		}
		if containsUnsafeLabelRecordMarker(record) {
			blockers = append(blockers, "label_record_contains_private_marker:"+recordRef)
		}
	}
	return uniqueStrings(blockers)
}

func labelRecordOutcome(record CorpusAcceptanceLabelRecordItem, candidate *CorpusAcceptanceLabelingCandidateReference, hasCandidate bool) SemanticExpectedOutcome {
	expectedKind := record.ExpectedKind
	if expectedKind == "" && hasCandidate {
		expectedKind = candidate.CandidateKind
	}
	confidence := record.MinimumConfidenceFloor
	if confidence == "" && hasCandidate {
		confidence = candidate.Confidence
	}
	requiredEvidence := append([]string{}, record.RequiredEvidence...)
	if record.Decision == CorpusAcceptanceLabelExpectedAbsent && hasCandidate && !hasLabelRecordCandidateConstraints(record) {
		requiredEvidence = append(requiredEvidence, candidate.EvidenceNodes...)
	}
	return SemanticExpectedOutcome{
		ExpectedOutcomeID:      record.ExpectedOutcomeID,
		ExpectedState:          SemanticExpectedOutcomeState(record.Decision),
		ExpectedKind:           expectedKind,
		RequiredEvidence:       requiredEvidence,
		AcceptableAlternates:   append([]string{}, record.AcceptableAlternates...),
		TitleSignals:           append([]string{}, record.TitleSignals...),
		SummarySignals:         append([]string{}, record.SummarySignals...),
		RelationRequirements:   append([]SemanticRelationshipType{}, record.RelationRequirements...),
		MinimumConfidenceFloor: confidence,
		Notes:                  record.Notes,
	}
}

func hasLabelRecordCandidateConstraints(record CorpusAcceptanceLabelRecordItem) bool {
	return hasNonBlankString(record.RequiredEvidence) ||
		hasNonBlankString(record.AcceptableAlternates) ||
		hasNonBlankString(record.TitleSignals) ||
		hasNonBlankString(record.SummarySignals) ||
		len(record.RelationRequirements) > 0
}

func corpusAcceptanceLabelRecordingCoverageBlockers(answerKey CorpusAcceptanceAnswerKey, coverage CorpusAcceptanceCoverage) []string {
	kinds := map[SemanticCandidateKind]bool{}
	relations := map[SemanticRelationshipType]bool{}
	failures := map[SemanticAcceptanceReason]bool{}
	for _, source := range answerKey.Sources {
		for _, outcome := range source.ExpectedOutcomes {
			kinds[outcome.ExpectedKind] = true
			for _, relation := range outcome.RelationRequirements {
				relations[relation] = true
			}
			if outcome.ExpectedState == ExpectedOutcomeAbsent {
				failures[SemanticAcceptanceReasonUnexpectedCandidate] = true
			}
			if outcome.ExpectedState == ExpectedOutcomePresent {
				failures[SemanticAcceptanceReasonMissingExpectedOutcome] = true
				failures[SemanticAcceptanceReasonWrongKind] = true
			}
		}
	}
	var blockers []string
	for _, kind := range coverage.CandidateKinds {
		if !validSemanticCandidateKind(kind) {
			blockers = append(blockers, "invalid_candidate_kind_coverage")
			continue
		}
		if !kinds[kind] {
			blockers = append(blockers, "missing_candidate_kind_coverage:"+string(kind))
		}
	}
	for _, relation := range coverage.RelationTypes {
		if !validSemanticRelationshipType(relation) {
			blockers = append(blockers, "invalid_relation_coverage")
			continue
		}
		if !relations[relation] {
			blockers = append(blockers, "missing_relation_coverage:"+string(relation))
		}
	}
	for _, failure := range coverage.FailureModes {
		if !validSemanticAcceptanceReason(failure) {
			blockers = append(blockers, "invalid_failure_mode_coverage")
			continue
		}
		if !failures[failure] {
			blockers = append(blockers, "missing_failure_mode_coverage:"+string(failure))
		}
	}
	return blockers
}

func sanitizedCorpusAcceptanceCoverage(coverage CorpusAcceptanceCoverage) CorpusAcceptanceCoverage {
	sanitized := CorpusAcceptanceCoverage{MinSourceCount: coverage.MinSourceCount}
	for _, kind := range coverage.CandidateKinds {
		if validSemanticCandidateKind(kind) {
			sanitized.CandidateKinds = append(sanitized.CandidateKinds, kind)
		}
	}
	for _, relation := range coverage.RelationTypes {
		if validSemanticRelationshipType(relation) {
			sanitized.RelationTypes = append(sanitized.RelationTypes, relation)
		}
	}
	for _, failure := range coverage.FailureModes {
		if validSemanticAcceptanceReason(failure) {
			sanitized.FailureModes = append(sanitized.FailureModes, failure)
		}
	}
	return sanitized
}

func corpusAcceptanceLabelRecordingGuardrailBlockers(guardrails CorpusPressureGuardrailCounters) []string {
	var blockers []string
	if guardrails.NetworkFetches > 0 {
		blockers = append(blockers, "network_fetch")
	}
	if guardrails.HostedInferenceCalls > 0 {
		blockers = append(blockers, "hosted_inference_call")
	}
	if guardrails.HostedTelemetryExports > 0 {
		blockers = append(blockers, "hosted_telemetry_export")
	}
	if guardrails.BrowserCalls > 0 {
		blockers = append(blockers, "browser_call")
	}
	if guardrails.SlackAPICalls > 0 {
		blockers = append(blockers, "slack_api_call")
	}
	if guardrails.DestinationWrites > 0 {
		blockers = append(blockers, "destination_write")
	}
	if guardrails.ProductBrainWrites > 0 {
		blockers = append(blockers, "product_brain_write")
	}
	if guardrails.TolariaWrites > 0 {
		blockers = append(blockers, "tolaria_write")
	}
	if guardrails.AutoAccepts > 0 {
		blockers = append(blockers, "auto_accept")
	}
	if guardrails.NoHumanClaims > 0 {
		blockers = append(blockers, "no_human_claim")
	}
	if guardrails.CommittedPrivateArtifacts > 0 {
		blockers = append(blockers, "committed_private_artifact")
	}
	return blockers
}

func sourceDocumentIDForLabel(candidate *CorpusAcceptanceLabelingCandidateReference, hasCandidate bool) string {
	if hasCandidate {
		return candidate.SourceDocumentID
	}
	return ""
}

func corpusLabelStringListContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasFatalCorpusAcceptanceLabelRecordBlocker(blockers []string) bool {
	for _, blocker := range blockers {
		switch {
		case blocker == "unsupported_records_schema",
			blocker == "unsupported_suite_kind",
			blocker == "unsafe_suite_id",
			blocker == "unsafe_labeler",
			blocker == "unsafe_independence",
			blocker == "unsafe_record_id",
			blocker == "duplicate_record_id",
			blocker == "duplicate_expected_outcome_id":
			return true
		case strings.HasPrefix(blocker, "unknown_"),
			strings.HasPrefix(blocker, "unsupported_decision:"),
			strings.HasPrefix(blocker, "missing_source_id:"),
			strings.HasPrefix(blocker, "source_id_mismatch:"),
			strings.HasPrefix(blocker, "source_document_id_mismatch:"),
			strings.HasPrefix(blocker, "missing_candidate:"),
			strings.HasPrefix(blocker, "duplicate_label_ref:"),
			strings.HasPrefix(blocker, "missing_candidate_constraints:"),
			strings.HasPrefix(blocker, "unsafe_expected_outcome_id:"),
			strings.HasPrefix(blocker, "invalid_expected_outcome:"),
			strings.HasPrefix(blocker, "label_record_contains_private_marker:"):
			return true
		}
	}
	return false
}

func validCorpusAcceptanceLabelDecision(decision CorpusAcceptanceLabelDecision) bool {
	switch decision {
	case CorpusAcceptanceLabelExpectedPresent, CorpusAcceptanceLabelExpectedAbsent, CorpusAcceptanceLabelUncertain, CorpusAcceptanceLabelAbstain:
		return true
	default:
		return false
	}
}

func validSemanticAcceptanceReason(reason SemanticAcceptanceReason) bool {
	switch reason {
	case SemanticAcceptanceReasonCorrect,
		SemanticAcceptanceReasonWrongKind,
		SemanticAcceptanceReasonUnsupportedEvidence,
		SemanticAcceptanceReasonMissingEvidence,
		SemanticAcceptanceReasonUnsafeOrPrivate,
		SemanticAcceptanceReasonDuplicate,
		SemanticAcceptanceReasonTooBroad,
		SemanticAcceptanceReasonTooNarrow,
		SemanticAcceptanceReasonStaleOrContradicted,
		SemanticAcceptanceReasonAmbiguous,
		SemanticAcceptanceReasonMissingExpectedOutcome,
		SemanticAcceptanceReasonUnexpectedCandidate:
		return true
	default:
		return false
	}
}

func labelRecordEvidenceRefs(record CorpusAcceptanceLabelRecordItem) []string {
	refs := append([]string{}, record.RequiredEvidence...)
	refs = append(refs, record.AcceptableAlternates...)
	return refs
}

func labelRecordDuplicateRefKeys(record CorpusAcceptanceLabelRecordItem, source *CorpusAcceptanceLabelingSource, candidate *CorpusAcceptanceLabelingCandidateReference, hasCandidate bool) []string {
	var keys []string
	if hasCandidate && strings.TrimSpace(record.CandidateID) != "" {
		keys = append(keys, source.SourceID+"\x00candidate\x00"+record.CandidateID)
		return keys
	}
	if inferred := uniqueCandidateForLabelEvidence(source, record); inferred != nil {
		keys = append(keys, source.SourceID+"\x00candidate\x00"+inferred.CandidateID)
	}
	return keys
}

func uniqueCandidateForLabelEvidence(source *CorpusAcceptanceLabelingSource, record CorpusAcceptanceLabelRecordItem) *CorpusAcceptanceLabelingCandidateReference {
	evidenceRefs := nonBlankCorpusLabelStrings(labelRecordEvidenceRefs(record))
	if source == nil || len(evidenceRefs) == 0 {
		return nil
	}
	var match *CorpusAcceptanceLabelingCandidateReference
	for i := range source.Candidates {
		candidate := &source.Candidates[i]
		if record.ExpectedKind != "" && candidate.CandidateKind != record.ExpectedKind {
			continue
		}
		if !candidateHasAllEvidence(candidate, evidenceRefs) {
			continue
		}
		if match != nil {
			return nil
		}
		match = candidate
	}
	return match
}

func candidateHasAllEvidence(candidate *CorpusAcceptanceLabelingCandidateReference, evidenceRefs []string) bool {
	for _, evidence := range evidenceRefs {
		if !corpusLabelStringListContains(candidate.EvidenceNodes, evidence) {
			return false
		}
	}
	return true
}

func nonBlankCorpusLabelStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func sourceHasDocumentID(source *CorpusAcceptanceLabelingSource, sourceDocumentID string) bool {
	for _, candidate := range source.Candidates {
		if candidate.SourceDocumentID == sourceDocumentID {
			return true
		}
	}
	return false
}

func sourceHasEvidenceNode(source *CorpusAcceptanceLabelingSource, evidenceNode string) bool {
	for _, candidate := range source.Candidates {
		if corpusLabelStringListContains(candidate.EvidenceNodes, evidenceNode) {
			return true
		}
	}
	return false
}

func safeLabelRecordBlockerID(recordID string) string {
	if strings.TrimSpace(recordID) == "" || sanitizeID(recordID) != recordID || containsUnsafeMarker(recordID) || containsGovernanceID(recordID) {
		return "unsafe_record_id"
	}
	return recordID
}

func containsUnsafeLabelRecordMarker(record CorpusAcceptanceLabelRecordItem) bool {
	parts := []string{record.RecordID, record.CaseID, record.CandidateID, record.ExpectedOutcomeID, record.SourceID, record.SourceDocumentID, record.Notes}
	parts = append(parts, record.RequiredEvidence...)
	parts = append(parts, record.AcceptableAlternates...)
	parts = append(parts, record.TitleSignals...)
	parts = append(parts, record.SummarySignals...)
	body := strings.Join(parts, "\n")
	return containsUnsafeMarker(body) || containsGovernanceID(body)
}

func CorpusAcceptanceLabelRecordingOutputFor(summary CorpusAcceptanceLabelRecordingSummary) CorpusAcceptanceLabelRecordingOutputSummary {
	return CorpusAcceptanceLabelRecordingOutputSummary{
		SchemaVersion:   summary.SchemaVersion,
		LabelingStatus:  summary.LabelingStatus,
		RecordCount:     summary.RecordCount,
		EvalCount:       summary.EvalCount,
		UncertainCount:  summary.UncertainCount,
		AbstainCount:    summary.AbstainCount,
		HeldOutReady:    summary.HeldOutReady,
		BenchmarkReady:  summary.BenchmarkReady,
		Blockers:        append([]string{}, summary.Blockers...),
		AnswerKeyPath:   summary.AnswerKeyPath,
		ReportPath:      summary.ReportPath,
		ClaimBoundaries: append([]string{}, summary.ClaimBoundaries...),
	}
}
