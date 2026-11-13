[English](backend-en.md) | [Русский](backend-ru.md)

# Imagecaster — Backend (Go)

How the Go side is built: startup, configuration, services and their Wails bindings, events, error handling. For the overall picture and the server protocol see [architecture-en.md](architecture-en.md).

## Startup

`main.go`:

1. `config.Load()` — defaults and the DB path.
2. `preset.Open(cfg.DBPath)` — opens SQLite and runs migrations.
3. Creates `llm.Client`, `sd.Client` (gets the install ID from the DB), `serverclient.Client`.
4. `settings.Restore(...)` — reads `server_url` and model choices from the DB and points the clients at the server.
5. `NewApp(...)` — builds all services; then embedded presets (`data/presets/*.json`) are handed to the settings service.
6. `wails.Run(...)` with `App` as the only bound object. The asset handler serves `/api/img/<id>` and `/api/thumb/<id>` from `data/sessions` and `data/thumbs`.

`App.startup`: clears finished queue jobs, starts the queue worker, starts the 2 s DB watch for sessions created by `imagecaster-mcp`, clears `data/batch`, restores window size and position.
`App.shutdown`: stops the queue (up to 5 s); a running job goes back to `pending`.

`NewApp` wiring:

```go
a.sessions    = session.New(presets, dataDir, emitter)
a.settingsSvc = settings.New(presets, llm, sd, cfg, log, srvClient)
a.ieSvc       = importexport.New(presets, sd, log)
a.gen         = generation.New(presets, llm, sd, cfg, dataDir, emitter, kidsMgr, sessions, settingsSvc, settingsSvc, log)
queueProc     = queue.NewProcessor(gen, sd.(sd.QueueScoper), queueStore, dataDir, emitter)
a.queueSvc    = queue.NewService(queueStore, queueProc, emitter)
a.batch       = batch.New(dataDir + "/batch")
```

`cmd/imagecaster-mcp/main.go` repeats steps 1–4 and builds `generation` the same way, without the queue and with a no-op event emitter.

## Configuration

### Sources

| Source | What |
|--------|------|
| `config.Load()` | Built-in defaults and prompt texts. |
| Env `DB_PATH` | DB file; relative path is resolved next to the executable. Default `data/presets.db`. |
| Table `settings` | Everything the user changes: `server_url`, chosen models, prompt texts, kids mode, UI state, last form values. |

Only keys listed in `config.AllowedSettings` are written by `UpdateSettings`; other keys are ignored. `server_url` is normalized (adds `http://`, strips trailing `/`); `llm_num_ctx` and `llm_max_tokens` become `0` if not a non-negative number.

### Server address

`settings.ApplyServerURL` sets the base URL of `serverclient` and derives `<server_url>/api/sd` for `sd` and `<server_url>/api/llm` for `llm`. It is called at startup (`Restore`) and when `server_url` is saved.

On first start after an update from a version with direct connections, `Restore` looks at old `sd_url` / `llm_url` settings, probes port 8080 on the same host and saves it as `server_url` if an Imagecaster Server answers.

### Config struct

Key fields of `config.Config`: `DBPath`, `LLMModel`, `SDPromptModel` (prompt model), `VisionModel` (photo model), `SystemPrompt`, `DefaultNegative`, defaults for sampler/steps/CFG/size. Model fields are overwritten from settings (`llm_model`, `sd_prompt_model`, `vision_model`, `llm_generate_model`, `llm_analyze_model`).

Constants in `config`: default prompt instructions (tags and prose variants, analysis, garment description), `KidsModePrompt`, `MaxImageBytes` = 128 MB with `ImageTooLarge(what)`.

### imagecaster-mcp

| Variable / flag | Meaning |
|-----------------|---------|
| `-db` or `IMAGECASTER_MCP_DB` | DB path (otherwise `DB_PATH` / default) |
| `IMAGECASTER_MCP_OUT` | Output folder, default `~/imagecaster-mcp-out` |
| `IMAGECASTER_MCP_NO_IMAGE_BYTES=1` | Older tools do not return `image_base64` |

Old names with the `SD_MCP_` prefix are still read.

## Services and bindings by area

All bindings are exported methods of `*App` in `app.go`. Most only delegate to a service.

### Generation — `internal/generation`

