package resourcequeue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf8"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/resourcefetch"
)

// FromResourcefetchResult is the one-way, narrow adapter between the public
// fetch subsystem and the derived queue. It never persists a URL and maps the
// fetcher's response-too-large condition to the signed fixed budget reason.
// Resourcefetch publishes exact redirect-inclusive RequestCount, so the queue
// can reserve and settle the request budget without approximation.
func FromResourcefetchResult(result resourcefetch.Result) FetchResult {
	adapted := FetchResult{State: result.State, BlockedReason: result.Reason, Usage: Usage{Requests: result.RequestCount, DownloadedBytes: result.WireBytes, DecodedBytes: result.DecodedBytes, ExtractedBytes: int64(result.ExtractedBytes), RuntimeStorageBytes: int64(result.ExtractedBytes), WallSeconds: result.WallSeconds}, RetryAfterSeconds: int64(result.RetryAfterSeconds)}
	if result.State == "blocked" {
		if result.Reason == resourcefetch.ReasonBudgetExhausted {
			adapted.BlockedReason = "budget_exhausted"
		}
		if result.Retryable && result.Reason == resourcefetch.ReasonRateLimited {
			adapted.HTTPStatus = 429
		} else if result.Retryable {
			adapted.TransientNetwork = true
		}
		return adapted
	}
	adapted.Evidence.Metadata.Title = result.Title
	adapted.Evidence.Metadata.Author = result.Author
	adapted.Evidence.Metadata.PublishedAt = result.PublishedAt
	adapted.Evidence.Missingness = append([]string(nil), result.Missingness...)
	// Related URLs are public values actually extracted by resourcefetch. The
	// canonical schema requires an evidence reference, so retain each one with
	// a bounded, deterministic extracted-link excerpt rather than inventing a
	// relationship without provenance.
	for _, related := range result.RelatedURLs {
		if !utf8.ValidString(related) || len([]rune(related)) > 1000 {
			continue
		}
		sum := sha256.Sum256([]byte(related))
		id := fmt.Sprintf("%s%s", personalmemory.GenericExtractorEvidencePrefix, hex.EncodeToString(sum[:])[:16])
		adapted.Evidence.Excerpts = append(adapted.Evidence.Excerpts, acquisition.ImportedExcerpt{ExcerptID: id, Text: related, Locator: "discovered_outbound_link"})
		adapted.Evidence.RelatedURLs = append(adapted.Evidence.RelatedURLs, acquisition.ImportedRelated{URL: related, Relation: "source_links_to", DiscoveryEvidenceRef: id, SemanticallyRelevant: false})
	}
	if result.Text != "" {
		completeness := "full"
		if result.State == StatePartial {
			completeness = "partial"
		}
		adapted.Content = &personalmemory.ExtractedContent{MediaType: result.MediaType, Completeness: completeness, Text: result.Text, Missingness: append([]string(nil), result.Missingness...), AccessClass: "public"}
	}
	return adapted
}
