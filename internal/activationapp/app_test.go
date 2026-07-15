package activationapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/integrations"
	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/productbrain"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func TestSyntheticActivationFreezesFullQueueAndDeliversOnlySealedProof(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	transport := newMemoryTransport()
	app, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: transport}}))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manifestPath := filepath.Join(root, "synthetic-manifest.json")
	payload, _ := json.Marshal(syntheticManifest(t, 4, true))
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportExternalInventory(context.Background(), manifestPath, filepath.Base(manifestPath)); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Execute(context.Background(), controlui.Command{Kind: "save_strategy", Payload: testStrategyPayload("Product strategy", "Promote evidence-backed relevant sources; hold missing evidence.")}); err != nil {
		t.Fatal(err)
	}
	secret := []byte("pb_sk_SENTINEL_NEVER_PERSIST_1234567890")
	if _, err := app.Execute(context.Background(), controlui.Command{Kind: "connect_destination", Payload: secret}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Execute(context.Background(), controlui.Command{Kind: "freeze_inventory", Payload: struct{}{}}); err != nil {
		t.Fatal(err)
	}
	viewAny, _ := app.State(context.Background())
	view := viewAny.(View)
	if view.Inventory.CanonicalItems != 4 || view.Inventory.SelectedItems != 3 || view.Inventory.UnselectedItems != 1 {
		t.Fatalf("full queue or capped sample changed: %+v", view.Inventory)
	}
	if _, err := app.Execute(context.Background(), controlui.Command{Kind: "start_proof", Payload: struct{}{}}); err != nil {
		t.Fatal(err)
	}
	reviewAllSelected(t, app)
	preview, err := app.PreviewBatch(context.Background(), 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.OperationFingerprints) != 3 || preview.MaximumDestinationWrites != 3 {
		t.Fatalf("unexpected exact batch: %+v", preview)
	}
	human := controlui.HumanApproval{Preview: preview, InitiationFingerprint: strings.Repeat("a", 64), SessionFingerprint: strings.Repeat("b", 64), GestureRecordedAt: now.Format(time.RFC3339Nano)}
	receiptAny, err := app.approveBatch(context.Background(), human, true)
	if err != nil {
		t.Fatal(err)
	}
	receipt := receiptAny.(productbrain.ApprovedDeliveryReceipt)
	if receipt.Status != "completed" || receipt.AcknowledgedOperations != 3 || transport.mutationCalls != 3 {
		t.Fatalf("unexpected delivery receipt: %+v calls=%d", receipt, transport.mutationCalls)
	}
	if _, err := executeHumanTest(app, "founder_review", map[string]any{
		"receipt_fingerprint": receipt.Fingerprint, "useful_draft_ids": receipt.RemoteObjectIDs, "value_verdict": "useful", "usefulness_reason": "The drafts preserve usable evidence.",
		"credential_burden": "acceptable for the spike", "manual_support_burden": "bounded", "approval_burden": "one exact batch", "zero_draft": false,
		"discovery_metrics": founderMetricFixture(now, true),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeHumanTest(app, "confirm_experimental_drain", struct{}{}); err != nil {
		t.Fatal(err)
	}
	viewAny, _ = app.State(context.Background())
	view = viewAny.(View)
	if !view.Drain.ExperimentalDrainAuthorized || !view.Drain.FullInventoryQueued || view.Drain.ProcessedRemainder || !view.Founder.TrustedValueObserved {
		t.Fatalf("drain readiness or founder outcome is false: %+v", view)
	}
	data, err := os.ReadFile(filepath.Join(root, stateFilename))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SENTINEL_NEVER_PERSIST") {
		t.Fatal("session credential reached durable activation state")
	}
}

func TestSensitiveRedactedOccurrenceIsReviewedButCannotReachDestination(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	transport := newMemoryTransport()
	app, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: transport}}))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manifest, err := acquisitionslack.BuildExternalManifest(acquisitionslack.BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: acquisitionslack.DataClassSynthetic,
		Messages: []acquisitionslack.NativeMessage{{NativeMessageID: "message-sensitive", Timestamp: "1700000000.000001", Text: "https://example.com/shared/" + "xoxb" + "-synthetic-value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(manifest)
	if strings.Contains(string(payload), "synthetic-value") || strings.Contains(string(payload), "amp;token") {
		t.Fatal("secret-bearing URL material reached the serialized inventory")
	}
	manifestPath := filepath.Join(root, "sensitive-redacted.json")
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportExternalInventory(context.Background(), manifestPath, filepath.Base(manifestPath)); err != nil {
		t.Fatal(err)
	}
	for _, command := range []controlui.Command{
		{Kind: "save_strategy", Payload: testStrategyPayload("Product strategy", "Hold sources without public evidence.")},
		{Kind: "connect_destination", Payload: []byte("pb_sk_SYNTHETIC_REDACTED")},
		{Kind: "freeze_inventory", Payload: struct{}{}},
		{Kind: "start_proof", Payload: struct{}{}},
	} {
		if _, err := app.Execute(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	stateAny, _ := app.State(context.Background())
	state := stateAny.(View)
	if len(state.Proof.Items) != 1 || !state.Proof.Items[0].RequiresManualReview || state.Proof.Items[0].CanonicalURL != "" || len(state.Proof.Items[0].SourceReferences) != 1 {
		t.Fatalf("redacted item did not reach the content-free manual queue: %+v", state.Proof.Items)
	}
	item := state.Proof.Items[0]
	if item.SourceReferences[0].NativeMessageID != "message-sensitive" || item.SourceReferences[0].URLOrdinal != 0 {
		t.Fatalf("redacted item lost content-free source provenance: %+v", item.SourceReferences)
	}
	if _, err := executeHumanTest(app, "review_item", map[string]string{
		"item_id": item.CanonicalItemID, "decision": "revise", "role": "evidence_backed_finding", "disposition": "promote",
		"rationale": "Attempted promotion without evidence.", "manual_support_outcome": "queued_for_manual_processing",
	}); err == nil {
		t.Fatal("sensitive-redacted item was promotable without evidence")
	}
	reviewAllSelected(t, app)
	preview, err := app.PreviewBatch(context.Background(), 1, 1)
	if err != nil || preview.OperationCount != 0 || transport.mutationCalls != 0 {
		t.Fatalf("sensitive-redacted item reached the destination boundary: preview=%+v calls=%d err=%v", preview, transport.mutationCalls, err)
	}
}

func TestPreSTD20ActivationStateRequiresRebuildBeforeFingerprintValidation(t *testing.T) {
	root := t.TempDir()
	oldState := []byte(`{"schema_version":"mindline-trusted-activation-state/v0.2","fingerprint":"old-state","run_id":"trusted-slack-activation","configuration_fingerprint":"old-configuration"}`)
	if err := os.WriteFile(filepath.Join(root, stateFilename), oldState, 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Connector: fakeConnector{transport: newMemoryTransport()}}))
	if app != nil {
		app.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "requires rebuild after STD-20") {
		t.Fatalf("old activation state did not fail at the explicit rebuild boundary: %v", err)
	}
}

func TestLiveModeRequiresCommitBoundCompleteReceipt(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	if _, err := New(Options{RuntimeRoot: root, Now: func() time.Time { return now }}); err == nil {
		t.Fatal("live mode started without a receipt")
	}
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "synthetic-test", Outcome: "pass", EvidenceFingerprint: "sha256:test-" + name})
	}
	receipt, err := assurance.Build("commit-1", DefaultConfigurationFingerprint(), "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now, checks)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(Options{RuntimeRoot: root, Commit: "commit-1", ConfigurationFingerprint: DefaultConfigurationFingerprint(), PreLiveReceipt: &receipt, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	app.Close()
	if _, err := New(Options{RuntimeRoot: t.TempDir(), Commit: "commit-drift", ConfigurationFingerprint: DefaultConfigurationFingerprint(), PreLiveReceipt: &receipt, Now: func() time.Time { return now }}); err == nil {
		t.Fatal("commit drift was accepted")
	}
}

