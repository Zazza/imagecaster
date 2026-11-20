package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/queue"
)

func shootWithParams(t *testing.T, st *lsStore, garment string) *spQueue {
	t.Helper()
	q := &spQueue{}
	rec := &recordingQueue{q: q}
	seed := int64(10)
	for i, label := range []string{"Стилист: Анна, Анфас (1 из 2)", "Стилист: Анна, Профиль (2 из 2)"} {
		p := mcp.ShootFrameParams{Person: "anna_z", Prompt: "frame", Outfit: "a red dress", Index: i + 1, Total: 2, Seed: &seed, ToFolder: true}
		if i == 0 {
			p.GarmentBase64 = garment
		}
		_, err := rec.EnqueueLabeled(queue.JobStylistFrame, p, stylistPageSource, label)
		require.NoError(t, err)
	}
	require.NoError(t, rememberShoot(st, []int64{101, 102}))
	require.NoError(t, saveShootParams(st, rec.frames))
	return q
}

func TestReshoot_SameFrameNewSeedSamePlace(t *testing.T) {
	st := newLsStore()
	shootWithParams(t, st, "GARMENT")
	raw := st.settings[stylistShootParamsKey]
	assert.Equal(t, 1, strings.Count(raw, "GARMENT"), "garment is stored once per shoot")

	q := &spQueue{jobs: make([]spJob, 2)}
	id, err := reshootFrame(st, q, false, 101, "")
	require.NoError(t, err)
	require.Len(t, q.jobs, 3)
	j := q.jobs[2]
	assert.Equal(t, queue.JobStylistFrame, j.jobType)
	assert.Equal(t, stylistPageSource, j.source)
	assert.Equal(t, "Стилист: Анна, Анфас (1 из 2)", j.label)
	assert.Equal(t, "GARMENT", j.params.GarmentBase64)
	assert.Equal(t, "frame", j.params.Prompt)
	assert.True(t, j.params.ToFolder)
	require.NotNil(t, j.params.Seed)
	assert.NotEqual(t, int64(10), *j.params.Seed)
	assert.Equal(t, []int64{id, 102}, lastShootIDs(st), "new job takes the old frame's place")

	_, err = reshootFrame(st, q, false, 102, "")
	require.NoError(t, err)
	assert.Empty(t, q.jobs[3].params.GarmentBase64, "frame without garment stays without it")

	_, err = reshootFrame(st, q, false, id, "")
	require.NoError(t, err, "a reshot frame can be reshot again")
	assert.Equal(t, "GARMENT", q.jobs[4].params.GarmentBase64)
	assert.Equal(t, []int64{105, 104}, lastShootIDs(st))
}

func TestReshoot_Refusals(t *testing.T) {
	st := newLsStore()
	shootWithParams(t, st, "")
	_, err := reshootFrame(st, &spQueue{}, false, 999, "")
	assert.ErrorIs(t, err, errStylistReshootGone)
	_, err = reshootFrame(st, &spQueue{}, true, 101, "")
	assert.ErrorIs(t, err, errStylistPageNotLocal)

	q := &spQueue{failAt: 1}
	_, err = reshootFrame(st, q, false, 101, "")
	require.Error(t, err)
	assert.Equal(t, []int64{101, 102}, lastShootIDs(st), "failed enqueue keeps the strip")

	st.settings[stylistShootParamsKey] = "{broken"
	_, err = reshootFrame(st, &spQueue{}, false, 101, "")
	require.Error(t, err)
}

func TestStylistPlan_SaveLoad(t *testing.T) {
	st := newLsStore()
	plan := `{"form":{"person":"Анна"},"series":[]}`
	require.NoError(t, saveStylistPlan(st, plan))
	got, err := loadStylistPlan(st)
	require.NoError(t, err)
	assert.Equal(t, plan, got)
	assert.Error(t, saveStylistPlan(st, "{not json"))
	assert.Error(t, saveStylistPlan(st, `"`+strings.Repeat("a", stylistPlanMax)+`"`))
	require.NoError(t, saveStylistPlan(st, ""))
	got, err = loadStylistPlan(st)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStylistGarment_SaveShrinksAndClears(t *testing.T) {
	st := newLsStore()
	require.NoError(t, saveStylistGarment(st, ""))
	assert.Empty(t, st.settings[stylistGarmentKey])
	assert.Error(t, saveStylistGarment(st, "not base64 !!"))
}

func TestReshoot_Size(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		size          string
		wantW, wantH  int
	}{
		{"empty keeps size", 640, 960, "", 640, 960},
		{"draft portrait to normal", 640, 960, "normal", 1024, 1536},
		{"draft landscape to large keeps orientation", 960, 640, "large", 1824, 1216},
		{"unknown size means normal", 1216, 1824, "huge", 1024, 1536},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newLsStore()
			q := &spQueue{}
			rec := &recordingQueue{q: q}
			p := mcp.ShootFrameParams{Person: "anna_z", Prompt: "frame", Width: tt.width, Height: tt.height}
			_, err := rec.EnqueueLabeled(queue.JobStylistFrame, p, stylistPageSource, "Стилист: Анна")
			require.NoError(t, err)
			require.NoError(t, rememberShoot(st, []int64{101}))
			require.NoError(t, saveShootParams(st, rec.frames))

			q2 := &spQueue{jobs: make([]spJob, 1)}
			id, err := reshootFrame(st, q2, false, 101, tt.size)
			require.NoError(t, err)
			got := q2.jobs[1].params
			assert.Equal(t, [2]int{tt.wantW, tt.wantH}, [2]int{got.Width, got.Height})

			q3 := &spQueue{jobs: make([]spJob, 2)}
			_, err = reshootFrame(st, q3, false, id, "")
			require.NoError(t, err)
			assert.Equal(t, [2]int{tt.wantW, tt.wantH}, [2]int{q3.jobs[2].params.Width, q3.jobs[2].params.Height}, "new size is remembered for the next reshoot")
		})
	}
}
