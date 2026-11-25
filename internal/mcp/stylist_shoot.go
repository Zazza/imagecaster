package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/people"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
	"github.com/Zazza/imagecaster/internal/stylisttrace"
)

type StylistPeople interface {
	People(ctx context.Context) ([]serverclient.Person, error)
}

type StylistShooter interface {
	TestGenerate(p generation.TestGenerateParams) ([]generation.TestGenerateResultItem, error)
}

type DiscardSessions struct{}

func (DiscardSessions) AddToSession(string, json.RawMessage, string, *preset.GenerationMeta) int64 {
	return 0
}

type shootModel = stylist.Model

var shootModels = stylist.Models

const (
	shootWidth         = stylist.Width
	shootHeight        = stylist.Height
	shootMaxShots      = 6
	shootPeopleWait    = 20 * time.Second
	shootBackground    = stylist.DefaultBackground
	shootQuality       = stylist.Quality
	shootQualityStudio = stylist.QualityStudio
	shootNegative      = "deformed, extra limbs, extra fingers, blurry, text, watermark"
	shootDefaultOutfit = stylist.DefaultOutfit
	shootNoLoraPrompt  = "LoRA нет — работай правкой фото (stylist_try)"
)

var shootDefaultShots = []string{
	"standing facing the camera, full body, arms relaxed",
	"three-quarter view, full body, one foot slightly forward",
	"side profile view, full body, standing straight",
	"view from behind, full body, looking over the shoulder",
}

func (s *Server) shootDeps() (StylistPeople, StylistShooter, AppSessions, StylistEngine, FaceRefiner, error) {
	if s.deps.People == nil || s.deps.Shooter == nil || s.deps.Sessions == nil {
		return nil, nil, nil, nil, nil, errors.New("фотосессия: недоступно в этой сборке imagecaster-mcp (нет списка людей или генерации)")
	}
	if err := s.localOnly("стилист"); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return s.deps.People, s.deps.Shooter, s.deps.Sessions, s.deps.Stylist, s.deps.Refiner, nil
}

func allPeople(people StylistPeople) ([]serverclient.Person, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shootPeopleWait)
	defer cancel()
	all, err := people.People(ctx)
	if err != nil {
		return nil, fmt.Errorf("список людей с сервера: %w", err)
	}
	return all, nil
}

func onlySupported(all []serverclient.Person) []serverclient.Person {
	out := make([]serverclient.Person, 0, len(all))
	for _, p := range all {
		if _, ok := shootModels[people.BaseOf(p)]; ok {
			p.Base = people.BaseOf(p)
			out = append(out, p)
		}
	}
	return out
}

func lookParts(l Look) []string {
	var parts []string
	for _, v := range []string{l.HairCut, l.HairColor, l.Outfit, l.Headwear, l.Makeup, l.Free} {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	switch g := strings.TrimSpace(l.Glasses); {
	case strings.EqualFold(g, "none"):
		parts = append(parts, "no glasses")
	case g != "":
		parts = append(parts, g)
	}
	return parts
}

func personBase(p serverclient.Person) string {
	return people.BaseOf(p)
}

func shootPromptFor(base, lora, appearance string, look Look, shot, background string) string {
	parts := []string{appearance}
	if strings.TrimSpace(look.Outfit) == "" {
		parts = append(parts, shootDefaultOutfit)
	}
	parts = append(parts, lookParts(look)...)
	parts = append(parts, shot)
	if !stylist.Prose(base) {
		return stylist.ComposePrompt(lora, parts, background)
	}
	outfit := look.Outfit
	if strings.TrimSpace(outfit) == "" {
		outfit = shootDefaultOutfit
	}
	hair := joinNonEmpty(look.HairCut, look.HairColor)
	details := []string{stylist.Wearing(outfit), stylist.Labeled("Hair", hair), look.Headwear, stylist.Labeled("Makeup", look.Makeup), look.Free}
	if lp := lookParts(Look{Glasses: look.Glasses}); len(lp) > 0 {
		details = append(details, lp...)
	}
	return stylist.ComposeProse(lora, stylist.Subject("", appearance, shot), details, background)
}

func joinNonEmpty(vs ...string) string {
	var out []string
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, ", ")
}

func unknownPerson(list []serverclient.Person, name string) error {
	names := strings.Split(personNames(list), ", ")
	if len(list) == 0 {
		names = nil
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Errorf("человек %q не найден: обученных людей под поддерживаемые модели нет — %s", name, shootNoLoraPrompt)
	}
	return fmt.Errorf("человек %q не найден среди обученных под поддерживаемые модели: %s", name, strings.Join(names, ", "))
}

