package main

import (
	"github.com/Zazza/imagecaster/internal/stylisttrace"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnosticsAppUsesDedicatedFolderAndLeavesHistoryUntouched(t *testing.T) {
	dir := t.TempDir()
	a := &App{dataDir: dir}
	history := filepath.Join(dir, "history-sentinel")
	require.NoError(t, os.WriteFile(history, []byte("history"), 0600))
	stats, err := a.DiagnosticsStorage()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, stylisttrace.Directory), stats.Path)
	require.False(t, stats.Exists)
	require.Error(t, a.ClearDiagnostics(false))
	require.NoError(t, a.ClearDiagnostics(true))
	data, err := os.ReadFile(history)
	require.NoError(t, err)
	require.Equal(t, "history", string(data))
}

func TestDiagnosticsAppOpensOnlyExistingDedicatedFolder(t *testing.T) {
	old := openDiagnosticsDirectory
	t.Cleanup(func() { openDiagnosticsDirectory = old })
	var opened []string
	openDiagnosticsDirectory = func(path string) error { opened = append(opened, path); return nil }
	a := &App{dataDir: t.TempDir()}
	require.Error(t, a.OpenDiagnosticsFolder())
	require.Empty(t, opened)
	root := filepath.Join(a.dataDir, stylisttrace.Directory)
	require.NoError(t, os.Mkdir(root, 0700))
	require.NoError(t, a.OpenDiagnosticsFolder())
	require.Equal(t, []string{root}, opened)
}

func TestDiagnosticsAppRejectsUnavailableDataDirectory(t *testing.T) {
	a := &App{}
	_, err := a.DiagnosticsStorage()
	require.Error(t, err)
	require.Error(t, a.ClearDiagnostics(true))
	require.Error(t, a.OpenDiagnosticsFolder())
}
