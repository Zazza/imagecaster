package stylist

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustBase(t *testing.T) *Base {
	t.Helper()
	b, err := Load()
	require.NoError(t, err)
	return b
}

func allKeep() map[string]AspectChoice {
	return map[string]AspectChoice{
		"outfit": {Mode: ModeKeep}, "hair": {Mode: ModeKeep}, "makeup": {Mode: ModeKeep},
		"background": {Mode: ModeKeep}, "pose": {Mode: ModeKeep},
	}
}

func TestSeriesSizes(t *testing.T) {
	tests := []struct {
		frames int
		want   []int
	}{
		{1, []int{1}},
		{4, []int{4}},
		{6, []int{4, 2}},
		{24, []int{4, 4, 4, 4, 4, 4}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, SeriesSizes(tt.frames), "frames %d", tt.frames)
	}
}

func TestPlan_FramesLimits(t *testing.T) {
	b := mustBase(t)
	tests := []struct {
		frames int
		ok     bool
	}{{0, false}, {1, true}, {24, true}, {25, false}}
	for _, tt := range tests {
		_, _, err := Plan(PlanRequest{Frames: tt.frames, Aspects: allKeep()}, b, nil)
		if tt.ok {
			assert.NoError(t, err, "frames %d", tt.frames)
		} else {
			assert.Error(t, err, "frames %d", tt.frames)
		}
	}
}

func TestPlan_KeepRemovesAspectFromPrompt(t *testing.T) {
	b := mustBase(t)
	tests := []struct {
		aspect string
		field  func(Series) string
	}{
		{"outfit", func(s Series) string { return s.Outfit }},
		{"hair", func(s Series) string { return s.Hair }},
		{"makeup", func(s Series) string { return s.Makeup }},
		{"background", func(s Series) string { return s.Background }},
	}
	for _, tt := range tests {
		t.Run(tt.aspect, func(t *testing.T) {
			aspects := map[string]AspectChoice{}
			for _, a := range []string{"outfit", "hair", "makeup", "background", "pose"} {
				aspects[a] = AspectChoice{Mode: ModePick, IDs: []string{"o01", "h01", "m04", "b03", "p01"}}
			}
			aspects[tt.aspect] = AspectChoice{Mode: ModeKeep, IDs: []string{"o01", "h01", "m04", "b03"}}
			series, _, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Seed: 1}, b, nil)
			require.NoError(t, err)
			for _, s := range series {
				assert.Empty(t, tt.field(s))
				for _, sh := range s.Shots {
					prompt := FramePrompt("SDXL", "lora", "a woman", s, sh)
					for _, id := range []string{"o01", "h01", "m04", "b03"} {
						var kind Aspect
						switch id[0] {
						case 'o':
							kind = AspectOutfit
						case 'h':
							kind = AspectHair
						case 'm':
							kind = AspectMakeup
						case 'b':
							kind = AspectBackground
						}
						it, _ := b.Find(kind, id)
						want := it.Prompt
						if kind == AspectOutfit && (sh.Framing == "close" || sh.Framing == "waist") {
							want = upperOutfit(want)
						}
						if string(kind) == tt.aspect {
							assert.NotContains(t, prompt, want)
						} else {
							assert.Contains(t, prompt, want)
						}
					}
				}
			}
		})
	}
}

func TestPlan_PickRoundRobin(t *testing.T) {
	b := mustBase(t)
	aspects := allKeep()
	aspects["outfit"] = AspectChoice{Mode: ModePick, IDs: []string{"o01", "nope", "o02"}}
	aspects["pose"] = AspectChoice{Mode: ModePick, IDs: []string{"p01", "p02", "p03"}}
	series, warnings, err := Plan(PlanRequest{Frames: 10, Aspects: aspects, Seed: 3}, b, nil)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, series, 3)
	o1, _ := b.Find(AspectOutfit, "o01")
	o2, _ := b.Find(AspectOutfit, "o02")
	assert.Equal(t, []string{o1.Prompt, o2.Prompt, o1.Prompt}, []string{series[0].Outfit, series[1].Outfit, series[2].Outfit})
	var poses []string
	for _, s := range series {
		for _, sh := range s.Shots {
			poses = append(poses, sh.Prompt)
		}
	}
	p := func(id string) string { v, _ := b.FindPose(id); return v.Prompt }
	want := []string{p("p01"), p("p02"), p("p03"), p("p01"), p("p02"), p("p03"), p("p01"), p("p02"), p("p03"), p("p01")}
	assert.Equal(t, want, poses)
}

func TestPlan_PickUnknownFallsBackWithWarning(t *testing.T) {
	b := mustBase(t)
	aspects := allKeep()
	aspects["hair"] = AspectChoice{Mode: ModePick, IDs: []string{"zzz"}}
	series, warnings, err := Plan(PlanRequest{Frames: 1, Aspects: aspects, Seed: 3}, b, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, series[0].Hair)
	assert.Len(t, warnings, 1)
}

