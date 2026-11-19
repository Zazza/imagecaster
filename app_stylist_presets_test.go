package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/stylist"
)

type spRename struct {
	id   int64
	name string
}

type spPresetStore struct {
	list      []preset.StylistPreset
	listErr   error
	createErr error
	opErr     error
	nextID    int64
	created   []preset.StylistPreset
	renamed   []spRename
	deleted   []int64
}

func (s *spPresetStore) ListStylistPresets() ([]preset.StylistPreset, error) {
	return s.list, s.listErr
}

func (s *spPresetStore) CreateStylistPreset(p *preset.StylistPreset) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.nextID == 0 {
		s.nextID = 1
	}
	p.ID = s.nextID
	s.nextID++
	s.created = append(s.created, *p)
	return nil
}

func (s *spPresetStore) RenameStylistPreset(id int64, name string) error {
	s.renamed = append(s.renamed, spRename{id, name})
	return s.opErr
}

func (s *spPresetStore) DeleteStylistPreset(id int64) error {
	s.deleted = append(s.deleted, id)
	return s.opErr
}

func spStylistBase(t *testing.T) *stylist.Base {
	t.Helper()
	b, err := stylist.Load()
	require.NoError(t, err)
	return b
}

func TestStylistUserBase_MergesStoredPresets(t *testing.T) {
	b := spStylistBase(t)
	outfits, poses := len(b.Outfit), len(b.Pose)
	store := &spPresetStore{list: []preset.StylistPreset{
		{ID: 3, Aspect: "outfit", Name: "Красное платье", Prompt: "red dress", PromptRu: "Красное шёлковое платье"},
		{ID: 9, Aspect: "pose", Name: "Сидя", Prompt: "sitting on a stool", Framing: "knee", Places: []string{"studio"}, Orientation: "landscape"},
		{ID: 10, Aspect: "background", Name: "Мой пляж", Prompt: "white sand", Place: "beach"},
	}}
	var logged []error
	m, err := stylistUserBase(store, b, func(e error) { logged = append(logged, e) })
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Empty(t, logged)

	it, ok := m.Find(stylist.AspectOutfit, "u3")
	require.True(t, ok)
	assert.Equal(t, "red dress", it.Prompt)
	assert.Equal(t, "Красное шёлковое платье", it.PromptRu)
	assert.True(t, it.User)
	p, ok := m.FindPose("u9")
	require.True(t, ok)
	assert.Equal(t, "knee", p.Framing)
	assert.Equal(t, []string{"studio"}, p.Places)
	assert.Equal(t, "landscape", p.Orientation)
	assert.Equal(t, "beach", m.PlaceOf("u10"))

	assert.Len(t, b.Outfit, outfits)
	assert.Len(t, b.Pose, poses)
}

func TestStylistUserBase_EmptyStore(t *testing.T) {
	b := spStylistBase(t)
	m, err := stylistUserBase(&spPresetStore{}, b, func(error) { t.Fatal("nothing to log") })
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Len(t, m.Outfit, len(b.Outfit))
	assert.Len(t, m.Pose, len(b.Pose))
}

func TestStylistUserBase_ListErrorIsError(t *testing.T) {
	_, err := stylistUserBase(&spPresetStore{listErr: errors.New("db locked")}, spStylistBase(t), func(error) {})
	assert.Error(t, err)
}

func TestStylistUserBase_InvalidLoggedAndSkipped(t *testing.T) {
	b := spStylistBase(t)
	store := &spPresetStore{list: []preset.StylistPreset{
		{ID: 1, Aspect: "hair", Name: "Коса", Prompt: "braid"},
		{ID: 2, Aspect: "pose", Name: "Плохая", Prompt: "bad", Framing: "wide", Places: []string{"beach"}, Orientation: "portrait"},
		{ID: 3, Aspect: "background", Name: "Марс", Prompt: "mars", Place: "mars"},
	}}
	var logged []error
	m, err := stylistUserBase(store, b, func(e error) { logged = append(logged, e) })
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.NotEmpty(t, logged)
	_, ok := m.Find(stylist.AspectHair, "u1")
	assert.True(t, ok)
	_, ok = m.FindPose("u2")
	assert.False(t, ok)
	_, ok = m.Find(stylist.AspectBackground, "u3")
	assert.False(t, ok)
}

func TestSaveStylistPreset_Valid(t *testing.T) {
	tests := []struct {
		name string
		in   stylist.UserPreset
		want preset.StylistPreset
	}{
		{
			"outfit trims text",
			stylist.UserPreset{Aspect: stylist.AspectOutfit, Name: "  Красное платье ", Prompt: " red dress  ", PromptRu: "  Красное платье\n"},
			preset.StylistPreset{Aspect: "outfit", Name: "Красное платье", Prompt: "red dress", PromptRu: "Красное платье"},
		},
		{
			"background with place",
			stylist.UserPreset{Aspect: stylist.AspectBackground, Name: "Мой пляж", Prompt: "white sand", Place: "beach"},
			preset.StylistPreset{Aspect: "background", Name: "Мой пляж", Prompt: "white sand", Place: "beach"},
		},
		{
			"pose with framing and places",
			stylist.UserPreset{Aspect: stylist.AspectPose, Name: "Сидя", Prompt: "sitting", Framing: "knee", Places: []string{"studio", "indoor"}, Orientation: "landscape"},
			preset.StylistPreset{Aspect: "pose", Name: "Сидя", Prompt: "sitting", Framing: "knee", Places: []string{"studio", "indoor"}, Orientation: "landscape"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &spPresetStore{nextID: 7}
			id, err := saveStylistPreset(store, spStylistBase(t), tt.in)
			require.NoError(t, err)
			assert.Equal(t, "u7", id)
			require.Len(t, store.created, 1)
			got := store.created[0]
			assert.Equal(t, tt.want.Aspect, got.Aspect)
			assert.Equal(t, tt.want.Name, got.Name)
			assert.Equal(t, tt.want.Prompt, got.Prompt)
			assert.Equal(t, tt.want.PromptRu, got.PromptRu)
			if tt.want.Aspect == "background" {
				assert.Equal(t, tt.want.Place, got.Place)
			}
			if tt.want.Aspect == "pose" {
				assert.Equal(t, tt.want.Framing, got.Framing)
				assert.Equal(t, tt.want.Places, got.Places)
				assert.Equal(t, tt.want.Orientation, got.Orientation)
			}
		})
	}
}