type shootPlan struct {
	name     string
	person   serverclient.Person
	m        shootModel
	shots    []string
	prompts  []string
	seed     *int64
	garment  string
	ge       StylistGarmentEngine
	outfit   string
	refineOn bool
	faceSwap bool
	w, h     int
	label    string
	pipeline int64
	pipeName string
	loras    map[string]string
	swapName string
}

func planShoot(people StylistPeople, eng StylistEngine, args map[string]any, pipes pipelineLookup) (*shootPlan, error) {
	name := strings.TrimSpace(argString(args, "person"))
	if name == "" {
		return nil, errors.New("person — имя обученного человека из stylist_people")
	}
	appearance := strings.TrimSpace(argString(args, "appearance"))
	if appearance == "" {
		return nil, errors.New("appearance пуст — опиши основу внешности с фото: телосложение, цвет кожи, причёску")
	}
	var shots []string
	for _, v := range argStringSlice(args, "shots") {
		if v = strings.TrimSpace(v); v != "" {
			shots = append(shots, v)
		}
	}
	if len(shots) == 0 {
		shots = shootDefaultShots
	}
	if len(shots) > shootMaxShots {
		return nil, fmt.Errorf("кадров за раз не больше %d, передано %d", shootMaxShots, len(shots))
	}
	look, err := decodeArg[Look](args, "look")
	if err != nil {
		return nil, err
	}
	if look == nil {
		look = &Look{}
	}
	garment, ge, err := readGarmentArg(args, eng)
	if err != nil {
		return nil, err
	}
	background := strings.TrimSpace(argString(args, "background"))
	if background == "" {
		background = shootBackground
	}
	all, err := allPeople(people)
	if err != nil {
		return nil, err
	}
	person, label, tgt, err := shootTarget(all, name, strings.TrimSpace(argString(args, "base")), int64(argInt(args, "pipeline")), pipes, optBool(args, "face_swap", false))
	if err != nil {
		return nil, err
	}
	m := shootModels[person.Base]
	plan := &shootPlan{
		label: label, pipeline: tgt.pipeline, pipeName: tgt.name, loras: tgt.loras, swapName: tgt.swap,
		name: person.Name, person: person, m: m, shots: shots, seed: optInt(args, "seed"),
		garment: garment, ge: ge, outfit: strings.TrimSpace(look.Outfit), refineOn: optBool(args, "face_pass", false),
		faceSwap: optBool(args, "face_swap", false), w: shootWidth, h: shootHeight,
	}
	if strings.EqualFold(strings.TrimSpace(argString(args, "orientation")), "landscape") {
		plan.w, plan.h = shootHeight, shootWidth
	}
	promptBase, promptLora := personBase(person), person.Lora
	if plan.pipeline > 0 {
		promptBase, promptLora = tgt.base, ""
	}
	if stylist.Prose(promptBase) {
		appearance = stylist.WithSubject(appearance, person.Gender)
	}
	for _, shot := range shots {
		plan.prompts = append(plan.prompts, shootPromptFor(promptBase, promptLora, appearance, *look, shot, background))
	}
	return plan, nil
}

func (p *shootPlan) shownName() string {
	if p.label != "" {
		return p.label
	}
	return p.name
}

func (p *shootPlan) frameSeed(i int) *int64 {
	if p.seed == nil {
		return nil
	}
	v := *p.seed + int64(i)
	return &v
}

type stylistTracer interface {
	NewStylistTrace() *stylisttrace.Recorder
}

func traceError(err error) {
	if err != nil {
		log.Printf("[stylist diagnostics] %v", err)
	}
}

func (out *frameOut) closeTrace() { traceError(out.trace.Close()) }

func (out *frameOut) finishTrace(id int64) { traceError(out.trace.Finish(id)) }

type frameOut struct {
	anatomyWarning string
	trace          *stylisttrace.Recorder
	img            string
	info           json.RawMessage
	garment        string
	facePass       string
	faceSwap       string
	stopped        bool
}

type frameBase func() (string, json.RawMessage, int64, error)

func txt2imgBase(shooter StylistShooter, m shootModel, prompt string, seed *int64, w, h int) frameBase {
	seed = copyShootSeed(seed)
	return func() (string, json.RawMessage, int64, error) {
		img, info, actualSeed, err := shootOne(shooter, generation.TestGenerateParams{
			Mode:           "models",
			SelectedModels: []string{m.Model},
			Prompt:         prompt,
			NegativePrompt: shootNegative,
			Sampler:        m.Sampler,
			Steps:          m.Steps,
			CfgScale:       m.CFG,
			Width:          w,
			Height:         h,
			Seed:           seed,
		})
		seed = nextShootSeed(actualSeed, seed)
		return img, info, actualSeed, err
	}
}

