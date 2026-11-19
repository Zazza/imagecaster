package stylist

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeChat struct {
	reply  string
	err    error
	system string
	user   string
	calls  int
}

func (f *fakeChat) Chat(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
	f.calls++
	f.system, f.user = systemPrompt, userMessage
	return f.reply, f.err
}

func TestForbidden(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"a 25 year old woman", true},
		{"young woman in a dress", true},
		{"fair skin, freckled", true},
		{"olive skin tone", true},
		{"asian features", true},
		{"blue eyes", true},
		{"curvy figure", true},
		{"slim woman", true},
		{"short man in a suit", true},
		{"short man bun", false},
		{"tall man bun, undercut", false},
		{"blonde waves", true},
		{"long black hair", true},
		{"black satin gown, thin straps", false},
		{"black skinny jeans", false},
		{"snowy old town at dusk", false},
		{"smoky eyes, nude lips", false},
		{"golden hour light", false},
		{"long waves with honey balayage", true},
		{"ombre bob", true},
		{"loose chestnut waves", true},
		{"jet-black bob", true},
		{"raven hair in a bun", true},
		{"hair dyed copper", true},
		{"curly-haired", true},
		{"a woman in her 30s", true},
		{"youthful look", true},
		{"a girl in a dress", true},
		{"mature elegance", true},
		{"porcelain glow", true},
		{"tanned skin", true},
		{"tan skin", true},
		{"olive-toned skin", true},
		{"fair skin", true},
		{"long legs in heels", true},
		{"slender silhouette", true},
		{"curvy", true},
		{"petite frame", true},
		{"tan trench coat, beige scarf", false},
		{"low sleek bun with a middle part", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Forbidden(tt.text), tt.text)
	}
}

func TestAskLLM(t *testing.T) {
	b := mustBase(t)
	req := PlanRequest{Frames: 8, Wish: "summer by the sea", Themes: []string{"beach"}}
	tests := []struct {
		name      string
		reply     string
		err       error
		wantErr   bool
		wantIdeas []Idea
		warnings  int
	}{
		{
			name:  "valid",
			reply: "```json\n{\"series\":[{\"outfit\":\"white linen midi dress, square neckline, short puff sleeves, sandals\",\"hair\":\"beach waves\",\"makeup\":\"no makeup\",\"background\":\"sandy beach at noon\",\"place\":\"beach\"},{\"outfit\":\"blue one-piece swimsuit, high-cut legs, thin straps, straw hat\",\"hair\":\"high ponytail\",\"makeup\":\"glossy lips\",\"background\":\"hotel pool\",\"place\":\"urban\"}]}\n```",
			wantIdeas: []Idea{
				{Outfit: "white linen midi dress, square neckline, short puff sleeves, sandals", Hair: "beach waves", Makeup: "no makeup", Background: "sandy beach at noon", Place: "beach"},
				{Outfit: "blue one-piece swimsuit, high-cut legs, thin straps, straw hat", Hair: "high ponytail", Makeup: "glossy lips", Background: "hotel pool", Place: "urban"},
			},
		},
		{
			name:  "forbidden word and foreign place",
			reply: `{"series":[{"outfit":"dress for a young woman","hair":"blonde waves","makeup":"soft","background":"a mars base","place":"space"},{"outfit":"slim blue denim jeans, white cotton t-shirt, sneakers","hair":"bun","makeup":"","background":"park","place":"urban"}]}`,
			wantIdeas: []Idea{
				{Makeup: "soft"},
				{Outfit: "slim blue denim jeans, white cotton t-shirt, sneakers", Hair: "bun", Background: "park", Place: "urban"},
			},
			warnings: 4,
		},
		{
			name:  "fewer series",
			reply: `{"series":[{"outfit":"slim blue denim jeans, white cotton t-shirt, sneakers","hair":"bun","makeup":"soft","background":"park","place":"urban"}]}`,
			wantIdeas: []Idea{
				{Outfit: "slim blue denim jeans, white cotton t-shirt, sneakers", Hair: "bun", Makeup: "soft", Background: "park", Place: "urban"},
				{},
			},
			warnings: 1,
		},
		{name: "garbage", reply: "sure! here are ideas", wantErr: true},
		{name: "empty series", reply: `{"series":[]}`, wantErr: true},
		{name: "llm error", err: errors.New("down"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat := &fakeChat{reply: tt.reply, err: tt.err}
			ideas, warnings, err := AskLLM(chat, "m", req, b, 2)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantIdeas, ideas)
			assert.Len(t, warnings, tt.warnings, warnings)
		})
	}
}

func TestAskLLM_PromptAsksOnlyAutoAspects(t *testing.T) {
	b := mustBase(t)
	chat := &fakeChat{reply: `{"series":[{"hair":"bun"}]}`}
	req := PlanRequest{Frames: 1, Wish: "office", Aspects: map[string]AspectChoice{
		"outfit": {Mode: ModeKeep}, "makeup": {Mode: ModePick, IDs: []string{"m01"}}, "background": {Mode: ModeKeep},
	}}
	ideas, warnings, err := AskLLM(chat, "m", req, b, 1)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, []Idea{{Hair: "bun"}}, ideas)
	assert.Contains(t, chat.system, `"hair"`)
	assert.NotContains(t, chat.system, `"outfit"`)
	assert.NotContains(t, chat.system, `"place"`)
	assert.Contains(t, chat.system, "JSON only")
	assert.Contains(t, chat.user, "office")
}

