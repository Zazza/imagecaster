package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strconv"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sourceTestMaxVariants = 8
const sourceTestInstructionMaxRunes = 2000

// This fake imports the real photo once, then reports its durable history item.
type sourceTestSessions struct {
	imports       int
	image, source string
	item          *preset.SessionItem
	err           error
	importID      int64
}

func (s *sourceTestSessions) AddToSession(image string, _ json.RawMessage, source string, _ *preset.GenerationMeta) int64 {
	s.imports++
	s.image, s.source = image, source
	return s.importID
}
func (s *sourceTestSessions) SessionItem(int64) (*preset.SessionItem, error) { return s.item, s.err }

func sourceTestRequest(t *testing.T) StylistSourceShootRequest {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	return StylistSourceShootRequest{
		ImageBase64:  base64.StdEncoding.EncodeToString(b.Bytes()),
		Instructions: []string{"Move the existing right hand into the hair.", "Turn the head slightly left."},
		Editor:       &generation.PostEditorParams{Model: stylistEditorModel, Loras: []preset.LoRAEntry{{Name: "Chosen", Weight: stylistEditorWeight}}},
	}
}

func sourceTestSession() *sourceTestSessions {
	return &sourceTestSessions{importID: editSourceItem, item: &preset.SessionItem{ID: editSourceItem, SessionID: editSourceSession}}
}

func TestSourceVariantsImportOnceAndQueueIndependentOriginalReferences(t *testing.T) {
	st, q, sessions := newLsStore(), &spQueue{}, sourceTestSession()
	req := sourceTestRequest(t)
	res, err := enqueueSourceVariants(st, q, sessions, false, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Error)
	assert.Equal(t, []int64{101, 102}, res.JobIDs)
	assert.Equal(t, res.JobIDs, lastShootIDs(st))
	assert.Equal(t, 1, sessions.imports)
	assert.Equal(t, req.ImageBase64, sessions.image)
	assert.NotEmpty(t, sessions.source)
	require.Len(t, q.jobs, len(req.Instructions))
	for i, job := range q.jobs {
		p := job.params
		assert.Equal(t, queue.JobStylistFrame, job.jobType)
		assert.Equal(t, stylistPageSource, job.source)
		assert.NotEmpty(t, job.label)
		assert.Equal(t, editSourceItem, p.SourceItemID)
		assert.Equal(t, editSourceSession, p.SourceSessionID)
		assert.True(t, p.SourcePose)
		assert.Equal(t, req.Instructions[i], p.EditInstruction)
		assert.Empty(t, p.Person)
		assert.False(t, p.FaceSwap)
		assert.False(t, p.FacePass)
		assert.Empty(t, p.FaceSwapName)
		assert.True(t, p.ToFolder)
		assert.Equal(t, req.Editor, p.Editor)
	}
	// A later editor selection cannot change jobs that are already queued.
	req.Editor.Model = "qwen_image_2.1-Q8_0.gguf"
	req.Editor.Loras[0].Name = "Changed after launch"
	for _, job := range q.jobs {
		require.NotNil(t, job.params.Editor)
		assert.Equal(t, stylistEditorModel, job.params.Editor.Model)
		assert.Equal(t, "Chosen", job.params.Editor.Loras[0].Name)
	}
	for _, value := range st.settings {
		assert.NotContains(t, value, req.ImageBase64, "the photo belongs in history, not shared settings")
	}
	// Every ID must have persisted parameters so reshoot still works after restart.
	for _, id := range res.JobIDs {
		replay := &spQueue{jobs: make([]spJob, len(q.jobs))}
		_, err := reshootFrame(st, replay, false, id, "")
		require.NoError(t, err)
		p := replay.jobs[len(q.jobs)].params
		assert.Equal(t, editSourceItem, p.SourceItemID)
		assert.Equal(t, editSourceSession, p.SourceSessionID)
		assert.Equal(t, stylistEditorModel, p.Editor.Model)
	}
}

