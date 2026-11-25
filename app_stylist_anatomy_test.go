package main

import (
	"encoding/json"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStylistAnatomyWarningSurvivesQueueCleanupAndRestart(t *testing.T) {
	const jobID int64 = 7
	const itemID int64 = 107
	st := newLsStore(itemID)
	require.NoError(t, rememberShoot(st, []int64{jobID}))
	initial, err := mergeLastShoot(st, []StylistFrame{{JobID: jobID, Status: "completed", ItemID: itemID, AnatomyWarning: "garment_defect"}})
	require.NoError(t, err)
	require.Len(t, initial, 1)
	restarted := newLsStore(itemID)
	for key, value := range st.settings {
		restarted.settings[key] = value
	}
	restored, err := mergeLastShoot(restarted, []StylistFrame{{JobID: jobID, Status: stylistFrameGone}})
	require.NoError(t, err)
	require.Len(t, restored, 1)
	assert.Equal(t, "garment_defect", restored[0].AnatomyWarning)
	raw, err := json.Marshal(restored[0])
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"anatomy_warning":"garment_defect"`)
}

func TestStylistAnatomyWarningCrossesQueueResultBoundary(t *testing.T) {
	db, err := preset.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	store := queue.NewStore(db.DB())
	job, err := store.CreateLabeled(queue.JobStylistFrame, `{}`, "stylist", "frame")
	require.NoError(t, err)
	frame, err := json.Marshal(mcp.ShootFrameResult{ID: 107, AnatomyWarning: "base_defect"})
	require.NoError(t, err)
	result, err := json.Marshal(queue.JobResult{Frame: frame})
	require.NoError(t, err)
	require.NoError(t, store.UpdateCompleted(job, string(result)))
	frames, err := stylistFrames(store, []int64{job})
	require.NoError(t, err)
	require.Len(t, frames, 1)
	assert.Equal(t, "base_defect", frames[0].AnatomyWarning)
}