func TestAskLLM_SkipsCall(t *testing.T) {
	b := mustBase(t)
	chat := &fakeChat{}
	ideas, _, err := AskLLM(chat, "m", PlanRequest{Frames: 1, Aspects: allKeep()}, b, 1)
	assert.NoError(t, err)
	assert.Nil(t, ideas)
	assert.Zero(t, chat.calls)
	_, _, err = AskLLM(chat, " ", PlanRequest{Frames: 1}, b, 1)
	assert.Error(t, err)
	assert.Zero(t, chat.calls)
}

func TestAskLLM_ThenPlanFallsBackToPresets(t *testing.T) {
	b := mustBase(t)
	chat := &fakeChat{reply: `{"series":[{"outfit":"a 30 years old look","hair":"bun","makeup":"soft","background":"park","place":"urban"}]}`}
	req := PlanRequest{Frames: 1, Themes: []string{"winter"}, Seed: 2}
	ideas, warnings, err := AskLLM(chat, "m", req, b, 1)
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.True(t, strings.Contains(warnings[0], "одежда"), warnings[0])
	series, _, err := Plan(req, b, ideas)
	require.NoError(t, err)
	it := Item{}
	for _, o := range b.Outfit {
		if o.Prompt == series[0].Outfit {
			it = o
		}
	}
	assert.Equal(t, "winter", it.Theme)
	assert.Equal(t, "bun", series[0].Hair)
}

func TestAskLLM_ShortOutfitFallsBack(t *testing.T) {
	b := mustBase(t)
	chat := &fakeChat{reply: `{"series":[{"outfit":"casual cotton sundress","hair":"low bun with a middle part","makeup":"soft","background":"park","place":"nature"}]}`}
	ideas, warnings, err := AskLLM(chat, "m", PlanRequest{Frames: 1}, b, 1)
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "слишком коротко")
	assert.Empty(t, ideas[0].Outfit)
	assert.Equal(t, "nature", ideas[0].Place)
	assert.Contains(t, chat.system, "neckline")
	assert.Contains(t, chat.system, "nature")
	assert.NotContains(t, chat.system, "outdoor")
}

func TestForbiddenRu(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"женщина 25 лет", true},
		{"молодая женщина в платье", true},
		{"по возрасту за тридцать", true},
		{"светлая кожа, веснушки", true},
		{"цвет кожи оливковый", true},
		{"загорелая, в купальнике", true},
		{"смуглая", true},
		{"азиатская внешность", true},
		{"национальный костюм", true},
		{"черты лица мягкие", true},
		{"голубые глаза", true},
		{"курносый нос", true},
		{"высокие скулы", true},
		{"стройная фигура", true},
		{"худая", true},
		{"полная женщина", true},
		{"блондинка с локонами", true},
		{"брюнетка", true},
		{"рыжие волосы", true},
		{"русые волосы в косе", true},
		{"седые пряди", true},
		{"окрашенные пряди", true},
		{"чёрные волосы до плеч", true},
		{"длинное чёрное атласное платье в пол", false},
		{"светлая кожаная юбка, балетки, жилет, браслет", false},
		{"платье, облегающее по фигуре", false},
		{"волнистые волосы до подбородка, пробор посередине", false},
		{"острый подбородок", true},
		{"худи оверсайз и носки", false},
		{"анфас в рост", false},
		{"низкий гладкий пучок с пробором посередине", false},
		{"дымчатые тени, нюдовые губы", false},
		{"заснеженный старый город в сумерках", false},
		{"Летний макияж с бронзером", false},
		{"сумка из тонкой кожи", false},
		{"ботинки из тёмной кожи", false},
		{"жакет в европейском стиле", false},
		{"снимок в полный рост", false},
		{"фигурные пуговицы", false},
		{"рыжая замшевая сумка", false},
		{"девушка с тёмной кожей", true},
		{"тон кожи тёплый", true},
		{"европейская внешность", true},
		{"рыжеволосая", true},
		{"рыжие локоны", true},
		{"фигура песочные часы", true},
		{"полная, в платье", true},
		{"модель 30 лет", true},
		{"раскосые глаза, острый подбородок", true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ForbiddenRu(tt.text), tt.text)
	}
}

func TestAskLLM_EnglishOnly(t *testing.T) {
	b := mustBase(t)
	const outfit = "white linen midi dress, square neckline, short puff sleeves, sandals"
	chat := &fakeChat{reply: `{"series":[{"outfit":"` + outfit + `","outfit_ru":"платье","hair":"beach waves","makeup":"no makeup","background":"sandy beach at noon","place":"beach"}]}`}
	ideas, _, err := AskLLM(chat, "m", PlanRequest{Frames: 4, Locale: "ru"}, b, 1)
	require.NoError(t, err)
	assert.Equal(t, []Idea{{Outfit: outfit, Hair: "beach waves", Makeup: "no makeup", Background: "sandy beach at noon", Place: "beach"}}, ideas)
	assert.NotContains(t, chat.system, "_ru")
	assert.NotContains(t, chat.system, "Russian")
	assert.Contains(t, chat.system, "neckline")
	assert.Equal(t, 1, chat.calls)
}
