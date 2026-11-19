package stylist

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const customTestBaseJSON = `{
	"version": 1,
	"themes": [{"id": "winter", "name": "Зима"}, {"id": "summer", "name": "Лето"}],
	"outfit": [
		{"id": "o1", "theme": "winter", "name": "Платье", "prompt": "velvet dress"},
		{"id": "o2", "theme": "winter", "name": "Свитер", "prompt": "wool sweater and jeans", "male": true},
		{"id": "o3", "theme": "summer", "name": "Футболка", "prompt": "white t-shirt and shorts", "male": true},
		{"id": "o4", "name": "Юбка", "prompt": "pleated skirt"}
	],
	"hair": [
		{"id": "h1", "name": "Пучок", "prompt": "low bun"},
		{"id": "h2", "name": "Короткая", "prompt": "short cropped hair", "male": true},
		{"id": "h3", "name": "Локоны", "prompt": "long curls"}
	],
	"makeup": [
		{"id": "m1", "name": "Стрелки", "prompt": "winged eyeliner"},
		{"id": "m2", "name": "Помада", "prompt": "red lipstick"}
	],
	"background": [
		{"id": "b1", "name": "Пляж", "prompt": "sandy beach", "place": "beach"},
		{"id": "b2", "name": "Студия", "prompt": "grey studio backdrop", "place": "studio", "male": true}
	],
	"pose": [
		{"id": "p1", "name": "Анфас", "prompt": "facing camera", "framing": "close", "places": ["any"], "orientation": "portrait"},
		{"id": "p2", "name": "Лёжа", "prompt": "lying on sand", "framing": "full", "places": ["beach"], "orientation": "landscape"},
		{"id": "p3", "name": "Стоя", "prompt": "standing by the wall", "framing": "full", "places": ["studio"], "orientation": "portrait"}
	]
}`

func customTestBase(t *testing.T) *Base {
	t.Helper()
	b, err := Parse([]byte(customTestBaseJSON))
	require.NoError(t, err)
	return b
}

func customKeep() map[string]AspectChoice {
	return map[string]AspectChoice{
		"outfit": {Mode: ModeKeep}, "hair": {Mode: ModeKeep}, "makeup": {Mode: ModeKeep},
		"background": {Mode: ModeKeep}, "pose": {Mode: ModeKeep},
	}
}

func warnsWith(warns []string, sub string) []string {
	var out []string
	for _, w := range warns {
		if strings.Contains(w, sub) {
			out = append(out, w)
		}
	}
	return out
}

type aspectField struct {
	aspect Aspect
	get    func(Series) string
	getRu  func(Series) string
}

var customFields = []aspectField{
	{AspectOutfit, func(s Series) string { return s.Outfit }, func(s Series) string { return s.OutfitRu }},
	{AspectHair, func(s Series) string { return s.Hair }, func(s Series) string { return s.HairRu }},
	{AspectMakeup, func(s Series) string { return s.Makeup }, func(s Series) string { return s.MakeupRu }},
	{AspectBackground, func(s Series) string { return s.Background }, func(s Series) string { return s.BackgroundRu }},
}

func TestParse_AcceptsMaleFlag(t *testing.T) {
	b := customTestBase(t)
	tests := []struct {
		kind Aspect
		id   string
		male bool
	}{
		{AspectOutfit, "o1", false},
		{AspectOutfit, "o2", true},
		{AspectOutfit, "o3", true},
		{AspectHair, "h2", true},
		{AspectHair, "h1", false},
		{AspectMakeup, "m1", false},
		{AspectBackground, "b2", true},
		{AspectBackground, "b1", false},
	}
	for _, tt := range tests {
		it, ok := b.Find(tt.kind, tt.id)
		require.True(t, ok, tt.id)
		assert.Equal(t, tt.male, it.Male, tt.id)
	}
}

