package preset

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const autoInstalledKey = "auto_installed_presets"

type syncTestPreset struct {
	Name         string  `json:"name"`
	PresetType   string  `json:"preset_type"`
	TypeName     string  `json:"type_name"`
	Prompt       string  `json:"prompt"`
	Sampler      string  `json:"sampler"`
	ScheduleType string  `json:"schedule_type"`
	Steps        int     `json:"steps"`
	CfgScale     float64 `json:"cfg_scale"`
	ModelName    string  `json:"model_name"`
	Loras        string  `json:"loras,omitempty"`
}

func syncPreset(name, model string) syncTestPreset {
	return syncTestPreset{
		Name:         name,
		PresetType:   "sync_type",
		TypeName:     "sync_type",
		Prompt:       "prompt for " + name,
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        8,
		CfgScale:     2,
		ModelName:    model,
	}
}

func syncPresetWithLoras(name, model string, loras ...string) syncTestPreset {
	p := syncPreset(name, model)
	entries := make([]map[string]any, 0, len(loras))
	for _, l := range loras {
		entries = append(entries, map[string]any{"name": l, "weight": 0.8})
	}
	raw, _ := json.Marshal(entries)
	p.Loras = string(raw)
	return p
}

func syncSource(t *testing.T, file string, presets ...syncTestPreset) PresetSource {
	t.Helper()
	if presets == nil {
		presets = []syncTestPreset{}
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "presets": presets})
	require.NoError(t, err)
	return PresetSource{File: file, Raw: raw}
}

func presetNames(t *testing.T, db *DB) []string {
	t.Helper()
	all, err := db.List()
	require.NoError(t, err)
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

func installLog(t *testing.T, db *DB) []string {
	t.Helper()
	v, err := db.GetSetting(autoInstalledKey)
	require.NoError(t, err)
	require.NotEmpty(t, v, "setting %s must exist", autoInstalledKey)
	var names []string
	require.NoError(t, json.Unmarshal([]byte(v), &names))
	sort.Strings(names)
	return names
}

func availabilityByName(t *testing.T, db *DB, sd, lora []string, name string) PresetInstallStatus {
	t.Helper()
	st, err := db.PresetAvailability(sd, lora)
	require.NoError(t, err)
	for _, s := range st {
		if s.Name == name {
			return s
		}
	}
	require.Failf(t, "availability not found", "preset %q", name)
	return PresetInstallStatus{}
}

func sorted(v ...string) []string {
	out := append([]string{}, v...)
	sort.Strings(out)
	return out
}

func TestSyncModelPresets_InstallsOnlyForPresentModels(t *testing.T) {
	tests := []struct {
		name     string
		sdModels []string
		want     []string
	}{
		{"safetensors present", []string{"m-alpha.safetensors"}, []string{"Alpha", "NoModel"}},
		{"gguf on server, name without extension", []string{"m-beta.gguf"}, []string{"Beta", "NoModel"}},
		{"ckpt on server", []string{"m-alpha.ckpt"}, []string{"Alpha", "NoModel"}},
		{"preset name with extension, server gguf", []string{"m-gamma.gguf"}, []string{"Gamma", "NoModel"}},
		{"all present", []string{"m-alpha.safetensors", "m-beta.gguf", "m-gamma.safetensors"}, []string{"Alpha", "Beta", "Gamma", "NoModel"}},
		{"other model only", []string{"other.safetensors"}, []string{"NoModel"}},
		{"empty list received", []string{}, []string{"NoModel"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testDB(t)
			sources := []PresetSource{
				syncSource(t, "a.json", syncPreset("Alpha", "m-alpha"), syncPreset("NoModel", "")),
				syncSource(t, "catalog/b.json", syncPreset("Beta", "m-beta"), syncPreset("Gamma", "m-gamma.safetensors")),
			}

			n, err := db.SyncModelPresets(sources, tt.sdModels, nil)
			require.NoError(t, err)
			assert.Equal(t, len(tt.want), n)
			assert.Equal(t, sorted(tt.want...), presetNames(t, db))

			all, err := db.List()
			require.NoError(t, err)
			for _, p := range all {
				assert.True(t, p.IsBundled, p.Name)
			}
		})
	}
}

func TestSyncModelPresets_CopiesPresetFields(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPresetWithLoras("Fields", "m-alpha", "lora-x"))}

	_, err := db.SyncModelPresets(sources, []string{"m-alpha.safetensors"}, []string{"lora-x.safetensors"})
	require.NoError(t, err)

	got := presetsByName(t, db)
	require.Len(t, got["Fields"], 1)
	p := got["Fields"][0]
	assert.Equal(t, "m-alpha", p.ModelName)
	assert.Equal(t, "prompt for Fields", p.Prompt)
	assert.Equal(t, 8, p.Steps)
	assert.Contains(t, p.Loras, "lora-x")
	assert.True(t, p.IsBundled)
}

