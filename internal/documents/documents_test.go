package documents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type httpRoundTripper func(*http.Request) (*http.Response, error)

func (fn httpRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type fakeLLMSemanticProvider struct {
	request LLMSemanticRequest
	calls   int
}

func (provider *fakeLLMSemanticProvider) Classify(request LLMSemanticRequest) (llmSemanticResponse, error) {
	provider.calls++
	provider.request = request
	evidenceNode := ""
	if len(request.Nodes) > 0 {
		evidenceNode = request.Nodes[0].NodeID
	}
	return llmSemanticResponse{Candidates: []llmSemanticCandidate{{
		Kind:          string(SemanticCandidateKindAction),
		Title:         "Prepare the migration checklist",
		Summary:       "Prepare the migration checklist using the cited structure node.",
		Confidence:    string(ConfidenceMedium),
		EvidenceNodes: []string{evidenceNode},
	}}}, nil
}

type fakeLLMSemanticReviewer struct {
	responses []llmSemanticReviewResponse
	requests  []LLMSemanticReviewRequest
}

func (reviewer *fakeLLMSemanticReviewer) ReviewSemanticJudgment(request LLMSemanticReviewRequest) (llmSemanticReviewResponse, error) {
	reviewer.requests = append(reviewer.requests, request)
	if len(reviewer.responses) == 0 {
		return llmSemanticReviewResponse{
			Choice:              string(SemanticJudgmentChoiceAccept),
			Confidence:          string(ConfidenceHigh),
			HumanReviewRequired: false,
			ReviewReasonCodes:   []string{string(SemanticAgentReviewReasonMachineTriaged)},
			Rationale:           "Evidence supports the candidate.",
		}, nil
	}
	response := reviewer.responses[0]
	reviewer.responses = reviewer.responses[1:]
	return response, nil
}

func TestGoldenMarkdownDecomposition(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		minCount   int
		typeCounts map[SemanticType]int
		status     map[ReviewStatus]int
	}{
		{
			name:     "transcript decision action",
			file:     "transcript-decision-action.md",
			minCount: 6,
			typeCounts: map[SemanticType]int{
				SemanticTypeMeetingNote: 1,
				SemanticTypeDecision:    1,
				SemanticTypeAction:      2,
				SemanticTypeTension:     1,
			},
		},
		{
			name:     "mixed thread capture",
			file:     "mixed-thread-capture.md",
			minCount: 7,
			typeCounts: map[SemanticType]int{
				SemanticTypeSourceNote: 1,
				SemanticTypeReference:  1,
				SemanticTypeAction:     1,
				SemanticTypeInsight:    1,
				SemanticTypeUnknown:    1,
			},
			status: map[ReviewStatus]int{
				ReviewStatusNeedsReview: 1,
				ReviewStatusBlocked:     1,
			},
		},
		{
			name:     "strategy capability table",
			file:     "strategy-capability-table.md",
			minCount: 7,
			typeCounts: map[SemanticType]int{
				SemanticTypeStandard:  2,
				SemanticTypeDecision:  1,
				SemanticTypeReference: 2,
				SemanticTypeWorkItem:  1,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			summary, err := DecomposePath(fixturePath(t, "markdown", tc.file), out)
			if err != nil {
				t.Fatalf("decompose: %v", err)
			}
			if summary.SchemaVersion != SegmentSummarySchemaVersion {
				t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
			}
			if summary.SegmentCount < tc.minCount {
				t.Fatalf("expected at least %d segments, got %d", tc.minCount, summary.SegmentCount)
			}
			for semanticType, min := range tc.typeCounts {
				if got := summary.TypeCounts[semanticType]; got < min {
					t.Fatalf("expected at least %d %s segments, got %d in %+v", min, semanticType, got, summary.TypeCounts)
				}
			}
			for status, min := range tc.status {
				if got := countStatus(t, out, summary, status); got < min {
					t.Fatalf("expected at least %d %s segments, got %d", min, status, got)
				}
			}
		})
	}
}

func TestGeneratedOutputMatchesGoldenFixtures(t *testing.T) {
	cases := []string{
		"transcript-decision-action",
		"mixed-thread-capture",
		"strategy-capability-table",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			if _, err := DecomposePath(fixturePath(t, "markdown", name+".md"), out); err != nil {
				t.Fatalf("decompose: %v", err)
			}
			assertTreeMatches(t,
				filepath.Join(out, "document-segments"),
				fixturePath(t, "expected", name, "document-segments"),
			)
		})
	}
}

func TestGeneratedStructureOutputMatchesGoldenFixtures(t *testing.T) {
	out := t.TempDir()
	if _, err := StructurePath(fixturePath(t, "structure"), out); err != nil {
		t.Fatalf("structure: %v", err)
	}
	assertTreeMatches(t,
		filepath.Join(out, "document-structure"),
		fixturePath(t, "expected", "structure"),
	)
}

func TestDocumentStructurePreservesHierarchyAndReviewStates(t *testing.T) {
	out := t.TempDir()
	summary, err := StructurePath(fixturePath(t, "structure"), out)
	if err != nil {
		t.Fatalf("structure: %v", err)
	}
	if summary.SchemaVersion != StructureSummarySchemaVersion {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.SourceCount != 7 {
		t.Fatalf("expected 7 sources, got %d", summary.SourceCount)
	}
	for nodeType, min := range map[StructureNodeType]int{
		StructureNodeTypeDocument:       7,
		StructureNodeTypeSection:        11,
		StructureNodeTypeTable:          3,
		StructureNodeTypeTableRow:       4,
		StructureNodeTypeCapability:     12,
		StructureNodeTypeTranscriptTurn: 4,
		StructureNodeTypeRequirement:    2,
		StructureNodeTypeWorkflow:       2,
		StructureNodeTypeAudience:       1,
		StructureNodeTypeUnknown:        2,
	} {
		if got := summary.NodeTypeCounts[nodeType]; got < min {
			t.Fatalf("expected at least %d %s nodes, got %d in %+v", min, nodeType, got, summary.NodeTypeCounts)
		}
	}
	if summary.NeedsReviewCount < 2 {
		t.Fatalf("expected needs_review nodes, got %+v", summary)
	}
	for _, node := range summary.Nodes {
		if !strings.Contains(node.NodePath, "broken-table") {
			continue
		}
		if (node.NodeType == StructureNodeTypeTable || node.NodeType == StructureNodeTypeTableRow) && node.ReviewStatus == ReviewStatusReady {
			t.Fatalf("malformed table node must need review, got %+v", node)
		}
	}
	if summary.BlockedCount < 2 {
		t.Fatalf("expected blocked unsafe nodes, got %+v", summary)
	}
	assertStructureNodePath(t, summary, StructureNodeTypeDocument, "process-no-h1-capabilities")
	assertStructureNodePath(t, summary, StructureNodeTypeSection, "process-no-h1-capabilities/essential-master-data-access")
	assertStructureNodePath(t, summary, StructureNodeTypeSection, "process-no-h1-capabilities/programme-rulebook")
	assertMissingStructureNodePath(t, summary, "process-no-h1-capabilities/essential-master-data-access/programme-rulebook")
	assertStructureNodePath(t, summary, StructureNodeTypeCapability, "process-no-h1-capabilities/essential-master-data-access/pl-1-access-and-central-entry")
	assertStructureNodePath(t, summary, StructureNodeTypeCapability, "process-no-h1-capabilities/essential-master-data-access/pl-23-contacts-and-relationships")
	assertStructureNodePath(t, summary, StructureNodeTypeCapability, "process-no-h1-capabilities/essential-master-data-access/p-s1-maintain-chemical-inventory")
	assertStructureNodePath(t, summary, StructureNodeTypeCapability, "process-no-h1-capabilities/programme-rulebook/table-programme-rulebook/pl-23-contacts-and-relationships/pl-23-contacts-and-relationships")
	assertStructureNodePath(t, summary, StructureNodeTypeCapability, "process-no-h1-capabilities/programme-rulebook/table-programme-rulebook/pl-10-12-rulebook-stewardship/pl-10-12-rulebook-stewardship")
	assertMissingStructureNodePath(t, summary, "abc-1-not-a-capability")
	assertMissingStructureNodePath(t, summary, "this-sentence-merely-mentions-pl-1-without-defining-it")
	assertStructureNodeTitle(t, out, summary, StructureNodeTypeCapability, "process-no-h1-capabilities/essential-master-data-access/pl-23-contacts-and-relationships", "PL-23 - Contacts and relationships")
	assertTranscriptTurnEvidence(t, out, summary)
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-structure"), "private_content", "secret", "authority_ids", "authority_id", "example private person")
}

func TestDocumentStructureDeterministicAcrossRuns(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	if _, err := StructurePath(fixturePath(t, "structure"), first); err != nil {
		t.Fatalf("first structure: %v", err)
	}
	if _, err := StructurePath(fixturePath(t, "structure"), second); err != nil {
		t.Fatalf("second structure: %v", err)
	}
	assertTreeMatches(t, filepath.Join(first, "document-structure"), filepath.Join(second, "document-structure"))
}

func TestSemanticCandidateArtifactsFromMarkdownDirectory(t *testing.T) {
	out := t.TempDir()
	summary, err := SemanticPath(fixturePath(t, "semantic"), out)
	if err != nil {
		t.Fatalf("semantic: %v", err)
	}
	if summary.SchemaVersion != SemanticSummarySchemaVersion {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.SourceCount != 3 {
		t.Fatalf("expected 3 sources, got %d", summary.SourceCount)
	}
	if summary.ObservationCount == 0 || summary.CandidateCount == 0 || summary.RelationCount == 0 {
		t.Fatalf("expected observations, candidates, and relations, got %+v", summary)
	}
	if summary.CandidateKindCounts[SemanticCandidateKindAction] == 0 {
		t.Fatalf("expected action candidate, got %+v", summary.CandidateKindCounts)
	}
	if summary.CandidateKindCounts[SemanticCandidateKindCapability] == 0 {
		t.Fatalf("expected capability candidate, got %+v", summary.CandidateKindCounts)
	}
	if summary.RelationshipTypeCounts[SemanticRelationshipDerivedFrom] == 0 {
		t.Fatalf("expected derived_from relation, got %+v", summary.RelationshipTypeCounts)
	}
	if _, err := os.Stat(filepath.Join(out, "document-structure", "structure-summary.json")); err != nil {
		t.Fatalf("expected semantic markdown directory run to persist document structure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "semantic-candidates", "semantic-summary.json")); err != nil {
		t.Fatalf("missing semantic summary artifact: %v", err)
	}
	for _, item := range summary.Candidates {
		if _, err := os.Stat(filepath.Join(out, "semantic-candidates", item.CandidatePath)); err != nil {
			t.Fatalf("missing candidate artifact %s: %v", item.CandidatePath, err)
		}
		if _, err := os.Stat(filepath.Join(out, "semantic-candidates", item.PreviewPath)); err != nil {
			t.Fatalf("missing candidate preview %s: %v", item.PreviewPath, err)
		}
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "semantic-candidates"), "productbrain", "product brain", "tolaria", "notion", "obsidian", "slack", "authority_ids", "wp-13", "dec-49", ": null")
}

func TestSemanticTranscriptConsolidationAndContradiction(t *testing.T) {
	out := t.TempDir()
	summary, err := SemanticPath(fixturePath(t, "semantic"), out)
	if err != nil {
		t.Fatalf("semantic: %v", err)
	}
	var readyAction bool
	var contradictedNeedsReview bool
	for _, item := range summary.Candidates {
		data, err := os.ReadFile(filepath.Join(out, "semantic-candidates", item.CandidatePath))
		if err != nil {
			t.Fatalf("read candidate %s: %v", item.CandidatePath, err)
		}
		var candidate SemanticCandidate
		if err := json.Unmarshal(data, &candidate); err != nil {
			t.Fatalf("decode candidate %s: %v", item.CandidatePath, err)
		}
		if candidate.CandidateKind == SemanticCandidateKindAction && candidate.ReviewStatus == ReviewStatusReady && len(candidate.ObservationIDs) >= 2 && len(candidate.RelationIDs) > 0 {
			readyAction = true
		}
		if candidate.CandidateKind == SemanticCandidateKindIssue && candidate.ReviewStatus == ReviewStatusNeedsReview && len(candidate.Blockers) > 0 {
			contradictedNeedsReview = true
		}
		if candidate.DestinationStatus != SemanticDestinationUnresolved {
			t.Fatalf("destination status must stay unresolved, got %+v", candidate)
		}
	}
	if !readyAction {
		t.Fatalf("expected ready consolidated action candidate in %+v", summary.Candidates)
	}
	if !contradictedNeedsReview {
		t.Fatalf("expected contradicted transcript candidate to remain needs_review in %+v", summary.Candidates)
	}
}

func TestSemanticStructureRunInputPreservesTranscriptOutcomes(t *testing.T) {
	structureOut := t.TempDir()
	if _, err := StructurePath(fixturePath(t, "semantic"), structureOut); err != nil {
		t.Fatalf("structure: %v", err)
	}
	out := t.TempDir()
	summary, err := SemanticPath(filepath.Join(structureOut, "document-structure"), out)
	if err != nil {
		t.Fatalf("semantic from structure: %v", err)
	}
	var readyAction bool
	var contradictedNeedsReview bool
	for _, item := range summary.Candidates {
		data, err := os.ReadFile(filepath.Join(out, "semantic-candidates", item.CandidatePath))
		if err != nil {
			t.Fatalf("read candidate %s: %v", item.CandidatePath, err)
		}
		var candidate SemanticCandidate
		if err := json.Unmarshal(data, &candidate); err != nil {
			t.Fatalf("decode candidate %s: %v", item.CandidatePath, err)
		}
		if candidate.CandidateKind == SemanticCandidateKindAction && candidate.ReviewStatus == ReviewStatusReady {
			readyAction = true
		}
		if candidate.CandidateKind == SemanticCandidateKindIssue && candidate.ReviewStatus == ReviewStatusNeedsReview {
			contradictedNeedsReview = true
		}
	}
	if !readyAction || !contradictedNeedsReview {
		t.Fatalf("expected structure-run input to preserve ready action and needs_review issue, got %+v", summary.Candidates)
	}
}

func TestSemanticWriterRejectsDuplicateAndStaleGeneratedFiles(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	relation := validSemanticRelation(candidate, observation, node)
	out := t.TempDir()
	stale := filepath.Join(out, "semantic-candidates", "candidates", "stale.json")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("mkdir stale parent: %v", err)
	}
	if err := os.WriteFile(stale, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if err := WriteSemantic(out, "run-sem-demo", 1, []SemanticObservation{observation}, []SemanticCandidate{candidate}, []SemanticRelation{relation}); err == nil {
		t.Fatalf("expected stale generated file rejection")
	}
	if err := WriteSemantic(t.TempDir(), "run-sem-demo", 1, []SemanticObservation{observation, observation}, []SemanticCandidate{candidate}, []SemanticRelation{relation}); err == nil {
		t.Fatalf("expected duplicate observation id rejection")
	}
}

func TestSemanticWriterRedactsUnsafeEndpointAndEvidenceFields(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	observation.ObservationID = "obs-private_content"
	observation.SourceDocumentID = "doc-private_content"
	observation.EvidenceNodes = []string{"node-safe", "DEC-49"}
	observation.Title = "Unsafe semantic observation"
	observation.Summary = "Contains private_content marker."
	candidate := validSemanticCandidate(observation, node)
	candidate.CandidateID = "cand-secret-" + unsafeTokenMarker()
	candidate.EvidenceNodes = []string{"node-safe", "WP-13"}
	candidate.ObservationIDs = []string{observation.ObservationID, "obs-secret-" + unsafeTokenMarker()}
	candidate.Summary = "secret candidate summary"
	relation := validSemanticRelation(candidate, observation, node)
	relation.RelationID = "rel-private_content"
	relation.FromID = "cand-secret-" + unsafeTokenMarker()
	relation.ToID = "DEC-49"
	relation.EvidenceNodes = []string{"node-safe", "private_content-node"}

	out := t.TempDir()
	if err := WriteSemantic(out, "run-sem-demo", 1, []SemanticObservation{observation}, []SemanticCandidate{candidate}, []SemanticRelation{relation}); err != nil {
		t.Fatalf("write semantic: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "semantic-candidates"), "private_content", "secret", unsafeTokenMarker(), "DEC-49", "WP-13")
}

func TestLLMClassifierRejectsInventedEvidenceNode(t *testing.T) {
	nodes := []StructureNode{{
		NodeID:           "node-real",
		SourceDocumentID: "doc-test",
		Evidence: StructureEvidence{
			LineStart: 1,
			LineEnd:   1,
		},
	}}
	response := llmSemanticResponse{Candidates: []llmSemanticCandidate{{
		Kind:          string(SemanticCandidateKindAction),
		Title:         "Do it",
		Summary:       "Do it",
		Confidence:    string(ConfidenceMedium),
		EvidenceNodes: []string{"node-fake"},
	}}}

	_, _, err := buildLLMSemanticArtifacts("run-test", nodes, response)

	if err == nil || !strings.Contains(err.Error(), "unknown evidence node: node-fake") {
		t.Fatalf("expected unknown evidence node rejection, got %v", err)
	}
}

func TestLLMClassifierBuildsProviderNeutralArtifacts(t *testing.T) {
	nodes := []StructureNode{{
		NodeID:           "node-real",
		SourceDocumentID: "doc-test",
		Evidence: StructureEvidence{
			LineStart: 3,
			LineEnd:   5,
		},
	}}
	response := llmSemanticResponse{Candidates: []llmSemanticCandidate{{
		Kind:          string(SemanticCandidateKindDecision),
		Title:         "Adopt the review gate",
		Summary:       "The team decided to use review gates before publishing.",
		Confidence:    string(ConfidenceHigh),
		EvidenceNodes: []string{"node-real"},
	}}}

	candidates, relations, err := buildLLMSemanticArtifacts("run-test", nodes, response)

	if err != nil {
		t.Fatalf("build LLM artifacts: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.CandidateKind != SemanticCandidateKindDecision {
		t.Fatalf("unexpected candidate kind: %s", candidate.CandidateKind)
	}
	if candidate.DestinationStatus != SemanticDestinationUnresolved {
		t.Fatalf("expected unresolved destination, got %s", candidate.DestinationStatus)
	}
	if candidate.SourceDocumentID != "doc-test" {
		t.Fatalf("unexpected source: %s", candidate.SourceDocumentID)
	}
	if len(candidate.ObservationIDs) == 0 || len(candidate.RelationIDs) == 0 {
		t.Fatalf("expected observation and relation IDs: %+v", candidate)
	}
	if len(relations) != 1 || relations[0].RelationshipType != SemanticRelationshipDerivedFrom {
		t.Fatalf("expected derived_from relation, got %+v", relations)
	}
}

func TestLLMClassifierDisambiguatesDuplicateCandidateOutput(t *testing.T) {
	nodes := []StructureNode{{
		NodeID:           "node-real",
		SourceDocumentID: "doc-test",
		Evidence: StructureEvidence{
			LineStart: 3,
			LineEnd:   5,
		},
	}}
	response := llmSemanticResponse{Candidates: []llmSemanticCandidate{
		{
			Kind:          string(SemanticCandidateKindAction),
			Title:         "Prepare evidence pack",
			Summary:       "Prepare the evidence pack from the cited node.",
			Confidence:    string(ConfidenceMedium),
			EvidenceNodes: []string{"node-real"},
		},
		{
			Kind:          string(SemanticCandidateKindAction),
			Title:         "Prepare evidence pack",
			Summary:       "Prepare the evidence pack from the cited node.",
			Confidence:    string(ConfidenceMedium),
			EvidenceNodes: []string{"node-real"},
		},
	}}

	observations, candidates, relations, err := buildLLMSemanticObservationsAndArtifacts("run-test", nodes, LLMSemanticRequest{}, response)

	if err != nil {
		t.Fatalf("build duplicate LLM artifacts: %v", err)
	}
	if len(observations) != 2 || len(candidates) != 2 || len(relations) != 2 {
		t.Fatalf("expected two observations, candidates, and relations; got obs=%d candidates=%d relations=%d", len(observations), len(candidates), len(relations))
	}
	if observations[0].ObservationID == observations[1].ObservationID {
		t.Fatalf("expected duplicate LLM observations to get distinct IDs: %+v", observations)
	}
	if candidates[0].CandidateID == candidates[1].CandidateID {
		t.Fatalf("expected duplicate LLM candidates to get distinct IDs: %+v", candidates)
	}
	if relations[0].RelationID == relations[1].RelationID {
		t.Fatalf("expected duplicate LLM relations to get distinct IDs: %+v", relations)
	}
	if err := WriteSemantic(t.TempDir(), "run-test", 1, observations, candidates, relations); err != nil {
		t.Fatalf("duplicate LLM artifacts should remain writable: %v", err)
	}
}

func TestOpenAIProviderPostsResponsesRequestAndParsesCandidates(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	var capturedBody map[string]any
	provider := NewOpenAIProvider("sk-test", "gpt-test", httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		capturedPath = req.URL.Path
		capturedAuth = req.Header.Get("Authorization")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if bytes.Contains(body, []byte("sk-test")) {
			t.Fatalf("request body must not contain API key: %s", string(body))
		}
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(strings.NewReader(`{
				"output": [{
					"content": [{
						"type": "output_text",
						"text": "{\"candidates\":[{\"kind\":\"action_candidate\",\"title\":\"Prepare rollout checklist\",\"summary\":\"Prepare the rollout checklist from the cited evidence.\",\"confidence\":\"medium\",\"evidence_nodes\":[\"node-1\"]}]}"
					}]
				}]
			}`)),
			Header: make(http.Header),
		}, nil
	}))
	request := LLMSemanticRequest{
		SourceDocumentID: "doc-test",
		Nodes: []LLMSemanticNode{{
			NodeID: "node-1",
			Text:   "Prepare the rollout checklist.",
		}},
	}

	response, err := provider.Classify(request)

	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if capturedPath != "/v1/responses" {
		t.Fatalf("expected responses endpoint, got %s", capturedPath)
	}
	if capturedAuth != "Bearer sk-test" {
		t.Fatalf("expected authorization header")
	}
	if got := int(capturedBody["max_output_tokens"].(float64)); got != openAISemanticMaxOutputTokens {
		t.Fatalf("expected semantic response token budget %d, got %d", openAISemanticMaxOutputTokens, got)
	}
	if len(response.Candidates) != 1 || response.Candidates[0].Kind != string(SemanticCandidateKindAction) {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestOpenAIProviderRejectsMalformedCandidateJSON(t *testing.T) {
	provider := NewOpenAIProvider("sk-test", "gpt-test", httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(strings.NewReader(`{
				"output": [{
					"content": [{
						"type": "output_text",
						"text": "not-json"
					}]
				}]
			}`)),
			Header: make(http.Header),
		}, nil
	}))

	_, err := provider.Classify(LLMSemanticRequest{SourceDocumentID: "doc-test"})

	if err == nil || !strings.Contains(err.Error(), "parse OpenAI semantic response") {
		t.Fatalf("expected malformed JSON rejection, got %v", err)
	}
}

func TestConsolidateSemanticCandidatesKeepsRedactedCandidateRelationEndpoint(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	observation.ObservationKind = SemanticObservationKindCapabilityStatement
	observation.Title = "Capability contains " + unsafeTokenMarker()
	observation.Summary = "Safe observation summary."

	candidates, relations := ConsolidateSemanticCandidates("run-sem-demo", []SemanticObservation{observation})
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d", len(candidates))
	}
	if len(relations) != 1 {
		t.Fatalf("relation count = %d", len(relations))
	}

	candidate := candidates[0]
	relation := relations[0]
	if candidate.ReviewStatus != ReviewStatusBlocked {
		t.Fatalf("candidate review status = %s", candidate.ReviewStatus)
	}
	if relation.ReviewStatus != ReviewStatusBlocked {
		t.Fatalf("relation review status = %s", relation.ReviewStatus)
	}
	if relation.FromType != SemanticRelationEndpointCandidate {
		t.Fatalf("relation from type = %s", relation.FromType)
	}
	if relation.FromID != candidate.CandidateID {
		t.Fatalf("relation from_id = %q, candidate_id = %q", relation.FromID, candidate.CandidateID)
	}
}

