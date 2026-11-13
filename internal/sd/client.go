package sd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxResponseBodySize = 50 * 1024 * 1024
	listTimeout         = 30 * time.Second
	shortTimeout        = 5 * time.Second
)

var base64SnippetRe = regexp.MustCompile(`[A-Za-z0-9+/=]{120,}`)

func redactBase64(s string) string {
	return base64SnippetRe.ReplaceAllString(s, "<redacted>")
}

type Client struct {
	mu         sync.RWMutex
	baseURL    string
	httpClient *http.Client

	PollInterval time.Duration

	progressMu sync.RWMutex
	progressFn func(JobStatus)

	name      string
	installID string

	directMu  sync.Mutex
	direct    map[uint64]context.CancelCauseFunc
	directSeq uint64
}

var _ Service = (*Client)(nil)

func New(baseURL string) *Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     120 * time.Second,
	}
	return &Client{
		baseURL:      baseURL,
		httpClient:   &http.Client{Transport: transport},
		PollInterval: time.Second,
		installID:    randomHex(4),
	}
}

func (c *Client) SetURL(baseURL string) {
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return
	}
	c.mu.Lock()
	c.baseURL = baseURL
	c.mu.Unlock()
}

func (c *Client) SetClientName(name string) {
	c.mu.Lock()
	c.name = name
	c.mu.Unlock()
}

func (c *Client) SetInstallID(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	c.mu.Lock()
	c.installID = id
	c.mu.Unlock()
}

func (c *Client) InstallID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.installID
}

func (c *Client) refPrefix() string {
	return c.InstallID() + ":"
}

func (c *Client) clientName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.name == "" {
		return DefaultClientName
	}
	return c.name
}

func (c *Client) base() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *Client) SetProgressHandler(fn func(JobStatus)) {
	c.progressMu.Lock()
	c.progressFn = fn
	c.progressMu.Unlock()
}

func (c *Client) progressHandler() func(JobStatus) {
	c.progressMu.RLock()
	defer c.progressMu.RUnlock()
	return c.progressFn
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code == "" && e.Message == "" {
		return fmt.Sprintf("sd server: status %d", e.Status)
	}
	return fmt.Sprintf("sd server: %s: %s (status %d)", e.Code, e.Message, e.Status)
}

func isAPIError(err error) bool {
	var ae *APIError
	return errors.As(err, &ae)
}

func isNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

var ErrBadResponse = errors.New("unreadable sd server response")

func (c *Client) do(ctx context.Context, method, p string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+p, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return err
	}
	if len(data) > maxResponseBodySize {
		return fmt.Errorf("%w: larger than %d MB", ErrBadResponse, maxResponseBodySize>>20)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		ae := &APIError{Status: resp.StatusCode}
		var eb struct {
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &eb) == nil && eb.Error != nil {
			ae.Code = eb.Error.Code
			ae.Message = eb.Error.Message
		} else if len(data) > 0 {
			msg := redactBase64(strings.TrimSpace(string(data)))
			if len(msg) > 512 {
				msg = msg[:512]
			}
			ae.Message = msg
		}
		return ae
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w: %w", ErrBadResponse, err)
	}
	return nil
}

func (c *Client) get(p string, timeout time.Duration, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.do(ctx, http.MethodGet, p, nil, out)
}

type Txt2ImgRequest struct {
	Model                  ModelSpec
	Prompt                 string
	NegativePrompt         string
	SamplerName            string
	Scheduler              string
	Steps                  int
	CfgScale               float64
	Width                  int
	Height                 int
	Seed                   *int64
	DenoisingStrength      *float64
	ClipSkip               *int
	BatchSize              *int
	BatchCount             *int
	HiresFix               *bool
	HiresUpscale           *float64
	HiresDenoisingStrength *float64
	HiresUpscaler          string
	HiresResizeX           int
	HiresResizeY           int
	HiresSecondPassSteps   int
}

type Img2ImgRequest struct {
	Model             ModelSpec
	InitImages        []string
	Prompt            string
	NegativePrompt    string
	SamplerName       string
	Scheduler         string
	Steps             int
	CfgScale          float64
	Width             int
	Height            int
	Seed              *int64
	DenoisingStrength *float64
	ClipSkip          *int
	BatchSize         *int
	BatchCount        *int
	Mask              string
}

