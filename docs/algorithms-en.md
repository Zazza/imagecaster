[English](algorithms-en.md) | [Русский](algorithms-ru.md)

# Imagecaster — Algorithms and Data Flows

The app does no image or text inference itself. All work goes to **Imagecaster Server** (the `server_url` setting):

| Server path | Engine | Used for |
|---|---|---|
| `/api/llm/v1/chat/completions` | llama.cpp | text: prompts from ideas, look prompts, recipes |
| `/api/llm/api/chat` (with image) | llama.cpp (vision model) | photo description, tags, object detection |
| `/api/sd/jobs` | stable-diffusion.cpp | every image: txt2img, img2img, upscale |
| `/api/sd/memory` | — | free VRAM/RAM for the memory guard |

Code: `internal/generation` (algorithms), `internal/sd` (job client), `internal/llm` (LLM client), `internal/queue` (app queue), `internal/preset/history.go` (gallery).

## 1. Text-to-image generation

```
Frontend            App (generation)               Server: llama.cpp       Server: sd.cpp
   │ GenerateImage /   │                                  │                      │
   │ EnqueueTxt        │                                  │                      │
   │──────────────────>│ idea given?                      │                      │
   │                   │  GenerateSDPrompt ──────────────>│ chat completion      │
   │                   │  <────────── prompt + negative ──│                      │
   │                   │ build prompt: preset + LLM text  │                      │
   │                   │   + <lora:name:w> tags           │                      │
   │                   │ memory guard (may unload LLM) ──>│                      │
   │                   │ POST /api/sd/jobs  kind=txt2img, model=ModelSpec ──────>│
   │                   │ poll job every 1 s (progress) ─────────────────────────>│
   │                   │ <──────────────────────────────────────── images ───────│
   │                   │ save to session (data/sessions/<id>/)                   │
   │<── result ────────│                                                          │
```

**Prompt from an idea** (`GenerateSDPrompt`):

1. Kids mode filters the user text (blocked words → error) and adds rules to the system prompt.
2. System prompt = `sd_prompt_instruction` (tag style) or, for "prose" models, `sd_prompt_instruction_prose`. Prose models are names containing flux, z-image, klein, qwen-image, chroma, hunyuan, krea and similar (full list: `config.NewGenModelFamilies`). A rule for text inside the image and a response-length line (`llm_max_tokens`) are appended.
3. User message: the preset prompt and negative as read-only style reference, then the user scene and user negative.
4. The answer is parsed (JSON `{prompt, negative_prompt}` or plain text), then cleaned: Cyrillic outside quotes is dropped, junk and repeats removed, examples copied from the instruction removed, length capped (1000 chars for tag models, 2000 for prose), kids output filter applied.

**Image request**: the preset gives sampler and scheduler (sd.cpp names), steps, CFG, seed, clip skip, batch, VAE. The resolution comes from the selected resolution or the preset. `ModelSpec` = checkpoint + VAE + modules (text encoder/VAE files the model needs: built-in lists for Klein, otherwise from the server catalog).

**Hires fix**: a hires profile adds a second pass inside the same txt2img job. If the job fails with hires on, the app waits 3 s, generates without hires, then upscales and re-denoises with img2img itself. If that also fails, the plain image is returned and marked "hires skipped".

## 2. SD job lifecycle

Each image request is one server job (`internal/sd/jobs.go`):

1. `POST /api/sd/jobs` with `kind`, `client`, `client_ref`, `model`, `params`. A network error on submit is retried up to 3 times, 1 s apart; an HTTP error from the server is not retried.
2. The app polls `GET /jobs/<id>` every second and forwards progress to the UI.
3. `completed` → fetch result. Direct (non-queue) jobs are then deleted on the server.
4. `lost`, or `failed` with code `server_restarted` (the server restarted mid-job), or a 404 → submit the same job once more; a second loss is an error.
5. "Interrupt" cancels local waits and calls cancel on every unfinished server job whose `client_ref` starts with this install's id.