func TestSyncModelPresets_NilSDModelsInstallsNothing(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("Alpha", "m-alpha"), syncPreset("NoModel", ""))}

	n, err := db.SyncModelPresets(sources, nil, []string{"lora-x.safetensors"})
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, presetNames(t, db))

	n, err = db.SyncModelPresets(sources, []string{"m-alpha.safetensors"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, []string{"Alpha", "NoModel"}, presetNames(t, db))
}

func TestSyncModelPresets_LoRANotRequiredForInstall(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPresetWithLoras("TwoLoras", "m-alpha", "lora-have", "lora-missing"))}
	sd := []string{"m-alpha.safetensors"}
	lora := []string{"lora-have.safetensors"}

	n, err := db.SyncModelPresets(sources, sd, lora)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, []string{"TwoLoras"}, presetNames(t, db))

	s := availabilityByName(t, db, sd, lora, "TwoLoras")
	assert.False(t, s.Installed)
	assert.Empty(t, s.MissingSD)
	assert.Contains(t, s.MissingLoRA, "lora-missing")
	assert.NotContains(t, s.MissingLoRA, "lora-have")
}

func TestSyncModelPresets_SecondSyncDoesNotDuplicate(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("Alpha", "m-alpha"), syncPreset("Alpha 2", "m-alpha"))}
	sd := []string{"m-alpha.safetensors"}

	n, err := db.SyncModelPresets(sources, sd, nil)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = db.SyncModelPresets(sources, sd, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	got := presetsByName(t, db)
	assert.Len(t, got["Alpha"], 1)
	assert.Len(t, got["Alpha 2"], 1)
}

func TestSyncModelPresets_DeletedPresetDoesNotReturn(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("Keep", "m-alpha"), syncPreset("Gone", "m-alpha"))}
	sd := []string{"m-alpha.safetensors"}

	_, err := db.SyncModelPresets(sources, sd, nil)
	require.NoError(t, err)

	got := presetsByName(t, db)
	require.Len(t, got["Gone"], 1)
	require.NoError(t, db.Delete(got["Gone"][0].ID))

	n, err := db.SyncModelPresets(sources, sd, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, []string{"Keep"}, presetNames(t, db))
}

func TestSyncModelPresets_NewPresetInSetForPresentModelInstalled(t *testing.T) {
	db := testDB(t)
	sd := []string{"m-alpha.safetensors"}
	v1 := []PresetSource{syncSource(t, "a.json", syncPreset("Old", "m-alpha"))}

	_, err := db.SyncModelPresets(v1, sd, nil)
	require.NoError(t, err)

	v2 := []PresetSource{syncSource(t, "a.json", syncPreset("Old", "m-alpha"), syncPreset("New", "m-alpha"))}
	n, err := db.SyncModelPresets(v2, sd, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, []string{"New", "Old"}, presetNames(t, db))
}

