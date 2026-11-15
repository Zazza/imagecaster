package serverclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var wantCacheByBase = map[string]string{
	"Moss-Image": "dbcache",
	"Flux.1":     "easycache",
	"Qwen-Image": "easycache",
	"Chroma":     "dbcache",
}

func TestCatalog_FastCacheMode(t *testing.T) {
	t.Parallel()
	c, err := LoadCatalog()
	require.NoError(t, err)
	tests := []struct {
		model string
		want  string
	}{
		{"moss_image_sft_bf16", "dbcache"},
		{"flux1-dev-fp8", "easycache"},
		{"qwen_image_2.1-Q8_0", "easycache"},
		{"Chroma1-HD", "dbcache"},
		{"juggernautXL_ragnarokBy", ""},
		{"dreamshaper_8", ""},
		{"ponyDiffusionV6XL_v6StartWithThisOne", ""},
		{"riMixPONYIllustrious_riMixV2", ""},
		{"z-image-turbo-fp8-aio", ""},
		{"flux-2-klein-4b", ""},
		{"flux3-image-klein-Q8_0", ""},
		{"Krea-2-Turbo-Q4_K_M", ""},
		{"anima-turbo-v1.1", ""},
		{"hidream_o1_image_dev_fp8_scaled", ""},
		{"flux1-dev-fp8.gguf", "easycache"},
		{"Chroma1-HD.gguf", "dbcache"},
		{"unknown-model-xyz", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, c.FastCacheMode(tt.model))
		})
	}
}

func TestCatalog_FastCacheModeCoversEveryCatalogModel(t *testing.T) {
	t.Parallel()
	c, err := LoadCatalog()
	require.NoError(t, err)
	require.NotEmpty(t, c.SDModels)
	for _, m := range c.SDModels {
		assert.Equal(t, wantCacheByBase[m.Base], c.FastCacheMode(m.Name), "%s (%s)", m.Name, m.Base)
	}
}
