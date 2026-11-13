package serverclient

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type qhPreset struct {
	Name       string  `json:"name"`
	PresetType string  `json:"preset_type"`
	ModelName  string  `json:"model_name"`
	CfgScale   float64 `json:"cfg_scale"`
	Prompt     string  `json:"prompt"`
}

func loadQHPresets(t *testing.T, m *CatalogSDModel) []qhPreset {
	t.Helper()
	require.NotEmpty(t, m.Presets, m.Name)
	raw, err := LoadCatalogPresets(m.Presets)
	require.NoError(t, err)
	var file struct {
		Presets []qhPreset `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &file))
	return file.Presets
}

func TestCatalog_QwenImageHasVisionModule(t *testing.T) {
	c := loadTestCatalog(t)
	m := c.FindSDModel("qwen_image_2.1-Q8_0")
	require.NotNil(t, m)
	var found *CatalogModule
	for i := range m.Modules {
		if m.Modules[i].Filename == "mmproj-Qwen3VL-8B-Instruct-F16.gguf" {
			found = &m.Modules[i]
		}
	}
	require.NotNil(t, found, "mmproj module missing")
	assert.Equal(t, "text_encoder", found.Type)
	assert.InDelta(t, 1.16, found.SizeGB, 0.01)
	assert.Contains(t, found.URL, "Qwen/Qwen3-VL-8B-Instruct-GGUF")
	assert.False(t, m.Recommended)
}

func TestCatalog_HiDreamEntry(t *testing.T) {
	c := loadTestCatalog(t)
	m := c.FindSDModel("hidream_o1_image_dev_fp8_scaled")
	require.NotNil(t, m)
	assert.InDelta(t, 8.07, m.SizeGB, 0.01)
	assert.Contains(t, m.URL, "Comfy-Org/HiDream-O1-Image")
	assert.Empty(t, m.Modules)
	assert.False(t, m.Recommended)
	assert.NotEmpty(t, m.Presets)
}

func TestCatalog_HiDreamPresets(t *testing.T) {
	c := loadTestCatalog(t)
	m := c.FindSDModel("hidream_o1_image_dev_fp8_scaled")
	require.NotNil(t, m)
	presets := loadQHPresets(t, m)
	assert.GreaterOrEqual(t, len(presets), 6)
	assert.LessOrEqual(t, len(presets), 8)
	for _, p := range presets {
		assert.InDelta(t, 1.0, p.CfgScale, 1e-9, p.Name)
		assert.NotNil(t, c.FindSDModel(p.ModelName), "%s: model %q", p.Name, p.ModelName)
		assert.Equal(t, m.Name, c.FindSDModel(p.ModelName).Name, p.Name)
		assert.NotEmpty(t, strings.TrimSpace(p.Prompt), p.Name)
	}
}

func TestCatalog_QwenEffectPresets(t *testing.T) {
	c := loadTestCatalog(t)
	m := c.FindSDModel("qwen_image_2.1-Q8_0")
	require.NotNil(t, m)
	effects := 0
	for _, p := range loadQHPresets(t, m) {
		if !strings.HasPrefix(p.PresetType, "post") {
			continue
		}
		effects++
		assert.True(t, strings.HasSuffix(p.Name, " (Qwen-Image)"), p.Name)
		mm := c.FindSDModel(p.ModelName)
		require.NotNil(t, mm, p.Name)
		assert.Equal(t, m.Name, mm.Name, p.Name)
	}
	assert.Positive(t, effects, "Qwen-Image preset set has no post_* effects")
}
