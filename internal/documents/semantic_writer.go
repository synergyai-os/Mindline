package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteSemantic(outDir, runID string, sourceCount int, observations []SemanticObservation, candidates []SemanticCandidate, relations []SemanticRelation) error {
	if err := writeSemantic(outDir, runID, sourceCount, observations, candidates, relations); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func writeSemantic(outDir, runID string, sourceCount int, observations []SemanticObservation, candidates []SemanticCandidate, relations []SemanticRelation) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("missing required --out")
	}
	observations = finalizeSemanticObservations(observations)
	candidates = finalizeSemanticCandidates(candidates)
	relations = finalizeSemanticRelations(relations)
	if err := RejectDuplicateSemanticObservationIDs(observations); err != nil {
		return err
	}
	if err := RejectDuplicateSemanticCandidateIDs(candidates); err != nil {
		return err
	}
	if err := RejectDuplicateSemanticRelationIDs(relations); err != nil {
		return err
	}
	root, err := filepath.Abs(filepath.Join(outDir, "semantic-candidates"))
	if err != nil {
		return err
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return err
	}
	if err := rejectIfSymlink(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	for _, observation := range observations {
		if err := ValidateSemanticObservation(observation); err != nil {
			return err
		}
	}
	for _, candidate := range candidates {
		if err := ValidateSemanticCandidate(candidate); err != nil {
			return err
		}
	}
	for _, relation := range relations {
		if err := ValidateSemanticRelation(relation); err != nil {
			return err
		}
	}
	expectedFiles := map[string]bool{"semantic-summary.json": true}
	for _, observation := range observations {
		expectedFiles[SemanticObservationJSONPath(observation.ObservationID)] = true
	}
	for _, candidate := range candidates {
		expectedFiles[SemanticCandidateJSONPath(candidate.CandidateID)] = true
		expectedFiles[SemanticPreviewPath(candidate.CandidateID)] = true
	}
	for _, relation := range relations {
		expectedFiles[SemanticRelationJSONPath(relation.RelationID)] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expectedFiles); err != nil {
		return err
	}
	summary := BuildSemanticSummary(runID, sourceCount, observations, candidates, relations)
	if err := writeJSON(realRoot, "semantic-summary.json", summary); err != nil {
		return err
	}
	for _, observation := range observations {
		if err := writeJSON(realRoot, SemanticObservationJSONPath(observation.ObservationID), observation); err != nil {
			return err
		}
	}
	for _, candidate := range candidates {
		if err := writeJSON(realRoot, SemanticCandidateJSONPath(candidate.CandidateID), candidate); err != nil {
			return err
		}
		if err := writeFile(realRoot, SemanticPreviewPath(candidate.CandidateID), []byte(semanticPreviewMarkdown(candidate))); err != nil {
			return err
		}
	}
	for _, relation := range relations {
		if err := writeJSON(realRoot, SemanticRelationJSONPath(relation.RelationID), relation); err != nil {
			return err
		}
	}
	return nil
}

func finalizeSemanticObservations(observations []SemanticObservation) []SemanticObservation {
	out := make([]SemanticObservation, 0, len(observations))
	for _, observation := range observations {
		observation.EvidenceNodes = cloneStringList(observation.EvidenceNodes)
		observation.Blockers = cloneBlockerList(observation.Blockers)
		out = append(out, ClassifyUnsafeSemanticObservation(observation))
	}
	return orderSemanticObservations(out)
}

func finalizeSemanticCandidates(candidates []SemanticCandidate) []SemanticCandidate {
	out := make([]SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.EvidenceNodes = cloneStringList(candidate.EvidenceNodes)
		candidate.ObservationIDs = cloneStringList(candidate.ObservationIDs)
		candidate.RelationIDs = cloneStringList(candidate.RelationIDs)
		candidate.Blockers = cloneBlockerList(candidate.Blockers)
		out = append(out, ClassifyUnsafeSemanticCandidate(candidate))
	}
	return orderSemanticCandidates(out)
}

func finalizeSemanticRelations(relations []SemanticRelation) []SemanticRelation {
	out := make([]SemanticRelation, 0, len(relations))
	for _, relation := range relations {
		relation.EvidenceNodes = cloneStringList(relation.EvidenceNodes)
		relation.Blockers = cloneBlockerList(relation.Blockers)
		out = append(out, ClassifyUnsafeSemanticRelation(relation))
	}
	return orderSemanticRelations(out)
}

