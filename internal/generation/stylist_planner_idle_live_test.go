package generation

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStylistPlannerIdleCacheLive(t *testing.T) {
	captionCacheMode := os.Getenv("IMAGECASTER_PLANNER_CAPTION_CACHE_LIVE") == "1"
	emptyCacheMode := os.Getenv("IMAGECASTER_PLANNER_EMPTY_CACHE_LIVE") == "1"
	if os.Getenv("IMAGECASTER_PLANNER_MEMORY_LIVE") != "1" && !emptyCacheMode && !captionCacheMode {
		t.Skip("opt-in live planner capacity release")
	}
	source, err := sql.Open("sqlite", "file:../../build/bin/data/presets.db?mode=ro")
	require.NoError(t, err)
	defer source.Close()
	var baseURL, original string
	require.NoError(t, source.QueryRow("SELECT value FROM settings WHERE key='server_url'").Scan(&baseURL))
	require.NoError(t, source.QueryRow("SELECT value FROM settings WHERE key='llm_generate_model'").Scan(&original))
	var active int
	require.NoError(t, source.QueryRow("SELECT COUNT(*) FROM job_queue WHERE status IN ('pending','running')").Scan(&active))
	require.Zero(t, active, "live App queue must be idle")
	require.True(t, lanonly.IsLocalURL(baseURL) && !lanonly.ViaProxy(baseURL), "only direct local server is allowed")
	server := serverclient.NewClient()
	server.SetBaseURL(baseURL)
	sdURL, llmURL := server.ProxyURLs()
	client := sd.New(sdURL)
	jobs, err := client.ListJobs(context.Background(), "")
	require.NoError(t, err)
	for _, job := range jobs {
		require.True(t, job.Terminal(), "SD job %s active", job.ID)
	}
	training, err := server.TrainingJobs(context.Background(), "")
	require.NoError(t, err)
	for _, job := range training {
		require.Contains(t, []string{"completed", "failed", "cancelled", "paused"}, job.Status, "training %s active", job.ID)
	}
	before, err := client.State()
	require.NoError(t, err)
	if emptyCacheMode || captionCacheMode {
		require.Empty(t, before.LoadedCheckpoint, "empty-cache mode requires SD cache already empty")
	} else {
		require.NotEmpty(t, before.LoadedCheckpoint, "prepare cached SD generation before live test")
	}
	require.Empty(t, before.RunningJob)
	db := openTestDB(t)
	require.NoError(t, db.SetSetting("server_url", baseURL))
	require.NoError(t, db.SetSetting("llm_generate_model", plannerCaptionModel))
	llmClient := llm.New(llmURL)
	if emptyCacheMode || captionCacheMode {
		beforeMemory, err := client.MemoryInfo()
		require.NoError(t, err)
		loaded, err := llmClient.ListLoaded()
		require.NoError(t, err)
		require.NotNil(t, beforeMemory)
		require.NotNil(t, beforeMemory.CUDA)
		t.Logf("before planner: free VRAM=%.0f bytes; free RAM=%.0f bytes; loaded LLM count=%d", beforeMemory.CUDA.System.Free, beforeMemory.RAM.Free, len(loaded))
		if captionCacheMode {
			require.Equal(t, plannerCaptionModel, original, "live mode must use the exact configured caption model")
			names := make([]string, 0, len(loaded))
			for _, model := range loaded {
				names = append(names, model.Name)
			}
			require.Contains(t, names, plannerCaptionModel, "prepare cached caption model before live test")
			_, capacityErr := stylist.ResolvePlannerModelWithCapacity(plannerCaptionModel, func() ([]llm.LLMModel, error) { return []llm.LLMModel{{ID: plannerText14B}}, nil }, func() (stylist.PlannerCapacity, error) {
				return stylist.PlannerCapacity{FreeVRAM: beforeMemory.CUDA.System.Free, FreeRAM: beforeMemory.RAM.Free, Loaded: loaded}, nil
			})
			require.ErrorIs(t, capacityErr, stylist.ErrPlannerCapacity, "initial capacity must require caption release")
		}
	}
	svc := New(db, llmClient, client, &config.Config{}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
	got, err := svc.StylistPlannerModel()
	require.NoError(t, err)
	assert.Equal(t, plannerText14B, got)
	if captionCacheMode {
		loaded, err := llmClient.ListLoaded()
		require.NoError(t, err)
		for _, model := range loaded {
			assert.NotEqual(t, plannerCaptionModel, model.Name, "caption cache must be released")
		}
	}
	after, err := client.State()
	require.NoError(t, err)
	assert.Empty(t, after.RunningJob)
	assert.Empty(t, after.LoadedCheckpoint, "SD cache must remain empty after planner selection")
	assert.Equal(t, plannerCaptionModel, svc.getGenerateModel(), "automatic choice must not rewrite test global setting")
	var current string
	require.NoError(t, source.QueryRow("SELECT value FROM settings WHERE key='llm_generate_model'").Scan(&current))
	assert.Equal(t, original, current)
	t.Logf("planner=%s; cache %q -> %q; source global setting unchanged: %q", got, before.LoadedCheckpoint, after.LoadedCheckpoint, current)
}
