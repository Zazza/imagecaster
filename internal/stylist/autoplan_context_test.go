package stylist

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextChatCall struct {
	system, user string
	maxTokens    int
}

type contextRecordingChat struct {
	reply string
	calls []contextChatCall
}

const contextWindowTokens = 8192
const contextMessageOverhead = 256
const contextSelectionResponseTokens = 96

func assertContextBudget(t *testing.T, call contextChatCall) {
	t.Helper()
	assert.LessOrEqual(t, len(call.system)+len(call.user)+call.maxTokens+contextMessageOverhead, contextWindowTokens)
}

func (c *contextRecordingChat) Chat(model, system, user string, temperature float64, maxTokens int) (string, error) {
	c.calls = append(c.calls, contextChatCall{system: system, user: user, maxTokens: maxTokens})
	return c.reply, nil
}

func contextLargeCatalog() []ShootOption {
	const optionCount = 100
	const descriptionRepeats = 100
	options := make([]ShootOption, 0, optionCount+1)
	for i := 0; i < optionCount; i++ {
		options = append(options, ShootOption{ID: fmt.Sprintf("preset:%d", i), Name: "Unrelated catalog marker", Description: strings.Repeat("irrelevant description ", descriptionRepeats)})
	}
	return append(options, autoTestOptions()...)
}

