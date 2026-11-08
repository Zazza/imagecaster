[English](setup-en.md) | [Русский](setup-ru.md)

# Imagecaster — Service Setup Guide

## Contents

- [Stable Diffusion WebUI](#stable-diffusion-webui)
- [Ollama](#ollama)
- [LM Studio](#lm-studio)
- [llama.cpp](#llamacpp)
- [Imagecaster Server](#imagecaster-server)

---

## Stable Diffusion WebUI

Image generation engine. Imagecaster connects via REST API.

### Recommended: Forge Neo

Every feature works on Forge Neo, including Edit mode (Moss-Image) and Z-Image models. The easiest way to install it is [Imagecaster Server](#imagecaster-server) — the server downloads and sets up Forge Neo by itself.

Manually: [sd-webui-forge-classic, `neo` branch](https://github.com/Haoming02/sd-webui-forge-classic/tree/neo) — install per the repository README, run with `--listen --api`.

A1111 and the original Forge (options below) cover basic generation only: Z-Image models and Edit mode do not work on them.

### Option 1: A1111

```bash
git clone https://github.com/AUTOMATIC1111/stable-diffusion-webui.git
cd stable-diffusion-webui

# Start with network access
./webui.sh --listen --api --enable-insecure-extension-access
```

Windows:
```bat
webui.bat --listen --api --enable-insecure-extension-access
```

### Option 2: original Forge

[Stable Diffusion WebUI Forge](https://github.com/lllyasviel/stable-diffusion-webui-forge) is an optimized A1111 fork with significantly faster generation.

```bash
git clone https://github.com/lllyasviel/stable-diffusion-webui-forge.git
cd stable-diffusion-webui-forge

# Start with network access
./webui.sh --listen --api --enable-insecure-extension-access
```

Windows:
```bat
webui.bat --listen --api --enable-insecure-extension-access
```

> **Known Forge issue:** Hires Fix may trigger `argument of type 'NoneType' is not iterable` error. Imagecaster automatically retries generation without Hires Fix and shows a warning. For full Hires Fix support, use A1111.

> **Z-Image models:** the bundled `(Z-Image)` preset family uses `z-image-turbo` checkpoints. The original Forge does **not** support this architecture — use a fork with Z-Image support (e.g. [sd-webui-forge-classic, `neo` branch](https://github.com/Haoming02/sd-webui-forge-classic)); it is a drop-in replacement speaking the same REST API. All other bundled preset families work on any of the backends above.

> **Moss-Image model (Edit mode):** instruction editing needs the `moss_image_sft_bf16` checkpoint and Forge Neo (the reference engine). The checkpoint downloads from the catalog (Settings → Catalog); place the `qwen35_9b_int8_convrot.safetensors` text encoder and the `flux2-vae_small.safetensors` VAE manually into `models/text_encoder/` and `models/VAE/` (the Comfy-Org/MOSS-Image repo). The app wires the modules automatically when switching models.

### Flags

| Flag | Description |
|------|-------------|
| `--listen` | Allow connections from other devices |
| `--api` | Enable REST API |
| `--enable-insecure-extension-access` | Allow extension installation |
| `--port 7860` | Port (default: 7860) |
| `--xformers` | Accelerate generation (NVIDIA) |
| `--medvram` | Save VRAM (4–6 GB) |
| `--lowvram` | Minimal VRAM usage (2–4 GB) |

### Verification

Open `http://localhost:7860/sdapi/v1/sd-models` — should return JSON with model list.

### Models

Download `.safetensors` into `models/Stable-diffusion/`:

- [Civitai](https://civitai.com/) — main model repository
- [HuggingFace](https://huggingface.co/models?pipeline_tag=text-to-image)

LoRA -> `models/Lora/`, VAE -> `models/VAE/`.

---

## Ollama

Local LLM server. Imagecaster uses it for prompt generation and image analysis.

### Installation

**macOS / Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

**macOS (alternative):** Download from [ollama.com/download](https://ollama.com/download)

**Linux (manual):**
```bash
curl -L https://ollama.com/download/ollama-linux-amd64 -o /usr/local/bin/ollama
chmod +x /usr/local/bin/ollama
```

### Running

```bash
# Auto-start (macOS app / systemd)
ollama serve

# Run a model
ollama run llama3.2-vision
```

### Recommended Models

```bash
# For prompt generation (required)
ollama pull llama3.2

# For image analysis (vision model required)
ollama pull llama3.2-vision
ollama pull llava:13b
ollama pull minicpm-v
```

### Network Access

Ollama listens on `localhost` by default. To allow remote connections:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
```

Or set environment variable:
```bash
export OLLAMA_HOST=0.0.0.0:11434
```

systemd (`/etc/systemd/system/ollama.service`):
```ini
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
```

### Verification

```bash
curl http://localhost:11434/api/tags
```

In Imagecaster -> Settings -> Connection: select **Ollama**, URL `http://localhost:11434`.

---

## LM Studio

GUI application for LLMs. Easier to set up than Ollama.

### Installation

Download from [lmstudio.ai](https://lmstudio.ai/) (macOS / Windows / Linux).

### Setup

1. Open LM Studio
2. Download a model (Search tab):
   - `Llama 3.2 3B Instruct` — for prompt generation
   - `Llama 3.2 11B Vision Instruct` — for image analysis
3. Go to **Local Server** tab (arrow icon on the left panel)
4. Select a model from the dropdown
5. Click **Start Server**
6. Default port: `1234`

### Network Access

In LM Studio: Settings -> Advanced -> **Enable CORS** and set server host to `0.0.0.0`.

### Verification

```bash
curl http://localhost:1234/v1/models
```

In Imagecaster -> Settings -> Connection: select **LM Studio**, URL `http://localhost:1234`.

---

## llama.cpp

Minimal C++ LLM server. Good for low-end machines without Python.

### Installation

**Build from source:**
```bash
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp

# With CUDA
cmake -B build -DGGML_CUDA=ON && cmake --build build --config Release

# CPU only
cmake -B build && cmake --build build --config Release
```

**macOS (Homebrew):**
```bash
brew install llama.cpp
```

### Running the Server

```bash
# CPU
./llama-server -m model.gguf --host 0.0.0.0 --port 8081 -c 4096

# With GPU (CUDA)
./llama-server -m model.gguf --host 0.0.0.0 --port 8081 -ngl 99 -c 4096
```

| Parameter | Description |
|-----------|-------------|
| `-m` | Path to .gguf model file |
| `--host` | Bind address (`0.0.0.0` for network) |
| `--port` | Port number |
| `-ngl` | GPU layers (99 = all) |
| `-c` | Context size |

### Models

Download `.gguf` files from [HuggingFace](https://huggingface.co/models?search=gguf):

- [Llama 3.2 3B](https://huggingface.co/bartowski/Llama-3.2-3B-Instruct-GGUF)
- [Llama 3.2 Vision](https://huggingface.co/bartowski/Llama-3.2-11B-Vision-Instruct-GGUF) — for analysis

### Verification

```bash
curl http://localhost:8081/v1/models
```

In Imagecaster -> Settings -> Connection: select **llama.cpp**, URL `http://localhost:8081`.

> **Note:** llama.cpp does not support model switching via API. The model is set at server startup.

---

## Imagecaster Server

A separate program for the computer with the graphics card: it installs Forge Neo and Ollama into one folder, starts and restarts them, downloads models from the app's catalog and puts every GPU request through one queue so the card doesn't run out of memory. Imagecaster finds the server on the local network by itself (mDNS).

Download the archive for your system from [Releases](https://github.com/Zazza/imagecaster-server/releases), unpack and run it. The first start downloads Forge Neo, Python, PyTorch and Ollama — about 20 GB without models. Requires an NVIDIA graphics card with a recent driver.

Step-by-step setup for Windows, Linux and macOS, settings and API — in the [server README](https://github.com/Zazza/imagecaster-server/blob/main/README.md) and the [full documentation](https://github.com/Zazza/imagecaster-server/blob/main/docs/server-en.md).

---

## Quick Reference

| Service | Default Port | Imagecaster URL |
|---------|-------------|---------------|
| Stable Diffusion | 7860 | `http://localhost:7860` |
| Ollama | 11434 | `http://localhost:11434` |
| LM Studio | 1234 | `http://localhost:1234` |
| llama.cpp | 8081 | `http://localhost:8081` |
| Imagecaster Server | 8080 | `http://localhost:8080` |
