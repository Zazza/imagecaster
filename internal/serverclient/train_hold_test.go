package serverclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const trHoldJobJSON = `{"id":"j9","client":"inst-1","client_ref":"inst-1:train:anna","name":"anna","gender":"woman","photos":12,"status":"paused","progress":{"step":0,"steps":2000},"error":null,"started_at":null,"finished_at":null}`

func TestSubmitTraining_Hold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		hold     bool
		wantSent bool
	}{
		{"hold sends start=false", true, true},
		{"no hold sends no start field", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, http.StatusAccepted, trHoldJobJSON)
			paths, _ := trPhotos(t, 10)

			job, err := c.SubmitTraining(context.Background(), TrainRequest{
				Name:      "anna",
				Gender:    "woman",
				Base:      "zimage",
				Client:    "inst-1",
				ClientRef: "inst-1:train:anna",
				Photos:    paths,
				Hold:      tt.hold,
			})

			require.NoError(t, err)
			require.NotNil(t, job)
			mp := trParseMultipart(t, rec.last(t))
			got, ok := mp.fields["start"]
			if tt.wantSent {
				require.True(t, ok, "start field missing")
				assert.Equal(t, []string{"false"}, got)
			} else {
				assert.False(t, ok, "start field must be absent, got %v", got)
			}
			assert.Equal(t, []string{"anna"}, mp.fields["name"])
			assert.Len(t, mp.order, 10)
		})
	}
}

func TestSubmitTraining_HoldReturnsPausedNotStarted(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusAccepted, trHoldJobJSON)
	paths, _ := trPhotos(t, 10)

	job, err := c.SubmitTraining(context.Background(), TrainRequest{
		Name: "anna", Gender: "woman", Client: "inst-1", ClientRef: "inst-1:train:anna", Photos: paths, Hold: true,
	})

	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "paused", job.Status)
	assert.Nil(t, job.StartedAt)
	assert.Equal(t, TrainProgress{Step: 0, Steps: 2000}, job.Progress)
}
