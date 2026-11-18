package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moveImagePath(dataDir string, sessionID int64, name string) string {
	return filepath.Join(dataDir, "sessions", strconv.FormatInt(sessionID, 10), name)
}

func moveThumbPath(dataDir string, sessionID int64, name string) string {
	return filepath.Join(dataDir, "thumbs", strconv.FormatInt(sessionID, 10), name)
}

func mustItem(t *testing.T, svc *Service, id int64) *preset.SessionItem {
	t.Helper()
	item, err := svc.SessionItem(id)
	require.NoError(t, err)
	require.NotNil(t, item, "item %d", id)
	return item
}

func mustSession(t *testing.T, svc *Service, name string) int64 {
	t.Helper()
	si, err := svc.CreateSession(name)
	require.NoError(t, err)
	return si.ID
}

func mustAdd(t *testing.T, svc *Service, sessionID int64, meta *preset.GenerationMeta) int64 {
	t.Helper()
	id := svc.AddToSessionID(sessionID, pngBase64(t, 48, 32), nil, "generate", meta)
	require.Greater(t, id, int64(0))
	return id
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, path)
	return data
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "%s must not exist", path)
}

func sessionItemIDs(t *testing.T, svc *Service, sessionID int64) []int64 {
	t.Helper()
	items, err := svc.SessionItems(sessionID)
	require.NoError(t, err)
	var ids []int64
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids
}

func toInt64s(t *testing.T, v any) []int64 {
	t.Helper()
	switch x := v.(type) {
	case []int64:
		return x
	case []any:
		out := make([]int64, 0, len(x))
		for _, e := range x {
			n, err := strconv.ParseInt(fmt.Sprint(e), 10, 64)
			require.NoError(t, err)
			out = append(out, n)
		}
		return out
	}
	t.Fatalf("ids has unexpected type %T", v)
	return nil
}

func lastRemovedPayload(t *testing.T, m *mockEmitter) map[string]any {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.events) - 1; i >= 0; i-- {
		e := m.events[i]
		if e.name != "session:removed" {
			continue
		}
		require.NotEmpty(t, e.data)
		payload, ok := e.data[0].(map[string]any)
		require.True(t, ok, "session:removed payload must be map[string]any, got %T", e.data[0])
		return payload
	}
	t.Fatal("session:removed not emitted")
	return nil
}

func TestMoveSessionItems_KeepsIDDataAndMovesFiles(t *testing.T) {
	t.Parallel()
	svc, _, dataDir, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	mustAdd(t, svc, dst, nil)

	presetID := int64(7)
	meta := &preset.GenerationMeta{
		PresetID:   &presetID,
		Model:      "model.safetensors",
		Label:      "Подпись кадра",
		Chain:      []preset.StepMeta{{Step: 1, Preset: "Стиль", Op: "generate"}},
		SourceItem: 3,
	}
	id := mustAdd(t, svc, src, meta)
	before := mustItem(t, svc, id)
	require.NotEmpty(t, before.FileName)
	require.NotEmpty(t, before.ThumbName)
	imgBytes := readFile(t, moveImagePath(dataDir, src, before.FileName))
	thumbBytes := readFile(t, moveThumbPath(dataDir, src, before.ThumbName))

	n, err := svc.MoveSessionItems([]int64{id}, dst)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	after := mustItem(t, svc, id)
	assert.Equal(t, id, after.ID)
	assert.Equal(t, dst, after.SessionID)
	want := *before
	want.SessionID = dst
	want.IsActive = after.IsActive
	assert.Equal(t, want, *after, "generation data, caption and meta must survive the move")

	assert.Equal(t, imgBytes, readFile(t, moveImagePath(dataDir, dst, after.FileName)))
	assert.Equal(t, thumbBytes, readFile(t, moveThumbPath(dataDir, dst, after.ThumbName)))
	assertMissing(t, moveImagePath(dataDir, src, before.FileName))
	assertMissing(t, moveThumbPath(dataDir, src, before.ThumbName))

	img, err := svc.GetSessionImage(id)
	require.NoError(t, err)
	assert.NotEmpty(t, img)
	thumb, err := svc.GetSessionThumb(id)
	require.NoError(t, err)
	assert.NotEmpty(t, thumb)
}

func TestMoveSessionItems_GoneFromSourcePresentInTarget(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	a := mustAdd(t, svc, src, nil)
	keep := mustAdd(t, svc, src, nil)
	b := mustAdd(t, svc, src, nil)
	existing := mustAdd(t, svc, dst, nil)

	n, err := svc.MoveSessionItems([]int64{a, b}, dst)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	assert.ElementsMatch(t, []int64{keep}, sessionItemIDs(t, svc, src))
	assert.ElementsMatch(t, []int64{existing, a, b}, sessionItemIDs(t, svc, dst))
}

