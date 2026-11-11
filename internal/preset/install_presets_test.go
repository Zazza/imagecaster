package preset

import (
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installFile(t *testing.T, presets ...seedTestPreset) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": 2, "presets": presets})
	require.NoError(t, err)
	return raw
}

func installPreset(name, typ, model string) seedTestPreset {
	return seedTestPreset{
		Name:         name,
		PresetType:   typ,
		TypeName:     typ,
		Prompt:       "prompt for " + name,
		Sampler:      "DPM++ 2M",
		ScheduleType: "Simple",
		Steps:        12,
		CfgScale:     3.5,
		ModelName:    model,
	}
}

func TestInstallPresets_InsertsAll(t *testing.T) {
	db := testDB(t)
	raw := installFile(t,
		installPreset("Install A", "install_type_a", "anima-aesthetic-v1.1"),
		installPreset("Install B", "install_type_a", "anima-aesthetic-v1.1"),
		installPreset("Install C", "install_type_a", "anima-aesthetic-v1.1"),
	)

	n, err := db.InstallPresets(raw)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	got := presetsByName(t, db)
	for _, name := range []string{"Install A", "Install B", "Install C"} {
		require.Len(t, got[name], 1, name)
		p := got[name][0]
		assert.Equal(t, "anima-aesthetic-v1.1", p.ModelName)
		assert.Equal(t, 12, p.Steps)
		assert.InDelta(t, 3.5, p.CfgScale, 1e-9)
		assert.Equal(t, "DPM++ 2M", p.Sampler)
		assert.Equal(t, "prompt for "+name, p.Prompt)
	}
}

func TestInstallPresets_SecondCallInsertsNothing(t *testing.T) {
	db := testDB(t)
	raw := installFile(t,
		installPreset("Repeat A", "repeat_type", "m1"),
		installPreset("Repeat B", "repeat_type", "m1"),
	)

	n, err := db.InstallPresets(raw)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	before, err := db.List()
	require.NoError(t, err)

	n, err = db.InstallPresets(raw)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	after, err := db.List()
	require.NoError(t, err)
	assert.Len(t, after, len(before))
	got := presetsByName(t, db)
	assert.Len(t, got["Repeat A"], 1)
	assert.Len(t, got["Repeat B"], 1)
}

func TestInstallPresets_DoesNotOverwriteUserPreset(t *testing.T) {
	db := testDB(t)
	user := &Preset{Name: "Clash", Prompt: "my own prompt", ModelName: "user_model", Steps: 30, CfgScale: 7, Sampler: "Euler a"}
	require.NoError(t, db.Create(user))

	raw := installFile(t,
		installPreset("Clash", "clash_type", "catalog_model"),
		installPreset("Fresh", "clash_type", "catalog_model"),
	)
	n, err := db.InstallPresets(raw)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got := presetsByName(t, db)
	require.Len(t, got["Clash"], 1)
	assert.Equal(t, "my own prompt", got["Clash"][0].Prompt)
	assert.Equal(t, "user_model", got["Clash"][0].ModelName)
	assert.Equal(t, 30, got["Clash"][0].Steps)
	require.Len(t, got["Fresh"], 1)
}

func TestInstallPresets_CreatesMissingType(t *testing.T) {
	db := testDB(t)
	typ := "brand_new_install_type_xyz"
	types, err := db.ListPresetTypes()
	require.NoError(t, err)
	for _, pt := range types {
		require.NotEqual(t, typ, pt.Name)
	}

	n, err := db.InstallPresets(installFile(t, installPreset("Typed", typ, "m1")))
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	types, err = db.ListPresetTypes()
	require.NoError(t, err)
	var typeID int64
	for _, pt := range types {
		if pt.Name == typ {
			typeID = pt.ID
		}
	}
	require.NotZero(t, typeID)

	got := presetsByName(t, db)
	require.Len(t, got["Typed"], 1)
	require.NotNil(t, got["Typed"][0].TypeID)
	assert.Equal(t, typeID, *got["Typed"][0].TypeID)
}

func TestInstallPresets_ExistingTypeNotDuplicated(t *testing.T) {
	db := testDB(t)
	typ := "existing_install_type_xyz"
	require.NoError(t, db.CreatePresetType(&PresetType{Name: typ}))

	_, err := db.InstallPresets(installFile(t, installPreset("Typed Existing", typ, "m1")))
	require.NoError(t, err)

	types, err := db.ListPresetTypes()
	require.NoError(t, err)
	count := 0
	for _, pt := range types {
		if pt.Name == typ {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestInstallPresets_Errors(t *testing.T) {
	wrongVersion, err := json.Marshal(map[string]any{"version": 1, "presets": []seedTestPreset{installPreset("V1", "t", "m")}})
	require.NoError(t, err)
	tests := []struct {
		name string
		raw  []byte
	}{
		{"invalid json", []byte("{not json")},
		{"empty input", []byte("")},
		{"wrong version", wrongVersion},
		{"missing version", []byte(`{"presets":[{"name":"NoVer","prompt":"p","model_name":"m"}]}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testDB(t)
			before, err := db.List()
			require.NoError(t, err)

			n, err := db.InstallPresets(tt.raw)
			assert.Error(t, err)
			assert.Equal(t, 0, n)

			after, err := db.List()
			require.NoError(t, err)
			assert.Len(t, after, len(before))
		})
	}
}

func TestInstallPresets_EmptyPresetList(t *testing.T) {
	db := testDB(t)
	n, err := db.InstallPresets(installFile(t))
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func bundledStatusByName(t *testing.T, db *DB, sd []string, name string) PresetInstallStatus {
	t.Helper()
	st, err := db.GetBundledInstallStatus(sd, nil)
	require.NoError(t, err)
	for _, s := range st {
		if s.Name == name {
			return s
		}
	}
	require.Failf(t, "status not found", "preset %q", name)
	return PresetInstallStatus{}
}

func TestGetBundledInstallStatus_GGUFCountsAsInstalled(t *testing.T) {
	db := testDB(t)
	p := installPreset("GGUF Bundled", "post", "gguf_model_xyz")
	require.NoError(t, db.SeedBundled(fstest.MapFS{"data/presets/gguf.json": seedFile(t, p)}))

	tests := []struct {
		name      string
		sd        []string
		installed bool
	}{
		{"gguf file", []string{"gguf_model_xyz.gguf"}, true},
		{"safetensors file", []string{"gguf_model_xyz.safetensors"}, true},
		{"other gguf", []string{"another_model.gguf"}, false},
		{"no models", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := bundledStatusByName(t, db, tt.sd, "GGUF Bundled")
			assert.Equal(t, tt.installed, s.Installed)
			if tt.installed {
				assert.Empty(t, s.MissingSD)
			} else {
				assert.Contains(t, s.MissingSD, "gguf_model_xyz")
			}
		})
	}
}
