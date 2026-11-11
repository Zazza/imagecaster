package serverclient

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := LoadCatalog()
	require.NoError(t, err)
	require.NotNil(t, c)
	return c
}

func TestCatalogSDModel_MainFilename(t *testing.T) {
	tests := []struct {
		name  string
		model CatalogSDModel
		want  string
	}{
		{"filename set gguf", CatalogSDModel{Name: "flux3-image-klein-Q8_0", Filename: "flux3-image-klein-Q8_0.gguf"}, "flux3-image-klein-Q8_0.gguf"},
		{"filename set differs from name", CatalogSDModel{Name: "abc", Filename: "other.safetensors"}, "other.safetensors"},
		{"filename empty", CatalogSDModel{Name: "dreamshaper_8"}, "dreamshaper_8.safetensors"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.model.MainFilename())
		})
	}
}

func TestCatalog_FindSDModel(t *testing.T) {
	c := &Catalog{SDModels: []CatalogSDModel{
		{Name: "flux3-image-klein-Q8_0", Filename: "flux3-image-klein-Q8_0.gguf"},
		{Name: "dreamshaper_8"},
		{Name: "alias-model", Filename: "real_file_v2.gguf"},
	}}
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"exact name", "dreamshaper_8", "dreamshaper_8"},
		{"name with safetensors", "dreamshaper_8.safetensors", "dreamshaper_8"},
		{"name with ckpt", "dreamshaper_8.ckpt", "dreamshaper_8"},
		{"upper case", "DREAMSHAPER_8", "dreamshaper_8"},
		{"main filename gguf", "flux3-image-klein-Q8_0.gguf", "flux3-image-klein-Q8_0"},
		{"main filename case-insensitive", "FLUX3-IMAGE-KLEIN-q8_0.GGUF", "flux3-image-klein-Q8_0"},
		{"directory prefix", "models/Stable-diffusion/dreamshaper_8.safetensors", "dreamshaper_8"},
		{"forge hash suffix", "dreamshaper_8 [abc123]", "dreamshaper_8"},
		{"forge hash suffix with extension", "dreamshaper_8.safetensors [abc123]", "dreamshaper_8"},
		{"lookup by filename differing from name", "real_file_v2.gguf", "alias-model"},
		{"lookup by filename stem", "real_file_v2", "alias-model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.FindSDModel(tt.query)
			require.NotNil(t, got, tt.query)
			assert.Equal(t, tt.want, got.Name)
		})
	}
}

func TestCatalog_FindSDModel_NotFound(t *testing.T) {
	c := &Catalog{SDModels: []CatalogSDModel{{Name: "dreamshaper_8"}}}
	for _, q := range []string{"", "unknown_model", "unknown_model.gguf", "dreamshaper"} {
		t.Run(q, func(t *testing.T) {
			assert.Nil(t, c.FindSDModel(q))
		})
	}
}

func TestCatalog_FindSDModel_EmptyCatalog(t *testing.T) {
	c := &Catalog{}
	assert.Nil(t, c.FindSDModel("dreamshaper_8"))
}

func TestLoadCatalogPresets_RejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "../catalog.json", "sub/anima.json", "sub\\anima.json", "anima", "anima.txt", "anima.json.bak"} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadCatalogPresets(name)
			assert.Error(t, err)
		})
	}
}

func TestLoadCatalogPresets_MissingFile(t *testing.T) {
	_, err := LoadCatalogPresets("does-not-exist-xyz.json")
	assert.Error(t, err)
}

type catalogPresetFile struct {
	Version int `json:"version"`
	Presets []struct {
		Name      string `json:"name"`
		Prompt    string `json:"prompt"`
		ModelName string `json:"model_name"`
	} `json:"presets"`
}

func TestCatalog_PresetFilesValid(t *testing.T) {
	c := loadTestCatalog(t)
	count := 0
	for _, m := range c.SDModels {
		if m.Presets == "" {
			continue
		}
		count++
		t.Run(m.Name, func(t *testing.T) {
			raw, err := LoadCatalogPresets(m.Presets)
			require.NoError(t, err)
			var f catalogPresetFile
			require.NoError(t, json.Unmarshal(raw, &f))
			assert.Equal(t, 2, f.Version)
			require.GreaterOrEqual(t, len(f.Presets), 3)
			seen := map[string]bool{}
			for _, p := range f.Presets {
				assert.Equal(t, m.Name, p.ModelName, p.Name)
				assert.NotEmpty(t, strings.TrimSpace(p.Name))
				assert.NotEmpty(t, strings.TrimSpace(p.Prompt), p.Name)
				assert.False(t, seen[p.Name], "duplicate preset name %q", p.Name)
				seen[p.Name] = true
			}
		})
	}
	assert.Greater(t, count, 0)
}

func TestCatalog_ModulesValid(t *testing.T) {
	c := loadTestCatalog(t)
	count := 0
	for _, m := range c.SDModels {
		if len(m.Modules) == 0 {
			continue
		}
		count++
		t.Run(m.Name, func(t *testing.T) {
			assert.NotEmpty(t, m.URL)
			types := map[string]int{}
			for _, mod := range m.Modules {
				assert.Contains(t, []string{"vae", "text_encoder"}, mod.Type)
				assert.NotEmpty(t, mod.Filename)
				assert.True(t, strings.HasPrefix(mod.URL, "https://"), mod.URL)
				if mod.Type == "text_encoder" && strings.HasSuffix(mod.Filename, ".json") {
					types["tokenizer"]++
					continue
				}
				types[mod.Type]++
			}
			assert.Equal(t, 1, types["vae"])
			assert.Equal(t, 1, types["text_encoder"])
			assert.LessOrEqual(t, types["tokenizer"], 1)
		})
	}
	assert.Greater(t, count, 0)
}

func TestCatalog_ExpectedMultiFileModels(t *testing.T) {
	c := loadTestCatalog(t)
	tests := []struct {
		name         string
		wantPresets  bool
		wantFilename string
	}{
		{"flux-2-klein-4b", false, ""},
		{"moss_image_sft_bf16", false, ""},
		{"flux3-image-klein-Q8_0", true, "flux3-image-klein-Q8_0.gguf"},
		{"Krea-2-Turbo-Q4_K_M", true, ""},
		{"qwen_image_2.1-Q8_0", true, ""},
		{"anima-aesthetic-v1.1", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found *CatalogSDModel
			for i := range c.SDModels {
				if c.SDModels[i].Name == tt.name {
					found = &c.SDModels[i]
				}
			}
			require.NotNil(t, found)
			assert.NotEmpty(t, found.Modules)
			if tt.wantPresets {
				assert.NotEmpty(t, found.Presets)
			}
			if tt.wantFilename != "" {
				assert.Equal(t, tt.wantFilename, found.Filename)
				assert.Equal(t, tt.wantFilename, found.MainFilename())
			}
			byFind := c.FindSDModel(found.MainFilename())
			require.NotNil(t, byFind)
			assert.Equal(t, tt.name, byFind.Name)
		})
	}
}

func TestCatalog_EverythingDownloadable(t *testing.T) {
	c, err := LoadCatalog()
	require.NoError(t, err)
	for _, m := range c.SDModels {
		assert.True(t, strings.HasPrefix(m.URL, "https://"), "sd model %s has no download link", m.Name)
	}
	for _, l := range c.LoRA {
		assert.True(t, strings.HasPrefix(l.URL, "https://"), "lora %s has no download link", l.Name)
	}
}
