package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

type ptTrainCall struct {
	folder, name, gender, base string
	hold                       bool
}

type ptFakeTrainer struct {
	mu       sync.Mutex
	bases    []string
	jobs     []serverclient.TrainJob
	trainJob *serverclient.TrainJob
	trainErr error
	trains   []ptTrainCall
	resumes  []string
}

func (f *ptFakeTrainer) Train(_ context.Context, folder, name, _, gender, base string, hold bool) (*serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trains = append(f.trains, ptTrainCall{folder, name, gender, base, hold})
	if f.trainErr != nil {
		return nil, f.trainErr
	}
	if f.trainJob != nil {
		return f.trainJob, nil
	}
	status := "queued"
	if hold {
		status = "paused"
	}
	return &serverclient.TrainJob{ID: "new-1", Name: name, Base: base, Status: status, Progress: serverclient.TrainProgress{Step: 0, Steps: 2000}}, nil
}

func (f *ptFakeTrainer) Jobs(_ context.Context) ([]serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs, nil
}

func (f *ptFakeTrainer) Resume(_ context.Context, id string) (*serverclient.TrainJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumes = append(f.resumes, id)
	for _, j := range f.jobs {
		if j.ID == id {
			j.Status = "queued"
			return &j, nil
		}
	}
	return nil, errors.New("not_found")
}

func (f *ptFakeTrainer) Bases(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bases, nil
}

func (f *ptFakeTrainer) trainCalls() []ptTrainCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ptTrainCall(nil), f.trains...)
}

func (f *ptFakeTrainer) resumeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.resumes...)
}

func ptServer(t *testing.T, tr PeopleTrainer) *Server {
	t.Helper()
	s := NewServer(Deps{OutDir: t.TempDir(), Cfg: serverModeCfg(t, "http://192.168.1.10:8080"), Trainer: tr})
	RegisterTools(s)
	return s
}

func ptJob(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	j, ok := m["job"].(map[string]any)
	require.True(t, ok, "job must be an object: %v", m)
	return j
}

func ptBases() []string { return []string{"Z-Image", "SDXL", "Flux.1 dev"} }

func TestPeopleTrain_StartDefaultRunsNow(t *testing.T) {
	t.Parallel()
	tr := &ptFakeTrainer{bases: ptBases()}
	s := ptServer(t, tr)
	folder := t.TempDir()

	m, out := styCall(t, s, "people_train", map[string]any{"folder": folder, "name": "anna", "gender": "woman", "base": "SDXL"})

	calls := tr.trainCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, ptTrainCall{folder: folder, name: "anna", gender: "woman", base: "SDXL", hold: false}, calls[0])
	j := ptJob(t, m)
	assert.Equal(t, "new-1", j["id"])
	assert.Equal(t, "anna", j["name"])
	assert.Equal(t, "SDXL", j["base"])
	assert.Equal(t, "queued", j["status"])
	assert.Equal(t, false, j["not_started"])
	assert.EqualValues(t, 0, j["step"])
	assert.EqualValues(t, 2000, j["steps"])
	assert.NotContains(t, out, folder)
	assert.NotContains(t, out, "base64")
}

func TestPeopleTrain_StartFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		start    bool
		wantHold bool
		status   string
		notStart bool
	}{
		{"start true runs now", true, false, "queued", false},
		{"start false holds", false, true, "paused", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tr := &ptFakeTrainer{bases: ptBases()}
			s := ptServer(t, tr)

			m, _ := styCall(t, s, "people_train", map[string]any{"folder": t.TempDir(), "name": "anna", "gender": "woman", "base": "Z-Image", "start": tt.start})

			calls := tr.trainCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, tt.wantHold, calls[0].hold)
			j := ptJob(t, m)
			assert.Equal(t, tt.status, j["status"])
			assert.Equal(t, tt.notStart, j["not_started"])
		})
	}
}

func TestPeopleTrain_BadBaseListsAvailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		base any
	}{
		{"empty base", ""},
		{"missing base", nil},
		{"unknown base", "Moss"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tr := &ptFakeTrainer{bases: ptBases()}
			s := ptServer(t, tr)
			args := map[string]any{"folder": t.TempDir(), "name": "anna", "gender": "woman"}
			if tt.base != nil {
				args["base"] = tt.base
			}

			_, err := styExec(s, "people_train", args)

			require.Error(t, err)
			for _, b := range ptBases() {
				assert.Contains(t, err.Error(), b)
			}
			assert.Empty(t, tr.trainCalls())
		})
	}
}

