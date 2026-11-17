package serverclient

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trainBase(t *testing.T, c *Catalog, base string) CatalogTrainBase {
	t.Helper()
	for _, b := range c.Train {
		if b.Base == base {
			return b
		}
	}
	t.Fatalf("train base %q not in catalog", base)
	return CatalogTrainBase{}
}

func trainFileKeys(b CatalogTrainBase) []string {
	out := make([]string, 0, len(b.Files))
	for _, f := range b.Files {
		out = append(out, f.Type+"/"+f.Filename)
	}
	return out
}

func TestCatalog_TrainHasAllBases(t *testing.T) {
	c := loadTestCatalog(t)
	var bases []string
	for _, b := range c.Train {
		bases = append(bases, b.Base)
	}
	for _, want := range []string{"Z-Image", "Moss-Image", "SDXL", "Flux.2"} {
		assert.Contains(t, bases, want)
	}
	assert.NotContains(t, bases, "Chroma")
}

func TestCatalog_TrainBasesUnique(t *testing.T) {
	c := loadTestCatalog(t)
	seen := map[string]int{}
	for _, b := range c.Train {
		seen[b.Base]++
	}
	for base, n := range seen {
		assert.Equal(t, 1, n, base)
	}
}

func TestCatalog_TrainFilesValid(t *testing.T) {
	c := loadTestCatalog(t)
	require.NotEmpty(t, c.Train)
	for _, b := range c.Train {
		t.Run(b.Base, func(t *testing.T) {
			require.NotEmpty(t, b.Files)
			seen := map[string]bool{}
			for _, f := range b.Files {
				assert.Contains(t, []string{"sd", "text_encoder", "vae", "train"}, f.Type, f.Filename)
				assert.NotEmpty(t, f.Filename)
				assert.True(t, strings.HasPrefix(f.URL, "https://"), f.URL)
				assert.Greater(t, f.SizeGB, 0.0, f.Filename)
				key := f.Type + "/" + f.Filename
				assert.False(t, seen[key], "duplicate %s", key)
				seen[key] = true
			}
		})
	}
}

func TestCatalog_TrainExpectedFiles(t *testing.T) {
	c := loadTestCatalog(t)
	tests := []struct {
		base string
		want []string
	}{
		{"Z-Image", []string{
			"train/z_image_turbo_bf16.safetensors",
			"train/zimage_turbo_training_adapter_v2.safetensors",
			"text_encoder/qwen_3_4b.safetensors",
			"vae/ae.safetensors",
		}},
		{"Flux.2", []string{
			"sd/flux-2-klein-4b.safetensors",
			"text_encoder/qwen_3_4b.safetensors",
			"vae/flux2-klein-vae.safetensors",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			keys := trainFileKeys(trainBase(t, c, tt.base))
			for _, w := range tt.want {
				assert.Contains(t, keys, w)
			}
		})
	}
}

func TestCatalog_TrainSDXLAndMossHaveModel(t *testing.T) {
	c := loadTestCatalog(t)
	for _, base := range []string{"SDXL", "Moss-Image"} {
		t.Run(base, func(t *testing.T) {
			b := trainBase(t, c, base)
			n := 0
			for _, f := range b.Files {
				if f.Type == "sd" {
					n++
				}
			}
			assert.GreaterOrEqual(t, n, 1)
		})
	}
	keys := trainFileKeys(trainBase(t, c, "SDXL"))
	assert.Contains(t, keys, "sd/juggernautXL_ragnarokBy.safetensors")
}

func TestLoadCatalog_TrainIsIndependentCopy(t *testing.T) {
	first, err := LoadCatalog()
	require.NoError(t, err)
	require.NotEmpty(t, first.Train)
	require.NotEmpty(t, first.Train[0].Files)

	base := first.Train[0].Base
	file := first.Train[0].Files[0]
	first.Train[0].Base = "mutated"
	first.Train[0].Files[0].Filename = "mutated"
	first.Train[0].Files[0].URL = "mutated"
	first.Train[0].Files = append(first.Train[0].Files, CatalogModule{Type: "sd", Filename: "extra"})

	second, err := LoadCatalog()
	require.NoError(t, err)
	require.NotEmpty(t, second.Train)
	assert.Equal(t, base, second.Train[0].Base)
	assert.Equal(t, file, second.Train[0].Files[0])
	for _, f := range second.Train[0].Files {
		assert.NotEqual(t, "extra", f.Filename)
	}
}
