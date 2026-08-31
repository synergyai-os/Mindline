package localservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const (
	wp53PrivacySpecSHA256      = "d111240d62d43314df221f87b1878c7baa4b9153c31cfb50b17cfd51e0f9185f"
	wp53PrivacyScorecardSHA256 = "3694ecc6e7006a141b459ea2e76cc787b1ea6c81a0064eb95d0cda2c1475a7df"
	wp53PrivacyPlanSHA256      = "28f45237fcc380a3969b5e11ce17725e673523f24ca41001482215fdae1cbfa3"
)

func TestWP53ReadonlyBetaPrivacySentinel(t *testing.T) {
	repositoryRoot := wp53RepositoryRoot(t)
	wp53RequirePrivacyAuthority(t, repositoryRoot)
	root, err := os.MkdirTemp("/tmp", "mindline-wp53pathcanary8m2h-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	binary := filepath.Join(root, "mindline-privacy-candidate")
	wp53BuildMindline(t, repositoryRoot, binary)
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(config.RuntimeRoot, "config.json")
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	wp53RequireNoHostedTelemetrySurface(t, "service config", configBytes)

	query := "wp53querycanary7nq5 alphaomega betagamma"
	sourceText := "wp53sourcecanary4kq9 retained only in personal evidence"
	recordID := wp53SeedPrivacyEvidence(t, config.MemoryRoot, query, sourceText)
	connectionDigest := strings.Repeat("c", 64)
	scopeID, lensID, agentID := "wp53-privacy-scope", "wp53-privacy-lens", "agent-wp53-privacy"
	service := wp53StartExternalService(t, binary, configPath, config.SocketPath)
	if _, err := service.client.PutScope(context.Background(), agentstate.Scope{
		ID: scopeID, Name: "WP-53 privacy scope", Purpose: "privacy-safe local recall",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.client.PutScopedLens(context.Background(), agentstate.ScopedLens{
		ScopeID: scopeID, ID: lensID, Name: "WP-53 privacy lens", Query: "local evidence",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.client.PutActor(context.Background(), agentstate.AgentActor{
		ID: agentID, Name: "WP-53 privacy agent",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.client.BindProjectConnection(context.Background(), ProjectConnectionInput{
		Digest: connectionDigest, ScopeID: scopeID, LensID: lensID, AgentID: agentID,
	}); err != nil {
		t.Fatal(err)
	}
	if resolution, err := service.client.ResolveProjectConnection(context.Background(), connectionDigest); err != nil ||
		resolution.State != "ready" {
		t.Fatalf("privacy connection resolution=%+v err=%v", resolution, err)
	}
	packet, err := service.client.SearchScoped(context.Background(), ScopedSearchInput{
		Query: query, ScopeID: scopeID, LensID: lensID, AgentID: agentID, Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	var citation personalmemory.CompactCitation
	found := false
	for _, candidate := range packet.Citations {
		if candidate.RecordID == recordID {
			citation, found = candidate, true
			break
		}
	}
	if !found || citation.QualifyingSource.SourceKind != "current_resource" ||
		citation.QualifyingSource.SourceID == "" || citation.QualifyingSource.ContentHash == "" {
		t.Fatalf("privacy sentinel did not exercise an exact resource binding: %+v", packet.Citations)
	}
	if _, err := service.client.GetScoped(context.Background(), ScopedGetInput{
		RunID: packet.RunID, ScopeID: scopeID, LensID: lensID, AgentID: agentID, RecordID: recordID,
	}); err != nil {
		t.Fatal(err)
	}
	capabilities, err := service.client.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	capabilityBytes, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatal(err)
	}
	wp53RequireNoHostedTelemetrySurface(t, "agent capabilities", capabilityBytes)
	for _, route := range []string{"/v1/telemetry", "/v1/observability", "/v1/posthog", "/v1/events"} {
		request, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, "http://mindline.local"+route, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		response, err := service.client.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("hosted telemetry route %s is reachable: status=%d", route, response.StatusCode)
		}
	}
	service.stop(t)
	serviceLogs := service.stdout.String() + "\n" + service.stderr.String()
	for label, sentinel := range map[string]string{
		"query": query, "source text": sourceText, "local path": root,
		"opaque connection": connectionDigest, "retrieval handle": packet.RunID,
		"binding source": citation.QualifyingSource.SourceID,
		"binding hash":   citation.QualifyingSource.ContentHash,
	} {
		if strings.Contains(serviceLogs, sentinel) {
			t.Fatalf("%s entered service stdout/stderr logs", label)
		}
	}
	if strings.TrimSpace(service.stderr.String()) != "" {
		t.Fatalf("privacy sentinel service wrote stderr: %s", service.stderr.String())
	}
}

func wp53RequirePrivacyAuthority(t *testing.T, repositoryRoot string) {
	t.Helper()
	for path, expected := range map[string]string{
		filepath.Join(repositoryRoot, ".productbrain", "specs", "2026-08-31-wp-53-readonly-beta.md"):                wp53PrivacySpecSHA256,
		filepath.Join(repositoryRoot, ".productbrain", "specs", "fixtures", "wp53-readonly-beta-scorecard-v1.json"): wp53PrivacyScorecardSHA256,
		filepath.Join(repositoryRoot, ".productbrain", "plans", "2026-08-31-wp-53-readonly-beta.md"):                wp53PrivacyPlanSHA256,
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if actual := hex.EncodeToString(sum[:]); actual != expected {
			t.Fatalf("privacy proof authority changed for %s: got %s want %s", path, actual, expected)
		}
	}
}

func wp53RequireNoHostedTelemetrySurface(t *testing.T, label string, data []byte) {
	t.Helper()
	lower := strings.ToLower(string(data))
	for _, forbidden := range []string{"telemetry", "posthog", "hosted"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("%s exposes hosted telemetry configuration %q", label, forbidden)
		}
	}
}

func wp53SeedPrivacyEvidence(t *testing.T, root, query, sourceText string) string {
	t.Helper()
	repository, err := personalmemory.NewFileRepository(root, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	url := "https://wp53-privacy.invalid/log-canary"
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "wp53-public-fixture", SourceScopeID: "privacy", SourceContainerID: "records",
		ExternalID: "privacy-record", OccurredAt: "2026-08-31T12:00:00Z",
		SourceRef: "fixture://wp53/privacy-record", RawText: sourceText + " " + url,
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "wp53-public-privacy", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	content := query + " " + sourceText
	enrichment := personalmemory.EnrichmentBatch{
		SchemaVersion: personalmemory.EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: "wp53-privacy-resource", CanonicalURL: url, State: "complete",
			RetrievedAt: "2026-08-31T12:01:00Z", AccessClass: "public",
			Metadata: acquisition.ImportedMetadata{Title: "WP-53 privacy resource"},
			Excerpts: []acquisition.ImportedExcerpt{{ExcerptID: "main", Text: content, Locator: "fixture"}},
		}},
		Contents: []personalmemory.ExtractedContent{{
			CanonicalURL: url, MediaType: "text/plain", Completeness: "full",
			Text: content, AccessClass: "public",
		}},
	}
	if _, err := repository.MergeEnrichment(enrichment); err != nil {
		t.Fatal(err)
	}
	return record.RecordID
}
