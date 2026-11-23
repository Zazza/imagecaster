package main

import (
	"errors"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStylistEditPreparationUsesSourceAndKeepsUserDescription(t *testing.T) {
	eng, sessions, p := editRunFixture()
	p.EditInstruction = "Unbutton the jacket"
	eng.prepared = "The jacket is open, revealing the blouse beneath."
	res, err := runStylistEdit(eng, sessions, p, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, eng.prepareCalls)
	assert.Equal(t, sessions.image, eng.prepareImage)
	assert.Equal(t, p.EditInstruction, eng.prepareInstruction)
	assert.Equal(t, sessions.image, eng.image)
	assert.Contains(t, eng.instruction, eng.prepared)
	assert.Equal(t, eng.instruction, res.Prompt)
	assert.Equal(t, p.EditInstruction, res.Shot)
	require.NotNil(t, sessions.savedMeta)
	assert.Contains(t, sessions.savedMeta.Label, p.EditInstruction)
	assert.NotContains(t, sessions.savedMeta.Label, eng.prepared)
}

func TestStylistEditPreparationFailureNeverGeneratesOrSaves(t *testing.T) {
	plannerErr := errors.New("local vision unavailable")
	for _, tc := range []struct {
		name, prepared string
		err            error
	}{
		{name: "planner error", prepared: "unused", err: plannerErr},
		{name: "empty response"},
		{name: "whitespace response", prepared: " \n "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eng, sessions, p := editRunFixture()
			eng.prepared, eng.prepareErr = tc.prepared, tc.err
			res, err := runStylistEdit(eng, sessions, p, nil)
			require.Error(t, err)
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
			}
			assert.Nil(t, res)
			assert.Zero(t, eng.calls)
			assert.Empty(t, eng.faceImage)
			assert.Empty(t, sessions.savedImage)
		})
	}
}

func TestStylistEditCancelledAfterPreparationSkipsGPU(t *testing.T) {
	eng, sessions, p := editRunFixture()
	res, err := runStylistEdit(eng, sessions, p, func() bool { return eng.prepareCalls > 0 })
	require.ErrorIs(t, err, generation.ErrInterrupted)
	assert.Nil(t, res)
	assert.Equal(t, 1, eng.prepareCalls)
	assert.Zero(t, eng.calls)
	assert.Empty(t, eng.faceImage)
	assert.Empty(t, sessions.savedImage)
}