func shootFrame(base frameBase, eng StylistEngine, ge StylistGarmentEngine, refiner FaceRefiner, m shootModel, prompt string, garment, outfit, wear string, refineOn bool) (frameOut, error) {
	return shootFrameChecked(base, eng, ge, refiner, m, prompt, garment, outfit, wear, refineOn, nil)
}

func shootFrameChecked(base frameBase, eng StylistEngine, ge StylistGarmentEngine, refiner FaceRefiner, m shootModel, prompt string, garment, outfit, wear string, refineOn bool, stopped func() bool) (frameOut, error) {
	var out frameOut
	var frameSeed int64
	var err error
	for attempt := 0; attempt < shootAnatomyAttempts; attempt++ {
		if shootStopped(eng, stopped) {
			return out, generation.ErrInterrupted
		}
		out.img, out.info, frameSeed, err = base()
		if err != nil {
			return out, err
		}
		out.anatomyWarning, err = checkShootAnatomy(eng, out.img, prompt, anatomyBaseDefect, stopped)
		if err != nil {
			return out, err
		}
		if out.anatomyWarning != anatomyBaseDefect {
			break
		}
	}
	if tracer, ok := eng.(stylistTracer); ok {
		out.trace = tracer.NewStylistTrace()
	}
	traceError(out.trace.Save(stylisttrace.Base, out.img))
	var passSeed *int64
	if frameSeed != 0 {
		passSeed = &frameSeed
	}
	if garment != "" && out.anatomyWarning == anatomyBaseDefect {
		out.garment = facePassSkipped(anatomyBaseSkipped)
	}
	if garment != "" && out.anatomyWarning != anatomyBaseDefect {
		acceptedBase, baseWarning := out.img, out.anatomyWarning
		for attempt := 0; attempt < shootAnatomyAttempts; attempt++ {
			if shootStopped(eng, stopped) {
				out.closeTrace()
				return out, generation.ErrInterrupted
			}
			res, tryErr := tryOnGarment(ge, acceptedBase, garment, outfit, wear, passSeed)
			switch {
			case tryErr != nil && generation.IsInterrupted(tryErr):
				if _, checked := eng.(StylistAnatomyChecker); checked {
					out.closeTrace()
					return out, generation.ErrInterrupted
				}
				out.stopped = true
				out.garment = facePassSkipped("прервано")
			case tryErr != nil:
				out.img, out.anatomyWarning = acceptedBase, baseWarning
				out.garment = facePassSkipped(faceSkipReason(tryErr))
			case res == nil || res.Image == "":
				out.img, out.anatomyWarning = acceptedBase, baseWarning
				out.garment = facePassSkipped("примерка не вернула картинку")
			default:
				out.img = res.Image
				out.garment = shootFaceDone
				warning, checkErr := checkShootAnatomy(eng, out.img, prompt, anatomyGarmentDefect, stopped)
				if checkErr != nil {
					out.closeTrace()
					return out, checkErr
				}
				if warning != "" {
					out.anatomyWarning = warning
				}
				if warning == anatomyGarmentDefect && attempt+1 < shootAnatomyAttempts {
					passSeed = nextShootSeed(infoSeed(res.Info), passSeed)
					continue
				}
				if warning == "" && out.anatomyWarning == anatomyGarmentDefect {
					out.anatomyWarning = baseWarning
				}
			}
			break
		}
		if out.garment == shootFaceDone {
			traceError(out.trace.Save(stylisttrace.Garment, out.img))
		}
	}
	if shootStopped(eng, stopped) {
		out.closeTrace()
		return out, generation.ErrInterrupted
	}
	out.facePass = shootFacePassMissing
	if !refineOn {
		out.facePass = "off"
	}
	if out.stopped {
		out.facePass = facePassSkipped("прервано")
	} else if refineOn && eng != nil && refiner != nil {
		out.img, out.facePass, out.stopped = refineShootFace(eng, refiner, out.img, prompt, shootNegative, m, passSeed)
	}
	if _, checked := eng.(StylistAnatomyChecker); checked && (out.stopped || shootStopped(eng, stopped)) {
		out.closeTrace()
		return out, generation.ErrInterrupted
	}
	if out.facePass == shootFaceDone {
		traceError(out.trace.Save(stylisttrace.FacePass, out.img))
	}
	return out, nil
}

