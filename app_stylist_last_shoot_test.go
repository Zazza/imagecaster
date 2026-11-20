package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lsStore struct {
	settings map[string]string
	items    map[int64]bool
	itemErr  error
	writes   int
}

func (s *lsStore) GetSetting(key string) (string, error) { return s.settings[key], nil }
func (s *lsStore) SetSetting(key, value string) error {
	s.settings[key] = value
	s.writes++
	return nil
}
func (s *lsStore) ExistingSessionItems(ids []int64) (map[int64]bool, error) {
	if s.itemErr != nil {
		return nil, s.itemErr
	}
	out := map[int64]bool{}
	for _, id := range ids {
		if s.items[id] {
			out[id] = true
		}
	}
	return out, nil
}

func newLsStore(items ...int64) *lsStore {
	s := &lsStore{settings: map[string]string{}, items: map[int64]bool{}}
	for _, id := range items {
		s.items[id] = true
	}
	return s
}

func TestLastShoot_RememberReplacesPrevious(t *testing.T) {
	st := newLsStore()
	require.NoError(t, rememberShoot(st, []int64{1, 2}))
	require.NoError(t, rememberShoot(st, []int64{7, 8, 9}))
	assert.Equal(t, []int64{7, 8, 9}, lastShootIDs(st))
}

func TestLastShoot_EmptyStore(t *testing.T) {
	assert.Empty(t, lastShootIDs(newLsStore()))
	st := newLsStore()
	st.settings[stylistLastShootKey] = "not json"
	assert.Empty(t, lastShootIDs(st))
}

func TestLastShoot_MergeAfterRestart(t *testing.T) {
	st := newLsStore(500, 501)
	require.NoError(t, rememberShoot(st, []int64{1, 2, 3}))

	got, err := mergeLastShoot(st, []StylistFrame{
		{JobID: 1, Status: "completed", ItemID: 500, Shot: "Анфас"},
		{JobID: 2, Status: "completed", ItemID: 501, Shot: "Профиль", Garment: "done"},
		{JobID: 3, Status: "running"},
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", got[0].Status)

	got, err = mergeLastShoot(st, []StylistFrame{
		{JobID: 1, Status: stylistFrameGone},
		{JobID: 2, Status: stylistFrameGone},
		{JobID: 3, Status: "pending"},
	})
	require.NoError(t, err)
	assert.Equal(t, StylistFrame{JobID: 1, Status: "completed", ItemID: 500, Shot: "Анфас"}, got[0], "queue cleared on start: frame comes from the store")
	assert.Equal(t, "done", got[1].Garment)
	assert.Equal(t, "pending", got[2].Status, "unfinished job stays from the queue")
}

func TestLastShoot_DeletedInHistory(t *testing.T) {
	st := newLsStore(500)
	require.NoError(t, rememberShoot(st, []int64{1, 2}))

	got, err := mergeLastShoot(st, []StylistFrame{
		{JobID: 1, Status: "completed", ItemID: 500},
		{JobID: 2, Status: "completed", ItemID: 501},
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", got[0].Status)
	assert.Equal(t, stylistFrameDeleted, got[1].Status)
	assert.Equal(t, []int64{1}, lastShootIDs(st), "deleted frame leaves the stored shoot")

	delete(st.items, 500)
	got, err = mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: stylistFrameGone}})
	require.NoError(t, err)
	assert.Equal(t, stylistFrameDeleted, got[0].Status, "deleted while the app was closed")
	assert.Empty(t, lastShootIDs(st))
}

func TestLastShoot_OtherIDsDoNotOverwrite(t *testing.T) {
	st := newLsStore(500)
	require.NoError(t, rememberShoot(st, []int64{1, 2}))
	writes := st.writes
	_, err := mergeLastShoot(st, []StylistFrame{{JobID: 9, Status: "completed", ItemID: 500}})
	require.NoError(t, err)
	assert.Equal(t, writes, st.writes)
	assert.Equal(t, []int64{1, 2}, lastShootIDs(st))
}

func TestLastShoot_NoWriteWithoutChange(t *testing.T) {
	st := newLsStore()
	require.NoError(t, rememberShoot(st, []int64{1}))
	writes := st.writes
	_, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: "pending"}})
	require.NoError(t, err)
	assert.Equal(t, writes, st.writes)
}

func TestLastShoot_ItemLookupError(t *testing.T) {
	st := newLsStore()
	st.itemErr = errors.New("db locked")
	got, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: "completed", ItemID: 500}})
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestLastShoot_FrameFinishedWhilePageClosed(t *testing.T) {
	st := newLsStore(500)
	require.NoError(t, rememberShoot(st, []int64{1, 2}))
	require.NoError(t, recordShootFrame(st, 1, []byte(`{"id":500,"shot":"Анфас","garment":"done"}`)))
	require.NoError(t, recordShootFrame(st, 9, []byte(`{"id":777}`)))
	require.NoError(t, recordShootFrame(st, 2, []byte(`not json`)))

	got, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: stylistFrameGone}, {JobID: 2, Status: "running"}})
	require.NoError(t, err)
	assert.Equal(t, StylistFrame{JobID: 1, Status: "completed", ItemID: 500, Shot: "Анфас", Garment: "done"}, got[0])
	assert.Equal(t, "running", got[1].Status)
	assert.Equal(t, []int64{1, 2}, lastShootIDs(st))
}

func TestLastShoot_StaleCallKeepsItem(t *testing.T) {
	st := newLsStore(500)
	require.NoError(t, rememberShoot(st, []int64{1}))
	require.NoError(t, recordShootFrame(st, 1, []byte(`{"id":500}`)))
	got, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: "running"}})
	require.NoError(t, err)
	assert.Equal(t, int64(500), got[0].ItemID)
	assert.Equal(t, "completed", got[0].Status)
}

func TestLastShoot_GoneWithoutPhotoIsDropped(t *testing.T) {
	st := newLsStore()
	require.NoError(t, rememberShoot(st, []int64{1, 2}))
	got, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: stylistFrameGone}, {JobID: 2, Status: "pending"}})
	require.NoError(t, err)
	assert.Equal(t, stylistFrameDeleted, got[0].Status)
	assert.Equal(t, []int64{2}, lastShootIDs(st))
}

func TestLastShoot_CancelledAndFailedSurviveRestart(t *testing.T) {
	st := newLsStore()
	require.NoError(t, rememberShoot(st, []int64{1, 2, 3}))
	_, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: "cancelled"}, {JobID: 2, Status: "failed", Error: "SD упал"}, {JobID: 3, Status: "running"}})
	require.NoError(t, err)
	got, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: stylistFrameGone}, {JobID: 2, Status: stylistFrameGone}, {JobID: 3, Status: stylistFrameGone}})
	require.NoError(t, err)
	assert.Equal(t, "cancelled", got[0].Status)
	assert.Equal(t, StylistFrame{JobID: 2, Status: "failed", Error: "SD упал"}, got[1])
	assert.Equal(t, stylistFrameDeleted, got[2].Status, "job vanished without result")
	assert.Equal(t, []int64{1, 2}, lastShootIDs(st))
}
