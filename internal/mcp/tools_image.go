package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/generation"
)

func registerImageTools(s *Server) {
	s.Register(Tool{
		Name:        "analyze",
		Description: "Разобрать изображение LLM-моделью: mode=tags — SD-теги для from_image, mode=describe — связное описание. Источник: image_path или image_base64.",
		InputSchema: props(map[string]any{
			"image_path":   prop("путь к изображению (png/jpg)", "string"),
			"image_base64": prop("картинка в base64", "string"),
			"mode":         prop("tags | describe (по умолчанию tags)", "string"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if err := s.localOnly("analyze"); err != nil {
				return "", err
			}
			mode := argString(args, "mode")
			if mode == "" {
				mode = "tags"
			}
			if mode != "tags" && mode != "describe" {
				return "", fmt.Errorf("mode должен быть tags или describe")
			}
			b64 := argString(args, "image_base64")
			if b64 == "" {
				path := argString(args, "image_path")
				if path == "" {
					return "", fmt.Errorf("нужен image_path или image_base64")
				}
				var err error
				b64, err = filebrowser.ReadFileAsBase64(path)
				if err != nil {
					return "", err
				}
			}
			return s.deps.Gen.AnalyzeImage(b64, mode)
		},
	})

	s.Register(Tool{
		Name:        "upscale",
		Description: "Апскейл x2 (img2img, denoise 0.4) по image_path. Параметры генерации берутся из sidecar .json рядом с файлом (создаётся инструментами generate/best_of) — он обязателен. preset_id — пресет исходной генерации (модель, VAE, лоры, clip skip), без него — только параметры sidecar. Результат: PNG + sidecar в out_dir.",
		InputSchema: props(map[string]any{
			"image_path": prop("путь к PNG со sidecar .json", "string"),
			"preset_id":  prop("id пресета исходной генерации", "integer"),
		}, "image_path"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if err := s.localOnly("upscale"); err != nil {
				return "", err
			}
			path := argString(args, "image_path")
			if path == "" {
				return "", fmt.Errorf("image_path обязателен")
			}
			b64, err := filebrowser.ReadFileAsBase64(path)
			if err != nil {
				return "", err
			}
			sidecar := sidecarInfoPath(path)
			if fi, err := os.Stat(sidecar); err == nil && fi.Size() > 1<<20 {
				return "", fmt.Errorf("sidecar %s слишком большой (%d байт)", sidecar, fi.Size())
			}
			infoBytes, err := os.ReadFile(sidecar)
			if err != nil {
				return "", fmt.Errorf("sidecar .json не найден (%s) — параметры генерации неизвестны: %w", sidecar, err)
			}
			res, err := s.deps.Gen.UpscaleImage(generation.UpscaleImageParams{
				ImageBase64: b64,
				GenInfo:     string(infoBytes),
				PresetID:    argInt(args, "preset_id"),
			})
			if err != nil {
				return "", err
			}
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			outPath, err := s.saveArtifact(base, "upscale-"+slugify(base), res.Image, res.Info)
			if err != nil {
				return "", err
			}
			return toJSON(map[string]any{
				"path": outPath,
				"seed": infoSeed(res.Info),
			}), nil
		},
	})

	s.Register(Tool{
		Name: "from_image",
		Description: "Ремикс img2img по image_path: перерисовка пресетом (preset_id) или пайплайном (compound_preset_id). tags (описание изменений) проходит LLM-конвертацию. " +
			"quality=true (только preset_id, требует resolution_id): после генерации — ESRGAN-апскейл до бокса разрешения и дочерний img2img denoise 0.35. " +
			"preserve_identity=true (только preset_id на klein- или qwen-image-модели): reference-ремикс — лицо исходника сохраняется, denoise ≤0.9, оптимально 0.6–0.7. " +
			"Удаление и замена объекта по маске — инструмент object_edit, правка по инструкции — edit. Результат: PNG + sidecar в out_dir.",
		InputSchema: props(map[string]any{
			"image_path":         prop("путь к исходному изображению", "string"),
			"preset_id":          prop("id пресета-стиля (или compound_preset_id)", "integer"),
			"compound_preset_id": prop("id пайплайна из compound_list (вместо preset_id)", "integer"),
			"tags":               prop("что изменить — теги или описание для LLM", "string"),
			"denoising_strength": prop("сила denoise 0..1 (по умолчанию 0.5)", "number"),
			"extra_negative":     prop("дополнительный негатив", "string"),
			"resolution_id":      prop("id разрешения: цель апскейла для quality, иначе fit исходника в бокс", "integer"),
			"hires_profile_id":   prop("id hires-профиля (только compound_preset_id)", "integer"),
			"seed":               prop("фиксированный сид (только preset_id)", "integer"),
			"overrides":          overridesSchema,
			"quality":            prop("качественный режим: 3 шага — img2img нативно, апскейл до бокса resolution_id, дочерний img2img (только preset_id)", "boolean"),
			"preserve_identity":  prop("сохранять лицо исходника (reference-ремикс; только preset_id с klein- или qwen-image-моделью)", "boolean"),
		}, "image_path"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if err := s.localOnly("from_image"); err != nil {
				return "", err
			}
			img, err := readImageArg(args, "image_path", true)
			if err != nil {
				return "", err
			}
			presetID := argInt(args, "preset_id")
			compoundID := argInt(args, "compound_preset_id")
			if (presetID > 0) == (compoundID > 0) {
				return "", fmt.Errorf("укажи ровно одно: preset_id или compound_preset_id")
			}
			overrides, err := decodeArg[generation.PresetOverrides](args, "overrides")
			if err != nil {
				return "", err
			}
			gp := generation.GenerateFromImageParams{
				ImageBase64:         img,
				Mode:                "img2img",
				DenoisingStrength:   argFloat(args, "denoising_strength"),
				Tags:                argString(args, "tags"),
				ExtraNegativePrompt: argString(args, "extra_negative"),
			}
			var name string
			if presetID > 0 {
				p, err := s.deps.DB.Get(presetID)
				if err != nil {
					return "", fmt.Errorf("preset not found: %w", err)
				}
				name = p.Name
				gp.GenMode = "preset"
				gp.PresetID = presetID
				gp.Seed = optInt(args, "seed")
				gp.Overrides = overrides
				if argBool(args, "quality") {
					gp.OutputMode = "quality"
				}
				gp.PreserveIdentity = argBool(args, "preserve_identity")
			} else {
				cp, err := s.deps.DB.GetCompoundPresetFull(compoundID)
				if err != nil {
					return "", fmt.Errorf("compound preset not found: %w", err)
				}
				if argBool(args, "quality") || argBool(args, "preserve_identity") || overrides != nil {
					return "", fmt.Errorf("quality, preserve_identity и overrides работают только с preset_id")
				}
				name = cp.Name
				gp.GenMode = "compound"
				gp.CompoundPresetID = compoundID
				if hid := optInt(args, "hires_profile_id"); hid != nil && *hid > 0 {
					gp.HiresProfileID = hid
				}
			}
			if rid := optInt(args, "resolution_id"); rid != nil && *rid > 0 {
				gp.ResolutionID = rid
			}
			res, err := s.deps.Gen.GenerateFromImage(gp)
			if err != nil {
				return "", err
			}
			return s.imageResult(name, "fromimg-"+slugify(name), res, true, nil)
		},
	})
}
