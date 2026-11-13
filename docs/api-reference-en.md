[English](api-reference-en.md) | [Русский](api-reference-ru.md)

# Imagecaster — API Reference (Wails Bindings)

The frontend talks to Go only through Wails bindings: exported methods on `*App` in `app.go` (the only bound struct, see `Bind` in `main.go`). Wails generates JS wrappers in `frontend/src/wailsjs/go/main/App.js` / `App.d.ts`; the UI calls them through `frontend/src/api.js` (camelCase names, e.g. `GetSettings` → `api.getSettings()`).

There are 156 bindings; the list below matches `App.d.ts`. Every call returns a Promise. Go `error` becomes a rejected Promise.

Images travel as base64 strings (PNG or JPEG without the `data:` prefix). Saved history images are also served over HTTP by the Wails asset handler in `main.go`: `/api/img/<id>.jpg` (full image) and `/api/thumb/<id>.jpg` (thumbnail), where `<id>` is a session item id.

Server-dependent calls in `api.js` (`generateSdPrompt`, `generateImage`, `upscaleImage`, `analyzeImage`, `generateCompoundImage`, `editImage`, `editObject`, `repairImage`, `describeForStyle`, `applyLookDirect`, all `enqueue*`, `retryQueueJob`) are blocked on the frontend with the message `server.need_server` while the server check state is not `ok`.

## Common result: GenerateImageResult

Returned by all direct generation and edit methods (Go: `internal/generation/service.go`).

| Field | Meaning |
|---|---|
| `image` | result, base64 |
| `parameters`, `info` | generation details (JSON) |
| `effective_prompt`, `effective_negative_prompt` | final prompts sent to the model |
| `hires_fix_skipped`, `hires_fix_manual` | hires fix state |
| `quality_upscale_skipped`, `quality_redenoise_skipped` | quality-mode steps that were skipped |
| `detected_object` | object name found by Object edit |
| `session_item_id` | id of the history item the result was saved to |

## Generation (direct)

These run right away and wait for the result. Progress comes through events (see below).

| Binding | What it does |
|---|---|
| `GenerateSDPrompt(params)` | LLM writes an image prompt from a description for a style (`preset_id`, `description`, `negative`, `overrides`). Returns `{prompt, negative_prompt}`. |
| `GenerateImage(params)` | Text-to-image with a style: `preset_id`, `extra_prompt`, `extra_negative_prompt`, `resolution_id`, `hires_profile_id`, `seed`, `overrides`, `idea`, `idea_negative`. |
| `GenerateCompoundImage(params)` | Runs a workflow (`compound_preset_id`) or custom steps (`steps`) — several styles in a row. |
| `GenerateFromImage(params)` | Image-to-image from a photo (style or workflow, denoising, mask, keep color, preserve identity). Not called by the UI — the UI queues it with `EnqueueFromImage`. |
| `UpscaleImage(params)` | Upscales a generated image using its `gen_info` and style. |
| `EditImage(params)` | Edit a photo by a text instruction (`instruction`, optional reference image, style, LoRAs, `model`, resolution, seed). Post-processing → Edit. |
| `EditObject(params)` | Remove or replace a painted-over object (`mask_base64`, `op`, `replacement`, `object`). Post-processing → Object. |
| `RepairImage(params)` | Removes scratches, dust and creases inside a mask, tile by tile (`mask_base64`, `strength`). Post-processing → Repair. |
| `ApplyLookDirect(image, text, strength, seed)` | Applies a look described in words to a photo. |
| `AnalyzeImage(image, mode)` | Describes a photo with the vision LLM. `mode`: `describe` (prose), `garment` (clothing), anything else — tags. |
| `DescribeForStyle(image)` | Short 1–2 sentence photo description for picking an effect (Post-processing → Effect → Auto-style). |
| `CompilePostStyle(text)` | Turns a text wish into a recipe: up to 3 effect steps plus correction values. |
| `InterruptGeneration()` | Stops the current generation. |
| `GetLastImage()` / `ClearLastImage()` | Read / clear the last generated image. `GetLastImage` is not used by the UI. |

