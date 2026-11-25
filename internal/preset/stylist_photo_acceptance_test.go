package preset

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStylistPhotoBundleInstallsAvailablePreservingUserAndDeleted(t *testing.T) {
	raw, err := os.ReadFile("../../data/presets/stylist-photography.json")
	require.NoError(t, err)
	var bundle struct {
		Presets []Preset `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &bundle))
	require.NotEmpty(t, bundle.Presets)
	model := bundle.Presets[0].ModelName
	expected := 0
	for _, p := range bundle.Presets {
		if p.ModelName == model {
			expected++
		}
	}
	require.Greater(t, expected, 1)
	db := testDB(t)
	user := bundle.Presets[0]
	user.Prompt = "User authored photographic direction"
	require.NoError(t, db.Create(&user))
	sources := []PresetSource{{File: "stylist-photography.json", Raw: raw}}
	n, err := db.SyncModelPresets(sources, []string{model + ".safetensors"}, nil)
	require.NoError(t, err)
	assert.Equal(t, expected-1, n)
	rows := presetsByName(t, db)
	assert.Equal(t, user.Prompt, rows[user.Name][0].Prompt)
	var deleted string
	for name, ps := range rows {
		if name != user.Name {
			deleted = name
			require.NoError(t, db.Delete(ps[0].ID))
			break
		}
	}
	require.NotEmpty(t, deleted)
	n, err = db.SyncModelPresets(sources, []string{model + ".safetensors"}, nil)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.NotContains(t, presetsByName(t, db), deleted)
	for _, ps := range presetsByName(t, db) {
		for _, p := range ps {
			assert.Equal(t, model, p.ModelName)
		}
	}
}

func TestStylistPhotoProfilesDescribeIntentWithoutIdentity(t *testing.T) {
	raw, err := os.ReadFile("../../data/presets/stylist-photography.json")
	require.NoError(t, err)
	var bundle struct {
		Presets []struct {
			Preset
			Tags string `json:"tags"`
		} `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &bundle))
	families := map[string]map[string]bool{}
	for _, p := range bundle.Presets {
		assert.Equal(t, "stylist_photo", p.PresetType)
		require.NotEmpty(t, p.ModelName)
		assert.NotEmpty(t, p.Sampler)
		assert.NotEmpty(t, p.ScheduleType)
		assert.Positive(t, p.Steps)
		assert.Positive(t, p.CfgScale)
		text := strings.ToLower(p.Prompt)
		for _, forbidden := range []string{"blue eyes", "blonde hair", "young woman", "white skin", "<lora:"} {
			assert.NotContains(t, text, forbidden)
		}
		if families[p.ModelName] == nil {
			families[p.ModelName] = map[string]bool{}
		}
		for _, intent := range []string{"skin", "material", "environment", "noir", "artistic"} {
			if strings.Contains(strings.ToLower(p.Name)+" "+p.Tags+" "+text, intent) {
				families[p.ModelName][intent] = true
			}
		}
	}
	const supportedFamilies = 5
	assert.Len(t, families, supportedFamilies)
	for model, intents := range families {
		for _, intent := range []string{"skin", "material", "environment", "noir", "artistic"} {
			assert.True(t, intents[intent], model+": "+intent)
		}
	}
}
