package generation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/stretchr/testify/require"
)

const photoEvalWait = 30 * time.Minute
const photoEvalPoll = 5 * time.Second

const photoEvalSeed int64 = 14729
const photoEvalDB = "/srv/go-sd/build/bin/data/presets.db"
const photoEvalOutput = "/tmp/stylist-photo-evals"
const photoEvalDirMode os.FileMode = 0755
const photoEvalFileMode os.FileMode = 0644

var photoEvalSlug = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type photoEvalSD struct {
	*sd.Client
	server   *serverclient.Client
	requests []sd.Txt2ImgRequest
	appDB    *sql.DB
}

func (c *photoEvalSD) Txt2Img(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
	deadline := time.Now().Add(photoEvalWait)
	for {
		var active int
		if err := c.appDB.QueryRow("SELECT count(*) FROM job_queue WHERE status IN ('pending','running')").Scan(&active); err != nil {
			return nil, fmt.Errorf("cannot verify application queue: %w", err)
		}
		if active == 0 {
			break
		}
		if os.Getenv("IMAGECASTER_PHOTO_WAIT_IDLE") != "1" || time.Now().After(deadline) {
			return nil, fmt.Errorf("live photo eval refuses pending or running application jobs")
		}
		time.Sleep(photoEvalPoll)
	}
	jobs, err := c.Client.ListJobs(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("cannot verify idle image queue: %w", err)
	}
	for _, job := range jobs {
		if !job.Terminal() {
			return nil, fmt.Errorf("live photo eval refuses active image jobs")
		}
	}
	training, err := c.server.TrainingJobs(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("cannot verify training queue: %w", err)
	}
	for _, job := range training {
		if !slices.Contains([]string{"completed", "failed", "cancelled", "paused"}, job.Status) {
			return nil, fmt.Errorf("live photo eval refuses active training jobs")
		}
	}
	if strings.Contains(strings.ToLower(req.Prompt), "<lora:") {
		return nil, fmt.Errorf("synthetic eval forbids personal or external LoRAs")
	}
	c.requests = append(c.requests, req)
	return c.Client.Txt2Img(req)
}

