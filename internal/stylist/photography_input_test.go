package stylist

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type photographyCandidateChat struct {
	t            *testing.T
	candidates   []ShootOption
	selected     string
	plannerInput string
}

func (c *photographyCandidateChat) Chat(_ string, system, user string, _ float64, _ int) (string, error) {
	if strings.Contains(system, `"series"`) {
		c.plannerInput = user
		return autoTestReply(c.t, nil), nil
	}
	var input struct {
		Options []ShootOption `json:"options"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(user), &input))
	if len(input.Options) == 0 {
		return `{"intent":"general"}`, nil
	}
	c.candidates = append(c.candidates, input.Options...)
	raw, err := json.Marshal(map[string]string{"target": c.selected, "reason": "Photographic intent", "reason_ru": "Фотографическая задача"})
	return string(raw), err
}

func TestPhotographyCandidatesPreferDedicatedPoolAndPreservePinnedBase(t *testing.T) {
	const profiledModel = "installed-photo-model"
	const otherModel = "installed-other-model"
	options := []ShootOption{
		{ID: "base:SDXL", Name: "Base with photo profile", Model: profiledModel},
		{ID: "preset:photo", Name: "Dedicated photograph", Model: profiledModel, Category: "stylist_photo", Profile: "skin"},
		{ID: "base:Flux.1", Name: "Base without photo profile", Model: otherModel},
		{ID: "preset:ordinary", Name: "User style", Model: profiledModel, Category: "user"},
		{ID: "workflow:custom", Name: "Custom workflow", Model: profiledModel},
	}
	for _, pinned := range []bool{false, true} {
		name := "auto"
		if pinned {
			name = "pinned_base"
		}
		t.Run(name, func(t *testing.T) {
			req := autoTestRequest()
			chat := &photographyCandidateChat{t: t, selected: "preset:photo"}
			if pinned {
				req.Target = "base:SDXL"
			}
			got, err := Suggest(chat, "planner", req, mustBase(t), options)
			require.NoError(t, err)
			require.NotNil(t, got)
			if pinned {
				assert.Equal(t, "base:SDXL", got.Target, "an explicit base pin remains usable")
				return
			}
			var planner struct {
				Options []ShootOption `json:"options"`
			}
			require.NoError(t, json.Unmarshal([]byte(chat.plannerInput), &planner))
			ids := []string{}
			for _, option := range planner.Options {
				ids = append(ids, option.ID)
			}
			assert.Equal(t, []string{"preset:photo"}, ids, "matching curated profiles form the entire automatic pool")
		})
	}
}

func TestPhotographyPlannerInputContainsOnlyExplicitVisualConstraints(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "auto"
		if mixed {
			name = "mixed"
		}
		t.Run(name, func(t *testing.T) {
			base := mustBase(t)
			req := autoTestRequest()
			req.Target = autoTestTarget
			if mixed {
				req.Plan.Aspects = map[string]AspectChoice{
					"outfit": {Mode: ModePick, IDs: []string{base.Outfit[0].ID}},
					"hair":   {Mode: ModeCustom, Text: "neat braided bun"},
					"makeup": {Mode: ModeKeep},
				}
			}
			chat := &photographyCandidateChat{t: t}
			_, err := Suggest(chat, "planner", req, base, autoTestOptions())
			require.NoError(t, err)
			var input struct {
				Constraints []map[string]any `json:"constraints"`
				Request     struct {
					Plan struct {
						Aspects map[string]string `json:"aspects"`
					} `json:"plan"`
				} `json:"request"`
			}
			require.NoError(t, json.Unmarshal([]byte(chat.plannerInput), &input))
			require.NotEmpty(t, input.Constraints, "planner must retain its series shape")
			for aspect, mode := range input.Request.Plan.Aspects {
				assert.NotEqual(t, "auto", mode, "automatic %s mode is implicit and must be omitted", aspect)
			}
			if mixed {
				assert.Equal(t, string(ModePick), input.Request.Plan.Aspects["outfit"])
				assert.Equal(t, string(ModeCustom), input.Request.Plan.Aspects["hair"])
				assert.Equal(t, string(ModeKeep), input.Request.Plan.Aspects["makeup"])
			}
			for _, constraint := range input.Constraints {
				for _, field := range []string{"outfit", "hair", "makeup", "background", "place"} {
					if mixed && field == "outfit" {
						assert.Equal(t, base.Outfit[0].Prompt, constraint[field])
						continue
					}
					if mixed && field == "hair" {
						assert.Equal(t, "neat braided bun", constraint[field])
						continue
					}
					assert.Empty(t, constraint[field], "random skeleton %s must not become an auto constraint", field)
				}
			}
		})
	}
}
