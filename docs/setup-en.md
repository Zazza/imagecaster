[English](setup-en.md) | [Русский](setup-ru.md)

# Imagecaster — Setup

Imagecaster works only through [Imagecaster Server](https://github.com/Zazza/imagecaster-server). The server runs on the computer with the graphics card: it installs the image engine (sd.cpp) and the text engine (llama.cpp), starts them, downloads models from the app's catalog and puts every GPU request through one queue so the card doesn't run out of memory.

## Install the server

Download the archive for your system from [Releases](https://github.com/Zazza/imagecaster-server/releases), unpack and run it. Requires an NVIDIA graphics card with a recent driver.

Step-by-step setup for Windows, Linux and macOS, settings and API — in the [server README](https://github.com/Zazza/imagecaster-server/blob/main/README.md) and the [full documentation](https://github.com/Zazza/imagecaster-server/blob/main/docs/server-en.md).

Upgrading from an older server that used Forge and Ollama: the server switches to the new engines on its own and keeps the downloaded models.

## Connect the app

On start Imagecaster checks the server:

- **Server found** — the app opens as usual.
- **Server not found** — a window searches the local network (mDNS), lets you type the address (`http://192.168.1.100:8080`) and links to the install guide.
- **Server is outdated** — the window links to the update guide. Old servers don't report the features the app needs.

The window can be closed: the app then opens in limited mode with a banner at the top. Library, history and settings work; generation, editing and prompt writing wait for the server. The banner button opens the check again.

The address can also be changed in Settings → Connection.

Upgrading from an app version that connected to Forge and Ollama directly: on the first start the app looks for the server on the computer where Forge or Ollama ran (port 8080) and uses it if found.
