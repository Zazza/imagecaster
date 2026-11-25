package stylist

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedChat struct {
	replies   []string
	failureAt int
	calls     []contextChatCall
}

func (c *scriptedChat) Chat(model, system, user string, temperature float64, maxTokens int) (string, error) {
	c.calls = append(c.calls, contextChatCall{system: system, user: user, maxTokens: maxTokens})
	if c.failureAt == len(c.calls) {
		return "", errors.New("supplement unavailable")
	}
	if len(c.calls) > len(c.replies) {
		return "", errors.New("unexpected additional call")
	}
	return c.replies[len(c.calls)-1], nil
}

func singleShotJSON(t *testing.T, index int) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"pose": fmt.Sprintf("Pose %d", index), "prompt": fmt.Sprintf("standing by window number %d, looking aside", index), "framing": "full", "landscape": false})
	require.NoError(t, err)
	return string(b)
}

func TestSuggestRepairsShotShortageForEveryFrameCount(t *testing.T) {
	for frames := 1; frames <= MaxFrames; frames++ {
		t.Run(fmt.Sprint(frames), func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Frames = frames
			req.Target = autoTestTarget
			req.Plan.Aspects = map[string]AspectChoice{"hair": {Mode: ModeCustom, Text: "neat braided bun"}, "makeup": {Mode: ModeKeep}}
			before, err := json.Marshal(req)
			require.NoError(t, err)
			var response map[string]any
			require.NoError(t, json.Unmarshal([]byte(contextReplyForFrames(t, frames)), &response))
			missing := 0
			for _, raw := range response["series"].([]any) {
				series := raw.(map[string]any)
				shots := series["shots"].([]any)
				missing += len(shots)
				series["shots"] = []any{}
			}
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			chat := &scriptedChat{replies: []string{string(encoded)}}
			for i := 0; i < missing; i++ {
				chat.replies = append(chat.replies, singleShotJSON(t, i))
			}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Len(t, chat.calls, missing+1)
			assert.NotEmpty(t, got.Warnings)
			require.Len(t, got.Series, len(SeriesSizes(frames)))
			next := 0
			for i, series := range got.Series {
				require.Len(t, series.Shots, SeriesSizes(frames)[i])
				for _, shot := range series.Shots {
					assert.Equal(t, fmt.Sprintf("Pose %d", next), shot.Pose)
					next++
				}
				assert.Equal(t, "neat braided bun", series.Hair)
				assert.Empty(t, series.Makeup)
			}
			for _, call := range chat.calls[1:] {
				assertContextBudget(t, call)
				for _, value := range []string{"Night portrait in the city", "Hard side light", "Person and city", "neat braided bun", autoTestTarget} {
					assert.Contains(t, call.user, value)
				}
				assert.True(t, json.Valid([]byte(call.user)), "complete supplemental request")
			}
			after, err := json.Marshal(req)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}

func TestSuggestTrimsExcessShotsBeforeValidation(t *testing.T) {
	chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) {
		s := autoTestSeries(v)
		s["shots"] = append(s["shots"].([]any), map[string]any{"framing": "invalid"})
	})}}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Series[0].Shots, 1)
	assert.Equal(t, "Standing", got.Series[0].Shots[0].Pose)
	require.Len(t, chat.calls, 1)
	assert.NotEmpty(t, got.Warnings)
}

func TestSuggestRetainsValidPrefixAndAddsSingleShot(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Frames = 2
	chat := &scriptedChat{replies: []string{autoTestReply(t, nil), singleShotJSON(t, 1)}}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Series[0].Shots, 2)
	assert.Equal(t, "Standing", got.Series[0].Shots[0].Pose)
	assert.Equal(t, "standing near a window, looking aside", got.Series[0].Shots[0].Prompt)
	assert.Equal(t, "Pose 1", got.Series[0].Shots[1].Pose)
	require.Len(t, chat.calls, 2)
}

func TestSuggestRepairsInvalidRetainedShotsBeforeSupplementing(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Frames = 2
	for _, field := range []string{"prompt", "framing"} {
		t.Run(field, func(t *testing.T) {
			chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) { autoTestSeries(v)["shots"].([]any)[0].(map[string]any)[field] = "" }), singleShotJSON(t, 1), singleShotJSON(t, 2)}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.Len(t, chat.calls, 3)
			require.Len(t, got.Series[0].Shots, req.Plan.Frames)
			assert.Equal(t, "Pose 1", got.Series[0].Shots[0].Pose)
			assert.Equal(t, "Pose 2", got.Series[0].Shots[1].Pose)
			var input struct {
				Existing []Shot `json:"existing_shots"`
			}
			require.NoError(t, json.Unmarshal([]byte(chat.calls[1].user), &input))
			assert.Empty(t, input.Existing, "rejected initial shot must not constrain its replacement")
			require.NoError(t, json.Unmarshal([]byte(chat.calls[2].user), &input))
			require.Len(t, input.Existing, 1)
			assert.Equal(t, got.Series[0].Shots[0].Prompt, input.Existing[0].Prompt)
		})
	}
}

