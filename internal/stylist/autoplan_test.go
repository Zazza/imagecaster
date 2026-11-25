package stylist

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const autoTestTarget = "base:SDXL"

func autoTestOptions() []ShootOption {
	return []ShootOption{{ID: autoTestTarget, Name: "Studio portrait", Description: "Soft studio photography"}}
}

func autoTestRequest() SuggestRequest {
	return SuggestRequest{Plan: PlanRequest{Person: "Example", Frames: 1, Wish: "night noir portrait", Locale: "en", Seed: 1}}
}

func autoTestReply(t *testing.T, change func(map[string]any)) string {
	t.Helper()
	response := map[string]any{
		"concept": "Night portrait in the city", "light": "Hard side light", "focus": "Person and city", "target": autoTestTarget,
		"series": []any{map[string]any{
			"outfit": "black cotton jacket, straight trousers, white silk shirt, leather shoes",
			"hair":   "low neat bun", "makeup": "matte neutral makeup", "background": "city street at night", "place": "urban",
			"direction": "Hard side light, deep shadows, crisp fabric detail",
			"shots":     []any{map[string]any{"pose": "Standing", "prompt": "standing near a window, looking aside", "framing": "full", "landscape": false}},
		}},
	}
	if change != nil {
		change(response)
	}
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	return string(encoded)
}

func autoTestSeries(response map[string]any) map[string]any {
	return response["series"].([]any)[0].(map[string]any)
}

func TestSuggestCoherentPlanCarriesDirectionIntoGeneration(t *testing.T) {
	chat := &fakeChat{reply: autoTestReply(t, nil)}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, autoTestTarget, got.Target)
	assert.NotEmpty(t, got.Concept)
	assert.NotEmpty(t, got.Light)
	assert.NotEmpty(t, got.Focus)
	require.Len(t, got.Series, 1)
	require.Len(t, got.Series[0].Shots, 1)
	assert.Contains(t, chat.user, "night noir portrait")
	assert.Contains(t, chat.user, autoTestTarget)
	for _, model := range []string{"SDXL", "Z-Image"} {
		assert.Contains(t, FramePrompt(model, "", "a person", got.Series[0], got.Series[0].Shots[0]), got.Series[0].Direction)
		assert.Contains(t, StyledFramePrompt(model, "", "a person", got.Series[0], got.Series[0].Shots[0]), got.Series[0].Direction)
	}
}

