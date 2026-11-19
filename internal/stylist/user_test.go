package stylist

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const userTestBaseJSON = `{
	"version": 1,
	"themes": [{"id": "winter", "name": "Зима"}],
	"outfit": [{"id": "o1", "theme": "winter", "name": "Пальто", "prompt": "wool coat"}],
	"hair": [{"id": "h1", "theme": "winter", "name": "Пучок", "prompt": "low bun"}],
	"makeup": [{"id": "m1", "theme": "winter", "name": "Без", "prompt": "no makeup"}],
	"background": [
		{"id": "b1", "name": "Пляж", "prompt": "sandy beach", "place": "beach"},
		{"id": "b2", "name": "Студия", "prompt": "grey studio backdrop", "place": "studio"}
	],
	"pose": [
		{"id": "p1", "name": "Анфас", "prompt": "facing camera", "framing": "close", "places": ["any"], "orientation": "portrait"},
		{"id": "p2", "name": "Лёжа", "prompt": "lying on sand", "framing": "full", "places": ["beach"], "orientation": "landscape"}
	]
}`

func userTestBase(t *testing.T) *Base {
	t.Helper()
	b, err := Parse([]byte(userTestBaseJSON))
	require.NoError(t, err)
	return b
}

func withUser(t *testing.T, b *Base, list ...UserPreset) *Base {
	t.Helper()
	out, err := b.WithUser(list)
	require.NoError(t, err)
	require.NotNil(t, out)
	return out
}

func keepAll() map[string]AspectChoice {
	return map[string]AspectChoice{
		"outfit": {Mode: ModeKeep}, "hair": {Mode: ModeKeep}, "makeup": {Mode: ModeKeep},
		"background": {Mode: ModeKeep}, "pose": {Mode: ModeKeep},
	}
}

func TestUserID(t *testing.T) {
	tests := []struct {
		id   int64
		want string
	}{{1, "u1"}, {12, "u12"}, {100500, "u100500"}}
	for _, tt := range tests {
		assert.Equal(t, tt.want, UserID(tt.id))
	}
}

