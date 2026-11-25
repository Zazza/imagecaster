package serverclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trRequest struct {
	method string
	path   string
	query  string
	ctype  string
	body   []byte
}

type trServer struct {
	mu   sync.Mutex
	reqs []trRequest
}

func (s *trServer) last(t *testing.T) trRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.NotEmpty(t, s.reqs, "server received no request")
	return s.reqs[len(s.reqs)-1]
}

func newTrServer(t *testing.T, status int, body string) (*Client, *trServer) {
	t.Helper()
	rec := &trServer{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, trRequest{
			method: r.Method,
			path:   r.URL.EscapedPath(),
			query:  r.URL.RawQuery,
			ctype:  r.Header.Get("Content-Type"),
			body:   b,
		})
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(ts.Close)
	c := NewClient()
	c.SetBaseURL(ts.URL)
	return c, rec
}

func trJPEG(t *testing.T, seed int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: uint8(seed * 7), G: uint8(x * 40), B: uint8(y * 40), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

func trPhotos(t *testing.T, n int) ([]string, map[string][]byte) {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, n)
	contents := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("photo_%02d.jpg", i)
		data := trJPEG(t, i)
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, data, 0o644))
		paths = append(paths, p)
		contents[name] = data
	}
	return paths, contents
}

type trMultipart struct {
	fields map[string][]string
	files  map[string][]byte
	order  []string
	other  []string
}

func trParseMultipart(t *testing.T, req trRequest) trMultipart {
	t.Helper()
	mt, params, err := mime.ParseMediaType(req.ctype)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mt)
	mr := multipart.NewReader(bytes.NewReader(req.body), params["boundary"])
	out := trMultipart{fields: map[string][]string{}, files: map[string][]byte{}}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		switch {
		case part.FormName() == "photos" && part.FileName() != "":
			out.files[part.FileName()] = data
			out.order = append(out.order, part.FileName())
		case part.FileName() != "":
			out.other = append(out.other, part.FormName())
		default:
			out.fields[part.FormName()] = append(out.fields[part.FormName()], string(data))
		}
	}
	return out
}

const trJobJSON = `{"id":"j1","client":"inst-1","client_ref":"inst-1:train:anna","name":"anna","trigger":"anna_woman","gender":"woman","photos":12,"status":"queued","progress":{"step":0,"steps":2000},"error":null,"created_at":"2026-11-16T10:00:00Z","started_at":null,"finished_at":null}`

func TestSubmitTraining_Multipart(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusAccepted, trJobJSON)
	paths, contents := trPhotos(t, 12)

	job, err := c.SubmitTraining(context.Background(), TrainRequest{
		Name:      "anna",
		Gender:    "woman",
		Client:    "inst-1",
		ClientRef: "inst-1:train:anna",
		Photos:    paths,
	})

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/api/train/jobs", req.path)
	mp := trParseMultipart(t, req)
	assert.Equal(t, []string{"anna"}, mp.fields["name"])
	assert.Equal(t, []string{"woman"}, mp.fields["gender"])
	assert.Equal(t, []string{"inst-1"}, mp.fields["client"])
	assert.Equal(t, []string{"inst-1:train:anna"}, mp.fields["client_ref"])
	assert.Empty(t, mp.other)
	require.Len(t, mp.order, 12)
	for name, data := range contents {
		got, ok := mp.files[name]
		require.True(t, ok, "missing photos part %q", name)
		assert.Equal(t, data, got, "bytes of %q", name)
	}

	require.NotNil(t, job)
	assert.Equal(t, "j1", job.ID)
	assert.Equal(t, "inst-1", job.Client)
	assert.Equal(t, "inst-1:train:anna", job.ClientRef)
	assert.Equal(t, "anna", job.Name)
	assert.Equal(t, "anna_woman", job.Trigger)
	assert.Equal(t, "woman", job.Gender)
	assert.Equal(t, 12, job.Photos)
	assert.Equal(t, "queued", job.Status)
	assert.Equal(t, TrainProgress{Step: 0, Steps: 2000}, job.Progress)
	assert.Nil(t, job.Error)
	require.NotNil(t, job.CreatedAt)
	assert.Equal(t, 2026, job.CreatedAt.Year())
	assert.Nil(t, job.StartedAt)
	assert.Nil(t, job.FinishedAt)
}

