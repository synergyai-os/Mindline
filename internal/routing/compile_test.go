package routing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func TestCompileGraphKeepsMeaningIndependentFromAlternateUserLenses(t *testing.T) {
	canonical := "https://example.com/public-resource"
	id := CanonicalURLID(canonical)
	graph := SourceGraph{SchemaVersion: SourceGraphSchema, Adapter: AdapterRef{Kind: "bookmark", Version: "v1"}, SourceRecords: []SourceRecord{{SourceRecordID: "src-public", SourceKind: "bookmark", OccurredAt: "2026-07-14T10:00:00Z", RawProvenanceRef: "local-source-1", URLOccurrenceIDs: []string{"occ-public"}}}, URLOccurrences: []URLOccurrence{{URLOccurrenceID: "occ-public", SourceRecordID: "src-public", ObservedURL: canonical, CanonicalURLID: id}}, CanonicalURLs: []CanonicalURL{{CanonicalURLID: id, CanonicalURL: canonical, Kind: "generic_web", Depth: 0, Discovery: "source_occurrence", EnrichmentState: "complete"}}, Edges: []GraphEdge{{EdgeID: "edge-public", Type: "source_record_contains_url", From: "src-public", To: id, EvidenceRefs: []string{"occ-public"}}}}
	artifacts := LinkArtifacts{SchemaVersion: LinkArtifactsSchema, Items: []LinkArtifact{{CanonicalURL: canonical, State: "complete", PublicExcerpts: []PublicExcerpt{{ExcerptID: "e1", Text: "A reusable public resource.", Locator: "page"}}}}}
	profileA := testLensProfile()
	judgmentA := testJudgment(id, "external_entity", "A reusable public resource.", "e1", "promote")
	judgmentA.SemanticNodes = []SemanticNode{{SemanticNodeID: "public-resource", Role: "external_entity", Name: "Public resource", Description: "A reusable public resource.", Confidence: .8, LensRefs: []string{"building-product"}, EvidenceRefs: []string{"e1"}, Attributes: map[string]any{}}}
	manifestA := Judgments{SchemaVersion: JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: "2026-07-14T10:00:00Z", ProfileID: profileA.ProfileID, ProfileVersion: profileA.ProfileVersion, Sources: []SourceJudgment{judgmentA}}
	resultA, err := CompileGraph(graph, artifacts, profileA, manifestA)
	if err != nil {
		t.Fatal(err)
	}
	profileB := LensProfile{SchemaVersion: LensProfileSchemaVersion, ProfileID: "another-user", ProfileVersion: "1", Lenses: []Lens{{LensID: "gardening", Name: "Gardening", Question: "Does this help with a garden?"}}}
	judgmentB := judgmentA
	judgmentB.LensResults = []LensResult{{LensID: "gardening", Result: "not_matched", Confidence: .9, Rationale: "The evidence is unrelated to gardening.", EvidenceRefs: []string{"e1"}}}
	judgmentB.Disposition = "archive"
	judgmentB.SemanticNodes = append([]SemanticNode{}, judgmentA.SemanticNodes...)
	judgmentB.SemanticNodes[0].LensRefs = []string{"gardening"}
	manifestB := Judgments{SchemaVersion: JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: "2026-07-14T10:00:00Z", ProfileID: profileB.ProfileID, ProfileVersion: profileB.ProfileVersion, Sources: []SourceJudgment{judgmentB}}
	resultB, err := CompileGraph(graph, artifacts, profileB, manifestB)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(resultA.Decisions.Sources[0].SemanticAssessment)
	right, _ := json.Marshal(resultB.Decisions.Sources[0].SemanticAssessment)
	if string(left) != string(right) {
		t.Fatalf("meaning changed with lenses")
	}
	if resultA.Decisions.Sources[0].Disposition != "promote" || resultB.Decisions.Sources[0].Disposition != "archive" || resultA.Decisions.Sources[0].SemanticNodes[0].Role != resultB.Decisions.Sources[0].SemanticNodes[0].Role {
		t.Fatalf("lens variant did not preserve semantic role while changing disposition")
	}
	leftEnrichment, _ := json.Marshal(struct {
		Metadata PublicMetadata
		Excerpts []PublicExcerpt
	}{resultA.Decisions.Sources[0].PublicMetadata, resultA.Decisions.Sources[0].PublicExcerpts})
	rightEnrichment, _ := json.Marshal(struct {
		Metadata PublicMetadata
		Excerpts []PublicExcerpt
	}{resultB.Decisions.Sources[0].PublicMetadata, resultB.Decisions.Sources[0].PublicExcerpts})
	if string(leftEnrichment) != string(rightEnrichment) {
		t.Fatal("public enrichment changed with lenses")
	}
	routingBody, _ := json.Marshal(resultB)
	for _, destinationField := range []string{"collection_slug", "entry_id", "force_draft", "created_by"} {
		if strings.Contains(string(routingBody), destinationField) {
			t.Fatalf("routing artifact leaked destination field %q", destinationField)
		}
	}
	neutralBody := strings.ToLower(string(routingBody))
	for _, vocabulary := range []string{"slack", "product brain", "tolaria"} {
		if strings.Contains(neutralBody, vocabulary) {
			t.Fatalf("generic routing result leaked adapter vocabulary %q", vocabulary)
		}
	}
}

