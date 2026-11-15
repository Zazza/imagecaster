package importexport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zazza/imagecaster/internal/logger"
)

const maxImportFileSize = 10 * 1024 * 1024

type ImportFileError struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

var errEmptyImportFile = errors.New("no items in file")

func readImportFile(filePath string) ([]byte, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > maxImportFileSize {
		return nil, fmt.Errorf("file too large (max 10 MB)")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

func parseImportFiles[T any](log *logger.Logger, paths []string, parse func(string) ([]T, error)) ([]T, []ImportFileError) {
	var items []T
	var fileErrs []ImportFileError
	for _, p := range paths {
		parsed, err := parse(p)
		if err == nil && len(parsed) == 0 {
			err = errEmptyImportFile
		}
		if err != nil {
			log.Warn("import %s: %s", p, err)
			fileErrs = append(fileErrs, ImportFileError{File: filepath.Base(p), Error: err.Error()})
			continue
		}
		items = append(items, parsed...)
	}
	return items, fileErrs
}

func noItemsError(what string, fileErrs []ImportFileError) error {
	if len(fileErrs) == 0 {
		return fmt.Errorf("no %s found in selected files", what)
	}
	parts := make([]string, 0, len(fileErrs))
	for _, fe := range fileErrs {
		parts = append(parts, fe.File+": "+fe.Error)
	}
	return fmt.Errorf("no %s found in selected files: %s", what, strings.Join(parts, "; "))
}

func (s *Service) PreviewImportFiles(paths []string) (*ImportPreview, error) {
	items, fileErrs := parseImportFiles(s.log, paths, s.ParseImportFile)
	if len(items) == 0 {
		return nil, noItemsError("presets", fileErrs)
	}
	return &ImportPreview{Presets: items, Total: len(items), Errors: fileErrs}, nil
}

func (s *Service) PreviewCompoundImportFiles(paths []string) (*CompoundImportPreview, error) {
	items, fileErrs := parseImportFiles(s.log, paths, s.ParseCompoundImportFile)
	if len(items) == 0 {
		return nil, noItemsError("pipelines", fileErrs)
	}
	return &CompoundImportPreview{Pipelines: items, Total: len(items), Errors: fileErrs}, nil
}