func TestSemanticArtifactsReferenceInspectableStructureNodes(t *testing.T) {
	out := t.TempDir()
	summary, err := SemanticPath(fixturePath(t, "semantic"), out)
	if err != nil {
		t.Fatalf("semantic: %v", err)
	}
	structureNodes := map[string]bool{}
	structureRoot := filepath.Join(out, "document-structure", "nodes")
	if err := filepath.WalkDir(structureRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		var node StructureNode
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}
		structureNodes[node.NodeID] = true
		return nil
	}); err != nil {
		t.Fatalf("read structure nodes: %v", err)
	}
	for _, item := range summary.Candidates {
		data, err := os.ReadFile(filepath.Join(out, "semantic-candidates", item.CandidatePath))
		if err != nil {
			t.Fatalf("read candidate: %v", err)
		}
		var candidate SemanticCandidate
		if err := json.Unmarshal(data, &candidate); err != nil {
			t.Fatalf("decode candidate: %v", err)
		}
		if len(candidate.EvidenceNodes) == 0 {
			t.Fatalf("candidate missing evidence nodes: %+v", candidate)
		}
		for _, nodeID := range candidate.EvidenceNodes {
			if !structureNodes[nodeID] {
				t.Fatalf("candidate references non-inspectable structure node %s", nodeID)
			}
		}
	}
}

func TestGeneratedSemanticOutputMatchesGoldenFixtures(t *testing.T) {
	out := t.TempDir()
	if _, err := SemanticPath(fixturePath(t, "semantic"), out); err != nil {
		t.Fatalf("semantic: %v", err)
	}
	assertTreeMatches(t,
		filepath.Join(out, "semantic-candidates"),
		fixturePath(t, "expected", "semantic", "semantic-candidates"),
	)
}

func TestDocumentSemanticsDeterministicAcrossRuns(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	if _, err := SemanticPath(fixturePath(t, "semantic"), first); err != nil {
		t.Fatalf("first semantic: %v", err)
	}
	if _, err := SemanticPath(fixturePath(t, "semantic"), second); err != nil {
		t.Fatalf("second semantic: %v", err)
	}
	assertTreeMatches(t, filepath.Join(first, "semantic-candidates"), filepath.Join(second, "semantic-candidates"))
}

func TestSemanticPathWithLLMProviderWritesSemanticArtifacts(t *testing.T) {
	out := t.TempDir()
	provider := &fakeLLMSemanticProvider{}

	summary, err := SemanticPathWithOptions(fixturePath(t, "semantic"), out, SemanticOptions{
		Classifier:  SemanticClassifierLLM,
		LLMProvider: "openai",
		LLMModel:    "fake-model",
		LLMAPIKey:   "fake-key",
		LLMClient:   provider,
	})

	if err != nil {
		t.Fatalf("semantic LLM path: %v", err)
	}
	if provider.request.SourceDocumentID == "" || len(provider.request.Nodes) == 0 {
		t.Fatalf("expected provider request with source and nodes: %+v", provider.request)
	}
	if summary.CandidateCount != 1 {
		t.Fatalf("expected one LLM candidate, got %+v", summary)
	}
	if got := summary.CandidateKindCounts[SemanticCandidateKindAction]; got != 1 {
		t.Fatalf("expected action candidate count 1, got %d in %+v", got, summary.CandidateKindCounts)
	}
	if _, err := os.Stat(filepath.Join(out, "semantic-candidates", "semantic-summary.json")); err != nil {
		t.Fatalf("expected semantic summary: %v", err)
	}
	previews, err := filepath.Glob(filepath.Join(out, "semantic-candidates", "previews", "*.md"))
	if err != nil || len(previews) != 1 {
		t.Fatalf("expected one semantic preview, previews=%v err=%v", previews, err)
	}
	previewBody, err := os.ReadFile(previews[0])
	if err != nil {
		t.Fatalf("read semantic preview: %v", err)
	}
	if !strings.Contains(string(previewBody), "## Evidence") || !strings.Contains(string(previewBody), "Requirement: every source") {
		t.Fatalf("expected inline evidence excerpt in preview:\n%s", string(previewBody))
	}
}

