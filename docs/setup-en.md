[English](setup-en.md) | [Русский](setup-ru.md)

# Imagecaster — Setup

Imagecaster is two programs:

- **Imagecaster Server** ([repository](https://github.com/Zazza/imagecaster-server)) runs on the computer with the graphics card. It installs and starts stable-diffusion.cpp (images) and llama.cpp (text and photo-reading models), downloads models and puts every GPU request through one queue.
- **Imagecaster** (this app) runs on the same computer or any other one in the local network. It does all its AI work through the server.

Install the server first.

## Install the server

Download the archive for your system from [server releases](https://github.com/Zazza/imagecaster-server/releases) and unpack it anywhere. Every archive has a `.sha256` file next to it.

| System | Archive | Inside |
|---|---|---|
| Windows x64 | `imagecaster-server-<version>-windows-amd64.zip` | `imagecaster-server.exe` (setup window, then tray icon), `imagecaster-server-console.exe` (terminal interface) |
| Linux x64 | `imagecaster-server-<version>-linux-amd64.tar.gz` | `imagecaster-server` |
| macOS arm64 (Apple Silicon) | `imagecaster-server-<version>-darwin-arm64.tar.gz` | `imagecaster-server`, `Imagecaster Server.command` |

**Windows.** Start `imagecaster-server.exe`. The setup window shows the graphics card, asks for the data folder (any drive with free space) and installs everything after **Install**. When it says **Ready**, the window can be closed: the server keeps running with an icon in the tray. The tray menu shows the engine state, the address for the app (click copies it), **Dashboard**, the log and **Exit**. Next time just start `imagecaster-server.exe` again.

**Linux.**

```bash
tar xzf imagecaster-server-*-linux-amd64.tar.gz
cd imagecaster-server-*-linux-amd64
./imagecaster-server
```

The first start asks for the data folder (Enter keeps the default `~/imagecaster-server`), installs everything and shows the terminal dashboard. Linux builds need `libssl.so.3`.

**macOS.** Open `Imagecaster Server.command` (the first time: right click → Open). It removes the download quarantine from the server and starts it in Terminal; the rest is the same as on Linux. The macOS build has not been tested on a Mac yet.

Nothing is registered in the system and nothing starts with it: you start the server when you need it.

### Requirements

- NVIDIA graphics card with a recent driver (CUDA 12 build); other cards use the Vulkan build, Macs use Metal. Without a graphics card generation runs on the CPU and is very slow.
- Linux ARM, Windows ARM and Intel Macs have no stable-diffusion.cpp build, so no image generation there.
- Disk: about 8 GB for the engines plus models.
- Internet for the first start.

### What the first start installs

- stable-diffusion.cpp — the build for your graphics card;
- llama.cpp — text and photo-reading models;
- ONNX Runtime and the DWPose models — face and pose points.

Image models and assistant models are **not** downloaded here — the app does it (see [Models](models-en.md)). An interrupted download resumes where it stopped.

### Data folder

Everything the server has lives in one folder:

```
<data>/
  server-config.yaml
  engines/sd-cpp-<OS>/       stable-diffusion.cpp
  engines/llama-cpp-<OS>/    llama.cpp
  engines/onnxruntime/       ONNX Runtime
  models/checkpoints/        image models
  models/text_encoders/      text encoders and tokenizers
  models/vae/
  models/loras/
  models/upscalers/
  models/openpose/           DWPose models
  sdjobs/                    image job results (kept 24 hours)
```

Which folder is used: `--data DIR`, otherwise the folder chosen at the first start, otherwise `~/imagecaster-server`.

### Running as a service

`imagecaster-server --headless` runs without any interface and writes the log to stdout — use it for a systemd service. The same mode is used when stdin is not a terminal. The API answers at once; the install runs in the background.

| Flag | Meaning |
|---|---|
| `--data DIR` | data folder |
| `--config FILE` | config file (default `<data>/server-config.yaml`) |
| `--port N` | API port (default 8080) |
| `--headless` | no interface, log to stdout |
| `--version` | print the version |

`imagecaster-server attach [host[:port]]` opens the dashboard of a server that is already running, on this computer or another one.

### Ports

| Port | Who listens | Used by |
|---|---|---|
| 8080 | the server (all interfaces) | the app — this is the address you connect to |
| 7860 | the server | other programs that talk to stable-diffusion.cpp directly; requests still go through the GPU queue |
| 11434 | the server | other programs that use the text models; requests still go through the GPU queue |
| 7861, 11435 | stable-diffusion.cpp, llama.cpp | loopback only, not reachable from the network |

The server announces itself on the local network over mDNS (`_imagecaster._tcp`), so the app can find it without typing an address.

Full server documentation: [README](https://github.com/Zazza/imagecaster-server#readme), [docs/server-en.md](https://github.com/Zazza/imagecaster-server/blob/main/docs/server-en.md).

## Install the app

Download the archive from [app releases](https://github.com/Zazza/imagecaster/releases). There is no installer: unpack and run.

| System | Archive | Inside | Needs |
|---|---|---|---|
| Windows x64 | `imagecaster-windows-amd64.zip` | `imagecaster.exe`, `imagecaster-mcp.exe` | WebView2 (built into Windows 10/11) |
| Linux x64 | `imagecaster-linux-amd64.tar.gz` | `imagecaster`, `imagecaster-mcp` | GTK 3 and WebKitGTK 4.1 |
| macOS arm64 (Apple Silicon) | `imagecaster-darwin-arm64.tar.gz` | `imagecaster.app` (`imagecaster-mcp` inside `Contents/MacOS`) | — |

- The builds are not signed. Windows SmartScreen may warn on the first start ("More info" → "Run anyway"). If macOS refuses to open the app, remove the quarantine flag: `xattr -dr com.apple.quarantine imagecaster.app`.
- The app keeps its data in a `data` folder next to the program: the database `presets.db`, history images (`sessions/`). Unpack it where you can write. The `DB_PATH` environment variable moves the database (and the folder with it).
- `imagecaster-mcp` is the MCP server for AI agents. It uses the same database, so run it as the same user as the app.

## First start of the app

The app shows up to three windows, in this order.

1. **Server check** — only if the server is not set, does not answer or is outdated.
   - It searches the local network and lists the servers it found; one click connects.
   - Or type the address, for example `http://192.168.1.100:8080`.
   - If the server is too old, the window says what is missing and links to the update guide. Models already on the server are not downloaded again.
   - **Check again** repeats the check. **Close** opens the app in limited mode: a banner on top says there is no server; history, file browsing and non-AI post-processing still work. The banner button opens the check again.
2. **Welcome** — what the app is, and a **Models** block: it checks which files the built-in styles need, shows how many are missing and their total size, and has a "Download all for presets" button. Files go to the server; the window can be closed while they download. After an app update this window shows "What's new" instead. "Don't show at startup" hides it; the same download is on the Catalog page.
3. **AI assistant models** — shown when no prompt model is chosen or the chosen one is not on the server. Two lists: the model for prompts and the model for photos. Each option shows whether it is installed, its download size and which one is recommended. "Download and save", or "Later".

Styles appear on their own: when the app gets the model list from the server, it installs the built-in styles for the models the server has. Styles whose model or LoRA is missing are hidden.

The server address can be changed later in Settings → Connection.

## Build from source

Needs [Go](https://go.dev/dl/) 1.25+, [Node.js](https://nodejs.org/) 20+ and the [Wails CLI](https://wails.io/) v2.

| System | Extra |
|---|---|
| Windows | WebView2 |
| Linux | `libgtk-3-dev`, `libwebkit2gtk-4.1-dev` |
| macOS | Xcode command line tools (`xcode-select --install`) |

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
git clone https://github.com/Zazza/imagecaster.git
cd imagecaster
make setup   # Go modules and npm packages
make dev     # run with hot reload (wails dev)
make build   # build/bin/imagecaster
make mcp     # build/bin/imagecaster-mcp
make test    # go vet, go test, frontend build
```

`make build` runs `wails build` and puts the version from the latest git tag into the program. On Linux it adds `-tags webkit2_41` so the app links WebKitGTK 4.1; plain `wails build` needs the same tag.

Frontend tests: `cd frontend && npm test`.
