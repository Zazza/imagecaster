package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
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

type shootModel struct {
	model       string
	sampler     string
	steps       int
	cfg         float64
	faceDenoise float64
}

var shootModels = map[string]shootModel{
	"Z-Image":    {model: "z-image-turbo-fp8-aio", sampler: "euler", steps: 8, cfg: 1, faceDenoise: 0.4},
	"Moss-Image": {model: "moss_image_sft_bf16", sampler: "euler", steps: 30, cfg: 3, faceDenoise: 0.5},
	"SDXL":       {model: "juggernautXL_ragnarokBy", sampler: "dpm++2m_sde", steps: 30, cfg: 6, faceDenoise: 0.5},
	"Flux.2":     {model: "flux-2-klein-4b", sampler: "euler", steps: 4, cfg: 1, faceDenoise: 0.6},
}

const (
	shootWidth         = 1024
	shootHeight        = 1536
	shootMaxShots      = 6
	shootPeopleWait    = 20 * time.Second
	shootBackground    = "plain light grey studio backdrop, soft even studio light"
	shootQuality       = "BREAK, a candid photograph with natural proportions and honest imperfections: realistic skin texture, natural light, documentary framing, no stylization"
	shootQualityStudio = "BREAK, a professional studio portrait with realistic skin texture and natural pores, sharp focus, straight symmetrical framing, no stylization"
	shootNegative      = "deformed, extra limbs, extra fingers, blurry, text, watermark"
	shootDefaultOutfit = "wearing simple casual everyday clothes"
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
		if _, ok := shootModels[p.Base]; ok {
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

func shootPrompt(p serverclient.Person, appearance string, look Look, shot, background string) string {
	parts := []string{appearance}
	if strings.TrimSpace(look.Outfit) == "" {
		parts = append(parts, shootDefaultOutfit)
	}
	parts = append(parts, lookParts(look)...)
	quality := shootQuality
	if strings.Contains(strings.ToLower(background), "studio") {
		quality = shootQualityStudio
	}
	parts = append(parts, shot, background, quality)
	clean := make([]string, 0, len(parts))
	for _, v := range parts {
		if v = strings.TrimSpace(v); v != "" {
			clean = append(clean, v)
		}
	}
	return strings.Join(clean, ", ") + fmt.Sprintf(" <lora:%s:1>", p.Lora)
}

func unknownPerson(list []serverclient.Person, name string) error {
	names := make([]string, 0, len(list))
	for _, p := range list {
		names = append(names, p.Name)
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
	w, h     int
}

func planShoot(people StylistPeople, eng StylistEngine, args map[string]any) (*shootPlan, error) {
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
	var person serverclient.Person
	found := false
	for _, p := range all {
		if p.Name == name {
			person, found = p, true
			break
		}
	}
	if !found {
		return nil, unknownPerson(onlySupported(all), name)
	}
	m, ok := shootModels[person.Base]
	if !ok {
		return nil, fmt.Errorf("LoRA %q обучена под %s — фотосессия умеет только Z-Image, Moss-Image, SDXL и Flux.2", name, person.Base)
	}
	plan := &shootPlan{
		name: name, person: person, m: m, shots: shots, seed: optInt(args, "seed"),
		garment: garment, ge: ge, outfit: strings.TrimSpace(look.Outfit), refineOn: optBool(args, "face_pass", false),
		w: shootWidth, h: shootHeight,
	}
	if strings.EqualFold(strings.TrimSpace(argString(args, "orientation")), "landscape") {
		plan.w, plan.h = shootHeight, shootWidth
	}
	for _, shot := range shots {
		plan.prompts = append(plan.prompts, shootPrompt(person, appearance, *look, shot, background))
	}
	return plan, nil
}

func (p *shootPlan) frameSeed(i int) *int64 {
	if p.seed == nil {
		return nil
	}
	v := *p.seed + int64(i)
	return &v
}

type frameOut struct {
	img      string
	info     json.RawMessage
	garment  string
	facePass string
	stopped  bool
}

func shootFrame(shooter StylistShooter, eng StylistEngine, ge StylistGarmentEngine, refiner FaceRefiner, m shootModel, prompt string, seed *int64, garment, outfit string, refineOn bool, w, h int) (frameOut, error) {
	img, info, frameSeed, err := shootOne(shooter, generation.TestGenerateParams{
		Mode:           "models",
		SelectedModels: []string{m.model},
		Prompt:         prompt,
		NegativePrompt: shootNegative,
		Sampler:        m.sampler,
		Steps:          m.steps,
		CfgScale:       m.cfg,
		Width:          w,
		Height:         h,
		Seed:           seed,
	})
	if err != nil {
		return frameOut{}, err
	}
	out := frameOut{img: img, info: info}
	var passSeed *int64
	if frameSeed != 0 {
		passSeed = &frameSeed
	}
	if garment != "" {
		res, err := ge.KleinTryOn(out.img, garment, outfit, passSeed)
		switch {
		case err != nil && generation.IsInterrupted(err):
			out.stopped = true
			out.garment = facePassSkipped("прервано")
		case err != nil:
			out.garment = facePassSkipped(faceSkipReason(err))
		case res == nil || res.Image == "":
			out.garment = facePassSkipped("примерка не вернула картинку")
		default:
			out.img = res.Image
			out.garment = shootFaceDone
		}
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
	return out, nil
}

func stylistShoot(people StylistPeople, shooter StylistShooter, sessions AppSessions, eng StylistEngine, refiner FaceRefiner, args map[string]any) (string, error) {
	plan, err := planShoot(people, eng, args)
	if err != nil {
		return "", err
	}
	var variants []map[string]any
	var saved []string
	stopped := false
	for i, shot := range plan.shots {
		prompt := plan.prompts[i]
		out, err := shootFrame(shooter, eng, plan.ge, refiner, plan.m, prompt, plan.frameSeed(i), plan.garment, plan.outfit, plan.refineOn, plan.w, plan.h)
		if err != nil {
			if generation.IsInterrupted(err) {
				break
			}
			if len(saved) > 0 {
				return "", fmt.Errorf("кадр %d из %d не получился: %s; уже готовы кадры (id в сессии «Стилист»): %s", i+1, len(plan.shots), faceSkipReason(err), strings.Join(saved, ", "))
			}
			return "", fmt.Errorf("кадр %d из %d не получился: %s", i+1, len(plan.shots), faceSkipReason(err))
		}
		stopped = out.stopped
		meta := &preset.GenerationMeta{Model: plan.m.model, Label: truncateLabel("Стилист: " + plan.name + ", " + shot)}
		id := sessions.AddToNamedSession(StylistSessionName, out.img, out.info, "stylist", meta)
		saved = append(saved, fmt.Sprint(id))
		v := map[string]any{"id": id, "shot": shot, "prompt": prompt, "face_pass": out.facePass}
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
		"model":    plan.m.model,
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
		Description: "Стилист: список обученных LoRA людей на сервере (name, base — модель, под которую обучена, gender, created_at), только под модели, с которыми умеет stylist_shoot. " +
			"По имени не всегда понятно, кто это и какую из нескольких брать — покажи список человеку и спроси, какую LoRA использовать; сам не выбирай. Пусто — LoRA нет, работай правкой фото stylist_try.",
		InputSchema: props(map[string]any{}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			people, _, _, _, _, err := s.shootDeps()
			if err != nil {
				return "", err
			}
			all, err := allPeople(people)
			if err != nil {
				return "", err
			}
			list := onlySupported(all)
			out := make([]map[string]any, 0, len(list))
			for _, p := range list {
				out = append(out, map[string]any{"name": p.Name, "base": p.Base, "gender": p.Gender, "created_at": p.CreatedAt})
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
			"person — имя из stylist_people (какую LoRA брать, решает человек). appearance (по-английски) — только то, чего НЕТ в датасете LoRA: пол, телосложение, рост, татуировки и приметы; НЕ описывай возраст, тон кожи, лысину/бороду/причёску и черты лица — LoRA знает их из фото, промптовые токены искажают (возраст «старит», тон кожи перекрашивает, «bald» раздувает лоб). Совсем пустым appearance оставлять нельзя. " +
			"look — образ, как у stylist_try (одежда подробно: тип, силуэт, длина, вырез, рукава, ткань, цвет, принт, застёжки). shots — до 6 поз/ракурсов по-английски, по кадру на каждый (пусто — 4: спереди, вполоборота, сбоку, со спины); look и appearance одинаковы для всей серии, чтобы вещь была одна. background — фон (по умолчанию светлая студия). " +
			"face_pass=true — после генерации кадра лицо перерисовывается вторым проходом img2img с той же LoRA (крупный режим), серия идёт дольше; у каждого кадра поле face_pass: done | off | skipped:<причина>. " +
			"garment_path — фото вещи: на каждый кадр её надевает правка Klein по фото (стороны, которых нет на фото, модель дорисует сама), затем идёт проход лица; поле garment у кадра — done | skipped:<причина> (кадр без вещи), +6–11 с на кадр. Годится для 1–2 вещей; комплект из 3+ предметов примерка НЕ ТЯНЕТ (Klein калечит пропорции на любой базе) — такой лук опиши текстом подробно по каждой вещи (цвет/ткань/фасон с фото через локальную vision), без garment_path. Фото вещи лучше давать НА ЧЕЛОВЕКЕ. Цвет/узор/ткань текстом НЕ описывать — вещь с фото; но СТРУКТУРУ комплекта перечислять ВСЕ предметы поимённо, включая мелочи (a headscarf, stockings, a belt) — неназванный предмет Klein не переносит; если состав неясен — спроси у человека до съёмки. " +
			"Кадры не рисуются сразу: каждый ставится заданием в очередь приложения Imagecaster (там прогресс, «осталось», пауза и отмена; приложение закрыто — начнёт при запуске). Ответ: jobs — номера заданий по порядку кадров; готовность, id кадров в сессии, face_pass и garment — stylist_shoot_status(job_ids). Скажи человеку, что серия идёт в приложении, и проверяй статус, когда он спросит или через пару минут. Картинок и путей нет.",
		InputSchema: props(map[string]any{
			"person":       prop("имя обученного человека из stylist_people", "string"),
			"appearance":   prop("по-английски, только то, чего нет в датасете LoRA: пол, телосложение, рост, тату; без возраста, тона кожи и причёски", "string"),
			"look":         lookSchema,
			"shots":        prop("позы и ракурсы по-английски, 1–6, по кадру на каждый", "array", map[string]any{"items": map[string]any{"type": "string"}}),
			"background":   prop("фон по-английски (по умолчанию светлая студия)", "string"),
			"garment_path": prop("путь к фото вещи (лучше на человеке): Klein надевает её с фото; цвет/узор текстом не описывать, но структуру комплекта (top+bottom раздельно) — указать", "string"),
			"seed":         prop("сид первого кадра, дальше +1", "integer"),
			"orientation":  prop("ориентация кадра: portrait (по умолчанию) или landscape (1536×1024); подбирай по позам сам: лёжа/полёжа и широкие композиции — landscape, стоя/крупные планы — portrait", "string"),
			"face_pass":    prop("перерисовка лица вторым проходом, по умолчанию false (A/B 2026-11-18: сходство не улучшает, только сглаживает кожу); true — включить", "boolean"),
		}, "person", "appearance"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			people, shooter, sessions, eng, refiner, err := s.shootDeps()
			if err != nil {
				return "", err
			}
			if s.deps.Queue != nil {
				plan, err := planShoot(people, eng, args)
				if err != nil {
					return "", err
				}
				return enqueueShoot(s.deps.Queue, plan)
			}
			return stylistShoot(people, shooter, sessions, eng, refiner, args)
		},
	})
}