func TestPlan_AutoPosesFitBackgroundPlace(t *testing.T) {
	b := mustBase(t)
	for seed := int64(1); seed <= 30; seed++ {
		aspects := allKeep()
		aspects["background"] = AspectChoice{Mode: ModeAuto}
		aspects["pose"] = AspectChoice{Mode: ModeAuto}
		series, _, err := Plan(PlanRequest{Frames: 8, Aspects: aspects, Seed: seed}, b, nil)
		require.NoError(t, err)
		for _, s := range series {
			require.NotEmpty(t, s.Place)
			seen := map[string]bool{}
			for _, sh := range s.Shots {
				var pose Pose
				for _, p := range b.Pose {
					if p.Prompt == sh.Prompt {
						pose = p
					}
				}
				require.NotEmpty(t, pose.ID, sh.Prompt)
				assert.True(t, slices.Contains(pose.Places, s.Place) || slices.Contains(pose.Places, PlaceAny), "pose %s at %s", pose.ID, s.Place)
				assert.Equal(t, pose.Orientation == "landscape", sh.Landscape)
				assert.False(t, seen[pose.ID], "pose repeats within a series")
				seen[pose.ID] = true
			}
		}
	}
}

func TestPlan_LandscapeFromPose(t *testing.T) {
	b := mustBase(t)
	var land, port Pose
	for _, p := range b.Pose {
		if p.Orientation == "landscape" && land.ID == "" {
			land = p
		}
		if p.Orientation == "portrait" && port.ID == "" {
			port = p
		}
	}
	aspects := allKeep()
	aspects["pose"] = AspectChoice{Mode: ModePick, IDs: []string{land.ID, port.ID}}
	series, _, err := Plan(PlanRequest{Frames: 2, Aspects: aspects}, b, nil)
	require.NoError(t, err)
	assert.True(t, series[0].Shots[0].Landscape)
	assert.False(t, series[0].Shots[1].Landscape)
}

func TestPlan_GarmentLocksOutfit(t *testing.T) {
	b := mustBase(t)
	aspects := allKeep()
	aspects["outfit"] = AspectChoice{Mode: ModePick, IDs: []string{"o01"}}
	series, _, err := Plan(PlanRequest{Frames: 4, Aspects: aspects, Garment: true}, b, nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultOutfit, series[0].Outfit)
	req := PlanRequest{Garment: true, Aspects: map[string]AspectChoice{}}
	assert.NotContains(t, req.AutoAspects(), AspectOutfit)
}

func TestPlan_NoIdeasUsesThemedPresetsWithWarning(t *testing.T) {
	b := mustBase(t)
	series, warnings, err := Plan(PlanRequest{Frames: 8, Themes: []string{"beach"}, Seed: 5}, b, nil)
	require.NoError(t, err)
	assert.Contains(t, warnings, warnNoIdeas)
	for _, s := range series {
		found := false
		for _, it := range b.Items(AspectOutfit) {
			if it.Prompt == s.Outfit {
				assert.Equal(t, "beach", it.Theme)
				found = true
			}
		}
		assert.True(t, found, s.Outfit)
		assert.Equal(t, "beach", s.Place)
	}
}

func TestPlan_UsesIdeas(t *testing.T) {
	b := mustBase(t)
	ideas := []Idea{{Outfit: "red silk dress", Hair: "low bun", Makeup: "soft makeup", Background: "a quiet library", Place: "indoor"}}
	series, warnings, err := Plan(PlanRequest{Frames: 2, Seed: 1}, b, ideas)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	s := series[0]
	assert.Equal(t, []string{"red silk dress", "low bun", "soft makeup", "a quiet library", "indoor"}, []string{s.Outfit, s.Hair, s.Makeup, s.Background, s.Place})
}

func TestRegenerateSeries(t *testing.T) {
	b := mustBase(t)
	aspects := allKeep()
	aspects["outfit"] = AspectChoice{Mode: ModePick, IDs: []string{"o01", "o02"}}
	s, _, err := RegenerateSeries(PlanRequest{Frames: 6, Aspects: aspects}, b, nil, 1)
	require.NoError(t, err)
	o2, _ := b.Find(AspectOutfit, "o02")
	assert.Equal(t, o2.Prompt, s.Outfit)
	assert.Len(t, s.Shots, 2)
	_, _, err = RegenerateSeries(PlanRequest{Frames: 6, Aspects: aspects}, b, nil, 2)
	assert.Error(t, err)
}

