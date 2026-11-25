package generation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const captionDeferredCompletionPoll = 2

type captionReleaseLLM struct {
	plannerModelLLM
	serverURL      string
	loaded         []llm.LoadedModel
	unloaded       []string
	unloadErr      error
	loadedErr      error
	deferred       bool
	neverCompletes bool
	pendingPolls   int
}

func (c *captionReleaseLLM) IdleModelServerURL() string { return c.serverURL }
func (c *captionReleaseLLM) ListLoaded() ([]llm.LoadedModel, error) {
	if c.deferred && len(c.unloaded) > 0 {
		c.pendingPolls++
		if !c.neverCompletes && c.pendingPolls >= captionDeferredCompletionPoll {
			c.removeCaption()
		}
	}
	return c.loaded, c.loadedErr
}
func (c *captionReleaseLLM) UnloadModel(ctx context.Context, model string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("release must have a deadline")
	}
	c.unloaded = append(c.unloaded, model)
	if c.unloadErr != nil {
		return c.unloadErr
	}
	if !c.deferred {
		c.removeCaption()
	}
	return nil
}
func (c *captionReleaseLLM) removeCaption() {
	model := plannerCaptionModel
	retained := c.loaded[:0]
	for _, candidate := range c.loaded {
		if candidate.Name != model {
			retained = append(retained, candidate)
		}
	}
	c.loaded = retained
}