| Bindings | What they do |
|----------|-------------|
| `GenerateSDPrompt`, `GetDefaultPromptInstruction`, `GetDefaultPromptInstructionProse` | LLM turns a description into a prompt (tags or prose, chosen by model). |
| `GenerateImage`, `GenerateCompoundImage`, `GenerateFromImage`, `UpscaleImage` | Style, pipeline (several steps), from image, upscale. |
| `EditImage`, `EditObject`, `RepairImage`, `ApplyLookDirect` | Instruction edit, remove/replace object by mask, repair by tiles, look (outfit/hair). |
| `AnalyzeImage`, `GetDefaultAnalyzePrompts`, `GetDefaultEditGarmentPrompt`, `DescribeForStyle` | Photo description by the vision model. |
| `CompilePostStyle` | LLM turns a text into a post-processing recipe. |
| `GetLastImage`, `ClearLastImage`, `SetLastImage` | Current image. |
| `InterruptGeneration` | Cancels this install's unfinished server jobs. |

Results are saved to the active history session. Kids mode filters input and output text and adds negative tags.

### Queue — `internal/queue`

| Bindings | What they do |
|----------|-------------|
| `EnqueueTxt2Img`, `EnqueueFromImage`, `EnqueueCompound`, `EnqueueLook`, `EnqueueEdit` | Add a job; returns its ID. |
| `GetQueue`, `RemoveQueueJob`, `RetryQueueJob`, `CancelQueueJob`, `CancelQueue`, `ClearCompletedQueueJobs` | Manage jobs. |
| `PauseQueue`, `ResumeQueue`, `IsQueuePaused`, `ResumePausedQueueJobs` | Pause and resume. |

- Table `job_queue`; types `txt2img`, `from_image`, `compound`, `look`, `edit`; statuses `pending`, `running`, `completed`, `failed`, `cancelled`, `paused`.
- One worker. Each job runs with an SD client scoped to that job (`sd.ForQueueJob`) and an LLM wrapper that saves answers to `data/queue-llm/<id>.json`.
- Network errors (connection refused, timeout, EOF, reset, unknown host, network unreachable) are retried after 5 s × 2ⁿ, at most 60 s; after `max_retries` (default 3) the job becomes `paused`. Other errors mark it `failed`.
- On start, jobs left `running` go back to `pending` (or `paused` if retries are used up).
- When a job ends, its server jobs are cleaned up in the background.

### History and sessions — `internal/session`, `internal/preset`

| Bindings | What they do |
|----------|-------------|
| `CreateSession`, `ListSessions`, `SwitchSession`, `RenameSession`, `DeleteSession` | Sessions. |
| `GetSessionItems`, `GetActiveSessionItem`, `SetActiveSessionItem`, `DeleteSessionItem`, `ClearSession`, `GetSessionImage`, `GetSessionThumb`, `HasSessionItems` | Items of the active session. |
| `ListHistory`, `OpenInHistory`, `DeleteHistoryItems` | Gallery over all history: filters by session, model, style/pipeline, type, shape, period, prompt text; facet counts. Search ignores case also for non-Latin text (SQL function `ic_lower`). |
| `ConfirmClose` | Answer to the close dialog, then quit. |

### Styles, pipelines, saved texts — `internal/preset`, `internal/importexport`

| Bindings | What they do |
|----------|-------------|
| `ListPresets`, `ListPresetsByType`, `GetPreset`, `CreatePreset`, `UpdatePreset`, `DeletePreset`, `GetAllTags` | Styles. |
| `ListPresetTypes`, `GetPresetType`, `CreatePresetType`, `UpdatePresetType`, `DeletePresetType` | Style types. |
| `ListCompoundPresets`, `GetCompoundPreset`, `CreateCompoundPreset`, `UpdateCompoundPreset`, `DeleteCompoundPreset` | Pipelines (name and at least one step required). |
| `ListResolutions` … `DeleteResolution`, `ListHiresProfiles` … `DeleteHiresProfile` | Sizes and hires profiles. |
| `ListDescriptions`, `CreateDescription`, `CreateDescriptionFull`, `UpdateDescription`, `DeleteDescription`, `ListPrompts`, `CreatePrompt`, `DeletePrompt` | Saved descriptions and prompts. |
| `ExportPresets`, `PreparePresetsExport`, `OpenImportFile`, `ValidateImportModels`, `ImportPresets` | Style import/export. |
| `ExportCompoundPresets`, `PrepareCompoundPresetsExport`, `OpenImportCompoundFile`, `ImportCompoundPresets` | Pipeline import/export. |

### Post-processing and export — `internal/adjust`, `internal/finish`, `internal/importexport`

