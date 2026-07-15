package controlui

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/controlrun"
	"github.com/synergyai-os/Mindline/internal/controlsettings"
)

type fakeApplication struct {
	lastCommand Command
	importData  []byte
	preview     BatchPreview
	approval    HumanApproval
	authority   *HumanAuthority
	runAction   string
	runID       string
	runExpected controlrun.Revision
	saveResult  controlsettings.Snapshot
	saveErr     error
}

type fakePairingConfirmer struct{ challenge string }

func (f *fakePairingConfirmer) ConfirmPairing(_ context.Context, challenge string) error {
	f.challenge = challenge
	return nil
}

func (f *fakeApplication) ControlUIAuthority() *HumanAuthority { return f.authority }
func (f *fakeApplication) ConnectSlackSource(context.Context, []byte, string) (any, error) {
	return map[string]any{"connected": true}, nil
}
func (f *fakeApplication) DrainSlackSource(context.Context, string, string) (any, error) {
	return map[string]any{"accepted": true}, nil
}
func (f *fakeApplication) DisconnectSlackSource() (any, error) {
	return map[string]any{"disconnected": true}, nil
}

func (f *fakeApplication) State(context.Context) (any, error) {
	return map[string]any{"stage": "configured"}, nil
}
func (f *fakeApplication) Execute(_ context.Context, command Command) (any, error) {
	f.lastCommand = command
	return map[string]any{"ok": true}, nil
}
func (f *fakeApplication) ImportExternalInventory(_ context.Context, path, _ string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f.importData = append([]byte{}, data...)
	return map[string]any{"accepted": true}, nil
}
func (f *fakeApplication) PreviewBatch(context.Context, int, int) (BatchPreview, error) {
	return f.preview, nil
}
func (f *fakeApplication) ApproveBatch(_ context.Context, approval HumanApproval) (any, error) {
	f.approval = approval
	return map[string]any{"approved": true}, nil
}
func (f *fakeApplication) SaveSettings(_ context.Context, _ controlsettings.Revision, draft controlsettings.Draft) (controlsettings.Snapshot, error) {
	if f.saveErr != nil {
		return f.saveResult, f.saveErr
	}
	return controlsettings.Snapshot{State: controlsettings.StateSaved, Document: controlsettings.Document{SchemaVersion: controlsettings.SchemaVersion, Version: 1, Generation: strings.Repeat("a", 43), Fingerprint: "sha256:test", Draft: draft}}, nil
}
func (f *fakeApplication) CreateRun(_ context.Context, expected controlrun.Revision, _ controlsettings.Revision, _ string) (controlrun.Snapshot, error) {
	f.runAction, f.runExpected, f.runID = "create", expected, "run-20260715T143000Z-aaaaaaaaaaaaaaaaaaaaaaaaaa"
	return controlrun.Snapshot{State: controlrun.StateSelected, Document: controlrun.Document{Version: expected.Version + 1, SelectedRunID: f.runID}}, nil
}
func (f *fakeApplication) SelectRun(_ context.Context, expected controlrun.Revision, runID string) (controlrun.Snapshot, error) {
	f.runAction, f.runExpected, f.runID = "select", expected, runID
	return controlrun.Snapshot{State: controlrun.StateSelected, Document: controlrun.Document{Version: expected.Version + 1, SelectedRunID: runID}}, nil
}
func (f *fakeApplication) RecoverRunSelection(_ context.Context, _ string, expected *controlrun.Revision, _ string, runID string) (controlrun.Snapshot, error) {
	f.runAction, f.runID = "recover", runID
	if expected != nil {
		f.runExpected = *expected
	}
	return controlrun.Snapshot{State: controlrun.StateNone, Document: controlrun.Document{Version: 1}}, nil
}

type testSession struct {
	server  *Server
	app     *fakeApplication
	session string
	csrf    string
}

