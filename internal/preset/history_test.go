package preset

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type histRow struct {
	session  int64
	source   string
	prompt   string
	w, h     int
	meta     string
	presetID *int64
	at       string
}

func addHist(t *testing.T, d *DB, r histRow) int64 {
	t.Helper()
	id, err := d.AddSessionItem(&SessionItem{
		SessionID: r.session, Source: r.source, Prompt: r.prompt,
		Width: r.w, Height: r.h, PresetID: r.presetID,
		FileName: "f.png", ThumbName: "t.jpg",
	})
	require.NoError(t, err)
	at := r.at
	if at == "" {
		at = "2026-11-10 12:00:00"
	}
	var meta any
	if r.meta != "" {
		meta = r.meta
	}
	_, err = d.db.Exec(`UPDATE session_items SET created_at = ?, meta = ? WHERE id = ?`, at, meta, id)
	require.NoError(t, err)
	return id
}

func newSession(t *testing.T, d *DB, name string) int64 {
	t.Helper()
	s, err := d.CreateSession(name)
	require.NoError(t, err)
	return s.ID
}

func listHist(t *testing.T, d *DB, f HistoryFilter) *HistoryPage {
	t.Helper()
	p, err := d.ListHistory(f)
	require.NoError(t, err)
	require.NotNil(t, p)
	return p
}

func ids(p *HistoryPage) []int64 {
	out := make([]int64, 0, len(p.Items))
	for _, it := range p.Items {
		out = append(out, it.ID)
	}
	return out
}

func facet(fs []HistoryFacet, value string) (HistoryFacet, bool) {
	for _, f := range fs {
		if f.Value == value {
			return f, true
		}
	}
	return HistoryFacet{}, false
}

func facetCount(fs []HistoryFacet, value string) int {
	f, _ := facet(fs, value)
	return f.Count
}

func TestListHistory_EmptyDB(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	p := listHist(t, d, HistoryFilter{})
	assert.Empty(t, p.Items)
	assert.Equal(t, 0, p.Total)
	for _, fs := range [][]HistoryFacet{p.Facets.Models, p.Facets.Labels, p.Facets.Kinds, p.Facets.Shapes, p.Facets.Sessions} {
		for _, f := range fs {
			assert.Equal(t, 0, f.Count, f.Value)
		}
	}
}

func TestListHistory_KindFromSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		source string
		kind   string
	}{
		{"generate", "generate"},
		{"from-image", "from_image"},
		{"compound", "pipeline"},
		{"compound-from-image", "pipeline"},
		{"edit", "edit"},
		{"look", "edit"},
		{"object", "edit"},
		{"repair", "edit"},
		{"stylist", "stylist"},
		{"editor", "editor"},
		{"sd", "other"},
		{"", "other"},
		{"upscale", "other"},
	}
	for _, c := range cases {
		t.Run(c.source, func(t *testing.T) {
			t.Parallel()
			d := testDB(t)
			s := newSession(t, d, "S")
			id := addHist(t, d, histRow{session: s, source: c.source, w: 512, h: 512})
			p := listHist(t, d, HistoryFilter{})
			require.Len(t, p.Items, 1)
			assert.Equal(t, id, p.Items[0].ID)
			assert.Equal(t, c.kind, p.Items[0].Kind)
			assert.Equal(t, 1, facetCount(p.Facets.Kinds, c.kind))

			f := listHist(t, d, HistoryFilter{Kinds: []string{c.kind}})
			assert.Equal(t, []int64{id}, ids(f))
		})
	}
}