func TestSubmitTraining_Status200IsSuccess(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, trJobJSON)
	paths, _ := trPhotos(t, 10)

	job, err := c.SubmitTraining(context.Background(), TrainRequest{Name: "anna", Gender: "woman", Client: "inst-1", ClientRef: "inst-1:train:anna", Photos: paths})

	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "j1", job.ID)
}

func TestSubmitTraining_APIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
	}{
		{name: "name taken", status: http.StatusConflict, body: `{"error":{"code":"name_taken","message":"name anna is taken"}}`, wantCode: "name_taken"},
		{name: "trainer not installed", status: http.StatusServiceUnavailable, body: `{"error":{"code":"not_available","message":"trainer is not installed"}}`, wantCode: "not_available"},
		{name: "bad request", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_request","message":"need at least 10 photos"}}`, wantCode: "invalid_request"},
		{name: "too large", status: http.StatusRequestEntityTooLarge, body: `{"error":{"code":"too_large","message":"file too large"}}`, wantCode: "too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, tt.status, tt.body)
			paths, _ := trPhotos(t, 12)

			job, err := c.SubmitTraining(context.Background(), TrainRequest{Name: "anna", Gender: "woman", Client: "inst-1", Photos: paths})

			require.Error(t, err)
			assert.Nil(t, job)
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, tt.status, apiErr.Status)
			assert.Equal(t, tt.wantCode, apiErr.Code)
			assert.NotEmpty(t, apiErr.Message)
		})
	}
}

func TestSubmitTraining_MissingPhotoFile(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusAccepted, trJobJSON)
	paths, _ := trPhotos(t, 10)
	paths = append(paths, filepath.Join(t.TempDir(), "absent.jpg"))

	job, err := c.SubmitTraining(context.Background(), TrainRequest{Name: "anna", Gender: "woman", Client: "inst-1", Photos: paths})

	require.Error(t, err)
	assert.Nil(t, job)
}

func TestTrainingJobs_SendsClient(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"jobs":[`+trJobJSON+`]}`)

	jobs, err := c.TrainingJobs(context.Background(), "inst-1")

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodGet, req.method)
	assert.Equal(t, "/api/train/jobs", req.path)
	assert.Equal(t, "client=inst-1", req.query)
	require.Len(t, jobs, 1)
	assert.Equal(t, "j1", jobs[0].ID)
	assert.Equal(t, "anna_woman", jobs[0].Trigger)
}

func TestTrainingJobs_EscapesClient(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"jobs":[]}`)

	jobs, err := c.TrainingJobs(context.Background(), "a b&c")

	require.NoError(t, err)
	assert.Empty(t, jobs)
	req := rec.last(t)
	assert.Contains(t, []string{"client=a+b%26c", "client=a%20b%26c"}, req.query)
}

func TestTrainingJob_GetByID(t *testing.T) {
	t.Parallel()
	running := `{"id":"j1","client":"inst-1","name":"anna","trigger":"anna_woman","gender":"woman","photos":12,"status":"running","progress":{"step":5,"steps":10},"error":null}`
	c, rec := newTrServer(t, http.StatusOK, running)

	job, err := c.TrainingJob(context.Background(), "j1")

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodGet, req.method)
	assert.Equal(t, "/api/train/jobs/j1", req.path)
	require.NotNil(t, job)
	assert.Equal(t, "running", job.Status)
	assert.Equal(t, TrainProgress{Step: 5, Steps: 10}, job.Progress)
}

func TestTrainingJob_FailedHasError(t *testing.T) {
	t.Parallel()
	failed := `{"id":"j2","status":"failed","progress":{"step":3,"steps":10},"error":{"code":"train_failed","message":"out of memory"}}`
	c, _ := newTrServer(t, http.StatusOK, failed)

	job, err := c.TrainingJob(context.Background(), "j2")

	require.NoError(t, err)
	require.NotNil(t, job)
	require.NotNil(t, job.Error)
	assert.Equal(t, "train_failed", job.Error.Code)
	assert.Equal(t, "out of memory", job.Error.Message)
}