func TestPeopleTrain_TrainErrorReturned(t *testing.T) {
	t.Parallel()
	tr := &ptFakeTrainer{bases: ptBases(), trainErr: errors.New("server refused: name_taken")}
	s := ptServer(t, tr)

	_, err := styExec(s, "people_train", map[string]any{"folder": t.TempDir(), "name": "anna", "gender": "woman", "base": "Z-Image"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "name_taken")
}

func TestPeopleTrainJobs_ListsJobsAndBases(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	tr := &ptFakeTrainer{
		bases: ptBases(),
		jobs: []serverclient.TrainJob{
			{ID: "j1", Name: "anna", Base: "Z-Image", Status: "paused", Progress: serverclient.TrainProgress{Step: 0, Steps: 2000}},
			{ID: "j2", Name: "boris", Base: "SDXL", Status: "paused", StartedAt: &started, Progress: serverclient.TrainProgress{Step: 750, Steps: 2000}},
			{ID: "j3", Name: "vera", Base: "Z-Image", Status: "running", StartedAt: &started, Progress: serverclient.TrainProgress{Step: 100, Steps: 2000}},
			{ID: "j4", Name: "gleb", Base: "Z-Image", Status: "queued", Progress: serverclient.TrainProgress{Step: 0, Steps: 2000}},
		},
	}
	s := ptServer(t, tr)

	m, _ := styCall(t, s, "people_train_jobs", map[string]any{})

	raw, ok := m["jobs"].([]any)
	require.True(t, ok, "jobs must be an array: %v", m)
	require.Len(t, raw, 4)
	byName := map[string]map[string]any{}
	for _, r := range raw {
		j, ok := r.(map[string]any)
		require.True(t, ok)
		byName[j["name"].(string)] = j
	}
	assert.Equal(t, true, byName["anna"]["not_started"])
	assert.Equal(t, false, byName["boris"]["not_started"])
	assert.Equal(t, false, byName["vera"]["not_started"])
	assert.Equal(t, false, byName["gleb"]["not_started"])
	assert.Equal(t, "j2", byName["boris"]["id"])
	assert.Equal(t, "SDXL", byName["boris"]["base"])
	assert.Equal(t, "paused", byName["boris"]["status"])
	assert.EqualValues(t, 750, byName["boris"]["step"])
	assert.EqualValues(t, 2000, byName["boris"]["steps"])

	bases, ok := m["bases"].([]any)
	require.True(t, ok, "bases must be an array: %v", m)
	got := make([]string, 0, len(bases))
	for _, b := range bases {
		got = append(got, b.(string))
	}
	assert.ElementsMatch(t, ptBases(), got)
}

func TestPeopleTrainJobs_Empty(t *testing.T) {
	t.Parallel()
	tr := &ptFakeTrainer{bases: ptBases()}
	s := ptServer(t, tr)

	out, err := styExec(s, "people_train_jobs", map[string]any{})

	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	jobs, ok := m["jobs"].([]any)
	require.True(t, ok, "jobs must be an array, not null: %s", out)
	assert.Empty(t, jobs)
}

func TestPeopleTrainStart_ResumesPausedByName(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	tr := &ptFakeTrainer{
		bases: ptBases(),
		jobs: []serverclient.TrainJob{
			{ID: "j0", Name: "anna", Base: "Z-Image", Status: "failed", StartedAt: &started},
			{ID: "j1", Name: "anna", Base: "Z-Image", Status: "paused", Progress: serverclient.TrainProgress{Step: 0, Steps: 2000}},
			{ID: "j2", Name: "boris", Base: "SDXL", Status: "paused"},
		},
	}
	s := ptServer(t, tr)

	m, _ := styCall(t, s, "people_train_start", map[string]any{"name": "anna"})

	assert.Equal(t, []string{"j1"}, tr.resumeCalls())
	j := ptJob(t, m)
	assert.Equal(t, "j1", j["id"])
	assert.Equal(t, "queued", j["status"])
}

func TestPeopleTrainStart_NoPausedJob(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		jobs []serverclient.TrainJob
	}{
		{"no jobs", nil},
		{"same name but running", []serverclient.TrainJob{{ID: "j1", Name: "anna", Status: "running", StartedAt: &started}}},
		{"paused but other name", []serverclient.TrainJob{{ID: "j2", Name: "boris", Status: "paused"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tr := &ptFakeTrainer{bases: ptBases(), jobs: tt.jobs}
			s := ptServer(t, tr)

			_, err := styExec(s, "people_train_start", map[string]any{"name": "anna"})

			require.Error(t, err)
			assert.Empty(t, tr.resumeCalls())
		})
	}
}

func TestPeopleTrainTools_TrainerNil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"people_train", map[string]any{"folder": "/nonexistent", "name": "anna", "gender": "woman", "base": "Z-Image"}},
		{"people_train_jobs", map[string]any{}},
		{"people_train_start", map[string]any{"name": "anna"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			t.Parallel()
			s := ptServer(t, nil)

			_, err := styExec(s, tt.tool, tt.args)

			require.Error(t, err)
		})
	}
}
