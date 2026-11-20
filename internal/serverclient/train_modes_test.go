package serverclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tmParts struct {
	fields map[string][]string
	files  map[string][][]byte
}

func tmParse(t *testing.T, req trRequest) tmParts {
	t.Helper()
	mt, params, err := mime.ParseMediaType(req.ctype)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mt)
	mr := multipart.NewReader(bytes.NewReader(req.body), params["boundary"])
	out := tmParts{fields: map[string][]string{}, files: map[string][][]byte{}}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FileName() != "" {
			out.files[part.FormName()] = append(out.files[part.FormName()], data)
		} else {
			out.fields[part.FormName()] = append(out.fields[part.FormName()], string(data))
		}
	}
	return out
}

func tmPhotos(t *testing.T, n, seedBase int) ([]string, [][]byte) {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, n)
	contents := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		data := trJPEG(t, seedBase+i)
		p := filepath.Join(dir, "img_"+string(rune('a'+i))+".jpg")
		require.NoError(t, os.WriteFile(p, data, 0o644))
		paths = append(paths, p)
		contents = append(contents, data)
	}
	return paths, contents
}

func TestSubmitTraining_ModeAndNewPhotos(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		mode      string
		oldN      int
		newN      int
		wantMode  []string
		modeField bool
	}{
		{name: "continue with new photos", mode: "continue", oldN: 6, newN: 4, wantMode: []string{"continue"}, modeField: true},
		{name: "continue without new photos", mode: "continue", oldN: 10, newN: 0, wantMode: []string{"continue"}, modeField: true},
		{name: "empty mode sends no mode field", mode: "", oldN: 10, newN: 0},
		{name: "empty mode with new photos", mode: "", oldN: 7, newN: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, http.StatusAccepted, trJobJSON)
			oldPaths, oldData := tmPhotos(t, tt.oldN, 0)
			var newPaths []string
			var newData [][]byte
			if tt.newN > 0 {
				newPaths, newData = tmPhotos(t, tt.newN, 100)
			}

			job, err := c.SubmitTraining(context.Background(), TrainRequest{
				Name:      "anna",
				Gender:    "woman",
				Base:      "Z-Image",
				Client:    "inst-1",
				ClientRef: "inst-1:train:anna",
				Photos:    oldPaths,
				NewPhotos: newPaths,
				Mode:      tt.mode,
			})

			require.NoError(t, err)
			require.NotNil(t, job)
			mp := tmParse(t, rec.last(t))
			got, ok := mp.fields["mode"]
			if tt.modeField {
				require.True(t, ok, "mode field missing")
				assert.Equal(t, tt.wantMode, got)
			} else {
				assert.False(t, ok, "mode field must be absent, got %v", got)
			}
			assert.ElementsMatch(t, oldData, mp.files["photos"])
			if tt.newN > 0 {
				assert.ElementsMatch(t, newData, mp.files["new_photos"])
			} else {
				assert.Empty(t, mp.files["new_photos"])
			}
			assert.Equal(t, []string{"anna"}, mp.fields["name"])
		})
	}
}

func TestRollbackPerson_Posts(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"person":{"name":"anna","gender":"woman","base":"Z-Image","lora":"anna","steps":2000,"mode":"new","rounds":1,"previous":false}}`)

	p, err := c.RollbackPerson(context.Background(), "anna")

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/api/train/people/anna/rollback", req.path)
	require.NotNil(t, p)
	assert.Equal(t, "anna", p.Name)
	assert.Equal(t, "anna", p.Lora)
	assert.Equal(t, 2000, p.Steps)
	assert.Equal(t, 1, p.Rounds)
	assert.False(t, p.Previous)
}

func TestRollbackPerson_APIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusConflict, "no_previous"},
		{http.StatusNotFound, "not_found"},
		{http.StatusConflict, "training_running"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, tt.status, `{"error":{"code":"`+tt.code+`","message":"nope"}}`)

			p, err := c.RollbackPerson(context.Background(), "anna")

			assert.Nil(t, p)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, tt.status, apiErr.Status)
			assert.Equal(t, tt.code, apiErr.Code)
		})
	}
}

func TestPeople_DecodesRoundFields(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, `{"people":[
		{"name":"anna","base":"Z-Image","lora":"anna","steps":3000,"mode":"continue","rounds":3,"photos_at":"2026-11-19T08:30:00Z","previous":true},
		{"name":"olga","base":"SDXL","lora":"olga","steps":2000}
	]}`)

	people, err := c.People(context.Background())

	require.NoError(t, err)
	require.Len(t, people, 2)
	anna := people[0]
	assert.Equal(t, "continue", anna.Mode)
	assert.Equal(t, 3, anna.Rounds)
	assert.True(t, anna.Previous)
	require.NotNil(t, anna.PhotosAt)
	assert.True(t, anna.PhotosAt.Equal(time.Date(2026, 11, 19, 8, 30, 0, 0, time.UTC)), "photos_at = %v", anna.PhotosAt)

	olga := people[1]
	assert.Empty(t, olga.Mode)
	assert.Zero(t, olga.Rounds)
	assert.False(t, olga.Previous)
	assert.Nil(t, olga.PhotosAt)
}

func TestTrainingJobs_DecodesModeFields(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, `{"jobs":[
		{"id":"j2","client":"inst-1","name":"anna","photos":20,"new_photos":8,"mode":"continue","steps":1200,"status":"queued","progress":{"step":0,"steps":1200},"error":null},
		{"id":"j3","client":"inst-1","name":"olga","photos":12,"status":"queued","progress":{"step":0,"steps":2000},"error":null}
	]}`)

	jobs, err := c.TrainingJobs(context.Background(), "inst-1")

	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, "continue", jobs[0].Mode)
	assert.Equal(t, 1200, jobs[0].Steps)
	assert.Equal(t, 8, jobs[0].NewPhotos)
	assert.Equal(t, 20, jobs[0].Photos)
	assert.Empty(t, jobs[1].Mode)
	assert.Zero(t, jobs[1].Steps)
	assert.Zero(t, jobs[1].NewPhotos)
}

func TestSubmitTraining_PhotosAt(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusAccepted, trJobJSON)
	paths, _ := tmPhotos(t, 10, 0)
	at := time.Date(2026, 11, 20, 13, 0, 0, 5000000, time.FixedZone("MSK", 3*3600))
	_, err := c.SubmitTraining(context.Background(), TrainRequest{Name: "anna", Gender: "woman", Client: "i", Photos: paths, Mode: "continue", PhotosAt: &at})
	require.NoError(t, err)
	mp := tmParse(t, rec.last(t))
	assert.Equal(t, []string{"2026-11-20T10:00:00.005Z"}, mp.fields["photos_at"])

	_, err = c.SubmitTraining(context.Background(), TrainRequest{Name: "anna", Gender: "woman", Client: "i", Photos: paths})
	require.NoError(t, err)
	_, ok := tmParse(t, rec.last(t)).fields["photos_at"]
	assert.False(t, ok)
}
