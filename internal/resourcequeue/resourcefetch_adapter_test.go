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
