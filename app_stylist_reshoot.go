package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/stylist"
)

const (
	stylistShootParamsKey = "stylist_last_shoot_params"
	stylistPlanKey        = "stylist_last_plan"
	stylistGarmentKey     = "stylist_last_garment"
	stylistPlanMax        = 2 << 20
)

var errStylistReshootGone = errors.New("этот кадр нельзя переснять: его параметров нет (кадр не из последней съёмки)")

type shootParamsStore struct {
	Garment string                      `json:"garment,omitempty"`
	Frames  map[string]shootParamsFrame `json:"frames"`
}

type shootParamsFrame struct {
	Label   string               `json:"label"`
	Garment bool                 `json:"garment,omitempty"`
	Params  mcp.ShootFrameParams `json:"params"`
}

type recordingQueue struct {
	q      stylistQueue
	frames map[int64]shootParamsFrame
}

func (r *recordingQueue) EnqueueLabeled(jobType queue.JobType, params any, source, label string) (int64, error) {
	id, err := r.q.EnqueueLabeled(jobType, params, source, label)
	if err == nil {
		if p, ok := params.(mcp.ShootFrameParams); ok {
			if r.frames == nil {
				r.frames = map[int64]shootParamsFrame{}
			}
			r.frames[id] = shootParamsFrame{Label: label, Params: p}
		}
	}
	return id, err
}

func saveShootParams(st stylistSettings, frames map[int64]shootParamsFrame) error {
	out := shootParamsStore{Frames: make(map[string]shootParamsFrame, len(frames))}
	for id, f := range frames {
		if f.Params.GarmentBase64 != "" {
			out.Garment = f.Params.GarmentBase64
			f.Params.GarmentBase64 = ""
			f.Garment = true
		}
		out.Frames[strconv.FormatInt(id, 10)] = f
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return st.SetSetting(stylistShootParamsKey, string(raw))
}

func loadShootParams(st stylistSettings) (shootParamsStore, error) {
	var out shootParamsStore
	raw, err := st.GetSetting(stylistShootParamsKey)
	if err != nil {
		return out, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return out, fmt.Errorf("параметры последней съёмки: %w", err)
		}
	}
	if out.Frames == nil {
		out.Frames = map[string]shootParamsFrame{}
	}
	return out, nil
}

func reshootFrame(st stylistSettings, q stylistQueue, notLocal bool, jobID int64, size string) (int64, error) {
	if notLocal {
		return 0, errStylistPageNotLocal
	}
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	frames := loadLastShoot(st)
	idx := -1
	for i, f := range frames {
		if f.JobID == jobID {
			idx = i
		}
	}
	store, err := loadShootParams(st)
	if err != nil {
		return 0, err
	}
	key := strconv.FormatInt(jobID, 10)
	pf, ok := store.Frames[key]
	if idx < 0 || !ok {
		return 0, errStylistReshootGone
	}
	p := pf.Params
	p.GarmentBase64 = ""
	if pf.Garment && store.Garment != "" {
		p.GarmentBase64 = store.Garment
	}
	if size != "" {
		p.Width, p.Height = stylist.FrameDims(size, p.Width > p.Height)
	}
	seed := rand.Int64N(1 << 53)
	p.Seed = &seed
	id, err := q.EnqueueLabeled(queue.JobStylistFrame, p, stylistPageSource, pf.Label)
	if err != nil {
		return 0, fmt.Errorf("кадр не поставлен в очередь: %w", err)
	}
	frames[idx] = StylistFrame{JobID: id, Status: string(queue.StatusPending)}
	if err := saveLastShoot(st, frames); err != nil {
		return id, err
	}
	delete(store.Frames, key)
	pf.Params.Seed = &seed
	pf.Params.Width, pf.Params.Height = p.Width, p.Height
	store.Frames[strconv.FormatInt(id, 10)] = pf
	raw, err := json.Marshal(store)
	if err != nil {
		return id, err
	}
	return id, st.SetSetting(stylistShootParamsKey, string(raw))
}

func loadStylistPlan(st stylistSettings) (string, error) {
	return st.GetSetting(stylistPlanKey)
}

func saveStylistPlan(st stylistSettings, plan string) error {
	if len(plan) > stylistPlanMax {
		return fmt.Errorf("план слишком большой: %d байт", len(plan))
	}
	if plan != "" && !json.Valid([]byte(plan)) {
		return errors.New("план: не JSON")
	}
	return st.SetSetting(stylistPlanKey, plan)
}

func saveStylistGarment(st stylistSettings, b64 string) error {
	if len(b64) > config.MaxImageBase64Len {
		return config.ImageTooLarge("image")
	}
	small, err := shrinkGarment(b64)
	if err != nil {
		return err
	}
	return st.SetSetting(stylistGarmentKey, small)
}

func (a *App) StylistSaveGarment(b64 string) error {
	if a.presets == nil {
		return errStylistNoDB
	}
	return saveStylistGarment(a.presets, b64)
}

func (a *App) StylistLoadGarment() (string, error) {
	if a.presets == nil {
		return "", nil
	}
	return a.presets.GetSetting(stylistGarmentKey)
}

func (a *App) StylistReshoot(jobID int64, size string) (int64, error) {
	if a.presets == nil || a.queueSvc == nil {
		return 0, errStylistNoDB
	}
	return reshootFrame(a.presets, a.queueSvc, sdNotLocal(a.config), jobID, size)
}

func (a *App) StylistLoadPlan() (string, error) {
	if a.presets == nil {
		return "", nil
	}
	return loadStylistPlan(a.presets)
}

func (a *App) StylistSavePlan(plan string) error {
	if a.presets == nil {
		return errStylistNoDB
	}
	return saveStylistPlan(a.presets, plan)
}
