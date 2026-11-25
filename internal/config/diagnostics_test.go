package config

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiagnosticsDefaultsAndStrictParsing(t *testing.T) {
	read := func(values map[string]string) DiagnosticsConfig {
		return ReadDiagnostics(func(key string) (string, error) { return values[key], nil })
	}
	require.Equal(t, DiagnosticsConfig{RestoreOriginals: true, Retention: 10}, read(nil))
	require.Equal(t, DiagnosticsConfig{VerboseLogs: true, IntermediateImages: true, Retention: 50}, read(map[string]string{"diagnostics_verbose_logs": "true", "diagnostics_intermediate_images": "true", "stylist_keep_originals": "false", "diagnostics_retention": "50"}))
	for _, invalid := range []string{"TRUE", "1", "yes", " true "} {
		got := read(map[string]string{"diagnostics_verbose_logs": invalid, "diagnostics_intermediate_images": invalid, "stylist_keep_originals": invalid})
		require.False(t, got.VerboseLogs)
		require.False(t, got.IntermediateImages)
		require.True(t, got.RestoreOriginals)
	}
	for _, invalid := range []string{"0", "51", "-1", "ten", "1.5", " 10 "} {
		require.Equal(t, 10, read(map[string]string{"diagnostics_retention": invalid}).Retention)
	}
	for _, valid := range []string{"1", "50"} {
		require.NotEqual(t, 10, read(map[string]string{"diagnostics_retention": valid}).Retention)
	}
}

func TestDiagnosticsReadFailureDisablesOptionalStorage(t *testing.T) {
	require.Equal(t, DiagnosticsConfig{RestoreOriginals: true, Retention: 10}, ReadDiagnostics(nil))
	cfg := ReadDiagnostics(func(string) (string, error) { return "", errors.New("database unavailable") })
	require.False(t, cfg.VerboseLogs)
	require.False(t, cfg.IntermediateImages)
	require.False(t, cfg.RestoreOriginals)
	require.Equal(t, 10, cfg.Retention)
}