func TestSourceVariantsValidateWholeRequestBeforeImportOrQueue(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remote bool
		mutate func(*StylistSourceShootRequest)
	}{
		{name: "remote GPU", remote: true},
		{name: "missing photo", mutate: func(p *StylistSourceShootRequest) { p.ImageBase64 = "" }},
		{name: "invalid base64", mutate: func(p *StylistSourceShootRequest) { p.ImageBase64 = "not base64!!" }},
		{name: "base64 text", mutate: func(p *StylistSourceShootRequest) {
			p.ImageBase64 = base64.StdEncoding.EncodeToString([]byte("not a photo"))
		}},
		{name: "truncated photo", mutate: func(p *StylistSourceShootRequest) {
			b, err := base64.StdEncoding.DecodeString(p.ImageBase64)
			require.NoError(t, err)
			p.ImageBase64 = base64.StdEncoding.EncodeToString(b[:len(b)/2])
		}},
		{name: "oversize photo", mutate: func(p *StylistSourceShootRequest) { p.ImageBase64 = appOverLimitBase64() }},
		{name: "no variants", mutate: func(p *StylistSourceShootRequest) { p.Instructions = nil }},
		{name: "too many variants", mutate: func(p *StylistSourceShootRequest) {
			p.Instructions = make([]string, sourceTestMaxVariants+1)
			for i := range p.Instructions {
				p.Instructions[i] = "Smile"
			}
		}},
		{name: "blank later variant", mutate: func(p *StylistSourceShootRequest) { p.Instructions[1] = " \n " }},
		{name: "long later variant", mutate: func(p *StylistSourceShootRequest) {
			p.Instructions[1] = strings.Repeat("я", sourceTestInstructionMaxRunes+1)
		}},
		{name: "explicit editor required", mutate: func(p *StylistSourceShootRequest) { p.Editor = nil }},
		{name: "invalid editor", mutate: func(p *StylistSourceShootRequest) { p.Editor.Model = "invented-editor.gguf" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, q, sessions := newLsStore(), &spQueue{}, sourceTestSession()
			require.NoError(t, rememberShoot(st, []int64{900}))
			req := sourceTestRequest(t)
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			_, err := enqueueSourceVariants(st, q, sessions, tc.remote, req)
			require.Error(t, err)
			assert.Zero(t, sessions.imports)
			assert.Empty(t, q.jobs)
			assert.Equal(t, []int64{900}, lastShootIDs(st))
		})
	}
}

func TestSourceVariantsAcceptsUpperCountAndRuneBoundaries(t *testing.T) {
	st, q, sessions := newLsStore(), &spQueue{}, sourceTestSession()
	req := sourceTestRequest(t)
	req.Instructions = make([]string, sourceTestMaxVariants)
	for i := range req.Instructions {
		req.Instructions[i] = strings.Repeat("я", sourceTestInstructionMaxRunes-1) + strconv.Itoa(i)
	}
	res, err := enqueueSourceVariants(st, q, sessions, false, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Error)
	require.Len(t, q.jobs, sourceTestMaxVariants)
	assert.Len(t, res.JobIDs, sourceTestMaxVariants)
	for i, job := range q.jobs {
		require.NotNil(t, job.params.Editor)
		assert.Equal(t, stylistEditorModel, job.params.Editor.Model)
		assert.Equal(t, req.Instructions[i], job.params.EditInstruction)
	}
}

func TestSourceVariantsRequireDurableOriginalBeforeQueue(t *testing.T) {
	for _, scenario := range []string{"import failed", "lookup failed", "missing item", "wrong item", "missing session"} {
		t.Run(scenario, func(t *testing.T) {
			st, q, sessions := newLsStore(), &spQueue{}, sourceTestSession()
			switch scenario {
			case "import failed":
				sessions.importID = 0
			case "lookup failed":
				sessions.err = errors.New("history unavailable")
			case "missing item":
				sessions.item = nil
			case "wrong item":
				sessions.item.ID++
			case "missing session":
				sessions.item.SessionID = 0
			}
			_, err := enqueueSourceVariants(st, q, sessions, false, sourceTestRequest(t))
			require.Error(t, err)
			assert.Empty(t, q.jobs)
		})
	}
}

// Check durability at the next enqueue boundary, while the series is still running.
type sourceDurableQueue struct {
	spQueue
	t     *testing.T
	store *lsStore
}

func (q *sourceDurableQueue) EnqueueLabeled(kind queue.JobType, params any, source, label string) (int64, error) {
	if len(q.jobs) > 0 {
		frames := loadLastShoot(q.store)
		require.Len(q.t, frames, 1)
		assert.Equal(q.t, int64(101), frames[0].JobID)
		assert.NotEmpty(q.t, q.store.settings[stylistShootParamsKey])
	}
	return q.spQueue.EnqueueLabeled(kind, params, source, label)
}

func TestSourceVariantsPartialEnqueuePreservesRealIDsAndReplay(t *testing.T) {
	st := newLsStore()
	q := &sourceDurableQueue{spQueue: spQueue{failAt: 2}, t: t, store: st}
	res, err := enqueueSourceVariants(st, q, sourceTestSession(), false, sourceTestRequest(t))
	require.NoError(t, err, "a Go error would discard IDs in the UI binding")
	require.NotNil(t, res)
	assert.Equal(t, []int64{101}, res.JobIDs)
	assert.NotEmpty(t, res.Error)
	assert.Equal(t, res.JobIDs, lastShootIDs(st))
	replay := &spQueue{jobs: make([]spJob, len(q.jobs))}
	_, err = reshootFrame(st, replay, false, res.JobIDs[0], "")
	require.NoError(t, err)
	assert.Equal(t, editSourceItem, replay.jobs[len(q.jobs)].params.SourceItemID)
}

