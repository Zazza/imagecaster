[English](api-reference-en.md) | [Русский](api-reference-ru.md)

# Imagecaster — справочник API (Wails bindings)

Фронтенд общается с Go только через Wails bindings: экспортируемые методы `*App` в `app.go` (единственная привязанная структура, см. `Bind` в `main.go`). Wails генерирует JS-обёртки в `frontend/src/wailsjs/go/main/App.js` / `App.d.ts`; интерфейс вызывает их через `frontend/src/api.js` (имена в camelCase, например `GetSettings` → `api.getSettings()`).

Всего 156 методов; список ниже совпадает с `App.d.ts`. Каждый вызов возвращает Promise. Go-ошибка (`error`) превращается в отклонённый Promise.

Изображения передаются строками base64 (PNG или JPEG без префикса `data:`). Сохранённые картинки истории ещё и отдаются по HTTP обработчиком ассетов Wails в `main.go`: `/api/img/<id>.jpg` (полное изображение) и `/api/thumb/<id>.jpg` (миниатюра), где `<id>` — id элемента сессии.

Вызовы, которым нужен сервер (`generateSdPrompt`, `generateImage`, `upscaleImage`, `analyzeImage`, `generateCompoundImage`, `editImage`, `editObject`, `repairImage`, `describeForStyle`, `applyLookDirect`, все `enqueue*`, `retryQueueJob`), `api.js` блокирует с сообщением `server.need_server`, пока проверка сервера не вернула `ok`.

## Общий результат: GenerateImageResult

Возвращают все прямые методы генерации и правки (Go: `internal/generation/service.go`).

| Поле | Смысл |
|---|---|
| `image` | результат, base64 |
| `parameters`, `info` | детали генерации (JSON) |
| `effective_prompt`, `effective_negative_prompt` | итоговые промпты, ушедшие в модель |
| `hires_fix_skipped`, `hires_fix_manual` | состояние hires fix |
| `quality_upscale_skipped`, `quality_redenoise_skipped` | пропущенные шаги режима качества |
| `detected_object` | название объекта, найденного правкой «Объект» |
| `session_item_id` | id элемента истории, куда сохранён результат |

## Генерация (напрямую)

Выполняются сразу и ждут результата. Прогресс приходит событиями (см. ниже).

| Метод | Что делает |
|---|---|
| `GenerateSDPrompt(params)` | LLM пишет промпт для картинки по описанию и стилю (`preset_id`, `description`, `negative`, `overrides`). Возвращает `{prompt, negative_prompt}`. |
| `GenerateImage(params)` | Генерация из текста по стилю: `preset_id`, `extra_prompt`, `extra_negative_prompt`, `resolution_id`, `hires_profile_id`, `seed`, `overrides`, `idea`, `idea_negative`. |
| `GenerateCompoundImage(params)` | Запуск рабочего процесса (`compound_preset_id`) или своих шагов (`steps`) — несколько стилей подряд. |
| `GenerateFromImage(params)` | Генерация по фото (стиль или рабочий процесс, сила изменения, маска, сохранение цвета, сохранение лица). Интерфейс его не вызывает — ставит в очередь через `EnqueueFromImage`. |
| `UpscaleImage(params)` | Увеличение сгенерированной картинки по её `gen_info` и стилю. |
| `EditImage(params)` | Правка фото по текстовой инструкции (`instruction`, необязательный референс, стиль, LoRA, `model`, размер, seed). Постобработка → Правка. |
| `EditObject(params)` | Удалить или заменить закрашенный предмет (`mask_base64`, `op`, `replacement`, `object`). Постобработка → Объект. |
| `RepairImage(params)` | Убирает царапины, пыль и заломы внутри маски, по плиткам (`mask_base64`, `strength`). Постобработка → Ремонт. |
| `ApplyLookDirect(image, text, strength, seed)` | Применяет к фото вид, описанный словами. |
| `AnalyzeImage(image, mode)` | Описывает фото vision-моделью. `mode`: `describe` (текстом), `garment` (одежда), иначе — теги. |
| `DescribeForStyle(image)` | Короткое описание фото в 1–2 предложения для подбора эффекта (Постобработка → Эффект → Авто-стиль). |
| `CompilePostStyle(text)` | Превращает пожелание текстом в рецепт: до 3 шагов-эффектов и значения коррекции. |
| `InterruptGeneration()` | Прерывает текущую генерацию. |
| `GetLastImage()` / `ClearLastImage()` | Прочитать / очистить последнюю картинку. `GetLastImage` интерфейс не использует. |

