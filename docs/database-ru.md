[English](database-en.md) | [Русский](database-ru.md)

# Imagecaster — База данных

## SQLite

Путь: `data/presets.db` (настраивается через `DB_PATH`).

Версия схемы хранится в `PRAGMA user_version` (текущая — `schemaVersion` = 33, `internal/preset/schema.go`). `preset.Open()`:

- пустая база (нет таблиц) — итоговая схема и начальные данные создаются сразу одной транзакцией (`createLatestSchema`), версия = 33;
- база с таблицами и `user_version = 0` (всё, что создано до версии 1.6) — один раз выполняется старая цепочка `legacyMigrations` (`migrate` … `migrateV32`, порядок прежний, метки вроде `migration_v20_done` работают), затем версия = 32 и шаги `schemaSteps`;
- `user_version` ≥ 33 — ничего не выполняется, запись в базу при запуске не идёт.

Шаги `schemaSteps` (с 33) идут в транзакции `BEGIN IMMEDIATE`. v33 пересобирает таблицу `presets` ради значения по умолчанию `sampler` = `euler_a` (было имя Forge `Euler a`), данные, индексы, триггеры и `sqlite_sequence` сохраняются.

Новая миграция — шаг в `schemaSteps` с номером 34, 35, …: выполняется один раз, если номер больше `user_version`, после чего версия поднимается. Изменение схемы правится и в шаге, и в `latestSchemaDDL`; тест `TestSchema_FreshEqualsFullLegacyChain`/`TestSchema_OldDBUpgradesToLatest` сверяет, что новая и обновлённая база совпадают.

v32 переводит сохранённые имена Forge в имена sd.cpp: `presets.sampler`/`schedule_type` (составное имя вроде «DPM++ 2M Karras» разделяется), `hires_profiles.upscaler` («R-ESRGAN 4x+» → «RealESRGAN_x4plus», «Lanczos» → «lanczos», «Latent» → «latent», «None» → ""), настройки `test_sampler`/`test_schedule_type`. Неизвестное имя Forge становится "" (по умолчанию) и пишется в лог. Импорт пресетов из файла переводит имена так же.

## Схема

### presets
Основная таблица пресетов генерации.

| Поле | Тип | Описание |
|------|-----|----------|
| id | INTEGER PK | Auto-increment |
| name | TEXT | Название пресета |
| type_id | INTEGER FK | Ссылка на preset_types.id |
| preset_type | TEXT | Название типа (денормрализовано) |
| prompt | TEXT | Positive prompt |
| negative_prompt | TEXT | Negative prompt |
| sampler | TEXT | Сэмплер sd.cpp ("euler_a", "dpm++2m", …) |
| schedule_type | TEXT | Планировщик sd.cpp: "", "karras", "simple", "discrete", … ("" — по умолчанию) |
| steps | INTEGER | Количество шагов |
| cfg_scale | REAL | CFG Scale |
| width | INTEGER | Ширина |
| height | INTEGER | Высота |
| model_name | TEXT | SD model filename |
| vae | TEXT | VAE filename |
| seed | INTEGER | Seed (NULL = random) |
| clip_skip | INTEGER | CLIP Skip (NULL = default) |
| loras | TEXT | JSON: `[{"name":"lora1","weight":0.8}]` |
| hires_fix | BOOLEAN | Enable Hires fix |
| hires_upscaler | TEXT | Hires upscaler name |
| hires_scale | REAL | Hires upscale factor |
| hires_denoising | REAL | Hires denoising strength |
| hires_resize_x | INTEGER | Hires target width |
| hires_resize_y | INTEGER | Hires target height |
| hires_steps | INTEGER | Hires steps |
| batch_size | INTEGER | Batch size |
| batch_count | INTEGER | Batch count |
| sort_order | INTEGER | Порядок сортировки |
| created_at | TEXT | ISO 8601 |
| updated_at | TEXT | ISO 8601 |

### preset_types
Типы пресетов (Anime, Realistic, etc.)

| Поле | Тип | Описание |
|------|-----|----------|
| id | INTEGER PK | |
| name | TEXT UNIQUE | Название типа |
| description | TEXT | Описание |
| sort_order | INTEGER | Порядок |
| created_at | TEXT | |

### compound_presets
Compound пресеты (pipeline из нескольких шагов).

| Поле | Тип | Описание |
|------|-----|----------|
| id | INTEGER PK | |
| name | TEXT | Название |
| description | TEXT | Описание |
| steps | TEXT | JSON: `[{step_type, preset_id, ...}]` |
| created_at | TEXT | |
| updated_at | TEXT | |

**compound_preset_steps (JSON):**
```json
[
  {"step_type": "txt2img", "preset_id": 1, "order": 0},
  {"step_type": "upscale", "preset_id": 2, "order": 1}
]
```

### settings
Key-value хранилище настроек.

| Поле | Тип | Описание |
|------|-----|----------|
| key | TEXT PK | Ключ настройки |
| value | TEXT | Значение |

**Ключи:**