func TestSourceVariantsFirstEnqueueFailureDoesNotReportPhantomIDs(t *testing.T) {
	st, q := newLsStore(), &spQueue{failAt: 1}
	res, err := enqueueSourceVariants(st, q, sourceTestSession(), false, sourceTestRequest(t))
	if err == nil {
		require.NotNil(t, res)
		assert.NotEmpty(t, res.Error)
	}
	if res != nil {
		assert.Empty(t, res.JobIDs)
	}
	assert.Empty(t, q.jobs)
	assert.Empty(t, lastShootIDs(st))
}

type sourcePoseTestEngine struct {
	*editTestEngine
	strictCalls                    int
	strictImage, strictInstruction string
}

func (e *sourcePoseTestEngine) PrepareSourcePose(image, instruction string) (string, error) {
	e.strictCalls++
	e.strictImage, e.strictInstruction = image, instruction
	return "Both existing arms hang relaxed alongside the body.", nil
}

func TestStylistSourcePosePreparationDispatch(t *testing.T) {
	for _, scenario := range []string{"source pose", "missing strict capability", "manual edit"} {
		t.Run(scenario, func(t *testing.T) {
			base, sessions, p := editRunFixture()
			p.SourcePose = scenario != "manual edit"
			p.FaceSwap = false
			p.EditInstruction = "Обе руки свободно опущены вдоль тела."
			strict := &sourcePoseTestEngine{editTestEngine: base}
			var eng stylistEditEngine = strict
			if scenario == "missing strict capability" {
				eng = base
			}
			res, err := runStylistEdit(eng, sessions, p, nil)
			if scenario == "missing strict capability" {
				require.Error(t, err)
				assert.Nil(t, res)
				assert.Zero(t, base.prepareCalls)
				assert.Zero(t, base.calls)
				assert.Empty(t, sessions.savedImage)
				return
			}
			require.NoError(t, err)
			if p.SourcePose {
				assert.Equal(t, 1, strict.strictCalls)
				assert.Zero(t, base.prepareCalls)
				assert.Equal(t, sessions.image, strict.strictImage)
				assert.Equal(t, p.EditInstruction, strict.strictInstruction)
				assert.Contains(t, base.instruction, "Both existing arms hang relaxed alongside the body.")
				assert.NotContains(t, base.instruction, base.prepared)
			} else {
				assert.Zero(t, strict.strictCalls)
				assert.Equal(t, 1, base.prepareCalls)
				assert.Contains(t, base.instruction, base.prepared)
			}
		})
	}
}

func TestSourceVariantsKeepSelectedHistoryGroupAfterSwitch(t *testing.T) {
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	sessions := session.New(db, t.TempDir(), resolutionTestEmitter{})
	selected, err := sessions.CreateSession("Selected group")
	require.NoError(t, err)
	captured := struct {
		generation.SessionAdder
		stylistEditItems
	}{sessions.PinnedTo(selected.ID), sessions}
	other, err := sessions.CreateSession("Opened later")
	require.NoError(t, err)
	groupsBefore, err := sessions.ListSessions()
	require.NoError(t, err)
	q := &spQueue{}
	result, err := enqueueSourceVariants(newLsStore(), q, captured, false, sourceTestRequest(t))
	require.NoError(t, err)
	require.Empty(t, result.Error)
	require.Len(t, q.jobs, 2)
	originalID := q.jobs[0].params.SourceItemID
	original, err := sessions.SessionItem(originalID)
	require.NoError(t, err)
	require.Equal(t, selected.ID, original.SessionID)
	for _, job := range q.jobs {
		assert.Equal(t, originalID, job.params.SourceItemID)
		assert.Equal(t, selected.ID, job.params.SourceSessionID)
	}
	active, err := db.GetActiveSessionID()
	require.NoError(t, err)
	assert.Equal(t, other.ID, active)
	groupsAfter, err := sessions.ListSessions()
	require.NoError(t, err)
	assert.Len(t, groupsAfter, len(groupsBefore))
}

func TestSourceVariantsRequireSelectedHistoryGroup(t *testing.T) {
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.DB().Exec("DELETE FROM session_active")
	require.NoError(t, err)
	groupsBefore, err := db.ListSessions()
	require.NoError(t, err)
	sessions := session.New(db, t.TempDir(), resolutionTestEmitter{})
	app := &App{presets: db, sessions: sessions, queueSvc: queue.NewService(queue.NewStore(db.DB()), nil, resolutionTestEmitter{})}
	_, err = app.StylistSourceShoot(sourceTestRequest(t))
	require.EqualError(t, err, "выберите папку истории")
	groups, err := sessions.ListSessions()
	require.NoError(t, err)
	assert.Len(t, groups, len(groupsBefore))
}
