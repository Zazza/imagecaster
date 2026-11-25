package stylist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/promptutil"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

const (
	liveDefaultDB     = "/srv/go-sd/build/bin/data/presets.db"
	liveMaxCalls      = 128
	liveFloatBits     = 64
	liveSingleFrame   = 1
	livePartialSeries = FramesPerSeries + 1
	liveStyleFrames   = 3
	liveMixedFrames   = 3
	liveMixedOutfit   = "a charcoal linen suit and a white cotton shirt"
)

type liveIdeas struct {
	t                *testing.T
	chat             Ideas
	server           *serverclient.Client
	images           *sd.Client
	model            string
	calls, maxBudget int
	appDB            *sql.DB
	allowModelSwitch bool
}

func (c *liveIdeas) Chat(model, system, user string, temperature float64, budget int) (string, error) {
	if c.appDB != nil {
		var active int
		if err := c.appDB.QueryRow("SELECT count(*) FROM job_queue WHERE status IN ('pending','running')").Scan(&active); err != nil {
			return "", errors.New("live smoke cannot verify application queue")
		}
		if active > 0 {
			return "", errors.New("live smoke refuses pending or running application jobs")
		}
	}
	if c.calls >= liveMaxCalls {
		return "", errors.New("live smoke call limit reached")
	}
	if model != c.model {
		return "", errors.New("live smoke refuses a model change")
	}
	status, err := c.server.GetStatus()
	if err != nil {
		return "", errors.New("live smoke cannot verify running model")
	}
	if !c.allowModelSwitch && len(status.Models.LLMRunning) > 0 && !slices.Contains(status.Models.LLMRunning, model) {
		return "", errors.New("live smoke refuses to replace another running model")
	}
	jobs, err := c.images.ListJobs(context.Background(), "")
	if err != nil {
		return "", errors.New("live smoke cannot verify image queue")
	}
	for _, j := range jobs {
		if !j.Terminal() {
			return "", errors.New("live smoke refuses active image jobs")
		}
	}
	training, err := c.server.TrainingJobs(context.Background(), "")
	if err != nil {
		return "", errors.New("live smoke cannot verify training queue")
	}
	for _, j := range training {
		switch j.Status {
		case "completed", "failed", "cancelled", "paused":
		default:
			return "", errors.New("live smoke refuses active or unknown training status")
		}
	}
	c.calls++
	c.maxBudget = max(c.maxBudget, budget)
	raw, err := c.chat.Chat(model, system, user, temperature, budget)
	if err != nil {
		return "", errors.New("live LLM request failed (response text withheld)")
	}
	var observed struct {
		Series []struct {
			Shots []struct {
				Framing string `json:"framing"`
			} `json:"shots"`
		} `json:"series"`
	}
	if json.Unmarshal([]byte(promptutil.ExtractJSON(raw)), &observed) == nil && len(observed.Series) > 0 {
		counts := make([]int, len(observed.Series))
		framing := map[string]int{}
		for i, series := range observed.Series {
			counts[i] = len(series.Shots)
			for _, shot := range series.Shots {
				switch shot.Framing {
				case "close", "waist", "knee", "full":
					framing[shot.Framing]++
				default:
					framing["invalid"]++
				}
			}
		}
		c.t.Logf("response_series=%d response_shot_counts=%v response_framing_counts=%v", len(observed.Series), counts, framing)
	}
	var vocabulary map[string]int = make(map[string]int)
	var scan func(any)
	scan = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, value := range x {
				if text, ok := value.(string); ok && (key == "prompt" || key == "direction") {
					for _, match := range forbiddenRe.FindAllString(text, -1) {
						label := strings.ToLower(match)
						switch label {
						case "jaw", "jawline", "nose", "cheekbones", "highlights", "platinum", "porcelain":
						default:
							label = "identity_descriptor"
						}
						vocabulary[label]++
					}
				} else {
					scan(value)
				}
			}
		case []any:
			for _, value := range x {
				scan(value)
			}
		}
	}
	var decoded any
	if json.Unmarshal([]byte(promptutil.ExtractJSON(raw)), &decoded) == nil {
		scan(decoded)
	}
	if len(vocabulary) > 0 {
		c.t.Logf("response_validation_vocabulary=%v", vocabulary)
	}
	return raw, nil
}

