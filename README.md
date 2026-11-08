# Imagecaster

**AI-powered desktop app for image generation with Stable Diffusion + LLM.**

[Русский](README-ru.md) | [Changelog](CHANGELOG.md)

**Why, and how it differs.** Stable Diffusion can do a lot, but every image means hand-tuning: sampler, CFG, LoRA weights, tags. ComfyUI can do everything, but it is a node construction kit you have to wire yourself. Imagecaster is simpler: pick a style, describe the scene in plain language, get the image. An LLM writes the SD prompt, and the technical settings live in the style. Everything runs on your own machine, so you can try as many variants as you like and pay nothing for them.

| | What you need to know | Cost per variant | For whom |
|---|---|---|---|
| Raw SD (WebUI) | samplers, CFG, LoRA, tag syntax | free, on your hardware | people ready to dig into settings |
| ComfyUI | all of the above, plus wiring node graphs | free, on your hardware | tinkerers who want full freedom |
| Cloud services | nothing | subscription or pay per generation | people with no hardware to run a model |
| **Imagecaster** | **nothing: a style and a plain-language description** | **free, on your hardware** | **people who want the image, not the settings** |

> **Note:** Imagecaster connects to your local Stable Diffusion WebUI ([Forge Neo](https://github.com/Haoming02/sd-webui-forge-classic/tree/neo) — see [Setup Guide](docs/setup-en.md)) and an [OpenAI-compatible LLM](https://ollama.com). Both run on your hardware — no cloud, no subscriptions, no data leaving your machine.

---

## Highlights

- **LLM Prompt Engineering** — describe in natural language, LLM merges your intent with a style preset into a production-ready SD prompt
- **Edit Mode** — instruction-based editing of real photos: "replace the pants with a skirt" changes exactly that, keeping the person, pose and scene
- **Smart Remove** — draw a mask, LLM analyzes the context, inpaints the background automatically
- **Custom Mode & Pipelines** — assemble multi-step chains inline or save them as reusable workflows
- **Quality Output** — native resolution → ESRGAN upscale → detail redenoise, one checkbox
- **Model Catalog** — one-click download of recommended models and LoRAs (server mode)
- **Session Management** — every image remembers how it was created; organize work into sessions
- **Kids Mode** — PIN-protected content filtering with category controls

## Screenshots

<p align="center">
  <a href="docs/screenshots/main-generation.png" target="_blank"><img src="docs/screenshots/main-generation.png" width="45%" alt="Text-to-Image generation"></a>
  <a href="docs/screenshots/edit.png" target="_blank"><img src="docs/screenshots/edit.png" width="45%" alt="Edit mode — instruction-based editing"></a>
</p>
<p align="center">
  <a href="docs/screenshots/from-image.png" target="_blank"><img src="docs/screenshots/from-image.png" width="45%" alt="Remix — image-to-image"></a>
  <a href="docs/screenshots/presets.png" target="_blank"><img src="docs/screenshots/presets.png" width="45%" alt="Style presets library"></a>
</p>
<p align="center">
  <a href="docs/screenshots/settings-catalog.png" target="_blank"><img src="docs/screenshots/settings-catalog.png" width="45%" alt="Model catalog with one-click downloads"></a>
  <a href="docs/screenshots/compare.png" target="_blank"><img src="docs/screenshots/compare.png" width="45%" alt="Compare — side-by-side model testing"></a>
</p>

<p align="center">
  <a href="docs/screenshots/edit-viewer.png" target="_blank"><img src="docs/screenshots/edit-viewer.png" width="45%" alt="Fullscreen viewer with generation meta"></a>
  <a href="docs/screenshots/edit-viewer-zoom.png" target="_blank"><img src="docs/screenshots/edit-viewer-zoom.png" width="45%" alt="Viewer zoom controls"></a>
</p>

## Features

### Generation

| Feature | Description |
|---------|-------------|
| Text-to-Image | Generate from text with style presets, LoRA stacks, sampler overrides |
| Image-to-Image (Remix) | Transform existing images with denoising control, batch count, quality mode |
| Inpainting | Canvas mask editor with fullscreen mode, brush controls, undo |
| Smart Remove | AI-powered object removal — draw mask, context auto-analyzed by LLM vision |
| **Edit Mode** | True instruction editing on reference-capable models (Moss-Image on Forge Neo): the model changes what you ask for and keeps the face, pose and scene. Style presets contribute LoRAs without restyling the photo |
| Custom Mode | Assemble a pipeline inline (per-step presets, denoise and LoRAs), run it or save as a workflow |
| Pipelines | Multi-step compound presets with per-step LoRAs, import/export as JSON |
| Quality Output | Native-resolution pass → neural upscale into the target resolution → low-denoise detail pass; skip warnings surfaced in the UI |
| Batch Generation | N images with live progress, interrupt and queue recovery |
| Hires / Resolution Profiles | Independent DB-stored profiles; hires fix profiles with automatic fallback on unsupported backends |
| Upscale | Preview mode with one-click upscale to full resolution; neural upscaler resolved from the backend's list |
| Explicit Seed | Optional seed with batch `seed+i` semantics |

### LLM Integration

| Feature | Description |
|---------|-------------|
| Smart Merge | Natural language description → merged SD prompt via LLM; prose-prompt instruction for Flux/Z-Image-class models |
| Vision Analysis | Analyze uploaded images (quick or deep chain mode) straight into a remix |
| Memory Guard | LLM models auto-unload from VRAM before heavy SD generations — only when it actually helps |
| Customizable Instructions | Edit the system prompts that shape LLM output |

### Workflow & Management

| Feature | Description |
|---------|-------------|
| Sessions | Project-based sessions; every item stores its generation meta (model, LoRAs, pipeline chain) |
| Presets | Save, organize by type, import/export with model validation |
| Compare | Side-by-side comparison of presets and models |
| Model Catalog | Recommended models/LoRAs/LLMs with install status and one-click download (server mode) |
| File Browser | Thumbnail grid, fullscreen viewer with zoom, send to Remix/Edit/Export |
| Export | Resize, convert (PNG/JPEG/WebP), quality/interpolation control, export presets |
| Queue | Persistent job queue with retry, pause/resume and recovery after restarts |
| MCP Server | Built-in Model Context Protocol server (`imagecaster-mcp`) — drive the studio from AI agents |
| Light/Dark Theme | System-aware theme with manual toggle; RU/EN interface |

### Safety

| Feature | Description |
|---------|-------------|
| Kids Mode | PIN protection, content filtering by category, safe prompt modification |

## Models

| Model | Best for |
|-------|----------|
| SD 1.5 / SDXL | Light models for low-end graphics cards, any style |
| Pony / Illustrious | Anime, cartoon and stylized characters |
| Flux 1 | Precise following of detailed descriptions, realism |
| Z-Image Turbo | Fast photorealism |
| Moss-Image | Precise following of descriptions, photorealistic people, Edit mode |
| Flux 2 Klein | Post-processing effects, object replace/remove, keeping the face |

Details and the full preset list: [Models and Presets](docs/models-en.md).

## Requirements

### External Services

Imagecaster connects to two services on your local network:

- **Stable Diffusion WebUI** — [Forge Neo](https://github.com/Haoming02/sd-webui-forge-classic/tree/neo) required, run with `--api` (A1111 and the original Forge cover basic generation only). Works with modern models out of the box (Z-Image Turbo, FLUX.1 fp8, Moss-Image — the catalog can download them). Default: `http://localhost:7860`. See [Setup guide](docs/setup-en.md).
- **LLM API** — any OpenAI-compatible server: [Ollama](https://ollama.com/), [llama.cpp](https://github.com/ggerganov/llama.cpp), or [LM Studio](https://lmstudio.ai/). Default: `http://localhost:11434/v1`

Easiest: [Imagecaster Server](https://github.com/Zazza/imagecaster-server) installs Forge Neo and Ollama on the computer with the graphics card, runs them and downloads models from the catalog.

### Development

- [Go](https://go.dev/dl/) >= 1.25
- [Node.js](https://nodejs.org/) >= 18
- [Wails CLI](https://wails.io/) v2

| Platform | Requirements |
|----------|-------------|
| macOS | Xcode CLI tools (`xcode-select --install`), 10.15+ |
| Linux | `libgtk-3-dev`, `libwebkit2gtk-4.1-dev` |
| Windows | [WebView2](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) (built into Win 10/11) |

## Download

Pre-built releases are available on the [Releases page](https://github.com/Zazza/imagecaster/releases).

## Quick Start

```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Clone and run
git clone https://github.com/Zazza/imagecaster.git
cd sd-ai
make setup   # first-time: download deps
make dev     # launch with hot-reload
```

Frontend hot-reload at `http://localhost:34115`.

### Build

```bash
make build   # production binary → build/bin/imagecaster
```

## Server

The server component has been extracted to a separate repository: [imagecaster-server](https://github.com/Zazza/imagecaster-server). In server mode the app manages the backend lifecycle (start/stop/restart, GPU stats) and can download models straight from the catalog.

## Tech Stack

| Layer | Stack |
|-------|-------|
| Backend | Go 1.25 |
| Desktop | [Wails](https://wails.io/) v2 |
| Frontend | Vue 3 + Vite |
| Database | SQLite (pure Go, no CGo) |
| LLM | OpenAI-compatible API |
| Image Gen | Stable Diffusion WebUI API (Forge Neo) |

## How It Works

```
User writes description in plain language
         │
         ▼
  LLM Smart Merge
  (description + style preset + instruction → SD prompt)
         │
         ▼
  Generate via Stable Diffusion WebUI
         │
         ▼
  Preview → Quality upscale → Export
```

**From Image (Remix):** upload → optional vision analysis → img2img / inpaint / smart remove, batch and quality mode

**Edit:** upload a photo → write an instruction in any language ("replace the pants with a skirt") → the reference-edit engine changes exactly that; iterate on the result

## Project Structure

```
├── main.go              # Entrypoint
├── app.go               # Wails RPC bindings
├── internal/
│   ├── config/          # Configuration
│   ├── llm/             # LLM client
│   ├── preset/          # SQLite CRUD
│   ├── sd/              # Stable Diffusion client
│   ├── generation/      # Image generation service (incl. Edit, quality, memory guard)
│   ├── queue/           # Job queue with retry and paused state
│   ├── session/         # Session management
│   ├── importexport/    # Preset import/export
│   ├── settings/        # Settings service
│   ├── promptutil/      # Prompt utilities
│   ├── filebrowser/     # File browser backend
│   ├── serverclient/    # Server API client + embedded model catalog
│   ├── kids/            # Kids mode filtering
│   ├── logger/          # Event logger
│   └── mcp/             # MCP server implementation
├── cmd/imagecaster-mcp/ # MCP server entrypoint
├── frontend/
│   └── src/
│       ├── components/  # Vue components
│       ├── composables/ # Vue composables (useQueue, etc.)
│       └── wailsjs/     # Auto-generated Wails bindings
└── data/                # Bundled preset seeds
```

## License

[GNU Affero General Public License v3.0](LICENSE)
