package sd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	DefaultClientName = "imagecaster"

	KindTxt2Img = "txt2img"
	KindImg2Img = "img2img"
	KindUpscale = "upscale"

	JobQueued    = "queued"
	JobRunning   = "running"
	JobCompleted = "completed"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
	JobLost      = "lost"

	submitRetries = 3

	pollTimeout   = 30 * time.Second
	submitTimeout = 2 * time.Minute
	resultTimeout = 2 * time.Minute
)

var (
	ErrUserCancel   = errors.New("cancelled by user")
	ErrJobLost      = errors.New("sd job lost")
	ErrJobCancelled = errors.New("sd job cancelled")
)

type ModelSpec struct {
	Checkpoint     string   `json:"checkpoint"`
	VAE            string   `json:"vae"`
	Modules        []string `json:"modules"`
	KleinReference bool     `json:"klein_reference"`
	MossReference  bool     `json:"moss_reference"`
}

type JobRequest struct {
	Kind      string
	ClientRef string
	Model     ModelSpec
	Params    any
}

type JobProgress struct {
	Step  int `json:"step"`
	Steps int `json:"steps"`
}

type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *JobError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type JobStatus struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Client     string      `json:"client"`
	ClientRef  string      `json:"client_ref"`
	Status     string      `json:"status"`
	Position   int         `json:"position"`
	Progress   JobProgress `json:"progress"`
	Error      *JobError   `json:"error"`
	CreatedAt  *time.Time  `json:"created_at"`
	StartedAt  *time.Time  `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at"`
	ExpiresAt  *time.Time  `json:"expires_at"`
}

func (s *JobStatus) Terminal() bool {
	switch s.Status {
	case JobCompleted, JobFailed, JobCancelled, JobLost:
		return true
	}
	return false
}

type JobResult struct {
	Images []string        `json:"images"`
	Info   json.RawMessage `json:"info"`
}

type HiresParams struct {
	Enable    bool    `json:"enable"`
	Scale     float64 `json:"scale"`
	Upscaler  string  `json:"upscaler"`
	Denoising float64 `json:"denoising"`
	Steps     int     `json:"steps"`
	ResizeX   int     `json:"resize_x"`
	ResizeY   int     `json:"resize_y"`
}

type Txt2ImgParams struct {
	Prompt         string       `json:"prompt"`
	NegativePrompt string       `json:"negative_prompt"`
	Sampler        string       `json:"sampler"`
	Scheduler      string       `json:"scheduler"`
	Steps          int          `json:"steps"`
	CfgScale       float64      `json:"cfg_scale"`
	Width          int          `json:"width"`
	Height         int          `json:"height"`
	Seed           int64        `json:"seed"`
	ClipSkip       *int         `json:"clip_skip,omitempty"`
	BatchSize      int          `json:"batch_size"`
	Hires          *HiresParams `json:"hires,omitempty"`
}

type Img2ImgParams struct {
	Txt2ImgParams
	InitImage         string  `json:"init_image"`
	Mask              string  `json:"mask"`
	DenoisingStrength float64 `json:"denoising_strength"`
}

type UpscaleParams struct {
	Image    string  `json:"image"`
	Upscaler string  `json:"upscaler"`
	Scale    float64 `json:"scale"`
}

type submitBody struct {
	Kind      string     `json:"kind"`
	Client    string     `json:"client"`
	ClientRef string     `json:"client_ref,omitempty"`
	Model     *ModelSpec `json:"model,omitempty"`
	Params    any        `json:"params"`
}

func normalizeModel(m ModelSpec) ModelSpec {
	if m.Modules == nil {
		m.Modules = []string{}
	}
	return m
}

func (c *Client) SubmitJob(ctx context.Context, req JobRequest) (*JobStatus, error) {
	body := submitBody{Kind: req.Kind, Client: c.clientName(), ClientRef: req.ClientRef, Params: req.Params}
	if req.Kind != KindUpscale || req.Model.Checkpoint != "" {
		m := normalizeModel(req.Model)
		body.Model = &m
	}
	var st JobStatus
	if err := c.do(ctx, http.MethodPost, "/jobs", body, &st); err != nil {
		return nil, fmt.Errorf("submit sd job: %w", err)
	}
	return &st, nil
}

func (c *Client) GetJob(ctx context.Context, id string) (*JobStatus, error) {
	var st JobStatus
	if err := c.do(ctx, http.MethodGet, "/jobs/"+url.PathEscape(id), nil, &st); err != nil {
		return nil, fmt.Errorf("get sd job: %w", err)
	}
	return &st, nil
}