func newTestSession(t *testing.T) testSession {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := NewHumanAuthority()
	if err != nil {
		t.Fatal(err)
	}
	app := &fakeApplication{authority: authority, preview: BatchPreview{BatchFingerprint: "batch-1", DestinationWorkspaceID: "workspace-1", DestinationKeyID: "key-1", OperationFingerprints: []string{"operation-1"}, MaximumDestinationWrites: 1, MaximumMutationAttempts: 1, ExpiresAt: "2030-01-01T00:00:00Z"}}
	pairing := &fakePairingConfirmer{}
	server, err := New(app, Options{ExpectedHost: "127.0.0.1:43123", Origin: "http://127.0.0.1:43123", RuntimeRoot: root, Pairing: pairing})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43123/api/session/pair", strings.NewReader("{}"))
	request.Host = "127.0.0.1:43123"
	request.RemoteAddr = "127.0.0.1:49100"
	request.Header.Set("Origin", "http://127.0.0.1:43123")
	request.Header.Set("X-Mindline-Origin", "http://127.0.0.1:43123")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap status %d: %s", response.Code, response.Body.String())
	}
	decoder := json.NewDecoder(response.Body)
	var challenge map[string]any
	if err := decoder.Decode(&challenge); err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := decoder.Decode(&values); err != nil {
		t.Fatal(err)
	}
	if challenge["type"] != "challenge" || pairing.challenge == "" || values["type"] != "paired" {
		t.Fatalf("unexpected pairing frames: challenge=%v paired=%v", challenge, values)
	}
	return testSession{server: server, app: app, session: values["session"], csrf: values["csrf"]}
}

func (s testSession) request(method, path, contentType string, body *bytes.Reader) *http.Request {
	request := httptest.NewRequest(method, "http://127.0.0.1:43123"+path, body)
	request.Host = "127.0.0.1:43123"
	request.RemoteAddr = "127.0.0.1:49100"
	request.Header.Set("Origin", "http://127.0.0.1:43123")
	request.Header.Set("X-Mindline-Origin", "http://127.0.0.1:43123")
	request.Header.Set("X-Mindline-Session", s.session)
	request.Header.Set("X-Mindline-CSRF", s.csrf)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request
}

func TestServerBootstrapUsesHeadersAndNoCookies(t *testing.T) {
	session := newTestSession(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/app.js", nil)
	request.Host = "127.0.0.1:43123"
	request.RemoteAddr = "127.0.0.1:49100"
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if response.Header().Get("Set-Cookie") != "" {
		t.Fatal("loopback capability must not use a cookie")
	}
	body := response.Body.String()
	for _, forbidden := range []string{"localStorage", "document.cookie", "innerHTML", "indexedDB", "serviceWorker"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("browser asset contains forbidden capability/content primitive %q", forbidden)
		}
	}
	if !strings.Contains(body, "X-Mindline-Session") || !strings.Contains(body, "history.replaceState") || !strings.Contains(body, "sessionStorage") {
		t.Fatal("browser bootstrap boundary missing")
	}
}

func TestWP46_ControlUIRunRoutesAreExplicitAndCASBounded(t *testing.T) {
	session := newTestSession(t)
	generation := strings.Repeat("a", 43)
	settingsGeneration := strings.Repeat("b", 43)
	createBody, _ := json.Marshal(map[string]any{
		"expected_selection_version": 0, "expected_selection_generation": generation,
		"settings_version": 1, "settings_generation": settingsGeneration, "settings_fingerprint": "sha256:settings",
	})
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, session.request(http.MethodPost, "/api/runs", "application/json", bytes.NewReader(createBody)))
	if response.Code != http.StatusCreated || session.app.runAction != "create" || session.app.runExpected.Generation != generation {
		t.Fatalf("create route = %d action=%s expected=%+v body=%s", response.Code, session.app.runAction, session.app.runExpected, response.Body.String())
	}
	runID := "run-20260715T143000Z-aaaaaaaaaaaaaaaaaaaaaaaaaa"
	selectBody, _ := json.Marshal(map[string]any{"expected_version": 1, "expected_generation": generation, "run_id": runID})
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, session.request(http.MethodPost, "/api/runs/select", "application/json", bytes.NewReader(selectBody)))
	if response.Code != http.StatusOK || session.app.runAction != "select" || session.app.runID != runID || session.app.runExpected.Version != 1 {
		t.Fatalf("select route = %d action=%s run=%s expected=%+v", response.Code, session.app.runAction, session.app.runID, session.app.runExpected)
	}
	readableVersion := uint64(2)
	recoveryBody, _ := json.Marshal(map[string]any{
		"problem_fingerprint": "sha256:problem", "expected_version": readableVersion,
		"expected_generation": generation, "acknowledgement": controlrun.RecoveryAcknowledgement, "run_id": "",
	})
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, session.request(http.MethodPost, "/api/runs/recover-selection", "application/json", bytes.NewReader(recoveryBody)))
	if response.Code != http.StatusOK || session.app.runAction != "recover" || session.app.runID != "" || session.app.runExpected.Version != readableVersion {
		t.Fatalf("recovery route = %d action=%s run=%s expected=%+v body=%s", response.Code, session.app.runAction, session.app.runID, session.app.runExpected, response.Body.String())
	}
	duplicateBody := `{"expected_version":1,"expected_version":2,"expected_generation":"` + generation + `","run_id":"` + runID + `"}`
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, session.request(http.MethodPost, "/api/runs/select", "application/json", bytes.NewReader([]byte(duplicateBody))))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duplicate run CAS field status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestWP46_SafeShellAssetsExposeGateMissingAndKeepRunRecovery(t *testing.T) {
	session := newTestSession(t)
	for _, asset := range []struct {
		path    string
		markers []string
	}{
		{path: "/", markers: []string{`id="gate-status"`, `id="create-run"`, `id="run-recovery"`, `id="recover-run-clear"`}},
		{path: "/app.js", markers: []string{"gate_missing", `data.pre_live_ready`, `"/api/runs/recover-selection"`}},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123"+asset.path, nil)
		request.Host = "127.0.0.1:43123"
		request.RemoteAddr = "127.0.0.1:49100"
		response := httptest.NewRecorder()
		session.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("asset %s = %d", asset.path, response.Code)
		}
		for _, marker := range asset.markers {
			if !strings.Contains(response.Body.String(), marker) {
				t.Fatalf("asset %s missing %q", asset.path, marker)
			}
		}
	}
}