func TestParseUserID(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"u12", 12, true},
		{"u1", 1, true},
		{"u100500", 100500, true},
		{"o1", 0, false},
		{"u", 0, false},
		{"u0", 0, false},
		{"ux", 0, false},
		{"", 0, false},
		{"u-3", 0, false},
		{"U5", 0, false},
		{"u1x", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := ParseUserID(tt.in)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseUserID_RoundTrip(t *testing.T) {
	for _, id := range []int64{1, 7, 42, 999999} {
		got, ok := ParseUserID(UserID(id))
		assert.True(t, ok)
		assert.Equal(t, id, got)
	}
}

func TestCheckUser(t *testing.T) {
	b := userTestBase(t)
	pose := func(mut func(*UserPreset)) UserPreset {
		p := UserPreset{Aspect: AspectPose, Name: "Сидя", Prompt: "sitting", Framing: "knee", Places: []string{"beach"}, Orientation: "portrait"}
		if mut != nil {
			mut(&p)
		}
		return p
	}
	bg := func(place string) UserPreset {
		return UserPreset{Aspect: AspectBackground, Name: "Мой фон", Prompt: "white wall", Place: place}
	}
	tests := []struct {
		name string
		p    UserPreset
		ok   bool
	}{
		{"outfit", UserPreset{Aspect: AspectOutfit, Name: "Платье", Prompt: "red dress", PromptRu: "Красное платье"}, true},
		{"hair", UserPreset{Aspect: AspectHair, Name: "Коса", Prompt: "braid"}, true},
		{"makeup", UserPreset{Aspect: AspectMakeup, Name: "Smoky", Prompt: "smoky eyes"}, true},
		{"outfit ignores pose and background fields", UserPreset{Aspect: AspectOutfit, Name: "x", Prompt: "y", Place: "mars", Framing: "wide", Places: []string{"mars"}, Orientation: "square"}, true},
		{"background beach", bg("beach"), true},
		{"background studio", bg("studio"), true},
		{"pose knee beach portrait", pose(nil), true},
		{"pose close any", pose(func(p *UserPreset) { p.Framing = "close"; p.Places = []string{"any"} }), true},
		{"pose waist", pose(func(p *UserPreset) { p.Framing = "waist" }), true},
		{"pose full landscape two places", pose(func(p *UserPreset) {
			p.Framing = "full"
			p.Orientation = "landscape"
			p.Places = []string{"beach", "studio"}
		}), true},
		{"name exactly 60 runes", UserPreset{Aspect: AspectHair, Name: strings.Repeat("я", UserNameMax), Prompt: "braid"}, true},
		{"prompt exactly 1000 runes", UserPreset{Aspect: AspectHair, Name: "x", Prompt: strings.Repeat("a", UserPromptMax), PromptRu: strings.Repeat("я", UserPromptMax)}, true},

		{"unknown aspect", UserPreset{Aspect: "shoes", Name: "x", Prompt: "y"}, false},
		{"empty aspect", UserPreset{Name: "x", Prompt: "y"}, false},
		{"empty name", UserPreset{Aspect: AspectHair, Name: "", Prompt: "braid"}, false},
		{"blank name", UserPreset{Aspect: AspectHair, Name: "   ", Prompt: "braid"}, false},
		{"empty prompt", UserPreset{Aspect: AspectHair, Name: "Коса", Prompt: ""}, false},
		{"blank prompt", UserPreset{Aspect: AspectHair, Name: "Коса", Prompt: " \t "}, false},
		{"name 61 runes", UserPreset{Aspect: AspectHair, Name: strings.Repeat("я", UserNameMax+1), Prompt: "braid"}, false},
		{"prompt too long", UserPreset{Aspect: AspectHair, Name: "x", Prompt: strings.Repeat("a", UserPromptMax+1)}, false},
		{"prompt_ru too long", UserPreset{Aspect: AspectHair, Name: "x", Prompt: "braid", PromptRu: strings.Repeat("я", UserPromptMax+1)}, false},
		{"background without place", bg(""), false},
		{"background place any", bg("any"), false},
		{"background unknown place", bg("mars"), false},
		{"pose unknown framing", pose(func(p *UserPreset) { p.Framing = "wide" }), false},
		{"pose empty framing", pose(func(p *UserPreset) { p.Framing = "" }), false},
		{"pose without places", pose(func(p *UserPreset) { p.Places = nil }), false},
		{"pose empty places slice", pose(func(p *UserPreset) { p.Places = []string{} }), false},
		{"pose unknown place", pose(func(p *UserPreset) { p.Places = []string{"mars"} }), false},
		{"pose one of places unknown", pose(func(p *UserPreset) { p.Places = []string{"beach", "mars"} }), false},
		{"pose empty orientation", pose(func(p *UserPreset) { p.Orientation = "" }), false},
		{"pose unknown orientation", pose(func(p *UserPreset) { p.Orientation = "square" }), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := b.CheckUser(tt.p)
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func userList() []UserPreset {
	return []UserPreset{
		{ID: 1, Aspect: AspectOutfit, Name: "Красное платье", Prompt: "red dress", PromptRu: "Красное шёлковое платье"},
		{ID: 2, Aspect: AspectHair, Name: "Коса", Prompt: "braid"},
		{ID: 3, Aspect: AspectMakeup, Name: "Smoky", Prompt: "smoky eyes", PromptRu: "Дымчатые глаза"},
		{ID: 4, Aspect: AspectBackground, Name: "Белая стена", Prompt: "white wall", Place: "studio"},
		{ID: 5, Aspect: AspectPose, Name: "Сидя", Prompt: "sitting on a stool", PromptRu: "Сидит на табурете", Framing: "knee", Places: []string{"studio"}, Orientation: "landscape"},
	}
}

func TestWithUser_MergesAtEndOfEachSection(t *testing.T) {
	b := userTestBase(t)
	m := withUser(t, b, userList()...)

	require.Len(t, m.Outfit, len(b.Outfit)+1)
	require.Len(t, m.Hair, len(b.Hair)+1)
	require.Len(t, m.Makeup, len(b.Makeup)+1)
	require.Len(t, m.Background, len(b.Background)+1)
	require.Len(t, m.Pose, len(b.Pose)+1)

	assert.Equal(t, b.Outfit, m.Outfit[:len(b.Outfit)])
	assert.Equal(t, b.Pose, m.Pose[:len(b.Pose)])

	o := m.Outfit[len(m.Outfit)-1]
	assert.Equal(t, "u1", o.ID)
	assert.True(t, o.User)
	assert.Equal(t, "Красное платье", o.Name)
	assert.Equal(t, "red dress", o.Prompt)
	assert.Equal(t, "Красное шёлковое платье", o.PromptRu)
	assert.Empty(t, o.Theme)

	h := m.Hair[len(m.Hair)-1]
	assert.Equal(t, "u2", h.ID)
	assert.True(t, h.User)
	assert.Empty(t, h.PromptRu)

	assert.Equal(t, "u3", m.Makeup[len(m.Makeup)-1].ID)

	bg := m.Background[len(m.Background)-1]
	assert.Equal(t, "u4", bg.ID)
	assert.True(t, bg.User)
	assert.Equal(t, "studio", bg.Place)
	assert.Equal(t, "white wall", bg.Prompt)
	assert.Empty(t, bg.Theme)

	p := m.Pose[len(m.Pose)-1]
	assert.Equal(t, "u5", p.ID)
	assert.True(t, p.User)
	assert.Equal(t, "sitting on a stool", p.Prompt)
	assert.Equal(t, "Сидит на табурете", p.PromptRu)
	assert.Equal(t, "knee", p.Framing)
	assert.Equal(t, []string{"studio"}, p.Places)
	assert.Equal(t, "landscape", p.Orientation)
	assert.Empty(t, p.Themes)

	for _, it := range b.Outfit {
		assert.False(t, it.User)
	}
	for _, it := range m.Pose[:len(b.Pose)] {
		assert.False(t, it.User)
	}
}

func TestWithUser_FindsUserPresets(t *testing.T) {
	m := withUser(t, userTestBase(t), userList()...)
	tests := []struct {
		aspect Aspect
		id     string
		prompt string
	}{
		{AspectOutfit, "u1", "red dress"},
		{AspectHair, "u2", "braid"},
		{AspectMakeup, "u3", "smoky eyes"},
		{AspectBackground, "u4", "white wall"},
		{AspectOutfit, "o1", "wool coat"},
	}
	for _, tt := range tests {
		it, ok := m.Find(tt.aspect, tt.id)
		require.True(t, ok, tt.id)
		assert.Equal(t, tt.prompt, it.Prompt)
	}
	_, ok := m.Find(AspectHair, "u1")
	assert.False(t, ok)
	pose, ok := m.FindPose("u5")
	require.True(t, ok)
	assert.Equal(t, "sitting on a stool", pose.Prompt)
	assert.Equal(t, "studio", m.PlaceOf("u4"))
}

func TestWithUser_OriginalUnchanged(t *testing.T) {
	b := userTestBase(t)
	before := userTestBase(t)
	_ = withUser(t, b, userList()...)
	assert.Equal(t, before, b)
	_, ok := b.Find(AspectOutfit, "u1")
	assert.False(t, ok)
	_, ok = b.FindPose("u5")
	assert.False(t, ok)
}

func TestWithUser_EmbeddedBaseUnchanged(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	counts := []int{len(b.Outfit), len(b.Hair), len(b.Makeup), len(b.Background), len(b.Pose)}
	list := []UserPreset{
		{ID: 1, Aspect: AspectOutfit, Name: "x", Prompt: "y"},
		{ID: 2, Aspect: AspectPose, Name: "p", Prompt: "q", Framing: "full", Places: []string{"any"}, Orientation: "portrait"},
	}
	m := withUser(t, b, list...)
	assert.Len(t, m.Outfit, counts[0]+1)
	again, err := Load()
	require.NoError(t, err)
	assert.Equal(t, counts, []int{len(again.Outfit), len(again.Hair), len(again.Makeup), len(again.Background), len(again.Pose)})
	_, ok := again.Find(AspectOutfit, "u1")
	assert.False(t, ok)
	_, ok = again.FindPose("u2")
	assert.False(t, ok)
}

func TestWithUser_EmptyListKeepsBase(t *testing.T) {
	b := userTestBase(t)
	m, err := b.WithUser(nil)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Len(t, m.Outfit, len(b.Outfit))
	assert.Len(t, m.Pose, len(b.Pose))
}

func TestWithUser_InvalidSkippedWithError(t *testing.T) {
	b := userTestBase(t)
	list := append(userList(),
		UserPreset{ID: 6, Aspect: AspectPose, Name: "Плохая", Prompt: "bad", Framing: "wide", Places: []string{"beach"}, Orientation: "portrait"},
		UserPreset{ID: 7, Aspect: AspectBackground, Name: "Марс", Prompt: "mars", Place: "mars"},
		UserPreset{ID: 8, Aspect: "shoes", Name: "Туфли", Prompt: "heels"},
	)
	m, err := b.WithUser(list)
	assert.Error(t, err)
	require.NotNil(t, m)
	assert.Len(t, m.Pose, len(b.Pose)+1)
	assert.Len(t, m.Background, len(b.Background)+1)
	_, ok := m.FindPose("u6")
	assert.False(t, ok)
	_, ok = m.Find(AspectBackground, "u7")
	assert.False(t, ok)
	_, ok = m.FindPose("u5")
	assert.True(t, ok)
	_, ok = m.Find(AspectOutfit, "u1")
	assert.True(t, ok)
}

func TestPlan_PickUserPose(t *testing.T) {
	tests := []struct {
		name      string
		pose      UserPreset
		landscape bool
		ru        string
	}{
		{"landscape with ru", UserPreset{ID: 7, Aspect: AspectPose, Name: "Сидя", Prompt: "sitting on a stool", PromptRu: "Сидит на табурете", Framing: "knee", Places: []string{"studio"}, Orientation: "landscape"}, true, "Сидит на табурете"},
		{"portrait without ru", UserPreset{ID: 8, Aspect: AspectPose, Name: "Руки вверх", Prompt: "arms raised", Framing: "waist", Places: []string{"any"}, Orientation: "portrait"}, false, "Руки вверх"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withUser(t, userTestBase(t), tt.pose)
			aspects := keepAll()
			aspects["pose"] = AspectChoice{Mode: ModePick, IDs: []string{UserID(tt.pose.ID)}}
			series, warnings, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Seed: 1}, m, nil)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			require.Len(t, series, 1)
			require.Len(t, series[0].Shots, 4)
			for _, sh := range series[0].Shots {
				assert.Equal(t, tt.pose.Prompt, sh.Prompt)
				assert.Equal(t, tt.pose.Framing, sh.Framing)
				assert.Equal(t, tt.landscape, sh.Landscape)
				assert.Equal(t, tt.ru, sh.PromptRu)
			}
		})
	}
}

func TestPlan_PickDeletedUserPoseWarnsAndFallsBack(t *testing.T) {
	m := withUser(t, userTestBase(t), userList()...)
	aspects := keepAll()
	aspects["pose"] = AspectChoice{Mode: ModePick, IDs: []string{"u99"}}
	series, warnings, err := Plan(PlanRequest{Frames: 2, Aspects: aspects, Seed: 1}, m, nil)
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], aspectNames[AspectPose])
	for _, sh := range series[0].Shots {
		assert.NotEmpty(t, sh.Prompt)
	}
}

