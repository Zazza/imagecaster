package stylist

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hairRepairSeries(t *testing.T, hair string) string {
	t.Helper()
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(autoTestReply(t, nil)), &plan))
	series := autoTestSeries(plan)
	series["hair"] = hair
	return recoveryJSON(t, series)
}

func assertHairRepairFeedback(t *testing.T, call contextChatCall, rejected, matched string) {
	t.Helper()
	var input struct {
		Feedback *struct {
			Field    string `json:"field"`
			Rejected string `json:"rejected_text"`
			Matched  string `json:"matched_text"`
		} `json:"repair_feedback"`
		Reference *suggestPromptSeries `json:"reference_series"`
	}
	require.NoError(t, json.Unmarshal([]byte(call.user), &input))
	require.NotNil(t, input.Feedback, "repair must identify rejected data instead of repeating a generic prohibition")
	assert.Equal(t, "hair", input.Feedback.Field)
	assert.Equal(t, rejected, input.Feedback.Rejected)
	assert.Equal(t, matched, strings.ToLower(input.Feedback.Matched))
	assert.Nil(t, input.Reference, "invalid series is feedback data, never a valid reference")
	assertContextBudget(t, call)
}

func TestSuggestHairRepairReportsRejectedValueAndLatestFailure(t *testing.T) {
	const initialHair = "Wavy Brown Hair"
	const retryHair = "Blonde Waves"
	const validHair = "soft waves tucked behind ears"
	for _, retryInvalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "first repair succeeds", true: "latest rejection replaces initial feedback"}[retryInvalid], func(t *testing.T) {
			replies := []string{autoTestReply(t, func(plan map[string]any) { autoTestSeries(plan)["hair"] = initialHair })}
			if retryInvalid {
				replies = append(replies, hairRepairSeries(t, retryHair))
			}
			replies = append(replies, hairRepairSeries(t, validHair))
			chat := &scriptedChat{replies: replies}
			got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.Len(t, chat.calls, len(replies))
			assert.Equal(t, validHair, got.Series[0].Hair)
			assertHairRepairFeedback(t, chat.calls[1], initialHair, "brown hair")
			if retryInvalid {
				assertHairRepairFeedback(t, chat.calls[2], retryHair, "blonde")
				assert.NotContains(t, chat.calls[2].user, initialHair)
			}
		})
	}
}

func TestSuggestHairRepairRepeatedIdentityFailureReturnsNoFallback(t *testing.T) {
	const invalidHair = "Wavy Brown Hair"
	chat := &scriptedChat{replies: []string{
		autoTestReply(t, func(plan map[string]any) { autoTestSeries(plan)["hair"] = invalidHair }),
		hairRepairSeries(t, invalidHair), hairRepairSeries(t, invalidHair),
	}}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	require.Error(t, err)
	assert.ErrorContains(t, err, "Brown Hair", "the final error must identify the rejected fragment")
	assert.Nil(t, got)
	require.Len(t, chat.calls, 1+recoveryMaxAttempts)
	for _, call := range chat.calls[1:] {
		assertHairRepairFeedback(t, call, invalidHair, "brown hair")
	}
}

func TestSuggestHairRepairOversizedFeedbackNeverSendsOversizedRequest(t *testing.T) {
	const feedbackRepeats = contextWindowTokens
	invalidHair := "Wavy Brown Hair " + strings.Repeat("waves ", feedbackRepeats)
	chat := &scriptedChat{replies: []string{
		autoTestReply(t, func(plan map[string]any) { autoTestSeries(plan)["hair"] = invalidHair }),
		hairRepairSeries(t, "low neat bun"),
	}}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	require.Error(t, err, "exact rejected text cannot fit the repair context")
	assert.Nil(t, got)
	require.Len(t, chat.calls, 1, "oversized feedback must be rejected before the repair Chat call")
	for _, call := range chat.calls {
		assertContextBudget(t, call)
	}
}

func TestSuggestHairRepairKeepsValidNeighborAsReference(t *testing.T) {
	const rejectedHair = "Wavy Brown Hair"
	const neighborHair = "neat braided bun"
	req := autoTestRequest()
	req.Plan.Frames = seriesRecoveryFrames
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(contextReplyForFrames(t, seriesRecoveryFrames)), &plan))
	series := plan["series"].([]any)
	series[0].(map[string]any)["hair"] = rejectedHair
	series[1].(map[string]any)["hair"] = neighborHair
	replacement := make(map[string]any)
	for key, value := range series[0].(map[string]any) {
		replacement[key] = value
	}
	replacement["hair"] = "low neat bun"
	chat := &scriptedChat{replies: []string{recoveryJSON(t, plan), recoveryJSON(t, replacement)}}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.Len(t, chat.calls, 2)
	assert.Equal(t, neighborHair, got.Series[1].Hair)
	var input struct {
		Reference *suggestPromptSeries `json:"reference_series"`
		Feedback  map[string]string    `json:"repair_feedback"`
	}
	require.NoError(t, json.Unmarshal([]byte(chat.calls[1].user), &input))
	require.NotNil(t, input.Reference)
	assert.Equal(t, neighborHair, input.Reference.Hair)
	assert.Equal(t, rejectedHair, input.Feedback["rejected_text"])
	assert.Equal(t, "hair", input.Feedback["field"])
	assertContextBudget(t, chat.calls[1])
}
