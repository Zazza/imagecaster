package generation

import "github.com/Zazza/imagecaster/internal/config"

func (s *Service) diagnosticsConfig() config.DiagnosticsConfig {
	if s.db == nil {
		return config.ReadDiagnostics(nil)
	}
	return config.ReadDiagnostics(s.db.GetSetting)
}

func (g *MemoryGuard) diagnosticsVerbose() bool {
	if g.db == nil {
		return false
	}
	return config.ReadDiagnostics(g.db.GetSetting).VerboseLogs
}

func (g *MemoryGuard) traceMemory(enabled bool, format string, args ...any) {
	if enabled && g.log != nil {
		g.log.Info(format, args...)
	}
}