func TestFramePrompt(t *testing.T) {
	s := Series{Outfit: "a floor-length dress, high heels", Hair: "", Makeup: "red lips", Background: "studio backdrop"}
	tail := "red lips, studio backdrop, " + QualityStudio + " <lora:anna:1>"
	tests := []struct {
		name    string
		framing string
		want    string
	}{
		{"no framing keeps the whole outfit", "", "facing camera, a woman, a floor-length dress, high heels, " + tail},
		{"close-up leads and drops shoes and length", "close", "close-up portrait, head and shoulders, facing camera, a woman, a dress, " + tail},
		{"waist-up leads and drops shoes and length", "waist", "waist-up shot, upper body, facing camera, a woman, a dress, " + tail},
		{"knee-up keeps the whole outfit", "knee", "knee-up shot, facing camera, a woman, a floor-length dress, high heels, " + tail},
		{"full body keeps the whole outfit", "full", "full body shot, facing camera, a woman, a floor-length dress, high heels, " + tail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FramePrompt("SDXL", "anna", "a woman", s, Shot{Prompt: "facing camera", Framing: tt.framing})
			assert.Equal(t, tt.want, got)
			assert.False(t, strings.Contains(got, ", ,"))
		})
	}
}

func TestUpperOutfit(t *testing.T) {
	tests := []struct{ in, want string }{
		{"floor-length black satin evening gown, thin straps, open back", "black satin evening gown, thin straps, open back"},
		{"long chiffon evening gown with off-the-shoulder neckline, floor length, sheer fabric, deep plum color, strappy high heels, gold clutch", "long chiffon evening gown with off-the-shoulder neckline, sheer fabric, deep plum color, gold clutch"},
		{"white silk blouse, black pencil skirt, black pumps", "white silk blouse, black pencil skirt"},
		{"black lace lingerie set, sheer black stockings, open black silk robe", "black lace lingerie set, open black silk robe"},
		{"midi wrap dress reaching to the knees, ankle boots", "wrap dress"},
		{"white cotton t-shirt, oversized denim jacket", "white cotton t-shirt, oversized denim jacket"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, upperOutfit(tt.in))
		})
	}
}

func TestPlan_ShotsCarryPoseFraming(t *testing.T) {
	b := mustBase(t)
	series, _, err := Plan(PlanRequest{Frames: 8, Seed: 3}, b, nil)
	require.NoError(t, err)
	for _, s := range series {
		for _, sh := range s.Shots {
			assert.Contains(t, []string{"close", "waist", "knee", "full"}, sh.Framing, sh.Pose)
		}
	}
}

func TestPlan_NightUrbanGetsNoGrassOrSportPoses(t *testing.T) {
	b := mustBase(t)
	night, ok := b.Find(AspectBackground, "b29")
	require.True(t, ok)
	require.Equal(t, "urban", b.PlaceOf(night.ID))
	for _, themes := range [][]string{nil, {"street"}, {"party", "evening"}} {
		for seed := int64(1); seed <= 40; seed++ {
			aspects := allKeep()
			aspects["background"] = AspectChoice{Mode: ModePick, IDs: []string{"b29"}}
			aspects["pose"] = AspectChoice{Mode: ModeAuto}
			series, _, err := Plan(PlanRequest{Frames: 24, Themes: themes, Aspects: aspects, Seed: seed}, b, nil)
			require.NoError(t, err)
			for _, s := range series {
				for _, sh := range s.Shots {
					p := strings.ToLower(sh.Prompt)
					assert.NotContains(t, p, "grass", sh.Pose)
					assert.NotContains(t, p, "tennis", sh.Pose)
					assert.NotContains(t, p, "stretching, one leg", sh.Pose)
					assert.NotContains(t, p, "sand", sh.Pose)
				}
			}
		}
	}
}

func TestPlan_SportPosesOnlyForSport(t *testing.T) {
	b := mustBase(t)
	found := false
	for seed := int64(1); seed <= 60 && !found; seed++ {
		aspects := allKeep()
		aspects["background"] = AspectChoice{Mode: ModePick, IDs: []string{"b26"}}
		aspects["pose"] = AspectChoice{Mode: ModeAuto}
		series, _, err := Plan(PlanRequest{Frames: 24, Aspects: aspects, Seed: seed}, b, nil)
		require.NoError(t, err)
		for _, s := range series {
			for _, sh := range s.Shots {
				if strings.Contains(sh.Prompt, "tennis racket") {
					found = true
				}
			}
		}
	}
	assert.True(t, found, "a sport background allows sport poses")
}

