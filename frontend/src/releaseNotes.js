export const RELEASE_NOTES = [
  {
    version: '1.6.0',
    ru: [
      'Из «Правки», «Эффекта» и «Сохранять лицо» убрана модель Moss: новый быстрый движок генерации не умеет менять фото с её помощью.',
      'В этих местах работает Klein. Выбирать модель больше не нужно, сохранённый раньше выбор Moss не мешает.',
      'Генерация картинок по описанию на Moss осталась как была.',
      'Пресеты приходят вместе с моделью: скачали модель — появились её пресеты. Пресеты без модели или LoRA на сервере скрыты, флажок «Показать пресеты без моделей» в «Стилях» показывает их. В каталоге у каждой модели виден список её пресетов.',
      'При скачивании модели, LoRA или LLM в её строке видна полоска с процентом и размером; у моделей из нескольких файлов — какой файл качается.',
      'Просмотр картинки больше не закрывает кнопки окна. Все кнопки — в одной полосе сверху, видно «5 из 12», сведения открываются панелью справа, а случайный клик по фону не закрывает просмотр.',
    ],
    en: [
      'Moss was removed from Edit, Effect and Preserve face: the new fast generation engine cannot edit photos with it.',
      'Klein does this work now. There is no model to choose, and a previously saved Moss choice does no harm.',
      'Generating images from text with Moss works as before.',
      'Presets come with their model: download a model and its presets appear. Presets whose model or LoRA is missing on the server are hidden; "Show presets without models" in Styles shows them. The catalog lists the presets of each model.',
      'While a model, LoRA or LLM downloads, its row shows a progress bar with percent and size; multi-file models show which file is downloading.',
      'The image viewer no longer covers the window buttons. All controls sit in one top bar with a "5 of 12" counter, details open in a side panel, and a stray click on the background no longer closes the viewer.',
    ],
  },
  {
    version: '1.5.0',
    ru: [
      'В «Стилях» три вкладки: «Стили», «Эффекты» и «Сценарии». Эффекты постобработки больше не перемешаны со стилями, у них свой фильтр по типу.',
      'В постобработке у «Эффекта» можно выбрать модель: Klein, как раньше, или Moss. Moss точнее, но тратит на фото 20–60 секунд вместо 6–10.',
      'У «Правки» тоже выбор модели: Moss, как раньше, или Klein. Klein правит примерно за 10 секунд вместо 30–90 и не меняет размер фото.',
      'У каждой модели свой список LoRA и свои стили. Пакетная обработка берёт ту модель, которую вы выбрали.',
    ],
    en: [
      'Styles now has three tabs: Styles, Effects and Workflows. Post-process effects are no longer mixed with styles and have their own type filter.',
      'Post-Process Effect lets you pick the model: Klein, as before, or Moss. Moss is more precise but takes 20–60 seconds per photo instead of 6–10.',
      'Edit lets you pick the model too: Moss, as before, or Klein. Klein edits in about 10 seconds instead of 30–90 and keeps the photo size.',
      'Each model has its own LoRA list and its own styles. Batch processing uses the model you chose.',
    ],
  },
]
