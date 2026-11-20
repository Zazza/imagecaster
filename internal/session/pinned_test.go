package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinnedTo_SavesToPinnedSessionNotActive(t *testing.T) {
	svc, emitter, _, db := serviceWithDB(t)
	a, err := svc.CreateSession("A")
	require.NoError(t, err)
	b, err := svc.CreateSession("B")
	require.NoError(t, err)

	id := svc.PinnedTo(a.ID).AddToSession(pngBase64(t, 8, 8), nil, "generate", nil)
	require.NotZero(t, id)

	active, err := db.GetActiveSessionID()
	require.NoError(t, err)
	assert.Equal(t, b.ID, active, "saving must not switch the open folder")
	inA, err := db.GetSessionItems(a.ID)
	require.NoError(t, err)
	require.Len(t, inA, 1)
	assert.Equal(t, id, inA[0].ID)
	inB, err := db.GetSessionItems(b.ID)
	require.NoError(t, err)
	assert.Empty(t, inB)

	var payload map[string]int64
	for _, e := range emitter.events {
		if e.name == "session:added" {
			payload = e.data[0].(map[string]int64)
		}
	}
	assert.Equal(t, map[string]int64{"id": id, "session_id": a.ID}, payload)
}

func TestPinnedTo_DeletedSessionFallsBackToActive(t *testing.T) {
	svc, _, _, db := serviceWithDB(t)
	a, err := svc.CreateSession("A")
	require.NoError(t, err)
	b, err := svc.CreateSession("B")
	require.NoError(t, err)
	require.NoError(t, svc.DeleteSession(a.ID))

	id := svc.PinnedTo(a.ID).AddToSession(pngBase64(t, 8, 8), nil, "generate", nil)
	require.NotZero(t, id, "the result must not be lost")
	inB, err := db.GetSessionItems(b.ID)
	require.NoError(t, err)
	require.Len(t, inB, 1)
	assert.Equal(t, id, inB[0].ID)
}
