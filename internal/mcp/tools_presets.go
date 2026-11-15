package mcp

import (
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/preset"
)

func registerPresetTools(s *Server) {
	s.Register(Tool{
		Name:        "presets",
		Description: "Список пресетов компактными строками (id, имя, тип, теги, модель). Фильтры: query — подстрока в имени или тегах, tag — точный тег, type — тип пресета, limit (по умолчанию 50).",
		InputSchema: props(map[string]any{
			"query": prop("слова через пробел; каждое должно входить в имя или теги (AND); короткие запросы точнее длинных фраз", "string"),
			"tag":   prop("точный тег из tags", "string"),
			"type":  prop("тип пресета (preset_type)", "string"),
			"limit": prop("максимум строк (по умолчанию 50)", "integer"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			items, err := s.deps.DB.List()
			if err != nil {
				return "", err
			}
			queryWords := strings.Fields(strings.ToLower(argString(args, "query")))
			tag := argString(args, "tag")
			ptype := argString(args, "type")
			limit := int(argInt(args, "limit"))
			if limit <= 0 {
				limit = 50
			}
			lines := make([]string, 0, 16)
			for _, p := range items {
				if len(queryWords) > 0 {
					lowerName := strings.ToLower(p.Name)
					lowerTags := strings.ToLower(p.Tags)
					matched := true
					for _, w := range queryWords {
						if !strings.Contains(lowerName, w) && !strings.Contains(lowerTags, w) {
							matched = false
							break
						}
					}
					if !matched {
						continue
					}
				}
				if tag != "" && !hasTag(p.Tags, tag) {
					continue
				}
				if ptype != "" && !strings.EqualFold(p.PresetType, ptype) {
					continue
				}
				line := fmt.Sprintf("%d  %s", p.ID, p.Name)
				if p.PresetType != "" {
					line += "  [" + p.PresetType + "]"
				}
				if p.Tags != "" {
					line += "  tags: " + p.Tags
				}
				if p.ModelName != "" {
					line += "  model: " + p.ModelName
				}
				lines = append(lines, line)
				if len(lines) >= limit {
					break
				}
			}
			if len(lines) == 0 {
				return "ничего не найдено", nil
			}
			return fmt.Sprintf("найдено %d (всего пресетов: %d)\n%s", len(lines), len(items), strings.Join(lines, "\n")), nil
		},
	})

	s.Register(Tool{
		Name:        "styles",
		Description: "Список стилей (типов пресетов) со счётчиком пресетов и примерами. Пресет = СТИЛЬ (реализм, аниме, мультфильм…), не сцена. Используй, чтобы показать пользователю выбор стилей перед генерацией.",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			types, err := s.deps.DB.ListPresetTypes()
			if err != nil {
				return "", err
			}
			items, err := s.deps.DB.List()
			if err != nil {
				return "", err
			}
			counts := map[int64]int{}
			examples := map[int64][]string{}
			untyped := 0
			for _, pr := range items {
				if pr.TypeID == nil {
					untyped++
					continue
				}
				counts[*pr.TypeID]++
				if len(examples[*pr.TypeID]) < 3 {
					examples[*pr.TypeID] = append(examples[*pr.TypeID], pr.Name)
				}
			}
			lines := make([]string, 0, len(types)+1)
			for _, t := range types {
				line := fmt.Sprintf("%d  %s (%d пресетов)", t.ID, t.Name, counts[t.ID])
				if t.Description != "" {
					line += " — " + t.Description
				}
				if ex := examples[t.ID]; len(ex) > 0 {
					line += "  примеры: " + strings.Join(ex, ", ")
				}
				lines = append(lines, line)
			}
			if untyped > 0 {
				lines = append(lines, fmt.Sprintf("без типа: %d пресетов", untyped))
			}
			if len(lines) == 0 {
				return "типов пресетов нет", nil
			}
			return strings.Join(lines, "\n"), nil
		},
	})

	s.Register(Tool{
		Name:        "preset_get",
		Description: "Полное тело пресета по id: prompt, negative_prompt, sampler, steps, cfg_scale, модель, теги, сид.",
		InputSchema: props(map[string]any{
			"id": prop("id пресета", "integer"),
		}, "id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id := argInt(args, "id")
			if id <= 0 {
				return "", fmt.Errorf("id обязателен")
			}
			p, err := s.deps.DB.Get(id)
			if err != nil {
				return "", fmt.Errorf("preset not found: %w", err)
			}
			return toJSON(p), nil
		},
	})

	s.Register(Tool{
		Name:        "preset_create",
		Description: "Создать пресет, возвращает id. Пишущий инструмент: требует confirm=true — сначала покажи пользователю имя и промпт и получи согласие. Сначала убедись, что подходящего пресета нет (presets/compound_list). Дефолты: sampler=euler_a, steps=20, cfg_scale=7.0.",
		InputSchema: props(map[string]any{
			"name":            prop("имя пресета", "string"),
			"prompt":          prop("положительный промпт (SD-теги или prose — по модели)", "string"),
			"negative_prompt": prop("негативный промпт", "string"),
			"model_name":      prop("модель SD (title из sd_meta)", "string"),
			"sampler":         prop("сэмплер sd.cpp (по умолчанию euler_a)", "string"),
			"steps":           prop("шаги (по умолчанию 20)", "integer"),
			"cfg_scale":       prop("CFG (по умолчанию 7.0)", "number"),
			"tags":            prop("теги для поиска", "array", map[string]any{"items": map[string]any{"type": "string"}}),
			"type_id":         prop("id типа пресета (preset_types из БД)", "integer"),
			"confirm":         prop("явное подтверждение пользователя на создание", "boolean"),
		}, "name", "prompt", "confirm"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			name := strings.TrimSpace(argString(args, "name"))
			prompt := strings.TrimSpace(argString(args, "prompt"))
			if name == "" || prompt == "" {
				return "", fmt.Errorf("name и prompt обязательны")
			}
			if !argBool(args, "confirm") {
				preview := prompt
				if r := []rune(preview); len(r) > 60 {
					preview = string(r[:60]) + "…"
				}
				return "", fmt.Errorf("подтвердите создание пресета %q (промпт: %s): получите согласие пользователя и повторите с confirm=true. Возможно, подходящий пресет уже есть — поищите через presets", name, preview)
			}
			p := preset.Preset{
				Name:           name,
				Prompt:         prompt,
				NegativePrompt: argString(args, "negative_prompt"),
				ModelName:      argString(args, "model_name"),
				Sampler:        argString(args, "sampler"),
				Steps:          int(argInt(args, "steps")),
				CfgScale:       argFloat(args, "cfg_scale"),
			}
			if p.Sampler == "" {
				p.Sampler = "euler_a"
			}
			if p.Steps <= 0 {
				p.Steps = 20
			}
			if p.CfgScale <= 0 {
				p.CfgScale = 7.0
			}
			if tags := argStringSlice(args, "tags"); len(tags) > 0 {
				p.Tags = strings.Join(tags, ", ")
			}
			if tid := argInt(args, "type_id"); tid > 0 {
				pt, err := s.deps.DB.GetPresetType(tid)
				if err != nil {
					return "", fmt.Errorf("type_id %d not found", tid)
				}
				p.TypeID = &tid
				p.PresetType = pt.Name
			}
			if err := s.deps.DB.Create(&p); err != nil {
				return "", err
			}
			return fmt.Sprintf("создан пресет id=%d (%s)", p.ID, p.Name), nil
		},
	})

	s.Register(Tool{
		Name:        "preset_delete",
		Description: "Удалить пресет по id. Деструктивное: требует confirm=true (спроси пользователя).",
		InputSchema: props(map[string]any{
			"id":      prop("id пресета", "integer"),
			"confirm": prop("явное подтверждение пользователя", "boolean"),
		}, "id", "confirm"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id := argInt(args, "id")
			if id <= 0 {
				return "", fmt.Errorf("id обязателен")
			}
			if !argBool(args, "confirm") {
				name := ""
				if p, err := s.deps.DB.Get(id); err == nil {
					name = p.Name
				}
				return "", fmt.Errorf("подтвердите delete: preset %d %s", id, name)
			}
			if err := s.deps.DB.Delete(id); err != nil {
				return "", err
			}
			return fmt.Sprintf("пресет %d удалён", id), nil
		},
	})
}
