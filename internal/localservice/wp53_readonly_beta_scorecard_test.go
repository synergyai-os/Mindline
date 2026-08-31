package localservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const (
	wp53ScorecardModeEnv   = "MINDLINE_WP53_SCORECARD_MODE"
	wp53ScorecardReportEnv = "MINDLINE_WP53_SCORECARD_REPORT"
)

type wp53ScorecardManifest struct {
	SchemaVersion string `json:"schema_version"`
	Records       []struct {
		RecordID string `json:"record_id"`
		Text     string `json:"text"`
	} `json:"records"`
	Resources []struct {
		ResourceID    string `json:"resource_id"`
		OwnerRecordID string `json:"owner_record_id"`
		Title         string `json:"title"`
		Content       string `json:"content"`
	} `json:"resources"`
	ResourceRevisions []struct {
		RevisionID    string `json:"revision_id"`
		ResourceID    string `json:"resource_id"`
		OwnerRecordID string `json:"owner_record_id"`
		Content       string `json:"content"`
	} `json:"resource_revisions"`
	Contexts []struct {
		ScopeID   string `json:"scope_id"`
		LensID    string `json:"lens_id"`
		LensQuery string `json:"lens_query"`
	} `json:"contexts"`
	AnswerableCases      []wp53ScorecardCase `json:"answerable_cases"`
	AbsentCases          []wp53ScorecardCase `json:"absent_cases"`
	SharedMembershipCase struct {
		Query                     string              `json:"query"`
		ScopeID                   string              `json:"scope_id"`
		LensIDs                   []string            `json:"lens_ids"`
		ExpectedEligibleRecordIDs []string            `json:"expected_eligible_record_ids"`
		ExpectedTopThreeByLens    map[string][]string `json:"expected_top_three_by_lens"`
	} `json:"shared_membership_case"`
	QualifyingSourceCase struct {
		RecordID                string   `json:"record_id"`
		Query                   string   `json:"query"`
		QualifyingResourceID    string   `json:"qualifying_resource_id"`
		HiddenSiblingResourceID string   `json:"hidden_sibling_resource_id"`
		HistoricalRevisionID    string   `json:"historical_revision_id"`
		RequiredVisibleMarker   string   `json:"required_visible_marker"`
		ForbiddenMarkers        []string `json:"forbidden_markers"`
	} `json:"qualifying_source_case"`
	MeasurementProfile struct {
		SemanticModel                 string `json:"semantic_model"`
		SemanticModelDigest           string `json:"semantic_model_digest"`
		CalibrationIdentity           string `json:"calibration_identity"`
		WarmupCompleteScorecardRuns   int    `json:"warmup_complete_scorecard_runs"`
		MeasuredCompleteScorecardRuns int    `json:"measured_complete_scorecard_runs"`
		MaximumWarmP95Seconds         int    `json:"maximum_warm_p95_seconds"`
	} `json:"measurement_profile"`
}

type wp53ScorecardCase struct {
	CaseID           string `json:"case_id"`
	ScopeID          string `json:"scope_id"`
	LensID           string `json:"lens_id"`
	Query            string `json:"query"`
	ExpectedRecordID string `json:"expected_record_id,omitempty"`
}

type wp53CaseResult struct {
	CaseID        string   `json:"case_id"`
	AnswerState   string   `json:"answer_state"`
	RankedRecords []string `json:"ranked_record_ids"`
	ExpectedRank  int      `json:"expected_rank,omitempty"`
	Passed        bool     `json:"passed"`
}

