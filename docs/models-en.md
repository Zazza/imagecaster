[English](models-en.md) | [Русский](models-ru.md)

# Models and Presets

Imagecaster uses a dedicated model for each job. Every model except Flux 2 Klein downloads from the app's catalog (Settings → Catalog). Moss and Klein run on [Forge Neo](setup-en.md#stable-diffusion-webui) only.

## Models

| Model | Best for |
|-------|----------|
| **SD 1.5 / SDXL** | Light models for low-end graphics cards. Any style, from photo to illustration |
| **Pony / Illustrious** | Anime, cartoon and stylized characters |
| **Flux 1** | Follows detailed plain-language descriptions precisely, realism |
| **Z-Image Turbo** | Photorealism, the fastest of the set: 8 steps instead of the usual 20–50 |
| **Moss-Image** | Follows detailed descriptions as precisely as Flux 1, photorealistic people |
| **Flux 2 Klein** | Post-processing: effects, object replace and remove, remix that keeps the face, stylist |

### SD 1.5 / SDXL

The first generation of Stable Diffusion models. Presets use `dreamshaper_8` (all-round), `realisticVisionV60B1_v51HyperVAE` (portraits). The catalog has a few more for photorealism.

### Pony / Illustrious

SDXL-based models fine-tuned on anime and drawn characters. Presets use `riMixPONYIllustrious_riMixV2` (anime-realism blend). Prompts use quality tags `score_9, score_8_up…` — the presets add them for you.

### Flux 1

`flux1-dev-fp8` understands long scene descriptions in plain sentences well: who stands where, what they hold, what the light is. On Hugging Face the model is gated behind a license acceptance.

### Z-Image Turbo

`z-image-turbo-fp8-aio` — photorealism in 8 steps at CFG 1. Good for going through many variants. The original Forge does not support it.

### Moss-Image

`moss_image_sft_bf16` follows detailed descriptions as precisely as Flux 1. It is used for text-to-image generation only: Edit, Effect and Preserve face run on Flux 2 Klein. Besides the checkpoint it needs the `qwen35_9b_int8_convrot.safetensors` text encoder in `models/text_encoder/` and the `flux2-vae_small.safetensors` VAE in `models/VAE/` (the Comfy-Org/MOSS-Image repo).

### Flux 2 Klein

`flux-2-klein-4b` powers the sections that change a finished photo:

- **Post-Process** — light, film, skin, clothing material and style-change effects (presets below);
- **Object** — replace or remove what the mask outlines;
- **Remix** with "Preserve face" checked;
- **Stylist** for AI agents — trying on haircuts and outfits.

Klein is not in the catalog: put the checkpoint into `models/Stable-diffusion/`, the `qwen_3_4b.safetensors` text encoder into `models/text_encoder/` and the `flux2-klein-vae.safetensors` VAE into `models/VAE/`.

## Generation presets

A style is a preset: prompt, settings and model. Most styles come in several variants for different models — pick by your graphics card and task.

| Style | SD | Pony | Flux | Z-Image | Moss |
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

## Post-processing presets (Flux 2 Klein)

| Group | Presets |
|-------|---------|
| General | Shallow DOF, Color Pop, Soft Glow, Moody Dark, Crisp Reality, Bright & Airy |
| Light | Golden Hour, Evening Mood, Night City Neon, Soft Studio, Flash Photo, Candlelight, Blue Hour, Backlit, Overcast Soft, Harsh Noon |
| Film | Film Real 35mm, B&W Film, Vintage 70s, Cinematic Teal-Orange, Polaroid, Portra 400, Expired Film, VHS 90s, Super 8, Cross Process |
| Skin | Wet Skin, Dewy Skin, Glam Glow, Matte Skin |
| Clothing materials | Satin Gloss, Leather, Denim, Lace, Velvet, Fur, Latex, Knit & Wool, Sequins, Mesh, Wet Fabric, Cotton, Linen, Tweed, Corduroy, Suede, Chiffon, Tulle, Fleece, Shearling, Metallic |
| Style change | Realism, Photo Restore, Colorize, 3D Animation, Cartoon, Anime, Watercolor, Pencil Sketch, Oil Painting, Pixel Art |

Presets can be edited, created, imported and exported as JSON.
