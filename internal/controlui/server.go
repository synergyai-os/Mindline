package controlui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/synergyai-os/Mindline/internal/controlrun"
	"github.com/synergyai-os/Mindline/internal/controlsettings"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	maxJSONBytes   = 64 << 10
	maxImportBytes = 64 << 20
)

//go:embed assets/index.html assets/app.js assets/style.css
var assets embed.FS

var (
	ErrUnauthorized          = errors.New("unauthorized")
	ErrInvalidInput          = errors.New("invalid_input")
	ErrPairingInputMalformed = errors.New("pairing input malformed")
)

type Command struct {
	Kind        string
	Payload     any
	HumanAction *HumanAction
}

type BatchPreview struct {
	BatchFingerprint         string                  `json:"batch_fingerprint"`
	OutboxFingerprint        string                  `json:"outbox_fingerprint"`
	PreflightFingerprint     string                  `json:"preflight_fingerprint,omitempty"`
	PrivacyFingerprint       string                  `json:"privacy_fingerprint"`
	DestinationWorkspaceID   string                  `json:"destination_workspace_id"`
	DestinationKeyID         string                  `json:"destination_key_id"`
	OperationFingerprints    []string                `json:"operation_fingerprints"`
	MaximumDestinationWrites int                     `json:"maximum_destination_writes"`
	MaximumMutationAttempts  int                     `json:"maximum_mutation_attempts"`
	OperationCount           int                     `json:"operation_count"`
	EntryOperationCount      int                     `json:"entry_operation_count"`
	RelationOperationCount   int                     `json:"relation_operation_count"`
	PrivacyFindingCount      int                     `json:"privacy_finding_count"`
	DraftOnly                bool                    `json:"draft_only"`
	OperatorJudged           bool                    `json:"operator_judged"`
	TypeDistribution         map[string]int          `json:"type_distribution"`
	Operations               []BatchOperationPreview `json:"operations"`
	PreflightGates           []BatchGatePreview      `json:"preflight_gates,omitempty"`
	ExpiresAt                string                  `json:"expires_at"`
}

