package cli

import (
	"html/template"
	"net/http"

	"github.com/synergyai-os/Mindline/internal/documents"
)

type corpusConceptUIState struct {
	SchemaVersion string                         `json:"schema_version"`
	Summary       documents.CorpusConceptSummary `json:"summary"`
	Index         documents.CorpusConceptIndex   `json:"index"`
}

func newCorpusConceptUIHandlerWithAllowedHosts(root string, allowedHosts []string) http.Handler {
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
		if err := corpusConceptUITemplate.Execute(w, nil); err != nil {
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
	return corpusConceptUIState{
		SchemaVersion: "corpus-concept-ui-state/v0.1",
		Summary:       summary,
		Index:         index,
	}, nil
}

var corpusConceptUITemplate = template.Must(template.New("corpus-concept-ui").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mindline Concepts</title>
<style>
:root {
  color-scheme: light;
  --bg: #f6f7f4;
  --panel: #ffffff;
  --ink: #17211f;
  --muted: #68726f;
  --line: #d8ded7;
  --accent: #0f6b5d;
  --soft: #edf4ef;
  --warn: #875021;
  --bad: #983141;
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
  gap: 12px;
}
h1, h2, h3, p { margin: 0; letter-spacing: 0; }
h1 { font-size: 20px; }
.run { color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
.metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(126px, 1fr));
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
.workspace {
  display: grid;
  grid-template-columns: minmax(280px, 420px) minmax(0, 1fr);
  gap: 16px;
  padding: 18px 22px;
}
.list, .detail {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  min-width: 0;
}
.list {
  overflow: hidden;
}
.list-head, .detail-head {
  padding: 14px 16px;
  border-bottom: 1px solid var(--line);
  display: grid;
  gap: 4px;
}
.concepts {
  max-height: calc(100vh - 220px);
  overflow: auto;
}
button.concept {
  width: 100%;
  border: 0;
  border-bottom: 1px solid var(--line);
  background: #fff;
  text-align: left;
  padding: 12px 14px;
  display: grid;
  gap: 6px;
  cursor: pointer;
  font: inherit;
}
button.concept:hover, button.concept.active { background: var(--soft); }
.title { font-weight: 700; overflow-wrap: anywhere; }
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
.tag.cross_source { color: var(--accent); }
.tag.needs_review { color: var(--warn); }
.tag.blocked { color: var(--bad); }
.detail-body {
  padding: 16px;
  display: grid;
  gap: 16px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
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
.refs {
  display: grid;
  gap: 8px;
}
.ref {
  border-left: 3px solid var(--accent);
  background: #fbfbf8;
  padding: 9px 11px;
  overflow-wrap: anywhere;
  font-size: 13px;
}
.empty {
  padding: 36px 18px;
  color: var(--muted);
  text-align: center;
}
@media (max-width: 860px) {
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
      <h1>Mindline Concepts</h1>
      <div class="run" id="run">Loading...</div>
    </div>
    <div class="metrics" id="metrics"></div>
  </header>
  <div class="workspace">
    <section class="list">
      <div class="list-head">
        <h2>Concepts</h2>
        <p class="muted" id="list-summary"></p>
      </div>
      <div class="concepts" id="concepts"></div>
    </section>
    <section class="detail" id="detail"></section>
  </div>
</main>
<script>
let state = null;
let selected = "";

function fmtRatio(value) {
  if (typeof value !== "number") return "0.00";
  return value.toFixed(2);
}

function coverage(map) {
  if (!map) return "";
  return Object.keys(map).sort().map((key) => key + ":" + map[key]).join(", ");
}

function metric(label, value) {
  return '<div class="metric"><span>' + label + '</span><strong>' + value + '</strong></div>';
}

function render() {
  const summary = state.summary;
  const concepts = state.index.concepts || [];
  document.getElementById("run").textContent = summary.corpus_id + " | " + summary.processed_source_count + "/" + summary.source_count + " sources | " + summary.scale_status;
  document.getElementById("metrics").innerHTML = [
    metric("Concepts", summary.concept_count),
    metric("Cross-source", summary.cross_source_concept_count),
    metric("Atoms", summary.atom_count),
    metric("Relations", summary.relation_count),
    metric("Compression", fmtRatio(summary.relation_review_compression_ratio)),
    metric("Burden", fmtRatio(summary.concept_review_burden_ratio))
  ].join("");
  document.getElementById("list-summary").textContent = concepts.length + " review groups; coverage " + fmtRatio(summary.atom_coverage_ratio);
  if (!selected && concepts.length) selected = concepts[0].concept_id;
  document.getElementById("concepts").innerHTML = concepts.map((concept) => {
    const active = concept.concept_id === selected ? " active" : "";
    return '<button class="concept' + active + '" type="button" data-id="' + concept.concept_id + '">' +
      '<div class="title">' + concept.title + '</div>' +
      '<div class="meta">atoms=' + concept.atom_count + ' sources=' + concept.source_count + ' evidence=' + concept.evidence_reference_count + '</div>' +
      '<div class="tags"><span class="tag ' + concept.section + '">' + concept.section + '</span><span class="tag">' + coverage(concept.source_kind_coverage) + '</span></div>' +
      '</button>';
  }).join("") || '<div class="empty">No concepts found.</div>';
  document.querySelectorAll("button.concept").forEach((button) => {
    button.addEventListener("click", () => {
      selected = button.dataset.id;
      render();
    });
  });
  renderDetail(concepts.find((concept) => concept.concept_id === selected));
}

function renderDetail(concept) {
  const detail = document.getElementById("detail");
  if (!concept) {
    detail.innerHTML = '<div class="empty">Select a concept.</div>';
    return;
  }
  const reasons = concept.reason_codes && concept.reason_codes.length ? concept.reason_codes.join(", ") : "none";
  const refs = (concept.evidence_refs || []).slice(0, 24).map((ref) => {
    return '<div class="ref"><strong>' + ref.evidence_ref_id + '</strong><br>source=' + ref.source_id + ' lines=' + ref.line_start + '-' + ref.line_end + ' hash=' + ref.content_hash + '</div>';
  }).join("") || '<p class="muted">No evidence refs.</p>';
  detail.innerHTML = '<div class="detail-head"><h2>' + concept.title + '</h2><p class="muted">' + concept.concept_id + '</p></div>' +
    '<div class="detail-body">' +
    '<div class="grid">' +
    '<div class="box"><span>Section</span><strong>' + concept.section + '</strong></div>' +
    '<div class="box"><span>Status</span><strong>' + concept.review_status + '</strong></div>' +
    '<div class="box"><span>Atoms</span><strong>' + concept.atom_count + '</strong></div>' +
    '<div class="box"><span>Sources</span><strong>' + concept.source_count + '</strong></div>' +
    '<div class="box"><span>Coverage</span><strong>' + coverage(concept.source_kind_coverage) + '</strong></div>' +
    '<div class="box"><span>Reasons</span><strong>' + reasons + '</strong></div>' +
    '</div>' +
    '<h3>Evidence</h3><div class="refs">' + refs + '</div>' +
    '</div>';
}

fetch("/api/state")
  .then((response) => response.json())
  .then((data) => { state = data; render(); })
  .catch((error) => {
    document.getElementById("detail").innerHTML = '<div class="empty">' + error + '</div>';
  });
</script>
</body>
</html>`))