func TestSyncModelPresets_ModelAppearsLaterInstallsItsPresets(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{
		syncSource(t, "a.json", syncPreset("Klein", "flux-2-klein-4b")),
		syncSource(t, "catalog/anima.json", syncPreset("Anima 1", "anima"), syncPreset("Anima 2", "anima")),
	}

	_, err := db.SyncModelPresets(sources, []string{"flux-2-klein-4b.gguf"}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Klein"}, presetNames(t, db))

	n, err := db.SyncModelPresets(sources, []string{"flux-2-klein-4b.gguf", "anima.safetensors"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, []string{"Anima 1", "Anima 2", "Klein"}, presetNames(t, db))
}

func TestSyncModelPresets_ModelGoneDeletesNothing(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("Alpha", "m-alpha"))}

	_, err := db.SyncModelPresets(sources, []string{"m-alpha.safetensors"}, nil)
	require.NoError(t, err)

	n, err := db.SyncModelPresets(sources, []string{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, []string{"Alpha"}, presetNames(t, db))

	s := availabilityByName(t, db, []string{}, nil, "Alpha")
	assert.False(t, s.Installed)
	assert.Contains(t, s.MissingSD, "m-alpha")
}

func TestSyncModelPresets_ExistingNameNotInsertedButRemembered(t *testing.T) {
	db := testDB(t)
	user := &Preset{Name: "Clash", Prompt: "my own prompt", ModelName: "user_model", Steps: 30, CfgScale: 7, Sampler: "Euler a"}
	require.NoError(t, db.Create(user))
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("Clash", "m-alpha"))}
	sd := []string{"m-alpha.safetensors"}

	n, err := db.SyncModelPresets(sources, sd, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	got := presetsByName(t, db)
	require.Len(t, got["Clash"], 1)
	assert.Equal(t, "my own prompt", got["Clash"][0].Prompt)
	assert.Contains(t, installLog(t, db), "Clash")

	require.NoError(t, db.Delete(user.ID))
	n, err = db.SyncModelPresets(sources, sd, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, presetNames(t, db))
}

func TestSyncModelPresets_RemembersInstalledNames(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("Alpha", "m-alpha"), syncPreset("Beta", "m-beta"))}

	_, err := db.SyncModelPresets(sources, []string{"m-alpha.safetensors"}, nil)
	require.NoError(t, err)

	log := installLog(t, db)
	assert.Contains(t, log, "Alpha")
	assert.NotContains(t, log, "Beta")
}

func TestPresetAvailability_Models(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		sd        []string
		installed bool
		missingSD bool
	}{
		{"model present", "m-alpha", []string{"m-alpha.safetensors"}, true, false},
		{"model present as gguf", "m-alpha", []string{"m-alpha.gguf"}, true, false},
		{"model present as ckpt", "m-alpha", []string{"m-alpha.ckpt"}, true, false},
		{"model absent", "m-alpha", []string{"other.safetensors"}, false, true},
		{"empty model, nothing on server", "", []string{}, true, false},
		{"sd list not received", "m-alpha", nil, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testDB(t)
			p := &Preset{Name: "P", Prompt: "p", ModelName: tt.model, Steps: 10, CfgScale: 5, Sampler: "Euler"}
			require.NoError(t, db.Create(p))

			s := availabilityByName(t, db, tt.sd, []string{}, "P")
			assert.Equal(t, p.ID, s.ID)
			assert.Equal(t, tt.installed, s.Installed)
			if tt.missingSD {
				assert.Contains(t, s.MissingSD, tt.model)
			} else {
				assert.Empty(t, s.MissingSD)
			}
			assert.Empty(t, s.MissingLoRA)
		})
	}
}

