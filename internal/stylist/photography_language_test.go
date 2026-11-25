package stylist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type photographyLanguageChat struct {
	t          *testing.T
	replies    []string
	generation []contextChatCall
	display    proposalDisplayChat
}

func (c *photographyLanguageChat) Chat(model, system, user string, temperature float64, tokens int) (string, error) {
	var payload map[string]any
	require.NoError(c.t, json.Unmarshal([]byte(user), &payload))
	if _, ok := payload["texts"]; ok {
		if len(c.display.calls) == 0 {
			c.display.calls = append(c.display.calls, contextChatCall{})
		}
		return c.display.Chat(model, system, user, temperature, tokens)
	}
	c.generation = append(c.generation, contextChatCall{system: system, user: user, maxTokens: tokens})
	require.NotEmpty(c.t, c.replies, "unexpected generation call")
	reply := c.replies[0]
	c.replies = c.replies[1:]
	return reply, nil
}

func TestPhotographyRussianDisplayUsesCanonicalEnglishPlannerAndRecovery(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		name := "main"
		if recovery {
			name = "series_recovery"
		}
		t.Run(name, func(t *testing.T) {
			req := autoTestRequest()
			req.Plan.Locale = "ru"
			req.Target = autoTestTarget
			replies := []string{autoTestReply(t, nil)}
			if recovery {
				req.Plan.Frames = seriesRecoveryFrames
				initial, missing := seriesRecoveryReplies(t)
				replies = []string{initial, recoveryJSON(t, missing)}
			}
			before, err := json.Marshal(req)
			require.NoError(t, err)
			chat := &photographyLanguageChat{t: t, replies: replies, display: proposalDisplayChat{t: t}}
			got, err := Suggest(chat, "planner", req, mustBase(t), autoTestOptions())
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Len(t, chat.generation, len(replies))
			for _, call := range chat.generation {
				var input struct {
					Request struct {
						Plan struct {
							Locale string `json:"locale"`
						} `json:"plan"`
					} `json:"request"`
				}
				require.NoError(t, json.Unmarshal([]byte(call.user), &input))
				assert.Equal(t, "en", input.Request.Plan.Locale, "canonical generation context must not ask for Russian prose")
				assertContextBudget(t, call)
			}
			assert.Equal(t, "Night portrait in the city", got.Concept)
			assert.Equal(t, "Hard side light", got.Light)
			assert.Contains(t, got.ConceptRu, "Русский перевод:")
			assert.Contains(t, got.Series[0].DirectionRu, "Русский перевод:")
			assert.Equal(t, "Hard side light, deep shadows, crisp fabric detail", got.Series[0].Direction)
			after, err := json.Marshal(req)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}
