[English](database-en.md) | [Русский](database-ru.md)

# Imagecaster — База данных

## SQLite

- Файл: `data/presets.db` рядом с исполняемым файлом. `DB_PATH` задаёт другой путь (относительный считается от папки исполняемого файла). `imagecaster-mcp` принимает `-db <путь>` или `IMAGECASTER_MCP_DB` и должен указывать на тот же файл, чтобы видеть данные приложения.
- Драйвер: `modernc.org/sqlite` (чистый Go), сырой SQL через `database/sql`, без ORM.
- При открытии (`preset.Open`): одно соединение (`SetMaxOpenConns(1)`), `busy_timeout = 5000`, `journal_mode = WAL`, `synchronous = NORMAL`, затем миграция схемы.
- SQL-функция `ic_lower(text)` регистрируется для всего драйвера (`internal/preset/history.go`): нижний регистр с Unicode, нужна поиску в галерее, потому что `lower()` в SQLite работает только с ASCII.

## Миграции

Версия схемы хранится в `PRAGMA user_version` (`internal/preset/schema.go`, текущая `schemaVersion` = 33).

| Состояние базы при открытии | Что происходит |
|---|---|
| пустая (нет таблиц) | `latestSchemaDDL` + начальные данные создаются одной транзакцией; версия = 33 |
| есть таблицы, `user_version = 0` (создана старой версией) | один раз выполняется старая цепочка `legacyMigrations` (`internal/preset/db.go`), версия ставится 32, затем `schemaSteps` |
| `user_version` ≥ 33 | ничего не выполняется; открытие ничего не пишет |

Шаги `schemaSteps` идут по порядку, каждый в транзакции `BEGIN IMMEDIATE` и только если его номер больше `user_version`; версия поднимается в той же транзакции.

**Новая миграция:** добавить в `schemaSteps` шаг со следующим номером (34, 35, …), то же изменение внести в `latestSchemaDDL`, поднять `schemaVersion`. Тесты `TestSchema_FreshEqualsFullLegacyChain` и `TestSchema_OldDBUpgradesToLatest` проверяют, что новая и обновлённая базы получают одинаковую схему.

## Таблицы

Типы — как в DDL. Время — текст SQLite `CURRENT_TIMESTAMP` / `datetime('now')` (UTC).

### presets
Стили и эффекты (пресеты генерации).

| Колонка | Тип | Примечание |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| name | TEXT NOT NULL | |
| preset_type | TEXT, по умолчанию `''` | имя типа; типы эффектов — `post`, `post_light`, `post_film`, `post_weather`, `post_background`, … |
| type_id | INTEGER | → `preset_types.id` |
| prompt | TEXT, по умолчанию `''` | позитивный промпт |
| negative_prompt | TEXT, по умолчанию `''` | |
| sampler | TEXT, по умолчанию `'euler_a'` | имя сэмплера sd.cpp |
| schedule_type | TEXT, по умолчанию `''` | планировщик sd.cpp, `''` = по умолчанию модели |
| steps | INTEGER, по умолчанию 20 | |
| cfg_scale | REAL, по умолчанию 7.0 | |
| model_name | TEXT, по умолчанию `''` | файл чекпоинта |
| vae | TEXT, по умолчанию `''` | файл VAE |
| seed | INTEGER | NULL = случайный |
| denoising_strength | REAL | NULL = не задано |
| clip_skip | INTEGER | |
| batch_size, batch_count | INTEGER | |
| tags | TEXT, по умолчанию `''` | |
| loras | TEXT, по умолчанию `''` | список LoRA (JSON), добавляется в промпт как `<lora:имя:вес>` |
| is_bundled | INTEGER, по умолчанию 0 | 1 = поставлен из встроенного набора пресетов |
| created_at, updated_at | DATETIME | |

Колонок ширины/высоты и hires нет: размер берётся из `resolutions`, hires — из `hires_profiles`.

### preset_types

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| description | TEXT, по умолчанию `''` |
| sort_order | INTEGER, по умолчанию 0 |
| created_at | DATETIME |

### compound_presets
Пайплайны (только заголовок; шаги — в `compound_preset_steps`).

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| description | TEXT, по умолчанию `''` |
| created_at, updated_at | DATETIME |

### compound_preset_steps