func TestSaveStylistPreset_InvalidDoesNotStore(t *testing.T) {
	tests := []struct {
		name string
		in   stylist.UserPreset
	}{
		{"blank name", stylist.UserPreset{Aspect: stylist.AspectHair, Name: "   ", Prompt: "braid"}},
		{"blank prompt", stylist.UserPreset{Aspect: stylist.AspectHair, Name: "Коса", Prompt: "  "}},
		{"long name", stylist.UserPreset{Aspect: stylist.AspectHair, Name: strings.Repeat("я", stylist.UserNameMax+1), Prompt: "braid"}},
		{"unknown aspect", stylist.UserPreset{Aspect: "shoes", Name: "Туфли", Prompt: "heels"}},
		{"background without place", stylist.UserPreset{Aspect: stylist.AspectBackground, Name: "Фон", Prompt: "wall"}},
		{"background unknown place", stylist.UserPreset{Aspect: stylist.AspectBackground, Name: "Фон", Prompt: "wall", Place: "mars"}},
		{"pose unknown framing", stylist.UserPreset{Aspect: stylist.AspectPose, Name: "П", Prompt: "p", Framing: "wide", Places: []string{"any"}, Orientation: "portrait"}},
		{"pose without places", stylist.UserPreset{Aspect: stylist.AspectPose, Name: "П", Prompt: "p", Framing: "full", Orientation: "portrait"}},
		{"pose unknown place", stylist.UserPreset{Aspect: stylist.AspectPose, Name: "П", Prompt: "p", Framing: "full", Places: []string{"mars"}, Orientation: "portrait"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &spPresetStore{}
			id, err := saveStylistPreset(store, spStylistBase(t), tt.in)
			assert.Error(t, err)
			assert.Empty(t, id)
			assert.Empty(t, store.created)
		})
	}
}

func TestSaveStylistPreset_StoreError(t *testing.T) {
	store := &spPresetStore{createErr: errors.New("db locked")}
	id, err := saveStylistPreset(store, spStylistBase(t), stylist.UserPreset{Aspect: stylist.AspectHair, Name: "Коса", Prompt: "braid"})
	assert.Error(t, err)
	assert.Empty(t, id)
}

func TestRenameStylistPreset(t *testing.T) {
	store := &spPresetStore{}
	require.NoError(t, renameStylistPreset(store, "u3", "Новое имя"))
	assert.Equal(t, []spRename{{3, "Новое имя"}}, store.renamed)

	store = &spPresetStore{}
	require.NoError(t, renameStylistPreset(store, "u12", strings.Repeat("я", stylist.UserNameMax)))
	require.Len(t, store.renamed, 1)
	assert.Equal(t, int64(12), store.renamed[0].id)
}

func TestRenameStylistPreset_RejectedWithoutStore(t *testing.T) {
	tests := []struct {
		name string
		id   string
		nm   string
	}{
		{"builtin id", "o1", "x"},
		{"bare u", "u", "x"},
		{"zero", "u0", "x"},
		{"not a number", "ux", "x"},
		{"empty id", "", "x"},
		{"empty name", "u3", ""},
		{"blank name", "u3", "   "},
		{"long name", "u3", strings.Repeat("я", stylist.UserNameMax+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &spPresetStore{}
			assert.Error(t, renameStylistPreset(store, tt.id, tt.nm))
			assert.Empty(t, store.renamed)
		})
	}
}

func TestRenameStylistPreset_StoreError(t *testing.T) {
	assert.Error(t, renameStylistPreset(&spPresetStore{opErr: errors.New("нет такой")}, "u5", "x"))
}

func TestDeleteStylistPreset(t *testing.T) {
	store := &spPresetStore{}
	require.NoError(t, deleteStylistPreset(store, "u4"))
	assert.Equal(t, []int64{4}, store.deleted)
}

func TestDeleteStylistPreset_BadIDWithoutStore(t *testing.T) {
	for _, id := range []string{"o1", "p01", "u", "u0", "ux", "", "u-1"} {
		t.Run(id, func(t *testing.T) {
			store := &spPresetStore{}
			assert.Error(t, deleteStylistPreset(store, id))
			assert.Empty(t, store.deleted)
		})
	}
}

func TestDeleteStylistPreset_StoreError(t *testing.T) {
	assert.Error(t, deleteStylistPreset(&spPresetStore{opErr: errors.New("нет такой")}, "u5"))
}