func TestCancelTraining_Posts(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"id":"j1","status":"cancelled","progress":{"step":5,"steps":10},"error":null}`)

	job, err := c.CancelTraining(context.Background(), "j1")

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/api/train/jobs/j1/cancel", req.path)
	require.NotNil(t, job)
	assert.Equal(t, "cancelled", job.Status)
}

func TestPeople_List(t *testing.T) {
	t.Parallel()
	c, rec := newTrServer(t, http.StatusOK, `{"people":[{"name":"anna","trigger":"anna_woman","gender":"woman","base":"Z-Image","lora":"anna","steps":2000,"created_at":"2026-11-16T11:00:00Z"}]}`)

	people, err := c.People(context.Background())

	require.NoError(t, err)
	req := rec.last(t)
	assert.Equal(t, http.MethodGet, req.method)
	assert.Equal(t, "/api/train/people", req.path)
	require.Len(t, people, 1)
	p := people[0]
	assert.Equal(t, "anna", p.Name)
	assert.Equal(t, "anna_woman", p.Trigger)
	assert.Equal(t, "woman", p.Gender)
	assert.Equal(t, "Z-Image", p.Base)
	assert.Equal(t, "anna", p.Lora)
	assert.Equal(t, 2000, p.Steps)
	require.NotNil(t, p.CreatedAt)
}

func TestPeople_Empty(t *testing.T) {
	t.Parallel()
	c, _ := newTrServer(t, http.StatusOK, `{"people":[]}`)

	people, err := c.People(context.Background())

	require.NoError(t, err)
	assert.Empty(t, people)
}

func TestDeletePerson(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
	}{
		{name: "deleted", status: http.StatusOK, body: `{}`},
		{name: "not found", status: http.StatusNotFound, body: `{"error":{"code":"not_found","message":"no such person"}}`, wantCode: "not_found"},
		{name: "busy", status: http.StatusConflict, body: `{"error":{"code":"busy","message":"training in progress"}}`, wantCode: "busy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTrServer(t, tt.status, tt.body)

			err := c.DeletePerson(context.Background(), "anna")

			req := rec.last(t)
			assert.Equal(t, http.MethodDelete, req.method)
			assert.Equal(t, "/api/train/people/anna", req.path)
			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}
			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, tt.status, apiErr.Status)
			assert.Equal(t, tt.wantCode, apiErr.Code)
		})
	}
}

func TestTrainEndpoints_NotFoundIsAPIError(t *testing.T) {
	t.Parallel()
	body := `{"error":{"code":"not_found","message":"no such job"}}`
	calls := map[string]func(c *Client) error{
		"TrainingJob": func(c *Client) error {
			_, err := c.TrainingJob(context.Background(), "nope")
			return err
		},
		"CancelTraining": func(c *Client) error {
			_, err := c.CancelTraining(context.Background(), "nope")
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTrServer(t, http.StatusNotFound, body)

			err := call(c)

			var apiErr *TrainAPIError
			require.True(t, errors.As(err, &apiErr), "want *TrainAPIError, got %T: %v", err, err)
			assert.Equal(t, http.StatusNotFound, apiErr.Status)
			assert.Equal(t, "not_found", apiErr.Code)
		})
	}
}

func TestTrainJob_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	var job TrainJob
	require.NoError(t, json.Unmarshal([]byte(trJobJSON), &job))
	out, err := json.Marshal(job)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	for _, k := range []string{"id", "client", "client_ref", "name", "trigger", "gender", "photos", "status", "progress"} {
		assert.Contains(t, m, k)
	}
}

func TestTrainingJobsRejectsUnknownQueueMetadata(t *testing.T) {
	for _, body := range []string{`{}`, `{"jobs":null}`} {
		t.Run(body, func(t *testing.T) {
			client, _ := newTrServer(t, http.StatusOK, body)
			jobs, err := client.TrainingJobs(context.Background(), "")
			require.Error(t, err, "missing queue metadata must not prove an idle server")
			assert.Nil(t, jobs)
		})
	}
	t.Run("explicit empty queue", func(t *testing.T) {
		client, _ := newTrServer(t, http.StatusOK, `{"jobs":[]}`)
		jobs, err := client.TrainingJobs(context.Background(), "")
		require.NoError(t, err)
		require.NotNil(t, jobs)
		assert.Empty(t, jobs)
	})
}