func TestPresetAvailability_ListsNotReceivedSeparately(t *testing.T) {
	tests := []struct {
		name        string
		sd          []string
		lora        []string
		installed   bool
		missingSD   bool
		missingLoRA bool
	}{
		{"sd not received, lora received", nil, []string{"other-lora.safetensors"}, false, false, true},
		{"sd received, lora not received", []string{"other.safetensors"}, nil, false, true, false},
		{"both not received", nil, nil, true, false, false},
		{"both received and present", []string{"m-alpha.safetensors"}, []string{"lora-x.safetensors"}, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testDB(t)
			sources := []PresetSource{syncSource(t, "a.json", syncPresetWithLoras("WithLora", "m-alpha", "lora-x"))}
			_, err := db.SyncModelPresets(sources, []string{"m-alpha.safetensors"}, nil)
			require.NoError(t, err)

			s := availabilityByName(t, db, tt.sd, tt.lora, "WithLora")
			assert.Equal(t, tt.installed, s.Installed)
			if tt.missingSD {
				assert.Contains(t, s.MissingSD, "m-alpha")
			} else {
				assert.Empty(t, s.MissingSD)
			}
			if tt.missingLoRA {
				assert.Contains(t, s.MissingLoRA, "lora-x")
			} else {
				assert.Empty(t, s.MissingLoRA)
			}
		})
	}
}

func TestPresetAvailability_UserPresetWithoutModelHiddenButKept(t *testing.T) {
	db := testDB(t)
	user := &Preset{Name: "Mine", Prompt: "p", ModelName: "user_model", Steps: 10, CfgScale: 5, Sampler: "Euler"}
	require.NoError(t, db.Create(user))
	plain := &Preset{Name: "Plain", Prompt: "p", Steps: 10, CfgScale: 5, Sampler: "Euler"}
	require.NoError(t, db.Create(plain))

	s := availabilityByName(t, db, []string{"m-alpha.safetensors"}, []string{}, "Mine")
	assert.False(t, s.Installed)
	assert.Contains(t, s.MissingSD, "user_model")

	s = availabilityByName(t, db, []string{"m-alpha.safetensors"}, []string{}, "Plain")
	assert.True(t, s.Installed)

	got, err := db.Get(user.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.IsBundled)
	assert.Equal(t, []string{"Mine", "Plain"}, presetNames(t, db))
}

func TestPresetAvailability_UserPresetMissingLoRA(t *testing.T) {
	db := testDB(t)
	user := &Preset{Name: "MineLora", Prompt: "p", ModelName: "m-alpha", Steps: 10, CfgScale: 5, Sampler: "Euler",
		Loras: `[{"name":"lora-a","weight":1},{"name":"lora-b","weight":0.5}]`}
	require.NoError(t, db.Create(user))

	s := availabilityByName(t, db, []string{"m-alpha.safetensors"}, []string{"lora-a.safetensors"}, "MineLora")
	assert.False(t, s.Installed)
	assert.Empty(t, s.MissingSD)
	assert.Equal(t, []string{"lora-b"}, s.MissingLoRA)
}

func TestPresetAvailability_EmptyDB(t *testing.T) {
	db := testDB(t)
	st, err := db.PresetAvailability([]string{"m.safetensors"}, []string{})
	require.NoError(t, err)
	assert.Empty(t, st)
}