func TestWP46_SettingsConflictReturnsExactCurrentWithoutOverwritingDirtyDraft(t *testing.T) {
	session := newTestSession(t)
	currentDraft := controlsettings.DefaultDraft()
	currentDraft.RoutingPolicy = "Authoritative current routing policy."
	session.app.saveResult = controlsettings.Snapshot{State: controlsettings.StateSaved, Document: controlsettings.Document{
		SchemaVersion: controlsettings.SchemaVersion, Version: 7, Generation: strings.Repeat("c", 43),
		SavedAt: "2026-07-15T14:30:00Z", Fingerprint: "sha256:current", Draft: currentDraft,
	}}
	session.app.saveErr = controlsettings.ErrConflict
	dirtyDraft := controlsettings.DefaultDraft()
	dirtyDraft.RoutingPolicy = "Unsaved founder edit that must remain in the browser."
	body, _ := json.Marshal(map[string]any{"expected_version": 6, "expected_generation": strings.Repeat("b", 43), "draft": dirtyDraft})
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, session.request(http.MethodPut, "/api/settings", "application/json", bytes.NewReader(body)))
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		ErrorCode string                   `json:"error_code"`
		Changed   string                   `json:"changed"`
		Current   controlsettings.Document `json:"current"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ErrorCode != "settings_version_conflict" || payload.Changed != "none" || payload.Current.Version != 7 || payload.Current.Generation != strings.Repeat("c", 43) || payload.Current.Draft.RoutingPolicy != currentDraft.RoutingPolicy {
		t.Fatalf("unsafe or incomplete conflict projection: %+v", payload)
	}
	assetRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/app.js", nil)
	assetRequest.Host = "127.0.0.1:43123"
	assetRequest.RemoteAddr = "127.0.0.1:49100"
	assetResponse := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(assetResponse, assetRequest)
	for _, marker := range []string{"const localDraft = collectSettingsDraft();", "settingsBaseline = JSON.parse(encoded);", "Your edits are still here"} {
		if !strings.Contains(assetResponse.Body.String(), marker) {
			t.Fatalf("dirty-edit retention marker missing: %q", marker)
		}
	}
}

func TestFounderTruthAssetsUseVerdictSafeAuthorityAndRequiredJudgments(t *testing.T) {
	session := newTestSession(t)
	for _, asset := range []struct {
		path     string
		required []string
	}{
		{path: "/app.js", required: []string{"Unauthorized while blockers remain.", "Unauthorized until every named condition passes.", `stage.verdict === "READY"`}},
		{path: "/", required: []string{`id="slack-key"`, `id="slack-channel"`, `id="slack-latest"`, `id="destination-key"`, `id="context-lenses"`, `id="usefulness" required`, `id="manual-burden" required`, "enter N/A explicitly if none was needed"}},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123"+asset.path, nil)
		request.Host = "127.0.0.1:43123"
		request.RemoteAddr = "127.0.0.1:49100"
		response := httptest.NewRecorder()
		session.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("asset %s status %d", asset.path, response.Code)
		}
		for _, expected := range asset.required {
			if !strings.Contains(response.Body.String(), expected) {
				t.Fatalf("asset %s missing founder-truth marker %q", asset.path, expected)
			}
		}
	}
}

func TestStrategyDefaultsAreServerOwnedNotEmbeddedInHTML(t *testing.T) {
	session := newTestSession(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/", nil)
	request.Host = "127.0.0.1:43123"
	request.RemoteAddr = "127.0.0.1:49100"
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"Product Brain landscape:", "AI-dominant organization design:", "Content and narrative intelligence:", "value=\"5000\"", "value=\"14400\""} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("HTML embeds authoritative setting %q", forbidden)
		}
	}
	for _, required := range []string{`id="context-lenses"`, `id="routing-policy"`, `id="settings-status"`, `id="save-settings"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("HTML missing settings control %q", required)
		}
	}
}