func TestMoveSessionItems_EmitsRemovedWithIDsAndTarget(t *testing.T) {
	t.Parallel()
	svc, emitter, _, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	a := mustAdd(t, svc, src, nil)
	b := mustAdd(t, svc, src, nil)
	before := emitter.eventCount("session:removed")

	_, err := svc.MoveSessionItems([]int64{a, b}, dst)
	require.NoError(t, err)

	assert.Equal(t, before+1, emitter.eventCount("session:removed"))
	payload := lastRemovedPayload(t, emitter)
	require.Contains(t, payload, "ids")
	require.Contains(t, payload, "moved_to")
	assert.Subset(t, toInt64s(t, payload["ids"]), []int64{a, b})
	assert.Equal(t, fmt.Sprint(dst), fmt.Sprint(payload["moved_to"]))
}

func TestMoveSessionItems_ActiveFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		moveActive bool
	}{
		{name: "moving the active photo leaves source without active", moveActive: true},
		{name: "moving an inactive photo keeps source active", moveActive: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _, db := serviceWithDB(t)
			src := mustSession(t, svc, "Откуда")
			dst := mustSession(t, svc, "Куда")
			dstActive := mustAdd(t, svc, dst, nil)
			first := mustAdd(t, svc, src, nil)
			last := mustAdd(t, svc, src, nil)
			require.True(t, mustItem(t, svc, last).IsActive)
			require.True(t, mustItem(t, svc, dstActive).IsActive)

			moved := first
			if tt.moveActive {
				moved = last
			}
			n, err := svc.MoveSessionItems([]int64{moved}, dst)
			require.NoError(t, err)
			require.Equal(t, 1, n)

			assert.False(t, mustItem(t, svc, moved).IsActive, "moved photo must not become active in target")
			active, err := db.GetActiveItem(dst)
			require.NoError(t, err)
			require.NotNil(t, active)
			assert.Equal(t, dstActive, active.ID, "target keeps its own active photo")

			srcActive, err := db.GetActiveItem(src)
			require.NoError(t, err)
			if tt.moveActive {
				assert.Nil(t, srcActive)
			} else {
				require.NotNil(t, srcActive)
				assert.Equal(t, last, srcActive.ID)
			}
		})
	}
}

func TestMoveSessionItems_TargetWithoutActiveStaysWithoutActive(t *testing.T) {
	t.Parallel()
	svc, _, _, db := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	id := mustAdd(t, svc, src, nil)

	_, err := svc.MoveSessionItems([]int64{id}, dst)
	require.NoError(t, err)

	active, err := db.GetActiveItem(dst)
	require.NoError(t, err)
	assert.Nil(t, active)
}

func TestMoveSessionItems_FileMoveFailureKeepsPhoto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		blocked string
	}{
		{name: "image directory cannot be created", blocked: "sessions"},
		{name: "thumbnail directory cannot be created", blocked: "thumbs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, emitter, dataDir, _ := serviceWithDB(t)
			src := mustSession(t, svc, "Откуда")
			dst := mustSession(t, svc, "Куда")
			id := mustAdd(t, svc, src, nil)
			before := mustItem(t, svc, id)
			imgBytes := readFile(t, moveImagePath(dataDir, src, before.FileName))
			thumbBytes := readFile(t, moveThumbPath(dataDir, src, before.ThumbName))

			blocker := filepath.Join(dataDir, tt.blocked, strconv.FormatInt(dst, 10))
			require.NoError(t, os.MkdirAll(filepath.Dir(blocker), 0o755))
			require.NoError(t, os.WriteFile(blocker, []byte("not a dir"), 0o644))
			removedBefore := emitter.eventCount("session:removed")

			n, err := svc.MoveSessionItems([]int64{id}, dst)
			require.Error(t, err)
			assert.Equal(t, 0, n)

			after := mustItem(t, svc, id)
			assert.Equal(t, *before, *after, "row must stay in the source folder unchanged")
			assert.Equal(t, imgBytes, readFile(t, moveImagePath(dataDir, src, before.FileName)))
			assert.Equal(t, thumbBytes, readFile(t, moveThumbPath(dataDir, src, before.ThumbName)))
			_, err = svc.GetSessionImage(id)
			assert.NoError(t, err)
			_, err = svc.GetSessionThumb(id)
			assert.NoError(t, err)
			assert.Equal(t, removedBefore, emitter.eventCount("session:removed"))
		})
	}
}

