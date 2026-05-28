package documents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSourceEnrichmentWritesPressureCompatibleEnrichedSources(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Captured link\n\nSave this: https://example.com/research\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:         "https://example.com/research",
		Kind:        "web_url",
		Title:       "Gateway evidence review",
		Description: "Requirement: Review captured gateway evidence before destination routing.",
		Excerpt:     "Requirement: Gateway evidence must stay linked to source context before routing.",
		SourceName:  "Example Research",
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.SourceCount != 1 || summary.URLCount != 1 || summary.AccountedURLCount != 1 || summary.EnrichedURLCount != 1 {
		t.Fatalf("bad summary counts: %+v", summary)
	}
	if summary.URLAccountingCoverage != 1 || summary.EnrichedArtifactCoverage != 1 {
		t.Fatalf("bad coverage: %+v", summary)
	}
	if summary.Guardrails.DestinationWrites != 0 || summary.Guardrails.ProductBrainWrites != 0 || summary.Guardrails.TolariaWrites != 0 || summary.Guardrails.HostedInferenceCalls != 0 || summary.Guardrails.HostedTelemetryExports != 0 {
		t.Fatalf("expected zero guardrails: %+v", summary.Guardrails)
	}

	enrichedSource := mustReadString(t, filepath.Join(out, "sources", "source-1", "source.md"))
	for _, expected := range []string{"## Enriched Sources", "Retrieval mode: `local_artifact`", "Gateway evidence review", "Requirement: Gateway evidence must stay linked"} {
		if !strings.Contains(enrichedSource, expected) {
			t.Fatalf("missing %q in enriched source:\n%s", expected, enrichedSource)
		}
	}

	pressureOut := filepath.Join(root, "pressure")
	if _, _, err := BuildCorpusPressure(filepath.Join(out, "corpus-pressure-manifest.json"), pressureOut, CorpusPressureOptions{}); err != nil {
		t.Fatalf("BuildCorpusPressure over enriched manifest: %v", err)
	}
	meaningOut := filepath.Join(root, "meaning")
	meaning, _, err := BuildSourceMeaningPreview(pressureOut, meaningOut)
	if err != nil {
		t.Fatalf("BuildSourceMeaningPreview over enriched pressure: %v", err)
	}
	if meaning.PreviewedSourceCount != 1 {
		t.Fatalf("expected meaning preview for enriched source: %+v", meaning)
	}
	preview := mustReadString(t, filepath.Join(meaningOut, SourceMeaningPreviewDirName, "sources", "source-1.md"))
	if !strings.Contains(preview, "Review captured gateway evidence") && !strings.Contains(preview, "Requirement: Gateway evidence") {
		t.Fatalf("meaning preview did not expose enriched evidence:\n%s", preview)
	}
}

func TestBuildSourceEnrichmentAccountsForMissingUnsupportedAndBlockedURLs(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Captured links",
		"https://example.com/missing",
		"https://localhost/private",
		"https://example.com/file.zip",
		"file:///tmp/source.md",
		"slack-file-private://redacted",
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root)

	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, filepath.Join(root, "enriched"))
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 5 || summary.AccountedURLCount != 5 {
		t.Fatalf("expected every URL accounted for: %+v", summary)
	}
	if summary.NeedsManualURLCount == 0 || summary.BlockedURLCount < 3 || summary.UnsupportedURLCount != 1 {
		t.Fatalf("expected missing, unsupported, and blocked URL counts: %+v", summary)
	}
	artifact := mustReadString(t, filepath.Join(root, "enriched", SourceEnrichmentDirName, "sources", "source-1.json"))
	if strings.Contains(artifact, "file:///tmp/source.md") || strings.Contains(artifact, "slack-file-private://redacted") || strings.Contains(artifact, "https://localhost/private") {
		t.Fatalf("blocked URLs leaked in artifact:\n%s", artifact)
	}
	if !strings.Contains(artifact, "unsupported_source") || !strings.Contains(artifact, "blocked_private_or_secret") || !strings.Contains(artifact, "blocked_by_policy") {
		t.Fatalf("expected unsupported/private/policy states in artifact:\n%s", artifact)
	}
}

