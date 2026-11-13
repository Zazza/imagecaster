[English](architecture-en.md) | [Русский](architecture-ru.md)

# Imagecaster — Architecture

Imagecaster is a desktop app. It does not run models itself: all image generation and all LLM work (prompts, photo descriptions) happen on **Imagecaster Server** — a separate program ([imagecaster-server](https://github.com/Zazza/imagecaster-server)) on this or another computer in the local network.

## Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25, module `github.com/Zazza/imagecaster` |
| Desktop | Wails v2 |
| Frontend | Vue 3 + Vite |
| Database | SQLite, `modernc.org/sqlite`, raw SQL (no ORM) |
| Images | stable-diffusion.cpp on the server |
| Text and vision LLM | llama.cpp on the server |
| Face/pose points | ONNX Runtime + DWPose on the server |

Two binaries are built from this repository:

| Binary | Source | Purpose |
|--------|--------|---------|
| `imagecaster` | `main.go`, `app.go` | GUI app |
| `imagecaster-mcp` | `cmd/imagecaster-mcp` | MCP server for AI agents (stdio, JSON-RPC) |

## Diagram

```
┌──────────────────────── desktop ─────────────────────────┐
│                                                           │
│  Vue 3 UI ──Wails bindings──▶ App (app.go)                │
│     ▲                          │                          │
│     └──── Wails events ◀───────┤                          │
│                                ▼                          │
│        generation · queue · session · settings · ...      │
│                │                        │                 │
│                ▼                        ▼                 │
│         sd / llm / serverclient     SQLite + data/        │
│                │                        ▲                 │
│  imagecaster-mcp ───────────────────────┘ (same DB)       │
└────────────────┼──────────────────────────────────────────┘
                 │ HTTP (LAN)
                 ▼
┌──────────── Imagecaster Server (default port 8080) ───────┐
│  GET /                 name, version, features            │
│  /api/sd/...           image jobs (stable-diffusion.cpp)  │
│  /api/llm/...          chat and vision (llama.cpp)        │
│  /api/server/...       status, models, downloads, logs    │
└───────────────────────────────────────────────────────────┘
```

## Module map

### Root

| File | Role |
|------|------|
| `main.go` | Loads config, opens the DB, creates clients, restores the server address, starts Wails. Serves session images to the UI at `/api/img/<id>` and thumbnails at `/api/thumb/<id>`. |
| `app.go` | `App` — the facade. Every Wails binding is an exported method on `*App`. Wires all services together. |

### `internal/`

| Package | Role |
|---------|------|
| `adjust` | Pixel corrections: levels, brightness, contrast, saturation, temperature, sharpen, vignette; auto-analysis (`AnalyzeAuto`). |
| `batch` | Temporary folder `data/batch` for batch image saving; cleared on start. |
| `clipboard` | Reads an image from the system clipboard. |
| `config` | `Config` struct, default prompts, allowed setting keys, image size limit (128 MB). |
| `faceguard` | Face work by face points: pastes eyes, nose and lips from the source photo into an edited one, rejects a different face, retouch. Used by the stylist and the MCP photo editor. |
| `filebrowser` | Lists folders and reads image files / thumbnails for the "My images" page. |
| `finish` | Format (crop or fit to ratio), frame and caption. |
| `generation` | Main service: prompt generation, txt2img, pipelines, from-image, edit, object remove/replace, repair, effects, look, post-processing recipes, photo descriptions. |
| `importexport` | Import/export of styles and pipelines; image export (format, size, finish). |
| `kids` | Kids mode: PIN, content categories, filters for input text, LLM output and negative prompt. |
| `llm` | HTTP client to the server LLM (`/api/llm`). |
| `logger` | In-app log: ring buffer of 500 entries, `log:entry` event, captures the standard Go `log`. |
| `maskcomp` | Mask helpers: composite edited area into the original, blend, outline, keep color. |
| `mcp` | MCP server: tools, prompts `stylist` and `photo_editor`. |
| `preset` | SQLite repository: styles, pipelines, settings, sessions/history, resolutions, hires profiles, export presets, post recipes; schema migrations; bundled presets. |
| `promptutil` | LLM answer cleanup: extract JSON, strip junk, dedupe tags. |
| `queue` | Persistent job queue (`job_queue` table) with retries and pause. |
| `sd` | Client for server image jobs (`/api/sd/jobs`), model lists, face points. |
| `sdnames` | Converts old sampler/scheduler/upscaler names from earlier versions to stable-diffusion.cpp names. |
| `serverclient` | Server probe, mDNS discovery, model management and download streams, embedded model catalog. |
| `session` | Image history: sessions, items, files on disk, events; watches the DB for changes made by `imagecaster-mcp`. |
| `settings` | Settings, server check, server address, model downloads, LLM setup, preset sync. |

### `cmd/imagecaster-mcp`

Separate process started by an MCP client. It opens the same `presets.db`, builds the same `generation` service and talks to the same server. Images it makes go to app sessions ("AI agent", "Stylist", "Photo editor"); the running app sees them by polling the DB every 2 s. Files are also written to `~/imagecaster-mcp-out` (or `IMAGECASTER_MCP_OUT`). It does not use the job queue.

Photos never leave the local network: photo tools refuse to work if the server address is not local (loopback, private or link-local IP), and return only text produced by the server's local vision model.

### `frontend/`

| Path | Role |
|------|------|
| `src/App.vue` | Shell: sidebar, pages, footer, first-run modals. |
| `src/components/` | Pages and widgets (Generate, Post-processing, Gallery, My images, Styles, Catalog, Settings, Queue panel, modals). |
| `src/composables/` | Shared state: `useQueue`, `usePresets`, `useGenerationProgress`, `useKidsMode`, `useMaskCanvas`, `useZoomPan`, `useWindowFrame`. |
| `src/api.js` | Thin layer over Wails bindings; subscribes to download progress events. |
| `src/i18n/` | Russian and English strings. |
| `src/wailsjs/` | Generated Wails bindings — do not edit. |

## How the app talks to the server

### Connection

- One setting: `server_url`. SD and LLM addresses are derived from it: `<server_url>/api/sd` and `<server_url>/api/llm`.
- Discovery: mDNS service `_imagecaster._tcp`; manual address is also possible. Server default port is 8080.
- Check at start (`CheckServer`): `GET /` must return name `Imagecaster Server` and features `sdcpp`, `download_stream`, `sd_jobs`, `llamacpp`. Result: `ok`, `not_configured`, `not_found` or `outdated` (with missing features). The UI shows a modal for anything but `ok`; closing it leaves the app in a limited mode.

### Image jobs

- Every image operation is a server job: `POST /api/sd/jobs` with `kind` (`txt2img`, `img2img`, `upscale`), parameters and its own model spec (checkpoint, VAE, modules). The server keeps no shared "current model" for the app.
- The client polls `GET /api/sd/jobs/<id>`, then takes `GET /api/sd/jobs/<id>/result`. Progress goes to the UI as the `sd:progress` event.
- Each job has a `client_ref` starting with the install ID. "Stop" cancels this install's unfinished jobs (`POST /api/sd/jobs/<id>/cancel`).
- A lost job (status `lost`, or `failed` with `server_restarted`, or 404) is resubmitted once.
- Server side: one GPU queue for images and LLM with a memory budget from VRAM; job results are kept 24 h; jobs unfinished at server restart end with `server_restarted`.

### Queue jobs survive restarts

- Steps of an app queue job use `client_ref` = `<install_id>:q<queue id>:s<step>:<hash>`. The server replays a job with the same ref, so after an app restart the step continues instead of starting over.
- LLM answers inside a queue job are saved to `data/queue-llm/<id>.json` and reused on retry.
- Closing the app does not cancel server jobs.

### LLM

The LLM client calls the server under `/api/llm`: OpenAI-style `/v1/chat/completions` for text, `/api/chat` with images for vision, plus endpoints for the model list, loaded models and unloading.

### Model downloads

Downloads run on the server. The app opens a stream (`/api/server/models/download/stream`, `/api/server/models/llm/pull`), reads `data:` lines until `[DONE]` or `[ERROR] …`, and forwards each line to the UI as the event `server:download:progress:<jobID>`. If the connection drops, the error is "connection to server lost".

## Data on disk

All next to the DB (default `data/` beside the executable, or `DB_PATH`):

| Path | Content |
|------|---------|
| `presets.db` | SQLite database |
| `sessions/<session>/` | Images of history sessions |
| `thumbs/<session>/` | Thumbnails |
| `queue-llm/<id>.json` | LLM answers of a queue job |
| `batch/` | Temporary batch files |

## Principles

- **Facade:** the frontend calls only `App` methods.
- **Interfaces for external services:** `llm.Service`, `sd.Service` — tests swap them for fakes.
- **Constructor injection:** `New*` functions; wiring in `main.go` / `NewApp` and in `cmd/imagecaster-mcp/main.go`.
- **Repository:** `preset.DB` hides SQL.

## Tests

- Go: `go test ./...` (table-driven tests, `httptest` servers for clients, in-memory SQLite).
- Frontend: `cd frontend && npm test` (Vitest).