func TestListHistory_Shape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		w, h  int
		shape string
	}{
		{"landscape", 1024, 768, "landscape"},
		{"portrait", 768, 1024, "portrait"},
		{"square", 1024, 1024, "square"},
		{"near square wide", 1050, 1000, "square"},
		{"near square tall", 1000, 1050, "square"},
		{"just landscape", 1060, 1000, "landscape"},
		{"just portrait", 1000, 1060, "portrait"},
		{"zero width", 0, 512, "unknown"},
		{"zero height", 512, 0, "unknown"},
		{"both zero", 0, 0, "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			d := testDB(t)
			s := newSession(t, d, "S")
			id := addHist(t, d, histRow{session: s, source: "generate", w: c.w, h: c.h})
			p := listHist(t, d, HistoryFilter{})
			require.Len(t, p.Items, 1)
			assert.Equal(t, c.shape, p.Items[0].Shape)
			assert.Equal(t, 1, facetCount(p.Facets.Shapes, c.shape))
			assert.Equal(t, []int64{id}, ids(listHist(t, d, HistoryFilter{Shapes: []string{c.shape}})))
		})
	}
}

func TestListHistory_ModelAndLabelFromMeta(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	s := newSession(t, d, "S")
	pr := &Preset{Name: "Cinematic Portrait", PresetType: "general"}
	require.NoError(t, d.Create(pr))

	withLabel := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, meta: `{"model":"flux1-dev","label":"Neon"}`})
	byPreset := addHist(t, d, histRow{session: s, source: "from-image", w: 1, h: 1, presetID: &pr.ID, meta: `{"model":"sdxl"}`})
	pipeline := addHist(t, d, histRow{session: s, source: "compound", w: 1, h: 1, meta: `{"model":"klein","label":"Two Steps"}`})
	editLabeled := addHist(t, d, histRow{session: s, source: "edit", w: 1, h: 1, meta: `{"model":"klein","label":"haircut"}`})
	stylist := addHist(t, d, histRow{session: s, source: "stylist", w: 1, h: 1, meta: `{"label":"bob"}`})
	noMeta := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1})

	p := listHist(t, d, HistoryFilter{})
	got := map[int64]HistoryItem{}
	for _, it := range p.Items {
		got[it.ID] = it
	}
	cases := []struct {
		id           int64
		model, label string
	}{
		{withLabel, "flux1-dev", "Neon"},
		{byPreset, "sdxl", "Cinematic Portrait"},
		{pipeline, "klein", "Two Steps"},
		{editLabeled, "klein", ""},
		{stylist, "", ""},
		{noMeta, "", ""},
	}
	for _, c := range cases {
		it, ok := got[c.id]
		require.True(t, ok, c.id)
		assert.Equal(t, c.model, it.ModelName, c.id)
		assert.Equal(t, c.label, it.Label, c.id)
	}

	assert.Equal(t, 1, facetCount(p.Facets.Models, "flux1-dev"))
	assert.Equal(t, 1, facetCount(p.Facets.Models, "sdxl"))
	assert.Equal(t, 2, facetCount(p.Facets.Models, "klein"))

	for _, f := range p.Facets.Labels {
		assert.NotEmpty(t, f.Value)
	}
	_, hasHaircut := facet(p.Facets.Labels, "haircut")
	assert.False(t, hasHaircut, "edit labels are not styles")
	_, hasBob := facet(p.Facets.Labels, "bob")
	assert.False(t, hasBob, "stylist labels are not styles")
	neon, ok := facet(p.Facets.Labels, "Neon")
	require.True(t, ok)
	assert.False(t, neon.Pipeline)
	assert.Equal(t, 1, neon.Count)
	cp, ok := facet(p.Facets.Labels, "Cinematic Portrait")
	require.True(t, ok)
	assert.False(t, cp.Pipeline)
	pf, ok := facet(p.Facets.Labels, "Two Steps")
	require.True(t, ok)
	assert.True(t, pf.Pipeline)

	assert.Equal(t, []int64{byPreset}, ids(listHist(t, d, HistoryFilter{Labels: []string{"Cinematic Portrait"}})))
	assert.ElementsMatch(t, []int64{withLabel, pipeline}, ids(listHist(t, d, HistoryFilter{Labels: []string{"Neon", "Two Steps"}})))
	assert.ElementsMatch(t, []int64{pipeline, editLabeled}, ids(listHist(t, d, HistoryFilter{Models: []string{"klein"}})))
	assert.Empty(t, ids(listHist(t, d, HistoryFilter{Labels: []string{"haircut"}})))
}

