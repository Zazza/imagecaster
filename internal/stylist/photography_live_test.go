package stylist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/stretchr/testify/require"
)

const photographyLiveWait = 30 * time.Minute
const photographyLivePoll = 5 * time.Second

const photographyLiveFrames = 5
const photographyLiveSingle = 1
const photographyLiveDirMode os.FileMode = 0755
const photographyLiveFileMode os.FileMode = 0644

type photographyWaitingIdeas struct{ inner *liveIdeas }

func (w photographyWaitingIdeas) Chat(model, system, user string, temperature float64, tokens int) (string, error) {
	if os.Getenv("IMAGECASTER_PHOTO_WAIT_IDLE") == "1" {
		deadline := time.Now().Add(photographyLiveWait)
		for {
			var active int
			if err := w.inner.appDB.QueryRow("SELECT count(*) FROM job_queue WHERE status IN ('pending','running')").Scan(&active); err != nil {
				return "", err
			}
			if active == 0 {
				jobs, err := w.inner.images.ListJobs(context.Background(), "")
				if err != nil {
					return "", err
				}
				for _, job := range jobs {
					if !job.Terminal() {
						active++
					}
				}
				training, err := w.inner.server.TrainingJobs(context.Background(), "")
				if err != nil {
					return "", err
				}
				for _, job := range training {
					if !slices.Contains([]string{"completed", "failed", "cancelled", "paused"}, job.Status) {
						active++
					}
				}
			}
			if active == 0 {
				break
			}
			if time.Now().After(deadline) {
				return "", errors.New("GPU queues remained busy; no synthetic chat submitted")
			}
			time.Sleep(photographyLivePoll)
		}
	}
	reply, err := w.inner.Chat(model, system, user, temperature, tokens)
	w.inner.t.Logf("synthetic reply: %s", reply)
	return reply, err
}

