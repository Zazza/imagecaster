[English](backend-en.md) | [Русский](backend-ru.md)

# Imagecaster — Backend (Go)

## Конструкторы (Dependency Injection)

Все основные компоненты создаются в `main.go` и внедряются через конструкторы:

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
    SDPromptModel  string  // env: SD_PROMPT_MODEL (алиас для LLMModel)
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

### Константы промптов
- `DefaultSDPromptInstruction` — системный промпт для генерации SD промптов
- `DefaultAnalyzeSystemPrompt` — промпт для анализа изображений
- `DefaultAnalyzeChainPrompts` — цепочка промптов для глубокого анализа
- `KidsModePrompt` — промпт для безопасного режима

## internal/llm

### Client
HTTP-клиент для OpenAI-совместимого API.

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
    KeepAlive string  // "5m", "0" (выгрузить сразу)
    NumCtx    int     // размер контекста
}
```

### Утилиты
- `CleanTags(s string) string` — очистка и обрезка тегов
- `stripThinkTags(s string) string` — удаление `<think/>` блоков

## internal/sd

### Client
HTTP-клиент картиночной части Imagecaster Server (`<server>/api/sd`). Каждая генерация — задание на сервере: поставить, опрашивать, забрать результат, удалить. Общей «текущей модели» нет: каждое задание само называет checkpoint, VAE, доп. модули и флаги опорного кадра.

```go
type ModelSpec struct {
    Checkpoint, VAE string
    Modules         []string
    KleinReference  bool
    MossReference   bool
}
type JobRequest struct { Kind, ClientRef string; Model ModelSpec; Params any }

func New(baseURL string) *Client
func (c *Client) SetClientName(name string)            // по умолчанию "imagecaster"; MCP — "imagecaster-mcp"
func (c *Client) SubmitJob(ctx, JobRequest) (*JobStatus, error)    // POST /jobs
func (c *Client) GetJob(ctx, id) (*JobStatus, error)               // GET /jobs/{id}
func (c *Client) JobResult(ctx, id) (*JobResult, error)            // GET /jobs/{id}/result
func (c *Client) CancelJob(ctx, id) error                          // POST /jobs/{id}/cancel
func (c *Client) DeleteJob(ctx, id) error                          // DELETE /jobs/{id}
func (c *Client) ListJobs(ctx, client string) ([]JobStatus, error) // GET /jobs?client=
func (c *Client) RunJob(ctx, JobRequest, onProgress func(JobStatus)) (*JobResult, error)
func (c *Client) ForQueueJob(ctx, queueJobID int64) Service
func (c *Client) Txt2Img(req Txt2ImgRequest) (*Txt2ImgResponse, error)   // поставить и ждать
func (c *Client) Img2Img(req Img2ImgRequest) (*Txt2ImgResponse, error)   // поставить и ждать
func (c *Client) UpscaleImage(image, upscaler string, scale float64) (string, error)
func (c *Client) Interrupt() error        // отменяет незавершённые задания своего имени клиента и своей установки
func (c *Client) SetProgressHandler(fn func(JobStatus))
func (c *Client) GetModels/GetSamplers/GetSchedulers/GetUpscalers/GetVAEs/GetLoRAs()
func (c *Client) State() (*ServerState, error)   // GET /state: загруженная модель, идущее задание
func (c *Client) MemoryInfo() (*MemoryStats, error)
func (c *Client) HealthCheck() error             // GET /state
```

### RunJob
- Интервал опроса — `PollInterval` (по умолчанию 1 с). Сетевая ошибка опроса не прерывает ожидание.
- Статус `lost` (или `failed` с кодом `server_restarted`) либо 404 при опросе — задание ставится заново один раз с тем же `client_ref`; повторная потеря — ошибка `ErrJobLost`.
- `failed` — ошибка с текстом сервера. Сетевая ошибка при постановке — ещё до 3 постановок с тем же `client_ref`; HTTP 4xx/5xx при постановке не повторяются.
- Отмена контекста с причиной `sd.ErrUserCancel` отменяет задание на сервере. Любая другая отмена (закрытие приложения) задание на сервере не трогает.
- После получения результата задание удаляется на сервере.

### Задания очереди
`ForQueueJob(ctx, id)` возвращает `Service`, у которого вызовы генерации идут с `ctx` и `client_ref = <inst>:q<id>:s<seq>:<hash8>` (seq = 1, 2, 3… по порядку вызовов, hash8 — первые 8 hex sha256 от kind+model+params). Повтор той же цепочки даёт те же ref: сервер сразу отдаёт готовые шаги и подключает к идущему.

`<inst>` — id установки: случайные 16 hex, создаются один раз и хранятся в настройке `install_id` (`preset.DB.InstallID`), передаются в `SetInstallID`. Прямые вызовы идут с `<inst>:d:<random>`. `CleanupQueueJob` удаляет только `<inst>:q<id>:*`, `Interrupt` — только задания с префиксом `<inst>:`, поэтому два компьютера с одним сервером не трогают задания друг друга. imagecaster-mcp читает ту же БД и тот же id, но работает под своим именем клиента.

### Запросы
`Txt2ImgRequest` / `Img2ImgRequest` несут `Model ModelSpec`, промпт, сэмплер и планировщик (имена sd.cpp, напр. `euler_a`, `karras`, `""` — по умолчанию), шаги, CFG, размер, seed (nil — случайный), clip skip, batch size × batch count (уходит одним `batch_size`), hires (txt2img), исходник, маску и силу denoise (img2img).

## internal/preset

### DB
SQLite CRUD для всех данных приложения.

```go
type DB struct { ... }

