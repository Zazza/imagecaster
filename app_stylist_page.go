package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	xdraw "golang.org/x/image/draw"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/imgformat"
	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/people"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"

	_ "golang.org/x/image/webp"
)

const (
	stylistAppearanceKey      = "stylist_appearance"
	stylistAppearanceMax      = 500
	stylistPageSource         = "stylist"
	stylistPageLabelMax       = 80
	stylistPagePeopleWait     = 30 * time.Second
	stylistGarmentMaxSide     = 2048
	stylistGarmentJPEGQuality = 92
	stylistFramesMax          = 200
	stylistFrameGone          = "gone"
	stylistLocaleRu           = "ru"
	stylistWarnNoRu           = "Перевод не удался — показан английский"
)

var errStylistPageNotLocal = errors.New("Стилист: сервер не в локальной сети или запросы идут через прокси — съёмка не запущена")

type stylistPeople interface {
	List(ctx context.Context) ([]serverclient.Person, error)
}

type stylistPipelines interface {
	PipelineModels(id int64) (string, []string, error)
}

type shootTarget struct {
	person       serverclient.Person
	base         string
	model        string
	lora         string
	pipeline     int64
	pipelineName string
	loras        map[string]string
	faceSwapName string
}

func resolveShootTarget(all []serverclient.Person, pipes stylistPipelines, req StylistShootRequest) (*shootTarget, error) {
	name := strings.TrimSpace(req.Person)
	key, ok := people.Find(all, name)
	if !ok {
		return nil, fmt.Errorf("человек %q не найден на сервере", name)
	}
	var mine []serverclient.Person
	for _, p := range all {
		if strings.EqualFold(people.Key(p), key) {
			mine = append(mine, p)
		}
	}
	if req.Pipeline > 0 {
		if pipes == nil {
			return nil, errors.New("пайплайны недоступны")
		}
		title, models, err := pipes.PipelineModels(req.Pipeline)
		if err != nil {
			return nil, fmt.Errorf("пайплайн: %w", err)
		}
		byBase := people.LorasByBase(all, key)
		loras := map[string]string{}
		var bases []string
		for _, m := range models {
			b := generation.ModelBase(m)
			if l, ok := byBase[b]; ok && b != "" {
				if _, dup := loras[b]; !dup {
					bases = append(bases, b)
				}
				loras[b] = l
			}
		}
		if len(loras) == 0 {
			return nil, fmt.Errorf("«%s»: нет LoRA ни для одного шага пайплайна «%s»", key, title)
		}
		t := &shootTarget{person: mine[0], pipeline: req.Pipeline, pipelineName: title, loras: loras}
		if req.FaceSwap {
			t.faceSwapName = people.FaceSwapName(all, key, bases)
		}
		return t, nil
	}
	var picked *serverclient.Person
	switch {
	case req.Base != "":
		for i := range mine {
			if people.BaseOf(mine[i]) == req.Base {
				picked = &mine[i]
			}
		}
		if picked == nil {
			return nil, fmt.Errorf("«%s»: нет LoRA для %s", key, req.Base)
		}
	case len(mine) == 1:
		picked = &mine[0]
	case slices.ContainsFunc(mine, func(p serverclient.Person) bool { return p.Name == name }):
		for i := range mine {
			if mine[i].Name == name {
				picked = &mine[i]
			}
		}
	default:
		bases := make([]string, 0, len(mine))
		for _, p := range mine {
			bases = append(bases, people.BaseOf(p))
		}
		return nil, fmt.Errorf("«%s»: несколько LoRA (%s) — выберите модель", key, strings.Join(bases, ", "))
	}
	m, ok := stylist.Models[people.BaseOf(*picked)]
	if !ok || picked.Lora == "" {
		return nil, fmt.Errorf("LoRA %q обучена под %s — съёмка умеет только Z-Image, Moss-Image, SDXL, Flux.2 и Flux.1", picked.Name, picked.Base)
	}
	t := &shootTarget{person: *picked, base: people.BaseOf(*picked), model: m.Model, lora: picked.Lora}
	if req.FaceSwap && picked.Identity {
		t.faceSwapName = picked.Name
	}
	return t, nil
}

