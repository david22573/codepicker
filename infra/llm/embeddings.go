package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

// EmbeddingClient handles vector generation
type EmbeddingClient struct {
	apiKey  string
	model   string
	client  *http.Client
	baseURL string
}

func NewEmbeddingClient(apiKey, model string) *EmbeddingClient {
	return &EmbeddingClient{
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://openrouter.ai/api/v1/embeddings",
	}
}

// [FIX] Added helper method for single string embedding (Required by SearchTool)
func (e *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, _, err := e.CreateEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding returned from API")
	}
	return vectors[0], nil
}

type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// CreateEmbeddings generates vectors for a batch of strings
func (e *EmbeddingClient) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, int, error) {
	if len(texts) == 0 {
		return nil, 0, nil
	}

	reqBody := embeddingRequest{
		Input: texts,
		Model: e.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, errors.NewSystem("llm.Embeddings", "marshal failed", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, 0, errors.NewLLM("llm.Embeddings", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, errors.NewLLM("llm.Embeddings", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body)))
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, 0, err
	}

	if embedResp.Error != nil {
		return nil, 0, errors.NewLLM("llm.Embeddings", fmt.Errorf("api error: %s", embedResp.Error.Message))
	}

	// Ensure results are sorted by index
	results := make([][]float32, len(texts))
	for _, item := range embedResp.Data {
		if item.Index < len(results) {
			results[item.Index] = item.Embedding
		}
	}

	return results, embedResp.Usage.TotalTokens, nil
}