func TestAppRejectsCallerConstructedHumanGesture(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	app, err := New(Options{RuntimeRoot: t.TempDir(), SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: newMemoryTransport()}})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	payload := map[string]any{"value_verdict": "useful"}
	forged := controlui.HumanAction{
		Kind: "founder_review", PayloadFingerprint: controlui.FingerprintPayload(payload),
		SessionFingerprint: strings.Repeat("a", 64), GestureRecordedAt: now.Format(time.RFC3339Nano), NonceFingerprint: strings.Repeat("b", 64),
	}
	if _, err := app.Execute(context.Background(), controlui.Command{Kind: "founder_review", Payload: payload, HumanAction: &forged}); err == nil || !strings.Contains(err.Error(), "human browser action rejected") {
		t.Fatalf("caller-constructed gesture reached application authority: %v", err)
	}
}

func TestLiveSlackSourceReservesWindowRestartsSafelyAndClearsOnlyAfterAdoption(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	receipt := liveReceiptFixture(t, now)
	firstClient := &activationSlackClient{
		workspace: "T-PROOF",
		history: map[string]acquisitionslack.WebAPIPage{
			"":       {Messages: []acquisitionslack.WebAPIMessage{{Timestamp: "120.000001", Text: "https://github.com/acme/tool"}}, NextCursor: "page-2"},
			"page-2": {Messages: []acquisitionslack.WebAPIMessage{{Timestamp: "140.000001", Text: "https://youtu.be/proofvideo"}}},
		},
		failures: map[string]int{"page-2": 1},
	}
	firstSource := &fakeSlackSourceConnector{client: firstClient, now: now}
	app, err := New(Options{
		RuntimeRoot: root, Commit: "commit-1", ConfigurationFingerprint: DefaultConfigurationFingerprint(), PreLiveReceipt: &receipt,
		Now: func() time.Time { return now }, Connector: fakeConnector{transport: newMemoryTransport()}, SourceConnector: firstSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Execute(context.Background(), controlui.Command{Kind: "save_strategy", Payload: testStrategyPayload("Product landscape", "Hold incomplete evidence.")}); err != nil {
		t.Fatal(err)
	}
	slackSecret := []byte("xoxp-SENTINEL-SLACK-SESSION-ONLY")
	if _, err := app.ConnectSlackSource(context.Background(), slackSecret, "CPROOF"); err != nil {
		t.Fatal(err)
	}
	for index := range slackSecret {
		slackSecret[index] = 0
	}
	if firstSource.budgets.MaximumRequests != 5_000 || firstSource.budgets.MaximumRetries != 8 || firstSource.budgets.MaximumWallTime != 20*time.Minute || firstSource.budgets.MaximumCostMicrounits != 20_000 {
		t.Fatalf("operator policy was not intersected with Slack safety limits: %+v", firstSource.budgets)
	}
	if _, err := app.DrainSlackSource(context.Background(), "100.000001", "199.000001"); err == nil {
		t.Fatal("interrupted Slack pagination unexpectedly completed")
	}
	if firstClient.calls[""] != 1 || firstClient.calls["page-2"] != 1 || app.state.SlackDrainWindow == nil {
		t.Fatalf("first drain did not reserve and checkpoint exact scope: calls=%v window=%+v", firstClient.calls, app.state.SlackDrainWindow)
	}
	app.Close()

	secondClient := &activationSlackClient{workspace: "T-PROOF", history: firstClient.history}
	secondSource := &fakeSlackSourceConnector{client: secondClient, now: now.Add(time.Minute)}
	app, err = New(Options{
		RuntimeRoot: root, Commit: "commit-1", ConfigurationFingerprint: DefaultConfigurationFingerprint(), PreLiveReceipt: &receipt,
		Now: func() time.Time { return now.Add(time.Minute) }, Connector: fakeConnector{transport: newMemoryTransport()}, SourceConnector: secondSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	viewAny, err := app.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	view := viewAny.(View)
	if view.Connections.SourceConnected || view.Connections.SourceIdentity == nil || view.Connections.SourceIdentity.ChannelID != "CPROOF" {
		t.Fatalf("restart did not preserve non-authorizing source identity: %+v", view.Connections)
	}
	if _, err := app.ConnectSlackSource(context.Background(), []byte("xoxp-SENTINEL-SLACK-SESSION-ONLY"), "CPROOF"); err != nil {
		t.Fatal(err)
	}
	result, err := app.DrainSlackSource(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if secondClient.calls[""] != 1 || secondClient.calls["page-2"] != 1 {
		t.Fatalf("restart did not safely re-read the exact frozen window: %v", secondClient.calls)
	}
	accepted := result.(map[string]any)
	if accepted["source_records"] != 2 || accepted["url_occurrences"] != 2 || accepted["canonical_items"] != 2 {
		t.Fatalf("resumed source inventory accounting drifted: %+v", accepted)
	}
	if _, err := app.Execute(context.Background(), controlui.Command{Kind: "freeze_inventory", Payload: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if app.state.Plan == nil || app.state.Plan.ComponentVersions["acquisition"] != acquisitionslack.WebAPIAdapterVersion {
		t.Fatalf("run plan mislabeled the actual source adapter: %+v", app.state.Plan)
	}
	viewAny, err = app.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	view = viewAny.(View)
	if len(view.Drain.Stages) == 0 || view.Drain.Stages[0].Stage != orchestration.StageInventory || view.Drain.Stages[0].Verdict != orchestration.VerdictReady {
		t.Fatalf("Slack Web API readiness did not use its own contributor contract: %+v", view.Drain.Stages)
	}
	checkpointStore, err := acquisitionslack.NewFileWebAPICheckpointStore(filepath.Join(root, "slack-web-api-checkpoints"))
	if err != nil {
		t.Fatal(err)
	}
	scope := acquisitionslack.WebAPICheckpointScope{WorkspaceID: "T-PROOF", ChannelID: "CPROOF", Oldest: "100.000001", Latest: "199.000001"}
	if _, found, err := checkpointStore.Load(context.Background(), scope); err != nil || found {
		t.Fatalf("checkpoint was not cleared after durable inventory adoption: found=%v err=%v", found, err)
	}
	stateBytes, err := os.ReadFile(filepath.Join(root, stateFilename))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stateBytes), "SENTINEL-SLACK") {
		t.Fatal("Slack credential reached durable activation state")
	}
}

func liveReceiptFixture(t *testing.T, now time.Time) assurance.Receipt {
	t.Helper()
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "synthetic-test", Outcome: "pass", EvidenceFingerprint: "sha256:test-" + name})
	}
	receipt, err := assurance.Build("commit-1", DefaultConfigurationFingerprint(), "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now, checks)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestNoLinkInventoryCompletesTruthfulZeroDraftActivation(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	transport := newMemoryTransport()
	app, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: transport}}))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manifest, err := acquisitionslack.BuildExternalManifest(acquisitionslack.BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000001.000001", Watermark: "1700000001.000001", DataClass: acquisitionslack.DataClassSynthetic,
		Messages: []acquisitionslack.NativeMessage{{NativeMessageID: "message-no-link", Timestamp: "1700000001.000001", Text: "a source record with no URL"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "no-link.json")
	payload, _ := json.Marshal(manifest)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportExternalInventory(context.Background(), path, filepath.Base(path)); err != nil {
		t.Fatal(err)
	}
	for _, command := range []controlui.Command{
		{Kind: "save_strategy", Payload: testStrategyPayload("Product strategy", "Hold missing evidence.")},
		{Kind: "connect_destination", Payload: []byte("pb_sk_SYNTHETIC_ZERO_DRAFT")},
		{Kind: "freeze_inventory", Payload: struct{}{}},
		{Kind: "start_proof", Payload: struct{}{}},
	} {
		if _, err := app.Execute(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := app.PreviewBatch(context.Background(), 1, 1)
	if err != nil || preview.OperationCount != 0 {
		t.Fatalf("zero preview mismatch: %+v err=%v", preview, err)
	}
	human := controlui.HumanApproval{Preview: preview, InitiationFingerprint: strings.Repeat("a", 64), SessionFingerprint: strings.Repeat("b", 64), GestureRecordedAt: now.Format(time.RFC3339Nano)}
	receiptAny, err := app.approveBatch(context.Background(), human, true)
	if err != nil {
		t.Fatal(err)
	}
	receipt := receiptAny.(ZeroDeliveryReceipt)
	if _, err := executeHumanTest(app, "founder_review", map[string]any{
		"receipt_fingerprint": receipt.Fingerprint, "useful_draft_ids": []string{}, "value_verdict": "zero_draft", "usefulness_reason": "No eligible source existed.",
		"credential_burden": "bounded", "manual_support_burden": "none", "approval_burden": "one zero-batch review", "zero_draft": true,
		"discovery_metrics": founderMetricFixture(now, false),
	}); err != nil {
		t.Fatal(err)
	}
	viewAny, _ := app.State(context.Background())
	view := viewAny.(View)
	if !view.Founder.TrustedActivationCompletion || view.Founder.TrustedValueObserved || !view.Drain.FullInventoryQueued {
		t.Fatalf("zero-draft truth drifted: %+v", view)
	}
}

func founderMetricFixture(now time.Time, value bool) map[string]any {
	metrics := map[string]any{"started_at": now.Add(-time.Minute).Format(time.RFC3339Nano), "submitted_at": now.Format(time.RFC3339Nano), "elapsed_milliseconds": int64(60_000), "errors": 0, "retries": 0, "backtracks": 0, "help_requests": 0}
	if value {
		metrics["time_to_trusted_value_milliseconds"] = int64(60_000)
	}
	return metrics
}

func TestControlUIOneTimeCeremonyAuthorizesExactDelivery(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	transport := newMemoryTransport()
	app, err := New(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manifestPath := filepath.Join(root, "synthetic-manifest.json")
	payload, _ := json.Marshal(syntheticManifest(t, 3, true))
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportExternalInventory(context.Background(), manifestPath, filepath.Base(manifestPath)); err != nil {
		t.Fatal(err)
	}
	commands := []controlui.Command{
		{Kind: "save_strategy", Payload: testStrategyPayload("Product strategy", "Promote evidence-backed relevant sources.")},
		{Kind: "connect_destination", Payload: []byte("pb_sk_SYNTHETIC_BROWSER_CEREMONY")},
		{Kind: "freeze_inventory", Payload: struct{}{}},
		{Kind: "start_proof", Payload: struct{}{}},
	}
	for _, command := range commands {
		if _, err := app.Execute(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	reviewAllSelected(t, app)
	server, err := controlui.New(app, controlui.Options{ExpectedHost: "127.0.0.1:43125", Origin: "http://127.0.0.1:43125", RuntimeRoot: root, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := strings.TrimPrefix(server.BootstrapFragment(), "bootstrap=")
	response := uiRequest(t, server, http.MethodPost, "/api/bootstrap", strings.NewReader("{}"), map[string]string{"Content-Type": "application/json", "X-Mindline-Bootstrap": bootstrap})
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap failed: %d %s", response.Code, response.Body.String())
	}
	var session map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Content-Type": "application/json", "X-Mindline-Session": session["session"], "X-Mindline-CSRF": session["csrf"]}
	response = uiRequest(t, server, http.MethodPost, "/api/commands/preview-batch", strings.NewReader(`{"maximum_destination_writes":3,"maximum_mutation_attempts":6}`), headers)
	if response.Code != http.StatusOK {
		t.Fatalf("preview failed: %d %s", response.Code, response.Body.String())
	}
	var preview struct {
		ReviewNonce string `json:"review_nonce"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"review_nonce": preview.ReviewNonce})
	response = uiRequest(t, server, http.MethodPost, "/api/commands/approve-batch", strings.NewReader(string(body)), headers)
	if response.Code != http.StatusOK || transport.mutationCalls != 3 {
		t.Fatalf("approval failed: %d calls=%d body=%s", response.Code, transport.mutationCalls, response.Body.String())
	}
	response = uiRequest(t, server, http.MethodPost, "/api/commands/approve-batch", strings.NewReader(string(body)), headers)
	if response.Code != http.StatusConflict || transport.mutationCalls != 3 {
		t.Fatalf("one-time approval replay was not blocked: %d calls=%d", response.Code, transport.mutationCalls)
	}
}

func TestControlUIFreshGestureResumesOnlyExistingSealedBatch(t *testing.T) {
	root := t.TempDir()
	clock := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	transport := newMemoryTransport()
	transport.mutationFailures = 1
	app, err := New(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return clock }, Connector: fakeConnector{transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manifestPath := filepath.Join(root, "resume-manifest.json")
	payload, _ := json.Marshal(syntheticManifest(t, 1, true))
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportExternalInventory(context.Background(), manifestPath, filepath.Base(manifestPath)); err != nil {
		t.Fatal(err)
	}
	for _, command := range []controlui.Command{
		{Kind: "save_strategy", Payload: testStrategyPayload("Product strategy", "Promote evidence-backed relevant sources.")},
		{Kind: "connect_destination", Payload: []byte("pb_sk_SYNTHETIC_BROWSER_RESUME")},
		{Kind: "freeze_inventory", Payload: struct{}{}},
		{Kind: "start_proof", Payload: struct{}{}},
	} {
		if _, err := app.Execute(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	reviewAllSelected(t, app)
	preview, err := app.PreviewBatch(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	human := controlui.HumanApproval{Preview: preview, InitiationFingerprint: strings.Repeat("a", 64), SessionFingerprint: strings.Repeat("b", 64), GestureRecordedAt: clock.Format(time.RFC3339Nano)}
	firstReceipt, firstErr := app.approveBatch(context.Background(), human, true)
	if firstErr == nil || firstReceipt.(productbrain.ApprovedDeliveryReceipt).Status == "completed" || transport.mutationCalls != 1 {
		t.Fatalf("synthetic first attempt did not stop in resumable state: receipt=%+v err=%v calls=%d", firstReceipt, firstErr, transport.mutationCalls)
	}
	approvalFingerprint := app.state.Approval.Fingerprint
	clock = clock.Add(10 * time.Second)
	capture := &capturingApplication{App: app}
	server, err := controlui.New(capture, controlui.Options{ExpectedHost: "127.0.0.1:43125", Origin: "http://127.0.0.1:43125", RuntimeRoot: root, Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := strings.TrimPrefix(server.BootstrapFragment(), "bootstrap=")
	response := uiRequest(t, server, http.MethodPost, "/api/bootstrap", strings.NewReader("{}"), map[string]string{"Content-Type": "application/json", "X-Mindline-Bootstrap": bootstrap})
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap failed: %d %s", response.Code, response.Body.String())
	}
	var session map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Content-Type": "application/json", "X-Mindline-Session": session["session"], "X-Mindline-CSRF": session["csrf"]}
	response = uiRequest(t, server, http.MethodPost, "/api/commands/resume-delivery", strings.NewReader("{}"), headers)
	if response.Code != http.StatusOK || capture.command.Kind != "resume_delivery" || capture.command.HumanAction == nil {
		t.Fatalf("control UI did not mint the fresh resume gesture: status=%d body=%s command=%+v", response.Code, response.Body.String(), capture.command)
	}
	if _, err := app.Execute(context.Background(), capture.command); err != nil || transport.mutationCalls != 2 {
		t.Fatalf("fresh browser resume failed: err=%v calls=%d consent=%+v delivery=%+v", err, transport.mutationCalls, app.state.DeliveryResume, app.state.Delivery)
	}
	if app.state.Approval.Fingerprint != approvalFingerprint || app.state.Delivery == nil || app.state.Delivery.Status != "completed" || app.state.DeliveryResume == nil {
		t.Fatalf("resume replaced authority or failed completion: approval=%+v delivery=%+v consent=%+v", app.state.Approval, app.state.Delivery, app.state.DeliveryResume)
	}
}

type capturingApplication struct {
	*App
	command controlui.Command
}

func (application *capturingApplication) Execute(_ context.Context, command controlui.Command) (any, error) {
	application.command = command
	return map[string]any{"captured": true}, nil
}

func TestRunJournalRestoresMissingActivationProjectionFromImmutableSnapshot(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	app, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: newMemoryTransport()}}))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "synthetic-manifest.json")
	payload, _ := json.Marshal(syntheticManifest(t, 2, true))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportExternalInventory(context.Background(), path, filepath.Base(path)); err != nil {
		t.Fatal(err)
	}
	for _, command := range []controlui.Command{
		{Kind: "save_strategy", Payload: testStrategyPayload("Product strategy", "Promote evidence-backed sources.")},
		{Kind: "connect_destination", Payload: []byte("pb_sk_SYNTHETIC_JOURNAL_RECOVERY")},
		{Kind: "freeze_inventory", Payload: struct{}{}}, {Kind: "start_proof", Payload: struct{}{}},
	} {
		if _, err := app.Execute(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	reviewAllSelected(t, app)
	beforeAny, _ := app.State(context.Background())
	before := beforeAny.(View)
	app.Close()
	if err := os.Remove(filepath.Join(root, stateFilename)); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: newMemoryTransport()}}))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	afterAny, _ := restarted.State(context.Background())
	after := afterAny.(View)
	if !after.Proof.Completed || after.Proof.ReviewedCount != before.Proof.ReviewedCount || after.Inventory.QueueFingerprint != before.Inventory.QueueFingerprint {
		t.Fatalf("journal/snapshot recovery drifted: before=%+v after=%+v", before, after)
	}
}

func TestPersistedProjectionWithoutJournalFailsClosed(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	app, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: newMemoryTransport()}}))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "synthetic-manifest.json")
	payload, _ := json.Marshal(syntheticManifest(t, 1, true))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportExternalInventory(context.Background(), path, filepath.Base(path)); err != nil {
		t.Fatal(err)
	}
	app.Close()
	if err := os.RemoveAll(filepath.Join(root, "run-journal")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(withTestHuman(Options{RuntimeRoot: root, SyntheticOnly: true, Now: func() time.Time { return now }, Connector: fakeConnector{transport: newMemoryTransport()}})); err == nil || !strings.Contains(err.Error(), "without its authoritative run journal") {
		t.Fatalf("mutable projection re-anchored without its journal: %v", err)
	}
}

func uiRequest(t *testing.T, server *controlui.Server, method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1:43125"+path, body)
	request.Host = "127.0.0.1:43125"
	request.RemoteAddr = "127.0.0.1:49125"
	request.Header.Set("Origin", "http://127.0.0.1:43125")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func syntheticManifest(t *testing.T, count int, withEvidence bool) acquisitionslack.ExternalManifest {
	t.Helper()
	scope := acquisition.SealSourceScope(acquisition.SourceScope{
		ConnectorKind: "external_slack_inventory", WorkspaceID: "T-synthetic", ChannelID: "C-synthetic",
		LowerInclusive: "1700000000.000001", UpperInclusive: "1700000009.000001", IncludeThreads: true, IncludeReplies: true,
		AttachmentPolicy: "metadata_only", PrivateFilePolicy: "manual", EditDeletePolicy: "account", AdapterVersion: acquisitionslack.ExternalInventorySchema,
	})
	manifest := acquisitionslack.ExternalManifest{
		DataClass:      acquisitionslack.DataClassSynthetic,
		SourceIdentity: acquisition.SourceIdentity{ConnectorKind: "external_slack_inventory", WorkspaceID: "T-synthetic", ChannelID: "C-synthetic"},
		SourceScope:    scope, Watermark: "1700000009.000001",
	}
	for index := 0; index < count; index++ {
		canonicalURL := "https://example.com/product-" + string(rune('a'+index))
		canonicalID := routing.CanonicalURLID(canonicalURL)
		sourceID := "source-" + string(rune('a'+index))
		occurrenceID := "occurrence-" + string(rune('a'+index))
		digest := sha256.Sum256([]byte(sourceID))
		manifest.SourceRecords = append(manifest.SourceRecords, acquisition.SourceRecord{SourceRecordID: sourceID, NativeMessageID: "message-" + string(rune('a'+index)), NativeTimestamp: "170000000" + string(rune('0'+index)) + ".000001", ContentFingerprint: hex.EncodeToString(digest[:]), URLOccurrenceIDs: []string{occurrenceID}, EditDeleteState: "original"})
		manifest.URLOccurrences = append(manifest.URLOccurrences, acquisition.URLOccurrence{URLOccurrenceID: occurrenceID, SourceRecordID: sourceID, ObservedURL: canonicalURL, CanonicalItemID: canonicalID})
		manifest.CanonicalItems = append(manifest.CanonicalItems, acquisition.InventoryItem{CanonicalItemID: canonicalID, CanonicalURL: canonicalURL, Kind: "article", URLOccurrenceIDs: []string{occurrenceID}, RetrievalStrategy: "generic", Format: "html"})
		if withEvidence {
			manifest.ImportedEvidence = append(manifest.ImportedEvidence, acquisition.ImportedEvidence{
				CanonicalItemID: canonicalID, CanonicalURL: canonicalURL, State: "complete", RetrievedAt: "2026-07-14T11:00:00Z",
				Metadata: acquisition.ImportedMetadata{Title: "Product strategy platform " + string(rune('A'+index)), Author: "Synthetic Author"},
				Excerpts: []acquisition.ImportedExcerpt{{ExcerptID: "excerpt-" + string(rune('a'+index)), Text: "A product strategy platform for teams.", Locator: "page"}},
			})
		}
	}
	manifest.Strata = []acquisition.StratumCount{{RetrievalStrategy: "generic", Format: "html", Count: count}}
	manifest.Completeness = []acquisition.EvidenceCheck{
		{Check: "source_record_denominator", Status: "pass", Count: count},
		{Check: "url_occurrence_denominator", Status: "pass", Count: count},
		{Check: "canonical_item_denominator", Status: "pass", Count: count},
		{Check: "bidirectional_occurrence_accounting", Status: "pass", Count: count},
		{Check: "sensitive_redacted_url_occurrences", Status: "pass", Count: 0},
		{Check: "non_semantic_url_sanitizations", Status: "pass", Count: 0},
	}
	return acquisitionslack.SealExternalManifest(manifest)
}

func reviewAllSelected(t *testing.T, app *App) {
	t.Helper()
	state, err := app.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range state.(View).Proof.Items {
		if _, err := executeHumanTest(app, "review_item", map[string]string{
			"item_id": item.CanonicalItemID, "decision": "accept", "role": item.ProposedRole,
			"disposition": item.ProposedDisposition, "rationale": "Founder confirmed the evidence-bound proposal.", "manual_support_outcome": map[bool]string{true: "queued_for_manual_processing", false: "not_required"}[item.RequiresManualReview],
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func withTestHuman(options Options) Options {
	return options
}

func testStrategyPayload(contextLenses, routingPolicy string) map[string]any {
	return map[string]any{
		"context_lenses": contextLenses, "routing_policy": routingPolicy,
		"maximum_network_requests": 5000, "maximum_wall_time_seconds": 14400,
		"maximum_cost_microunits": int64(1_000_000), "maximum_retry_attempts": 2000,
		"manual_support_tolerance": 250,
	}
}

func executeHumanTest(app *App, kind string, payload any) (any, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	switch kind {
	case "review_item":
		var decoded reviewItemPayload
		if err := decodePayload(payload, &decoded); err != nil {
			return nil, err
		}
		return app.reviewItemLocked(context.Background(), decoded)
	case "founder_review":
		var decoded founderReviewPayload
		if err := decodePayload(payload, &decoded); err != nil {
			return nil, err
		}
		return app.recordFounderReviewLocked(context.Background(), decoded)
	case "confirm_experimental_drain":
		aggregate, err := app.service.Get(context.Background(), app.state.RunID)
		if err != nil {
			return nil, err
		}
		verdict := app.readinessLocked(aggregate)
		if app.state.Plan == nil || app.state.Queue == nil || verdict.Verdict != orchestration.VerdictConditional {
			return nil, errors.New("test drain readiness is blocked")
		}
		aggregate, err = app.service.Execute(context.Background(), app.state.RunID, orchestration.Command{Kind: orchestration.CommandConfirmDrain, ExpectedVersion: aggregate.Version, Plan: app.state.Plan})
		if err != nil {
			return nil, err
		}
		confirmation := DrainConfirmation{SchemaVersion: DrainConfirmationSchema, RunPlanFingerprint: app.state.Plan.Fingerprint, QueueFingerprint: app.state.Queue.Fingerprint, SessionFingerprint: "test-session", NonceFingerprint: "test-nonce", ConfirmedAt: app.now().UTC().Format(time.RFC3339Nano)}
		confirmation.Fingerprint = acquisition.Fingerprint(confirmation)
		app.state.DrainConfirmation = &confirmation
		if err := app.commitAuthorityLocked(context.Background(), "drain_confirmation", confirmation.Fingerprint); err != nil {
			return nil, err
		}
		return map[string]any{"ready": true, "state": aggregate.State}, nil
	default:
		return nil, errors.New("unsupported test-only human command")
	}
}

type fakeConnector struct{ transport *memoryTransport }

func (connector fakeConnector) Connect(_ context.Context, _ *integrations.Registry, _ []byte) (*DestinationConnection, error) {
	capability, _ := connector.transport.ResolveWorkspace(context.Background())
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	snapshot := integrations.ConnectionSnapshot{ConnectionID: "conn-synthetic", Kind: integrations.ConnectionProductBrain, Identity: integrations.VerifiedIdentity{Provider: "product_brain", WorkspaceID: capability.ID, KeyID: capability.KeyID, CapabilityVersion: "aki/v0.2"}, CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}
	return &DestinationConnection{Snapshot: snapshot, Capability: capability, Transport: connector.transport, Disconnect: func() error { return nil }}, nil
}

type fakeSlackSourceConnector struct {
	client  acquisitionslack.WebAPIClient
	now     time.Time
	budgets acquisitionslack.SlackHTTPBudgets
}

func (connector *fakeSlackSourceConnector) Connect(_ context.Context, _ *integrations.Registry, _ []byte, channelID string, budgets acquisitionslack.SlackHTTPBudgets) (*SlackSourceConnection, error) {
	connector.budgets = budgets
	snapshot := integrations.ConnectionSnapshot{
		ConnectionID: "slack-" + connector.now.Format("150405"), Kind: integrations.ConnectionSlackWebAPI,
		Identity:  integrations.VerifiedIdentity{Provider: "slack", WorkspaceID: "T-PROOF", ChannelID: channelID, CapabilityVersion: acquisitionslack.WebAPIAdapterVersion},
		CreatedAt: connector.now, LastUsedAt: connector.now, IdleExpiresAt: connector.now.Add(time.Hour), AbsoluteExpiresAt: connector.now.Add(2 * time.Hour),
	}
	return &SlackSourceConnection{Snapshot: snapshot, Client: connector.client, Disconnect: func() error { return nil }}, nil
}

type activationSlackClient struct {
	workspace string
	history   map[string]acquisitionslack.WebAPIPage
	failures  map[string]int
	calls     map[string]int
}

func (client *activationSlackClient) Probe(context.Context) (string, error) {
	return client.workspace, nil
}

func (client *activationSlackClient) History(_ context.Context, channel, oldest, latest, cursor string, limit int) (acquisitionslack.WebAPIPage, error) {
	if channel != "CPROOF" || oldest != "100.000001" || latest != "199.000001" || limit != 200 {
		return acquisitionslack.WebAPIPage{}, errors.New("Slack test scope drift")
	}
	if client.calls == nil {
		client.calls = map[string]int{}
	}
	client.calls[cursor]++
	if client.failures[cursor] > 0 {
		client.failures[cursor]--
		return acquisitionslack.WebAPIPage{}, errors.New("synthetic Slack interruption")
	}
	return client.history[cursor], nil
}

func (client *activationSlackClient) Replies(context.Context, string, string, string, string, string, int) (acquisitionslack.WebAPIPage, error) {
	return acquisitionslack.WebAPIPage{}, errors.New("unexpected Slack replies call")
}

type memoryTransport struct {
	mu               sync.Mutex
	entries          map[string]productbrain.EntryReadback
	mutationCalls    int
	mutationFailures int
}

func newMemoryTransport() *memoryTransport {
	return &memoryTransport{entries: map[string]productbrain.EntryReadback{}}
}

func (transport *memoryTransport) ResolveWorkspace(context.Context) (productbrain.WorkspaceCapability, error) {
	return productbrain.WorkspaceCapability{ID: "ws-synthetic", Slug: "synthetic", GovernanceMode: "consensus", KeyScope: "readwrite", KeyID: "key-synthetic"}, nil
}

func (transport *memoryTransport) GetCollectionFields(_ context.Context, slug string) (productbrain.CollectionCapability, error) {
	if slug != "landscape" {
		return productbrain.CollectionCapability{Found: false}, nil
	}
	return productbrain.CollectionCapability{Found: true, Slug: slug, Fields: []productbrain.CollectionFieldCapability{{Key: "description", Type: "text"}, {Key: "url", Type: "string"}}}, nil
}

func (transport *memoryTransport) GetEntry(_ context.Context, id string) (productbrain.EntryReadback, error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	entry, ok := transport.entries[id]
	if !ok {
		return productbrain.EntryReadback{Found: false}, nil
	}
	return entry, nil
}

func (transport *memoryTransport) SearchEntries(_ context.Context, query, collection string) ([]productbrain.EntrySearchResult, error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	var result []productbrain.EntrySearchResult
	for _, entry := range transport.entries {
		if entry.Name == query && entry.CollectionSlug == collection {
			result = append(result, productbrain.EntrySearchResult{DocID: entry.DocID, EntryID: entry.EntryID, CollectionSlug: entry.CollectionSlug, Name: entry.Name, Status: entry.Status})
		}
	}
	return result, nil
}

func (transport *memoryTransport) CreateEntry(_ context.Context, request productbrain.CreateEntryRequest) (productbrain.CreateEntryResult, error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.mutationCalls++
	if transport.mutationFailures > 0 {
		transport.mutationFailures--
		return productbrain.CreateEntryResult{}, &productbrain.TransportError{Category: "validation_failed"}
	}
	if _, exists := transport.entries[request.EntryID]; exists {
		return productbrain.CreateEntryResult{}, &productbrain.TransportError{Category: "already_exists"}
	}
	transport.entries[request.EntryID] = productbrain.EntryReadback{Found: true, DocID: "doc-" + request.EntryID, EntryID: request.EntryID, CollectionSlug: request.CollectionSlug, Name: request.Name, Status: "draft", Data: request.Data, SourceRef: request.SourceRef, SourceExcerpt: request.SourceExcerpt, CreatedBy: request.CreatedBy}
	return productbrain.CreateEntryResult{EntryID: request.EntryID, Status: "draft"}, nil
}

func (*memoryTransport) ListEntryRelations(context.Context, string) ([]productbrain.RelationReadback, error) {
	return nil, nil
}

func (*memoryTransport) CreateEntryRelation(context.Context, productbrain.CreateRelationRequest) (productbrain.CreateRelationResult, error) {
	return productbrain.CreateRelationResult{}, errors.New("unexpected relation mutation")
}

func (*memoryTransport) RuntimeSecretFindings(any) []productbrain.PrivacyFinding { return nil }

var _ productbrain.ProductBrainTransport = (*memoryTransport)(nil)
var _ productbrain.RuntimeSecretScanner = (*memoryTransport)(nil)