func TestSuggestLockedTargetOmitsUnrelatedCatalogAndKeepsConstraints(t *testing.T) {
	req := autoTestRequest()
	req.Target = autoTestTarget
	req.Plan.Aspects = map[string]AspectChoice{"hair": {Mode: ModeCustom, Text: "neat braided bun", TextRu: "аккуратный пучок"}}
	before, err := json.Marshal(req)
	require.NoError(t, err)
	chat := &contextRecordingChat{reply: autoTestReply(t, nil)}
	got, err := Suggest(chat, "planner", req, mustBase(t), contextLargeCatalog())
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotEmpty(t, chat.calls)
	for _, call := range chat.calls {
		assertContextBudget(t, call)
		assert.NotContains(t, call.user, "Unrelated catalog marker")
		assert.NotContains(t, call.user, "irrelevant description")
	}
	assert.Contains(t, chat.calls[len(chat.calls)-1].user, "neat braided bun")
	assert.Equal(t, "neat braided bun", got.Series[0].Hair)
	assert.Equal(t, autoTestTarget, got.Target)
	t.Logf("locked target calls=%d complete context=%d", len(chat.calls), len(chat.calls[0].system)+len(chat.calls[0].user)+chat.calls[0].maxTokens+contextMessageOverhead)
	after, err := json.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func contextReplyForFrames(t *testing.T, frames int) string {
	return autoTestReply(t, func(v map[string]any) {
		template := autoTestSeries(v)
		shot := template["shots"].([]any)[0]
		series := make([]any, 0)
		for _, count := range SeriesSizes(frames) {
			item := make(map[string]any, len(template))
			for k, val := range template {
				item[k] = val
			}
			shots := make([]any, count)
			for i := range shots {
				shots[i] = shot
			}
			item["shots"] = shots
			series = append(series, item)
		}
		v["series"] = series
	})
}

func TestSuggestResponseBudgetScalesAndSupportsEveryFrameCount(t *testing.T) {
	var smallestBudget, largestBudget int
	for frames := 1; frames <= MaxFrames; frames++ {
		req := autoTestRequest()
		req.Plan.Frames = frames
		chat := &contextRecordingChat{reply: contextReplyForFrames(t, frames)}
		got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
		require.NoError(t, err, "frames=%d", frames)
		count := 0
		for _, series := range got.Series {
			count += len(series.Shots)
		}
		assert.Equal(t, frames, count)
		require.NotEmpty(t, chat.calls)
		budget := chat.calls[len(chat.calls)-1].maxTokens
		for _, call := range chat.calls {
			assertContextBudget(t, call)
		}
		if frames == 1 {
			smallestBudget = budget
		}
		if frames == MaxFrames {
			largestBudget = budget
		}
	}
	assert.Greater(t, largestBudget, smallestBudget, "reserve output according to requested frame count")
}

type contextCatalogChat struct {
	t                *testing.T
	calls            []contextChatCall
	invalidSelection bool
}

func (c *contextCatalogChat) Chat(model, system, user string, temperature float64, maxTokens int) (string, error) {
	call := contextChatCall{system: system, user: user, maxTokens: maxTokens}
	c.calls = append(c.calls, call)
	assertContextBudget(c.t, call)
	if strings.Contains(system, `"series"`) {
		return autoTestReply(c.t, nil), nil
	}
	if c.invalidSelection {
		return `{"target":"missing:target"}`, nil
	}
	var input struct {
		Options []ShootOption `json:"options"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(user), &input), "selection request must remain complete JSON")
	require.NotEmpty(c.t, input.Options)
	reply, err := json.Marshal(map[string]string{"target": input.Options[len(input.Options)-1].ID, "reason": "Studio portrait matches the photographic request"})
	require.NoError(c.t, err)
	return string(reply), nil
}

func TestSuggestLargeCatalogUsesBaseFallbackWithinBudget(t *testing.T) {
	options := contextLargeCatalog()
	before, err := json.Marshal(options)
	require.NoError(t, err)
	chat := &contextCatalogChat{t: t}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), options)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, autoTestTarget, got.Target)
	allUsers := ""
	maxContext := 0
	for _, call := range chat.calls {
		allUsers += call.user
		maxContext = max(maxContext, len(call.system)+len(call.user)+call.maxTokens+contextMessageOverhead)
	}
	t.Logf("catalog options=%d calls=%d maximum complete context=%d", len(options), len(chat.calls), maxContext)
	assert.Contains(t, allUsers, autoTestTarget)
	assert.NotContains(t, allUsers, "Unrelated catalog marker")
	assert.NotContains(t, allUsers, "preset:")
	after, err := json.Marshal(options)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestSuggestRejectsInvalidCatalogSelectionWithoutFallback(t *testing.T) {
	chat := &contextCatalogChat{t: t, invalidSelection: true}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), photographyOptions(t, 2))
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestSuggestRejectsIndivisibleOversizedConstraintBeforeAI(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Frames = MaxFrames
	const customConstraintLength = 490
	const wishLength = 900
	req.Plan.Wish = strings.Repeat("w", wishLength)
	req.Plan.Aspects = make(map[string]AspectChoice)
	for _, aspect := range []string{"outfit", "hair", "makeup", "background"} {
		req.Plan.Aspects[aspect] = AspectChoice{Mode: ModeCustom, Text: strings.Repeat("a", customConstraintLength)}
	}
	before, err := json.Marshal(req)
	require.NoError(t, err)
	chat := &contextRecordingChat{reply: autoTestReply(t, nil)}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, chat.calls, "oversized source must not be silently truncated or sent")
	after, err := json.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestSuggestManyLockedSeriesBoundOrRejectBeforeAI(t *testing.T) {
	base := mustBase(t)
	require.GreaterOrEqual(t, len(base.Outfit), MaxFrames)
	ids := make([]string, MaxFrames)
	for i := range ids {
		ids[i] = base.Outfit[i].ID
	}
	req := autoTestRequest()
	req.Plan.Frames = MaxFrames
	req.Plan.Aspects = map[string]AspectChoice{"outfit": {Mode: ModePick, IDs: ids}}
	before, err := json.Marshal(req)
	require.NoError(t, err)
	chat := &contextRecordingChat{reply: contextReplyForFrames(t, MaxFrames)}
	got, err := Suggest(chat, "planner", req, base, autoTestOptions())
	for _, call := range chat.calls {
		assertContextBudget(t, call)
		assert.True(t, json.Valid([]byte(call.user)), "every request is complete JSON")
	}
	if err != nil {
		assert.Nil(t, got)
		assert.Empty(t, chat.calls, "indivisible constraints rejected locally")
	} else {
		require.NotNil(t, got)
		require.Len(t, got.Series, MaxFrames)
		for i, series := range got.Series {
			assert.Equal(t, base.Outfit[i].Prompt, series.Outfit)
		}
	}
	after, err := json.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

type contextTranslationChat struct {
	t     *testing.T
	calls []contextChatCall
}

func (c *contextTranslationChat) Chat(model, system, user string, temperature float64, maxTokens int) (string, error) {
	c.calls = append(c.calls, contextChatCall{system: system, user: user, maxTokens: maxTokens})
	assertContextBudget(c.t, c.calls[len(c.calls)-1])
	if len(c.calls) == 1 {
		return `{"texts":["neat neutral styling","neat neutral styling","neat neutral styling","neat neutral styling"]}`, nil
	}
	if len(c.calls) == 2 {
		return autoTestReply(c.t, nil), nil
	}
	return displayTranslationReply(user)
}

func TestSuggestBoundsTranslationAndPreservesRawRussianConstraints(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Locale = "ru"
	req.Plan.Aspects = make(map[string]AspectChoice)
	const russianConstraintRepeats = 45
	for _, aspect := range []string{"outfit", "hair", "makeup", "background"} {
		req.Plan.Aspects[aspect] = AspectChoice{Mode: ModeCustom, Text: strings.Repeat("аккуратно ", russianConstraintRepeats)}
	}
	before, err := json.Marshal(req)
	require.NoError(t, err)
	chat := &contextTranslationChat{t: t}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	if err != nil {
		assert.Nil(t, got)
		assert.Empty(t, chat.calls, "oversized translation must fail before external call")
	} else {
		require.NotNil(t, got)
		require.Greater(t, len(chat.calls), 2)
		assert.Contains(t, chat.calls[0].user, "аккуратно")
		assert.Contains(t, chat.calls[1].user, "neat neutral styling")
		for _, call := range chat.calls {
			assertContextBudget(t, call)
		}
		assert.Equal(t, "neat neutral styling", got.Series[0].Hair)
	}
	after, err := json.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}
