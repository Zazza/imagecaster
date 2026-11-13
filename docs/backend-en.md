[English](backend-en.md) | [Русский](backend-ru.md)

# Imagecaster — Backend (Go)

## Constructors (Dependency Injection)

All core components are created in `main.go` and injected via constructors:

```go
cfg := config.Load()
presets, _ := preset.Open(cfg.DBPath)
llmClient := llm.New(cfg.LLMUrl)
sdClient := sd.New(cfg.SDUrl)
app := NewApp(presets, llmClient, sdClient, cfg)
```

## internal/config

### Config
```go
type Config struct {
    LLMUrl         string  // env: LLM_URL
    SDUrl          string  // env: SD_URL
    LLMModel       string  // env: LLM_MODEL
    SDPromptModel  string  // env: SD_PROMPT_MODEL (alias for LLMModel)
    VisionModel    string  // env: VISION_MODEL
    Port           string  // env: PORT
    DBPath         string  // env: DB_PATH
    SystemPrompt   string  // env: SYSTEM_PROMPT
    DefaultNegative string // env: DEFAULT_NEGATIVE
    DefaultSampler  string
    DefaultSteps    int
    DefaultCfgScale float64
    DefaultWidth    int
    DefaultHeight   int
}
```

### Prompt Constants
- `DefaultSDPromptInstruction` — system prompt for SD prompt generation
- `DefaultAnalyzeSystemPrompt` — prompt for image analysis
- `DefaultAnalyzeChainPrompts` — chain of prompts for deep analysis
- `KidsModePrompt` — prompt for kids mode (safe mode)

## internal/llm

### Client
HTTP client for OpenAI-compatible API.

```go
type Client struct { ... }

func New(baseURL, backend string) *Client
func (c *Client) Chat(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error)
func (c *Client) ChatVision(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error)
func (c *Client) AnalyzeImage(model, systemPrompt, imageBase64 string, maxTokens int) (string, error)
func (c *Client) GenerateSDPrompt(systemPrompt, description, presetType, model string, maxTokens int) (string, error)
func (c *Client) ChatWithMessages(model string, messages []Message, temperature float64, maxTokens int) (string, error)
func (c *Client) GetModels() ([]LLMModel, error)
func (c *Client) HealthCheck() error
func (c *Client) SetURL(baseURL string)
func (c *Client) SetBackendConfig(cfg BackendConfig)
```

### BackendConfig
```go
type BackendConfig struct {
    KeepAlive string  // "5m", "0" (unload immediately)
    NumCtx    int     // context size
}
```

### Utilities
- `CleanTags(s string) string` — tag cleanup and trimming
- `stripThinkTags(s string) string` — removal of `<think/>` blocks

## internal/sd

### Client
HTTP client for the image part of Imagecaster Server (`<server>/api/sd`). Every generation is a server job: submit, poll, fetch the result, delete. There is no global "current model": each job names its checkpoint, VAE, extra modules and reference flags.

```go
type ModelSpec struct {
    Checkpoint, VAE string
    Modules         []string
    KleinReference  bool
    MossReference   bool
}
type JobRequest struct { Kind, ClientRef string; Model ModelSpec; Params any }

func New(baseURL string) *Client
func (c *Client) SetClientName(name string)            // "imagecaster" by default; MCP uses "imagecaster-mcp"
func (c *Client) SubmitJob(ctx, JobRequest) (*JobStatus, error)    // POST /jobs
func (c *Client) GetJob(ctx, id) (*JobStatus, error)               // GET /jobs/{id}
func (c *Client) JobResult(ctx, id) (*JobResult, error)            // GET /jobs/{id}/result
func (c *Client) CancelJob(ctx, id) error                          // POST /jobs/{id}/cancel
func (c *Client) DeleteJob(ctx, id) error                          // DELETE /jobs/{id}
func (c *Client) ListJobs(ctx, client string) ([]JobStatus, error) // GET /jobs?client=
func (c *Client) RunJob(ctx, JobRequest, onProgress func(JobStatus)) (*JobResult, error)
func (c *Client) ForQueueJob(ctx, queueJobID int64) Service
func (c *Client) Txt2Img(req Txt2ImgRequest) (*Txt2ImgResponse, error)   // submit and wait
func (c *Client) Img2Img(req Img2ImgRequest) (*Txt2ImgResponse, error)   // submit and wait
func (c *Client) UpscaleImage(image, upscaler string, scale float64) (string, error)
func (c *Client) Interrupt() error        // cancels unfinished jobs of this client name and this installation
func (c *Client) SetProgressHandler(fn func(JobStatus))
func (c *Client) GetModels/GetSamplers/GetSchedulers/GetUpscalers/GetVAEs/GetLoRAs()
func (c *Client) State() (*ServerState, error)   // GET /state: loaded checkpoint, running job
func (c *Client) MemoryInfo() (*MemoryStats, error)
func (c *Client) HealthCheck() error             // GET /state
```