func TestPlan_PickUserItems(t *testing.T) {
	m := withUser(t, userTestBase(t), userList()...)
	aspects := keepAll()
	aspects["outfit"] = AspectChoice{Mode: ModePick, IDs: []string{"u1"}}
	aspects["hair"] = AspectChoice{Mode: ModePick, IDs: []string{"u2"}}
	aspects["makeup"] = AspectChoice{Mode: ModePick, IDs: []string{"u3"}}
	aspects["background"] = AspectChoice{Mode: ModePick, IDs: []string{"u4"}}
	series, warnings, err := Plan(PlanRequest{Frames: 1, Aspects: aspects, Seed: 1}, m, nil)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	s := series[0]
	assert.Equal(t, "red dress", s.Outfit)
	assert.Equal(t, "braid", s.Hair)
	assert.Equal(t, "smoky eyes", s.Makeup)
	assert.Equal(t, "white wall", s.Background)
	assert.Equal(t, "studio", s.Place)
	assert.Equal(t, "Красное шёлковое платье", s.OutfitRu)
	assert.Equal(t, "Коса", s.HairRu)
	assert.Equal(t, "Дымчатые глаза", s.MakeupRu)
	assert.Equal(t, "Белая стена", s.BackgroundRu)
}

func TestRegenerateSeries_PickUserItems(t *testing.T) {
	m := withUser(t, userTestBase(t), userList()...)
	aspects := keepAll()
	aspects["outfit"] = AspectChoice{Mode: ModePick, IDs: []string{"u1"}}
	aspects["pose"] = AspectChoice{Mode: ModePick, IDs: []string{"u5"}}
	s, warnings, err := RegenerateSeries(PlanRequest{Frames: 2, Aspects: aspects, Seed: 3}, m, nil, 0)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, "red dress", s.Outfit)
	assert.Equal(t, "Красное шёлковое платье", s.OutfitRu)
	require.NotEmpty(t, s.Shots)
	assert.Equal(t, "sitting on a stool", s.Shots[0].Prompt)
}