func TestPhotographySelectionLive(t *testing.T) {
	if os.Getenv("IMAGECASTER_PHOTO_SELECTION_LIVE") != "1" {
		t.Skip("opt-in local photographic selection evaluation")
	}
	settingsDB, err := sql.Open("sqlite", "file:"+liveDefaultDB+"?mode=ro")
	require.NoError(t, err)
	defer settingsDB.Close()
	rows, err := settingsDB.Query("SELECT key,value FROM settings WHERE key IN ('server_url','llm_generate_model','llm_generate_num_ctx','llm_generate_num_thread','llm_generate_top_p')")
	require.NoError(t, err)
	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		require.NoError(t, rows.Scan(&k, &v))
		settings[k] = v
	}
	require.NoError(t, rows.Err())
	rows.Close()
	baseURL := settings["server_url"]
	require.True(t, lanonly.IsLocalURL(baseURL) && !lanonly.ViaProxy(baseURL))
	server := serverclient.NewClient()
	server.SetBaseURL(baseURL)
	sdURL, llmURL := server.ProxyURLs()
	images := sd.New(sdURL)
	models, err := images.GetModels()
	require.NoError(t, err)
	var names []string
	for _, m := range models {
		names = append(names, m.Name)
	}
	raw, err := os.ReadFile("../../data/presets/stylist-photography.json")
	require.NoError(t, err)
	db, err := preset.Open(filepath.Join(t.TempDir(), "selection.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.SyncModelPresets([]preset.PresetSource{{File: "stylist-photography.json", Raw: raw}}, names, nil)
	require.NoError(t, err)
	presets, err := db.List()
	require.NoError(t, err)
	var people []serverclient.Person
	for base := range Models {
		people = append(people, serverclient.Person{Name: "synthetic_" + base, DisplayName: "Synthetic", Base: base, Lora: "synthetic_" + base})
	}
	options, err := BuildShootOptions(people, "Synthetic", presets, nil, models, nil)
	require.NoError(t, err)
	require.NotEmpty(t, options)
	plannerModel := settings["llm_generate_model"]
	allowSwitch := false
	if override := os.Getenv("IMAGECASTER_PHOTO_PLANNER_MODEL"); override != "" {
		available, err := server.GetLLMModels()
		require.NoError(t, err)
		found := false
		for _, item := range available {
			if item.Name == override {
				found = true
			}
		}
		require.True(t, found, "explicit planner override must already be installed")
		plannerModel, allowSwitch = override, true
	}
	client := llm.New(llmURL)
	cfg := llm.BackendConfig{}
	cfg.NumCtx, _ = strconv.Atoi(settings["llm_generate_num_ctx"])
	cfg.NumThread, _ = strconv.Atoi(settings["llm_generate_num_thread"])
	cfg.TopP, _ = strconv.ParseFloat(settings["llm_generate_top_p"], liveFloatBits)
	client.SetConfigSource(func(string) llm.BackendConfig { return cfg })
	if !allowSwitch {
		plannerModel, err = ResolvePlannerModelWithCapacity(plannerModel, client.GetModels, func() (PlannerCapacity, error) {
			stats, err := images.MemoryInfo()
			if err != nil {
				return PlannerCapacity{}, err
			}
			if stats == nil || stats.CUDA == nil {
				return PlannerCapacity{}, errors.New("GPU memory metadata unavailable")
			}
			loaded, err := client.ListLoaded()
			return PlannerCapacity{FreeVRAM: stats.CUDA.System.Free, FreeRAM: stats.RAM.Free, Loaded: loaded}, err
		})
		require.NoError(t, err)
	}
	allowSwitch = allowSwitch || plannerModel != settings["llm_generate_model"]
	t.Logf("configured prompt model=%s resolved planner model=%s", settings["llm_generate_model"], plannerModel)
	real := &liveIdeas{t: t, chat: llm.ForPlanning(client), server: server, images: images, model: plannerModel, appDB: settingsDB, allowModelSwitch: allowSwitch}
	if os.Getenv("IMAGECASTER_PHOTO_TRANSPORT_ONLY") == "1" {
		const smokeReplyTokens = 32
		reply, err := (photographyWaitingIdeas{inner: real}).Chat(real.model, `Return JSON only: {"ok":true}.`, "Synthetic transport test.", 0, smokeReplyTokens)
		require.NoError(t, err)
		var result struct {
			OK bool `json:"ok"`
		}
		require.NoError(t, json.Unmarshal([]byte(reply), &result))
		require.True(t, result.OK)
		return
	}
	base, err := Load()
	require.NoError(t, err)
	cases := []struct {
		name, wish, contains string
		frames               int
	}{
		{"skin", "Естественный фотографический портрет: фактура кожи без пластиковой ретуши, мягкий свет окна, спокойные натуральные цвета, человек одет.", "Natural Skin", photographyLiveFrames},
		{"fabric", "Фотосессия одежды: подчеркнуть фактуру шерсти и льна, швы, складки и серебряный браслет; точные цвета ткани, человек в одежде.", "Fabric Study", photographyLiveSingle},
		{"scene", "Экологический портрет в кафе: человек и выразительная архитектура террасы одинаково важны; фон и пространство должны быть хорошо различимы, глубокая резкость.", "Environmental Story", photographyLiveSingle},
		{"noir", "Чёрно-белая фотографическая съёмка в стиле нуар: выразительный боковой свет, глубокие тени и читаемые детали одежды.", "Noir Chiaroscuro", photographyLiveSingle},
		{"artistic", "Художественная полностью одетая будуарная съёмка: мягкий тёплый направленный свет, выразительные тени, аккуратная тональная обработка, без обнажения.", "Artistic Boudoir", photographyLiveSingle},
	}
	garment := os.Getenv("IMAGECASTER_PHOTO_SELECTION_GARMENT") == "1"
	if garment {
		cases = cases[:1]
		cases[0].name, cases[0].contains = "garment", "Fabric Study"
		cases[0].wish = "Фотосессия шерстяного пальто в студии: подчеркнуть фактуру ткани, швы и складки; показать всё пальто до подола, единые фон и свет во всех кадрах."
		cases[0].frames = photographyLiveFrames
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := SuggestRequest{Plan: PlanRequest{Person: "Synthetic", Frames: tc.frames, Wish: tc.wish, Locale: "ru"}}
			if garment {
				req.Plan.Garment = true
				req.GarmentOutfit = "a red wool coat with long sleeves and a knee-length hem"
			}
			got, err := Suggest(photographyWaitingIdeas{inner: real}, real.model, req, base, options)
			require.NoError(t, err)
			require.NotNil(t, got)
			data, err := json.MarshalIndent(got, "", "  ")
			require.NoError(t, err)
			t.Logf("selected photographic plan for wish %q:\n%s", tc.wish, data)
			out := os.Getenv("IMAGECASTER_PHOTO_SELECTION_OUT")
			if out != "" {
				require.NoError(t, os.MkdirAll(out, photographyLiveDirMode))
				require.NoError(t, os.WriteFile(filepath.Join(out, tc.name+".json"), data, photographyLiveFileMode))
			}
			require.Contains(t, got.TargetName, tc.contains, "choice must match the photographic task rather than prior/base model")
			require.NotEmpty(t, got.TargetModel)
			require.NotEmpty(t, got.TargetReason)
			if !suggestRussian(got.TargetReasonRu) {
				require.Contains(t, got.Warnings, suggestDisplayWarning, "optional localization fallback must be explained")
			}
			count := 0
			for _, series := range got.Series {
				count += len(series.Shots)
				if garment {
					for _, shot := range series.Shots {
						require.Contains(t, []string{"knee", "full"}, shot.Framing, "garment shots must show the whole coat")
					}
				}
			}
			require.Equal(t, tc.frames, count)
			t.Logf("selected=%s model=%s reason_source=%s reason_ru=%s frames=%d warnings=%v", got.TargetName, got.TargetModel, got.TargetReason, got.TargetReasonRu, count, got.Warnings)
			assertPhotographyWishInPlan(t, tc.name, data)
		})
	}
}

func assertPhotographyWishInPlan(t *testing.T, scenario string, data []byte) {
	t.Helper()
	var plan struct {
		Light  string `json:"light"`
		Series []struct {
			Background string `json:"background"`
			Direction  string `json:"direction"`
			Shots      []struct {
				Prompt string `json:"prompt"`
			} `json:"shots"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(data, &plan))
	require.NotEmpty(t, plan.Series)
	for _, series := range plan.Series {
		fields := []string{series.Background, series.Direction}
		for _, shot := range series.Shots {
			fields = append(fields, shot.Prompt)
		}
		shoot := strings.ToLower(strings.Join(fields, " "))
		switch scenario {
		case "scene":
			require.Regexp(t, `\b(cafe|coffee\s*(shop|house))\b|café`, shoot, "the requested café must shape the English shoot")
			require.Regexp(t, `\b(terrace|patio|veranda|architectur[a-z]*)\b`, shoot, "the requested terrace architecture must shape the English shoot")
			require.NotContains(t, shoot, "plain studio wall", "a studio example does not fulfil the café terrace wish")
		case "noir":
			lighting := strings.ToLower(series.Direction + " " + plan.Light)
			for _, shot := range series.Shots {
				lighting += " " + strings.ToLower(shot.Prompt)
			}
			require.Regexp(t, `\b(noir|monochrom[a-z]*|black[ -]and[ -]white|chiaroscuro|deep shadows|dramatic shadows|high[ -]contrast)\b`, lighting, "noir must shape actual direction, light or shot prompt, beyond the selected preset name")
		}
	}
}
