package main

import (
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stylistEditorModel = "flux3-image-klein-Q4_K.gguf"
const stylistEditorWeight = 0.75

func TestStylistExplicitEditorPersistsSnapshotAndReshoot(t *testing.T) {
	st, q, items := editEnqueueFixture(t)
	editor := &generation.PostEditorParams{Model: stylistEditorModel, Loras: []preset.LoRAEntry{{Name: "Chosen", Weight: stylistEditorWeight}}}
	id, err := enqueueStylistEditWithEditor(st, q, items, false, StylistEditParams{JobID: editSourceJob, Instruction: "change the pose", Editor: editor})
	require.NoError(t, err)
	require.Len(t, q.jobs, 2)
	require.NotNil(t, q.jobs[1].params.Editor)
	editor.Model = "qwen_image_2.1-Q8_0.gguf"
	editor.Loras[0].Name = "ChangedLater"
	assert.Equal(t, stylistEditorModel, q.jobs[1].params.Editor.Model)
	assert.Equal(t, "Chosen", q.jobs[1].params.Editor.Loras[0].Name)
	_, err = reshootFrame(st, q, false, id, "")
	require.NoError(t, err)
	require.Len(t, q.jobs, 3)
	assert.Equal(t, q.jobs[1].params.Editor, q.jobs[2].params.Editor)
	assert.Equal(t, "change the pose", q.jobs[2].params.EditInstruction)
	assert.Equal(t, editSourceItem, q.jobs[2].params.SourceItemID)
	assert.Equal(t, "original-face", q.jobs[2].params.FaceSwapName)
}

func TestStylistExplicitInvalidEditorNeverEnqueues(t *testing.T) {
	for _, model := range []string{"", "invented-model.gguf", "flux3-image-klein-invented.gguf"} {
		t.Run(model, func(t *testing.T) {
			st, q, items := editEnqueueFixture(t)
			_, err := enqueueStylistEditWithEditor(st, q, items, false, StylistEditParams{JobID: editSourceJob, Instruction: "change the pose", Editor: &generation.PostEditorParams{Model: model}})
			require.Error(t, err)
			assert.Len(t, q.jobs, 1)
			assert.Equal(t, []int64{editSourceJob}, lastShootIDs(st))
		})
	}
}

type selectedStylistEngine struct {
	editTestEngine
	editor *generation.PostEditorParams
}

func (e *selectedStylistEngine) StylistReferenceEdit(image, instruction string, seed *int64, editor *generation.PostEditorParams) (*generation.GenerateImageResult, error) {
	e.editor = editor
	return e.editTestEngine.KleinEdit(image, instruction, seed)
}
func TestStylistExplicitEditorUsesPreparedInstructionThenFaceSwap(t *testing.T) {
	legacy, sessions, p := editRunFixture()
	eng := &selectedStylistEngine{editTestEngine: *legacy}
	p.Editor = &generation.PostEditorParams{Model: stylistEditorModel, Loras: []preset.LoRAEntry{}}
	res, err := runStylistEdit(eng, sessions, p, func() bool { return false })
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, eng.editor)
	assert.Equal(t, p.Editor.Model, eng.editor.Model)
	assert.Empty(t, eng.editor.Loras)
	assert.Equal(t, "exact-source-image", eng.image)
	assert.Contains(t, eng.instruction, eng.prepared)
	assert.Equal(t, "edited-image", eng.faceImage)
	assert.Equal(t, "swapped-image", sessions.savedImage)
	assert.Equal(t, editSourceSession, sessions.savedSession)
	require.NotNil(t, sessions.savedMeta)
	assert.Equal(t, stylistEditorModel, sessions.savedMeta.Model)
}
func TestStylistInvalidExplicitEditorStopsBeforePreparation(t *testing.T) {
	legacy, sessions, p := editRunFixture()
	eng := &selectedStylistEngine{editTestEngine: *legacy}
	p.Editor = &generation.PostEditorParams{Model: "not-an-editor"}
	_, err := runStylistEdit(eng, sessions, p, func() bool { return false })
	require.Error(t, err)
	assert.Zero(t, eng.prepareCalls)
	assert.Zero(t, eng.calls)
	assert.Empty(t, sessions.savedImage)
}
