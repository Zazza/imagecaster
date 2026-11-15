package importexport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
)

func TestExportImport_FastModeRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode string
		want string
	}{
		{"on", "on", "on"},
		{"off", "off", "off"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srcDB := openTestDB(t)
			src := New(srcDB, logger.New(nil))
			p := &preset.Preset{Name: "fm-" + tt.name, PresetType: "test", Prompt: "p", Sampler: "euler_a", Steps: 20, CfgScale: 7, ModelName: "flux1-dev-fp8", FastMode: tt.mode}
			require.NoError(t, srcDB.Create(p))

			data, err := src.PrepareExportData([]int64{p.ID})
			require.NoError(t, err)
			file, err := src.BuildExportFile(data)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "presets.json")
			require.NoError(t, os.WriteFile(path, file, 0o600))

			dstDB := openTestDB(t)
			dst := New(dstDB, logger.New(nil))
			items, err := dst.ParseImportFile(path)
			require.NoError(t, err)
			require.Len(t, items, 1)
			imported, err := dst.ImportItems(items)
			require.NoError(t, err)
			require.Len(t, imported, 1)
			got, err := dstDB.Get(imported[0].ID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.FastMode)
		})
	}
}

func TestImport_FastModeFromFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		field string
		want  string
	}{
		{"on", `,"fast_mode":"on"`, "on"},
		{"off", `,"fast_mode":"off"`, "off"},
		{"invalid", `,"fast_mode":"maybe"`, ""},
		{"absent in old file", ``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			svc := New(db, logger.New(nil))
			item := makePreset("imp-"+tt.name, "test", "flux1-dev-fp8", 20)
			file, err := svc.BuildExportFile([]PresetData{item})
			require.NoError(t, err)
			raw := string(file)
			if tt.field != "" {
				raw = injectAfterName(t, raw, "imp-"+tt.name, tt.field)
			}
			path := filepath.Join(t.TempDir(), "presets.json")
			require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
			items, err := svc.ParseImportFile(path)
			require.NoError(t, err)
			imported, err := svc.ImportItems(items)
			require.NoError(t, err)
			require.Len(t, imported, 1)
			got, err := db.Get(imported[0].ID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.FastMode)
		})
	}
}

func injectAfterName(t *testing.T, raw, name, field string) string {
	t.Helper()
	key := `"name":"` + name + `"`
	idx := strings.Index(raw, key)
	if idx < 0 {
		key = `"name": "` + name + `"`
		idx = strings.Index(raw, key)
	}
	require.GreaterOrEqual(t, idx, 0, raw)
	end := idx + len(key)
	return raw[:end] + field + raw[end:]
}

func TestCompoundExportImport_FastModeRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode string
	}{
		{"on", "on"},
		{"off", "off"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srcDB := openTestDB(t)
			src := New(srcDB, logger.New(nil))
			p := &preset.Preset{Name: "cfm-" + tt.name, PresetType: "test", Prompt: "p", Sampler: "euler_a", Steps: 20, CfgScale: 7, ModelName: "dreamshaper_8", FastMode: tt.mode}
			require.NoError(t, srcDB.Create(p))
			cp := &preset.CompoundPreset{Name: "pipe-" + tt.name, Steps: []preset.CompoundPresetStep{{StepOrder: 1, PresetID: p.ID, DenoisingStrength: 0.5}}}
			require.NoError(t, srcDB.CreateCompoundPreset(cp))

			data, err := src.PrepareCompoundExportData([]int64{cp.ID})
			require.NoError(t, err)
			file, err := src.BuildCompoundExportFile(data)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "compound.json")
			require.NoError(t, os.WriteFile(path, file, 0o600))

			dstDB := openTestDB(t)
			dst := New(dstDB, logger.New(nil))
			items, err := dst.ParseCompoundImportFile(path)
			require.NoError(t, err)
			result, err := dst.ImportCompoundItems(items)
			require.NoError(t, err)
			require.Len(t, result, 1)
			got, err := dstDB.GetCompoundPreset(result[0].ID)
			require.NoError(t, err)
			require.Len(t, got.Steps, 1)
			step, err := dstDB.Get(got.Steps[0].PresetID)
			require.NoError(t, err)
			assert.Equal(t, tt.mode, step.FastMode)
		})
	}
}

func TestImport_FastModeOverExistingSameStyle(t *testing.T) {
	t.Parallel()
	srcDB := openTestDB(t)
	src := New(srcDB, logger.New(nil))
	p := &preset.Preset{Name: "same", PresetType: "test", Prompt: "p", Sampler: "euler_a", Steps: 20, CfgScale: 7, ModelName: "flux1-dev-fp8", FastMode: "on"}
	require.NoError(t, srcDB.Create(p))
	data, err := src.PrepareExportData([]int64{p.ID})
	require.NoError(t, err)
	file, err := src.BuildExportFile(data)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "presets.json")
	require.NoError(t, os.WriteFile(path, file, 0o600))

	dstDB := openTestDB(t)
	existing := *p
	existing.ID = 0
	existing.FastMode = ""
	require.NoError(t, dstDB.Create(&existing))
	dst := New(dstDB, logger.New(nil))
	items, err := dst.ParseImportFile(path)
	require.NoError(t, err)
	imported, err := dst.ImportItems(items)
	require.NoError(t, err)
	require.Len(t, imported, 1)
	got, err := dstDB.Get(imported[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "on", got.FastMode)
}
