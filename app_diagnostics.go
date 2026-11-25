package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Zazza/imagecaster/internal/stylisttrace"
)

var openDiagnosticsDirectory = func(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", path)
	case "darwin":
		command = exec.Command("open", path)
	default:
		command = exec.Command("xdg-open", path)
	}
	return command.Run()
}

func (a *App) diagnosticsRoot() (string, error) {
	if a.dataDir == "" {
		return "", errors.New("application data directory is unavailable")
	}
	root, err := filepath.Abs(a.dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, stylisttrace.Directory), nil
}

func (a *App) DiagnosticsStorage() (*stylisttrace.StorageStats, error) {
	root, err := a.diagnosticsRoot()
	if err != nil {
		return nil, err
	}
	return stylisttrace.Storage(root)
}

func (a *App) OpenDiagnosticsFolder() error {
	root, err := a.diagnosticsRoot()
	if err != nil {
		return err
	}
	path, err := stylisttrace.ExistingDirectory(root)
	if err != nil {
		return err
	}
	return openDiagnosticsDirectory(path)
}

func (a *App) ClearDiagnostics(confirmed bool) error {
	root, err := a.diagnosticsRoot()
	if err != nil {
		return err
	}
	return stylisttrace.Clear(root, confirmed)
}
