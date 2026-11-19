package stylist

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslate(t *testing.T) {
	in := []string{"красное шёлковое платье миди", "низкий пучок", "кафе у окна"}
	tests := []struct {
		name    string
		model   string
		texts   []string
		reply   string
		err     error
		want    []string
		wantErr string
		calls   int
	}{
		{
			name:  "one call, order kept",
			texts: in,
			reply: "```json\n{\"texts\":[\" red silk midi dress \",\"low bun\",\"cafe by the window\"]}\n```",
			want:  []string{"red silk midi dress", "low bun", "cafe by the window"},
			calls: 1,
		},
		{name: "nothing to translate", texts: nil, want: []string{}},
		{name: "length mismatch", texts: in, reply: `{"texts":["red silk midi dress","low bun"]}`, wantErr: "2 из 3", calls: 1},
		{name: "forbidden word names the text", texts: in, reply: `{"texts":["red silk midi dress","low bun on blonde hair","cafe"]}`, wantErr: "низкий пучок", calls: 1},
		{name: "empty translation", texts: in, reply: `{"texts":["red silk midi dress","","cafe"]}`, wantErr: "низкий пучок", calls: 1},
		{name: "not json", texts: in, reply: "Sure!", wantErr: "не JSON", calls: 1},
		{name: "Russian echoed back", texts: in, reply: `{"texts":["red silk midi dress","низкий пучок","cafe"]}`, wantErr: "низкий пучок", calls: 1},
		{name: "llm error", texts: in, err: errors.New("connection refused"), wantErr: "connection refused", calls: 1},
		{name: "no model", model: " ", texts: in, wantErr: "не настроена"},
		{name: "empty input text", texts: []string{"платье", " "}, wantErr: "пустой"},
		{name: "too many", texts: make([]string, TranslateMax+1), wantErr: "не больше"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := tt.model
			if model == "" {
				model = "m"
			}
			chat := &fakeChat{reply: tt.reply, err: tt.err}
			got, err := Translate(chat, model, tt.texts)
			assert.Equal(t, tt.calls, chat.calls)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTranslate_SendsTextsInOrder(t *testing.T) {
	chat := &fakeChat{reply: `{"texts":["a","b"]}`}
	_, err := Translate(chat, "m", []string{"первое", "второе"})
	require.NoError(t, err)
	var sent struct {
		Texts []string `json:"texts"`
	}
	require.NoError(t, json.Unmarshal([]byte(chat.user), &sent))
	assert.Equal(t, []string{"первое", "второе"}, sent.Texts)
	assert.Contains(t, chat.system, "в том же порядке")
	assert.Contains(t, chat.system, "ткань")
}

func TestTranslate_RejectsCJK(t *testing.T) {
	chat := &fakeChat{reply: `{"texts":["red dress","低发髻"]}`}
	_, err := Translate(chat, "m", []string{"красное платье", "низкий пучок"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "низкий пучок")
}

func TestTranslateToRu(t *testing.T) {
	in := []string{"red silk midi dress", "low bun", "cafe by the window"}
	tests := []struct {
		name    string
		reply   string
		err     error
		want    []string
		wantErr bool
	}{
		{name: "order kept", reply: `{"texts":[" красное шёлковое платье миди ","низкий пучок","кафе у окна"]}`, want: []string{"красное шёлковое платье миди", "низкий пучок", "кафе у окна"}},
		{name: "wrong count", reply: `{"texts":["красное платье","низкий пучок"]}`, wantErr: true},
		{name: "chinese leaked", reply: `{"texts":["красное платье","低发髻","кафе"]}`, wantErr: true},
		{name: "forbidden ru dropped", reply: `{"texts":["платье для молодой блондинки","низкий пучок","кафе у окна"]}`, want: []string{"", "низкий пучок", "кафе у окна"}},
		{name: "not json", reply: "Конечно!", wantErr: true},
		{name: "llm error", err: errors.New("down"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat := &fakeChat{reply: tt.reply, err: tt.err}
			got, err := TranslateToRu(chat, "m", in)
			assert.Equal(t, 1, chat.calls)
			if tt.wantErr {
				assert.ErrorIs(t, err, errNoTranslation)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, chat.system, "по-русски")
			var sent struct {
				Texts []string `json:"texts"`
			}
			require.NoError(t, json.Unmarshal([]byte(chat.user), &sent))
			assert.Equal(t, in, sent.Texts)
		})
	}
}

func TestAddRussian(t *testing.T) {
	ideas := []Idea{{Outfit: "red dress, thin straps, black heels", Background: "park", Place: "nature"}, {}, {Hair: "low bun"}}
	chat := &fakeChat{reply: `{"texts":["красное платье","парк","низкий пучок"]}`}
	require.NoError(t, AddRussian(chat, "m", ideas))
	assert.Equal(t, 1, chat.calls)
	assert.Contains(t, chat.user, "red dress")
	assert.Equal(t, []string{"красное платье", "парк", "низкий пучок"}, []string{ideas[0].OutfitRu, ideas[0].BackgroundRu, ideas[2].HairRu})
	assert.Empty(t, ideas[0].HairRu)

	none := &fakeChat{}
	require.NoError(t, AddRussian(none, "m", []Idea{{}}))
	assert.Zero(t, none.calls)
}