func TestMoveSessionItems_RejectedChangesNothing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		ids    func(a, b int64) []int64
		target func(src, dst int64) int64
	}{
		{name: "same folder", ids: func(a, b int64) []int64 { return []int64{a, b} }, target: func(src, _ int64) int64 { return src }},
		{name: "missing folder", ids: func(a, b int64) []int64 { return []int64{a, b} }, target: func(_, _ int64) int64 { return 987654 }},
		{name: "zero folder id", ids: func(a, b int64) []int64 { return []int64{a, b} }, target: func(_, _ int64) int64 { return 0 }},
		{name: "nil selection", ids: func(_, _ int64) []int64 { return nil }, target: func(_, dst int64) int64 { return dst }},
		{name: "empty selection", ids: func(_, _ int64) []int64 { return []int64{} }, target: func(_, dst int64) int64 { return dst }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, emitter, dataDir, _ := serviceWithDB(t)
			src := mustSession(t, svc, "Откуда")
			dst := mustSession(t, svc, "Куда")
			a := mustAdd(t, svc, src, nil)
			b := mustAdd(t, svc, src, nil)
			itemsBefore := map[int64]preset.SessionItem{a: *mustItem(t, svc, a), b: *mustItem(t, svc, b)}
			removedBefore := emitter.eventCount("session:removed")

			n, err := svc.MoveSessionItems(tt.ids(a, b), tt.target(src, dst))
			require.Error(t, err)
			assert.Equal(t, 0, n)

			for id, was := range itemsBefore {
				now := mustItem(t, svc, id)
				assert.Equal(t, was, *now)
				_, statErr := os.Stat(moveImagePath(dataDir, src, was.FileName))
				assert.NoError(t, statErr)
				_, statErr = os.Stat(moveThumbPath(dataDir, src, was.ThumbName))
				assert.NoError(t, statErr)
			}
			assert.Empty(t, sessionItemIDs(t, svc, dst))
			assert.Equal(t, removedBefore, emitter.eventCount("session:removed"))
		})
	}
}

func TestMoveSessionItems_SourceItemReferenceStillResolves(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	origin := mustAdd(t, svc, src, nil)
	derived := mustAdd(t, svc, src, &preset.GenerationMeta{Label: "ретушь", SourceItem: origin})

	_, err := svc.MoveSessionItems([]int64{origin}, dst)
	require.NoError(t, err)

	ref := mustItem(t, svc, derived).Meta
	require.NotNil(t, ref)
	require.Equal(t, origin, ref.SourceItem)
	target := mustItem(t, svc, ref.SourceItem)
	assert.Equal(t, dst, target.SessionID)
	img, err := svc.GetSessionImage(ref.SourceItem)
	require.NoError(t, err)
	assert.NotEmpty(t, img)
}

func TestMoveSessionItems_ItemWithoutFileMovesRowOnly(t *testing.T) {
	t.Parallel()
	svc, _, dataDir, db := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	id, err := db.AddSessionItem(&preset.SessionItem{SessionID: src, Source: "generate", Prompt: "без файла"})
	require.NoError(t, err)

	n, err := svc.MoveSessionItems([]int64{id}, dst)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	item := mustItem(t, svc, id)
	assert.Equal(t, dst, item.SessionID)
	assert.Equal(t, "без файла", item.Prompt)
	assert.Empty(t, item.FileName)
	assert.Empty(t, item.ThumbName)
	assert.Empty(t, dirFiles(t, filepath.Join(dataDir, "sessions", strconv.FormatInt(dst, 10))))
}

func TestMoveSessionItems_PartlyDeletedSelection(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	a := mustAdd(t, svc, src, nil)
	gone := mustAdd(t, svc, src, nil)
	b := mustAdd(t, svc, src, nil)
	require.NoError(t, svc.DeleteSessionItem(gone))

	n, err := svc.MoveSessionItems([]int64{a, gone, b, 99999}, dst)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.ElementsMatch(t, []int64{a, b}, sessionItemIDs(t, svc, dst))
	assert.Empty(t, sessionItemIDs(t, svc, src))
	gotGone, err := svc.SessionItem(gone)
	require.NoError(t, err)
	assert.Nil(t, gotGone)
}

func TestMoveSessionItems_CreatesTargetDirectories(t *testing.T) {
	t.Parallel()
	svc, _, dataDir, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Новая")
	id := mustAdd(t, svc, src, nil)
	assertMissing(t, filepath.Join(dataDir, "sessions", strconv.FormatInt(dst, 10)))
	assertMissing(t, filepath.Join(dataDir, "thumbs", strconv.FormatInt(dst, 10)))

	n, err := svc.MoveSessionItems([]int64{id}, dst)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	item := mustItem(t, svc, id)
	_, err = os.Stat(moveImagePath(dataDir, dst, item.FileName))
	assert.NoError(t, err)
	_, err = os.Stat(moveThumbPath(dataDir, dst, item.ThumbName))
	assert.NoError(t, err)
}
