package cli

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/synergyai-os/Mindline/internal/documents"
)

type semanticJudgmentUIState struct {
	SchemaVersion string                            `json:"schema_version"`
	Summary       documents.SemanticJudgmentSummary `json:"summary"`
	Page          documents.SemanticJudgmentPage    `json:"page"`
}

type semanticJudgmentUIPost struct {
	CandidateID      string   `json:"candidate_id"`
	Choice           string   `json:"choice"`
	FailureReason    string   `json:"failure_reason"`
	SecondaryReasons []string `json:"secondary_failure_reasons"`
	Note             string   `json:"note"`
	ReviewerID       string   `json:"reviewer_id"`
}

type semanticJudgmentUITemplateData struct {
	ReviewToken string
}

func newSemanticJudgmentUIHandler(root, reviewerID string) http.Handler {
	return newSemanticJudgmentUIHandlerWithAllowedHosts(root, reviewerID, nil)
}

func newSemanticJudgmentUIHandlerWithAllowedHosts(root, reviewerID string, allowedHosts []string) http.Handler {
	token, err := newSemanticJudgmentReviewToken()
	if err != nil {
		panic(err)
	}
	return newSemanticJudgmentUIHandlerWithToken(root, reviewerID, token, allowedHosts)
}

func newSemanticJudgmentUIHandlerWithToken(root, reviewerID, reviewToken string, allowedHosts []string) http.Handler {
	hostAllowlist := semanticJudgmentHostAllowlist(allowedHosts)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := semanticJudgmentUITemplate.Execute(w, semanticJudgmentUITemplateData{ReviewToken: reviewToken}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		state, err := loadSemanticJudgmentUIState(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, state)
	})
	mux.HandleFunc("/api/judgments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if status, err := validateSemanticJudgmentWriteRequest(r, reviewToken, hostAllowlist); err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		defer r.Body.Close()
		var post semanticJudgmentUIPost
		if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
			http.Error(w, "invalid judgment payload", http.StatusBadRequest)
			return
		}
		recordReviewer := strings.TrimSpace(post.ReviewerID)
		if recordReviewer == "" {
			recordReviewer = reviewerID
		}
		_, err := documents.RecordSemanticJudgment(root, documents.SemanticJudgmentRecordInput{
			CandidateID:      post.CandidateID,
			Choice:           documents.SemanticJudgmentChoice(post.Choice),
			FailureReason:    documents.SemanticFailureReason(post.FailureReason),
			SecondaryReasons: semanticJudgmentUISecondaryReasons(post.SecondaryReasons),
			Note:             post.Note,
			ReviewerID:       recordReviewer,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		state, err := loadSemanticJudgmentUIState(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, state)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := validateSemanticJudgmentLoopbackHost(r.Host, hostAllowlist); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func semanticJudgmentUISecondaryReasons(values []string) []documents.SemanticFailureReason {
	out := make([]documents.SemanticFailureReason, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, documents.SemanticFailureReason(value))
	}
	return out
}

func semanticJudgmentHostAllowlist(hosts []string) map[string]bool {
	out := map[string]bool{}
	for _, hostPort := range hosts {
		hostKey := semanticJudgmentRequestHostPort(hostPort)
		if hostKey != "" {
			out[hostKey] = true
		}
	}
	return out
}

func newSemanticJudgmentReviewToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func validateSemanticJudgmentWriteRequest(r *http.Request, reviewToken string, allowedHosts map[string]bool) (int, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return http.StatusUnsupportedMediaType, fmt.Errorf("content type must be application/json")
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Mindline-Review-Token")), []byte(reviewToken)) != 1 {
		return http.StatusForbidden, fmt.Errorf("missing or invalid review token")
	}
	if err := validateSemanticJudgmentSameOrigin(r, allowedHosts); err != nil {
		return http.StatusForbidden, err
	}
	return http.StatusOK, nil
}

func validateSemanticJudgmentSameOrigin(r *http.Request, allowedHosts map[string]bool) error {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return requireSemanticJudgmentSameOrigin(origin, r, allowedHosts)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return requireSemanticJudgmentSameOrigin(referer, r, allowedHosts)
	}
	return nil
}