type wp53ScorecardReport struct {
	SchemaVersion        string              `json:"schema_version"`
	Mode                 string              `json:"mode"`
	SourceCommit         string              `json:"source_commit"`
	ManifestSHA256       string              `json:"manifest_sha256"`
	SemanticModel        string              `json:"semantic_model"`
	SemanticModelDigest  string              `json:"semantic_model_digest"`
	CalibrationIdentity  string              `json:"calibration_identity"`
	Answerable           []wp53CaseResult    `json:"answerable_cases"`
	Absent               []wp53CaseResult    `json:"absent_cases"`
	AnswerableTopThree   int                 `json:"answerable_top_three"`
	AbsentAbstentions    int                 `json:"absent_abstentions"`
	SharedMembership     map[string][]string `json:"shared_membership_by_lens"`
	SharedTopThree       map[string][]string `json:"shared_top_three_by_lens"`
	SharedMembershipPass bool                `json:"shared_membership_pass"`
	SharedOrderingPass   bool                `json:"shared_ordering_pass"`
	SourceIsolationPass  bool                `json:"source_isolation_pass"`
	VisibleMarkerFound   bool                `json:"visible_marker_found"`
	ForbiddenMarkersSeen []string            `json:"forbidden_markers_seen"`
	MeasuredSamples      int                 `json:"measured_samples"`
	WarmP95Milliseconds  int64               `json:"warm_p95_milliseconds"`
	Checks               map[string]bool     `json:"checks"`
	Passed               bool                `json:"passed"`
}

func TestWP53ReadonlyBetaScorecard(t *testing.T) {
	mode := strings.TrimSpace(os.Getenv(wp53ScorecardModeEnv))
	if mode == "" {
		t.Skip("WP-53 scorecard is an explicit proof test")
	}
	if mode != "baseline" && mode != "candidate" {
		t.Fatalf("%s must be baseline or candidate", wp53ScorecardModeEnv)
	}
	repoRoot, manifestPath := wp53ScorecardPaths(t)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest wp53ScorecardManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != "mindline-wp53-readonly-beta-scorecard/v0.1" ||
		len(manifest.AnswerableCases) != 8 || len(manifest.AbsentCases) != 4 {
		t.Fatal("invalid WP-53 scorecard authority")
	}
	manifestSum := sha256.Sum256(manifestBytes)
	modelDigest := wp53ResolveOllamaDigest(t, manifest.MeasurementProfile.SemanticModel)
	if modelDigest != strings.TrimPrefix(manifest.MeasurementProfile.SemanticModelDigest, "sha256:") {
		t.Fatalf("semantic model digest mismatch: got %s", modelDigest)
	}

	root, err := os.MkdirTemp("/tmp", "mindline-wp53-scorecard-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	aliasByRecordID, resourceAliasByID := wp53SeedScorecard(t, config.MemoryRoot, manifest)
	server, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve() }()
	client := NewClient(config.SocketPath)
	waitForService(t, client)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Close(ctx)
		<-serveResult
	})
	wp53WaitForSemanticIndex(t, client)
	wp53SeedContexts(t, client, manifest)
	if os.Getenv("MINDLINE_WP53_SCORECARD_DEBUG") == "1" {
		wp53LogScorecardCandidates(t, server, manifest, aliasByRecordID)
	}

	for range manifest.MeasurementProfile.WarmupCompleteScorecardRuns {
		wp53RunCases(t, client, manifest.AnswerableCases, aliasByRecordID, false)
		wp53RunCases(t, client, manifest.AbsentCases, aliasByRecordID, false)
	}
	var samples []time.Duration
	var answerable, absent []wp53CaseResult
	for run := 0; run < manifest.MeasurementProfile.MeasuredCompleteScorecardRuns; run++ {
		answerableRun, durations := wp53RunCases(t, client, manifest.AnswerableCases, aliasByRecordID, true)
		samples = append(samples, durations...)
		absentRun, durations := wp53RunCases(t, client, manifest.AbsentCases, aliasByRecordID, true)
		samples = append(samples, durations...)
		if run == 0 {
			answerable, absent = answerableRun, absentRun
		}
	}

	report := wp53ScorecardReport{
		SchemaVersion:       "mindline-wp53-readonly-beta-report/v0.1",
		Mode:                mode,
		SourceCommit:        wp53GitCommit(t, repoRoot, mode),
		ManifestSHA256:      hex.EncodeToString(manifestSum[:]),
		SemanticModel:       manifest.MeasurementProfile.SemanticModel,
		SemanticModelDigest: "sha256:" + modelDigest,
		CalibrationIdentity: manifest.MeasurementProfile.CalibrationIdentity,
		Answerable:          answerable,
		Absent:              absent,
		MeasuredSamples:     len(samples),
		WarmP95Milliseconds: wp53NearestRankP95(samples).Milliseconds(),
		Checks:              map[string]bool{},
	}
	for _, result := range answerable {
		if result.Passed {
			report.AnswerableTopThree++
		}
	}
	for _, result := range absent {
		if result.Passed {
			report.AbsentAbstentions++
		}
	}
	wp53EvaluateSharedMembership(t, client, manifest, aliasByRecordID, &report)
	wp53EvaluateSourceIsolation(t, client, manifest, aliasByRecordID, resourceAliasByID, &report)
	report.Checks["answerable_at_least_7_of_8"] = report.AnswerableTopThree >= 7
	report.Checks["absent_4_of_4"] = report.AbsentAbstentions == 4
	report.Checks["shared_membership"] = report.SharedMembershipPass
	report.Checks["shared_ordering"] = report.SharedOrderingPass
	report.Checks["source_isolation"] = report.SourceIsolationPass
	report.Checks["warm_p95_within_budget"] = report.WarmP95Milliseconds <= int64(manifest.MeasurementProfile.MaximumWarmP95Seconds*1000)
	report.Passed = true
	for _, passed := range report.Checks {
		report.Passed = report.Passed && passed
	}

	if mode == "candidate" {
		baseline := wp53ReadBaselineReport(t, repoRoot)
		report.Checks["at_least_two_more_hits_than_main"] = report.AnswerableTopThree >= baseline.AnswerableTopThree+2
		report.Checks["frozen_main_miss_now_found"] = wp53HasMainMissCandidateHit(baseline.Answerable, report.Answerable)
		report.Passed = report.Passed && report.Checks["at_least_two_more_hits_than_main"] && report.Checks["frozen_main_miss_now_found"]
	}
	wp53WriteReport(t, repoRoot, report)
	if mode == "candidate" && !report.Passed {
		t.Fatalf("WP-53 candidate scorecard failed: %+v", report.Checks)
	}
}

