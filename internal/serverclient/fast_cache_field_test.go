package serverclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_FastCacheField(t *testing.T) {
	t.Parallel()
	c, err := LoadCatalog()
	require.NoError(t, err)
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"moss", "moss_image_sft_bf16", "dbcache"},
		{"flux1", "flux1-dev-fp8", "easycache"},
		{"qwen image", "qwen_image_2.1-Q8_0", "easycache"},
		{"chroma", "Chroma1-HD", "dbcache"},
		{"sdxl", "juggernautXL_ragnarokBy", ""},
		{"z-image", "z-image-turbo-fp8-aio", ""},
		{"klein", "flux-2-klein-4b", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var found *CatalogSDModel
			for i := range c.SDModels {
				if c.SDModels[i].Name == tt.model {
					found = &c.SDModels[i]
					break
				}
			}
			require.NotNil(t, found, tt.model)
			assert.Equal(t, tt.want, found.FastCache)
			assert.Equal(t, found.FastCache, c.FastCacheMode(tt.model))
		})
	}
}

func TestCatalog_FastCacheMode_UnknownModel(t *testing.T) {
	t.Parallel()
	c, err := LoadCatalog()
	require.NoError(t, err)
	tests := []struct {
		name  string
		model string
	}{
		{"empty", ""},
		{"not in catalog", "no_such_model_xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "", c.FastCacheMode(tt.model))
		})
	}
}

func TestCatalog_FastCacheJSON(t *testing.T) {
	t.Parallel()
	c, err := LoadCatalog()
	require.NoError(t, err)
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	var decoded struct {
		SDModels []map[string]any `json:"sd_models"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	byName := map[string]map[string]any{}
	for _, m := range decoded.SDModels {
		if n, ok := m["name"].(string); ok {
			byName[n] = m
		}
	}
	tests := []struct {
		name    string
		model   string
		want    string
		present bool
	}{
		{"moss has field", "moss_image_sft_bf16", "dbcache", true},
		{"flux1 has field", "flux1-dev-fp8", "easycache", true},
		{"sdxl omits field", "juggernautXL_ragnarokBy", "", false},
		{"klein omits field", "flux-2-klein-4b", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, ok := byName[tt.model]
			require.True(t, ok, tt.model)
			v, has := m["fast_cache"]
			assert.Equal(t, tt.present, has)
			if tt.present {
				assert.Equal(t, tt.want, v)
			}
		})
	}
}
