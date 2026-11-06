package mcp

import (
	"fmt"
	"image/color"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

type fakeFolderSessions struct {
	*fakeItemSessions
	fmu       sync.Mutex
	folders   []preset.SessionInfo
	order     map[int64][]int64
	failImage map[int64]bool
	nextItem  int64
}

func newFakeFolderSessions(names ...string) *fakeFolderSessions {
	f := &fakeFolderSessions{fakeItemSessions: newFakeItemSessions(), order: map[int64][]int64{}, failImage: map[int64]bool{}, nextItem: 1000}
	for i, n := range names {
		f.folders = append(f.folders, preset.SessionInfo{ID: int64(i + 1), Name: n, IsActive: i == 0})
	}
	return f
}

func (f *fakeFolderSessions) folderID(t *testing.T, name string) int64 {
	t.Helper()
	for _, si := range f.folders {
		if si.Name == name {
			return si.ID
		}
	}
	require.FailNow(t, "no folder "+name)
	return 0
}

func (f *fakeFolderSessions) add(t *testing.T, folder, source string, isPreview bool, shade uint8) int64 {
	t.Helper()
	sid := f.folderID(t, folder)
	f.fmu.Lock()
	f.nextItem++
	id := f.nextItem
	f.order[sid] = append(f.order[sid], id)
	f.fmu.Unlock()
	f.put(&preset.SessionItem{
		ID:        id,
		SessionID: sid,
		Source:    source,
		IsPreview: isPreview,
		FileName:  fmt.Sprintf("sessions/%d/item_%d.png", sid, id),
		Width:     200,
		Height:    300,
		CreatedAt: fmt.Sprintf("2026-11-0%d 10:00:%02d", 1+int(id%5), id%60),
	}, styPhotoPNG(200, 300, color.NRGBA{R: shade, G: shade, B: shade, A: 255}))
	return id
}

func (f *fakeFolderSessions) ListSessions() ([]preset.SessionInfo, error) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	return append([]preset.SessionInfo(nil), f.folders...), nil
}

func (f *fakeFolderSessions) SessionItems(sessionID int64) ([]preset.SessionItem, error) {
	f.fmu.Lock()
	ids := append([]int64(nil), f.order[sessionID]...)
	f.fmu.Unlock()
	out := make([]preset.SessionItem, 0, len(ids))
	for _, id := range ids {
		it, err := f.SessionItem(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, nil
}

func (f *fakeFolderSessions) GetSessionImage(id int64) (string, error) {
	f.fmu.Lock()
	fail := f.failImage[id]
	f.fmu.Unlock()
	if fail {
		return "", fmt.Errorf("open /home/user/.imagecaster/sessions/1/item_%d.png: no such file or directory", id)
	}
	return f.fakeItemSessions.GetSessionImage(id)
}

func styPhotos(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["photos"].([]any)
	require.True(t, ok, "photos must be an array: %v", m)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		obj, ok := v.(map[string]any)
		require.True(t, ok, "photo must be an object: %v", v)
		out = append(out, obj)
	}
	return out
}

func styPhotoItems(t *testing.T, photos []map[string]any) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(photos))
	for _, p := range photos {
		ids = append(ids, styNum(t, p, "item"))
	}
	return ids
}

func styDescribedBytes(t *testing.T, eng *fakeStylist) [][]byte {
	t.Helper()
	var out [][]byte
	for _, c := range eng.describeCalls() {
		out = append(out, styDecodeB64(t, c.image))
	}
	return out
}

