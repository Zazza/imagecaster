package mcp

import (
	"errors"
	"image"
	"image/draw"
	"image/png"
	"math"
	"net/url"
	"strings"

	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/imgcodec"

	xdraw "golang.org/x/image/draw"
)

type FaceRefiner interface {
	FaceRefine(p generation.FaceRefineParams) (string, error)
}

const (
	shootFaceLong        = 1024
	shootFaceMargin      = 0.5
	shootFaceFeather     = 0.15
	shootFaceMeanFit     = 0.08
	shootFaceKeyFit      = 0.12
	shootFaceAspectFit   = 0.03
	shootFaceMinBox      = 32
	shootFaceSkipLarge   = 800
	shootFaceReasonMax   = 120
	shootFaceDone        = "done"
	shootFacePassMissing = "skipped:второй проход недоступен"
)

func facePassSkipped(reason string) string {
	return "skipped:" + reason
}

func faceSkipReason(err error) string {
	msg := err.Error()
	var ue *url.Error
	if errors.As(err, &ue) {
		msg = ue.Err.Error()
	}
	msg = strings.ReplaceAll(msg, "\n", " ")
	if r := []rune(msg); len(r) > shootFaceReasonMax {
		msg = string(r[:shootFaceReasonMax]) + "…"
	}
	return msg
}

func faceCropBox(pts []faceguard.Point, w, h int, margin float64) (image.Rectangle, bool) {
	if len(pts) == 0 || w <= 0 || h <= 0 {
		return image.Rectangle{}, false
	}
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		x, y := p.X*float64(w), p.Y*float64(h)
		x0, y0 = math.Min(x0, x), math.Min(y0, y)
		x1, y1 = math.Max(x1, x), math.Max(y1, y)
	}
	pad := margin * math.Max(x1-x0, y1-y0)
	rect := image.Rect(int(math.Floor(x0-pad)), int(math.Floor(y0-pad)), int(math.Ceil(x1+pad)), int(math.Ceil(y1+pad)))
	rect = rect.Intersect(image.Rect(0, 0, w, h))
	if rect.Dx() <= 0 || rect.Dy() <= 0 || max(rect.Dx(), rect.Dy()) < shootFaceMinBox {
		return image.Rectangle{}, false
	}
	return rect, true
}

func cropBase64(img image.Image, rect image.Rectangle, targetLong int) string {
	w, h := rect.Dx(), rect.Dy()
	if w <= 0 || h <= 0 || targetLong <= 0 {
		return ""
	}
	crop := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(crop, crop.Bounds(), img, rect.Min, draw.Src)
	scale := float64(targetLong) / float64(max(w, h))
	dw, dh := int(math.Round(float64(w)*scale)), int(math.Round(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), crop, crop.Bounds(), draw.Src, nil)
	b64, err := imgcodec.EncodePNG(dst, png.DefaultCompression)
	if err != nil {
		return ""
	}
	return b64
}

func remapPtsToCrop(pts []faceguard.Point, rect image.Rectangle, w, h int) []faceguard.Point {
	dx, dy := float64(rect.Dx()), float64(rect.Dy())
	out := make([]faceguard.Point, len(pts))
	for i, p := range pts {
		out[i] = faceguard.Point{
			X: (p.X*float64(w) - float64(rect.Min.X)) / dx,
			Y: (p.Y*float64(h) - float64(rect.Min.Y)) / dy,
		}
	}
	return out
}

func refineShootFace(eng StylistEngine, refiner FaceRefiner, frameB64, prompt, negative string, m shootModel, seed *int64) (string, string, bool) {
	frame, err := imgcodec.Decode(frameB64, imgcodec.Options{Name: "frame", DataURL: true, MaxDim: 8192})
	if err != nil {
		return frameB64, facePassSkipped("кадр не прочитан"), false
	}
	fb := frame.Bounds()
	pts, err := eng.FaceLandmarks(frameB64)
	if err != nil {
		return frameB64, facePassSkipped("лицо не найдено"), false
	}
	rect, ok := faceCropBox(pts, fb.Dx(), fb.Dy(), shootFaceMargin)
	if !ok {
		return frameB64, facePassSkipped("лицо слишком мелкое"), false
	}
	if max(rect.Dx(), rect.Dy()) >= shootFaceSkipLarge {
		return frameB64, facePassSkipped("лицо уже крупное"), false
	}
	crop := cropBase64(frame, rect, shootFaceLong)
	if crop == "" {
		return frameB64, facePassSkipped("кроп лица не собрался"), false
	}
	refined, err := refiner.FaceRefine(generation.FaceRefineParams{
		ImageBase64:       crop,
		Prompt:            prompt,
		NegativePrompt:    negative,
		Model:             m.Model,
		Sampler:           m.Sampler,
		Steps:             m.Steps,
		CfgScale:          m.CFG,
		DenoisingStrength: m.FaceDenoise,
		Seed:              seed,
	})
	if err != nil {
		if generation.IsInterrupted(err) {
			return frameB64, facePassSkipped("прервано"), true
		}
		return frameB64, facePassSkipped("второй проход не удался: " + faceSkipReason(err)), false
	}
	resPts, err := eng.FaceLandmarks(refined)
	if err != nil {
		return frameB64, facePassSkipped("лицо на результате не найдено"), false
	}
	rw, rh := filebrowser.DecodeImageSize(refined)
	if rw <= 0 || rh <= 0 {
		return frameB64, facePassSkipped("результат не прочитан"), false
	}
	if math.Abs(float64(rw)/float64(rh)-float64(rect.Dx())/float64(rect.Dy()))/(float64(rect.Dx())/float64(rect.Dy())) > shootFaceAspectFit {
		return frameB64, facePassSkipped("аспект результата изменился"), false
	}
	fit, err := faceguard.MeasurePosition(remapPtsToCrop(pts, rect, fb.Dx(), fb.Dy()), resPts, rw, rh)
	if err != nil || fit.Mean > shootFaceMeanFit || fit.Key > shootFaceKeyFit {
		return frameB64, facePassSkipped("лицо сместилось"), false
	}
	final, err := faceguard.BlendRegionBase64(frameB64, refined, rect, shootFaceFeather)
	if err != nil {
		return frameB64, facePassSkipped("вклейка не удалась"), false
	}
	return final, shootFaceDone, false
}