The server replays a job when the same client sends the same `client_ref` while the old job is queued, running or completed. It keeps job results for 24 hours.

## 3. Pipelines (compound presets)

A pipeline is an ordered list of steps; each step has a preset, a denoise strength, optional LoRA overrides and an optional resolution (`compound_preset_steps`). The Constructor mode sends the same steps without saving them.

```
idea / scene text ──LLM──> scene prompt (styled for the step 1 preset)
step 1: txt2img   (preset 1 + scene prompt)          ──> image 1
step 2: img2img   (preset 2, init = image 1, denoise) ──> image 2
...
last step: hires profile applied (in-job for a 1-step pipeline,
           otherwise upscale + img2img after the last step)
```

- Each step runs the memory guard for its own model and is its own server job with its own model.
- The session item stores the chain (step, preset, model, denoise, LoRA) in `meta.chain`.

## 4. From image / Remix

The Remix card (Post-processing) and the "from image" flow call `GenerateFromImage`.

**Describe the photo first** (`AnalyzeImage`, local vision model via llama.cpp):

| Mode | Prompt setting | Output |
|---|---|---|
| default (tags) | `analyze_system_prompt` + `analyze_prompt` | comma-separated tags, deduplicated |
| `describe` | `analyze_describe_prompt` | prose description |
| `garment` | `edit_garment_prompt` | clothing description only |

**Generation** (`GenerateFromImage`):

| Option | Effect |
|---|---|
| `mode=txt2img` | the photo is used only for its description; a new image is generated |
| `mode=img2img` | the photo is the init image; `denoising_strength` 0–1 |
| `gen_mode=preset` / `compound` | one preset, or a pipeline whose first step starts from the photo |
| tags given | LLM turns tags into a prompt for the preset (img2img gets "transform into this scene" instructions); no tags → preset prompt only, no LLM |
| `preserve_identity` | Klein models only, img2img preset mode, denoise ≤ 0.9: the photo is also sent as a reference image (`klein_reference` → sd.cpp `ref_images`) |
| `output_mode=quality` | needs a resolution: upscale to target, then a light img2img re-denoise; on failure the step is skipped and flagged |
| mask | img2img preset mode only: the result is blended back into the original only inside the mask (`maskcomp.Blend`); for material/pattern presets the vision model first names the masked object and the prompt is built around it |
| keep color | material presets: restore the original colors (`maskcomp.KeepColor`) |

With a Klein model and no resolution selected, img2img runs at about 1 MP (`kleinWorkSize`); the output size is the photo size, raised to 1024 px on the long side if smaller.

## 5. Post-processing operations

All use `flux-2-klein-4b` with the source photo as reference image unless noted.

| Card | Algorithm |
|---|---|
| **Effect** | queue job `from_image`: img2img with the effect preset, `preserve_identity=true`, chosen strength; optional mask → blended inside the mask only |
| **Look** (text style) | LLM turns the look description into a 1–2 sentence English prompt (2 tries) → Klein img2img with the given strength at ~1 MP → upscale to original size |
| **Edit** | instruction (+ optional style tags/LoRA from a style preset) → Klein img2img, denoise 1.0, 8 steps, CFG 1 → upscale; optional quality output as in §4 |
| **Object remove/replace** | see below |
| **Repair** | mask split into 768 px tiles (+128 px margin, only tiles touching the mask); each tile: Klein img2img with a fixed "remove scratches, dust, cracks" prompt, strength 0.3–0.9 (default 0.8), same seed for all tiles; the fixed core is blended back through a dilated, feathered mask |
| **Correction**, **Format/frame/caption**, **Export** | local Go image processing (`internal/adjust`, `internal/finish`), no server |
| **Recipes** | saved list of effect/look steps + correction values (`post_recipes`); applying runs the steps in order |

**Object remove/replace** (`EditObject`):