func stylistShoot(people StylistPeople, shooter StylistShooter, sessions AppSessions, eng StylistEngine, refiner FaceRefiner, args map[string]any) (string, error) {
	plan, err := planShoot(people, eng, args, nil)
	if err != nil {
		return "", err
	}
	var variants []map[string]any
	var saved []string
	stopped := false
	for i, shot := range plan.shots {
		prompt := plan.prompts[i]
		out, err := shootFrame(txt2imgBase(shooter, plan.m, prompt, plan.frameSeed(i), plan.w, plan.h), eng, plan.ge, refiner, plan.m, prompt, plan.garment, plan.outfit, "", plan.refineOn)
		if err != nil {
			if generation.IsInterrupted(err) {
				if _, checked := eng.(StylistAnatomyChecker); checked {
					return "", err
				}
				break
			}
			if len(saved) > 0 {
				return "", fmt.Errorf("кадр %d из %d не получился: %s; уже готовы кадры (id в сессии «Стилист»): %s", i+1, len(plan.shots), faceSkipReason(err), strings.Join(saved, ", "))
			}
			return "", fmt.Errorf("кадр %d из %d не получился: %s", i+1, len(plan.shots), faceSkipReason(err))
		}
		defer out.closeTrace()
		if shootStopped(eng, nil) {
			return "", generation.ErrInterrupted
		}
		shootFaceSwap(eng, &out, plan.name, plan.faceSwap)
		if _, checked := eng.(StylistAnatomyChecker); checked && (out.stopped || shootStopped(eng, nil)) {
			return "", generation.ErrInterrupted
		}
		stopped = out.stopped
		meta := &preset.GenerationMeta{Model: plan.m.Model, Label: truncateLabel("Стилист: " + plan.name + ", " + shot)}
		id := sessions.AddToNamedSession(StylistSessionName, out.img, out.info, "stylist", meta)
		out.finishTrace(id)
		out.closeTrace()
		saved = append(saved, fmt.Sprint(id))
		v := map[string]any{"id": id, "shot": shot, "prompt": prompt, "face_pass": out.facePass, "face_swap": out.faceSwap}
		if out.anatomyWarning != "" {
			v["anatomy_warning"] = out.anatomyWarning
		}
		if out.garment != "" {
			v["garment"] = out.garment
		}
		variants = append(variants, v)
		if stopped {
			break
		}
	}
	res := map[string]any{
		"person":   plan.name,
		"base":     plan.person.Base,
		"model":    plan.m.Model,
		"variants": variants,
	}
	if stopped {
		res["interrupted"] = true
	}
	return toJSON(res), nil
}

func truncateLabel(s string) string {
	r := []rune(s)
	if len(r) <= stylistLabelMax {
		return s
	}
	return string(r[:stylistLabelMax-1]) + "…"
}

func shootOne(shooter StylistShooter, p generation.TestGenerateParams) (string, json.RawMessage, int64, error) {
	items, err := shooter.TestGenerate(p)
	if err != nil {
		return "", nil, 0, err
	}
	if len(items) == 0 {
		return "", nil, 0, errors.New("генерация не вернула картинку")
	}
	if items[0].Error != "" {
		return "", nil, 0, errors.New(items[0].Error)
	}
	if items[0].Image == "" {
		return "", nil, 0, errors.New("генерация не вернула картинку")
	}
	return items[0].Image, items[0].Info, items[0].Seed, nil
}