| Bindings | What they do |
|----------|-------------|
| `PreviewAdjust`, `PreviewPost`, `AutoAdjust` | Correction preview (JPEG, limited width) and auto values. |
| `ExportImage` | Applies correction, size, format/frame/caption and saves. |
| `ListExportPresets`, `SaveExportPreset`, `DeleteExportPreset` | Export presets. |
| `ListPostRecipes`, `SavePostRecipe`, `DeletePostRecipe` | Post-processing recipes. |

### Files — `internal/filebrowser`, `internal/clipboard`, `internal/batch`

`ReadImageFile` (open dialog), `ReadClipboardImage`, `SaveImage` (save dialog), `FastSaveImage` (to the folder chosen in "My images"), `BrowseDirectory`, `ReadFileAsBase64`, `ReadThumbnail`, `SelectBrowserFolder`, `BatchSaveImage`, `BatchReset`.

### Server, models, settings — `internal/settings`, `internal/serverclient`

| Bindings | What they do |
|----------|-------------|
| `CheckServer`, `DiscoverServers`, `GetServerStatus`, `CheckServices` | Server check (`ok` / `not_configured` / `not_found` / `outdated`), mDNS search, status, SD and LLM health. |
| `GetSettings`, `UpdateSettings` | Settings. |
| `GetSDModels`, `GetSDSamplers`, `GetSDSchedulers`, `GetSDUpscalers`, `GetSDVAEs`, `GetSDLoRAs`, `GetLLMModels` | Lists from the server. |
| `GetServerModels`, `GetServerLLMModels`, `DownloadServerModel`, `DownloadServerModelStream`, `PullServerLLMModel`, `PullServerLLMModelStream`, `DeleteServerModel`, `DeleteServerLLMModel` | Model files on the server; stream variants send progress events. |
| `StartServerProcess`, `StopServerProcess`, `RestartServerProcess`, `GetServerProcessLogs` | Server processes. |
| `GetModelCatalog`, `GetDefaultDownloadPlan`, `InstallPresetDeps` | Embedded catalog, list of files the bundled styles need, download of one style's files. |
| `GetLLMSetup`, `SaveLLMChoice` | Pick the prompt model and the photo model. |
| `SyncPresets`, `GetModelPresets` | Install bundled presets for models present on the server (names kept in `auto_installed_presets`; deleted ones are not reinstalled) and return availability. |

### Kids mode — `internal/kids`

`IsKidsModeActive`, `SetKidsMode(enabled, pin)`, `GetKidsCategories`, `SetKidsCategory(name, enabled, pin)`. PIN is 4 digits, stored as a salted hash. Categories `nsfw` and `selfharm` are always on; `violence`, `horror`, `weapons`, `substances`, `mature` can be turned off with the PIN.

### Other

`Version`, `SaveWindowLayout`, `GetFooterHeight`, `GetLogEntries`.

## Events (backend → frontend)

| Event | When |
|-------|------|
| `sd:progress`, `sd:progress:start`, `sd:progress:stop` | Image job progress. |
| `llm:status` | LLM `thinking` / `done`. |
| `object:stage`, `repair:progress`, `test:progress` | Stages of object edit, repair, test generation. |
| `queue:started`, `queue:progress`, `queue:completed`, `queue:failed`, `queue:paused`, `queue:changed` | Queue. |
| `session:created`, `session:switched`, `session:deleted`, `session:added`, `session:active`, `session:selected`, `session:removed`, `session:cleared` | History. `session:created` with `external: true` — change made by `imagecaster-mcp`. |
| `server:download:progress:<jobID>` | One progress line of a model download. |
| `log:entry` | New log entry. |

## Error handling

- Style: `if err != nil`, wrap with `fmt.Errorf("...: %w", err)`. A binding that returns an error rejects the frontend promise with the error text.
- Input checks in bindings return plain errors (`no image provided`, `name is required`, `no save directory set …`). Images over 128 MB are rejected with `… too large (max 128 MB)`.
- `sd` client:
  - `APIError` — server answered with an error status (`sd server: <code>: <message> (status N)`);
  - `JobError` — job failed on the server (`code: message`);
  - `ErrUserCancel`, `ErrJobCancelled`, `ErrJobLost`, `ErrBadResponse`;
  - submit is retried up to 3 times on network errors, not on server errors; a lost job is resubmitted once, then `ErrJobLost`.
- `serverclient`: download streams end with `[DONE]`, `[ERROR] <text>` becomes an error, a broken connection is `ErrStreamLost` ("connection to server lost").
- Queue: retry rules above; a panic inside a job marks it `failed` with `panic: …`.
- Logging: `logger.Logger` keeps the last 500 entries for the Logs tab; the standard `log` output is captured into it. Errors that are not returned (cleanup, settings save) are logged as warnings.