func TestSemanticPathWithLLMProviderFailsBeforeWritingStructureArtifacts(t *testing.T) {
	source := filepath.Join(t.TempDir(), "private.md")
	if err := os.WriteFile(source, []byte("# Private\n\nDo not read or write artifacts without LLM config.\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()

	_, err := SemanticPathWithOptions(source, out, SemanticOptions{
		Classifier:  SemanticClassifierLLM,
		LLMProvider: "openai",
	})

	if err == nil || !strings.Contains(err.Error(), "missing OpenAI model") {
		t.Fatalf("expected missing model before artifact writes, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(out, "document-structure")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no structure artifacts before LLM config validation, stat err=%v", statErr)
	}
}

func TestSemanticPathWithLLMProviderExplainsAllBlockedInput(t *testing.T) {
	source := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(source, []byte("# WP-17 Review\n\nDEC-75 private review material should be blocked.\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()
	provider := &fakeLLMSemanticProvider{}

	summary, err := SemanticPathWithOptions(source, out, SemanticOptions{
		Classifier:  SemanticClassifierLLM,
		LLMProvider: "openai",
		LLMModel:    "fake-model",
		LLMAPIKey:   "fake-key",
		LLMClient:   provider,
	})

	if err != nil {
		t.Fatalf("semantic LLM path: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("expected all-blocked input to skip provider call, got %d calls", provider.calls)
	}
	if summary.CandidateCount != 0 || !strings.Contains(summary.SkippedReason, "all structure nodes are blocked") {
		t.Fatalf("expected explicit skipped reason, got %+v", summary)
	}
	data, err := os.ReadFile(filepath.Join(out, "semantic-candidates", "semantic-summary.json"))
	if err != nil {
		t.Fatalf("read semantic summary: %v", err)
	}
	if !strings.Contains(string(data), "skipped_reason") {
		t.Fatalf("expected persisted skipped reason:\n%s", string(data))
	}
}

func TestLLMClassifierAcceptsBareUniqueEvidenceNodeSuffix(t *testing.T) {
	nodes := []StructureNode{{
		NodeID:           "node-abc123",
		SourceDocumentID: "doc",
		Evidence: StructureEvidence{
			LineStart: 1,
			LineEnd:   1,
		},
	}}

	candidates, _, err := buildLLMSemanticArtifacts("run", nodes, llmSemanticResponse{Candidates: []llmSemanticCandidate{{
		Kind:          string(SemanticCandidateKindDecision),
		Title:         "Use review pagination",
		Summary:       "Use review pagination for human acceptance.",
		Confidence:    string(ConfidenceHigh),
		EvidenceNodes: []string{"abc123"},
	}}})

	if err != nil {
		t.Fatalf("expected bare node suffix to resolve: %v", err)
	}
	if len(candidates) != 1 || candidates[0].EvidenceNodes[0] != "node-abc123" {
		t.Fatalf("expected normalized evidence node, got %+v", candidates)
	}
}

func TestLLMClassifierDeduplicatesRepeatedEvidenceNodes(t *testing.T) {
	nodes := []StructureNode{{
		NodeID:           "node-abc123",
		SourceDocumentID: "doc",
		Evidence: StructureEvidence{
			LineStart: 1,
			LineEnd:   1,
		},
	}}

	request := LLMSemanticRequest{Nodes: []LLMSemanticNode{{NodeID: "node-abc123", Text: "Prepare the evidence pack from the real source excerpt."}}}
	observations, candidates, relations, err := buildLLMSemanticObservationsAndArtifacts("run", nodes, request, llmSemanticResponse{Candidates: []llmSemanticCandidate{{
		Kind:          string(SemanticCandidateKindAction),
		Title:         "Prepare evidence pack",
		Summary:       "Prepare the evidence pack from the cited node.",
		Confidence:    string(ConfidenceMedium),
		EvidenceNodes: []string{"node-abc123", "abc123", "node-abc123"},
	}}})

	if err != nil {
		t.Fatalf("expected repeated evidence nodes to deduplicate: %v", err)
	}
	if len(observations) != 1 || len(candidates) != 1 || len(relations) != 1 {
		t.Fatalf("expected one observation, candidate, and relation; got obs=%d candidates=%d relations=%d", len(observations), len(candidates), len(relations))
	}
	if len(candidates[0].EvidenceNodes) != 1 || candidates[0].EvidenceNodes[0] != "node-abc123" {
		t.Fatalf("expected one normalized evidence node, got %+v", candidates[0].EvidenceNodes)
	}
	if len(candidates[0].EvidenceExcerpts) != 1 || !strings.Contains(candidates[0].EvidenceExcerpts[0].Text, "real source excerpt") {
		t.Fatalf("expected evidence excerpt, got %+v", candidates[0].EvidenceExcerpts)
	}
}

func TestDocumentStructureDuplicateBasenamesAreRelativePathDeterministic(t *testing.T) {
	firstRoot := duplicateStructureTree(t)
	secondRoot := duplicateStructureTree(t)
	firstOut := t.TempDir()
	secondOut := t.TempDir()
	if _, err := StructurePath(firstRoot, firstOut); err != nil {
		t.Fatalf("first structure: %v", err)
	}
	if _, err := StructurePath(secondRoot, secondOut); err != nil {
		t.Fatalf("second structure: %v", err)
	}
	assertTreeMatches(t, filepath.Join(firstOut, "document-structure"), filepath.Join(secondOut, "document-structure"))
}

func TestDocumentStructureDisambiguatesSanitizedBasenameCollisions(t *testing.T) {
	root := t.TempDir()
	body := []byte("# Shared Title\n\nCapability: preserve source identity.\n")
	for _, name := range []string{"a b.md", "a-b.md"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	out := t.TempDir()
	summary, err := StructurePath(root, out)
	if err != nil {
		t.Fatalf("structure: %v", err)
	}

	sourceIDs := map[string]bool{}
	for _, node := range summary.Nodes {
		if node.NodeType == StructureNodeTypeDocument {
			sourceIDs[node.SourceDocumentID] = true
		}
	}
	if len(sourceIDs) != 2 {
		t.Fatalf("expected two disambiguated source document ids, got %+v", sourceIDs)
	}
	for sourceID := range sourceIDs {
		if !strings.HasPrefix(sourceID, "doc-a-b-") {
			t.Fatalf("expected sanitized disambiguated id, got %s", sourceID)
		}
	}
}

func TestDocumentStructureUnsafeSourceIDsMatchDecomposeOutput(t *testing.T) {
	root := t.TempDir()
	inputPath := filepath.Join(root, "PRIVATE_CONTENT.md")
	if err := os.WriteFile(inputPath, []byte("# Public Title\n\nSafe body.\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	decomposeOut := t.TempDir()
	decomposeSummary, err := DecomposePath(inputPath, decomposeOut)
	if err != nil {
		t.Fatalf("decompose: %v", err)
	}
	if len(decomposeSummary.Segments) == 0 {
		t.Fatalf("expected decompose segments")
	}
	wantSourceID := decomposeSummary.Segments[0].SourceDocumentID

	structureOut := t.TempDir()
	structureSummary, err := StructurePath(inputPath, structureOut)
	if err != nil {
		t.Fatalf("structure: %v", err)
	}
	for _, node := range structureSummary.Nodes {
		if node.NodeType == StructureNodeTypeDocument && node.SourceDocumentID != wantSourceID {
			t.Fatalf("expected structure source id %s to match decompose, got %s", wantSourceID, node.SourceDocumentID)
		}
	}
}

func TestDocumentStructurePreservesRepeatedHeadingSections(t *testing.T) {
	root := t.TempDir()
	inputPath := filepath.Join(root, "repeated-headings.md")
	body := "# Root\n\n## Notes\n\n- Capability: first repeated section\n\n## Notes\n\n- Capability: second repeated section\n"
	if err := os.WriteFile(inputPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out := t.TempDir()
	summary, err := StructurePath(inputPath, out)
	if err != nil {
		t.Fatalf("structure: %v", err)
	}

	var notesSections []StructureNode
	for _, item := range summary.Nodes {
		if item.NodeType != StructureNodeTypeSection {
			continue
		}
		data, err := os.ReadFile(filepath.Join(out, "document-structure", StructureNodeJSONPath(item.NodeID)))
		if err != nil {
			t.Fatalf("read node %s: %v", item.NodeID, err)
		}
		var node StructureNode
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatalf("decode node %s: %v", item.NodeID, err)
		}
		if node.Title == "Notes" {
			notesSections = append(notesSections, node)
		}
	}
	if len(notesSections) != 2 {
		t.Fatalf("expected two distinct repeated Notes sections, got %+v", notesSections)
	}

	parentIDsByTitle := map[string]string{}
	for _, item := range summary.Nodes {
		if item.NodeType != StructureNodeTypeCapability {
			continue
		}
		data, err := os.ReadFile(filepath.Join(out, "document-structure", StructureNodeJSONPath(item.NodeID)))
		if err != nil {
			t.Fatalf("read capability %s: %v", item.NodeID, err)
		}
		var node StructureNode
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatalf("decode capability %s: %v", item.NodeID, err)
		}
		parentIDsByTitle[node.Title] = node.ParentNodeID
	}
	if parentIDsByTitle["first repeated section"] == "" || parentIDsByTitle["second repeated section"] == "" {
		t.Fatalf("expected repeated section capabilities, got %+v", parentIDsByTitle)
	}
	if parentIDsByTitle["first repeated section"] == parentIDsByTitle["second repeated section"] {
		t.Fatalf("expected repeated section capabilities to have distinct parents, got %+v", parentIDsByTitle)
	}
}

func TestDocumentStructureRelatedSegmentIDsMatchDecomposeOutput(t *testing.T) {
	root := duplicateStructureTree(t)
	decomposeOut := t.TempDir()
	decomposeSummary, err := DecomposePath(root, decomposeOut)
	if err != nil {
		t.Fatalf("decompose: %v", err)
	}
	segmentIDs := map[string]bool{}
	for _, segment := range decomposeSummary.Segments {
		segmentIDs[segment.SegmentID] = true
	}

	structureOut := t.TempDir()
	structureSummary, err := StructurePath(root, structureOut)
	if err != nil {
		t.Fatalf("structure: %v", err)
	}
	relatedCount := 0
	for _, item := range structureSummary.Nodes {
		data, err := os.ReadFile(filepath.Join(structureOut, "document-structure", StructureNodeJSONPath(item.NodeID)))
		if err != nil {
			t.Fatalf("read structure node %s: %v", item.NodeID, err)
		}
		var node StructureNode
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatalf("decode structure node %s: %v", item.NodeID, err)
		}
		for _, segmentID := range node.RelatedSegmentIDs {
			relatedCount++
			if !segmentIDs[segmentID] {
				t.Fatalf("structure node %s related unknown segment %s; known=%+v", item.NodeID, segmentID, segmentIDs)
			}
		}
	}
	if relatedCount == 0 {
		t.Fatalf("expected structure nodes to relate to decomposed segments")
	}
}

func TestStructureWriterRejectsUnexpectedExistingFile(t *testing.T) {
	out := t.TempDir()
	stale := filepath.Join(out, "document-structure", "nodes", "stale.json")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("mkdir stale parent: %v", err)
	}
	if err := os.WriteFile(stale, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if err := WriteStructure(out, "run-struct-demo", 1, []StructureNode{validStructureNode()}); err == nil {
		t.Fatalf("expected stale generated file rejection")
	}
}

func TestStructureWriterRejectsSymlinkedGeneratedPath(t *testing.T) {
	out := t.TempDir()
	root := filepath.Join(out, "document-structure")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "nodes")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := WriteStructure(out, "run-struct-demo", 1, []StructureNode{validStructureNode()}); err == nil {
		t.Fatalf("expected symlink write error")
	}
}

func TestStructureWriterRejectsDuplicateNodeIDs(t *testing.T) {
	node := validStructureNode()
	if err := WriteStructure(t.TempDir(), "run-struct-demo", 1, []StructureNode{node, node}); err == nil {
		t.Fatalf("expected duplicate structure node id error")
	}
}

func TestStructureWriterRedactsUnsafeProvidedNode(t *testing.T) {
	out := t.TempDir()
	node := validStructureNode()
	node.Title = "PRIVATE_CONTENT node"
	node.Summary = "secret " + unsafeTokenMarker() + " body"
	node.SourceDocumentID = "doc-secret-" + unsafeTokenMarker()
	node.Evidence.SourceDocumentID = node.SourceDocumentID
	node.Evidence.HeadingPath = []string{"DEC-29 unsafe heading"}
	node.NodePath = "WP-11/private"
	if err := WriteStructure(out, "run-struct-demo", 1, []StructureNode{node}); err != nil {
		t.Fatalf("write structure: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-structure"), "private_content", "secret", unsafeTokenMarker(), "DEC-29", "WP-11")
}

func TestStructureWriterSerializesEmptyRelatedSegmentLists(t *testing.T) {
	out := t.TempDir()
	node := validStructureNode()
	node.RelatedSegmentIDs = nil
	node.Evidence.RelatedSegmentIDs = nil
	if err := WriteStructure(out, "run-struct-demo", 1, []StructureNode{node}); err != nil {
		t.Fatalf("write structure: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "document-structure", StructureNodeJSONPath(node.NodeID)))
	if err != nil {
		t.Fatalf("read structure node: %v", err)
	}
	body := string(data)
	if strings.Contains(body, `"related_segment_ids": null`) {
		t.Fatalf("expected related segment ids to serialize as arrays:\n%s", body)
	}
	if !strings.Contains(body, `"related_segment_ids": []`) {
		t.Fatalf("expected empty related segment array:\n%s", body)
	}
}

func TestDocumentsDecomposeWritesCompleteArtifactTree(t *testing.T) {
	out := t.TempDir()
	summary, err := DecomposePath(fixturePath(t, "markdown", "mixed-thread-capture.md"), out)
	if err != nil {
		t.Fatalf("decompose: %v", err)
	}
	root := filepath.Join(out, "document-segments")
	if _, err := os.Stat(filepath.Join(root, "segment-summary.json")); err != nil {
		t.Fatalf("missing summary: %v", err)
	}
	referenced := map[string]bool{"segment-summary.json": true}
	for _, item := range summary.Segments {
		for _, rel := range []string{item.SegmentPath, item.PreviewPath} {
			if rel == "" {
				t.Fatalf("missing path in summary item: %+v", item)
			}
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				t.Fatalf("missing referenced artifact %s: %v", rel, err)
			}
			referenced[filepath.ToSlash(rel)] = true
		}
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !referenced[filepath.ToSlash(rel)] {
			t.Fatalf("unreferenced generated file: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk output: %v", err)
	}
}

func TestWriterRejectsUnexpectedExistingFile(t *testing.T) {
	out := t.TempDir()
	stale := filepath.Join(out, "document-segments", "segments", "stale.json")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("mkdir stale parent: %v", err)
	}
	if err := os.WriteFile(stale, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if err := Write(out, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected stale generated file rejection")
	}
}

func TestWriterRedactsUnsafeProvidedSegment(t *testing.T) {
	out := t.TempDir()
	segment := validSegment()
	segment.Title = "PRIVATE_CONTENT ready segment"
	segment.Summary = "secret " + unsafeTokenMarker() + " body must not persist"
	segment.Evidence.HeadingPath = []string{"SECRET heading"}
	segment.SourceDocumentID = "doc-secret-" + unsafeTokenMarker()
	if err := Write(out, Summary{RunID: segment.RunID, SourceCount: 1}, []Segment{segment}); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "private_content", "secret", unsafeTokenMarker())
}

func TestWriterRedactsGovernanceIDProvidedSegment(t *testing.T) {
	out := t.TempDir()
	segment := validSegment()
	segment.Title = "WP-17 private review notes"
	segment.Summary = "Review notes for DEC-75 should not persist in segment previews."
	segment.Evidence.HeadingPath = []string{"WP-17 review"}
	if err := Write(out, Summary{RunID: segment.RunID, SourceCount: 1}, []Segment{segment}); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "wp-17", "dec-75", "private review notes")
}

func TestSemanticWriterRedactsUnsafeEvidenceExcerpt(t *testing.T) {
	out := t.TempDir()
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.EvidenceExcerpts = []SemanticEvidenceExcerpt{{
		StructureNodeID: "node-demo",
		Text:            "This excerpt contains DEC-75 and must be redacted.",
	}}
	relation := validSemanticRelation(candidate, observation, node)
	if err := WriteSemantic(out, candidate.RunID, 1, []SemanticObservation{observation}, []SemanticCandidate{candidate}, []SemanticRelation{relation}); err != nil {
		t.Fatalf("write semantic: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "semantic-candidates"), "dec-75", "must be redacted")
}

func TestWriterRebuildsSummaryFromFinalizedSegments(t *testing.T) {
	out := t.TempDir()
	segment := validSegment()
	segment.Title = "PRIVATE_CONTENT ready segment"
	segment.Summary = "secret " + unsafeTokenMarker() + " body must not persist"
	segment.SourceDocumentID = "doc-secret-" + unsafeTokenMarker()
	summary := BuildSummary(segment.RunID, 1, []Segment{segment})
	if err := Write(out, summary, []Segment{segment}); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "private_content", "secret", unsafeTokenMarker(), "doc-secret-"+unsafeTokenMarker())
}

func TestUnsupportedSchema(t *testing.T) {
	segment := Segment{SchemaVersion: "document-segment/v9"}
	if err := ValidateSegment(segment); err == nil {
		t.Fatalf("expected unsupported schema error")
	}
}

func TestReviewStatusConfidence(t *testing.T) {
	unknown := validSegment()
	unknown.SemanticType = SemanticTypeUnknown
	unknown.ReviewStatus = ReviewStatusReady
	if err := ValidateSegment(unknown); err == nil {
		t.Fatalf("expected unknown ready segment to fail validation")
	}
	low := validSegment()
	low.Confidence = ConfidenceLow
	low.ReviewStatus = ReviewStatusReady
	if err := ValidateSegment(low); err == nil {
		t.Fatalf("expected low confidence ready segment to fail validation")
	}
}

func TestUnsafePrivateContentMarkerBlocksSegment(t *testing.T) {
	segment := ClassifyUnsafeMarkers(Segment{
		SchemaVersion:    SegmentSchemaVersion,
		SegmentID:        "seg-private-marker",
		RunID:            "run-doc-private-marker",
		SourceDocumentID: "doc-private-marker",
		SourceKind:       SourceKindMarkdown,
		SemanticType:     SemanticTypeSourceNote,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceHigh,
		Title:            "Synthetic private marker",
		Summary:          "PRIVATE_CONTENT marker should fail closed.",
		Evidence: Evidence{
			Kind:        EvidenceKindLocation,
			HeadingPath: []string{"Private marker sample"},
			LineStart:   1,
			LineEnd:     1,
			ContentHash: "sha256:abc",
		},
	})
	if segment.ReviewStatus != ReviewStatusBlocked {
		t.Fatalf("expected blocked, got %s", segment.ReviewStatus)
	}
	if len(segment.Blockers) == 0 {
		t.Fatalf("expected blocker reason")
	}
	body := strings.ToLower(segment.Title + "\n" + segment.Summary)
	for _, forbidden := range []string{"private_content", "secret", unsafeTokenMarker()} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("expected unsafe marker redaction, found %q in %s", forbidden, body)
		}
	}
}

func TestUnsafeMarkerGeneratedArtifactsAreRedacted(t *testing.T) {
	out := t.TempDir()
	if _, err := DecomposePath(fixturePath(t, "markdown", "mixed-thread-capture.md"), out); err != nil {
		t.Fatalf("decompose: %v", err)
	}
	files := readTree(t, filepath.Join(out, "document-segments"))
	for path, body := range files {
		lower := strings.ToLower(body)
		for _, forbidden := range []string{"private_content", "must block downstream flow"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("generated artifact %s leaked unsafe marker content %q:\n%s", path, forbidden, body)
			}
		}
	}
}

func TestUnsafeMarkerTableArtifactsAreRedacted(t *testing.T) {
	out := t.TempDir()
	segments := decomposeSection("run-doc-unsafe-table", "doc-unsafe-table", section{
		headingPath: []string{"Reference table"},
		lines: []line{
			{number: 1, text: "| Topic | Detail |"},
			{number: 2, text: "| --- | --- |"},
			{number: 3, text: "| PRIVATE_CONTENT row | " + unsafeTokenMarker() + " value must not persist |"},
		},
	})
	if len(segments) == 0 {
		t.Fatalf("expected table segments")
	}
	if err := Write(out, BuildSummary("run-doc-unsafe-table", 1, segments), segments); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "private_content", unsafeTokenMarker()+" value must not persist")
}

func TestUnsafeMarkerHeadingArtifactsAreRedacted(t *testing.T) {
	out := t.TempDir()
	segments := []Segment{
		newSegment("run-doc-unsafe-heading", "doc-unsafe-heading", []string{"SECRET roadmap heading"}, 1, 1, SemanticTypeSourceNote, ReviewStatusReady, ConfidenceMedium, "Safe body", "Safe body"),
	}
	if err := Write(out, BuildSummary("run-doc-unsafe-heading", 1, segments), segments); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "secret roadmap heading")
}

func TestUnsafeMarkerFilenameArtifactsAreRedacted(t *testing.T) {
	input := filepath.Join(t.TempDir(), "secret-"+unsafeTokenMarker()+".md")
	if err := os.WriteFile(input, []byte("# Safe heading\n\nSafe body.\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out := t.TempDir()
	if _, err := DecomposePath(input, out); err != nil {
		t.Fatalf("decompose: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "secret", unsafeTokenMarker())
}

func TestDirectoryInputsDisambiguateDuplicateBasenames(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"alpha", "beta"} {
		path := filepath.Join(root, dir, "notes.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir input parent: %v", err)
		}
		if err := os.WriteFile(path, []byte("# Decisions\n\nDecision: Ship the staged release.\n"), 0o644); err != nil {
			t.Fatalf("write input: %v", err)
		}
	}
	out := t.TempDir()
	summary, err := DecomposePath(root, out)
	if err != nil {
		t.Fatalf("decompose duplicate basenames: %v", err)
	}
	if summary.SourceCount != 2 || summary.SegmentCount != 2 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	sourceIDs := map[string]bool{}
	for _, item := range summary.Segments {
		sourceIDs[item.SourceDocumentID] = true
	}
	if len(sourceIDs) != 2 {
		t.Fatalf("expected distinct source document ids, got %+v", sourceIDs)
	}
}

func TestDirectoryInputsWithUnsafeFilenamesAreRelativePathDeterministic(t *testing.T) {
	firstRoot := unsafeFilenameTree(t)
	secondRoot := unsafeFilenameTree(t)
	firstOut := t.TempDir()
	secondOut := t.TempDir()
	if _, err := DecomposePath(firstRoot, firstOut); err != nil {
		t.Fatalf("first decompose: %v", err)
	}
	if _, err := DecomposePath(secondRoot, secondOut); err != nil {
		t.Fatalf("second decompose: %v", err)
	}
	assertTreeMatches(t, filepath.Join(firstOut, "document-segments"), filepath.Join(secondOut, "document-segments"))
}

func TestDecomposePathRejectsMarkdownScannerErrors(t *testing.T) {
	input := filepath.Join(t.TempDir(), "long-line.md")
	longLine := strings.Repeat("a", 1024*1024)
	if err := os.WriteFile(input, []byte("# Oversized\n\n"+longLine+"\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if _, err := DecomposePath(input, t.TempDir()); err == nil {
		t.Fatalf("expected scanner error for oversized markdown line")
	}
}

func TestUncertaintyMarkersTakePrecedenceOverActionHeuristics(t *testing.T) {
	segment := segmentFromText("run-doc-demo", "doc-demo", []string{"Open questions"}, 3, 3, "Maybe we need to revisit this")
	if segment.SemanticType != SemanticTypeUnknown {
		t.Fatalf("expected unknown semantic type, got %s", segment.SemanticType)
	}
	if segment.ReviewStatus != ReviewStatusNeedsReview {
		t.Fatalf("expected needs_review status, got %s", segment.ReviewStatus)
	}
}

func TestParseSectionsPreservesHeadingHierarchy(t *testing.T) {
	sections, err := parseSections("# Strategy\n\nIntro note.\n\n## Risks\n\nRisk: ambiguous provenance.\n\n### Follow up\n\nAction: map nested headings.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d: %+v", len(sections), sections)
	}
	assertHeadingPath(t, sections[0].headingPath, []string{"Strategy"})
	assertHeadingPath(t, sections[1].headingPath, []string{"Strategy", "Risks"})
	assertHeadingPath(t, sections[2].headingPath, []string{"Strategy", "Risks", "Follow up"})
}

func TestParseSectionsNormalizesNoH1HeadingHierarchy(t *testing.T) {
	sections, err := parseSections("Intro note.\n\n### First Area\n\nSource content.\n\n### Second Area\n\nMore content.\n\n## Later Top Area\n\nTop content.\n\n### Later Child Detail\n\nNested content.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 5 {
		t.Fatalf("expected pre-heading plus 4 headed sections, got %d: %+v", len(sections), sections)
	}
	assertHeadingPath(t, sections[0].headingPath, nil)
	assertHeadingPath(t, sections[1].headingPath, []string{"First Area"})
	assertHeadingPath(t, sections[2].headingPath, []string{"Second Area"})
	assertHeadingPath(t, sections[3].headingPath, []string{"Later Top Area"})
	assertHeadingPath(t, sections[4].headingPath, []string{"Later Top Area", "Later Child Detail"})
	if sections[1].sourceHeadingLevel != 3 || sections[1].normalizedHeadingLevel != 1 {
		t.Fatalf("expected source h3 normalized to level 1, got %+v", sections[1])
	}
}

func TestMixedTableSectionKeepsNonTableSegments(t *testing.T) {
	segments := decomposeSection("run-doc-demo", "doc-demo", section{
		headingPath: []string{"Capability review"},
		lines: []line{
			{number: 3, text: "Decision: keep local segment artifacts destination-neutral."},
			{number: 5, text: "| Capability | Status |"},
			{number: 6, text: "| --- | --- |"},
			{number: 7, text: "| Segment writer | Ready |"},
			{number: 9, text: "Action: validate downstream proposal adapters separately."},
		},
	})
	if len(segments) < 4 {
		t.Fatalf("expected table and non-table segments, got %d: %+v", len(segments), segments)
	}
	assertHasSemanticType(t, segments, SemanticTypeDecision)
	assertHasSemanticType(t, segments, SemanticTypeAction)
	assertHasSemanticType(t, segments, SemanticTypeReference)
}

func TestTableRowsMayContainDashesAsData(t *testing.T) {
	segments := decomposeTable("run-doc-demo", "doc-demo", section{
		headingPath: []string{"Version table"},
		lines: []line{
			{number: 1, text: "| Name | Range |"},
			{number: 2, text: "| --- | --- |"},
			{number: 3, text: "| Parser | v1---v2 |"},
		},
	})
	if len(segments) != 2 {
		t.Fatalf("expected table-level plus dashed data row segments, got %d: %+v", len(segments), segments)
	}
	if segments[1].Title != "Parser" || !strings.Contains(segments[1].Summary, "v1---v2") {
		t.Fatalf("expected dashed data row to be preserved, got %+v", segments[1])
	}
}

func TestParseSectionsIgnoresFencedCodeBlocks(t *testing.T) {
	sections, err := parseSections("# Notes\n\nDecision: keep semantic prose.\n\n```go\n// Decision: code sample must not become a segment.\nfmt.Println(\"Action: skip code\")\n```\n\nAction: process real prose.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	segments := decomposeSection("run-doc-demo", "doc-demo", sections[0])
	if len(segments) != 2 {
		t.Fatalf("expected only prose segments, got %d: %+v", len(segments), segments)
	}
	for _, segment := range segments {
		if strings.Contains(strings.ToLower(segment.Summary), "code sample") || strings.Contains(strings.ToLower(segment.Summary), "fmt.println") {
			t.Fatalf("code block content became a segment: %+v", segment)
		}
	}
}

func TestParseSectionsTracksFenceMarkerType(t *testing.T) {
	sections, err := parseSections("# Notes\n\n~~~md\n```not a closing fence\nDecision: code sample must stay ignored.\n~~~\n\nAction: process real prose.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	segments := decomposeSection("run-doc-demo", "doc-demo", sections[0])
	if len(segments) != 1 {
		t.Fatalf("expected only real prose segment, got %d: %+v", len(segments), segments)
	}
	if strings.Contains(strings.ToLower(segments[0].Summary), "code sample") {
		t.Fatalf("code fence content became a segment: %+v", segments[0])
	}
}

func TestParseSectionsRequiresValidATXHeading(t *testing.T) {
	sections, err := parseSections("# Notes\n\n#123 should remain prose.\n#include should remain prose too.\n## Follow up\n\nAction: keep valid headings.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 valid heading sections, got %d: %+v", len(sections), sections)
	}
	if len(sections[0].lines) != 2 {
		t.Fatalf("expected invalid heading markers to remain prose, got %+v", sections[0].lines)
	}
	assertHeadingPath(t, sections[1].headingPath, []string{"Notes", "Follow up"})
}

func TestParseSectionsAcceptsIndentedATXHeading(t *testing.T) {
	sections, err := parseSections("# Notes\n\n   ## Indented valid heading\n\nAction: preserve section provenance.\n\n    # Code-like line remains prose.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d: %+v", len(sections), sections)
	}
	assertHeadingPath(t, sections[1].headingPath, []string{"Notes", "Indented valid heading"})
	if len(sections[1].lines) != 2 {
		t.Fatalf("expected 4-space heading-like line to remain prose, got %+v", sections[1].lines)
	}
}

func TestDocumentSegmentHasNoDestinationHints(t *testing.T) {
	data, err := json.Marshal(validSegment())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{
		"destination_hints",
		"surface",
		"product" + "brain",
		"no" + "tion",
		"ob" + "sidian",
		"to" + "laria",
	} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("segment JSON contains destination-specific value %q: %s", forbidden, string(data))
		}
	}
}

func TestDocumentsPackageDoesNotImportProductBrain(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal", "documents"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		forbidden := "internal/" + "productbrain"
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("documents package imports Product Brain code in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documents package: %v", err)
	}
}

func TestSegmentID(t *testing.T) {
	got := SegmentID("run-doc-demo", "doc-demo", []string{"Actions"}, 12, "Product Lead to prepare checklist.")
	if got != "seg-986c470fb2625d48" {
		t.Fatalf("unexpected segment id: %s", got)
	}
}

func TestSegmentIDSerializesLineStartNumerically(t *testing.T) {
	first := SegmentID("run-doc-demo", "doc-demo", []string{"Actions"}, 0xD800, "repeat text")
	second := SegmentID("run-doc-demo", "doc-demo", []string{"Actions"}, 0xD801, "repeat text")
	if first == second {
		t.Fatalf("line numbers must produce distinct segment ids, both got %s", first)
	}
}

func TestSourceDocumentIDRedactsUnsafeFilename(t *testing.T) {
	got := SourceDocumentID("/tmp/secret-" + unsafeTokenMarker() + ".md")
	if strings.Contains(got, "secret") || strings.Contains(got, unsafeTokenMarker()) {
		t.Fatalf("expected redacted source document id, got %s", got)
	}
	if !strings.HasPrefix(got, "doc-redacted-") {
		t.Fatalf("expected redacted source document id prefix, got %s", got)
	}
}

func TestSegmentPath(t *testing.T) {
	if got := SegmentJSONPath("seg-demo"); got != "segments/seg-demo.json" {
		t.Fatalf("unexpected segment path: %s", got)
	}
	if got := SegmentPreviewPath("seg-demo"); got != "previews/seg-demo.md" {
		t.Fatalf("unexpected preview path: %s", got)
	}
}

func TestDuplicateSegmentID(t *testing.T) {
	segments := []Segment{validSegment(), validSegment()}
	if err := RejectDuplicateSegmentIDs(segments); err == nil {
		t.Fatalf("expected duplicate segment id error")
	}
}

func TestWriterRejectsTraversal(t *testing.T) {
	out := t.TempDir()
	segment := validSegment()
	segment.SegmentID = "../escape"
	if err := Write(out, Summary{RunID: segment.RunID}, []Segment{segment}); err == nil {
		t.Fatalf("expected traversal write error")
	}
}

func TestWriterRejectsSymlink(t *testing.T) {
	out := t.TempDir()
	root := filepath.Join(out, "document-segments")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "segments")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := Write(out, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlink write error")
	}
}

func TestWriterRejectsSymlinkedOutRoot(t *testing.T) {
	base := t.TempDir()
	realOut := filepath.Join(base, "real")
	linkOut := filepath.Join(base, "link")
	if err := os.Mkdir(realOut, 0o755); err != nil {
		t.Fatalf("mkdir real out: %v", err)
	}
	if err := os.Symlink(realOut, linkOut); err != nil {
		t.Fatalf("symlink out: %v", err)
	}
	if err := Write(linkOut, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlinked --out rejection")
	}
}

func TestWriterRejectsSymlinkedOutParent(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	linkParent := filepath.Join(base, "link-parent")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, linkParent); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	if err := Write(filepath.Join(linkParent, "generated"), Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlinked --out parent rejection")
	}
}

func TestWriterRejectsExistingOutUnderSymlinkedParent(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	linkParent := filepath.Join(base, "link-parent")
	if err := os.MkdirAll(filepath.Join(outside, "generated"), 0o755); err != nil {
		t.Fatalf("mkdir existing out: %v", err)
	}
	if err := os.Symlink(outside, linkParent); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	if err := Write(filepath.Join(linkParent, "generated"), Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected existing --out under symlinked parent rejection")
	}
}

func TestWriterRejectsSymlinkedDocumentSegmentsRoot(t *testing.T) {
	out := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(out, "document-segments")); err != nil {
		t.Fatalf("symlink document-segments root: %v", err)
	}
	if err := Write(out, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlinked document-segments root rejection")
	}
}

func validSegment() Segment {
	return Segment{
		SchemaVersion:    SegmentSchemaVersion,
		SegmentID:        "seg-demo",
		RunID:            "run-doc-demo",
		SourceDocumentID: "doc-demo",
		SourceKind:       SourceKindMarkdown,
		SemanticType:     SemanticTypeAction,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceMedium,
		Title:            "Prepare checklist",
		Summary:          "Product Lead should prepare the release checklist.",
		Evidence: Evidence{
			Kind:        EvidenceKindLocation,
			HeadingPath: []string{"Actions"},
			LineStart:   12,
			LineEnd:     12,
			ContentHash: "sha256:abc",
		},
		Blockers: []Blocker{},
	}
}

func duplicateStructureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"alpha", "beta"} {
		path := filepath.Join(root, dir, "notes.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir duplicate tree: %v", err)
		}
		body := "# Duplicate Notes\n\n## Capability Set\n\n- Capability: preserve relative path identity\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write duplicate tree: %v", err)
		}
	}
	return root
}

func unsafeFilenameTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"alpha", "beta"} {
		path := filepath.Join(root, dir, "secret-"+unsafeTokenMarker()+".md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir unsafe tree: %v", err)
		}
		body := "# Safe Heading\n\nDecision: redact unsafe filenames without path drift.\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write unsafe tree: %v", err)
		}
	}
	return root
}

