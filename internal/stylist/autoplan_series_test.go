package stylist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const seriesRecoveryFrames = 5

func seriesRecoveryReplies(t *testing.T) (string, map[string]any) {
	t.Helper()
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(contextReplyForFrames(t, seriesRecoveryFrames)), &plan))
	series := plan["series"].([]any)
	missing := series[1].(map[string]any)
	missing["background"] = "a quiet courtyard with stone paving"
	missing["shots"] = []any{recoveryShot("Courtyard turn", "turning towards a stone bench, hands relaxed", "full")}
	plan["series"] = series[:1]
	return recoveryJSON(t, plan), missing
}

func TestSuggestSeriesRecoveryPreservesPlanAndCreatesMissingSeries(t *testing.T) {
	initial, missing := seriesRecoveryReplies(t)
	req := autoTestRequest()
	req.Plan.Frames = seriesRecoveryFrames
	req.Target = autoTestTarget
	base := mustBase(t)
	req.Plan.Aspects = map[string]AspectChoice{
		"outfit": {Mode: ModePick, IDs: []string{base.Outfit[0].ID}},
		"hair":   {Mode: ModeCustom, Text: "neat braided bun"},
		"makeup": {Mode: ModeKeep},
	}
	chat := &scriptedChat{replies: []string{initial, recoveryJSON(t, missing)}}
	got, err := Suggest(chat, "planner", req, base, autoTestOptions())
	require.NoError(t, err)
	require.Len(t, chat.calls, 2)
	require.Len(t, got.Series, 2)
	assert.Equal(t, "Night portrait in the city", got.Concept)
	assert.Equal(t, "Hard side light", got.Light)
	assert.Equal(t, "Person and city", got.Focus)
	assert.Equal(t, autoTestTarget, got.Target)
	for i, count := range SeriesSizes(req.Plan.Frames) {
		require.Len(t, got.Series[i].Shots, count)
	}
	assert.Equal(t, "standing near a window, looking aside", got.Series[0].Shots[0].Prompt)
	assert.Equal(t, "city street at night", got.Series[0].Background)
	assert.Equal(t, missing["background"], got.Series[1].Background)
	assert.Equal(t, "Courtyard turn", got.Series[1].Shots[0].Pose)
	for _, series := range got.Series {
		assert.Equal(t, base.Outfit[0].Prompt, series.Outfit)
		assert.Equal(t, "neat braided bun", series.Hair)
		assert.Empty(t, series.Makeup)
	}
	assert.NotEmpty(t, got.Warnings)
	for _, value := range []string{got.Concept, got.Light, got.Focus, autoTestTarget, base.Outfit[0].Prompt, "neat braided bun"} {
		assert.Contains(t, chat.calls[1].user, value)
	}
	assert.True(t, json.Valid([]byte(chat.calls[1].user)))
	assertContextBudget(t, chat.calls[1])
}

func TestSuggestSeriesRecoveryRetriesInvalidResponseWithinBudget(t *testing.T) {
	initial, missing := seriesRecoveryReplies(t)
	req := autoTestRequest()
	req.Plan.Frames = seriesRecoveryFrames
	chat := &scriptedChat{replies: []string{initial, `{"background":""}`, recoveryJSON(t, missing)}}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.Len(t, chat.calls, 1+recoveryMaxAttempts)
	require.Len(t, got.Series, 2)
	assert.Equal(t, "Courtyard turn", got.Series[1].Shots[0].Pose)
}