func requireSemanticJudgmentSameOrigin(raw string, r *http.Request, allowedHosts map[string]bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("invalid request origin")
	}
	if err := validateSemanticJudgmentLoopbackHost(parsed.Host, allowedHosts); err != nil {
		return err
	}
	if !strings.EqualFold(parsed.Host, r.Host) {
		return fmt.Errorf("request origin is not the review UI origin")
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	if parsed.Scheme != "" && parsed.Scheme != expectedScheme {
		return fmt.Errorf("request origin is not the review UI origin")
	}
	return nil
}

func validateSemanticJudgmentLoopbackHost(hostPort string, allowedHosts map[string]bool) error {
	hostKey := semanticJudgmentRequestHostPort(hostPort)
	host := semanticJudgmentRequestHost(hostKey)
	if hostKey == "" || host == "" {
		return fmt.Errorf("request host must be loopback")
	}
	if allowedHosts[hostKey] && semanticJudgmentNonRebindableHost(host) {
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("request host must be loopback")
}

func semanticJudgmentRequestHostPort(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	return strings.ToLower(hostPort)
}

func semanticJudgmentRequestHost(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

func semanticJudgmentNonRebindableHost(host string) bool {
	return strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost")
}

func loadSemanticJudgmentUIState(root string) (semanticJudgmentUIState, error) {
	summary, err := documents.ReadSemanticJudgmentSummary(root)
	if err != nil {
		return semanticJudgmentUIState{}, err
	}
	page, err := documents.NextSemanticJudgmentPage(root)
	if err != nil {
		return semanticJudgmentUIState{}, err
	}
	return semanticJudgmentUIState{
		SchemaVersion: "semantic-judgment-ui-state/v0.1",
		Summary:       summary,
		Page:          page,
	}, nil
}

func writeJSONResponse(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func validateLoopbackAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if strings.TrimSpace(port) == "" {
		return fmt.Errorf("missing port")
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("invalid port: %s", port)
	}
	if host == "" {
		return fmt.Errorf("missing host")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("host does not resolve: %s", host)
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return fmt.Errorf("host must resolve only to loopback addresses: %s", host)
		}
	}
	return nil
}

var semanticJudgmentUITemplate = template.Must(template.New("semantic-judgment-ui").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="mindline-review-token" content="{{.ReviewToken}}">
<title>Mindline Review</title>
<style>
:root {
  color-scheme: light;
  --bg: #f7f6f2;
  --panel: #ffffff;
  --ink: #18201f;
  --muted: #66706d;
  --line: #d9ded8;
  --accent: #116b5f;
  --accent-ink: #ffffff;
  --warn: #8d4b1f;
  --bad: #9d2f3b;
  --soft: #eef3ef;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--ink);
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  line-height: 1.45;
}
main {
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto 1fr;
}
header {
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  padding: 16px 22px;
  display: grid;
  gap: 14px;
}
.topline {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}
.titlebar {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
}
h1, h2, h3 { margin: 0; letter-spacing: 0; }
h1 { font-size: 20px; }
.run { color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
.mode-switch {
  display: inline-grid;
  grid-template-columns: repeat(2, minmax(78px, 1fr));
  border: 1px solid var(--line);
  border-radius: 8px;
  overflow: hidden;
  background: #fbfbf9;
}
.mode-switch button {
  border: 0;
  border-radius: 0;
  min-height: 34px;
  padding: 6px 12px;
  background: transparent;
}
.mode-switch button.active {
  background: var(--accent);
  color: var(--accent-ink);
}
.metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(86px, 1fr));
  gap: 8px;
}
.metric {
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 8px 10px;
  background: var(--soft);
}
.metric span { display: block; color: var(--muted); font-size: 12px; }
.metric strong { display: block; font-size: 18px; margin-top: 2px; }
.progress {
  height: 8px;
  background: #e4e8e3;
  border-radius: 999px;
  overflow: hidden;
}
.progress div { height: 100%; width: 0; background: var(--accent); transition: width 160ms ease; }
.workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 18px;
  padding: 18px 22px;
}
section, aside {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
}
section { min-width: 0; }
.candidate-head {
  padding: 18px;
  border-bottom: 1px solid var(--line);
  display: grid;
  gap: 10px;
}
.tags { display: flex; flex-wrap: wrap; gap: 8px; }
.tag {
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 4px 9px;
  font-size: 12px;
  color: var(--muted);
  background: #fbfbf9;
}
.candidate-body { padding: 18px; display: grid; gap: 18px; }
.review-task {
  border-left: 3px solid var(--accent);
  background: var(--soft);
  padding: 12px 14px;
  display: grid;
  gap: 6px;
}
.review-task p { margin: 0; }
.summary { font-size: 16px; }
.summary-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.summary-box {
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 10px 12px;
  background: #fbfbf9;
}
.summary-box span { display: block; color: var(--muted); font-size: 12px; }
.summary-box strong { display: block; margin-top: 3px; overflow-wrap: anywhere; }
.evidence-list { display: grid; gap: 12px; }
.evidence {
  border-left: 3px solid var(--accent);
  background: #fbfbf9;
  padding: 10px 12px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 14px;
}
.relation-list { display: grid; gap: 10px; }
.relation-card {
  border: 1px solid var(--line);
  border-left: 3px solid var(--accent);
  border-radius: 6px;
  background: #fbfbf9;
  padding: 10px 12px;
  display: grid;
  gap: 8px;
  overflow-wrap: anywhere;
}
.relation-card p { margin: 0; }
.relation-summary {
  display: grid;
  gap: 8px;
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 10px 12px;
  background: #fbfbf9;
}
.raw-details {
  border: 1px solid var(--line);
  border-radius: 6px;
  background: #fbfbf9;
  padding: 10px 12px;
}
.raw-details > summary {
  cursor: pointer;
  font-weight: 700;
}
.raw-details-body {
  display: grid;
  gap: 14px;
  margin-top: 12px;
}
.hidden-count { color: var(--muted); font-size: 13px; margin: 6px 0 0; }
.muted { color: var(--muted); }
aside { padding: 16px; align-self: start; display: grid; gap: 14px; }
textarea {
  width: 100%;
  min-height: 100px;
  resize: vertical;
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 10px;
  font: inherit;
}
.reason-control { font-weight: 700; font-size: 13px; color: var(--muted); }
select {
  width: 100%;
  min-height: 38px;
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 8px 10px;
  font: inherit;
  background: #ffffff;
}
.reason-hint { margin: -6px 0 0; color: var(--muted); font-size: 13px; }
.decision-controls { display: grid; gap: 8px; }
.decision-controls button.selected {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
  background: var(--soft);
}
.save-row { display: grid; gap: 8px; }
button {
  border: 1px solid var(--line);
  background: #ffffff;
  color: var(--ink);
  border-radius: 6px;
  min-height: 38px;
  padding: 8px 10px;
  font: inherit;
  cursor: pointer;
}
button.primary { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
button.reject { color: var(--bad); }
button.warn { color: var(--warn); }
button:disabled { cursor: not-allowed; opacity: .55; }
.status { min-height: 22px; color: var(--muted); font-size: 13px; }
.done {
  padding: 42px 24px;
  text-align: center;
}
.guide {
  padding: 22px;
  display: grid;
  gap: 18px;
}
.guide-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}
.guide-panel {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 14px;
  background: #fbfbf9;
}
.guide-panel p { margin: 8px 0 0; color: var(--muted); }
.guide-list {
  margin: 8px 0 0;
  padding-left: 18px;
}
.guide-list li { margin: 6px 0; }
@media (max-width: 920px) {
  .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .summary-grid { grid-template-columns: 1fr; }
  .guide-grid { grid-template-columns: 1fr; }
  .workspace { grid-template-columns: 1fr; padding: 12px; }
  header { padding: 14px 12px; }
}
</style>
</head>
<body>
<main>
  <header>
    <div class="topline">
      <div class="titlebar">
        <h1>Mindline Review</h1>
        <div class="mode-switch" aria-label="view mode">
          <button id="review-mode" class="active" type="button">Review</button>
          <button id="guide-mode" type="button">Guide</button>
        </div>
      </div>
      <div class="run" id="run-context">Loading...</div>
    </div>
    <div class="metrics" id="metrics" aria-live="polite"></div>
    <div class="progress" aria-label="review progress"><div id="progress-bar"></div></div>
  </header>
  <div class="workspace">
    <section id="current-candidate" aria-live="polite"></section>
    <aside>
      <h2>Decision</h2>
      <p class="muted" id="decision-context">Select a judgment, confirm the reason if needed, then save.</p>
      <textarea id="note" placeholder="Optional note"></textarea>
      <label class="reason-control" for="failure-reason">Failure reason</label>
      <select id="failure-reason"></select>
      <p class="reason-hint" id="reason-hint"></p>
      <div class="decision-controls" id="decision-controls"></div>
      <div class="save-row">
        <button id="save-decision" class="primary" type="button" disabled>Save decision</button>
      </div>
      <div class="status" id="status"></div>
    </aside>
  </div>
