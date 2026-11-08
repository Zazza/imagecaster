package preset

import (
	"encoding/json"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var newMaterialNames = []string{
	"Cotton", "Linen", "Tweed", "Corduroy", "Suede",
	"Chiffon", "Tulle", "Fleece", "Shearling", "Metallic",
}

func TestSeedBundled_RealPresetsFreshDBHasNewMaterials(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.SeedBundled(os.DirFS("../..")))

	got := presetsByName(t, db)
	for _, name := range newMaterialNames {
		require.Len(t, got[name], 1, name)
		p := got[name][0]
		assert.Equal(t, "post_material", p.PresetType, name)
		assert.True(t, p.IsBundled, name)
		assert.Equal(t, "flux-2-klein-4b", p.ModelName, name)
		assert.Equal(t, "Euler", p.Sampler, name)
		assert.Equal(t, "Simple", p.ScheduleType, name)
		assert.Equal(t, 4, p.Steps, name)
		assert.InDelta(t, 1.0, p.CfgScale, 1e-9, name)
		assert.NotEmpty(t, p.Prompt, name)
	}
}

func TestSeedBundled_RealPresetsExistingEffectsDBGetsNewMaterials(t *testing.T) {
	db := testDB(t)
	raw, err := os.ReadFile("../../data/presets/effects.json")
	require.NoError(t, err)
	require.NoError(t, db.SeedBundled(fstest.MapFS{
		"data/presets/effects.json": &fstest.MapFile{Data: raw},
	}))

	var effects struct {
		Presets []struct {
			Name string `json:"name"`
		} `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &effects))
	require.NotEmpty(t, effects.Presets)

	before := presetsByName(t, db)
	for _, name := range newMaterialNames {
		require.Empty(t, before[name], name)
	}

	require.NoError(t, db.SeedBundled(os.DirFS("../..")))

	got := presetsByName(t, db)
	for _, name := range newMaterialNames {
		require.Len(t, got[name], 1, name)
		assert.Equal(t, "post_material", got[name][0].PresetType, name)
	}
	for _, p := range effects.Presets {
		require.Len(t, got[p.Name], 1, p.Name)
		require.Len(t, before[p.Name], 1, p.Name)
		assert.Equal(t, before[p.Name][0].ID, got[p.Name][0].ID, p.Name)
	}
}
