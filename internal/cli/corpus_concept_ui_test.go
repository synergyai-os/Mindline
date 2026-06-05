package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestDocumentsConceptIndexCLIAndUIServeState(t *testing.T) {
	root := t.TempDir()
	writeConceptCLIFixture(t, root)
	out := filepath.Join(root, "concept-out")
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "concept-index", root, "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected concept-index exit %d got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	var summary documents.CorpusConceptSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if summary.ConceptCount == 0 || summary.CrossSourceConceptCount == 0 {
		t.Fatalf("expected cross-source concept summary: %+v", summary)
	}

	handler := newCorpusConceptUIHandlerWithAllowedHosts(filepath.Join(out, documents.CorpusConceptsDirName), []string{"127.0.0.1:8788"})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8788/api/state", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected UI state 200 got %d body=%s", rec.Code, rec.Body.String())
	}
	var state corpusConceptUIState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if len(state.Index.Concepts) == 0 {
		t.Fatalf("expected UI concepts")
	}
}

func writeConceptCLIFixture(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{
		filepath.Join(root, "corpus-pressure"),
		filepath.Join(root, "corpus-graph", "atoms"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir fixture dir: %v", err)
		}
	}
	pressure := documents.CorpusPressureSummary{
		SchemaVersion:            documents.CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-cli-concepts",
		SourceCount:              2,
		ProcessedSourceCount:     2,
		ScaleStatus:              "scale_complete",
		GraphSummaryPath:         "corpus-graph/graph-summary.json",
		CorpusFingerprint:        "corpus-cli",
		CommandConfigFingerprint: "config-cli",
		ReplayFingerprint:        "pressure-cli",
	}
	writeCLITestJSON(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), pressure)
	atoms := []documents.CorpusGraphAtom{
		cliConceptAtom("atom-a", "gmail-source", "Concept review combines repeated methodology"),
		cliConceptAtom("atom-b", "slack-source", "Repeated methodology belongs in concept review"),
	}
	graph := documents.CorpusGraphSummary{
		SchemaVersion:     documents.CorpusGraphSummarySchemaVersion,
		CorpusID:          pressure.CorpusID,
		SourceCount:       2,
		AtomCount:         len(atoms),
		RelationCount:     25,
		ReplayFingerprint: "graph-cli",
		Atoms: []documents.CorpusGraphSummaryAtom{
			{AtomID: atoms[0].AtomID, SourceID: atoms[0].SourceID, CandidateKind: atoms[0].CandidateKind, ReviewStatus: atoms[0].ReviewStatus, AtomPath: "atoms/atom-a.json"},
			{AtomID: atoms[1].AtomID, SourceID: atoms[1].SourceID, CandidateKind: atoms[1].CandidateKind, ReviewStatus: atoms[1].ReviewStatus, AtomPath: "atoms/atom-b.json"},
		},
	}
	writeCLITestJSON(t, filepath.Join(root, "corpus-graph", "graph-summary.json"), graph)
	writeCLITestJSON(t, filepath.Join(root, "corpus-graph", "atoms", "atom-a.json"), atoms[0])
	writeCLITestJSON(t, filepath.Join(root, "corpus-graph", "atoms", "atom-b.json"), atoms[1])
}

func cliConceptAtom(id, sourceID, title string) documents.CorpusGraphAtom {
	return documents.CorpusGraphAtom{
		SchemaVersion:    documents.CorpusGraphAtomSchemaVersion,
		AtomID:           id,
		CorpusID:         "corpus-cli-concepts",
		SourceID:         sourceID,
		SourceKind:       "markdown",
		SourceDocumentID: sourceID,
		CandidateKind:    documents.SemanticCandidateKindTopic,
		ReviewStatus:     documents.ReviewStatusReady,
		Confidence:       documents.ConfidenceMedium,
		Title:            title,
		Summary:          title,
		LineStart:        1,
		LineEnd:          2,
		ContentHash:      "hash-" + id,
	}
}
