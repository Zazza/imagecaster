[English](frontend-en.md) | [Русский](frontend-ru.md)

# Imagecaster — фронтенд (Vue 3)

Vue 3 (`<script setup>`) + Vite, без роутера и без библиотеки состояния. Иконки — `lucide-vue-next`, шрифты — IBM Plex Sans / Mono (`@fontsource`). Код — в `frontend/src/`.

```
frontend/src/
├── main.js            # монтирует App.vue, подключает шрифты и style.css
├── App.vue            # оболочка: заголовок окна, боковое меню, страницы, подвал, окна запуска
├── api.js             # обёртка над Wails bindings
├── i18n/              # index.js, en.js, ru.js
├── components/        # страницы, секции, модальные окна, мелкие виджеты
├── composables/       # функции use*
├── *.js               # модули состояния и чистые помощники
├── style.css          # глобальные стили и переменные темы
├── wailsjs/           # генерирует Wails, не править
└── __tests__/         # vitest
```

## Оболочка (App.vue)

- **Заголовок окна** — `TitleBar.vue`. На Linux и Windows окно без системной рамки (`Frameless: runtime.GOOS != "darwin"` в `main.go`); `composables/useWindowFrame.js` следит за развёрнутым/полноэкранным состоянием и платформой.
- **Боковое меню** — страницы (ключ i18n → страница):

| Ключ | Подпись | Компонент |
|---|---|---|
| `app.nav_generate` | Генерация | `UnifiedGeneratePage.vue` → `GeneratePage.vue` |
| `app.nav_postprocess` | Постобработка | `PostProcessPage.vue` |
| `app.nav_gallery` | Галерея | `GalleryPage.vue` |
| `app.nav_my_images` | Мои изображения | `FileBrowserPage.vue` |
| `app.nav_styles` | Стили | `UnifiedPresetsPage.vue` |
| `app.nav_catalog` | Каталог | `CatalogPage.vue` |
| `app.nav_settings` | Настройки | `SettingsPage.vue` |

  Внизу меню: кнопка сворачивания (состояние в `localStorage`, ключ `imagecaster_sidebar`), светлая/тёмная тема (сохраняется в настройку `theme`), переключатель RU/EN, «Сбросить всё» (`app.reset_all`, два нажатия) — очищает сохранённые значения форм Генерации и Постобработки и последнюю картинку.
- **Полоса о сервере** — видна, когда проверка сервера не `ok` и окно проверки закрыто: `server.banner_offline`, `server.banner_outdated` или `server.banner_incompatible`, с кнопкой `server.banner_fix`, которая снова открывает окно проверки.
- **Подвал** — `AppFooter.vue`.
- Настройки и Каталог остаются смонтированными (`v-show`); остальные страницы создаются при открытии.

### Порядок запуска

1. `checkServer()` из `serverState.js` (метод `CheckServer`).
2. Не `ok` → `ServerCheckModal.vue`: поиск в локальной сети (`DiscoverServers`), ввод адреса вручную, ссылка «Как установить/обновить сервер» (`server.how_to_install` / `server.how_to_update`), «Повторить», «Закрыть» (`server.close_limited`) — приложение продолжает работать в ограниченном режиме с полосой о сервере.
3. `WelcomeModal.vue` — вкладки «Что это» (`welcome.tab_about`) и «Что нового» (`welcome.tab_news`, из `releaseNotes.js`). Открывается при запуске, пока не отмечено «Больше не показывать при запуске» (`welcome.dont_show`); после обновления открывается на «Что нового»; при новой установке (на сервере нет SD-моделей) открывается, даже если его отключили. Блок «Модели» предлагает «Скачать всё для пресетов» (`settings.btn_download_all`) — файлы, нужные встроенным стилям (`GetDefaultDownloadPlan`). Состояние — `localStorage`, ключ `imagecaster_welcome`. Клик по логотипу в меню открывает окно снова.
4. `LLMSetupModal.vue` — если `GetLLMSetup().needed`: выбор модели для промптов и модели для фото (vision); можно скачать выбранные модели и сохранить (`SaveLLMChoice`).

## Страницы и секции

