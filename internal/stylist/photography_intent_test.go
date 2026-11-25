package stylist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhotographyIntentPrecedesModelSelection(t *testing.T) {
	options := []ShootOption{
		{ID: "preset:skin", Name: "Skin", Profile: "skin", Category: "stylist_photo", Model: "skin-model"},
		{ID: "preset:scene", Name: "Scene", Profile: "environment", Category: "stylist_photo", Model: "scene-model"},
		{ID: "preset:scene2", Name: "Scene alternative", Profile: "environment", Category: "stylist_photo", Model: "scene-alternative-model"},
		{ID: "preset:user", Name: "User style", Model: "user-model"},
	}
	chat := &scriptedChat{replies: []string{`{"intent":"environment"}`, `{"target":"preset:scene","reason":"Scene intent and architecture"}`, autoTestReply(t, nil)}}
	req := autoTestRequest()
	req.Plan.Locale = ""
	got, err := Suggest(chat, "planner", req, mustBase(t), options)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, chat.calls, 3)
	assert.Equal(t, "preset:scene", got.Target)
	for _, value := range []string{"skin-model", "scene-model", "scene-alternative-model", "user-model", "preset:skin", "preset:scene", "preset:scene2", "preset:user"} {
		assert.NotContains(t, chat.calls[0].user, value, "intent classification must precede exposure to the model catalog")
	}
	assert.Contains(t, chat.calls[0].system+chat.calls[0].user, "environment")
	var selection struct {
		Options []ShootOption `json:"options"`
	}
	require.NoError(t, json.Unmarshal([]byte(chat.calls[1].user), &selection))
	ids := []string{}
	for _, option := range selection.Options {
		ids = append(ids, option.ID)
	}
	assert.NotContains(t, ids, "preset:skin")
	assert.Contains(t, ids, "preset:scene")
	assert.Contains(t, ids, "preset:scene2")
	assert.NotContains(t, ids, "preset:user")
	for _, call := range chat.calls {
		assertContextBudget(t, call)
	}
}

func TestPhotographyUnmatchedIntentPreservesBaseBeforeDominanceFiltering(t *testing.T) {
	const sameModel = "installed-same-model"
	options := []ShootOption{
		{ID: autoTestTarget, Name: "Base", Model: sameModel},
		{ID: "preset:skin", Name: "Skin", Profile: "skin", Category: "stylist_photo", Model: sameModel},
	}
	chat := &scriptedChat{replies: []string{`{"intent":"noir"}`, autoTestReply(t, nil)}}
	req := autoTestRequest()
	req.Plan.Locale = ""
	got, err := Suggest(chat, "planner", req, mustBase(t), options)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, autoTestTarget, got.Target)
	require.Len(t, chat.calls, 2)
	var selection struct {
		Options []ShootOption `json:"options"`
	}
	require.NoError(t, json.Unmarshal([]byte(chat.calls[1].user), &selection))
	require.Len(t, selection.Options, 1)
	assert.Equal(t, autoTestTarget, selection.Options[0].ID)
}

func TestPhotographyIntentPinGeneralAndInvalidBoundaries(t *testing.T) {
	options := []ShootOption{
		{ID: "preset:skin", Name: "Skin", Profile: "skin", Category: "stylist_photo"},
		{ID: "preset:scene", Name: "Scene", Profile: "environment", Category: "stylist_photo"},
	}
	cases := []struct {
		name      string
		pinned    bool
		replies   []string
		calls     int
		wantError bool
	}{
		{name: "explicit pin skips classification", pinned: true, replies: []string{autoTestReply(t, nil)}, calls: 1},
		{name: "general keeps both profiles", replies: []string{`{"intent":"general"}`, `{"target":"preset:scene","reason":"Photographic fit"}`, autoTestReply(t, nil)}, calls: 3},
		{name: "repeated unknown intent fails closed", replies: []string{`{"intent":"unknown-intent"}`, `{"intent":"unknown-intent"}`}, calls: 2, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Locale = ""
			if tc.pinned {
				req.Target = "preset:skin"
			}
			chat := &scriptedChat{replies: tc.replies}
			got, err := Suggest(chat, "planner", req, mustBase(t), options)
			if tc.wantError {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
			}
			require.Len(t, chat.calls, tc.calls)
			if tc.pinned {
				assert.Equal(t, "preset:skin", got.Target)
			}
			if tc.name == "general keeps both profiles" {
				var input struct {
					Options []ShootOption `json:"options"`
				}
				require.NoError(t, json.Unmarshal([]byte(chat.calls[1].user), &input))
				ids := []string{}
				for _, option := range input.Options {
					ids = append(ids, option.ID)
				}
				assert.ElementsMatch(t, []string{"preset:skin", "preset:scene"}, ids)
			}
			for _, call := range chat.calls {
				assertContextBudget(t, call)
			}
		})
	}
}