func TestBuildSourceEnrichmentStripsSlackMrkdwnLinkLabels(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Slack link\n\nSave this: <https://example.com/research|read this>\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:     "https://example.com/research",
		Title:   "Enriched Slack source",
		Excerpt: "Requirement: Slack mrkdwn labels must not alter artifact matching.",
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 1 || summary.EnrichedURLCount != 1 || summary.NeedsManualURLCount != 0 {
		t.Fatalf("expected Slack mrkdwn URL to match local artifact: %+v", summary)
	}
	artifact := mustReadString(t, filepath.Join(out, SourceEnrichmentDirName, "sources", "source-1.json"))
	if strings.Contains(artifact, "|read this") {
		t.Fatalf("Slack label leaked into extracted URL:\n%s", artifact)
	}
	if !strings.Contains(artifact, `"normalized_url": "https://example.com/research"`) {
		t.Fatalf("missing normalized URL without Slack label:\n%s", artifact)
	}
}

func TestBuildSourceEnrichmentIgnoresSlackAdapterPermalinkProvenance(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Slack capture",
		"- Permalink: slack://missing-permalink/DTEST/1710000001000001",
		"",
		"## Content",
		"https://example.com/research",
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:     "https://example.com/research",
		Title:   "Enriched Slack link",
		Excerpt: "Requirement: Slack destination URL has enough local evidence for enrichment.",
	})

	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, filepath.Join(root, "enriched"))
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 1 || summary.EnrichedURLCount != 1 || summary.BlockedURLCount != 0 {
		t.Fatalf("expected Slack adapter permalink to be ignored, not counted as a link: %+v", summary)
	}
	artifact := mustReadString(t, filepath.Join(root, "enriched", SourceEnrichmentDirName, "sources", "source-1.json"))
	report := mustReadString(t, filepath.Join(root, "enriched", SourceEnrichmentDirName, "enrichment-report.md"))
	all := readAllFiles(t, filepath.Join(root, "enriched"))
	if strings.Contains(all, "slack://missing-permalink") {
		t.Fatalf("Slack adapter permalink should be redacted from all enrichment outputs:\n%s", all)
	}
	if strings.Contains(artifact, "url_policy_blocked") || strings.Contains(report, "url_policy_blocked") {
		t.Fatalf("Slack adapter permalink should not be counted in enrichment artifacts:\n%s\n%s", artifact, report)
	}
}

func TestBuildSourceEnrichmentRedactsIgnoredSlackPermalinkWithoutOtherURLs(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Slack capture",
		"- Permalink: slack://missing-permalink/DTEST/1710000001000001",
		"",
		"## Content",
		"Captured without an external URL.",
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root)

	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, filepath.Join(root, "enriched"))
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 0 || summary.NoURLSourceCount != 1 {
		t.Fatalf("expected ignored Slack permalink not to count as a URL: %+v", summary)
	}
	all := readAllFiles(t, filepath.Join(root, "enriched"))
	if strings.Contains(all, "slack://missing-permalink") {
		t.Fatalf("ignored Slack permalink should be redacted from no-URL outputs:\n%s", all)
	}
}

func TestBuildSourceEnrichmentBlocksUnsafeSlackMrkdwnLinkLabels(t *testing.T) {
	root := t.TempDir()
	unsafeSecret := "sk-proj-" + strings.Repeat("c", 48)
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Slack link\n\nSave this: <https://example.com/research|"+unsafeSecret+">\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:   "https://example.com/research",
		Title: "Should not enrich",
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.BlockedURLCount != 1 {
		t.Fatalf("expected unsafe Slack label to block enrichment: %+v", summary)
	}
	all := readAllFiles(t, out)
	if strings.Contains(all, unsafeSecret) || strings.Contains(all, "Should not enrich") {
		t.Fatalf("unsafe Slack label or artifact payload leaked:\n%s", all)
	}
	if !strings.Contains(all, "unsafe_or_private_url") || !strings.Contains(all, redactedSourceEnrichmentURL) {
		t.Fatalf("expected unsafe Slack label redaction proof:\n%s", all)
	}
}

func TestBuildSourceEnrichmentBlocksWorkspaceSlackArchivePermalinks(t *testing.T) {
	root := t.TempDir()
	privateURL := "https://acme.slack.com/archives/C123/p1710000001000001"
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Slack permalink\n\n"+privateURL+"\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root)

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.NeedsManualURLCount != 0 || summary.BlockedURLCount != 1 {
		t.Fatalf("expected workspace Slack permalink to be blocked: %+v", summary)
	}
	all := readAllFiles(t, out)
	if strings.Contains(all, privateURL) || strings.Contains(all, "acme.slack.com/archives") {
		t.Fatalf("private Slack permalink leaked:\n%s", all)
	}
	if !strings.Contains(all, "unsafe_or_private_url") || !strings.Contains(all, redactedSourceEnrichmentURL) {
		t.Fatalf("expected private Slack permalink redaction proof:\n%s", all)
	}
}