func validStructureNode() StructureNode {
	return StructureNode{
		SchemaVersion:    StructureNodeSchemaVersion,
		NodeID:           "node-demo",
		RunID:            "run-struct-demo",
		SourceDocumentID: "doc-demo",
		NodeType:         StructureNodeTypeSection,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceMedium,
		Title:            "Demo section",
		Summary:          "Synthetic section summary.",
		ParentNodeID:     "",
		ChildNodeIDs:     []string{},
		RelatedSegmentIDs: []string{
			"seg-demo",
		},
		Evidence: StructureEvidence{
			SourceKind:        SourceKindMarkdown,
			SourceDocumentID:  "doc-demo",
			HeadingPath:       []string{"Demo section"},
			LineStart:         1,
			LineEnd:           2,
			ContentHash:       "sha256:abc",
			RelatedSegmentIDs: []string{"seg-demo"},
		},
		Blockers: []Blocker{},
	}
}

func validSemanticObservation(node StructureNode) SemanticObservation {
	return SemanticObservation{
		SchemaVersion:    SemanticObservationSchemaVersion,
		ObservationID:    "obs-demo",
		RunID:            "run-sem-demo",
		SourceDocumentID: node.SourceDocumentID,
		ObservationKind:  SemanticObservationKindActionSignal,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceMedium,
		Title:            "Prepare checklist",
		Summary:          "Lead will prepare the checklist.",
		EvidenceNodes:    []string{node.NodeID},
		EvidenceRanges:   []SemanticEvidenceRange{{StructureNodeID: node.NodeID, LineStart: 1, LineEnd: 2}},
		ContentHash:      "sha256:abc",
		Blockers:         []Blocker{},
	}
}

func validSemanticCandidate(observation SemanticObservation, node StructureNode) SemanticCandidate {
	return SemanticCandidate{
		SchemaVersion:     SemanticCandidateSchemaVersion,
		CandidateID:       "cand-demo",
		RunID:             observation.RunID,
		SourceDocumentID:  observation.SourceDocumentID,
		CandidateKind:     SemanticCandidateKindAction,
		ReviewStatus:      ReviewStatusReady,
		Confidence:        ConfidenceMedium,
		Title:             "Prepare checklist",
		Summary:           "Lead will prepare the checklist.",
		EvidenceNodes:     []string{node.NodeID},
		EvidenceRanges:    []SemanticEvidenceRange{{StructureNodeID: node.NodeID, LineStart: 1, LineEnd: 2}},
		ObservationIDs:    []string{observation.ObservationID},
		RelationIDs:       []string{"rel-demo"},
		DestinationStatus: SemanticDestinationUnresolved,
		Blockers:          []Blocker{},
	}
}

func validSemanticRelation(candidate SemanticCandidate, observation SemanticObservation, node StructureNode) SemanticRelation {
	return SemanticRelation{
		SchemaVersion:    SemanticRelationSchemaVersion,
		RelationID:       "rel-demo",
		RunID:            candidate.RunID,
		RelationshipType: SemanticRelationshipDerivedFrom,
		FromID:           candidate.CandidateID,
		FromType:         SemanticRelationEndpointCandidate,
		ToID:             observation.ObservationID,
		ToType:           SemanticRelationEndpointObservation,
		EvidenceNodes:    []string{node.NodeID},
		Confidence:       ConfidenceMedium,
		ReviewStatus:     ReviewStatusReady,
		Blockers:         []Blocker{},
	}
}

func TestSemanticAcceptanceEvaluatesExpectedOutcomes(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
		{
			SchemaVersion:     SemanticCandidateSchemaVersion,
			CandidateID:       "cand-unexpected",
			RunID:             "run-sem-demo",
			SourceDocumentID:  "doc-demo",
			CandidateKind:     SemanticCandidateKindDecision,
			ReviewStatus:      ReviewStatusReady,
			Confidence:        ConfidenceMedium,
			Title:             "Decide launch scope",
			Summary:           "The team decided the launch scope.",
			EvidenceNodes:     []string{"node-decision"},
			EvidenceRanges:    []SemanticEvidenceRange{{StructureNodeID: "node-decision", LineStart: 3, LineEnd: 4}},
			ObservationIDs:    []string{"obs-decision"},
			RelationIDs:       []string{"rel-decision"},
			DestinationStatus: SemanticDestinationUnresolved,
			Blockers:          []Blocker{},
		},
	})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-demo",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{
			{
				ExpectedOutcomeID:      "exp-action",
				ExpectedState:          ExpectedOutcomePresent,
				ExpectedKind:           SemanticCandidateKindAction,
				RequiredEvidence:       []string{"node-demo"},
				TitleSignals:           []string{"checklist"},
				SummarySignals:         []string{"prepare"},
				MinimumConfidenceFloor: ConfidenceLow,
			},
			{
				ExpectedOutcomeID:      "exp-no-risk",
				ExpectedState:          ExpectedOutcomeAbsent,
				ExpectedKind:           SemanticCandidateKindRisk,
				TitleSignals:           []string{"security risk"},
				MinimumConfidenceFloor: ConfidenceLow,
			},
		},
	})
	out := t.TempDir()
	summary, err := AcceptSemantic(semanticRun, answerKey, out)
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.SchemaVersion != SemanticAcceptanceSummarySchemaVersion {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.MatchedExpectedCount != 1 || summary.MissedExpectedCount != 0 || summary.UnexpectedCandidateCount != 1 {
		t.Fatalf("unexpected acceptance counts: %+v", summary)
	}
	if summary.FalsePositiveCount != 1 || summary.FalseNegativeCount != 0 || summary.EvidenceMissingCount != 0 {
		t.Fatalf("unexpected false-positive/false-negative counts: %+v", summary)
	}
	if summary.PrecisionLikeMatchRate != 0.5 || summary.RecallLikeExpectedOutcomeCoverage != 1 {
		t.Fatalf("unexpected rates: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(out, "semantic-acceptance", "acceptance-summary.json")); err != nil {
		t.Fatalf("expected acceptance summary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "semantic-acceptance", "reports", "quality-report.md")); err != nil {
		t.Fatalf("expected quality report: %v", err)
	}
	report, err := os.ReadFile(filepath.Join(out, "semantic-acceptance", "reports", "quality-report.md"))
	if err != nil {
		t.Fatalf("read quality report: %v", err)
	}
	if !strings.Contains(string(report), "False positives: 1") || !strings.Contains(string(report), "False negatives: 0") {
		t.Fatalf("quality report must label false-positive and false-negative counts:\n%s", string(report))
	}
	for _, item := range summary.Items {
		if item.CandidateID != "cand-demo" {
			continue
		}
		if item.SourceDocumentID != "doc-demo" || len(item.EvidenceRanges) == 0 || len(item.RelationIDs) == 0 {
			t.Fatalf("acceptance item must preserve source, ranges, and relations: %+v", item)
		}
	}
}

func TestSemanticAcceptanceReportsMissedExpectedOutcome(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-missed",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-required-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	summary, err := AcceptSemantic(semanticRun, answerKey, t.TempDir())
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.MissedExpectedCount != 1 || summary.RecallLikeExpectedOutcomeCoverage != 0 {
		t.Fatalf("expected missed outcome and zero recall-like coverage: %+v", summary)
	}
	if summary.FalseNegativeCount != 1 || summary.EvidenceMissingCount != 1 {
		t.Fatalf("expected missed outcome to count as false negative and missing evidence: %+v", summary)
	}
}

func TestSemanticAcceptanceDoesNotMatchWrongSourceDocument(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.SourceDocumentID = "doc-other"
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-source-scope",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	summary, err := AcceptSemantic(semanticRun, answerKey, t.TempDir())
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.MatchedExpectedCount != 0 || summary.MissedExpectedCount != 1 {
		t.Fatalf("expected wrong-source candidate to miss: %+v", summary)
	}
	if summary.CandidateCount != 0 || summary.FalsePositiveCount != 0 || summary.UnexpectedCandidateCount != 0 {
		t.Fatalf("expected wrong-source candidate excluded from scoped metrics: %+v", summary)
	}
}

func TestSemanticAcceptanceRequiresFullEvidenceSetAndRanges(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.EvidenceNodes = []string{"node-demo", "node-other"}
	candidate.EvidenceRanges = []SemanticEvidenceRange{{StructureNodeID: "node-demo", LineStart: 1, LineEnd: 2}}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-evidence-set",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo", "node-other"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	summary, err := AcceptSemantic(semanticRun, answerKey, t.TempDir())
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.MatchedExpectedCount != 0 || summary.MissedExpectedCount != 1 {
		t.Fatalf("expected missing evidence range to fail match: %+v", summary)
	}
}

func TestSemanticAcceptanceRequiresRelationRequirements(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.RelationIDs = []string{SemanticRelationID(candidate.RunID, SemanticRelationshipDerivedFrom, candidate.CandidateID, observation.ObservationID)}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-relation",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			RelationRequirements:   []SemanticRelationshipType{SemanticRelationshipContradicts},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	summary, err := AcceptSemantic(semanticRun, answerKey, t.TempDir())
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.MatchedExpectedCount != 0 || summary.MissedExpectedCount != 1 {
		t.Fatalf("expected missing relation requirement to fail match: %+v", summary)
	}
}

func TestSemanticAcceptanceIgnoresBlockedRelationRequirements(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	relationID := SemanticRelationID(candidate.RunID, SemanticRelationshipDerivedFrom, candidate.CandidateID, observation.ObservationID)
	candidate.RelationIDs = []string{relationID}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticRelationJSONPath(relationID)), SemanticRelation{
		SchemaVersion:    SemanticRelationSchemaVersion,
		RelationID:       relationID,
		RunID:            candidate.RunID,
		RelationshipType: SemanticRelationshipDerivedFrom,
		FromID:           candidate.CandidateID,
		FromType:         SemanticRelationEndpointCandidate,
		ToID:             observation.ObservationID,
		ToType:           SemanticRelationEndpointObservation,
		EvidenceNodes:    []string{"node-demo"},
		Confidence:       ConfidenceLow,
		ReviewStatus:     ReviewStatusBlocked,
		Blockers:         []Blocker{{Code: "relation_blocked", Message: "Relation evidence is blocked."}},
	})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-blocked-relation",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			RelationRequirements:   []SemanticRelationshipType{SemanticRelationshipDerivedFrom},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	summary, err := AcceptSemantic(semanticRun, answerKey, t.TempDir())
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.MatchedExpectedCount != 0 || summary.MissedExpectedCount != 1 {
		t.Fatalf("expected blocked relation not to satisfy requirement: %+v", summary)
	}
}

func TestSemanticAcceptanceDoesNotCountBlockedCandidateAsMatch(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.ReviewStatus = ReviewStatusBlocked
	candidate.Blockers = []Blocker{{Code: "blocked_candidate", Message: "Candidate blocked by review policy."}}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-blocked",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	summary, err := AcceptSemantic(semanticRun, answerKey, t.TempDir())
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.MatchedExpectedCount != 0 || summary.BlockedCount != 1 || summary.RecallLikeExpectedOutcomeCoverage != 0 {
		t.Fatalf("expected blocked candidate excluded from quality match: %+v", summary)
	}
	if summary.PrecisionLikeMatchRate != 0 || summary.ReviewBurdenCount != 1 {
		t.Fatalf("expected blocked candidate excluded from precision denominator: %+v", summary)
	}
}

func TestSemanticAcceptancePreservesNeedsReviewCandidateWithReviewBlocker(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.ReviewStatus = ReviewStatusNeedsReview
	candidate.Confidence = ConfidenceLow
	candidate.Blockers = []Blocker{{Code: "semantic_review_required", Message: "Candidate requires review because evidence is weak, contradicted, or ambiguous."}}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-needs-review",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	summary, err := AcceptSemantic(semanticRun, answerKey, t.TempDir())
	if err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if summary.BlockedCount != 0 || summary.NeedsReviewCount != 1 || summary.ReviewBurdenCount != 1 {
		t.Fatalf("expected needs_review candidate to remain review burden, got %+v", summary)
	}
	if len(summary.Items) != 1 || summary.Items[0].AcceptanceState != SemanticAcceptanceNeedsReview {
		t.Fatalf("expected item to need review, got %+v", summary.Items)
	}
}

func TestSemanticAcceptanceRejectsPrivateMarkerInBlocker(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.ReviewStatus = ReviewStatusBlocked
	candidate.Blockers = []Blocker{{Code: "unsafe_marker", Message: unsafeTokenMarker()}}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-blocker-marker",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected private marker in blocker to be rejected")
	}
}

func TestSemanticAcceptanceRejectsPrivateMarkerInRunID(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode())})
	summaryPath := filepath.Join(semanticRun, "semantic-candidates", "semantic-summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read semantic summary: %v", err)
	}
	var summary SemanticSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode semantic summary: %v", err)
	}
	summary.RunID = "run-" + unsafeTokenMarker()
	writeDocumentsTestJSON(t, summaryPath, summary)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-run-marker",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected private marker in run id to be rejected")
	}
}

func TestSemanticAcceptanceRejectsDuplicateExpectedOutcomes(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-duplicate",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{
			{ExpectedOutcomeID: "exp-duplicate", ExpectedState: ExpectedOutcomeAbsent, ExpectedKind: SemanticCandidateKindRisk, MinimumConfidenceFloor: ConfidenceLow},
			{ExpectedOutcomeID: "exp-duplicate", ExpectedState: ExpectedOutcomeAbsent, ExpectedKind: SemanticCandidateKindRisk, MinimumConfidenceFloor: ConfidenceLow},
		},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected duplicate expected outcome rejection")
	}
}

func TestSemanticAcceptanceRejectsExpectedPresentWithoutEvidence(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-missing-evidence",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-missing-evidence",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected missing evidence rejection")
	}
}

func TestSemanticAcceptanceRejectsBlankRequiredEvidence(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-blank-evidence",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-blank-evidence",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{" "},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected blank required evidence rejection")
	}
}

func TestSemanticAcceptanceRejectsPrivateMarkers(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-private",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-private",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{unsafeTokenMarker()},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected private marker rejection")
	}
}

func TestSemanticAcceptanceRejectsCandidatePathTraversal(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode())})
	summaryPath := filepath.Join(semanticRun, "semantic-candidates", "semantic-summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read semantic summary: %v", err)
	}
	var summary SemanticSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode semantic summary: %v", err)
	}
	summary.Candidates[0].CandidatePath = "../outside.json"
	writeDocumentsTestJSON(t, summaryPath, summary)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-path-traversal",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected candidate path traversal rejection")
	}
}

func TestSemanticAcceptanceRejectsSymlinkedInputArtifact(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode())})
	candidatePath := filepath.Join(semanticRun, "semantic-candidates", SemanticCandidateJSONPath("cand-demo"))
	if err := os.Remove(candidatePath); err != nil {
		t.Fatalf("remove candidate artifact: %v", err)
	}
	if err := os.Symlink(outside, candidatePath); err != nil {
		t.Fatalf("symlink candidate artifact: %v", err)
	}
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-input-symlink",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected symlinked candidate artifact rejection")
	}
}

func TestSemanticAcceptanceRejectsSymlinkedSummaryArtifact(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside-summary.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write outside summary: %v", err)
	}
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	summaryPath := filepath.Join(semanticRun, "semantic-candidates", "semantic-summary.json")
	if err := os.Remove(summaryPath); err != nil {
		t.Fatalf("remove semantic summary: %v", err)
	}
	if err := os.Symlink(outside, summaryPath); err != nil {
		t.Fatalf("symlink semantic summary: %v", err)
	}
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-summary-symlink",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil {
		t.Fatalf("expected symlinked semantic summary rejection")
	}
}

func TestSemanticAcceptanceDeterministicAcrossRuns(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-deterministic",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			TitleSignals:           []string{"checklist"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	first := t.TempDir()
	second := t.TempDir()
	if _, err := AcceptSemantic(semanticRun, answerKey, first); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	if _, err := AcceptSemantic(semanticRun, answerKey, second); err != nil {
		t.Fatalf("second accept: %v", err)
	}
	assertTreeMatches(t, filepath.Join(first, "semantic-acceptance"), filepath.Join(second, "semantic-acceptance"))
}

func TestSemanticAcceptanceRejectsSymlinkedOutParent(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	linkParent := filepath.Join(base, "link-parent")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, linkParent); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-symlink",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-no-risk",
			ExpectedState:          ExpectedOutcomeAbsent,
			ExpectedKind:           SemanticCandidateKindRisk,
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, filepath.Join(linkParent, "generated")); err == nil {
		t.Fatalf("expected symlinked --out parent rejection")
	}
}

func TestSemanticCalibrationFailsClosedBelowThreshold(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
		unexpectedDecisionCandidate(),
	}, true)
	out := t.TempDir()
	summary, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	if summary.SchemaVersion != SemanticCalibrationSummarySchemaVersion {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.ThresholdStatus != SemanticCalibrationThresholdNotTrusted || summary.NoHumanEligible {
		t.Fatalf("below-threshold batch must fail closed: %+v", summary)
	}
	if summary.ScoredCount != 2 || summary.AcceptedCount != 1 || summary.FailureClassCounts[SemanticCalibrationFailureFalsePositive] != 1 {
		t.Fatalf("unexpected calibration counts: %+v", summary)
	}
	if summary.FailureReasonCounts[SemanticFailureUnexpectedCandidate] != 1 {
		t.Fatalf("expected canonical failure reason count: %+v", summary.FailureReasonCounts)
	}
	if summary.MeasuredAccuracy != 0.5 {
		t.Fatalf("unexpected measured accuracy: %+v", summary)
	}
	report, err := os.ReadFile(filepath.Join(out, "semantic-calibration", "reports", "calibration-report.md"))
	if err != nil {
		t.Fatalf("read calibration report: %v", err)
	}
	if !strings.Contains(string(report), "temporary calibration evidence") ||
		!strings.Contains(string(report), "Failure reasons") ||
		!strings.Contains(string(report), "unexpected_candidate: 1") {
		t.Fatalf("report must frame human review as temporary calibration evidence:\n%s", string(report))
	}
}

func TestSemanticJudgmentInitializesReviewBundleAndPagesOneCandidate(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.EvidenceRanges = []SemanticEvidenceRange{{StructureNodeID: node.NodeID, LineStart: 2, LineEnd: 4}}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticObservationJSONPath(observation.ObservationID)), observation)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte("one\ntwo\nthree\nfour\nfive\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()
	summary, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{SourceRoot: sourceRoot, SourcePath: "source.md"})
	if err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	if summary.SchemaVersion != SemanticJudgmentSummarySchemaVersion || summary.CandidateCount != 1 || summary.RemainingCount != 1 {
		t.Fatalf("unexpected judgment summary: %+v", summary)
	}
	if summary.EvidenceReadyCount != 1 || summary.EvalCountedCount != 1 || summary.EvidenceExcludedCount != 0 {
		t.Fatalf("expected fully grounded candidate to be eval-counted: %+v", summary)
	}
	page, err := NextSemanticJudgmentPage(filepath.Join(out, "semantic-judgment"))
	if err != nil {
		t.Fatalf("next semantic judgment page: %v", err)
	}
	if page.SchemaVersion != SemanticJudgmentPageSchemaVersion || page.Done || page.Item == nil {
		t.Fatalf("expected one judgment page: %+v", page)
	}
	if page.Item.EvidenceReadiness.Status != SemanticEvidenceReadinessPass || !page.Item.EvidenceReadiness.EvalCounted {
		t.Fatalf("expected page item to pass evidence readiness: %+v", page.Item.EvidenceReadiness)
	}
	if !strings.Contains(page.PageMarkdown, "Prepare checklist") ||
		!strings.Contains(page.PageMarkdown, "Adjudication choices") ||
		!strings.Contains(page.PageMarkdown, "Evidence readiness: pass") ||
		!strings.Contains(page.PageMarkdown, "Eval counted: true") ||
		!strings.Contains(page.PageMarkdown, "source.md lines 2-4") ||
		!strings.Contains(page.PageMarkdown, "Relation context") ||
		!strings.Contains(page.PageMarkdown, string(SemanticRelationshipDerivedFrom)) ||
		!strings.Contains(page.PageMarkdown, "This is the evidence link") ||
		!strings.Contains(page.PageMarkdown, "two\nthree\nfour") {
		t.Fatalf("judgment page is not self-contained:\n%s", page.PageMarkdown)
	}
}