type stylistQueue interface {
	EnqueueLabeled(jobType queue.JobType, params any, source, label string) (int64, error)
}

type stylistFrameResults interface {
	Results(ids []int64) ([]*queue.Job, error)
}

type stylistSettings interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

type StylistPlanResult struct {
	Series   []stylist.Series `json:"series"`
	Warnings []string         `json:"warnings"`
	UsedLLM  bool             `json:"used_llm"`
}

type StylistSeriesResult struct {
	Series   stylist.Series `json:"series"`
	Warnings []string       `json:"warnings"`
	UsedLLM  bool           `json:"used_llm"`
}

type StylistShootRequest struct {
	Person        string           `json:"person"`
	Appearance    string           `json:"appearance"`
	Garment       string           `json:"garment"`
	GarmentOutfit string           `json:"garment_outfit"`
	Series        []stylist.Series `json:"series"`
	FaceSwap      bool             `json:"face_swap"`
	Seed          *int64           `json:"seed"`
	Base          string           `json:"base,omitempty"`
	Pipeline      int64            `json:"pipeline,omitempty"`
}

type StylistFrame struct {
	JobID    int64  `json:"job_id"`
	Status   string `json:"status"`
	ItemID   int64  `json:"item_id"`
	Error    string `json:"error"`
	Shot     string `json:"shot"`
	FaceSwap string `json:"face_swap"`
	Garment  string `json:"garment,omitempty"`
}

func (a *App) StylistPresets() (*stylist.Base, error) {
	return a.stylistBase()
}

func (a *App) stylistModel() string {
	if a.gen == nil {
		return ""
	}
	return a.gen.GenerateModel()
}

func (a *App) StylistPlan(req stylist.PlanRequest) (*StylistPlanResult, error) {
	base, err := a.stylistBase()
	if err != nil {
		return nil, err
	}
	if req.Seed == 0 {
		req.Seed = time.Now().UnixNano()
	}
	return planStylist(a.llm, a.stylistModel(), req, base, a.logStylist)
}

func (a *App) StylistRegenerateSeries(req stylist.PlanRequest, index int) (*StylistSeriesResult, error) {
	base, err := a.stylistBase()
	if err != nil {
		return nil, err
	}
	if req.Seed == 0 {
		req.Seed = time.Now().UnixNano()
	}
	return regenerateStylistSeries(a.llm, a.stylistModel(), req, base, index, a.logStylist)
}

func (a *App) StylistTranslate(texts []string) ([]string, error) {
	return stylist.Translate(a.llm, a.stylistModel(), texts)
}

func (a *App) logStylist(err error) {
	if a.log != nil {
		a.log.Warn("stylist page: llm ideas: %s", err)
	}
}

func askStylistIdeas(chat stylist.Ideas, model string, req stylist.PlanRequest, base *stylist.Base, count int, logf func(error)) ([]stylist.Idea, []string) {
	if len(req.AutoAspects()) == 0 {
		return nil, nil
	}
	ideas, warnings, err := stylist.AskLLM(chat, model, req, base, count)
	if err != nil {
		if logf != nil {
			logf(err)
		}
		return nil, nil
	}
	if req.Locale == stylistLocaleRu {
		if err := stylist.AddRussian(chat, model, ideas); err != nil {
			if logf != nil {
				logf(err)
			}
			warnings = append(warnings, stylistWarnNoRu)
		}
	}
	return ideas, warnings
}

func planStylist(chat stylist.Ideas, model string, req stylist.PlanRequest, base *stylist.Base, logf func(error)) (*StylistPlanResult, error) {
	if err := stylist.CheckFrames(req.Frames); err != nil {
		return nil, err
	}
	if err := stylist.CheckCustom(req); err != nil {
		return nil, err
	}
	ideas, llmWarnings := askStylistIdeas(chat, model, req, base, len(stylist.SeriesSizes(req.Frames)), logf)
	series, warnings, err := stylist.Plan(req, base, ideas)
	if err != nil {
		return nil, err
	}
	return &StylistPlanResult{Series: series, Warnings: append(llmWarnings, warnings...), UsedLLM: ideas != nil}, nil
}