func TestSuggestSupplementFailureReturnsNoPartialPlan(t *testing.T) {
	for _, reply := range []string{"not json", `[]`, `{"shots":[]}`, `{"pose":"Standing","prompt":"","framing":"full"}`, `{"pose":"Standing","prompt":"standing near window","framing":"invalid"}`} {
		t.Run(reply, func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Frames = 3
			chat := &scriptedChat{replies: []string{autoTestReply(t, nil), singleShotJSON(t, 1), reply, reply}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			assert.Error(t, err)
			assert.Nil(t, got)
			require.Len(t, chat.calls, 2+recoveryMaxAttempts)
		})
	}
	req := autoTestRequest()
	req.Plan.Frames = 2
	chat := &scriptedChat{replies: []string{autoTestReply(t, nil)}, failureAt: 2}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	assert.Error(t, err)
	assert.Nil(t, got)
	require.Len(t, chat.calls, 2)
}

func TestSuggestManualPosesDoNotRequestSupplement(t *testing.T) {
	base := mustBase(t)
	for _, mode := range []Mode{ModeKeep, ModePick} {
		t.Run(string(mode), func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Frames = 2
			req.Plan.Aspects = map[string]AspectChoice{"pose": {Mode: mode, IDs: []string{base.Pose[0].ID}}}
			chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) { autoTestSeries(v)["shots"] = []any{} })}}
			got, err := Suggest(chat, "planner", req, base, autoTestOptions())
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Len(t, chat.calls, 1)
			require.Len(t, got.Series[0].Shots, 2)
			for _, shot := range got.Series[0].Shots {
				if mode == ModePick {
					assert.Equal(t, base.Pose[0].Prompt, shot.Prompt)
				} else {
					assert.Empty(t, shot.Prompt)
				}
			}
		})
	}
}

func TestSuggestRepairsInvalidLaterSeriesBeforeRequestingAnyPose(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Frames = 6
	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(contextReplyForFrames(t, req.Plan.Frames)), &response))
	series := response["series"].([]any)
	series[0].(map[string]any)["shots"] = []any{}
	series[1].(map[string]any)["background"] = ""
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	chat := &scriptedChat{replies: []string{string(encoded), `{}`, `{}`}}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	assert.Error(t, err)
	assert.Nil(t, got)
	require.Len(t, chat.calls, 1+recoveryMaxAttempts)
	for _, call := range chat.calls[1:] {
		assert.NotContains(t, call.system, "Plan exactly one additional photographic pose")
		assert.Contains(t, call.system, "series")
	}
}

func TestSuggestSupplementUsesSelectedTargetAndRestoredOutfit(t *testing.T) {
	const selectedTarget = "preset:7"
	base := mustBase(t)
	req := autoTestRequest()
	req.Plan.Frames = 2
	req.Plan.Aspects = map[string]AspectChoice{"outfit": {Mode: ModePick, IDs: []string{base.Outfit[0].ID}}}
	chat := &scriptedChat{replies: []string{`{"target":"preset:7","reason":"Selected photographic style fits the request"}`, autoTestReply(t, func(v map[string]any) { v["target"] = selectedTarget }), singleShotJSON(t, 1)}}
	options := []ShootOption{
		{ID: "preset:alternative", Name: "Alternative photograph", Category: "stylist_photo"},
		{ID: selectedTarget, Name: "Compatible selected style", Category: "stylist_photo"},
	}
	got, err := Suggest(chat, "planner", req, base, options)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, chat.calls, 3)
	assert.Equal(t, selectedTarget, got.Target)
	assert.Contains(t, chat.calls[2].user, selectedTarget)
	assert.Contains(t, chat.calls[2].user, base.Outfit[0].Prompt)
	assert.Contains(t, chat.calls[2].user, "Hard side light, deep shadows, crisp fabric detail")
	assert.Contains(t, chat.calls[2].user, "standing near a window, looking aside")
}

func TestSuggestRejectsOversizedSupplementBeforeCallingAI(t *testing.T) {
	const summaryLength = 990
	const seriesTextLength = 500
	const retainedPromptLength = 550
	const requestedFrames = 4
	const retainedShots = 3
	req := autoTestRequest()
	req.Plan.Frames = requestedFrames
	reply := autoTestReply(t, func(v map[string]any) {
		for _, field := range []string{"concept", "light", "focus"} {
			v[field] = strings.Repeat("a", summaryLength)
		}
		series := autoTestSeries(v)
		for _, field := range []string{"outfit", "hair", "makeup", "background", "direction"} {
			series[field] = strings.Repeat("a", seriesTextLength)
		}
		shots := make([]any, retainedShots)
		for i := range shots {
			shots[i] = map[string]any{"pose": "Standing", "prompt": strings.Repeat("a", retainedPromptLength), "framing": "full", "landscape": false}
		}
		series["shots"] = shots
	})
	chat := &scriptedChat{replies: []string{reply, singleShotJSON(t, 1)}}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	assert.ErrorContains(t, err, "контекст автоподбора")
	assert.Nil(t, got)
	require.Len(t, chat.calls, 1)
	assertContextBudget(t, chat.calls[0])
}
