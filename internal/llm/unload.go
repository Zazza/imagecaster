package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *Client) IdleModelServerURL() string {
	return strings.TrimSuffix(strings.TrimRight(c.url(), "/"), "/api/llm")
}

func (c *Client) UnloadModel(ctx context.Context, model string) error {
	if strings.TrimSpace(model) == "" {
		return errors.New("unload model: model is required")
	}
	body, err := json.Marshal(ollamaUnloadRequest{Model: model})
	if err != nil {
		return fmt.Errorf("unload model: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url()+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unload model: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unload model: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxLLMResponseBodySize))
	if err != nil {
		return fmt.Errorf("unload model response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unload model: status %d: %s", response.StatusCode, raw)
	}
	var result struct {
		Done  bool   `json:"done"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("unload model response: %w", err)
	}
	if result.Error != "" || !result.Done {
		return fmt.Errorf("unload model not acknowledged: %s", result.Error)
	}
	return nil
}