### Генерация
`UnifiedGeneratePage.vue` держит общее состояние (`provide('sharedGenState')`) и принимает перетащенный файл картинки — открывает Постобработку → Ремикс с этой картинкой. `GeneratePage.vue`:
- режимы: Стиль (`generate.btn_style`), Рабочий процесс (`generate.btn_workflow`), Конструктор (`generate.btn_custom`, шаги редактируются в `CustomPipelineEditor.vue`, до 8 — `customSteps.js`);
- «Сгенерировать изображение» (`generate.btn_generate`): LLM пишет промпт по описанию (`GenerateSDPrompt`), затем нужное число картинок (1–100) уходит в очередь (`EnqueueTxt2Img` / `EnqueueCompound`);
- «Сгенерировать напрямую» (`generate.btn_generate_directly`): сразу отправляет уже написанный промпт (`GenerateImage` / `GenerateCompoundImage`);
- сохранённые описания и пакет идей — `SavedDescriptionsModal.vue`, `ideasBatch.js` (до 20 картинок на идею);
- `ResolutionSelector.vue`, `HiresProfileSelector.vue`, `PresetOverridesPanel.vue` (разовая подмена настроек стиля, веса LoRA через `loraOverride.js`), увеличение ×2 (`UpscaleImage`);
- значения формы сохраняются в настройки (ключи `gen_*`).

### Постобработка
`PostProcessPage.vue` — одна картинка (или пакет), карточки пронумерованы по порядку:

| № | Ключ заголовка | Подпись | Реализация |
|---|---|---|---|
| 01 | `post.section_edit` | Правка | `EditSection.vue`: правка по инструкции (`EditImage` / `EnqueueEdit`) или «Описать вид» (`edit.mode_look`: `CompilePostStyle`, `ApplyLookDirect` / `EnqueueLook`); референс, стиль, LoRA |
| 02 | `post.section_remix` | Ремикс | `RemixSection.vue`: перерисовка в стиле или рабочем процессе (`EnqueueFromImage`), анализ фото (`AnalyzeImage`) |
| 03 | `post.section_object` | Объект | `ObjectSection.vue`: маска кистью, Удалить / Заменить (`EditObject`) |
| 04 | `post.section_repair` | Ремонт | `RepairSection.vue`: маска по повреждениям (`RepairImage`) |
| 05 | `post.effect_title` | Эффект | стили-эффекты типов `post`, `post_*` (группы в `effectTypes.js`) через `EnqueueFromImage`; Авто-стиль (`post.autostyle`, `DescribeForStyle`) |
| 06 | `post.adjust_title` | Коррекция | локальная коррекция без ИИ (`PreviewAdjust`, `AutoAdjust`) |
| 07 | `post.finish_title` | Формат, рамка, подпись | применяется при сохранении (`PreviewPost`) |
| 08 | `export.export_settings` | Настройки экспорта | `ExportImage`, сохранённые настройки экспорта |
| 09 | `post.recipes_title` | Рецепты | сохранённые цепочки (`RecipeComposition.vue`, `ListPostRecipes` / `SavePostRecipe`) |

Ещё: отмена/повтор применённых шагов (Ctrl+Z / Ctrl+Y или Ctrl+Shift+Z), масштаб (`useZoomPan.js`), маски (`useMaskCanvas.js`). Пакетный режим (`postBatch.js`) обрабатывает через очередь несколько картинок, присланных из «Мои изображения» или Истории; временные файлы — через `BatchSaveImage`. Страница слушает `session:selected`, чтобы загрузить картинку, выбранную в Истории.

### Галерея
`GalleryPage.vue` — все картинки истории по всем сессиям (`ListHistory`): фильтры по сессии (группе), периоду (`galleryFilters.js`: всё / сегодня / неделя / месяц / свой), модели, стилю или рабочему процессу, типу и форме, поиск по промпту; подгрузка «Загрузить ещё»; выбор и удаление (`DeleteHistoryItems`); «Открыть в истории» (`gallery.open_in_history`, `OpenInHistory`); просмотр в `ImageViewer.vue`.

### Мои изображения
`FileBrowserPage.vue` — просмотр локальной папки (`BrowseDirectory`, `ReadThumbnail`), сортировка по дате или имени (`fileSort.js`), выбор папки (`SelectBrowserFolder`), выбор нескольких файлов и отправка в Постобработку (`browser.send_to_postprocess_multi`).