type Txt2ImgResponse struct {
	Images     []string        `json:"images"`
	Parameters json.RawMessage `json:"parameters"`
	Info       json.RawMessage `json:"info"`
}

type ServerState struct {
	LoadedCheckpoint string `json:"loaded_checkpoint"`
	RunningJob       string `json:"running_job"`
}

func (c *Client) State() (*ServerState, error) {
	var st ServerState
	if err := c.get("/state", 3*time.Second, &st); err != nil {
		return nil, fmt.Errorf("sd state: %w", err)
	}
	return &st, nil
}

func (c *Client) HealthCheck() error {
	if _, err := c.State(); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

type SDModel struct {
	Title  string `json:"title"`
	Name   string `json:"model_name"`
	Hash   string `json:"hash"`
	Config string `json:"config"`
}

type Sampler struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

type Scheduler struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type Upscaler struct {
	Name  string  `json:"name"`
	Model string  `json:"model_name"`
	Scale float64 `json:"scale"`
}

type VAE struct {
	ModelName string `json:"model_name"`
	Path      string `json:"path"`
}

type LoRA struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (c *Client) GetModels() ([]SDModel, error) {
	var body struct {
		Checkpoints []struct {
			Name string `json:"name"`
			File string `json:"file"`
		} `json:"checkpoints"`
	}
	if err := c.get("/checkpoints", listTimeout, &body); err != nil {
		return nil, fmt.Errorf("get models: %w", err)
	}
	out := make([]SDModel, 0, len(body.Checkpoints))
	for _, m := range body.Checkpoints {
		out = append(out, SDModel{Title: m.Name, Name: strings.TrimSuffix(m.Name, path.Ext(m.Name)), Config: m.File})
	}
	return out, nil
}

func (c *Client) GetSamplers() ([]Sampler, error) {
	var body struct {
		Samplers []string `json:"samplers"`
	}
	if err := c.get("/samplers", listTimeout, &body); err != nil {
		return nil, fmt.Errorf("get samplers: %w", err)
	}
	out := make([]Sampler, 0, len(body.Samplers))
	for _, s := range body.Samplers {
		out = append(out, Sampler{Name: s})
	}
	return out, nil
}

func (c *Client) GetSchedulers() ([]Scheduler, error) {
	var body struct {
		Schedulers []string `json:"schedulers"`
	}
	if err := c.get("/schedulers", listTimeout, &body); err != nil {
		return nil, fmt.Errorf("get schedulers: %w", err)
	}
	out := make([]Scheduler, 0, len(body.Schedulers))
	for _, s := range body.Schedulers {
		if s == "" {
			continue
		}
		out = append(out, Scheduler{Name: s, Label: s})
	}
	return out, nil
}

func (c *Client) GetUpscalers() ([]Upscaler, error) {
	var body struct {
		Upscalers []struct {
			Name  string  `json:"name"`
			Scale float64 `json:"scale"`
		} `json:"upscalers"`
	}
	if err := c.get("/upscalers", listTimeout, &body); err != nil {
		return nil, fmt.Errorf("get upscalers: %w", err)
	}
	out := make([]Upscaler, 0, len(body.Upscalers))
	for _, u := range body.Upscalers {
		out = append(out, Upscaler{Name: u.Name, Model: u.Name, Scale: u.Scale})
	}
	return out, nil
}

func (c *Client) GetVAEs() ([]VAE, error) {
	var body struct {
		VAEs []struct {
			Name string `json:"name"`
			File string `json:"file"`
		} `json:"vaes"`
	}
	if err := c.get("/vaes", listTimeout, &body); err != nil {
		return nil, fmt.Errorf("get vae: %w", err)
	}
	out := make([]VAE, 0, len(body.VAEs))
	for _, v := range body.VAEs {
		out = append(out, VAE{ModelName: v.Name, Path: v.File})
	}
	return out, nil
}

func (c *Client) GetLoRAs() ([]LoRA, error) {
	var body struct {
		LoRAs []LoRA `json:"loras"`
	}
	if err := c.get("/loras", listTimeout, &body); err != nil {
		return nil, fmt.Errorf("get loras: %w", err)
	}
	if body.LoRAs == nil {
		body.LoRAs = []LoRA{}
	}
	return body.LoRAs, nil
}