func regenerateStylistSeries(chat stylist.Ideas, model string, req stylist.PlanRequest, base *stylist.Base, index int, logf func(error)) (*StylistSeriesResult, error) {
	if err := stylist.CheckFrames(req.Frames); err != nil {
		return nil, err
	}
	if err := stylist.CheckCustom(req); err != nil {
		return nil, err
	}
	ideas, llmWarnings := askStylistIdeas(chat, model, req, base, 1, logf)
	var idea *stylist.Idea
	if len(ideas) > 0 {
		idea = &ideas[0]
	}
	s, warnings, err := stylist.RegenerateSeries(req, base, idea, index)
	if err != nil {
		return nil, err
	}
	return &StylistSeriesResult{Series: s, Warnings: append(llmWarnings, warnings...), UsedLLM: idea != nil}, nil
}

func (a *App) StylistPickGarment() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Фото вещи",
		Filters: []runtime.FileFilter{{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg;*.webp"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if len(data) > generation.MaxImageBytes {
		return "", config.ImageTooLarge("image")
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

var errStylistGarmentUnnamed = errors.New("назовите вещи с фото в поле «Что на фото», например: a top and a skirt — без названий модель сливает их в одну вещь")

var errStylistAppearanceCorrupt = errors.New("сохранённые описания внешности повреждены")

func loadStylistAppearances(st stylistSettings) (map[string]string, error) {
	raw, err := st.GetSetting(stylistAppearanceKey)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}, fmt.Errorf("%w: %v", errStylistAppearanceCorrupt, err)
	}
	return out, nil
}

func getStylistAppearance(st stylistSettings, name string) (string, error) {
	m, err := loadStylistAppearances(st)
	if err != nil {
		return "", err
	}
	return m[strings.TrimSpace(name)], nil
}

func setStylistAppearance(st stylistSettings, name, text string, logf func(error)) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("не выбран человек")
	}
	text = strings.TrimSpace(text)
	if len([]rune(text)) > stylistAppearanceMax {
		return fmt.Errorf("внешность — не длиннее %d символов", stylistAppearanceMax)
	}
	m, err := loadStylistAppearances(st)
	if err != nil {
		if !errors.Is(err, errStylistAppearanceCorrupt) {
			return err
		}
		if logf != nil {
			logf(fmt.Errorf("перезаписываю: %w", err))
		}
	}
	if text == "" {
		delete(m, name)
	} else {
		m[name] = text
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return st.SetSetting(stylistAppearanceKey, string(raw))
}

func (a *App) StylistAppearance(name string) (string, error) {
	return getStylistAppearance(a.presets, name)
}

func (a *App) SetStylistAppearance(name, text string) error {
	return setStylistAppearance(a.presets, name, text, func(err error) {
		if a.log != nil {
			a.log.Warn("stylist page: appearance: %s", err)
		}
	})
}

func (a *App) StylistShoot(req StylistShootRequest) ([]int64, error) {
	if a.serverClient == nil || a.serverClient.GetBaseURL() == "" {
		return nil, errors.New("сервер Imagecaster не настроен")
	}
	ctx, cancel := a.peopleCtx(stylistPagePeopleWait)
	defer cancel()
	return stylistShoot(ctx, a.peopleSvc(), appPipelines{a.presets}, a.queueSvc, sdNotLocal(a.config), req)
}

func defaultStylistAppearance(gender string) string {
	switch strings.ToLower(strings.TrimSpace(gender)) {
	case "man", "male":
		return "a man"
	case "woman", "female", "":
		return "a woman"
	}
	return "a person"
}

func stylistPageLabel(name, pose string, i, total int) string {
	s := "Стилист: " + name
	if pose != "" {
		s += ", " + pose
	}
	if r := []rune(s); len(r) > stylistPageLabelMax {
		s = string(r[:stylistPageLabelMax-1]) + "…"
	}
	return s + fmt.Sprintf(" (%d из %d)", i, total)
}