func TestSemanticJudgmentAgentReviewTriageDoesNotCreateJudgments(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	machineCandidate := validSemanticCandidate(observation, node)
	machineCandidate.CandidateID = "cand-machine"
	machineCandidate.RelationIDs = []string{"rel-machine"}
	machineCandidate.Title = "Prepare checklist"
	machineCandidate.Summary = "Prepare the checklist from the cited evidence."
	machineCandidate.EvidenceRanges = []SemanticEvidenceRange{{StructureNodeID: node.NodeID, LineStart: 2, LineEnd: 4}}
	humanCandidate := validSemanticCandidate(observation, node)
	humanCandidate.CandidateID = "cand-human"
	humanCandidate.RelationIDs = []string{"rel-human"}
	humanCandidate.Title = "Maybe prepare checklist"
	humanCandidate.Summary = "Maybe prepare the checklist from partial evidence."
	humanCandidate.EvidenceRanges = []SemanticEvidenceRange{{StructureNodeID: node.NodeID, LineStart: 2, LineEnd: 4}}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{machineCandidate, humanCandidate})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticObservationJSONPath(observation.ObservationID)), observation)
	machineRelation := validSemanticRelation(machineCandidate, observation, node)
	machineRelation.RelationID = "rel-machine"
	humanRelation := validSemanticRelation(humanCandidate, observation, node)
	humanRelation.RelationID = "rel-human"
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticRelationJSONPath(machineRelation.RelationID)), machineRelation)
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticRelationJSONPath(humanRelation.RelationID)), humanRelation)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte("one\ntwo\nthree\nfour\nfive\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	reviewer := &fakeLLMSemanticReviewer{responses: []llmSemanticReviewResponse{
		{
			Choice:              string(SemanticJudgmentChoiceUnclear),
			FailureReason:       string(SemanticFailureAmbiguous),
			Confidence:          string(ConfidenceLow),
			HumanReviewRequired: true,
			ReviewReasonCodes:   []string{string(SemanticAgentReviewReasonModelUncertain)},
			Rationale:           "Evidence is not decisive enough.",
		},
		{
			Choice:              string(SemanticJudgmentChoiceAccept),
			Confidence:          string(ConfidenceHigh),
			HumanReviewRequired: false,
			ReviewReasonCodes:   []string{string(SemanticAgentReviewReasonMachineTriaged)},
			Rationale:           "Evidence supports the candidate.",
		},
	}}
	out := t.TempDir()
	summary, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{
		SourceRoot:  sourceRoot,
		SourcePath:  "source.md",
		Reviewer:    SemanticJudgmentReviewerLLM,
		LLMProvider: "openai",
		LLMModel:    "test-model",
		LLMClient:   reviewer,
	})
	if err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	if summary.AgentReviewedCount != 2 || summary.HumanReviewRequiredCount != 1 || summary.MachineTriagedCount != 1 {
		t.Fatalf("unexpected agent review counts: %+v", summary)
	}
	if summary.JudgedCount != 0 || summary.AcceptedCount != 0 || summary.RejectedCount != 0 || summary.RemainingCount != 2 {
		t.Fatalf("agent proposals must not create judgment counts: %+v", summary)
	}
	if len(summary.Judgments) != 0 {
		t.Fatalf("agent proposals must not create judgment records: %+v", summary.Judgments)
	}
	page, err := NextSemanticJudgmentPage(filepath.Join(out, "semantic-judgment"))
	if err != nil {
		t.Fatalf("next semantic judgment page: %v", err)
	}
	if page.Done || page.Item == nil || page.Item.CandidateID != "cand-human" {
		t.Fatalf("expected human-required candidate to be served first: %+v", page)
	}
	if page.Cursor.RemainingCount != 1 {
		t.Fatalf("cursor should count human-required remaining items, got %+v", page.Cursor)
	}
	if page.Item.AgentReview == nil || !page.Item.AgentReview.HumanReviewRequired {
		t.Fatalf("expected human-required agent proposal: %+v", page.Item.AgentReview)
	}
	if !strings.Contains(page.PageMarkdown, "Agent review proposal") || !strings.Contains(page.PageMarkdown, "proposal only") {
		t.Fatalf("page must show non-final agent proposal:\n%s", page.PageMarkdown)
	}
	_, err = RecordSemanticJudgment(filepath.Join(out, "semantic-judgment"), SemanticJudgmentRecordInput{
		CandidateID:   page.Item.CandidateID,
		Choice:        SemanticJudgmentChoiceUnclear,
		FailureReason: SemanticFailureAmbiguous,
		ReviewerID:    "test",
		RecordedAt:    time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("record semantic judgment: %v", err)
	}
	donePage, err := NextSemanticJudgmentPage(filepath.Join(out, "semantic-judgment"))
	if err != nil {
		t.Fatalf("next done semantic judgment page: %v", err)
	}
	if !donePage.Done || !strings.Contains(donePage.PageMarkdown, "machine-triaged proposal-only") || !strings.Contains(donePage.PageMarkdown, "remain unjudged") {
		t.Fatalf("done page must not imply machine-triaged candidates are complete: %+v", donePage)
	}
}

func TestSemanticJudgmentAgentReviewDoesNotSendUnsafeCandidates(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	item := semanticJudgmentCandidates([]SemanticCandidate{candidate}, nil, []SemanticObservation{observation}, semanticCalibrationSourceContext{}, nil)[0]
	item.EvidenceExcerpts = append(item.EvidenceExcerpts, SemanticCalibrationEvidenceExcerpt{
		SourceLabel:     "source.md",
		StructureNodeID: node.NodeID,
		LineStart:       1,
		LineEnd:         1,
		Text:            "private_content",
	})
	item.EvidenceReadiness = semanticEvidenceReadiness(item)
	reviewer := &fakeLLMSemanticReviewer{}
	items, err := attachSemanticAgentReviews([]SemanticJudgmentCandidate{item}, SemanticJudgmentOptions{
		Reviewer:    SemanticJudgmentReviewerLLM,
		LLMProvider: "openai",
		LLMModel:    "test-model",
		LLMClient:   reviewer,
	})
	if err != nil {
		t.Fatalf("attach semantic agent reviews: %v", err)
	}
	if len(reviewer.requests) != 0 {
		t.Fatalf("unsafe candidate must not be sent to LLM: %+v", reviewer.requests)
	}
	if items[0].AgentReview == nil || !items[0].AgentReview.HumanReviewRequired || items[0].AgentReview.ReviewReasonCodes[0] != SemanticAgentReviewReasonUnsafeOrPrivate {
		t.Fatalf("expected local unsafe proposal: %+v", items[0].AgentReview)
	}
}

func TestSemanticJudgmentIncludesRelationEndpointContext(t *testing.T) {
	node := validStructureNode()
	action := validSemanticCandidate(validSemanticObservation(node), node)
	decision := unexpectedDecisionCandidate()
	decision.Summary = strings.Repeat("long related decision summary ", 20) + "\nsecond line must collapse"
	relationID := "rel-action-decision"
	action.RelationIDs = []string{relationID}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{action, decision})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticRelationJSONPath(relationID)), SemanticRelation{
		SchemaVersion:    SemanticRelationSchemaVersion,
		RelationID:       relationID,
		RunID:            action.RunID,
		RelationshipType: SemanticRelationshipContradicts,
		FromID:           action.CandidateID,
		FromType:         SemanticRelationEndpointCandidate,
		ToID:             decision.CandidateID,
		ToType:           SemanticRelationEndpointCandidate,
		EvidenceNodes:    []string{"node-demo"},
		Confidence:       ConfidenceMedium,
		ReviewStatus:     ReviewStatusNeedsReview,
		Blockers:         []Blocker{},
	})
	out := t.TempDir()
	summary, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{})
	if err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	var judgedAction SemanticJudgmentCandidate
	for _, item := range summary.Items {
		if item.CandidateID == action.CandidateID {
			judgedAction = item
			break
		}
	}
	if len(judgedAction.RelationContext) != 1 {
		t.Fatalf("expected relation context for action candidate: %+v", judgedAction)
	}
	relation := judgedAction.RelationContext[0]
	if relation.RelationshipType != SemanticRelationshipContradicts ||
		relation.OtherEndpoint.EndpointID != decision.CandidateID ||
		relation.OtherEndpoint.Role != "to" ||
		relation.OtherEndpoint.Label != decision.Title ||
		!strings.Contains(relation.ReviewHint, "conflicts") {
		t.Fatalf("unexpected relation context: %+v", relation)
	}
	if relation.OtherEndpoint.Summary != semanticSummaryText(decision.Summary) ||
		strings.Contains(relation.OtherEndpoint.Summary, "\n") ||
		len(relation.OtherEndpoint.Summary) > 160 {
		t.Fatalf("expected bounded one-line endpoint summary, got %q", relation.OtherEndpoint.Summary)
	}
	page, err := NextSemanticJudgmentPage(filepath.Join(out, "semantic-judgment"))
	if err != nil {
		t.Fatalf("next semantic judgment page: %v", err)
	}
	if !strings.Contains(page.PageMarkdown, decision.Title) ||
		!strings.Contains(page.PageMarkdown, "Other endpoint role: to") ||
		!strings.Contains(page.PageMarkdown, "conflicts") {
		t.Fatalf("expected endpoint label and hint in page:\n%s", page.PageMarkdown)
	}
}

func TestSemanticJudgmentLoadsRelationEndpointObservationContext(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	action := validSemanticCandidate(observation, node)
	relatedObservation := observation
	relatedObservation.ObservationID = "obs-related-owner"
	relatedObservation.Title = "Owner signal"
	relatedObservation.Summary = "Sam owns the rollout follow-up."
	relationID := "rel-action-owner"
	action.RelationIDs = []string{relationID}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{action})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticObservationJSONPath(relatedObservation.ObservationID)), relatedObservation)
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticRelationJSONPath(relationID)), SemanticRelation{
		SchemaVersion:    SemanticRelationSchemaVersion,
		RelationID:       relationID,
		RunID:            action.RunID,
		RelationshipType: SemanticRelationshipMentionsOwner,
		FromID:           action.CandidateID,
		FromType:         SemanticRelationEndpointCandidate,
		ToID:             relatedObservation.ObservationID,
		ToType:           SemanticRelationEndpointObservation,
		EvidenceNodes:    []string{"node-demo"},
		Confidence:       ConfidenceMedium,
		ReviewStatus:     ReviewStatusNeedsReview,
		Blockers:         []Blocker{},
	})

	out := t.TempDir()
	summary, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{})
	if err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	if len(summary.Items) != 1 || len(summary.Items[0].RelationContext) != 1 {
		t.Fatalf("expected relation context for action candidate: %+v", summary.Items)
	}
	endpoint := summary.Items[0].RelationContext[0].OtherEndpoint
	if endpoint.Unavailable || endpoint.EndpointID != relatedObservation.ObservationID || endpoint.Label != relatedObservation.Title || endpoint.Summary != relatedObservation.Summary {
		t.Fatalf("expected loaded relation endpoint observation context, got %+v", endpoint)
	}
}

func TestSemanticJudgmentMarksUnrelatedRelationEndpointUnavailable(t *testing.T) {
	node := validStructureNode()
	action := validSemanticCandidate(validSemanticObservation(node), node)
	relationID := "rel-objection-proposal"
	action.RelationIDs = []string{relationID}
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{action})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticRelationJSONPath(relationID)), SemanticRelation{
		SchemaVersion:    SemanticRelationSchemaVersion,
		RelationID:       relationID,
		RunID:            action.RunID,
		RelationshipType: SemanticRelationshipContradicts,
		FromID:           "obs-objection",
		FromType:         SemanticRelationEndpointObservation,
		ToID:             "obs-proposal",
		ToType:           SemanticRelationEndpointObservation,
		EvidenceNodes:    []string{"node-demo"},
		Confidence:       ConfidenceMedium,
		ReviewStatus:     ReviewStatusNeedsReview,
		Blockers:         []Blocker{},
	})
	out := t.TempDir()
	summary, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{})
	if err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	if len(summary.Items) != 1 || len(summary.Items[0].RelationContext) != 1 {
		t.Fatalf("expected unrelated relation context to remain visible: %+v", summary.Items)
	}
	endpoint := summary.Items[0].RelationContext[0].OtherEndpoint
	if !endpoint.Unavailable || endpoint.Role != "unknown" || endpoint.UnavailableReason != "relation does not reference current candidate" {
		t.Fatalf("expected unrelated relation endpoint to be unknown and unavailable, got %+v", endpoint)
	}
	page, err := NextSemanticJudgmentPage(filepath.Join(out, "semantic-judgment"))
	if err != nil {
		t.Fatalf("next semantic judgment page: %v", err)
	}
	if !strings.Contains(page.PageMarkdown, "Other endpoint role: unknown") ||
		!strings.Contains(page.PageMarkdown, "relation does not reference current candidate") {
		t.Fatalf("expected unknown unrelated relation endpoint in page:\n%s", page.PageMarkdown)
	}
}

func TestSemanticJudgmentRejectsUnsafeRelationContext(t *testing.T) {
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	item := semanticJudgmentCandidates([]SemanticCandidate{candidate}, nil, nil, semanticCalibrationSourceContext{}, nil)[0]
	item.RelationContext = []SemanticJudgmentRelationContext{{
		RelationID:       "rel-safe",
		RelationshipType: SemanticRelationshipSameTopicAs,
		FromID:           candidate.CandidateID,
		FromType:         SemanticRelationEndpointCandidate,
		ToID:             "cand-related",
		ToType:           SemanticRelationEndpointCandidate,
		Confidence:       ConfidenceMedium,
		ReviewStatus:     ReviewStatusReady,
		OtherEndpoint: SemanticJudgmentEndpointContext{
			EndpointID:   "cand-related",
			EndpointType: SemanticRelationEndpointCandidate,
			Role:         "to",
			Label:        "related " + unsafeTokenMarker(),
		},
		ReviewHint: "unsafe relation context should fail closed",
	}}
	item.EvidenceReadiness = semanticEvidenceReadiness(item)
	if err := ValidateSemanticJudgmentCandidate(item); err == nil || !strings.Contains(err.Error(), "private marker") {
		t.Fatalf("expected unsafe relation context validation failure, got %v", err)
	}
}

func TestSemanticJudgmentToleratesMissingRelationWithoutLooseningAcceptance(t *testing.T) {
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	if err := os.Remove(filepath.Join(semanticRun, "semantic-candidates", SemanticRelationJSONPath(candidate.RelationIDs[0]))); err != nil {
		t.Fatalf("remove relation fixture: %v", err)
	}
	out := t.TempDir()
	summary, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{})
	if err != nil {
		t.Fatalf("judge should tolerate missing relation context: %v", err)
	}
	if len(summary.Items) != 1 || len(summary.Items[0].RelationIDs) != 1 || len(summary.Items[0].RelationContext) != 0 {
		t.Fatalf("expected raw relation id without fabricated relation context: %+v", summary.Items)
	}
	page, err := NextSemanticJudgmentPage(filepath.Join(out, "semantic-judgment"))
	if err != nil {
		t.Fatalf("next semantic judgment page: %v", err)
	}
	if !strings.Contains(page.PageMarkdown, "Relation context unavailable for: "+candidate.RelationIDs[0]) {
		t.Fatalf("expected explicit unavailable relation context:\n%s", page.PageMarkdown)
	}
	if summary.EvalCountedCount != 0 || summary.EvidenceExcludedCount != 1 || summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingSourceExcerpt] != 1 || summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingRelationContext] != 1 {
		t.Fatalf("expected missing source/relation readiness exclusion: %+v", summary)
	}
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-strict-missing-relation",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-action",
			ExpectedState:          ExpectedOutcomePresent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			RelationRequirements:   []SemanticRelationshipType{SemanticRelationshipDerivedFrom},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	if _, err := AcceptSemantic(semanticRun, answerKey, t.TempDir()); err == nil || !strings.Contains(err.Error(), "read semantic relation") {
		t.Fatalf("expected acceptance to remain strict for missing relation, got %v", err)
	}
}

func TestSemanticJudgmentEvidenceReadinessExcludesUnsafeAndBlockedCandidates(t *testing.T) {
	node := validStructureNode()
	unsafeCandidate := validSemanticCandidate(validSemanticObservation(node), node)
	unsafeCandidate.CandidateID = "cand-unsafe"
	unsafeCandidate.Title = "Unsafe candidate"
	unsafeCandidate.Blockers = []Blocker{{Code: "unsafe", Message: "contains " + unsafeTokenMarker()}}
	blockedCandidate := validSemanticCandidate(validSemanticObservation(node), node)
	blockedCandidate.CandidateID = "cand-blocked"
	blockedCandidate.ReviewStatus = ReviewStatusBlocked
	blockedCandidate.RelationIDs = nil

	items := semanticJudgmentCandidates([]SemanticCandidate{unsafeCandidate, blockedCandidate}, nil, nil, semanticCalibrationSourceContext{}, nil)
	summary := BuildSemanticJudgmentSummary("run-demo", 1, items, nil)

	if summary.EvalCountedCount != 0 || summary.EvidenceExcludedCount != 2 {
		t.Fatalf("expected both candidates excluded: %+v", summary)
	}
	if summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessPrivateOrGovernanceMarker] != 1 ||
		summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessBlockedOrSkipped] != 1 ||
		summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingRelationContext] == 0 {
		t.Fatalf("expected readiness reason counts for unsafe and blocked candidates: %+v", summary.EvidenceReadinessReasonCounts)
	}
	var unsafeItem SemanticJudgmentCandidate
	for _, item := range items {
		if item.CandidateID == unsafeCandidate.CandidateID {
			unsafeItem = item
			break
		}
	}
	if err := ValidateSemanticJudgmentCandidate(unsafeItem); err == nil || !strings.Contains(err.Error(), "private marker") {
		t.Fatalf("expected unsafe candidate validation failure, got %v", err)
	}
}

func TestSemanticJudgmentRejectsForgedEvidenceReadiness(t *testing.T) {
	item := SemanticJudgmentCandidate{
		SchemaVersion:     SemanticJudgmentCandidateSchemaVersion,
		CandidateID:       "cand-forged",
		RunID:             "run-forged",
		SourceDocumentID:  "doc-demo",
		CandidateKind:     SemanticCandidateKindAction,
		ReviewStatus:      ReviewStatusReady,
		Confidence:        ConfidenceMedium,
		Title:             "Forged readiness",
		Summary:           "This candidate claims readiness without evidence.",
		EvidenceReadiness: SemanticEvidenceReadiness{Status: SemanticEvidenceReadinessPass, EvalCounted: true},
	}
	if err := ValidateSemanticJudgmentCandidate(item); err == nil || !strings.Contains(err.Error(), "does not match evidence state") {
		t.Fatalf("expected forged readiness validation failure, got %v", err)
	}
	summary := SemanticJudgmentSummary{
		SchemaVersion:                 SemanticJudgmentSummarySchemaVersion,
		RunID:                         "run-forged",
		CandidateCount:                1,
		EvidenceReadyCount:            1,
		EvalCountedCount:              1,
		EvidenceReadinessReasonCounts: semanticEvidenceReadinessReasonCountMap(),
		FailureModeCounts:             map[SemanticJudgmentChoice]int{},
		FailureReasonCounts:           emptySemanticFailureReasonCounts(),
		Candidates: []SemanticJudgmentCandidateSummary{{
			CandidateID:              item.CandidateID,
			CandidateKind:            item.CandidateKind,
			ReviewStatus:             item.ReviewStatus,
			Confidence:               item.Confidence,
			CandidatePath:            SemanticJudgmentCandidateJSONPath(item.CandidateID),
			PagePath:                 SemanticJudgmentPagePath(item.CandidateID),
			SourceDocumentID:         item.SourceDocumentID,
			EvidenceReadinessStatus:  SemanticEvidenceReadinessPass,
			EvalCounted:              true,
			EvidenceReadinessReasons: nil,
		}},
		Items: []SemanticJudgmentCandidate{item},
	}
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "does not match evidence state") {
		t.Fatalf("expected summary to reject forged readiness, got %v", err)
	}
}

func TestSemanticJudgmentWriterValidatesCandidateArtifactsBeforeWrite(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	item := semanticJudgmentCandidates([]SemanticCandidate{candidate}, []SemanticRelation{validSemanticRelation(candidate, observation, node)}, []SemanticObservation{observation}, semanticCalibrationSourceContext{
		Label: "source.md",
		Lines: []string{"one", "two"},
	}, nil)[0]
	item.CandidateKind = SemanticCandidateKind("unsupported_kind")
	summary := BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, nil)

	if err := WriteSemanticJudgment(t.TempDir(), summary); err == nil || !strings.Contains(err.Error(), "unsupported semantic judgment candidate kind") {
		t.Fatalf("expected candidate artifact validation failure before write, got %v", err)
	}
}

func TestSemanticJudgmentLegacySummaryReadFailsClosed(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	root := filepath.Join(out, "semantic-judgment")
	summaryPath := filepath.Join(root, "judgment-summary.json")
	var summary SemanticJudgmentSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	summary.SchemaVersion = SemanticJudgmentSummaryLegacySchemaVersion
	summary.EvidenceReadyCount = 0
	summary.EvalCountedCount = 0
	summary.EvidenceExcludedCount = 0
	summary.EvidenceReadinessReasonCounts = nil
	for i := range summary.Candidates {
		summary.Candidates[i].EvidenceReadinessStatus = ""
		summary.Candidates[i].EvalCounted = false
		summary.Candidates[i].EvidenceReadinessReasons = nil
	}
	writeDocumentsTestJSON(t, summaryPath, summary)

	loaded, err := ReadSemanticJudgmentSummary(root)
	if err != nil {
		t.Fatalf("read legacy judgment summary: %v", err)
	}
	if loaded.SchemaVersion != SemanticJudgmentSummarySchemaVersion ||
		loaded.EvalCountedCount != 0 ||
		loaded.EvidenceExcludedCount != loaded.CandidateCount ||
		loaded.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingSourceExcerpt] != loaded.CandidateCount {
		t.Fatalf("legacy summary must be normalized as fail-closed, got %+v", loaded)
	}
	for _, item := range loaded.Candidates {
		if item.EvidenceReadinessStatus != SemanticEvidenceReadinessFail || item.EvalCounted || !containsSemanticEvidenceReadinessReason(item.EvidenceReadinessReasons, SemanticEvidenceReadinessMissingSourceExcerpt) {
			t.Fatalf("legacy candidate summary must be fail-closed, got %+v", item)
		}
	}
}