func (c *Client) JobResult(ctx context.Context, id string) (*JobResult, error) {
	var res JobResult
	if err := c.do(ctx, http.MethodGet, "/jobs/"+url.PathEscape(id)+"/result", nil, &res); err != nil {
		return nil, fmt.Errorf("get sd job result: %w", err)
	}
	return &res, nil
}

func (c *Client) CancelJob(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodPost, "/jobs/"+url.PathEscape(id)+"/cancel", map[string]any{}, nil); err != nil {
		return fmt.Errorf("cancel sd job: %w", err)
	}
	return nil
}

func (c *Client) DeleteJob(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodDelete, "/jobs/"+url.PathEscape(id), nil, nil); err != nil {
		return fmt.Errorf("delete sd job: %w", err)
	}
	return nil
}

func (c *Client) ListJobs(ctx context.Context, client string) ([]JobStatus, error) {
	p := "/jobs"
	if client != "" {
		p += "?client=" + url.QueryEscape(client)
	}
	var body struct {
		Jobs []JobStatus `json:"jobs"`
	}
	if err := c.do(ctx, http.MethodGet, p, nil, &body); err != nil {
		return nil, fmt.Errorf("list sd jobs: %w", err)
	}
	return body.Jobs, nil
}

func (c *Client) pollInterval() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return time.Second
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (c *Client) directRef() string {
	return c.refPrefix() + "d:" + randomHex(8)
}

func (c *Client) withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

func (c *Client) submitWithRetry(ctx context.Context, req JobRequest) (*JobStatus, error) {
	var lastErr error
	for attempt := 0; attempt <= submitRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, c.pollInterval()); err != nil {
				return nil, err
			}
		}
		sctx, cancel := c.withTimeout(ctx, submitTimeout)
		st, err := c.SubmitJob(sctx, req)
		cancel()
		if err == nil {
			return st, nil
		}
		if ctx.Err() != nil || isAPIError(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) ctxError(ctx context.Context, jobID string) error {
	cause := context.Cause(ctx)
	if errors.Is(cause, ErrUserCancel) && jobID != "" {
		cctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
		_ = c.CancelJob(cctx, jobID)
		cancel()
	}
	if cause != nil && cause != ctx.Err() {
		return fmt.Errorf("%w: %w", ctx.Err(), cause)
	}
	return ctx.Err()
}

func isLostStatus(st *JobStatus) bool {
	if st.Status == JobLost {
		return true
	}
	return st.Status == JobFailed && st.Error != nil && st.Error.Code == "server_restarted"
}

func (c *Client) RunJob(ctx context.Context, req JobRequest, onProgress func(JobStatus)) (*JobResult, error) {
	return c.runJob(ctx, req, onProgress, true)
}

func (c *Client) runJob(ctx context.Context, req JobRequest, onProgress func(JobStatus), deleteAfter bool) (*JobResult, error) {
	if req.ClientRef == "" {
		req.ClientRef = c.directRef()
	}
	if ctx.Err() != nil {
		return nil, c.ctxError(ctx, "")
	}
	st, err := c.submitWithRetry(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, c.ctxError(ctx, "")
		}
		return nil, err
	}
	resubmitted := false
	lost := func() error {
		if resubmitted {
			return ErrJobLost
		}
		resubmitted = true
		next, err := c.submitWithRetry(ctx, req)
		if err != nil {
			return err
		}
		st = next
		return nil
	}
	for {
		if onProgress != nil && !st.Terminal() {
			onProgress(*st)
		}
		switch {
		case st.Status == JobCompleted:
			rctx, rcancel := c.withTimeout(ctx, resultTimeout)
			res, err := c.JobResult(rctx, st.ID)
			rcancel()
			if err == nil {
				if deleteAfter {
					dctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
					_ = c.DeleteJob(dctx, st.ID)
					cancel()
				}
				return res, nil
			}
			if ctx.Err() != nil {
				return nil, c.ctxError(ctx, st.ID)
			}
			if isNotFound(err) {
				if lerr := lost(); lerr != nil {
					return nil, c.wrapLoopErr(ctx, lerr)
				}
				continue
			}
			if errors.Is(err, ErrBadResponse) {
				return nil, err
			}
		case isLostStatus(st):
			if lerr := lost(); lerr != nil {
				return nil, c.wrapLoopErr(ctx, lerr)
			}
			continue
		case st.Status == JobFailed:
			if st.Error != nil {
				return nil, fmt.Errorf("sd job failed: %w", st.Error)
			}
			return nil, errors.New("sd job failed")
		case st.Status == JobCancelled:
			if ctx.Err() != nil {
				return nil, c.ctxError(ctx, "")
			}
			return nil, ErrJobCancelled
		}
		if err := sleepCtx(ctx, c.pollInterval()); err != nil {
			return nil, c.ctxError(ctx, st.ID)
		}
		pctx, pcancel := c.withTimeout(ctx, pollTimeout)
		next, err := c.GetJob(pctx, st.ID)
		pcancel()
		if err != nil {
			if ctx.Err() != nil {
				return nil, c.ctxError(ctx, st.ID)
			}
			if isNotFound(err) {
				if lerr := lost(); lerr != nil {
					return nil, c.wrapLoopErr(ctx, lerr)
				}
			}
			continue
		}
		st = next
	}
}

