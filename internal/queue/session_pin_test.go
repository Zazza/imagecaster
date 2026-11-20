package queue

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setActive(t *testing.T, store *Store, name string) int64 {
	t.Helper()
	res, err := store.db.Exec(`INSERT INTO sessions (name) VALUES (?)`, name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = store.db.Exec(`UPDATE session_active SET session_id = ? WHERE id = 1`, id)
	require.NoError(t, err)
	return id
}

func TestStore_JobRemembersFolderOpenAtEnqueue(t *testing.T) {
	store := NewStore(openTestDB(t))
	a := setActive(t, store, "A")
	plain, err := store.Create(JobTxt2Img, `{}`, "test")
	require.NoError(t, err)
	labeled, err := store.CreateLabeled(JobTxt2Img, `{}`, "test", "x")
	require.NoError(t, err)
	setActive(t, store, "B")

	for _, id := range []int64{plain, labeled} {
		job, err := store.Get(id)
		require.NoError(t, err)
		assert.Equal(t, a, job.SessionID)
	}
	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, a, claimed.SessionID)
}

func TestStore_JobWithoutActiveFolder(t *testing.T) {
	store := NewStore(openTestDB(t))
	_, err := store.db.Exec(`DELETE FROM session_active`)
	require.NoError(t, err)
	id, err := store.Create(JobTxt2Img, `{}`, "test")
	require.NoError(t, err)
	job, err := store.Get(id)
	require.NoError(t, err)
	assert.Zero(t, job.SessionID)
}

type pinSessions struct {
	xrSessions
	mu     sync.Mutex
	pinned []int64
}

type pinnedAdder struct {
	owner *pinSessions
	id    int64
}

func (s *pinSessions) PinnedTo(id int64) generation.SessionAdder {
	return pinnedAdder{owner: s, id: id}
}

func (p pinnedAdder) AddToSession(img string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64 {
	p.owner.mu.Lock()
	p.owner.pinned = append(p.owner.pinned, p.id)
	p.owner.mu.Unlock()
	return 100
}

func TestProcessJob_ResultGoesToJobFolder(t *testing.T) {
	env := newXREnv(t)
	a := setActive(t, env.store, "A")
	pinnedJob := env.ideaJob(t)
	_, err := env.store.db.Exec(`DELETE FROM session_active`)
	require.NoError(t, err)
	oldJob := env.ideaJob(t)

	sessions := &pinSessions{}
	proc, _ := env.processor(t, &xrLLM{variant: "A"}, &xrSD{}, sessions)
	for _, id := range []int64{pinnedJob, oldJob} {
		job, err := env.store.Get(id)
		require.NoError(t, err)
		_, err = proc.ProcessJob(context.Background(), job)
		require.NoError(t, err)
	}

	assert.Equal(t, []int64{a}, sessions.pinned, "job with a folder saves into it")
	assert.Equal(t, 1, sessions.count(), "job without a folder saves the old way, into the active one")
}
