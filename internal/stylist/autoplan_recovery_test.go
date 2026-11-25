package stylist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const recoveryMaxAttempts = 2

func recoveryShot(pose, prompt, framing string) map[string]any {
	return map[string]any{"pose": pose, "prompt": prompt, "framing": framing, "landscape": false}
}

func recoveryJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	require.NoError(t, err)
	return string(b)
}

func TestSuggestRecoveryReplacesWholeInvalidShotAndPreservesNeighbors(t *testing.T) {
	for _, invalid := range []struct{ name, prompt, framing string }{
		{"garment close", "close-up of fabric buttons", "close"},
		{"garment waist", "waist-up portrait near window", "waist"},
		{"mislabelled garment close-up", "close-up of fabric buttons", "full"},
		{"mislabelled garment waist crop", "Portrait cropped at the waist, hands on hips", "full"},
		{"mislabelled garment headshot", "Tight headshot, looking at the camera", "full"},
		{"unknown framing", "standing near a window", "unknown"},
		{"empty prompt", "", "full"},
		{"invented appearance", "a blonde woman standing near a window", "full"},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			req := autoTestRequest()
			const requestedFrames = 3
			req.Plan.Frames = requestedFrames
			req.Plan.Garment = true
			req.GarmentOutfit = "a red wool coat"
			before := recoveryShot("Before", "standing beside a chair, looking aside", "full")
			after := recoveryShot("After", "walking along the wall, arms relaxed", "knee")
			bad := recoveryShot("Rejected original pose", invalid.prompt, invalid.framing)
			replacement := recoveryShot("New photographic pose", "turning sideways with one hand resting on a chair", "knee")
			chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) {
				autoTestSeries(v)["shots"] = []any{before, bad, after}
			}), recoveryJSON(t, replacement)}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.Len(t, chat.calls, 2)
			require.Len(t, got.Series[0].Shots, requestedFrames)
			shots := got.Series[0].Shots
			assert.Equal(t, before["prompt"], shots[0].Prompt)
			assert.Equal(t, before["pose"], shots[0].Pose)
			assert.Equal(t, replacement["prompt"], shots[1].Prompt)
			assert.Equal(t, replacement["pose"], shots[1].Pose)
			assert.Equal(t, replacement["framing"], shots[1].Framing)
			assert.Equal(t, after["prompt"], shots[2].Prompt)
			assert.Equal(t, after["pose"], shots[2].Pose)
			assert.NotEmpty(t, got.Warnings)
			var input struct {
				Existing []Shot `json:"existing_shots"`
			}
			require.NoError(t, json.Unmarshal([]byte(chat.calls[1].user), &input))
			require.Len(t, input.Existing, 2)
			assert.Equal(t, before["prompt"], input.Existing[0].Prompt)
			assert.Equal(t, after["prompt"], input.Existing[1].Prompt)
			assert.NotContains(t, chat.calls[1].user, "Rejected original pose")
			assertContextBudget(t, chat.calls[1])
		})
	}
}

func TestSuggestRecoveryRetriesInvalidReplyWithinSlotBudget(t *testing.T) {
	for _, initialMissing := range []bool{false, true} {
		t.Run(map[bool]string{true: "missing", false: "invalid"}[initialMissing], func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Garment = true
			req.GarmentOutfit = "a red wool coat"
			bad := recoveryShot("Rejected", "close-up of the coat collar", "close")
			chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) {
				shots := []any{bad}
				if initialMissing {
					shots = []any{}
				}
				autoTestSeries(v)["shots"] = shots
			}), recoveryJSON(t, bad), singleShotJSON(t, 1)}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.Len(t, chat.calls, 1+recoveryMaxAttempts)
			assert.Equal(t, "Pose 1", got.Series[0].Shots[0].Pose)
			assert.NotEmpty(t, got.Warnings)
			for _, call := range chat.calls[1:] {
				assert.NotContains(t, call.user, "close-up of the coat collar")
			}
		})
	}
}