func TestEmbeddedBase_MaleMarks(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	var got []string
	for _, kind := range []Aspect{AspectOutfit, AspectHair, AspectMakeup, AspectBackground} {
		for _, it := range b.Items(kind) {
			if it.Male {
				got = append(got, it.ID)
			}
		}
	}
	slices.Sort(got)
	assert.Equal(t, []string{"h02", "h16", "h17", "m01", "o07", "o08", "o24", "o34", "o35"}, got)
}

func TestChoice_Custom(t *testing.T) {
	tests := []struct {
		name    string
		aspect  Aspect
		garment bool
		want    Mode
	}{
		{"outfit custom", AspectOutfit, false, ModeCustom},
		{"hair custom", AspectHair, false, ModeCustom},
		{"makeup custom", AspectMakeup, false, ModeCustom},
		{"background custom", AspectBackground, false, ModeCustom},
		{"pose custom is auto", AspectPose, false, ModeAuto},
		{"outfit custom with garment is keep", AspectOutfit, true, ModeKeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := PlanRequest{Garment: tt.garment, Aspects: map[string]AspectChoice{
				string(tt.aspect): {Mode: ModeCustom, Text: "something"},
			}}
			assert.Equal(t, tt.want, req.Choice(tt.aspect).Mode)
		})
	}
}

func TestPlan_CustomTextInEverySeries(t *testing.T) {
	b := customTestBase(t)
	for _, f := range customFields {
		t.Run(string(f.aspect), func(t *testing.T) {
			aspects := customKeep()
			aspects[string(f.aspect)] = AspectChoice{Mode: ModeCustom, Text: "  my own words, exactly  ", TextRu: "  мои слова  ", IDs: []string{"o1", "h1", "m1", "b1"}}
			series, warns, err := Plan(PlanRequest{Frames: 10, Aspects: aspects, Seed: 5}, b, nil)
			require.NoError(t, err)
			require.Len(t, series, 3)
			for _, s := range series {
				assert.Equal(t, "my own words, exactly", f.get(s))
				assert.Equal(t, "мои слова", f.getRu(s))
			}
			assert.Empty(t, warnsWith(warns, "Своё"))
		})
	}
}

func TestPlan_CustomTextWithoutRu(t *testing.T) {
	b := customTestBase(t)
	aspects := customKeep()
	aspects["hair"] = AspectChoice{Mode: ModeCustom, Text: "messy bun"}
	series, _, err := Plan(PlanRequest{Frames: 1, Aspects: aspects}, b, nil)
	require.NoError(t, err)
	assert.Equal(t, "messy bun", series[0].Hair)
	assert.Empty(t, series[0].HairRu)
}

func TestPlan_CustomEmptyTextWarns(t *testing.T) {
	b := customTestBase(t)
	for _, f := range customFields {
		for _, text := range []string{"", "   \t "} {
			t.Run(string(f.aspect)+"/"+text, func(t *testing.T) {
				aspects := customKeep()
				aspects[string(f.aspect)] = AspectChoice{Mode: ModeCustom, Text: text}
				series, warns, err := Plan(PlanRequest{Frames: 8, Aspects: aspects, Seed: 2}, b, nil)
				require.NoError(t, err)
				for _, s := range series {
					assert.Empty(t, f.get(s))
				}
				assert.Len(t, warnsWith(warns, "Своё"), 1)
			})
		}
	}
}

func TestPlan_CustomTextLimit(t *testing.T) {
	b := customTestBase(t)
	tests := []struct {
		name   string
		text   string
		textRu string
		ok     bool
	}{
		{"text exactly max", strings.Repeat("a", CustomTextMax), "", true},
		{"text_ru exactly max runes", "dress", strings.Repeat("я", CustomTextMax), true},
		{"text over max", strings.Repeat("a", CustomTextMax+1), "", false},
		{"text_ru over max", "dress", strings.Repeat("я", CustomTextMax+1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aspects := customKeep()
			aspects["outfit"] = AspectChoice{Mode: ModeCustom, Text: tt.text, TextRu: tt.textRu}
			req := PlanRequest{Frames: 4, Aspects: aspects}
			_, _, err := Plan(req, b, nil)
			_, _, rerr := RegenerateSeries(req, b, nil, 0)
			if tt.ok {
				assert.NoError(t, err)
				assert.NoError(t, rerr)
			} else {
				assert.Error(t, err)
				assert.Error(t, rerr)
			}
		})
	}
}

