[English](setup-en.md) | [Русский](setup-ru.md)

# Imagecaster — Руководство по настройке сервисов

## Содержание

- [Stable Diffusion WebUI](#stable-diffusion-webui)
- [Ollama](#ollama)
- [LM Studio](#lm-studio)
- [llama.cpp](#llamacpp)
- [Imagecaster Server](#imagecaster-server)

---

## Stable Diffusion WebUI

Движок генерации изображений. Imagecaster подключается через REST API.

### Рекомендуется: Forge Neo

На Forge Neo работают все функции, включая режим правки (Moss-Image) и модели Z-Image. Проще всего поставить его через [Imagecaster Server](#imagecaster-server) — сервер скачивает и настраивает Forge Neo сам.

Вручную: [sd-webui-forge-classic, ветка `neo`](https://github.com/Haoming02/sd-webui-forge-classic/tree/neo) — установка по README репозитория, запуск с флагами `--listen --api`.

A1111 и оригинальный Forge (варианты ниже) подходят только для базовой генерации: модели Z-Image и режим правки на них не работают.

### Вариант 1: A1111

```bash
git clone https://github.com/AUTOMATIC1111/stable-diffusion-webui.git
cd stable-diffusion-webui

# Запуск с сетевым доступом
./webui.sh --listen --api --enable-insecure-extension-access
```

Windows:
```bat
webui.bat --listen --api --enable-insecure-extension-access
```

### Вариант 2: оригинальный Forge

[Stable Diffusion WebUI Forge](https://github.com/lllyasviel/stable-diffusion-webui-forge) — оптимизированный форк A1111, работает значительно быстрее.

```bash
git clone https://github.com/lllyasviel/stable-diffusion-webui-forge.git
cd stable-diffusion-webui-forge

# Запуск с сетевым доступом
./webui.sh --listen --api --enable-insecure-extension-access
```

Windows:
```bat
webui.bat --listen --api --enable-insecure-extension-access
```

> **Известная проблема Forge:** Hires Fix может вызывать ошибку `argument of type 'NoneType' is not iterable`. Imagecaster автоматически повторит генерацию без Hires Fix и покажет предупреждение. Для полноценной работы Hires Fix используйте A1111.

> **Модели Z-Image:** бандл-семейство пресетов `(Z-Image)` использует чекпоинты `z-image-turbo`. Оригинальный Forge эту архитектуру **не поддерживает** — нужен форк с поддержкой Z-Image (например, [sd-webui-forge-classic, ветка `neo`](https://github.com/Haoming02/sd-webui-forge-classic)); это замена «как есть» с тем же REST API. Остальные бандл-семейства работают на любом из бэкендов выше.

> **Модель Moss-Image (режим правки):** для редактирования по инструкции нужен чекпоинт `moss_image_sft_bf16` и Forge Neo (reference-движок). Чекпоинт скачивается из каталога (Настройки → Каталог); текстовый энкодер `qwen35_9b_int8_convrot.safetensors` и VAE `flux2-vae_small.safetensors` положите вручную в `models/text_encoder/` и `models/VAE/` (репозиторий Comfy-Org/MOSS-Image). Приложение подключает модули автоматически при смене модели.

### Флаги

| Флаг | Описание |
|------|----------|
| `--listen` | Разрешить подключения с других устройств |
| `--api` | Включить REST API |
| `--enable-insecure-extension-access` | Разрешить установку расширений |
| `--port 7860` | Порт (по умолчанию: 7860) |
| `--xformers` | Ускорить генерацию (NVIDIA) |
| `--medvram` | Экономить VRAM (4–6 ГБ) |
| `--lowvram` | Минимальное использование VRAM (2–4 ГБ) |

### Проверка

Откройте `http://localhost:7860/sdapi/v1/sd-models` — должен вернуться JSON со списком моделей.

### Модели

Скачайте `.safetensors` в `models/Stable-diffusion/`:

- [Civitai](https://civitai.com/) — основной репозиторий моделей
- [HuggingFace](https://huggingface.co/models?pipeline_tag=text-to-image)

LoRA -> `models/Lora/`, VAE -> `models/VAE/`.

---

## Ollama

Локальный LLM-сервер. Imagecaster использует его для генерации промптов и анализа изображений.

### Установка

**macOS / Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

**macOS (альтернатива):** Скачайте с [ollama.com/download](https://ollama.com/download)

**Linux (вручную):**
```bash
curl -L https://ollama.com/download/ollama-linux-amd64 -o /usr/local/bin/ollama
chmod +x /usr/local/bin/ollama
```

### Запуск

```bash
# Автозапуск (приложение macOS / systemd)
ollama serve

# Запуск модели
ollama run llama3.2-vision
```

### Рекомендуемые модели

```bash
# Для генерации промптов (обязательно)
ollama pull llama3.2

# Для анализа изображений (нужна vision-модель)
ollama pull llama3.2-vision
ollama pull llava:13b
ollama pull minicpm-v
```

### Сетевой доступ

Ollama по умолчанию слушает `localhost`. Чтобы разрешить удалённые подключения:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
```

Или задайте переменную окружения:
```bash
export OLLAMA_HOST=0.0.0.0:11434
```

systemd (`/etc/systemd/system/ollama.service`):
```ini
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
```

### Проверка

```bash
curl http://localhost:11434/api/tags
```

В Imagecaster -> Настройки -> Подключение: выберите **Ollama**, URL `http://localhost:11434`.

---

## LM Studio

GUI-приложение для LLM. Проще в настройке, чем Ollama.

### Установка

Скачайте с [lmstudio.ai](https://lmstudio.ai/) (macOS / Windows / Linux).

### Настройка

1. Откройте LM Studio
2. Скачайте модель (вкладка Search):
   - `Llama 3.2 3B Instruct` — для генерации промптов
   - `Llama 3.2 11B Vision Instruct` — для анализа изображений
3. Перейдите на вкладку **Local Server** (значок стрелки на левой панели)
4. Выберите модель из выпадающего списка
5. Нажмите **Start Server**
6. Порт по умолчанию: `1234`

### Сетевой доступ

В LM Studio: Settings -> Advanced -> **Enable CORS** и укажите хост сервера `0.0.0.0`.

### Проверка

```bash
curl http://localhost:1234/v1/models
```

В Imagecaster -> Настройки -> Подключение: выберите **LM Studio**, URL `http://localhost:1234`.

---

## llama.cpp

Минимальный LLM-сервер на C++. Подходит для слабых машин без Python.

### Установка

**Сборка из исходников:**
```bash
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp

# С поддержкой CUDA
cmake -B build -DGGML_CUDA=ON && cmake --build build --config Release

# Только CPU
cmake -B build && cmake --build build --config Release
```

**macOS (Homebrew):**
```bash
brew install llama.cpp
```

### Запуск сервера

```bash
# CPU
./llama-server -m model.gguf --host 0.0.0.0 --port 8081 -c 4096

# С GPU (CUDA)
./llama-server -m model.gguf --host 0.0.0.0 --port 8081 -ngl 99 -c 4096
```

| Параметр | Описание |
|----------|----------|
| `-m` | Путь к файлу модели .gguf |
| `--host` | Адрес привязки (`0.0.0.0` для сетевого доступа) |
| `--port` | Номер порта |
| `-ngl` | Слоёв на GPU (99 = все) |
| `-c` | Размер контекста |

### Модели

Скачайте файлы `.gguf` с [HuggingFace](https://huggingface.co/models?search=gguf):

- [Llama 3.2 3B](https://huggingface.co/bartowski/Llama-3.2-3B-Instruct-GGUF)
- [Llama 3.2 Vision](https://huggingface.co/bartowski/Llama-3.2-11B-Vision-Instruct-GGUF) — для анализа

### Проверка

```bash
curl http://localhost:8081/v1/models
```

В Imagecaster -> Настройки -> Подключение: выберите **llama.cpp**, URL `http://localhost:8081`.

> **Примечание:** llama.cpp не поддерживает переключение моделей через API. Модель задаётся при запуске сервера.

---

## Imagecaster Server

Отдельная программа для компьютера с видеокартой: сама ставит Forge Neo и Ollama в одну папку, запускает и перезапускает их, скачивает модели из каталога приложения и пропускает все запросы к видеокарте через одну очередь, чтобы хватало памяти. Imagecaster находит сервер в локальной сети сам (mDNS).

Скачайте архив для своей системы со страницы [Releases](https://github.com/Zazza/imagecaster-server/releases), распакуйте и запустите. Первый запуск скачивает Forge Neo, Python, PyTorch и Ollama — около 20 ГБ без моделей. Нужна видеокарта NVIDIA со свежим драйвером.

Установка по шагам для Windows, Linux и macOS, настройки и API — в [README сервера](https://github.com/Zazza/imagecaster-server/blob/main/README-ru.md) и [полной документации](https://github.com/Zazza/imagecaster-server/blob/main/docs/server-ru.md).

---

## Краткая справка

| Сервис | Порт по умолчанию | URL в Imagecaster |
|--------|-------------------|-----------------|
| Stable Diffusion | 7860 | `http://localhost:7860` |
| Ollama | 11434 | `http://localhost:11434` |
| LM Studio | 1234 | `http://localhost:1234` |
| llama.cpp | 8081 | `http://localhost:8081` |
| Imagecaster Server | 8080 | `http://localhost:8080` |