Default prompt texts (shown in Settings → Prompts):

| Binding | Returns |
|---|---|
| `GetDefaultPromptInstruction()` | default instruction for writing image prompts (tags) |
| `GetDefaultPromptInstructionProse()` | same, prose variant |
| `GetDefaultAnalyzePrompts()` | `{system_prompt, single_prompt, describe_prompt}` for photo description |
| `GetDefaultEditGarmentPrompt()` | default prompt for describing clothing from a reference |

## Queue

Background jobs stored in SQLite (`internal/queue`). Each `Enqueue*` returns the job id.

| Binding | What it does |
|---|---|
| `EnqueueTxt2Img(params)` | queue text-to-image (same params as `GenerateImage`) |
| `EnqueueCompound(params)` | queue a workflow or custom steps |
| `EnqueueFromImage(params)` | queue image-to-image / effect |
| `EnqueueLook(params)` | queue a look by text (`image_base64`, `text`, `strength`, `seed`) |
| `EnqueueEdit(params)` | queue an edit by instruction |
| `GetQueue()` | list of jobs |
| `RemoveQueueJob(id)` / `CancelQueueJob(id)` / `RetryQueueJob(id)` | remove, cancel, retry one job |
| `PauseQueue()` / `ResumeQueue()` / `IsQueuePaused()` | pause and resume the whole queue |
| `CancelQueue()` | cancel all jobs |
| `ResumePausedQueueJobs()` | put jobs paused after running out of retries back to pending; returns how many |
| `ClearCompletedQueueJobs()` | delete finished jobs |

## Styles (presets) and types

| Binding | What it does |
|---|---|
| `ListPresets()`, `ListPresetsByType(type)`, `GetPreset(id)` | read styles (`GetPreset` is not used by the UI) |
| `CreatePreset(p)`, `UpdatePreset(p)`, `DeletePreset(id)` | change styles |
| `ListPresetTypes()`, `GetPresetType(id)`, `CreatePresetType(t)`, `UpdatePresetType(t)`, `DeletePresetType(id)` | style types (generation types and effect types `post`, `post_*`) |
| `GetAllTags()` | all tags used by styles |
| `SyncPresets()` | installs bundled styles for models the server has; returns per-style statuses and the number added |
| `GetModelPresets()` | which bundled styles belong to which model |
| `InstallPresetDeps(id)` | downloads the model and missing LoRAs of a style to the server from the catalog |

## Workflows (compound presets)

| Binding | What it does |
|---|---|
| `ListCompoundPresets()`, `GetCompoundPreset(id)` | read workflows with steps |
| `CreateCompoundPreset(cp)`, `UpdateCompoundPreset(cp)` | save; name and at least one step are required |
| `DeleteCompoundPreset(id)` | delete |

## Resolutions and hires profiles

| Binding | What it does |
|---|---|
| `ListResolutions()`, `GetResolution(id)`, `CreateResolution(r)`, `UpdateResolution(r)`, `DeleteResolution(id)` | output sizes; width and height 64–4096 |
| `ListHiresProfiles()`, `GetHiresProfile(id)`, `CreateHiresProfile(h)`, `UpdateHiresProfile(h)`, `DeleteHiresProfile(id)` | hires profiles; upscale 1.0–4.0, denoising 0–1 |

`GetResolution` and `GetHiresProfile` are not used by the UI.

## Saved descriptions and prompts

| Binding | What it does |
|---|---|
| `ListDescriptions()`, `CreateDescription(text)`, `CreateDescriptionFull(d)`, `UpdateDescription(d)`, `DeleteDescription(id)` | saved descriptions (ideas) for Generate |
| `ListPrompts()`, `CreatePrompt(text)`, `DeletePrompt(id)` | saved prompts; not used by the UI |

## Post-processing (local, no AI)

