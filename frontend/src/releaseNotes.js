export const RELEASE_NOTES = [
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
