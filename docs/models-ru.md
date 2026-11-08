[English](models-en.md) | [Русский](models-ru.md)

# Модели и пресеты

Под каждую задачу в Imagecaster подобрана своя модель. Все модели, кроме Flux 2 Klein, скачиваются из каталога приложения (Настройки → Каталог). Moss и Klein работают только на [Forge Neo](setup-ru.md#stable-diffusion-webui).

## Модели

| Модель | Для чего |
|--------|----------|
| **SD 1.5 / SDXL** | Лёгкие модели для слабых видеокарт. Любые стили — от фото до иллюстрации |
| **Pony / Illustrious** | Аниме, мультяшные и стилизованные персонажи |
| **Flux 1** | Точно следует подробному описанию обычным языком, реализм |
| **Z-Image Turbo** | Фотореализм, самая быстрая из набора: 8 шагов вместо обычных 20–50 |
| **Moss-Image** | Точно следует подробному описанию не хуже Flux 1, фотореализм людей, режим правки: меняет на фото ровно то, что написано в инструкции, понимает русский |
| **Flux 2 Klein** | Постобработка: эффекты, замена и удаление объектов, ремикс с сохранением лица, стилист |

### SD 1.5 / SDXL

Первое поколение моделей Stable Diffusion. В пресетах — `dreamshaper_8` (универсальная), `realisticVisionV60B1_v51HyperVAE` (портреты) и `leosamsHelloworldXL_helloworldXL70` (3D и стилизация). В каталоге ещё около десятка моделей: фотореализм, кино, живопись.

### Pony / Illustrious

Модели на базе SDXL, дообученные на аниме и рисованных персонажах. В пресетах — `riMixPONYIllustrious_riMixV2` (смесь аниме и реализма). Промпт пишется тегами качества `score_9, score_8_up…` — пресеты подставляют их сами.

### Flux 1

`flux1-dev-fp8` — хорошо понимает длинное описание сцены обычными предложениями: кто где стоит, что держит, какой свет. На Hugging Face доступ к модели открывается после принятия лицензии.

### Z-Image Turbo

`z-image-turbo-fp8-aio` — фотореализм за 8 шагов при CFG 1. Подходит для перебора большого числа вариантов. Оригинальный Forge эту модель не поддерживает.

### Moss-Image

`moss_image_sft_bf16` — следует подробному описанию так же точно, как Flux 1, и при этом основная модель режима правки: «замени брюки на юбку» меняет одежду, сохраняя человека, позу и фон. Инструкции можно писать по-русски. Кроме чекпойнта, нужны текстовый энкодер `qwen35_9b_int8_convrot.safetensors` в `models/text_encoder/` и VAE `flux2-vae_small.safetensors` в `models/VAE/` (репозиторий Comfy-Org/MOSS-Image).

### Flux 2 Klein

`flux-2-klein-4b` работает в разделах, которые меняют уже готовое фото:

- **Постобработка** — эффекты света, плёнки, кожи, материалов одежды и смены стиля (пресеты ниже);
- **Объект** — замена или удаление того, что обведено маской;
- **Ремикс** с галочкой «Сохранять лицо»;
- **Стилист** для ИИ-агентов — примерка стрижки и образа.

В каталоге Klein нет: чекпойнт кладётся вручную в `models/Stable-diffusion/`, текстовый энкодер `qwen_3_4b.safetensors` — в `models/text_encoder/`, VAE `flux2-klein-vae.safetensors` — в `models/VAE/`.

## Пресеты генерации

Стиль — это пресет: промпт, настройки и модель. Большинство стилей есть в нескольких вариантах под разные модели — выбирайте по видеокарте и задаче.

| Стиль | SD | Pony | Flux | Z-Image | Moss |
|---|---|---|---|---|---|
| 3D Render | ✓ | ✓ | ✓ | ✓ |  |
| Anime Art | ✓ | ✓ | ✓ | ✓ |  |
| Architecture Photo | ✓ |  | ✓ | ✓ | ✓ |
| Art Nouveau | ✓ | ✓ | ✓ | ✓ |  |
| Cartoon Art | ✓ | ✓ | ✓ | ✓ |  |
| Cinematic Still | ✓ |  | ✓ | ✓ | ✓ |
| Comic Art | ✓ | ✓ | ✓ | ✓ |  |
| Concept Art | ✓ | ✓ | ✓ | ✓ |  |
| Fantasy Art | ✓ | ✓ | ✓ | ✓ |  |
| Fashion Editorial | ✓ |  | ✓ | ✓ | ✓ |
| Food Photo | ✓ |  | ✓ | ✓ | ✓ |
| Horror Art | ✓ | ✓ | ✓ | ✓ |  |
| Landscape Photo | ✓ |  | ✓ | ✓ | ✓ |
| Mecha Art | ✓ | ✓ | ✓ | ✓ |  |
| Miniature Effect | ✓ |  | ✓ | ✓ |  |
| Pencil Sketch | ✓ | ✓ | ✓ | ✓ |  |
| Photo Realism | ✓ |  | ✓ | ✓ | ✓ |
| Pixel Art | ✓ | ✓ | ✓ | ✓ |  |
| Portrait Photo | ✓ |  | ✓ | ✓ | ✓ |
| Product Shot | ✓ |  | ✓ | ✓ | ✓ |
| Sci-Fi Art | ✓ | ✓ | ✓ | ✓ |  |
| Wallpaper Desktop | ✓ | ✓ | ✓ | ✓ |  |
| Wallpaper Phone | ✓ | ✓ | ✓ | ✓ |  |
| Watercolor Art | ✓ | ✓ | ✓ | ✓ |  |
| Wildlife Photo | ✓ |  | ✓ | ✓ | ✓ |
| B&W Film |  |  |  |  | ✓ |
| Golden Hour |  |  |  |  | ✓ |
| Night City |  |  |  |  | ✓ |
| Street Photo |  |  |  |  | ✓ |
| Studio Portrait |  |  |  |  | ✓ |

## Пресеты постобработки (Flux 2 Klein)

| Группа | Пресеты |
|--------|---------|
| Общие | Shallow DOF, Color Pop, Soft Glow, Moody Dark, Crisp Reality, Bright & Airy |
| Свет | Golden Hour, Evening Mood, Night City Neon, Soft Studio, Flash Photo, Candlelight, Blue Hour, Backlit, Overcast Soft, Harsh Noon |
| Плёнка | Film Real 35mm, B&W Film, Vintage 70s, Cinematic Teal-Orange, Polaroid, Portra 400, Expired Film, VHS 90s, Super 8, Cross Process |
| Кожа | Wet Skin, Dewy Skin, Glam Glow, Matte Skin |
| Материалы одежды | Satin Gloss, Leather, Denim, Lace, Velvet, Fur, Latex, Knit & Wool, Sequins, Mesh, Wet Fabric, Cotton, Linen, Tweed, Corduroy, Suede, Chiffon, Tulle, Fleece, Shearling, Metallic |
| Смена стиля | Realism, Photo Restore, Colorize, 3D Animation, Cartoon, Anime, Watercolor, Pencil Sketch, Oil Painting, Pixel Art |

Пресеты можно менять, создавать свои, импортировать и экспортировать в JSON.
