package stylist

import (
	"errors"
	"fmt"
	"math"

	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

const plannerGiB = 1 << 30
const plannerVRAMReserve = 2 * plannerGiB
const plannerRAMReserve = 2 * plannerGiB
const verifiedPlannerModel = "huihui_ai/qwen2.5-abliterate:14b"
const incompatiblePlannerModel = "qwen3:4b"

var ErrPlannerCapacity = errors.New("нет установленной текстовой модели, которая безопасно помещается в свободную память; освободите видеопамять")
var ErrPlannerNoCandidate = errors.New("для автоподбора нужна установленная совместимая текстовая модель; модель описания фотографий и Qwen3:4b не используются для планирования")

type PlannerCapacity struct {
	FreeVRAM float64
	FreeRAM  float64
	Loaded   []llm.LoadedModel
}

func ResolvePlannerModel(configured string, list func() ([]llm.LLMModel, error)) (string, error) {
	return resolvePlannerModel(configured, list, nil)
}

func ResolvePlannerModelWithCapacity(configured string, list func() ([]llm.LLMModel, error), probe func() (PlannerCapacity, error)) (string, error) {
	if probe == nil {
		probe = func() (PlannerCapacity, error) {
			return PlannerCapacity{}, errors.New("данные памяти недоступны")
		}
	}
	return resolvePlannerModel(configured, list, probe)
}

func resolvePlannerModel(configured string, list func() ([]llm.LLMModel, error), probe func() (PlannerCapacity, error)) (string, error) {
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return "", err
	}
	replaceConfigured := configured == incompatiblePlannerModel
	for _, model := range catalog.LLMVision {
		if model.Name == configured && model.CaptionOnly {
			replaceConfigured = true
			break
		}
	}
	if !replaceConfigured {
		return configured, nil
	}
	models, err := list()
	if err != nil {
		return "", fmt.Errorf("не удалось получить текстовые модели для плана: %w", err)
	}
	installed := make(map[string]bool, len(models))
	for _, model := range models {
		installed[model.ID] = true
	}
	available := false
	for _, candidate := range catalog.LLMGenerate {
		if candidate.Name != incompatiblePlannerModel && !candidate.CaptionOnly && installed[candidate.Name] && candidate.SizeGB > 0 && !math.IsNaN(candidate.SizeGB) && !math.IsInf(candidate.SizeGB, 0) {
			available = true
			break
		}
	}
	if !available {
		return "", ErrPlannerNoCandidate
	}
	var capacity PlannerCapacity
	if probe != nil {
		capacity, err = probe()
		if err != nil {
			return "", fmt.Errorf("не удалось проверить память для ИИ плана: %w", err)
		}
		if capacity.FreeVRAM <= 0 || capacity.FreeRAM < plannerRAMReserve || math.IsNaN(capacity.FreeVRAM) || math.IsInf(capacity.FreeVRAM, 0) || math.IsNaN(capacity.FreeRAM) || math.IsInf(capacity.FreeRAM, 0) {
			return "", errors.New("недостаточно свободной памяти для автоматической загрузки ИИ плана; освободите память или выберите лёгкую текстовую модель")
		}
	}
	var candidates []serverclient.CatalogModel
	for _, candidate := range catalog.LLMGenerate {
		if candidate.Name == verifiedPlannerModel {
			candidates = append(candidates, candidate)
		}
	}
	for _, recommended := range []bool{true, false} {
		for _, candidate := range catalog.LLMGenerate {
			if candidate.Name != verifiedPlannerModel && candidate.Recommended == recommended {
				candidates = append(candidates, candidate)
			}
		}
	}
	for _, candidate := range candidates {
		if candidate.Name != incompatiblePlannerModel && !candidate.CaptionOnly && installed[candidate.Name] {
			if probe != nil {
				credited := float64(0)
				for _, loaded := range capacity.Loaded {
					if loaded.Name == candidate.Name && loaded.SizeVRAM > 0 {
						credited = max(credited, float64(loaded.SizeVRAM))
					}
				}
				if candidate.SizeGB <= 0 || math.IsNaN(candidate.SizeGB) || math.IsInf(candidate.SizeGB, 0) || capacity.FreeVRAM+credited < candidate.SizeGB*plannerGiB+plannerVRAMReserve {
					continue
				}
			}
			return candidate.Name, nil
		}
	}
	if probe != nil {
		return "", ErrPlannerCapacity
	}
	return "", ErrPlannerNoCandidate
}
