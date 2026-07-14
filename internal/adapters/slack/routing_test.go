package slack

import (
	"testing"

	"github.com/synergyai-os/Mindline/internal/routing"
)

func TestCompileRoutingPreservesDuplicateOccurrencesAndAdmitsOneHop(t *testing.T) {
	primary := "https://example.com/tool?utm_source=slack"
	canonical, _ := routing.CanonicalizeURL(primary)
	child := "https://example.org/evidence"
	childCanonical, _ := routing.CanonicalizeURL(child)
	payload := Payload{Source: Source{AdapterID: "slack", ChannelID: "private-channel"}, Messages: []Message{{TS: "1700000000.000001", User: "u", Text: primary}, {TS: "1700000001.000001", User: "u", Text: primary}}}
	artifacts := routing.LinkArtifacts{SchemaVersion: routing.LinkArtifactsSchema, Items: []routing.LinkArtifact{
		{CanonicalURL: canonical, State: "complete", PublicExcerpts: []routing.PublicExcerpt{{ExcerptID: "p1", Text: "A public tool with linked evidence.", Locator: "page"}}, RelatedURLs: []routing.RelatedURL{{URL: child, Relation: "source_links_to", DiscoveryEvidenceRef: "p1", SemanticallyRelevant: true}}},
		{CanonicalURL: childCanonical, State: "complete", PublicExcerpts: []routing.PublicExcerpt{{ExcerptID: "c1", Text: "Public supporting evidence.", Locator: "page"}}},
	}}
	profile := routing.LensProfile{SchemaVersion: routing.LensProfileSchemaVersion, ProfileID: "test", ProfileVersion: "1", Lenses: []routing.Lens{{LensID: "building-product", Name: "Building product", Question: "Relevant to building?"}, {LensID: "team-design", Name: "Team design", Question: "Relevant to teams?"}}}
	judgments := routing.Judgments{SchemaVersion: routing.JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: "2026-07-14T10:00:00Z", ProfileID: profile.ProfileID, ProfileVersion: profile.ProfileVersion, Sources: []routing.SourceJudgment{
		slackRoutingJudgment(routing.CanonicalURLID(canonical), "external_entity", "A public tool.", "p1", "hold"), slackRoutingJudgment(routing.CanonicalURLID(childCanonical), "evidence_backed_finding", "Supporting evidence.", "c1", "monitor"),
	}}
	result, err := CompileRouting(payload, artifacts, profile, judgments)
	if err != nil {
		t.Fatalf("CompileRouting: %v", err)
	}
	if result.Summary.InputRecordCount != 2 || result.Summary.URLOccurrenceCount != 2 || result.Summary.PrimaryCanonicalURLCount != 1 || result.Summary.DepthOneURLCount != 1 || result.Summary.CanonicalSourceCount != 2 || result.Summary.DuplicateOccurrenceCount != 1 {
		t.Fatalf("unexpected accounting: %+v", result.Summary)
	}
	if result.Summary.LensResultCount != 4 || result.Summary.RequiredLensResultCount != 4 {
		t.Fatalf("unexpected lens accounting")
	}
	if len(result.Graph.Edges) != 3 {
		t.Fatalf("unexpected edge count: %d", len(result.Graph.Edges))
	}
	related := result.Graph.Edges[2]
	if related.Type != "source_links_to" || related.From != routing.CanonicalURLID(canonical) || related.To != routing.CanonicalURLID(childCanonical) || len(related.EvidenceRefs) != 1 || related.EvidenceRefs[0] != "p1" {
		t.Fatalf("unexpected related-source edge: %+v", related)
	}
}

func slackRoutingJudgment(id, role, summary, evidence, disposition string) routing.SourceJudgment {
	return routing.SourceJudgment{CanonicalURLID: id, LensResults: []routing.LensResult{{LensID: "building-product", Result: "matched", Confidence: .8, Rationale: "The public evidence is relevant.", EvidenceRefs: []string{evidence}}, {LensID: "team-design", Result: "not_matched", Confidence: .8, Rationale: "The public evidence is not about teams.", EvidenceRefs: []string{evidence}}}, SemanticAssessment: routing.SemanticAssessment{PrimaryRole: role, Summary: summary, Confidence: .8, EvidenceRefs: []string{evidence}}, Disposition: disposition, DispositionRationale: "The explicit operator judgment selects this disposition."}
}
