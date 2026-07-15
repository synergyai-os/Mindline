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

	"github.com/synergyai-os/Mindline/internal/processing"
)

type fakeApplication struct {
	lastCommand Command
	importData  []byte
	preview     BatchPreview
	approval    HumanApproval
	authority   *HumanAuthority
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
	server, err := New(app, Options{ExpectedHost: "127.0.0.1:43123", Origin: "http://127.0.0.1:43123", RuntimeRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	fragment := strings.TrimPrefix(server.BootstrapFragment(), "bootstrap=")
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43123/api/bootstrap", strings.NewReader("{}"))
	request.Host = "127.0.0.1:43123"
	request.RemoteAddr = "127.0.0.1:49100"
	request.Header.Set("Origin", "http://127.0.0.1:43123")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Mindline-Bootstrap", fragment)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap status %d: %s", response.Code, response.Body.String())
	}
	var values map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &values); err != nil {
		t.Fatal(err)
	}
	return testSession{server: server, app: app, session: values["session"], csrf: values["csrf"]}
}

func (s testSession) request(method, path, contentType string, body *bytes.Reader) *http.Request {
	request := httptest.NewRequest(method, "http://127.0.0.1:43123"+path, body)
	request.Host = "127.0.0.1:43123"
	request.RemoteAddr = "127.0.0.1:49100"
	request.Header.Set("Origin", "http://127.0.0.1:43123")
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
	for _, forbidden := range []string{"localStorage", "sessionStorage", "document.cookie", "innerHTML"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("browser asset contains forbidden capability/content primitive %q", forbidden)
		}
	}
	if !strings.Contains(body, "X-Mindline-Session") || !strings.Contains(body, "history.replaceState") {
		t.Fatal("browser bootstrap boundary missing")
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

func TestStrategyDefaultsIncludeContentNarrativeIntelligence(t *testing.T) {
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
	contextTag, contextDefault := servedTextarea(t, body, "context-lenses")
	lenses := processing.ContextLenses(processing.StrategySnapshot{ContextLenses: contextDefault})
	if len(lenses) != 3 || !strings.HasPrefix(lenses[0], "Product Brain landscape:") || !strings.HasPrefix(lenses[1], "AI-dominant organization design:") || !strings.HasPrefix(lenses[2], "Content and narrative intelligence:") {
		t.Fatalf("unexpected default lenses: %#v", lenses)
	}
	strategy := processing.SealStrategy(processing.StrategySnapshot{
		StrategyID: "served-default", Version: "1", ContextLenses: contextDefault,
		RoutingPolicy: "Validated separately below.",
	})
	if err := processing.ValidateStrategy(strategy); err != nil {
		t.Fatalf("served context-lens default is invalid: %v", err)
	}
	routingTag, routingDefault := servedTextarea(t, body, "routing-policy")
	if strings.Contains(contextTag+routingTag, "readonly") || strings.Contains(contextTag+routingTag, "disabled") {
		t.Fatal("strategy defaults must remain editable")
	}
	for _, expected := range []string{
		"Evaluate each source against every configured lens.",
		"one source may support multiple outcomes",
		"selected role as exhaustive",
		"an original angle",
		"Treat engagement and comments as signals, not truth.",
		"Never copy or fabricate.",
		"does not generate or publish finished content",
	} {
		if !strings.Contains(routingDefault, expected) {
			t.Fatalf("strategy default missing %q", expected)
		}
	}
	for _, expected := range []string{"less-technical audiences", "Product Brain Chain"} {
		if !strings.Contains(contextDefault, expected) {
			t.Fatalf("context-lens default missing %q", expected)
		}
	}
	if strings.Contains(routingDefault, "Product Brain") {
		t.Fatal("routing default must remain destination-neutral")
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