func (c *Client) wrapLoopErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return c.ctxError(ctx, "")
	}
	return err
}

func (c *Client) runDirect(req JobRequest) (*JobResult, error) {
	ctx, cancel := context.WithCancelCause(context.Background())
	key := c.trackDirect(cancel)
	defer func() {
		c.untrackDirect(key)
		cancel(nil)
	}()
	req.ClientRef = c.directRef()
	return c.RunJob(ctx, req, c.progressHandler())
}

func (c *Client) trackDirect(cancel context.CancelCauseFunc) uint64 {
	c.directMu.Lock()
	defer c.directMu.Unlock()
	if c.direct == nil {
		c.direct = map[uint64]context.CancelCauseFunc{}
	}
	c.directSeq++
	c.direct[c.directSeq] = cancel
	return c.directSeq
}

func (c *Client) untrackDirect(k uint64) {
	c.directMu.Lock()
	delete(c.direct, k)
	c.directMu.Unlock()
}

func (c *Client) Interrupt() error {
	c.directMu.Lock()
	for _, cancel := range c.direct {
		cancel(ErrUserCancel)
	}
	c.directMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	jobs, err := c.ListJobs(ctx, c.clientName())
	if err != nil {
		return fmt.Errorf("interrupt: %w", err)
	}
	prefix := c.refPrefix()
	var firstErr error
	for _, j := range jobs {
		if j.Terminal() || !strings.HasPrefix(j.ClientRef, prefix) {
			continue
		}
		if err := c.CancelJob(ctx, j.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("interrupt: %w", firstErr)
	}
	return nil
}

func txt2imgParams(req Txt2ImgRequest) Txt2ImgParams {
	p := Txt2ImgParams{
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		Sampler:        req.SamplerName,
		Scheduler:      req.Scheduler,
		Steps:          req.Steps,
		CfgScale:       req.CfgScale,
		Width:          req.Width,
		Height:         req.Height,
		Seed:           seedOf(req.Seed),
		ClipSkip:       req.ClipSkip,
		BatchSize:      batchOf(req.BatchSize, req.BatchCount),
	}
	if req.HiresFix != nil && *req.HiresFix {
		h := &HiresParams{
			Enable:   true,
			Upscaler: req.HiresUpscaler,
			Steps:    req.HiresSecondPassSteps,
			ResizeX:  req.HiresResizeX,
			ResizeY:  req.HiresResizeY,
		}
		if req.HiresUpscale != nil {
			h.Scale = *req.HiresUpscale
		}
		if req.HiresDenoisingStrength != nil {
			h.Denoising = *req.HiresDenoisingStrength
		} else if req.DenoisingStrength != nil {
			h.Denoising = *req.DenoisingStrength
		}
		p.Hires = h
	}
	return p
}

func img2imgParams(req Img2ImgRequest) Img2ImgParams {
	p := Img2ImgParams{
		Txt2ImgParams: Txt2ImgParams{
			Prompt:         req.Prompt,
			NegativePrompt: req.NegativePrompt,
			Sampler:        req.SamplerName,
			Scheduler:      req.Scheduler,
			Steps:          req.Steps,
			CfgScale:       req.CfgScale,
			Width:          req.Width,
			Height:         req.Height,
			Seed:           seedOf(req.Seed),
			ClipSkip:       req.ClipSkip,
			BatchSize:      batchOf(req.BatchSize, req.BatchCount),
		},
		Mask: req.Mask,
	}
	if len(req.InitImages) > 0 {
		p.InitImage = req.InitImages[0]
	}
	if req.DenoisingStrength != nil {
		p.DenoisingStrength = *req.DenoisingStrength
	}
	return p
}

func seedOf(s *int64) int64 {
	if s == nil {
		return -1
	}
	return *s
}

func batchOf(size, count *int) int {
	n := 1
	if size != nil && *size > 0 {
		n = *size
	}
	if count != nil && *count > 1 {
		n *= *count
	}
	return n
}

func txt2imgJob(req Txt2ImgRequest) JobRequest {
	return JobRequest{Kind: KindTxt2Img, Model: req.Model, Params: txt2imgParams(req)}
}

func img2imgJob(req Img2ImgRequest) JobRequest {
	return JobRequest{Kind: KindImg2Img, Model: req.Model, Params: img2imgParams(req)}
}

func upscaleJob(base64Img, upscaler string, scale float64) JobRequest {
	return JobRequest{Kind: KindUpscale, Params: UpscaleParams{Image: base64Img, Upscaler: upscaler, Scale: scale}}
}

func toResponse(job JobRequest, res *JobResult) (*Txt2ImgResponse, error) {
	params, _ := json.Marshal(job.Params)
	out := &Txt2ImgResponse{Images: res.Images, Parameters: params, Info: res.Info}
	if len(out.Images) == 0 {
		return out, errors.New("sd job returned no images")
	}
	return out, nil
}

func upscaleResult(res *JobResult) (string, error) {
	if len(res.Images) == 0 || res.Images[0] == "" {
		return "", errors.New("upscale image: empty response")
	}
	return res.Images[0], nil
}

func (c *Client) Txt2Img(req Txt2ImgRequest) (*Txt2ImgResponse, error) {
	job := txt2imgJob(req)
	res, err := c.runDirect(job)
	if err != nil {
		return nil, err
	}
	return toResponse(job, res)
}

func (c *Client) Img2Img(req Img2ImgRequest) (*Txt2ImgResponse, error) {
	job := img2imgJob(req)
	res, err := c.runDirect(job)
	if err != nil {
		return nil, err
	}
	return toResponse(job, res)
}

func (c *Client) UpscaleImage(base64Img string, upscaler string, scale float64) (string, error) {
	res, err := c.runDirect(upscaleJob(base64Img, upscaler, scale))
	if err != nil {
		return "", fmt.Errorf("upscale image: %w", err)
	}
	return upscaleResult(res)
}

func (c *Client) ForQueueJob(ctx context.Context, queueJobID int64) Service {
	return &queueScoped{Client: c, ctx: ctx, queueJobID: queueJobID}
}

var _ QueueScoper = (*Client)(nil)

type queueScoped struct {
	*Client
	ctx        context.Context
	queueJobID int64
	seq        atomic.Int64
}

func jobHash(job JobRequest) string {
	b, _ := json.Marshal(struct {
		Kind   string    `json:"kind"`
		Model  ModelSpec `json:"model"`
		Params any       `json:"params"`
	}{job.Kind, normalizeModel(job.Model), job.Params})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:8]
}

