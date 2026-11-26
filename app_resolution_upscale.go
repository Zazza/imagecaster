package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

const (
	resolutionSourceMaxDim  = 8192
	resolution1080Long      = 1920
	resolution2KLong        = 2560
	resolution4KLong        = 3840
	resolutionUpscaleSource = "upscale"
)

func (a *App) EnqueueImageResolutionUpscale(imageBase64 string, targetLong int) (int64, error) {
	if !validResolutionTarget(targetLong) {
		return 0, errors.New("неизвестный размер изображения")
	}
	if err := checkPostImage(imageBase64); err != nil {
		return 0, err
	}
	if a.queueSvc == nil {
		return 0, errors.New("очередь недоступна")
	}
	if sdNotLocal(a.config) {
		return 0, errPost4KNotLocal
	}
	img, err := imgcodec.Decode(imageBase64, imgcodec.Options{Name: "image", MaxDim: resolutionSourceMaxDim, DataURL: true})
	if err != nil {
		return 0, err
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if max(w, h) >= targetLong {
		return 0, errors.New("изображение уже имеет этот размер или больше")
	}
	info, err := json.Marshal(map[string]int{"width": w, "height": h})
	if err != nil {
		return 0, err
	}
	label := fmt.Sprintf("%d px", targetLong)
	return a.queueSvc.EnqueueLabeled(queue.JobResolutionUpscale, resolutionUpscaleParams{ImageBase64: imgcodec.StripDataURL(imageBase64), TargetLong: targetLong, Info: info, Meta: preset.GenerationMeta{Label: label}}, resolutionUpscaleSource, label)
}

type resolutionUpscaleParams struct {
	ImageBase64 string                `json:"image_base64"`
	TargetLong  int                   `json:"target_long"`
	Info        json.RawMessage       `json:"info"`
	Meta        preset.GenerationMeta `json:"meta"`
}

func validResolutionTarget(target int) bool {
	return target == resolution1080Long || target == resolution2KLong || target == resolution4KLong
}

func (a *App) GetQueueJobResult(id int64) (*queue.Job, error) {
	if a.queueSvc == nil {
		return nil, errors.New("очередь недоступна")
	}
	jobs, err := a.queueSvc.Results([]int64{id})
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, errors.New("задание не найдено")
	}
	return jobs[0], nil
}

func (a *App) EnqueueResolutionUpscale(itemID int64, targetLong int) (int64, error) {
	if !validResolutionTarget(targetLong) {
		return 0, errors.New("неизвестный размер изображения")
	}
	if a.presets == nil || a.sessions == nil || a.queueSvc == nil {
		return 0, errors.New("история недоступна")
	}
	if sdNotLocal(a.config) {
		return 0, errPost4KNotLocal
	}
	item, err := a.presets.GetSessionItem(itemID)
	if err != nil {
		return 0, err
	}
	if item == nil {
		return 0, errors.New("кадра нет в истории")
	}
	if max(item.Width, item.Height) >= targetLong {
		return 0, errors.New("изображение уже имеет этот размер или больше")
	}
	image, err := a.sessions.GetSessionImage(itemID)
	if err != nil {
		return 0, err
	}
	info, err := sessionItemGenInfo(item)
	if err != nil {
		return 0, err
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(info), &fields); err != nil {
		return 0, err
	}
	fields["denoising_strength"] = item.Denoising
	rawInfo, err := json.Marshal(fields)
	if err != nil {
		return 0, err
	}
	meta := preset.GenerationMeta{PresetID: item.PresetID}
	if item.Meta != nil {
		meta = *item.Meta
	}
	meta.SourceItem = itemID
	meta.Label = fmt.Sprintf("%s · %d px", meta.Label, targetLong)
	return a.queueSvc.EnqueueLabeled(queue.JobResolutionUpscale, resolutionUpscaleParams{ImageBase64: image, TargetLong: targetLong, Info: rawInfo, Meta: meta}, resolutionUpscaleSource, meta.Label)
}

func resolutionUpscaleHandler() queue.Handler {
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		var p resolutionUpscaleParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, err
		}
		if gen == nil || !validResolutionTarget(p.TargetLong) {
			return nil, errors.New("апскейл недоступен или размер неизвестен")
		}
		out, err := gen.UpscaleLongStrict(p.ImageBase64, p.TargetLong)
		if err != nil {
			return nil, err
		}
		if !out.Upscaled {
			return nil, errors.New("изображение уже имеет этот размер или больше")
		}
		var info map[string]any
		if err := json.Unmarshal(p.Info, &info); err != nil {
			return nil, err
		}
		if info == nil {
			return nil, errors.New("нет параметров исходного изображения")
		}
		info["width"], info["height"] = out.Width, out.Height
		raw, err := json.Marshal(info)
		if err != nil {
			return nil, err
		}
		if gen.Stopped() {
			return nil, generation.ErrInterrupted
		}
		id := gen.AddToSession(out.Image, raw, resolutionUpscaleSource, &p.Meta)
		if id == 0 {
			return nil, errors.New("не удалось сохранить увеличенное изображение в историю")
		}
		return &queue.JobResult{ImageBase64: out.Image, SessionItemID: id, Info: string(raw)}, nil
	}
}