</main>
<script>
const choices = [
  ["accept", "Accept", "primary"],
  ["reject", "Reject", "reject"],
  ["unclear", "Unclear", "warn"],
  ["duplicate", "Duplicate", ""],
  ["wrong-kind", "Wrong kind", "warn"]
];
const failureReasonsByChoice = {
  "accept": [],
  "reject": ["unexpected_candidate", "unsupported_evidence", "missing_evidence", "too_broad", "too_narrow", "stale_or_contradicted", "unsafe_or_private", "relation_error", "source_scope_error", "other"],
  "unclear": ["ambiguous", "missing_evidence", "unsupported_evidence", "relation_error", "source_scope_error", "other"],
  "duplicate": ["duplicate"],
  "wrong-kind": ["wrong_kind"]
};
const defaultReasonByChoice = {
  "reject": "unexpected_candidate",
  "unclear": "ambiguous",
  "duplicate": "duplicate",
  "wrong-kind": "wrong_kind"
};
const reviewToken = "{{.ReviewToken}}";
const visibleEvidenceLimit = 5;
let currentCandidateId = "";
let currentState = null;
let mode = "review";
let selectedChoice = "";
let isSubmitting = false;

function text(value, fallback = "") {
  return value === undefined || value === null || value === "" ? fallback : String(value);
}