type BatchOperationPreview struct {
	OperationID        string         `json:"operation_id"`
	Kind               string         `json:"kind"`
	PayloadFingerprint string         `json:"payload_fingerprint"`
	Dependencies       []string       `json:"dependencies"`
	CollectionSlug     string         `json:"collection_slug,omitempty"`
	EntryID            string         `json:"entry_id,omitempty"`
	Name               string         `json:"name,omitempty"`
	SourceRef          string         `json:"source_ref,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
	RelationIdentity   string         `json:"relation_identity,omitempty"`
	RelationType       string         `json:"relation_type,omitempty"`
	FromEntryID        string         `json:"from_entry_id,omitempty"`
	ToEntryID          string         `json:"to_entry_id,omitempty"`
}

type BatchGatePreview struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
	Actual  string `json:"actual,omitempty"`
}

type DiscoveryMetrics struct {
	StartedAt                string `json:"started_at"`
	SubmittedAt              string `json:"submitted_at"`
	ElapsedMilliseconds      int64  `json:"elapsed_milliseconds"`
	Errors                   int    `json:"errors"`
	Retries                  int    `json:"retries"`
	Backtracks               int    `json:"backtracks"`
	HelpRequests             int    `json:"help_requests"`
	TimeToTrustedValueMillis *int64 `json:"time_to_trusted_value_milliseconds,omitempty"`
}

type HumanApproval struct {
	Preview               BatchPreview
	InitiationFingerprint string
	SessionFingerprint    string
	GestureRecordedAt     string
	seal                  [32]byte
}

func (a HumanApproval) ValidFor(preview BatchPreview, authority *HumanAuthority) bool {
	return authority != nil && previewEqual(a.Preview, preview) && authority.VerifyAndConsumeApproval(a)
}

func previewEqual(left, right BatchPreview) bool {
	leftData, leftErr := json.Marshal(left)
	rightData, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && len(leftData) == len(rightData) && subtle.ConstantTimeCompare(leftData, rightData) == 1
}

type Application interface {
	ControlUIAuthority() *HumanAuthority
	ConnectSlackSource(context.Context, []byte, string) (any, error)
	DrainSlackSource(context.Context, string, string) (any, error)
	DisconnectSlackSource() (any, error)
	State(context.Context) (any, error)
	Execute(context.Context, Command) (any, error)
	ImportExternalInventory(context.Context, string, string) (any, error)
	PreviewBatch(context.Context, int, int) (BatchPreview, error)
	ApproveBatch(context.Context, HumanApproval) (any, error)
	SaveSettings(context.Context, controlsettings.Revision, controlsettings.Draft) (controlsettings.Snapshot, error)
}

type RunApplication interface {
	CreateRun(context.Context, controlrun.Revision, controlsettings.Revision, string) (controlrun.Snapshot, error)
	SelectRun(context.Context, controlrun.Revision, string) (controlrun.Snapshot, error)
	RecoverRunSelection(context.Context, string, *controlrun.Revision, string, string) (controlrun.Snapshot, error)
}

type Options struct {
	ExpectedHost string
	Origin       string
	RuntimeRoot  string
	Now          func() time.Time
	Pairing      PairingConfirmer
}

// PairingConfirmer is the narrow operator-channel port. Production supplies
// the verified anonymous launcher pipe; tests may inject a deterministic
// confirmer. The challenge is non-authorizing until this call succeeds.
type PairingConfirmer interface {
	ConfirmPairing(context.Context, string) error
}

type Server struct {
	app          Application
	expectedHost string
	origin       string
	runtimeRoot  string
	now          func() time.Time

	mu             sync.Mutex
	session        string
	csrf           string
	serverInstance string
	pairing        PairingConfirmer
	pairingPending bool
	pairingBlocked bool
	pairingStarts  []time.Time
	humanAuthority *HumanAuthority
	reviewNonces   map[string]reviewNonce
	startedAt      time.Time
	actionCounts   map[string]int
	errors         int
	retries        int
	backtracks     int
	helpRequests   int
	furthestStage  int
}

type reviewNonce struct {
	Preview   BatchPreview
	Session   string
	ExpiresAt time.Time
}

func New(app Application, options Options) (*Server, error) {
	if app == nil || strings.TrimSpace(options.ExpectedHost) == "" || strings.TrimSpace(options.Origin) == "" {
		return nil, errors.New("incomplete control UI configuration")
	}
	if options.Origin != "http://"+options.ExpectedHost {
		return nil, errors.New("control UI origin must match listener host")
	}
	if strings.TrimSpace(options.RuntimeRoot) == "" {
		return nil, errors.New("missing private runtime root")
	}
	if err := privateio.ValidateContained(options.RuntimeRoot, options.RuntimeRoot); err != nil {
		return nil, err
	}
	authority := app.ControlUIAuthority()
	if authority == nil {
		return nil, errors.New("control UI human authority unavailable")
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	instance, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	return &Server{app: app, expectedHost: options.ExpectedHost, origin: options.Origin, runtimeRoot: options.RuntimeRoot, now: now, pairing: options.Pairing, serverInstance: instance, humanAuthority: authority, reviewNonces: map[string]reviewNonce{}, startedAt: now().UTC(), actionCounts: map[string]int{}}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleAsset("assets/index.html", "text/html; charset=utf-8"))
	mux.HandleFunc("GET /app.js", s.handleAsset("assets/app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("GET /style.css", s.handleAsset("assets/style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("POST /api/session/pair", s.handlePair)
	mux.HandleFunc("POST /api/session/lock", s.handleLock)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("PUT /api/settings", s.handleSaveSettings)
	mux.HandleFunc("POST /api/runs", s.handleCreateRun)
	mux.HandleFunc("POST /api/runs/select", s.handleSelectRun)
	mux.HandleFunc("POST /api/runs/recover-selection", s.handleRecoverRunSelection)
	mux.HandleFunc("POST /api/import/external-slack", s.handleImport)
	mux.HandleFunc("POST /api/commands/connect-slack-source", s.handleConnectSlackSource)
	mux.HandleFunc("POST /api/commands/drain-slack-source", s.handleDrainSlackSource)
	mux.HandleFunc("POST /api/commands/disconnect-slack-source", s.handleDisconnectSlackSource)
	mux.HandleFunc("POST /api/commands/connect-destination", s.handleConnectDestination)
	mux.HandleFunc("POST /api/commands/use-settings-for-proof", s.handleUseSettingsForProof)
	mux.HandleFunc("POST /api/commands/freeze-inventory", s.handleEmptyCommand("freeze_inventory"))
	mux.HandleFunc("POST /api/commands/start-proof", s.handleEmptyCommand("start_proof"))
	mux.HandleFunc("POST /api/commands/review-item", s.handleReviewItem)
	mux.HandleFunc("POST /api/commands/preview-batch", s.handlePreviewBatch)
	mux.HandleFunc("POST /api/commands/approve-batch", s.handleApproveBatch)
	mux.HandleFunc("POST /api/commands/resume-delivery", s.handleEmptyCommand("resume_delivery"))
	mux.HandleFunc("POST /api/commands/founder-review", s.handleFounderReview)
	mux.HandleFunc("POST /api/commands/confirm-drain", s.handleEmptyCommand("confirm_experimental_drain"))
	mux.HandleFunc("POST /api/commands/cancel", s.handleCancel)
	mux.HandleFunc("POST /api/commands/disconnect", s.handleEmptyCommand("disconnect"))
	mux.HandleFunc("POST /api/discovery/help", s.handleHelp)
	return s.securityHeaders(s.requestBoundary(mux))
}

type connectSlackSourceRequest struct {
	Credential string `json:"credential"`
	ChannelID  string `json:"channel_id"`
}

func (s *Server) handleConnectSlackSource(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	s.recordAction("connect_slack_source")
	var request connectSlackSourceRequest
	if decodeStrictJSON(w, r, &request, maxJSONBytes) != nil {
		return
	}
	if len(request.Credential) < 16 || strings.TrimSpace(request.ChannelID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	credential := []byte(request.Credential)
	request.Credential = ""
	result, err := s.app.ConnectSlackSource(r.Context(), credential, strings.TrimSpace(request.ChannelID))
	for index := range credential {
		credential[index] = 0
	}
	if err != nil {
		s.recordFailure()
		writeError(w, http.StatusUnprocessableEntity, safeCategory(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type drainSlackSourceRequest struct {
	Oldest string `json:"oldest"`
	Latest string `json:"latest"`
}

func (s *Server) handleDrainSlackSource(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	s.recordAction("drain_slack_source")
	var request drainSlackSourceRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	result, err := s.app.DrainSlackSource(r.Context(), request.Oldest, request.Latest)
	if err != nil {
		s.recordFailure()
		writeError(w, http.StatusConflict, safeCategory(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDisconnectSlackSource(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	if decodeStrictJSON(w, r, &struct{}{}, 256) != nil {
		return
	}
	s.recordAction("disconnect_slack_source")
	result, err := s.app.DisconnectSlackSource()
	if err != nil {
		s.recordFailure()
		writeError(w, http.StatusConflict, safeCategory(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func HTTPServer(listener net.Listener, handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

func (s *Server) handleAsset(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := assets.ReadFile(path)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(data)
	}
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if !s.requireOrigin(r) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if !isJSON(r.Header.Get("Content-Type")) {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_input")
		return
	}
	if decodeStrictJSON(w, r, &struct{}{}, 256) != nil {
		return
	}
	if s.pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "pairing_channel_unavailable")
		return
	}
	now := s.now()
	s.mu.Lock()
	retained := s.pairingStarts[:0]
	for _, started := range s.pairingStarts {
		if now.Sub(started) < time.Minute {
			retained = append(retained, started)
		}
	}
	s.pairingStarts = retained
	if s.pairingBlocked || s.pairingPending || len(s.pairingStarts) >= 3 {
		s.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "pairing_channel_unavailable")
		return
	}
	s.pairingStarts = append(s.pairingStarts, now)
	s.pairingPending = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.pairingPending = false
		s.mu.Unlock()
	}()

	challenge, err := randomToken(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "local_state_failure")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "challenge", "challenge": challenge, "expires_in_seconds": 300})
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
	pairContext, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := s.pairing.ConfirmPairing(pairContext, challenge); err != nil {
		if errors.Is(err, ErrPairingInputMalformed) {
			s.mu.Lock()
			s.pairingBlocked = true
			s.mu.Unlock()
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"type": "error", "error_code": "pairing_expired"})
		flusher.Flush()
		return
	}
	session, err := randomToken(32)
	if err != nil {
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.session = session
	s.csrf = csrf
	s.reviewNonces = map[string]reviewNonce{}
	instance := s.serverInstance
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"type": "paired", "session": session, "csrf": csrf, "server_instance": instance})
	flusher.Flush()
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	if decodeStrictJSON(w, r, &struct{}{}, 256) != nil {
		return
	}
	s.mu.Lock()
	s.session = ""
	s.csrf = ""
	s.reviewNonces = map[string]reviewNonce{}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"changed": "browser_session_revoked", "provider_leases_revoked": false})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	state, err := s.app.State(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, safeCategory(err))
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type saveSettingsRequest struct {
	ExpectedVersion    uint64                `json:"expected_version"`
	ExpectedGeneration string                `json:"expected_generation"`
	Draft              controlsettings.Draft `json:"draft"`
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request saveSettingsRequest
	if decodeStrictJSON(w, r, &request, maxJSONBytes) != nil {
		return
	}
	result, err := s.app.SaveSettings(r.Context(), controlsettings.Revision{Version: request.ExpectedVersion, Generation: request.ExpectedGeneration}, request.Draft)
	if err != nil {
		s.recordFailure()
		if errors.Is(err, controlsettings.ErrConflict) {
			writeSettingsConflict(w, result.Document)
			return
		}
		if errors.Is(err, controlsettings.ErrRecoveryRequired) {
			writeError(w, http.StatusConflict, "settings_corrupt")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": result, "changed": "settings_saved"})
}

type useSettingsForProofRequest struct {
	SettingsVersion     uint64 `json:"settings_version"`
	SettingsGeneration  string `json:"settings_generation"`
	SettingsFingerprint string `json:"settings_fingerprint"`
}

type createRunRequest struct {
	ExpectedSelectionVersion    uint64 `json:"expected_selection_version"`
	ExpectedSelectionGeneration string `json:"expected_selection_generation"`
	SettingsVersion             uint64 `json:"settings_version"`
	SettingsGeneration          string `json:"settings_generation"`
	SettingsFingerprint         string `json:"settings_fingerprint"`
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request createRunRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	if strings.TrimSpace(request.ExpectedSelectionGeneration) == "" || request.SettingsVersion == 0 || strings.TrimSpace(request.SettingsGeneration) == "" || strings.TrimSpace(request.SettingsFingerprint) == "" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	application, ok := s.app.(RunApplication)
	if !ok {
		writeError(w, http.StatusNotImplemented, "run_control_unavailable")
		return
	}
	result, err := application.CreateRun(r.Context(),
		controlrun.Revision{Version: request.ExpectedSelectionVersion, Generation: request.ExpectedSelectionGeneration},
		controlsettings.Revision{Version: request.SettingsVersion, Generation: request.SettingsGeneration},
		request.SettingsFingerprint,
	)
	if err != nil {
		s.writeRunError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"run_selection": result, "changed": "run_created_and_selected"})
}

type selectRunRequest struct {
	ExpectedVersion    uint64 `json:"expected_version"`
	ExpectedGeneration string `json:"expected_generation"`
	RunID              string `json:"run_id"`
}

func (s *Server) handleSelectRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request selectRunRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	if strings.TrimSpace(request.ExpectedGeneration) == "" || strings.TrimSpace(request.RunID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	application, ok := s.app.(RunApplication)
	if !ok {
		writeError(w, http.StatusNotImplemented, "run_control_unavailable")
		return
	}
	result, err := application.SelectRun(r.Context(), controlrun.Revision{Version: request.ExpectedVersion, Generation: request.ExpectedGeneration}, request.RunID)
	if err != nil {
		s.writeRunError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_selection": result, "changed": "run_selected"})
}

type recoverRunSelectionRequest struct {
	ProblemFingerprint string  `json:"problem_fingerprint"`
	ExpectedVersion    *uint64 `json:"expected_version"`
	ExpectedGeneration string  `json:"expected_generation"`
	Acknowledgement    string  `json:"acknowledgement"`
	RunID              string  `json:"run_id"`
}

func (s *Server) handleRecoverRunSelection(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request recoverRunSelectionRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	if strings.TrimSpace(request.ProblemFingerprint) == "" || request.Acknowledgement != controlrun.RecoveryAcknowledgement || (request.ExpectedVersion == nil) != (request.ExpectedGeneration == "") {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	var expected *controlrun.Revision
	if request.ExpectedVersion != nil {
		revision := controlrun.Revision{Version: *request.ExpectedVersion, Generation: request.ExpectedGeneration}
		expected = &revision
	}
	application, ok := s.app.(RunApplication)
	if !ok {
		writeError(w, http.StatusNotImplemented, "run_control_unavailable")
		return
	}
	result, err := application.RecoverRunSelection(r.Context(), request.ProblemFingerprint, expected, request.Acknowledgement, request.RunID)
	if err != nil {
		s.writeRunError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_selection": result, "changed": "run_selection_recovered"})
}

func (s *Server) writeRunError(w http.ResponseWriter, err error) {
	s.recordFailure()
	switch {
	case errors.Is(err, controlrun.ErrConflict), errors.Is(err, controlsettings.ErrConflict):
		writeError(w, http.StatusConflict, "version_conflict")
	case errors.Is(err, controlrun.ErrRecoveryRequired):
		writeError(w, http.StatusConflict, "run_selection_recovery_required")
	case errors.Is(err, controlrun.ErrInvalid), errors.Is(err, controlrun.ErrRunNotFound):
		writeError(w, http.StatusBadRequest, "invalid_input")
	default:
		writeError(w, http.StatusConflict, safeCategory(err))
	}
}

func (s *Server) handleUseSettingsForProof(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request useSettingsForProofRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	if request.SettingsVersion == 0 || strings.TrimSpace(request.SettingsGeneration) == "" || strings.TrimSpace(request.SettingsFingerprint) == "" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	s.execute(w, r, Command{Kind: "use_settings_for_proof", Payload: request})
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, false) {
		return
	}
	s.recordAction("import_inventory")
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || strings.TrimSpace(params["boundary"]) == "" {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_input")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+4096)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "manifest" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	quarantine := filepath.Join(s.runtimeRoot, fmt.Sprintf("import-quarantine-%d.json", s.now().UnixNano()))
	file, err := os.OpenFile(quarantine, os.O_CREATE|os.O_EXCL|os.O_WRONLY, privateio.FileMode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "local_state_failure")
		return
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(quarantine)
		}
	}()
	written, copyErr := io.Copy(file, io.LimitReader(part, maxImportBytes+1))
	if syncErr := file.Sync(); copyErr == nil {
		copyErr = syncErr
	}
	if closeErr := file.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil || written > maxImportBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "invalid_input")
		return
	}
	if extra, err := reader.NextPart(); err != io.EOF || extra != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	displayName := filepath.Base(part.FileName())
	if displayName == "." || displayName == string(filepath.Separator) || len(displayName) > 255 {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	result, err := s.app.ImportExternalInventory(r.Context(), quarantine, displayName)
	if err != nil {
		s.recordFailure()
		writeError(w, http.StatusUnprocessableEntity, safeCategory(err))
		return
	}
	remove = true
	writeJSON(w, http.StatusOK, result)
}

type connectDestinationRequest struct {
	Credential string `json:"credential"`
}

func (s *Server) handleConnectDestination(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	s.recordAction("connect_destination")
	var request connectDestinationRequest
	if decodeStrictJSON(w, r, &request, 8192) != nil {
		return
	}
	if len(request.Credential) < 16 || len(request.Credential) > 4096 {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	credential := []byte(request.Credential)
	request.Credential = ""
	result, err := s.app.Execute(r.Context(), Command{Kind: "connect_destination", Payload: credential})
	for index := range credential {
		credential[index] = 0
	}
	if err != nil {
		s.recordFailure()
		writeError(w, http.StatusUnprocessableEntity, safeCategory(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type saveStrategyRequest struct {
	ContextLenses          string `json:"context_lenses"`
	RoutingPolicy          string `json:"routing_policy"`
	MaximumNetworkRequests int    `json:"maximum_network_requests"`
	MaximumWallTimeSeconds int    `json:"maximum_wall_time_seconds"`
	MaximumCostMicrounits  int64  `json:"maximum_cost_microunits"`
	MaximumRetryAttempts   int    `json:"maximum_retry_attempts"`
	ManualSupportTolerance int    `json:"manual_support_tolerance"`
}

func (s *Server) handleSaveStrategy(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request saveStrategyRequest
	if decodeStrictJSON(w, r, &request, maxJSONBytes) != nil {
		return
	}
	if strings.TrimSpace(request.ContextLenses) == "" || strings.TrimSpace(request.RoutingPolicy) == "" || request.MaximumNetworkRequests <= 0 || request.MaximumWallTimeSeconds < 60 || request.MaximumCostMicrounits < 0 || request.MaximumRetryAttempts < 0 || request.ManualSupportTolerance < 0 {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	s.execute(w, r, Command{Kind: "save_strategy", Payload: request})
}

type reviewItemRequest struct {
	ItemID               string `json:"item_id"`
	Decision             string `json:"decision"`
	Role                 string `json:"role"`
	Disposition          string `json:"disposition"`
	Rationale            string `json:"rationale"`
	ManualSupportOutcome string `json:"manual_support_outcome"`
}

func (s *Server) handleReviewItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request reviewItemRequest
	if decodeStrictJSON(w, r, &request, maxJSONBytes) != nil {
		return
	}
	if strings.TrimSpace(request.ItemID) == "" || request.Decision != "accept" && request.Decision != "revise" || strings.TrimSpace(request.Role) == "" || strings.TrimSpace(request.Disposition) == "" || strings.TrimSpace(request.ManualSupportOutcome) == "" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	s.execute(w, r, Command{Kind: "review_item", Payload: request})
}

type previewBatchRequest struct {
	MaximumDestinationWrites int `json:"maximum_destination_writes"`
	MaximumMutationAttempts  int `json:"maximum_mutation_attempts"`
}

func (s *Server) handlePreviewBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	s.recordAction("preview_batch")
	var request previewBatchRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	if request.MaximumDestinationWrites <= 0 || request.MaximumMutationAttempts < request.MaximumDestinationWrites {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	preview, err := s.app.PreviewBatch(r.Context(), request.MaximumDestinationWrites, request.MaximumMutationAttempts)
	if err != nil {
		s.recordFailure()
		writeError(w, http.StatusConflict, safeCategory(err))
		return
	}
	nonce, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "local_state_failure")
		return
	}
	s.mu.Lock()
	s.reviewNonces = map[string]reviewNonce{}
	s.reviewNonces[nonce] = reviewNonce{Preview: preview, Session: s.session, ExpiresAt: s.now().Add(5 * time.Minute)}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, struct {
		Preview BatchPreview `json:"preview"`
		Nonce   string       `json:"review_nonce"`
	}{Preview: preview, Nonce: nonce})
}

type approveBatchRequest struct {
	ReviewNonce string `json:"review_nonce"`
}

func (s *Server) handleApproveBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	s.recordAction("approve_batch")
	var request approveBatchRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	if strings.TrimSpace(request.ReviewNonce) == "" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	s.mu.Lock()
	nonce, ok := s.reviewNonces[request.ReviewNonce]
	if ok {
		delete(s.reviewNonces, request.ReviewNonce)
	}
	session := s.session
	s.mu.Unlock()
	if !ok || nonce.Session != session || s.now().After(nonce.ExpiresAt) {
		writeError(w, http.StatusConflict, "approval_invalid")
		return
	}
	approval := s.humanAuthority.sealApproval(nonce.Preview, session, s.now())
	result, err := s.app.ApproveBatch(r.Context(), approval)
	if err != nil {
		s.recordFailure()
		writeError(w, http.StatusConflict, safeCategory(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type founderReviewRequest struct {
	ReceiptFingerprint  string   `json:"receipt_fingerprint"`
	UsefulDraftIDs      []string `json:"useful_draft_ids"`
	ValueVerdict        string   `json:"value_verdict"`
	UsefulnessReason    string   `json:"usefulness_reason"`
	CredentialBurden    string   `json:"credential_burden"`
	ManualSupportBurden string   `json:"manual_support_burden"`
	ApprovalBurden      string   `json:"approval_burden"`
	ZeroDraft           bool     `json:"zero_draft"`
}

func (s *Server) handleFounderReview(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request founderReviewRequest
	if decodeStrictJSON(w, r, &request, maxJSONBytes) != nil {
		return
	}
	if strings.TrimSpace(request.ReceiptFingerprint) == "" || request.ZeroDraft && len(request.UsefulDraftIDs) > 0 || strings.TrimSpace(request.UsefulnessReason) == "" || strings.TrimSpace(request.CredentialBurden) == "" || strings.TrimSpace(request.ManualSupportBurden) == "" || strings.TrimSpace(request.ApprovalBurden) == "" || request.ValueVerdict != "useful" && request.ValueVerdict != "not_useful" && request.ValueVerdict != "zero_draft" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	metrics := s.discoveryMetrics(!request.ZeroDraft && len(request.UsefulDraftIDs) > 0 && strings.TrimSpace(request.UsefulnessReason) != "")
	payload := map[string]any{
		"receipt_fingerprint": request.ReceiptFingerprint, "useful_draft_ids": request.UsefulDraftIDs,
		"value_verdict":     request.ValueVerdict,
		"usefulness_reason": request.UsefulnessReason, "credential_burden": request.CredentialBurden,
		"manual_support_burden": request.ManualSupportBurden, "approval_burden": request.ApprovalBurden,
		"zero_draft": request.ZeroDraft, "discovery_metrics": metrics,
	}
	s.execute(w, r, Command{Kind: "founder_review", Payload: payload})
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	if decodeStrictJSON(w, r, &struct{}{}, 256) != nil {
		return
	}
	s.mu.Lock()
	s.helpRequests++
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": true})
}

type cancelRequest struct {
	ApprovalFingerprint string `json:"approval_fingerprint"`
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requireMutation(w, r, true) {
		return
	}
	var request cancelRequest
	if decodeStrictJSON(w, r, &request, 4096) != nil {
		return
	}
	if strings.TrimSpace(request.ApprovalFingerprint) == "" {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	s.execute(w, r, Command{Kind: "cancel", Payload: request})
}

func (s *Server) handleEmptyCommand(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.requireMutation(w, r, true) {
			return
		}
		if decodeStrictJSON(w, r, &struct{}{}, 256) != nil {
			return
		}
		s.execute(w, r, Command{Kind: kind, Payload: struct{}{}})
	}
}

func (s *Server) execute(w http.ResponseWriter, r *http.Request, command Command) {
	s.recordAction(command.Kind)
	if command.Kind == "review_item" || command.Kind == "founder_review" || command.Kind == "confirm_experimental_drain" || command.Kind == "resume_delivery" {
		s.mu.Lock()
		session := s.session
		s.mu.Unlock()
		action, err := s.humanAuthority.sealAction(command.Kind, command.Payload, session, s.now())
		if err != nil {
			s.recordFailure()
			writeError(w, http.StatusInternalServerError, "local_state_failure")
			return
		}
		command.HumanAction = &action
	}
	result, err := s.app.Execute(r.Context(), command)
	if err != nil {
		s.mu.Lock()
		s.errors++
		s.mu.Unlock()
		writeError(w, http.StatusConflict, safeCategory(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) recordAction(kind string) {
	stage := map[string]int{"import_inventory": 1, "connect_slack_source": 1, "drain_slack_source": 1, "connect_destination": 1, "save_strategy": 2, "freeze_inventory": 3, "start_proof": 4, "review_item": 5, "preview_batch": 6, "approve_batch": 6, "resume_delivery": 6, "founder_review": 7, "confirm_experimental_drain": 8}[kind]
	s.mu.Lock()
	defer s.mu.Unlock()
	if kind != "review_item" && s.actionCounts[kind] > 0 {
		s.retries++
	}
	if stage > 0 && stage < s.furthestStage {
		s.backtracks++
	}
	if stage > s.furthestStage {
		s.furthestStage = stage
	}
	s.actionCounts[kind]++
}

func (s *Server) recordFailure() { s.mu.Lock(); s.errors++; s.mu.Unlock() }

func (s *Server) discoveryMetrics(valueObserved bool) DiscoveryMetrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	metrics := DiscoveryMetrics{StartedAt: s.startedAt.Format(time.RFC3339Nano), SubmittedAt: now.Format(time.RFC3339Nano), ElapsedMilliseconds: now.Sub(s.startedAt).Milliseconds(), Errors: s.errors, Retries: s.retries, Backtracks: s.backtracks, HelpRequests: s.helpRequests}
	if valueObserved {
		elapsed := metrics.ElapsedMilliseconds
		metrics.TimeToTrustedValueMillis = &elapsed
	}
	return metrics
}

func (s *Server) requestBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.expectedHost || !remoteIsTCP4Loopback(r.RemoteAddr) || r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Cookies()) != 0 || r.Header.Get("Authorization") != "" || r.Method == http.MethodOptions {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if r.Header.Get("Access-Control-Request-Method") != "" {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("X-Mindline-Origin") != s.origin {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if r.Method == http.MethodGet {
			if origin := r.Header.Get("Origin"); origin != "" && origin != s.origin {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			if referer := r.Header.Get("Referer"); referer != "" && !strings.HasPrefix(referer, s.origin+"/") {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireMutation(w http.ResponseWriter, r *http.Request, requireJSON bool) bool {
	if !s.requireOrigin(r) || !s.requireSession(r) || !s.requireCSRF(r) {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	if requireJSON && !isJSON(r.Header.Get("Content-Type")) {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_input")
		return false
	}
	return true
}

func (s *Server) requireOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == s.origin
}

func (s *Server) requireSession(r *http.Request) bool {
	s.mu.Lock()
	expected := s.session
	s.mu.Unlock()
	return expected != "" && constantEqual(r.Header.Get("X-Mindline-Session"), expected)
}

func (s *Server) requireCSRF(r *http.Request) bool {
	s.mu.Lock()
	expected := s.csrf
	s.mu.Unlock()
	return expected != "" && constantEqual(r.Header.Get("X-Mindline-CSRF"), expected)
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) == 0 || int64(len(data)) > limit {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return ErrInvalidInput
	}
	if err := privateio.DecodeJSONStrict(data, target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return ErrInvalidInput
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, category string) {
	correlation, _ := randomToken(8)
	writeJSON(w, status, map[string]any{"error": category, "error_code": category, "user_message": safeMessage(category), "changed": "none", "retryable": status >= 500 || status == http.StatusConflict || status == http.StatusTooManyRequests, "recovery_action": safeRecovery(category), "correlation_id": correlation})
}

func writeSettingsConflict(w http.ResponseWriter, current controlsettings.Document) {
	correlation, _ := randomToken(8)
	category := "settings_version_conflict"
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": category, "error_code": category, "user_message": safeMessage(category),
		"changed": "none", "retryable": true, "recovery_action": safeRecovery(category),
		"correlation_id": correlation, "current": current,
	})
}

func safeMessage(category string) string {
	switch category {
	case "session_stale", "unauthorized":
		return "This browser session is no longer valid. Pair again."
	case "pairing_expired":
		return "The pairing code expired or was not confirmed. Create a new code."
	case "pairing_channel_unavailable":
		return "The Codex pairing channel is unavailable. Restart Mindline from Codex."
	case "settings_version_conflict":
		return "Saved settings changed elsewhere. Your edits were not overwritten."
	case "port_occupied":
		return "Mindline could not start because 127.0.0.1:9876 is already in use."
	case "invalid_input":
		return "Review the highlighted input and try again."
	default:
		return "Mindline blocked the operation without changing durable state."
	}
}

func safeRecovery(category string) string {
	switch category {
	case "session_stale", "unauthorized":
		return "pair_again"
	case "pairing_expired":
		return "create_new_code"
	case "settings_version_conflict":
		return "review_conflict"
	default:
		return "retry_or_review"
	}
}

func safeCategory(err error) string {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	default:
		return "operation_blocked"
	}
}

func randomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func constantEqual(actual, expected string) bool {
	if len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func remoteIsTCP4Loopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.IsLoopback()
}

func isJSON(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}