func TestSemanticJudgmentPreviousSummaryReadPreservesReadiness(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticObservationJSONPath(observation.ObservationID)), observation)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{SourceRoot: sourceRoot, SourcePath: "source.md"}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	root := filepath.Join(out, "semantic-judgment")
	summaryPath := filepath.Join(root, "judgment-summary.json")
	var summary SemanticJudgmentSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.EvalCountedCount != 1 || summary.Candidates[0].EvidenceReadinessStatus != SemanticEvidenceReadinessPass {
		t.Fatalf("expected generated summary to start eval-counted: %+v", summary)
	}
	summary.SchemaVersion = SemanticJudgmentSummaryPreviousSchemaVersion
	summary.FailureReasonCounts = nil
	summary.JudgmentByFailureReason = nil
	writeDocumentsTestJSON(t, summaryPath, summary)

	itemPath := filepath.Join(root, summary.Candidates[0].CandidatePath)
	var item SemanticJudgmentCandidate
	data, err = os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("read item: %v", err)
	}
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatalf("decode item: %v", err)
	}
	item.SchemaVersion = SemanticJudgmentCandidatePreviousSchemaVersion
	writeDocumentsTestJSON(t, itemPath, item)

	updated, err := RecordSemanticJudgment(root, SemanticJudgmentRecordInput{
		CandidateID: summary.Candidates[0].CandidateID,
		Choice:      SemanticJudgmentChoiceAccept,
		ReviewerID:  "tester",
		RecordedAt:  fixedTestTime(),
	})
	if err != nil {
		t.Fatalf("record judgment against previous summary: %v", err)
	}
	if updated.SchemaVersion != SemanticJudgmentSummarySchemaVersion ||
		updated.JudgedCount != 1 ||
		updated.AcceptedCount != 1 ||
		updated.EvalCountedCount != 1 ||
		updated.EvidenceReadyCount != 1 ||
		updated.EvidenceExcludedCount != 0 ||
		updated.Candidates[0].EvidenceReadinessStatus != SemanticEvidenceReadinessPass ||
		!updated.Candidates[0].EvalCounted ||
		len(updated.Candidates[0].EvidenceReadinessReasons) != 0 {
		t.Fatalf("previous summary readiness must be preserved after append, got %+v", updated)
	}
	if updated.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingSourceExcerpt] != 0 {
		t.Fatalf("previous summary must not gain missing source excerpt reason after append: %+v", updated.EvidenceReadinessReasonCounts)
	}

	var persistedItem SemanticJudgmentCandidate
	data, err = os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("read persisted item: %v", err)
	}
	if err := json.Unmarshal(data, &persistedItem); err != nil {
		t.Fatalf("decode persisted item: %v", err)
	}
	if persistedItem.SchemaVersion != SemanticJudgmentCandidateSchemaVersion ||
		persistedItem.EvidenceReadiness.Status != SemanticEvidenceReadinessPass ||
		!persistedItem.EvidenceReadiness.EvalCounted ||
		len(persistedItem.EvidenceReadiness.ReasonCodes) != 0 {
		t.Fatalf("previous candidate readiness must be preserved after append, got %+v", persistedItem)
	}
}

func TestSemanticJudgmentPreviousSummaryReadFailsClosedForMissingReadiness(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	root := filepath.Join(out, "semantic-judgment")
	summaryPath := filepath.Join(root, "judgment-summary.json")
	var summary SemanticJudgmentSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	summary.SchemaVersion = SemanticJudgmentSummaryPreviousSchemaVersion
	summary.EvidenceReadyCount = 1
	summary.EvalCountedCount = 1
	summary.EvidenceExcludedCount = 0
	summary.EvidenceReadinessReasonCounts = nil
	summary.FailureReasonCounts = nil
	summary.JudgmentByFailureReason = nil
	summary.Candidates[0].EvidenceReadinessStatus = ""
	summary.Candidates[0].EvalCounted = true
	summary.Candidates[0].EvidenceReadinessReasons = nil
	writeDocumentsTestJSON(t, summaryPath, summary)

	loaded, err := ReadSemanticJudgmentSummary(root)
	if err != nil {
		t.Fatalf("read previous judgment summary: %v", err)
	}
	if loaded.SchemaVersion != SemanticJudgmentSummarySchemaVersion ||
		loaded.EvidenceReadyCount != 0 ||
		loaded.EvalCountedCount != 0 ||
		loaded.EvidenceExcludedCount != 1 ||
		loaded.Candidates[0].EvidenceReadinessStatus != SemanticEvidenceReadinessFail ||
		loaded.Candidates[0].EvalCounted ||
		!containsSemanticEvidenceReadinessReason(loaded.Candidates[0].EvidenceReadinessReasons, SemanticEvidenceReadinessMissingSourceExcerpt) ||
		loaded.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingSourceExcerpt] != 1 {
		t.Fatalf("previous summary missing readiness must fail closed, got %+v", loaded)
	}
}

func TestSemanticJudgmentReadRejectsSummaryCandidateReadinessDrift(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	writeDocumentsTestJSON(t, filepath.Join(semanticRun, "semantic-candidates", SemanticObservationJSONPath(observation.ObservationID)), observation)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{SourceRoot: sourceRoot, SourcePath: "source.md"}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	root := filepath.Join(out, "semantic-judgment")
	summaryPath := filepath.Join(root, "judgment-summary.json")
	var summary SemanticJudgmentSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	summary.Candidates[0].EvidenceReadinessStatus = SemanticEvidenceReadinessFail
	summary.Candidates[0].EvalCounted = false
	summary.Candidates[0].EvidenceReadinessReasons = []SemanticEvidenceReadinessReason{SemanticEvidenceReadinessMissingSourceExcerpt}
	summary.EvidenceReadyCount = 0
	summary.EvalCountedCount = 0
	summary.EvidenceExcludedCount = 1
	summary.EvidenceReadinessReasonCounts = semanticEvidenceReadinessReasonCountMap()
	summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingSourceExcerpt] = 1
	writeDocumentsTestJSON(t, summaryPath, summary)

	if _, err := NextSemanticJudgmentPage(root); err == nil || !strings.Contains(err.Error(), "summary readiness does not match item") {
		t.Fatalf("expected summary/candidate readiness drift rejection, got %v", err)
	}
}

func TestSemanticJudgmentSummaryRejectsReadinessInconsistency(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	item := semanticJudgmentCandidates([]SemanticCandidate{candidate}, []SemanticRelation{validSemanticRelation(candidate, observation, node)}, []SemanticObservation{observation}, semanticCalibrationSourceContext{
		Label: "source.md",
		Lines: []string{"one", "two"},
	}, nil)[0]
	summary := BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, nil)

	summary.Candidates[0].EvidenceReadinessStatus = SemanticEvidenceReadinessFail
	summary.Candidates[0].EvalCounted = false
	summary.Candidates[0].EvidenceReadinessReasons = []SemanticEvidenceReadinessReason{SemanticEvidenceReadinessMissingSourceExcerpt}
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "candidate summaries") {
		t.Fatalf("expected aggregate/candidate summary mismatch failure, got %v", err)
	}

	summary = BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, nil)
	summary.Candidates[0].EvidenceReadinessStatus = SemanticEvidenceReadinessFail
	summary.Candidates[0].EvalCounted = false
	summary.Candidates[0].EvidenceReadinessReasons = []SemanticEvidenceReadinessReason{SemanticEvidenceReadinessMissingSourceExcerpt}
	summary.EvidenceReadyCount = 0
	summary.EvalCountedCount = 0
	summary.EvidenceExcludedCount = 1
	summary.EvidenceReadinessReasonCounts = semanticEvidenceReadinessReasonCountMap()
	summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingSourceExcerpt] = 1
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "does not match item") {
		t.Fatalf("expected item/candidate summary mismatch failure, got %v", err)
	}

	summary = BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, nil)
	summary.Items = nil
	summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessReason("WP-21")] = 1
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "unsupported semantic judgment evidence readiness reason count") {
		t.Fatalf("expected unsupported readiness reason count failure, got %v", err)
	}

	summary = BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, nil)
	summary.Items = nil
	summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessReason("private_content")] = 1
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "unsupported semantic judgment evidence readiness reason count") {
		t.Fatalf("expected unsafe readiness reason count failure, got %v", err)
	}

	summary = BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, nil)
	summary.Candidates = append(summary.Candidates, SemanticJudgmentCandidateSummary{
		CandidateID:             "cand-missing-item",
		CandidateKind:           SemanticCandidateKindAction,
		ReviewStatus:            ReviewStatusReady,
		Confidence:              ConfidenceMedium,
		CandidatePath:           SemanticJudgmentCandidateJSONPath("cand-missing-item"),
		PagePath:                SemanticJudgmentPagePath("cand-missing-item"),
		EvidenceReadinessStatus: SemanticEvidenceReadinessPass,
		EvalCounted:             true,
	})
	summary.CandidateCount = 2
	summary.EvidenceReadyCount = 2
	summary.EvalCountedCount = 2
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "has no matching item") {
		t.Fatalf("expected missing item failure, got %v", err)
	}
}

func TestSemanticJudgmentAbsentReadinessFailsClosedInSummary(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	item := semanticJudgmentCandidates([]SemanticCandidate{candidate}, []SemanticRelation{validSemanticRelation(candidate, observation, node)}, []SemanticObservation{observation}, semanticCalibrationSourceContext{
		Label: "source.md",
		Lines: []string{"one", "two"},
	}, nil)[0]
	item.EvidenceReadiness = SemanticEvidenceReadiness{}

	summary := BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, nil)
	if summary.EvalCountedCount != 0 || summary.EvidenceExcludedCount != 1 || summary.EvidenceReadinessReasonCounts[SemanticEvidenceReadinessMissingSourceExcerpt] != 1 {
		t.Fatalf("absent readiness must fail closed, got %+v", summary)
	}
}

func TestSemanticJudgmentReadinessRequiresMatchingRelationIDs(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	item := semanticJudgmentCandidates([]SemanticCandidate{candidate}, []SemanticRelation{validSemanticRelation(candidate, observation, node)}, []SemanticObservation{observation}, semanticCalibrationSourceContext{
		Label: "source.md",
		Lines: []string{"one", "two"},
	}, nil)[0]
	item.RelationIDs = []string{"rel-missing"}
	item.EvidenceReadiness = semanticEvidenceReadiness(item)

	if item.EvidenceReadiness.EvalCounted || !containsSemanticEvidenceReadinessReason(item.EvidenceReadiness.ReasonCodes, SemanticEvidenceReadinessMissingRelationContext) {
		t.Fatalf("expected mismatched relation id to fail readiness: %+v", item.EvidenceReadiness)
	}
}

func TestSemanticJudgmentToleratesUnreadableObservationContext(t *testing.T) {
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	observationPath := filepath.Join(semanticRun, "semantic-candidates", SemanticObservationJSONPath(observation.ObservationID))
	if err := os.MkdirAll(filepath.Dir(observationPath), 0o755); err != nil {
		t.Fatalf("mkdir observation fixture: %v", err)
	}
	if err := os.WriteFile(observationPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("corrupt observation fixture: %v", err)
	}

	out := t.TempDir()
	summary, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{})
	if err != nil {
		t.Fatalf("judge should tolerate unreadable observation context: %v", err)
	}
	if len(summary.Items) != 1 || len(summary.Items[0].RelationContext) != 1 {
		t.Fatalf("expected relation context from readable relation: %+v", summary.Items)
	}
	endpoint := summary.Items[0].RelationContext[0].OtherEndpoint
	if !endpoint.Unavailable || endpoint.UnavailableReason != "endpoint context unavailable" {
		t.Fatalf("expected unavailable observation endpoint context, got %+v", endpoint)
	}
}

func TestSemanticJudgmentRecordsChoiceAndUpdatesReport(t *testing.T) {
	node := validStructureNode()
	action := validSemanticCandidate(validSemanticObservation(node), node)
	decision := unexpectedDecisionCandidate()
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{action, decision})
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	summary, err := RecordSemanticJudgment(filepath.Join(out, "semantic-judgment"), SemanticJudgmentRecordInput{
		CandidateID: action.CandidateID,
		Choice:      SemanticJudgmentChoiceAccept,
		Note:        "Useful action.",
		ReviewerID:  "tester",
		RecordedAt:  fixedTestTime(),
	})
	if err != nil {
		t.Fatalf("record semantic judgment: %v", err)
	}
	if summary.JudgedCount != 1 || summary.AcceptedCount != 1 || summary.RemainingCount != 1 || summary.PrecisionEstimate != 1 {
		t.Fatalf("unexpected judgment counts after accept: %+v", summary)
	}
	if summary.JudgmentByCandidateKind[SemanticCandidateKindAction][SemanticJudgmentChoiceAccept] != 1 ||
		summary.JudgmentByConfidence[ConfidenceMedium][SemanticJudgmentChoiceAccept] != 1 ||
		summary.JudgmentByReviewStatus[ReviewStatusReady][SemanticJudgmentChoiceAccept] != 1 ||
		summary.JudgmentBySourceDocument["doc-demo"][SemanticJudgmentChoiceAccept] != 1 ||
		summary.JudgmentByRelationPresence["with_relations"][SemanticJudgmentChoiceAccept] != 1 ||
		summary.JudgmentByRelationType[SemanticRelationshipDerivedFrom][SemanticJudgmentChoiceAccept] != 1 {
		t.Fatalf("expected grouped judgment analytics after accept: %+v", summary)
	}
	if _, err := RecordSemanticJudgment(filepath.Join(out, "semantic-judgment"), SemanticJudgmentRecordInput{
		CandidateID: action.CandidateID,
		Choice:      SemanticJudgmentChoiceReject,
		RecordedAt:  fixedTestTime(),
	}); err == nil {
		t.Fatalf("expected duplicate judgment to fail closed")
	}
	summary, err = RecordSemanticJudgment(filepath.Join(out, "semantic-judgment"), SemanticJudgmentRecordInput{
		CandidateID:      decision.CandidateID,
		Choice:           SemanticJudgmentChoiceReject,
		FailureReason:    SemanticFailureUnexpectedCandidate,
		SecondaryReasons: []SemanticFailureReason{SemanticFailureUnsupportedEvidence},
		ReviewerID:       "tester",
		RecordedAt:       fixedTestTime(),
	})
	if err != nil {
		t.Fatalf("record rejected semantic judgment: %v", err)
	}
	reasonByCandidate := map[string]SemanticFailureReason{}
	for _, candidate := range summary.Candidates {
		reasonByCandidate[candidate.CandidateID] = candidate.FailureReason
	}
	if summary.FailureReasonCounts[SemanticFailureUnexpectedCandidate] != 1 ||
		summary.JudgmentByFailureReason[SemanticFailureUnexpectedCandidate][SemanticJudgmentChoiceReject] != 1 ||
		reasonByCandidate[decision.CandidateID] != SemanticFailureUnexpectedCandidate {
		t.Fatalf("expected failure reason aggregation after reject: %+v", summary)
	}
	report, err := os.ReadFile(filepath.Join(out, "semantic-judgment", "reports", "judgment-report.md"))
	if err != nil {
		t.Fatalf("read judgment report: %v", err)
	}
	if !strings.Contains(string(report), "Accepted: 1") ||
		!strings.Contains(string(report), "Review burden: 1") ||
		!strings.Contains(string(report), "unexpected_candidate: 1") ||
		!strings.Contains(string(report), "By candidate kind") ||
		!strings.Contains(string(report), "action_candidate accept=1") ||
		!strings.Contains(string(report), "derived_from accept=1") {
		t.Fatalf("report did not update counts:\n%s", string(report))
	}
}

func TestSemanticJudgmentFailureReasonContract(t *testing.T) {
	base := SemanticJudgmentRecord{
		SchemaVersion:    SemanticJudgmentRecordSchemaVersion,
		RunID:            "run-demo",
		CandidateID:      "cand-demo",
		SourceDocumentID: "doc-demo",
		CandidateKind:    SemanticCandidateKindAction,
		Confidence:       ConfidenceMedium,
		RecordedAt:       fixedTestTime().Format(time.RFC3339),
	}
	record := base
	record.Choice = SemanticJudgmentChoiceReject
	if err := ValidateSemanticJudgmentRecord(record); err == nil || !strings.Contains(err.Error(), "requires failure reason") {
		t.Fatalf("expected non-accept without reason to fail, got %v", err)
	}
	record = base
	record.Choice = SemanticJudgmentChoiceAccept
	record.FailureReason = SemanticFailureUnexpectedCandidate
	if err := ValidateSemanticJudgmentRecord(record); err == nil || !strings.Contains(err.Error(), "cannot include failure reason") {
		t.Fatalf("expected accept with reason to fail, got %v", err)
	}
	record = base
	record.Choice = SemanticJudgmentChoiceDuplicate
	record.FailureReason = SemanticFailureMissingEvidence
	if err := ValidateSemanticJudgmentRecord(record); err == nil || !strings.Contains(err.Error(), "cannot use failure reason") {
		t.Fatalf("expected incompatible reason to fail, got %v", err)
	}
	record = base
	record.Choice = SemanticJudgmentChoiceReject
	record.FailureReason = SemanticFailureUnsupportedEvidence
	record.SecondaryReasons = []SemanticFailureReason{SemanticFailureMissingEvidence}
	if err := ValidateSemanticJudgmentRecord(record); err != nil {
		t.Fatalf("expected valid non-accept reason, got %v", err)
	}
}

func TestSemanticFailureReasonMappingsCoverExistingValues(t *testing.T) {
	acceptanceReasons := []SemanticAcceptanceReason{
		SemanticAcceptanceReasonCorrect,
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
		SemanticAcceptanceReasonUnexpectedCandidate,
	}
	for _, reason := range acceptanceReasons {
		mapped, _, ok := semanticFailureReasonForAcceptanceReason(reason)
		if reason == SemanticAcceptanceReasonCorrect {
			if ok || mapped != "" {
				t.Fatalf("correct should not map to failure reason: %s -> %s", reason, mapped)
			}
			continue
		}
		if !ok || !validSemanticFailureReason(mapped) {
			t.Fatalf("acceptance reason lacks canonical mapping: %s -> %s ok=%t", reason, mapped, ok)
		}
	}
	for _, class := range semanticCalibrationFailureClasses {
		mapped, inferred, ok := semanticFailureReasonForCalibrationClass(class)
		if class == SemanticCalibrationFailureAccepted {
			if ok || mapped != "" || !inferred {
				t.Fatalf("accepted class should not map to failure reason: %s -> %s ok=%t inferred=%t", class, mapped, ok, inferred)
			}
			continue
		}
		if !ok || !inferred || !validSemanticFailureReason(mapped) {
			t.Fatalf("calibration class lacks inferred canonical mapping: %s -> %s ok=%t inferred=%t", class, mapped, ok, inferred)
		}
	}
	if mapped, inferred, ok := semanticFailureReasonForAcceptanceReason(SemanticAcceptanceReason("new_reason")); !ok || !inferred || mapped != SemanticFailureOther {
		t.Fatalf("unknown acceptance reason must map to inferred other, got %s inferred=%t ok=%t", mapped, inferred, ok)
	}
	if mapped, inferred, ok := semanticFailureReasonForCalibrationClass(SemanticCalibrationFailureClass("new_class")); !ok || !inferred || mapped != SemanticFailureOther {
		t.Fatalf("unknown calibration class must map to inferred other, got %s inferred=%t ok=%t", mapped, inferred, ok)
	}
}

func TestSemanticJudgmentSerializesConcurrentRecords(t *testing.T) {
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	root := filepath.Join(out, "semantic-judgment")

	const attempts = 12
	start := make(chan struct{})
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		i := i
		go func() {
			<-start
			choice := SemanticJudgmentChoiceReject
			reason := SemanticFailureUnexpectedCandidate
			if i%2 == 0 {
				choice = SemanticJudgmentChoiceAccept
				reason = ""
			}
			_, err := RecordSemanticJudgment(root, SemanticJudgmentRecordInput{
				CandidateID:   candidate.CandidateID,
				Choice:        choice,
				FailureReason: reason,
				ReviewerID:    fmt.Sprintf("reviewer-%d", i),
				RecordedAt:    fixedTestTime(),
			})
			results <- err
		}()
	}
	close(start)

	successes := 0
	for i := 0; i < attempts; i++ {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		if !strings.Contains(err.Error(), "semantic judgment already exists") {
			t.Fatalf("expected duplicate judgment error, got %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one concurrent judgment success, got %d", successes)
	}
	summary, err := ReadSemanticJudgmentSummary(root)
	if err != nil {
		t.Fatalf("read semantic judgment summary: %v", err)
	}
	if summary.JudgedCount != 1 || summary.RemainingCount != 0 {
		t.Fatalf("expected one durable judgment after concurrent writes: %+v", summary)
	}
}