func TestSuggestRecoveryExhaustionNeverReturnsPartialPlan(t *testing.T) {
	for _, invalid := range []string{
		"not json",
		recoveryJSON(t, recoveryShot("Bad", "close-up of fabric", "close")),
		recoveryJSON(t, recoveryShot("Bad", "a blonde woman standing beside a chair", "full")),
		recoveryJSON(t, recoveryShot("Bad", "standing beside a chair", "unknown")),
	} {
		t.Run(invalid, func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Garment = true
			req.GarmentOutfit = "a red wool coat"
			chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) {
				autoTestSeries(v)["shots"] = []any{recoveryShot("Bad", "close-up of fabric", "close")}
			}), invalid, invalid}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			assert.Error(t, err)
			assert.Nil(t, got)
			require.Len(t, chat.calls, 1+recoveryMaxAttempts)
		})
	}
}

func TestSuggestRecoveryNetworkFailureDoesNotRetry(t *testing.T) {
	chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) {
		autoTestSeries(v)["shots"] = []any{recoveryShot("Bad", "standing beside a chair", "unknown")}
	})}, failureAt: 2}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	assert.ErrorContains(t, err, "supplement unavailable")
	assert.Nil(t, got)
	require.Len(t, chat.calls, 2)
}

func TestSuggestRecoveryNeverOverridesManualPose(t *testing.T) {
	base := mustBase(t)
	for _, mode := range []Mode{ModeKeep, ModePick} {
		t.Run(string(mode), func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Aspects = map[string]AspectChoice{"pose": {Mode: mode, IDs: []string{base.Pose[0].ID}}}
			chat := &scriptedChat{replies: []string{autoTestReply(t, func(v map[string]any) {
				autoTestSeries(v)["shots"] = []any{recoveryShot("Rejected", "", "unknown")}
			})}}
			got, err := Suggest(chat, "planner", req, base, autoTestOptions())
			require.NoError(t, err)
			require.Len(t, chat.calls, 1)
			if mode == ModePick {
				assert.Equal(t, base.Pose[0].Prompt, got.Series[0].Shots[0].Prompt)
				assert.Equal(t, base.Pose[0].Framing, got.Series[0].Shots[0].Framing)
			} else {
				assert.Empty(t, got.Series[0].Shots[0].Prompt)
			}
		})
	}
}

func TestSuggestSeriesRecoveryUsesOnlyValidReference(t *testing.T) {
	for _, invalid := range []struct {
		name    string
		change  func(*Series)
		garment bool
	}{
		{"automatic hair identity", func(s *Series) { s.Hair = "Wavy Brown Hair" }, false},
		{"automatic outfit identity", func(s *Series) { s.Outfit = "a blonde woman in a coat" }, false},
		{"direction identity", func(s *Series) { s.Direction = "portrait of a blonde woman" }, false},
		{"unknown place", func(s *Series) { s.Place = "unknown-place" }, false},
		{"garment tight direction", func(s *Series) { s.Direction = "close-up of fabric" }, true},
		{"garment tight background", func(s *Series) { s.Background = "close-up of fabric" }, true},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			var fixture SuggestResult
			require.NoError(t, json.Unmarshal([]byte(autoTestReply(t, nil)), &fixture))
			valid := fixture.Series[0]
			valid.Shots[0].PromptRu = "Сохранённый перевод"
			valid.Shots[0].Wear = "retained wardrobe note"
			bad := valid
			invalid.change(&bad)
			for _, withValidNeighbor := range []bool{false, true} {
				plan := fixture
				plan.Series = []Series{{}, bad}
				if withValidNeighbor {
					plan.Series = append(plan.Series, valid)
				}
				before := recoveryJSON(t, plan)
				req := autoTestRequest()
				req.Plan.Garment = invalid.garment
				requestBefore := recoveryJSON(t, req)
				chat := &scriptedChat{replies: []string{recoveryJSON(t, valid)}}
				_, err := requestSuggestSeries(chat, "planner", req, &plan, valid, mustBase(t), 0, "invalid series", autoTestOptions()[0])
				require.NoError(t, err)
				require.Len(t, chat.calls, 1)
				var input struct {
					Reference *suggestPromptSeries `json:"reference_series"`
				}
				require.NoError(t, json.Unmarshal([]byte(chat.calls[0].user), &input))
				if withValidNeighbor {
					require.NotNil(t, input.Reference)
					assert.Equal(t, valid.Hair, input.Reference.Hair)
					assert.Equal(t, valid.Outfit, input.Reference.Outfit)
					assert.Equal(t, valid.Place, input.Reference.Place)
				} else {
					assert.Nil(t, input.Reference)
				}
				assert.NotContains(t, chat.calls[0].user, "Wavy Brown Hair")
				assertContextBudget(t, chat.calls[0])
				assert.Equal(t, before, recoveryJSON(t, plan), "reference selection must not mutate the current plan")
				assert.Equal(t, requestBefore, recoveryJSON(t, req))
			}
		})
	}
}

