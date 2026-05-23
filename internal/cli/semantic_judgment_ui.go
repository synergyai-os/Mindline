package cli

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
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
	CandidateID string `json:"candidate_id"`
	Choice      string `json:"choice"`
	Note        string `json:"note"`
	ReviewerID  string `json:"reviewer_id"`
}

func newSemanticJudgmentUIHandler(root, reviewerID string) http.Handler {
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
		if err := semanticJudgmentUITemplate.Execute(w, nil); err != nil {
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
			CandidateID: post.CandidateID,
			Choice:      documents.SemanticJudgmentChoice(post.Choice),
			Note:        post.Note,
			ReviewerID:  recordReviewer,
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
	return mux
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
h1, h2, h3 { margin: 0; letter-spacing: 0; }
h1 { font-size: 20px; }
.run { color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
.metrics {
  display: grid;
  grid-template-columns: repeat(8, minmax(86px, 1fr));
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
.summary { font-size: 16px; }
.evidence-list { display: grid; gap: 12px; }
.evidence {
  border-left: 3px solid var(--accent);
  background: #fbfbf9;
  padding: 10px 12px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 14px;
}
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
.decision-controls { display: grid; gap: 8px; }
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
@media (max-width: 920px) {
  .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .workspace { grid-template-columns: 1fr; padding: 12px; }
  header { padding: 14px 12px; }
}
</style>
</head>
<body>
<main>
  <header>
    <div class="topline">
      <h1>Mindline Review</h1>
      <div class="run" id="run-context">Loading...</div>
    </div>
    <div class="metrics" id="metrics" aria-live="polite"></div>
    <div class="progress" aria-label="review progress"><div id="progress-bar"></div></div>
  </header>
  <div class="workspace">
    <section id="current-candidate" aria-live="polite"></section>
    <aside>
      <h2>Decision</h2>
      <textarea id="note" placeholder="Optional note"></textarea>
      <div class="decision-controls" id="decision-controls"></div>
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
let currentCandidateId = "";

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
  const summary = state.summary;
  const page = state.page;
  document.getElementById("run-context").textContent = "Run " + summary.run_id + " · " + summary.source_count + " source(s)";
  const metricRows = [
    ["Total", summary.candidate_count],
    ["Judged", summary.judged_count],
    ["Remaining", summary.remaining_count],
    ["Accepted", summary.accepted_count],
    ["Rejected", summary.rejected_count],
    ["Unclear", summary.unclear_count],
    ["Duplicate", summary.duplicate_count],
    ["Wrong kind", summary.wrong_kind_count]
  ];
  document.getElementById("metrics").innerHTML = metricRows.map(([label, value]) => "<div class=\"metric\"><span>" + escapeHtml(label) + "</span><strong>" + escapeHtml(value) + "</strong></div>").join("");
  const pct = summary.candidate_count === 0 ? 100 : Math.round((summary.judged_count / summary.candidate_count) * 100);
  document.getElementById("progress-bar").style.width = pct + "%";
  document.getElementById("note").value = "";
  document.getElementById("status").textContent = "";
  if (page.done || !page.item) {
    currentCandidateId = "";
    document.getElementById("current-candidate").innerHTML = "<div class=\"done\"><h2>Review complete</h2><p class=\"muted\">" + escapeHtml(summary.judged_count) + "/" + escapeHtml(summary.candidate_count) + " judged. Precision estimate: " + escapeHtml(summary.precision_estimate) + ".</p></div>";
    document.getElementById("decision-controls").innerHTML = "";
    return;
  }
  const item = page.item;
  currentCandidateId = item.candidate_id;
  const ranges = (item.evidence_ranges || []).map(range => "<span class=\"tag\">" + escapeHtml(range.structure_node_id) + " lines " + escapeHtml(range.line_start) + "-" + escapeHtml(range.line_end) + "</span>").join("");
  const excerpts = (item.evidence_excerpts || []).map(excerpt => {
    if (excerpt.unavailable) {
      return "<div class=\"evidence muted\">" + escapeHtml(excerpt.unavailable_reason || "source excerpt unavailable") + "</div>";
    }
    return "<div class=\"evidence\"><strong>" + escapeHtml(excerpt.source_label) + " lines " + escapeHtml(excerpt.line_start) + "-" + escapeHtml(excerpt.line_end) + "</strong>\n" + escapeHtml(excerpt.text) + "</div>";
  }).join("") || "<div class=\"evidence muted\">No source excerpts available.</div>";
  const sourceTag = item.source_document_id ? "<span class=\"tag\">" + escapeHtml(item.source_document_id) + "</span>" : "";
  const rangesHtml = ranges || "<span class=\"tag\">No evidence ranges</span>";
  const relationIds = (item.relation_ids || []).map(id => "<span class=\"tag\">" + escapeHtml(id) + "</span>").join("") || "<span class=\"tag\">No relation ids</span>";
  const blockers = (item.blockers || []).map(blocker => "<div class=\"evidence warn\"><strong>" + escapeHtml(blocker.code || "blocker") + "</strong>\n" + escapeHtml(blocker.message || "No blocker message") + "</div>").join("") || "<div class=\"evidence muted\">No blockers</div>";
  document.getElementById("current-candidate").innerHTML =
    "<div class=\"candidate-head\">" +
      "<h2>" + escapeHtml(item.title || "Untitled candidate") + "</h2>" +
      "<div class=\"tags\">" +
        "<span class=\"tag\">" + escapeHtml(item.candidate_kind) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(item.confidence) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(item.review_status) + "</span>" +
        "<span class=\"tag\">" + escapeHtml(item.candidate_id) + "</span>" +
        sourceTag +
      "</div>" +
    "</div>" +
    "<div class=\"candidate-body\">" +
      "<div><h3>Summary</h3><p class=\"summary\">" + escapeHtml(item.summary || "No summary") + "</p></div>" +
      "<div><h3>Evidence</h3><div class=\"tags\">" + rangesHtml + "</div></div>" +
      "<div class=\"evidence-list\">" + excerpts + "</div>" +
      "<div><h3>Relation ids</h3><div class=\"tags\">" + relationIds + "</div></div>" +
      "<div><h3>Blockers</h3><div class=\"evidence-list\">" + blockers + "</div></div>" +
    "</div>";
  document.getElementById("decision-controls").innerHTML = choices.map(([choice, label, klass]) => "<button class=\"" + escapeHtml(klass) + "\" data-choice=\"" + escapeHtml(choice) + "\">" + escapeHtml(label) + "</button>").join("");
  for (const button of document.querySelectorAll("[data-choice]")) {
    button.addEventListener("click", () => submitChoice(button.dataset.choice));
  }
}

async function submitChoice(choice) {
  if (!currentCandidateId) return;
  document.getElementById("status").textContent = "Saving...";
  for (const button of document.querySelectorAll("[data-choice]")) button.disabled = true;
  const response = await fetch("/api/judgments", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ candidate_id: currentCandidateId, choice, note: document.getElementById("note").value })
  });
  if (!response.ok) {
    document.getElementById("status").textContent = await response.text();
    for (const button of document.querySelectorAll("[data-choice]")) button.disabled = false;
    return;
  }
  render(await response.json());
}

loadState().catch(error => {
  document.getElementById("current-candidate").innerHTML = "<div class=\"done\"><h2>Unable to load review</h2><p class=\"muted\">" + escapeHtml(error.message) + "</p></div>";
});
</script>
</body>
</html>`))