| Колонка | Тип | Примечание |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| compound_preset_id | INTEGER NOT NULL | → `compound_presets.id`, ON DELETE CASCADE |
| step_order | INTEGER NOT NULL | 1, 2, … |
| preset_id | INTEGER NOT NULL | → `presets.id` |
| denoising_strength | REAL, по умолчанию 0.5 | сила img2img для шагов после первого |
| resolution_id | INTEGER | → `resolutions.id`, NULL = разрешение запуска |
| loras | TEXT, по умолчанию `''` | замена LoRA для шага |

### resolutions
Изначально 14 встроенных размеров (`is_builtin = 1`): квадрат 512–1024, портрет, альбом, широкий.

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| width, height | INTEGER NOT NULL |
| is_builtin | INTEGER, по умолчанию 0 |
| created_at | TEXT |

### hires_profiles
Изначально: Light (1.5×, 0.3), Standard (2.0×, 0.45), Heavy (2.5×, 0.55), Max (3.0×, 0.6), у всех апскейлер `RealESRGAN_x4plus`.

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| upscale | REAL, по умолчанию 2.0 |
| denoising_strength | REAL, по умолчанию 0.45 |
| upscaler | TEXT, по умолчанию `''` |
| is_builtin | INTEGER, по умолчанию 0 |
| created_at | TEXT |

### export_presets
Настройки экспорта. Изначально: Quality Photo (PNG, исходный размер), Web Optimized (JPEG шириной 1920, q85), Social Media (JPEG 1080×1080, q90), Thumbnail (JPEG 256×256, q75).

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| format | TEXT, по умолчанию `'png'` |
| width, height | INTEGER, по умолчанию 0 (0 = не менять) |
| lock_ratio | INTEGER, по умолчанию 1 |
| quality | INTEGER, по умолчанию 90 |
| interpolation | TEXT, по умолчанию `'lanczos'` |
| created_at, updated_at | TEXT |

### post_recipes
Сохранённые рецепты постобработки.

| Колонка | Тип | Примечание |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| name | TEXT NOT NULL | |
| steps | TEXT, по умолчанию `'[]'` | JSON-список: `{"preset_id", "strength"}` (эффект) или `{"prompt", "strength"}` (образ) |
| adjust | TEXT, по умолчанию `'{}'` | JSON со значениями коррекции |
| created_at, updated_at | TEXT | |

### sessions
Группы истории. Изначально одна сессия «Default».

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| name | TEXT NOT NULL |
| created_at, updated_at | DATETIME |

### session_active
Одна строка (`id = 1`) с текущей сессией.

| Колонка | Тип |
|---|---|
| id | INTEGER PK, CHECK (id = 1) |
| session_id | INTEGER → `sessions.id` |

### session_items
Каждая сгенерированная или обработанная картинка. Файлы: `data/sessions/<session_id>/<file_name>`, миниатюры — `data/thumbs/<session_id>/<thumb_name>`.

| Колонка | Тип | Примечание |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| session_id | INTEGER NOT NULL | → `sessions.id`, ON DELETE CASCADE |
| file_name, thumb_name | TEXT, по умолчанию `''` | |
| source | TEXT, по умолчанию `''` | `generate`, `from-image`, `compound`, `compound-from-image`, `edit`, `look`, `object`, `repair`, `stylist`, `editor`, … (галерея объединяет их в типы) |
| prompt, negative_prompt | TEXT, по умолчанию `''` | |
| sampler | TEXT, по умолчанию `''` | |
| steps | INTEGER, по умолчанию 0 | |
| cfg_scale | REAL, по умолчанию 0 | |
| seed | INTEGER | |
| denoising | REAL, по умолчанию 0 | |
| width, height | INTEGER, по умолчанию 0 | |
| is_preview | INTEGER, по умолчанию 0 | |
| preset_id | INTEGER | |
| is_active | INTEGER, по умолчанию 0 | выбранный элемент сессии; один на сессию |
| created_at | DATETIME | |
| meta | TEXT | JSON: `preset_id`, `model`, `label`, `chain` (шаги пайплайна), `source_item` |

### job_queue
Очередь обработки приложения (см. «Алгоритмы», §7).