func TestPlan_BuiltinRuUnchanged(t *testing.T) {
	m := withUser(t, userTestBase(t), userList()...)
	aspects := keepAll()
	aspects["outfit"] = AspectChoice{Mode: ModePick, IDs: []string{"o1"}}
	aspects["background"] = AspectChoice{Mode: ModePick, IDs: []string{"b1"}}
	aspects["pose"] = AspectChoice{Mode: ModePick, IDs: []string{"p2"}}
	series, _, err := Plan(PlanRequest{Frames: 1, Aspects: aspects, Seed: 1}, m, nil)
	require.NoError(t, err)
	s := series[0]
	assert.Equal(t, "Пальто", s.OutfitRu)
	assert.Equal(t, "Пляж", s.BackgroundRu)
	assert.Equal(t, "Лёжа", s.Shots[0].PromptRu)
}

func userPoseSeen(t *testing.T, m *Base, bgID string, garment bool, poseID string) bool {
	t.Helper()
	pose, ok := m.FindPose(poseID)
	require.True(t, ok)
	for seed := int64(1); seed <= 300; seed++ {
		aspects := keepAll()
		aspects["background"] = AspectChoice{Mode: ModePick, IDs: []string{bgID}}
		aspects["pose"] = AspectChoice{Mode: ModeAuto}
		series, _, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Garment: garment, Seed: seed}, m, nil)
		require.NoError(t, err)
		for _, s := range series {
			for _, sh := range s.Shots {
				if sh.Prompt == pose.Prompt {
					return true
				}
			}
		}
	}
	return false
}

