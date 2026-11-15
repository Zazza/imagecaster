package generation

import (
	"log"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

const fastGenerationSetting = "fast_generation"

func fastEnabled(presetMode, global string) bool {
	switch preset.NormalizeFastMode(presetMode) {
	case preset.FastModeOn:
		return true
	case preset.FastModeOff:
		return false
	}
	return global == "true"
}

func fastCacheMode(presetMode, global, model string) string {
	if !fastEnabled(presetMode, global) {
		return ""
	}
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		log.Printf("[fast] load catalog: %v", err)
		return ""
	}
	return catalog.FastCacheMode(model)
}

func (s *Service) cacheModeFor(p *preset.Preset, model string) string {
	if p == nil {
		return ""
	}
	global, err := s.db.GetSetting(fastGenerationSetting)
	if err != nil {
		log.Printf("[fast] read %s: %v", fastGenerationSetting, err)
	}
	return fastCacheMode(p.FastMode, global, model)
}
