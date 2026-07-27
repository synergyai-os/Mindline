package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddingGemmaRetrievalInputsUseAsymmetricPrompts(t *testing.T) {
	var observed [][]string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		observed = append(observed, body.Input)
		vectors := make([][]float64, len(body.Input))
		for index := range vectors {
			vectors[index] = []float64{1, 0}
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"embeddings": vectors})
	}))
	defer server.Close()

	adapter, err := NewOllama(server.URL, "embeddinggemma:latest", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.EmbedDocuments(context.Background(), []string{"saved lesson"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.EmbedQuery(context.Background(), "what did I learn?"); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 2 ||
		observed[0][0] != "title: none | text: saved lesson" ||
		observed[1][0] != "task: search result | query: what did I learn?" ||
		!strings.HasSuffix(adapter.ModelID(), "/retrieval-input-v0.2") {
		t.Fatalf("retrieval prompt/profile mismatch: observed=%v model=%s", observed, adapter.ModelID())
	}
}