func TestStylistDescribe_SessionExactNameCaseInsensitive(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := newFakeFolderSessions("Default", "Жена лето", "Жена зима")
	sess.add(t, "Default", "sd", false, 10)
	a := sess.add(t, "Жена лето", "sd", false, 60)
	b := sess.add(t, "Жена лето", "upload", false, 90)
	sess.add(t, "Жена зима", "sd", false, 150)
	s, _ := styNewServer(t, eng, sess)

	m, out := styCall(t, s, "stylist_describe", map[string]any{"session": "жена лето"})

	assert.Equal(t, "Жена лето", m["session"], out)
	assert.Equal(t, sess.folderID(t, "Жена лето"), styNum(t, m, "session_id"))
	photos := styPhotos(t, m)
	assert.ElementsMatch(t, []int64{a, b}, styPhotoItems(t, photos))
	for _, p := range photos {
		d, ok := p["photo"].(string)
		assert.True(t, ok && d != "", "photo must be described: %v", p)
		assert.Contains(t, p, "created_at")
		assert.Contains(t, p, "description")
	}
	assert.NotEmpty(t, m["ask_user"], out)
	assert.NotEmpty(t, m["note"], out)
	want := [][]byte{
		styPhotoPNG(200, 300, color.NRGBA{R: 60, G: 60, B: 60, A: 255}),
		styPhotoPNG(200, 300, color.NRGBA{R: 90, G: 90, B: 90, A: 255}),
	}
	assert.ElementsMatch(t, want, styDescribedBytes(t, eng))
}

func TestStylistDescribe_SessionUniquePartOfName(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := newFakeFolderSessions("Default", "Отпуск 2025")
	sess.add(t, "Default", "sd", false, 10)
	id := sess.add(t, "Отпуск 2025", "sd", false, 70)
	s, _ := styNewServer(t, eng, sess)

	m, out := styCall(t, s, "stylist_describe", map[string]any{"session": "отпуск"})

	assert.Equal(t, "Отпуск 2025", m["session"], out)
	assert.Equal(t, sess.folderID(t, "Отпуск 2025"), styNum(t, m, "session_id"))
	assert.Equal(t, []int64{id}, styPhotoItems(t, styPhotos(t, m)))
}

func TestStylistDescribe_SessionNotResolved(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		folders []string
		query   string
		want    []string
	}{
		{name: "ambiguous part", folders: []string{"Default", "Жена лето", "Жена зима"}, query: "жена", want: []string{"Жена лето", "Жена зима"}},
		{name: "no match", folders: []string{"Default", "Отпуск 2025"}, query: "дача", want: []string{"Default", "Отпуск 2025"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess := newFakeFolderSessions(tt.folders...)
			for _, f := range tt.folders {
				sess.add(t, f, "sd", false, 80)
			}
			s, _ := styNewServer(t, eng, sess)

			_, err := styExec(s, "stylist_describe", map[string]any{"session": tt.query})

			require.Error(t, err)
			for _, w := range tt.want {
				assert.Contains(t, err.Error(), w)
			}
			assert.Empty(t, eng.describeCalls())
		})
	}
}

func TestStylistDescribe_SessionSkipsVariantsAndPreviews(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := newFakeFolderSessions("Default", "Стиль")
	a := sess.add(t, "Стиль", "sd", false, 50)
	sess.add(t, "Стиль", "stylist", false, 51)
	sess.add(t, "Стиль", "sd", true, 52)
	b := sess.add(t, "Стиль", "upload", false, 53)
	sess.add(t, "Стиль", "stylist", false, 54)
	sess.add(t, "Стиль", "stylist", false, 55)
	s, _ := styNewServer(t, eng, sess)

	m, out := styCall(t, s, "stylist_describe", map[string]any{"session": "Стиль"})

	assert.ElementsMatch(t, []int64{a, b}, styPhotoItems(t, styPhotos(t, m)), out)
	assert.Equal(t, int64(3), styNum(t, m, "skipped_variants"))
	want := [][]byte{
		styPhotoPNG(200, 300, color.NRGBA{R: 50, G: 50, B: 50, A: 255}),
		styPhotoPNG(200, 300, color.NRGBA{R: 53, G: 53, B: 53, A: 255}),
	}
	assert.ElementsMatch(t, want, styDescribedBytes(t, eng))
}