func TestCanonicalizeURLDropsTrackingAndNormalizesIdentity(t *testing.T) {
	got, err := CanonicalizeURL("HTTPS://Example.COM:443/path/?utm_source=x&rcm=recipient-marker&b=2&a=1#frag")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/path?a=1&b=2" {
		t.Fatalf("got %s", got)
	}
}

func TestCanonicalizeURLDoesNotDoubleEscapeEncodedPath(t *testing.T) {
	got, err := CanonicalizeURL("https://example.com/a%20b/%2Fdetail/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/a%20b/%2Fdetail" {
		t.Fatalf("encoded path was not preserved: %s", got)
	}
}

func TestValidateSourceGraphRejectsBrokenAccountingAndEdges(t *testing.T) {
	base := validSourceGraphFixture()
	tests := []struct {
		name   string
		mutate func(*SourceGraph)
	}{
		{"duplicate canonical identity", func(graph *SourceGraph) { graph.CanonicalURLs = append(graph.CanonicalURLs, graph.CanonicalURLs[0]) }},
		{"unlisted occurrence", func(graph *SourceGraph) {
			graph.URLOccurrences = append(graph.URLOccurrences, URLOccurrence{URLOccurrenceID: "occ-unlisted", SourceRecordID: "src-public", ObservedURL: graph.CanonicalURLs[0].CanonicalURL, CanonicalURLID: graph.CanonicalURLs[0].CanonicalURLID})
		}},
		{"occurrence assigned to wrong record", func(graph *SourceGraph) {
			graph.SourceRecords = append(graph.SourceRecords, SourceRecord{SourceRecordID: "src-other", SourceKind: "bookmark", OccurredAt: "2026-07-14T10:01:00Z", RawProvenanceRef: "local-source-2", URLOccurrenceIDs: []string{"occ-public"}})
		}},
		{"unknown edge type", func(graph *SourceGraph) { graph.Edges[0].Type = "adapter_private_edge" }},
		{"dangling edge endpoint", func(graph *SourceGraph) { graph.Edges[0].To = "url-missing" }},
		{"invalid depth-one parent", func(graph *SourceGraph) {
			childURL := "https://example.org/child"
			graph.CanonicalURLs = append(graph.CanonicalURLs, CanonicalURL{CanonicalURLID: CanonicalURLID(childURL), CanonicalURL: childURL, Kind: "generic_web", Depth: 1, ParentCanonicalURLID: "url-missing", Discovery: "enrichment_related_url", EnrichmentState: "complete"})
		}},
		{"orphan primary canonical source", func(graph *SourceGraph) {
			orphanURL := "https://example.org/orphan"
			graph.CanonicalURLs = append(graph.CanonicalURLs, CanonicalURL{CanonicalURLID: CanonicalURLID(orphanURL), CanonicalURL: orphanURL, Kind: "generic_web", Discovery: "source_occurrence", EnrichmentState: "complete"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := cloneSourceGraph(t, base)
			test.mutate(&graph)
			if err := validateSourceGraph(graph); err == nil {
				t.Fatal("malformed source graph was accepted")
			}
		})
	}
}

func TestLoadResultRejectsRefingerprintedMalformedGraph(t *testing.T) {
	result := compiledRoutingFixture(t)
	dir := filepath.Join(t.TempDir(), "routing")
	if err := Write(dir, result); err != nil {
		t.Fatal(err)
	}
	result.Graph.Edges[0].Type = "adapter_private_edge"
	result.Graph.Fingerprint = fingerprint(result.Graph)
	result.Decisions.SourceGraphFingerprint = result.Graph.Fingerprint
	result.Decisions.Fingerprint = fingerprint(result.Decisions)
	result.Summary.SourceGraphFingerprint = result.Graph.Fingerprint
	result.Summary.RouteDecisionsFingerprint = result.Decisions.Fingerprint
	result.Summary.Fingerprint = fingerprint(result.Summary)
	if err := privateio.WriteJSON(filepath.Join(dir, "source-graph.json"), result.Graph); err != nil {
		t.Fatal(err)
	}
	if err := privateio.WriteJSON(filepath.Join(dir, "route-decisions.json"), result.Decisions); err != nil {
		t.Fatal(err)
	}
	if err := privateio.WriteJSON(filepath.Join(dir, "route-summary.json"), result.Summary); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResult(dir); err == nil {
		t.Fatal("refingerprinted malformed routing authority was accepted")
	}
}

func TestCompileGraphRejectsUnresolvedRelatedSourceEvidence(t *testing.T) {
	graph := validSourceGraphFixture()
	parent := graph.CanonicalURLs[0]
	childURL := "https://example.org/child"
	child := CanonicalURL{CanonicalURLID: CanonicalURLID(childURL), CanonicalURL: childURL, Kind: "generic_web", Depth: 1, ParentCanonicalURLID: parent.CanonicalURLID, Discovery: "enrichment_related_url", EnrichmentState: "complete"}
	graph.CanonicalURLs = append(graph.CanonicalURLs, child)
	graph.Edges = append(graph.Edges, GraphEdge{EdgeID: "edge-related", Type: "source_links_to", From: parent.CanonicalURLID, To: child.CanonicalURLID, EvidenceRefs: []string{"missing-evidence"}})
	artifacts := LinkArtifacts{SchemaVersion: LinkArtifactsSchema, Items: []LinkArtifact{
		{CanonicalURL: parent.CanonicalURL, State: "complete", PublicExcerpts: []PublicExcerpt{{ExcerptID: "parent-evidence", Text: "A parent resource.", Locator: "page"}}},
		{CanonicalURL: child.CanonicalURL, State: "complete", PublicExcerpts: []PublicExcerpt{{ExcerptID: "child-evidence", Text: "A child resource.", Locator: "page"}}},
	}}
	profile := testLensProfile()
	manifest := Judgments{SchemaVersion: JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: "2026-07-14T10:00:00Z", ProfileID: profile.ProfileID, ProfileVersion: profile.ProfileVersion, Sources: []SourceJudgment{
		testJudgment(parent.CanonicalURLID, "reference_resource", "A parent resource.", "parent-evidence", "hold"),
		testJudgment(child.CanonicalURLID, "reference_resource", "A child resource.", "child-evidence", "hold"),
	}}
	if _, err := CompileGraph(graph, artifacts, profile, manifest); err == nil {
		t.Fatal("unresolved related-source evidence was accepted")
	}
}

func TestLoadResultRejectsUnknownAndTrailingJSONForEveryArtifact(t *testing.T) {
	result := compiledRoutingFixture(t)
	tests := []struct {
		name   string
		file   string
		mutate func(map[string]any)
	}{
		{"graph unknown top-level", "source-graph.json", func(value map[string]any) { value["future"] = true }},
		{"graph unknown nested", "source-graph.json", func(value map[string]any) { value["adapter"].(map[string]any)["secret"] = "ignored" }},
		{"decisions unknown top-level", "route-decisions.json", func(value map[string]any) { value["future"] = true }},
		{"decisions unknown nested", "route-decisions.json", func(value map[string]any) { value["sources"].([]any)[0].(map[string]any)["future"] = true }},
		{"summary unknown top-level", "route-summary.json", func(value map[string]any) { value["future"] = true }},
		{"summary unknown nested", "route-summary.json", func(value map[string]any) { value["eval_projection"].(map[string]any)["future"] = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "routing")
			if err := Write(dir, result); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, test.file)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
			test.mutate(value)
			body, _ = json.Marshal(value)
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadResult(dir); err == nil {
				t.Fatal("routing artifact with unknown field was accepted")
			}
		})
	}
	for _, file := range []string{"source-graph.json", "route-decisions.json", "route-summary.json"} {
		t.Run(file+" trailing", func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "routing")
			if err := Write(dir, result); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, file)
			body, _ := os.ReadFile(path)
			if err := os.WriteFile(path, append(body, []byte("\n{}")...), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadResult(dir); err == nil {
				t.Fatal("routing artifact with trailing JSON was accepted")
			}
		})
	}
}