func TestListHistory_MalformedMeta(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	s := newSession(t, d, "S")
	bad := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, meta: `{"model": "x"`})
	ok := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, meta: `{"model":"good"}`})

	p := listHist(t, d, HistoryFilter{})
	assert.ElementsMatch(t, []int64{bad, ok}, ids(p))
	assert.Equal(t, 2, p.Total)
	for _, it := range p.Items {
		if it.ID == bad {
			assert.Equal(t, "", it.ModelName)
			assert.Equal(t, "", it.Label)
		}
	}
	assert.Equal(t, []int64{ok}, ids(listHist(t, d, HistoryFilter{Models: []string{"good"}})))
}

func TestListHistory_Sessions(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	a := newSession(t, d, "Портреты")
	b := newSession(t, d, "Pets")
	c := newSession(t, d, "Empty")
	a1 := addHist(t, d, histRow{session: a, source: "generate", w: 1, h: 1})
	a2 := addHist(t, d, histRow{session: a, source: "edit", w: 1, h: 1})
	b1 := addHist(t, d, histRow{session: b, source: "generate", w: 1, h: 1})

	p := listHist(t, d, HistoryFilter{})
	for _, it := range p.Items {
		switch it.ID {
		case a1, a2:
			assert.Equal(t, "Портреты", it.SessionName)
		case b1:
			assert.Equal(t, "Pets", it.SessionName)
		}
	}
	fa, ok := facet(p.Facets.Sessions, strconv.FormatInt(a, 10))
	require.True(t, ok)
	assert.Equal(t, "Портреты", fa.Name)
	assert.Equal(t, 2, fa.Count)
	fb, ok := facet(p.Facets.Sessions, strconv.FormatInt(b, 10))
	require.True(t, ok)
	assert.Equal(t, "Pets", fb.Name)
	assert.Equal(t, 1, fb.Count)

	assert.ElementsMatch(t, []int64{a1, a2}, ids(listHist(t, d, HistoryFilter{SessionIDs: []int64{a}})))
	assert.ElementsMatch(t, []int64{a1, a2, b1}, ids(listHist(t, d, HistoryFilter{SessionIDs: []int64{a, b}})))

	sel := listHist(t, d, HistoryFilter{SessionIDs: []int64{c}})
	assert.Empty(t, sel.Items)
	assert.Equal(t, 0, sel.Total)
	fc, ok := facet(sel.Facets.Sessions, strconv.FormatInt(c, 10))
	require.True(t, ok, "selected session with no items is still listed")
	assert.Equal(t, 0, fc.Count)
	assert.Equal(t, "Empty", fc.Name)
}

func TestListHistory_Search(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	s := newSession(t, d, "S")
	cat := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, prompt: "A ginger Cat on a roof"})
	kot := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, prompt: "Рыжий КОТ на крыше"})
	dog := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, prompt: "dog in the park"})

	cases := []struct {
		q    string
		want []int64
	}{
		{"cat", []int64{cat}},
		{"CAT", []int64{cat}},
		{"кот", []int64{kot}},
		{"Кот На", []int64{kot}},
		{"рыжий", []int64{kot}},
		{"ginger cat on", []int64{cat}},
		{"o", []int64{cat, dog}},
		{"zebra", nil},
		{"%", nil},
		{"_", nil},
	}
	for _, c := range cases {
		p := listHist(t, d, HistoryFilter{Search: c.q})
		assert.ElementsMatch(t, c.want, ids(p), c.q)
		assert.Equal(t, len(c.want), p.Total, c.q)
	}
	assert.ElementsMatch(t, []int64{cat, kot, dog}, ids(listHist(t, d, HistoryFilter{Search: ""})))
}