func TestPlan_CustomPoseActsAsAuto(t *testing.T) {
	b := customTestBase(t)
	aspects := customKeep()
	aspects["background"] = AspectChoice{Mode: ModePick, IDs: []string{"b1"}}
	aspects["pose"] = AspectChoice{Mode: ModeCustom, Text: "jumping"}
	series, _, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Seed: 1}, b, nil)
	require.NoError(t, err)
	allowed := []string{"facing camera", "lying on sand"}
	for _, sh := range series[0].Shots {
		assert.Contains(t, allowed, sh.Prompt)
		assert.NotContains(t, sh.Prompt, "jumping")
	}
}

func TestPlan_CustomBackgroundHasNoPlace(t *testing.T) {
	b := customTestBase(t)
	for _, seed := range []int64{1, 2, 3, 4, 5, 6, 7, 8} {
		aspects := customKeep()
		aspects["background"] = AspectChoice{Mode: ModeCustom, Text: "rooftop at sunset"}
		aspects["pose"] = AspectChoice{Mode: ModeAuto}
		series, _, err := Plan(PlanRequest{Frames: 8, Aspects: aspects, Seed: seed}, b, nil)
		require.NoError(t, err)
		for _, s := range series {
			assert.Equal(t, "rooftop at sunset", s.Background)
			assert.Empty(t, s.Place)
			for _, sh := range s.Shots {
				assert.Equal(t, "facing camera", sh.Prompt, "only poses for any place, seed %d", seed)
			}
		}
	}
}

func TestPlan_GarmentIgnoresCustomOutfit(t *testing.T) {
	b := customTestBase(t)
	aspects := customKeep()
	aspects["outfit"] = AspectChoice{Mode: ModeCustom, Text: "red silk dress", TextRu: "красное платье"}
	series, _, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Garment: true}, b, nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultOutfit, series[0].Outfit)
	assert.NotContains(t, series[0].Outfit, "red silk dress")
}

func TestPlan_CustomIsNotAuto(t *testing.T) {
	b := customTestBase(t)
	aspects := customKeep()
	aspects["outfit"] = AspectChoice{Mode: ModeCustom, Text: "red silk dress"}
	aspects["hair"] = AspectChoice{Mode: ModeAuto}
	req := PlanRequest{Frames: 4, Aspects: aspects, Seed: 1}
	assert.Equal(t, []Aspect{AspectHair}, req.AutoAspects())

	ideas := []Idea{{Outfit: "idea outfit from the model", Hair: "idea hair from the model"}}
	series, _, err := Plan(req, b, ideas)
	require.NoError(t, err)
	assert.Equal(t, "red silk dress", series[0].Outfit)
	assert.Equal(t, "idea hair from the model", series[0].Hair)

	aspects["hair"] = AspectChoice{Mode: ModeCustom, Text: "braid"}
	only := PlanRequest{Frames: 4, Aspects: aspects}
	assert.Empty(t, only.AutoAspects())
	_, warns, err := Plan(only, b, nil)
	require.NoError(t, err)
	assert.NotContains(t, warns, warnNoIdeas)
}

