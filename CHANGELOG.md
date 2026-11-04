# Changelog

All notable changes to Imagecaster are documented here.

## [1.1.1] — 2026-11-04

### Changed
- **Live preview over the photo in Post-Process**: while Edit, Remix, Object, an effect, a recipe or "Describe the look" is generating, the SD preview covers the main image (as on Generate) with percent, step and time left at the bottom; a spinner is shown until the first preview arrives. The small previews in the right panel are gone; the progress line and Interrupt button stay.

### Fixed
- **Klein effects barely applied on large photos.** Klein is tuned for about 1 megapixel; at 2048 px an effect such as Lace or Satin was almost invisible. Effects and "Describe the look" now run at ~1 MP and the result is upscaled back to the previous size with the neural upscaler (falls back to plain resize if it is unavailable).
- Small photos no longer get their proportions slightly skewed before a Klein effect (double rounding).
- Time left was never shown during generation anywhere in the app.

## [1.1.0] — 2026-11-04

### Added
- **Post-Process editor**: one workspace for a photo — before/after split preview with zoom and pan, layer undo/redo (15 steps, Ctrl+Z / Ctrl+Shift+Z / Ctrl+Y), and an accordion of sections on the right: Effect, Edit, Remix, Object, Correction, Recipes. Export goes through the Save As dialog. The former Remix and Export pages live here now; dropping a photo on Generate, the File Browser and the image viewer open it in Post-Process.
- **Effects**: 51 Klein effects grouped by family — Basic, Materials, Skin, Light, Film and the new **Transform** group (Realism, Photo Restore, Colorize, 3D Animation, Cartoon, Anime, Watercolor, Pencil Sketch, Oil Painting, Pixel Art). All of them ship with the app and are available right after install; each effect sets its own default strength. The face is preserved.
- **Correction**: Photoshop-style adjustments (levels, brightness/contrast, saturation, temperature, RGB balance, sharpen, vignette) with a live preview, plus Auto levels and Auto WB. Adjustments stay parameters and survive re-applying effects.
- **Auto-style and recipes**: Auto-style describes the photo with the vision model and compiles an effect + correction recipe; "Describe the look" turns your own words into a recipe or applies them directly through Klein. Recipes are saved and re-applied in one click.
- **Folder conveyor**: select photos in the File Browser and run the chosen effect + correction over all of them with auto-export to a folder.
- **Object section**: remove or replace an object by painting a mask; the result is composited back into the full-size photo.
- **Virtual try-on in Edit**: load a garment photo next to your photo and the model puts it on; no instruction needed.
- **Ideas batch on Generate**: select saved ideas and generate N images per idea through the queue with the style chosen on the page; progress, stop, failed ideas skipped.
- **History shows the prompt** and generation parameters of each image.

### Changed
- Sidebar regrouped into Create / Refine / View / Library / Setup; UI texts localized, "preset" is called "style" in the Russian UI, clearer save buttons.
- Image size limit raised to **128 MB** everywhere (one limit for the whole app). A 3 MB photo grows to ~27 MB after lossless steps and used to be rejected by the next step.
- Klein img2img sizes are aligned to 16 px; small photos are upscaled to 1024 px on the long side before a Klein effect, so restoring small photos no longer invents a different face.
- Bundled styles and effects are added to existing installs on update; styles you deleted are not brought back and your own styles with the same name are kept.

### Removed
- Compare page.

### Fixed
- Every app start deleted and re-created all bundled styles and deleted empty categories (a migration ran on each start); it now runs once.
- History items lost their prompt, sampler, steps and seed.
- The history folder switcher sometimes showed a different folder than the one whose images were listed; it is now styled like the other dropdowns.
- Klein effects failed on photos whose size was not a multiple of 16.
- Post-Process: decompression-bomb images are rejected before decoding; export never overwrites an existing file.
- Modals no longer close on an overlay click; an empty folder no longer breaks the File Browser.

## [1.0.1] — 2026-11-02

### Added
- **Bundled moss preset family for fresh installs**: 14 photo-realistic presets (Portrait, Fashion, Cinematic, Architecture, Food, Landscape, Product, Wildlife, Street, Golden Hour, Night City, Studio Portrait, B&W Film, Photo Realism) with clean prose prompts and stock sampler settings (Euler/Simple, 50 steps, CFG 3, no loras) — a fresh install now has moss styles in Generate and a non-empty style selector in Edit right after downloading the checkpoint from the catalog. Existing databases are untouched (seeding only applies when no bundled rows exist). The project is also listed in the Wails and music-production awesome lists (PRs pending merge), and GitHub topics were set for both Imagecaster and Partscaster.

