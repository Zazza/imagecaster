package mcp

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/stylist"
)

func (s *Server) stylistOptions(person string) ([]stylist.ShootOption, error) {
	if err := s.localOnly("автоподбор стилиста"); err != nil {
		return nil, err
	}
	if s.deps.DB == nil || s.deps.People == nil || s.deps.SD == nil {
		return nil, errors.New("автоподбор: каталог, база или список людей недоступны")
	}
	all, err := allPeople(s.deps.People)
	if err != nil {
		return nil, err
	}
	models, err := s.deps.SD.GetModels()
	if err != nil {
		return nil, fmt.Errorf("каталог моделей: %w", err)
	}
	presets, err := s.deps.DB.List()
	if err != nil {
		return nil, err
	}
	compounds, err := s.deps.DB.ListCompoundPresetsFull()
	if err != nil {
		return nil, err
	}
	loras, err := s.deps.SD.GetLoRAs()
	if err != nil {
		return nil, fmt.Errorf("каталог LoRA: %w", err)
	}
	return stylist.BuildShootOptions(all, person, presets, compounds, models, loras)
}

func (s *Server) suggestStylist(req stylist.SuggestRequest) (*stylist.SuggestResult, error) {
	options, err := s.stylistOptions(req.Plan.Person)
	if err != nil {
		return nil, err
	}
	if s.deps.LLM == nil || s.deps.Gen == nil {
		return nil, errors.New("ИИ для автоподбора недоступен")
	}
	base, err := stylist.Load()
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.DB.ListStylistPresets()
	if err != nil {
		return nil, err
	}
	var custom []stylist.UserPreset
	for _, p := range rows {
		custom = append(custom, stylist.UserPreset{ID: p.ID, Aspect: stylist.Aspect(p.Aspect), Name: p.Name, Prompt: p.Prompt, PromptRu: p.PromptRu, Place: p.Place, Framing: p.Framing, Places: p.Places, Orientation: p.Orientation})
	}
	base, err = base.WithUser(custom)
	if err != nil {
		return nil, err
	}
	model, err := s.deps.Gen.StylistPlannerModel()
	if err != nil {
		return nil, err
	}
	return stylist.Suggest(llm.ForPlanning(s.deps.LLM), model, req, base, options)
}

func registerStylistAutoTools(s *Server) {
	s.Register(Tool{
		Name:        "stylist_shoot_options",
		Description: "Стилист: доступные модели, пресеты и рабочие процессы для человека. Только каталог, без генерации и чтения фотографий. id можно закрепить в target при stylist_suggest.",
		InputSchema: props(map[string]any{"person": prop("Имя из stylist_people", "string")}, "person"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			options, err := s.stylistOptions(argString(args, "person"))
			if err != nil {
				return "", err
			}
			return toJSON(options), nil
		},
	})
	s.Register(Tool{
		Name:        "stylist_suggest",
		Description: "Согласованный план съёмки тем же планировщиком, что в приложении: замысел, свет, акцент, техническая цель и серии с позами. Не генерирует изображения и не меняет план приложения. Покажи предложение пользователю до съёмки. target — необязательный закреплённый id из stylist_shoot_options; без него ИИ выбирает совместимый вариант. Ручные aspects: auto, keep (не описывать), pick (ids заготовок), custom (text). Свет и композиция находятся в series[].direction; учитывай их при последующей съёмке. Автоматическая постобработка в этот план не входит.",
		InputSchema: props(map[string]any{
			"plan": props(map[string]any{
				"person":  prop("Имя человека из stylist_people", "string"),
				"gender":  prop("man или woman, если известен", "string"),
				"wish":    prop("Желаемый результат съёмки", "string"),
				"frames":  prop("Число кадров", "integer", map[string]any{"minimum": 1, "maximum": stylist.MaxFrames}),
				"themes":  prop("Темы съёмки", "array", map[string]any{"items": map[string]any{"type": "string"}}),
				"aspects": prop("Ручные ограничения outfit/hair/makeup/background/pose: mode, ids, text", "object"),
				"garment": prop("Есть фото вещи; передай её состав в garment_outfit, без фотографии", "boolean"),
				"locale":  prop("Язык описания замысла: ru или en", "string"),
			}, "person", "frames"),
			"target":         prop("Закреплённый id из stylist_shoot_options; пусто — подобрать", "string"),
			"garment_outfit": prop("Состав одежды с фото вещи, только текст", "string"),
		}, "plan"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if err := s.localOnly("автоподбор стилиста"); err != nil {
				return "", err
			}
			raw, err := json.Marshal(args)
			if err != nil {
				return "", err
			}
			var req stylist.SuggestRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				return "", fmt.Errorf("параметры автоподбора: %w", err)
			}
			result, err := s.suggestStylist(req)
			if err != nil {
				return "", err
			}
			return toJSON(result), nil
		},
	})
}
