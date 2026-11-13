[English](database-en.md) | [Русский](database-ru.md)

# Imagecaster — Database

## SQLite

- File: `data/presets.db` next to the executable. `DB_PATH` overrides it (a relative path is resolved from the executable's folder). `imagecaster-mcp` takes `-db <path>` or `IMAGECASTER_MCP_DB` and must point at the same file to see the app's data.
- Driver: `modernc.org/sqlite` (pure Go), raw SQL through `database/sql`, no ORM.
- On open (`preset.Open`): one connection (`SetMaxOpenConns(1)`), `busy_timeout = 5000`, `journal_mode = WAL`, `synchronous = NORMAL`, then schema migration.
- The SQL function `ic_lower(text)` is registered for the whole driver (`internal/preset/history.go`): Unicode lower-case, used by gallery search because SQLite's `lower()` only handles ASCII.

## Migrations

The schema version is stored in `PRAGMA user_version` (`internal/preset/schema.go`, current `schemaVersion` = 33).

| Database state at open | What happens |
|---|---|
| empty (no tables) | `latestSchemaDDL` + seed data are created in one transaction; version = 33 |
| has tables, `user_version = 0` (created by an old version) | the old migration chain `legacyMigrations` (`internal/preset/db.go`) runs once, version is set to 32, then `schemaSteps` |
| `user_version` ≥ 33 | nothing runs; opening does not write |

`schemaSteps` entries run in order, each in a `BEGIN IMMEDIATE` transaction, only if its number is above `user_version`; the version is raised in the same transaction.

**Adding a migration:** add a `schemaSteps` entry with the next number (34, 35, …), make the same change in `latestSchemaDDL`, raise `schemaVersion`. Tests `TestSchema_FreshEqualsFullLegacyChain` and `TestSchema_OldDBUpgradesToLatest` check that a new database and an upgraded one end with the same schema.

## Tables

Types are as written in the DDL. Timestamps are SQLite `CURRENT_TIMESTAMP` / `datetime('now')` text (UTC).

### presets
Styles and effects (generation presets).

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| name | TEXT NOT NULL | |
| preset_type | TEXT, default `''` | type name; effect types are `post`, `post_light`, `post_film`, `post_weather`, `post_background`, … |
| type_id | INTEGER | → `preset_types.id` |
| prompt | TEXT, default `''` | positive prompt |
| negative_prompt | TEXT, default `''` | |
| sampler | TEXT, default `'euler_a'` | sd.cpp sampler name |
| schedule_type | TEXT, default `''` | sd.cpp scheduler, `''` = model default |
| steps | INTEGER, default 20 | |
| cfg_scale | REAL, default 7.0 | |
| model_name | TEXT, default `''` | checkpoint file |
| vae | TEXT, default `''` | VAE file |
| seed | INTEGER | NULL = random |
| denoising_strength | REAL | NULL = not set |
| clip_skip | INTEGER | |
| batch_size, batch_count | INTEGER | |
| tags | TEXT, default `''` | |
| loras | TEXT, default `''` | LoRA list (JSON), added to the prompt as `<lora:name:weight>` |
| is_bundled | INTEGER, default 0 | 1 = installed from the bundled preset set |
| created_at, updated_at | DATETIME | |

There are no width/height or hires columns: size comes from `resolutions`, hires from `hires_profiles`.

### preset_types

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| description | TEXT, default `''` |
| sort_order | INTEGER, default 0 |
| created_at | DATETIME |

### compound_presets
Pipelines (header only; steps are in `compound_preset_steps`).

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| description | TEXT, default `''` |
| created_at, updated_at | DATETIME |

### compound_preset_steps

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| compound_preset_id | INTEGER NOT NULL | → `compound_presets.id`, ON DELETE CASCADE |
| step_order | INTEGER NOT NULL | 1, 2, … |
| preset_id | INTEGER NOT NULL | → `presets.id` |
| denoising_strength | REAL, default 0.5 | img2img strength for steps after the first |
| resolution_id | INTEGER | → `resolutions.id`, NULL = use the run's resolution |
| loras | TEXT, default `''` | LoRA override for this step |

### resolutions
Seeded with 14 built-in sizes (`is_builtin = 1`): square 512–1024, portrait, landscape, widescreen.

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| width, height | INTEGER NOT NULL |
| is_builtin | INTEGER, default 0 |
| created_at | TEXT |

### hires_profiles
Seeded: Light (1.5×, 0.3), Standard (2.0×, 0.45), Heavy (2.5×, 0.55), Max (3.0×, 0.6), all with upscaler `RealESRGAN_x4plus`.

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| upscale | REAL, default 2.0 |
| denoising_strength | REAL, default 0.45 |
| upscaler | TEXT, default `''` |
| is_builtin | INTEGER, default 0 |
| created_at | TEXT |

### export_presets
Export settings. Seeded: Quality Photo (PNG, original size), Web Optimized (JPEG 1920 wide, q85), Social Media (JPEG 1080×1080, q90), Thumbnail (JPEG 256×256, q75).

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| format | TEXT, default `'png'` |
| width, height | INTEGER, default 0 (0 = keep) |
| lock_ratio | INTEGER, default 1 |
| quality | INTEGER, default 90 |
| interpolation | TEXT, default `'lanczos'` |
| created_at, updated_at | TEXT |

### post_recipes
Saved post-processing recipes.

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| name | TEXT NOT NULL | |
| steps | TEXT, default `'[]'` | JSON list: `{"preset_id", "strength"}` (effect) or `{"prompt", "strength"}` (look) |
| adjust | TEXT, default `'{}'` | JSON correction values |
| created_at, updated_at | TEXT | |

### sessions
History groups. Seeded with one session "Default".

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| created_at, updated_at | DATETIME |

### session_active
One row (`id = 1`) pointing at the current session.

| Column | Type |
|---|---|
| id | INTEGER PK, CHECK (id = 1) |
| session_id | INTEGER → `sessions.id` |

### session_items
Every generated or edited image. Files: `data/sessions/<session_id>/<file_name>`, thumbnails `data/thumbs/<session_id>/<thumb_name>`.

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| session_id | INTEGER NOT NULL | → `sessions.id`, ON DELETE CASCADE |
| file_name, thumb_name | TEXT, default `''` | |
| source | TEXT, default `''` | `generate`, `from-image`, `compound`, `compound-from-image`, `edit`, `look`, `object`, `repair`, `stylist`, `editor`, … (gallery groups these into kinds) |
| prompt, negative_prompt | TEXT, default `''` | |
| sampler | TEXT, default `''` | |
| steps | INTEGER, default 0 | |
| cfg_scale | REAL, default 0 | |
| seed | INTEGER | |
| denoising | REAL, default 0 | |
| width, height | INTEGER, default 0 | |
| is_preview | INTEGER, default 0 | |
| preset_id | INTEGER | |
| is_active | INTEGER, default 0 | the session's selected item; one per session |
| created_at | DATETIME | |
| meta | TEXT | JSON: `preset_id`, `model`, `label`, `chain` (pipeline steps), `source_item` |

### job_queue
App processing queue (see Algorithms, §7).

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| type | TEXT NOT NULL | `txt2img`, `from_image`, `compound`, `look`, `edit` |
| status | TEXT, default `'pending'` | `pending`, `running`, `completed`, `failed`, `cancelled`, `paused` |
| params | TEXT, default `'{}'` | JSON request |
| progress | REAL, default 0 | 0–1 |
| progress_detail | TEXT, default `'{}'` | JSON |
| result | TEXT, default `''` | JSON result (session item id, info, …) |
| error | TEXT, default `''` | |
| source | TEXT, default `''` | |
| created_at | DATETIME | |
| started_at, completed_at | DATETIME | |
| retry_count | INTEGER, default 0 | |
| max_retries | INTEGER, default 3 | |
| next_retry_at | DATETIME | earliest time to retry |

Indexes: `idx_job_queue_status (status)`; partial `idx_job_queue_next_retry (next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL`.

### saved_descriptions
Saved scene descriptions (Generate page).

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| text | TEXT NOT NULL |
| name | TEXT, default `''` |
| negative_prompt | TEXT, default `''` |
| type | TEXT, default `''` |
| created_at | DATETIME |

### saved_prompts

| Column | Type |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| text | TEXT NOT NULL |
| created_at | DATETIME |

### settings
Key–value store: `key TEXT PRIMARY KEY`, `value TEXT NOT NULL DEFAULT ''`. The UI can write only keys listed in `config.AllowedSettings`; other keys are written by the app itself.

## Settings keys

**Connection and models**

| Key | Default | What it does |
|---|---|---|
| `server_url` | `''` | Imagecaster Server address; `http://` is added if missing. Empty = not configured |
| `llm_generate_model` | — | llama.cpp model for text prompts; empty or `default` = first model the server lists |
| `llm_analyze_model` | — | llama.cpp vision model for photo description |
| `llm_max_tokens` | 256 in Settings (512 if unset) | answer length limit for LLM calls |
| `llm_keep_alive` | `5m` | sent as `keep_alive` with every LLM request. The server's llama.cpp layer unloads the model after this idle time; `0` = unload right after the answer. Without it the server uses its own `llm.keep_alive` |
| `llm_num_ctx`, `llm_generate_num_ctx`, `llm_analyze_num_ctx` | 4096 | sent as `options.num_ctx`, but the server ignores it: llama.cpp context size is the server's `llm.context_size` (default 8192) |
| `llm_generate_*` / `llm_analyze_*` `temperature`, `top_p`, `num_predict`, `num_thread` | 0.4 / 0.9 / 256 / 0 | stored and shown in Settings, but not sent to the server; temperatures are fixed in code per operation |
| `llm_auto_unload` | `true` | memory guard: unload LLM models before an SD job when free VRAM/RAM is low |
| `auto_installed_presets` | — | names of bundled presets already installed by `SyncPresets` (deleted ones are not reinstalled) |
| `install_id` | random | 16 hex chars; prefix of every server job `client_ref` |

**Prompts** (empty = built-in default)

| Key | Used for |
|---|---|
| `sd_prompt_instruction` | system prompt for idea → tags |
| `sd_prompt_instruction_prose` | same for prose models (Flux, Klein, Z-Image, …) |
| `analyze_system_prompt`, `analyze_prompt` | photo → tags |
| `analyze_describe_prompt` | photo → prose description |
| `edit_garment_prompt` | photo → clothing description |

**Kids mode**: `kids_mode`, `kids_cat_violence`, `kids_cat_horror`, `kids_cat_weapons`, `kids_cat_substances`, `kids_cat_mature` (category switches), `kids_pin_hash`, `kids_pin_salt`, `kids_pin_attempts`, `kids_pin_lockout` (PIN).

**UI state**: `ui_locale`, `theme`, window size/position (`window_*`), last form values (`gen_*`, `batch_*`, `test_*`, `fi_*`, `object_*`, `edit_*`, `effect_*`), `file_browser_path`.
