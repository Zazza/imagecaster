package generation

import (
	"log"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

const fastGenerationSetting = "fast_generation"

func fastCacheMode(global, model string) string {
	if global != "true" {
		return ""
	}
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		log.Printf("[fast] load catalog: %v", err)
		return ""
	}
	return catalog.FastCacheMode(model)
}

func (s *Service) cacheModeFor(model string) string {
	global, err := s.db.GetSetting(fastGenerationSetting)
	if err != nil {
		log.Printf("[fast] read %s: %v", fastGenerationSetting, err)
	}
	return fastCacheMode(global, model)
}