func ClassifyUnsafeSemanticObservation(observation SemanticObservation) SemanticObservation {
	body := observation.ObservationID + "\n" + observation.Title + "\n" + observation.Summary + "\n" + observation.SourceDocumentID + "\n" + strings.Join(observation.EvidenceNodes, "\n")
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		observation.ReviewStatus = ReviewStatusBlocked
		observation.Confidence = ConfidenceLow
		observation.ObservationID = redactUnsafeSemanticID("obs", observation.ObservationID)
		observation.SourceDocumentID = redactedDocumentID(observation.SourceDocumentID)
		observation.Title = "Unsafe content redacted"
		observation.Summary = "Semantic observation content was redacted because it contains an unsafe marker."
		observation.EvidenceNodes = redactUnsafeSemanticIDs("node", observation.EvidenceNodes)
		for i := range observation.EvidenceRanges {
			observation.EvidenceRanges[i].StructureNodeID = redactUnsafeSemanticID("node", observation.EvidenceRanges[i].StructureNodeID)
		}
		observation.Blockers = append(observation.Blockers, Blocker{Code: "unsafe_private_marker", Message: "Semantic observation contains an unsafe or private marker."})
	}
	return observation
}

func ClassifyUnsafeSemanticCandidate(candidate SemanticCandidate) SemanticCandidate {
	body := candidate.CandidateID + "\n" + candidate.Title + "\n" + candidate.Summary + "\n" + candidate.SourceDocumentID + "\n" + strings.Join(candidate.EvidenceNodes, "\n") + "\n" + strings.Join(candidate.ObservationIDs, "\n") + "\n" + strings.Join(candidate.RelationIDs, "\n")
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		candidate.ReviewStatus = ReviewStatusBlocked
		candidate.Confidence = ConfidenceLow
		candidate.CandidateID = redactUnsafeSemanticID("cand", candidate.CandidateID)
		candidate.SourceDocumentID = redactedDocumentID(candidate.SourceDocumentID)
		candidate.Title = "Unsafe content redacted"
		candidate.Summary = "Semantic candidate content was redacted because it contains an unsafe marker."
		candidate.EvidenceNodes = redactUnsafeSemanticIDs("node", candidate.EvidenceNodes)
		for i := range candidate.EvidenceRanges {
			candidate.EvidenceRanges[i].StructureNodeID = redactUnsafeSemanticID("node", candidate.EvidenceRanges[i].StructureNodeID)
		}
		candidate.ObservationIDs = redactUnsafeSemanticIDs("obs", candidate.ObservationIDs)
		candidate.RelationIDs = redactUnsafeSemanticIDs("rel", candidate.RelationIDs)
		candidate.Blockers = append(candidate.Blockers, Blocker{Code: "unsafe_private_marker", Message: "Semantic candidate contains an unsafe or private marker."})
	}
	return candidate
}

func ClassifyUnsafeSemanticRelation(relation SemanticRelation) SemanticRelation {
	body := relation.RelationID + "\n" + relation.FromID + "\n" + relation.ToID + "\n" + strings.Join(relation.EvidenceNodes, "\n")
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		relation.ReviewStatus = ReviewStatusBlocked
		relation.Confidence = ConfidenceLow
		relation.RelationID = redactUnsafeSemanticID("rel", relation.RelationID)
		relation.FromID = redactUnsafeSemanticID("endpoint", relation.FromID)
		relation.ToID = redactUnsafeSemanticID("endpoint", relation.ToID)
		relation.EvidenceNodes = redactUnsafeSemanticIDs("node", relation.EvidenceNodes)
		relation.Blockers = append(relation.Blockers, Blocker{Code: "unsafe_private_marker", Message: "Semantic relation contains an unsafe or private marker."})
	}
	return relation
}

func redactUnsafeSemanticIDs(prefix string, values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, redactUnsafeSemanticID(prefix, value))
	}
	return out
}

func redactUnsafeSemanticID(prefix, value string) string {
	if containsUnsafeMarker(value) || containsGovernanceID(value) {
		return prefix + "-" + strings.TrimPrefix(redactedDocumentID(value), "doc-")
	}
	return value
}

