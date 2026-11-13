package settings

import (
	"fmt"
	"slices"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

type LLMChoice struct {
	Name        string  `json:"name"`
	Size        string  `json:"size,omitempty"`
	SizeGB      float64 `json:"size_gb,omitempty"`
	Installed   bool    `json:"installed"`
	Recommended bool    `json:"recommended"`
}

type LLMSetup struct {
	Needed           bool        `json:"needed"`
	Generate         []LLMChoice `json:"generate"`
	Vision           []LLMChoice `json:"vision"`
	GenerateSelected string      `json:"generate_selected"`
	VisionSelected   string      `json:"vision_selected"`
}

func (s *Service) savedModel(key string) string {
	v, err := s.db.GetSetting(key)
	if err != nil || v == "default" {
		return ""
	}
	return v
}

func (s *Service) GetLLMSetup() (*LLMSetup, error) {
	if err := s.ensureServerClient(); err != nil {
		return nil, err
	}
	installed, err := s.serverClient.GetLLMModels()
	if err != nil {
		return &LLMSetup{Generate: []LLMChoice{}, Vision: []LLMChoice{}}, nil
	}
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return nil, err
	}

	setup := &LLMSetup{Generate: []LLMChoice{}, Vision: []LLMChoice{}}
	have := map[string]bool{}
	for _, m := range installed {
		have[m.Name] = true
		caps, err := s.serverClient.LLMCapabilities(m.Name)
		if err != nil {
			if s.log != nil {
				s.log.Warn("LLM %s capabilities: %v", m.Name, err)
			}
			caps = []string{"completion"}
		}
		if slices.Contains(caps, "embedding") && !slices.Contains(caps, "completion") {
			continue
		}
		choice := LLMChoice{Name: m.Name, Size: m.Size, Installed: true}
		choice.Recommended = recommendedIn(catalog.LLMGenerate, m.Name)
		setup.Generate = append(setup.Generate, choice)
		if slices.Contains(caps, "vision") {
			choice.Recommended = recommendedIn(catalog.LLMVision, m.Name)
			setup.Vision = append(setup.Vision, choice)
		}
	}
	setup.Generate = append(setup.Generate, catalogChoices(catalog.LLMGenerate, have)...)
	setup.Vision = append(setup.Vision, catalogChoices(catalog.LLMVision, have)...)

	generate := s.savedModel("llm_generate_model")
	analyze := s.savedModel("llm_analyze_model")
	setup.Needed = generate == "" || !have[generate] || (analyze != "" && !have[analyze])
	setup.GenerateSelected = preselect(setup.Generate, generate)
	setup.VisionSelected = preselect(setup.Vision, analyze)
	return setup, nil
}

func recommendedIn(list []serverclient.CatalogModel, name string) bool {
	for _, m := range list {
		if m.Name == name {
			return m.Recommended
		}
	}
	return false
}

func catalogChoices(list []serverclient.CatalogModel, have map[string]bool) []LLMChoice {
	out := []LLMChoice{}
	for _, m := range list {
		if have[m.Name] {
			continue
		}
		out = append(out, LLMChoice{Name: m.Name, SizeGB: m.SizeGB, Recommended: m.Recommended})
	}
	return out
}

func preselect(list []LLMChoice, saved string) string {
	anyInstalled := false
	for _, c := range list {
		if c.Installed && c.Name == saved && saved != "" {
			return saved
		}
	}
	for _, c := range list {
		if c.Installed && c.Recommended {
			return c.Name
		}
		if c.Installed {
			anyInstalled = true
		}
	}
	if anyInstalled {
		return ""
	}
	for _, c := range list {
		if c.Recommended {
			return c.Name
		}
	}
	return ""
}

func (s *Service) SaveLLMChoice(generate, vision string) error {
	if generate == "" || vision == "" {
		return fmt.Errorf("both models must be chosen")
	}
	return s.UpdateSettings(map[string]string{
		"llm_generate_model": generate,
		"llm_analyze_model":  vision,
	})
}