func servedTextarea(t *testing.T, body, id string) (string, string) {
	t.Helper()
	marker := `id="` + id + `"`
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatalf("textarea %q missing", id)
	}
	tagEnd := strings.Index(body[start:], ">")
	if tagEnd < 0 {
		t.Fatalf("textarea %q opening tag is incomplete", id)
	}
	tagEnd += start
	valueStart := tagEnd + 1
	valueEnd := strings.Index(body[valueStart:], "</textarea>")
	if valueEnd < 0 {
		t.Fatalf("textarea %q closing tag is missing", id)
	}
	valueEnd += valueStart
	return body[start : tagEnd+1], html.UnescapeString(body[valueStart:valueEnd])
}

func TestFounderReviewRequiresReasonAndAllBurdenJudgments(t *testing.T) {
	session := newTestSession(t)
	valid := map[string]any{
		"receipt_fingerprint": "receipt-1", "useful_draft_ids": []string{}, "value_verdict": "not_useful",
		"usefulness_reason": "The acknowledged draft did not yet improve the decision.",
		"credential_burden": "Acceptable for one session.", "manual_support_burden": "N/A - no manual support was needed.",
		"approval_burden": "Acceptable for one exact batch.", "zero_draft": false,
	}
	for _, field := range []string{"usefulness_reason", "credential_burden", "manual_support_burden", "approval_burden"} {
		payload := make(map[string]any, len(valid))
		for key, value := range valid {
			payload[key] = value
		}
		payload[field] = "  "
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		request := session.request(http.MethodPost, "/api/commands/founder-review", "application/json", bytes.NewReader(body))
		response := httptest.NewRecorder()
		session.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("blank %s accepted with status %d", field, response.Code)
		}
	}
	body, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	request := session.request(http.MethodPost, "/api/commands/founder-review", "application/json", bytes.NewReader(body))
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || session.app.lastCommand.Kind != "founder_review" {
		t.Fatalf("complete founder review rejected: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestServerRejectsHostOriginSessionAndUnknownJSON(t *testing.T) {
	session := newTestSession(t)
	tests := []struct {
		name   string
		mutate func(*http.Request)
		body   string
	}{
		{name: "host", mutate: func(r *http.Request) { r.Host = "localhost:43123" }, body: "{}"},
		{name: "origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:9999") }, body: "{}"},
		{name: "session", mutate: func(r *http.Request) { r.Header.Set("X-Mindline-Session", "wrong") }, body: "{}"},
		{name: "csrf", mutate: func(r *http.Request) { r.Header.Set("X-Mindline-CSRF", "wrong") }, body: "{}"},
		{name: "unknown field", mutate: func(*http.Request) {}, body: `{"unexpected":true}`},
		{name: "duplicate field", mutate: func(*http.Request) {}, body: `{"unexpected":true,"unexpected":false}`},
		{name: "trailing JSON", mutate: func(*http.Request) {}, body: `{} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := session.request(http.MethodPost, "/api/commands/freeze-inventory", "application/json", bytes.NewReader([]byte(test.body)))
			test.mutate(request)
			response := httptest.NewRecorder()
			session.server.Handler().ServeHTTP(response, request)
			if response.Code < 400 {
				t.Fatalf("expected rejection, got %d", response.Code)
			}
		})
	}
}

func TestServerRejectsNonLoopbackAndCorsPreflight(t *testing.T) {
	session := newTestSession(t)
	request := session.request(http.MethodPost, "/api/commands/freeze-inventory", "application/json", bytes.NewReader([]byte("{}")))
	request.RemoteAddr = "10.0.0.2:50000"
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status %d", response.Code)
	}
	request = session.request(http.MethodOptions, "/api/commands/freeze-inventory", "", bytes.NewReader(nil))
	request.Header.Set("Access-Control-Request-Method", "POST")
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("CORS preflight was not blocked: %d", response.Code)
	}
}

func TestServerMultipartImportIsSinglePartAndQuarantineIsRemoved(t *testing.T) {
	session := newTestSession(t)
	importOnce := func() {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("manifest", "private-name.json")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(`{"schema_version":"external_slack_inventory/v1"}`))
		_ = writer.Close()
		request := session.request(http.MethodPost, "/api/import/external-slack", writer.FormDataContentType(), bytes.NewReader(body.Bytes()))
		response := httptest.NewRecorder()
		session.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("import status %d: %s", response.Code, response.Body.String())
		}
	}
	importOnce()
	if session.server.retries != 0 {
		t.Fatalf("first successful import was falsely counted as a retry: %d", session.server.retries)
	}
	if string(session.app.importData) != `{"schema_version":"external_slack_inventory/v1"}` {
		t.Fatal("import application did not receive exact bytes")
	}
	entries, err := os.ReadDir(session.server.runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("quarantine artifact remained: %v", entries)
	}
	importOnce()
	if session.server.retries != 1 {
		t.Fatalf("second import was not counted as exactly one retry: %d", session.server.retries)
	}
}

func TestServerOneTimeHumanApprovalCeremony(t *testing.T) {
	session := newTestSession(t)
	request := session.request(http.MethodPost, "/api/commands/preview-batch", "application/json", bytes.NewReader([]byte(`{"maximum_destination_writes":1,"maximum_mutation_attempts":1}`)))
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status %d: %s", response.Code, response.Body.String())
	}
	var preview struct {
		Nonce string `json:"review_nonce"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	approveBody := []byte(`{"review_nonce":"` + preview.Nonce + `"}`)
	request = session.request(http.MethodPost, "/api/commands/approve-batch", "application/json", bytes.NewReader(approveBody))
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !session.server.humanAuthority.VerifyAndConsumeApproval(session.app.approval) {
		t.Fatalf("approval not sealed: %d %s", response.Code, response.Body.String())
	}
	request = session.request(http.MethodPost, "/api/commands/approve-batch", "application/json", bytes.NewReader(approveBody))
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code < 400 {
		t.Fatal("review nonce replay was accepted")
	}
}

func TestServerInvalidatesNonceWhenExactPreviewAuthorityChanges(t *testing.T) {
	session := newTestSession(t)
	request := session.request(http.MethodPost, "/api/commands/preview-batch", "application/json", bytes.NewReader([]byte(`{"maximum_destination_writes":1,"maximum_mutation_attempts":1}`)))
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	var first struct {
		Nonce string `json:"review_nonce"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &first) != nil {
		t.Fatalf("first preview failed: %d %s", response.Code, response.Body.String())
	}
	session.app.preview.MaximumMutationAttempts = 2
	session.app.preview.ExpiresAt = "2030-01-01T00:05:00Z"
	request = session.request(http.MethodPost, "/api/commands/preview-batch", "application/json", bytes.NewReader([]byte(`{"maximum_destination_writes":1,"maximum_mutation_attempts":2}`)))
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("second preview failed: %d %s", response.Code, response.Body.String())
	}
	request = session.request(http.MethodPost, "/api/commands/approve-batch", "application/json", bytes.NewReader([]byte(`{"review_nonce":"`+first.Nonce+`"}`)))
	response = httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code < 400 {
		t.Fatal("stale nonce authorized changed retry/expiry authority")
	}
}

func TestServerCredentialIsNotReflectedAndPayloadIsCleared(t *testing.T) {
	session := newTestSession(t)
	secret := "pb_sk_SENTINEL_NOT_REAL_123456789"
	request := session.request(http.MethodPost, "/api/commands/connect-destination", "application/json", bytes.NewReader([]byte(`{"credential":"`+secret+`"}`)))
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), secret) {
		t.Fatalf("credential reflected: %d %s", response.Code, response.Body.String())
	}
	payload, ok := session.app.lastCommand.Payload.([]byte)
	if !ok {
		t.Fatal("credential payload type missing")
	}
	for _, value := range payload {
		if value != 0 {
			t.Fatal("credential payload remains usable after command")
		}
	}
}

func TestServerSecurityHeaders(t *testing.T) {
	session := newTestSession(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43123/", nil)
	request.Host = "127.0.0.1:43123"
	request.RemoteAddr = "127.0.0.1:49100"
	response := httptest.NewRecorder()
	session.server.Handler().ServeHTTP(response, request)
	for _, header := range []string{"Content-Security-Policy", "Cache-Control", "X-Content-Type-Options", "Referrer-Policy", "Permissions-Policy"} {
		if response.Header().Get(header) == "" {
			t.Fatalf("missing security header %s", header)
		}
	}
}
