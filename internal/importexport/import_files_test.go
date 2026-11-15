package importexport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeImportFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func TestPreviewImportFiles_PartialImportReportsBadFiles(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	dir := t.TempDir()
	good := writeImportFile(t, dir, "good.json", `{"version":2,"presets":[{"name":"A","preset_type":"post"},{"name":"B","preset_type":"post"}]}`)
	broken := writeImportFile(t, dir, "broken.json", `{"version":2,"presets":[`)
	oldVer := writeImportFile(t, dir, "future.json", `{"version":9,"presets":[{"name":"C"}]}`)
	empty := writeImportFile(t, dir, "empty.json", `{"version":2,"presets":[]}`)
	missing := filepath.Join(dir, "missing.json")

	preview, err := svc.PreviewImportFiles([]string{good, broken, oldVer, empty, missing})

	require.NoError(t, err)
	require.Equal(t, 2, preview.Total)
	assert.Equal(t, "A", preview.Presets[0].Name)
	assert.Equal(t, "good.json", preview.Presets[0].SourceFile)
	files := map[string]string{}
	for _, fe := range preview.Errors {
		files[fe.File] = fe.Error
	}
	assert.Len(t, files, 4)
	assert.Contains(t, files["broken.json"], "parse json")
	assert.Contains(t, files["future.json"], "unsupported version")
	assert.NotEmpty(t, files["empty.json"])
	assert.NotEmpty(t, files["missing.json"])
}

func TestPreviewImportFiles_AllBadNamesFilesInError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	dir := t.TempDir()
	broken := writeImportFile(t, dir, "broken.json", `not json`)

	preview, err := svc.PreviewImportFiles([]string{broken})

	require.Error(t, err)
	assert.Nil(t, preview)
	assert.Contains(t, err.Error(), "no presets found")
	assert.Contains(t, err.Error(), "broken.json")
}

func TestPreviewImportFiles_AllGoodHasNoErrors(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	dir := t.TempDir()
	a := writeImportFile(t, dir, "a.json", `{"version":1,"presets":[{"name":"A"}]}`)
	b := writeImportFile(t, dir, "b.json", `{"version":2,"presets":[{"name":"B"}]}`)

	preview, err := svc.PreviewImportFiles([]string{a, b})

	require.NoError(t, err)
	assert.Equal(t, 2, preview.Total)
	assert.Empty(t, preview.Errors)
}

func TestPreviewCompoundImportFiles_PartialImportReportsBadFiles(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	dir := t.TempDir()
	good := writeImportFile(t, dir, "pipes.json", `{"version":1,"pipelines":[{"name":"P","steps":[{"preset":{"name":"s"}}]}]}`)
	big := writeImportFile(t, dir, "big.json", strings.Repeat(" ", maxImportFileSize+1))

	preview, err := svc.PreviewCompoundImportFiles([]string{good, big})

	require.NoError(t, err)
	require.Equal(t, 1, preview.Total)
	assert.Equal(t, "P", preview.Pipelines[0].Name)
	require.Len(t, preview.Errors, 1)
	assert.Equal(t, "big.json", preview.Errors[0].File)
	assert.Contains(t, preview.Errors[0].Error, "too large")
}

func TestPreviewCompoundImportFiles_AllBadReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	dir := t.TempDir()
	bad := writeImportFile(t, dir, "bad.json", `{"version":2,"pipelines":[{"name":"P"}]}`)

	_, err := svc.PreviewCompoundImportFiles([]string{bad})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pipelines found")
	assert.Contains(t, err.Error(), "bad.json")
}
