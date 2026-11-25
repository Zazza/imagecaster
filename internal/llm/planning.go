package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const PlanningContextTokens = 8192
const planningMaxThreads = 4
const planningKeepAlive = 30 * time.Second

type planningClient struct{ *Client }

func ForPlanning(service Service) Service {
	c, ok := service.(*Client)
	if !ok || c == nil {
		return service
	}
	c.mu.RLock()
	clone := &Client{baseURL: c.baseURL, cfgSource: c.cfgSource, httpClient: c.httpClient}
	c.mu.RUnlock()
	return &planningClient{clone}
}

func (c *planningClient) Chat(model, system, user string, temperature float64, maxTokens int) (string, error) {
	base, cfg := c.snapshot(ModeGenerate)
	if cfg.NumThread <= 0 || cfg.NumThread > planningMaxThreads {
		cfg.NumThread = planningMaxThreads
	}
	cfg.NumCtx = PlanningContextTokens
	keepAlive, err := time.ParseDuration(cfg.KeepAlive)
	if err != nil || keepAlive < 0 || keepAlive > planningKeepAlive || cfg.KeepAlive == "" {
		cfg.KeepAlive = planningKeepAlive.String()
	}
	options := cfg.ollamaOptions(map[string]any{"temperature": temperature, "num_predict": maxTokens})
	body, err := json.Marshal(struct {
		Model     string         `json:"model"`
		Messages  []Message      `json:"messages"`
		Stream    bool           `json:"stream"`
		Think     bool           `json:"think"`
		Options   map[string]any `json:"options"`
		KeepAlive string         `json:"keep_alive"`
	}{Model: model, Messages: []Message{{Role: "system", Content: system}, {Role: "user", Content: user}}, Options: options, KeepAlive: cfg.KeepAlive})
	if err != nil {
		return "", fmt.Errorf("planning request: %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("planning request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	log.Printf("[LLM planning] POST %s/api/chat model=%s max_tokens=%d threads=%d context=%d", base, model, maxTokens, cfg.NumThread, cfg.NumCtx)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("planning request: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxLLMResponseBodySize))
	if err != nil {
		return "", fmt.Errorf("planning response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("planning API error %d: %s", response.StatusCode, string(raw))
	}
	var parsed ollamaVisionResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("planning decode: %w", err)
	}
	return strings.TrimSpace(stripThinkTags(parsed.Message.Content)), nil
}