func TestListHistory_DateRange(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	s := newSession(t, d, "S")
	before := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-11-09 23:59:59"})
	start := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-11-10 00:00:00"})
	mid := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-11-10 13:30:00"})
	end := addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-11-11 00:00:00"})

	cases := []struct {
		name     string
		from, to string
		want     []int64
	}{
		{"date only", "2026-11-10", "2026-11-11", []int64{start, mid}},
		{"datetime", "2026-11-10 00:00:00", "2026-11-11 00:00:00", []int64{start, mid}},
		{"rfc3339 utc", "2026-11-10T00:00:00Z", "2026-11-11T00:00:00Z", []int64{start, mid}},
		{"rfc3339 offset", "2026-11-10T03:00:00+03:00", "2026-11-10T16:30:00+03:00", []int64{start}},
		{"from only", "2026-11-10 13:30:00", "", []int64{mid, end}},
		{"to only", "", "2026-11-10", []int64{before}},
		{"empty range", "2026-11-10", "2026-11-10", nil},
		{"no bounds", "", "", []int64{before, start, mid, end}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := listHist(t, d, HistoryFilter{DateFrom: c.from, DateTo: c.to})
			assert.ElementsMatch(t, c.want, ids(p))
			assert.Equal(t, len(c.want), p.Total)
		})
	}
}

func TestListHistory_InvalidDates(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	s := newSession(t, d, "S")
	addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1})
	cases := []HistoryFilter{
		{DateFrom: "yesterday"},
		{DateTo: "2026-13-01"},
		{DateFrom: "10.11.2026"},
		{DateTo: "2026-11-10 25:00:00"},
		{DateFrom: "2026-11-10'; DROP TABLE session_items; --"},
	}
	for _, f := range cases {
		_, err := d.ListHistory(f)
		assert.Error(t, err, fmt.Sprintf("%+v", f))
	}
	p := listHist(t, d, HistoryFilter{})
	assert.Equal(t, 1, p.Total)
}

func TestListHistory_OrderAndPaging(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	a := newSession(t, d, "A")
	b := newSession(t, d, "B")
	times := []string{
		"2026-11-01 10:00:00",
		"2026-11-05 10:00:00",
		"2026-11-03 10:00:00",
		"2026-11-05 10:00:00",
		"2026-11-02 10:00:00",
		"2026-11-05 10:00:00",
		"2026-11-04 10:00:00",
	}
	var all []int64
	for i, at := range times {
		sess := a
		if i%2 == 1 {
			sess = b
		}
		all = append(all, addHist(t, d, histRow{session: sess, source: "generate", w: 1, h: 1, at: at}))
	}
	want := []int64{all[5], all[3], all[1], all[6], all[2], all[4], all[0]}

	full := listHist(t, d, HistoryFilter{})
	assert.Equal(t, want, ids(full))
	assert.Equal(t, 7, full.Total)

	var paged []int64
	for off := 0; off < 10; off += 3 {
		p := listHist(t, d, HistoryFilter{Offset: off, Limit: 3})
		assert.Equal(t, 7, p.Total)
		assert.LessOrEqual(t, len(p.Items), 3)
		paged = append(paged, ids(p)...)
	}
	assert.Equal(t, want, paged)

	beyond := listHist(t, d, HistoryFilter{Offset: 100, Limit: 3})
	assert.Empty(t, beyond.Items)
	assert.Equal(t, 7, beyond.Total)
}

func TestListHistory_LimitBounds(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	s := newSession(t, d, "S")
	tx, err := d.db.Begin()
	require.NoError(t, err)
	for i := 0; i < 520; i++ {
		_, err := tx.Exec(`INSERT INTO session_items (session_id, source, width, height, created_at) VALUES (?, 'generate', 10, 10, ?)`,
			s, fmt.Sprintf("2026-11-01 00:%02d:%02d", (i/60)%60, i%60))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())

	cases := []struct {
		limit int
		want  int
	}{
		{0, 120},
		{-5, 120},
		{10, 10},
		{500, 500},
		{1000, 500},
	}
	for _, c := range cases {
		p := listHist(t, d, HistoryFilter{Limit: c.limit})
		assert.Len(t, p.Items, c.want, c.limit)
		assert.Equal(t, 520, p.Total)
	}
}

