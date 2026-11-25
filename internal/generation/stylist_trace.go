package generation

import (
	"log"
	"path/filepath"

	"github.com/Zazza/imagecaster/internal/stylisttrace"
)

func (s *Service) NewStylistTrace() *stylisttrace.Recorder {
	if s.dataDir == "" || !s.sdLocal() {
		return nil
	}
	settings := s.diagnosticsConfig()
	r, err := stylisttrace.NewWithOptions(filepath.Join(s.dataDir, stylisttrace.Directory), stylisttrace.Options{
		DebugImages: settings.IntermediateImages, KeepOriginals: settings.RestoreOriginals, Retention: settings.Retention,
	})
	if err != nil {
		if s.log != nil {
			s.log.Warn("stylist diagnostics create: %v", err)
		} else {
			log.Printf("[stylist diagnostics] create: %v", err)
		}
	}
	return r
}
