package stylist

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Display translation is deliberately derived from the caller's text list, so
// batching and ordering may change without prescribing the implementation.
type proposalDisplayChat struct {
	t               *testing.T
	calls           []contextChatCall
	failTranslation bool
}

func (c *proposalDisplayChat) Chat(model, system, user string, temperature float64, budget int) (string, error) {
	c.calls = append(c.calls, contextChatCall{system: system, user: user, maxTokens: budget})
	if len(c.calls) == 1 {
		return autoTestReply(c.t, nil), nil
	}
	if c.failTranslation {
		return "", errors.New("display translator unavailable")
	}
	var request struct {
		Texts []string `json:"texts"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(user), &request))
	require.NotEmpty(c.t, request.Texts)
	translated := make([]string, len(request.Texts))
	for i := range translated {
		translated[i] = "Русский перевод: " + request.Texts[i]
	}
	data, err := json.Marshal(map[string]any{"texts": translated})
	return string(data), err
}

func TestProposalDisplayRussianTranslationDoesNotChangeGeneration(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Locale = "ru"
	chat := &proposalDisplayChat{t: t}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Greater(t, len(chat.calls), 1, "Russian display must be translated after the English plan is valid")
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	var display map[string]any
	require.NoError(t, json.Unmarshal(encoded, &display))
	for _, field := range []string{"concept_ru", "light_ru", "focus_ru"} {
		assert.Contains(t, display[field], "Русский перевод:", field)
	}
	series := display["series"].([]any)[0].(map[string]any)
	for _, field := range []string{"direction_ru", "outfit_ru", "hair_ru", "makeup_ru", "background_ru"} {
		assert.Contains(t, series[field], "Русский перевод:", field)
	}
	shot := series["shots"].([]any)[0].(map[string]any)
	assert.Contains(t, shot["prompt_ru"], "Русский перевод:")
	prompt := FramePrompt("SDXL", "", "a person", got.Series[0], got.Series[0].Shots[0])
	assert.Contains(t, prompt, "Hard side light")
	assert.NotContains(t, prompt, "Русский перевод:")
	for _, call := range chat.calls {
		assertContextBudget(t, call)
	}
}

func TestProposalDisplayTranslationFailureKeepsValidEnglishPlan(t *testing.T) {
	req := autoTestRequest()
	req.Plan.Locale = "ru"
	chat := &proposalDisplayChat{t: t, failTranslation: true}
	got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Night portrait in the city", got.Concept)
	assert.Equal(t, "standing near a window, looking aside", got.Series[0].Shots[0].Prompt)
	assert.NotEmpty(t, got.Warnings, "display fallback must be explained")
}

func TestProposalDisplayRejectsPlaceholderAutomaticDetails(t *testing.T) {
	for _, field := range []string{"outfit", "hair", "makeup", "background", "direction"} {
		for _, placeholder := range []string{"auto", "English photographic light, composition and focus instructions"} {
			t.Run(field+"/"+placeholder, func(t *testing.T) {
				reply := autoTestReply(t, func(v map[string]any) { autoTestSeries(v)[field] = placeholder })
				chat := &fakeChat{reply: reply}
				got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
				assert.Error(t, err, "repeated placeholder answers must exhaust bounded repair")
				assert.Nil(t, got)
				assert.Greater(t, chat.calls, 1, "invalid automatic details require repair, not presentation filtering")
				assert.LessOrEqual(t, chat.calls, 1+recoveryMaxAttempts)
			})
		}
	}
}

func TestProposalDisplayAllowsConcreteEnglishGarden(t *testing.T) {
	chat := &fakeChat{reply: autoTestReply(t, func(v map[string]any) {
		autoTestSeries(v)["background"] = "English garden with rose beds and a stone path"
	})}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, strings.Contains(got.Series[0].Background, "English garden"))
	assert.Equal(t, 1, chat.calls)
}

func TestProposalDisplayRepairsPlaceholderShotPrompt(t *testing.T) {
	for _, placeholder := range []string{"auto", "English photographic light, composition and focus instructions"} {
		t.Run(placeholder, func(t *testing.T) {
			initial := autoTestReply(t, func(v map[string]any) { autoTestSeries(v)["shots"].([]any)[0].(map[string]any)["prompt"] = placeholder })
			replacement := recoveryJSON(t, recoveryShot("Window turn", "turning beside a window with hands resting on the sill", "full"))
			chat := &scriptedChat{replies: []string{initial, replacement}}
			got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Len(t, chat.calls, 2)
			assert.Equal(t, "turning beside a window with hands resting on the sill", got.Series[0].Shots[0].Prompt)
			assert.NotEmpty(t, got.Warnings)
		})
	}
}

func displayTranslationReply(user string) (string, error) {
	var request struct {
		Texts []string `json:"texts"`
	}
	if err := json.Unmarshal([]byte(user), &request); err != nil {
		return "", err
	}
	for i, text := range request.Texts {
		request.Texts[i] = "Русский перевод: " + text
	}
	encoded, err := json.Marshal(request)
	return string(encoded), err
}

func TestProposalDisplaySchemaUsesConcreteEscapedTarget(t *testing.T) {
	const escapedTarget = "preset:quoted\"target\\path"
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic", true: "pinned"}[locked], func(t *testing.T) {
			req := autoTestRequest()
			if locked {
				req.Target = escapedTarget
			}
			chat := &contextRecordingChat{reply: autoTestReply(t, func(v map[string]any) { v["target"] = escapedTarget })}
			options := []ShootOption{{ID: escapedTarget, Name: "Quoted model", Description: "Studio photography"}}
			got, err := Suggest(chat, "planner", req, mustBase(t), options)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, escapedTarget, got.Target)
			require.NotEmpty(t, chat.calls)
			encodedTarget, err := json.Marshal(escapedTarget)
			require.NoError(t, err)
			final := chat.calls[len(chat.calls)-1]
			assert.Contains(t, final.system, `"target":`+string(encodedTarget))
			assert.NotContains(t, final.system, "selected option id")
			for _, call := range chat.calls {
				assertContextBudget(t, call)
			}
		})
	}
}

func TestProposalDisplayFinalSchemaUsesPagedCatalogWinner(t *testing.T) {
	const candidateCount = 32
	const modelDescriptionRepeats = 40
	options := photographyOptions(t, candidateCount)
	for i := range options {
		options[i].ModelDescription = strings.Repeat("Photographic model retains natural skin and fabric texture. ", modelDescriptionRepeats)
	}
	chat := &contextCatalogChat{t: t}
	got, err := Suggest(chat, "planner", autoTestRequest(), mustBase(t), options)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, options[len(options)-1].ID, got.Target)
	require.Greater(t, len(chat.calls), 2, "catalog must select page winners before final planning")
	final := chat.calls[len(chat.calls)-1]
	encodedTarget, err := json.Marshal(got.Target)
	require.NoError(t, err)
	assert.Contains(t, final.system, `"target":`+string(encodedTarget))
	assert.Contains(t, final.user, string(encodedTarget))
	assert.NotContains(t, final.system, "selected option id")
	for _, call := range chat.calls {
		assertContextBudget(t, call)
	}
}
