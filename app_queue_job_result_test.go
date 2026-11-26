package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/stretchr/testify/require"
)

func TestGetQueueJobResult_OutsidePanelHistoryWithoutInputSnapshot(t *testing.T) {
	const finishedJobsBeyondPanelLimit = 101
	const unknownJobID int64 = -1
	const sourceSnapshot = "PRIVATE_INPUT_IMAGE_BASE64"
	const completedResult = `{"session_item_id":42,"info":"{}"}`
	db, err := preset.Open(filepath.Join(t.TempDir(), "queue-results.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store := queue.NewStore(db.DB())
	app := &App{queueSvc: queue.NewService(store, nil, resolutionTestEmitter{})}

	id, err := store.Create(queue.JobResolutionUpscale, `{"image_base64":"`+sourceSnapshot+`"}`, "test")
	require.NoError(t, err)
	require.NoError(t, store.UpdateCompleted(id, completedResult))
	for i := 0; i < finishedJobsBeyondPanelLimit; i++ {
		newerID, err := store.Create(queue.JobResolutionUpscale, "{}", "test")
		require.NoError(t, err)
		require.NoError(t, store.UpdateCompleted(newerID, "{}"))
	}
	panel, err := app.GetQueue()
	require.NoError(t, err)
	for _, job := range panel {
		require.NotEqual(t, id, job.ID, "fixture must put the requested result outside panel history")
	}

	job, err := app.GetQueueJobResult(id)
	require.NoError(t, err)
	require.Equal(t, id, job.ID)
	require.Equal(t, queue.JobResolutionUpscale, job.Type)
	require.Equal(t, queue.StatusCompleted, job.Status)
	require.Equal(t, completedResult, job.Result)
	require.Empty(t, job.Params)
	require.Empty(t, job.Error)
	encoded, err := json.Marshal(job)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), sourceSnapshot)

	missing, err := app.GetQueueJobResult(unknownJobID)
	require.Error(t, err)
	require.Nil(t, missing)
}

func TestGetQueueJobResult_PreservesFailureStatusAndError(t *testing.T) {
	const failureMessage = "upscale server unavailable"
	db, err := preset.Open(filepath.Join(t.TempDir(), "failed-result.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store := queue.NewStore(db.DB())
	app := &App{queueSvc: queue.NewService(store, nil, resolutionTestEmitter{})}
	id, err := store.Create(queue.JobResolutionUpscale, `{"image_base64":"INPUT"}`, "test")
	require.NoError(t, err)
	require.NoError(t, store.UpdateStatus(id, queue.StatusFailed, failureMessage))
	job, err := app.GetQueueJobResult(id)
	require.NoError(t, err)
	require.Equal(t, id, job.ID)
	require.Equal(t, queue.StatusFailed, job.Status)
	require.Equal(t, failureMessage, job.Error)
	require.Empty(t, job.Params)
	require.Empty(t, job.Result)
}