func TestStylistPhotoLive(t *testing.T) {
	if os.Getenv("IMAGECASTER_PHOTO_LIVE") != "1" {
		t.Skip("opt-in synthetic photo preset evaluation")
	}
	cfgDB, err := sql.Open("sqlite", "file:"+photoEvalDB+"?mode=ro")
	require.NoError(t, err)
	defer cfgDB.Close()
	var baseURL string
	require.NoError(t, cfgDB.QueryRow("SELECT value FROM settings WHERE key='server_url'").Scan(&baseURL))
	require.True(t, lanonly.IsLocalURL(baseURL) && !lanonly.ViaProxy(baseURL), "only direct local server is allowed")
	server := serverclient.NewClient()
	server.SetBaseURL(baseURL)
	sdURL, _ := server.ProxyURLs()
	client := &photoEvalSD{Client: sd.New(sdURL), server: server, appDB: cfgDB}
	client.SetClientName("stylist-synthetic-preset-eval")
	models, err := client.GetModels()
	require.NoError(t, err)
	var installed []string
	for _, m := range models {
		installed = append(installed, m.Name)
	}
	raw, err := os.ReadFile("../../data/presets/stylist-photography.json")
	require.NoError(t, err)
	db, err := preset.Open(filepath.Join(t.TempDir(), "synthetic.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SetSetting("llm_auto_unload", "false"))
	_, err = db.SyncModelPresets([]preset.PresetSource{{File: "stylist-photography.json", Raw: raw}}, installed, nil)
	require.NoError(t, err)
	rows, err := db.List()
	require.NoError(t, err)
	outDir := os.Getenv("IMAGECASTER_PHOTO_OUT")
	if outDir == "" {
		outDir = photoEvalOutput
	}
	require.NoError(t, os.MkdirAll(outDir, photoEvalDirMode))
	svc := New(db, &mockLLM{}, client, &config.Config{SDUrl: sdURL}, t.TempDir(), &mockEmitter{}, kids.NewManager(db), &mockSessions{}, logger.New(nil))
	only := os.Getenv("IMAGECASTER_PHOTO_MODEL")
	actor := "a fictional adult person"
	switch os.Getenv("IMAGECASTER_PHOTO_ACTOR") {
	case "man":
		actor = "a fictional adult man"
	case "woman":
		actor = "a fictional adult woman"
	case "":
	default:
		t.Fatal("photo eval actor must be empty, man or woman")
	}
	var selectedPlan *stylist.SuggestResult
	if planPath := os.Getenv("IMAGECASTER_PHOTO_PLAN"); planPath != "" {
		raw, err := os.ReadFile(planPath)
		require.NoError(t, err)
		selectedPlan = new(stylist.SuggestResult)
		require.NoError(t, json.Unmarshal(raw, selectedPlan))
		require.NotEmpty(t, selectedPlan.TargetName)
		require.NotEmpty(t, selectedPlan.Series)
		require.NotEmpty(t, selectedPlan.Series[0].Shots)
	}
	profileFilter := os.Getenv("IMAGECASTER_PHOTO_PROFILE")
	environmentScene := os.Getenv("IMAGECASTER_PHOTO_SCENE") == "environment"
	if environmentScene {
		profileFilter = "Environmental Story"
	}
	evaluated := 0
	families := []string{"Flux.2", "Z-Image", "SDXL", "Flux.1", "Moss-Image"}
	for _, family := range families {
		if only != "" && family != only {
			continue
		}
		m := stylist.Models[family]
		var profiles []preset.Preset
		for _, p := range rows {
			if p.ModelName == m.Model && (profileFilter == "" || strings.Contains(p.Name, profileFilter)) && (selectedPlan == nil || p.Name == selectedPlan.TargetName) {
				profiles = append(profiles, p)
			}
		}
		if len(profiles) == 0 {
			t.Logf("family=%s unavailable; no new model installed", family)
			continue
		}
		baseline := preset.Preset{Name: "Baseline " + family, ModelName: m.Model, Sampler: m.Sampler, Steps: m.Steps, CfgScale: m.CFG, ScheduleType: profiles[0].ScheduleType, Loras: "[]"}
		require.NoError(t, db.Create(&baseline))
		t.Logf("family=%s profiles=%d", family, len(profiles))
		profiles = append([]preset.Preset{baseline}, profiles...)
		evaluated += len(profiles)
		for _, p := range profiles {
			t.Run(p.Name, func(t *testing.T) {
				series := stylist.Series{Outfit: "a charcoal wool coat over a cream linen shirt, dark tailored trousers and a small silver bracelet", Background: "a quiet cafe terrace with wooden chairs, a stone wall and distant greenery"}
				shot := stylist.Shot{Prompt: "standing naturally beside a wooden chair, the complete head and face fully visible inside the frame with a small margin above the head, both hands held visibly at waist level, looking toward the camera", Framing: "waist"}
				if environmentScene {
					shot.Framing, shot.Landscape = "full", true
					shot.Prompt = "a wide environmental photograph, the whole person visible from head to shoes with space around the body, standing beside the chair at the cafe terrace, the surrounding architecture and furniture clearly visible and important to the composition"
				}
				if selectedPlan != nil {
					series = selectedPlan.Series[0]
					shot = series.Shots[0]
				}
				width, height := stylist.FrameDims(stylist.SizeNormal, shot.Landscape)
				scene := stylist.StyledFramePrompt(family, "", actor, series, shot)
				if p.ID == baseline.ID {
					scene = stylist.FramePrompt(family, "", actor, series, shot)
				}
				spec := struct {
					Preset        preset.Preset
					Scene         string
					Seed          int64
					Width, Height int
				}{Preset: p, Scene: scene, Seed: photoEvalSeed, Width: width, Height: height}
				spec.Preset.ID = 0
				spec.Preset.CreatedAt = ""
				spec.Preset.UpdatedAt = ""
				encoded, err := json.Marshal(spec)
				require.NoError(t, err)
				hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
				name := photoEvalSlug.ReplaceAllString(p.Name, "-")
				pngPath := filepath.Join(outDir, name+".png")
				recordPath := filepath.Join(outDir, name+".json")
				if prior, readErr := os.ReadFile(recordPath); readErr == nil {
					var saved struct {
						Hash string `json:"hash"`
					}
					if json.Unmarshal(prior, &saved) == nil && saved.Hash == hash {
						if _, statErr := os.Stat(pngPath); statErr == nil {
							t.Log("matching synthetic artifact already exists")
							return
						}
					}
				}
				before := len(client.requests)
				seed := photoEvalSeed
				result, err := svc.GeneratePresetShot(PresetShotParams{PresetID: p.ID, ScenePrompt: scene, Width: width, Height: height, Seed: &seed})
				require.NoError(t, err)
				require.Greater(t, len(client.requests), before)
				used := client.requests[len(client.requests)-1]
				require.Equal(t, p.ModelName, used.Model.Checkpoint)
				require.Equal(t, p.Sampler, used.SamplerName)
				require.Equal(t, p.Steps, used.Steps)
				require.Equal(t, p.CfgScale, used.CfgScale)
				imageBytes, err := base64.StdEncoding.DecodeString(result.Image)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(pngPath, imageBytes, photoEvalFileMode))
				record := map[string]any{"hash": hash, "synthetic": true, "preset": p.Name, "model": used.Model.Checkpoint, "sampler": used.SamplerName, "scheduler": used.Scheduler, "steps": used.Steps, "cfg": used.CfgScale, "width": used.Width, "height": used.Height, "seed": photoEvalSeed, "cache_mode": used.CacheMode, "prompt": used.Prompt, "negative_prompt": used.NegativePrompt, "image": pngPath}
				data, err := json.MarshalIndent(record, "", "  ")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(recordPath, data, photoEvalFileMode))
				t.Logf("synthetic artifact=%s", pngPath)
			})
			if t.Failed() {
				return
			}
		}
	}
	require.Positive(t, evaluated, "no eligible synthetic preset cases were evaluated")
}