function escapeHtml(value) {
  return text(value).replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" }[ch]));
}

async function loadState() {
  const response = await fetch("/api/state");
  if (!response.ok) throw new Error(await response.text());
  render(await response.json());
}

function render(state) {
  currentState = state;
  const summary = state.summary;
  const page = state.page;
  const queueRemaining = activeReviewRemaining(summary, page);
  document.getElementById("run-context").textContent = "Run " + summary.run_id + " · " + summary.source_count + " source(s)";
  const metricRows = [
    ["Total", summary.candidate_count],
    ["Judged", summary.judged_count],
    ["Queue remaining", queueRemaining],
    ["Unjudged", summary.remaining_count],
    ["Agent reviewed", summary.agent_reviewed_count || 0],
    ["Needs human", summary.human_review_required_count || 0],
    ["Machine triaged", summary.machine_triaged_count || 0],
    ["Accepted", summary.accepted_count],
    ["Rejected", summary.rejected_count],
    ["Unclear", summary.unclear_count],
    ["Duplicate", summary.duplicate_count],
    ["Wrong kind", summary.wrong_kind_count],
    ["Eval counted", summary.eval_counted_count]
  ];
  document.getElementById("metrics").innerHTML = metricRows.map(([label, value]) => "<div class=\"metric\"><span>" + escapeHtml(label) + "</span><strong>" + escapeHtml(value) + "</strong></div>").join("");
  const pct = summary.candidate_count === 0 ? 100 : Math.round((summary.judged_count / summary.candidate_count) * 100);
  document.getElementById("progress-bar").style.width = pct + "%";
  document.getElementById("note").value = "";
  document.getElementById("status").textContent = "";
  selectedChoice = "";
  renderFailureReasonOptions(selectedChoice);
  updateSaveState();
  renderModeButtons();
  if (mode === "guide") {
    renderGuide(summary, queueRemaining);
    return;
  }
  renderReview(page, summary);
}

function activeReviewRemaining(summary, page) {
  if (page && page.cursor && page.cursor.remaining_count !== undefined && page.cursor.remaining_count !== null) {
    return page.cursor.remaining_count;
  }
  return summary.remaining_count;
}

function renderModeButtons() {
  document.getElementById("review-mode").classList.toggle("active", mode === "review");
  document.getElementById("guide-mode").classList.toggle("active", mode === "guide");
}