### RunJob
- Poll interval: `PollInterval` (1s by default). A network error while polling does not stop waiting.
- Status `lost` (or `failed` with code `server_restarted`) or 404 while polling: the job is submitted again once with the same `client_ref`; the second loss returns `ErrJobLost`.
- `failed`: error with the server message. Network error on submit: up to 3 more submits with the same `client_ref`; HTTP 4xx/5xx on submit are not retried.
- Context cancelled with cause `sd.ErrUserCancel`: the server job is cancelled. Any other cancellation (app shutdown) leaves the server job running.
- After the result is fetched the job is deleted on the server.

### Queue jobs
`ForQueueJob(ctx, id)` returns a `Service` whose generation calls use `ctx` and `client_ref = <inst>:q<id>:s<seq>:<hash8>` (seq = 1, 2, 3… in call order, hash8 = first 8 hex of sha256 over kind+model+params). Running the same chain again gives the same refs, so the server returns finished steps at once and re-attaches to the running one.

`<inst>` is the installation id: 16 random hex chars, created once and stored in the `install_id` setting (`preset.DB.InstallID`), passed to `SetInstallID`. Direct calls use `<inst>:d:<random>`. `CleanupQueueJob` deletes only `<inst>:q<id>:*`, `Interrupt` only jobs prefixed `<inst>:`, so two PCs sharing one server do not touch each other's jobs. imagecaster-mcp reads the same DB and id but uses its own client name.

### Requests
`Txt2ImgRequest` / `Img2ImgRequest` carry `Model ModelSpec` plus prompt, sampler and scheduler (sd.cpp names, e.g. `euler_a`, `karras`, `""` = default), steps, CFG, size, seed (nil = random), clip skip, batch size × batch count (sent as one `batch_size`), hires (txt2img), init image, mask and denoising strength (img2img).

## internal/preset

### DB
SQLite CRUD for all application data.

```go
type DB struct { ... }

func Open(dbPath string) (*DB, error)
func (db *DB) Close() error
```

### CRUD Methods
Presets: `ListPresets`, `Get`, `Create`, `Update`, `Delete`, `ListByType`
Types: `ListPresetTypes`, `GetPresetType`, `CreatePresetType`, `UpdatePresetType`, `DeletePresetType`
Compound: `ListCompoundPresets`, `GetCompoundPreset`, `CreateCompoundPreset`, `UpdateCompoundPreset`, `DeleteCompoundPreset`
Settings: `GetSetting(key)`, `SetSetting(key, value)`
Descriptions: `ListDescriptions`, `CreateDescription`, `UpdateDescription`, `DeleteDescription`
Prompts: `ListPrompts`, `CreatePrompt`, `DeletePrompt`
Sessions: `CreateSession`, `ListSessions`, `SwitchSession`, `DeleteSession`, etc.
- `AddSessionItem` deactivates previous items (`is_active=0`) before inserting a new one
Export: `ListExportPresets`, `SaveExportPreset`, `DeleteExportPreset`

### Migrations
Versions 1-10 in `db.go`. Automatically executed on `Open()`.

### Models

**Preset:**
```go
type Preset struct {
    ID             int64
    Name           string
    TypeID         int64
    PresetType     string
    Prompt         string
    NegativePrompt string
    Sampler        string
    ScheduleType   string
    Steps          int
    CfgScale       float64
    Width, Height  int
    ModelName      string
    VAE            string
    Seed           *int64
    ClipSkip       *int
    Loras          string   // JSON: [{"name":"lora1","weight":0.8}]
}
```

**LoRAEntry:**
```go
type LoRAEntry struct {
    Name   string  `json:"name"`
    Weight float64 `json:"weight"`
}
```

## internal/generation

Generation service that handles all image generation modes. Extracted from `app.go` to isolate generation logic from Wails bindings.

```go
type Service struct { ... }

func New(presetDB *preset.DB, llmClient llm.Service, sdClient sd.Service, cfg *config.Config, ...) *Service
func (s *Service) SetContext(ctx context.Context)
func (s *Service) GenerateImage(params GenerateImageParams) (*GenerateImageResult, error)
func (s *Service) GenerateFromImage(params GenerateFromImageParams) (*GenerateImageResult, error)
func (s *Service) GenerateSDPrompt(description, presetType string) (string, error)
func (s *Service) GenerateCompoundImage(params CompoundParams) (*GenerateImageResult, error)
func (s *Service) UpscaleImage(image, mode string, scale float64) (*GenerateImageResult, error)
func (s *Service) StartSDPolling()
func (s *Service) StopSDPolling()
```

Methods:
- `GenerateImage` — txt2img generation with preset resolution, LLM prompt generation, and optional batch
- `GenerateFromImage` — img2img generation with init image analysis and inpainting
- `GenerateSDPrompt` — converts text description to SD prompt via LLM
- `GenerateCompoundImage` — multi-step generation using compound presets
- `UpscaleImage` — upscale x2 via an img2img job
- `StartSDPolling` / `StopSDPolling` — turn `sd:progress` events on/off; progress comes from the job status of `RunJob`
- `WithSD(sd.Service)` — shallow copy that runs on another `sd.Service` (the queue passes `ForQueueJob`); progress state is shared

