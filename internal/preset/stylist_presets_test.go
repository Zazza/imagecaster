package preset

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stylistPresetsDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "p.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func normPlaces(p StylistPreset) StylistPreset {
	if len(p.Places) == 0 {
		p.Places = nil
	}
	return p
}

func TestStylistPresets_EmptyList(t *testing.T) {
	t.Parallel()
	db := stylistPresetsDB(t)
	list, err := db.ListStylistPresets()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStylistPresets_CreateEachAspect(t *testing.T) {
	t.Parallel()
	db := stylistPresetsDB(t)
	in := []StylistPreset{
		{Aspect: "outfit", Name: "Красное платье", Prompt: "red dress", PromptRu: "Красное платье"},
		{Aspect: "hair", Name: "Коса", Prompt: "braid"},
		{Aspect: "makeup", Name: "Smoky", Prompt: "smoky eyes", PromptRu: "Дымчатые глаза"},
		{Aspect: "background", Name: "Мой пляж", Prompt: "white sand beach", Place: "beach"},
		{Aspect: "pose", Name: "Сидя", Prompt: "sitting on a chair", PromptRu: "Сидит на стуле", Framing: "knee", Places: []string{"indoor", "studio"}, Orientation: "landscape"},
	}
	var ids []int64
	for i := range in {
		p := in[i]
		require.NoError(t, db.CreateStylistPreset(&p))
		assert.Positive(t, p.ID)
		ids = append(ids, p.ID)
		in[i].ID = p.ID
	}
	list, err := db.ListStylistPresets()
	require.NoError(t, err)
	require.Len(t, list, len(in))
	for i := range in {
		assert.Equal(t, normPlaces(in[i]), normPlaces(list[i]))
	}
	for i := 1; i < len(ids); i++ {
		assert.Greater(t, ids[i], ids[i-1])
	}
}

func TestStylistPresets_UnknownAspectRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		aspect string
	}{
		{"unknown", "shoes"},
		{"empty", ""},
		{"wrong case", "Outfit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := stylistPresetsDB(t)
			p := StylistPreset{Aspect: tt.aspect, Name: "x", Prompt: "y"}
			assert.Error(t, db.CreateStylistPreset(&p))
			list, err := db.ListStylistPresets()
			require.NoError(t, err)
			assert.Empty(t, list)
		})
	}
}

func TestStylistPresets_Rename(t *testing.T) {
	t.Parallel()
	db := stylistPresetsDB(t)
	a := StylistPreset{Aspect: "hair", Name: "Коса", Prompt: "braid"}
	b := StylistPreset{Aspect: "hair", Name: "Пучок", Prompt: "bun"}
	require.NoError(t, db.CreateStylistPreset(&a))
	require.NoError(t, db.CreateStylistPreset(&b))
	require.NoError(t, db.RenameStylistPreset(a.ID, "Французская коса"))
	list, err := db.ListStylistPresets()
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "Французская коса", list[0].Name)
	assert.Equal(t, "braid", list[0].Prompt)
	assert.Equal(t, "Пучок", list[1].Name)
}

func TestStylistPresets_MissingIDIsError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		op   func(db *DB, id int64) error
	}{
		{"rename", func(db *DB, id int64) error { return db.RenameStylistPreset(id, "new") }},
		{"delete", func(db *DB, id int64) error { return db.DeleteStylistPreset(id) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := stylistPresetsDB(t)
			assert.Error(t, tt.op(db, 1))
			p := StylistPreset{Aspect: "outfit", Name: "a", Prompt: "b"}
			require.NoError(t, db.CreateStylistPreset(&p))
			assert.Error(t, tt.op(db, p.ID+100))
			assert.Error(t, tt.op(db, 0))
			require.NoError(t, db.DeleteStylistPreset(p.ID))
			assert.Error(t, tt.op(db, p.ID))
		})
	}
}

func TestStylistPresets_DeleteRemovesOnlyThatOne(t *testing.T) {
	t.Parallel()
	db := stylistPresetsDB(t)
	a := StylistPreset{Aspect: "makeup", Name: "a", Prompt: "pa"}
	b := StylistPreset{Aspect: "makeup", Name: "b", Prompt: "pb"}
	require.NoError(t, db.CreateStylistPreset(&a))
	require.NoError(t, db.CreateStylistPreset(&b))
	require.NoError(t, db.DeleteStylistPreset(a.ID))
	list, err := db.ListStylistPresets()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, b.ID, list[0].ID)
}

func TestStylistPresets_DeletedIDNotReused(t *testing.T) {
	t.Parallel()
	db := stylistPresetsDB(t)
	a := StylistPreset{Aspect: "outfit", Name: "a", Prompt: "pa"}
	b := StylistPreset{Aspect: "outfit", Name: "b", Prompt: "pb"}
	require.NoError(t, db.CreateStylistPreset(&a))
	require.NoError(t, db.CreateStylistPreset(&b))
	require.NoError(t, db.DeleteStylistPreset(b.ID))
	c := StylistPreset{Aspect: "outfit", Name: "c", Prompt: "pc"}
	require.NoError(t, db.CreateStylistPreset(&c))
	assert.Greater(t, c.ID, b.ID)
	require.NoError(t, db.DeleteStylistPreset(a.ID))
	require.NoError(t, db.DeleteStylistPreset(c.ID))
	d := StylistPreset{Aspect: "outfit", Name: "d", Prompt: "pd"}
	require.NoError(t, db.CreateStylistPreset(&d))
	assert.Greater(t, d.ID, c.ID)
}

func TestStylistPresets_SurviveReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.db")
	db, err := Open(path)
	require.NoError(t, err)
	p := StylistPreset{Aspect: "pose", Name: "Лёжа", Prompt: "lying", Framing: "full", Places: []string{"beach"}, Orientation: "landscape"}
	require.NoError(t, db.CreateStylistPreset(&p))
	require.NoError(t, db.Close())
	db, err = Open(path)
	require.NoError(t, err)
	defer db.Close()
	list, err := db.ListStylistPresets()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, p, list[0])
}
