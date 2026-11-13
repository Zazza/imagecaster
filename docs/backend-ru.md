[English](backend-en.md) | [Русский](backend-ru.md)

# Imagecaster — Бэкенд (Go)

Как устроена часть на Go: запуск, конфигурация, сервисы и их Wails bindings, события, обработка ошибок. Общая картина и протокол сервера — в [architecture-ru.md](architecture-ru.md).

## Запуск

`main.go`:

1. `config.Load()` — значения по умолчанию и путь к БД.
2. `preset.Open(cfg.DBPath)` — открывает SQLite и применяет миграции.
3. Создаёт `llm.Client`, `sd.Client` (берёт ID установки из БД), `serverclient.Client`.
4. `settings.Restore(...)` — читает из БД `server_url` и выбор моделей, направляет клиенты на сервер.
5. `NewApp(...)` — собирает все сервисы; затем сервису настроек передаются встроенные пресеты (`data/presets/*.json`).
6. `wails.Run(...)`, единственный привязанный объект — `App`. Обработчик ресурсов отдаёт `/api/img/<id>` и `/api/thumb/<id>` из `data/sessions` и `data/thumbs`.

`App.startup`: удаляет завершённые задания очереди, запускает обработчик очереди, запускает опрос БД раз в 2 с (сессии от `imagecaster-mcp`), очищает `data/batch`, восстанавливает размер и положение окна.
`App.shutdown`: останавливает очередь (до 5 с); выполнявшееся задание возвращается в `pending`.

Сборка в `NewApp`:

```go
a.sessions    = session.New(presets, dataDir, emitter)
a.settingsSvc = settings.New(presets, llm, sd, cfg, log, srvClient)
a.ieSvc       = importexport.New(presets, sd, log)
a.gen         = generation.New(presets, llm, sd, cfg, dataDir, emitter, kidsMgr, sessions, settingsSvc, settingsSvc, log)
queueProc     = queue.NewProcessor(gen, sd.(sd.QueueScoper), queueStore, dataDir, emitter)
a.queueSvc    = queue.NewService(queueStore, queueProc, emitter)
a.batch       = batch.New(dataDir + "/batch")
```

`cmd/imagecaster-mcp/main.go` повторяет шаги 1–4 и так же собирает `generation`, но без очереди и с пустым отправителем событий.

## Конфигурация

### Источники

| Источник | Что |
|----------|-----|
| `config.Load()` | Встроенные значения по умолчанию и тексты промптов. |
| Переменная `DB_PATH` | Файл БД; относительный путь считается от исполняемого файла. По умолчанию `data/presets.db`. |
| Таблица `settings` | Всё, что меняет пользователь: `server_url`, выбранные модели, тексты промптов, детский режим, состояние интерфейса, последние значения форм. |

`UpdateSettings` записывает только ключи из `config.AllowedSettings`, остальные пропускает. `server_url` нормализуется (добавляется `http://`, убирается `/` в конце); `llm_num_ctx` и `llm_max_tokens` становятся `0`, если это не неотрицательное число.

### Адрес сервера

`settings.ApplyServerURL` задаёт базовый адрес `serverclient` и выводит `<server_url>/api/sd` для `sd` и `<server_url>/api/llm` для `llm`. Вызывается при запуске (`Restore`) и при сохранении `server_url`.

При первом запуске после обновления с версии с прямым подключением `Restore` смотрит старые настройки `sd_url` / `llm_url`, проверяет порт 8080 на том же хосте и, если отвечает Imagecaster Server, сохраняет его как `server_url`.

### Структура Config

Главные поля `config.Config`: `DBPath`, `LLMModel`, `SDPromptModel` (модель для промптов), `VisionModel` (модель для фото), `SystemPrompt`, `DefaultNegative`, значения по умолчанию для сэмплера/шагов/CFG/размера. Поля моделей перезаписываются из настроек (`llm_model`, `sd_prompt_model`, `vision_model`, `llm_generate_model`, `llm_analyze_model`).

Константы `config`: инструкции промптов по умолчанию (варианты с тегами и связным текстом, анализ, описание одежды), `KidsModePrompt`, `MaxImageBytes` = 128 МБ и `ImageTooLarge(what)`.

### imagecaster-mcp

| Переменная / флаг | Значение |
|-------------------|----------|
| `-db` или `IMAGECASTER_MCP_DB` | путь к БД (иначе `DB_PATH` / по умолчанию) |
| `IMAGECASTER_MCP_OUT` | папка для файлов, по умолчанию `~/imagecaster-mcp-out` |
| `IMAGECASTER_MCP_NO_IMAGE_BYTES=1` | старые инструменты не отдают `image_base64` |

Старые имена с префиксом `SD_MCP_` тоже читаются.

## Сервисы и bindings по областям

Все bindings — экспортируемые методы `*App` в `app.go`. Большинство просто передаёт вызов сервису.

### Генерация — `internal/generation`

