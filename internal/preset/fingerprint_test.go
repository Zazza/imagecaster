package preset

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionFingerprint(t *testing.T) {
	t.Parallel()
	db := testDB(t)

	maxID, count, err := db.SessionFingerprint()
	require.NoError(t, err)
	assert.Equal(t, int64(0), maxID, "fresh DB has no session items")
	sessions, err := db.ListSessions()
	require.NoError(t, err)
	assert.Equal(t, int64(len(sessions)), count)

	si, err := db.CreateSession("fp")
	require.NoError(t, err)
	maxID2, count2, err := db.SessionFingerprint()
	require.NoError(t, err)
	assert.Equal(t, count+1, count2, "creating a session increases sessionCount")
	assert.Equal(t, maxID, maxID2)

	itemID, err := db.AddSessionItem(&SessionItem{SessionID: si.ID, Source: "sd"})
	require.NoError(t, err)
	maxID3, count3, err := db.SessionFingerprint()
	require.NoError(t, err)
	assert.Equal(t, maxID2, maxID3, "item without thumb_name must not change maxItemID")
	assert.Equal(t, count2, count3)

	name := fmt.Sprintf("%d.png", itemID)
	require.NoError(t, db.UpdateSessionItemPaths(itemID, name, name))
	maxID4, count4, err := db.SessionFingerprint()
	require.NoError(t, err)
	assert.Equal(t, itemID, maxID4, "item with thumb_name is counted")
	assert.Equal(t, count2, count4)
}
