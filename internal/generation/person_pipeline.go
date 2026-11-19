package generation

import (
	"encoding/json"
	"log"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/promptutil"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

func stepPersonLora(ov *PersonOverride, base string) string {
	if ov == nil || base == "" {
		return ""
	}
	name := ov.Loras[base]
	if !personNameRe.MatchString(name) {
		return ""
	}
	return name
}

func ModelBase(model string) string {
	if model == "" {
		return ""
	}
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		log.Printf("[person] load catalog: %v", err)
		return ""
	}
	if m := catalog.FindSDModelOrVariant(model); m != nil {
		return m.Base
	}
	return ""
}

func addLoraOnce(lorasJSON, name string) string {
	var loras []preset.LoRAEntry
	if lorasJSON != "" {
		if err := json.Unmarshal([]byte(lorasJSON), &loras); err != nil {
			return lorasJSON
		}
	}
	for _, l := range loras {
		if l.Name == name {
			return lorasJSON
		}
	}
	loras = append(loras, preset.LoRAEntry{Name: name, Weight: 1})
	data, err := json.Marshal(loras)
	if err != nil {
		return lorasJSON
	}
	return string(data)
}

func applyStepPerson(p *preset.Preset, ov *PersonOverride) {
	if ov == nil || len(ov.Loras) == 0 {
		return
	}
	if name := stepPersonLora(ov, ModelBase(p.ModelName)); name != "" {
		p.Loras = addLoraOnce(p.Loras, name)
	}
}

func (s *Service) pipelineFaceSwap(ov *PersonOverride, img string) (string, string) {
	if ov == nil || !ov.FaceSwap || !personNameRe.MatchString(ov.FaceSwapName) {
		return img, ""
	}
	return s.FaceSwap(img, ov.FaceSwapName)
}

func fromImagePerson(ov *PresetOverrides) *PersonOverride {
	if ov == nil {
		return nil
	}
	return ov.Person
}

func ProseModels(models []string) bool {
	if len(models) == 0 {
		return false
	}
	for _, m := range models {
		if !promptutil.ProseBase(ModelBase(m)) {
			return false
		}
	}
	return true
}

func (s *Service) proseSteps(steps []preset.CompoundPresetStep) bool {
	models := make([]string, 0, len(steps))
	for _, st := range steps {
		p, err := s.db.Get(st.PresetID)
		if err != nil {
			return false
		}
		models = append(models, p.ModelName)
	}
	return ProseModels(models)
}