| Bindings | Что делают |
|----------|-----------|
| `GenerateSDPrompt`, `GetDefaultPromptInstruction`, `GetDefaultPromptInstructionProse` | ИИ превращает описание в промпт (теги или связный текст — по модели). |
| `GenerateImage`, `GenerateCompoundImage`, `GenerateFromImage`, `UpscaleImage` | Стиль, пайплайн (несколько шагов), из картинки, увеличение. |
| `EditImage`, `EditObject`, `RepairImage`, `ApplyLookDirect` | Правка по инструкции, удаление/замена объекта по маске, восстановление по тайлам, образ (одежда/причёска). |
| `AnalyzeImage`, `GetDefaultAnalyzePrompts`, `GetDefaultEditGarmentPrompt`, `DescribeForStyle` | Описание фото vision-моделью. |
| `CompilePostStyle` | ИИ превращает текст в рецепт постобработки. |
| `GetLastImage`, `ClearLastImage`, `SetLastImage` | Текущая картинка. |
| `InterruptGeneration` | Отменяет незавершённые задания этой установки на сервере. |

Результаты сохраняются в активную сессию истории. Детский режим фильтрует входной и выходной текст и добавляет негативные теги.

### Очередь — `internal/queue`

| Bindings | Что делают |
|----------|-----------|
| `EnqueueTxt2Img`, `EnqueueFromImage`, `EnqueueCompound`, `EnqueueLook`, `EnqueueEdit` | Добавить задание; возвращает ID. |
| `GetQueue`, `RemoveQueueJob`, `RetryQueueJob`, `CancelQueueJob`, `CancelQueue`, `ClearCompletedQueueJobs` | Управление заданиями. |
| `PauseQueue`, `ResumeQueue`, `IsQueuePaused`, `ResumePausedQueueJobs` | Пауза и продолжение. |

- Таблица `job_queue`; типы `txt2img`, `from_image`, `compound`, `look`, `edit`; статусы `pending`, `running`, `completed`, `failed`, `cancelled`, `paused`.
- Один обработчик. Каждое задание выполняется с SD-клиентом, привязанным к этому заданию (`sd.ForQueueJob`), и обёрткой LLM, которая сохраняет ответы в `data/queue-llm/<id>.json`.
- Сетевые ошибки (connection refused, timeout, EOF, reset, неизвестный хост, сеть недоступна) повторяются через 5 с × 2ⁿ, не больше 60 с; после `max_retries` (по умолчанию 3) задание становится `paused`. Остальные ошибки — `failed`.
- При запуске задания в статусе `running` возвращаются в `pending` (или `paused`, если повторы исчерпаны).
- Когда задание заканчивается, его задания на сервере удаляются в фоне.

### История и сессии — `internal/session`, `internal/preset`

| Bindings | Что делают |
|----------|-----------|
| `CreateSession`, `ListSessions`, `SwitchSession`, `RenameSession`, `DeleteSession` | Сессии. |
| `GetSessionItems`, `GetActiveSessionItem`, `SetActiveSessionItem`, `DeleteSessionItem`, `ClearSession`, `GetSessionImage`, `GetSessionThumb`, `HasSessionItems` | Элементы активной сессии. |
| `ListHistory`, `OpenInHistory`, `DeleteHistoryItems` | Галерея по всей истории: фильтры по сессии, модели, стилю/пайплайну, типу, форме, периоду, тексту промпта; счётчики по фильтрам. Поиск не учитывает регистр и для нелатинского текста (SQL-функция `ic_lower`). |
| `ConfirmClose` | Ответ на диалог закрытия, затем выход. |

### Стили, пайплайны, сохранённые тексты — `internal/preset`, `internal/importexport`

| Bindings | Что делают |
|----------|-----------|
| `ListPresets`, `ListPresetsByType`, `GetPreset`, `CreatePreset`, `UpdatePreset`, `DeletePreset`, `GetAllTags` | Стили. |
| `ListPresetTypes`, `GetPresetType`, `CreatePresetType`, `UpdatePresetType`, `DeletePresetType` | Типы стилей. |
| `ListCompoundPresets`, `GetCompoundPreset`, `CreateCompoundPreset`, `UpdateCompoundPreset`, `DeleteCompoundPreset` | Пайплайны (нужны имя и хотя бы один шаг). |
| `ListResolutions` … `DeleteResolution`, `ListHiresProfiles` … `DeleteHiresProfile` | Размеры и hires-профили. |
| `ListDescriptions`, `CreateDescription`, `CreateDescriptionFull`, `UpdateDescription`, `DeleteDescription`, `ListPrompts`, `CreatePrompt`, `DeletePrompt` | Сохранённые описания и промпты. |
| `ExportPresets`, `PreparePresetsExport`, `OpenImportFile`, `ValidateImportModels`, `ImportPresets` | Импорт/экспорт стилей. |
| `ExportCompoundPresets`, `PrepareCompoundPresetsExport`, `OpenImportCompoundFile`, `ImportCompoundPresets` | Импорт/экспорт пайплайнов. |

### Постобработка и экспорт — `internal/adjust`, `internal/finish`, `internal/importexport`

