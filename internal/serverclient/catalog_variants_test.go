package serverclient

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var variantModelStems = []string{
	"z-image-turbo-fp8-aio",
	"moss_image_sft_bf16",
	"flux-2-klein-4b",
	"flux3-image-klein-Q8_0",
	"Krea-2-Turbo-Q4_K_M",
	"qwen_image_2.1-Q8_0",
	"flux1-dev-fp8",
	"hidream_o1_image_dev_fp8_scaled",
	"Chroma1-HD",
}

func fileStem(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func expectedVRAM(m CatalogSDModel, v CatalogVariant) float64 {
	largest := v.SizeGB
	vae := 0.0
	for _, mod := range m.Modules {
		low := strings.ToLower(mod.Filename)
		switch {
		case strings.HasSuffix(low, ".json"):
		case strings.Contains(low, "mmproj"):
		case mod.Type == "vae":
			vae += mod.SizeGB
		default:
			if mod.SizeGB > largest {
				largest = mod.SizeGB
			}
		}
	}
	return math.Round((largest+vae+1.5)*10) / 10
}

func variantModel(t *testing.T, c *Catalog, stem string) *CatalogSDModel {
	t.Helper()
	m := c.FindSDModel(stem)
	require.NotNil(t, m, stem)
	require.NotEmpty(t, m.Variants, stem)
	return m
}

func TestCatalog_VariantsValid(t *testing.T) {
	c := loadTestCatalog(t)
	for _, stem := range variantModelStems {
		t.Run(stem, func(t *testing.T) {
			m := variantModel(t, c, stem)
			ids := map[string]bool{}
			defaults := 0
			for _, v := range m.Variants {
				assert.NotEmpty(t, strings.TrimSpace(v.ID), v.Filename)
				assert.NotEmpty(t, strings.TrimSpace(v.Label), v.ID)
				assert.NotEmpty(t, v.Filename, v.ID)
				assert.True(t, strings.HasPrefix(v.URL, "https://"), v.URL)
				assert.Greater(t, v.SizeGB, 0.0, v.ID)
				assert.False(t, ids[v.ID], "duplicate variant id %q", v.ID)
				ids[v.ID] = true
				if v.Default {
					defaults++
				}
			}
			assert.Equal(t, 1, defaults)
		})
	}
}

func TestCatalog_DefaultVariantMatchesModel(t *testing.T) {
	c := loadTestCatalog(t)
	for _, stem := range variantModelStems {
		t.Run(stem, func(t *testing.T) {
			m := variantModel(t, c, stem)
			d := m.DefaultVariant()
			require.NotNil(t, d)
			assert.True(t, d.Default)
			assert.Equal(t, m.URL, d.URL)
			assert.InDelta(t, m.SizeGB, d.SizeGB, 1e-9)
			assert.Equal(t, m.MainFilename(), d.SaveAs)
			assert.Equal(t, strings.ToLower(stem), strings.ToLower(fileStem(m.MainFilename())))
		})
	}
}

func TestCatalog_ModelsWithoutVariants(t *testing.T) {
	c := loadTestCatalog(t)
	count := 0
	for _, m := range c.SDModels {
		switch m.Base {
		case "Anima", "SD 1.5", "SDXL", "Pony XL", "Pony/Illustrious":
			count++
			assert.Empty(t, m.Variants, m.Name)
			assert.Nil(t, m.DefaultVariant(), m.Name)
			assert.Nil(t, m.FitVariant(16376), m.Name)
		}
	}
	assert.Greater(t, count, 0)
}

func TestCatalog_VariantSaveAs(t *testing.T) {
	c := loadTestCatalog(t)
	for _, stem := range variantModelStems {
		t.Run(stem, func(t *testing.T) {
			m := variantModel(t, c, stem)
			base := fileStem(m.MainFilename())
			for _, v := range m.Variants {
				assert.Equal(t, base+filepath.Ext(v.Filename), v.SaveAs, v.ID)
			}
		})
	}
}

func TestCatalog_ZImageGGUFSavedUnderModelStem(t *testing.T) {
	c := loadTestCatalog(t)
	m := variantModel(t, c, "z-image-turbo-fp8-aio")
	found := false
	for _, v := range m.Variants {
		if strings.HasSuffix(strings.ToLower(v.Filename), ".gguf") {
			found = true
			assert.Equal(t, "z-image-turbo-fp8-aio.gguf", v.SaveAs, v.ID)
		}
	}
	assert.True(t, found, "Z-Image has no gguf variant")
}

func TestCatalog_VariantVRAM(t *testing.T) {
	c := loadTestCatalog(t)
	for _, stem := range variantModelStems {
		t.Run(stem, func(t *testing.T) {
			m := variantModel(t, c, stem)
			for _, v := range m.Variants {
				assert.InDelta(t, expectedVRAM(*m, v), v.VRAMGB, 0.0001, v.ID)
			}
		})
	}
}

func TestCatalog_QwenQ8VRAMIgnoresMMProj(t *testing.T) {
	c := loadTestCatalog(t)
	m := variantModel(t, c, "qwen_image_2.1-Q8_0")
	var q8 *CatalogVariant
	for i := range m.Variants {
		if math.Abs(m.Variants[i].SizeGB-7.69) < 0.005 {
			q8 = &m.Variants[i]
		}
	}
	require.NotNil(t, q8, "Qwen Q8_0 variant (7.69 GB) missing")
	te, vae, mmproj := 0.0, 0.0, 0.0
	for _, mod := range m.Modules {
		low := strings.ToLower(mod.Filename)
		switch {
		case strings.Contains(low, "mmproj"):
			mmproj = mod.SizeGB
		case mod.Type == "vae":
			vae += mod.SizeGB
		case mod.Type == "text_encoder" && !strings.HasSuffix(low, ".json"):
			te = math.Max(te, mod.SizeGB)
		}
	}
	require.Greater(t, mmproj, 0.0)
	require.Greater(t, vae, 0.0)
	want := math.Round((math.Max(7.69, te)+vae+1.5)*10) / 10
	assert.InDelta(t, want, q8.VRAMGB, 0.0001)
	withMMProj := math.Round((math.Max(7.69, te)+vae+mmproj+1.5)*10) / 10
	assert.NotEqual(t, withMMProj, q8.VRAMGB)
}

func TestCatalog_VariantVRAMRule(t *testing.T) {
	c := loadTestCatalog(t)
	moss := variantModel(t, c, "moss_image_sft_bf16")
	for _, v := range moss.Variants {
		assert.Greater(t, v.VRAMGB, v.SizeGB, v.ID)
	}
	hidream := variantModel(t, c, "hidream_o1_image_dev_fp8_scaled")
	require.Empty(t, hidream.Modules)
	for _, v := range hidream.Variants {
		assert.InDelta(t, math.Round((v.SizeGB+1.5)*10)/10, v.VRAMGB, 0.0001, v.ID)
	}
}

func moduleFiles(m *CatalogSDModel) map[string]string {
	out := map[string]string{}
	for _, mod := range m.Modules {
		out[mod.Filename] = mod.Type
	}
	return out
}

func TestCatalog_ZImageModules(t *testing.T) {
	c := loadTestCatalog(t)
	m := variantModel(t, c, "z-image-turbo-fp8-aio")
	assert.Equal(t, map[string]string{
		"Qwen3-4B-Q8_0.gguf": "text_encoder",
		"ae.safetensors":     "vae",
	}, moduleFiles(m))
	for _, mod := range m.Modules {
		assert.True(t, strings.HasPrefix(mod.URL, "https://"), mod.URL)
		assert.Greater(t, mod.SizeGB, 0.0, mod.Filename)
	}
}

func TestCatalog_Flux1DevModules(t *testing.T) {
	c := loadTestCatalog(t)
	m := variantModel(t, c, "flux1-dev-fp8")
	assert.Equal(t, map[string]string{
		"t5xxl_fp8_e4m3fn.safetensors": "text_encoder",
		"clip_l.safetensors":           "text_encoder",
		"ae.safetensors":               "vae",
	}, moduleFiles(m))
}

func TestCatalog_ChromaEntry(t *testing.T) {
	c := loadTestCatalog(t)
	m := variantModel(t, c, "Chroma1-HD")
	assert.Equal(t, "Chroma1-HD", m.Name)
	assert.Equal(t, "Chroma", m.Base)
	assert.False(t, m.Recommended)
	assert.Equal(t, "chroma1-hd.json", m.Presets)
	assert.Equal(t, map[string]string{
		"t5xxl_fp8_e4m3fn.safetensors": "text_encoder",
		"ae.safetensors":               "vae",
	}, moduleFiles(m))
	quants := map[string]bool{}
	for _, v := range m.Variants {
		for _, q := range []string{"Q4_K_S", "Q6_K", "Q8_0"} {
			if strings.Contains(v.Filename, q) {
				quants[q] = true
			}
		}
	}
	assert.Equal(t, map[string]bool{"Q4_K_S": true, "Q6_K": true, "Q8_0": true}, quants)
}

func TestCatalog_ChromaPresets(t *testing.T) {
	c := loadTestCatalog(t)
	m := variantModel(t, c, "Chroma1-HD")
	raw, err := LoadCatalogPresets(m.Presets)
	require.NoError(t, err)
	var file struct {
		Presets []struct {
			Name      string  `json:"name"`
			ModelName string  `json:"model_name"`
			CfgScale  float64 `json:"cfg_scale"`
			Sampler   string  `json:"sampler"`
			Prompt    string  `json:"prompt"`
		} `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &file))
	assert.GreaterOrEqual(t, len(file.Presets), 4)
	assert.LessOrEqual(t, len(file.Presets), 6)
	for _, p := range file.Presets {
		assert.InDelta(t, 4.0, p.CfgScale, 1e-9, p.Name)
		assert.Equal(t, "euler", p.Sampler, p.Name)
		assert.Equal(t, m.Name, p.ModelName, p.Name)
		assert.NotEmpty(t, strings.TrimSpace(p.Prompt), p.Name)
	}
}

func syntheticVariantModel() CatalogSDModel {
	return CatalogSDModel{
		Name:     "z-image-turbo-fp8-aio",
		Filename: "z-image-turbo-fp8-aio.gguf",
		Variants: []CatalogVariant{
			{ID: "q8", SizeGB: 6.58, VRAMGB: 12.7, Filename: "z_image_turbo-Q8_0.gguf"},
			{ID: "q2", SizeGB: 2.59, VRAMGB: 8.7, Filename: "z_image_turbo-Q2_K.gguf"},
			{ID: "bf16", SizeGB: 12.31, VRAMGB: 18.4, Filename: "z_image_turbo_bf16.safetensors"},
			{ID: "q4", SizeGB: 3.86, VRAMGB: 10.0, Filename: "z_image_turbo-Q4_K.gguf", Default: true},
		},
	}
}

func TestCatalogSDModel_FitVariant(t *testing.T) {
	m := syntheticVariantModel()
	tests := []struct {
		name   string
		vramMB int
		want   string
	}{
		{"16 GB card picks largest fitting", 16376, "q8"},
		{"24 GB card picks largest", 24576, "bf16"},
		{"exact boundary fits", 10240, "q4"},
		{"one MB below boundary", 10239, "q2"},
		{"too small picks smallest", 4000, "q2"},
		{"unknown vram picks default", 0, "q4"},
		{"negative vram picks default", -1, "q4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.FitVariant(tt.vramMB)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.ID)
		})
	}
}

func TestCatalogSDModel_FitVariant_NoVariants(t *testing.T) {
	m := CatalogSDModel{Name: "dreamshaper_8"}
	for _, vram := range []int{0, 4000, 16376} {
		assert.Nil(t, m.FitVariant(vram))
	}
	assert.Nil(t, m.DefaultVariant())
}

func TestCatalogSDModel_DefaultVariant(t *testing.T) {
	m := syntheticVariantModel()
	d := m.DefaultVariant()
	require.NotNil(t, d)
	assert.Equal(t, "q4", d.ID)
}

func TestCatalog_FitVariantReal(t *testing.T) {
	c := loadTestCatalog(t)
	for _, stem := range variantModelStems {
		t.Run(stem, func(t *testing.T) {
			m := variantModel(t, c, stem)
			var best, smallest *CatalogVariant
			for i := range m.Variants {
				v := &m.Variants[i]
				if smallest == nil || v.SizeGB < smallest.SizeGB {
					smallest = v
				}
				if v.VRAMGB*1024 <= 16376 && (best == nil || v.SizeGB > best.SizeGB) {
					best = v
				}
			}
			got := m.FitVariant(16376)
			require.NotNil(t, got)
			if best != nil {
				assert.InDelta(t, best.SizeGB, got.SizeGB, 1e-9)
				assert.LessOrEqual(t, got.VRAMGB*1024, 16376.0)
			} else {
				assert.InDelta(t, smallest.SizeGB, got.SizeGB, 1e-9)
			}
			low := m.FitVariant(1)
			require.NotNil(t, low)
			assert.InDelta(t, smallest.SizeGB, low.SizeGB, 1e-9)
			def := m.FitVariant(0)
			require.NotNil(t, def)
			assert.True(t, def.Default)
		})
	}
}

func TestCatalogSDModel_InstalledVariant(t *testing.T) {
	m := syntheticVariantModel()
	tests := []struct {
		name        string
		files       []ModelInfo
		wantPresent bool
		wantID      string
	}{
		{"exact size", []ModelInfo{{Name: "z-image-turbo-fp8-aio.gguf", Size: 6580000000}}, true, "q8"},
		{"within 3 percent above", []ModelInfo{{Name: "z-image-turbo-fp8-aio.gguf", Size: 6770000000}}, true, "q8"},
		{"within 3 percent below", []ModelInfo{{Name: "z-image-turbo-fp8-aio.gguf", Size: 3760000000}}, true, "q4"},
		{"other extension case-insensitive", []ModelInfo{{Name: "Z-Image-Turbo-FP8-AIO.safetensors", Size: 12310000000}}, true, "bf16"},
		{"aio size matches nothing", []ModelInfo{{Name: "z-image-turbo-fp8-aio.safetensors", Size: 10345190658}}, true, ""},
		{"just outside 3 percent", []ModelInfo{{Name: "z-image-turbo-fp8-aio.gguf", Size: 6800000000}}, true, ""},
		{"zero size", []ModelInfo{{Name: "z-image-turbo-fp8-aio.gguf", Size: 0}}, true, ""},
		{"among other files", []ModelInfo{{Name: "dreamshaper_8.safetensors", Size: 2590000000}, {Name: "z-image-turbo-fp8-aio.gguf", Size: 2590000000}}, true, "q2"},
		{"no file", []ModelInfo{{Name: "dreamshaper_8.safetensors", Size: 6580000000}}, false, ""},
		{"stem prefix is not the model", []ModelInfo{{Name: "z-image-turbo-fp8-aio-old.gguf", Size: 6580000000}}, false, ""},
		{"empty list", nil, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, present := m.InstalledVariant(tt.files)
			assert.Equal(t, tt.wantPresent, present)
			if tt.wantID == "" {
				assert.Nil(t, v)
				return
			}
			require.NotNil(t, v)
			assert.Equal(t, tt.wantID, v.ID)
		})
	}
}

func TestCatalog_InstalledVariantReal(t *testing.T) {
	c := loadTestCatalog(t)
	z := variantModel(t, c, "z-image-turbo-fp8-aio")
	v, present := z.InstalledVariant([]ModelInfo{{Name: "z-image-turbo-fp8-aio.safetensors", Size: 10345190658}})
	assert.True(t, present)
	assert.Nil(t, v)

	for _, stem := range variantModelStems {
		t.Run(stem, func(t *testing.T) {
			m := variantModel(t, c, stem)
			for _, want := range m.Variants {
				size := int64(math.Round(want.SizeGB * 1e9))
				got, ok := m.InstalledVariant([]ModelInfo{{Name: want.SaveAs, Size: size}})
				assert.True(t, ok, want.ID)
				require.NotNil(t, got, want.ID)
				assert.InDelta(t, want.SizeGB, got.SizeGB, want.SizeGB*0.03, want.ID)
			}
			got, ok := m.InstalledVariant(nil)
			assert.False(t, ok)
			assert.Nil(t, got)
		})
	}
}

func TestCatalogSDModel_InstalledVariant_NoVariants(t *testing.T) {
	m := CatalogSDModel{Name: "dreamshaper_8"}
	v, present := m.InstalledVariant([]ModelInfo{{Name: "dreamshaper_8.safetensors", Size: 2000000000}})
	assert.True(t, present)
	assert.Nil(t, v)
	v, present = m.InstalledVariant(nil)
	assert.False(t, present)
	assert.Nil(t, v)
}