func TestRegenerateSeries_Custom(t *testing.T) {
	b := customTestBase(t)
	aspects := customKeep()
	aspects["makeup"] = AspectChoice{Mode: ModeCustom, Text: "glitter on cheeks", TextRu: "блёстки"}
	s, _, err := RegenerateSeries(PlanRequest{Frames: 8, Aspects: aspects, Seed: 9}, b, nil, 1)
	require.NoError(t, err)
	assert.Equal(t, "glitter on cheeks", s.Makeup)
	assert.Equal(t, "блёстки", s.MakeupRu)
}

func autoAll() map[string]AspectChoice {
	return map[string]AspectChoice{
		"outfit": {Mode: ModeAuto}, "hair": {Mode: ModeAuto}, "makeup": {Mode: ModeAuto},
		"background": {Mode: ModeAuto}, "pose": {Mode: ModeAuto},
	}
}

func TestPlan_ManAutoTakesMalePresets(t *testing.T) {
	b := customTestBase(t)
	prompt := func(kind Aspect, id string) string {
		it, _ := b.Find(kind, id)
		return it.Prompt
	}
	tests := []struct {
		name       string
		gender     string
		themes     []string
		wantOutfit []string
	}{
		{"man no themes", "man", nil, []string{prompt(AspectOutfit, "o2"), prompt(AspectOutfit, "o3")}},
		{"male winter", "male", []string{"winter"}, []string{prompt(AspectOutfit, "o2")}},
		{"man summer", "man", []string{"summer"}, []string{prompt(AspectOutfit, "o3")}},
		{"Male with spaces", "  Male ", nil, []string{prompt(AspectOutfit, "o2"), prompt(AspectOutfit, "o3")}},
		{"MAN", "MAN", nil, []string{prompt(AspectOutfit, "o2"), prompt(AspectOutfit, "o3")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for seed := int64(0); seed < 40; seed++ {
				series, _, err := Plan(PlanRequest{Frames: 8, Gender: tt.gender, Themes: tt.themes, Aspects: autoAll(), Seed: seed}, b, nil)
				require.NoError(t, err)
				for _, s := range series {
					assert.Contains(t, tt.wantOutfit, s.Outfit, "seed %d", seed)
					assert.Equal(t, prompt(AspectHair, "h2"), s.Hair, "seed %d", seed)
				}
			}
		})
	}
}

func TestPlan_ManAutoIncludesOwnPresets(t *testing.T) {
	ub := withUser(t, customTestBase(t), UserPreset{ID: 7, Aspect: AspectOutfit, Name: "Моё", Prompt: "linen suit"})
	tests := []struct {
		name   string
		gender string
	}{{"man", "man"}, {"woman", "woman"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := false
			for seed := int64(0); seed < 60 && !got; seed++ {
				series, warns, err := Plan(PlanRequest{Frames: 4, Gender: tt.gender, Aspects: autoAll(), Seed: seed}, ub, nil)
				require.NoError(t, err)
				assert.Empty(t, warnsWith(warns, "задумано"))
				got = series[0].Outfit == "linen suit"
			}
			assert.True(t, got, "own preset never picked for %s", tt.gender)
		})
	}
}