func TestListHistory_CombinedFiltersAndFacets(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	a := newSession(t, d, "A")
	b := newSession(t, d, "B")
	g1 := addHist(t, d, histRow{session: a, source: "generate", w: 1024, h: 768, prompt: "red fox", meta: `{"model":"flux","label":"Neon"}`})
	g2 := addHist(t, d, histRow{session: a, source: "generate", w: 768, h: 1024, prompt: "blue fox", meta: `{"model":"sdxl","label":"Neon"}`})
	g3 := addHist(t, d, histRow{session: b, source: "generate", w: 1024, h: 768, prompt: "red car", meta: `{"model":"flux","label":"Retro"}`})
	e1 := addHist(t, d, histRow{session: a, source: "edit", w: 1024, h: 768, prompt: "red fox edit", meta: `{"model":"klein"}`})
	p1 := addHist(t, d, histRow{session: b, source: "compound", w: 1024, h: 1024, prompt: "red fox", meta: `{"model":"flux","label":"Flow"}`})

	t.Run("each dimension combined", func(t *testing.T) {
		cases := []struct {
			name string
			f    HistoryFilter
			want []int64
		}{
			{"kind+model", HistoryFilter{Kinds: []string{"generate"}, Models: []string{"flux"}}, []int64{g1, g3}},
			{"kind+model+shape", HistoryFilter{Kinds: []string{"generate", "pipeline"}, Models: []string{"flux"}, Shapes: []string{"landscape"}}, []int64{g1, g3}},
			{"session+search", HistoryFilter{SessionIDs: []int64{a}, Search: "RED"}, []int64{g1, e1}},
			{"label+session", HistoryFilter{Labels: []string{"Neon", "Flow"}, SessionIDs: []int64{b}}, []int64{p1}},
			{"shape multi", HistoryFilter{Shapes: []string{"portrait", "square"}}, []int64{g2, p1}},
			{"all dims", HistoryFilter{SessionIDs: []int64{a, b}, Models: []string{"flux"}, Labels: []string{"Neon"}, Kinds: []string{"generate"}, Shapes: []string{"landscape"}, Search: "fox"}, []int64{g1}},
			{"no match", HistoryFilter{Kinds: []string{"stylist"}, Models: []string{"flux"}}, nil},
		}
		for _, c := range cases {
			p := listHist(t, d, c.f)
			assert.ElementsMatch(t, c.want, ids(p), c.name)
			assert.Equal(t, len(c.want), p.Total, c.name)
		}
	})

	t.Run("facet ignores its own filter but applies others", func(t *testing.T) {
		p := listHist(t, d, HistoryFilter{Kinds: []string{"generate"}, Models: []string{"flux"}})
		assert.Equal(t, 2, facetCount(p.Facets.Kinds, "generate"))
		assert.Equal(t, 1, facetCount(p.Facets.Kinds, "pipeline"))
		assert.Equal(t, 0, facetCount(p.Facets.Kinds, "edit"))

		assert.Equal(t, 2, facetCount(p.Facets.Models, "flux"))
		assert.Equal(t, 1, facetCount(p.Facets.Models, "sdxl"))
		assert.Equal(t, 0, facetCount(p.Facets.Models, "klein"))

		assert.Equal(t, 1, facetCount(p.Facets.Labels, "Neon"))
		assert.Equal(t, 1, facetCount(p.Facets.Labels, "Retro"))
		assert.Equal(t, 0, facetCount(p.Facets.Labels, "Flow"))

		assert.Equal(t, 2, facetCount(p.Facets.Shapes, "landscape"))
		assert.Equal(t, 0, facetCount(p.Facets.Shapes, "portrait"))

		assert.Equal(t, 1, facetCount(p.Facets.Sessions, strconv.FormatInt(a, 10)))
		assert.Equal(t, 1, facetCount(p.Facets.Sessions, strconv.FormatInt(b, 10)))
	})

	t.Run("search and dates narrow every facet", func(t *testing.T) {
		p := listHist(t, d, HistoryFilter{Search: "car"})
		assert.Equal(t, 1, facetCount(p.Facets.Kinds, "generate"))
		assert.Equal(t, 0, facetCount(p.Facets.Kinds, "edit"))
		assert.Equal(t, 1, facetCount(p.Facets.Models, "flux"))
		assert.Equal(t, 0, facetCount(p.Facets.Sessions, strconv.FormatInt(a, 10)))
	})

	t.Run("selected values with zero matches stay listed", func(t *testing.T) {
		p := listHist(t, d, HistoryFilter{Models: []string{"ghost"}, Labels: []string{"Nope"}, Kinds: []string{"stylist"}, Shapes: []string{"unknown"}})
		assert.Empty(t, p.Items)
		for _, c := range []struct {
			fs    []HistoryFacet
			value string
		}{
			{p.Facets.Models, "ghost"},
			{p.Facets.Labels, "Nope"},
			{p.Facets.Kinds, "stylist"},
			{p.Facets.Shapes, "unknown"},
		} {
			f, ok := facet(c.fs, c.value)
			assert.True(t, ok, c.value)
			assert.Equal(t, 0, f.Count, c.value)
		}
	})
}