function renderGuide(summary, queueRemaining) {
  currentCandidateId = "";
  const machineTriaged = summary.machine_triaged_count || 0;
  const guideStatus = queueRemaining > 0
    ? escapeHtml(summary.judged_count) + " reviewed, " + escapeHtml(queueRemaining) + " queue remaining. Switch back to Review to continue."
    : machineTriaged > 0
      ? "Human review queue clear. " + escapeHtml(machineTriaged) + " machine-triaged proposal-only candidate(s) remain unjudged/auditable."
      : escapeHtml(summary.judged_count) + "/" + escapeHtml(summary.candidate_count) + " judged.";
  document.getElementById("current-candidate").innerHTML =
    "<div class=\"guide\">" +
      "<div><h2>How to review</h2><p class=\"muted\">You are evaluating the extraction system, not approving a final knowledge write.</p></div>" +
      "<div class=\"guide-grid\">" +
        "<div class=\"guide-panel\"><h3>1. Useful object</h3><p>Ask whether this candidate deserves to exist as an action, issue, risk, requirement, capability, decision, or topic.</p></div>" +
        "<div class=\"guide-panel\"><h3>2. Type and scope</h3><p>Check whether the label and scope are right. Real content with the wrong label is usually wrong kind.</p></div>" +
        "<div class=\"guide-panel\"><h3>3. Evidence support</h3><p>Use the shown evidence, blockers, and relations. If the candidate says more than the evidence proves, mark it unclear or reject.</p></div>" +
        "<div class=\"guide-panel\"><h3>4. Duplicates and resolution</h3><p>If multiple candidates point to the same thing, use duplicate. If something was resolved elsewhere, do not accept a stale issue as-is.</p></div>" +
      "</div>" +
      "<div class=\"guide-panel\"><h3>Decision meanings</h3><ul class=\"guide-list\">" +
        "<li><strong>Accept</strong>: useful, correctly typed, scoped, and evidence-backed.</li>" +
        "<li><strong>Reject</strong>: should not have been emitted.</li>" +
        "<li><strong>Unclear</strong>: not enough context, relation meaning, or evidence to judge confidently.</li>" +
        "<li><strong>Duplicate</strong>: already represented by another candidate.</li>" +
        "<li><strong>Wrong kind</strong>: real signal, but wrong object type or scope.</li>" +
      "</ul></div>" +
      "<p class=\"muted\">" + guideStatus + "</p>" +
    "</div>";
  document.getElementById("decision-controls").innerHTML = "";
  renderFailureReasonOptions(selectedChoice);
  document.getElementById("reason-hint").textContent = "";
  updateSaveState();
}

