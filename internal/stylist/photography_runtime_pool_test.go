package stylist

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const runtimeLegacyCatalogSize = 500
const runtimeMatchingProfileCount = 5
const runtimeMaximumSelectionCalls = 3

type runtimePoolChat struct {
	t *testing.T
	scriptedChat
	selections [][]ShootOption
	intent     string
}

func (c *runtimePoolChat) Chat(model, system, user string, temperature float64, maxTokens int) (string, error) {
	var input struct {
		Options []ShootOption `json:"options"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(user), &input))
	reply := autoTestReply(c.t, nil)
	if len(input.Options) > 0 && !strings.Contains(system, `"series"`) {
		c.selections = append(c.selections, input.Options)
		raw, err := json.Marshal(map[string]string{"target": input.Options[0].ID, "reason": "Matches photographic intent"})
		require.NoError(c.t, err)
		reply = string(raw)
	} else if !strings.Contains(system, `"series"`) {
		raw, err := json.Marshal(map[string]string{"intent": c.intent})
		require.NoError(c.t, err)
		reply = string(raw)
	}
	c.replies = append(c.replies, reply)
	return c.scriptedChat.Chat(model, system, user, temperature, maxTokens)
}

func runtimeLegacyOptions() []ShootOption {
	options := make([]ShootOption, runtimeLegacyCatalogSize)
	for i := range options {
		options[i] = ShootOption{ID: fmt.Sprintf("preset:legacy-%d", i), Name: fmt.Sprintf("Old user style %d", i), Category: "user", Model: "installed-legacy-model"}
	}
	return options
}

func runtimePoolRequest() SuggestRequest {
	req := autoTestRequest()
	req.Plan.Locale = ""
	return req
}

func TestPhotographyRuntimeCuratedPoolExcludesLargeLegacyCatalog(t *testing.T) {
	for _, intent := range []string{"noir", "general"} {
		t.Run(intent, func(t *testing.T) {
			options := runtimeLegacyOptions()
			options[0].Profile = "noir"
			wantIDs := []string{}
			for i := 0; i < runtimeMatchingProfileCount; i++ {
				id := fmt.Sprintf("preset:curated-%d", i)
				options = append(options, ShootOption{ID: id, Name: "Dedicated photograph", Category: "stylist_photo", Profile: "noir", Model: fmt.Sprintf("installed-photo-%d", i)})
				wantIDs = append(wantIDs, id)
			}
			options = append(options, ShootOption{ID: "preset:other-intent", Name: "Skin photograph", Category: "stylist_photo", Profile: "skin", Model: "installed-skin"})
			if intent == "general" {
				wantIDs = append(wantIDs, "preset:other-intent")
			}
			chat := &runtimePoolChat{t: t, intent: intent}
			got, err := Suggest(chat, "planner", runtimePoolRequest(), mustBase(t), options)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Len(t, chat.selections, 1)
			ids := []string{}
			for _, option := range chat.selections[0] {
				ids = append(ids, option.ID)
			}
			assert.ElementsMatch(t, wantIDs, ids)
			assert.LessOrEqual(t, len(chat.calls), runtimeMaximumSelectionCalls)
			for _, call := range chat.calls {
				assert.NotContains(t, call.user, "preset:legacy-")
				assertContextBudget(t, call)
			}
		})
	}
}

func TestPhotographyRuntimeExplicitLegacyPinBypassesPoolLimit(t *testing.T) {
	options := runtimeLegacyOptions()
	options = append(options, ShootOption{ID: "preset:curated", Name: "Noir", Category: "stylist_photo", Profile: "noir", Model: "installed-photo"})
	req := runtimePoolRequest()
	req.Target = options[len(options)-2].ID
	chat := &scriptedChat{replies: []string{autoTestReply(t, nil)}}
	got, err := Suggest(chat, "planner", req, mustBase(t), options)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, req.Target, got.Target)
	require.Len(t, chat.calls, 1)
}

func TestPhotographyRuntimeBaseFallbackExcludesLegacyCatalog(t *testing.T) {
	options := append(runtimeLegacyOptions(), ShootOption{ID: autoTestTarget, Name: "Compatible base", Model: "installed-base"})
	chat := &runtimePoolChat{t: t, intent: "noir"}
	got, err := Suggest(chat, "planner", runtimePoolRequest(), mustBase(t), options)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, autoTestTarget, got.Target)
	assert.LessOrEqual(t, len(chat.calls), runtimeMaximumSelectionCalls)
	for _, call := range chat.calls {
		assert.NotContains(t, call.user, "preset:legacy-")
	}
}

func TestPhotographyRuntimeOversizedLegacyCatalogFailsBeforeLLM(t *testing.T) {
	chat := &scriptedChat{}
	got, err := Suggest(chat, "planner", runtimePoolRequest(), mustBase(t), runtimeLegacyOptions())
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, chat.calls, "oversized unprofiled catalog must fail locally before any catalog LLM call")
}

func TestPhotographyRuntimeLegacyFallbackLimitBoundary(t *testing.T) {
	const maximumLegacyCandidates = 32
	for _, count := range []int{maximumLegacyCandidates, maximumLegacyCandidates + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			options := runtimeLegacyOptions()[:count]
			chat := &runtimePoolChat{t: t, intent: "general"}
			got, err := Suggest(chat, "planner", runtimePoolRequest(), mustBase(t), options)
			if count > maximumLegacyCandidates {
				assert.Error(t, err)
				assert.Nil(t, got)
				assert.Empty(t, chat.calls, "over-limit legacy pools cannot silently slice candidates")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			ids := map[string]bool{}
			for _, page := range chat.selections {
				for _, option := range page {
					ids[option.ID] = true
				}
			}
			assert.Len(t, ids, count, "small legacy fallback must consider every candidate")
		})
	}
}
