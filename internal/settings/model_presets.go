package settings

import (
	"io/fs"
	"sync"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

type PresetSync struct {
	Statuses []preset.PresetInstallStatus `json:"statuses"`
	Added    int                          `json:"added"`
}

func (s *Service) SetBundledPresets(fsys fs.FS) {
	s.bundledFS = fsys
}

func (s *Service) bundledSources() ([]preset.PresetSource, error) {
	if s.bundledFS == nil {
		return nil, nil
	}
	return preset.BundledSources(s.bundledFS)
}

func catalogSources() ([]preset.PresetSource, error) {
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return nil, err
	}
	var out []preset.PresetSource
	for _, m := range catalog.SDModels {
		if m.Presets == "" {
			continue
		}
		raw, err := serverclient.LoadCatalogPresets(m.Presets)
		if err != nil {
			return nil, err
		}
		out = append(out, preset.PresetSource{File: "catalog/" + m.Presets, Raw: raw})
	}
	return out, nil
}

func (s *Service) presetSources() ([]preset.PresetSource, error) {
	bundled, err := s.bundledSources()
	if err != nil {
		return nil, err
	}
	catalog, err := catalogSources()
	if err != nil {
		return nil, err
	}
	return append(bundled, catalog...), nil
}

func (s *Service) MigratePresetInstallLog() error {
	sources, err := s.presetSources()
	if err != nil {
		return err
	}
	return s.db.MigratePresetInstallLog(sources)
}

func (s *Service) serverModelNames(modelType string) []string {
	if s.ensureServerClient() != nil {
		return s.directModelNames(modelType)
	}
	items, err := s.GetServerModels(modelType)
	if err != nil {
		s.log.Warn("presets: list server %s models: %s", modelType, err)
		return nil
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return names
}

func (s *Service) directModelNames(modelType string) []string {
	if s.sd == nil {
		return nil
	}
	var names []string
	switch modelType {
	case "sd":
		items, err := s.sd.GetModels()
		if err != nil {
			s.log.Warn("presets: list SD models: %s", err)
			return nil
		}
		names = make([]string, 0, len(items))
		for _, it := range items {
			names = append(names, it.Name)
		}
	case "lora":
		items, err := s.sd.GetLoRAs()
		if err != nil {
			s.log.Warn("presets: list SD LoRAs: %s", err)
			return nil
		}
		names = make([]string, 0, len(items))
		for _, it := range items {
			names = append(names, it.Name)
		}
	}
	return names
}

func (s *Service) SyncPresets() (*PresetSync, error) {
	var sdModels, loraModels []string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); sdModels = s.serverModelNames("sd") }()
	go func() { defer wg.Done(); loraModels = s.serverModelNames("lora") }()
	wg.Wait()
	sources, err := s.presetSources()
	if err != nil {
		return nil, err
	}
	added, err := s.db.SyncModelPresets(sources, sdModels, loraModels)
	if err != nil {
		return nil, err
	}
	statuses, err := s.db.PresetAvailability(sdModels, loraModels)
	if err != nil {
		return nil, err
	}
	return &PresetSync{Statuses: statuses, Added: added}, nil
}

func (s *Service) ModelPresets() (map[string][]string, error) {
	sources, err := s.presetSources()
	if err != nil {
		return nil, err
	}
	return preset.ModelPresetNames(sources)
}
