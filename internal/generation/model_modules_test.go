package generation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catalogModuleFilenames(t *testing.T, name string) []string {
	t.Helper()
	c, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	m := c.FindSDModel(name)
	require.NotNil(t, m, name)
	require.NotEmpty(t, m.Modules, name)
	out := make([]string, 0, len(m.Modules))
	for _, mod := range m.Modules {
		out = append(out, mod.Filename)
	}
	return out
}

func TestModulesForModel_BareFileNames(t *testing.T) {
	for _, model := range []string{"moss_image_sft_bf16", "moss_image_sft_bf16.safetensors", "flux-2-klein-4b", "flux-2-klein-4b.safetensors"} {
		t.Run(model, func(t *testing.T) {
			got := modulesForModel(model)
			require.NotEmpty(t, got)
			for _, p := range got {
				assert.Equal(t, filepath.Base(p), p)
			}
		})
	}
}

func TestModulesForModel_Flux3UsesCatalogModules(t *testing.T) {
	want := []string{"Qwen3VL-4B-Instruct-Q4_K_M.gguf", "flux2-klein-vae.safetensors"}
	for _, model := range []string{"flux3-image-klein-Q8_0", "flux3-image-klein-Q8_0.gguf"} {
		t.Run(model, func(t *testing.T) {
			got := modulesForModel(model)
			assert.ElementsMatch(t, want, got)
			assert.NotContains(t, got, kleinTEModule)
		})
	}
}

func TestModulesForModel_CatalogModels(t *testing.T) {
	for _, model := range []string{"anima-aesthetic-v1.1", "Krea-2-Turbo-Q4_K_M", "qwen_image_2.1-Q8_0"} {
		t.Run(model, func(t *testing.T) {
			want := []string{}
			for _, f := range catalogModuleFilenames(t, model) {
				if !strings.Contains(f, "mmproj") {
					want = append(want, f)
				}
			}
			assert.ElementsMatch(t, want, modulesForModel(model))
		})
	}
}

func TestModulesForModel_SingleFileOrUnknown(t *testing.T) {
	for _, model := range []string{"dreamshaper_8", "dreamshaper_8.safetensors", "unknown_model_xyz", ""} {
		t.Run(model, func(t *testing.T) {
			got := modulesForModel(model)
			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}
