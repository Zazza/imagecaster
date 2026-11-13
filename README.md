# Imagecaster

**A desktop app that turns a plain-language description into a finished image — on your own graphics card.**

[Русский](README-ru.md) · [Changelog](CHANGELOG.md) · [Releases](https://github.com/Zazza/imagecaster/releases) · [Imagecaster Server](https://github.com/Zazza/imagecaster-server)

Stable Diffusion can do a lot, but every image means hand-tuning: model, sampler, CFG, LoRA weights, tags. Node editors can do everything, but you wire the graph yourself. Imagecaster is simpler: pick a style, describe the scene in any language, get the image. A local language model writes the prompt; the technical settings live in the style.

| | What you need to know | Cost per image | Your photos |
|---|---|---|---|
| Raw Stable Diffusion tools | samplers, CFG, LoRA, tag syntax | free, your hardware | stay with you |
| Node editors | all of the above plus wiring graphs | free, your hardware | stay with you |
| Cloud services | nothing | subscription or pay per image | uploaded to someone else |
| **Imagecaster** | **a style and a description** | **free, your hardware** | **stay on your network** |

---

## How it fits together

Imagecaster is two programs:

- **Imagecaster** (this repository) — the window you work in: styles, prompts, post-processing, history, queue.
- **[Imagecaster Server](https://github.com/Zazza/imagecaster-server)** — runs the models on the graphics card: [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) for images, [llama.cpp](https://github.com/ggml-org/llama.cpp) for the text and vision assistant, ONNX Runtime with DWPose for face and pose points. It stores the models and downloads them from the catalog.

They can run on one computer, or the server can live on another machine on your local network — the app finds it by itself. Without a server the app opens in limited mode: history, file browsing and non-AI post-processing still work.

```
 Imagecaster (desktop)                         Imagecaster Server (GPU machine)
 ┌──────────────────────────┐   local network  ┌────────────────────────────────┐
 │ styles · prompts · queue │ ───────────────▶ │ one GPU queue                  │
 │ history · gallery        │   jobs, models,  │  ├─ stable-diffusion.cpp       │
 │ SQLite (data/presets.db) │   assistant      │  ├─ llama.cpp (text, vision)   │
 └──────────────────────────┘                  │  └─ ONNX Runtime + DWPose      │
                                               │ models/ · engines/ · logs/     │
                                               └────────────────────────────────┘
```

## Screenshots

<p align="center">
  <a href="docs/screenshots/generate.png" target="_blank"><img src="docs/screenshots/generate.png" width="45%" alt="Generate"></a>
  <a href="docs/screenshots/postprocess.png" target="_blank"><img src="docs/screenshots/postprocess.png" width="45%" alt="Post-Process"></a>
</p>
<p align="center">
  <a href="docs/screenshots/gallery.png" target="_blank"><img src="docs/screenshots/gallery.png" width="45%" alt="Gallery"></a>
  <a href="docs/screenshots/styles.png" target="_blank"><img src="docs/screenshots/styles.png" width="45%" alt="Styles"></a>
</p>
<p align="center">
  <a href="docs/screenshots/catalog.png" target="_blank"><img src="docs/screenshots/catalog.png" width="45%" alt="Catalog"></a>
  <a href="docs/screenshots/queue.png" target="_blank"><img src="docs/screenshots/queue.png" width="45%" alt="Processing queue"></a>
</p>

## What it can do

**Generate**
- **Style** — pick a style, describe the scene in plain words; the assistant merges your description with the style into a prompt for the chosen model.
- **Workflow** — multi-step chains (for example draft → refine → detail) saved as reusable workflows.
- **Custom** — assemble a chain right on the page: per-step style, denoise and LoRAs; run it or save it as a workflow.
- Style tuning, direct prompts, resolution and seed series. Drop a photo onto the page to remix it.

**Post-Process** — cards applied to a photo, one or a batch:

| Card | What it does |
|---|---|
| Edit | Change a photo by instruction ("replace the jacket with a red coat") or describe a new look, optionally with a garment reference |
| Remix | Redraw a photo in another style or workflow |
| Object | Remove or replace an object by mask |
| Repair | Paint over scratches, dust and creases on an old photo — only those spots are fixed |
| Effect | Ready-made effects: light, film, weather, background |
| Correction | Brightness, contrast, levels, saturation, temperature, colour balance, sharpness, vignette — no AI |
| Format, frame, caption | Crop to a format, add a frame and a caption |
| Export settings | Size, format (PNG or JPEG), quality, resize method |
| Recipes | Save a set of steps and apply it to any photo |

**Gallery** — every image from every history in one place, with filters by history, period, model, style or workflow, type, shape and prompt text.

**My Images** — browse any folder and send pictures to Post-Process.

**Styles** — styles, effects and workflows as filterable tables: built-in or your own, by model and group; import and export.

**Catalog** — image models, LoRAs and assistant models with sizes and install status; one click downloads to the server.

**Bottom panel** — **History** (images grouped into histories, each remembers how it was made), **Log**, **Processing** (the queue).

**Also:** Safe Mode (PIN-protected content filter for children), light and dark theme, English and Russian interface, the `imagecaster-mcp` server for AI agents.

## Models

Imagecaster works with the families stable-diffusion.cpp supports. The catalog includes:

| Family | Good for |
|---|---|
| SD 1.5, SDXL | light models for small graphics cards, any style |
| Pony, Illustrious, Anima | anime, cartoon, stylized characters |
| Flux.1 | following detailed descriptions, realism |
| Z-Image Turbo | fast photorealism |
| Moss-Image | photorealistic people |
| Flux.2 Klein | post-processing: edits, effects, object replace and remove |
| Krea 2, Qwen-Image, Flux.3 | extra models with their own styles |

The assistant uses Qwen 3 models for prompts and Qwen 3 VL / Qwen 3.5 / Gemma 3 for photos. Details: [Models](docs/models-en.md).

## Download and install

Both programs are on the GitHub Releases pages. Install the server first.

### Imagecaster Server — [releases](https://github.com/Zazza/imagecaster-server/releases)

| System | Archive | How to start |
|---|---|---|
| Windows x64 | `imagecaster-server-<version>-windows-amd64.zip` | `imagecaster-server.exe` — a setup window, then an icon in the tray. `imagecaster-server-console.exe` — the same in a terminal |
| Linux x64 | `imagecaster-server-<version>-linux-amd64.tar.gz` | `./imagecaster-server` — terminal setup, then a dashboard. `--headless` for a systemd service |
| macOS (Apple Silicon) | `imagecaster-server-<version>-darwin-arm64.tar.gz` | `Imagecaster Server.command` — removes the download quarantine and opens Terminal |

A graphics card is recommended: NVIDIA uses the CUDA build, other cards the Vulkan build, Macs use Metal. Without a graphics card generation runs on the CPU and is very slow. Linux ARM, Windows ARM and Intel Macs have no stable-diffusion.cpp build. Details: [Setup](docs/setup-en.md).

### Imagecaster — [releases](https://github.com/Zazza/imagecaster/releases)

| System | Archive | Inside | Needs |
|---|---|---|---|
| Windows x64 | `imagecaster-windows-amd64.zip` | `imagecaster.exe`, `imagecaster-mcp.exe` | WebView2 (built into Windows 10/11) |
| Linux x64 | `imagecaster-linux-amd64.tar.gz` | `imagecaster`, `imagecaster-mcp` | GTK 3 and WebKitGTK 4.1 (`libwebkit2gtk-4.1-0`) |
| macOS (Apple Silicon) | `imagecaster-darwin-arm64.tar.gz` | `imagecaster.app` (`imagecaster-mcp` inside `Contents/MacOS`) | macOS 11 or newer |

No installer: unpack and run. The app keeps its data in a `data` folder next to the program, so unpack it where you can write.

The builds are not signed. Windows SmartScreen may warn on first start ("More info" → "Run anyway"). On a Mac, if the system refuses to open the app, remove the quarantine flag: `xattr -dr com.apple.quarantine imagecaster.app`. The macOS build has not been tested on a real Mac yet.

## First start

### Server

1. Asks for the data folder (default `~/imagecaster-server`; on Windows — in the setup window).
2. Picks the right engine build for your graphics card and installs stable-diffusion.cpp, llama.cpp, ONNX Runtime and the DWPose model. Image and assistant models are **not** downloaded here — the app does it.
3. Starts on port 8080 and announces itself on the local network.

### App

The app asks at most three things, in this order:

1. **Where is the server?** If no server is set or it does not answer, a window offers to search the local network (one click to connect to what it finds) or to type an address like `http://192.168.1.100:8080`; "Check again" repeats the check. If the server is too old, the window says what is missing and links to the update guide. "Close" opens limited mode with a banner on top.
2. **Welcome.** A short description of the app and a **Models** block: it checks which files the built-in styles need and shows how many are missing and their size, with a "Download all for presets" button. The download runs on the server — you can close the window. Other models are on the Catalog page. After an update this window shows "What's new" instead. Tick "Don't show at startup" to hide it.
3. **AI assistant models.** Two lists: the model for prompts and the model for photos. Each option shows whether it is installed, its download size, and which one is recommended. "Download and save" — or "Later".

Styles appear on their own: when the app gets the list of models from the server, it installs the built-in styles for the models the server has. Styles whose model is missing are hidden.

## How the queue works

Long work goes through a queue so you can keep working, close the app, or restart the computer.

**In the app**
- Batch post-processing, workflows, remix series and recipes become jobs in the **Processing** tab of the bottom panel. The queue is stored in the app database, so it survives a restart.
- A job that fails because the server is unreachable or a connection drops is retried twice, after 10 and 20 seconds; on the third failure it is paused. "Resume Paused" starts it again, "Retry" restarts a failed one.
- Pause, resume, cancel one job, or cancel everything.
- Each image is a separate job on the server with a stable name. If the app was closed mid-job, on the next start it picks up the server job where it is instead of starting over. Assistant answers already received for the job are kept, so the prompt is not rewritten. Closing the app does not cancel work on the server.

**On the server**
- One queue for the graphics card serves both image jobs and assistant requests, within a memory budget measured from the card's VRAM.
- Before a heavy image job the server unloads an idle assistant model to free memory; it never interrupts a running request.
- Finished results are kept for 24 hours for the app to collect. If a server restart cuts a job off, the app sends it once more.
- The server's terminal dashboard shows the queue (key `g`): memory budget, what runs, what waits; jobs can be moved or cancelled.

## AI assistant

The assistant is llama.cpp on the server, nothing else. It writes prompts from your description and the style, and describes photos for remix, edits and looks. Photos never leave your network: only the local vision model sees them. System prompts are editable in Settings → Prompts.

## For AI agents: imagecaster-mcp

`imagecaster-mcp` ships next to the app. It is an MCP server: an AI client (Claude Desktop, Claude Code, Codex and others) can list styles, generate, upscale, remix, edit by instruction, try looks (`stylist_describe` / `stylist_try`) and retouch (`editor_describe` / `editor_apply`). Results land in the app's history. Run it as the same user as the app — it uses the same database. The agent gets text only; photos are described by the local vision model.

## Build from source

Needs [Go](https://go.dev/dl/) 1.25+, [Node.js](https://nodejs.org/) 20+, [Wails CLI](https://wails.io/) v2.

| System | Extra |
|---|---|
| Windows | WebView2 |
| Linux | `libgtk-3-dev`, `libwebkit2gtk-4.1-dev` (`make build` and `make dev` add `-tags webkit2_41` on Linux) |
| macOS | Xcode command line tools (`xcode-select --install`) |

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
git clone https://github.com/Zazza/imagecaster.git
cd imagecaster
make setup   # Go and npm dependencies
make dev     # run with hot reload
make build   # build/bin/imagecaster
make mcp     # build/bin/imagecaster-mcp
```

Tests: `go test ./...` and `cd frontend && npx vitest run`.

## Project structure

```
├── main.go, app.go        # entry point; App — the single facade for the frontend (Wails bindings)
├── cmd/imagecaster-mcp/   # MCP server for AI agents
├── internal/
│   ├── serverclient/      # Imagecaster Server API: probe, discovery, downloads, model catalog
│   ├── sd/                # image jobs on the server (/api/sd/jobs), face points
│   ├── llm/               # assistant client (llama.cpp through the server)
│   ├── generation/        # generation service: styles, workflows, remix, edit, object
│   ├── queue/             # persistent job queue: retries, pause, resume after restart
│   ├── preset/            # SQLite: styles, workflows, settings, history, migrations
│   ├── session/           # histories and their images
│   ├── settings/          # settings, server check, "Download all"
│   ├── adjust/            # Correction card (no AI)
│   ├── finish/            # format, frame, caption
│   ├── faceguard/         # face points, retouch
│   ├── maskcomp/          # mask compositing
│   ├── batch/             # batch post-processing
│   ├── sdnames/           # sampler and scheduler names
│   ├── importexport/      # import and export of styles and workflows
│   ├── filebrowser/       # My Images
│   ├── kids/              # Safe Mode
│   ├── clipboard/, config/, logger/, promptutil/
│   └── mcp/               # imagecaster-mcp tools and prompts
├── frontend/src/          # Vue 3: components, composables, i18n (en, ru)
└── data/                  # built-in styles (presets/) and workflows (pipelines/)
```

## Documentation

| | |
|---|---|
| [Setup](docs/setup-en.md) | server and app installation, first start, building |
| [Models](docs/models-en.md) | model families, catalog, downloads |
| [Architecture](docs/architecture-en.md) | modules and how the app talks to the server |
| [Backend](docs/backend-en.md) | Go services |
| [Frontend](docs/frontend-en.md) | pages, components, state |
| [API reference](docs/api-reference-en.md) | Wails bindings and events |
| [Algorithms](docs/algorithms-en.md) | generation, workflows, queue |
| [Database](docs/database-en.md) | tables and migrations |

## License

[GNU Affero General Public License v3.0](LICENSE)
