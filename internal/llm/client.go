package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Zazza/imagecaster/internal/promptutil"
)

const maxLLMResponseBodySize = 10 * 1024 * 1024

type Client struct {
	mu         sync.RWMutex
	baseURL    string
	cfgSource  ConfigSource
	httpClient *http.Client
}

var _ Service = (*Client)(nil)

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLPart `json:"image_url,omitempty"`
}

type ImageURLPart struct {
	URL string `json:"url"`
}

type ChatOptions struct {
	NumCtx    int     `json:"num_ctx,omitempty"`
	TopP      float64 `json:"top_p,omitempty"`
	NumThread int     `json:"num_thread,omitempty"`
}

type ChatRequest struct {
	Model            string       `json:"model"`
	Messages         []Message    `json:"messages"`
	Temperature      float64      `json:"temperature"`
	MaxTokens        int          `json:"max_tokens,omitempty"`
	TopP             float64      `json:"top_p,omitempty"`
	FrequencyPenalty float64      `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64      `json:"presence_penalty,omitempty"`
	Stream           bool         `json:"stream"`
	KeepAlive        string       `json:"keep_alive,omitempty"`
	Options          *ChatOptions `json:"options,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) Chat(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}
	reqBody, base := c.chatRequest(model, messages, temperature, maxTokens)
	logPrefix := fmt.Sprintf("[LLM] POST %s/v1/chat/completions model=%s max_tokens=%d temperature=%.1f prompt_len=%d", base, model, reqBody.MaxTokens, temperature, len(userMessage))
	return c.doChatRequest(base, reqBody, logPrefix)
}

func (c *Client) chatRequest(model string, messages []Message, temperature float64, maxTokens int) (ChatRequest, string) {
	base, cfg := c.snapshot(ModeGenerate)
	req := ChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		TopP:        cfg.TopP,
		KeepAlive:   cfg.KeepAlive,
	}
	if cfg.NumCtx > 0 || cfg.TopP > 0 || cfg.NumThread > 0 {
		req.Options = &ChatOptions{NumCtx: cfg.NumCtx, TopP: cfg.TopP, NumThread: cfg.NumThread}
	}
	return req, base
}

func (c *Client) snapshot(mode string) (string, BackendConfig) {
	c.mu.RLock()
	base, src := c.baseURL, c.cfgSource
	c.mu.RUnlock()
	var cfg BackendConfig
	if src != nil {
		cfg = src(mode)
	}
	return base, cfg
}

func (c *Client) url() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *Client) ChatVision(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
	return c.chatVisionOllama(model, systemPrompt, userText, imageBase64, temperature, maxTokens)
}

type ollamaVisionMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaVisionRequest struct {
	Model    string                `json:"model"`
	Messages []ollamaVisionMessage `json:"messages"`
	Stream   bool                  `json:"stream"`
	Options  map[string]any        `json:"options,omitempty"`
}

type ollamaVisionResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func (c *Client) chatVisionOllama(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
	msgs := make([]ollamaVisionMessage, 0)
	if systemPrompt != "" {
		msgs = append(msgs, ollamaVisionMessage{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, ollamaVisionMessage{
		Role:    "user",
		Content: userText,
		Images:  []string{imageBase64},
	})
	base, cfg := c.snapshot(ModeAnalyze)
	if cfg.NumCtx < 8192 {
		cfg.NumCtx = 8192
	}
	opts := cfg.ollamaOptions(map[string]any{"temperature": temperature})
	if maxTokens > 0 {
		opts["num_predict"] = maxTokens
	}
	body := ollamaVisionRequest{
		Model:    model,
		Messages: msgs,
		Stream:   false,
		Options:  opts,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("vision request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("vision request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("vision request: status %d: %s", resp.StatusCode, string(body))
	}
	var result ollamaVisionResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&result); err != nil {
		return "", fmt.Errorf("vision decode: %w", err)
	}
	content := stripThinkTags(result.Message.Content)
	return strings.TrimSpace(content), nil
}

func (c *Client) AnalyzeImage(model, systemPrompt, imageBase64 string, maxTokens int) (string, error) {
	userText := "Describe this image as comma-separated SD tags. Output ONLY tags, nothing else."
	result, err := c.ChatVision(model, systemPrompt, userText, imageBase64, 0.4, maxTokens)
	if err != nil {
		return "", err
	}
	result = strings.TrimSpace(extractTags(result))
	result = promptutil.TruncateRepetitive(result, 1000)
	result = promptutil.DedupeTags(result)
	return result, nil
}

func (c *Client) AnalyzeImageDescribe(model, prompt, imageBase64 string, maxTokens int) (string, error) {
	return c.ChatVision(model, "", prompt, imageBase64, 0.4, maxTokens)
}

var thinkRe = regexp.MustCompile(`(?s)<think\s*>.*?</think\s*>`)

func stripThinkTags(s string) string {
	return thinkRe.ReplaceAllString(s, "")
}

func extractTags(s string) string {
	lower := strings.ToLower(s)
	tagMarkers := []string{"masterpiece", "score_9"}
	tagStart := -1
	for _, m := range tagMarkers {
		if idx := strings.Index(lower, m); idx >= 0 {
			if tagStart < 0 || idx < tagStart {
				tagStart = idx
			}
		}
	}

	if tagStart < 0 {
		return cleanResponse(s)
	}

	if nl := strings.LastIndex(s[:tagStart], "\n"); nl >= 0 {
		tagStart = nl + 1
	}

	result := s[tagStart:]

	cutMarkers := []string{"\n\nLet me", "\n\nHere ", "\n\nNote:", "\n\n---", "\n\n**", "\n\n#",
		"\nLet me create", "\nNow let me", "\nI'll ", "\nFirst,", "\nSo the"}
	for _, m := range cutMarkers {
		if idx := strings.Index(result, m); idx > 0 {
			result = result[:idx]
		}
	}

	result = strings.TrimSpace(result)
	result = strings.ReplaceAll(result, "\n", ", ")
	for strings.Contains(result, ", ,") {
		result = strings.ReplaceAll(result, ", ,", ",")
	}
	for strings.Contains(result, ",,") {
		result = strings.ReplaceAll(result, ",,", ",")
	}
	return result
}

func cleanResponse(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "**") && !strings.HasPrefix(line, "#") &&
			!strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "*") &&
			!strings.HasPrefix(line, ">") && !strings.HasSuffix(line, ":") &&
			!strings.HasPrefix(line, "```") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return s
}

func (c *Client) GenerateSDPrompt(systemPrompt, description, presetType, model string, maxTokens int) (string, error) {
	userMessage := description
	if presetType != "" {
		userMessage = fmt.Sprintf("[Type: %s] %s", presetType, description)
	}

	result, err := c.Chat(model, systemPrompt, userMessage, 0.7, maxTokens)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (c *Client) ChatWithMessages(model string, messages []Message, temperature float64, maxTokens int) (string, error) {
	reqBody, base := c.chatRequest(model, messages, temperature, maxTokens)
	logPrefix := fmt.Sprintf("[LLM] POST %s/v1/chat/completions model=%s msgs=%d", base, model, len(messages))
	return c.doChatRequest(base, reqBody, logPrefix)
}

func (c *Client) doChatRequest(base string, reqBody ChatRequest, logMsg string) (string, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	log.Printf("%s", logMsg)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[LLM] request error: %v", err)
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLLMResponseBodySize))
	if err != nil {
		log.Printf("[LLM] read error: %v", err)
		return "", fmt.Errorf("read response: %w", err)
	}

	log.Printf("[LLM] response status=%d body_len=%d", resp.StatusCode, len(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		log.Printf("[LLM] decode error: %v", err)
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM (body: %s)", string(respBody))
	}

	return strings.TrimSpace(stripThinkTags(chatResp.Choices[0].Message.Content)), nil
}

func CleanTags(s string) string {
	s = strings.TrimSpace(extractTags(s))
	s = promptutil.TruncateRepetitive(s, 1000)
	return s
}

func (c *Client) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url()+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) SetURL(baseURL string) {
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return
	}
	c.mu.Lock()
	c.baseURL = baseURL
	c.mu.Unlock()
}

func (c *Client) SetConfigSource(src ConfigSource) {
	c.mu.Lock()
	c.cfgSource = src
	c.mu.Unlock()
}

func (c *Client) ListLoaded() ([]LoadedModel, error) {
	return c.listOllamaLoaded()
}

func (c *Client) UnloadAll(ctx context.Context) error {
	return c.unloadOllamaAll(ctx)
}

type LLMModel struct {
	ID     string `json:"id"`
	Object string `json:"object"`
}

func (c *Client) GetModels() ([]LLMModel, error) {
	return c.getOllamaModels()
}