Тексты промптов по умолчанию (показываются в Настройки → Промпты):

| Метод | Возвращает |
|---|---|
| `GetDefaultPromptInstruction()` | инструкцию по умолчанию для промптов картинок (теги) |
| `GetDefaultPromptInstructionProse()` | то же, вариант текстом |
| `GetDefaultAnalyzePrompts()` | `{system_prompt, single_prompt, describe_prompt}` для описания фото |
| `GetDefaultEditGarmentPrompt()` | промпт по умолчанию для описания одежды с референса |

## Очередь

Фоновые задания в SQLite (`internal/queue`). Каждый `Enqueue*` возвращает id задания.

| Метод | Что делает |
|---|---|
| `EnqueueTxt2Img(params)` | генерация из текста в очередь (параметры как у `GenerateImage`) |
| `EnqueueCompound(params)` | рабочий процесс или свои шаги в очередь |
| `EnqueueFromImage(params)` | генерация по фото / эффект в очередь |
| `EnqueueLook(params)` | вид по тексту в очередь (`image_base64`, `text`, `strength`, `seed`) |
| `EnqueueEdit(params)` | правка по инструкции в очередь |
| `GetQueue()` | список заданий |
| `RemoveQueueJob(id)` / `CancelQueueJob(id)` / `RetryQueueJob(id)` | удалить, отменить, повторить одно задание |
| `PauseQueue()` / `ResumeQueue()` / `IsQueuePaused()` | пауза и продолжение всей очереди |
| `CancelQueue()` | отменить все задания |
| `ResumePausedQueueJobs()` | вернуть в ожидание задания, приостановленные после исчерпания повторов; возвращает их число |
| `ClearCompletedQueueJobs()` | удалить завершённые задания |

## Стили (пресеты) и типы

| Метод | Что делает |
|---|---|
| `ListPresets()`, `ListPresetsByType(type)`, `GetPreset(id)` | чтение стилей (`GetPreset` интерфейс не использует) |
| `CreatePreset(p)`, `UpdatePreset(p)`, `DeletePreset(id)` | изменение стилей |
| `ListPresetTypes()`, `GetPresetType(id)`, `CreatePresetType(t)`, `UpdatePresetType(t)`, `DeletePresetType(id)` | типы стилей (типы генерации и типы эффектов `post`, `post_*`) |
| `GetAllTags()` | все теги стилей |
| `SyncPresets()` | ставит встроенные стили для моделей, которые есть на сервере; возвращает статусы стилей и число добавленных |
| `GetModelPresets()` | какие встроенные стили относятся к какой модели |
| `InstallPresetDeps(id)` | скачивает на сервер модель и недостающие LoRA стиля из каталога |

## Рабочие процессы (составные пресеты)

| Метод | Что делает |
|---|---|
| `ListCompoundPresets()`, `GetCompoundPreset(id)` | чтение рабочих процессов с шагами |
| `CreateCompoundPreset(cp)`, `UpdateCompoundPreset(cp)` | сохранение; нужны название и хотя бы один шаг |
| `DeleteCompoundPreset(id)` | удаление |

## Разрешения и профили hires

| Метод | Что делает |
|---|---|
| `ListResolutions()`, `GetResolution(id)`, `CreateResolution(r)`, `UpdateResolution(r)`, `DeleteResolution(id)` | размеры результата; ширина и высота 64–4096 |
| `ListHiresProfiles()`, `GetHiresProfile(id)`, `CreateHiresProfile(h)`, `UpdateHiresProfile(h)`, `DeleteHiresProfile(id)` | профили hires; увеличение 1.0–4.0, denoising 0–1 |

`GetResolution` и `GetHiresProfile` интерфейс не использует.

## Сохранённые описания и промпты

| Метод | Что делает |
|---|---|
| `ListDescriptions()`, `CreateDescription(text)`, `CreateDescriptionFull(d)`, `UpdateDescription(d)`, `DeleteDescription(id)` | сохранённые описания (идеи) для Генерации |
| `ListPrompts()`, `CreatePrompt(text)`, `DeletePrompt(id)` | сохранённые промпты; интерфейс не использует |

## Постобработка (локально, без ИИ)

