package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExistingSessionItems(t *testing.T) {
	db := testDB(t)
	s, err := db.CreateSession("a")
	require.NoError(t, err)
	a, err := db.AddSessionItem(&SessionItem{SessionID: s.ID, FileName: "a.png", Source: "stylist"})
	require.NoError(t, err)
	b, err := db.AddSessionItem(&SessionItem{SessionID: s.ID, FileName: "b.png", Source: "stylist"})
	require.NoError(t, err)

	got, err := db.ExistingSessionItems([]int64{a, b, 9999})
	require.NoError(t, err)
	assert.Equal(t, map[int64]bool{a: true, b: true}, got)

	got, err = db.ExistingSessionItems(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