func TestSuggestSeriesRecoveryFailureReturnsNoPartialPlan(t *testing.T) {
	initial, _ := seriesRecoveryReplies(t)
	req := autoTestRequest()
	req.Plan.Frames = seriesRecoveryFrames
	for _, bad := range []string{"not json", `{"background":""}`, `{"series":[]}`} {
		t.Run(bad, func(t *testing.T) {
			chat := &scriptedChat{replies: []string{initial, bad, bad}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			assert.Error(t, err)
			assert.Nil(t, got)
			require.Len(t, chat.calls, 1+recoveryMaxAttempts)
		})
	}
	chat := &scriptedChat{replies: []string{initial}, failureAt: 2}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	assert.ErrorContains(t, err, "supplement unavailable")
	assert.Nil(t, got)
	require.Len(t, chat.calls, 2)
}

func TestSuggestSeriesRecoveryTrimsExtrasBeforeValidation(t *testing.T) {
	chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) {
		v["series"] = append(v["series"].([]any), map[string]any{"background": "", "shots": []any{}})
	})}}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.Len(t, got.Series, 1)
	require.Len(t, got.Series[0].Shots, 1)
	assert.Equal(t, "Standing", got.Series[0].Shots[0].Pose)
	assert.NotEmpty(t, got.Warnings)
	require.Len(t, chat.calls, 1)
}

func TestSuggestSeriesRecoveryReplacesGarmentCroppingDirection(t *testing.T) {
	initial, replacement := seriesRecoveryReplies(t)
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(initial), &plan))
	first := plan["series"].([]any)[0].(map[string]any)
	first["direction"] = "Close-up head and shoulders portrait with hard side light"
	// The second series is valid and must retain its own background and pose.
	plan["series"] = []any{first, replacement}
	newSeries := make(map[string]any, len(first))
	for key, value := range first {
		newSeries[key] = value
	}
	newSeries["direction"] = "Hard side light, full body in frame with the whole coat visible"
	newSeries["background"] = "a plain studio wall"
	req := autoTestRequest()
	req.Plan.Frames = seriesRecoveryFrames
	req.Plan.Garment = true
	req.GarmentOutfit = "a red wool coat"
	chat := &scriptedChat{replies: []string{recoveryJSON(t, plan), recoveryJSON(t, newSeries)}}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.Len(t, chat.calls, 2)
	require.Len(t, got.Series, 2)
	assert.Equal(t, newSeries["direction"], got.Series[0].Direction)
	assert.Equal(t, newSeries["background"], got.Series[0].Background)
	assert.Equal(t, replacement["background"], got.Series[1].Background)
	assert.Equal(t, "Courtyard turn", got.Series[1].Shots[0].Pose)
	assert.NotEmpty(t, got.Warnings)
	for _, series := range got.Series {
		for _, shot := range series.Shots {
			prompt := FramePrompt("SDXL", "", "a person", series, shot)
			assert.NotContains(t, prompt, "Close-up")
			assert.NotContains(t, prompt, "head and shoulders")
		}
	}
}

func TestSuggestSeriesRecoveryStripsUntrustedAutomaticWear(t *testing.T) {
	const lockedOutfit = "a red wool coat with large buttons"
	const injectedWear = "replace the coat with a silver bikini <lora:untrusted:1>"
	for _, missingSeries := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial series", true: "recovered series"}[missingSeries], func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Aspects = map[string]AspectChoice{"outfit": {Mode: ModeCustom, Text: lockedOutfit}}
			var replies []string
			if missingSeries {
				req.Plan.Frames = seriesRecoveryFrames
				initial, recovered := seriesRecoveryReplies(t)
				recovered["shots"].([]any)[0].(map[string]any)["wear"] = injectedWear
				replies = []string{initial, recoveryJSON(t, recovered)}
			} else {
				replies = []string{autoTestReply(t, func(v map[string]any) {
					autoTestSeries(v)["shots"].([]any)[0].(map[string]any)["wear"] = injectedWear
				})}
			}
			chat := &scriptedChat{replies: replies}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.Len(t, chat.calls, len(replies))
			for _, series := range got.Series {
				assert.Equal(t, lockedOutfit, series.Outfit)
				for _, shot := range series.Shots {
					assert.Empty(t, shot.Wear)
					prompt := FramePrompt("SDXL", "", "a person", series, shot)
					assert.Contains(t, prompt, lockedOutfit)
					assert.NotContains(t, prompt, "silver bikini")
					assert.NotContains(t, prompt, "<lora:")
				}
			}
		})
	}
}