func TestStylistDescribe_SessionLimitsToSixNewest(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := newFakeFolderSessions("Default", "Много")
	var ids []int64
	for i := 0; i < 8; i++ {
		ids = append(ids, sess.add(t, "Много", "sd", false, uint8(20+i*10)))
	}
	s, _ := styNewServer(t, eng, sess)

	m, out := styCall(t, s, "stylist_describe", map[string]any{"session": "Много"})

	assert.ElementsMatch(t, ids[2:], styPhotoItems(t, styPhotos(t, m)), out)
	assert.Equal(t, int64(2), styNum(t, m, "older_not_described"))
	for _, id := range ids[:2] {
		assert.NotContains(t, sess.imageReads(), id, "older photo must not be read")
	}
}

func TestStylistDescribe_SessionOnePhotoUnreadable(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := newFakeFolderSessions("Default", "Фото")
	a := sess.add(t, "Фото", "sd", false, 40)
	bad := sess.add(t, "Фото", "sd", false, 41)
	c := sess.add(t, "Фото", "sd", false, 42)
	sess.failImage[bad] = true
	s, _ := styNewServer(t, eng, sess)

	m, out := styCall(t, s, "stylist_describe", map[string]any{"session": "Фото"})

	photos := styPhotos(t, m)
	assert.ElementsMatch(t, []int64{a, bad, c}, styPhotoItems(t, photos), out)
	for _, p := range photos {
		id := styNum(t, p, "item")
		if id == bad {
			msg, ok := p["error"].(string)
			require.True(t, ok, "unreadable photo must have error: %v", p)
			assert.NotEmpty(t, msg)
			assert.NotContains(t, msg, "/")
			assert.NotContains(t, msg, ".png")
			continue
		}
		d, ok := p["photo"].(string)
		assert.True(t, ok && d != "", "photo %d must be described: %v", id, p)
		assert.Nil(t, p["error"])
	}
	assert.NotContains(t, out, "item_")
	assert.NotContains(t, out, "/home/")
}

func TestStylistDescribe_SessionWithOtherSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "with item", args: map[string]any{"session": "Фото", "item": "active"}},
		{name: "with numeric item", args: map[string]any{"session": "Фото", "item": float64(styItemID)}},
		{name: "with photo_path", args: map[string]any{"session": "Фото", "photo_path": "/tmp/whatever.png"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess := newFakeFolderSessions("Default", "Фото")
			sess.add(t, "Фото", "sd", false, 40)
			sess.put(&preset.SessionItem{ID: styItemID, SessionID: 1, Source: "sd", Width: 200, Height: 300}, styPhotoPNG(200, 300, color.NRGBA{R: 1, G: 1, B: 1, A: 255}))
			sess.active = styItemID
			s, _ := styNewServer(t, eng, sess)

			_, err := styExec(s, "stylist_describe", tt.args)

			require.Error(t, err)
			assert.Empty(t, eng.describeCalls())
		})
	}
}

func TestStylistDescribe_SessionWithoutFolderSupport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sess AppSessions
	}{
		{name: "named only", sess: &fakeSessions{}},
		{name: "items without folders", sess: newFakeItemSessions()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			s, _ := styNewServer(t, eng, tt.sess)

			var err error
			require.NotPanics(t, func() {
				_, err = styExec(s, "stylist_describe", map[string]any{"session": "Default"})
			})

			require.Error(t, err)
			assert.Empty(t, eng.describeCalls())
		})
	}
}

func TestStylistDescribe_SessionOnlyVariants(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := newFakeFolderSessions("Default", "Варианты")
	sess.add(t, "Варианты", "stylist", false, 30)
	sess.add(t, "Варианты", "stylist", false, 31)
	s, _ := styNewServer(t, eng, sess)

	_, err := styExec(s, "stylist_describe", map[string]any{"session": "Варианты"})

	require.Error(t, err)
	assert.Empty(t, eng.describeCalls())
}

func TestStylistDescribe_SessionEmptyFolder(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := newFakeFolderSessions("Default", "Пустая")
	s, _ := styNewServer(t, eng, sess)

	var err error
	require.NotPanics(t, func() {
		_, err = styExec(s, "stylist_describe", map[string]any{"session": "Пустая"})
	})

	require.Error(t, err)
	assert.Empty(t, eng.describeCalls())
}
