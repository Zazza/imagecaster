[English](models-en.md) | [Русский](models-ru.md)

# Models and Presets

All models live on Imagecaster Server. The app has a built-in catalog of models with download links and downloads them to the server from the **Catalog** page. Files you put into the server folders by hand are used as well.

## File types and where they land

| Type | What it is | Server folder |
|---|---|---|
| Image model (checkpoint) | the main model file, `.safetensors` or `.gguf` | `models/checkpoints/` |
| Text encoder, tokenizer | extra files some image models need | `models/text_encoders/` |
| VAE | image decoder for some models | `models/vae/` |
| LoRA | small add-ons that change detail or look | `models/loras/` |
| Upscaler | model for enlarging images; not in the catalog — put the file in by hand (`.pth`, `.safetensors`, `.bin`, `.gguf`). Built in: `lanczos`, `nearest` | `models/upscalers/` |
| Assistant model | text model for prompts, or a model that reads photos | the server's text model folder |

Folders are relative to the server data folder (see [Setup](setup-en.md#data-folder)).

## Image models

"Extra files" are downloaded together with the model. ★ — marked as recommended in the catalog.

| Model | Family | Best for | Size, GB | Extra files |
|---|---|---|---|---|
| `z-image-turbo-fp8-aio` ★ | Z-Image | Photorealism, 8 steps, about 3 s per image after load. The main model | 9.6 | — |
| `moss_image_sft_bf16` ★ | Moss-Image | Photorealistic people, precise prompt following | 9.5 | text encoder, VAE, tokenizer (10.0) |
| `flux-2-klein-4b` ★ | Flux.2 | Photo editing: post-processing effects, Edit, Object, Repair, the stylist | 7.75 | text encoder, VAE (8.4) |
| `flux1-dev-fp8` ★ | Flux.1 | Best at following long plain-language prompts; one file with everything | 17.2 | — |
| `anima-aesthetic-v1.1` ★ | Anima | Anime and illustration: danbooru tags plus plain sentences, 30 steps | 4.18 | text encoder, VAE (1.4), presets |
| `anima-turbo-v1.1` | Anima | Faster Anima, 10 steps, about 5 s per image | 4.18 | text encoder, VAE (1.4), presets |
| `flux3-image-klein-Q8_0` | Flux.3 | Plain-language prompts, 16 steps | 6.2 | text encoder, VAE (2.8), presets |
| `Krea-2-Turbo-Q4_K_M` | Krea-2 | Aesthetic photos in few steps | 7.2 | text encoder, VAE (2.8), presets |
| `qwen_image_2.1-Q8_0` | Qwen-Image | Strong prompt following, readable text in images | 7.7 | text encoder, VAE (5.7), presets |
| `juggernautXL_ragnarokBy` | SDXL | Versatile photorealism | 6.6 | — |
| `epicrealismXL_pureFix` | SDXL | Realistic people | 6.5 | — |
| `photon_v1` | SDXL | Cinematic, film-like look | 2.0 | — |
| `realisticVisionV60B1_v51HyperVAE` | SD 1.5 | Portraits, fast on low VRAM | 2.0 | — |
| `epicrealism_naturalSinRC1VAE` | SD 1.5 | Natural skin, soft-light portraits | 2.0 | — |
| `dreamshaper_8` | SD 1.5 | All-round, from photo to illustration | 2.0 | — |
| `revAnimated_v2Rebirth` | SD 1.5 | Classic anime look | 4.0 | — |
| `ponyDiffusionV6XL_v6StartWithThisOne` | Pony XL | Pony V6 base, many styles | 6.5 | — |
| `riMixPONYIllustrious_riMixV2` | Pony/Illustrious | Anime-realism blend | 6.5 | — |

### How the prompt is written

The assistant writes the prompt differently by model family:

- **Plain sentences** — Flux (1, 2, 3), Z-Image, Moss, Klein, Qwen-Image, Krea, and also Chroma and Hunyuan models if you add them by hand.
- **Comma-separated tags** — every other model (SD 1.5, SDXL, Pony, Anima).

The family is recognized by the model file name.

## LoRA

| LoRA | For | Weight | Size, MB | What it does |
|---|---|---|---|---|
| `add-detail-xl` | SDXL | 0.7 | 136 | More detail and sharpness |
| `Add_Details_v1.2` | SDXL | 0.6 | 5.2 | Detail slider: positive weight adds, negative removes; also for Pony and Illustrious |
| `HDR_Photography` | Flux.1 | 0.5 | 18.4 | HDR photo look |
| `hairdetailer` | SD 1.5/SDXL | 0.6 | 68 | Hair strand detail |

"Weight" is the default weight the app uses.

## Assistant models

The assistant needs two models: one writes prompts, the other reads photos (remix, edits, looks, photo descriptions). They are chosen in the first-start window or in Settings and run on the server's llama.cpp. Photos are described only by this local model.

**Prompt model**

| Model | Graphics card | Size, GB |
|---|---|---|
| `qwen3:4b` | 8 GB | 2.5 |
| `qwen3:8b` ★ | 12 GB | 5.2 |
| `qwen3:14b` | 16 GB | 9.3 |
| `qwen3:30b` | 24 GB and more | 18.6 |

**Photo model**

| Model | Graphics card | Size, GB |
|---|---|---|
| `qwen3.5:4b` | 8 GB | 3.3 |
| `qwen3-vl:8b` ★ | 12 GB | 6.2 |
| `qwen3.5:9b` | 16 GB — also writes text, so it can take both roles | 6.6 |
| `gemma3:12b` | 16 GB | 8.2 |
| `qwen3.6:27b` | 24 GB and more | 17.8 |

## The Catalog page

- Sections: prompt models, photo models, image models, LoRA. Each entry shows its size, "Recommended" for ★ entries and "Installed" when the server already has it.
- **Download** fetches the model with its extra files; files already on the server are skipped. For models that bring their own presets the app adds those presets right away.
- **Delete** removes the model from the server.
- **Download all for presets** downloads everything the built-in presets need and the server lacks: models with their extra files, and LoRAs. Before starting it shows the number of files and total size. The same download is offered in the welcome window on the first start. Models not used by built-in presets (Krea 2, Qwen-Image, FLUX.3 and others) are downloaded one by one with their own **Download** button.

## Presets

A preset (style) is a ready set: prompt, settings, model and LoRAs. Presets come with their model:

- When the app gets the model list from the server, it adds the built-in presets of the models the server has. The names of added presets are remembered, so a preset you delete is not added back.
- Presets whose model or LoRA is not on the server are hidden. The "Show presets without models" box on the Styles page shows them.
- If the server does not return the model list, nothing is hidden.

| Model | Built-in presets |
|---|---|
| `z-image-turbo-fp8-aio`, `flux1-dev-fp8` | one variant of each of the 25 generation styles (3D Render, Anime, Architecture, Portrait, Watercolor and others) |
| `realisticVisionV60B1_v51HyperVAE`, `dreamshaper_8` | SD 1.5 variants of the same styles: photo styles on Realistic Vision, drawn styles on DreamShaper |
| `riMixPONYIllustrious_riMixV2` | variants of the drawn styles |
| `juggernautXL_ragnarokBy` | 3D Render |
| `moss_image_sft_bf16` | 14 photo presets of its own |
| `flux-2-klein-4b` | 100 post-processing effects |
| Anima aesthetic, Anima turbo | 5 each, added when the model is downloaded |
| FLUX.3 Klein, Krea 2, Qwen-Image 2.1 | 4 each, added when the model is downloaded |

### Post-processing effects (Flux.2 Klein)

| Group | Count |
|---|---|
| General | 7 |
| Light | 15 |
| Film | 13 |
| Skin | 6 |
| Clothing materials | 26 |
| Style change | 16 |
| Pattern | 5 |
| Hair | 5 |
| Background | 4 |
| Weather | 3 |

Presets can be edited, created, imported and exported.