func ValidateSemanticObservation(observation SemanticObservation) error {
	if observation.SchemaVersion != SemanticObservationSchemaVersion {
		return fmt.Errorf("unsupported semantic observation schema version: %s", observation.SchemaVersion)
	}
	if strings.TrimSpace(observation.ObservationID) == "" || sanitizeID(observation.ObservationID) != observation.ObservationID {
		return fmt.Errorf("unsafe semantic observation id: %s", observation.ObservationID)
	}
	if strings.TrimSpace(observation.RunID) == "" || strings.TrimSpace(observation.SourceDocumentID) == "" {
		return fmt.Errorf("semantic observation missing run or source id")
	}
	if !validSemanticObservationKind(observation.ObservationKind) {
		return fmt.Errorf("unsupported semantic observation kind: %s", observation.ObservationKind)
	}
	return validateSemanticReviewFields(observation.ReviewStatus, observation.Confidence, observation.Title, observation.Summary, observation.EvidenceNodes)
}

func ValidateSemanticCandidate(candidate SemanticCandidate) error {
	if candidate.SchemaVersion != SemanticCandidateSchemaVersion {
		return fmt.Errorf("unsupported semantic candidate schema version: %s", candidate.SchemaVersion)
	}
	if strings.TrimSpace(candidate.CandidateID) == "" || sanitizeID(candidate.CandidateID) != candidate.CandidateID {
		return fmt.Errorf("unsafe semantic candidate id: %s", candidate.CandidateID)
	}
	if strings.TrimSpace(candidate.RunID) == "" {
		return fmt.Errorf("semantic candidate missing run id")
	}
	if !validSemanticCandidateKind(candidate.CandidateKind) {
		return fmt.Errorf("unsupported semantic candidate kind: %s", candidate.CandidateKind)
	}
	if candidate.DestinationStatus != SemanticDestinationUnresolved {
		return fmt.Errorf("semantic destination status must be unresolved")
	}
	if candidate.ReviewStatus == ReviewStatusReady && (len(candidate.ObservationIDs) == 0 || len(candidate.RelationIDs) == 0) {
		return fmt.Errorf("ready semantic candidates require observations and relations")
	}
	return validateSemanticReviewFields(candidate.ReviewStatus, candidate.Confidence, candidate.Title, candidate.Summary, candidate.EvidenceNodes)
}

func ValidateSemanticRelation(relation SemanticRelation) error {
	if relation.SchemaVersion != SemanticRelationSchemaVersion {
		return fmt.Errorf("unsupported semantic relation schema version: %s", relation.SchemaVersion)
	}
	if strings.TrimSpace(relation.RelationID) == "" || sanitizeID(relation.RelationID) != relation.RelationID {
		return fmt.Errorf("unsafe semantic relation id: %s", relation.RelationID)
	}
	if strings.TrimSpace(relation.RunID) == "" || strings.TrimSpace(relation.FromID) == "" || strings.TrimSpace(relation.ToID) == "" {
		return fmt.Errorf("semantic relation missing run or endpoint")
	}
	if !validSemanticRelationshipType(relation.RelationshipType) {
		return fmt.Errorf("unsupported semantic relationship type: %s", relation.RelationshipType)
	}
	if !validSemanticRelationEndpointType(relation.FromType) || !validSemanticRelationEndpointType(relation.ToType) {
		return fmt.Errorf("unsupported semantic relation endpoint")
	}
	return validateSemanticReviewFields(relation.ReviewStatus, relation.Confidence, "relation", "relation", relation.EvidenceNodes)
}

func validateSemanticReviewFields(status ReviewStatus, confidence Confidence, title, summary string, evidenceNodes []string) error {
	if !validReviewStatus(status) {
		return fmt.Errorf("unsupported review status: %s", status)
	}
	if !validConfidence(confidence) {
		return fmt.Errorf("unsupported confidence: %s", confidence)
	}
	if confidence == ConfidenceLow && status == ReviewStatusReady {
		return fmt.Errorf("low confidence semantic artifacts cannot be ready")
	}
	if status == ReviewStatusReady && (strings.TrimSpace(title) == "" || strings.TrimSpace(summary) == "" || len(evidenceNodes) == 0) {
		return fmt.Errorf("ready semantic artifacts require title, summary, and evidence nodes")
	}
	return nil
}

func RejectDuplicateSemanticObservationIDs(observations []SemanticObservation) error {
	seen := map[string]bool{}
	for _, observation := range observations {
		if seen[observation.ObservationID] {
			return fmt.Errorf("duplicate semantic observation id: %s", observation.ObservationID)
		}
		seen[observation.ObservationID] = true
	}
	return nil
}

func RejectDuplicateSemanticCandidateIDs(candidates []SemanticCandidate) error {
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate.CandidateID] {
			return fmt.Errorf("duplicate semantic candidate id: %s", candidate.CandidateID)
		}
		seen[candidate.CandidateID] = true
	}
	return nil
}

