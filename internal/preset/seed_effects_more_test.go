package preset

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var emNewGroups = []string{"post_weather", "post_pattern", "post_hair", "post_background"}

var emAllowedTypes = []string{
	"post", "post_material", "post_skin", "post_light", "post_film", "post_transform",
	"post_weather", "post_pattern", "post_hair", "post_background",
}

type emFilePreset struct {
	Name         string  `json:"name"`
	PresetType   string  `json:"preset_type"`
	Prompt       string  `json:"prompt"`
	Sampler      string  `json:"sampler"`
	ScheduleType string  `json:"schedule_type"`
	Steps        int     `json:"steps"`
	CfgScale     float64 `json:"cfg_scale"`
	ModelName    string  `json:"model_name"`
}

func emReadFile(t *testing.T, path string) ([]byte, []emFilePreset) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var f struct {
		Version int            `json:"version"`
		Presets []emFilePreset `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &f))
	return raw, f.Presets
}

func TestEffectsMoreFile_Version2AndKleinParams(t *testing.T) {
	raw, err := os.ReadFile("../../data/presets/effects-more.json")
	require.NoError(t, err)
	var f struct {
		Version int            `json:"version"`
		Presets []emFilePreset `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &f))
	assert.Equal(t, 2, f.Version)
	require.NotEmpty(t, f.Presets)

	seen := map[string]bool{}
	types := map[string]int{}
	for _, p := range f.Presets {
		require.NotEmpty(t, p.Name)
		assert.False(t, seen[p.Name], "duplicate name in file: %s", p.Name)
		seen[p.Name] = true
		types[p.PresetType]++
		assert.Contains(t, emAllowedTypes, p.PresetType, p.Name)
		assert.Equal(t, "flux-2-klein-4b", p.ModelName, p.Name)
		assert.Equal(t, "Euler", p.Sampler, p.Name)
		assert.Equal(t, "Simple", p.ScheduleType, p.Name)
		assert.Equal(t, 4, p.Steps, p.Name)
		assert.InDelta(t, 1.0, p.CfgScale, 1e-9, p.Name)
		assert.NotEmpty(t, p.Prompt, p.Name)
	}
	for _, g := range emNewGroups {
		assert.Positive(t, types[g], "new group %s has no effects", g)
	}
}

func TestSeedBundled_EM1_FreshDBHasAllEffectsMoreWithTypes(t *testing.T) {
	_, file := emReadFile(t, "../../data/presets/effects-more.json")
	require.NotEmpty(t, file)

	db := testDB(t)
	require.NoError(t, seedAllForTest(db, os.DirFS("../..")))

	got := presetsByName(t, db)
	for _, fp := range file {
		require.Len(t, got[fp.Name], 1, fp.Name)
		p := got[fp.Name][0]
		assert.Equal(t, fp.PresetType, p.PresetType, fp.Name)
		assert.True(t, p.IsBundled, fp.Name)
		assert.Equal(t, "flux-2-klein-4b", p.ModelName, fp.Name)
		assert.Equal(t, "Euler", p.Sampler, fp.Name)
		assert.Equal(t, "Simple", p.ScheduleType, fp.Name)
		assert.Equal(t, 4, p.Steps, fp.Name)
		assert.InDelta(t, 1.0, p.CfgScale, 1e-9, fp.Name)
		assert.Equal(t, fp.Prompt, p.Prompt, fp.Name)
	}

	types, err := db.ListPresetTypes()
	require.NoError(t, err)
	var names []string
	for _, ty := range types {
		names = append(names, ty.Name)
	}
	for _, g := range emNewGroups {
		assert.True(t, slices.Contains(names, g), "preset type %s missing after seed", g)
	}
}

func TestSeedBundled_EM2_SeededEffectsAndMaterialsGetNewNoDuplicates(t *testing.T) {
	effRaw, eff := emReadFile(t, "../../data/presets/effects.json")
	matRaw, mat := emReadFile(t, "../../data/presets/materials.json")
	_, more := emReadFile(t, "../../data/presets/effects-more.json")
	require.NotEmpty(t, eff)
	require.NotEmpty(t, mat)
	require.NotEmpty(t, more)

	old := map[string]bool{}
	for _, p := range append(append([]emFilePreset{}, eff...), mat...) {
		old[p.Name] = true
	}
	for _, p := range more {
		require.False(t, old[p.Name], "effects-more name collides with an already bundled effect: %s", p.Name)
	}

	db := testDB(t)
	require.NoError(t, seedAllForTest(db, fstest.MapFS{
		"data/presets/effects.json":   &fstest.MapFile{Data: effRaw},
		"data/presets/materials.json": &fstest.MapFile{Data: matRaw},
	}))
	before := presetsByName(t, db)
	for _, p := range more {
		require.Empty(t, before[p.Name], p.Name)
	}

	require.NoError(t, seedAllForTest(db, os.DirFS("../..")))
	got := presetsByName(t, db)
	for _, p := range more {
		require.Len(t, got[p.Name], 1, p.Name)
		assert.Equal(t, p.PresetType, got[p.Name][0].PresetType, p.Name)
	}
	for name := range old {
		require.Len(t, before[name], 1, name)
		require.Len(t, got[name], 1, name)
		assert.Equal(t, before[name][0].ID, got[name][0].ID, name)
	}

	require.NoError(t, seedAllForTest(db, os.DirFS("../..")))
	again := presetsByName(t, db)
	for _, p := range more {
		assert.Len(t, again[p.Name], 1, "third seed duplicated %s", p.Name)
	}
}
