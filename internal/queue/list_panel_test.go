package queue

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertJobAt(t *testing.T, db *sql.DB, status JobStatus, createdAt time.Time) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO job_queue (type, status, params, source, created_at) VALUES (?, ?, '{}', 'test', ?)`,
		JobTxt2Img, status, createdAt.UTC().Format("2006-01-02 15:04:05"),
	)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

type seededJob struct {
	id     int64
	status JobStatus
	at     time.Time
}

func TestStore_ListForPanel(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		statuses     func(i int) JobStatus
		count        int
		limit        int
		wantActive   int
		wantFinished int
	}{
		{
			name: "Q1 150 pending and 150 completed",
			statuses: func(i int) JobStatus {
				if i%2 == 0 {
					return StatusPending
				}
				return StatusCompleted
			},
			count:        300,
			limit:        100,
			wantActive:   150,
			wantFinished: 100,
		},
		{
			name: "Q2 running and paused without finished",
			statuses: func(i int) JobStatus {
				if i == 0 {
					return StatusRunning
				}
				return StatusPaused
			},
			count:        3,
			limit:        100,
			wantActive:   3,
			wantFinished: 0,
		},
		{
			name: "Q3 only failed and cancelled",
			statuses: func(i int) JobStatus {
				if i%3 == 0 {
					return StatusCancelled
				}
				return StatusFailed
			},
			count:        120,
			limit:        100,
			wantActive:   0,
			wantFinished: 100,
		},
		{
			name:         "empty queue",
			statuses:     func(i int) JobStatus { return StatusPending },
			count:        0,
			limit:        100,
			wantActive:   0,
			wantFinished: 0,
		},
		{
			name: "finished below limit all returned",
			statuses: func(i int) JobStatus {
				if i%2 == 0 {
					return StatusRunning
				}
				return StatusCompleted
			},
			count:        10,
			limit:        100,
			wantActive:   5,
			wantFinished: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			defer db.Close()
			store := NewStore(db)

			var seeded []seededJob
			for i := 0; i < tt.count; i++ {
				st := tt.statuses(i)
				at := base.Add(time.Duration(i) * time.Minute)
				seeded = append(seeded, seededJob{id: insertJobAt(t, db, st, at), status: st, at: at})
			}

			jobs, err := store.ListForPanel(tt.limit)
			require.NoError(t, err)

			wantIDs := map[int64]bool{}
			finishedSeen := 0
			for i := len(seeded) - 1; i >= 0; i-- {
				s := seeded[i]
				if isActiveStatus(s.status) {
					wantIDs[s.id] = true
					continue
				}
				if finishedSeen < tt.limit {
					wantIDs[s.id] = true
					finishedSeen++
				}
			}

			gotActive, gotFinished := 0, 0
			gotIDs := map[int64]bool{}
			for _, j := range jobs {
				assert.False(t, gotIDs[j.ID], "duplicate job %d", j.ID)
				gotIDs[j.ID] = true
				if isActiveStatus(j.Status) {
					gotActive++
				} else {
					gotFinished++
				}
			}
			assert.Equal(t, tt.wantActive, gotActive, "active count")
			assert.Equal(t, tt.wantFinished, gotFinished, "finished count")
			assert.Equal(t, wantIDs, gotIDs, "returned set")

			for i := 1; i < len(jobs); i++ {
				assert.GreaterOrEqual(t, jobs[i-1].CreatedAt, jobs[i].CreatedAt,
					fmt.Sprintf("order at %d: %s before %s", i, jobs[i-1].CreatedAt, jobs[i].CreatedAt))
			}
		})
	}
}

func TestStore_ListForPanel_ActiveNotCountedAgainstLimit(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := NewStore(db)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		insertJobAt(t, db, StatusCompleted, base.Add(time.Duration(i)*time.Minute))
	}
	for i := 0; i < 5; i++ {
		insertJobAt(t, db, StatusPending, base.Add(time.Duration(10+i)*time.Minute))
	}

	jobs, err := store.ListForPanel(2)
	require.NoError(t, err)
	require.Len(t, jobs, 7)
	for _, j := range jobs[:5] {
		assert.Equal(t, StatusPending, j.Status)
	}
	assert.Equal(t, StatusCompleted, jobs[5].Status)
	assert.Equal(t, StatusCompleted, jobs[6].Status)
	assert.Equal(t, base.Add(4*time.Minute).Format("2006-01-02 15:04:05"), normalizeTS(jobs[5].CreatedAt))
	assert.Equal(t, base.Add(3*time.Minute).Format("2006-01-02 15:04:05"), normalizeTS(jobs[6].CreatedAt))
}

func normalizeTS(s string) string {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z"} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts.UTC().Format("2006-01-02 15:04:05")
		}
	}
	return s
}

func isActiveStatus(s JobStatus) bool {
	return s == StatusPending || s == StatusRunning || s == StatusPaused
}