| Bindings | Что делают |
|----------|-----------|
| `PreviewAdjust`, `PreviewPost`, `AutoAdjust` | Превью коррекции (JPEG, ограниченная ширина) и автозначения. |
| `ExportImage` | Применяет коррекцию, размер, формат/рамку/подпись и сохраняет. |
| `ListExportPresets`, `SaveExportPreset`, `DeleteExportPreset` | Пресеты экспорта. |
| `ListPostRecipes`, `SavePostRecipe`, `DeletePostRecipe` | Рецепты постобработки. |

### Файлы — `internal/filebrowser`, `internal/clipboard`, `internal/batch`

`ReadImageFile` (диалог открытия), `ReadClipboardImage`, `SaveImage` (диалог сохранения), `FastSaveImage` (в папку, выбранную в «Моих изображениях»), `BrowseDirectory`, `ReadFileAsBase64`, `ReadThumbnail`, `SelectBrowserFolder`, `BatchSaveImage`, `BatchReset`.

### Сервер, модели, настройки — `internal/settings`, `internal/serverclient`

| Bindings | Что делают |
|----------|-----------|
| `CheckServer`, `DiscoverServers`, `GetServerStatus`, `CheckServices` | Проверка сервера (`ok` / `not_configured` / `not_found` / `outdated`), поиск по mDNS, статус, доступность SD и LLM. |
| `GetSettings`, `UpdateSettings` | Настройки. |
| `GetSDModels`, `GetSDSamplers`, `GetSDSchedulers`, `GetSDUpscalers`, `GetSDVAEs`, `GetSDLoRAs`, `GetLLMModels` | Списки с сервера. |
| `GetServerModels`, `GetServerLLMModels`, `DownloadServerModel`, `DownloadServerModelStream`, `PullServerLLMModel`, `PullServerLLMModelStream`, `DeleteServerModel`, `DeleteServerLLMModel` | Файлы моделей на сервере; варианты `Stream` шлют события прогресса. |
| `StartServerProcess`, `StopServerProcess`, `RestartServerProcess`, `GetServerProcessLogs` | Процессы сервера. |
| `GetModelCatalog`, `GetDefaultDownloadPlan`, `InstallPresetDeps` | Встроенный каталог, список файлов, нужных встроенным стилям, загрузка файлов одного стиля. |
| `GetLLMSetup`, `SaveLLMChoice` | Выбор модели для промптов и модели для фото. |
| `SyncPresets`, `GetModelPresets` | Ставит встроенные пресеты для моделей, которые есть на сервере (имена — в `auto_installed_presets`; удалённые не возвращаются), и отдаёт доступность. |

### Детский режим — `internal/kids`

`IsKidsModeActive`, `SetKidsMode(enabled, pin)`, `GetKidsCategories`, `SetKidsCategory(name, enabled, pin)`. PIN — 4 цифры, хранится как хеш с солью. Категории `nsfw` и `selfharm` включены всегда; `violence`, `horror`, `weapons`, `substances`, `mature` можно выключить с PIN.

### Прочее

`Version`, `SaveWindowLayout`, `GetFooterHeight`, `GetLogEntries`.

## События (бэкенд → фронтенд)

| Событие | Когда |
|---------|-------|
| `sd:progress`, `sd:progress:start`, `sd:progress:stop` | Прогресс задания картинки. |
| `llm:status` | ИИ `thinking` / `done`. |
| `object:stage`, `repair:progress`, `test:progress` | Этапы правки объекта, восстановления, тестовой генерации. |
| `queue:started`, `queue:progress`, `queue:completed`, `queue:failed`, `queue:paused`, `queue:changed` | Очередь. |
| `session:created`, `session:switched`, `session:deleted`, `session:added`, `session:active`, `session:selected`, `session:removed`, `session:cleared` | История. `session:created` с `external: true` — изменение от `imagecaster-mcp`. |
| `server:download:progress:<jobID>` | Одна строка прогресса загрузки модели. |
| `log:entry` | Новая запись журнала. |

## Обработка ошибок

- Стиль: `if err != nil`, обёртка `fmt.Errorf("...: %w", err)`. Если binding вернул ошибку, promise во фронтенде отклоняется с её текстом.
- Проверки входа в bindings возвращают простые ошибки (`no image provided`, `name is required`, `no save directory set …`). Картинки больше 128 МБ отклоняются с `… too large (max 128 MB)`.
- Клиент `sd`:
  - `APIError` — сервер ответил статусом ошибки (`sd server: <code>: <message> (status N)`);
  - `JobError` — задание упало на сервере (`code: message`);
  - `ErrUserCancel`, `ErrJobCancelled`, `ErrJobLost`, `ErrBadResponse`;
  - отправка задания повторяется до 3 раз при сетевых ошибках, но не при ошибках сервера; потерянное задание отправляется заново один раз, затем `ErrJobLost`.
- `serverclient`: поток загрузки заканчивается `[DONE]`, `[ERROR] <текст>` превращается в ошибку, оборванная связь — `ErrStreamLost` («connection to server lost»).
- Очередь: правила повторов выше; паника внутри задания помечает его `failed` с `panic: …`.
- Журнал: `logger.Logger` хранит последние 500 записей для вкладки «Логи»; вывод стандартного `log` перехватывается туда же. Ошибки, которые не возвращаются (уборка, сохранение настроек), пишутся как предупреждения.