func wp53ScorecardPaths(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("scorecard source path unavailable")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return repoRoot, filepath.Join(repoRoot, ".productbrain", "specs", "fixtures", "wp53-readonly-beta-scorecard-v1.json")
}

func wp53SeedScorecard(t *testing.T, memoryRoot string, manifest wp53ScorecardManifest) (map[string]string, map[string]string) {
	t.Helper()
	repository, err := personalmemory.NewFileRepository(memoryRoot, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	aliases := map[string]string{}
	records := make([]personalmemory.CaptureRecord, 0, len(manifest.Records))
	for index, fixture := range manifest.Records {
		text := fixture.Text
		if fixture.RecordID == manifest.QualifyingSourceCase.RecordID {
			text += " https://wp53-fixture.invalid/delegation https://wp53-fixture.invalid/personal-logistics"
		}
		record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
			SourceAdapter: "wp53-public-fixture", SourceScopeID: "scorecard", SourceContainerID: "records",
			ExternalID: fixture.RecordID, OccurredAt: fmt.Sprintf("2026-08-31T10:%02d:00Z", index),
			SourceRef: "fixture://wp53/" + fixture.RecordID, RawText: text,
		})
		if err != nil {
			t.Fatal(err)
		}
		aliases[record.RecordID] = fixture.RecordID
		records = append(records, record)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "wp53-public-scorecard", LowerInclusive: "1", UpperInclusive: "9",
		Watermark: "9", DeclaredRecords: len(records), Records: records,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	oldContent := manifest.ResourceRevisions[0].Content
	wp53MergeResourceFixtures(t, repository, manifest, oldContent)
	wp53MergeResourceFixtures(t, repository, manifest, manifest.Resources[0].Content)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	resourceAliases := map[string]string{}
	for _, resource := range library.Resources {
		switch resource.CanonicalURL {
		case "https://wp53-fixture.invalid/delegation":
			resourceAliases[resource.ResourceID] = manifest.Resources[0].ResourceID
		case "https://wp53-fixture.invalid/personal-logistics":
			resourceAliases[resource.ResourceID] = manifest.Resources[1].ResourceID
		}
	}
	return aliases, resourceAliases
}

