package mcp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Zazza/imagecaster/internal/generation"
)

func registerGenerateTools(s *Server) {
	s.Register(Tool{
		Name: "generate",
		Description: "Сгенерировать изображение. Ветка 1: preset_id — стиль пресета, description (и negative) конвертируются LLM в SD-промпт, размер через resolution_id. " +
			"Ветка 2: prompt+model — прямой прогон без LLM (как Test), размер через width/height. " +
			"seed фиксирует сид. overrides (ветка 1) — разовые переопределения пресета (модель, сэмплер, шаги, CFG, лоры), hires_profile_id — hires fix. " +
			"PNG + sidecar .json пишутся в out_dir; ответ: path, seed, prompt. save=false — вернёт image_base64 вместо файла.",
		InputSchema: props(map[string]any{
			"preset_id":        prop("id пресета (ветка 1)", "integer"),
			"description":      prop("описание сцены для LLM-конвертации (ветка 1)", "string"),
			"negative":         prop("пользовательский негатив (ветка 1)", "string"),
			"prompt":           prop("готовый SD-промпт (ветка 2)", "string"),
			"model":            prop("модель SD title (ветка 2)", "string"),
			"seed":             prop("фиксированный сид", "integer"),
			"resolution_id":    prop("id разрешения (ветка 1)", "integer"),
			"overrides":        overridesSchema,
			"hires_profile_id": prop("id hires-профиля (ветка 1)", "integer"),
			"width":            prop("ширина (ветка 2, по умолчанию 512)", "integer"),
			"height":           prop("высота (ветка 2, по умолчанию 512)", "integer"),
			"sampler":          prop("сэмплер (ветка 2)", "string"),
			"steps":            prop("шаги (ветка 2)", "integer"),
			"cfg_scale":        prop("CFG (ветка 2)", "number"),
			"save":             prop("сохранить PNG+sidecar в out_dir (по умолчанию true)", "boolean"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			save := s.wantSave(args)
			presetID := argInt(args, "preset_id")
			if presetID > 0 {
				p, err := s.deps.DB.Get(presetID)
				if err != nil {
					return "", fmt.Errorf("preset not found: %w", err)
				}
				overrides, err := decodeArg[generation.PresetOverrides](args, "overrides")
				if err != nil {
					return "", err
				}
				extraPrompt, extraNeg := "", ""
				description := argString(args, "description")
				negative := argString(args, "negative")
				if description != "" || negative != "" {
					pr, err := s.deps.Gen.GenerateSDPrompt(generation.GenerateSDPromptParams{
						PresetID:    presetID,
						Description: description,
						Negative:    negative,
						Overrides:   overrides,
					})
					if err != nil {
						return "", err
					}
					extraPrompt, extraNeg = pr.Prompt, pr.NegativePrompt
				}
				gp := generation.GenerateImageParams{
					PresetID:            presetID,
					ExtraPrompt:         extraPrompt,
					ExtraNegativePrompt: extraNeg,
					Seed:                optInt(args, "seed"),
					Overrides:           overrides,
				}
				if rid := optInt(args, "resolution_id"); rid != nil && *rid > 0 {
					gp.ResolutionID = rid
				}
				if hid := optInt(args, "hires_profile_id"); hid != nil && *hid > 0 {
					gp.HiresProfileID = hid
				}
				res, err := s.deps.Gen.GenerateImage(gp)
				if err != nil {
					return "", err
				}
				out := map[string]any{
					"seed":            infoSeed(res.Info),
					"prompt":          res.EffectivePrompt,
					"negative_prompt": res.EffectiveNegativePrompt,
				}
				if save {
					path, err := s.saveArtifact(p.Name, "gen-"+slugify(p.Name), res.Image, res.Info)
					if err != nil {
						return "", err
					}
					out["path"] = path
				} else {
					out["image_base64"] = res.Image
				}
				return toJSON(out), nil
			}

			prompt := argString(args, "prompt")
			model := argString(args, "model")
			if prompt == "" || model == "" {
				return "", fmt.Errorf("нужен preset_id или пара prompt+model")
			}
			tp := generation.TestGenerateParams{
				Mode:           "models",
				SelectedModels: []string{model},
				Prompt:         prompt,
				NegativePrompt: argString(args, "negative"),
				Sampler:        argString(args, "sampler"),
				Steps:          int(argInt(args, "steps")),
				CfgScale:       argFloat(args, "cfg_scale"),
				Width:          int(argInt(args, "width")),
				Height:         int(argInt(args, "height")),
				Seed:           optInt(args, "seed"),
			}
			if tp.Width <= 0 {
				tp.Width = 512
			}
			if tp.Height <= 0 {
				tp.Height = 512
			}
			if rid := optInt(args, "resolution_id"); rid != nil && *rid > 0 {
				tp.ResolutionID = rid
			}
			items, err := s.deps.Gen.TestGenerate(tp)
			if err != nil {
				return "", err
			}
			if len(items) == 0 || items[0].Error != "" {
				msg := "генерация не удалась"
				if len(items) > 0 {
					msg = items[0].Error
				}
				return "", fmt.Errorf("%s", msg)
			}
			item := items[0]
			info := item.Info
			if len(info) == 0 {
				steps := tp.Steps
				if steps <= 0 {
					steps = 20
				}
				info, _ = json.Marshal(map[string]any{
					"prompt":          tp.Prompt,
					"negative_prompt": tp.NegativePrompt,
					"sampler_name":    item.Sampler,
					"scheduler":       item.ScheduleType,
					"seed":            item.Seed,
					"steps":           steps,
					"cfg_scale":       item.CfgScale,
					"width":           tp.Width,
					"height":          tp.Height,
					"sd_model_name":   item.ModelName,
				})
			}
			out := map[string]any{
				"seed":   item.Seed,
				"prompt": tp.Prompt,
				"model":  item.ModelName,
			}
			if save {
				path, err := s.saveArtifact(model, "gen-"+slugify(model), item.Image, info)
				if err != nil {
					return "", err
				}
				out["path"] = path
			} else {
				out["image_base64"] = item.Image
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "generate_best_of",
		Description: "Веер best-of-N по пресету: description (опционально) конвертируется LLM один раз, затем n генераций с сидами base_seed+0..n-1 в полном размере (без preview-уменьшения). " +
			"Без description — веер чистого промпта пресета. Не используй пресеты с batch_size>1 (лишние картинки). " +
			"PNG + sidecar каждому, контактный лист bestof-{preset}-sheet.png. Ошибки отдельных картинок не роняют веер.",
		InputSchema: props(map[string]any{
			"preset_id":     prop("id пресета", "integer"),
			"description":   prop("описание сцены для LLM-конвертации", "string"),
			"negative":      prop("пользовательский негатив", "string"),
			"n":             prop("число картинок 1–12 (по умолчанию 4)", "integer"),
			"base_seed":     prop("базовый сид веера (по умолчанию случайный)", "integer"),
			"resolution_id": prop("id разрешения", "integer"),
		}, "preset_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			presetID := argInt(args, "preset_id")
			if presetID <= 0 {
				return "", fmt.Errorf("preset_id обязателен")
			}
			p, err := s.deps.DB.Get(presetID)
			if err != nil {
				return "", fmt.Errorf("preset not found: %w", err)
			}
			description := argString(args, "description")
			negative := argString(args, "negative")
			extraPrompt, extraNeg := "", ""
			if description != "" || negative != "" {
				pr, err := s.deps.Gen.GenerateSDPrompt(generation.GenerateSDPromptParams{
					PresetID:    presetID,
					Description: description,
					Negative:    negative,
				})
				if err != nil {
					return "", err
				}
				extraPrompt, extraNeg = pr.Prompt, pr.NegativePrompt
			}

			n := int(argInt(args, "n"))
			if n <= 0 {
				n = 4
			}
			if n > 12 {
				n = 12
			}
			base := argInt(args, "base_seed")
			if base == 0 {
				base = time.Now().UnixMilli() % 1_000_000_000
			}
			var resolutionID *int64
			if rid := optInt(args, "resolution_id"); rid != nil && *rid > 0 {
				resolutionID = rid
			}

			type fanItem struct {
				Path  string `json:"path,omitempty"`
				Seed  int64  `json:"seed"`
				Error string `json:"error,omitempty"`
			}
			images := make([]fanItem, 0, n)
			var paths, labels []string
			var lastEffPrompt, lastEffNeg string
			for i := 0; i < n; i++ {
				seed := base + int64(i)
				gp := generation.GenerateImageParams{
					PresetID:            presetID,
					ExtraPrompt:         extraPrompt,
					ExtraNegativePrompt: extraNeg,
					Seed:                &seed,
					ResolutionID:        resolutionID,
				}
				res, err := s.deps.Gen.GenerateImage(gp)
				if err != nil {
					images = append(images, fanItem{Seed: seed, Error: err.Error()})
					continue
				}
				actual := infoSeed(res.Info)
				if actual == 0 {
					actual = seed
				}
				path, err := s.saveArtifact(p.Name, fmt.Sprintf("bestof-%s-%d", slugify(p.Name), actual), res.Image, res.Info)
				if err != nil {
					images = append(images, fanItem{Seed: actual, Error: err.Error()})
					continue
				}
				images = append(images, fanItem{Path: path, Seed: actual})
				paths = append(paths, path)
				labels = append(labels, fmt.Sprintf("seed %d", actual))
				lastEffPrompt, lastEffNeg = res.EffectivePrompt, res.EffectiveNegativePrompt
			}

			if len(paths) == 0 {
				firstErr := "unknown"
				if len(images) > 0 && images[0].Error != "" {
					firstErr = images[0].Error
				}
				return "", fmt.Errorf("все %d генераций не удались: %s", n, firstErr)
			}
			usedPrompt, usedNeg := lastEffPrompt, lastEffNeg
			out := map[string]any{
				"images":          images,
				"prompt":          usedPrompt,
				"negative_prompt": usedNeg,
			}
			sheet := filepath.Join(s.deps.OutDir, fmt.Sprintf("bestof-%s-sheet.png", slugify(p.Name)))
			if err := buildContactSheet(paths, labels, sheet); err != nil {
				out["sheet_error"] = err.Error()
			} else {
				out["sheet"] = sheet
			}
			return toJSON(out), nil
		},
	})
}