func RejectDuplicateSemanticRelationIDs(relations []SemanticRelation) error {
	seen := map[string]bool{}
	for _, relation := range relations {
		if seen[relation.RelationID] {
			return fmt.Errorf("duplicate semantic relation id: %s", relation.RelationID)
		}
		seen[relation.RelationID] = true
	}
	return nil
}

func validSemanticCandidateKind(value SemanticCandidateKind) bool {
	switch value {
	case SemanticCandidateKindTopic, SemanticCandidateKindDecision, SemanticCandidateKindAction, SemanticCandidateKindIssue, SemanticCandidateKindQuestion, SemanticCandidateKindRequirement, SemanticCandidateKindCapability, SemanticCandidateKindDependency, SemanticCandidateKindRisk, SemanticCandidateKindReference, SemanticCandidateKindUnknown:
		return true
	default:
		return false
	}
}

func validSemanticObservationKind(value SemanticObservationKind) bool {
	switch value {
	case SemanticObservationKindAgendaFrame, SemanticObservationKindClaim, SemanticObservationKindQuestion, SemanticObservationKindProposal, SemanticObservationKindObjection, SemanticObservationKindDecisionSignal, SemanticObservationKindActionSignal, SemanticObservationKindOwnerSignal, SemanticObservationKindDeadlineSignal, SemanticObservationKindRecapSignal, SemanticObservationKindCapabilityStatement, SemanticObservationKindRequirementStatement, SemanticObservationKindDependencyStatement, SemanticObservationKindRiskStatement, SemanticObservationKindReferenceStatement, SemanticObservationKindUnknown:
		return true
	default:
		return false
	}
}

func validSemanticRelationshipType(value SemanticRelationshipType) bool {
	switch value {
	case SemanticRelationshipSupports, SemanticRelationshipRefines, SemanticRelationshipContradicts, SemanticRelationshipAnswers, SemanticRelationshipSupersedes, SemanticRelationshipSummarizes, SemanticRelationshipSameTopicAs, SemanticRelationshipDependsOn, SemanticRelationshipAssignsAction, SemanticRelationshipMentionsOwner, SemanticRelationshipMentionsDeadline, SemanticRelationshipDerivedFrom:
		return true
	default:
		return false
	}
}

func validSemanticRelationEndpointType(value SemanticRelationEndpointType) bool {
	switch value {
	case SemanticRelationEndpointStructureNode, SemanticRelationEndpointObservation, SemanticRelationEndpointCandidate:
		return true
	default:
		return false
	}
}

func semanticPreviewMarkdown(candidate SemanticCandidate) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(candidate.Title)
	b.WriteString("\n\n")
	b.WriteString("- Candidate: ")
	b.WriteString(candidate.CandidateID)
	b.WriteString("\n")
	b.WriteString("- Kind: ")
	b.WriteString(string(candidate.CandidateKind))
	b.WriteString("\n")
	b.WriteString("- Review status: ")
	b.WriteString(string(candidate.ReviewStatus))
	b.WriteString("\n")
	b.WriteString("- Confidence: ")
	b.WriteString(string(candidate.Confidence))
	b.WriteString("\n")
	b.WriteString("- Destination status: unresolved\n")
	b.WriteString("- Evidence nodes: ")
	b.WriteString(strings.Join(candidate.EvidenceNodes, ", "))
	b.WriteString("\n\n")
	b.WriteString(candidate.Summary)
	b.WriteString("\n")
	return b.String()
}

func orderSemanticObservations(observations []SemanticObservation) []SemanticObservation {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		return strings.Join([]string{left.SourceDocumentID, left.ObservationID}, "\x00") < strings.Join([]string{right.SourceDocumentID, right.ObservationID}, "\x00")
	})
	return observations
}

func orderSemanticCandidates(candidates []SemanticCandidate) []SemanticCandidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		return strings.Join([]string{string(left.CandidateKind), left.CandidateID}, "\x00") < strings.Join([]string{string(right.CandidateKind), right.CandidateID}, "\x00")
	})
	return candidates
}

func orderSemanticRelations(relations []SemanticRelation) []SemanticRelation {
	sort.SliceStable(relations, func(i, j int) bool {
		left, right := relations[i], relations[j]
		return strings.Join([]string{string(left.RelationshipType), left.RelationID}, "\x00") < strings.Join([]string{string(right.RelationshipType), right.RelationID}, "\x00")
	})
	return relations
}

func cloneBlockerList(values []Blocker) []Blocker {
	if len(values) == 0 {
		return []Blocker{}
	}
	return append([]Blocker(nil), values...)
}