func TestBuildSourceEnrichmentPrefersUnresolvedSourceStateOverEnriched(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Mixed links",
		"https://example.com/research",
		"https://example.com/missing",
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:     "https://example.com/research",
		Title:   "Enriched source",
		Excerpt: "Requirement: enriched source has supporting evidence.",
	})

	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, filepath.Join(root, "enriched"))
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.Sources[0].State != SourceEnrichmentStateNeedsManualProcessing || summary.StateCounts[SourceEnrichmentStateNeedsManualProcessing] != 1 {
		t.Fatalf("expected mixed enriched+missing source to remain unresolved: %+v", summary)
	}
	if summary.EnrichedURLCount != 1 || summary.NeedsManualURLCount != 1 {
		t.Fatalf("expected both URL states counted: %+v", summary)
	}
}

func TestBuildSourceEnrichmentRequiresLocalArtifactEvidence(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Captured link\n\nhttps://example.com/research\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:   "https://example.com/research",
		Title: "Title alone is not evidence",
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.NeedsManualURLCount != 1 || summary.EnrichedArtifactCoverage != 0 {
		t.Fatalf("expected title-only artifact to remain needs_manual_processing: %+v", summary)
	}
	artifact := mustReadString(t, filepath.Join(out, SourceEnrichmentDirName, "sources", "source-1.json"))
	if !strings.Contains(artifact, "insufficient_local_artifact_evidence") || strings.Contains(artifact, `"state": "enriched"`) {
		t.Fatalf("expected insufficient evidence reason and no enriched state:\n%s", artifact)
	}
}

func TestBuildSourceEnrichmentComputesPartialArtifactCoverage(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Mixed links",
		"https://example.com/research",
		"https://example.com/missing",
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:     "https://example.com/research",
		Title:   "Enriched source",
		Excerpt: "Requirement: enriched source has supporting evidence.",
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 2 || summary.EnrichedURLCount != 1 {
		t.Fatalf("expected one enriched URL across two source URLs: %+v", summary)
	}
	if summary.EnrichedArtifactCoverage != 0.5 {
		t.Fatalf("expected partial enriched artifact coverage, got %+v", summary)
	}
	report := mustReadString(t, filepath.Join(out, SourceEnrichmentDirName, "enrichment-report.md"))
	if !strings.Contains(report, "- Enriched artifact coverage: 0.50") {
		t.Fatalf("expected report to show partial artifact coverage:\n%s", report)
	}
}

func TestBuildSourceEnrichmentDoesNotTrimQueryValueSlashDuringMatching(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Query link\n\nhttps://example.com/?next=/\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:   "https://example.com/?next=",
		Title: "Wrong artifact",
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.NeedsManualURLCount != 1 {
		t.Fatalf("expected query-value slash URL not to match different artifact: %+v", summary)
	}
	artifact := mustReadString(t, filepath.Join(out, SourceEnrichmentDirName, "sources", "source-1.json"))
	if strings.Contains(artifact, "Wrong artifact") {
		t.Fatalf("query-value slash URL matched wrong artifact:\n%s", artifact)
	}
	if !strings.Contains(artifact, `"normalized_url": "https://example.com?next=/"`) {
		t.Fatalf("expected normalized URL to preserve query value slash:\n%s", artifact)
	}
}

func TestBuildSourceEnrichmentPreservesValidURLPunctuationDuringMatching(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Punctuated links",
		"https://example.com/Foo_(bar)",
		"https://example.com/reference[alpha]",
		"[markdown](https://example.com/plain)",
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root,
		LocalSourceEnrichmentArtifact{
			URL:     "https://example.com/Foo_(bar)",
			Title:   "Parenthesized source",
			Excerpt: "Requirement: parenthesized source has evidence.",
		},
		LocalSourceEnrichmentArtifact{
			URL:     "https://example.com/reference[alpha]",
			Title:   "Bracketed source",
			Excerpt: "Requirement: bracketed source has evidence.",
		},
		LocalSourceEnrichmentArtifact{
			URL:     "https://example.com/plain",
			Title:   "Markdown link source",
			Excerpt: "Requirement: markdown link source has evidence.",
		},
	)

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 3 || summary.EnrichedURLCount != 3 || summary.NeedsManualURLCount != 0 {
		t.Fatalf("expected punctuated URLs and Markdown link URL to match artifacts: %+v", summary)
	}
	artifact := mustReadString(t, filepath.Join(out, SourceEnrichmentDirName, "sources", "source-1.json"))
	for _, expected := range []string{
		`"normalized_url": "https://example.com/Foo_(bar)"`,
		`"normalized_url": "https://example.com/reference[alpha]"`,
		`"normalized_url": "https://example.com/plain"`,
		"Parenthesized source",
		"Bracketed source",
		"Markdown link source",
	} {
		if !strings.Contains(artifact, expected) {
			t.Fatalf("missing %q in artifact:\n%s", expected, artifact)
		}
	}
	if strings.Contains(artifact, `"raw_url": "https://example.com/Foo_("`) ||
		strings.Contains(artifact, `"normalized_url": "https://example.com/Foo_("`) ||
		strings.Contains(artifact, "https://example.com/reference[alpha]]") ||
		strings.Contains(artifact, "https://example.com/plain)") {
		t.Fatalf("URL punctuation was truncated or delimiter leaked:\n%s", artifact)
	}
}