### Стили
`UnifiedPresetsPage.vue` с вкладками:
- Стили (`unified.tab_presets`) и Эффекты (`unified.tab_effects`) — `PresetsPage.vue` в режимах `styles` / `effects`: таблица с фильтрами (`presetFilters.js`), редактирование в `PresetForm.vue`, импорт (`ImportModal.vue`) / экспорт, установка моделей стиля (`InstallPresetDeps`);
- Рабочие процессы (`unified.tab_pipelines`) — `CompoundPresetsPage.vue`: таблица с фильтрами (`pipelineFilters.js`), редактирование шагов с `SearchableSelect.vue`, импорт (`PipelineImportModal.vue`), удаление с подтверждением.

Стили, для которых на сервере нет модели или LoRA, скрыты (`presetAvailability.js`, статусы от `SyncPresets` через `composables/usePresets.js`); если список статусов не получен, ничего не скрывается.

### Каталог
`CatalogPage.vue` — каталог моделей сервера (`GetModelCatalog`): скачать / удалить модели и LLM, «Скачать всё для пресетов», прогресс в `DownloadProgress.vue`. Состояние — в `serverModels.js`.

### Настройки
`SettingsPage.vue`, вкладки:
- Подключение (`settings.tab_connection`) — адрес сервера и поиск в сети, процессы сервера (запуск / остановка / перезапуск / логи), память GPU, выбор моделей для промптов и для фото, параметры LLM (температура, контекст, максимум токенов, top-p, потоки);
- Промпты (`settings.tab_prompts`) — редактируемые промпты со значениями по умолчанию от бэкенда;
- Безопасность (`settings.tab_safety`) — безопасный режим (`settings.section_kids`) с PIN (`PinModal.vue`) и категориями;
- Дополнительно (`settings.tab_advanced`) — видеопамять (`settings.section_memory_guard`, автовыгрузка LLM), списки SD/LoRA и LLM-моделей на сервере.

### Подвал
`AppFooter.vue` — статус сервисов (`CheckServices`), версия приложения, три вкладки панели:
- История (`footer.session`) — элементы текущей сессии, переключение сессий (слушает `session:*`);
- Журнал (`footer.log`) — `GetLogEntries` + `log:entry`;
- Обработка (`footer.processing`) — `QueuePanel.vue`: пауза / продолжение, отмена, повтор, продолжение приостановленных (`queue.resume_paused`).

Одна кнопка закрытия открытой панели. Высота подвала сохраняется через `SaveWindowLayout`.

### Общие виджеты
`ImageViewer.vue` (полный просмотр, масштаб, пред./след., удаление, отправка в Постобработку, данные генерации), `FoldBlock.vue` (сворачиваемый блок), `StatusMessage.vue`, `ToggleSwitch.vue`, `SearchableSelect.vue`, `DownloadProgress.vue`.

## api.js

- Импортирует методы из `wailsjs/go/main/App.js` и экспортирует один объект `api` с методами в camelCase; часть добавляет значения по умолчанию или собирает объект параметров (например, `generateImage(presetId, extraPrompt, …)` → `GenerateImage({preset_id, …})`).
- `sessionImageUrl(id)` / `sessionThumbUrl(id)` возвращают `/api/img/<id>.jpg` / `/api/thumb/<id>.jpg`.
- `downloadServerModelStream` / `pullServerLLMModelStream` создают id задания, подписываются на `server:download:progress:<jobID>` и разбирают прогресс через `downloadProgress.js`.
- Вызовы сервера (генерация, анализ, правки, `enqueue*`, `retryQueueJob`) обёрнуты `serverGate.js`: пока проверка сервера не `ok`, они отклоняются с `server.need_server`.
- Методы: см. [api-reference-ru.md](api-reference-ru.md).

## Composables

| Файл | Назначение |
|---|---|
| `useGenerationProgress.js` | прогресс LLM/SD из `llm:status` и `sd:progress`, прерывание |
| `useQueue.js` | список очереди и состояние паузы, обновляется по событиям `queue:*` |
| `usePresets.js` | загрузка стилей, типов и рабочих процессов; `withPresetStatuses` ждёт `SyncPresets` до 1,5 с, чтобы скрыть недоступные стили |
| `useKidsMode.js` | включён ли безопасный режим |
| `useMaskCanvas.js` | рисование маски с отменой/повтором |
| `useZoomPan.js` | масштаб 10–500 % (быстрые 30/50/100), сдвиг, Ctrl+колесо |
| `useWindowFrame.js` | классы состояния окна для окна без рамки |

