package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

const (
	shootQueueSource = "stylist"
	shootQueueHint   = "Кадры выполнит приложение Imagecaster по очереди: прогресс, пауза и отмена — в панели очереди приложения; если приложение закрыто, кадры начнутся при его запуске. Готовность — stylist_shoot_status с этими job_ids."
	shootErrorMax    = 200
)

type ShootQueue interface {
	CreateLabeled(jobType queue.JobType, params, source, label string) (int64, error)
	Get(id int64) (*queue.Job, error)
}

type ShootFrameParams struct {
	Person        string `json:"person"`
	Base          string `json:"base"`
	Model         string `json:"model"`
	Shot          string `json:"shot"`
	Prompt        string `json:"prompt"`
	Index         int    `json:"index"`
	Total         int    `json:"total"`
	Seed          *int64 `json:"seed,omitempty"`
	GarmentBase64 string `json:"garment_base64,omitempty"`
	Outfit        string `json:"outfit,omitempty"`
	FacePass      bool   `json:"face_pass"`
	FaceSwap      bool   `json:"face_swap,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
}

type ShootFrameResult struct {
	ID       int64  `json:"id"`
	Shot     string `json:"shot"`
	Prompt   string `json:"prompt"`
	FacePass string `json:"face_pass"`
	Garment  string `json:"garment,omitempty"`
	FaceSwap string `json:"face_swap,omitempty"`
}

func enqueueShoot(q ShootQueue, plan *shootPlan) (string, error) {
	total := len(plan.shots)
	ids := make([]int64, 0, total)
	for i, shot := range plan.shots {
		p := ShootFrameParams{
			Person: plan.name, Base: plan.person.Base, Model: plan.m.Model, Shot: shot, Prompt: plan.prompts[i],
			Index: i + 1, Total: total, Seed: plan.frameSeed(i),
			GarmentBase64: plan.garment, Outfit: plan.outfit, FacePass: plan.refineOn, FaceSwap: plan.faceSwap,
			Width: plan.w, Height: plan.h,
		}
		raw, err := json.Marshal(p)
		if err != nil {
			return "", err
		}
		label := truncateLabel("Стилист: "+plan.name+", "+shot) + fmt.Sprintf(" (%d из %d)", i+1, total)
		id, err := q.CreateLabeled(queue.JobStylistFrame, string(raw), shootQueueSource, label)
		if err != nil {
			if len(ids) > 0 {
				return "", fmt.Errorf("кадр %d из %d не поставлен в очередь: %v; уже в очереди job_ids: %v", i+1, total, err, ids)
			}
			return "", fmt.Errorf("кадр %d из %d не поставлен в очередь: %v", i+1, total, err)
		}
		ids = append(ids, id)
	}
	return toJSON(map[string]any{
		"person": plan.name,
		"base":   plan.person.Base,
		"model":  plan.m.Model,
		"jobs":   ids,
		"hint":   shootQueueHint,
	}), nil
}

func RunShootFrame(shooter StylistShooter, sessions AppSessions, eng StylistEngine, refiner FaceRefiner, p ShootFrameParams) (*ShootFrameResult, error) {
	return runShootFrame(shooter, sessions, eng, refiner, p, nil)
}

func runShootFrame(shooter StylistShooter, sessions AppSessions, eng StylistEngine, refiner FaceRefiner, p ShootFrameParams, stopped func() bool) (*ShootFrameResult, error) {
	m, ok := shootModels[p.Base]
	if !ok {
		return nil, fmt.Errorf("модель %q фотосессия не умеет", p.Base)
	}
	if shooter == nil || sessions == nil {
		return nil, errors.New("кадр стилиста: нет генерации или сессий")
	}
	var ge StylistGarmentEngine
	if p.GarmentBase64 != "" {
		var ok bool
		if ge, ok = eng.(StylistGarmentEngine); !ok {
			return nil, errors.New("примерка вещи недоступна в этой сборке")
		}
	}
	w, h := p.Width, p.Height
	if w == 0 || h == 0 {
		w, h = shootWidth, shootHeight
	}
	out, err := shootFrame(shooter, eng, ge, refiner, m, p.Prompt, p.Seed, p.GarmentBase64, p.Outfit, p.FacePass, w, h)
	if err != nil {
		if generation.IsInterrupted(err) {
			return nil, err
		}
		return nil, errors.New(faceSkipReason(err))
	}
	shootFaceSwap(eng, &out, p.Person, p.FaceSwap)
	if out.stopped || (stopped != nil && stopped()) {
		return nil, generation.ErrInterrupted
	}
	meta := &preset.GenerationMeta{Model: m.Model, Label: truncateLabel("Стилист: " + p.Person + ", " + p.Shot)}
	id := sessions.AddToNamedSession(StylistSessionName, out.img, out.info, "stylist", meta)
	return &ShootFrameResult{ID: id, Shot: p.Shot, Prompt: p.Prompt, FacePass: out.facePass, Garment: out.garment, FaceSwap: out.faceSwap}, nil
}

func NewShootFrameHandler(sessions AppSessions) queue.Handler {
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if gen == nil {
			return nil, errors.New("кадр стилиста: генерация недоступна")
		}
		var p ShootFrameParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		res, err := runShootFrame(gen.WithSessions(DiscardSessions{}), sessions, gen, gen, p, gen.Stopped)
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

func shootJobIDs(args map[string]any) ([]int64, error) {
	raw, ok := args["job_ids"].([]any)
	if !ok || len(raw) == 0 {
		return nil, errors.New("job_ids — непустой список номеров заданий из ответа stylist_shoot")
	}
	ids := make([]int64, 0, len(raw))
	for _, v := range raw {
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			return nil, errors.New("job_ids — только целые номера заданий")
		}
		ids = append(ids, int64(f))
	}
	return ids, nil
}

func shootStatus(q ShootQueue, ids []int64) (string, error) {
	frames := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		job, err := q.Get(id)
		if err != nil {
			return "", err
		}
		if job == nil || job.Type != queue.JobStylistFrame {
			frames = append(frames, map[string]any{"job_id": id, "status": "not_found"})
			continue
		}
		var p ShootFrameParams
		_ = json.Unmarshal([]byte(job.Params), &p)
		f := map[string]any{"job_id": id, "status": string(job.Status), "shot": p.Shot, "index": p.Index, "total": p.Total}
		if job.Status == queue.StatusCompleted {
			var r queue.JobResult
			var fr ShootFrameResult
			if json.Unmarshal([]byte(job.Result), &r) == nil && json.Unmarshal(r.Frame, &fr) == nil {
				f["id"] = fr.ID
				f["face_pass"] = fr.FacePass
				if fr.Garment != "" {
					f["garment"] = fr.Garment
				}
				f["face_swap"] = shootSwapOff
				if fr.FaceSwap != "" {
					f["face_swap"] = fr.FaceSwap
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
		frames = append(frames, f)
	}
	return toJSON(map[string]any{"frames": frames}), nil
}

func registerShootQueueTools(s *Server) {
	s.Register(Tool{
		Name: "stylist_shoot_status",
		Description: "Стилист: состояние кадров фотосессии, поставленных stylist_shoot в очередь приложения. job_ids — номера из ответа stylist_shoot. " +
			"По каждому: status (pending — ждёт, running — рисуется, completed — готов, failed — не получился или отменён, cancelled — снят, paused — повторы исчерпаны, not_found — нет такого кадра), shot, index/total, у готового — id в сессии «Стилист», face_pass, face_swap и garment, у неудачного — error. Картинок и путей нет.",
		InputSchema: props(map[string]any{
			"job_ids": prop("номера заданий из ответа stylist_shoot", "array", map[string]any{"items": map[string]any{"type": "integer"}}),
		}, "job_ids"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if s.deps.Queue == nil {
				return "", errors.New("очередь приложения недоступна в этой сборке imagecaster-mcp")
			}
			ids, err := shootJobIDs(args)
			if err != nil {
				return "", err
			}
			return shootStatus(s.deps.Queue, ids)
		},
	})
}