| Колонка | Тип | Примечание |
|---|---|---|
| id | INTEGER PK AUTOINCREMENT | |
| type | TEXT NOT NULL | `txt2img`, `from_image`, `compound`, `look`, `edit` |
| status | TEXT, по умолчанию `'pending'` | `pending`, `running`, `completed`, `failed`, `cancelled`, `paused` |
| params | TEXT, по умолчанию `'{}'` | JSON запроса |
| progress | REAL, по умолчанию 0 | 0–1 |
| progress_detail | TEXT, по умолчанию `'{}'` | JSON |
| result | TEXT, по умолчанию `''` | JSON результата (id элемента сессии, info, …) |
| error | TEXT, по умолчанию `''` | |
| source | TEXT, по умолчанию `''` | |
| created_at | DATETIME | |
| started_at, completed_at | DATETIME | |
| retry_count | INTEGER, по умолчанию 0 | |
| max_retries | INTEGER, по умолчанию 3 | |
| next_retry_at | DATETIME | не раньше этого времени повтор |

Индексы: `idx_job_queue_status (status)`; частичный `idx_job_queue_next_retry (next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL`.

### saved_descriptions
Сохранённые описания сцен (страница «Генерация»).

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| text | TEXT NOT NULL |
| name | TEXT, по умолчанию `''` |
| negative_prompt | TEXT, по умолчанию `''` |
| type | TEXT, по умолчанию `''` |
| created_at | DATETIME |

### saved_prompts

| Колонка | Тип |
|---|---|
| id | INTEGER PK AUTOINCREMENT |
| text | TEXT NOT NULL |
| created_at | DATETIME |

### settings
Хранилище ключ–значение: `key TEXT PRIMARY KEY`, `value TEXT NOT NULL DEFAULT ''`. Интерфейс может записать только ключи из `config.AllowedSettings`; остальные пишет само приложение.

## Ключи настроек

**Подключение и модели**

| Ключ | По умолчанию | Что делает |
|---|---|---|
| `server_url` | `''` | адрес Imagecaster Server; `http://` добавляется, если не указан. Пусто = не настроен |
| `llm_generate_model` | — | модель llama.cpp для текстовых промптов; пусто или `default` = первая модель из списка сервера |
| `llm_analyze_model` | — | vision-модель llama.cpp для описания фото |
| `llm_max_tokens` | 256 в настройках (512, если не задано) | предел длины ответа LLM |
| `llm_keep_alive` | `5m` | уходит как `keep_alive` с каждым запросом к LLM. Слой llama.cpp на сервере выгружает модель после такого простоя; `0` = выгрузить сразу после ответа. Без него сервер берёт свой `llm.keep_alive` |
| `llm_num_ctx`, `llm_generate_num_ctx`, `llm_analyze_num_ctx` | 4096 | уходит как `options.num_ctx`, но сервер его игнорирует: размер контекста llama.cpp задаёт `llm.context_size` сервера (по умолчанию 8192) |
| `llm_generate_*` / `llm_analyze_*` `temperature`, `top_p`, `num_predict`, `num_thread` | 0.4 / 0.9 / 256 / 0 | хранятся и показываются в настройках, но на сервер не отправляются; температура задана в коде для каждой операции |
| `llm_auto_unload` | `true` | защита памяти: выгружать LLM-модели перед заданием SD, если мало свободной VRAM/RAM |
| `auto_installed_presets` | — | имена встроенных пресетов, уже поставленных `SyncPresets` (удалённые не возвращаются) |
| `install_id` | случайный | 16 hex-символов; префикс `client_ref` у каждого задания на сервере |

**Промпты** (пусто = встроенный по умолчанию)

| Ключ | Для чего |
|---|---|
| `sd_prompt_instruction` | системный промпт «идея → теги» |
| `sd_prompt_instruction_prose` | то же для прозаических моделей (Flux, Klein, Z-Image, …) |
| `analyze_system_prompt`, `analyze_prompt` | фото → теги |
| `analyze_describe_prompt` | фото → описание прозой |
| `edit_garment_prompt` | фото → описание одежды |

**Детский режим**: `kids_mode`, `kids_cat_violence`, `kids_cat_horror`, `kids_cat_weapons`, `kids_cat_substances`, `kids_cat_mature` (категории), `kids_pin_hash`, `kids_pin_salt`, `kids_pin_attempts`, `kids_pin_lockout` (PIN).

**Состояние интерфейса**: `ui_locale`, `theme`, размер и положение окна (`window_*`), последние значения форм (`gen_*`, `batch_*`, `test_*`, `fi_*`, `object_*`, `edit_*`, `effect_*`), `file_browser_path`.