1. If the user did not type the object, the app draws a red outline of the mask on the photo, scales it to ≤1024 px and asks the vision model "what object is inside the red outline?" (short noun phrase).
2. Instruction: "Remove X completely. Fill the area with the natural background…" or "Replace X with Y…".
3. Klein reference img2img on the whole photo, 4 steps, denoise 1.0.
4. `maskcomp.Composite` keeps the original outside the change: the image is split into 8 px cells; cells inside the mask take the edit; changed cells (mean difference > 28) connected to the mask also take it; the edge is feathered by 2 cells.

## 6. Memory guard

Before each SD job (`MemoryGuard.EnsureHeadroom`), unless `llm_auto_unload` is `false`:

1. Ask the server which LLM models are loaded; none → go on.
2. Read free VRAM and RAM from `/api/sd/memory`.
3. Needed: 12 GiB VRAM for "heavy" models (same name list as prose models), 8 GiB otherwise, and 10 GiB RAM.
4. Not enough → ask the server to unload all LLM models, wait up to 15 s, then run the job anyway.

## 7. App queue ("Processing" tab)

Table `job_queue`; types `txt2img`, `from_image`, `compound`, `look`, `edit`. One worker processes one job at a time. The MCP server does not use this queue.

```
Enqueue ──> pending ──claim──> running ──ok──────> completed
               ▲                  │
               │ retryable error, │ other error / cancel
               │ retries left     ▼
               └── (backoff) ─── failed / cancelled
                                  │
             retries used up ──> paused ──"Resume"──> pending (retry_count = 0)
```

- **Claim**: the oldest `pending` job whose `next_retry_at` has passed; jobs that already started once go first. The worker wakes on enqueue and every 5 s.
- **Retryable errors**: connection refused/reset, timeout, EOF, "SD is not available", no such host, network unreachable. Backoff `5 s × 2^retry`, max 60 s: in practice 10 and 20 s. On the third failure (`max_retries` = 3) the job becomes `paused`.
- **Resume after restart**: each server call of a queue job gets `client_ref = <install_id>:q<job id>:s<step n>:<hash of request>`. On re-run the same steps produce the same refs, so the server returns the already running or finished job instead of starting it again.
- **LLM answers** of a queue job are saved in `data/queue-llm/<job id>.json` and replayed on re-run, so the prompt stays the same. A failed LLM call is saved only once the next SD step starts.
- **App closed**: the running job goes back to `pending` without using a retry; its server jobs keep running. **App crashed** (job still `running` at start): it counts as a retry — back to `pending` with backoff, or `paused` if retries are used up.
- **Cancel**: a running job is interrupted (server jobs cancelled) and marked `failed` "cancelled by user"; a pending job becomes `cancelled`. "Pause" stops claiming new jobs.
- **Cleanup**: when a job finishes, fails or is cancelled, the app deletes its server jobs (by `client_ref` prefix) and its LLM memo file.

## 8. Gallery filtering

`ListHistory` (`internal/preset/history.go`) works over all `session_items` of all sessions.

1. One SQL query computes per item: `kind` from `source` (generate, from_image, pipeline, edit = edit/look/object/repair, stylist, editor, other), `model_name` and `label` from `meta` JSON (label falls back to the preset name, only for generation kinds), `shape` (landscape/portrait if one side is more than 5 % longer, else square; unknown if no size).
2. The prompt search is applied in SQL: ASCII → `LIKE`; non-ASCII → `instr(ic_lower(prompt), ?)`. `ic_lower` is a Go function registered in SQLite because SQLite's `lower()` only folds ASCII.
3. Other filters run in Go: groups (sessions), models, labels, kinds, shapes, date range.
4. Facet counts: each filter's counts are computed with all other filters applied except itself. Selected values stay in the list even with count 0. Periods ("all" plus the frontend's start dates) are counted the same way, ignoring the date filter.
5. Page: default 120 items, max 500, newest first.

## 9. Sessions

- Every result is saved by `AddToSession`: file in `data/sessions/<session id>/`, 128 px thumbnail in `data/thumbs/<session id>/`.
- A new item becomes the session's active item (`is_active`, one per session); `session_active` holds the current session.
- `meta` (JSON) keeps preset id, model, label, pipeline chain and source item.