| Ключ | Описание | Default |
|------|----------|---------|
| `server_url` | Адрес Imagecaster Server | — |
| `llm_generate_model` | Модель для промптов | из env |
| `llm_analyze_model` | Модель для vision | из env |
| `llm_max_tokens` | Max tokens | `256` |
| `llm_keep_alive` | Ollama keep_alive | `5m` |
| `llm_num_ctx` | Ollama context | `4096` |
| `sd_prompt_instruction` | System prompt для SD | DefaultSDPromptInstruction |
| `kids_mode` | Kids mode on/off | `false` |
| `kids_pin_hash` | SHA256 хэш PIN | — |
| `kids_pin_attempts` | Попытки ввода PIN | `0` |
| `kids_pin_lockout` | Время блокировки | — |
| `kids_categories` | JSON включённых категорий | — |
| `fi_mode` | From Image: режим | `img2img` |
| `fi_preset_id` | From Image: preset | — |
| `fi_gen_mode` | From Image: gen mode | `preset` |
| `fi_denoising` | From Image: denoising | `0.5` |
| `fi_extra_negative` | From Image: negative | — |
| `fi_analyze_mode` | From Image: quick/deep | `quick` |
| `fi_mask_padding` | From Image: mask dilation (px) | `8` |
| `fi_mask_feather` | From Image: mask feathering (px) | `8` |
| `gen_resolution_id` | Generate: выбранный профиль разрешения | первый из списка |
| `gen_hires_profile_id` | Generate: выбранный профиль hires | — (отключен) |
| `browser_folder` | File browser: папка | — |

### saved_descriptions
Сохранённые пользовательские описания.

| Поле | Тип |
|------|-----|
| id | INTEGER PK |
| text | TEXT |
| name | TEXT |
| negative_prompt | TEXT |
| type | TEXT |
| created_at | TEXT |

### saved_prompts
Сохранённые промпты.

| Поле | Тип |
|------|-----|
| id | INTEGER PK |
| text | TEXT |
| created_at | TEXT |

### sessions
Сессии генерации.

| Поле | Тип |
|------|-----|
| id | INTEGER PK |
| name | TEXT |
| created_at | TEXT |
| updated_at | TEXT |

### session_items
Элементы сессии (сгенерированные изображения).

| Поле | Тип | Описание |
|------|-----|----------|
| id | INTEGER PK | |
| session_id | INTEGER FK | Ссылка на sessions.id |
| file_name | TEXT | Имя файла PNG |
| thumb_name | TEXT | Имя файла thumbnail |
| source | TEXT | "preset" / "compound" / "remove" / "test" |
| prompt | TEXT | Использованный промпт |
| negative_prompt | TEXT | Использованный negative |
| model | TEXT | SD модель |
| sampler | TEXT | Sampler |
| steps | INTEGER | Шаги |
| cfg_scale | REAL | CFG |
| seed | INTEGER | Seed |
| is_active | BOOLEAN | Активный элемент сессии (только один на session_id) |
| preset_id | INTEGER FK | Ссылка на presets.id |
| created_at | TEXT | |

Файлы хранятся в:
- Изображения: `data/sessions/{session_id}/{file_name}`
- Thumbnails: `data/sessions/{session_id}/thumb/{thumb_name}`

### export_presets
Пресеты экспорта изображений.

| Поле | Тип | Описание |
|------|-----|----------|
| id | INTEGER PK | |
| name | TEXT | |
| format | TEXT | "png", "jpg", "webp" |
| width | INTEGER | Target width |
| height | INTEGER | Target height |
| lock_ratio | BOOLEAN | Сохранять пропорции |
| quality | INTEGER | Качество (jpg/webp) |
| interpolation | TEXT | nearest/bilinear/bicubic/lanczos |
| created_at | TEXT | |
| updated_at | TEXT | |

### resolutions
Профили разрешений для генерации.

| Поле | Тип | Описание |
|------|-----|----------|
| id | INTEGER PK | Auto-increment |
| name | TEXT | Название профиля |
| width | INTEGER | Ширина в пикселях |
| height | INTEGER | Высота в пикселях |
| sort_order | INTEGER | Порядок сортировки |
| created_at | TEXT | ISO 8601 |

### hires_profiles
Профили hires fix для генерации.

| Поле | Тип | Описание |
|------|-----|----------|
| id | INTEGER PK | Auto-increment |
| name | TEXT | Название профиля |
| enable | BOOLEAN | Включить hires fix |
| upscaler | TEXT | Апскейлер sd.cpp: имя файла модели без расширения или "lanczos"/"nearest" |
| scale | REAL | Множитель увеличения |
| denoising | REAL | Сила denoising |
| resize_x | INTEGER | Целевая ширина |
| resize_y | INTEGER | Целевая высота |
| steps | INTEGER | Шаги hires |
| sort_order | INTEGER | Порядок сортировки |
| created_at | TEXT | ISO 8601 |

## Raw SQL паттерны

Проект использует `database/sql` без ORM. Типичные паттерны:

**Query:**
```go
rows, err := db.Query("SELECT id, name FROM presets WHERE type_id = ?", typeID)
defer rows.Close()
for rows.Next() {
    var p Preset
    rows.Scan(&p.ID, &p.Name)
}
```

**QueryRow:**
```go
var value string
err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
if err == sql.ErrNoRows { ... }
```

**Exec:**
```go
result, err := db.Exec("INSERT INTO presets (name, prompt) VALUES (?, ?)", name, prompt)
id, _ := result.LastInsertId()
```

**Transaction:**
```go
tx, _ := db.Begin()
tx.Exec(...)
tx.Exec(...)
tx.Commit()
```
