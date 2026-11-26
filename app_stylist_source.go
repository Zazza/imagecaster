package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/people"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
)

type StylistSourceShootRequest struct {
	target       *shootTarget
	Person       string                       `json:"person,omitempty"`
	Base         string                       `json:"base,omitempty"`
	FaceSwap     bool                         `json:"face_swap,omitempty"`
	ImageBase64  string                       `json:"image_base64"`
	Instructions []string                     `json:"instructions"`
	Editor       *generation.PostEditorParams `json:"editor"`
}

type StylistSourceShootResult struct {
	JobIDs []int64 `json:"job_ids"`
	Error  string  `json:"error,omitempty"`
}

type sourceVariantSessions interface {
	AddToSession(image string, info json.RawMessage, source string, meta *preset.GenerationMeta) int64
	SessionItem(id int64) (*preset.SessionItem, error)
}

func enqueueSourceVariants(st stylistSettings, q stylistQueue, sessions sourceVariantSessions, notLocal bool, req StylistSourceShootRequest) (*StylistSourceShootResult, error) {
	if notLocal {
		return nil, errStylistPageNotLocal
	}
	if err := generation.ValidateSourcePoseImage(req.ImageBase64); err != nil {
		return nil, err
	}
	if len(req.Instructions) < 1 || len(req.Instructions) > generation.SourcePoseMaxCount {
		return nil, errors.New("недопустимое число вариантов")
	}
	editor, err := generation.NormalizePostEditor(req.Editor)
	if err != nil {
		return nil, err
	}
	instructions := make([]string, len(req.Instructions))
	for i, instruction := range req.Instructions {
		instructions[i], err = stylistEditInstruction(instruction)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(req.Person) != "" && req.target == nil {
		return nil, errors.New("не выбрана модель лица")
	}
	if strings.TrimSpace(req.Person) == "" && (strings.TrimSpace(req.Base) != "" || req.FaceSwap) {
		return nil, errors.New("выберите человека для уточнения лица")
	}
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	id := sessions.AddToSession(req.ImageBase64, nil, stylistPageSource, &preset.GenerationMeta{Label: "Исходное фото"})
	if id <= 0 {
		return nil, errors.New("исходное фото не сохранено")
	}
	item, err := sessions.SessionItem(id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.ID != id || item.SessionID <= 0 {
		return nil, errors.New("исходное фото не найдено")
	}
	result := &StylistSourceShootResult{JobIDs: []int64{}}
	frames := []StylistFrame{}
	params := map[int64]shootParamsFrame{}
	for _, instruction := range instructions {
		p := mcp.ShootFrameParams{SourcePose: true, SourceItemID: id, SourceSessionID: item.SessionID, EditInstruction: instruction, Editor: editor, Model: editor.Model, Shot: instruction, ToFolder: true}
		if req.target != nil {
			t := req.target
			p.Person, p.Base = t.person.Name, t.base
			p.Prompt = stylist.ComposePrompt(t.lora, []string{t.person.Trigger}, "")
			p.FacePass = true
			p.FaceSwapName, p.FaceSwap = t.faceSwapName, t.faceSwapName != ""
		}
		label := clipRunes("Стилист: "+strings.TrimSpace(instruction), stylistPageLabelMax)
		jobID, err := q.EnqueueLabeled(queue.JobStylistFrame, p, stylistPageSource, label)
		if err != nil {
			result.Error = err.Error()
			break
		}
		result.JobIDs = append(result.JobIDs, jobID)
		frames = append(frames, StylistFrame{JobID: jobID, Status: string(queue.StatusPending), Shot: instruction})
		params[jobID] = shootParamsFrame{Label: label, Params: p}
		paramsErr := saveShootParams(st, params)
		framesErr := saveLastShoot(st, frames)
		if err := errors.Join(paramsErr, framesErr); err != nil {
			result.Error = err.Error()
			break
		}
	}
	return result, nil
}

func (a *App) StylistSourcePlan(p generation.SourcePosePlanParams) ([]string, error) {
	if a.gen == nil {
		return nil, errors.New("планировщик недоступен")
	}
	return a.gen.PlanSourcePoses(p)
}

func (a *App) StylistSourceShoot(req StylistSourceShootRequest) (*StylistSourceShootResult, error) {
	if a.presets == nil || a.sessions == nil || a.queueSvc == nil {
		return nil, errors.New("история или очередь недоступна")
	}
	sessionID, err := a.presets.GetActiveSessionID()
	if err != nil {
		return nil, err
	}
	if sessionID <= 0 {
		return nil, errors.New("выберите папку истории")
	}
	sessions := struct {
		generation.SessionAdder
		stylistEditItems
	}{a.sessions.PinnedTo(sessionID), a.sessions}
	if sdNotLocal(a.config) {
		return nil, errStylistPageNotLocal
	}
	if strings.TrimSpace(req.Person) != "" {
		ctx, cancel := a.peopleCtx(stylistPagePeopleWait)
		defer cancel()
		all, err := a.peopleSvc().List(ctx)
		if err != nil {
			return nil, fmt.Errorf("список людей с сервера: %w", err)
		}
		req.target, err = resolveSourcePerson(all, req)
		if err != nil {
			return nil, err
		}
	}
	return enqueueSourceVariants(a.presets, a.queueSvc, sessions, sdNotLocal(a.config), req)
}

func resolveSourcePerson(all []serverclient.Person, req StylistSourceShootRequest) (*shootTarget, error) {
	base := strings.TrimSpace(req.Base)
	if base == "" {
		key, ok := people.Find(all, strings.TrimSpace(req.Person))
		if !ok {
			return nil, fmt.Errorf("человек %q не найден на сервере", req.Person)
		}
		for _, person := range all {
			if !strings.EqualFold(people.Key(person), key) || person.Lora == "" {
				continue
			}
			if _, ok := stylist.Models[people.BaseOf(person)]; ok {
				base = people.BaseOf(person)
				break
			}
		}
		if base == "" {
			return nil, errors.New("нет подходящей LoRA человека для уточнения лица")
		}
	}
	return resolveShootTarget(all, nil, StylistShootRequest{Person: req.Person, Base: base, FaceSwap: req.FaceSwap})
}
