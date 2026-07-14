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
	ConceptID      string `json:"concept_id"`
	ReviewWorkKind string `json:"review_work_kind"`
	Choice         string `json:"choice"`
	Note           string `json:"note"`
	ReviewerID     string `json:"reviewer_id"`
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
			ConceptID:      post.ConceptID,
			ReviewWorkKind: documents.CorpusConceptReviewWorkKind(post.ReviewWorkKind),
			Choice:         documents.CorpusConceptReviewChoice(post.Choice),
			Note:           post.Note,
			ReviewerID:     strings.TrimSpace(post.ReviewerID),
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
.tag.concept_review { color: var(--accent); background: var(--accent-soft); }
.tag.cleanup_triage { color: var(--gold); background: var(--gold-soft); }
.tag.enrichment_backlog, .tag.blocked_diagnostic { color: var(--red); background: var(--red-soft); }
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
.contract {
  border: 1px solid var(--line);
  border-left: 4px solid var(--accent);
  border-radius: 6px;
  padding: 12px 14px;
  display: grid;
  gap: 10px;
  background: #fbfbf8;
}
.contract-item { display: grid; gap: 3px; }
.contract-item span, .rubric-title {
  color: var(--muted);
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}
.rubric {
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 10px;
  display: grid;
  gap: 8px;
  background: #fbfbf8;
}
.rubric-list { display: grid; gap: 6px; }
.rubric-item { display: grid; grid-template-columns: minmax(80px, 140px) minmax(0, 1fr); gap: 8px; align-items: start; }
.rubric-item strong { overflow-wrap: anywhere; }
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
.copy {
  width: fit-content;
  border: 1px solid var(--line);
  background: #fff;
  color: var(--ink);
  border-radius: 6px;
  padding: 8px 12px;
  font: inherit;
  cursor: pointer;
}
.actions { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.copy-fallback {
  display: grid;
  gap: 6px;
}
.copy-fallback[hidden] { display: none; }
.copy-fallback textarea {
  min-height: 220px;
  font-size: 12px;
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
.source-card {
  border-left-color: var(--gold);
  background: #fffdf5;
}
.contribution {
  border-left: 3px solid var(--gold);
  padding-left: 9px;
  color: var(--ink);
}
.excerpt { font-size: 14px; }
.reason-list { display: grid; gap: 6px; }
.reason {
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 8px 10px;
  background: #fbfbf8;
}
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
function choiceLabel(choice) {
  const match = choices.find(([id]) => id === choice);
  return match ? match[1] : String(choice || "").replaceAll("_", " ");
}
const workKindChoices = {
  concept_review: ["accept", "reject_noisy", "split_needed", "merge_duplicate", "rename_needed", "needs_source_context"],
  cleanup_triage: ["reject_noisy", "split_needed", "merge_duplicate", "rename_needed"],
  enrichment_backlog: ["needs_source_context", "reject_noisy"],
  blocked_diagnostic: ["reject_noisy", "split_needed", "needs_source_context"]
};
let state = null;
let selected = "";
let filter = "concept_review";
let activeChoice = "";

function esc(value) {
  return String(value === null || value === undefined ? "" : value).replace(/[&<>"']/g, (ch) => ({
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

function numberOr(value, fallback) {
  return typeof value === "number" ? value : fallback;
}

function coverage(map) {
  if (!map) return "";
  return Object.keys(map).sort().map((key) => key + ":" + map[key]).join(", ");
}

function reasonLabel(code) {
  const labels = {
    "relation_neighborhood_requires_review": "Grouped by graph relations; this needs source-level validation before it can become accepted knowledge.",
    "duplicate_source_atom_support": "Multiple atoms came from the same source, so they count as trace detail, not independent support.",
    "link_only_evidence_requires_enrichment": "At least one source only contains a URL; Mindline has not read the linked content yet.",
    "no_readable_source_evidence": "At least one source has only trace metadata, not readable content.",
    "insufficient_reviewable_source_support": "Fewer than two distinct sources have readable, non-link evidence.",
    "insufficient_readable_source_kind_support": "Readable evidence comes from only one source kind, so this is not a supported cross-source concept.",
    "weak_cross_source_coherence": "The readable sources do not share enough meaning to support one cross-source concept.",
    "readable_source_outlier": "At least one readable source does not match the core concept and should be split or discarded.",
    "generic_term_bucket_requires_cleanup": "This group only shares a generic action or title word, not one coherent concept.",
    "single_source_concept": "Only one source supports this concept.",
    "single_source_kind_concept": "Only one source kind supports this concept.",
    "missing_evidence_reference": "One or more atoms are missing complete trace evidence.",
    "blocked_atom": "One or more atoms were already blocked upstream."
  };
  return labels[code] || code.replaceAll("_", " ");
}

function normalizedEvidenceText(value) {
  return String(value || "").toLowerCase().replace(/\s+/g, " ").trim();
}

function meaningfulSummary(item) {
  const summary = normalizedEvidenceText(item.summary);
  const excerpt = normalizedEvidenceText(item.excerpt);
  if (!summary || !excerpt) return item.summary || "";
  const excerptHead = excerpt.slice(0, Math.min(90, excerpt.length));
  if (summary === excerpt || summary.includes(excerptHead) || excerpt.includes(summary)) return "";
  return item.summary || "";
}

function metric(label, value) {
  return '<div class="metric"><span>' + esc(label) + '</span><strong>' + esc(value) + '</strong></div>';
}

function sourceCardCount(concept) {
  if (typeof concept.source_evidence_count === "number") return concept.source_evidence_count;
  return (concept.source_evidence || []).length;
}

function workKind(concept) {
  return concept.review_work_kind || "concept_review";
}

function workKindLabel(kind) {
  const labels = {
    concept_review: "Concept review",
    cleanup_triage: "Cleanup triage",
    enrichment_backlog: "Enrichment backlog",
    blocked_diagnostic: "Blocked diagnostic"
  };
  return labels[kind] || String(kind || "").replaceAll("_", " ");
}

function activeWorkProgress(progress) {
  const lane = ["concept_review", "cleanup_triage", "enrichment_backlog", "blocked_diagnostic"].includes(filter) ? filter : "concept_review";
  const buckets = progress.work_kind_counts || {};
  const bucket = buckets[lane] || {};
  return {
    lane,
    label: workKindLabel(lane),
    total: numberOr(bucket.total_count, lane === "concept_review" ? numberOr(progress.total_concept_count, 0) : 0),
    reviewed: numberOr(bucket.reviewed_count, lane === "concept_review" ? numberOr(progress.reviewed_concept_count, 0) : 0),
    remaining: numberOr(bucket.remaining_count, lane === "concept_review" ? numberOr(progress.remaining_concept_count, 0) : 0)
  };
}

function choicesForConcept(concept) {
  const allowed = workKindChoices[workKind(concept)] || workKindChoices.concept_review;
  return choices.filter(([id]) => allowed.includes(id));
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
  if (filter === "concept_review") return concepts.filter((concept) => workKind(concept) === "concept_review");
  if (filter === "cleanup_triage") return concepts.filter((concept) => workKind(concept) === "cleanup_triage");
  if (filter === "enrichment_backlog") return concepts.filter((concept) => workKind(concept) === "enrichment_backlog");
  if (filter === "blocked_diagnostic") return concepts.filter((concept) => workKind(concept) === "blocked_diagnostic");
  if (filter === "cross_source") return concepts.filter((concept) => concept.section === "cross_source");
  if (filter === "needs_review") return concepts.filter((concept) => concept.review_status === "needs_review" || concept.section === "needs_review");
  if (filter === "blocked") return concepts.filter((concept) => concept.review_status === "blocked" || concept.section === "blocked");
  return concepts;
}

function render() {
	const summary = state.summary;
	const progress = state.progress || {};
	const laneProgress = activeWorkProgress(progress);
	const concepts = filteredConcepts();
  document.getElementById("run").textContent = summary.corpus_id + " | " + summary.processed_source_count + "/" + summary.source_count + " sources | " + summary.scale_status;
  document.getElementById("metrics").innerHTML = [
	metric("Reviewed · " + laneProgress.label, laneProgress.reviewed + "/" + laneProgress.total),
    metric("Cross-source", summary.cross_source_concept_count),
    metric("Concepts", summary.concept_count),
    metric("Atoms", summary.atom_count),
    metric("Relations", summary.relation_count),
    metric("Compression", fmtRatio(summary.relation_review_compression_ratio, 4))
  ].join("");
	document.getElementById("list-summary").textContent = concepts.length + " shown; " + laneProgress.remaining + " remaining in " + laneProgress.label.toLowerCase();
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
      '<div class="meta">' + esc(concept.candidate_meaning || concept.grouping_rationale || "") + '</div>' +
      '<div class="meta">atoms=' + esc(concept.atom_count) + ' sources=' + esc(concept.source_count) + ' source cards=' + esc(sourceCardCount(concept)) + '</div>' +
      '<div class="tags"><span class="tag ' + esc(workKind(concept)) + '">' + esc(workKindLabel(workKind(concept))) + '</span><span class="tag ' + esc(concept.section) + '">' + esc(concept.section) + '</span><span class="tag">' + esc(coverage(concept.source_kind_coverage)) + '</span>' + reviewTag + '</div>' +
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
	const progress = (state && state.progress) || {};
	const buckets = progress.work_kind_counts || {};
	const filters = [
    ["concept_review", "Concept review"],
    ["cleanup_triage", "Cleanup"],
    ["enrichment_backlog", "Enrichment"],
    ["blocked_diagnostic", "Diagnostic"],
    ["all", "All"],
    ["unreviewed", "Unreviewed"],
    ["reviewed", "Reviewed"],
    ["cross_source", "Cross-source"],
    ["needs_review", "Needs review"],
    ["blocked", "Blocked"]
	];
	document.getElementById("filters").innerHTML = filters.map(([id, label]) => {
		const bucket = buckets[id];
		const count = bucket && typeof bucket.total_count === "number" ? " (" + bucket.total_count + ")" : "";
		return '<button class="filter' + (filter === id ? ' active' : '') + '" type="button" data-filter="' + id + '">' + esc(label + count) + '</button>';
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
  const availableChoices = choicesForConcept(concept);
  if (activeChoice && !availableChoices.find(([id]) => id === activeChoice)) activeChoice = "";
  const note = record && record.note ? record.note : "";
  const reasons = concept.reason_codes && concept.reason_codes.length ? concept.reason_codes : [];
  const reasonHTML = reasons.length ? '<div class="reason-list">' + reasons.map((code) => '<div class="reason"><strong>' + esc(reasonLabel(code)) + '</strong><div class="trace">' + esc(code) + '</div></div>').join("") + '</div>' : '<p class="muted">No quality reasons.</p>';
  const contractHTML = '<section class="contract">' +
    '<div class="contract-item"><span>Candidate Meaning</span><strong>' + esc(concept.candidate_meaning || "No candidate meaning supplied.") + '</strong></div>' +
    '<div class="contract-item"><span>Accept means</span><p>' + esc(concept.accept_meaning || "No accept contract supplied.") + '</p></div>' +
    '</section>';
  const rubricItems = (concept.decision_rubric || []).map((item) => {
    const label = item.label || choiceLabel(item.choice);
    return '<div class="rubric-item"><strong>' + esc(label) + '</strong><span>' + esc(item.criterion || "") + '</span></div>';
  }).join("");
  const rubricHTML = '<div class="rubric"><div class="rubric-title">Decision rubric</div>' + (rubricItems ? '<div class="rubric-list">' + rubricItems + '</div>' : '<p class="muted">No rubric supplied.</p>') + '</div>';
  const sourceEvidence = (concept.source_evidence || []).map((source) => {
    const flags = (source.flags || []).map((flag) => '<span class="tag">' + esc(reasonLabel(flag)) + '</span>').join("");
    const previews = (source.evidence || []).map((item) => {
      const summary = meaningfulSummary(item);
      return '<div>' +
        '<div class="title">' + esc(item.title || "Untitled evidence") + '</div>' +
        '<p class="excerpt">' + esc(item.excerpt || "No safe excerpt available.") + '</p>' +
        (summary ? '<p class="muted">' + esc(summary) + '</p>' : '') +
        '<div class="trace">' + esc(item.evidence_ref_id) + ' | lines ' + esc(item.line_start) + '-' + esc(item.line_end) + ' | ' + esc(item.content_hash) + '</div>' +
        '</div>';
    }).join("");
    return '<article class="evidence-card source-card">' +
      '<div class="tags"><span class="tag">' + esc(source.source_kind) + '</span><span class="tag">' + esc(source.source_ref) + '</span><span class="tag">atoms ' + esc(source.atom_count) + '</span><span class="tag">readable ' + esc(source.reviewable_atom_count) + '</span>' + (source.link_only ? '<span class="tag blocked">link-only</span>' : '') + flags + '</div>' +
      (source.contribution ? '<p class="contribution">' + esc(source.contribution) + '</p>' : '') +
      previews +
      '</article>';
  }).join("") || '<p class="muted">No source evidence cards.</p>';
  const evidence = (concept.representative_evidence || []).map((item) => {
    const summary = meaningfulSummary(item);
    return '<article class="evidence-card">' +
      '<div class="tags"><span class="tag">' + esc(item.source_kind) + '</span><span class="tag">' + esc(item.source_ref) + '</span><span class="tag">lines ' + esc(item.line_start) + '-' + esc(item.line_end) + '</span></div>' +
      '<div class="title">' + esc(item.title || "Untitled evidence") + '</div>' +
      '<p class="excerpt">' + esc(item.excerpt || "No safe excerpt available.") + '</p>' +
      (summary ? '<p class="muted">' + esc(summary) + '</p>' : '') +
      '<div class="trace">' + esc(item.evidence_ref_id) + ' | ' + esc(item.content_hash) + '</div>' +
      '</article>';
  }).join("") || '<p class="muted">No representative evidence previews.</p>';
  detail.innerHTML = '<div class="detail-head"><h2>' + esc(concept.title) + '</h2><p class="muted">' + esc(concept.concept_id) + '</p></div>' +
    '<div class="detail-body">' +
    '<div class="actions"><button class="copy" type="button" id="copy-concept">Copy concept</button><span class="muted" id="copy-status"></span></div>' +
    '<div class="copy-fallback" id="copy-fallback" hidden><textarea id="copy-fallback-text" aria-label="Copy packet text" readonly></textarea></div>' +
    contractHTML +
    '<section class="question"><h3>Review Question</h3><p>' + esc(concept.review_prompt || "") + '</p><p class="muted">' + esc(concept.grouping_rationale || "") + '</p></section>' +
    '<div class="grid">' +
    '<div class="box"><span>Work kind</span><strong>' + esc(workKindLabel(workKind(concept))) + '</strong></div>' +
    '<div class="box"><span>Status</span><strong>' + esc(concept.review_status) + '</strong></div>' +
    '<div class="box"><span>Section</span><strong>' + esc(concept.section) + '</strong></div>' +
    '<div class="box"><span>Atoms</span><strong>' + esc(concept.atom_count) + '</strong></div>' +
    '<div class="box"><span>Sources</span><strong>' + esc(concept.source_count) + '</strong></div>' +
    '<div class="box"><span>Coverage</span><strong>' + esc(coverage(concept.source_kind_coverage)) + '</strong></div>' +
    '<div class="box"><span>Source Cards</span><strong>' + esc((concept.source_evidence || []).length) + '</strong></div>' +
    '</div>' +
    '<section><h3>Quality Reasons</h3>' + reasonHTML + '</section>' +
    '<section class="review-form">' +
    '<h3>Decision</h3>' +
    rubricHTML +
    '<div class="decisions">' + availableChoices.map(([id, label]) => '<button class="decision' + (activeChoice === id ? ' active' : '') + '" type="button" data-choice="' + id + '">' + esc(label) + '</button>').join("") + '</div>' +
    '<textarea id="review-note" placeholder="Optional note">' + esc(note) + '</textarea>' +
    '<button class="save" type="button" id="save-review">Save decision</button>' +
    '<p class="muted">' + (record ? 'Last saved: ' + esc(record.choice) + ' at ' + esc(record.recorded_at) : 'No decision saved yet.') + '</p>' +
    '</section>' +
    '<section><h3>Source Evidence</h3><div class="evidence">' + sourceEvidence + '</div></section>' +
    '<section><h3>Atom Traces</h3><div class="evidence">' + evidence + '</div></section>' +
    '</div>';
  document.querySelectorAll("button.decision").forEach((button) => {
    button.addEventListener("click", () => {
      activeChoice = button.dataset.choice;
      renderDetail(concept);
    });
  });
  document.getElementById("save-review").addEventListener("click", () => saveReview(concept.concept_id));
  document.getElementById("copy-concept").addEventListener("click", () => copyConcept(concept));
}

function conceptCopyText(concept) {
  const record = recordsByConcept()[concept.concept_id];
  const lines = [
    "Mindline concept review packet",
    "",
    "Concept: " + (concept.title || ""),
    "Concept ID: " + concept.concept_id,
    "Review work kind: " + workKind(concept),
    "Section: " + concept.section,
    "Review status: " + concept.review_status,
    "Decision: " + (record ? record.choice : "unreviewed"),
    "Atoms: " + concept.atom_count,
    "Sources: " + concept.source_count,
    "Coverage: " + coverage(concept.source_kind_coverage),
    "Reasons: " + ((concept.reason_codes || []).map((code) => reasonLabel(code) + " [" + code + "]").join("; ") || "none"),
    "",
    "Candidate meaning:",
    concept.candidate_meaning || "",
    "",
    "Accept means:",
    concept.accept_meaning || "",
    "",
    "Review question:",
    concept.review_prompt || "",
    "",
    "Grouping rationale:",
    concept.grouping_rationale || "",
    ""
  ];
  lines.push("Decision rubric:");
  (concept.decision_rubric || []).forEach((item) => {
    lines.push("- " + (item.label || choiceLabel(item.choice)) + ": " + (item.criterion || ""));
  });
  if (!(concept.decision_rubric || []).length) {
    lines.push("none");
  }
  lines.push("");
  if (record && record.note) {
    lines.push("Reviewer note:", record.note, "");
  }
  lines.push("Source evidence:");
  (concept.source_evidence || []).forEach((source, index) => {
    lines.push(
      "",
      String(index + 1) + ". " + (source.source_kind || "source") + " " + (source.source_ref || ""),
      "Atoms from this source: " + source.atom_count,
      "Readable non-link atoms: " + source.reviewable_atom_count,
      "Contribution: " + (source.contribution || ""),
      "Flags: " + ((source.flags || []).map((flag) => reasonLabel(flag) + " [" + flag + "]").join("; ") || "none")
    );
    (source.evidence || []).forEach((item, itemIndex) => {
      const summary = meaningfulSummary(item);
      lines.push(
        "  " + String(itemIndex + 1) + ") lines " + item.line_start + "-" + item.line_end,
        "     Title: " + (item.title || ""),
        "     Excerpt: " + (item.excerpt || "")
      );
      if (summary) {
        lines.push("     Summary: " + summary);
      }
      lines.push(
        "     Trace: " + (item.evidence_ref_id || "") + " | " + (item.content_hash || "")
      );
    });
  });
  lines.push("", "Atom traces:");
  (concept.representative_evidence || []).forEach((item, index) => {
    const summary = meaningfulSummary(item);
    lines.push(
      "",
      String(index + 1) + ". " + (item.source_kind || "source") + " " + (item.source_ref || "") + " lines " + item.line_start + "-" + item.line_end,
      "Title: " + (item.title || ""),
      "Excerpt: " + (item.excerpt || "")
    );
    if (summary) {
      lines.push("Summary: " + summary);
    }
    lines.push("Trace: " + (item.evidence_ref_id || "") + " | " + (item.content_hash || ""));
  });
  return lines.join("\n");
}

function copyConcept(concept) {
  const text = conceptCopyText(concept);
  const status = document.getElementById("copy-status");
  hideCopyFallback();
  const done = () => { status.textContent = "Copied"; };
  const failed = () => showCopyFallback(text, status);
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(() => fallbackCopy(text, done, failed));
    return;
  }
  fallbackCopy(text, done, failed);
}

function hideCopyFallback() {
  const fallback = document.getElementById("copy-fallback");
  const area = document.getElementById("copy-fallback-text");
  if (fallback) fallback.hidden = true;
  if (area) area.value = "";
}

function showCopyFallback(text, status) {
  const fallback = document.getElementById("copy-fallback");
  const area = document.getElementById("copy-fallback-text");
  if (!fallback || !area) {
    status.textContent = "Copy unavailable";
    return;
  }
  area.value = text;
  fallback.hidden = false;
  area.focus();
  area.select();
  status.textContent = "Packet shown below";
}

function fallbackCopy(text, done, failed) {
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "readonly");
  area.style.position = "fixed";
  area.style.left = "-9999px";
  document.body.appendChild(area);
  area.select();
  try {
    document.execCommand("copy") ? done() : failed();
  } catch (_) {
    failed();
  }
  document.body.removeChild(area);
}

function saveReview(conceptID) {
  if (!activeChoice) return;
  const concept = (state.index.concepts || []).find((item) => item.concept_id === conceptID);
  fetch("/api/reviews", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Mindline-Review-Token": reviewToken },
    body: JSON.stringify({
      concept_id: conceptID,
      review_work_kind: concept ? workKind(concept) : "",
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
