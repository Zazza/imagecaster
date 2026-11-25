package config

import (
	"fmt"
	"strconv"
)

const (
	DiagnosticsVerboseLogs        = "diagnostics_verbose_logs"
	DiagnosticsIntermediateImages = "diagnostics_intermediate_images"
	StylistKeepOriginals          = "stylist_keep_originals"
	DiagnosticsRetention          = "diagnostics_retention"
	DefaultDiagnosticsRetention   = 10
	MinDiagnosticsRetention       = 1
	MaxDiagnosticsRetention       = 50
)

type DiagnosticsConfig struct {
	VerboseLogs        bool
	IntermediateImages bool
	RestoreOriginals   bool
	Retention          int
}

func ReadDiagnostics(get func(string) (string, error)) DiagnosticsConfig {
	cfg := DiagnosticsConfig{RestoreOriginals: true, Retention: DefaultDiagnosticsRetention}
	if get == nil {
		return cfg
	}
	readFlag := func(key string, fallback bool) bool {
		value, err := get(key)
		if err != nil {
			return false
		}
		if value != "true" && value != "false" {
			return fallback
		}
		return value == "true"
	}
	cfg.VerboseLogs = readFlag(DiagnosticsVerboseLogs, false)
	cfg.IntermediateImages = readFlag(DiagnosticsIntermediateImages, false)
	cfg.RestoreOriginals = readFlag(StylistKeepOriginals, true)
	value, err := get(DiagnosticsRetention)
	if n, e := strconv.Atoi(value); err == nil && e == nil && n >= MinDiagnosticsRetention && n <= MaxDiagnosticsRetention {
		cfg.Retention = n
	}
	return cfg
}

func ValidateDiagnostics(data map[string]string) error {
	for _, key := range []string{DiagnosticsVerboseLogs, DiagnosticsIntermediateImages, StylistKeepOriginals} {
		if v, ok := data[key]; ok && v != "true" && v != "false" {
			return fmt.Errorf("invalid %s: expected true or false", key)
		}
	}
	if v, ok := data[DiagnosticsRetention]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < MinDiagnosticsRetention || n > MaxDiagnosticsRetention {
			return fmt.Errorf("invalid %s: expected %d..%d", DiagnosticsRetention, MinDiagnosticsRetention, MaxDiagnosticsRetention)
		}
	}
	return nil
}
