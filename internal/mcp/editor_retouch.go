package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"math"
	"strconv"
	"strings"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

const (
	retouchTarget4K  = 3840
	retouchMaxItems  = 12
	retouchMax2x     = 7680
	retouchMaxSource = 8192
	retouchQueueHint = "Ретушь выполнит приложение Imagecaster по очереди: прогресс, пауза и отмена — в панели очереди приложения; если приложение закрыто, задания начнутся при его запуске. Один кадр в 4K — около 30 секунд. Готовность — editor_retouch_status с этими job_ids."
)

type RetouchEngine interface {
	UpscaleLong(imageBase64 string, targetLong int) (*generation.UpscaleResult, error)
	FaceLandmarks(imageBase64 string) ([]faceguard.Point, error)
}

type RetouchJobParams struct {
	ItemID      int64              `json:"item_id,omitempty"`
	ImageBase64 string             `json:"image_base64,omitempty"`
	Size        string             `json:"size"`
	Adjust      *adjust.Params     `json:"adjust,omitempty"`
	Retouch     *faceguard.Retouch `json:"retouch,omitempty"`
	Label       string             `json:"label"`
	Index       int                `json:"index"`
	Total       int                `json:"total"`
}

type RetouchResult struct {
	ID      int64  `json:"id"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Upscale string `json:"upscale"`
	Adjust  bool   `json:"adjust,omitempty"`
	Retouch string `json:"retouch,omitempty"`
}

func retouchTarget(size string, long int) (int, error) {
	switch size {
	case "", "4k":
		return retouchTarget4K, nil
	case "2x":
		return min(long*2, retouchMax2x), nil
	case "off":
		return 0, nil
	}
	return 0, errors.New("size: \"4k\", \"2x\" или \"off\"")
}

func retouchSource(sessions AppSessions, p RetouchJobParams) (string, *preset.SessionItem, error) {
	if p.ItemID <= 0 {
		if p.ImageBase64 == "" {
			return "", nil, errors.New("ретушь: нет фото")
		}
		return p.ImageBase64, nil, nil
	}
	items, ok := sessions.(AppSessionItems)
	if !ok {
		return "", nil, errors.New("фото из приложения недоступны в этой сборке")
	}
	item, err := items.SessionItem(p.ItemID)
	if err != nil || item == nil {
		return "", nil, fmt.Errorf("кадр %d не найден в приложении", p.ItemID)
	}
	img, err := items.GetSessionImage(item.ID)
	if err != nil || img == "" {
		return "", nil, fmt.Errorf("кадр %d: файл не прочитан", p.ItemID)
	}
	return img, item, nil
}

func RunRetouch(eng RetouchEngine, sessions AppSessions, p RetouchJobParams, stopped func() bool) (*RetouchResult, error) {
	if eng == nil || sessions == nil {
		return nil, errors.New("ретушь: нет генерации или сессий")
	}
	if stopped == nil {
		stopped = func() bool { return false }
	}
	src, item, err := retouchSource(sessions, p)
	if err != nil {
		return nil, err
	}
	srcImg, err := decodeImage(src)
	if err != nil {
		return nil, errors.New("ретушь: фото не читается")
	}
	sw, sh := srcImg.Bounds().Dx(), srcImg.Bounds().Dy()
	target, err := retouchTarget(p.Size, max(sw, sh))
	if err != nil {
		return nil, err
	}
	res := &RetouchResult{Upscale: "off", Width: sw, Height: sh}
	if target > 0 && max(sw, sh) >= target {
		res.Upscale = "skipped:фото уже не меньше цели"
		target = 0
	}
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	cur := src
	if target > 0 {
		out, err := eng.UpscaleLong(src, target)
		if err != nil {
			if generation.IsInterrupted(err) {
				return nil, err
			}
			return nil, errors.New("ретушь: " + faceSkipReason(err))
		}
		cur, res.Width, res.Height = out.Image, out.Width, out.Height
		if out.Upscaled {
			res.Upscale = "done"
			if out.Resized {
				res.Upscale = "done:обычный ресайз — на сервере нет ESRGAN или он не сработал"
			}
		} else {
			res.Upscale = "skipped:увеличение не сделано"
		}
	}
	if p.Adjust != nil && !p.Adjust.IsNeutral() {
		img, err := decodeImage(cur)
		if err != nil {
			return nil, errors.New("ретушь: результат не читается")
		}
		enc, err := imgcodec.EncodePNG(adjust.Apply(img, *p.Adjust), png.BestSpeed)
		if err != nil {
			return nil, err
		}
		cur, res.Adjust = enc, true
	}
	if p.Retouch != nil && !p.Retouch.IsZero() {
		cur, res.Retouch = retouchFace(eng, src, cur, *p.Retouch)
	}
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	res.ID = saveRetouch(sessions, item, cur, p.Label)
	if res.ID == 0 {
		return nil, errors.New("ретушь: результат не сохранён в приложении")
	}
	return res, nil
}

func retouchFace(eng RetouchEngine, src, img string, r faceguard.Retouch) (string, string) {
	note := ""
	var pts []faceguard.Point
	if r.Local() {
		got, err := eng.FaceLandmarks(src)
		switch {
		case err == nil:
			pts = got
		case r.Light:
			r, note = faceguard.Retouch{Light: true}, "сделан только свет: лицо не найдено"
		default:
			return img, "ретушь не сделана: лицо не найдено"
		}
	}
	out, err := faceguard.ApplyRetouchBase64(img, pts, r)
	if err != nil {
		return img, "ретушь не сделана: " + err.Error()
	}
	if note == "" {
		note = "done"
	}
	return out, note
}

func saveRetouch(sessions AppSessions, item *preset.SessionItem, img, label string) int64 {
	if item == nil {
		meta := &preset.GenerationMeta{Label: truncateLabel("Ретушь: фото")}
		return sessions.AddToNamedSession(EditorSessionName, img, nil, editorSource, meta)
	}
	meta := &preset.GenerationMeta{Label: truncateLabel("Ретушь: " + retouchItemLabel(item)), SourceItem: item.ID}
	if items, ok := sessions.(AppSessionItems); ok {
		return items.AddToSessionID(item.SessionID, img, nil, editorSource, meta)
	}
	return 0
}

func retouchItemLabel(item *preset.SessionItem) string {
	if item.Meta != nil && strings.TrimSpace(item.Meta.Label) != "" {
		return strings.TrimPrefix(strings.TrimSpace(item.Meta.Label), "Ретушь: ")
	}
	return "кадр " + strconv.FormatInt(item.ID, 10)
}

func NewRetouchHandler(sessions AppSessions) queue.Handler {
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if gen == nil {
			return nil, errors.New("ретушь: генерация недоступна")
		}
		var p RetouchJobParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		res, err := RunRetouch(gen, sessions, p, gen.Stopped)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return &queue.JobResult{Frame: raw}, nil
	}
}

type retouchJob struct {
	params RetouchJobParams
	label  string
}

func retouchItem(items AppSessionItems, raw any) (*preset.SessionItem, error) {
	var item *preset.SessionItem
	var err error
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		if strings.EqualFold(v, "active") {
			item, err = items.GetActiveSessionItem()
			break
		}
		id, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil || id < 1 {
			return nil, errors.New("item: \"active\" или id элемента сессии")
		}
		item, err = items.SessionItem(id)
	case float64:
		if v < 1 || v != math.Trunc(v) {
			return nil, errors.New("id кадра — целое положительное число")
		}
		item, err = items.SessionItem(int64(v))
	default:
		return nil, errors.New("item: \"active\" или числовой id элемента сессии")
	}
	if err != nil {
		return nil, errors.New("фото из приложения: не удалось прочитать элемент сессии")
	}
	if item == nil {
		return nil, errors.New("в приложении не выбрано фото (или такого элемента нет)")
	}
	return item, nil
}

func retouchSources(args map[string]any, sessions AppSessions) ([]retouchJob, error) {
	path := argString(args, "photo_path")
	rawItem, hasItem := args["item"]
	hasItem = hasItem && rawItem != nil
	rawItems, hasItems := args["items"]
	hasItems = hasItems && rawItems != nil
	n := 0
	for _, has := range []bool{path != "", hasItem, hasItems} {
		if has {
			n++
		}
	}
	if n == 0 {
		return nil, errors.New("нужен photo_path, item или items")
	}
	if n > 1 {
		return nil, errors.New("укажи что-то одно: photo_path, item или items")
	}
	if path != "" {
		img, err := filebrowser.ReadFileAsBase64(path)
		if err != nil {
			return nil, errors.New("не удалось прочитать фото по photo_path — проверь путь и формат")
		}
		raw, err := base64.StdEncoding.DecodeString(img)
		if err != nil {
			return nil, errors.New("не удалось прочитать фото по photo_path — проверь путь и формат")
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			return nil, errors.New("не удалось прочитать фото по photo_path — проверь путь и формат")
		}
		if err := retouchSizeOK(cfg.Width, cfg.Height); err != nil {
			return nil, err
		}
		return []retouchJob{{params: RetouchJobParams{ImageBase64: img}, label: "фото"}}, nil
	}
	items, ok := sessions.(AppSessionItems)
	if !ok {
		return nil, errors.New("фото из приложения недоступны в этой сборке imagecaster-mcp")
	}
	var raws []any
	if hasItem {
		raws = []any{rawItem}
	} else {
		list, ok := rawItems.([]any)
		if !ok || len(list) == 0 || len(list) > retouchMaxItems {
			return nil, fmt.Errorf("items — список id кадров, от 1 до %d", retouchMaxItems)
		}
		for _, v := range list {
			if _, isNum := v.(float64); !isNum {
				return nil, errors.New("items — только целые id кадров")
			}
		}
		raws = list
	}
	out := make([]retouchJob, 0, len(raws))
	for _, raw := range raws {
		item, err := retouchItem(items, raw)
		if err != nil {
			return nil, err
		}
		if err := retouchSizeOK(item.Width, item.Height); err != nil {
			return nil, fmt.Errorf("кадр %d: %w", item.ID, err)
		}
		out = append(out, retouchJob{params: RetouchJobParams{ItemID: item.ID}, label: retouchItemLabel(item)})
	}
	return out, nil
}

func retouchSizeOK(w, h int) error {
	if max(w, h) > retouchMaxSource {
		return fmt.Errorf("фото %d×%d больше %d px по стороне — ретушь не возьмёт", w, h, retouchMaxSource)
	}
	return nil
}

func retouchOptions(args map[string]any) (RetouchJobParams, error) {
	var p RetouchJobParams
	p.Size = "4k"
	if v, ok := args["size"]; ok && v != nil {
		s, isStr := v.(string)
		if !isStr {
			return p, errors.New("size: \"4k\", \"2x\" или \"off\"")
		}
		p.Size = strings.ToLower(strings.TrimSpace(s))
		if _, err := retouchTarget(p.Size, 1); err != nil || p.Size == "" {
			return p, errors.New("size: \"4k\", \"2x\" или \"off\"")
		}
	}
	adj, hasAdj, err := parseEditorAdjust(args["adjust"])
	if err != nil {
		return p, err
	}
	if hasAdj {
		p.Adjust = &adj
	}
	r, err := decodeArg[faceguard.Retouch](args, "retouch")
	if err != nil {
		return p, err
	}
	if r != nil {
		if r.Skin() {
			return p, errors.New("retouch: skin_tone и skin_lightness в ретуши не применяются")
		}
		if err := r.Validate(); err != nil {
			return p, fmt.Errorf("retouch: %w", err)
		}
		if !r.IsZero() {
			p.Retouch = r
		}
	}
	if p.Size == "off" && p.Adjust == nil && p.Retouch == nil {
		return p, errors.New("нечего делать: укажи size, adjust или retouch")
	}
	return p, nil
}

func enqueueRetouch(q ShootQueue, jobs []retouchJob, opts RetouchJobParams) (string, error) {
	total := len(jobs)
	ids := make([]int64, 0, total)
	for i, j := range jobs {
		p := opts
		p.ItemID, p.ImageBase64 = j.params.ItemID, j.params.ImageBase64
		p.Index, p.Total = i+1, total
		p.Label = truncateLabel("Ретушь: "+j.label) + fmt.Sprintf(" (%d из %d)", i+1, total)
		raw, err := json.Marshal(p)
		if err != nil {
			return "", err
		}
		id, err := q.CreateLabeled(queue.JobRetouch, string(raw), editorSource, p.Label)
		if err != nil {
			if len(ids) > 0 {
				return "", fmt.Errorf("кадр %d из %d не поставлен в очередь: %v; уже в очереди job_ids: %v", i+1, total, err, ids)
			}
			return "", fmt.Errorf("кадр %d из %d не поставлен в очередь: %v", i+1, total, err)
		}
		ids = append(ids, id)
	}
	return toJSON(map[string]any{"jobs": ids, "hint": retouchQueueHint}), nil
}

func retouchStatus(q ShootQueue, ids []int64) (string, error) {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		job, err := q.Get(id)
		if err != nil {
			return "", err
		}
		if job == nil || job.Type != queue.JobRetouch {
			out = append(out, map[string]any{"job_id": id, "status": "not_found"})
			continue
		}
		var p RetouchJobParams
		_ = json.Unmarshal([]byte(job.Params), &p)
		f := map[string]any{"job_id": id, "status": string(job.Status), "index": p.Index, "total": p.Total}
		if job.Status == queue.StatusCompleted {
			var r queue.JobResult
			var rr RetouchResult
			if json.Unmarshal([]byte(job.Result), &r) == nil && json.Unmarshal(r.Frame, &rr) == nil {
				f["id"], f["width"], f["height"] = rr.ID, rr.Width, rr.Height
				f["upscale"] = rr.Upscale
				if rr.Adjust {
					f["adjust"] = true
				}
				if rr.Retouch != "" {
					f["retouch"] = rr.Retouch
				}
			}
		}
		if job.Error != "" {
			msg := strings.ReplaceAll(job.Error, "\n", " ")
			if r := []rune(msg); len(r) > shootErrorMax {
				msg = string(r[:shootErrorMax]) + "…"
			}
			f["error"] = msg
		}
		out = append(out, f)
	}
	return toJSON(map[string]any{"jobs": out}), nil
}

func registerRetouchTools(s *Server) {
	s.Register(Tool{
		Name: "editor_retouch",
		Description: "Фоторедактор: ретушь кадров — увеличение до 4K, свет/цвет (adjust) и ретушь глаз и зубов (retouch); кадр не перерисовывается, лицо не меняется, работает на кадрах любой модели и на фото с камеры. Каждое фото — задание очереди приложения; ответ сразу — job_ids, готовность — editor_retouch_status. " +
			"Фото — photo_path, item (\"active\" или id) или items (список id, до 12; кадры стилиста тоже можно). size: \"4k\" (по умолчанию, длинная сторона 3840), \"2x\" или \"off\". Результат — новый кадр рядом с исходником. Ответ без картинок и путей.",
		InputSchema: props(map[string]any{
			"photo_path": prop("путь к фото (или item / items)", "string"),
			"item":       itemSchema,
			"items":      prop("id кадров из приложения (до 12)", "array", map[string]any{"items": map[string]any{"type": "integer"}}),
			"size":       prop("\"4k\" (по умолчанию), \"2x\" или \"off\"", "string"),
			"adjust":     prop("свет и цвет числами — как adjust в editor_apply", "object"),
			"retouch":    prop("under_eyes, teeth, eye_whites, eye_color, light, face_width — как retouch у стилиста (без skin_tone и skin_lightness)", "object"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if s.deps.Queue == nil || s.deps.Retoucher == nil || s.deps.Sessions == nil {
				return "", errors.New("ретушь недоступна в этой сборке imagecaster-mcp")
			}
			if err := s.localOnly("фоторедактор"); err != nil {
				return "", err
			}
			opts, err := retouchOptions(args)
			if err != nil {
				return "", err
			}
			jobs, err := retouchSources(args, s.deps.Sessions)
			if err != nil {
				return "", err
			}
			return enqueueRetouch(s.deps.Queue, jobs, opts)
		},
	})
	s.Register(Tool{
		Name:        "editor_retouch_status",
		Description: "Фоторедактор: состояние заданий ретуши из ответа editor_retouch. По каждому: status (pending, running, completed, failed, cancelled, paused, not_found), index/total, у готового — id нового кадра, width/height, upscale (done | off | skipped:<причина>), adjust, retouch; у неудачного — error. Картинок и путей нет.",
		InputSchema: props(map[string]any{
			"job_ids": prop("номера заданий из ответа editor_retouch", "array", map[string]any{"items": map[string]any{"type": "integer"}}),
		}, "job_ids"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if s.deps.Queue == nil {
				return "", errors.New("очередь приложения недоступна в этой сборке imagecaster-mcp")
			}
			ids, err := shootJobIDs(args)
			if err != nil {
				return "", err
			}
			return retouchStatus(s.deps.Queue, ids)
		},
	})
}
