package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
)

var overridesSchema = prop("разовые переопределения пресета: {sampler, schedule_type, steps, cfg_scale, clip_skip, model_name, loras} — loras JSON-строкой [{\"name\":\"x\",\"weight\":0.6}], \"[]\" — без лор", "object")

var customStepsSchema = prop("своя цепочка вместо compound_preset_id: [{preset_id, denoising_strength (0 для первого шага, >0 для следующих), loras?, resolution_id?}], до 8 шагов", "array",
	map[string]any{"items": map[string]any{"type": "object"}})

var lorasSchema = prop("лоры: [{name, weight}] (до 8, вес 0–2)", "array",
	map[string]any{"items": map[string]any{"type": "object"}})

func decodeArg[T any](args map[string]any, key string) (*T, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s: неверный формат: %w", key, err)
	}
	return &out, nil
}

func readImageArg(args map[string]any, key string, required bool) (string, error) {
	path := argString(args, key)
	if path == "" {
		if required {
			return "", fmt.Errorf("%s обязателен", key)
		}
		return "", nil
	}
	return filebrowser.ReadFileAsBase64(path)
}

func (s *Server) imageResult(name, slug string, res *generation.GenerateImageResult, save bool, extra map[string]any) (string, error) {
	out := map[string]any{"prompt": res.EffectivePrompt}
	for k, v := range extra {
		out[k] = v
	}
	if res.QualityUpscaleSkipped {
		out["quality_upscale_skipped"] = true
	}
	if res.QualityRedenoiseSkipped {
		out["quality_redenoise_skipped"] = true
	}
	if res.EditReferenceResetFailed {
		out["edit_reference_reset_failed"] = true
	}
	if res.PreserveIdentityResetFailed {
		out["preserve_identity_reset_failed"] = true
	}
	if !save {
		out["image_base64"] = res.Image
		return toJSON(out), nil
	}
	path, err := s.saveArtifact(name, slug, res.Image, res.Info)
	if err != nil {
		return "", err
	}
	out["path"] = path
	return toJSON(out), nil
}

func registerEditTools(s *Server) {
	s.Register(Tool{
		Name: "object_edit",
		Description: "Удалить или заменить объект на фото по примерной маске (белое = объект; можно грубо, с запасом). " +
			"Vision-LLM определяет, что под маской (или object задаётся вручную), затем flux-2-klein в reference-режиме удаляет/заменяет, результат вклеивается в оригинал только в изменённой области у маски. " +
			"Настроек силы нет. Результат: PNG + sidecar в out_dir; ответ содержит detected_object.",
		InputSchema: props(map[string]any{
			"image_path":  prop("путь к исходному изображению", "string"),
			"mask_path":   prop("путь к маске (PNG, белое/непрозрачное — объект)", "string"),
			"op":          prop("remove | replace", "string"),
			"replacement": prop("на что заменить (обязательно для replace, можно по-русски)", "string"),
			"object":      prop("что под маской, если не хочешь полагаться на vision (англ. фраза, напр. 'the red mug on the table')", "string"),
			"seed":        prop("фиксированный сид", "integer"),
			"save":        prop("сохранить PNG+sidecar в out_dir (по умолчанию true)", "boolean"),
		}, "image_path", "mask_path", "op"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			img, err := readImageArg(args, "image_path", true)
			if err != nil {
				return "", err
			}
			mask, err := readImageArg(args, "mask_path", true)
			if err != nil {
				return "", err
			}
			op := argString(args, "op")
			res, err := s.deps.Gen.EditObject(generation.ObjectEditParams{
				ImageBase64: img,
				MaskBase64:  mask,
				Op:          op,
				Replacement: argString(args, "replacement"),
				Object:      argString(args, "object"),
				Seed:        optInt(args, "seed"),
			})
			if err != nil {
				return "", err
			}
			return s.imageResult("object-"+op, "object-"+op, res, optBool(args, "save", true), map[string]any{
				"detected_object": res.DetectedObject,
			})
		},
	})

	s.Register(Tool{
		Name: "edit",
		Description: "Правка фото по текстовой инструкции (moss image edit, reference-режим): лицо, поза и фон сохраняются, меняется то, что сказано. Инструкции на русском работают. " +
			"reference_image_path — фото вещи для примерки. style_preset_id — moss-пресет (его лоры, style_tags=true — ещё и теги). quality=true + resolution_id — апскейл и доводка. " +
			"Результат: PNG + sidecar в out_dir.",
		InputSchema: props(map[string]any{
			"image_path":           prop("путь к исходному изображению", "string"),
			"instruction":          prop("что изменить", "string"),
			"reference_image_path": prop("референс (фото одежды для примерки)", "string"),
			"style_preset_id":      prop("id moss-пресета для стиля", "integer"),
			"style_tags":           prop("добавить теги стиль-пресета (по умолчанию false)", "boolean"),
			"loras":                lorasSchema,
			"quality":              prop("качественный режим (требует resolution_id)", "boolean"),
			"resolution_id":        prop("id разрешения для quality", "integer"),
			"seed":                 prop("фиксированный сид", "integer"),
			"save":                 prop("сохранить PNG+sidecar в out_dir (по умолчанию true)", "boolean"),
		}, "image_path", "instruction"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			img, err := readImageArg(args, "image_path", true)
			if err != nil {
				return "", err
			}
			ref, err := readImageArg(args, "reference_image_path", false)
			if err != nil {
				return "", err
			}
			loras, err := decodeArg[[]preset.LoRAEntry](args, "loras")
			if err != nil {
				return "", err
			}
			ep := generation.EditImageParams{
				ImageBase64:          img,
				ReferenceImageBase64: ref,
				Instruction:          argString(args, "instruction"),
				StylePresetID:        argInt(args, "style_preset_id"),
				StyleTags:            argBool(args, "style_tags"),
				Seed:                 optInt(args, "seed"),
			}
			if loras != nil {
				ep.Loras = *loras
			}
			if argBool(args, "quality") {
				ep.OutputMode = "quality"
			}
			if rid := optInt(args, "resolution_id"); rid != nil && *rid > 0 {
				ep.ResolutionID = rid
			}
			res, err := s.deps.Gen.EditImage(ep)
			if err != nil {
				return "", err
			}
			return s.imageResult("edit", "edit", res, optBool(args, "save", true), nil)
		},
	})

	s.Register(Tool{
		Name: "look",
		Description: "Сменить вид фото по описанию (свет, цвет, настроение, фактура) — flux-2-klein reference-ремикс, лицо и композиция сохраняются. " +
			"text — описание вида (LLM переписывает в промпт), strength — сила 0.35–0.65 (по умолчанию 0.5). Результат: PNG + sidecar в out_dir.",
		InputSchema: props(map[string]any{
			"image_path": prop("путь к исходному изображению", "string"),
			"text":       prop("желаемый вид, можно по-русски", "string"),
			"strength":   prop("сила изменения 0.35–0.65 (по умолчанию 0.5)", "number"),
			"seed":       prop("фиксированный сид", "integer"),
			"save":       prop("сохранить PNG+sidecar в out_dir (по умолчанию true)", "boolean"),
		}, "image_path", "text"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			img, err := readImageArg(args, "image_path", true)
			if err != nil {
				return "", err
			}
			strength := argFloat(args, "strength")
			if strength <= 0 {
				strength = 0.5
			}
			res, err := s.deps.Gen.ApplyLookDirect(img, argString(args, "text"), strength, optInt(args, "seed"))
			if err != nil {
				return "", err
			}
			return s.imageResult("look", "look", res, optBool(args, "save", true), nil)
		},
	})
}
