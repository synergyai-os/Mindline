package documents

import (
	"encoding/json"
	"fmt"
	"strings"
)

func jsonUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

func linkArtifactRequestReport(pack LinkArtifactRequestPack) string {
	var b strings.Builder
	b.WriteString("# Link artifact request pack\n\n")
	b.WriteString("## Result\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: `%s`\n", pack.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources: %d\n", pack.Summary.SourceCount))
	b.WriteString(fmt.Sprintf("- URL mentions: %d\n", pack.Summary.URLMentionCount))
	b.WriteString(fmt.Sprintf("- Unique URLs: %d\n", pack.Summary.UniqueURLCount))
	b.WriteString(fmt.Sprintf("- Accounted URLs: %d\n", pack.Summary.AccountedURLCount))
	b.WriteString(fmt.Sprintf("- Requestable: %d\n", pack.Summary.RequestableCount))
	b.WriteString(fmt.Sprintf("- Already artifacted: %d\n", pack.Summary.AlreadyArtifactedCount))
	b.WriteString(fmt.Sprintf("- Unsupported: %d\n", pack.Summary.UnsupportedCount))
	b.WriteString(fmt.Sprintf("- Blocked private/secret: %d\n", pack.Summary.BlockedPrivateCount))
	b.WriteString(fmt.Sprintf("- Blocked by policy: %d\n", pack.Summary.BlockedPolicyCount))
	b.WriteString(fmt.Sprintf("- Supplied artifacts: %d\n", pack.Summary.SuppliedArtifactCount))
	b.WriteString(fmt.Sprintf("- Matched artifacts: %d\n", pack.Summary.MatchedArtifactCount))
	b.WriteString(fmt.Sprintf("- Stale artifacts: %d\n", pack.Summary.StaleArtifactCount))
	b.WriteString("- Retrieval mode: local/manual artifacts only\n")
	b.WriteString(fmt.Sprintf("- Non-generalizable runtime proof: %t\n", pack.Summary.NonGeneralizableRuntime))
	b.WriteString("- Network fetches: 0\n")
	b.WriteString("- Destination writes: 0\n\n")
	b.WriteString("## Requests\n\n")
	for _, request := range pack.Requests {
		b.WriteString(fmt.Sprintf("### %s\n\n", request.RequestID))
		b.WriteString(fmt.Sprintf("- Source: `%s`\n", request.SourceID))
		b.WriteString(fmt.Sprintf("- State: `%s`\n", request.State))
		b.WriteString(fmt.Sprintf("- Kind: `%s`\n", request.Kind))
		if request.RawURL != "" {
			b.WriteString(fmt.Sprintf("- URL: %s\n", request.RawURL))
		}
		b.WriteString(fmt.Sprintf("- Normalized URL: `%s`\n", request.NormalizedURL))
		if len(request.ReasonCodes) > 0 {
			b.WriteString(fmt.Sprintf("- Reason codes: `%s`\n", strings.Join(request.ReasonCodes, "`, `")))
		}
		b.WriteString(fmt.Sprintf("- Requested fields: `%s`\n\n", strings.Join(request.RequestedArtifactFields, "`, `")))
	}
	if len(pack.Requests) == 0 {
		b.WriteString("- No link artifact requests.\n")
	}
	return b.String()
}

func linkEnrichmentComparisonReport(summary LinkEnrichmentComparisonSummary) string {
	var b strings.Builder
	b.WriteString("# Link enrichment comparison report\n\n")
	b.WriteString("## Result\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: `%s`\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Verdict: `%s`\n", summary.Verdict))
	b.WriteString(fmt.Sprintf("- Comparable: `%t`\n", summary.Comparable))
	if len(summary.ComparableBasis) > 0 {
		b.WriteString(fmt.Sprintf("- Comparable basis: `%s`\n", strings.Join(summary.ComparableBasis, "`, `")))
	}
	b.WriteString(fmt.Sprintf("- Baseline corpus fingerprint: `%s`\n", summary.BaselineCorpusFingerprint))
	b.WriteString(fmt.Sprintf("- Enriched corpus fingerprint: `%s`\n", summary.EnrichedCorpusFingerprint))
	b.WriteString(fmt.Sprintf("- Baseline source-set fingerprint: `%s`\n", summary.BaselineSourceSetFingerprint))
	b.WriteString(fmt.Sprintf("- Enriched source-set fingerprint: `%s`\n", summary.EnrichedSourceSetFingerprint))
	if len(summary.ReasonCodes) > 0 {
		b.WriteString(fmt.Sprintf("- Reason codes: `%s`\n", strings.Join(summary.ReasonCodes, "`, `")))
	}
	b.WriteString(fmt.Sprintf("- Missing link enrichment reduction: %.2f\n", summary.MissingLinkReductionRatio))
	b.WriteString(fmt.Sprintf("- Needs enrichment reduction: %.2f\n", summary.NeedsEnrichmentReductionRatio))
	b.WriteString(fmt.Sprintf("- Enriched artifact coverage: %.2f\n", summary.RequestSummary.ArtifactMatchCoverage))
	b.WriteString("- Retrieval mode: local/manual artifacts only\n")
	b.WriteString(fmt.Sprintf("- Non-generalizable runtime proof: %t\n", summary.RequestSummary.NonGeneralizableRuntime))
	b.WriteString("- Network fetches: 0\n")
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", summary.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n", summary.Guardrails.HostedTelemetryExports))
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", summary.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", summary.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n\n", summary.Guardrails.TolariaWrites))
	b.WriteString("## Missingness movement\n\n")
	writeMissingnessLine(&b, "missing_link_enrichment", summary.BaselineMissingnessCounts[SourceMeaningMissingnessMissingLinkEnrichment], summary.EnrichedMissingnessCounts[SourceMeaningMissingnessMissingLinkEnrichment])
	writeMissingnessLine(&b, "link_only_source", summary.BaselineMissingnessCounts[SourceMeaningMissingnessLinkOnlySource], summary.EnrichedMissingnessCounts[SourceMeaningMissingnessLinkOnlySource])
	writeMissingnessLine(&b, "reference_only", summary.BaselineMissingnessCounts[SourceMeaningMissingnessReferenceOnly], summary.EnrichedMissingnessCounts[SourceMeaningMissingnessReferenceOnly])
	b.WriteString("\n## Routing movement\n\n")
	writeMissingnessLine(&b, "needs_enrichment", summary.BaselineRoutingCounts[SourceMeaningRoutingNeedsEnrichment], summary.EnrichedRoutingCounts[SourceMeaningRoutingNeedsEnrichment])
	writeMissingnessLine(&b, "tolaria_candidate", summary.BaselineRoutingCounts[SourceMeaningRoutingTolariaCandidate], summary.EnrichedRoutingCounts[SourceMeaningRoutingTolariaCandidate])
	writeMissingnessLine(&b, "product_brain_candidate", summary.BaselineRoutingCounts[SourceMeaningRoutingProductBrainCandidate], summary.EnrichedRoutingCounts[SourceMeaningRoutingProductBrainCandidate])
	return b.String()
}

func writeMissingnessLine(b *strings.Builder, label string, before, after int) {
	b.WriteString(fmt.Sprintf("- `%s`: %d -> %d (delta %+d)\n", label, before, after, after-before))
}
