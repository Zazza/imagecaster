package preset

import (
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type seedTestPreset struct {
	Name              string   `json:"name"`
	PresetType        string   `json:"preset_type"`
	TypeName          string   `json:"type_name"`
	Prompt            string   `json:"prompt"`
	Sampler           string   `json:"sampler"`
	ScheduleType      string   `json:"schedule_type"`
	Steps             int      `json:"steps"`
	CfgScale          float64  `json:"cfg_scale"`
	ModelName         string   `json:"model_name"`
	DenoisingStrength *float64 `json:"denoising_strength,omitempty"`
}

func seedFile(t *testing.T, presets ...seedTestPreset) *fstest.MapFile {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": 2, "presets": presets})
	require.NoError(t, err)
	return &fstest.MapFile{Data: raw}
}

func seedPreset(name, typ string, denoising *float64) seedTestPreset {
	return seedTestPreset{
		Name:              name,
		PresetType:        typ,
		TypeName:          typ,
		Prompt:            "prompt for " + name,
		Sampler:           "Euler",
		ScheduleType:      "Simple",
		Steps:             4,
		CfgScale:          1,
		ModelName:         "flux-2-klein-4b",
		DenoisingStrength: denoising,
	}
}

func presetsByName(t *testing.T, db *DB) map[string][]Preset {
	t.Helper()
	all, err := db.List()
	require.NoError(t, err)
	out := make(map[string][]Preset)
	for _, p := range all {
		out[p.Name] = append(out[p.Name], p)
	}
	return out
}

func TestSeedBundled_FreshDBSeedsAllFiles(t *testing.T) {
	db := testDB(t)
	fsys := fstest.MapFS{
		"data/presets/a.json": seedFile(t,
			seedPreset("Alpha One", "post", nil),
			seedPreset("Alpha Two", "post_light", nil),
		),
		"data/presets/b.json": seedFile(t,
			seedPreset("Beta Cartoon", "post_transform", floatPtr(0.85)),
			seedPreset("Beta Enhance", "post_transform", floatPtr(0.75)),
		),
	}

	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	for _, name := range []string{"Alpha One", "Alpha Two", "Beta Cartoon", "Beta Enhance"} {
		require.Len(t, got[name], 1, name)
		assert.True(t, got[name][0].IsBundled, name)
	}
	assert.Equal(t, "post_transform", got["Beta Cartoon"][0].PresetType)
}

func TestSeedBundled_StoresDenoisingStrength(t *testing.T) {
	db := testDB(t)
	fsys := fstest.MapFS{
		"data/presets/fx.json": seedFile(t,
			seedPreset("Cartoon", "post_transform", floatPtr(0.85)),
			seedPreset("Enhance", "post_transform", floatPtr(0.75)),
			seedPreset("Plain", "post", nil),
		),
	}

	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	require.Len(t, got["Cartoon"], 1)
	require.NotNil(t, got["Cartoon"][0].DenoisingStrength)
	assert.InDelta(t, 0.85, *got["Cartoon"][0].DenoisingStrength, 1e-9)

	require.Len(t, got["Enhance"], 1)
	require.NotNil(t, got["Enhance"][0].DenoisingStrength)
	assert.InDelta(t, 0.75, *got["Enhance"][0].DenoisingStrength, 1e-9)

	require.Len(t, got["Plain"], 1)
	if got["Plain"][0].DenoisingStrength != nil {
		assert.LessOrEqual(t, *got["Plain"][0].DenoisingStrength, 0.0)
	}
}

func TestSeedBundled_NewFileSeededOnExistingDB(t *testing.T) {
	db := testDB(t)
	fileA := seedFile(t,
		seedPreset("Alpha One", "post", nil),
		seedPreset("Alpha Two", "post", nil),
	)
	require.NoError(t, seedAllForTest(db, fstest.MapFS{"data/presets/a.json": fileA}))

	fsys := fstest.MapFS{
		"data/presets/a.json": fileA,
		"data/presets/b.json": seedFile(t,
			seedPreset("Beta One", "post_transform", floatPtr(0.85)),
			seedPreset("Beta Two", "post_transform", floatPtr(0.85)),
		),
	}
	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	assert.Len(t, got["Alpha One"], 1)
	assert.Len(t, got["Alpha Two"], 1)
	require.Len(t, got["Beta One"], 1)
	require.Len(t, got["Beta Two"], 1)
	assert.True(t, got["Beta One"][0].IsBundled)
	require.NotNil(t, got["Beta One"][0].DenoisingStrength)
	assert.InDelta(t, 0.85, *got["Beta One"][0].DenoisingStrength, 1e-9)
}

func TestSeedBundled_RepeatedRunDoesNotDuplicate(t *testing.T) {
	db := testDB(t)
	fsys := fstest.MapFS{
		"data/presets/a.json": seedFile(t, seedPreset("Alpha One", "post", nil)),
		"data/presets/b.json": seedFile(t, seedPreset("Beta One", "post_transform", nil)),
	}
	require.NoError(t, seedAllForTest(db, fsys))
	require.NoError(t, seedAllForTest(db, fsys))
	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	assert.Len(t, got["Alpha One"], 1)
	assert.Len(t, got["Beta One"], 1)
}

func TestSeedBundled_DeletedPresetFromSeededFileNotResurrected(t *testing.T) {
	db := testDB(t)
	fileA := seedFile(t,
		seedPreset("Alpha Keep", "post", nil),
		seedPreset("Alpha Removed", "post", nil),
	)
	require.NoError(t, seedAllForTest(db, fstest.MapFS{"data/presets/a.json": fileA}))

	before := presetsByName(t, db)
	require.Len(t, before["Alpha Removed"], 1)
	require.NoError(t, db.Delete(before["Alpha Removed"][0].ID))

	fsys := fstest.MapFS{
		"data/presets/a.json": fileA,
		"data/presets/b.json": seedFile(t, seedPreset("Beta One", "post_transform", nil)),
	}
	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	assert.Empty(t, got["Alpha Removed"])
	assert.Len(t, got["Alpha Keep"], 1)
	assert.Len(t, got["Beta One"], 1)
}

func TestSeedBundled_SkipsNameOfExistingUserPreset(t *testing.T) {
	db := testDB(t)
	fileA := seedFile(t, seedPreset("Alpha One", "post", nil))
	require.NoError(t, seedAllForTest(db, fstest.MapFS{"data/presets/a.json": fileA}))

	user := &Preset{Name: "Cartoon", PresetType: "mine", Prompt: "my own cartoon", Steps: 20, CfgScale: 7}
	require.NoError(t, db.Create(user))

	fsys := fstest.MapFS{
		"data/presets/a.json": fileA,
		"data/presets/b.json": seedFile(t,
			seedPreset("Cartoon", "post_transform", floatPtr(0.85)),
			seedPreset("Anime", "post_transform", floatPtr(0.85)),
		),
	}
	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	require.Len(t, got["Cartoon"], 1)
	assert.Equal(t, user.ID, got["Cartoon"][0].ID)
	assert.False(t, got["Cartoon"][0].IsBundled)
	assert.Equal(t, "my own cartoon", got["Cartoon"][0].Prompt)
	require.Len(t, got["Anime"], 1)
	assert.True(t, got["Anime"][0].IsBundled)
}

func TestSeedBundled_FreshDBSkipsNameOfExistingUserPreset(t *testing.T) {
	db := testDB(t)
	user := &Preset{Name: "Cartoon", PresetType: "mine", Prompt: "my own cartoon", Steps: 20, CfgScale: 7}
	require.NoError(t, db.Create(user))

	fsys := fstest.MapFS{
		"data/presets/b.json": seedFile(t,
			seedPreset("Cartoon", "post_transform", floatPtr(0.85)),
			seedPreset("Anime", "post_transform", floatPtr(0.85)),
		),
	}
	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	require.Len(t, got["Cartoon"], 1)
	assert.Equal(t, user.ID, got["Cartoon"][0].ID)
	assert.Len(t, got["Anime"], 1)
}

func TestSeedBundled_AllPresetsOfSeededFileDeletedNotResurrected(t *testing.T) {
	db := testDB(t)
	fsys := fstest.MapFS{
		"data/presets/a.json": seedFile(t,
			seedPreset("Alpha One", "post", nil),
			seedPreset("Alpha Two", "post", nil),
		),
		"data/presets/b.json": seedFile(t,
			seedPreset("Beta One", "post_transform", floatPtr(0.85)),
		),
	}
	require.NoError(t, seedAllForTest(db, fsys))

	before := presetsByName(t, db)
	require.Len(t, before["Beta One"], 1)
	betaID := before["Beta One"][0].ID
	for _, name := range []string{"Alpha One", "Alpha Two"} {
		require.Len(t, before[name], 1, name)
		require.NoError(t, db.Delete(before[name][0].ID))
	}

	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	assert.Empty(t, got["Alpha One"])
	assert.Empty(t, got["Alpha Two"])
	require.Len(t, got["Beta One"], 1)
	assert.Equal(t, betaID, got["Beta One"][0].ID)
}

func TestSeedBundled_LegacyDBNewFileSeededThenDeletedNotResurrected(t *testing.T) {
	db := testDB(t)
	for _, name := range []string{"Alpha One", "Alpha Two"} {
		_, err := db.db.Exec(`INSERT INTO presets (name, preset_type, prompt, sampler, schedule_type, steps, cfg_scale, model_name, is_bundled) VALUES (?, 'post', 'legacy', 'Euler', 'Simple', 4, 1, 'flux-2-klein-4b', 1)`, name)
		require.NoError(t, err)
	}

	fsys := fstest.MapFS{
		"data/presets/a.json": seedFile(t,
			seedPreset("Alpha One", "post", nil),
			seedPreset("Alpha Two", "post", nil),
		),
		"data/presets/b.json": seedFile(t,
			seedPreset("Beta One", "post_transform", floatPtr(0.85)),
			seedPreset("Beta Two", "post_transform", floatPtr(0.85)),
		),
	}
	require.NoError(t, seedAllForTest(db, fsys))

	got := presetsByName(t, db)
	assert.Len(t, got["Alpha One"], 1)
	assert.Len(t, got["Alpha Two"], 1)
	require.Len(t, got["Beta One"], 1)
	require.Len(t, got["Beta Two"], 1)
	assert.True(t, got["Beta One"][0].IsBundled)

	require.NoError(t, db.Delete(got["Beta One"][0].ID))
	require.NoError(t, db.Delete(got["Beta Two"][0].ID))

	require.NoError(t, seedAllForTest(db, fsys))

	after := presetsByName(t, db)
	assert.Empty(t, after["Beta One"])
	assert.Empty(t, after["Beta Two"])
	assert.Len(t, after["Alpha One"], 1)
	assert.Len(t, after["Alpha Two"], 1)
}