## Модули состояния и помощники

Общее состояние — обычные `reactive`/`ref`, экспортируемые из модулей:

| Файл | Что хранит / делает |
|---|---|
| `serverState.js` | результат проверки сервера (`state`, `url`, `version`, `missing`, `reason`), `checkServer()`, `useServer(url)`; ссылки на установку сервера |
| `serverGate.js` | шлагбаум, блокирующий вызовы сервера в `api.js` |
| `serverModels.js` | файлы на сервере по типам, текущая загрузка, соответствие «модель → стили», помощники скачивания/удаления |
| `modelDownloads.js` | план и прогресс «скачать всё» |
| `remixQueue.js` | список картинок, переданных в пакетную постобработку |
| `historyViewer.js` | позволяет другим страницам открыть элемент в просмотрщике Истории в подвале |
| `welcome.js`, `releaseNotes.js` | когда показывать окно приветствия; список изменений |
| `customSteps.js`, `ideasBatch.js`, `loraOverride.js`, `modelLora.js`, `seed.js` | помощники Генерации |
| `postBatch.js`, `effectTypes.js` | пакет Постобработки и группы эффектов |
| `galleryFilters.js`, `presetFilters.js`, `pipelineFilters.js`, `presetAvailability.js`, `fileSort.js`, `highlight.js` | фильтры, сортировка, подсветка |
| `downloadProgress.js` | разбор и форматирование прогресса загрузки |
| `windowEdge.js` | курсор изменения размера у края окна |
| `constants.js` | `MAX_IMAGE_SIZE` (128 МБ) |

## i18n

- `i18n/index.js`: `t(key, params)`, `setLocale(l)`, `locale`. Порядок поиска: текущий язык → английский → сам ключ. Параметры в строках пишутся как `{name}`.
- Языки: `ru` (по умолчанию) и `en`. Выбор хранится в `localStorage` (ключ `ui_locale`) и передаётся бэкенду настройкой `ui_locale`.
- В `en.js` и `ru.js` одинаковые 1079 ключей, сгруппированные по префиксу (`app.`, `generate.`, `post.`, `settings.`, `gallery.`, `queue.`, `server.`, `welcome.`, …).
- Правило: каждая видимая пользователю строка идёт через `t()` и добавляется в оба файла.

## Тема

`style.css` задаёт переменные на `:root` (тёмная) и `[data-theme="light"]`: `--bg`, `--surface`, `--border`, `--text`, `--text-bright`, `--text-dim`, `--accent`, `--accent-hover`, `--danger`, `--warning`, `--success`, `--radius`, `--font-sans`, `--font-mono` и другие. Тема включается атрибутом `data-theme` на `<html>`.

## Клавиатура

| Клавиши | Где | Действие |
|---|---|---|
| Ctrl/Cmd+Enter | Генерация, Ремикс, Объект, Ремонт | запуск |
| Ctrl/Cmd+Z, Ctrl/Cmd+Y, Ctrl/Cmd+Shift+Z | Постобработка (не в текстовых полях) | отмена / повтор (маски — в режиме маски) |
| Ctrl+колесо | Постобработка, просмотрщик | масштаб |
| Esc, ←, →, Delete | просмотрщик | закрыть, предыдущая, следующая, удалить |
| Esc | окна проверки сервера и приветствия | закрыть |

## Тесты

- Vitest + `@vue/test-utils`, окружение `happy-dom` (`frontend/vitest.config.js`, файл настройки `src/__tests__/setup.js`).
- Запуск: `cd frontend && npm test` (`vitest run`), в режиме наблюдения — `npm run test:watch`.
- 75 файлов тестов в `src/__tests__/`, по областям: Генерация, Постобработка (эффекты, пакет, формат и рамка, живое превью, маски), Галерея, таблицы Стилей и Рабочих процессов, Каталог, Настройки, проверка сервера, приветствие и выбор LLM, очередь, подвал, просмотрщик, боковое меню, i18n отдельных текстов.
- Моки: тесты подменяют методы через `vi.mock('../wailsjs/go/main/App.js', …)` — обычно `mocks/wails-app.js` (все методы как `vi.fn`) — и runtime через `vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))`; `mocks/runtime.js` хранит настоящих слушателей и даёт `EventsEmit`, чтобы вызывать события в тестах.
