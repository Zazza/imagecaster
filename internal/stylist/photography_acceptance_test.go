package stylist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhotographyInstalledBundleExposesCanonicalCategoryAndModel(t *testing.T) {
	raw, err := os.ReadFile("../../data/presets/stylist-photography.json")
	require.NoError(t, err)
	db, err := preset.Open(filepath.Join(t.TempDir(), "photography.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	model := Models["SDXL"].Model
	n, err := db.SyncModelPresets([]preset.PresetSource{{File: "stylist-photography.json", Raw: raw}}, []string{model}, nil)
	require.NoError(t, err)
	require.Positive(t, n)
	installed, err := db.List()
	require.NoError(t, err)
	people := []serverclient.Person{{Name: "Synthetic", Base: "SDXL", Lora: "synthetic_lora"}}
	options, err := BuildShootOptions(people, "Synthetic", installed, nil, []sd.SDModel{{Name: model}}, []sd.LoRA{{Name: "synthetic_lora"}})
	require.NoError(t, err)
	count := 0
	profiles := []string{}
	for _, option := range options {
		if !strings.HasPrefix(option.ID, "preset:") {
			continue
		}
		count++
		profiles = append(profiles, option.Profile)
		assert.Equal(t, "stylist_photo", option.Category)
		assert.Equal(t, model, option.Model)
	}
	assert.Equal(t, n, count)
	assert.ElementsMatch(t, []string{"skin", "fabric", "environment", "noir", "artistic"}, profiles)
	ordinary := installed[0]
	ordinary.PresetType = "style"
	ordinaryOptions, err := BuildShootOptions(people, "Synthetic", []preset.Preset{ordinary}, nil, []sd.SDModel{{Name: model}}, []sd.LoRA{{Name: "synthetic_lora"}})
	require.NoError(t, err)
	foundOrdinary := false
	for _, option := range ordinaryOptions {
		if strings.HasPrefix(option.ID, "preset:") {
			foundOrdinary = true
			assert.Empty(t, option.Profile, "ordinary styles must not gain curated intent from inherited photography tags")
		}
	}
	require.True(t, foundOrdinary)

}

func TestPhotographyPresetParametersMatchTheirModelFamily(t *testing.T) {
	raw, err := os.ReadFile("../../data/presets/stylist-photography.json")
	require.NoError(t, err)
	var bundle struct {
		Presets []struct {
			Model   string  `json:"model_name"`
			Sampler string  `json:"sampler"`
			Steps   int     `json:"steps"`
			CFG     float64 `json:"cfg_scale"`
		} `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(raw, &bundle))
	for _, p := range bundle.Presets {
		found := false
		for _, model := range Models {
			if model.Model != p.Model {
				continue
			}
			found = true
			assert.Equal(t, model.Sampler, p.Sampler, p.Model)
			assert.Equal(t, model.Steps, p.Steps, p.Model)
			assert.Equal(t, model.CFG, p.CFG, p.Model)
		}
		assert.True(t, found, p.Model)
	}
}

type photographyChat struct {
	t         *testing.T
	selection string
	selectors int
	plans     int
}

func (c *photographyChat) Chat(_ string, system, user string, _ float64, tokens int) (string, error) {
	assertContextBudget(c.t, contextChatCall{system: system, user: user, maxTokens: tokens})
	if strings.Contains(system, `"series"`) {
		c.plans++
		return autoTestReply(c.t, func(v map[string]any) {
			v["target"] = "preset:hallucinated"
			v["target_name"] = "Invented name"
			v["target_model"] = "Invented model"
			v["target_reason"] = "Invented explanation"
		}), nil
	}
	c.selectors++
	if c.selection == "valid" {
		var input struct {
			Options []ShootOption `json:"options"`
		}
		require.NoError(c.t, json.Unmarshal([]byte(user), &input))
		require.NotEmpty(c.t, input.Options)
		selected := input.Options[0].ID
		for _, option := range input.Options {
			if option.ID == "preset:0" {
				selected = option.ID
			}
		}
		raw, err := json.Marshal(map[string]string{"target": selected, "reason": "Natural skin and wool detail", "reason_ru": "Естественная кожа и фактура шерсти"})
		require.NoError(c.t, err)
		return string(raw), nil
	}
	return c.selection, nil
}

func photographyOptions(t *testing.T, count int) []ShootOption {
	t.Helper()
	rows := make([]map[string]string, count)
	for i := range rows {
		rows[i] = map[string]string{"id": fmt.Sprintf("preset:%d", i), "name": fmt.Sprintf("Photo %d", i), "model": "Flux.1", "category": "stylist_photo", "description": "Natural skin and wool texture"}
	}
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	var options []ShootOption
	require.NoError(t, json.Unmarshal(raw, &options))
	return options
}

func TestPhotographyIndependentSelectionAcrossCatalogSizes(t *testing.T) {
	for _, count := range []int{2, 5, 100} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			chat := &photographyChat{t: t, selection: "valid"}
			got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), photographyOptions(t, count))
			const maximumLegacyCandidates = 32
			if count > maximumLegacyCandidates {
				assert.Error(t, err)
				assert.Nil(t, got)
				assert.Zero(t, chat.selectors)
				assert.Zero(t, chat.plans)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Positive(t, chat.selectors)
			assert.Equal(t, 1, chat.plans)
			raw, err := json.Marshal(got)
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal(raw, &result))
			assert.Equal(t, "preset:0", result["target"])
			assert.Equal(t, "Photo 0", result["target_name"])
			assert.Equal(t, "Flux.1", result["target_model"])
			assert.Equal(t, "Natural skin and wool detail", result["target_reason"])
		})
	}
}

func TestPhotographyRejectsInvalidSelectionBeforePlanning(t *testing.T) {
	for _, reply := range []string{`{"target":"preset:outside","reason":"Good light"}`, `{"target":"preset:0"}`, `{"target":"preset:0","reason":"   "}`} {
		t.Run(reply, func(t *testing.T) {
			chat := &photographyChat{t: t, selection: reply}
			got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), photographyOptions(t, 2))
			assert.Error(t, err)
			assert.Nil(t, got)
			assert.Positive(t, chat.selectors)
			assert.Zero(t, chat.plans, "invalid choice must not fall back to the planner")
		})
	}
}