## internal/queue

Job queue with retry logic and paused state.

```go
type Service struct { ... }

func NewService(store *Store, processor Processor, emit EventEmitter) *Service
func (s *Service) Start(ctx context.Context)
func (s *Service) Enqueue(jobType JobType, params any, source string) (int64, error)
func (s *Service) GetJobs() ([]*Job, error)
func (s *Service) RemoveJob(id int64) error
func (s *Service) CancelJob(id int64) error
func (s *Service) PauseQueue()
func (s *Service) ResumeQueue()
func (s *Service) ResumePausedJobs() (int, error)
```

Key features:
- Job states: pending -> running -> completed/failed/paused
- Retry with exponential backoff (5s -> 10s -> 20s -> ... -> 60s max)
- Paused state when max retries (3) exhausted
- `IsRetryableError()` checks for transient errors (connection refused, timeout, EOF, etc.)
- Worker polls every 5s for scheduled retries
- DB migration v24 adds retry_count, max_retries, next_retry_at columns
- Each job runs on `ForQueueJob(ctx, job.ID)`, so its steps keep the same `client_ref` across restarts
- LLM answers of a job (prompt from an idea, image description, look rewrite) are kept in `data/queue-llm/<id>.json` (key: call number + sha256 of the request) until the job ends, so after a restart the steps get the same prompts and the same `client_ref`; an LLM failure is kept too, but only once SD work was sent after it (a failure that ends the job is asked again on retry)
- A result is not saved to history once the job context is cancelled; the job then counts as stopped (requeued on shutdown) instead of completed with a half-done image
- User cancel of a queue item cancels only that item; a direct generation running at the same time is not touched
- User cancel cancels the job context with cause `sd.ErrUserCancel` (the server job is cancelled); app shutdown cancels without that cause and the job stays `running`
- On start `running` jobs go back to `pending` without spending a retry and are picked first
- Frontend events: `queue:updated`, `queue:job-progress`

### Store

```go
type Store struct { ... }

func NewStore(db *preset.DB) *Store
func (s *Store) CreateJob(job *Job) error
func (s *Store) UpdateJob(job *Job) error
func (s *Store) GetPendingJobs() ([]*Job, error)
func (s *Store) GetScheduledJobs() ([]*Job, error)
func (s *Store) GetJobByID(id int64) (*Job, error)
func (s *Store) DeleteJob(id int64) error
```

### Job Model

```go
type Job struct {
    ID          int64
    Type        JobType
    Status      JobStatus
    Params      string    // JSON-encoded params
    Source      string
    Result      string    // JSON-encoded result
    Error       string
    RetryCount  int
    MaxRetries  int
    NextRetryAt *time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## internal/logger

Event logger with LogBridge for capturing all log output.

```go
type Logger struct { ... }

func New(ctx context.Context) *Logger
func (l *Logger) SetContext(ctx context.Context)
func (l *Logger) InstallBridge()
func (l *Logger) Error(format string, args ...interface{})
func (l *Logger) Warn(format string, args ...interface{})
func (l *Logger) Info(format string, args ...interface{})
func (l *Logger) Debug(format string, args ...interface{})
```

`InstallBridge()` hooks `log.SetOutput` to capture all standard log output (from LLM, SD, queue packages) and re-emit as `log:entry` events visible in the frontend footer log panel.

## Window Layout Persistence

`SaveWindowLayout(footerHeight)` and `GetFooterHeight()` bindings persist window state:
- Window size (width, height), position (x, y), maximised state
- Footer panel height
- Stored in SQLite `settings` table
- Restored on startup via `restoreWindowLayout()`
- Saved on `beforeunload` event and after footer drag-resize

## app.go Helper Functions

### Preset Resolution
Common pattern for all generation modes:
```go
// Load preset -> extract parameters -> the model goes into the job
p, _ := s.db.Get(presetID)
spec := s.prepareSDContext(p, "generate") // sd.ModelSpec{Checkpoint, VAE, Modules}
s.sd.Txt2Img(sd.Txt2ImgRequest{Model: spec, ...})
```

### Image Processing
- `analyzeRemoveContext(image, mask)` — red overlay + LLM vision

### LLM Config
- `applyLLMConfig(mode)` — loads Ollama-specific settings from settings
- Modes: `"generate"` (text), `"analyze"` (vision)

### Events
```go
runtime.EventsEmit(a.ctx, "event:name", data)
```
- `analyze:step` — analysis progress
- `remove:stage` — "analyzing" / "generating"
- `batch:progress` / `batch:done` / `batch:error`
- `session:added` — new image added to session
- `queue:updated` — queue state changed
- `queue:job-progress` — job execution progress
- `log:entry` — log entry from LogBridge

### Kids Mode
- `isKidsMode()` — checks if enabled
- `filterKidsInput(text)` — input filtering
- `applyKidsSystemPrompt(prompt)` — system prompt modification