func TestStylistPlannerReleasesOnlyConfiguredCachedCaption(t *testing.T) {
	const freeBeforeBytes = 10872684544
	const freeAfterBytes = 32 << 30
	const captionVRAMBytes = 6190354567
	const foreignModel = "another-users-text-model"
	cases := []struct {
		name                                                                                                                 string
		configured                                                                                                           string
		noCaption, noCandidate, fit, urlMismatch, missingServer, busySD, probeError, loadedError, trainingError, unloadError bool
		sdJobsPayload, trainingJobsPayload                                                                                   string
		appStatus, sdStatus, trainStatus                                                                                     string
		wantUnload, wantSuccess                                                                                              bool
		deferred                                                                                                             bool
	}{
		{name: "real failure frees exact caption and keeps other LLM", wantUnload: true, wantSuccess: true},
		{name: "deferred active caption completes before retry", deferred: true, wantUnload: true, wantSuccess: true},
		{name: "no loaded caption"},
		{name: "foreign caption variant is not configured exact model", noCaption: true},
		{name: "capacity already fits", fit: true, wantSuccess: true},
		{name: "ordinary configured text", configured: "custom-text", wantSuccess: true},
		{name: "no eligible planner", noCandidate: true},
		{name: "LLM server mismatch", urlMismatch: true},
		{name: "missing training server metadata", missingServer: true},
		{name: "SD active", busySD: true},
		{name: "App pending", appStatus: "pending"},
		{name: "App running", appStatus: "running"},
		{name: "global SD queued", sdStatus: sd.JobQueued},
		{name: "global SD unknown", sdStatus: "unknown"},
		{name: "training active", trainStatus: "running"},
		{name: "training unknown", trainStatus: "unknown"},
		{name: "training probe fails", trainingError: true},
		{name: "unknown SD metadata missing jobs", sdJobsPayload: `{}`},
		{name: "unknown SD metadata null jobs", sdJobsPayload: `{"jobs":null}`},
		{name: "unknown training metadata missing jobs", trainingJobsPayload: `{}`},
		{name: "unknown training metadata null jobs", trainingJobsPayload: `{"jobs":null}`},
		{name: "memory probe fails", probeError: true},
		{name: "loaded model probe fails", loadedError: true},
		{name: "targeted unload fails", unloadError: true, wantUnload: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			configured := tc.configured
			if configured == "" {
				configured = plannerCaptionModel
			}
			require.NoError(t, db.SetSetting("llm_generate_model", configured))
			if tc.appStatus != "" {
				_, err := db.DB().Exec(`INSERT INTO job_queue (type,status) VALUES ('txt2img',?)`, tc.appStatus)
				require.NoError(t, err)
			}
			client := &captionReleaseLLM{plannerModelLLM: plannerModelLLM{installed: []llm.LLMModel{{ID: plannerText14B}}}, loaded: []llm.LoadedModel{{Name: foreignModel}}}
			if tc.name != "no loaded caption" {
				target := plannerCaptionModel
				if tc.noCaption {
					target += "-other"
				}
				client.loaded = append(client.loaded, llm.LoadedModel{Name: target, SizeVRAM: captionVRAMBytes})
			}
			client.deferred = tc.deferred
			if tc.noCandidate {
				client.installed = nil
			}
			if tc.loadedError {
				client.loadedErr = errors.New("loaded unavailable")
			}
			if tc.unloadError {
				client.unloadErr = errors.New("busy")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/sd/memory":
					if tc.probeError {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					free := float64(freeBeforeBytes)
					if tc.fit || (len(client.unloaded) > 0 && !tc.unloadError && (!client.deferred || client.pendingPolls >= captionDeferredCompletionPoll)) {
						free = freeAfterBytes
					}
					_ = json.NewEncoder(w).Encode(sd.MemoryStats{RAM: sd.MemoryPool{Free: freeAfterBytes}, CUDA: &sd.CUDAMemory{System: sd.MemoryPool{Free: free}}})
				case "/api/sd/state":
					state := sd.ServerState{}
					if tc.busySD {
						state.RunningJob = "foreign-job"
					}
					_ = json.NewEncoder(w).Encode(state)
				case "/api/sd/jobs":
					if tc.sdJobsPayload != "" {
						_, _ = w.Write([]byte(tc.sdJobsPayload))
						return
					}
					assert.Empty(t, r.URL.Query().Get("client"))
					jobs := []sd.JobStatus{}
					if tc.sdStatus != "" {
						jobs = append(jobs, sd.JobStatus{Status: tc.sdStatus})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs})
				case "/api/train/jobs":
					if tc.trainingJobsPayload != "" {
						_, _ = w.Write([]byte(tc.trainingJobsPayload))
						return
					}
					assert.Empty(t, r.URL.Query().Get("client"))
					if tc.trainingError {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					jobs := []map[string]string{}
					if tc.trainStatus != "" {
						jobs = append(jobs, map[string]string{"status": tc.trainStatus})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs})
				default:
					t.Errorf("unexpected operation %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			client.serverURL = server.URL
			if tc.urlMismatch {
				client.serverURL = "http://127.0.0.1:1"
			}
			if !tc.missingServer {
				require.NoError(t, db.SetSetting("server_url", server.URL))
			}
			svc := New(db, client, sd.New(server.URL+"/api/sd"), &config.Config{SDPromptModel: configured}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
			got, err := svc.StylistPlannerModel()
			if tc.wantSuccess {
				require.NoError(t, err)
				want := plannerText14B
				if tc.configured != "" {
					want = tc.configured
				}
				assert.Equal(t, want, got)
			} else {
				require.Error(t, err)
			}
			if tc.wantUnload {
				assert.Equal(t, []string{plannerCaptionModel}, client.unloaded)
			} else {
				assert.Empty(t, client.unloaded)
			}
			assert.Contains(t, client.loaded, llm.LoadedModel{Name: foreignModel})
			if tc.deferred {
				assert.GreaterOrEqual(t, client.pendingPolls, captionDeferredCompletionPoll, "retry must await target disappearance")
			}
			assert.Equal(t, configured, svc.getGenerateModel())
		})
	}
}

func TestPlannerCaptionReleaseDeadlineStopsDeferredWait(t *testing.T) {
	const releaseWait = 20 * time.Millisecond
	const serverURL = "http://127.0.0.1:8234"
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("llm_generate_model", plannerCaptionModel))
	client := &captionReleaseLLM{serverURL: serverURL, deferred: true, neverCompletes: true, loaded: []llm.LoadedModel{{Name: plannerCaptionModel}}}
	svc := New(db, client, &plannerCapacitySD{}, &config.Config{SDPromptModel: plannerCaptionModel}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
	ctx, cancel := context.WithTimeout(context.Background(), releaseWait)
	defer cancel()
	err := svc.releasePlannerCaption(ctx, serverURL)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, []string{plannerCaptionModel}, client.unloaded, "pending unload must not be resubmitted or replaced by unloading all models")
	assert.Contains(t, client.loaded, llm.LoadedModel{Name: plannerCaptionModel})
}