func registerShootTools(s *Server) {
	s.Register(Tool{
		Name: "stylist_people",
		Description: "Стилист: обученные люди на сервере по Имени (name — Имя человека или имя LoRA, если Имя не задано; base и created_at — его первой LoRA; gender; loras — его LoRA: lora, base — модель, face — есть лицо для подмены; pipelines — пайплайны приложения, где хотя бы один шаг на модели с его LoRA: id, name, steps, steps_with_lora), только под модели, с которыми умеет stylist_shoot. " +
			"Спроси человека, кого снимать (по Имени) и на чём — модель (base) или пайплайн; сам не выбирай. Пусто — LoRA нет, работай правкой фото stylist_try.",
		InputSchema: props(map[string]any{}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			peopleSvc, _, _, _, _, err := s.shootDeps()
			if err != nil {
				return "", err
			}
			all, err := allPeople(peopleSvc)
			if err != nil {
				return "", err
			}
			list := onlySupported(all)
			out := make([]map[string]any, 0, len(list))
			idx := map[string]int{}
			for _, p := range list {
				key := people.Key(p)
				lora := map[string]any{"lora": p.Name, "base": p.Base, "face": p.Identity, "created_at": p.CreatedAt}
				if i, ok := idx[strings.ToLower(key)]; ok {
					out[i]["loras"] = append(out[i]["loras"].([]map[string]any), lora)
					continue
				}
				idx[strings.ToLower(key)] = len(out)
				out = append(out, map[string]any{"name": key, "base": p.Base, "gender": p.Gender, "created_at": p.CreatedAt, "loras": []map[string]any{lora}})
			}
			for _, o := range out {
				o["pipelines"] = s.personPipelines(list, o["name"].(string))
			}
			res := map[string]any{"people": out}
			if len(out) == 0 {
				res["hint"] = shootNoLoraPrompt
			}
			return toJSON(res), nil
		},
	})
	s.Register(Tool{
		Name: "stylist_shoot",
		Description: "Стилист: фотосессия по LoRA человека — новые кадры (не правка фото) на модели, под которую обучена LoRA, без стилей приложения; готовые кадры появляются в приложении в сессии «Стилист». " +
			"person — Имя из stylist_people (старое имя LoRA тоже принимается); base — модель его LoRA или pipeline — id пайплайна из stylist_people (на каждом шаге LoRA человека под модель шага, примерка вещи и подмена лица — после последнего шага; кадр того же размера, что и на модели, с учётом orientation, если у шага пайплайна нет своего разрешения; face_pass не действует); у человека одна LoRA — base можно не передавать. Что выбрать, решает человек. appearance (по-английски) — только то, чего НЕТ в датасете LoRA: пол, телосложение, рост, татуировки и приметы; НЕ описывай возраст, тон кожи, лысину/бороду/причёску и черты лица — LoRA знает их из фото, промптовые токены искажают (возраст «старит», тон кожи перекрашивает, «bald» раздувает лоб). Совсем пустым appearance оставлять нельзя. " +
			"look — образ, как у stylist_try (одежда подробно: тип, силуэт, длина, вырез, рукава, ткань, цвет, принт, застёжки). shots — до 6 поз/ракурсов по-английски, по кадру на каждый (пусто — 4: спереди, вполоборота, сбоку, со спины); look и appearance одинаковы для всей серии, чтобы вещь была одна. background — фон (по умолчанию светлая студия). " +
			"face_pass=true — после генерации кадра лицо перерисовывается вторым проходом img2img с той же LoRA (крупный режим), серия идёт дольше; у каждого кадра поле face_pass: done | off | skipped:<причина>. " +
			"face_swap=true — последним шагом лицо на кадре подменяется лицом человека (нужно лицо для подмены на странице «Люди» приложения и модели подмены на сервере); у кадра поле face_swap: done | off | skipped:<причина> (кадр без подмены). " +
			"garment_path — фото вещи: на каждый кадр её надевает правка Klein по фото (стороны, которых нет на фото, модель дорисует сама), затем идёт проход лица; поле garment у кадра — done | skipped:<причина> (кадр без вещи), +6–11 с на кадр. Годится для 1–2 вещей; комплект из 3+ предметов примерка НЕ ТЯНЕТ (Klein калечит пропорции на любой базе) — такой лук опиши текстом подробно по каждой вещи (цвет/ткань/фасон с фото через локальную vision), без garment_path. Фото вещи лучше давать НА ЧЕЛОВЕКЕ. Цвет/узор/ткань текстом НЕ описывать — вещь с фото; но СТРУКТУРУ комплекта перечислять ВСЕ предметы поимённо, включая мелочи (a headscarf, stockings, a belt) — неназванный предмет Klein не переносит; если состав неясен — спроси у человека до съёмки. " +
			"Кадры не рисуются сразу: каждый ставится заданием в очередь приложения Imagecaster (там прогресс, «осталось», пауза и отмена; приложение закрыто — начнёт при запуске). Ответ: jobs — номера заданий по порядку кадров; готовность, id кадров в сессии, face_pass, face_swap и garment — stylist_shoot_status(job_ids). Скажи человеку, что серия идёт в приложении, и проверяй статус, когда он спросит или через пару минут. Картинок и путей нет.",
		InputSchema: props(map[string]any{
			"person":       prop("Имя человека из stylist_people (или имя LoRA)", "string"),
			"base":         prop("модель LoRA человека из stylist_people (loras[].base); не нужна, если у человека одна LoRA или передан pipeline", "string"),
			"pipeline":     prop("id пайплайна из stylist_people (pipelines[].id) вместо base", "integer"),
			"appearance":   prop("по-английски, только то, чего нет в датасете LoRA: пол, телосложение, рост, тату; без возраста, тона кожи и причёски", "string"),
			"look":         lookSchema,
			"shots":        prop("позы и ракурсы по-английски, 1–6, по кадру на каждый", "array", map[string]any{"items": map[string]any{"type": "string"}}),
			"background":   prop("фон по-английски (по умолчанию светлая студия)", "string"),
			"garment_path": prop("путь к фото вещи (лучше на человеке): Klein надевает её с фото; цвет/узор текстом не описывать, но структуру комплекта (top+bottom раздельно) — указать", "string"),
			"seed":         prop("сид первого кадра, дальше +1", "integer"),
			"orientation":  prop("ориентация кадра: portrait (по умолчанию) или landscape (1536×1024); подбирай по позам сам: лёжа/полёжа и широкие композиции — landscape, стоя/крупные планы — portrait", "string"),
			"face_pass":    prop("перерисовка лица вторым проходом, по умолчанию false (A/B 2026-11-18: сходство не улучшает, только сглаживает кожу); true — включить", "boolean"),
			"face_swap":    prop("подмена лица лицом человека последним шагом кадра, по умолчанию false", "boolean"),
		}, "person", "appearance"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			people, shooter, sessions, eng, refiner, err := s.shootDeps()
			if err != nil {
				return "", err
			}
			if s.deps.Queue != nil {
				plan, err := planShoot(people, eng, args, s.pipelineModels)
				if err != nil {
					return "", err
				}
				return enqueueShoot(s.deps.Queue, plan)
			}
			return stylistShoot(people, shooter, sessions, eng, refiner, args)
		},
	})
}

