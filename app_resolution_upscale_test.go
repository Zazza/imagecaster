package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/session"
	"github.com/stretchr/testify/require"
)

func TestEnqueueImageResolutionUpscale_SnapshotAndValidation(t *testing.T) {
	db, err := preset.Open(filepath.Join(t.TempDir(), "resolution.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store := queue.NewStore(db.DB())
	app := &App{queueSvc: queue.NewService(store, nil, resolutionTestEmitter{})}
	source := prImage(t)
	for _, target := range []int{1920, 2560, 3840} {
		id, err := app.EnqueueImageResolutionUpscale(source, target)
		require.NoError(t, err)
		job, err := store.Get(id)
		require.NoError(t, err)
		require.Equal(t, queue.JobResolutionUpscale, job.Type)
		var p resolutionUpscaleParams
		require.NoError(t, json.Unmarshal([]byte(job.Params), &p))
		require.Equal(t, source, p.ImageBase64)
		require.Equal(t, target, p.TargetLong)
		require.Zero(t, p.Meta.SourceItem)
	}
	for _, target := range []int{-1, 0, 1080, 2000} {
		_, err := app.EnqueueImageResolutionUpscale(source, target)
		require.Error(t, err)
		_, err = app.EnqueueResolutionUpscale(1, target)
		require.Error(t, err)
	}
	for _, image := range []string{"", "not-an-image"} {
		_, err := app.EnqueueImageResolutionUpscale(image, 1920)
		require.Error(t, err)
	}
	jobs, err := store.ListForPanel(0)
	require.NoError(t, err)
	require.Len(t, jobs, 3, "invalid requests must not enqueue")
}

type resolutionTestEmitter struct{}

func (resolutionTestEmitter) Emit(string, ...any) {}

func TestEnqueueResolutionUpscale_PreservesOriginalAndMetadata(t *testing.T) {
	dir := t.TempDir()
	db, err := preset.Open(filepath.Join(dir, "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	folder, err := db.CreateSession("Originals")
	require.NoError(t, err)
	require.NoError(t, db.SetActiveSession(folder.ID))
	sessions := session.New(db, dir, resolutionTestEmitter{})
	source := prImage(t)
	info := json.RawMessage(`{"prompt":"original prompt","negative_prompt":"blur","sampler_name":"euler","steps":8,"cfg_scale":2,"seed":42,"width":8,"height":8,"denoising_strength":0.4}`)
	sourceID := sessions.AddToSession(source, info, "generate", &preset.GenerationMeta{Model: "original-model", Label: "original label"})
	require.NotZero(t, sourceID)
	original, err := db.GetSessionItem(sourceID)
	require.NoError(t, err)
	store := queue.NewStore(db.DB())
	app := &App{presets: db, sessions: sessions, queueSvc: queue.NewService(store, nil, resolutionTestEmitter{})}
	id, err := app.EnqueueResolutionUpscale(sourceID, 1920)
	require.NoError(t, err)
	job, err := store.Get(id)
	require.NoError(t, err)
	var params resolutionUpscaleParams
	require.NoError(t, json.Unmarshal([]byte(job.Params), &params))
	require.Equal(t, source, params.ImageBase64)
	require.Equal(t, sourceID, params.Meta.SourceItem)
	require.Equal(t, "original-model", params.Meta.Model)
	var snapshot map[string]any
	require.NoError(t, json.Unmarshal(params.Info, &snapshot))
	require.Equal(t, "original prompt", snapshot["prompt"])
	require.Equal(t, "blur", snapshot["negative_prompt"])
	require.Equal(t, float64(42), snapshot["seed"])
	require.Equal(t, 0.4, snapshot["denoising_strength"])
	after, err := db.GetSessionItem(sourceID)
	require.NoError(t, err)
	require.Equal(t, original, after)
	items, err := db.GetSessionItems(folder.ID)
	require.NoError(t, err)
	require.Len(t, items, 1, "enqueue alone must not create or replace history images")
}