func wp53MergeResourceFixtures(t *testing.T, repository *personalmemory.FileRepository, manifest wp53ScorecardManifest, delegationContent string) {
	t.Helper()
	resources := []struct {
		url, title, content string
	}{
		{"https://wp53-fixture.invalid/delegation", manifest.Resources[0].Title, delegationContent},
		{"https://wp53-fixture.invalid/personal-logistics", manifest.Resources[1].Title, manifest.Resources[1].Content},
	}
	batch := personalmemory.EnrichmentBatch{SchemaVersion: personalmemory.EnrichmentBatchSchemaVersion}
	for index, fixture := range resources {
		batch.Resources = append(batch.Resources, acquisition.ImportedEvidence{
			CanonicalItemID: fmt.Sprintf("wp53-resource-%d", index), CanonicalURL: fixture.url,
			State: "complete", RetrievedAt: "2026-08-31T12:00:00Z", AccessClass: "public",
			Metadata: acquisition.ImportedMetadata{Title: fixture.title},
			Excerpts: []acquisition.ImportedExcerpt{{ExcerptID: "main", Text: fixture.content, Locator: "fixture"}},
		})
		batch.Contents = append(batch.Contents, personalmemory.ExtractedContent{
			CanonicalURL: fixture.url, MediaType: "text/plain", Completeness: "full",
			Text: fixture.content, AccessClass: "public",
		})
	}
	if _, err := repository.MergeEnrichment(batch); err != nil {
		t.Fatal(err)
	}
}