func TestPlan_CustomTranslatedLongerThanTyped(t *testing.T) {
	b := customTestBase(t)
	tests := []struct {
		name    string
		text    string
		textRu  string
		wantErr string
	}{
		{"translation longer than typed", strings.Repeat("a", 600), strings.Repeat("я", 400), ""},
		{"typed russian over limit", strings.Repeat("a", 100), strings.Repeat("я", 501), "не длиннее"},
		{"translation absurdly long", strings.Repeat("a", 1001), strings.Repeat("я", 400), "перевод"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aspects := customKeep()
			aspects["outfit"] = AspectChoice{Mode: ModeCustom, Text: tt.text, TextRu: tt.textRu}
			_, _, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Seed: 1}, b, nil)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPlan_ManThemeWithoutMaleWarns(t *testing.T) {
	b := customTestBase(t)
	male := []string{"wool sweater and jeans", "white t-shirt and shorts"}
	for seed := int64(0); seed < 20; seed++ {
		series, warns, err := Plan(PlanRequest{Frames: 4, Gender: "man", Themes: []string{"evening"}, Aspects: autoAll(), Seed: seed}, b, nil)
		require.NoError(t, err)
		assert.Contains(t, male, series[0].Outfit)
		other := warnsWith(warns, "другой темы")
		require.Len(t, other, 1, "warnings: %v", warns)
		assert.Contains(t, other[0], aspectNames[AspectOutfit])
	}
}

func TestPlan_ManNoMalePresetsWarns(t *testing.T) {
	b := customTestBase(t)
	makeups := []string{"winged eyeliner", "red lipstick"}
	for seed := int64(0); seed < 10; seed++ {
		series, warns, err := Plan(PlanRequest{Frames: 12, Gender: "man", Aspects: autoAll(), Seed: seed}, b, nil)
		require.NoError(t, err)
		for _, s := range series {
			assert.Contains(t, makeups, s.Makeup)
		}
		male := warnsWith(warns, "мужчин")
		require.Len(t, male, 1, "one warning, not one per series")
		assert.Contains(t, male[0], aspectNames[AspectMakeup])
	}
}

func TestPlan_GenderWarnings(t *testing.T) {
	b := customTestBase(t)
	ub := withUser(t, b, UserPreset{ID: 1, Aspect: AspectOutfit, Name: "Моё", Prompt: "linen suit"})
	pick := func(aspect string, ids ...string) map[string]AspectChoice {
		a := customKeep()
		a[aspect] = AspectChoice{Mode: ModePick, IDs: ids}
		return a
	}
	custom := customKeep()
	custom["makeup"] = AspectChoice{Mode: ModeCustom, Text: "glitter"}
	tests := []struct {
		name       string
		base       *Base
		gender     string
		aspects    map[string]AspectChoice
		wantMale   int
		wantFemale int
		outfit     string
	}{
		{"woman auto", b, "woman", autoAll(), 0, 0, ""},
		{"female auto", b, "female", autoAll(), 0, 0, ""},
		{"empty gender auto", b, "", autoAll(), 0, 0, ""},
		{"woman pick unmarked", b, "woman", pick("outfit", "o1"), 0, 0, "velvet dress"},
		{"man pick unmarked", b, "man", pick("outfit", "o1"), 0, 1, "velvet dress"},
		{"man pick unmarked hair", b, "man", pick("hair", "h1"), 0, 1, ""},
		{"man pick marked", b, "man", pick("outfit", "o2"), 0, 0, "wool sweater and jeans"},
		{"man pick own preset", ub, "man", pick("outfit", "u1"), 0, 0, "linen suit"},
		{"man custom makeup", b, "man", custom, 0, 0, ""},
		{"man keep all", b, "man", customKeep(), 0, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			series, warns, err := Plan(PlanRequest{Frames: 8, Gender: tt.gender, Aspects: tt.aspects, Seed: 3}, tt.base, nil)
			require.NoError(t, err)
			assert.Len(t, warnsWith(warns, "мужчин"), tt.wantMale, "warnings: %v", warns)
			assert.Len(t, warnsWith(warns, "женск"), tt.wantFemale, "warnings: %v", warns)
			if tt.outfit != "" {
				for _, s := range series {
					assert.Equal(t, tt.outfit, s.Outfit)
				}
			}
		})
	}
}

func TestRegenerateSeries_ManPickUnmarkedWarns(t *testing.T) {
	b := customTestBase(t)
	aspects := customKeep()
	aspects["outfit"] = AspectChoice{Mode: ModePick, IDs: []string{"o1"}}
	s, warns, err := RegenerateSeries(PlanRequest{Frames: 4, Gender: "man", Aspects: aspects}, b, nil, 0)
	require.NoError(t, err)
	assert.Equal(t, "velvet dress", s.Outfit)
	assert.Len(t, warnsWith(warns, "женск"), 1)
}
