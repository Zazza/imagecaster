package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const editSourceJob int64 = 101
const editSourceItem int64 = 501
const editSourceSession int64 = 7

type editTestQueue struct {
	spQueue
	result []*queue.Job
}

func (q *editTestQueue) Results([]int64) ([]*queue.Job, error) { return q.result, nil }

type editTestItems struct {
	item *preset.SessionItem
	err  error
}

func (s *editTestItems) SessionItem(int64) (*preset.SessionItem, error) { return s.item, s.err }

func editEnqueueFixture(t *testing.T) (*lsStore, *editTestQueue, *editTestItems) {
	t.Helper()
	st := newLsStore(editSourceItem)
	q := &editTestQueue{}
	rec := &recordingQueue{q: q}
	_, err := rec.EnqueueLabeled(queue.JobStylistFrame, mcp.ShootFrameParams{Person: "original-person", FaceSwap: true, FaceSwapName: "original-face", Prompt: "original pose", GarmentBase64: "original garment", ToFolder: true}, stylistPageSource, "Source frame")
	require.NoError(t, err)
	require.NoError(t, rememberShoot(st, []int64{editSourceJob}))
	require.NoError(t, saveShootParams(st, rec.frames))
	_, err = mergeLastShoot(st, []StylistFrame{{JobID: editSourceJob, Status: string(queue.StatusCompleted), ItemID: editSourceItem}})
	require.NoError(t, err)
	frame, err := json.Marshal(mcp.ShootFrameResult{ID: editSourceItem})
	require.NoError(t, err)
	result, err := json.Marshal(queue.JobResult{SessionItemID: editSourceItem, Frame: frame})
	require.NoError(t, err)
	q.result = []*queue.Job{{ID: editSourceJob, Type: queue.JobStylistFrame, Status: queue.StatusCompleted, Source: stylistPageSource, Result: string(result)}}
	return st, q, &editTestItems{item: &preset.SessionItem{ID: editSourceItem, SessionID: editSourceSession}}
}

func TestStylistEditEnqueuePreservesOriginalAndIdentity(t *testing.T) {
	st, q, items := editEnqueueFixture(t)
	id, err := enqueueStylistEdit(st, q, items, false, editSourceJob, " change the pose ")
	require.NoError(t, err)
	require.Len(t, q.jobs, 2)
	p := q.jobs[1].params
	assert.Equal(t, queue.JobStylistFrame, q.jobs[1].jobType)
	assert.Equal(t, stylistPageSource, q.jobs[1].source)
	assert.Equal(t, editSourceItem, p.SourceItemID)
	assert.Equal(t, editSourceSession, p.SourceSessionID)
	assert.Equal(t, "change the pose", p.EditInstruction)
	assert.Equal(t, "original-person", p.Person)
	assert.True(t, p.FaceSwap)
	assert.Equal(t, "original-face", p.FaceSwapName)
	assert.Equal(t, []int64{editSourceJob, id}, lastShootIDs(st))
}

func TestStylistEditEnqueueRefusesInvalidSourcesWithoutChangingShoot(t *testing.T) {
	cases := []struct {
		name        string
		instruction string
		remote      bool
		mutate      func(*editTestQueue, *editTestItems)
	}{
		{name: "blank", instruction: " \n "},
		{name: "too long", instruction: strings.Repeat("x", stylistEditInstructionMax+1)},
		{name: "remote", instruction: "red dress", remote: true},
		{name: "pending", instruction: "red dress", mutate: func(q *editTestQueue, _ *editTestItems) { q.result[0].Status = queue.StatusPending }},
		{name: "failed", instruction: "red dress", mutate: func(q *editTestQueue, _ *editTestItems) { q.result[0].Status = queue.StatusFailed }},
		{name: "missing", instruction: "red dress", mutate: func(_ *editTestQueue, s *editTestItems) { s.item = nil; s.err = errors.New("missing") }},
		{name: "enqueue fails", instruction: "red dress", mutate: func(q *editTestQueue, _ *editTestItems) { q.failAt = 2 }},
		{name: "result mismatch", instruction: "red dress", mutate: func(q *editTestQueue, _ *editTestItems) {
			q.result[0].Result = `{"session_item_id":999,"frame":{"id":999}}`
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, q, items := editEnqueueFixture(t)
			if tc.mutate != nil {
				tc.mutate(q, items)
			}
			_, err := enqueueStylistEdit(st, q, items, tc.remote, editSourceJob, tc.instruction)
			require.Error(t, err)
			assert.Equal(t, []int64{editSourceJob}, lastShootIDs(st))
			assert.Len(t, q.jobs, 1)
		})
	}
	t.Run("foreign job", func(t *testing.T) {
		st, q, items := editEnqueueFixture(t)
		_, err := enqueueStylistEdit(st, q, items, false, 999, "red dress")
		require.Error(t, err)
		assert.Len(t, q.jobs, 1)
	})
}

