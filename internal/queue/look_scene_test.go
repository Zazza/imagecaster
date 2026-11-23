package queue

import (
	"encoding/json"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLookSceneQueuePersistsImmutableOptIn(t *testing.T) {
	svc, _ := newLookTestService(t)
	const model = "flux-2-klein-4b.safetensors"
	params := generation.ApplyLookParams{ChangeScene: true, ImageBase64: "aGVsbG8=", Text: "turn head and change lighting", Editor: &generation.PostEditorParams{Model: model}}
	id, err := svc.Enqueue(JobLook, params, "look")
	require.NoError(t, err)
	params.ChangeScene = false
	params.Editor.Model = "changed"
	job, err := svc.store.Get(id)
	require.NoError(t, err)
	var restored generation.ApplyLookParams
	require.NoError(t, json.Unmarshal([]byte(job.Params), &restored))
	assert.True(t, restored.ChangeScene)
	require.NotNil(t, restored.Editor)
	assert.Equal(t, model, restored.Editor.Model)
	assert.Contains(t, job.Params, `"change_scene":true`)
}
func TestLookSceneQueueRejectsMissingOrUnknownEditor(t *testing.T) {
	for _, editor := range []*generation.PostEditorParams{nil, {Model: "unknown-editor.gguf"}} {
		svc, _ := newLookTestService(t)
		_, err := svc.Enqueue(JobLook, generation.ApplyLookParams{ChangeScene: true, ImageBase64: "aGVsbG8=", Text: "turn head", Editor: editor}, "look")
		require.Error(t, err)
		assert.Zero(t, countJobs(t, svc))
	}
}
func TestLookSceneDefaultJSONRemainsCompatible(t *testing.T) {
	raw, err := json.Marshal(generation.ApplyLookParams{Text: "lighting"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "change_scene")
}