func TestSuggestRejectsUnusableAIWithoutRandomFallback(t *testing.T) {
	cases := []struct {
		name   string
		reply  string
		err    error
		modify func(map[string]any)
	}{
		{name: "not json", reply: "I suggest a city shoot"},
		{name: "network error", err: errors.New("planner unavailable")},
		{name: "empty series", modify: func(v map[string]any) { v["series"] = []any{} }},
		{name: "missing concept", modify: func(v map[string]any) { delete(v, "concept") }},
		{name: "missing light", modify: func(v map[string]any) { delete(v, "light") }},
		{name: "missing focus", modify: func(v map[string]any) { delete(v, "focus") }},
		{name: "missing auto outfit", modify: func(v map[string]any) { delete(autoTestSeries(v), "outfit") }},
		{name: "missing auto hair", modify: func(v map[string]any) { delete(autoTestSeries(v), "hair") }},
		{name: "missing auto makeup", modify: func(v map[string]any) { delete(autoTestSeries(v), "makeup") }},
		{name: "missing auto background", modify: func(v map[string]any) { delete(autoTestSeries(v), "background") }},
		{name: "invalid place", modify: func(v map[string]any) { autoTestSeries(v)["place"] = "another planet" }},
		{name: "missing direction", modify: func(v map[string]any) { delete(autoTestSeries(v), "direction") }},
		{name: "invalid framing", modify: func(v map[string]any) { autoTestSeries(v)["shots"].([]any)[0].(map[string]any)["framing"] = "unknown" }},
		{name: "changes identity", modify: func(v map[string]any) { autoTestSeries(v)["hair"] = "blonde waves" }},
		{name: "excessive text", modify: func(v map[string]any) { autoTestSeries(v)["background"] = strings.Repeat("a", 10000) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reply := tc.reply
			if reply == "" {
				reply = autoTestReply(t, tc.modify)
			}
			chat := &fakeChat{reply: reply, err: tc.err}
			got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
			assert.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestSuggestValidatesInputBeforeCallingAI(t *testing.T) {
	cases := []struct {
		name string
		req  SuggestRequest
	}{
		{name: "zero frames", req: SuggestRequest{Plan: PlanRequest{Frames: 0}}},
		{name: "too many frames", req: SuggestRequest{Plan: PlanRequest{Frames: MaxFrames + 1}}},
		{name: "unavailable locked target", req: SuggestRequest{Plan: PlanRequest{Frames: 1}, Target: "preset:missing"}},
		{name: "unknown selected item", req: SuggestRequest{Plan: PlanRequest{Frames: 1, Aspects: map[string]AspectChoice{"outfit": {Mode: ModePick, IDs: []string{"missing"}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat := &fakeChat{reply: autoTestReply(t, nil)}
			_, err := Suggest(chat, "planner", tc.req, mustBase(t), autoTestOptions())
			assert.Error(t, err)
			assert.Zero(t, chat.calls)
		})
	}
	_, err := Suggest(nil, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	assert.Error(t, err)
	chat := &fakeChat{reply: autoTestReply(t, nil)}
	_, err = Suggest(chat, "", autoTestRequest(), mustBase(t), autoTestOptions())
	assert.Error(t, err)
	_, err = Suggest(chat, "planner", autoTestRequest(), mustBase(t), nil)
	assert.Error(t, err)
	assert.Zero(t, chat.calls)
}

func TestSuggestPreservesAndExplainsManualConstraints(t *testing.T) {
	base := mustBase(t)
	outfit := base.Outfit[0]
	pose := base.Pose[0]
	req := autoTestRequest()
	req.Target = autoTestTarget
	req.Plan.Aspects = map[string]AspectChoice{
		"outfit": {Mode: ModePick, IDs: []string{outfit.ID}},
		"hair":   {Mode: ModeCustom, Text: "neat braided bun", TextRu: "аккуратный пучок"},
		"makeup": {Mode: ModeKeep},
		"pose":   {Mode: ModePick, IDs: []string{pose.ID}},
	}
	before, err := json.Marshal(req)
	require.NoError(t, err)
	chat := &fakeChat{reply: autoTestReply(t, nil)}
	got, err := Suggest(chat, "planner", req, base, autoTestOptions())
	require.NoError(t, err)
	s := got.Series[0]
	assert.Equal(t, outfit.Prompt, s.Outfit)
	assert.Equal(t, "neat braided bun", s.Hair)
	assert.Empty(t, s.Makeup)
	assert.Equal(t, pose.Prompt, s.Shots[0].Prompt)
	assert.Equal(t, pose.Framing, s.Shots[0].Framing)
	assert.Equal(t, autoTestTarget, got.Target)
	assert.Contains(t, chat.user, outfit.Prompt)
	assert.Contains(t, chat.user, "neat braided bun")
	assert.Contains(t, chat.user, pose.Prompt)
	after, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "planning must not mutate the request")
}

func TestSuggestGarmentContextAndSafeFraming(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Garment = true
	req.GarmentOutfit = "a red wool coat with large buttons"
	chat := &fakeChat{reply: autoTestReply(t, nil)}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	assert.Contains(t, chat.user, req.GarmentOutfit)
	assert.Equal(t, DefaultOutfit, got.Series[0].Outfit)
	for _, framing := range []string{"close", "waist"} {
		chat.reply = autoTestReply(t, func(v map[string]any) {
			autoTestSeries(v)["shots"].([]any)[0].(map[string]any)["framing"] = framing
		})
		got, err = Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
		assert.Error(t, err)
		assert.Nil(t, got)
	}
}

func TestSuggestCannotChangeLockedTarget(t *testing.T) {
	req := autoTestRequest()
	req.Target = autoTestTarget
	options := append(autoTestOptions(), ShootOption{ID: "preset:7", Name: "Another compatible preset"})
	chat := &fakeChat{reply: autoTestReply(t, func(v map[string]any) { v["target"] = "preset:7" })}
	got, err := Suggest(chat, "planner", req, mustBase(t), options)
	if err == nil {
		require.NotNil(t, got)
		assert.Equal(t, autoTestTarget, got.Target)
	} else {
		assert.Nil(t, got)
	}
}

type autoTranslationChat struct {
	replies []string
	users   []string
	calls   []contextChatCall
}

func (c *autoTranslationChat) Chat(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
	c.users = append(c.users, userMessage)
	c.calls = append(c.calls, contextChatCall{system: systemPrompt, user: userMessage, maxTokens: maxTokens})
	if len(c.users) > len(c.replies) {
		return displayTranslationReply(userMessage)
	}
	return c.replies[len(c.users)-1], nil
}

func TestSuggestTranslatesRawCustomConstraintWithoutMutatingRequest(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Locale = "ru"
	req.Plan.Aspects = map[string]AspectChoice{"hair": {Mode: ModeCustom, Text: "аккуратный пучок"}}
	before, err := json.Marshal(req)
	require.NoError(t, err)
	chat := &autoTranslationChat{replies: []string{`{"texts":["neat braided bun"]}`, autoTestReply(t, nil)}}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.Greater(t, len(chat.users), 2)
	for _, call := range chat.calls {
		assertContextBudget(t, call)
	}
	assert.Contains(t, chat.users[0], "аккуратный пучок")
	assert.Contains(t, chat.users[1], "neat braided bun")
	require.Len(t, got.Series, 1)
	assert.Equal(t, "neat braided bun", got.Series[0].Hair)
	assert.Equal(t, "аккуратный пучок", got.Series[0].HairRu)
	after, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}