type editTestEngine struct {
	image, instruction, faceName, faceImage    string
	prepareImage, prepareInstruction, prepared string
	prepareErr                                 error
	prepareCalls                               int
	result                                     *generation.GenerateImageResult
	err                                        error
	calls                                      int
}

func (e *editTestEngine) PrepareStylistEdit(image, instruction string) (string, error) {
	e.prepareCalls++
	e.prepareImage, e.prepareInstruction = image, instruction
	return e.prepared, e.prepareErr
}

func (e *editTestEngine) KleinEdit(image, instruction string, _ *int64) (*generation.GenerateImageResult, error) {
	e.calls++
	e.image = image
	e.instruction = instruction
	return e.result, e.err
}
func (e *editTestEngine) FaceSwap(image, name string) (string, string) {
	e.faceImage = image
	e.faceName = name
	return "swapped-image", "done"
}

type editTestSessions struct {
	editTestItems
	image        string
	savedSession int64
	savedImage   string
	savedMeta    *preset.GenerationMeta
	savedID      int64
}

func (s *editTestSessions) GetActiveSessionItem() (*preset.SessionItem, error) {
	return nil, errors.New("active folder must not be used")
}
func (s *editTestSessions) GetSessionImage(int64) (string, error) { return s.image, nil }
func (s *editTestSessions) AddToSessionID(id int64, image string, _ json.RawMessage, _ string, meta *preset.GenerationMeta) int64 {
	s.savedSession = id
	s.savedImage = image
	s.savedMeta = meta
	return s.savedID
}
func editRunFixture() (*editTestEngine, *editTestSessions, mcp.ShootFrameParams) {
	return &editTestEngine{prepared: "prepared edit instruction", result: &generation.GenerateImageResult{Image: "edited-image"}},
		&editTestSessions{editTestItems: editTestItems{item: &preset.SessionItem{ID: editSourceItem, SessionID: editSourceSession}}, image: "exact-source-image", savedID: 601},
		mcp.ShootFrameParams{Person: "original-person", SourceItemID: editSourceItem, SourceSessionID: editSourceSession, EditInstruction: "change the pose", FaceSwap: true, FaceSwapName: "original-face"}
}
func TestStylistEditRunUsesSourceImageOriginalIdentityAndFolder(t *testing.T) {
	eng, sessions, p := editRunFixture()
	res, err := runStylistEdit(eng, sessions, p, func() bool { return false })
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "exact-source-image", eng.image)
	assert.Contains(t, eng.instruction, eng.prepared)
	assert.Equal(t, "change the pose", eng.prepareInstruction)
	for _, forbidden := range []string{"preserve the pose", "preserve pose", "preserve the composition", "preserve composition", "do not change the pose", "do not change the composition"} {
		assert.NotContains(t, strings.ToLower(eng.instruction), forbidden, "must not contradict the requested pose or composition change")
	}
	assert.Equal(t, "edited-image", eng.faceImage)
	assert.Equal(t, "original-face", eng.faceName)
	assert.Equal(t, editSourceSession, sessions.savedSession)
	assert.Equal(t, "swapped-image", sessions.savedImage)
	assert.Equal(t, sessions.savedID, res.ID)
	require.NotNil(t, sessions.savedMeta)
	assert.Equal(t, editSourceItem, sessions.savedMeta.SourceItem)
}
func TestStylistEditRunDoesNotSwapWhenOriginalDidNot(t *testing.T) {
	eng, sessions, p := editRunFixture()
	p.FaceSwap = false
	_, err := runStylistEdit(eng, sessions, p, func() bool { return false })
	require.NoError(t, err)
	assert.Empty(t, eng.faceName)
	assert.Equal(t, "edited-image", sessions.savedImage)
}
func TestStylistEditRunFailureNeverSavesFalseResult(t *testing.T) {
	for _, scenario := range []string{"deleted", "moved", "invalid item", "invalid session", "blank instruction", "nil result", "no image", "engine failed", "cancelled", "cancelled after generation", "save failed"} {
		t.Run(scenario, func(t *testing.T) {
			eng, sessions, p := editRunFixture()
			stopped := func() bool { return false }
			switch scenario {
			case "deleted":
				sessions.item = nil
				sessions.err = errors.New("deleted")
			case "moved":
				sessions.item.SessionID = editSourceSession + 1
			case "invalid item":
				p.SourceItemID = 0
			case "invalid session":
				p.SourceSessionID = 0
			case "blank instruction":
				p.EditInstruction = " "
			case "nil result":
				eng.result = nil
			case "no image":
				eng.result.Image = ""
			case "engine failed":
				eng.err = errors.New("Klein failed")
			case "cancelled":
				stopped = func() bool { return true }
			case "cancelled after generation":
				stopped = func() bool { return eng.calls > 0 }
			case "save failed":
				sessions.savedID = 0
			}
			res, err := runStylistEdit(eng, sessions, p, stopped)
			require.Error(t, err)
			assert.Nil(t, res)
			if scenario != "save failed" {
				assert.Empty(t, sessions.savedImage)
			}
		})
	}
}

