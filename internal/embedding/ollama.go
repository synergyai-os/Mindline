package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultOllamaURL   = "http://127.0.0.1:11434"
	defaultOllamaModel = "embeddinggemma:latest"
	maximumResponse    = 128 << 20
)

type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllama(baseURL, model string, client *http.Client) (*Ollama, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOllamaURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Ollama endpoint must be loopback HTTP")
	}
	if strings.TrimSpace(model) == "" {
		model = defaultOllamaModel
	}
	if len([]rune(model)) > 256 {
		return nil, errors.New("Ollama model is invalid")
	}
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &Ollama{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		model:   strings.TrimSpace(model),
		client:  client,
	}, nil
}

func (ollama *Ollama) ModelID() string {
	return "ollama/" + ollama.model + "/retrieval-input-v0.2"
}

func (ollama *Ollama) Embed(ctx context.Context, inputs []string) ([][]float64, error) {
	if len(inputs) == 0 || len(inputs) > 128 {
		return nil, errors.New("embedding batch is empty or too large")
	}
	for _, input := range inputs {
		if strings.TrimSpace(input) == "" || len([]rune(input)) > 64<<10 {
			return nil, errors.New("embedding input is empty or too large")
		}
	}
	body, err := json.Marshal(map[string]any{
		"model": ollama.model,
		"input": inputs,
	})
	if err != nil {
		return nil, errors.New("encode embedding request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ollama.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create embedding request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := ollama.client.Do(request)
	if err != nil {
		return nil, errors.New("local embedding provider unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumResponse+1))
	if err != nil || len(data) > maximumResponse {
		return nil, errors.New("read embedding response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("local embedding provider rejected the request")
	}
	var payload struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Embeddings) != len(inputs) {
		return nil, errors.New("invalid embedding response")
	}
	dimensions := 0
	for _, vector := range payload.Embeddings {
		if len(vector) == 0 || len(vector) > 16_384 {
			return nil, errors.New("invalid embedding dimensions")
		}
		if dimensions == 0 {
			dimensions = len(vector)
		} else if dimensions != len(vector) {
			return nil, errors.New("embedding dimensions changed within batch")
		}
		if _, err := Cosine(vector, vector); err != nil {
			return nil, err
		}
	}
	return payload.Embeddings, nil
}

func (ollama *Ollama) EmbedQuery(ctx context.Context, input string) ([]float64, error) {
	input = strings.TrimSpace(input)
	if ollama.usesEmbeddingGemmaPrompts() {
		input = "task: search result | query: " + input
	}
	vectors, err := ollama.Embed(ctx, []string{input})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (ollama *Ollama) EmbedDocuments(ctx context.Context, inputs []string) ([][]float64, error) {
	prepared := make([]string, 0, len(inputs))
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if ollama.usesEmbeddingGemmaPrompts() {
			input = "title: none | text: " + input
		}
		prepared = append(prepared, input)
	}
	return ollama.Embed(ctx, prepared)
}

func (ollama *Ollama) usesEmbeddingGemmaPrompts() bool {
	name := strings.ToLower(strings.TrimSpace(ollama.model))
	return name == "embeddinggemma" || strings.HasPrefix(name, "embeddinggemma:")
}