## [1.0.0] — 2026-10-31

### Project
- **Renamed from SD Studio to Imagecaster** — the name now says what the tool does: it casts images from your words. Repository is [Zazza/imagecaster](https://github.com/Zazza/imagecaster) (GitHub redirects the old `sd-ai` URLs), the Go module path, binary (`imagecaster`) and all docs follow; the [sd-ai-server](https://github.com/Zazza/sd-ai-server) companion keeps its name.

### Added
- **Edit mode (instruction-based image editing)**: new `EditImage` binding runs a true reference edit on the fixed `moss_image_sft_bf16` model. The Forge Neo `moss_do_reference` option (`Moss-Image-Edit`) makes the backend inject the source image's latent into attention (reference KV at timestep zero), so the model changes what the instruction asks for and keeps the person, face, pose and scene — verified live on a 1360×2048 photo (pants→skirt applied, face untouched), and Russian instructions work natively (Qwen text encoder). The reference option is enabled right before the request and reset after it (a failed reset flags the result with `edit_reference_reset_failed`; a startup goroutine also resets it). The prompt is built as `<lora:Moss_Edit_LoRA:1.0>[style LoRAs][user LoRAs][style tags] Change only {instruction}. Keep the person, face, pose, background and composition unchanged, and keep the original lighting and color palette so the new clothing matches the photo` — the palette clause is a live-verified upgrade (a same-seed A/B moved the new garment's color/lighting match from 5/10 ("pasted on", flat light) to 8/10 (folds, window highlight, scene-consistent light)). with fixed sampler settings (Euler/Simple, 30 steps, CFG 4.0, denoise 1.0 — the reference engine reconstructs from the source instead of redrawing freely), dimensions aligned to /16 capped at 2048, kids-mode filtering and angle-tag/`<lora:`-prefix stripping on the instruction. The result lands in the session history with an "Edit: …" label; the effective prompt is shown under the result; the instruction persists via `edit_instruction`. New "Edit" page: drop/paste/last-generated source, instruction, optional seed, style selector, LoRA panel, quality checkbox, live SD progress with interrupt, download / regenerate / "edit the result" actions.
- **Edit mode joins the last-image flow**: selecting a history item works as an Edit source (auto-load on mount and on `session:selected`, like Remix), and the fullscreen viewer gets an **Edit** action next to Remix; footer viewer actions share one activate-and-navigate helper (also fixes export not highlighting the active grid item).
- **Style presets in Edit mode**: the Edit page gets a style selector listing the library's moss presets, with **"No style (model only)"** as the default. By default a selected style contributes **only its LoRAs** (shown as badges, merged with extra LoRAs from the panel, total capped at 8) — style prompt tags would restyle the whole photo, so they are opt-in via the "Apply style tags" checkbox (persisted). Distillation LoRAs (DMD) are skipped from the style contribution: a live same-seed A/B showed they degrade edit quality (4/10 vs 6/10 — waxy skin, erased fabric texture) because they are trained for 8-step CFG 1 sampling while the edit contract runs 30 steps CFG 4. The session meta records the preset id.
- **Edit mode LoRA stack**: up to 8 user LoRAs appended after the built-in edit tag; weights clamped 0–2 (NaN/Inf → 0.6), names containing `<`, `>`, `:` or control characters rejected, collapsible LoRA panel with lazy server list + datalist, persisted as `edit_loras`.
- **Edit mode quality output**: `output_mode: "quality"` (requires `resolution_id`) runs the quality chain after the edit — the reference option is reset first so the redenoise runs in normal moss mode, then the image is upscaled into the resolution box (aligned /16, capped 2048) and redenoised at 0.35; skip flags surface as warnings and the session meta records the base → upscale → redenoise chain.

### Changed
- **Forge additional modules are now managed by the app (local Forge patch removed)**: switching the SD model sends a single atomic `POST /sdapi/v1/options` carrying both `sd_model_checkpoint` and `forge_additional_modules` (new `sd.Client.SetModelWithModules`), via a single `Service.setModel` helper used by every model switch. Models matching "moss" get the fixed text-encoder + VAE module pair; everything else resets the list to empty — the hand-patched `modules_forge/main_entry.py` on the Forge Neo backend is no longer needed and has been reverted to stock (verified live: z-image → flux → moss → z-image swaps and the moss suite pass without the patch). A failed model/module switch now fails the generation instead of only logging a warning.

