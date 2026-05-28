package documents

import (
	"fmt"
	"strings"
)

func appendSourceEnrichmentMarkdown(body string, source SourceEnrichmentSourceArtifact) string {
	if len(source.URLs) == 0 {
		return strings.TrimRight(body, "\n") + "\n"
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n\n## Enriched Sources\n\n")
	b.WriteString("Retrieval mode: local artifacts only. Mindline did not fetch the network.\n\n")
	for index, item := range source.URLs {
		b.WriteString(fmt.Sprintf("### URL %d\n\n", index+1))
		b.WriteString(fmt.Sprintf("- URL: %s\n", item.RawURL))
		b.WriteString(fmt.Sprintf("- Normalized URL: `%s`\n", item.NormalizedURL))
		b.WriteString(fmt.Sprintf("- Kind: `%s`\n", item.Kind))
		b.WriteString(fmt.Sprintf("- State: `%s`\n", item.State))
		b.WriteString(fmt.Sprintf("- Retrieval mode: `%s`\n", item.RetrievalMode))
		if len(item.ReasonCodes) > 0 {
			b.WriteString(fmt.Sprintf("- Reason codes: `%s`\n", strings.Join(item.ReasonCodes, "`, `")))
		}
		if item.Title != "" {
			b.WriteString(fmt.Sprintf("- Title: %s\n", item.Title))
		}
		if item.SourceName != "" {
			b.WriteString(fmt.Sprintf("- Source name: %s\n", item.SourceName))
		}
		if item.Description != "" {
			b.WriteString(fmt.Sprintf("- Description: %s\n", item.Description))
		}
		if item.Excerpt != "" {
			b.WriteString("\n")
			b.WriteString("> ")
			b.WriteString(strings.ReplaceAll(strings.TrimSpace(item.Excerpt), "\n", "\n> "))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func sourceEnrichmentReport(summary SourceEnrichmentSummary, sources []SourceEnrichmentSourceArtifact) string {
	var b strings.Builder
	b.WriteString("# Source enrichment report\n\n")
	b.WriteString("## Result\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: `%s`\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources: %d\n", summary.SourceCount))
	b.WriteString(fmt.Sprintf("- URLs: %d total, %d accounted\n", summary.URLCount, summary.AccountedURLCount))
	b.WriteString(fmt.Sprintf("- Enriched URLs: %d\n", summary.EnrichedURLCount))
	b.WriteString(fmt.Sprintf("- Needs manual processing: %d\n", summary.NeedsManualURLCount))
	b.WriteString(fmt.Sprintf("- Unsupported URLs: %d\n", summary.UnsupportedURLCount))
	b.WriteString(fmt.Sprintf("- Blocked URLs: %d\n", summary.BlockedURLCount))
	b.WriteString(fmt.Sprintf("- URL accounting coverage: %.2f\n", summary.URLAccountingCoverage))
	b.WriteString(fmt.Sprintf("- Enriched artifact coverage: %.2f\n", summary.EnrichedArtifactCoverage))
	b.WriteString("- Retrieval mode: local artifacts only\n")
	b.WriteString("- Network fetches: 0\n")
	b.WriteString("- Destination writes: 0\n")
	b.WriteString("- Product Brain writes: 0\n")
	b.WriteString("- Tolaria writes: 0\n\n")
	b.WriteString("## Sources\n\n")
	for _, source := range sources {
		b.WriteString(fmt.Sprintf("### %s\n\n", source.SourceID))
		b.WriteString(fmt.Sprintf("- State: `%s`\n", source.State))
		if len(source.ReasonCodes) > 0 {
			b.WriteString(fmt.Sprintf("- Reason codes: `%s`\n", strings.Join(source.ReasonCodes, "`, `")))
		}
		b.WriteString(fmt.Sprintf("- URLs: %d\n", len(source.URLs)))
		for _, item := range source.URLs {
			b.WriteString(fmt.Sprintf("  - `%s` %s %s", item.State, item.Kind, item.RawURL))
			if len(item.ReasonCodes) > 0 {
				b.WriteString(fmt.Sprintf(" reason=%s", strings.Join(item.ReasonCodes, ",")))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}
