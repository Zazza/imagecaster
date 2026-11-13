package serverclient_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var newGenBases = map[string]bool{
	"Z-Image":      true,
	"Moss-Image":   true,
	"Flux.1":       true,
	"Flux.2":       true,
	"Flux.3":       true,
	"Krea-2":       true,
	"Qwen-Image":   true,
	"Chroma":       true,
	"HunyuanImage": true,
	"HiDream-O1":   true,
}

var taggedBases = map[string]bool{
	"SDXL":             true,
	"SD 1.5":           true,
	"Pony XL":          true,
	"Illustrious":      true,
	"Pony/Illustrious": true,
	"Anima":            true,
}

func checkModelFamily(name, base string) error {
	switch {
	case newGenBases[base]:
		if !config.IsNewGenModel(name) {
			return fmt.Errorf("модель %q (base %q) — нового поколения, но config.IsNewGenModel вернул false: добавь её семейство (подстроку имени) в config.NewGenModelFamilies", name, base)
		}
	case taggedBases[base]:
		if config.IsNewGenModel(name) {
			return fmt.Errorf("модель %q (base %q) — теговая, но config.IsNewGenModel вернул true: какое-то семейство из config.NewGenModelFamilies случайно совпало с её именем", name, base)
		}
	default:
		return fmt.Errorf("модель %q: неизвестный base %q. Если это модель нового поколения — добавь её семейство в config.NewGenModelFamilies и base %q в newGenBases этого теста; если теговая — добавь base в taggedBases", name, base, base)
	}
	return nil
}

func TestCatalogModelFamilies(t *testing.T) {
	c, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	require.NotEmpty(t, c.SDModels)
	for _, m := range c.SDModels {
		assert.NoError(t, checkModelFamily(m.Name, m.Base))
	}
}

func TestCheckModelFamily_UnknownBaseMessage(t *testing.T) {
	err := checkModelFamily("brand-new-model", "Mystery-9")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "brand-new-model")
	assert.Contains(t, msg, "Mystery-9")
	assert.Contains(t, msg, "config.NewGenModelFamilies")
	assert.Contains(t, msg, "newGenBases")
	assert.Contains(t, msg, "taggedBases")
}

func TestCheckModelFamily_Mismatch(t *testing.T) {
	assert.Error(t, checkModelFamily("megatron-xl", "Flux.2"), "new-gen base, name not in families")
	assert.Error(t, checkModelFamily("flux-like-sdxl", "SDXL"), "tagged base, name matches a family")
	assert.NoError(t, checkModelFamily("flux1-dev-fp8", "Flux.1"))
	assert.NoError(t, checkModelFamily("dreamshaper_8", "SD 1.5"))
}

func collectModelNames(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && k == "model_name" && s != "" {
				*out = append(*out, s)
				continue
			}
			collectModelNames(val, out)
		}
	case []any:
		for _, val := range x {
			collectModelNames(val, out)
		}
	}
}

func TestBundledPresetModelFamilies(t *testing.T) {
	c, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	baseByName := map[string]string{}
	for _, m := range c.SDModels {
		baseByName[m.Name] = m.Base
	}

	files, err := filepath.Glob("../../data/presets/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	matched := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err, f)
		var doc any
		require.NoError(t, json.Unmarshal(raw, &doc), f)
		var names []string
		collectModelNames(doc, &names)
		for _, n := range names {
			base, ok := baseByName[n]
			if !ok {
				continue
			}
			matched++
			assert.NoError(t, checkModelFamily(n, base), "preset file %s", filepath.Base(f))
		}
	}
	assert.Positive(t, matched, "at least one bundled preset uses a catalog model")
}