func TestPlan_AutoUserPoseByPlace(t *testing.T) {
	tests := []struct {
		name    string
		pose    UserPreset
		bg      string
		garment bool
		seen    bool
	}{
		{"beach pose on beach", UserPreset{ID: 11, Aspect: AspectPose, Name: "Пляжная", Prompt: "user beach pose", Framing: "knee", Places: []string{"beach"}, Orientation: "portrait"}, "b1", false, true},
		{"any pose on beach", UserPreset{ID: 12, Aspect: AspectPose, Name: "Любая", Prompt: "user any pose", Framing: "waist", Places: []string{"any"}, Orientation: "portrait"}, "b1", false, true},
		{"studio pose on beach", UserPreset{ID: 13, Aspect: AspectPose, Name: "Студийная", Prompt: "user studio pose", Framing: "knee", Places: []string{"studio"}, Orientation: "portrait"}, "b1", false, false},
		{"studio pose in studio", UserPreset{ID: 14, Aspect: AspectPose, Name: "Студийная", Prompt: "user studio pose 2", Framing: "knee", Places: []string{"studio"}, Orientation: "portrait"}, "b2", false, true},
		{"close pose with garment", UserPreset{ID: 15, Aspect: AspectPose, Name: "Крупно", Prompt: "user close pose", Framing: "close", Places: []string{"any"}, Orientation: "portrait"}, "b1", true, false},
		{"waist pose with garment", UserPreset{ID: 16, Aspect: AspectPose, Name: "По пояс", Prompt: "user waist pose", Framing: "waist", Places: []string{"beach"}, Orientation: "portrait"}, "b1", true, false},
		{"full pose with garment", UserPreset{ID: 17, Aspect: AspectPose, Name: "В рост", Prompt: "user full pose", Framing: "full", Places: []string{"beach"}, Orientation: "portrait"}, "b1", true, true},
		{"knee pose with garment", UserPreset{ID: 18, Aspect: AspectPose, Name: "По колено", Prompt: "user knee pose", Framing: "knee", Places: []string{"any"}, Orientation: "landscape"}, "b1", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withUser(t, userTestBase(t), tt.pose)
			assert.Equal(t, tt.seen, userPoseSeen(t, m, tt.bg, tt.garment, UserID(tt.pose.ID)))
		})
	}
}

func TestPlan_AutoUserPoseOnUserBackground(t *testing.T) {
	m := withUser(t, userTestBase(t), userList()...)
	assert.True(t, userPoseSeen(t, m, "u4", false, "u5"))
}

func TestPlan_RandomPickIncludesUserItemsWithoutIdeas(t *testing.T) {
	tests := []struct {
		name   string
		aspect string
		themes []string
		prompt string
		field  func(Series) string
	}{
		{"outfit", "outfit", nil, "red dress", func(s Series) string { return s.Outfit }},
		{"outfit with theme", "outfit", []string{"winter"}, "red dress", func(s Series) string { return s.Outfit }},
		{"hair", "hair", nil, "braid", func(s Series) string { return s.Hair }},
		{"makeup", "makeup", []string{"winter"}, "smoky eyes", func(s Series) string { return s.Makeup }},
		{"background", "background", nil, "white wall", func(s Series) string { return s.Background }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withUser(t, userTestBase(t), userList()...)
			seen := false
			for seed := int64(1); seed <= 300 && !seen; seed++ {
				aspects := keepAll()
				aspects[tt.aspect] = AspectChoice{Mode: ModeAuto}
				series, _, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Themes: tt.themes, Seed: seed}, m, nil)
				require.NoError(t, err)
				for _, s := range series {
					if tt.field(s) == tt.prompt {
						seen = true
					}
				}
			}
			assert.True(t, seen)
		})
	}
}