| Метод | Что делает |
|---|---|
| `PreviewAdjust(image, adjust, maxW)` | превью Коррекции (яркость, контраст, цвет…), JPEG, уменьшено до `maxW` |
| `PreviewPost(image, adjust, finish, maxW)` | превью Коррекции вместе с «Формат, рамка, подпись» |
| `AutoAdjust(image)` | замеряет картинку и возвращает значения автокоррекции |
| `ExportImage(params)` | применяет настройки экспорта и сохраняет (в `output_dir` или через диалог) |
| `ListExportPresets()`, `SaveExportPreset(p)`, `DeleteExportPreset(id)` | сохранённые настройки экспорта |
| `ListPostRecipes()`, `SavePostRecipe(r)`, `DeletePostRecipe(id)` | сохранённые рецепты (цепочки эффектов и коррекций) |
| `BatchSaveImage(image)` / `BatchReset()` | временные файлы пакетной постобработки (`data/batch`, очищается при запуске) |

## История (сессии) и галерея

| Метод | Что делает |
|---|---|
| `CreateSession(name)`, `ListSessions()`, `SwitchSession(id)`, `RenameSession(id, name)`, `DeleteSession(id)` | группы истории (сессии) |
| `GetSessionItems()`, `GetActiveSessionItem()`, `SetActiveSessionItem(id)` | элементы текущей сессии и выбранный элемент |
| `DeleteSessionItem(id)`, `ClearSession()` | удалить один элемент / все элементы текущей сессии |
| `GetSessionImage(id)`, `GetSessionThumb(id)` | картинка / миниатюра в base64 |
| `HasSessionItems()` | есть ли элементы; интерфейс не использует |
| `ListHistory(filter)` | Галерея: элементы всех сессий с фильтрами (сессии, модели, метки, виды, формы, поиск, даты, периоды), постранично и со счётчиками |
| `OpenInHistory(id)` | переключает на сессию элемента и выбирает его |
| `DeleteHistoryItems(ids)` | удаляет несколько элементов; возвращает их число |
| `SetLastImage(image)` | добавляет картинку (например, из «Мои изображения») в текущую сессию и выбирает её |
| `ConfirmClose(action)` | закрывает приложение; при `discard` сначала удаляет все сессии. Интерфейс не использует. |

## Файлы, буфер обмена, окно

| Метод | Что делает |
|---|---|
| `ReadImageFile()` | диалог выбора PNG/JPEG, возвращает base64 |
| `ReadClipboardImage()` | картинка из буфера обмена |
| `SaveImage(image, name)` | сохранить PNG через диалог |
| `FastSaveImage(image, name, format)` | сохранить в папку «Мои изображения» без диалога; интерфейс не использует |
| `BrowseDirectory(path)`, `ReadFileAsBase64(path)`, `ReadThumbnail(path)` | «Мои изображения»: список папки, чтение файла, миниатюра |
| `SelectBrowserFolder()` | диалог выбора папки; запоминает её в `file_browser_path` |
| `SaveWindowLayout(footerHeight)`, `GetFooterHeight()` | размер/положение окна и высота подвала хранятся в настройках |
| `Version()` | версия приложения |
| `GetLogEntries()` | журнал приложения (подвал → Журнал) |

## Настройки, безопасный режим

| Метод | Что делает |
|---|---|
| `GetSettings()` / `UpdateSettings(map)` | настройки «ключ–значение» из таблицы `settings` |
| `IsKidsModeActive()`, `SetKidsMode(enabled, pin)` | включение/выключение безопасного режима с PIN |
| `GetKidsCategories()`, `SetKidsCategory(name, enabled, pin)` | запрещённые категории содержимого |

## Imagecaster Server

| Метод | Что делает |
|---|---|
| `CheckServer()` | проверяет `server_url`: `state` = `ok`, `not_configured`, `not_found` или `outdated` (со списком `missing` и `reason` `old_version` / `missing_features`) |
| `DiscoverServers()` | ищет серверы в локальной сети (mDNS) |
| `CheckServices()` | статус в подвале: доступны ли LLM и генерация картинок, какие модели загружены |
| `GetServerStatus()` | процессы сервера, здоровье, GPU, установки, модели |
| `StartServerProcess(name)`, `StopServerProcess(name)`, `RestartServerProcess(name)`, `GetServerProcessLogs(name, lines)` | управление процессами сервера (Настройки → Подключение) |
| `GetServerModels(type)`, `GetServerLLMModels()` | файлы на сервере по типу (`sd`, `lora`, `vae`, `text_encoder`…) и LLM-модели |
| `DownloadServerModel(type, url, file)`, `DeleteServerModel(type, file)` | скачать / удалить файл модели на сервере |
| `DownloadServerModelStream(jobID, type, url, file)` | то же с прогрессом через событие `server:download:progress:<jobID>` |
| `PullServerLLMModel(name)`, `PullServerLLMModelStream(jobID, name)`, `DeleteServerLLMModel(name)` | скачать / удалить LLM-модель |
| `GetModelCatalog()` | каталог моделей для страницы «Каталог» |
| `GetDefaultDownloadPlan()` | файлы, нужные встроенным стилям, и их общий размер (Приветствие → Модели) |
| `GetLLMSetup()` / `SaveLLMChoice(generate, vision)` | выбор при первом запуске модели для промптов и модели для фото (vision) |
| `GetSDModels()`, `GetSDSamplers()`, `GetSDSchedulers()`, `GetSDUpscalers()`, `GetSDVAEs()`, `GetSDLoRAs()` | списки для редактирования стилей, берутся с сервера |
| `GetLLMModels()` | LLM-модели, доступные на сервере |