func TestBuildSourceEnrichmentBlocksUnsafeArtifactPayload(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Link\n\nhttps://example.com/research\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	unsafeSecret := "sk-proj-" + strings.Repeat("a", 48)
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:     "https://example.com/research",
		Title:   "Unsafe artifact",
		Excerpt: "do not leak " + unsafeSecret,
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.BlockedURLCount != 1 {
		t.Fatalf("expected unsafe artifact to block enrichment: %+v", summary)
	}
	all := readAllFiles(t, out)
	if strings.Contains(all, unsafeSecret) || strings.Contains(all, "Unsafe artifact") {
		t.Fatalf("unsafe artifact payload leaked:\n%s", all)
	}
	if !strings.Contains(all, "unsafe_artifact_payload") {
		t.Fatalf("expected blocked reason in output:\n%s", all)
	}
}

func TestBuildSourceEnrichmentRedactsUnsafeSourceURLInAllOutput(t *testing.T) {
	root := t.TempDir()
	unsafeSecret := "sk-proj-" + strings.Repeat("b", 48)
	unsafeURL := "https://example.com/research?token=" + unsafeSecret
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Link\n\n"+unsafeURL+"\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root)

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 1 || summary.BlockedURLCount != 1 {
		t.Fatalf("expected unsafe URL accounted and blocked: %+v", summary)
	}
	all := readAllFiles(t, out)
	if strings.Contains(all, unsafeSecret) || strings.Contains(all, unsafeURL) {
		t.Fatalf("unsafe source URL leaked:\n%s", all)
	}
	if !strings.Contains(all, "unsafe_or_private_url") || !strings.Contains(all, redactedSourceEnrichmentURL) {
		t.Fatalf("expected redaction reason and marker in output:\n%s", all)
	}
}

func TestBuildSourceEnrichmentRedactsLongerBlockedURLPrefixMatches(t *testing.T) {
	root := t.TempDir()
	unsafeSecret := "sk-proj-" + strings.Repeat("c", 48)
	shortURL := "https://example.com/research?token=" + unsafeSecret
	longURL := shortURL + "TAILLEAK"
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Links",
		shortURL,
		longURL,
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root)

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.URLCount != 2 || summary.BlockedURLCount != 2 {
		t.Fatalf("expected both unsafe URLs accounted and blocked: %+v", summary)
	}
	all := readAllFiles(t, out)
	for _, forbidden := range []string{unsafeSecret, shortURL, longURL, "TAILLEAK"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("blocked URL prefix leaked %q:\n%s", forbidden, all)
		}
	}
}

func TestBuildSourceEnrichmentBlocksCredentialBearingURLUserinfo(t *testing.T) {
	root := t.TempDir()
	credentialURL := "https://user:pass@example.com/research"
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Link\n\n"+credentialURL+"\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:   credentialURL,
		Title: "Credentialed artifact",
	})

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.BlockedURLCount != 1 {
		t.Fatalf("expected credential-bearing URL to be blocked, not enriched: %+v", summary)
	}
	all := readAllFiles(t, out)
	for _, forbidden := range []string{credentialURL, "user:pass", "Credentialed artifact"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("credential-bearing URL leaked %q:\n%s", forbidden, all)
		}
	}
	if !strings.Contains(all, "url_policy_blocked") || !strings.Contains(all, redactedSourceEnrichmentURL) {
		t.Fatalf("expected policy redaction proof:\n%s", all)
	}
}