func stylistShoot(ctx context.Context, peopleSvc stylistPeople, pipes stylistPipelines, q stylistQueue, notLocal bool, req StylistShootRequest) ([]int64, error) {
	if notLocal {
		return nil, errStylistPageNotLocal
	}
	total := 0
	for _, s := range req.Series {
		total += len(s.Shots)
	}
	if total == 0 {
		return nil, errors.New("в плане нет кадров")
	}
	if total > stylist.MaxFrames {
		return nil, fmt.Errorf("кадров за раз не больше %d, в плане %d", stylist.MaxFrames, total)
	}
	if len(req.Garment) > config.MaxImageBase64Len {
		return nil, config.ImageTooLarge("image")
	}
	name := strings.TrimSpace(req.Person)
	if name == "" {
		return nil, errors.New("не выбран человек")
	}
	all, err := peopleSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("список людей с сервера: %w", err)
	}
	target, err := resolveShootTarget(all, pipes, req)
	if err != nil {
		return nil, err
	}
	appearance := strings.TrimSpace(req.Appearance)
	if appearance == "" {
		appearance = defaultStylistAppearance(target.person.Gender)
	}
	garmentOutfit := strings.TrimSpace(req.GarmentOutfit)
	if req.Garment != "" && garmentOutfit == "" {
		return nil, errStylistGarmentUnnamed
	}
	garment, err := shrinkGarment(req.Garment)
	if err != nil {
		return nil, err
	}
	if r := []rune(garmentOutfit); len(r) > stylistAppearanceMax {
		garmentOutfit = string(r[:stylistAppearanceMax])
	}
	ids := make([]int64, 0, total)
	i := 0
	for _, s := range req.Series {
		outfit := s.Outfit
		if garment != "" {
			outfit = garmentOutfit
			s.Outfit = garmentOutfit
			if s.Outfit == "" {
				s.Outfit = stylist.DefaultOutfit
			}
		}
		for _, sh := range s.Shots {
			p := mcp.ShootFrameParams{
				Person: target.person.Name, Base: target.base, Model: target.model, Shot: sh.Pose,
				Prompt: stylist.FramePrompt(target.lora, appearance, s, sh),
				Index:  i + 1, Total: total,
				GarmentBase64: garment, Outfit: outfit,
				FaceSwap: target.faceSwapName != "",
				Width:    stylist.Width, Height: stylist.Height,
				Pipeline: target.pipeline, PipelineName: target.pipelineName,
				PersonLoras: target.loras, FaceSwapName: target.faceSwapName,
			}
			if sh.Landscape {
				p.Width, p.Height = stylist.Height, stylist.Width
			}
			if req.Seed != nil {
				v := *req.Seed + int64(i)
				p.Seed = &v
			}
			id, err := q.EnqueueLabeled(queue.JobStylistFrame, p, stylistPageSource, stylistPageLabel(name, sh.Pose, i+1, total))
			if err != nil {
				if len(ids) > 0 {
					return ids, fmt.Errorf("кадр %d из %d не поставлен в очередь: %v; уже в очереди: %v", i+1, total, err, ids)
				}
				return nil, fmt.Errorf("кадр %d из %d не поставлен в очередь: %v", i+1, total, err)
			}
			ids = append(ids, id)
			i++
		}
	}
	return ids, nil
}