func TestMigratePresetInstallLog_OldDBDeletedPresetDoesNotReturn(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{
		syncSource(t, "anime.json", syncPreset("Anime Kept", "m-alpha"), syncPreset("Anime Deleted", "m-alpha")),
		syncSource(t, "catalog/anima.json", syncPreset("Anima Installed", "anima"), syncPreset("Anima Never", "anima")),
	}
	require.NoError(t, db.SetSetting("bundled_seeded_files", `["anime.json"]`))
	for _, name := range []string{"Anime Kept", "Anima Installed"} {
		require.NoError(t, db.Create(&Preset{Name: name, Prompt: "p", ModelName: "m-alpha", Steps: 10, CfgScale: 5, Sampler: "Euler"}))
	}

	require.NoError(t, db.MigratePresetInstallLog(sources))

	assert.Equal(t, sorted("Anime Kept", "Anime Deleted", "Anima Installed", "Anima Never"), installLog(t, db))

	n, err := db.SyncModelPresets(sources, []string{"m-alpha.safetensors", "anima.safetensors"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, sorted("Anime Kept", "Anima Installed"), presetNames(t, db))
}

func TestMigratePresetInstallLog_UntouchedCatalogSetInstalledLater(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "catalog/krea.json", syncPreset("Krea One", "krea"), syncPreset("Krea Two", "krea"))}
	require.NoError(t, db.SetSetting("bundled_seeded_files", `["anime.json"]`))

	require.NoError(t, db.MigratePresetInstallLog(sources))
	assert.Empty(t, installLog(t, db))

	n, err := db.SyncModelPresets(sources, []string{"krea.gguf"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestMigratePresetInstallLog_SecondRunDoesNotOverwrite(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("First", "m-alpha")), syncSource(t, "b.json", syncPreset("Later", "m-alpha"))}
	require.NoError(t, db.Create(&Preset{Name: "First", Prompt: "p", ModelName: "m-alpha", Steps: 10, CfgScale: 5, Sampler: "Euler"}))

	require.NoError(t, db.MigratePresetInstallLog(sources))
	require.Equal(t, []string{"First"}, installLog(t, db))

	require.NoError(t, db.Create(&Preset{Name: "Later", Prompt: "p", ModelName: "m-alpha", Steps: 10, CfgScale: 5, Sampler: "Euler"}))
	require.NoError(t, db.MigratePresetInstallLog(sources))
	assert.Equal(t, []string{"First"}, installLog(t, db))
}

func TestMigratePresetInstallLog_ExistingSettingUntouched(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{syncSource(t, "a.json", syncPreset("InDB", "m-alpha"))}
	require.NoError(t, db.SetSetting("bundled_seeded_files", `["a.json"]`))
	require.NoError(t, db.Create(&Preset{Name: "InDB", Prompt: "p", ModelName: "m-alpha", Steps: 10, CfgScale: 5, Sampler: "Euler"}))
	require.NoError(t, db.SetSetting(autoInstalledKey, `["Something"]`))

	require.NoError(t, db.MigratePresetInstallLog(sources))
	assert.Equal(t, []string{"Something"}, installLog(t, db))
}

func TestMigratePresetInstallLog_FreshDBThenSyncInstallsPresentModel(t *testing.T) {
	db := testDB(t)
	sources := []PresetSource{
		syncSource(t, "klein.json", syncPreset("Klein A", "flux-2-klein-4b"), syncPreset("Klein B", "flux-2-klein-4b")),
		syncSource(t, "flux.json", syncPreset("Flux Dev", "flux1-dev-fp8")),
	}

	require.NoError(t, db.MigratePresetInstallLog(sources))
	v, err := db.GetSetting(autoInstalledKey)
	require.NoError(t, err)
	require.NotEmpty(t, v)
	var log []string
	require.NoError(t, json.Unmarshal([]byte(v), &log))
	assert.Empty(t, log)

	n, err := db.SyncModelPresets(sources, []string{"flux-2-klein-4b.gguf"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, []string{"Klein A", "Klein B"}, presetNames(t, db))
}

func TestModelPresetNames(t *testing.T) {
	sources := []PresetSource{
		syncSource(t, "realistic.json",
			syncPreset("Реализм", "realisticVision"),
			syncPreset("Klein One", "flux-2-klein-4b"),
			syncPreset("Портрет", "realisticVision"),
		),
		syncSource(t, "catalog/anima.json", syncPreset("Anima 1", "anima"), syncPreset("Klein Two", "flux-2-klein-4b")),
		syncSource(t, "catalog/empty.json"),
	}

	got, err := ModelPresetNames(sources)
	require.NoError(t, err)

	assert.Equal(t, []string{"Реализм", "Портрет"}, got["realisticVision"])
	assert.Equal(t, []string{"Klein One", "Klein Two"}, got["flux-2-klein-4b"])
	assert.Equal(t, []string{"Anima 1"}, got["anima"])
	_, ok := got["model-without-presets"]
	assert.False(t, ok)
	assert.Len(t, got, 3)
}

func TestModelPresetNames_NoSources(t *testing.T) {
	got, err := ModelPresetNames(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
