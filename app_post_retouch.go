package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/importexport"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	postFaceNoFace      = "no_face"
	postFaceNotLocal    = "not_local"
	postFaceUnavailable = "unavailable"
	post4KLong          = 3840
	postSaveSource      = "post"
	postSaveLabel       = "До 4K: "
)

type PostFace struct {
	Points []faceguard.Point `json:"points"`
	Skip   string            `json:"skip,omitempty"`
	Error  string            `json:"error,omitempty"`
}

type PostSaveResult struct {
	Path    string `json:"path"`
	Upscale string `json:"upscale"`
}

type faceLandmarker interface {
	FaceLandmarks(imageBase64 string) ([]faceguard.Point, error)
}

type postUpscaler interface {
	UpscaleLong(imageBase64 string, targetLong int) (*generation.UpscaleResult, error)
}

type postExporter interface {
	ProcessExportImage(params ExportImageParams) (*importexport.ProcessedImage, error)
}

type postSaveQueue interface {
	EnqueueLabeled(jobType queue.JobType, params any, source, label string) (int64, error)
}

type savePathPicker func(defaultName, format string) (string, error)

func checkPostImage(imageBase64 string) error {
	if imageBase64 == "" {
		return errors.New("no image provided")
	}
	if len(imageBase64) > config.MaxImageBase64Len {
		return config.ImageTooLarge("image")
	}
	return nil
}

var errPost4KNotLocal = errors.New("до 4K: сервер не в локальной сети — фото на него не отправляется")

func postFaceLandmarks(det faceLandmarker, cfg *config.Config, imageBase64 string) (PostFace, error) {
	if err := checkPostImage(imageBase64); err != nil {
		return PostFace{}, err
	}
	none := []faceguard.Point{}
	if sdNotLocal(cfg) {
		return PostFace{Points: none, Skip: postFaceNotLocal}, nil
	}
	if det == nil {
		return PostFace{Points: none, Skip: postFaceUnavailable}, nil
	}
	pts, err := det.FaceLandmarks(imageBase64)
	switch {
	case errors.Is(err, sd.ErrNoFace):
		return PostFace{Points: none, Skip: postFaceNoFace}, nil
	case err != nil:
		return PostFace{Points: none, Skip: postFaceUnavailable, Error: err.Error()}, nil
	}
	if pts == nil {
		pts = none
	}
	return PostFace{Points: pts}, nil
}

func (a *App) PostFaceLandmarks(imageBase64 string) (PostFace, error) {
	face, err := postFaceLandmarks(a.gen, a.config, imageBase64)
	if face.Error != "" {
		a.log.Warn("post: face landmarks: %s", face.Error)
	}
	return face, err
}

func (a *App) PreviewPostRetouch(imageBase64 string, p AdjustParams, r faceguard.Retouch, pts []faceguard.Point, f FinishParams, maxW int) (string, error) {
	return a.preview.PostRetouch(imageBase64, p, r, pts, f, maxW)
}

func enqueuePostSave(q postSaveQueue, pick savePathPicker, notLocal bool, p ExportImageParams) (int64, error) {
	if err := checkPostImage(p.ImageBase64); err != nil {
		return 0, err
	}
	if notLocal {
		return 0, errPost4KNotLocal
	}
	if p.Format != "png" && p.Format != "jpeg" {
		return 0, fmt.Errorf("unsupported format: %s", p.Format)
	}
	if p.Retouch != nil {
		if err := p.Retouch.Validate(); err != nil {
			return 0, fmt.Errorf("retouch: %w", err)
		}
	}
	if p.OutputDir == "" {
		path, err := pick(filepath.Base(p.Filename), p.Format)
		if err != nil || path == "" {
			return 0, err
		}
		p.OutputDir, p.Filename = filepath.Dir(path), filepath.Base(path)
		p.Overwrite = true
	}
	p.Width, p.Height = 0, 0
	return q.EnqueueLabeled(queue.JobPostSave, p, postSaveSource, postSaveLabel+filepath.Base(p.Filename))
}

func (a *App) EnqueuePostSave(p ExportImageParams) (int64, error) {
	return enqueuePostSave(a.queueSvc, a.pickSavePath, sdNotLocal(a.config), p)
}

func (a *App) pickSavePath(name, format string) (string, error) {
	ext := "." + format
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: name,
		Filters:         []runtime.FileFilter{{DisplayName: strings.ToUpper(format) + " Image", Pattern: "*" + ext}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ext) {
		path += ext
	}
	return path, nil
}

func runPostSave(up postUpscaler, ex postExporter, p ExportImageParams, stopped func() bool) (*PostSaveResult, error) {
	if p.OutputDir == "" {
		return nil, errors.New("до 4K: не выбрана папка")
	}
	if stopped == nil {
		stopped = func() bool { return false }
	}
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	res := &PostSaveResult{Upscale: "skipped"}
	out, err := up.UpscaleLong(p.ImageBase64, post4KLong)
	if err != nil {
		return nil, err
	}
	if out.Upscaled {
		res.Upscale = "done"
		if out.Resized {
			res.Upscale = "resized"
		}
		p.ImageBase64 = out.Image
	}
	p.Width, p.Height = 0, 0
	processed, err := ex.ProcessExportImage(p)
	if err != nil {
		return nil, err
	}
	if stopped() {
		return nil, generation.ErrInterrupted
	}
	target := filepath.Join(p.OutputDir, filepath.Base(processed.Filename))
	var path string
	if p.Overwrite {
		path, err = target, os.WriteFile(target, processed.Data, 0o644)
	} else {
		path, err = importexport.WriteImageToPath(processed, target)
	}
	if err != nil {
		return nil, err
	}
	res.Path = path
	return res, nil
}

func postSaveHandler(ex postExporter, cfg *config.Config) queue.Handler {
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if gen == nil {
			return nil, errors.New("до 4K: генерация недоступна")
		}
		if sdNotLocal(cfg) {
			return nil, errPost4KNotLocal
		}
		var p ExportImageParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		res, err := runPostSave(gen, ex, p, gen.Stopped)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return &queue.JobResult{Info: string(raw)}, nil
	}
}