func TestValidateResultRejectsRefingerprintedInvalidEnrichment(t *testing.T) {
	base := compiledRoutingFixture(t)
	t.Run("duplicate excerpt identity", func(t *testing.T) {
		result := cloneRoutingResult(t, base)
		result.Decisions.Sources[0].PublicExcerpts = append(result.Decisions.Sources[0].PublicExcerpts, result.Decisions.Sources[0].PublicExcerpts[0])
		refingerprintRoutingResult(&result)
		if err := ValidateResult(result); err == nil {
			t.Fatal("duplicate routed excerpt was accepted")
		}
	})
	t.Run("inaccessible source with invented metadata", func(t *testing.T) {
		result := cloneRoutingResult(t, base)
		result.Graph.CanonicalURLs[0].EnrichmentState = "inaccessible"
		result.Graph.CanonicalURLs[0].Missingness = []string{"content unavailable"}
		source := &result.Decisions.Sources[0]
		source.EnrichmentState = "inaccessible"
		source.Missingness = []string{"content unavailable"}
		source.PublicMetadata = PublicMetadata{Title: "Invented title"}
		source.PublicExcerpts = nil
		for index := range source.LensResults {
			source.LensResults[index].Result = "unknown"
			source.LensResults[index].EvidenceRefs = nil
			source.LensResults[index].Missingness = []string{"content unavailable"}
		}
		source.SemanticAssessment = SemanticAssessment{PrimaryRole: "unknown", Confidence: 0, Missingness: []string{"content unavailable"}}
		refingerprintRoutingResult(&result)
		if err := ValidateResult(result); err == nil {
			t.Fatal("inaccessible routed source with invented metadata was accepted")
		}
	})
}