function renderReview(page, summary) {
  if (page.done || !page.item) {
    currentCandidateId = "";
    const machineTriaged = summary.machine_triaged_count || 0;
    const title = machineTriaged > 0 ? "Human review queue clear" : "Review complete";
    const message = machineTriaged > 0
      ? escapeHtml(machineTriaged) + " candidate(s) are machine-triaged proposal-only and remain unjudged/auditable."
      : escapeHtml(summary.judged_count) + "/" + escapeHtml(summary.candidate_count) + " judged. Precision estimate: " + escapeHtml(summary.precision_estimate) + ".";
    document.getElementById("current-candidate").innerHTML = "<div class=\"done\"><h2>" + title + "</h2><p class=\"muted\">" + message + "</p></div>";
    document.getElementById("decision-controls").innerHTML = "";
    renderFailureReasonOptions(selectedChoice);
    document.getElementById("reason-hint").textContent = "";
    updateSaveState();
    return;
  }
  const item = page.item;
  currentCandidateId = item.candidate_id;
  const agent = item.agent_review_proposal || null;
  const agentReasons = agent && agent.review_reason_codes ? agent.review_reason_codes.map(reason => "<span class=\"tag\">" + escapeHtml(reason) + "</span>").join("") : "";
  const agentPanel = agent ? (
    "<div class=\"agent-review\"><h3>Agent proposal</h3>" +
      "<p><strong>Proposal only.</strong> This is not a saved judgment, destination approval, or DEC-64 evidence.</p>" +
      "<div class=\"tags\">" +
        "<span class=\"tag\">choice: " + escapeHtml(agent.choice) + "</span>" +
        (agent.failure_reason ? "<span class=\"tag\">reason: " + escapeHtml(agent.failure_reason) + "</span>" : "") +
        "<span class=\"tag\">confidence: " + escapeHtml(agent.confidence) + "</span>" +
        "<span class=\"tag\">human review required: " + escapeHtml(Boolean(agent.human_review_required)) + "</span>" +
      "</div>" +
      (agentReasons ? "<div class=\"tags\">" + agentReasons + "</div>" : "") +
      "<p class=\"muted\">" + escapeHtml(agent.rationale || "No rationale") + "</p>" +
    "</div>"
  ) : "";
  const readiness = item.evidence_readiness || {};
  const readinessReasons = (readiness.reason_codes || []).map(reason => "<span class=\"tag\">" + escapeHtml(reason) + "</span>").join("") || "<span class=\"tag\">No readiness reasons</span>";
  const readinessTag = readiness.status ? "<span class=\"tag\">readiness: " + escapeHtml(readiness.status) + "</span>" : "<span class=\"tag\">readiness unavailable</span>";
  const evalTag = "<span class=\"tag\">eval counted: " + escapeHtml(Boolean(readiness.eval_counted)) + "</span>";
  const ranges = (item.evidence_ranges || []).map(range => "<span class=\"tag\">" + escapeHtml(range.structure_node_id) + " lines " + escapeHtml(range.line_start) + "-" + escapeHtml(range.line_end) + "</span>").join("");
  const allExcerpts = item.evidence_excerpts || [];
  const visibleExcerpts = allExcerpts.slice(0, visibleEvidenceLimit);
  const hiddenExcerptCount = Math.max(0, allExcerpts.length - visibleEvidenceLimit);
  const excerptHtml = excerpt => {
    if (excerpt.unavailable) {
      return "<div class=\"evidence muted\">" + escapeHtml(excerpt.unavailable_reason || "source excerpt unavailable") + "</div>";
    }
    return "<div class=\"evidence\"><strong>" + escapeHtml(excerpt.source_label) + " lines " + escapeHtml(excerpt.line_start) + "-" + escapeHtml(excerpt.line_end) + "</strong>\n" + escapeHtml(excerpt.text) + "</div>";
  };
  const excerpts = visibleExcerpts.map(excerptHtml).join("") || "<div class=\"evidence muted\">No source excerpts available.</div>";
  const hiddenExcerptNotice = hiddenExcerptCount > 0 ? "<p class=\"hidden-count\">" + escapeHtml(hiddenExcerptCount) + " more excerpt(s) available in Raw details.</p>" : "";
  const rawExcerpts = allExcerpts.map(excerptHtml).join("") || "<div class=\"evidence muted\">No source excerpts available.</div>";
  const sourceTag = item.source_document_id ? "<span class=\"tag\">" + escapeHtml(item.source_document_id) + "</span>" : "";
  const rangesHtml = ranges || "<span class=\"tag\">No evidence ranges</span>";
  const relationIds = (item.relation_ids || []).map(id => "<span class=\"tag\">" + escapeHtml(id) + "</span>").join("") || "<span class=\"tag\">No relation ids</span>";
  const relationSummary = renderRelationSummary(item);
  const relationContext = renderRelationContext(item);
  const blockers = (item.blockers || []).map(blocker => "<div class=\"evidence warn\"><strong>" + escapeHtml(blocker.code || "blocker") + "</strong>\n" + escapeHtml(blocker.message || "No blocker message") + "</div>").join("") || "<div class=\"evidence muted\">No blockers</div>";
  document.getElementById("current-candidate").innerHTML =
    "<div class=\"candidate-head\">" +
      "<h2>" + escapeHtml(item.title || "Untitled candidate") + "</h2>" +
      "<div class=\"tags\">" +
        "<span class=\"tag\">" + escapeHtml(item.candidate_kind) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(item.confidence) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(item.review_status) + "</span>" +
        readinessTag +
        evalTag +
        "<span class=\"tag\">" + escapeHtml(item.candidate_id) + "</span>" +
        sourceTag +
      "</div>" +
    "</div>" +
    "<div class=\"candidate-body\">" +
      "<div class=\"review-task\"><h3>Review task</h3><p><strong>Should this candidate count as a correct semantic extraction?</strong></p><p class=\"muted\">Judge the extraction system, not whether this is ready to write into a destination.</p></div>" +
      agentPanel +
      "<div><h3>Summary</h3><p class=\"summary\">" + escapeHtml(item.summary || "No summary") + "</p></div>" +
      "<div class=\"summary-grid\">" +
        "<div class=\"summary-box\"><span>Kind</span><strong>" + escapeHtml(item.candidate_kind) + "</strong></div>" +
        "<div class=\"summary-box\"><span>Evidence readiness</span><strong>" + escapeHtml(readiness.status || "unavailable") + "</strong></div>" +
        "<div class=\"summary-box\"><span>Relations</span><strong>" + escapeHtml((item.relation_context || []).length) + " loaded</strong></div>" +
      "</div>" +
      "<div><h3>Evidence highlights</h3><div class=\"evidence-list\">" + excerpts + "</div>" + hiddenExcerptNotice + "</div>" +
      "<div><h3>Relation summary</h3>" + relationSummary + "</div>" +
      "<details class=\"raw-details\" id=\"openRawDetails\"><summary>Raw details</summary><div class=\"raw-details-body\">" +
        "<div><h3>Evidence readiness internals</h3><div class=\"tags\">" + readinessReasons + readinessTag + evalTag + "</div></div>" +
        "<div><h3>Evidence ranges</h3><div class=\"tags\">" + rangesHtml + "</div></div>" +
        "<div><h3>All source excerpts</h3><div class=\"evidence-list\">" + rawExcerpts + "</div></div>" +
        "<div><h3>Relation context</h3>" + relationContext + "</div>" +
        "<div><h3>Relation ids</h3><div class=\"tags\">" + relationIds + "</div></div>" +
        "<div><h3>Blockers</h3><div class=\"evidence-list\">" + blockers + "</div></div>" +
      "</div></details>" +
    "</div>";
  document.getElementById("decision-controls").innerHTML = choices.map(([choice, label, klass]) => "<button class=\"" + escapeHtml(klass) + "\" data-choice=\"" + escapeHtml(choice) + "\">" + escapeHtml(label) + "</button>").join("");
  for (const button of document.querySelectorAll("[data-choice]")) {
    button.addEventListener("click", () => selectDecision(button.dataset.choice));
  }
  renderFailureReasonOptions(selectedChoice);
  updateSaveState();
}

