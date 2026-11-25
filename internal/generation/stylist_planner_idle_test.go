package generation

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStylistPlannerIdleCacheRelease(t *testing.T) {
	const gib = 1 << 30
	const freeBefore = 3 * gib
	const freeAfter = 32 * gib
	cases := []struct {
		name                                                                                                                            string
		fits, missingMetadata, noCandidate, listError, probeError, busyState, noCheckpoint, jobError, trainingError, conflict, staysLow bool
		refreshRecovers                                                                                                                 bool
		appStatus, sdStatus, trainingStatus, configured                                                                                 string
		wantUnload, wantSuccess                                                                                                         bool
	}{
		{name: "idle cached SD releases capacity", wantUnload: true, wantSuccess: true},
		{name: "memory already fits", fits: true, wantSuccess: true},
		{name: "ordinary configured text", configured: "custom-text", wantSuccess: true},
		{name: "no eligible installed candidate", noCandidate: true},
		{name: "model list failure", listError: true},
		{name: "memory probe failure", probeError: true},
		{name: "missing training metadata", missingMetadata: true},
		{name: "App queued job", appStatus: "pending"},
		{name: "App running job", appStatus: "running"},
		{name: "server active state", busyState: true},
		{name: "no cached checkpoint", noCheckpoint: true},
		{name: "empty SD waits for fresh capacity", noCheckpoint: true, refreshRecovers: true, wantSuccess: true},
		{name: "empty SD with active state does not refresh", noCheckpoint: true, busyState: true},
		{name: "global queued SD job", sdStatus: sd.JobQueued},
		{name: "global running SD job", sdStatus: sd.JobRunning},
		{name: "unknown SD job status", sdStatus: "new_server_status"},
		{name: "global job probe failure", jobError: true},
		{name: "training active", trainingStatus: "running"},
		{name: "training queued", trainingStatus: "queued"},
		{name: "training probe failure", trainingError: true},
		{name: "atomic server conflict", conflict: true, wantUnload: true},
		{name: "release does not free enough memory", staysLow: true, wantUnload: true},
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
			var events []string
			unloads := 0
			memoryProbes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				route := strings.TrimPrefix(r.URL.Path, "/api/sd")
				events = append(events, r.Method+" "+route)
				w.Header().Set("Content-Type", "application/json")
				switch route {
				case "/memory":
					memoryProbes++
					if tc.probeError {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					free := float64(freeBefore)
					if tc.fits || (unloads > 0 && !tc.staysLow) || (tc.refreshRecovers && memoryProbes > 1) {
						free = freeAfter
					}
					require.NoError(t, json.NewEncoder(w).Encode(sd.MemoryStats{RAM: sd.MemoryPool{Free: freeAfter}, CUDA: &sd.CUDAMemory{System: sd.MemoryPool{Free: free}}}))
				case "/state":
					state := sd.ServerState{LoadedCheckpoint: "cached.safetensors"}
					if tc.busyState {
						state.RunningJob = "other-client-job"
					}
					if tc.noCheckpoint {
						state.LoadedCheckpoint = ""
					}
					require.NoError(t, json.NewEncoder(w).Encode(state))
				case "/jobs":
					assert.Empty(t, r.URL.Query().Get("client"), "all clients must be inspected")
					if tc.jobError {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					jobs := []sd.JobStatus{}
					if tc.sdStatus != "" {
						jobs = append(jobs, sd.JobStatus{ID: "other-job", Client: "other-client", Status: tc.sdStatus})
					}
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"jobs": jobs}))
				case "/api/train/jobs":
					assert.Empty(t, r.URL.Query().Get("client"), "training guard must cover all clients")
					if tc.trainingError {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					jobs := []map[string]string{}
					if tc.trainingStatus != "" {
						jobs = append(jobs, map[string]string{"id": "other-training", "status": tc.trainingStatus})
					}
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"jobs": jobs}))
				case "/unload":
					assert.Equal(t, http.MethodPost, r.Method)
					unloads++
					if tc.conflict {
						w.WriteHeader(http.StatusConflict)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected operation %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			if !tc.missingMetadata {
				require.NoError(t, db.SetSetting("server_url", server.URL))
			}
			client := &plannerModelLLM{installed: []llm.LLMModel{{ID: plannerText14B}}}
			if tc.noCandidate {
				client.installed = nil
			}
			if tc.listError {
				client.listErr = errors.New("list failed")
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
				assert.Empty(t, got)
			}
			if tc.noCheckpoint && !tc.busyState {
				assert.Equal(t, 2, memoryProbes, "empty idle SD must recheck capacity after refreshing telemetry")
				if !tc.wantSuccess {
					assert.ErrorIs(t, err, stylist.ErrPlannerCapacity)
					assert.NotContains(t, err.Error(), "не удалось освободить память SD")
				}
			}
			if tc.noCheckpoint && tc.busyState {
				assert.Equal(t, 1, memoryProbes, "active SD state must stop the retry")
			}
			if tc.wantUnload {
				assert.Equal(t, 1, unloads)
				if !tc.conflict {
					assert.Equal(t, "GET /memory", events[len(events)-1], "successful release must recheck memory")
				}
			} else {
				assert.Zero(t, unloads)
			}
			assert.Equal(t, configured, svc.getGenerateModel(), "automatic choice must not rewrite global settings")
		})
	}
}