type shootPipeTarget struct {
	base     string
	pipeline int64
	name     string
	loras    map[string]string
	swap     string
}

func shootTarget(all []serverclient.Person, name, base string, pipeline int64, pipes pipelineLookup, faceSwap bool) (serverclient.Person, string, shootPipeTarget, error) {
	supported := onlySupported(all)
	key, ok := personKeyFor(supported, name)
	if !ok {
		return serverclient.Person{}, "", shootPipeTarget{}, unknownPerson(supported, name)
	}
	var mine []serverclient.Person
	for _, p := range supported {
		if strings.EqualFold(people.Key(p), key) {
			mine = append(mine, p)
		}
	}
	if pipeline > 0 {
		if pipes == nil {
			return serverclient.Person{}, "", shootPipeTarget{}, errors.New("пайплайн в фотосессии — только через очередь приложения")
		}
		title, models, err := pipes(pipeline)
		if err != nil {
			return serverclient.Person{}, "", shootPipeTarget{}, err
		}
		ov, err := resolvePerson(supported, name, models, faceSwap)
		if err != nil {
			return serverclient.Person{}, "", shootPipeTarget{}, err
		}
		person := mine[0]
		for _, p := range mine {
			if p.Name == ov.Name {
				person = p
			}
		}
		t := shootPipeTarget{pipeline: pipeline, name: title, loras: ov.Loras, swap: ov.FaceSwapName}
		if generation.ProseModels(models) {
			t.base = generation.ModelBase(models[0])
		}
		return person, key, t, nil
	}
	if p, ok := pickByBase(mine, name, base); ok {
		return p, key, shootPipeTarget{}, nil
	}
	bases := make([]string, 0, len(mine))
	for _, p := range mine {
		bases = append(bases, p.Base)
	}
	if base != "" {
		return serverclient.Person{}, "", shootPipeTarget{}, fmt.Errorf("«%s»: нет LoRA для %s; есть: %s", key, base, strings.Join(bases, ", "))
	}
	return serverclient.Person{}, "", shootPipeTarget{}, fmt.Errorf("«%s»: несколько LoRA (%s) — передай base или pipeline; что выбрать, решает человек", key, strings.Join(bases, ", "))
}

func pickByBase(mine []serverclient.Person, name, base string) (serverclient.Person, bool) {
	if base != "" {
		for _, p := range mine {
			if p.Base == base {
				return p, true
			}
		}
		return serverclient.Person{}, false
	}
	for _, p := range mine {
		if p.Name == name {
			return p, true
		}
	}
	if len(mine) == 1 {
		return mine[0], true
	}
	return serverclient.Person{}, false
}
