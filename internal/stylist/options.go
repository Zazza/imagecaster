package stylist

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Zazza/imagecaster/internal/people"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

const (
	shootDescriptionMax        = 1000
	stylistPhotographyCategory = "stylist_photo"
	stylistPhotographyTypeName = "Стилист / Stylist Photography"
)

func BuildShootOptions(all []serverclient.Person, person string, presets []preset.Preset, compounds []preset.CompoundPreset, models []sd.SDModel, loras []sd.LoRA) ([]ShootOption, error) {
	key, ok := people.Find(all, person)
	if !ok {
		return nil, errors.New("человек не найден на сервере")
	}
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return nil, err
	}
	compatible := map[string]bool{}
	availableLoras := map[string]bool{}
	for _, l := range loras {
		availableLoras[strings.TrimSuffix(l.Name, ".safetensors")] = true
	}
	for _, p := range all {
		if strings.EqualFold(people.Key(p), key) && p.Lora != "" {
			if _, supported := Models[people.BaseOf(p)]; supported {
				compatible[people.BaseOf(p)] = true
				availableLoras[strings.TrimSuffix(p.Lora, ".safetensors")] = true
				availableLoras[strings.TrimSuffix(p.Name, ".safetensors")] = true
			}
		}
	}
	modelBase := func(name string) string {
		if m := catalog.FindSDModelOrVariant(name); m != nil {
			return m.Base
		}
		return ""
	}
	photoObservations := map[string]string{
		Models["Z-Image"].Model:    "Synthetic fixed-seed waist-up check without identity LoRA repeatedly cropped the face, even with framing instructions. For portrait tasks requiring a visible face prefer another compatible photo model when available; this is a limited test observation, not a universal rating.",
		Models["Moss-Image"].Model: "Synthetic fixed-seed waist-up check without identity LoRA preserved the full head, visible waist-level hands and planned clothes; skin texture was visible. Environment backgrounds still softened. Limited test observation, not a quality guarantee.",
		Models["Flux.2"].Model:     "Synthetic fixed-seed waist-up check without identity LoRA preserved the full head and visible hands after explicit framing instructions. Environmental backgrounds remained soft. Limited test observation.",
		Models["Flux.1"].Model:     "Synthetic fixed-seed waist-up check without identity LoRA needed explicit head framing and often put requested hands in pockets. Limited test observation, not a universal rating.",
		Models["SDXL"].Model:       "Synthetic fixed-seed waist-up check without identity LoRA retained head and clothes and showed readable setting details, but often hid hands in pockets and softened skin. Limited test observation.",
	}
	modelDescription := func(name string) string {
		if m := catalog.FindSDModelOrVariant(name); m != nil {
			for tested, observation := range photoObservations {
				if shootModelStem(tested) == shootModelStem(name) {
					return m.Description + ". " + observation
				}
			}
			return m.Description
		}
		return ""
	}
	installed := func(name string) bool {
		if name == "" {
			return false
		}
		stem := shootModelStem(name)
		return slices.ContainsFunc(models, func(m sd.SDModel) bool {
			return stem == shootModelStem(m.Name) || stem == shootModelStem(m.Title)
		})
	}
	lorasInstalled := func(raw string) bool {
		if raw == "" {
			return true
		}
		var entries []preset.LoRAEntry
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return false
		}
		for _, l := range entries {
			if l.Name != "" && !availableLoras[strings.TrimSuffix(l.Name, ".safetensors")] {
				return false
			}
		}
		return true
	}
	var out []ShootOption
	for base, m := range Models {
		if compatible[base] && installed(m.Model) {
			out = append(out, ShootOption{ID: "base:" + base, Name: base, Model: m.Model, ModelDescription: modelDescription(m.Model), Category: "base", Description: "Base model: " + m.Model})
		}
	}
	for _, p := range presets {
		if p.ID <= 0 || p.PresetType == "post" || strings.HasPrefix(p.PresetType, "post_") || !compatible[modelBase(p.ModelName)] || !installed(p.ModelName) || !lorasInstalled(p.Loras) {
			continue
		}
		out = append(out, ShootOption{ID: fmt.Sprintf("preset:%d", p.ID), Name: p.Name, Model: p.ModelName, ModelDescription: modelDescription(p.ModelName), Category: shootPresetCategory(p.PresetType), Profile: shootPhotoProfile(p.PresetType, p.Tags), Description: shootDescription(strings.Join([]string{p.Name, p.Tags, p.Prompt}, ". "))})
	}
	byID := make(map[int64]*preset.Preset, len(presets))
	for i := range presets {
		byID[presets[i].ID] = &presets[i]
	}
	for _, c := range compounds {
		valid, personStep := len(c.Steps) > 0, false
		var modelNames []string
		for _, s := range c.Steps {
			p := s.Preset
			if p == nil {
				p = byID[s.PresetID]
			}
			if p == nil || !installed(p.ModelName) {
				valid = false
				break
			}
			if !slices.Contains(modelNames, p.ModelName) {
				modelNames = append(modelNames, p.ModelName)
			}
			effectiveLoras := p.Loras
			if s.Loras != nil && *s.Loras != "" {
				effectiveLoras = *s.Loras
			}
			if !lorasInstalled(effectiveLoras) {
				valid = false
				break
			}
			if compatible[modelBase(p.ModelName)] {
				personStep = true
			}
		}
		if c.ID > 0 && valid && personStep {
			out = append(out, ShootOption{ID: fmt.Sprintf("pipe:%d", c.ID), Name: c.Name, Model: strings.Join(modelNames, ", "), Category: "workflow", Description: shootDescription(c.Name + ". " + c.Description)})
		}
	}
	slices.SortFunc(out, func(a, b ShootOption) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func shootPresetCategory(presetType string) string {
	if presetType == stylistPhotographyTypeName {
		return stylistPhotographyCategory
	}
	return presetType
}

func shootModelStem(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	switch strings.ToLower(filepath.Ext(name)) {
	case ".safetensors", ".ckpt", ".gguf":
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	return strings.ToLower(name)
}

func shootDescription(text string) string {
	if r := []rune(text); len(r) > shootDescriptionMax {
		return string(r[:shootDescriptionMax])
	}
	return text
}
