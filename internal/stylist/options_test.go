package stylist

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildShootOptionsUsesInstalledCompatibleTargets(t *testing.T) {
	sdxl := Models["SDXL"].Model
	flux := Models["Flux.1"].Model
	person := []serverclient.Person{{Name: "example_sdxl", DisplayName: "Example", Base: "SDXL", Lora: "example_lora"}}
	presets := []preset.Preset{
		{ID: 1, Name: "Studio portrait", ModelName: sdxl, PresetType: "style"},
		{ID: 2, Name: "Wrong base", ModelName: flux, PresetType: "style"},
		{ID: 3, Name: "Skin effect", ModelName: sdxl, PresetType: "post_skin"},
		{ID: 4, Name: "Base effect", ModelName: sdxl, PresetType: "post"},
		{ID: 5, Name: "Missing model", ModelName: "missing_model", PresetType: "style"},
	}
	catalog := []sd.SDModel{{Name: sdxl, Title: sdxl}, {Name: flux, Title: flux}}
	options, err := BuildShootOptions(person, "Example", presets, nil, catalog, []sd.LoRA{{Name: "example_lora"}})
	require.NoError(t, err)
	ids := make([]string, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.ID)
		assert.NotEmpty(t, option.Name)
	}
	assert.ElementsMatch(t, []string{"base:SDXL", "preset:1"}, ids)
}

func TestBuildShootOptionsRejectsUnknownPersonAndUnavailableModel(t *testing.T) {
	person := []serverclient.Person{{Name: "Example", Base: "SDXL", Lora: "example_lora"}}
	_, err := BuildShootOptions(person, "Unknown", nil, nil, nil, nil)
	assert.Error(t, err)
	options, err := BuildShootOptions(person, "Example", nil, nil, nil, nil)
	if err == nil {
		assert.Empty(t, options)
	}
	person[0].Lora = ""
	options, err = BuildShootOptions(person, "Example", nil, nil, []sd.SDModel{{Name: Models["SDXL"].Model}}, nil)
	if err == nil {
		assert.Empty(t, options)
	}
}

func TestBuildShootOptionsPipelinesNeedEveryModelAndPersonalLoRA(t *testing.T) {
	sdxl := Models["SDXL"].Model
	flux := Models["Flux.1"].Model
	person := []serverclient.Person{{Name: "Example", Base: "SDXL", Lora: "example_lora"}}
	presets := []preset.Preset{
		{ID: 1, Name: "Portrait", ModelName: sdxl},
		{ID: 2, Name: "Flux finish", ModelName: flux},
		{ID: 3, Name: "Unavailable", ModelName: "missing_model"},
	}
	pipes := []preset.CompoundPreset{
		{ID: 1, Name: "Compatible", Steps: []preset.CompoundPresetStep{{PresetID: 1}, {PresetID: 2}}},
		{ID: 2, Name: "No personal base", Steps: []preset.CompoundPresetStep{{PresetID: 2}}},
		{ID: 3, Name: "Missing step model", Steps: []preset.CompoundPresetStep{{PresetID: 1}, {PresetID: 3}}},
		{ID: 4, Name: "Missing step preset", Steps: []preset.CompoundPresetStep{{PresetID: 1}, {PresetID: 999}}},
		{ID: 5, Name: "Empty", Steps: []preset.CompoundPresetStep{}},
	}
	options, err := BuildShootOptions(person, "Example", presets, pipes, []sd.SDModel{{Name: sdxl}, {Name: flux}}, []sd.LoRA{{Name: "example_lora"}})
	require.NoError(t, err)
	var pipeIDs []string
	for _, option := range options {
		if len(option.ID) >= len("pipe:") && option.ID[:len("pipe:")] == "pipe:" {
			pipeIDs = append(pipeIDs, option.ID)
		}
	}
	assert.Equal(t, []string{"pipe:1"}, pipeIDs)
}

func TestBuildShootOptionsRequiresInstalledPresetLoRA(t *testing.T) {
	model := Models["SDXL"].Model
	people := []serverclient.Person{{Name: "Example", Base: "SDXL", Lora: "example_lora"}}
	presets := []preset.Preset{{ID: 1, Name: "Textured portrait", ModelName: model, Loras: `[{"name":"fabric_detail","weight":0.7}]`}}
	models := []sd.SDModel{{Name: model}}
	for _, installed := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "installed"}[installed], func(t *testing.T) {
			loras := []sd.LoRA{{Name: "example_lora"}}
			if installed {
				loras = append(loras, sd.LoRA{Name: "fabric_detail"})
			}
			options, err := BuildShootOptions(people, "Example", presets, nil, models, loras)
			require.NoError(t, err)
			var ids []string
			for _, option := range options {
				ids = append(ids, option.ID)
			}
			if installed {
				assert.Contains(t, ids, "preset:1")
			} else {
				assert.NotContains(t, ids, "preset:1")
			}
		})
	}
}

func TestBuildShootOptionsUsesEffectiveStepLoRAOverrides(t *testing.T) {
	model := Models["SDXL"].Model
	people := []serverclient.Person{{Name: "Example", Base: "SDXL", Lora: "example_lora"}}
	presets := []preset.Preset{{ID: 1, Name: "Portrait", ModelName: model, Loras: `[{"name":"missing_preset_lora","weight":0.7}]`}}
	emptyOverride := `[]`
	missingOverride := `[{"name":"missing_override_lora","weight":0.7}]`
	installedOverride := `[{"name":"installed_override_lora","weight":0.7}]`
	pipes := []preset.CompoundPreset{
		{ID: 1, Name: "Inherits missing LoRA", Steps: []preset.CompoundPresetStep{{PresetID: 1}}},
		{ID: 2, Name: "Explicitly removes LoRAs", Steps: []preset.CompoundPresetStep{{PresetID: 1, Loras: &emptyOverride}}},
		{ID: 3, Name: "Missing override LoRA", Steps: []preset.CompoundPresetStep{{PresetID: 1, Loras: &missingOverride}}},
		{ID: 4, Name: "Installed override LoRA", Steps: []preset.CompoundPresetStep{{PresetID: 1, Loras: &installedOverride}}},
	}
	options, err := BuildShootOptions(people, "Example", presets, pipes, []sd.SDModel{{Name: model}}, []sd.LoRA{{Name: "example_lora"}, {Name: "installed_override_lora"}})
	require.NoError(t, err)
	var ids []string
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	assert.NotContains(t, ids, "preset:1")
	assert.NotContains(t, ids, "pipe:1")
	assert.Contains(t, ids, "pipe:2")
	assert.NotContains(t, ids, "pipe:3")
	assert.Contains(t, ids, "pipe:4")
}

func TestBuildShootOptionsAllowsServerManagedPersonalLoRA(t *testing.T) {
	model := Models["SDXL"].Model
	people := []serverclient.Person{{Name: "Example", Base: "SDXL", Lora: "example_personal"}}
	presets := []preset.Preset{{ID: 1, Name: "Personal portrait", ModelName: model, Loras: `[{"name":"example_personal","weight":0.8}]`}}
	options, err := BuildShootOptions(people, "Example", presets, nil, []sd.SDModel{{Name: model}}, nil)
	require.NoError(t, err)
	var ids []string
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	assert.Contains(t, ids, "base:SDXL")
	assert.Contains(t, ids, "preset:1")
}
