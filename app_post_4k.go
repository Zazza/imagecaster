package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/queue"
)

const post4KLabel = "До 4K без перерисовки"

type post4KParams struct {
	ImageBase64 string `json:"image_base64"`
}

func enqueuePost4K(q postSaveQueue, notLocal bool, imageBase64 string) (int64, error) {
	if err := checkPostImage(imageBase64); err != nil {
		return 0, err
	}
	if notLocal {
		return 0, errPost4KNotLocal
	}
	return q.EnqueueLabeled(queue.JobUpscale4K, post4KParams{ImageBase64: imageBase64}, postSaveSource, post4KLabel)
}

func (a *App) EnqueuePost4K(imageBase64 string) (int64, error) {
	return enqueuePost4K(a.queueSvc, sdNotLocal(a.config), imageBase64)
}

func runPost4K(up postUpscaler, imageBase64 string) (*queue.JobResult, error) {
	out, err := up.UpscaleLong(imageBase64, post4KLong)
	if err != nil {
		return nil, err
	}
	status, img := "skipped", imageBase64
	if out.Upscaled {
		status, img = "done", out.Image
		if out.Resized {
			status = "resized"
		}
	}
	info, err := json.Marshal(map[string]string{"upscale": status})
	if err != nil {
		return nil, err
	}
	return &queue.JobResult{ImageBase64: img, Info: string(info)}, nil
}

func post4KHandler(cfg *config.Config) queue.Handler {
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if gen == nil {
			return nil, errors.New("до 4K: генерация недоступна")
		}
		if sdNotLocal(cfg) {
			return nil, errPost4KNotLocal
		}
		var p post4KParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if p.ImageBase64 == "" {
			return nil, errors.New("до 4K: нет картинки")
		}
		return runPost4K(gen, p.ImageBase64)
	}
}