## Импорт / экспорт

| Метод | Что делает |
|---|---|
| `ExportPresets(ids)` / `PreparePresetsExport(ids)` | сохранить стили в JSON (диалог сохранения файла / выбора папки) |
| `OpenImportFile()` | выбрать один или несколько JSON, возвращает предпросмотр стилей |
| `ValidateImportModels(items)` | предупреждения о моделях, которых нет на сервере |
| `ImportPresets(items)` | импорт стилей |
| `ExportCompoundPresets(ids)` / `PrepareCompoundPresetsExport(ids)` | сохранить рабочие процессы в JSON |
| `OpenImportCompoundFile()`, `ImportCompoundPresets(items)` | импорт рабочих процессов |

## События (backend → frontend)

Отправляются через `runtime.EventsEmit`, принимаются через `EventsOn` из `wailsjs/runtime/runtime`.

| Событие | Данные | Кто отправляет | Где слушают |
|---|---|---|---|
| `llm:status` | `{status: "thinking" \| "done"}` | генерация (вызовы LLM) | `useGenerationProgress.js` |
| `sd:progress` | прогресс, ETA, шаг, превью | генерация | `useGenerationProgress.js` |
| `object:stage` | `"analyzing"` / `"generating"` | правка «Объект» | `ObjectSection.vue` |
| `repair:progress` | `{done, total}` плиток | «Ремонт» | `RepairSection.vue` |
| `queue:changed` | — | очередь | `useQueue.js`, `AppFooter.vue`, `PostProcessPage.vue` |
| `queue:started` | `{job_id, type}` | обработчик очереди | `useQueue.js`, `AppFooter.vue` |
| `queue:progress` | `{job_id, progress, detail}` | обработчик очереди | `useQueue.js` |
| `queue:completed` | `{job_id, result}` | обработчик очереди | `useQueue.js`, `AppFooter.vue`, `GeneratePage.vue`, `RemixSection.vue`, `PostProcessPage.vue` |
| `queue:failed` | `{job_id, error}` | обработчик очереди | там же |
| `queue:paused` | `{job_id, error, retry_count, max_retries}` | обработчик очереди при ошибке, которую можно повторить: задание повторится позже или встанет на паузу, когда повторы кончились | `useQueue.js`, `GeneratePage.vue`, `RemixSection.vue`, `PostProcessPage.vue` |
| `session:added` | `{id}` | новый элемент истории | `AppFooter.vue`, `GeneratePage.vue` |
| `session:active` | `{id}` | элемент выбран | `AppFooter.vue` |
| `session:selected` | `{id}` | элемент выбран пользователем или через `SetLastImage` | `PostProcessPage.vue` |
| `session:removed` | `{id}` или `{ids}` | элементы удалены | `AppFooter.vue` |
| `session:cleared` | — | сессия очищена | `AppFooter.vue` |
| `session:created` | `{session_id, name}` или `{external: true}` | новая сессия; `external` — создана imagecaster-mcp (проверка раз в 2 с) | `AppFooter.vue` |
| `session:switched` | `{session_id}` | сессия переключена | `AppFooter.vue` |
| `session:deleted` | `{session_id}` | сессия удалена | `AppFooter.vue` |
| `log:entry` | запись журнала в JSON | `internal/logger` | `AppFooter.vue` |
| `server:download:progress:<jobID>` | текст прогресса | `DownloadServerModelStream`, `PullServerLLMModelStream` | `api.js` |

Отправляются, но интерфейс их не слушает: `compound:progress`, `fromimage:progress`, `sd:progress:start`, `sd:progress:stop`.
