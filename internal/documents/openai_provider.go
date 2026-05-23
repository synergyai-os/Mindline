package documents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const openAIResponsesURL = "https://api.openai.com/v1/responses"
const openAISemanticMaxOutputTokens = 8192
const openAISemanticTimeout = 90 * time.Second

type OpenAIProvider struct {
	apiKey string
	model  string
	client *http.Client
}

func NewOpenAIProvider(apiKey, model string, transport http.RoundTripper) OpenAIProvider {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return OpenAIProvider{
		apiKey: strings.TrimSpace(apiKey),
		model:  strings.TrimSpace(model),
		client: &http.Client{
			Transport: transport,
			Timeout:   openAISemanticTimeout,
		},
	}
}

func (provider OpenAIProvider) Classify(request LLMSemanticRequest) (llmSemanticResponse, error) {
	if provider.apiKey == "" {
		return llmSemanticResponse{}, fmt.Errorf("missing OpenAI API key")
	}
	if provider.model == "" {
		return llmSemanticResponse{}, fmt.Errorf("missing OpenAI model")
	}
	body := map[string]any{
		"model": provider.model,
		"input": BuildLLMSemanticPrompt(request),
		"text": map[string]any{
			"format": map[string]any{"type": "text"},
		},
		"store":             false,
		"max_output_tokens": openAISemanticMaxOutputTokens,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return llmSemanticResponse{}, err
	}
	httpRequest, err := http.NewRequest(http.MethodPost, openAIResponsesURL, bytes.NewReader(data))
	if err != nil {
		return llmSemanticResponse{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+provider.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := provider.client.Do(httpRequest)
	if err != nil {
		return llmSemanticResponse{}, err
	}
	defer httpResponse.Body.Close()
	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return llmSemanticResponse{}, err
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return llmSemanticResponse{}, fmt.Errorf("OpenAI response status %d", httpResponse.StatusCode)
	}
	text, err := extractOpenAIOutputText(responseBody)
	if err != nil {
		return llmSemanticResponse{}, err
	}
	var semanticResponse llmSemanticResponse
	if err := json.Unmarshal([]byte(text), &semanticResponse); err != nil {
		return llmSemanticResponse{}, fmt.Errorf("parse OpenAI semantic response: %w", err)
	}
	return semanticResponse, nil
}

func extractOpenAIOutputText(data []byte) (string, error) {
	var response struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("decode OpenAI response: %w", err)
	}
	for _, output := range response.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return strings.TrimSpace(content.Text), nil
			}
		}
	}
	return "", fmt.Errorf("OpenAI response missing output text")
}
