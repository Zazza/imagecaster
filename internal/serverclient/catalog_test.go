package serverclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func TestCatalogHasRussianDescriptions(t *testing.T) {
	c, err := LoadCatalog()
	require.NoError(t, err)

	type entry struct{ section, name, desc, cat, catRu string }
	var all []entry
	for _, m := range c.LLMGenerate {
		all = append(all, entry{"llm_generate", m.Name, m.DescriptionRu, "", ""})
	}
	for _, m := range c.LLMVision {
		all = append(all, entry{"llm_vision", m.Name, m.DescriptionRu, "", ""})
	}
	for _, m := range c.SDModels {
		all = append(all, entry{"sd_models", m.Name, m.DescriptionRu, m.Category, m.CategoryRu})
	}
	for _, m := range c.LoRA {
		all = append(all, entry{"lora", m.Name, m.DescriptionRu, m.Category, m.CategoryRu})
	}
	require.NotEmpty(t, c.LLMGenerate)
	require.NotEmpty(t, c.LLMVision)
	require.NotEmpty(t, c.SDModels)
	require.NotEmpty(t, c.LoRA)

	for _, e := range all {
		id := e.section + "/" + e.name
		assert.NotEmpty(t, e.desc, id)
		assert.True(t, hasCyrillic(e.desc), "description_ru not russian: %s", id)
		if e.cat != "" {
			assert.NotEmpty(t, e.catRu, "category_ru: %s", id)
			assert.True(t, hasCyrillic(e.catRu), "category_ru not russian: %s", id)
		}
	}
}

var removedSDModels = []string{
	"chilloutmix_v10", "cyberrealistic_final", "jaggedBrushworksXL_v10",
	"leosamsHelloworldXL_helloworldXL70", "zavychromaxl_v100", "babesByStableYogiPony_v60FP16",
	"realismByStableYogi_ponyV3VAE", "revEnginePonyxl_v10", "fnReal25DNoobxl_v10",
	"illustriousRealismBy_v10VAE", "realismIllustriousBy_v55FP16", "veloriamixIllustrious_v20",
	"t00nBL00MXL_v10", "veteAnthroAnime_v02",
}

func TestCatalogHasNoRemovedSDModels(t *testing.T) {
	c, err := LoadCatalog()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, m := range c.SDModels {
		names[m.Name] = true
	}
	for _, r := range removedSDModels {
		assert.False(t, names[r], "removed model still in catalog: %s", r)
	}
}

func TestBuiltinPresetsDoNotUseRemovedSDModels(t *testing.T) {
	files, err := filepath.Glob("../../data/presets/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	removed := map[string]bool{}
	for _, r := range removedSDModels {
		removed[r] = true
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err, f)
		var v any
		require.NoError(t, json.Unmarshal(raw, &v), f)
		walk(v, func(name string) {
			base := name
			if ext := filepath.Ext(base); ext == ".safetensors" || ext == ".ckpt" {
				base = base[:len(base)-len(ext)]
			}
			assert.False(t, removed[base], "%s references removed model %s", f, name)
		})
	}
}

func walk(v any, fn func(string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && k == "model_name" {
				fn(s)
			}
			walk(val, fn)
		}
	case []any:
		for _, val := range x {
			walk(val, fn)
		}
	}
}