| Binding | What it does |
|---|---|
| `PreviewAdjust(image, adjust, maxW)` | preview of Correction (brightness, contrast, color…), JPEG, scaled to `maxW` |
| `PreviewPost(image, adjust, finish, maxW)` | preview of Correction plus Format/frame/caption |
| `AutoAdjust(image)` | measures the image and returns automatic correction values |
| `ExportImage(params)` | applies export settings and saves (to `output_dir` or via a save dialog) |
| `ListExportPresets()`, `SaveExportPreset(p)`, `DeleteExportPreset(id)` | saved export settings |
| `ListPostRecipes()`, `SavePostRecipe(r)`, `DeletePostRecipe(id)` | saved recipes (chains of effects and corrections) |
| `BatchSaveImage(image)` / `BatchReset()` | temporary files for batch post-processing (`data/batch`, cleared at startup) |

## History (sessions) and gallery

| Binding | What it does |
|---|---|
| `CreateSession(name)`, `ListSessions()`, `SwitchSession(id)`, `RenameSession(id, name)`, `DeleteSession(id)` | history groups (sessions) |
| `GetSessionItems()`, `GetActiveSessionItem()`, `SetActiveSessionItem(id)` | items of the current session and the selected one |
| `DeleteSessionItem(id)`, `ClearSession()` | delete one item / all items of the current session |
| `GetSessionImage(id)`, `GetSessionThumb(id)` | image / thumbnail as base64 |
| `HasSessionItems()` | whether there are items; not used by the UI |
| `ListHistory(filter)` | Gallery: items across all sessions with filters (sessions, models, labels, kinds, shapes, search, dates, periods), paging and counts |
| `OpenInHistory(id)` | switches to the item's session and selects the item |
| `DeleteHistoryItems(ids)` | deletes several items; returns how many |
| `SetLastImage(image)` | adds an image (e.g. from My Images) to the current session and selects it |
| `ConfirmClose(action)` | quits the app; `discard` first deletes all sessions. Not used by the UI. |

## Files, clipboard, window

| Binding | What it does |
|---|---|
| `ReadImageFile()` | file dialog for PNG/JPEG, returns base64 |
| `ReadClipboardImage()` | image from the clipboard |
| `SaveImage(image, name)` | save PNG via a save dialog |
| `FastSaveImage(image, name, format)` | save to the My Images folder without a dialog; not used by the UI |
| `BrowseDirectory(path)`, `ReadFileAsBase64(path)`, `ReadThumbnail(path)` | My Images: list a folder, read a file, read a thumbnail |
| `SelectBrowserFolder()` | folder dialog; remembers the folder in `file_browser_path` |
| `SaveWindowLayout(footerHeight)`, `GetFooterHeight()` | window size/position and footer height are stored in settings |
| `Version()` | app version |
| `GetLogEntries()` | app log (footer → Log) |

## Settings, safe mode

| Binding | What it does |
|---|---|
| `GetSettings()` / `UpdateSettings(map)` | key–value settings from the `settings` table |
| `IsKidsModeActive()`, `SetKidsMode(enabled, pin)` | safe mode on/off with a PIN |
| `GetKidsCategories()`, `SetKidsCategory(name, enabled, pin)` | blocked content categories |

## Imagecaster Server

| Binding | What it does |
|---|---|
| `CheckServer()` | checks `server_url`: `state` = `ok`, `not_configured`, `not_found` or `outdated` (with `missing` features and `reason` `old_version` / `missing_features`) |
| `DiscoverServers()` | finds servers in the local network (mDNS) |
| `CheckServices()` | footer status: whether the LLM and image generation are available and which models are loaded |
| `GetServerStatus()` | server processes, health, GPU, installs, models |
| `StartServerProcess(name)`, `StopServerProcess(name)`, `RestartServerProcess(name)`, `GetServerProcessLogs(name, lines)` | control server processes (Settings → Connection) |
| `GetServerModels(type)`, `GetServerLLMModels()` | files on the server by type (`sd`, `lora`, `vae`, `text_encoder`…) and LLM models |
| `DownloadServerModel(type, url, file)`, `DeleteServerModel(type, file)` | download / delete a model file on the server |
| `DownloadServerModelStream(jobID, type, url, file)` | same with progress via event `server:download:progress:<jobID>` |
| `PullServerLLMModel(name)`, `PullServerLLMModelStream(jobID, name)`, `DeleteServerLLMModel(name)` | download / delete an LLM model |
| `GetModelCatalog()` | model catalog for the Catalog page |
| `GetDefaultDownloadPlan()` | files the bundled styles need and their total size (Welcome → Models) |
| `GetLLMSetup()` / `SaveLLMChoice(generate, vision)` | first-run choice of the prompt model and the photo (vision) model |
| `GetSDModels()`, `GetSDSamplers()`, `GetSDSchedulers()`, `GetSDUpscalers()`, `GetSDVAEs()`, `GetSDLoRAs()` | lists for style editing, read from the server |
| `GetLLMModels()` | LLM models available on the server |

