package resourcequeue

import (
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/resourcefetch"
)

func TestResourcefetchDiscoveredLinksRemainUnclassifiedReferenceEvidence(t *testing.T) {
	result := FromResourcefetchResult(resourcefetch.Result{
		State:       StatePartial,
		RelatedURLs: []string{"https://example.com/generic"},
	})
	if len(result.Evidence.RelatedURLs) != 1 || len(result.Evidence.Excerpts) != 1 {
		t.Fatalf("generic reference evidence missing: %+v", result)
	}
	related := result.Evidence.RelatedURLs[0]
	if related.SemanticallyRelevant || !strings.HasPrefix(related.DiscoveryEvidenceRef, "related-") {
		t.Fatalf("generic extractor promoted reference semantics: %+v", related)
	}
	if related.DiscoveryEvidenceRef != result.Evidence.Excerpts[0].ExcerptID {
		t.Fatal("generic reference lost bounded provenance")
	}
}

func TestResourcefetchBudgetDimensionRemainsStructural(t *testing.T) {
	result := FromResourcefetchResult(resourcefetch.Result{
		State: "blocked", Reason: resourcefetch.ReasonBudgetExhausted,
		ExhaustedBudgetDimension: resourcefetch.BudgetDimensionDecoded,
		RequestCount:             1, WireBytes: 11, DecodedBytes: 12, ExtractedBytes: 13,
	})
	if result.ExhaustedBudgetDimension != BudgetDimensionDecoded ||
		result.Usage.Requests != 1 || result.Usage.DownloadedBytes != 11 ||
		result.Usage.DecodedBytes != 12 || result.Usage.ExtractedBytes != 13 {
		t.Fatalf("budget dimension was lost: %+v", result)
	}
}