func TestListHistory_PeriodFacets(t *testing.T) {
	t.Parallel()
	d := testDB(t)
	s := newSession(t, d, "S")
	addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-11-13 08:00:00"})
	addHist(t, d, histRow{session: s, source: "edit", w: 1, h: 1, at: "2026-11-13 09:00:00"})
	addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-11-10 12:00:00"})
	addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-10-20 12:00:00"})
	addHist(t, d, histRow{session: s, source: "generate", w: 1, h: 1, at: "2026-09-01 12:00:00"})
	starts := map[string]string{
		"today": "2026-11-13T00:00:00Z",
		"week":  "2026-11-07T00:00:00Z",
		"month": "2026-10-15T00:00:00Z",
	}

	t.Run("all first, then each start", func(t *testing.T) {
		p := listHist(t, d, HistoryFilter{PeriodStarts: starts})
		require.NotEmpty(t, p.Facets.Periods)
		assert.Equal(t, "all", p.Facets.Periods[0].Value)
		assert.Equal(t, 5, facetCount(p.Facets.Periods, "all"))
		assert.Equal(t, 2, facetCount(p.Facets.Periods, "today"))
		assert.Equal(t, 3, facetCount(p.Facets.Periods, "week"))
		assert.Equal(t, 4, facetCount(p.Facets.Periods, "month"))
		assert.Len(t, p.Facets.Periods, 4)
	})

	t.Run("ignores date filter, honors other filters", func(t *testing.T) {
		p := listHist(t, d, HistoryFilter{PeriodStarts: starts, Kinds: []string{"generate"}, DateFrom: "2026-11-13", DateTo: "2026-11-14"})
		assert.Equal(t, 1, p.Total)
		assert.Equal(t, 4, facetCount(p.Facets.Periods, "all"))
		assert.Equal(t, 1, facetCount(p.Facets.Periods, "today"))
		assert.Equal(t, 2, facetCount(p.Facets.Periods, "week"))
		assert.Equal(t, 3, facetCount(p.Facets.Periods, "month"))
	})

	t.Run("start is inclusive", func(t *testing.T) {
		p := listHist(t, d, HistoryFilter{PeriodStarts: map[string]string{"today": "2026-11-13T08:00:00Z"}})
		assert.Equal(t, 2, facetCount(p.Facets.Periods, "today"))
	})

	t.Run("no starts gives only all", func(t *testing.T) {
		p := listHist(t, d, HistoryFilter{})
		require.Len(t, p.Facets.Periods, 1)
		assert.Equal(t, "all", p.Facets.Periods[0].Value)
		assert.Equal(t, 5, p.Facets.Periods[0].Count)
	})

	t.Run("invalid start is an error", func(t *testing.T) {
		_, err := d.ListHistory(HistoryFilter{PeriodStarts: map[string]string{"today": "сегодня"}})
		assert.Error(t, err)
	})
}