func shrinkGarment(b64 string) (string, error) {
	if b64 == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(imgcodec.StripDataURL(b64))
	if err != nil {
		return "", fmt.Errorf("фото вещи: %w", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if errors.Is(err, image.ErrFormat) {
		return imgcodec.StripDataURL(b64), nil
	}
	if err != nil {
		return "", fmt.Errorf("фото вещи: %w", err)
	}
	long := max(cfg.Width, cfg.Height)
	if long <= stylistGarmentMaxSide {
		return imgcodec.StripDataURL(b64), nil
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("фото вещи: %w", err)
	}
	src = imgformat.ApplyOrientation(src, imgformat.JPEGOrientation(data))
	sb := src.Bounds()
	w := max(1, sb.Dx()*stylistGarmentMaxSide/long)
	h := max(1, sb.Dy()*stylistGarmentMaxSide/long)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	if f, ok := imgformat.Detect(data); ok && f == imgformat.PNG {
		return imgcodec.EncodePNG(dst, png.DefaultCompression)
	}
	return imgcodec.EncodeJPEG(dst, stylistGarmentJPEGQuality)
}

func (a *App) StylistFrames(jobIDs []int64) ([]StylistFrame, error) {
	if a.queueSvc == nil {
		return nil, errors.New("очередь недоступна")
	}
	return stylistFrames(a.queueSvc, jobIDs)
}

func stylistFrames(q stylistFrameResults, ids []int64) ([]StylistFrame, error) {
	if len(ids) > stylistFramesMax {
		ids = ids[len(ids)-stylistFramesMax:]
	}
	out := make([]StylistFrame, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	jobs, err := q.Results(ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*queue.Job, len(jobs))
	for _, j := range jobs {
		if j.Type == queue.JobStylistFrame {
			byID[j.ID] = j
		}
	}
	for i, id := range ids {
		f := StylistFrame{JobID: id, Status: stylistFrameGone}
		if j, ok := byID[id]; ok {
			f.Status = string(j.Status)
			f.Error = j.Error
			if j.Status == queue.StatusCompleted {
				var r queue.JobResult
				var fr mcp.ShootFrameResult
				if json.Unmarshal([]byte(j.Result), &r) == nil && json.Unmarshal(r.Frame, &fr) == nil {
					f.ItemID, f.Shot, f.FaceSwap, f.Garment = fr.ID, fr.Shot, fr.FaceSwap, fr.Garment
				}
			}
		}
		out[i] = f
	}
	return out, nil
}

const stylistGarmentNamesPrompt = "Name only the clothes and worn items in this photo (garments, shoes, belt, hat, scarf, gloves, stockings, a bag worn or carried), by type only, as one short English phrase joined with commas and \"and\", for example: \"a cropped top and a midi skirt\" or \"a blazer, a shirt and wide trousers\". Skip jewelry and anything that is not worn, such as a glass, a phone or flowers. Do not mention colours, fabrics, patterns, the person, the pose or the background. Answer with the phrase only."

var errStylistVisionNotLocal = errors.New("определить вещи можно только локальной моделью ИИ — модель не в локальной сети, фото не отправлено; напишите вещи сами")

type garmentDescriber interface {
	DescribeImage(imageBase64, prompt string) (string, error)
}

func stylistDescribeGarment(d garmentDescriber, notLocal bool, garment string) (string, error) {
	if garment == "" {
		return "", errors.New("нет фото вещи")
	}
	if notLocal {
		return "", errStylistVisionNotLocal
	}
	if d == nil {
		return "", errors.New("модель ИИ недоступна")
	}
	small, err := shrinkGarment(garment)
	if err != nil {
		return "", err
	}
	text, err := d.DescribeImage(small, stylistGarmentNamesPrompt)
	if err != nil {
		return "", fmt.Errorf("не удалось определить вещи: %w", err)
	}
	names := cleanGarmentNames(text)
	if names == "" {
		return "", errors.New("модель не назвала вещи — напишите их сами")
	}
	return names, nil
}

func cleanGarmentNames(text string) string {
	line := strings.TrimSpace(text)
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	line = strings.Trim(line, "\"'`«» .")
	if r := []rune(line); len(r) > stylistAppearanceMax {
		line = strings.TrimSpace(string(r[:stylistAppearanceMax]))
	}
	return line
}

func llmNotLocal(cfg *config.Config) bool {
	return cfg != nil && (!lanonly.IsLocalURL(cfg.LLMUrl) || lanonly.ViaProxy(cfg.LLMUrl))
}

func (a *App) StylistDescribeGarment(garment string) (string, error) {
	var d garmentDescriber
	if a.gen != nil {
		d = a.gen
	}
	return stylistDescribeGarment(d, llmNotLocal(a.config), garment)
}

type appPipelines struct{ db *preset.DB }

func (p appPipelines) PipelineModels(id int64) (string, []string, error) {
	if p.db == nil {
		return "", nil, errors.New("база пресетов недоступна")
	}
	return p.db.CompoundStepModels(id)
}