## Import / export

| Binding | What it does |
|---|---|
| `ExportPresets(ids)` / `PreparePresetsExport(ids)` | save styles to a JSON file (save-file dialog / folder dialog) |
| `OpenImportFile()` | pick one or more JSON files, returns a preview of styles |
| `ValidateImportModels(items)` | warnings for models missing on the server |
| `ImportPresets(items)` | import styles |
| `ExportCompoundPresets(ids)` / `PrepareCompoundPresetsExport(ids)` | save workflows to JSON |
| `OpenImportCompoundFile()`, `ImportCompoundPresets(items)` | import workflows |

## Events (backend → frontend)

Emitted with `runtime.EventsEmit`, received with `EventsOn` from `wailsjs/runtime/runtime`.

| Event | Payload | Emitted by | Listened in |
|---|---|---|---|
| `llm:status` | `{status: "thinking" \| "done"}` | generation (LLM calls) | `useGenerationProgress.js` |
| `sd:progress` | progress, ETA, step, preview | generation | `useGenerationProgress.js` |
| `object:stage` | `"analyzing"` / `"generating"` | Object edit | `ObjectSection.vue` |
| `repair:progress` | `{done, total}` tiles | Repair | `RepairSection.vue` |
| `queue:changed` | — | queue | `useQueue.js`, `AppFooter.vue`, `PostProcessPage.vue` |
| `queue:started` | `{job_id, type}` | queue worker | `useQueue.js`, `AppFooter.vue` |
| `queue:progress` | `{job_id, progress, detail}` | queue worker | `useQueue.js` |
| `queue:completed` | `{job_id, result}` | queue worker | `useQueue.js`, `AppFooter.vue`, `GeneratePage.vue`, `RemixSection.vue`, `PostProcessPage.vue` |
| `queue:failed` | `{job_id, error}` | queue worker | same as above |
| `queue:paused` | `{job_id, error, retry_count, max_retries}` | queue worker on a retryable error: the job is retried later, or paused once retries run out | `useQueue.js`, `GeneratePage.vue`, `RemixSection.vue`, `PostProcessPage.vue` |
| `session:added` | `{id}` | new history item | `AppFooter.vue`, `GeneratePage.vue` |
| `session:active` | `{id}` | item selected | `AppFooter.vue` |
| `session:selected` | `{id}` | item selected by the user or via `SetLastImage` | `PostProcessPage.vue` |
| `session:removed` | `{id}` or `{ids}` | items deleted | `AppFooter.vue` |
| `session:cleared` | — | session cleared | `AppFooter.vue` |
| `session:created` | `{session_id, name}` or `{external: true}` | new session; `external` — created by imagecaster-mcp (checked every 2 s) | `AppFooter.vue` |
| `session:switched` | `{session_id}` | session switched | `AppFooter.vue` |
| `session:deleted` | `{session_id}` | session deleted | `AppFooter.vue` |
| `log:entry` | JSON log entry | `internal/logger` | `AppFooter.vue` |
| `server:download:progress:<jobID>` | progress text | `DownloadServerModelStream`, `PullServerLLMModelStream` | `api.js` |

Also emitted but not listened to by the UI: `compound:progress`, `fromimage:progress`, `sd:progress:start`, `sd:progress:stop`.