func TestSemanticJudgmentNextPageDoesNotMutateCursorArtifact(t *testing.T) {
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	root := filepath.Join(out, "semantic-judgment")
	cursorPath := filepath.Join(root, "cursor.json")
	oldTime := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(cursorPath, oldTime, oldTime); err != nil {
		t.Fatalf("set cursor timestamp: %v", err)
	}
	if _, err := NextSemanticJudgmentPage(root); err != nil {
		t.Fatalf("next semantic judgment page: %v", err)
	}
	info, err := os.Stat(cursorPath)
	if err != nil {
		t.Fatalf("stat cursor: %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatalf("next page should not rewrite cursor artifact; got modtime %s want %s", info.ModTime(), oldTime)
	}
}

func TestSemanticJudgmentRejectsUnsafeInputs(t *testing.T) {
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	out := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, out, SemanticJudgmentOptions{}); err != nil {
		t.Fatalf("judge semantic candidates: %v", err)
	}
	if _, err := RecordSemanticJudgment(filepath.Join(out, "semantic-judgment"), SemanticJudgmentRecordInput{
		CandidateID: "cand-missing",
		Choice:      SemanticJudgmentChoiceAccept,
		RecordedAt:  fixedTestTime(),
	}); err == nil {
		t.Fatalf("expected unknown candidate to fail closed")
	}
	if _, err := RecordSemanticJudgment(filepath.Join(out, "semantic-judgment"), SemanticJudgmentRecordInput{
		CandidateID: candidate.CandidateID,
		Choice:      SemanticJudgmentChoice("promote"),
		RecordedAt:  fixedTestTime(),
	}); err == nil {
		t.Fatalf("expected unsupported judgment choice to fail closed")
	}
}

func TestSemanticJudgmentSummaryRejectsUnsafeAnalyticsKeys(t *testing.T) {
	summary := BuildSemanticJudgmentSummary("run-demo", 1, nil, nil)
	summary.JudgmentBySourceDocument = map[string]map[SemanticJudgmentChoice]int{
		"doc-" + unsafeTokenMarker(): {SemanticJudgmentChoiceAccept: 1},
	}
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "private marker") {
		t.Fatalf("expected unsafe source analytics key to fail validation, got %v", err)
	}

	summary = BuildSemanticJudgmentSummary("run-demo", 1, nil, nil)
	summary.JudgmentByRelationType = map[SemanticRelationshipType]map[SemanticJudgmentChoice]int{
		SemanticRelationshipType("DEC-49"): {SemanticJudgmentChoiceReject: 1},
	}
	if err := ValidateSemanticJudgmentSummary(summary); err == nil || !strings.Contains(err.Error(), "private marker") {
		t.Fatalf("expected governance relation analytics key to fail validation, got %v", err)
	}
}

func TestSemanticJudgmentRejectsEscapingSourcePath(t *testing.T) {
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	sourceRoot := t.TempDir()
	if _, err := JudgeSemanticCandidates(semanticRun, t.TempDir(), SemanticJudgmentOptions{
		SourceRoot: sourceRoot,
		SourcePath: "../outside.md",
	}); err == nil {
		t.Fatalf("expected escaping source path to fail closed")
	}
}

func TestSemanticJudgmentRejectsSymlinkedOutputParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on windows")
	}
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{candidate})
	linkParent := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(linkParent, "linked")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := JudgeSemanticCandidates(semanticRun, filepath.Join(linkParent, "linked", "out"), SemanticJudgmentOptions{}); err == nil {
		t.Fatalf("expected symlinked output parent to fail closed")
	}
}

func TestSemanticCalibrationThresholdCannotLowerTrustGate(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
		unexpectedDecisionCandidate(),
	}, true)
	summary, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.50, HeldOut: true})
	if err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	if summary.Threshold != 0.98 {
		t.Fatalf("threshold below 0.98 must be raised to hard minimum: %+v", summary)
	}
	if summary.ThresholdStatus != SemanticCalibrationThresholdNotTrusted || summary.NoHumanEligible {
		t.Fatalf("low requested threshold must not trust a 50%% batch: %+v", summary)
	}
}

func TestSemanticCalibrationRequiresHeldOutEvidenceForTrust(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	notHeldOut, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98})
	if err != nil {
		t.Fatalf("calibrate non-held-out batch: %v", err)
	}
	if notHeldOut.ThresholdStatus != SemanticCalibrationThresholdNotTrusted || notHeldOut.NoHumanEligible {
		t.Fatalf("non-held-out batch must not be trusted: %+v", notHeldOut)
	}
	heldOut, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("calibrate held-out batch: %v", err)
	}
	if heldOut.ThresholdStatus != SemanticCalibrationThresholdTrusted || !heldOut.NoHumanEligible {
		t.Fatalf("perfect held-out batch should be trusted: %+v", heldOut)
	}
}

func TestSemanticCalibrationDoesNotDoubleCountFalseNegativesAsMissingEvidence(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, nil, true)
	summary, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	if summary.FailureClassCounts[SemanticCalibrationFailureFalseNegative] != 1 {
		t.Fatalf("expected one false negative: %+v", summary)
	}
	if summary.FailureClassCounts[SemanticCalibrationFailureMissingEvidence] != 0 {
		t.Fatalf("false negative must not be double-counted as missing evidence: %+v", summary)
	}
	if summary.ScoredCount != 1 || summary.MeasuredAccuracy != 0 {
		t.Fatalf("unexpected denominator for missed expected outcome: %+v", summary)
	}
}

func TestSemanticCalibrationFailureTaxonomyCoversAllClasses(t *testing.T) {
	item := validSemanticAcceptanceItemForCalibration()
	items := []SemanticAcceptanceItem{
		item,
		semanticAcceptanceItemWith("cand-fp", SemanticAcceptanceRejected, SemanticAcceptanceReasonUnexpectedCandidate),
		semanticAcceptanceItemWith("cand-missing-evidence", SemanticAcceptanceRejected, SemanticAcceptanceReasonMissingEvidence),
		semanticAcceptanceItemWith("cand-duplicate", SemanticAcceptanceRejected, SemanticAcceptanceReasonDuplicate),
		semanticAcceptanceItemWith("cand-needs-review", SemanticAcceptanceNeedsReview, SemanticAcceptanceReasonAmbiguous),
		semanticAcceptanceItemWith("cand-blocked", SemanticAcceptanceBlocked, SemanticAcceptanceReasonUnsafeOrPrivate),
		semanticAcceptanceItemWith("cand-other", SemanticAcceptanceRejected, SemanticAcceptanceReasonWrongKind),
	}
	expected := []SemanticExpectedOutcomeResult{
		{
			SchemaVersion:     SemanticAcceptanceExpectedOutcomeSchemaVersion,
			ExpectedOutcomeID: "exp-missed",
			ExpectedState:     ExpectedOutcomePresent,
			ExpectedKind:      SemanticCandidateKindAction,
			AcceptanceState:   SemanticAcceptanceRejected,
			Reason:            SemanticAcceptanceReasonMissingExpectedOutcome,
			ExpectedPath:      SemanticAcceptanceExpectedOutcomeJSONPath("exp-missed"),
		},
	}
	summary := BuildSemanticCalibrationSummary(SemanticAcceptanceSummary{
		SchemaVersion: SemanticAcceptanceSummarySchemaVersion,
		RunID:         "run-sem-demo",
		AnswerKeyID:   "ak-taxonomy",
	}, items, expected, 0.98, true)
	for _, class := range semanticCalibrationFailureClasses {
		if _, ok := summary.FailureClassCounts[class]; !ok {
			t.Fatalf("missing failure class count for %s: %+v", class, summary.FailureClassCounts)
		}
	}
	want := map[SemanticCalibrationFailureClass]int{
		SemanticCalibrationFailureAccepted:             1,
		SemanticCalibrationFailureFalsePositive:        1,
		SemanticCalibrationFailureFalseNegative:        1,
		SemanticCalibrationFailureMissingEvidence:      1,
		SemanticCalibrationFailureDuplicate:            1,
		SemanticCalibrationFailureNeedsReviewAmbiguity: 1,
		SemanticCalibrationFailureBlockedPrivate:       1,
		SemanticCalibrationFailureOther:                1,
		SemanticCalibrationFailureRelationError:        0,
		SemanticCalibrationFailureSourceScopeError:     0,
	}
	for class, count := range want {
		if summary.FailureClassCounts[class] != count {
			t.Fatalf("unexpected count for %s: got %d want %d in %+v", class, summary.FailureClassCounts[class], count, summary.FailureClassCounts)
		}
	}
	if summary.ScoredCount != 7 {
		t.Fatalf("blocked_private must not contribute to scored denominator: %+v", summary)
	}
}

func TestSemanticCalibrationNextPaginatesOneItemAtATime(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
		unexpectedDecisionCandidate(),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	calibrationDir := filepath.Join(out, "semantic-calibration")
	first, err := NextSemanticCalibrationReviewPage(calibrationDir)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.Done || first.Item == nil || first.Cursor.ProcessedCount != 1 || first.Cursor.RemainingCount != 1 {
		t.Fatalf("first page must contain exactly one item and advance cursor: %+v", first)
	}
	second, err := NextSemanticCalibrationReviewPage(calibrationDir)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if second.Done || second.Item == nil || second.Item.ReviewItemID == first.Item.ReviewItemID || second.Cursor.ProcessedCount != 2 {
		t.Fatalf("second page must contain the next single item: first=%+v second=%+v", first, second)
	}
	done, err := NextSemanticCalibrationReviewPage(calibrationDir)
	if err != nil {
		t.Fatalf("done page: %v", err)
	}
	if !done.Done || done.Item != nil || !done.Cursor.Exhausted {
		t.Fatalf("third page must report exhaustion with no item: %+v", done)
	}
}

func TestSemanticCalibrationNextReturnsSelfContainedPageWithExpectedContextAndExcerpts(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte("Prepare checklist\nLead will prepare the checklist.\nExtra context\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: sourceRoot,
		SourcePath: "source.md",
	}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	page, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration"))
	if err != nil {
		t.Fatalf("next page: %v", err)
	}
	if page.SchemaVersion != SemanticCalibrationPageSchemaVersion {
		t.Fatalf("expected v0.3 page, got %+v", page)
	}
	if SemanticCalibrationPageSchemaVersion != "semantic-calibration-page/v0.3" {
		t.Fatalf("unexpected page schema constant: %s", SemanticCalibrationPageSchemaVersion)
	}
	if page.ReviewContext == nil || page.PageMarkdown == "" {
		t.Fatalf("expected review context and markdown: %+v", page)
	}
	if page.Item == nil || page.Item.ExpectedOutcome.LegacyContext || page.Item.ExpectedOutcome.Completeness != "complete" {
		t.Fatalf("new output must carry rich expected-outcome context: %+v", page.Item)
	}
	for _, want := range []string{
		"Prepare checklist",
		"Required evidence: node-demo",
		"Acceptable alternates: node-alt",
		"Title signals: checklist",
		"Summary signals: prepare",
		"Relation requirements: derived_from",
		"Minimum confidence floor: low",
		"Notes: Expected checklist action.",
		"Matched candidate: cand-demo",
		"Lead will prepare the checklist.",
		"Adjudication choices",
	} {
		if !strings.Contains(page.PageMarkdown, want) {
			t.Fatalf("page markdown missing %q:\n%s", want, page.PageMarkdown)
		}
	}
}

func TestSemanticCalibrationFalseNegativePageHasExpectedOutcomeContext(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, nil, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	page, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration"))
	if err != nil {
		t.Fatalf("next page: %v", err)
	}
	if page.Item == nil || page.Item.FailureClass != SemanticCalibrationFailureFalseNegative {
		t.Fatalf("expected false-negative page: %+v", page)
	}
	for _, want := range []string{
		"No candidate matched this expected outcome.",
		"Expected kind: action_candidate",
		"Required evidence: node-demo",
		"Acceptable alternates: node-alt",
		"Title signals: checklist",
		"Summary signals: prepare",
		"Relation requirements: derived_from",
		"Notes: Expected checklist action.",
	} {
		if !strings.Contains(page.PageMarkdown, want) {
			t.Fatalf("false-negative page missing %q:\n%s", want, page.PageMarkdown)
		}
	}
}

func TestSemanticCalibrationRejectsEscapingSourcePath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: t.TempDir(),
		SourcePath: "../source.md",
	}); err == nil {
		t.Fatalf("expected escaping source path to be rejected")
	}
}

func TestSemanticCalibrationRejectsAbsoluteSourcePath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	sourcePath := filepath.Join(t.TempDir(), "source.md")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: filepath.Dir(sourcePath),
		SourcePath: sourcePath,
	}); err == nil {
		t.Fatalf("expected absolute source path to be rejected")
	}
}

func TestSemanticCalibrationRejectsNonMarkdownSourcePath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.txt"), []byte("source"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: sourceRoot,
		SourcePath: "source.txt",
	}); err == nil {
		t.Fatalf("expected non-markdown source path to be rejected")
	}
}

func TestSemanticCalibrationRejectsSymlinkedSourceFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	sourceRoot := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("source"), 0o644); err != nil {
		t.Fatalf("write outside source: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(sourceRoot, "source.md")); err != nil {
		t.Fatalf("symlink source: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: sourceRoot,
		SourcePath: "source.md",
	}); err == nil {
		t.Fatalf("expected symlinked source file to be rejected")
	}
}

func TestSemanticCalibrationRejectsSymlinkedSourceRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	realRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(realRoot, "source.md"), []byte("source"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	linkRoot := filepath.Join(t.TempDir(), "source-root")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("symlink source root: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: linkRoot,
		SourcePath: "source.md",
	}); err == nil {
		t.Fatalf("expected symlinked source root to be rejected")
	}
}

func TestSemanticCalibrationRejectsSourceExcerptPrivateMarker(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte("Prepare checklist\ncontains "+unsafeTokenMarker()+"\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: sourceRoot,
		SourcePath: "source.md",
	}); err == nil {
		t.Fatalf("expected private marker in source excerpt to be rejected")
	}
}

func TestSemanticCalibrationClampsSourceExcerpts(t *testing.T) {
	candidate := validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode())
	candidate.EvidenceRanges = []SemanticEvidenceRange{
		{StructureNodeID: "node-one", LineStart: 1, LineEnd: 20},
		{StructureNodeID: "node-two", LineStart: 2, LineEnd: 20},
		{StructureNodeID: "node-three", LineStart: 3, LineEnd: 20},
		{StructureNodeID: "node-four", LineStart: 4, LineEnd: 20},
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{candidate}, true)
	sourceRoot := t.TempDir()
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, strings.Repeat("x", 300))
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: sourceRoot,
		SourcePath: "source.md",
	}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	page, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration"))
	if err != nil {
		t.Fatalf("next page: %v", err)
	}
	if got := len(page.Item.EvidenceExcerpts); got != maxSemanticCalibrationExcerptRanges {
		t.Fatalf("expected %d excerpts, got %d", maxSemanticCalibrationExcerptRanges, got)
	}
	total := 0
	for _, excerpt := range page.Item.EvidenceExcerpts {
		total += len(excerpt.Text)
		if excerpt.LineEnd-excerpt.LineStart+1 > maxSemanticCalibrationExcerptLines {
			t.Fatalf("excerpt line cap not enforced: %+v", excerpt)
		}
		if len(excerpt.Text) > maxSemanticCalibrationExcerptCharsPerRange {
			t.Fatalf("excerpt char cap not enforced: %d", len(excerpt.Text))
		}
		if !excerpt.Clamped {
			t.Fatalf("expected oversized excerpt to be marked clamped: %+v", excerpt)
		}
	}
	if total > maxSemanticCalibrationExcerptCharsPerItem {
		t.Fatalf("total excerpt cap not enforced: %d", total)
	}
}

func TestSemanticCalibrationClampsSourceExcerptBeyondEOF(t *testing.T) {
	candidate := validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode())
	candidate.EvidenceRanges = []SemanticEvidenceRange{{StructureNodeID: "node-late", LineStart: 99, LineEnd: 120}}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{candidate}, true)
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "source.md"), []byte("first\nlast\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{
		Threshold:  0.98,
		HeldOut:    true,
		SourceRoot: sourceRoot,
		SourcePath: "source.md",
	}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	page, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration"))
	if err != nil {
		t.Fatalf("next page: %v", err)
	}
	excerpt := page.Item.EvidenceExcerpts[0]
	if excerpt.Unavailable || !excerpt.Clamped || excerpt.LineStart != 2 || excerpt.LineEnd != 2 || excerpt.Text != "last" {
		t.Fatalf("expected beyond-EOF range clamped to final line: %+v", excerpt)
	}
}

func TestSemanticCalibrationLegacyReviewItemReturnsDegradedV2Page(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	itemPath := filepath.Join(out, "semantic-calibration", SemanticCalibrationReviewItemJSONPath("review-cand-demo"))
	var item SemanticCalibrationReviewItem
	data, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("read review item: %v", err)
	}
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatalf("decode review item: %v", err)
	}
	item.SchemaVersion = SemanticCalibrationReviewItemLegacySchemaVersion
	item.ExpectedOutcome = SemanticCalibrationExpectedOutcomeContext{}
	item.EvidenceExcerpts = nil
	writeDocumentsTestJSON(t, itemPath, item)
	page, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration"))
	if err != nil {
		t.Fatalf("next page: %v", err)
	}
	if page.SchemaVersion != SemanticCalibrationPageSchemaVersion || page.Item == nil || !page.Item.ExpectedOutcome.LegacyContext {
		t.Fatalf("expected degraded v0.2 page from legacy item: %+v", page)
	}
	if !strings.Contains(page.PageMarkdown, "not fully adjudication-ready") {
		t.Fatalf("expected legacy degradation marker:\n%s", page.PageMarkdown)
	}
}

func TestSemanticCalibrationRejectsEscapingInputItemPath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	summaryPath := filepath.Join(acceptanceDir, "acceptance-summary.json")
	var summary SemanticAcceptanceSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read acceptance summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode acceptance summary: %v", err)
	}
	summary.Candidates[0].ItemPath = "../escaped.json"
	writeDocumentsTestJSON(t, summaryPath, summary)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected escaping input item path to be rejected")
	}
}

func TestSemanticCalibrationRejectsEscapingInputPreviewPath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	summaryPath := filepath.Join(acceptanceDir, "acceptance-summary.json")
	var summary SemanticAcceptanceSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read acceptance summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode acceptance summary: %v", err)
	}
	summary.Candidates[0].PreviewPath = "../preview.md"
	writeDocumentsTestJSON(t, summaryPath, summary)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected escaping input preview path to be rejected")
	}
}

func TestSemanticCalibrationRejectsAcceptanceItemSummaryMismatch(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		unexpectedDecisionCandidate(),
	}, true)
	itemPath := filepath.Join(acceptanceDir, SemanticAcceptanceItemJSONPath("cand-unexpected"))
	var item SemanticAcceptanceItem
	data, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("read acceptance item: %v", err)
	}
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatalf("decode acceptance item: %v", err)
	}
	item.AcceptanceState = SemanticAcceptanceAccepted
	item.Reason = SemanticAcceptanceReasonCorrect
	writeDocumentsTestJSON(t, itemPath, item)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected acceptance item summary mismatch to be rejected")
	}
}

func TestSemanticCalibrationRejectsSymlinkedInputItem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	itemPath := filepath.Join(acceptanceDir, SemanticAcceptanceItemJSONPath("cand-demo"))
	outside := filepath.Join(t.TempDir(), "outside-item.json")
	data, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("read item: %v", err)
	}
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatalf("write outside item: %v", err)
	}
	if err := os.Remove(itemPath); err != nil {
		t.Fatalf("remove item: %v", err)
	}
	if err := os.Symlink(outside, itemPath); err != nil {
		t.Fatalf("symlink item: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected symlinked input item to be rejected")
	}
}

func TestSemanticCalibrationRejectsSymlinkedOutParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	linkParent := filepath.Join(base, "link-parent")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, linkParent); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, filepath.Join(linkParent, "generated"), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected symlinked calibration --out parent rejection")
	}
}

func TestSemanticCalibrationRejectsSymlinkedAcceptanceSummary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	summaryPath := filepath.Join(acceptanceDir, "acceptance-summary.json")
	outside := filepath.Join(t.TempDir(), "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatalf("write outside summary: %v", err)
	}
	if err := os.Remove(summaryPath); err != nil {
		t.Fatalf("remove summary: %v", err)
	}
	if err := os.Symlink(outside, summaryPath); err != nil {
		t.Fatalf("symlink summary: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected symlinked acceptance summary to be rejected")
	}
}

func TestSemanticCalibrationRejectsEscapingExpectedOutcomePath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	summaryPath := filepath.Join(acceptanceDir, "acceptance-summary.json")
	var summary SemanticAcceptanceSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read acceptance summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode acceptance summary: %v", err)
	}
	summary.ExpectedOutcomes[0].ExpectedPath = "../expected.json"
	writeDocumentsTestJSON(t, summaryPath, summary)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected escaping expected outcome path to be rejected")
	}
}

