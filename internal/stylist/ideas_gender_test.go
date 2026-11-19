package stylist

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var genderCases = []struct {
	name   string
	gender string
	want   string
}{
	{name: "man", gender: "man", want: "Person: a man"},
	{name: "male", gender: "male", want: "Person: a man"},
	{name: "man upper with spaces", gender: "  MAN ", want: "Person: a man"},
	{name: "woman", gender: "woman", want: "Person: a woman"},
	{name: "female", gender: "female", want: "Person: a woman"},
	{name: "female mixed case", gender: " Female", want: "Person: a woman"},
	{name: "person", gender: "person", want: "Person: a person"},
	{name: "nonbinary", gender: "nonbinary", want: "Person: a person"},
	{name: "empty", gender: "", want: ""},
}

func personLines(msg string) []string {
	var out []string
	for _, l := range strings.Split(msg, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Person:") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func TestIdeaUserMessage_Gender(t *testing.T) {
	for _, tt := range genderCases {
		t.Run(tt.name, func(t *testing.T) {
			req := PlanRequest{Frames: 8, Wish: "summer by the sea", Themes: []string{"beach"}, Gender: tt.gender}
			msg := ideaUserMessage(req, 2)
			if tt.want == "" {
				assert.Empty(t, personLines(msg), msg)
			} else {
				assert.Equal(t, []string{tt.want}, personLines(msg), msg)
			}
			assert.Contains(t, msg, "Wish: summer by the sea")
			assert.Contains(t, msg, "Themes:")
			assert.Contains(t, msg, "beach")
			assert.Contains(t, msg, "Series: 2")
		})
	}
}

func TestIdeaUserMessage_GenderKeepsOtherLines(t *testing.T) {
	req := PlanRequest{Frames: 8, Wish: "office", Themes: []string{"beach"}}
	base := ideaUserMessage(req, 2)
	req.Gender = "woman"
	with := ideaUserMessage(req, 2)
	var rest []string
	for _, l := range strings.Split(with, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "Person:") {
			rest = append(rest, l)
		}
	}
	assert.Equal(t, base, strings.Join(rest, "\n"))
}

func TestAskLLM_GenderInUserMessage(t *testing.T) {
	b := mustBase(t)
	for _, tt := range genderCases {
		t.Run(tt.name, func(t *testing.T) {
			chat := &fakeChat{reply: `{"series":[{"hair":"bun"}]}`}
			req := PlanRequest{Frames: 1, Wish: "office", Gender: tt.gender}
			_, _, err := AskLLM(chat, "m", req, b, 1)
			require.NoError(t, err)
			require.Equal(t, 1, chat.calls)
			if tt.want == "" {
				assert.Empty(t, personLines(chat.user), chat.user)
			} else {
				assert.Equal(t, []string{tt.want}, personLines(chat.user), chat.user)
			}
			assert.Contains(t, chat.user, "office")
		})
	}
}
