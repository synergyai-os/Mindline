package cli

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/synergyai-os/Mindline/internal/documents"
)

type corpusConceptUIState struct {
	SchemaVersion string                                `json:"schema_version"`
	Summary       documents.CorpusConceptSummary        `json:"summary"`
	Index         documents.CorpusConceptIndex          `json:"index"`
	ReviewRecords documents.CorpusConceptReviewRecords  `json:"review_records"`
	Progress      documents.CorpusConceptReviewProgress `json:"progress"`
}

type corpusConceptUIPost struct {
	ConceptID  string `json:"concept_id"`
	Choice     string `json:"choice"`
	Note       string `json:"note"`
	ReviewerID string `json:"reviewer_id"`
}

type corpusConceptUITemplateData struct {
	ReviewToken string
}

func newCorpusConceptUIHandlerWithAllowedHosts(root string, allowedHosts []string) http.Handler {
	token, err := newSemanticJudgmentReviewToken()
	if err != nil {
		panic(err)
	}
	return newCorpusConceptUIHandlerWithToken(root, token, allowedHosts)
}

func newCorpusConceptUIHandlerWithToken(root, reviewToken string, allowedHosts []string) http.Handler {
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
		if err := corpusConceptUITemplate.Execute(w, corpusConceptUITemplateData{ReviewToken: reviewToken}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		state, err := loadCorpusConceptUIState(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, state)
	})
	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if status, err := validateSemanticJudgmentWriteRequest(r, reviewToken, hostAllowlist); err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		defer r.Body.Close()
		var post corpusConceptUIPost
		if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
			http.Error(w, "invalid concept review payload", http.StatusBadRequest)
			return
		}
		_, err := documents.RecordCorpusConceptReview(root, documents.CorpusConceptReviewRecordInput{
			ConceptID:  post.ConceptID,
			Choice:     documents.CorpusConceptReviewChoice(post.Choice),
			Note:       post.Note,
			ReviewerID: strings.TrimSpace(post.ReviewerID),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		state, err := loadCorpusConceptUIState(root)
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

func loadCorpusConceptUIState(root string) (corpusConceptUIState, error) {
	summary, err := documents.ReadCorpusConceptSummary(root)
	if err != nil {
		return corpusConceptUIState{}, err
	}
	index, err := documents.ReadCorpusConceptIndex(root)
	if err != nil {
		return corpusConceptUIState{}, err
	}
	records, err := documents.ReadCorpusConceptReviewRecords(root)
	if err != nil {
		return corpusConceptUIState{}, err
	}
	return corpusConceptUIState{
		SchemaVersion: "corpus-concept-ui-state/v0.1",
		Summary:       summary,
		Index:         index,
		ReviewRecords: records,
		Progress:      documents.BuildCorpusConceptReviewProgress(index, records),
	}, nil
}

var corpusConceptUITemplate = template.Must(template.New("corpus-concept-ui").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="mindline-review-token" content="{{.ReviewToken}}">
<title>Mindline Concept Review</title>
<style>
:root {
  color-scheme: light;
  --bg: #f5f6f1;
  --panel: #ffffff;
  --ink: #18211f;
  --muted: #65716d;
  --line: #d8ded6;
  --accent: #0b6f5c;
  --accent-soft: #e8f3ef;
  --gold: #7c5a16;
  --gold-soft: #f7f0dc;
  --red: #8f2f3d;
  --red-soft: #f8e6e9;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--ink);
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  line-height: 1.42;
}
h1, h2, h3, p { margin: 0; letter-spacing: 0; }
h1 { font-size: 22px; }
h2 { font-size: 20px; }
h3 { font-size: 15px; }
main { min-height: 100vh; display: grid; grid-template-rows: auto 1fr; }
header {
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  padding: 16px 22px;
  display: grid;
  gap: 12px;
}
.run { color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
.metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 8px;
}
.metric {
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 9px 11px;
  background: #fafbf7;
}
.metric span { display: block; color: var(--muted); font-size: 12px; }
.metric strong { display: block; font-size: 18px; margin-top: 2px; }
.workspace {
  display: grid;
  grid-template-columns: minmax(300px, 440px) minmax(0, 1fr);
  gap: 16px;
  padding: 18px 22px;
}
.list, .detail {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  min-width: 0;
  overflow: hidden;
}
.list-head, .detail-head {
  padding: 15px 16px;
  border-bottom: 1px solid var(--line);
  display: grid;
  gap: 6px;
}
.filters { display: flex; flex-wrap: wrap; gap: 6px; }
.filter, .decision {
  border: 1px solid var(--line);
  background: #fff;
  border-radius: 6px;
  padding: 7px 9px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.filter.active, .decision.active { border-color: var(--accent); background: var(--accent-soft); color: var(--accent); }
.concepts { max-height: calc(100vh - 245px); overflow: auto; }
button.concept {
  width: 100%;
  border: 0;
  border-bottom: 1px solid var(--line);
  background: #fff;
  text-align: left;
  padding: 12px 14px;
  display: grid;
  gap: 7px;
  cursor: pointer;
  font: inherit;
}
button.concept:hover, button.concept.active { background: var(--accent-soft); }
.title { font-weight: 750; overflow-wrap: anywhere; }
.meta, .muted { color: var(--muted); font-size: 13px; }
.tags { display: flex; gap: 6px; flex-wrap: wrap; }
.tag {
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 3px 8px;
  font-size: 12px;
  color: var(--muted);
  background: #fbfbf8;
}
.tag.cross_source, .tag.reviewed { color: var(--accent); background: var(--accent-soft); }
.tag.needs_review { color: var(--gold); background: var(--gold-soft); }
.tag.blocked, .tag.unreviewed { color: var(--red); background: var(--red-soft); }
.detail-body {
  padding: 16px;
  display: grid;
  gap: 16px;
}
.question {
  border-left: 4px solid var(--accent);
  background: var(--accent-soft);
  padding: 12px 14px;
  border-radius: 6px;
  display: grid;
  gap: 6px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(135px, 1fr));
  gap: 8px;
}
.box {
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 10px 12px;
  background: #fbfbf8;
}
.box span { display: block; color: var(--muted); font-size: 12px; }
.box strong { display: block; margin-top: 3px; overflow-wrap: anywhere; }
.review-form {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 12px;
  display: grid;
  gap: 10px;
}
.decisions { display: flex; flex-wrap: wrap; gap: 7px; }
textarea {
  min-height: 76px;
  resize: vertical;
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 10px;
  font: inherit;
}
.save {
  width: fit-content;
  border: 1px solid var(--accent);
  background: var(--accent);
  color: #fff;
  border-radius: 6px;
  padding: 8px 12px;
  font: inherit;
  cursor: pointer;
}
.evidence { display: grid; gap: 10px; }
.evidence-card {
  border: 1px solid var(--line);
  border-left: 4px solid var(--accent);
  border-radius: 6px;
  padding: 12px;
  display: grid;
  gap: 7px;
  background: #fffefa;
}
.excerpt { font-size: 14px; }
.trace { color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
.empty {
  padding: 36px 18px;
  color: var(--muted);
  text-align: center;
}
@media (max-width: 900px) {
  header { padding: 14px 12px; }
  .workspace { grid-template-columns: 1fr; padding: 12px; }
  .concepts { max-height: none; }
}
</style>
</head>
<body>
<main>
  <header>
    <div>
      <h1>Mindline Concept Review</h1>
      <div class="run" id="run">Loading...</div>
    </div>
    <div class="metrics" id="metrics"></div>
  </header>
  <div class="workspace">
    <section class="list">
      <div class="list-head">
        <h2>Concept Queue</h2>
        <p class="muted" id="list-summary"></p>
        <div class="filters" id="filters"></div>
      </div>
      <div class="concepts" id="concepts"></div>
    </section>
    <section class="detail" id="detail"></section>
  </div>
</main>
<script>
const reviewToken = document.querySelector('meta[name="mindline-review-token"]').content;
const choices = [
  ["accept", "Accept"],
  ["reject_noisy", "Noisy"],
  ["split_needed", "Split"],
  ["merge_duplicate", "Merge"],
  ["rename_needed", "Rename"],
  ["needs_source_context", "Need context"]
];
let state = null;
let selected = "";
let filter = "all";
let activeChoice = "";

function esc(value) {
  return String(value || "").replace(/[&<>"']/g, (ch) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;"
  }[ch]));
}

function fmtRatio(value, digits = 2) {
  if (typeof value !== "number") return "0.00";
  return value.toFixed(digits);
}

function coverage(map) {
  if (!map) return "";
  return Object.keys(map).sort().map((key) => key + ":" + map[key]).join(", ");
}

function metric(label, value) {
  return '<div class="metric"><span>' + esc(label) + '</span><strong>' + esc(value) + '</strong></div>';
}

function recordsByConcept() {
  const out = {};
  ((state.review_records && state.review_records.records) || []).forEach((record) => {
    out[record.concept_id] = record;
  });
  return out;
}

function filteredConcepts() {
  const concepts = state.index.concepts || [];
  const reviewed = recordsByConcept();
  if (filter === "reviewed") return concepts.filter((concept) => reviewed[concept.concept_id]);
  if (filter === "unreviewed") return concepts.filter((concept) => !reviewed[concept.concept_id]);
  if (filter === "cross_source") return concepts.filter((concept) => concept.section === "cross_source");
  if (filter === "needs_review") return concepts.filter((concept) => concept.review_status === "needs_review" || concept.section === "needs_review");
  return concepts;
}

function render() {
  const summary = state.summary;
  const progress = state.progress || {};
  const concepts = filteredConcepts();
  document.getElementById("run").textContent = summary.corpus_id + " | " + summary.processed_source_count + "/" + summary.source_count + " sources | " + summary.scale_status;
  document.getElementById("metrics").innerHTML = [
    metric("Reviewed", (progress.reviewed_concept_count || 0) + "/" + (progress.total_concept_count || summary.concept_count)),
    metric("Cross-source", summary.cross_source_concept_count),
    metric("Concepts", summary.concept_count),
    metric("Atoms", summary.atom_count),
    metric("Relations", summary.relation_count),
    metric("Compression", fmtRatio(summary.relation_review_compression_ratio, 4))
  ].join("");
  document.getElementById("list-summary").textContent = concepts.length + " shown; " + (progress.remaining_concept_count || 0) + " remaining";
  renderFilters();
  if (!selected && concepts.length) selected = concepts[0].concept_id;
  if (selected && !concepts.find((concept) => concept.concept_id === selected) && concepts.length) selected = concepts[0].concept_id;
  const reviewed = recordsByConcept();
  document.getElementById("concepts").innerHTML = concepts.map((concept) => {
    const active = concept.concept_id === selected ? " active" : "";
    const record = reviewed[concept.concept_id];
    const reviewTag = record ? '<span class="tag reviewed">' + esc(record.choice) + '</span>' : '<span class="tag unreviewed">unreviewed</span>';
    return '<button class="concept' + active + '" type="button" data-id="' + esc(concept.concept_id) + '">' +
      '<div class="title">' + esc(concept.title) + '</div>' +
      '<div class="meta">' + esc(concept.grouping_rationale || "") + '</div>' +
      '<div class="meta">atoms=' + esc(concept.atom_count) + ' sources=' + esc(concept.source_count) + ' previews=' + esc(concept.representative_evidence_count || 0) + '</div>' +
      '<div class="tags"><span class="tag ' + esc(concept.section) + '">' + esc(concept.section) + '</span><span class="tag">' + esc(coverage(concept.source_kind_coverage)) + '</span>' + reviewTag + '</div>' +
      '</button>';
  }).join("") || '<div class="empty">No concepts match this filter.</div>';
  document.querySelectorAll("button.concept").forEach((button) => {
    button.addEventListener("click", () => {
      selected = button.dataset.id;
      activeChoice = "";
      render();
    });
  });
  renderDetail((state.index.concepts || []).find((concept) => concept.concept_id === selected));
}

function renderFilters() {
  const filters = [
    ["all", "All"],
    ["unreviewed", "Unreviewed"],
    ["reviewed", "Reviewed"],
    ["cross_source", "Cross-source"],
    ["needs_review", "Needs review"]
  ];
  document.getElementById("filters").innerHTML = filters.map(([id, label]) => {
    return '<button class="filter' + (filter === id ? ' active' : '') + '" type="button" data-filter="' + id + '">' + esc(label) + '</button>';
  }).join("");
  document.querySelectorAll("button.filter").forEach((button) => {
    button.addEventListener("click", () => {
      filter = button.dataset.filter;
      render();
    });
  });
}

function renderDetail(concept) {
  const detail = document.getElementById("detail");
  if (!concept) {
    detail.innerHTML = '<div class="empty">Select a concept.</div>';
    return;
  }
  const record = recordsByConcept()[concept.concept_id];
  if (!activeChoice && record) activeChoice = record.choice;
  const note = record && record.note ? record.note : "";
  const reasons = concept.reason_codes && concept.reason_codes.length ? concept.reason_codes.join(", ") : "none";
  const evidence = (concept.representative_evidence || []).map((item) => {
    return '<article class="evidence-card">' +
      '<div class="tags"><span class="tag">' + esc(item.source_kind) + '</span><span class="tag">' + esc(item.source_ref) + '</span><span class="tag">lines ' + esc(item.line_start) + '-' + esc(item.line_end) + '</span></div>' +
      '<div class="title">' + esc(item.title || "Untitled evidence") + '</div>' +
      '<p class="muted">' + esc(item.summary) + '</p>' +
      '<p class="excerpt">' + esc(item.excerpt || "No safe excerpt available.") + '</p>' +
      '<div class="trace">' + esc(item.evidence_ref_id) + ' | ' + esc(item.content_hash) + '</div>' +
      '</article>';
  }).join("") || '<p class="muted">No representative evidence previews.</p>';
  detail.innerHTML = '<div class="detail-head"><h2>' + esc(concept.title) + '</h2><p class="muted">' + esc(concept.concept_id) + '</p></div>' +
    '<div class="detail-body">' +
    '<section class="question"><h3>Review Question</h3><p>' + esc(concept.review_prompt || "") + '</p><p class="muted">' + esc(concept.grouping_rationale || "") + '</p></section>' +
    '<div class="grid">' +
    '<div class="box"><span>Status</span><strong>' + esc(concept.review_status) + '</strong></div>' +
    '<div class="box"><span>Section</span><strong>' + esc(concept.section) + '</strong></div>' +
    '<div class="box"><span>Atoms</span><strong>' + esc(concept.atom_count) + '</strong></div>' +
    '<div class="box"><span>Sources</span><strong>' + esc(concept.source_count) + '</strong></div>' +
    '<div class="box"><span>Coverage</span><strong>' + esc(coverage(concept.source_kind_coverage)) + '</strong></div>' +
    '<div class="box"><span>Reasons</span><strong>' + esc(reasons) + '</strong></div>' +
    '</div>' +
    '<section class="review-form">' +
    '<h3>Decision</h3>' +
    '<div class="decisions">' + choices.map(([id, label]) => '<button class="decision' + (activeChoice === id ? ' active' : '') + '" type="button" data-choice="' + id + '">' + esc(label) + '</button>').join("") + '</div>' +
    '<textarea id="review-note" placeholder="Optional note">' + esc(note) + '</textarea>' +
    '<button class="save" type="button" id="save-review">Save decision</button>' +
    '<p class="muted">' + (record ? 'Last saved: ' + esc(record.choice) + ' at ' + esc(record.recorded_at) : 'No decision saved yet.') + '</p>' +
    '</section>' +
    '<section><h3>Representative Evidence</h3><div class="evidence">' + evidence + '</div></section>' +
    '</div>';
  document.querySelectorAll("button.decision").forEach((button) => {
    button.addEventListener("click", () => {
      activeChoice = button.dataset.choice;
      renderDetail(concept);
    });
  });
  document.getElementById("save-review").addEventListener("click", () => saveReview(concept.concept_id));
}

function saveReview(conceptID) {
  if (!activeChoice) return;
  fetch("/api/reviews", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Mindline-Review-Token": reviewToken },
    body: JSON.stringify({
      concept_id: conceptID,
      choice: activeChoice,
      note: document.getElementById("review-note").value
    })
  })
    .then((response) => {
      if (!response.ok) throw new Error("review save failed: " + response.status);
      return response.json();
    })
    .then((data) => { state = data; render(); })
    .catch((error) => {
      document.getElementById("detail").insertAdjacentHTML("beforeend", '<p class="empty">' + esc(error.message) + '</p>');
    });
}

fetch("/api/state")
  .then((response) => response.json())
  .then((data) => { state = data; render(); })
  .catch((error) => {
    document.getElementById("detail").innerHTML = '<div class="empty">' + esc(error) + '</div>';
  });
</script>
</body>
</html>`))