func TestSuggestSeriesRecoveryPreservesManualIdentityReference(t *testing.T) {
	const manualHair = "Wavy Brown Hair"
	var plan SuggestResult
	require.NoError(t, json.Unmarshal([]byte(autoTestReply(t, nil)), &plan))
	locked := plan.Series[0]
	locked.Hair = manualHair
	plan.Series = []Series{{}, locked}
	req := autoTestRequest()
	req.Plan.Aspects = map[string]AspectChoice{"hair": {Mode: ModeCustom, Text: manualHair}}
	chat := &scriptedChat{replies: []string{recoveryJSON(t, locked)}}
	got, err := requestSuggestSeries(chat, "planner", req, &plan, locked, mustBase(t), 0, "missing series", autoTestOptions()[0])
	require.NoError(t, err)
	assert.Equal(t, manualHair, got.Hair)
	var input struct {
		Reference *suggestPromptSeries `json:"reference_series"`
	}
	require.NoError(t, json.Unmarshal([]byte(chat.calls[0].user), &input))
	require.NotNil(t, input.Reference)
	assert.Equal(t, manualHair, input.Reference.Hair)
	assertContextBudget(t, chat.calls[0])
}

func TestSuggestSeriesRecoveryInvalidReferencesStillExhaustBudget(t *testing.T) {
	const inventedHair = "Wavy Brown Hair"
	var plan SuggestResult
	require.NoError(t, json.Unmarshal([]byte(autoTestReply(t, nil)), &plan))
	valid := plan.Series[0]
	invalid := valid
	invalid.Hair = inventedHair
	plan.Series = []Series{invalid, invalid}
	skeleton := []Series{valid, valid}
	skeletonBefore := recoveryJSON(t, skeleton)
	chat := &scriptedChat{replies: []string{recoveryJSON(t, invalid), recoveryJSON(t, invalid)}}
	_, err := completeSuggestSeries(chat, "planner", autoTestRequest(), &plan, skeleton, mustBase(t), autoTestOptions()[0])
	require.Error(t, err)
	require.Len(t, chat.calls, recoveryMaxAttempts)
	for _, call := range chat.calls {
		var input struct {
			Reference *suggestPromptSeries `json:"reference_series"`
		}
		require.NoError(t, json.Unmarshal([]byte(call.user), &input))
		assert.Nil(t, input.Reference)
		assertContextBudget(t, call)
	}
	assert.Equal(t, skeletonBefore, recoveryJSON(t, skeleton), "repair exhaustion must preserve locked constraints")
	assert.Equal(t, inventedHair, plan.Series[0].Hair, "failed repairs must not install a fallback")
	assert.Equal(t, inventedHair, plan.Series[1].Hair)
}

func TestSuggestSeriesRecoveryPreservesManualGarmentBackgroundReference(t *testing.T) {
	const manualBackground = "close-up of fabric"
	var plan SuggestResult
	require.NoError(t, json.Unmarshal([]byte(autoTestReply(t, nil)), &plan))
	locked := plan.Series[0]
	locked.Background = manualBackground
	plan.Series = []Series{{}, locked}
	before := recoveryJSON(t, plan)
	req := autoTestRequest()
	req.Plan.Garment = true
	req.Plan.Aspects = map[string]AspectChoice{"background": {Mode: ModeCustom, Text: manualBackground}}
	chat := &scriptedChat{replies: []string{recoveryJSON(t, locked)}}
	got, err := requestSuggestSeries(chat, "planner", req, &plan, locked, mustBase(t), 0, "missing series", autoTestOptions()[0])
	require.NoError(t, err)
	assert.Equal(t, manualBackground, got.Background)
	var input struct {
		Reference *suggestPromptSeries `json:"reference_series"`
	}
	require.NoError(t, json.Unmarshal([]byte(chat.calls[0].user), &input))
	require.NotNil(t, input.Reference)
	assert.Equal(t, manualBackground, input.Reference.Background)
	assert.Equal(t, before, recoveryJSON(t, plan))
	assertContextBudget(t, chat.calls[0])
}