func compiledRoutingFixture(t *testing.T) Result {
	t.Helper()
	graph := validSourceGraphFixture()
	canonical := graph.CanonicalURLs[0].CanonicalURL
	id := graph.CanonicalURLs[0].CanonicalURLID
	artifacts := LinkArtifacts{SchemaVersion: LinkArtifactsSchema, Items: []LinkArtifact{{CanonicalURL: canonical, State: "complete", PublicExcerpts: []PublicExcerpt{{ExcerptID: "e1", Text: "A public resource.", Locator: "page"}}}}}
	profile := testLensProfile()
	judgment := testJudgment(id, "reference_resource", "A public resource.", "e1", "hold")
	manifest := Judgments{SchemaVersion: JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: "2026-07-14T10:00:00Z", ProfileID: profile.ProfileID, ProfileVersion: profile.ProfileVersion, Sources: []SourceJudgment{judgment}}
	result, err := CompileGraph(graph, artifacts, profile, manifest)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func refingerprintRoutingResult(result *Result) {
	result.Graph.Fingerprint = fingerprint(result.Graph)
	result.Decisions.SourceGraphFingerprint = result.Graph.Fingerprint
	result.Decisions.Fingerprint = fingerprint(result.Decisions)
	result.Summary = summarizeBound(result.Graph, result.Decisions, result.Summary.LensCount, result.Decisions.LensProfileFingerprint)
}

func cloneRoutingResult(t *testing.T, result Result) Result {
	t.Helper()
	body, _ := json.Marshal(result)
	var clone Result
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func validSourceGraphFixture() SourceGraph {
	canonical := "https://example.com/public-resource"
	id := CanonicalURLID(canonical)
	return SourceGraph{
		SchemaVersion:  SourceGraphSchema,
		Adapter:        AdapterRef{Kind: "bookmark", Version: "v1"},
		SourceRecords:  []SourceRecord{{SourceRecordID: "src-public", SourceKind: "bookmark", OccurredAt: "2026-07-14T10:00:00Z", RawProvenanceRef: "local-source-1", URLOccurrenceIDs: []string{"occ-public"}}},
		URLOccurrences: []URLOccurrence{{URLOccurrenceID: "occ-public", SourceRecordID: "src-public", ObservedURL: canonical, CanonicalURLID: id}},
		CanonicalURLs:  []CanonicalURL{{CanonicalURLID: id, CanonicalURL: canonical, Kind: "generic_web", Depth: 0, Discovery: "source_occurrence", EnrichmentState: "complete"}},
		Edges:          []GraphEdge{{EdgeID: "edge-public", Type: "source_record_contains_url", From: "src-public", To: id, EvidenceRefs: []string{"occ-public"}}},
	}
}

func cloneSourceGraph(t *testing.T, graph SourceGraph) SourceGraph {
	t.Helper()
	data, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	var clone SourceGraph
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func testLensProfile() LensProfile {
	return LensProfile{SchemaVersion: LensProfileSchemaVersion, ProfileID: "test", ProfileVersion: "1", Lenses: []Lens{{LensID: "building-product", Name: "Building product", Question: "Relevant to building?"}, {LensID: "team-design", Name: "Team design", Question: "Relevant to teams?"}}}
}
func testJudgment(id, role, summary, evidence, disposition string) SourceJudgment {
	return SourceJudgment{CanonicalURLID: id, LensResults: []LensResult{{LensID: "building-product", Result: "matched", Confidence: .8, Rationale: "The public evidence is relevant.", EvidenceRefs: []string{evidence}}, {LensID: "team-design", Result: "not_matched", Confidence: .8, Rationale: "The public evidence is not about teams.", EvidenceRefs: []string{evidence}}}, SemanticAssessment: SemanticAssessment{PrimaryRole: role, Summary: summary, Confidence: .8, EvidenceRefs: []string{evidence}}, Disposition: disposition, DispositionRationale: "The explicit operator judgment selects this disposition."}
}