func QueueClientRef(installID string, queueJobID int64, seq int64, job JobRequest) string {
	return fmt.Sprintf("%s:q%d:s%d:%s", installID, queueJobID, seq, jobHash(job))
}

func (q *queueScoped) run(job JobRequest) (*JobResult, error) {
	job.ClientRef = QueueClientRef(q.Client.InstallID(), q.queueJobID, q.seq.Add(1), job)
	return q.Client.runJob(q.ctx, job, q.Client.progressHandler(), false)
}

func (c *Client) CleanupQueueJob(ctx context.Context, queueJobID int64) error {
	jobs, err := c.ListJobs(ctx, c.clientName())
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("%sq%d:", c.refPrefix(), queueJobID)
	var firstErr error
	for _, j := range jobs {
		if !strings.HasPrefix(j.ClientRef, prefix) {
			continue
		}
		if err := c.DeleteJob(ctx, j.ID); err != nil && !isNotFound(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (q *queueScoped) Txt2Img(req Txt2ImgRequest) (*Txt2ImgResponse, error) {
	job := txt2imgJob(req)
	res, err := q.run(job)
	if err != nil {
		return nil, err
	}
	return toResponse(job, res)
}

func (q *queueScoped) Img2Img(req Img2ImgRequest) (*Txt2ImgResponse, error) {
	job := img2imgJob(req)
	res, err := q.run(job)
	if err != nil {
		return nil, err
	}
	return toResponse(job, res)
}

func (q *queueScoped) UpscaleImage(base64Img string, upscaler string, scale float64) (string, error) {
	res, err := q.run(upscaleJob(base64Img, upscaler, scale))
	if err != nil {
		return "", fmt.Errorf("upscale image: %w", err)
	}
	return upscaleResult(res)
}
