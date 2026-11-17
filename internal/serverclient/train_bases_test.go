package serverclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrainingBases_List(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want []string
	}{
		{name: "z-image and moss", body: `{"bases":["Z-Image","Moss-Image"]}`, want: []string{"Z-Image", "Moss-Image"}},
		{name: "only z-image", body: `{"bases":["Z-Image"]}`, want: []string{"Z-Image"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, http.StatusOK, tt.body)

			bases, err := c.TrainingBases(context.Background())

			require.NoError(t, err)
			req := rec.last(t)
			assert.Equal(t, http.MethodGet, req.method)
			assert.Equal(t, "/api/train/bases", req.path)
			assert.Equal(t, tt.want, bases)
		})
	}
}

func TestTrainingBases_Empty(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, `{"bases":[]}`)

	bases, err := c.TrainingBases(context.Background())

	require.NoError(t, err)
	assert.Empty(t, bases)
}

func TestTrainingBases_APIError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
	}{
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":{"code":"forbidden","message":"cross-origin request"}}`, wantCode: "forbidden"},
		{name: "trainer not installed", status: http.StatusServiceUnavailable, body: `{"error":{"code":"not_available","message":"trainer is not installed"}}`, wantCode: "not_available"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, tt.status, tt.body)

			bases, err := c.TrainingBases(context.Background())

			require.Error(t, err)
			assert.Empty(t, bases)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, tt.status, apiErr.Status)
			assert.Equal(t, tt.wantCode, apiErr.Code)
		})
	}
}

func TestSubmitTraining_SendsBase(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"Moss-Image", "Z-Image"} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, http.StatusAccepted, trJobJSON)
			paths, _ := trPhotos(t, 10)

			_, err := c.SubmitTraining(context.Background(), TrainRequest{
				Name:      "anna",
				Gender:    "woman",
				Client:    "inst-1",
				ClientRef: "inst-1:train:anna",
				Base:      base,
				Photos:    paths,
			})

			require.NoError(t, err)
			mp := trParseMultipart(t, rec.last(t))
			assert.Equal(t, []string{base}, mp.fields["base"])
			assert.Equal(t, []string{"anna"}, mp.fields["name"])
			assert.Len(t, mp.order, 10)
		})
	}
}

func TestSubmitTraining_EmptyBaseNotSentAsValue(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusAccepted, trJobJSON)
	paths, _ := trPhotos(t, 10)

	_, err := c.SubmitTraining(context.Background(), TrainRequest{Name: "anna", Gender: "woman", Client: "inst-1", ClientRef: "inst-1:train:anna", Photos: paths})

	require.NoError(t, err)
	mp := trParseMultipart(t, rec.last(t))
	got, present := mp.fields["base"]
	if present {
		assert.Equal(t, []string{""}, got)
	}
}

func TestSubmitTraining_BaseRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "base unavailable", body: `{"error":{"code":"base_unavailable","message":"Moss-Image files are missing on the server"}}`, wantCode: "base_unavailable"},
		{name: "unknown base", body: `{"error":{"code":"invalid_request","message":"unknown base SDXL"}}`, wantCode: "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, http.StatusBadRequest, tt.body)
			paths, _ := trPhotos(t, 10)

			job, err := c.SubmitTraining(context.Background(), TrainRequest{Name: "anna", Gender: "woman", Client: "inst-1", Base: "Moss-Image", Photos: paths})

			require.Error(t, err)
			assert.Nil(t, job)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			assert.Equal(t, tt.wantCode, apiErr.Code)
		})
	}
}

func TestTrainJob_BaseFromJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "moss", body: `{"id":"j1","name":"anna","status":"queued","base":"Moss-Image","progress":{"step":0,"steps":2000}}`, want: "Moss-Image"},
		{name: "z-image", body: `{"id":"j1","name":"anna","status":"queued","base":"Z-Image","progress":{"step":0,"steps":2000}}`, want: "Z-Image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, http.StatusOK, tt.body)

			job, err := c.TrainingJob(context.Background(), "j1")

			require.NoError(t, err)
			require.NotNil(t, job)
			assert.Equal(t, tt.want, job.Base)
		})
	}
}

func TestTrainingJobs_BaseFromJSON(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, `{"jobs":[{"id":"j1","status":"running","base":"Moss-Image"},{"id":"j2","status":"completed","base":"Z-Image"}]}`)

	jobs, err := c.TrainingJobs(context.Background(), "inst-1")

	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, "Moss-Image", jobs[0].Base)
	assert.Equal(t, "Z-Image", jobs[1].Base)
}

func TestTrainJob_BaseJSONKey(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(TrainJob{ID: "j1", Base: "Moss-Image"})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, "Moss-Image", m["base"])
}

func TestPeople_MossBase(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, `{"people":[{"name":"anna","trigger":"anna_woman","gender":"woman","base":"Moss-Image","lora":"anna","steps":2000},{"name":"zkv","trigger":"zkv_man","gender":"man","base":"Z-Image","lora":"zkv","steps":1200}]}`)

	people, err := c.People(context.Background())

	require.NoError(t, err)
	require.Len(t, people, 2)
	assert.Equal(t, "Moss-Image", people[0].Base)
	assert.Equal(t, "Z-Image", people[1].Base)
}
