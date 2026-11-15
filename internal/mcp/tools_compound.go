package mcp

import (
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/generation"
)

func registerCompoundTools(s *Server) {
	s.Register(Tool{
		Name:        "compound_list",
		Description: "Список составных пресетов (цепочек): id, имя, шаги с именами пресетов.",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			items, err := s.deps.DB.ListCompoundPresetsFull()
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "составных пресетов нет", nil
			}
			lines := make([]string, 0, len(items))
			for _, cp := range items {
				stepParts := make([]string, 0, len(cp.Steps))
				for _, st := range cp.Steps {
					name := fmt.Sprintf("preset #%d", st.PresetID)
					if st.Preset != nil {
						name = st.Preset.Name
					}
					stepParts = append(stepParts, fmt.Sprintf("%d:%s", st.StepOrder, name))
				}
				lines = append(lines, fmt.Sprintf("%d  %s  [%s]", cp.ID, cp.Name, strings.Join(stepParts, " → ")))
			}
			return strings.Join(lines, "\n"), nil
		},
	})

	s.Register(Tool{
		Name:        "compound_get",
		Description: "Составной пресет по id: полные шаги с телами пресетов и denoising_strength.",
		InputSchema: props(map[string]any{
			"id": prop("id составного пресета", "integer"),
		}, "id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id := argInt(args, "id")
			if id <= 0 {
				return "", fmt.Errorf("id обязателен")
			}
			cp, err := s.deps.DB.GetCompoundPresetFull(id)
			if err != nil {
				return "", fmt.Errorf("compound preset not found: %w", err)
			}
			return toJSON(cp), nil
		},
	})

	s.Register(Tool{
		Name: "compound_generate",
		Description: "Прогнать пайплайн (составной пресет) целиком: description сцены конвертируется LLM по стилю первого шага и проводится через все шаги цепочки (txt2img → img2img → …). Медленнее одиночной генерации. Результат: PNG + sidecar в out_dir. Пайплайны — из compound_list. " +
			"Вместо compound_preset_id можно передать steps — свою цепочку из пресетов (как режим Custom в приложении). seed фиксирует сид на каждом шаге.",
		InputSchema: props(map[string]any{
			"compound_preset_id": prop("id пайплайна из compound_list (или steps)", "integer"),
			"steps":              customStepsSchema,
			"description":        prop("сцена: что изобразить (стиль возьмётся из пресетов цепочки)", "string"),
			"negative":           prop("дополнительный негатив", "string"),
			"resolution_id":      prop("id разрешения", "integer"),
			"hires_profile_id":   prop("id hires-профиля для последнего шага", "integer"),
			"seed":               prop("фиксированный сид (на каждом шаге)", "integer"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id := argInt(args, "compound_preset_id")
			steps, err := decodeArg[[]generation.CustomStep](args, "steps")
			if err != nil {
				return "", err
			}
			gp := generation.GenerateCompoundImageParams{
				ExtraPrompt:         argString(args, "description"),
				ExtraNegativePrompt: argString(args, "negative"),
				Seed:                optInt(args, "seed"),
			}
			name := "custom"
			stepCount := 0
			switch {
			case steps != nil && len(*steps) > 0:
				if id > 0 {
					return "", fmt.Errorf("укажи либо compound_preset_id, либо steps")
				}
				if err := generation.ValidateCustomSteps(*steps); err != nil {
					return "", err
				}
				gp.Steps = *steps
				stepCount = len(*steps)
			case id > 0:
				cp, err := s.deps.DB.GetCompoundPresetFull(id)
				if err != nil {
					return "", fmt.Errorf("compound preset not found: %w", err)
				}
				gp.CompoundPresetID = id
				name = cp.Name
				stepCount = len(cp.Steps)
			default:
				return "", fmt.Errorf("нужен compound_preset_id или steps")
			}
			if rid := optInt(args, "resolution_id"); rid != nil && *rid > 0 {
				gp.ResolutionID = rid
			}
			if hid := optInt(args, "hires_profile_id"); hid != nil && *hid > 0 {
				gp.HiresProfileID = hid
			}
			res, err := s.deps.Gen.GenerateCompoundImage(gp)
			if err != nil {
				return "", err
			}
			path, err := s.saveArtifact(name, "compound-"+slugify(name), res.Image, res.Info)
			if err != nil {
				return "", err
			}
			return toJSON(map[string]any{
				"path":   path,
				"prompt": res.EffectivePrompt,
				"seed":   infoSeed(res.Info),
				"steps":  stepCount,
			}), nil
		},
	})
}
