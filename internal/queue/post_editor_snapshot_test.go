package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookJobPersistsEditorSnapshotAcrossSettingsMutation(t *testing.T) {
	svc, _ := newLookTestService(t)
	const selectedModel = "flux3-image-klein-Q4_K.gguf"
	const selectedWeight = 0.65
	params := generation.ApplyLookParams{ImageBase64: "aGVsbG8=", Text: "film look", Editor: &generation.PostEditorParams{Model: selectedModel, Loras: []preset.LoRAEntry{{Name: "SharedDetail", Weight: selectedWeight}}}}
	id, err := svc.Enqueue(JobLook, params, "look")
	require.NoError(t, err)
	params.Editor.Model = "qwen"
	params.Editor.Loras[0].Name = "ChangedAfterEnqueue"
	stored, err := svc.store.Get(id)
	require.NoError(t, err)
	var restored generation.ApplyLookParams
	require.NoError(t, json.Unmarshal([]byte(stored.Params), &restored))
	require.NotNil(t, restored.Editor)
	assert.Equal(t, selectedModel, restored.Editor.Model)
	require.Len(t, restored.Editor.Loras, 1)
	assert.Equal(t, "SharedDetail", restored.Editor.Loras[0].Name)
	assert.Equal(t, selectedWeight, restored.Editor.Loras[0].Weight)
	again, err := svc.store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, stored.Params, again.Params)
}

func TestProcessLookRestoresEditorBeforeCallingGeneration(t *testing.T) {
	p := NewProcessor(nil, nil, nil, t.TempDir(), &mockEmitter{})
	job := &Job{Type: JobLook, Params: `{"image_base64":"aGVsbG8=","text":"film look","editor":{"model":"unknown-editor.gguf","loras":[]}}`}
	result, err := p.ProcessJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported editor model")
	assert.Nil(t, result)
}
