package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func itemFiles(t *testing.T, svc *Service, dataDir string, id int64) []string {
	t.Helper()
	item, err := svc.SessionItem(id)
	require.NoError(t, err)
	require.NotNil(t, item)
	dir := filepath.Join(dataDir, "sessions", fmt.Sprintf("%d", item.SessionID))
	var out []string
	for _, n := range []string{item.FileName, item.ThumbName} {
		if n != "" {
			out = append(out, filepath.Join(dir, n))
		}
	}
	require.NotEmpty(t, out)
	return out
}

func TestDeleteSessionItems_RemovesRowsFilesOneEvent(t *testing.T) {
	t.Parallel()
	svc, emitter, dataDir := testService(t)
	other, err := svc.CreateSession("Другая")
	require.NoError(t, err)

	a := svc.AddToSession(makeImageJPEG(t, 64, 64), nil, "generate", false, nil)
	b := svc.AddToSessionID(other.ID, makeImageJPEG(t, 64, 64), nil, "edit", false, nil)
	keep := svc.AddToSession(makeImageJPEG(t, 64, 64), nil, "generate", false, nil)
	require.Greater(t, a, int64(0))
	require.Greater(t, b, int64(0))
	require.Greater(t, keep, int64(0))

	var files []string
	files = append(files, itemFiles(t, svc, dataDir, a)...)
	files = append(files, itemFiles(t, svc, dataDir, b)...)
	keepFiles := itemFiles(t, svc, dataDir, keep)
	for _, f := range append(files, keepFiles...) {
		_, err := os.Stat(f)
		require.NoError(t, err, f)
	}
	before := emitter.eventCount("session:removed")

	n, err := svc.DeleteSessionItems([]int64{a, b})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, before+1, emitter.eventCount("session:removed"))

	for _, id := range []int64{a, b} {
		item, _ := svc.SessionItem(id)
		assert.Nil(t, item, id)
	}
	for _, f := range files {
		_, err := os.Stat(f)
		assert.True(t, os.IsNotExist(err), f)
	}
	item, err := svc.SessionItem(keep)
	require.NoError(t, err)
	require.NotNil(t, item)
	for _, f := range keepFiles {
		_, err := os.Stat(f)
		assert.NoError(t, err, f)
	}
}

func TestDeleteSessionItems_EdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("empty list", func(t *testing.T) {
		t.Parallel()
		svc, emitter, _ := testService(t)
		keep := svc.AddToSession(makeImageJPEG(t, 32, 32), nil, "generate", false, nil)
		n, err := svc.DeleteSessionItems(nil)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, 0, emitter.eventCount("session:removed"))
		item, _ := svc.SessionItem(keep)
		assert.NotNil(t, item)
	})
	t.Run("unknown ids are skipped", func(t *testing.T) {
		t.Parallel()
		svc, _, _ := testService(t)
		a := svc.AddToSession(makeImageJPEG(t, 32, 32), nil, "generate", false, nil)
		n, err := svc.DeleteSessionItems([]int64{a, 99999})
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		item, _ := svc.SessionItem(a)
		assert.Nil(t, item)
	})
	t.Run("duplicate ids counted once", func(t *testing.T) {
		t.Parallel()
		svc, emitter, _ := testService(t)
		a := svc.AddToSession(makeImageJPEG(t, 32, 32), nil, "generate", false, nil)
		n, err := svc.DeleteSessionItems([]int64{a, a})
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, 1, emitter.eventCount("session:removed"))
	})
}

func TestOpenInHistory_SwitchesSessionAndActivatesItem(t *testing.T) {
	t.Parallel()
	svc, emitter, _ := testService(t)
	original := activeSession(t, svc)
	other, err := svc.CreateSession("Другая")
	require.NoError(t, err)
	require.NoError(t, svc.SwitchSession(original.ID))

	target := svc.AddToSessionID(other.ID, makeImageJPEG(t, 32, 32), nil, "generate", false, nil)
	newer := svc.AddToSessionID(other.ID, makeImageJPEG(t, 32, 32), nil, "generate", false, nil)
	require.Greater(t, target, int64(0))
	require.Greater(t, newer, int64(0))
	require.Equal(t, original.ID, activeSession(t, svc).ID)

	require.NoError(t, svc.OpenInHistory(target))

	assert.Equal(t, other.ID, activeSession(t, svc).ID)
	active, err := svc.GetActiveSessionItem()
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, target, active.ID)
	assert.True(t, emitter.hasEvent("session:switched"))
}

func TestOpenInHistory_ItemInActiveSession(t *testing.T) {
	t.Parallel()
	svc, _, _ := testService(t)
	first := svc.AddToSession(makeImageJPEG(t, 32, 32), nil, "generate", false, nil)
	svc.AddToSession(makeImageJPEG(t, 32, 32), nil, "generate", false, nil)
	sess := activeSession(t, svc)

	require.NoError(t, svc.OpenInHistory(first))
	assert.Equal(t, sess.ID, activeSession(t, svc).ID)
	active, err := svc.GetActiveSessionItem()
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, first, active.ID)
}

func TestOpenInHistory_UnknownID(t *testing.T) {
	t.Parallel()
	svc, _, _ := testService(t)
	sess := activeSession(t, svc)
	assert.Error(t, svc.OpenInHistory(99999))
	assert.Equal(t, sess.ID, activeSession(t, svc).ID)
}
