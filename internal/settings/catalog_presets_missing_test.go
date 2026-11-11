package settings

import (
	"encoding/json"
	"testing"

	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catalogPresetCounts(t *testing.T) map[string]int {
	t.Helper()
	cat, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	out := map[string]int{}
	for _, m := range cat.SDModels {
		if m.Presets == "" {
			continue
		}
		raw, err := serverclient.LoadCatalogPresets(m.Presets)
		require.NoError(t, err)
		var f struct {
			Presets []struct {
				Name string `json:"name"`
			} `json:"presets"`
		}
		require.NoError(t, json.Unmarshal(raw, &f))
		names := map[string]bool{}
		for _, p := range f.Presets {
			if p.Name != "" {
				names[p.Name] = true
			}
		}
		out[m.Name] = len(names)
	}
	return out
}

func emptyPresetsService(t *testing.T) *Service {
	t.Helper()
	svc, db := testService(t, nil, nil)
	_, err := db.DB().Exec(`DELETE FROM presets`)
	require.NoError(t, err)
	return svc
}

func TestCatalogPresetsMissing_KeysAreModelsWithPresets(t *testing.T) {
	svc := emptyPresetsService(t)
	got, err := svc.CatalogPresetsMissing()
	require.NoError(t, err)

	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{
		"flux3-image-klein-Q8_0", "Krea-2-Turbo-Q4_K_M", "qwen_image_2.1-Q8_0", "anima-aesthetic-v1.1", "anima-turbo-v1.1",
	}, keys)

	expected := catalogPresetCounts(t)
	assert.ElementsMatch(t, keys, func() []string {
		r := []string{}
		for k := range expected {
			r = append(r, k)
		}
		return r
	}())

	cat, err := serverclient.LoadCatalog()
	require.NoError(t, err)
	for _, m := range cat.SDModels {
		if m.Presets == "" {
			assert.NotContains(t, got, m.Name)
		}
	}
}

func TestCatalogPresetsMissing_EmptyTableEqualsFileCounts(t *testing.T) {
	svc := emptyPresetsService(t)
	got, err := svc.CatalogPresetsMissing()
	require.NoError(t, err)
	expected := catalogPresetCounts(t)
	for name, n := range expected {
		assert.Greater(t, n, 0, name)
		assert.Equal(t, n, got[name], name)
	}
}

func TestCatalogPresetsMissing_AfterInstallAnima(t *testing.T) {
	svc := emptyPresetsService(t)
	before, err := svc.CatalogPresetsMissing()
	require.NoError(t, err)

	_, err = svc.InstallCatalogPresets("anima-aesthetic-v1.1")
	require.NoError(t, err)

	after, err := svc.CatalogPresetsMissing()
	require.NoError(t, err)
	assert.Equal(t, 0, after["anima-aesthetic-v1.1"])
	for k, v := range before {
		if k == "anima-aesthetic-v1.1" {
			continue
		}
		assert.Equal(t, v, after[k], k)
	}
	assert.Len(t, after, len(before))
}
