package personalmemory

import "testing"

type semanticOnlyBackend struct{}

func (semanticOnlyBackend) MethodID() string { return "semantic-test/v0.1" }

func (semanticOnlyBackend) Rank(_ SearchRequest, documents []IndexDocument) ([]RankedHit, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	return []RankedHit{{
		DocumentID: documents[0].DocumentID, Score: 1,
		Components: map[string]float64{"semantic_cosine": 1},
	}}, nil
}

func TestSemanticOnlyHitStillCarriesEvidenceReference(t *testing.T) {
	repository := populatedRepository(t)
	packet, err := NewRetriever(repository, semanticOnlyBackend{}).Search(SearchRequest{
		Query: "conceptual paraphrase", Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Citations) != 1 || len(packet.Citations[0].MatchedTerms) != 0 ||
		len(packet.Citations[0].EvidenceRefs) == 0 {
		t.Fatalf("semantic citation lost its evidence: %+v", packet.Citations)
	}
	for _, reference := range packet.Citations[0].EvidenceRefs {
		if reference.Locator != "semantic_record_match" || reference.MatchedSnippet != "" {
			t.Fatalf("semantic record match overclaimed excerpt evidence: %+v", reference)
		}
	}
}
