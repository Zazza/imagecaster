package main

import (
	"errors"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type historyEditErrorQueue struct {
	*editTestQueue
	err error
}

func (q *historyEditErrorQueue) Results([]int64) ([]*queue.Job, error) {
	return q.result, q.err
}

func TestStylistEditHistoryWithoutQueue(t *testing.T) {
	st, q, items := editEnqueueFixture(t)
	q.result = nil
	id, err := enqueueStylistEdit(st, q, items, false, editSourceJob, "change the pose")
	require.NoError(t, err)
	require.Len(t, q.jobs, 2)
	p := q.jobs[1].params
	assert.Equal(t, editSourceItem, p.SourceItemID)
	assert.Equal(t, editSourceSession, p.SourceSessionID)
	assert.Equal(t, "original-person", p.Person)
	assert.True(t, p.FaceSwap)
	assert.Equal(t, "original-face", p.FaceSwapName)
	assert.Equal(t, []int64{editSourceJob, id}, lastShootIDs(st))
}

func TestStylistEditHistoryRejectsContradictoryQueueJob(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*queue.Job)
	}{
		{"wrong type", func(j *queue.Job) { j.Type = "other-job-type" }},
		{"pending", func(j *queue.Job) { j.Status = queue.StatusPending }},
		{"failed", func(j *queue.Job) { j.Status = queue.StatusFailed }},
		{"malformed result", func(j *queue.Job) { j.Result = "{broken" }},
		{"mismatched item", func(j *queue.Job) { j.Result = `{"session_item_id":999,"frame":{"id":999}}` }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, q, items := editEnqueueFixture(t)
			tc.mutate(q.result[0])
			_, err := enqueueStylistEdit(st, q, items, false, editSourceJob, "change the pose")
			require.Error(t, err)
			assert.Len(t, q.jobs, 1)
			assert.Equal(t, []int64{editSourceJob}, lastShootIDs(st))
		})
	}
}

func TestStylistEditHistoryRejectsQueueReadFailure(t *testing.T) {
	st, q, items := editEnqueueFixture(t)
	for _, resultPresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "without result", true: "with result"}[resultPresent], func(t *testing.T) {
			failing := &historyEditErrorQueue{editTestQueue: q, err: errors.New("queue unavailable")}
			if !resultPresent {
				copyQueue := *q
				copyQueue.result = nil
				failing.editTestQueue = &copyQueue
			}
			_, err := enqueueStylistEdit(st, failing, items, false, editSourceJob, "change the pose")
			require.Error(t, err)
			assert.Len(t, failing.jobs, 1)
			assert.Equal(t, []int64{editSourceJob}, lastShootIDs(st))
		})
	}
}

func TestStylistEditHistoryRejectsMissingSource(t *testing.T) {
	for _, scenario := range []string{"missing params", "missing image", "not member"} {
		t.Run(scenario, func(t *testing.T) {
			st, q, items := editEnqueueFixture(t)
			q.result = nil
			switch scenario {
			case "missing params":
				delete(st.settings, stylistShootParamsKey)
			case "missing image":
				delete(st.items, editSourceItem)
				items.item = nil
				items.err = errors.New("deleted")
			case "not member":
				require.NoError(t, rememberShoot(st, nil))
			}
			_, err := enqueueStylistEdit(st, q, items, false, editSourceJob, "change the pose")
			require.Error(t, err)
			assert.Len(t, q.jobs, 1)
		})
	}
}

func TestStylistEditHistoryRepeatedVariantUsesImmediateParent(t *testing.T) {
	st, q, items := editEnqueueFixture(t)
	variantJob, err := enqueueStylistEdit(st, q, items, false, editSourceJob, "change the pose")
	require.NoError(t, err)
	const variantItem int64 = 601
	const variantSession int64 = 17
	st.items[variantItem] = true
	_, err = mergeLastShoot(st, []StylistFrame{{JobID: editSourceJob, Status: string(queue.StatusCompleted), ItemID: editSourceItem}, {JobID: variantJob, Status: string(queue.StatusCompleted), ItemID: variantItem}})
	require.NoError(t, err)
	q.result = nil
	items.item = &preset.SessionItem{ID: variantItem, SessionID: variantSession}
	nextJob, err := enqueueStylistEdit(st, q, items, false, variantJob, "blue dress")
	require.NoError(t, err)
	require.Len(t, q.jobs, 3)
	p := q.jobs[2].params
	assert.Equal(t, variantItem, p.SourceItemID)
	assert.Equal(t, variantSession, p.SourceSessionID)
	assert.Equal(t, "original-person", p.Person)
	assert.True(t, p.FaceSwap)
	assert.Equal(t, "original-face", p.FaceSwapName)
	assert.Equal(t, []int64{editSourceJob, variantJob, nextJob}, lastShootIDs(st))
}