func TestSemanticCalibrationRejectsEscapingExpectedOutcomeInternalPath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	expectedPath := filepath.Join(acceptanceDir, SemanticAcceptanceExpectedOutcomeJSONPath("exp-action"))
	var outcome SemanticExpectedOutcomeResult
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected outcome: %v", err)
	}
	if err := json.Unmarshal(data, &outcome); err != nil {
		t.Fatalf("decode expected outcome: %v", err)
	}
	outcome.ExpectedPath = "../escaped.json"
	writeDocumentsTestJSON(t, expectedPath, outcome)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected escaping internal expected outcome path to be rejected")
	}
}

func TestSemanticCalibrationRejectsExpectedOutcomeSummaryMismatch(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, nil, true)
	expectedPath := filepath.Join(acceptanceDir, SemanticAcceptanceExpectedOutcomeJSONPath("exp-action"))
	var outcome SemanticExpectedOutcomeResult
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected outcome: %v", err)
	}
	if err := json.Unmarshal(data, &outcome); err != nil {
		t.Fatalf("decode expected outcome: %v", err)
	}
	outcome.Reason = SemanticAcceptanceReasonCorrect
	writeDocumentsTestJSON(t, expectedPath, outcome)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected expected-outcome summary mismatch to be rejected")
	}
}

func TestSemanticCalibrationRejectsMissingReferencedExpectedOutcome(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	summaryPath := filepath.Join(acceptanceDir, "acceptance-summary.json")
	var summary SemanticAcceptanceSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read acceptance summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode acceptance summary: %v", err)
	}
	summary.ExpectedOutcomes = nil
	writeDocumentsTestJSON(t, summaryPath, summary)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected missing referenced expected outcome to be rejected")
	}
}

func TestSemanticCalibrationRejectsNewExpectedOutcomeWithoutRichContext(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	expectedPath := filepath.Join(acceptanceDir, SemanticAcceptanceExpectedOutcomeJSONPath("exp-action"))
	var outcome SemanticExpectedOutcomeResult
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected outcome: %v", err)
	}
	if err := json.Unmarshal(data, &outcome); err != nil {
		t.Fatalf("decode expected outcome: %v", err)
	}
	outcome.TitleSignals = nil
	writeDocumentsTestJSON(t, expectedPath, outcome)
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected sparse v0.2 expected outcome to be rejected")
	}
}

func TestSemanticCalibrationAllowsSparseAcceptedAbsentOutcome(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, nil)
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-absent",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-no-risk",
			ExpectedState:          ExpectedOutcomeAbsent,
			ExpectedKind:           SemanticCandidateKindRisk,
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	acceptanceOut := t.TempDir()
	if _, err := AcceptSemantic(semanticRun, answerKey, acceptanceOut); err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	summary, err := CalibrateSemanticAcceptance(filepath.Join(acceptanceOut, "semantic-acceptance"), t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("accepted expected-absent outcome with no review item should not block calibration: %v", err)
	}
	if summary.ReviewItemCount != 0 {
		t.Fatalf("accepted absent outcome should not create review items: %+v", summary)
	}
}

func TestSemanticCalibrationRejectsSparseRejectedAbsentOutcome(t *testing.T) {
	semanticRun := writeSemanticAcceptanceRun(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	})
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-absent-rejected",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{{
			ExpectedOutcomeID:      "exp-no-action",
			ExpectedState:          ExpectedOutcomeAbsent,
			ExpectedKind:           SemanticCandidateKindAction,
			RequiredEvidence:       []string{"node-demo"},
			MinimumConfidenceFloor: ConfidenceLow,
		}},
	})
	acceptanceOut := t.TempDir()
	if _, err := AcceptSemantic(semanticRun, answerKey, acceptanceOut); err != nil {
		t.Fatalf("accept semantic candidates: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(filepath.Join(acceptanceOut, "semantic-acceptance"), t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("sparse rejected expected-absent outcome can feed review context and must be rejected")
	}
}

func TestSemanticCalibrationRejectsSymlinkedExpectedOutcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	expectedPath := filepath.Join(acceptanceDir, SemanticAcceptanceExpectedOutcomeJSONPath("exp-action"))
	outside := filepath.Join(t.TempDir(), "expected.json")
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected outcome: %v", err)
	}
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatalf("write outside expected outcome: %v", err)
	}
	if err := os.Remove(expectedPath); err != nil {
		t.Fatalf("remove expected outcome: %v", err)
	}
	if err := os.Symlink(outside, expectedPath); err != nil {
		t.Fatalf("symlink expected outcome: %v", err)
	}
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, t.TempDir(), SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err == nil {
		t.Fatalf("expected symlinked expected outcome to be rejected")
	}
}

func TestSemanticCalibrationNextRejectsEscapingItemPath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	summaryPath := filepath.Join(out, "semantic-calibration", "calibration-summary.json")
	var summary SemanticCalibrationSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read calibration summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode calibration summary: %v", err)
	}
	summary.ReviewItems[0].ItemPath = "../escaped.json"
	writeDocumentsTestJSON(t, summaryPath, summary)
	if _, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration")); err == nil {
		t.Fatalf("expected escaping calibration item path to be rejected")
	}
}

func TestSemanticCalibrationNextRejectsSymlinkedCursor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	cursorPath := filepath.Join(out, "semantic-calibration", "cursor.json")
	outside := filepath.Join(t.TempDir(), "cursor.json")
	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatalf("write outside cursor: %v", err)
	}
	if err := os.Remove(cursorPath); err != nil {
		t.Fatalf("remove cursor: %v", err)
	}
	if err := os.Symlink(outside, cursorPath); err != nil {
		t.Fatalf("symlink cursor: %v", err)
	}
	if _, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration")); err == nil {
		t.Fatalf("expected symlinked cursor to be rejected")
	}
}

func TestSemanticCalibrationNextRejectsSymlinkedReviewItem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	itemPath := filepath.Join(out, "semantic-calibration", SemanticCalibrationReviewItemJSONPath("review-cand-demo"))
	outside := filepath.Join(t.TempDir(), "item.json")
	data, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("read review item: %v", err)
	}
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatalf("write outside review item: %v", err)
	}
	if err := os.Remove(itemPath); err != nil {
		t.Fatalf("remove review item: %v", err)
	}
	if err := os.Symlink(outside, itemPath); err != nil {
		t.Fatalf("symlink review item: %v", err)
	}
	if _, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration")); err == nil {
		t.Fatalf("expected symlinked review item to be rejected")
	}
}

func TestSemanticCalibrationNextRejectsEscapingNonCurrentItemPath(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
		unexpectedDecisionCandidate(),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	summaryPath := filepath.Join(out, "semantic-calibration", "calibration-summary.json")
	var summary SemanticCalibrationSummary
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read calibration summary: %v", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode calibration summary: %v", err)
	}
	summary.ReviewItems[1].ItemPath = "../escaped.json"
	writeDocumentsTestJSON(t, summaryPath, summary)
	if _, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration")); err == nil {
		t.Fatalf("expected escaping non-current calibration item path to be rejected")
	}
}

func TestSemanticCalibrationNextRejectsPrivateMarkerInPersistedItem(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	itemPath := filepath.Join(out, "semantic-calibration", SemanticCalibrationReviewItemJSONPath("review-cand-demo"))
	var item SemanticCalibrationReviewItem
	data, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatalf("read calibration item: %v", err)
	}
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatalf("decode calibration item: %v", err)
	}
	item.Title = "contains " + unsafeTokenMarker()
	writeDocumentsTestJSON(t, itemPath, item)
	if _, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration")); err == nil {
		t.Fatalf("expected private marker in persisted calibration item to be rejected")
	}
}

func TestSemanticCalibrationNextRejectsInvalidCursor(t *testing.T) {
	acceptanceDir := writeSemanticAcceptanceOutput(t, []SemanticCandidate{
		validSemanticCandidate(validSemanticObservation(validStructureNode()), validStructureNode()),
	}, true)
	out := t.TempDir()
	if _, err := CalibrateSemanticAcceptance(acceptanceDir, out, SemanticCalibrationOptions{Threshold: 0.98, HeldOut: true}); err != nil {
		t.Fatalf("calibrate semantic acceptance: %v", err)
	}
	cursorPath := filepath.Join(out, "semantic-calibration", "cursor.json")
	var cursor SemanticCalibrationCursor
	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("read calibration cursor: %v", err)
	}
	if err := json.Unmarshal(data, &cursor); err != nil {
		t.Fatalf("decode calibration cursor: %v", err)
	}
	cursor.NextIndex = -1
	writeDocumentsTestJSON(t, cursorPath, cursor)
	if _, err := NextSemanticCalibrationReviewPage(filepath.Join(out, "semantic-calibration")); err == nil {
		t.Fatalf("expected invalid cursor to be rejected")
	}
}

func writeSemanticAcceptanceRun(t *testing.T, candidates []SemanticCandidate) string {
	t.Helper()
	root := t.TempDir()
	semanticRoot := filepath.Join(root, "semantic-candidates")
	if err := os.MkdirAll(filepath.Join(semanticRoot, "candidates"), 0o755); err != nil {
		t.Fatalf("mkdir semantic candidates: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(semanticRoot, "relations"), 0o755); err != nil {
		t.Fatalf("mkdir semantic relations: %v", err)
	}
	if candidates == nil {
		candidates = []SemanticCandidate{}
	}
	summaryCandidates := make([]SemanticSummaryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		path := SemanticCandidateJSONPath(candidate.CandidateID)
		writeDocumentsTestJSON(t, filepath.Join(semanticRoot, path), candidate)
		for _, relationID := range candidate.RelationIDs {
			relation := SemanticRelation{
				SchemaVersion:    SemanticRelationSchemaVersion,
				RelationID:       relationID,
				RunID:            candidate.RunID,
				RelationshipType: SemanticRelationshipDerivedFrom,
				FromID:           candidate.CandidateID,
				FromType:         SemanticRelationEndpointCandidate,
				ToID:             firstString(candidate.ObservationIDs),
				ToType:           SemanticRelationEndpointObservation,
				EvidenceNodes:    cloneStringList(candidate.EvidenceNodes),
				Confidence:       candidate.Confidence,
				ReviewStatus:     candidate.ReviewStatus,
				Blockers:         cloneBlockerList(candidate.Blockers),
			}
			writeDocumentsTestJSON(t, filepath.Join(semanticRoot, SemanticRelationJSONPath(relationID)), relation)
		}
		summaryCandidates = append(summaryCandidates, SemanticSummaryCandidate{
			CandidateID:   candidate.CandidateID,
			CandidateKind: candidate.CandidateKind,
			ReviewStatus:  candidate.ReviewStatus,
			Confidence:    candidate.Confidence,
			CandidatePath: path,
			PreviewPath:   SemanticPreviewPath(candidate.CandidateID),
		})
	}
	writeDocumentsTestJSON(t, filepath.Join(semanticRoot, "semantic-summary.json"), SemanticSummary{
		SchemaVersion:  SemanticSummarySchemaVersion,
		RunID:          "run-sem-demo",
		SourceCount:    1,
		CandidateCount: len(candidates),
		Candidates:     summaryCandidates,
	})
	return root
}

func writeSemanticAcceptanceOutput(t *testing.T, candidates []SemanticCandidate, expectPresent bool) string {
	t.Helper()
	semanticRun := writeSemanticAcceptanceRun(t, candidates)
	outcome := SemanticExpectedOutcome{
		ExpectedOutcomeID:      "exp-action",
		ExpectedState:          ExpectedOutcomePresent,
		ExpectedKind:           SemanticCandidateKindAction,
		RequiredEvidence:       []string{"node-demo"},
		AcceptableAlternates:   []string{"node-alt"},
		TitleSignals:           []string{"checklist"},
		SummarySignals:         []string{"prepare"},
		RelationRequirements:   []SemanticRelationshipType{SemanticRelationshipDerivedFrom},
		MinimumConfidenceFloor: ConfidenceLow,
		Notes:                  "Expected checklist action.",
	}
	if !expectPresent {
		outcome.ExpectedState = ExpectedOutcomeAbsent
	}
	answerKey := writeAcceptanceAnswerKey(t, SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      "ak-calibration",
		SourceDocumentID: "doc-demo",
		ExpectedOutcomes: []SemanticExpectedOutcome{outcome},
	})
	out := t.TempDir()
	if _, err := AcceptSemantic(semanticRun, answerKey, out); err != nil {
		t.Fatalf("write semantic acceptance output: %v", err)
	}
	return filepath.Join(out, "semantic-acceptance")
}

func validSemanticAcceptanceItemForCalibration() SemanticAcceptanceItem {
	node := validStructureNode()
	candidate := validSemanticCandidate(validSemanticObservation(node), node)
	return SemanticAcceptanceItem{
		SchemaVersion:     SemanticAcceptanceItemSchemaVersion,
		CandidateID:       candidate.CandidateID,
		RunID:             candidate.RunID,
		SourceDocumentID:  candidate.SourceDocumentID,
		CandidateKind:     candidate.CandidateKind,
		ReviewStatus:      candidate.ReviewStatus,
		Confidence:        candidate.Confidence,
		Title:             candidate.Title,
		Summary:           candidate.Summary,
		EvidenceNodes:     cloneStringList(candidate.EvidenceNodes),
		EvidenceRanges:    cloneSemanticEvidenceRanges(candidate.EvidenceRanges),
		RelationIDs:       cloneStringList(candidate.RelationIDs),
		AcceptanceState:   SemanticAcceptanceAccepted,
		Reason:            SemanticAcceptanceReasonCorrect,
		ExpectedOutcomeID: "exp-action",
		Blockers:          []Blocker{},
	}
}

func semanticAcceptanceItemWith(candidateID string, state SemanticAcceptanceState, reason SemanticAcceptanceReason) SemanticAcceptanceItem {
	item := validSemanticAcceptanceItemForCalibration()
	item.CandidateID = candidateID
	item.AcceptanceState = state
	item.Reason = reason
	item.ExpectedOutcomeID = ""
	if state == SemanticAcceptanceBlocked {
		item.Blockers = []Blocker{{Code: "unsafe_private_marker", Message: "blocked"}}
	}
	return item
}

func unexpectedDecisionCandidate() SemanticCandidate {
	return SemanticCandidate{
		SchemaVersion:     SemanticCandidateSchemaVersion,
		CandidateID:       "cand-unexpected",
		RunID:             "run-sem-demo",
		SourceDocumentID:  "doc-demo",
		CandidateKind:     SemanticCandidateKindDecision,
		ReviewStatus:      ReviewStatusReady,
		Confidence:        ConfidenceMedium,
		Title:             "Decide launch scope",
		Summary:           "The launch scope was decided.",
		EvidenceNodes:     []string{"node-decision"},
		EvidenceRanges:    []SemanticEvidenceRange{{StructureNodeID: "node-decision", LineStart: 3, LineEnd: 4}},
		ObservationIDs:    []string{"obs-decision"},
		RelationIDs:       []string{"rel-decision"},
		DestinationStatus: SemanticDestinationUnresolved,
		Blockers:          []Blocker{},
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return "obs-demo"
	}
	return values[0]
}

func fixedTestTime() time.Time {
	return time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
}

func writeAcceptanceAnswerKey(t *testing.T, answerKey SemanticAcceptanceAnswerKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "answer-key.json")
	writeDocumentsTestJSON(t, path, answerKey)
	return path
}

func writeDocumentsTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal test json: %v", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir test json dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test json: %v", err)
	}
}

func countStatus(t *testing.T, out string, summary Summary, status ReviewStatus) int {
	t.Helper()
	count := 0
	for _, item := range summary.Segments {
		data, err := os.ReadFile(filepath.Join(out, "document-segments", item.SegmentPath))
		if err != nil {
			t.Fatalf("read segment %s: %v", item.SegmentPath, err)
		}
		var segment Segment
		if err := json.Unmarshal(data, &segment); err != nil {
			t.Fatalf("decode segment %s: %v", item.SegmentPath, err)
		}
		if segment.ReviewStatus == status {
			count++
		}
	}
	return count
}

func assertTreeMatches(t *testing.T, actualRoot, expectedRoot string) {
	t.Helper()
	actual := readTree(t, actualRoot)
	expected := readTree(t, expectedRoot)
	if len(actual) != len(expected) {
		t.Fatalf("file count mismatch actual=%d expected=%d\nactual=%v\nexpected=%v", len(actual), len(expected), keys(actual), keys(expected))
	}
	for path, expectedBody := range expected {
		actualBody, ok := actual[path]
		if !ok {
			t.Fatalf("missing generated file %s", path)
		}
		if actualBody != expectedBody {
			t.Fatalf("golden mismatch for %s\nactual:\n%s\nexpected:\n%s", path, actualBody, expectedBody)
		}
	}
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("read tree %s: %v", root, err)
	}
	return files
}

func assertGeneratedTreeExcludes(t *testing.T, root string, forbidden ...string) {
	t.Helper()
	files := readTree(t, root)
	for path, body := range files {
		lower := strings.ToLower(body)
		for _, value := range forbidden {
			if strings.Contains(lower, strings.ToLower(value)) {
				t.Fatalf("generated artifact %s leaked %q:\n%s", path, value, body)
			}
		}
	}
}

func assertStructureNodePath(t *testing.T, summary StructureSummary, nodeType StructureNodeType, nodePath string) {
	t.Helper()
	for _, node := range summary.Nodes {
		if node.NodeType == nodeType && node.NodePath == nodePath {
			return
		}
	}
	t.Fatalf("expected %s node path %q in %+v", nodeType, nodePath, summary.Nodes)
}

func assertMissingStructureNodePath(t *testing.T, summary StructureSummary, nodePathFragment string) {
	t.Helper()
	for _, node := range summary.Nodes {
		if strings.Contains(node.NodePath, nodePathFragment) {
			t.Fatalf("unexpected node path containing %q: %+v", nodePathFragment, node)
		}
	}
}

func assertStructureNodeTitle(t *testing.T, out string, summary StructureSummary, nodeType StructureNodeType, nodePath, wantTitle string) {
	t.Helper()
	for _, item := range summary.Nodes {
		if item.NodeType != nodeType || item.NodePath != nodePath {
			continue
		}
		data, err := os.ReadFile(filepath.Join(out, "document-structure", StructureNodeJSONPath(item.NodeID)))
		if err != nil {
			t.Fatalf("read structure node: %v", err)
		}
		var node StructureNode
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatalf("decode structure node: %v", err)
		}
		if node.Title != wantTitle {
			t.Fatalf("unexpected node title got=%q want=%q node=%+v", node.Title, wantTitle, node)
		}
		if strings.Contains(node.Title, "*") || strings.Contains(strings.Join(node.Evidence.HeadingPath, "/"), "*") {
			t.Fatalf("expected emphasis-free title and evidence path, got %+v", node)
		}
		return
	}
	t.Fatalf("missing %s node at %q", nodeType, nodePath)
}

func assertTranscriptTurnEvidence(t *testing.T, out string, summary StructureSummary) {
	t.Helper()
	foundReady := false
	foundNeedsReview := false
	for _, item := range summary.Nodes {
		if item.NodeType != StructureNodeTypeTranscriptTurn {
			continue
		}
		data, err := os.ReadFile(filepath.Join(out, "document-structure", StructureNodeJSONPath(item.NodeID)))
		if err != nil {
			t.Fatalf("read transcript turn node: %v", err)
		}
		var node StructureNode
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatalf("decode transcript turn node: %v", err)
		}
		if !strings.Contains(node.Title, " - ") {
			t.Fatalf("expected timestamp and speaker in title, got %+v", node)
		}
		if node.Evidence.LineStart <= 0 || node.Evidence.LineEnd < node.Evidence.LineStart {
			t.Fatalf("expected transcript turn line range, got %+v", node)
		}
		if len(node.RelatedSegmentIDs) == 0 && node.ReviewStatus == ReviewStatusReady {
			t.Fatalf("ready transcript turn should preserve related segment ids: %+v", node)
		}
		if node.ReviewStatus == ReviewStatusReady {
			foundReady = true
		}
		if node.ReviewStatus == ReviewStatusNeedsReview {
			foundNeedsReview = true
		}
	}
	if !foundReady || !foundNeedsReview {
		t.Fatalf("expected ready and needs_review transcript turns, ready=%v needsReview=%v", foundReady, foundNeedsReview)
	}
}

func assertHeadingPath(t *testing.T, got []string, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected heading path got=%+v want=%+v", got, want)
	}
}

func assertHasSemanticType(t *testing.T, segments []Segment, semanticType SemanticType) {
	t.Helper()
	for _, segment := range segments {
		if segment.SemanticType == semanticType {
			return
		}
	}
	t.Fatalf("expected semantic type %s in %+v", semanticType, segments)
}

func keys(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

func containsSemanticEvidenceReadinessReason(reasons []SemanticEvidenceReadinessReason, want SemanticEvidenceReadinessReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func fixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	all := append([]string{repoRoot(t), "testdata", "documents"}, parts...)
	return filepath.Join(all...)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot resolve caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
