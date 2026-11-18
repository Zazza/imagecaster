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

func TestPauseTraining_Posts(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"id":"j1","status":"paused","progress":{"step":750,"steps":2000},"error":null,"finished_at":null}`)

	job, err := c.PauseTraining(context.Background(), "j1")

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/api/train/jobs/j1/pause", req.path)
	require.NotNil(t, job)
	assert.Equal(t, "paused", job.Status)
	assert.Equal(t, TrainProgress{Step: 750, Steps: 2000}, job.Progress)
	assert.Nil(t, job.FinishedAt)
}

func TestResumeTraining_Posts(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"id":"j1","status":"queued","progress":{"step":750,"steps":2000},"error":null}`)

	job, err := c.ResumeTraining(context.Background(), "j1")

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/api/train/jobs/j1/resume", req.path)
	require.NotNil(t, job)
	assert.Equal(t, "queued", job.Status)
}

func TestPauseResume_EscapesID(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"id":"a/b","status":"paused"}`)

	_, err := c.PauseTraining(context.Background(), "a/b")
	require.NoError(t, err)
	assert.Equal(t, "/api/train/jobs/a%2Fb/pause", rec.last(t).path)

	_, err = c.ResumeTraining(context.Background(), "a/b")
	require.NoError(t, err)
	assert.Equal(t, "/api/train/jobs/a%2Fb/resume", rec.last(t).path)
}

func TestPauseResume_APIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		code   string
		call   func(c *Client) (*TrainJob, error)
	}{
		{"pause not running", http.StatusConflict, "not_running", func(c *Client) (*TrainJob, error) {
			return c.PauseTraining(context.Background(), "j1")
		}},
		{"pause not found", http.StatusNotFound, "not_found", func(c *Client) (*TrainJob, error) {
			return c.PauseTraining(context.Background(), "j1")
		}},
		{"resume not paused", http.StatusConflict, "not_paused", func(c *Client) (*TrainJob, error) {
			return c.ResumeTraining(context.Background(), "j1")
		}},
		{"resume not found", http.StatusNotFound, "not_found", func(c *Client) (*TrainJob, error) {
			return c.ResumeTraining(context.Background(), "j1")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, tt.status, `{"error":{"code":"`+tt.code+`","message":"nope"}}`)

			job, err := tt.call(c)

			assert.Nil(t, job)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, tt.status, apiErr.Status)
			assert.Equal(t, tt.code, apiErr.Code)
		})
	}
}

func TestRenamePerson_PostsJSON(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"person":{"name":"maria","trigger":"anna_woman","gender":"woman","base":"Z-Image","lora":"maria","steps":2000}}`)

	p, err := c.RenamePerson(context.Background(), "anna", "maria")

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/api/train/people/anna/rename", req.path)
	assert.Contains(t, req.ctype, "application/json")
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.body, &body), string(req.body))
	assert.Equal(t, map[string]any{"name": "maria"}, body)
	require.NotNil(t, p)
	assert.Equal(t, "maria", p.Name)
	assert.Equal(t, "maria", p.Lora)
	assert.Equal(t, "woman", p.Gender)
	assert.Equal(t, 2000, p.Steps)
}

func TestRenamePerson_APIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusConflict, "name_taken"},
		{http.StatusBadRequest, "invalid_request"},
		{http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, tt.status, `{"error":{"code":"`+tt.code+`","message":"nope"}}`)

			p, err := c.RenamePerson(context.Background(), "anna", "maria")

			assert.Nil(t, p)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, tt.status, apiErr.Status)
			assert.Equal(t, tt.code, apiErr.Code)
		})
	}
}