func Open(dbPath string) (*DB, error)
func (db *DB) Close() error
```

### CRUD методы
Пресеты: `ListPresets`, `Get`, `Create`, `Update`, `Delete`, `ListByType`
Типы: `ListPresetTypes`, `GetPresetType`, `CreatePresetType`, `UpdatePresetType`, `DeletePresetType`
Compound: `ListCompoundPresets`, `GetCompoundPreset`, `CreateCompoundPreset`, `UpdateCompoundPreset`, `DeleteCompoundPreset`
Settings: `GetSetting(key)`, `SetSetting(key, value)`
Descriptions: `ListDescriptions`, `CreateDescription`, `UpdateDescription`, `DeleteDescription`
Prompts: `ListPrompts`, `CreatePrompt`, `DeletePrompt`
Sessions: `CreateSession`, `ListSessions`, `SwitchSession`, `DeleteSession`, etc.
- `AddSessionItem` деактивирует предыдущие элементы (`is_active=0`) перед вставкой нового
Export: `ListExportPresets`, `SaveExportPreset`, `DeleteExportPreset`

### Миграции
Версии 1-10 в `db.go`. Автоматически выполняются при `Open()`.

### Модели

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

Сервис генерации, обрабатывающий все режимы генерации изображений. Извлечён из `app.go` для изоляции логики генерации от Wails bindings.

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

Методы:
- `GenerateImage` — генерация txt2img с разрешением пресета, генерацией промпта через LLM и опциональным батчем
- `GenerateFromImage` — генерация img2img с анализом исходного изображения и инпейнтингом
- `GenerateSDPrompt` — преобразование текстового описания в SD промпт через LLM
- `GenerateCompoundImage` — многошаговая генерация с использованием составных пресетов
- `UpscaleImage` — апскейл x2 заданием img2img
- `StartSDPolling` / `StopSDPolling` — включают/выключают события `sd:progress`; прогресс берётся из статуса задания `RunJob`
- `WithSD(sd.Service)` — поверхностная копия, работающая через другой `sd.Service` (очередь передаёт `ForQueueJob`); состояние прогресса общее

## internal/queue

Очередь задач с логикой повторных попыток и состоянием паузы.

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

Основные возможности:
- Состояния задач: pending -> running -> completed/failed/paused
- Повторные попытки с экспоненциальной задержкой (5с -> 10с -> 20с -> ... -> 60с макс)
- Состояние paused при исчерпании лимита попыток (3)
- `IsRetryableError()` проверяет транзиентные ошибки (connection refused, timeout, EOF и т.д.)
- Воркер опрашивает БД каждые 5с для запланированных повторных попыток
- Миграция БД v24 добавляет колонки retry_count, max_retries, next_retry_at
- Каждое задание идёт через `ForQueueJob(ctx, job.ID)`, поэтому его шаги сохраняют `client_ref` между перезапусками
- Ответы ИИ задания (промпт из идеи, описание фото, переписанный образ) хранятся в `data/queue-llm/<id>.json` (ключ: номер вызова + sha256 запроса) до конца задания, поэтому после перезапуска шаги получают те же промпты и те же `client_ref`; отказ ИИ тоже запоминается, но только если после него ушла генерация (отказ, оборвавший задание, при повторе спрашивается заново)
- После отмены контекста задания результат в историю не сохраняется; задание считается остановленным (при закрытии приложения возвращается в очередь), а не выполненным с недоделанной картинкой
- Отмена пользователем одного задания очереди отменяет только его; прямая генерация, идущая в это же время, не прерывается
- Отмена пользователем отменяет контекст с причиной `sd.ErrUserCancel` (задание на сервере отменяется); закрытие приложения отменяет без этой причины, задание остаётся `running`
- При запуске `running` возвращаются в `pending` без траты попытки и берутся первыми
- События фронтенда: `queue:updated`, `queue:job-progress`

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

### Модель Job

```go
type Job struct {
    ID          int64
    Type        JobType
    Status      JobStatus
    Params      string    // JSON-закодированные параметры
    Source      string
    Result      string    // JSON-закодированный результат
    Error       string
    RetryCount  int
    MaxRetries  int
    NextRetryAt *time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## internal/logger

Логгер событий с LogBridge для перехвата всех log-сообщений.

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

`InstallBridge()` перехватывает `log.SetOutput` для захвата всех стандартных log-сообщений (из пакетов LLM, SD, queue) и повторной отправки как событий `log:entry`, отображаемых в панели логов подвала фронтенда.

## Сохранение Layout окна

Привязки `SaveWindowLayout(footerHeight)` и `GetFooterHeight()` сохраняют состояние окна:
- Размер окна (ширина, высота), позиция (x, y), состояние максимизации
- Высота панели подвала (footer)
- Хранятся в таблице `settings` SQLite
- Восстанавливаются при запуске через `restoreWindowLayout()`
- Сохраняются при событии `beforeunload` и после перетаскивания/изменения размера footer

## Вспомогательные функции app.go

### Preset Resolution
Общий паттерн для всех режимов генерации:
```go
// Загрузить пресет -> извлечь параметры -> установить модель/VAE
p, _ := a.presets.Get(presetID)
samplerName, steps, cfgScale, width, height = ...
spec := s.prepareSDContext(p, "generate") // sd.ModelSpec{Checkpoint, VAE, Modules}
s.sd.Txt2Img(sd.Txt2ImgRequest{Model: spec, ...})
```

### Image Processing
- `analyzeRemoveContext(image, mask)` — красный overlay + LLM vision

### LLM Config
- `applyLLMConfig(mode)` — загружает Ollama-специфичные настройки из settings
- Режимы: `"generate"` (текст), `"analyze"` (vision)

### Events
```go
runtime.EventsEmit(a.ctx, "event:name", data)
```
- `analyze:step` — прогресс анализа
- `remove:stage` — "analyzing" / "generating"
- `batch:progress` / `batch:done` / `batch:error`
- `session:added` — новое изображение в сессии
- `queue:updated` — изменение состояния очереди
- `queue:job-progress` — прогресс выполнения задачи
- `log:entry` — запись лога от LogBridge

### Kids Mode
- `isKidsMode()` — проверка включён
- `filterKidsInput(text)` — фильтрация входных данных
- `applyKidsSystemPrompt(prompt)` — модификация system prompt