### Fixed
- **Neural upscale in quality mode silently degraded to CatmullRom**: `sd.Client.UpscaleImage` posted to `/sdapi/v1/extra-single-img`, an alias that 404s on the Forge Neo backend. The endpoint is now `/sdapi/v1/extra-single-image`, and the quality upscaler is resolved from the backend's upscaler list (prefers `R-ESRGAN 4x+`, falls back to `ESRGAN` on Forge Neo, whose list is None/Lanczos/Nearest/ESRGAN; resize fallback preserved). `UpscaleImage` also gained a status check and a response size limit.

## [0.9.8] — 2026-10-12

### Fixed
- **Memory guard now checks free VRAM for all models, not just the heavy white-list**: the pre-generation ollama unload only triggered for models from `heavy_models` (flux, z-image, …) before even looking at actual VRAM, so an SDXL-class checkpoint (~7 GB) sharing a 16 GB card with a warm 14B LLM was never relieved (incident 2026-10-12, riMix + ollama); free VRAM/RAM is now checked for every model — 8 GiB VRAM headroom for regular models, 12 GiB for the heavy list.

### Added
- **Image viewer: collapsible info panel + zoom**: the bottom metadata/generation-chain panel no longer covers the image — it is collapsed by default and toggled by an "ℹ" button (Remix/Export/Delete stay available without expanding, in a compact row). New zoom controls (`[−] [Fit|NN%] [+]`): fit-to-window by default (as before), 30/50/100% presets via the indicator, ±10% steps (10–500%), double-click toggles fit/100%, Ctrl+wheel zooms; when the image overflows the window it can be dragged (grab cursor, edges stay clamped to the window, bar-aware). Switching images resets to fit; zoom clamps and fit-percentage react to window resize.
- **Generation metadata in the session history**: every image in the footer history now remembers what created it — the model, LoRAs, the preset or pipeline name (Custom-mode runs show as "Custom"), and for multi-step pipelines the full per-step chain (preset, model, denoise, LoRAs; quality mode shows its base → upscale → detail chain with skipped steps omitted). Stored as a single `session_items.meta` JSON column (migration V28; old items simply have none). The model is taken from the generation context with the SD response's `sd_model_name` as fallback; oversized metadata degrades (drops LoRAs, then the chain) instead of failing. UI: thumbnail tooltip in the footer (label/model/seed/step count) plus a step-count badge, and a "Generation chain" panel in the image viewer; entries without metadata degrade to the existing fields. No new bindings — the metadata travels through the existing `GetSessionItems`.
- **Custom mode (pipeline constructor) on the Generate page**: a third generation mode (Style | Pipeline | Custom) where a multi-step pipeline is assembled inline instead of being pre-built on the Pipelines page — each step picks a preset, a denoising strength (0.05–1, steps 2+; 0 is rejected rather than silently coerced to the 0.5 default) and its own LoRA set (untouched = inherit the step preset's LoRAs, editable with the same LoRA UI as the overrides panel, "disable all" supported), up to 8 steps. The Seed field is available in Custom mode and is applied to every step, so the intended workflow is: fix the seed, then iterate on the presets/denoise of the second and later steps until the result is right. Generation goes through the existing compound engine (queue, progress, hires on the last step, interrupt all work), including batch (`seed + i`, clamped to MAX_SAFE_INTEGER on both Generate and From Image). The assembled configuration can be saved as a regular pipeline ("Save as pipeline" with name/description) — per-step LoRAs are persisted via a new `compound_preset_steps.loras` column (empty = inherit, `[]` = off; the Pipelines editor and duplicate preserve them, and they apply in all compound entry points: Generate, From Image, Test), and are included in export/import (invalid/oversized LoRA payloads are dropped on import). The editor state persists between launches (`gen_custom_steps`); untouched steps re-sync their LoRA rows from the current preset on restore (library changes don't leave stale rows). `"custom"` never leaks into From Image via shared state (sanitized to `"preset"`). Removed the now-dead "Using multi-step workflows?" toggle link.

## [0.9.7] — 2026-10-09

