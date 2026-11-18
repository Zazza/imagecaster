package session

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveSessionItems_ThumbConflictRollsFileBack(t *testing.T) {
	t.Parallel()
	svc, emitter, dataDir, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	id := mustAdd(t, svc, src, nil)
	item := mustItem(t, svc, id)
	img := readFile(t, moveImagePath(dataDir, src, item.FileName))
	require.NoError(t, os.MkdirAll(filepath.Dir(moveThumbPath(dataDir, dst, item.ThumbName)), 0o755))
	require.NoError(t, os.WriteFile(moveThumbPath(dataDir, dst, item.ThumbName), []byte("other"), 0o644))

	n, err := svc.MoveSessionItems([]int64{id}, dst)

	require.Error(t, err)
	assert.Zero(t, n)
	assert.Equal(t, src, mustItem(t, svc, id).SessionID)
	assert.Equal(t, img, readFile(t, moveImagePath(dataDir, src, item.FileName)), "file is back in place")
	assertMissing(t, moveImagePath(dataDir, dst, item.FileName))
	assert.Equal(t, []byte("other"), readFile(t, moveThumbPath(dataDir, dst, item.ThumbName)), "existing file is not overwritten")
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	for _, e := range emitter.events {
		assert.NotEqual(t, "session:removed", e.name)
	}
}

func TestMoveSessionItems_PartialMoveReportsCount(t *testing.T) {
	t.Parallel()
	svc, emitter, dataDir, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	dst := mustSession(t, svc, "Куда")
	first := mustAdd(t, svc, src, nil)
	second := mustAdd(t, svc, src, nil)
	blocked := mustItem(t, svc, second)
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "sessions", strconv.FormatInt(dst, 10)), 0o755))
	require.NoError(t, os.WriteFile(moveImagePath(dataDir, dst, blocked.FileName), []byte("other"), 0o644))

	n, err := svc.MoveSessionItems([]int64{first, second}, dst)

	require.Error(t, err)
	assert.Equal(t, 1, n)
	assert.Contains(t, err.Error(), "1 из 2")
	assert.Equal(t, dst, mustItem(t, svc, first).SessionID)
	assert.Equal(t, src, mustItem(t, svc, second).SessionID)
	payload := lastRemovedPayload(t, emitter)
	assert.Equal(t, []int64{first}, toInt64s(t, payload["ids"]))
}

func TestMoveSessionItems_ConcurrentMovesOfOnePhotoStayConsistent(t *testing.T) {
	t.Parallel()
	svc, _, dataDir, _ := serviceWithDB(t)
	src := mustSession(t, svc, "Откуда")
	a := mustSession(t, svc, "А")
	b := mustSession(t, svc, "Б")
	id := mustAdd(t, svc, src, nil)
	name := mustItem(t, svc, id).FileName

	var wg sync.WaitGroup
	for _, dst := range []int64{a, b, a, b} {
		wg.Add(1)
		go func(dst int64) {
			defer wg.Done()
			_, _ = svc.MoveSessionItems([]int64{id}, dst)
		}(dst)
	}
	wg.Wait()

	got := mustItem(t, svc, id).SessionID
	readFile(t, moveImagePath(dataDir, got, name))
	for _, other := range []int64{src, a, b} {
		if other != got {
			assertMissing(t, moveImagePath(dataDir, other, name))
		}
	}
}