function renderFailureReasonOptions(choice) {
  const select = document.getElementById("failure-reason");
  const hint = document.getElementById("reason-hint");
  const activeChoice = choice || selectedChoice;
  const reasons = failureReasonsByChoice[selectedChoice] || failureReasonsByChoice[activeChoice] || [];
  if (!activeChoice) {
    select.disabled = true;
    select.innerHTML = "<option value=\"\">Select a decision first</option>";
    hint.textContent = "Failure reasons appear after you select a non-accept decision.";
    return;
  }
  if (activeChoice === "accept") {
    select.disabled = true;
    select.innerHTML = "<option value=\"\">Accept records no failure reason</option>";
    hint.textContent = "Accept means the candidate is useful, correctly typed, scoped, and evidence-backed.";
    return;
  }
  select.disabled = false;
  select.innerHTML = reasons.map(reason => "<option value=\"" + escapeHtml(reason) + "\">" + escapeHtml(reason) + "</option>").join("");
  select.value = defaultReasonByChoice[activeChoice] || reasons[0] || "";
  hint.textContent = "Only reasons compatible with " + activeChoice + " are available.";
}

function ensureCompatibleFailureReason(choice) {
  const select = document.getElementById("failure-reason");
  if (choice === "accept") return "";
  const allowed = failureReasonsByChoice[choice] || [];
  const fallback = defaultReasonByChoice[choice] || allowed[0] || "";
  if (!allowed.includes(select.value)) select.value = fallback;
  return select.value;
}

function selectDecision(choice) {
  selectedChoice = choice;
  renderFailureReasonOptions(selectedChoice);
  updateSaveState();
}

function updateSaveState() {
  const saveButton = document.getElementById("save-decision");
  const context = document.getElementById("decision-context");
  for (const button of document.querySelectorAll("[data-choice]")) {
    button.classList.toggle("selected", button.dataset.choice === selectedChoice);
    button.disabled = isSubmitting;
  }
  if (!saveButton) return;
  saveButton.disabled = isSubmitting || !currentCandidateId || !selectedChoice;
  if (context) {
    context.textContent = selectedChoice ? "Selected: " + selectedChoice + ". Confirm the reason if needed, then save." : "Select a judgment, confirm the reason if needed, then save.";
  }
}