func TestSuggestLive(t *testing.T) {
	if os.Getenv("IMAGECASTER_STYLIST_LIVE") != "1" {
		t.Skip("set IMAGECASTER_STYLIST_LIVE=1 for configured local LLM smoke")
	}
	path := os.Getenv("IMAGECASTER_STYLIST_LIVE_DB")
	if path == "" {
		path = liveDefaultDB
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal("open read-only configuration:", err)
	}
	defer db.Close()
	settings := map[string]string{}
	rows, err := db.Query(`SELECT key,value FROM settings WHERE key IN ('server_url','llm_generate_model','llm_generate_num_ctx','llm_generate_top_p','llm_generate_num_thread','stylist_last_plan')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		settings[k] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	baseURL := strings.TrimRight(settings["server_url"], "/")
	if !lanonly.IsLocalURL(baseURL) || lanonly.ViaProxy(baseURL) {
		t.Fatal("live smoke requires a direct local server URL")
	}
	model := settings["llm_generate_model"]
	if model == "" {
		t.Fatal("generate model is not configured")
	}
	server := serverclient.NewClient()
	server.SetBaseURL(baseURL)
	sdURL, llmURL := server.ProxyURLs()
	images := sd.New(sdURL)
	people, err := server.People(context.Background())
	if err != nil {
		t.Fatal("read people metadata failed")
	}
	models, err := images.GetModels()
	if err != nil {
		t.Fatal("read model catalog failed")
	}
	loras, err := images.GetLoRAs()
	if err != nil {
		t.Fatal("read LoRA catalog failed")
	}
	var saved struct {
		Form struct {
			Person        string                  `json:"person"`
			Gender        string                  `json:"gender"`
			Wish          string                  `json:"wish"`
			Frames        int                     `json:"frames"`
			Themes        []string                `json:"themes"`
			Aspects       map[string]AspectChoice `json:"aspects"`
			GarmentOutfit string                  `json:"garmentOutfit"`
			Target        string                  `json:"target"`
			PlanningMode  string                  `json:"planningMode"`
			TargetLocked  *bool                   `json:"targetLocked"`
		} `json:"form"`
	}
	if err := json.Unmarshal([]byte(settings["stylist_last_plan"]), &saved); err != nil {
		t.Fatal("saved textual form is missing or invalid")
	}
	form := saved.Form
	req := SuggestRequest{Plan: PlanRequest{Person: form.Person, Gender: form.Gender, Wish: form.Wish, Frames: form.Frames, Themes: form.Themes, Aspects: form.Aspects, Garment: strings.TrimSpace(form.GarmentOutfit) != "", Locale: "ru"}, GarmentOutfit: form.GarmentOutfit}
	if form.PlanningMode == "auto" {
		req.Plan.Aspects = nil
	}
	if form.TargetLocked != nil && *form.TargetLocked {
		req.Target = form.Target
		if req.Target == "" {
			t.Fatal("saved form has no locked target")
		}
	}
	base, err := liveBase(db)
	if err != nil {
		t.Fatal(err)
	}
	presets, compounds, err := liveTargets(db)
	if err != nil {
		t.Fatal(err)
	}
	options, err := BuildShootOptions(people, req.Plan.Person, presets, compounds, models, loras)
	if err != nil {
		t.Fatal("build options failed (private metadata withheld)")
	}
	client := llm.New(llmURL)
	cfg := llm.BackendConfig{}
	cfg.NumCtx, _ = strconv.Atoi(settings["llm_generate_num_ctx"])
	cfg.NumThread, _ = strconv.Atoi(settings["llm_generate_num_thread"])
	cfg.TopP, _ = strconv.ParseFloat(settings["llm_generate_top_p"], liveFloatBits)
	client.SetConfigSource(func(string) llm.BackendConfig { return cfg })
	real := &liveIdeas{t: t, chat: client, server: server, images: images, model: model}
	cases := []struct {
		name string
		req  SuggestRequest
	}{}
	selected := os.Getenv("IMAGECASTER_STYLIST_LIVE_CASE")
	if selected == "" {
		selected = "saved"
	}
	if selected == "saved" || selected == "matrix" {
		cases = append(cases, struct {
			name string
			req  SuggestRequest
		}{"saved", req})
	}
	addSynthetic := func(name string, frames int, garment bool) {
		r := req
		r.Plan.Wish = "Coherent studio fashion editorial with soft window light"
		r.Plan.Themes = nil
		r.Plan.Aspects = nil
		r.Plan.Frames = frames
		r.Plan.Garment = garment
		r.GarmentOutfit = ""
		if garment {
			r.GarmentOutfit = "a red wool coat with long sleeves and a knee length hem"
		}
		cases = append(cases, struct {
			name string
			req  SuggestRequest
		}{name, r})
	}
	switch selected {
	case "saved":
	case "synthetic-garment":
		addSynthetic(selected, FramesPerSeries, true)
	case "synthetic-no-garment":
		addSynthetic(selected, FramesPerSeries, false)
	case "maximum":
		addSynthetic(selected, MaxFrames, false)
	case "matrix":
		addSynthetic("single", liveSingleFrame, false)
		addSynthetic("partial-garment", livePartialSeries, true)
		addSynthetic("maximum", MaxFrames, false)
		addSynthetic("mixed", liveMixedFrames, false)
		if len(base.Pose) == 0 {
			t.Fatal("mixed smoke requires a manual pose")
		}
		cases[len(cases)-1].req.Plan.Aspects = map[string]AspectChoice{
			string(AspectOutfit): {Mode: ModeCustom, Text: liveMixedOutfit},
			string(AspectHair):   {Mode: ModeKeep},
			string(AspectPose):   {Mode: ModePick, IDs: []string{base.Pose[0].ID}},
		}
		addSynthetic("noir", liveStyleFrames, false)
		cases[len(cases)-1].req.Plan.Wish = "Noir portrait in a rainy city at night, hard side light, deep shadows, environment visible, monochrome atmosphere"
		addSynthetic("natural-skin", liveStyleFrames, false)
		cases[len(cases)-1].req.Plan.Wish = "Natural window-light portrait, realistic skin texture, gentle retouching look without changing identity or skin tone"
	default:
		t.Fatal("unknown IMAGECASTER_STYLIST_LIVE_CASE")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := real.calls
			requestBefore, marshalErr := json.Marshal(tc.req)
			if marshalErr != nil {
				t.Fatal("cannot snapshot request")
			}
			configBefore, marshalErr := json.Marshal(cfg)
			if marshalErr != nil {
				t.Fatal("cannot snapshot config")
			}
			got, err := Suggest(real, model, tc.req, base, options)
			requestAfter, marshalErr := json.Marshal(tc.req)
			if marshalErr != nil || string(requestBefore) != string(requestAfter) {
				t.Error("Suggest mutated its request")
			}
			configAfter, marshalErr := json.Marshal(cfg)
			if marshalErr != nil || string(configBefore) != string(configAfter) {
				t.Error("Suggest mutated generation configuration")
			}
			t.Logf("calls=%d total_calls=%d max_response_budget=%d", real.calls-before, real.calls, real.maxBudget)
			if err != nil {
				t.Fatalf("suggest rejected: %s", liveSafeError(err))
			}
			if !hasShootOption(options, got.Target) || (tc.req.Target != "" && got.Target != tc.req.Target) {
				t.Error("result changed locked target or chose unavailable target")
			}
			displayFallback := slices.Contains(got.Warnings, suggestDisplayWarning)
			missingDisplay := 0
			for _, display := range []string{got.ConceptRu, got.LightRu, got.FocusRu} {
				if !suggestRussian(display) {
					missingDisplay++
				}
			}
			framing := map[string]int{}
			shots := 0
			for _, s := range got.Series {
				if !suggestRussian(s.DirectionRu) {
					missingDisplay++
				}
				for _, aspect := range seriesAspects {
					if *seriesField(&s, aspect) != "" && !suggestRussian(*seriesRuField(&s, aspect)) {
						missingDisplay++
					}
				}
				if tc.name == "mixed" && (s.Outfit != liveMixedOutfit || s.Hair != "") {
					t.Error("mixed plan changed custom outfit or kept hair")
				}
				for _, sh := range s.Shots {
					if tc.req.Plan.Choice(AspectPose).Mode == ModeAuto && sh.Wear != "" {
						t.Error("AI added unsupported shot wardrobe override")
					}
					if tc.req.Plan.Garment && tc.req.Plan.Choice(AspectPose).Mode == ModeAuto && suggestTightPose.MatchString(FramePrompt("SDXL", "", "a person", s, sh)) {
						t.Error("composed garment prompt contains a conflicting crop")
					}
					if !suggestRussian(sh.PromptRu) {
						missingDisplay++
					}
					generationShot := sh
					generationShot.PromptRu = shotOf(base.Pose[0]).PromptRu
					if tc.name == "mixed" && generationShot != shotOf(base.Pose[0]) {
						t.Error("mixed plan changed manually selected pose")
					}
					if tc.req.Plan.Garment && tc.req.Plan.Choice(AspectPose).Mode == ModeAuto {
						if sh.Framing != "knee" && sh.Framing != "full" {
							t.Error("automatic garment shot has unsafe framing")
						}
						if suggestTightPose.MatchString(sh.Prompt) {
							t.Error("automatic garment shot text conflicts with knee/full framing")
						}
					}
					shots++
					switch sh.Framing {
					case "close", "waist", "knee", "full":
						framing[sh.Framing]++
					default:
						framing["invalid"]++
					}
				}
			}
			if missingDisplay > 0 && !displayFallback {
				t.Error("missing Russian display translations were not explained")
			}
			t.Logf("missing_display=%d translation_fallback=%t", missingDisplay, displayFallback)
			t.Logf("result_count=%d shots=%d expected_shots=%d framing_counts=%v warnings_count=%d", len(got.Series), shots, tc.req.Plan.Frames, framing, len(got.Warnings))
			if shots != tc.req.Plan.Frames {
				t.Fatal("result shot count differs from requested count")
			}
		})
	}
}

func liveSafeError(err error) string {
	text := err.Error()
	if strings.ContainsAny(text, "\"'\n") || strings.Contains(text, "invalid character") {
		return "validation failed (private text withheld)"
	}
	return text
}

func liveBase(db *sql.DB) (*Base, error) {
	base, err := Load()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id,aspect,name,prompt,prompt_ru,place,framing,places,orientation FROM stylist_presets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []UserPreset
	for rows.Next() {
		var p UserPreset
		var places string
		if err := rows.Scan(&p.ID, &p.Aspect, &p.Name, &p.Prompt, &p.PromptRu, &p.Place, &p.Framing, &places, &p.Orientation); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(places), &p.Places); err != nil {
			return nil, errors.New("invalid user preset places")
		}
		users = append(users, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	merged, err := base.WithUser(users)
	if err != nil {
		return nil, errors.New("user preset merge failed (private text withheld)")
	}
	return merged, nil
}

func liveTargets(db *sql.DB) ([]preset.Preset, []preset.CompoundPreset, error) {
	rows, err := db.Query(`SELECT id,name,preset_type,prompt,model_name,coalesce(loras,'') FROM presets ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	var ps []preset.Preset
	for rows.Next() {
		var p preset.Preset
		if err := rows.Scan(&p.ID, &p.Name, &p.PresetType, &p.Prompt, &p.ModelName, &p.Loras); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ps = append(ps, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	rows, err = db.Query(`SELECT id,name,description FROM compound_presets ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	var cs []preset.CompoundPreset
	for rows.Next() {
		var c preset.CompoundPreset
		if err := rows.Scan(&c.ID, &c.Name, &c.Description); err != nil {
			rows.Close()
			return nil, nil, err
		}
		cs = append(cs, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	rows, err = db.Query(`SELECT compound_preset_id,preset_id,loras FROM compound_preset_steps ORDER BY compound_preset_id,step_order`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var s preset.CompoundPresetStep
		if err := rows.Scan(&id, &s.PresetID, &s.Loras); err != nil {
			return nil, nil, err
		}
		i := slices.IndexFunc(cs, func(c preset.CompoundPreset) bool { return c.ID == id })
		if i < 0 {
			return nil, nil, fmt.Errorf("orphan compound step")
		}
		cs[i].Steps = append(cs[i].Steps, s)
	}
	return ps, cs, rows.Err()
}
