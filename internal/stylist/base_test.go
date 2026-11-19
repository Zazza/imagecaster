package stylist

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Counts(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"themes", len(b.Themes), 12},
		{"outfit", len(b.Outfit), 40},
		{"hair", len(b.Hair), 25},
		{"makeup", len(b.Makeup), 15},
		{"background", len(b.Background), 30},
		{"pose", len(b.Pose), 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

func TestLoad_EveryPoseReachable(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	reached := map[string]bool{}
	var all []string
	for _, th := range b.Themes {
		all = append(all, th.ID)
	}
	for _, place := range append(b.Places(), "") {
		for _, p := range b.PosesFor(place, all) {
			reached[p.ID] = true
		}
	}
	for _, p := range b.Pose {
		assert.True(t, reached[p.ID], "pose %s is not reachable from any background", p.ID)
	}
}

func TestLoad_HairHasNoColour(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	for _, h := range b.Hair {
		assert.False(t, Forbidden(h.Prompt), "hair %s: %q", h.ID, h.Prompt)
	}
}

func TestLoad_PresetsPassForbiddenFilter(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	for _, kind := range []Aspect{AspectOutfit, AspectHair, AspectMakeup, AspectBackground} {
		for _, it := range b.Items(kind) {
			assert.False(t, Forbidden(it.Prompt), "%s %s: %q", kind, it.ID, it.Prompt)
		}
	}
}

func TestParse_Rejects(t *testing.T) {
	const ok = `"themes":[{"id":"t","name":"T"}],"outfit":[{"id":"o","name":"O","prompt":"o"}],"hair":[{"id":"h","name":"H","prompt":"h"}],"makeup":[{"id":"m","name":"M","prompt":"m"}]`
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"valid", `{` + ok + `,"background":[{"id":"b","theme":"t","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["beach"],"themes":["t"],"orientation":"portrait"},{"id":"pa","name":"A","prompt":"a","framing":"close","places":["any"],"orientation":"portrait"}]}`, true},
		{"no pose for any place", `{` + ok + `,"background":[{"id":"b","theme":"t","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["beach"],"orientation":"portrait"}]}`, false},
		{"only themed any pose", `{` + ok + `,"background":[{"id":"b","theme":"t","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["any"],"themes":["t"],"orientation":"portrait"}]}`, false},
		{"unknown pose theme", `{` + ok + `,"background":[{"id":"b","theme":"t","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["beach"],"themes":["zz"],"orientation":"portrait"},{"id":"pa","name":"A","prompt":"a","framing":"close","places":["any"],"orientation":"portrait"}]}`, false},
		{"unreachable pose", `{` + ok + `,"background":[{"id":"b","theme":"t","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["bed"],"orientation":"portrait"},{"id":"pa","name":"A","prompt":"a","framing":"close","places":["any"],"orientation":"portrait"}]}`, false},
		{"unknown theme", `{` + ok + `,"background":[{"id":"b","theme":"x","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["any"],"orientation":"portrait"}]}`, false},
		{"bad orientation", `{` + ok + `,"background":[{"id":"b","theme":"t","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["any"],"orientation":"square"}]}`, false},
		{"duplicate id", `{` + ok + `,"background":[{"id":"o","theme":"t","name":"B","prompt":"b","place":"beach"}],"pose":[{"id":"p","name":"P","prompt":"p","framing":"full","places":["any"],"orientation":"portrait"}]}`, false},
		{"not json", `{`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.raw))
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestPick_RespectsThemes(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	rnd := rand.New(rand.NewSource(1))
	for range 50 {
		it := b.Pick(AspectOutfit, []string{"beach", "winter"}, rnd)
		assert.Contains(t, []string{"beach", "winter"}, it.Theme)
		bg := b.Pick(AspectBackground, []string{"lingerie"}, rnd)
		assert.Equal(t, "lingerie", bg.Theme)
	}
	all := map[string]bool{}
	for range 200 {
		all[b.Pick(AspectHair, []string{"beach"}, rnd).ID] = true
	}
	assert.Greater(t, len(all), 5, "hair has no themes, any preset may be picked")
}

func TestPosesFor_OnlyMatchingPlace(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	for _, place := range b.Places() {
		for _, p := range b.PosesFor(place, nil) {
			assert.Empty(t, p.Themes, "themed pose %s without its theme", p.ID)
			assert.True(t, slices.Contains(p.Places, place) || slices.Contains(p.Places, PlaceAny), "pose %s for %s", p.ID, place)
		}
	}
}

func TestLoad_NoCoarseOutdoorPlace(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"beach", "bed", "indoor", "nature", "studio", "urban"}, b.Places())
}

func TestPresetNamesPassRussianFilter(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	for _, k := range []Aspect{AspectOutfit, AspectHair, AspectMakeup, AspectBackground} {
		for _, it := range b.Items(k) {
			assert.False(t, ForbiddenRu(it.Name), it.Name)
		}
	}
	for _, p := range b.Pose {
		assert.False(t, ForbiddenRu(p.Name), p.Name)
	}
}