function renderRelationContext(item) {
  const contexts = item.relation_context || [];
  if (contexts.length === 0) {
    if ((item.relation_ids || []).length === 0) {
      return "<div class=\"evidence muted\">No relations</div>";
    }
    return "<div class=\"evidence muted\">Relation context unavailable for: " + escapeHtml(item.relation_ids.join(", ")) + "</div>";
  }
  const loaded = new Set(contexts.map(relation => relation.relation_id));
  const missing = (item.relation_ids || []).filter(id => !loaded.has(id));
  const cards = contexts.map(relation => {
    const endpoint = relation.other_endpoint || {};
    const endpointText = endpoint.unavailable
      ? escapeHtml((endpoint.endpoint_type || "endpoint") + " " + (endpoint.endpoint_id || "") + " - " + (endpoint.unavailable_reason || "endpoint context unavailable"))
      : escapeHtml((endpoint.endpoint_type || "endpoint") + " " + (endpoint.endpoint_id || "") + " - " + (endpoint.label || "no label") + "; " + (endpoint.summary || "no summary"));
    const endpointRole = endpoint.role ? escapeHtml(endpoint.role) : "unknown";
    const blockers = (relation.blockers || []).map(blocker => "<p><strong>Blocker:</strong> " + escapeHtml(blocker.code || "blocker") + " - " + escapeHtml(blocker.message || "No blocker message") + "</p>").join("");
    return "<div class=\"relation-card\">" +
      "<div class=\"tags\">" +
        "<span class=\"tag\">" + escapeHtml(relation.relationship_type) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(relation.confidence) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(relation.review_status) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(relation.relation_id) + "</span>" +
      "</div>" +
      "<p><strong>From:</strong> " + escapeHtml(relation.from_type) + " " + escapeHtml(relation.from_id) + "</p>" +
      "<p><strong>To:</strong> " + escapeHtml(relation.to_type) + " " + escapeHtml(relation.to_id) + "</p>" +
      "<p><strong>Other endpoint role:</strong> " + endpointRole + "</p>" +
      "<p><strong>Other endpoint:</strong> " + endpointText + "</p>" +
      "<p><strong>Review hint:</strong> " + escapeHtml(relation.review_hint || "Use this relation to judge the candidate.") + "</p>" +
      blockers +
    "</div>";
  });
  if (missing.length > 0) {
    cards.push("<div class=\"evidence muted\">Relation context unavailable for: " + escapeHtml(missing.join(", ")) + "</div>");
  }
  return "<div class=\"relation-list\">" + cards.join("") + "</div>";
}

function renderRelationSummary(item) {
  const contexts = item.relation_context || [];
  if (contexts.length === 0) {
    return "<div class=\"relation-summary\"><p class=\"muted\">No loaded relation context.</p></div>";
  }
  const counts = {};
  for (const relation of contexts) {
    const key = relation.relationship_type || "unknown";
    counts[key] = (counts[key] || 0) + 1;
  }
  const tags = Object.entries(counts).map(([key, value]) => "<span class=\"tag\">" + escapeHtml(key) + ": " + escapeHtml(value) + "</span>").join("");
  return "<div class=\"relation-summary\"><p>" + escapeHtml(contexts.length) + " relation(s) are linked to this candidate. Open Raw details only if the evidence highlights are not enough.</p><div class=\"tags\">" + tags + "</div></div>";
}

async function submitSelectedChoice() {
  if (isSubmitting) return;
  if (!selectedChoice) {
    document.getElementById("status").textContent = "Select a decision first.";
    return;
  }
  ensureCompatibleFailureReason(selectedChoice);
  await submitChoice(selectedChoice);
}

async function submitChoice(choice) {
  if (!currentCandidateId || isSubmitting) return;
  const allowed = failureReasonsByChoice[choice] || [];
  let failureReason = ensureCompatibleFailureReason(choice);
  if (choice !== "accept" && !allowed.includes(failureReason)) {
    document.getElementById("status").textContent = "Select a compatible reason for " + choice + ": " + allowed.join(", ");
    return;
  }
  document.getElementById("status").textContent = "Saving...";
  isSubmitting = true;
  updateSaveState();
  try {
    const response = await fetch("/api/judgments", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Mindline-Review-Token": reviewToken },
      body: JSON.stringify({ candidate_id: currentCandidateId, choice, failure_reason: failureReason, note: document.getElementById("note").value })
    });
    if (!response.ok) {
      document.getElementById("status").textContent = await response.text();
      return;
    }
    render(await response.json());
  } catch (error) {
    document.getElementById("status").textContent = error.message || String(error);
  } finally {
    isSubmitting = false;
    updateSaveState();
  }
}

document.getElementById("save-decision").addEventListener("click", submitSelectedChoice);

document.getElementById("review-mode").addEventListener("click", () => {
  mode = "review";
  if (currentState) render(currentState);
});
document.getElementById("guide-mode").addEventListener("click", () => {
  mode = "guide";
  if (currentState) render(currentState);
});

loadState().catch(error => {
  document.getElementById("current-candidate").innerHTML = "<div class=\"done\"><h2>Unable to load review</h2><p class=\"muted\">" + escapeHtml(error.message) + "</p></div>";
});
</script>
</body>
</html>`))