func TestBuildSourceEnrichmentBlocksCredentialMarkerURLs(t *testing.T) {
	root := t.TempDir()
	passwordURL := "https://example.com/research?password=open-sesame"
	apiKeyURL := "https://example.com/research?api_key=public-looking-value"
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", strings.Join([]string{
		"# Links",
		passwordURL,
		apiKeyURL,
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root,
		LocalSourceEnrichmentArtifact{
			URL:   passwordURL,
			Title: "Password artifact",
		},
		LocalSourceEnrichmentArtifact{
			URL:   apiKeyURL,
			Title: "API key artifact",
		},
	)

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.NeedsManualURLCount != 0 || summary.BlockedURLCount != 2 {
		t.Fatalf("expected credential marker URLs to be blocked: %+v", summary)
	}
	all := readAllFiles(t, out)
	for _, forbidden := range []string{passwordURL, apiKeyURL, "password=", "api_key=", "open-sesame", "public-looking-value", "Password artifact", "API key artifact"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("credential marker URL leaked %q:\n%s", forbidden, all)
		}
	}
	if !strings.Contains(all, "unsafe_or_private_url") || !strings.Contains(all, redactedSourceEnrichmentURL) {
		t.Fatalf("expected unsafe URL redaction proof:\n%s", all)
	}
}

func TestBuildSourceEnrichmentBlocksTrailingDotLocalhostURL(t *testing.T) {
	root := t.TempDir()
	privateURL := "http://localhost./private"
	sourcePath := writeSourceEnrichmentFixture(t, root, "source-1", "# Link\n\n"+privateURL+"\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-enrich-test", CorpusPressureManifestSource{
		SourceID:   "source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root)

	out := filepath.Join(root, "enriched")
	summary, err := BuildSourceEnrichment(manifestPath, artifactsPath, out)
	if err != nil {
		t.Fatalf("BuildSourceEnrichment: %v", err)
	}
	if summary.EnrichedURLCount != 0 || summary.NeedsManualURLCount != 0 || summary.BlockedURLCount != 1 {
		t.Fatalf("expected trailing-dot localhost URL to be blocked: %+v", summary)
	}
	all := readAllFiles(t, out)
	if strings.Contains(all, privateURL) || strings.Contains(all, "localhost./private") {
		t.Fatalf("trailing-dot localhost URL leaked:\n%s", all)
	}
	if !strings.Contains(all, "url_policy_blocked") || !strings.Contains(all, redactedSourceEnrichmentURL) {
		t.Fatalf("expected policy redaction proof:\n%s", all)
	}
}

func TestSourceEnrichmentURLPolicyBlocksUnsafeTargets(t *testing.T) {
	blocked := []string{
		"file:///tmp/source.md",
		"http://localhost/test",
		"http://localhost./test",
		"http://127.0.0.1/test",
		"http://10.0.0.1/test",
		"http://192.168.1.1/test",
		"http://169.254.169.254/latest/meta-data",
		"not a url",
	}
	for _, raw := range blocked {
		if _, _, ok := classifySourceEnrichmentURL(raw); ok {
			t.Fatalf("expected URL policy block for %s", raw)
		}
	}
	if normalized, kind, ok := classifySourceEnrichmentURL("https://example.com/research/"); !ok || normalized != "https://example.com/research" || kind != "web_url" {
		t.Fatalf("expected normalized public web URL, got normalized=%q kind=%q ok=%v", normalized, kind, ok)
	}
}

func writeSourceEnrichmentFixture(t *testing.T, root, sourceID, body string) string {
	t.Helper()
	rel := filepath.ToSlash(filepath.Join("fixtures", sourceID+".md"))
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return rel
}

func writeSourceEnrichmentManifest(t *testing.T, root, corpusID string, sources ...CorpusPressureManifestSource) string {
	t.Helper()
	path := filepath.Join(root, "corpus-pressure-manifest.json")
	writeDocumentsTestJSON(t, path, CorpusPressureManifest{
		SchemaVersion: CorpusPressureManifestSchemaVersion,
		CorpusID:      corpusID,
		Sources:       sources,
	})
	return path
}

func writeSourceEnrichmentArtifacts(t *testing.T, root string, artifacts ...LocalSourceEnrichmentArtifact) string {
	t.Helper()
	path := filepath.Join(root, "local-artifacts.json")
	data, err := json.MarshalIndent(LocalSourceEnrichmentArtifactManifest{
		SchemaVersion: LocalSourceEnrichmentArtifactsSchemaVersion,
		Artifacts:     artifacts,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal artifacts: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}
	return path
}

func readAllFiles(t *testing.T, root string) string {
	t.Helper()
	var combined strings.Builder
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		combined.Write(data)
		combined.WriteString("\n")
		return nil
	}); err != nil {
		t.Fatalf("walk output: %v", err)
	}
	return combined.String()
}