func wp53SeedContexts(t *testing.T, client *Client, manifest wp53ScorecardManifest) {
	t.Helper()
	seenScopes := map[string]bool{}
	for _, fixture := range manifest.Contexts {
		if !seenScopes[fixture.ScopeID] {
			if _, err := client.PutScope(context.Background(), agentstate.Scope{
				ID: fixture.ScopeID, Name: fixture.ScopeID, Purpose: fixture.ScopeID,
			}); err != nil {
				t.Fatal(err)
			}
			seenScopes[fixture.ScopeID] = true
		}
		if _, err := client.PutScopedLens(context.Background(), agentstate.ScopedLens{
			ScopeID: fixture.ScopeID, ID: fixture.LensID, Name: fixture.LensID, Query: fixture.LensQuery,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.PutActor(context.Background(), agentstate.AgentActor{ID: "agent-wp53-scorecard", Name: "WP-53 scorecard"}); err != nil {
		t.Fatal(err)
	}
}

func wp53LogScorecardCandidates(t *testing.T, server *Server, manifest wp53ScorecardManifest, aliases map[string]string) {
	t.Helper()
	snapshot, err := personalmemory.NewLexicalRetriever(server.repository).PrepareCompactIndex()
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[string]struct{ purpose, query string }{}
	for _, item := range manifest.Contexts {
		contexts[item.ScopeID+"\x00"+item.LensID] = struct{ purpose, query string }{item.ScopeID, item.LensQuery}
	}
	allCases := append(append([]wp53ScorecardCase(nil), manifest.AnswerableCases...), manifest.AbsentCases...)
	for _, item := range allCases {
		binding := contexts[item.ScopeID+"\x00"+item.LensID]
		hits, err := server.retrievalBackend(context.Background()).Rank(personalmemory.SearchRequest{
			Query: item.Query, LexicalQuery: item.Query, Limit: 100,
			ScopeID: item.ScopeID, ScopePurpose: binding.purpose,
			LensID: item.LensID, LensQuery: binding.query, AgentID: "agent-wp53-scorecard",
		}, snapshot.Documents)
		if err != nil {
			t.Fatal(err)
		}
		for index, hit := range hits {
			if index == 5 {
				break
			}
			t.Logf("%s candidate %d alias=%s document=%s components=%+v", item.CaseID, index+1, aliases[hit.DocumentID], hit.DocumentID, hit.Components)
		}
	}
	for _, lensID := range manifest.SharedMembershipCase.LensIDs {
		binding := contexts[manifest.SharedMembershipCase.ScopeID+"\x00"+lensID]
		hits, err := server.retrievalBackend(context.Background()).Rank(personalmemory.SearchRequest{
			Query:        manifest.SharedMembershipCase.Query,
			LexicalQuery: manifest.SharedMembershipCase.Query,
			Limit:        100,
			ScopeID:      manifest.SharedMembershipCase.ScopeID,
			ScopePurpose: binding.purpose,
			LensID:       lensID,
			LensQuery:    binding.query,
			AgentID:      "agent-wp53-scorecard",
		}, snapshot.Documents)
		if err != nil {
			t.Fatal(err)
		}
		for index, hit := range hits {
			if index == 8 {
				break
			}
			t.Logf("shared/%s candidate %d alias=%s document=%s components=%+v", lensID, index+1, aliases[hit.DocumentID], hit.DocumentID, hit.Components)
		}
	}
}

func wp53WaitForSemanticIndex(t *testing.T, client *Client) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		status, err := client.Status(context.Background())
		if err == nil && status.SemanticIndex.State == "ready" &&
			status.SemanticIndex.IndexedFingerprint == status.Memory.Fingerprint {
			return
		}
		if err == nil && status.SemanticIndex.State == "degraded" {
			t.Fatalf("semantic index degraded: %s", status.SemanticIndex.Reason)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("semantic index did not become ready")
}

func wp53RunCases(t *testing.T, client *Client, cases []wp53ScorecardCase, aliases map[string]string, measured bool) ([]wp53CaseResult, []time.Duration) {
	t.Helper()
	results := make([]wp53CaseResult, 0, len(cases))
	durations := make([]time.Duration, 0, len(cases))
	for _, fixture := range cases {
		started := time.Now()
		packet, err := client.SearchScoped(context.Background(), ScopedSearchInput{
			Query: fixture.Query, ScopeID: fixture.ScopeID, LensID: fixture.LensID,
			AgentID: "agent-wp53-scorecard", Limit: 3,
		})
		duration := time.Since(started)
		if err != nil {
			t.Fatalf("case %s: %v", fixture.CaseID, err)
		}
		if packet.RetrievalState != "hybrid" {
			t.Fatalf("case %s used degraded retrieval: %s", fixture.CaseID, packet.RetrievalState)
		}
		ranked := make([]string, 0, len(packet.Citations))
		for _, citation := range packet.Citations {
			ranked = append(ranked, aliases[citation.RecordID])
		}
		result := wp53CaseResult{CaseID: fixture.CaseID, AnswerState: packet.AnswerState, RankedRecords: ranked}
		if fixture.ExpectedRecordID == "" {
			result.Passed = packet.AnswerState == "abstained" && len(packet.Citations) == 0
		} else {
			for index, recordID := range ranked {
				if recordID == fixture.ExpectedRecordID {
					result.ExpectedRank = index + 1
					result.Passed = index < 3
				}
			}
		}
		results = append(results, result)
		if measured {
			durations = append(durations, duration)
		}
	}
	return results, durations
}

func wp53EvaluateSharedMembership(t *testing.T, client *Client, manifest wp53ScorecardManifest, aliases map[string]string, report *wp53ScorecardReport) {
	t.Helper()
	report.SharedMembership = map[string][]string{}
	report.SharedTopThree = map[string][]string{}
	report.SharedMembershipPass, report.SharedOrderingPass = true, true
	for _, lensID := range manifest.SharedMembershipCase.LensIDs {
		packet, err := client.SearchScoped(context.Background(), ScopedSearchInput{
			Query: manifest.SharedMembershipCase.Query, ScopeID: manifest.SharedMembershipCase.ScopeID,
			LensID: lensID, AgentID: "agent-wp53-scorecard", Limit: len(manifest.SharedMembershipCase.ExpectedEligibleRecordIDs),
		})
		if err != nil || packet.RetrievalState != "hybrid" {
			t.Fatalf("shared membership %s: packet=%+v err=%v", lensID, packet, err)
		}
		ordered := make([]string, 0, len(packet.Citations))
		for _, citation := range packet.Citations {
			ordered = append(ordered, aliases[citation.RecordID])
		}
		report.SharedMembership[lensID] = wp53Sorted(ordered)
		top := ordered
		if len(top) > 3 {
			top = top[:3]
		}
		report.SharedTopThree[lensID] = append([]string(nil), top...)
		report.SharedMembershipPass = report.SharedMembershipPass && wp53Equal(report.SharedMembership[lensID], wp53Sorted(manifest.SharedMembershipCase.ExpectedEligibleRecordIDs))
		report.SharedOrderingPass = report.SharedOrderingPass && wp53Equal(report.SharedTopThree[lensID], manifest.SharedMembershipCase.ExpectedTopThreeByLens[lensID])
	}
}

func wp53EvaluateSourceIsolation(t *testing.T, client *Client, manifest wp53ScorecardManifest, aliases, resourceAliases map[string]string, report *wp53ScorecardReport) {
	t.Helper()
	fixture := manifest.QualifyingSourceCase
	contextFixture := manifest.Contexts[0]
	packet, err := client.SearchScoped(context.Background(), ScopedSearchInput{
		Query: fixture.Query, ScopeID: contextFixture.ScopeID, LensID: contextFixture.LensID,
		AgentID: "agent-wp53-scorecard", Limit: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	var actualRecordID string
	for _, citation := range packet.Citations {
		if aliases[citation.RecordID] == fixture.RecordID {
			actualRecordID = citation.RecordID
			break
		}
	}
	if actualRecordID == "" {
		return
	}
	capture, err := client.GetScoped(context.Background(), ScopedGetInput{
		RunID: packet.RunID, ScopeID: contextFixture.ScopeID, LensID: contextFixture.LensID,
		AgentID: "agent-wp53-scorecard", RecordID: actualRecordID,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(capture)
	text := string(encoded)
	report.VisibleMarkerFound = strings.Contains(text, fixture.RequiredVisibleMarker)
	for _, marker := range fixture.ForbiddenMarkers {
		if strings.Contains(text, marker) {
			report.ForbiddenMarkersSeen = append(report.ForbiddenMarkersSeen, marker)
		}
	}
	qualifiedResources := []string{}
	for _, resource := range capture.Resources {
		qualifiedResources = append(qualifiedResources, resourceAliases[resource.ResourceID])
	}
	report.SourceIsolationPass = report.VisibleMarkerFound && len(report.ForbiddenMarkersSeen) == 0 &&
		wp53Equal(qualifiedResources, []string{fixture.QualifyingResourceID}) && len(capture.ResourceRevisions) == 0
}

func wp53ResolveOllamaDigest(t *testing.T, model string) string {
	t.Helper()
	response, err := http.Get("http://127.0.0.1:11434/api/tags")
	if err != nil {
		t.Fatalf("resolve Ollama model digest: %v", err)
	}
	defer response.Body.Close()
	var payload struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&payload) != nil {
		t.Fatal("resolve Ollama model digest")
	}
	for _, item := range payload.Models {
		if item.Name == model {
			return strings.TrimPrefix(item.Digest, "sha256:")
		}
	}
	t.Fatalf("Ollama model %s is unavailable", model)
	return ""
}

func wp53NearestRankP95(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	return sorted[index]
}

func wp53GitCommit(t *testing.T, repoRoot, mode string) string {
	t.Helper()
	revision := "HEAD"
	if mode == "baseline" {
		revision = "origin/main"
	}
	output, err := exec.Command("git", "-C", repoRoot, "rev-parse", revision).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func wp53WriteReport(t *testing.T, repoRoot string, report wp53ScorecardReport) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(wp53ScorecardReportEnv))
	if path == "" {
		t.Fatalf("%s is required", wp53ScorecardReportEnv)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func wp53ReadBaselineReport(t *testing.T, repoRoot string) wp53ScorecardReport {
	t.Helper()
	path := filepath.Join(repoRoot, ".productbrain", "proof", "wp53-main-scorecard.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report wp53ScorecardReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func wp53HasMainMissCandidateHit(baseline, candidate []wp53CaseResult) bool {
	mainPass := map[string]bool{}
	for _, result := range baseline {
		mainPass[result.CaseID] = result.Passed
	}
	for _, result := range candidate {
		if !mainPass[result.CaseID] && result.Passed {
			return true
		}
	}
	return false
}

func wp53Sorted(values []string) []string {
	copy := append([]string(nil), values...)
	sort.Strings(copy)
	return copy
}

func wp53Equal(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