### Added
- **Quality mode in From Image (Remix)**: a "Quality mode" checkbox (visible for preset + img2img with a resolution selected) runs a 3-step pipeline instead of stretching the source image to the target size: img2img at the source's native size (downscale-only fit into the chosen resolution box, never upscaled) → ESRGAN neural upscale (R-ESRGAN 4x+, Catmull-Rom fallback) into the box preserving the source aspect ratio → a detail img2img pass at denoise 0.35. If the upscale or the detail pass fails, the last good image is returned with a visible notice ("Upscale skipped…"/"Detail pass skipped…"). Validation rejects quality mode outside preset+img2img or without a resolution (enqueue + entry + MCP). The mode works in batch too (3 SD operations per image), persists per page (`fi_quality_mode`), and is exposed to MCP via `from_image` props `quality`/`resolution_id`.

### Fixed
- **Hires profile is now applied in From Image Pipeline mode**: the compound (pipeline) path on From Image ignored the selected hires profile — the last step's image is now upscaled + re-denoised via the profile (same manual hires mechanism as the Generate page pipelines), including single-step pipelines; on hires failure (or when no profile is selected) the last good image is returned with its metadata (session item info / job result info) preserved.
- **Fields no longer vanish when the gen-mode toggle is on "Pipeline"**: on From Image, the Seed field and the "Quality mode" checkbox used to disappear entirely in Pipeline mode (looking like the feature doesn't exist — the choice is also persisted between launches). They now stay visible but dimmed and disabled with an "Available in Style mode only" hint; the 3-operations hint only shows when the checkbox is actually usable. Regression-tested (6 vitest cases).
- **Huge results are no longer silently dropped from the session**: `AddToSession` re-encodes images over the 50 MB base64 cap to JPEG (Q95) instead of quietly discarding them — reachable via quality mode at 4K resolutions; a failed re-encode now logs a warning before dropping.

## [0.9.6] — 2026-10-08

### Added
- **Window icon**: the title-bar/taskbar window icon now uses the app logo (a dedicated 256×256 PNG embedded via `go:embed` and wired into the Wails Linux options — the 1024px packaging icon stays for builds/installers).
- **Local builds stamp their version**: `make build` passes `-ldflags "-X main.version=$(git describe --tags --abbrev=0)"`, so a locally built binary shows the real version (e.g. v0.9.5) instead of "vdev"; CI releases already stamped it.
- **Resolution picker in From Image (Remix)**: the params block (next to Denoising/Seed) now has a `ResolutionSelector` with a leading "Match original" option — `null` keeps the current behavior (backend stores the original size), picking a resolution sends `resolution_id` in `EnqueueFromImage` params (backend fits the original into the chosen resolution preserving aspect ratio, upscaling included). The choice is page-local and is not persisted.

### Changed
- **Safer destructive buttons**:
  - Queue "Cancel All" now requires a second click ("Cancel?" confirm state, resets after 3 s), same as "Clear Done".
  - Queue footer buttons got a stable `min-width` (wider for the morphing clear button) plus `white-space: nowrap`, so the "…→ confirm?" text morph no longer changes button size or shifts neighbors.
  - Session "Delete History" in the footer shows a native `window.confirm()` dialog ("Delete the session and all its files? This action cannot be undone.") after the inline two-click confirm, before files are removed; the button width is also fixed.
  - New i18n keys in both RU and EN: `fi.resolution_original`, `queue.confirm_cancel`, `footer.confirm_delete_dialog`.

## [0.9.5] — 2026-10-07

### Fixed
- **Forge memory-manager crashes no longer kill generations**: Forge (Neo) nulls the weakref of a torn-down lazy model object while it stays in the loaded list, so the next garbage-collection pass crashes with a 500 — `TypeError: 'NoneType' object is not callable` or `AttributeError: 'NoneType' object has no attribute 'model_size'/'model'` — and every blind retry hits the same nail (typically on img2img steps of pipelines that switch checkpoints). The SD client now detects the whole `NoneType` family in SD error responses, recovers the server state (unload checkpoint → re-select the current checkpoint, forcing a clean reload) and replays the request once — covering txt2img, img2img and every pipeline step. The local Forge install is also patched at the root: a `_safe_real_model` helper on every direct call site, `is_dead` treats finalized objects as dead, and the size helpers return 0 for a missing model, so the broken object can never crash the GC again (patches are reinstall-on-upgrade, tracked in the project memory).

## [0.9.3] — 2026-10-06

### Changed
- **Searchable style picker in the pipeline editor**: the per-step preset `<select>` on the Pipelines page (300+ entries in one flat list) is replaced by a reusable `SearchableSelect` combobox — type to filter by preset name or style type (hint on the right), full keyboard navigation (arrows + Enter, Esc cancels and restores the chosen label, ✕ clears the step back to "no style"). Pure Vue, no dependencies; other selects are untouched.