func TestStylistEditRepeatedVariantUsesImmediateParentAndOriginalIdentity(t *testing.T) {
	st, q, items := editEnqueueFixture(t)
	variantJob, err := enqueueStylistEdit(st, q, items, false, editSourceJob, "change the pose")
	require.NoError(t, err)
	const variantItem int64 = 601
	st.items[variantItem] = true
	_, err = mergeLastShoot(st, []StylistFrame{{JobID: editSourceJob, Status: string(queue.StatusCompleted), ItemID: editSourceItem}, {JobID: variantJob, Status: string(queue.StatusCompleted), ItemID: variantItem}})
	require.NoError(t, err)
	frame, err := json.Marshal(mcp.ShootFrameResult{ID: variantItem})
	require.NoError(t, err)
	result, err := json.Marshal(queue.JobResult{SessionItemID: variantItem, Frame: frame})
	require.NoError(t, err)
	q.result = []*queue.Job{{ID: variantJob, Type: queue.JobStylistFrame, Status: queue.StatusCompleted, Source: stylistPageSource, Result: string(result)}}
	items.item = &preset.SessionItem{ID: variantItem, SessionID: editSourceSession}
	nextJob, err := enqueueStylistEdit(st, q, items, false, variantJob, "blue dress")
	require.NoError(t, err)
	require.Len(t, q.jobs, 3)
	p := q.jobs[2].params
	assert.Equal(t, variantItem, p.SourceItemID)
	assert.Equal(t, editSourceSession, p.SourceSessionID)
	assert.True(t, p.FaceSwap)
	assert.Equal(t, "original-face", p.FaceSwapName)
	assert.Equal(t, []int64{editSourceJob, variantJob, nextJob}, lastShootIDs(st))
	eng, sessions, _ := editRunFixture()
	sessions.item = items.item
	sessions.image = "immediate-parent-image"
	_, err = runStylistEdit(eng, sessions, p, func() bool { return false })
	require.NoError(t, err)
	assert.Equal(t, "immediate-parent-image", eng.image)
	assert.Equal(t, "original-face", eng.faceName)
	assert.Equal(t, editSourceSession, sessions.savedSession)
}

func TestStylistEditShortPoseFragmentBecomesSourceAwareFinalState(t *testing.T) {
	eng, sessions, p := editRunFixture()
	p.EditInstruction = "Hand in hair"
	eng.prepared = "The existing right hand is in the hair; the right waist is clear."
	res, err := runStylistEdit(eng, sessions, p, func() bool { return false })
	require.NoError(t, err)
	assert.Equal(t, sessions.image, eng.prepareImage)
	assert.Equal(t, p.EditInstruction, eng.prepareInstruction)
	assert.Contains(t, eng.instruction, eng.prepared)
	assert.Equal(t, eng.instruction, res.Prompt)
	assert.Equal(t, p.EditInstruction, res.Shot)
}
