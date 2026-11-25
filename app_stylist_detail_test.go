package main

import (
	"context"
	"testing"

	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStylistDetailShootSnapshot(t *testing.T) {
	blend := 0.3
	q := &spQueue{}
	_, err := stylistShoot(context.Background(), spPeopleList(), nil, q, false, StylistShootRequest{
		Person: "anna", FaceSwap: true, FacePass: true, FaceRestoreBlend: &blend,
		Series: []stylist.Series{spSeries(stylist.Shot{Prompt: "front"})},
	})
	require.NoError(t, err)
	require.Len(t, q.jobs, 1)
	assert.True(t, q.jobs[0].params.FacePass)
	require.NotNil(t, q.jobs[0].params.FaceRestoreBlend)
	assert.Equal(t, blend, *q.jobs[0].params.FaceRestoreBlend)
	blend = 1
	assert.Equal(t, 0.3, *q.jobs[0].params.FaceRestoreBlend, "queued options must not follow later form changes")
}

func TestStylistDetailReshootPreservesOptions(t *testing.T) {
	st := newLsStore()
	q := &spQueue{}
	rec := &recordingQueue{q: q}
	blend := 0.0
	_, err := rec.EnqueueLabeled(queue.JobStylistFrame, mcp.ShootFrameParams{Person: "anna", Prompt: "front", FacePass: true, FaceRestoreBlend: &blend}, stylistPageSource, "front")
	require.NoError(t, err)
	require.NoError(t, rememberShoot(st, []int64{101}))
	require.NoError(t, saveShootParams(st, rec.frames))
	_, err = reshootFrame(st, q, false, 101, "")
	require.NoError(t, err)
	require.Len(t, q.jobs, 2)
	assert.True(t, q.jobs[1].params.FacePass)
	require.NotNil(t, q.jobs[1].params.FaceRestoreBlend)
	assert.Zero(t, *q.jobs[1].params.FaceRestoreBlend)
}

func TestStylistDetailLastShootRestoresFacePass(t *testing.T) {
	st := newLsStore(500)
	require.NoError(t, rememberShoot(st, []int64{1}))
	_, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: "completed", ItemID: 500, FacePass: "skipped:лицо не найдено"}})
	require.NoError(t, err)
	got, err := mergeLastShoot(st, []StylistFrame{{JobID: 1, Status: stylistFrameGone}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "skipped:лицо не найдено", got[0].FacePass)
}

func TestStylistDetailEditPreservesFaceOptions(t *testing.T) {
	st, q, items := editEnqueueFixture(t)
	store, err := loadShootParams(st)
	require.NoError(t, err)
	frame := store.Frames["101"]
	blend := 0.3
	frame.Params.FacePass = true
	frame.Params.FaceRestoreBlend = &blend
	require.NoError(t, saveShootParams(st, map[int64]shootParamsFrame{editSourceJob: frame}))
	_, err = enqueueStylistEdit(st, q, items, false, editSourceJob, "change the pose")
	require.NoError(t, err)
	require.Len(t, q.jobs, 2)
	assert.True(t, q.jobs[1].params.FacePass)
	require.NotNil(t, q.jobs[1].params.FaceRestoreBlend)
	assert.Equal(t, blend, *q.jobs[1].params.FaceRestoreBlend)
}

type detailSourceTestReader struct{ calls int }

func (r *detailSourceTestReader) StylistDetailSource(int64, string) (string, error) {
	r.calls++
	return "synthetic-source", nil
}
func TestStylistDetailSourceRejectsWrongAssociation(t *testing.T) {
	reader := &detailSourceTestReader{}
	_, _, items := editEnqueueFixture(t)
	_, err := detailSource(reader, items, editSourceItem+1, "base")
	require.Error(t, err)
	assert.Zero(t, reader.calls)
	_, err = detailSource(reader, items, editSourceItem, "../base")
	require.Error(t, err)
	assert.Zero(t, reader.calls)
	source, err := detailSource(reader, items, editSourceItem, "base")
	require.NoError(t, err)
	assert.Equal(t, "synthetic-source", source)
	assert.Equal(t, 1, reader.calls)
}