func TestPlan_RussianText(t *testing.T) {
	b := mustBase(t)
	o1, _ := b.Find(AspectOutfit, "o01")
	h := b.Hair[0]
	m := b.Makeup[0]
	bg := b.Background[0]
	pick := func(id string) AspectChoice { return AspectChoice{Mode: ModePick, IDs: []string{id}} }
	presets := map[string]AspectChoice{"outfit": pick(o1.ID), "hair": pick(h.ID), "makeup": pick(m.ID), "background": pick(bg.ID)}
	tests := []struct {
		name  string
		req   PlanRequest
		ideas []Idea
		want  Series
	}{
		{
			name: "presets carry Russian names",
			req:  PlanRequest{Frames: 1, Aspects: presets, Seed: 1},
			want: Series{Outfit: o1.Prompt, OutfitRu: o1.Name, Hair: h.Prompt, HairRu: h.Name, Makeup: m.Prompt, MakeupRu: m.Name, Background: bg.Prompt, BackgroundRu: bg.Name, Place: bg.Place},
		},
		{
			name:  "ideas carry their Russian text",
			req:   PlanRequest{Frames: 1, Seed: 1},
			ideas: []Idea{{Outfit: "red silk dress", OutfitRu: "красное шёлковое платье", Hair: "low bun", Makeup: "soft makeup", MakeupRu: "мягкий макияж", Background: "a quiet library", BackgroundRu: "тихая библиотека", Place: "indoor"}},
			want:  Series{Outfit: "red silk dress", OutfitRu: "красное шёлковое платье", Hair: "low bun", Makeup: "soft makeup", MakeupRu: "мягкий макияж", Background: "a quiet library", BackgroundRu: "тихая библиотека", Place: "indoor", RuAI: []string{"outfit", "makeup", "background"}},
		},
		{
			name:  "keep leaves both empty, garment drops the outfit Russian text and its mark",
			req:   PlanRequest{Frames: 1, Aspects: map[string]AspectChoice{"hair": {Mode: ModeKeep}, "makeup": {Mode: ModeKeep}, "background": {Mode: ModeKeep}, "pose": {Mode: ModeKeep}}, Garment: true, Seed: 1},
			ideas: []Idea{{Outfit: "red silk dress", OutfitRu: "красное шёлковое платье"}},
			want:  Series{Outfit: DefaultOutfit},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			series, _, err := Plan(tt.req, b, tt.ideas)
			require.NoError(t, err)
			got := series[0]
			if tt.want.OutfitRu != "" {
				assert.NotContains(t, FramePrompt("SDXL", "x", "a woman", got, Shot{}), tt.want.OutfitRu)
			}
			got.Shots = nil
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlan_ShotsShowRussianPoseName(t *testing.T) {
	b := mustBase(t)
	p := b.Pose[0]
	aspects := allKeep()
	aspects["pose"] = AspectChoice{Mode: ModePick, IDs: []string{p.ID}}
	series, _, err := Plan(PlanRequest{Frames: 1, Aspects: aspects}, b, nil)
	require.NoError(t, err)
	sh := series[0].Shots[0]
	assert.Equal(t, p.Name, sh.Pose)
	assert.Equal(t, p.Name, sh.PromptRu)
	assert.Equal(t, p.Prompt, sh.Prompt)
	assert.NotContains(t, FramePrompt("SDXL", "x", "a woman", series[0], sh), p.Name)
}

func TestPlan_GarmentAvoidsCloseAndWaistShots(t *testing.T) {
	b := mustBase(t)
	for seed := int64(1); seed <= 30; seed++ {
		series, _, err := Plan(PlanRequest{Frames: 12, Garment: true, Seed: seed}, b, nil)
		require.NoError(t, err)
		for _, s := range series {
			for _, sh := range s.Shots {
				assert.Contains(t, []string{"knee", "full"}, sh.Framing, "seed %d pose %s", seed, sh.Pose)
			}
		}
	}
	series, _, err := Plan(PlanRequest{Frames: 12, Seed: 1}, b, nil)
	require.NoError(t, err)
	var closeUps int
	for _, s := range series {
		for _, sh := range s.Shots {
			if sh.Framing == "close" {
				closeUps++
			}
		}
	}
	assert.Positive(t, closeUps, "without a garment close-ups are still used")
}

func TestPlan_GarmentWarnsOnPickedCloseUp(t *testing.T) {
	b := mustBase(t)
	pick := map[string]AspectChoice{string(AspectPose): {Mode: ModePick, IDs: []string{"p08", "p31"}}}
	_, warns, err := Plan(PlanRequest{Frames: 8, Garment: true, Aspects: pick, Seed: 1}, b, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, countOf(warns, warnCloseGarment), "one warning, not one per series")
	_, warns, err = Plan(PlanRequest{Frames: 8, Aspects: pick, Seed: 1}, b, nil)
	require.NoError(t, err)
	assert.Zero(t, countOf(warns, warnCloseGarment))
}

func countOf(list []string, s string) int {
	n := 0
	for _, v := range list {
		if v == s {
			n++
		}
	}
	return n
}
