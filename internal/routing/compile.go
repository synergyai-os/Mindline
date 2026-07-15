package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// CompileGraph is the source-adapter-neutral compiler seam. Source adapters own
// native ingestion and produce this graph; lens resolution and semantic routing
// do not inspect adapter-native fields.
func CompileGraph(graph SourceGraph, artifacts LinkArtifacts, profile LensProfile, judgments Judgments) (Result, error) {
	if err := validateProfile(profile); err != nil {
		return Result{}, err
	}
	if artifacts.SchemaVersion != LinkArtifactsSchema {
		return Result{}, fmt.Errorf("unsupported link artifacts schema: %s", artifacts.SchemaVersion)
	}
	if err := validateJudgmentHeader(judgments, profile); err != nil {
		return Result{}, err
	}
	if err := validateSourceGraph(graph); err != nil {
		return Result{}, err
	}
	artifactByCanonical := map[string]LinkArtifact{}
	for _, artifact := range artifacts.Items {
		canonical, err := CanonicalizeURL(artifact.CanonicalURL)
		if err != nil {
			return Result{}, errors.New("invalid enrichment canonical URL")
		}
		if err := validateArtifact(artifact); err != nil {
			return Result{}, err
		}
		artifact.CanonicalURL = canonical
		identity := canonicalIdentity(canonical)
		if _, exists := artifactByCanonical[identity]; exists {
			return Result{}, errors.New("duplicate enrichment canonical URL")
		}
		artifactByCanonical[identity] = artifact
	}
	artifactByID := map[string]LinkArtifact{}
	for _, source := range graph.CanonicalURLs {
		artifact, ok := artifactByCanonical[canonicalIdentity(source.CanonicalURL)]
		if !ok {
			return Result{}, fmt.Errorf("missing enrichment artifact for canonical source %s", source.CanonicalURLID)
		}
		artifactByID[source.CanonicalURLID] = artifact
	}
	if len(artifactByCanonical) != len(graph.CanonicalURLs) {
		return Result{}, errors.New("enrichment artifact contains unknown canonical source")
	}
	graph.Fingerprint = fingerprint(graph)
	decisions, err := compileDecisions(graph, artifactByID, profile, judgments)
	if err != nil {
		return Result{}, err
	}
	summary := summarize(graph, decisions, profile)
	result := Result{Graph: graph, Decisions: decisions, Summary: summary}
	if err := ValidateResult(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func validateSourceGraph(graph SourceGraph) error {
	if graph.SchemaVersion != SourceGraphSchema {
		return errors.New("unsupported source graph schema")
	}
	if strings.TrimSpace(graph.Adapter.Kind) == "" || strings.TrimSpace(graph.Adapter.Version) == "" {
		return errors.New("invalid source adapter identity")
	}
	sourceKinds := map[string]bool{"message": true, "document": true, "bookmark": true, "transcript_segment": true, "unknown": true}
	urlKinds := map[string]bool{"github_repository": true, "linkedin_post": true, "linkedin_article": true, "youtube_video": true, "article": true, "pdf": true, "generic_web": true, "unknown": true}
	enrichmentStates := map[string]bool{"complete": true, "partial": true, "inaccessible": true, "failed": true, "not_attempted": true}
	canonical := map[string]CanonicalURL{}
	canonicalIdentitySeen := map[string]bool{}
	for _, source := range graph.CanonicalURLs {
		normalized, err := CanonicalizeURL(source.CanonicalURL)
		identity := canonicalIdentity(normalized)
		if err != nil || normalized != source.CanonicalURL || source.CanonicalURLID != CanonicalURLID(source.CanonicalURL) || canonical[source.CanonicalURLID].CanonicalURLID != "" || canonicalIdentitySeen[identity] || !urlKinds[source.Kind] || !enrichmentStates[source.EnrichmentState] || source.Depth < 0 || source.Depth > 1 {
			return errors.New("invalid canonical source")
		}
		if source.Depth == 0 && (source.ParentCanonicalURLID != "" || source.Discovery != "source_occurrence") {
			return errors.New("invalid primary canonical source")
		}
		if source.Depth == 1 && (source.ParentCanonicalURLID == "" || source.Discovery != "enrichment_related_url") {
			return errors.New("invalid related canonical source")
		}
		canonical[source.CanonicalURLID] = source
		canonicalIdentitySeen[identity] = true
	}
	for _, source := range graph.CanonicalURLs {
		if source.Depth == 1 {
			parent, ok := canonical[source.ParentCanonicalURLID]
			if !ok || parent.Depth != 0 || parent.CanonicalURLID == source.CanonicalURLID {
				return errors.New("invalid related canonical parent")
			}
		}
	}
	records := map[string]SourceRecord{}
	for _, record := range graph.SourceRecords {
		if record.SourceRecordID == "" || records[record.SourceRecordID].SourceRecordID != "" || !sourceKinds[record.SourceKind] || strings.TrimSpace(record.RawProvenanceRef) == "" {
			return errors.New("invalid source record")
		}
		if _, err := time.Parse(time.RFC3339, record.OccurredAt); err != nil {
			return errors.New("invalid source record timestamp")
		}
		records[record.SourceRecordID] = record
	}
	occurrences := map[string]URLOccurrence{}
	primaryOccurrenceCoverage := map[string]int{}
	for _, occurrence := range graph.URLOccurrences {
		target, targetExists := canonical[occurrence.CanonicalURLID]
		observedCanonical, err := CanonicalizeURL(occurrence.ObservedURL)
		if occurrence.URLOccurrenceID == "" || occurrences[occurrence.URLOccurrenceID].URLOccurrenceID != "" || records[occurrence.SourceRecordID].SourceRecordID == "" || !targetExists || target.Depth != 0 || err != nil || canonicalIdentity(observedCanonical) != canonicalIdentity(target.CanonicalURL) {
			return errors.New("invalid URL occurrence")
		}
		occurrences[occurrence.URLOccurrenceID] = occurrence
		primaryOccurrenceCoverage[occurrence.CanonicalURLID]++
	}
	accountedOccurrences := map[string]bool{}
	for _, record := range graph.SourceRecords {
		recordOccurrenceIDs := map[string]bool{}
		for _, id := range record.URLOccurrenceIDs {
			occurrence, ok := occurrences[id]
			if !ok || occurrence.SourceRecordID != record.SourceRecordID || recordOccurrenceIDs[id] || accountedOccurrences[id] {
				return errors.New("missing URL occurrence accounting")
			}
			recordOccurrenceIDs[id] = true
			accountedOccurrences[id] = true
		}
	}
	if len(accountedOccurrences) != len(occurrences) {
		return errors.New("unaccounted URL occurrence")
	}

	edgeIDs := map[string]bool{}
	containsOccurrence := map[string]bool{}
	linkedChild := map[string]bool{}
	for _, edge := range graph.Edges {
		if strings.TrimSpace(edge.EdgeID) == "" || edgeIDs[edge.EdgeID] || len(edge.EvidenceRefs) == 0 || hasEmptyOrDuplicate(edge.EvidenceRefs) {
			return errors.New("invalid graph edge")
		}
		edgeIDs[edge.EdgeID] = true
		switch edge.Type {
		case "source_record_contains_url":
			if records[edge.From].SourceRecordID == "" || canonical[edge.To].CanonicalURLID == "" || canonical[edge.To].Depth != 0 || len(edge.EvidenceRefs) != 1 {
				return errors.New("invalid source occurrence edge")
			}
			occurrence, ok := occurrences[edge.EvidenceRefs[0]]
			if !ok || occurrence.SourceRecordID != edge.From || occurrence.CanonicalURLID != edge.To || containsOccurrence[occurrence.URLOccurrenceID] {
				return errors.New("invalid source occurrence edge evidence")
			}
			containsOccurrence[occurrence.URLOccurrenceID] = true
		case "source_links_to":
			from, fromOK := canonical[edge.From]
			to, toOK := canonical[edge.To]
			if !fromOK || !toOK || from.Depth != 0 || to.Depth != 1 || to.ParentCanonicalURLID != from.CanonicalURLID || linkedChild[to.CanonicalURLID] {
				return errors.New("invalid related source edge")
			}
			linkedChild[to.CanonicalURLID] = true
		case "canonical_duplicate_of":
			from, fromOK := canonical[edge.From]
			to, toOK := canonical[edge.To]
			if !fromOK || !toOK || from.CanonicalURLID == to.CanonicalURLID {
				return errors.New("invalid canonical duplicate edge")
			}
		default:
			return errors.New("unsupported graph edge type")
		}
	}
	if len(containsOccurrence) != len(occurrences) {
		return errors.New("incomplete source occurrence edge accounting")
	}
	for _, source := range graph.CanonicalURLs {
		if source.Depth == 0 && primaryOccurrenceCoverage[source.CanonicalURLID] == 0 {
			return errors.New("orphan primary canonical source")
		}
		if source.Depth == 1 && !linkedChild[source.CanonicalURLID] {
			return errors.New("missing related source edge")
		}
	}
	return nil
}

func hasEmptyOrDuplicate(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

// ValidateResult is the single authority boundary for compiled and reloaded
// routing artifacts. Fingerprints make tampering visible; these checks make a
// self-consistent, refingerprinted but malformed graph fail closed.
func ValidateResult(result Result) error {
	if err := validateSourceGraph(result.Graph); err != nil {
		return err
	}
	if result.Graph.Fingerprint == "" || result.Graph.Fingerprint != fingerprint(result.Graph) {
		return errors.New("source graph fingerprint mismatch")
	}
	decisions := result.Decisions
	if decisions.SchemaVersion != DecisionsSchema || decisions.Fingerprint == "" || decisions.Fingerprint != fingerprint(decisions) || decisions.SourceGraphFingerprint != result.Graph.Fingerprint || strings.TrimSpace(decisions.LensProfileFingerprint) == "" || decisions.JudgmentMethod != "operator_agent_review" {
		return errors.New("invalid route decisions authority")
	}

	canonicalByID := map[string]CanonicalURL{}
	for _, source := range result.Graph.CanonicalURLs {
		canonicalByID[source.CanonicalURLID] = source
	}
	routedByID := map[string]RoutedSource{}
	var lensOrder []string
	for index, source := range decisions.Sources {
		canonical, ok := canonicalByID[source.CanonicalURLID]
		if !ok || routedByID[source.CanonicalURLID].CanonicalURLID != "" || source.CanonicalURL != canonical.CanonicalURL || source.Depth != canonical.Depth || source.EnrichmentState != canonical.EnrichmentState || !equalStringSlices(source.Missingness, canonical.Missingness) {
			return errors.New("route decision source mismatch")
		}
		currentLensOrder := make([]string, 0, len(source.LensResults))
		for _, lens := range source.LensResults {
			currentLensOrder = append(currentLensOrder, lens.LensID)
		}
		if hasEmptyOrDuplicate(currentLensOrder) {
			return errors.New("invalid route decision lens coverage")
		}
		if index == 0 {
			lensOrder = currentLensOrder
		} else if !equalStringSlices(lensOrder, currentLensOrder) {
			return errors.New("inconsistent route decision lens coverage")
		}
		profile := LensProfile{SchemaVersion: LensProfileSchemaVersion, ProfileID: "loaded-result", ProfileVersion: "bound"}
		for _, lensID := range lensOrder {
			profile.Lenses = append(profile.Lenses, Lens{LensID: lensID, Name: lensID, Question: lensID})
		}
		artifact := LinkArtifact{CanonicalURL: source.CanonicalURL, State: source.EnrichmentState, PublicMetadata: source.PublicMetadata, PublicExcerpts: source.PublicExcerpts, Missingness: source.Missingness}
		if err := validateArtifact(artifact); err != nil {
			return fmt.Errorf("invalid routed enrichment for %s: %w", source.CanonicalURLID, err)
		}
		judgment := SourceJudgment{CanonicalURLID: source.CanonicalURLID, LensResults: source.LensResults, SemanticAssessment: source.SemanticAssessment, Disposition: source.Disposition, DispositionRationale: source.DispositionRationale, SemanticNodes: source.SemanticNodes, SemanticEdges: source.SemanticEdges}
		if err := validateSourceJudgment(judgment, canonical, artifact, profile); err != nil {
			return fmt.Errorf("invalid routed source %s: %w", source.CanonicalURLID, err)
		}
		routedByID[source.CanonicalURLID] = source
	}
	if len(routedByID) != len(canonicalByID) {
		return errors.New("incomplete route decision coverage")
	}
	if err := validateGraphEvidence(result.Graph, routedByID); err != nil {
		return err
	}

	lensCount := len(lensOrder)
	if len(decisions.Sources) == 0 {
		lensCount = result.Summary.LensCount
		if lensCount < 0 || lensCount > 8 {
			return errors.New("invalid empty-route lens count")
		}
	}
	expectedSummary := summarizeBound(result.Graph, decisions, lensCount, decisions.LensProfileFingerprint)
	actualSummary, _ := json.Marshal(result.Summary)
	expectedSummaryJSON, _ := json.Marshal(expectedSummary)
	if string(actualSummary) != string(expectedSummaryJSON) {
		return errors.New("route summary authority mismatch")
	}
	return nil
}

func validateGraphEvidence(graph SourceGraph, routed map[string]RoutedSource) error {
	occurrences := map[string]bool{}
	for _, occurrence := range graph.URLOccurrences {
		occurrences[occurrence.URLOccurrenceID] = true
	}
	for _, edge := range graph.Edges {
		switch edge.Type {
		case "source_links_to":
			artifact := LinkArtifact{PublicExcerpts: routed[edge.From].PublicExcerpts}
			for _, ref := range edge.EvidenceRefs {
				if !artifactHasEvidence(artifact, ref) {
					return errors.New("unresolved related source edge evidence")
				}
			}
		case "canonical_duplicate_of":
			artifact := LinkArtifact{PublicExcerpts: routed[edge.From].PublicExcerpts}
			for _, ref := range edge.EvidenceRefs {
				if !occurrences[ref] && !artifactHasEvidence(artifact, ref) {
					return errors.New("unresolved canonical duplicate edge evidence")
				}
			}
		}
	}
	return nil
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func compileDecisions(graph SourceGraph, artifacts map[string]LinkArtifact, profile LensProfile, judgments Judgments) (RouteDecisions, error) {
	byID := map[string]SourceJudgment{}
	for _, judgment := range judgments.Sources {
		if _, exists := byID[judgment.CanonicalURLID]; exists {
			return RouteDecisions{}, errors.New("duplicate source judgment")
		}
		byID[judgment.CanonicalURLID] = judgment
	}
	decisions := RouteDecisions{
		SchemaVersion: DecisionsSchema, SourceGraphFingerprint: graph.Fingerprint, LensProfileFingerprint: fingerprint(profile), JudgmentMethod: judgments.JudgmentMethod,
	}
	for _, canonical := range graph.CanonicalURLs {
		judgment, ok := byID[canonical.CanonicalURLID]
		if !ok {
			return RouteDecisions{}, fmt.Errorf("missing source judgment for %s", canonical.CanonicalURLID)
		}
		artifact := artifacts[canonical.CanonicalURLID]
		if err := validateSourceJudgment(judgment, canonical, artifact, profile); err != nil {
			return RouteDecisions{}, fmt.Errorf("invalid source judgment for %s: %w", canonical.CanonicalURLID, err)
		}
		decisions.Sources = append(decisions.Sources, RoutedSource{
			CanonicalURLID: canonical.CanonicalURLID, CanonicalURL: canonical.CanonicalURL, Depth: canonical.Depth, EnrichmentState: canonical.EnrichmentState,
			PublicMetadata: artifact.PublicMetadata, PublicExcerpts: cloneExcerpts(artifact.PublicExcerpts), Missingness: cloneStrings(artifact.Missingness),
			LensResults: cloneLensResults(judgment.LensResults), SemanticAssessment: judgment.SemanticAssessment, Disposition: judgment.Disposition,
			DispositionRationale: judgment.DispositionRationale, SemanticNodes: cloneNodes(judgment.SemanticNodes), SemanticEdges: cloneEdges(judgment.SemanticEdges),
		})
	}
	if len(byID) != len(graph.CanonicalURLs) {
		return RouteDecisions{}, errors.New("judgment manifest contains unknown canonical source")
	}
	decisions.Fingerprint = fingerprint(decisions)
	return decisions, nil
}

func validateProfile(profile LensProfile) error {
	if profile.SchemaVersion != LensProfileSchemaVersion {
		return fmt.Errorf("unsupported lens profile schema: %s", profile.SchemaVersion)
	}
	if strings.TrimSpace(profile.ProfileID) == "" || strings.TrimSpace(profile.ProfileVersion) == "" {
		return errors.New("missing lens profile identity")
	}
	if len(profile.Lenses) > 8 {
		return errors.New("context lens limit exceeded")
	}
	seen := map[string]bool{}
	for _, lens := range profile.Lenses {
		if !isSlug(lens.LensID) || seen[lens.LensID] || strings.TrimSpace(lens.Name) == "" || strings.TrimSpace(lens.Question) == "" {
			return errors.New("invalid context lens")
		}
		seen[lens.LensID] = true
	}
	return nil
}

func validateJudgmentHeader(judgments Judgments, profile LensProfile) error {
	if judgments.SchemaVersion != JudgmentsSchema || judgments.JudgmentMethod != "operator_agent_review" {
		return errors.New("unsupported judgment manifest")
	}
	if judgments.ProfileID != profile.ProfileID || judgments.ProfileVersion != profile.ProfileVersion {
		return errors.New("judgment profile mismatch")
	}
	if _, err := time.Parse(time.RFC3339, judgments.JudgedAt); err != nil {
		return errors.New("invalid judged_at")
	}
	return nil
}

func validateArtifact(artifact LinkArtifact) error {
	switch artifact.State {
	case "complete", "partial", "inaccessible", "failed", "not_attempted":
	default:
		return errors.New("invalid enrichment state")
	}
	total := 0
	seen := map[string]bool{}
	for _, excerpt := range artifact.PublicExcerpts {
		length := utf8.RuneCountInString(excerpt.Text)
		if strings.TrimSpace(excerpt.ExcerptID) == "" || seen[excerpt.ExcerptID] || length == 0 || length > 1000 {
			return errors.New("invalid public excerpt")
		}
		total += length
		seen[excerpt.ExcerptID] = true
	}
	if total > 4000 {
		return errors.New("public excerpt budget exceeded")
	}
	if artifact.State == "inaccessible" && (len(artifact.Missingness) == 0 || len(artifact.PublicExcerpts) > 0) {
		return errors.New("inaccessible source must be explicit and unevidenced")
	}
	if artifact.State == "inaccessible" && (strings.TrimSpace(artifact.PublicMetadata.Title) != "" || strings.TrimSpace(artifact.PublicMetadata.Author) != "" || strings.TrimSpace(artifact.PublicMetadata.PublishedAt) != "" || len(artifact.RelatedURLs) > 0) {
		return errors.New("inaccessible source cannot contain invented public context")
	}
	return nil
}

func validateSourceJudgment(j SourceJudgment, source CanonicalURL, artifact LinkArtifact, profile LensProfile) error {
	if len(j.LensResults) != len(profile.Lenses) {
		return errors.New("missing_lens_result")
	}
	lensIDs := map[string]bool{}
	for _, lens := range profile.Lenses {
		lensIDs[lens.LensID] = true
	}
	seenLens := map[string]bool{}
	for _, result := range j.LensResults {
		if !lensIDs[result.LensID] || seenLens[result.LensID] || result.Confidence < 0 || result.Confidence > 1 || !boundedRationale(result.Rationale) {
			return errors.New("invalid_lens_result")
		}
		switch result.Result {
		case "matched", "not_matched":
		case "unknown":
			if len(result.Missingness) == 0 {
				return errors.New("unknown lens result requires missingness")
			}
		default:
			return errors.New("invalid_lens_result")
		}
		if !evidenceResolves(artifact, result.EvidenceRefs) {
			return errors.New("unresolved_evidence_ref")
		}
		seenLens[result.LensID] = true
	}
	roles := map[string]bool{"external_entity": true, "evidence_backed_finding": true, "unresolved_tension": true, "reference_resource": true, "action": true, "unknown": true}
	a := j.SemanticAssessment
	if !roles[a.PrimaryRole] || a.Confidence < 0 || a.Confidence > 1 || !boundedOptional(a.Summary) || !evidenceResolves(artifact, a.EvidenceRefs) {
		return errors.New("invalid semantic assessment")
	}
	if a.PrimaryRole == "unknown" {
		if len(a.Missingness) == 0 || strings.TrimSpace(a.Summary) != "" || len(a.EvidenceRefs) != 0 {
			return errors.New("unknown assessment must preserve missingness without invention")
		}
	} else if strings.TrimSpace(a.Summary) == "" || len(a.EvidenceRefs) == 0 {
		return errors.New("complete assessment requires public evidence")
	}
	switch j.Disposition {
	case "promote", "hold", "monitor", "archive", "clarify":
	default:
		return errors.New("invalid_disposition")
	}
	if !boundedRationale(j.DispositionRationale) {
		return errors.New("invalid disposition rationale")
	}
	if j.Disposition == "promote" && (source.EnrichmentState != "complete" || len(j.SemanticNodes) == 0) {
		return errors.New("incomplete_source_promoted")
	}
	if len(j.SemanticNodes) > 3 {
		return errors.New("constellation_limit_exceeded")
	}
	nodeIDs := map[string]bool{}
	for _, node := range j.SemanticNodes {
		if strings.TrimSpace(node.SemanticNodeID) == "" || nodeIDs[node.SemanticNodeID] || !roles[node.Role] || node.Role == "unknown" || strings.TrimSpace(node.Name) == "" || !boundedRationale(node.Description) || node.Confidence < 0 || node.Confidence > 1 || !evidenceResolves(artifact, node.EvidenceRefs) || len(node.EvidenceRefs) == 0 {
			return errors.New("invalid semantic node")
		}
		for _, lensRef := range node.LensRefs {
			if !lensIDs[lensRef] {
				return errors.New("invalid semantic node lens ref")
			}
		}
		nodeIDs[node.SemanticNodeID] = true
	}
	for _, edge := range j.SemanticEdges {
		if !nodeIDs[edge.From] || !nodeIDs[edge.To] || edge.Type != "related_to" || !boundedRationale(edge.Rationale) || len(edge.EvidenceRefs) == 0 || !evidenceResolves(artifact, edge.EvidenceRefs) {
			return errors.New("invalid_semantic_edge")
		}
	}
	return nil
}

func summarize(graph SourceGraph, decisions RouteDecisions, profile LensProfile) RouteSummary {
	return summarizeBound(graph, decisions, len(profile.Lenses), fingerprint(profile))
}

func summarizeBound(graph SourceGraph, decisions RouteDecisions, lensCount int, lensProfileFingerprint string) RouteSummary {
	summary := RouteSummary{
		SchemaVersion: SummarySchema, SourceGraphFingerprint: graph.Fingerprint, RouteDecisionsFingerprint: decisions.Fingerprint, LensProfileFingerprint: lensProfileFingerprint,
		InputRecordCount: len(graph.SourceRecords), URLOccurrenceCount: len(graph.URLOccurrences), CanonicalSourceCount: len(graph.CanonicalURLs), LensCount: lensCount,
		RequiredLensResultCount: len(graph.CanonicalURLs) * lensCount, DispositionCounts: map[string]int{}, EnrichmentStateCounts: map[string]int{}, SemanticNodeRoleCounts: map[string]int{}, SemanticEdgeTypeCounts: map[string]int{}, OperatorJudged: true,
		EvalProjection: EvalProjection{IntendedUsers: "workspace members reviewing private captured sources", InputSourceTypes: []string{"normalized private source captures", "normalized source-adapter portability fixtures"}, OutputSurfaces: []string{"destination-neutral routing artifacts", "selected destination adapter outputs"}, WorkspaceAssumptions: "user-defined context lenses and an explicitly selected destination profile", ProviderAssumptions: "operator/agent judgment manifest; no autonomous model claim", PrivacyBoundary: "private adapter provenance remains local; public evidence only may enter a destination outbox", SampleStatus: "private_curated_sample", HeldOut: false, Generalizable: false, Thresholds: []string{"complete source and URL accounting", "complete lens matrix", "zero outbound privacy findings"}, Guardrails: []string{"no incomplete source promotion", "no destination write from routing", "no autonomy or generalization claim"}},
	}
	primarySeen := map[string]int{}
	for _, canonical := range graph.CanonicalURLs {
		summary.EnrichmentStateCounts[canonical.EnrichmentState]++
		if canonical.Depth == 0 {
			summary.PrimaryCanonicalURLCount++
		} else if canonical.Depth == 1 {
			summary.DepthOneURLCount++
		}
	}
	for _, occurrence := range graph.URLOccurrences {
		primarySeen[occurrence.CanonicalURLID]++
	}
	for _, count := range primarySeen {
		if count > 1 {
			summary.DuplicateOccurrenceCount += count - 1
		}
	}
	for _, source := range decisions.Sources {
		summary.LensResultCount += len(source.LensResults)
		summary.DispositionCounts[source.Disposition]++
		for _, node := range source.SemanticNodes {
			summary.SemanticNodeRoleCounts[node.Role]++
		}
		for _, edge := range source.SemanticEdges {
			summary.SemanticEdgeTypeCounts[edge.Type]++
		}
	}
	summary.Fingerprint = fingerprint(summary)
	return summary
}

func fingerprint(value any) string {
	data, _ := json.Marshal(value)
	var raw any
	_ = json.Unmarshal(data, &raw)
	if object, ok := raw.(map[string]any); ok {
		delete(object, "fingerprint")
	}
	canonical, _ := json.Marshal(raw)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
func artifactHasEvidence(a LinkArtifact, ref string) bool {
	for _, e := range a.PublicExcerpts {
		if e.ExcerptID == ref {
			return true
		}
	}
	return false
}
func evidenceResolves(a LinkArtifact, refs []string) bool {
	for _, ref := range refs {
		if !artifactHasEvidence(a, ref) {
			return false
		}
	}
	return true
}
func boundedRationale(v string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(v))
	return n > 0 && n <= 1000
}
func boundedOptional(v string) bool { return utf8.RuneCountInString(v) <= 1000 }
func isSlug(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func cloneStrings(v []string) []string                { return append([]string{}, v...) }
func cloneExcerpts(v []PublicExcerpt) []PublicExcerpt { return append([]PublicExcerpt{}, v...) }
func cloneLensResults(v []LensResult) []LensResult    { return append([]LensResult{}, v...) }
func cloneNodes(v []SemanticNode) []SemanticNode      { return append([]SemanticNode{}, v...) }
func cloneEdges(v []SemanticEdge) []SemanticEdge      { return append([]SemanticEdge{}, v...) }

func SortJudgmentsForGraph(graph SourceGraph, judgments *Judgments) {
	order := map[string]int{}
	for i, c := range graph.CanonicalURLs {
		order[c.CanonicalURLID] = i
	}
	sort.SliceStable(judgments.Sources, func(i, j int) bool {
		return order[judgments.Sources[i].CanonicalURLID] < order[judgments.Sources[j].CanonicalURLID]
	})
}
