package generation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math/rand/v2"

	xdraw "golang.org/x/image/draw"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/maskcomp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	repairCore            = 768
	repairMargin          = 128
	repairTile            = repairCore + 2*repairMargin
	repairMaxDim          = 8192
	repairDefaultStrength = 0.8
	repairMinStrength     = 0.3
	repairMaxStrength     = 0.9
	repairSteps           = 4
	repairDilate          = 1
	repairFeather         = 3
	repairPrompt          = "Remove all scratches, white and black dust spots, cracks, tears and creases from this old photo. Keep everything else exactly the same."
	repairLabel           = "Ремонт"
	repairEmptyMaskMsg    = "маска пустая — закрасьте повреждения"
)

type RepairParams struct {
	ImageBase64 string  `json:"image_base64"`
	MaskBase64  string  `json:"mask_base64"`
	Strength    float64 `json:"strength"`
	Seed        *int64  `json:"seed"`
}

type repairTileBox struct {
	core image.Rectangle
	tile image.Rectangle
}

func decodeRepairImage(b64, what string) (image.Image, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", what, err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", what, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > repairMaxDim || cfg.Height > repairMaxDim {
		return nil, fmt.Errorf("%s too large: %dx%d (max %d per side)", what, cfg.Width, cfg.Height, repairMaxDim)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", what, err)
	}
	return img, nil
}

func repairMaskAlpha(mask image.Image, w, h int) []uint8 {
	b := mask.Bounds()
	gray := image.NewGray(image.Rect(0, 0, w, h))
	if b.Dx() == w && b.Dy() == h {
		draw.Draw(gray, gray.Bounds(), mask, b.Min, draw.Src)
	} else {
		xdraw.NearestNeighbor.Scale(gray, gray.Bounds(), mask, b, xdraw.Src, nil)
	}
	a := make([]uint8, w*h)
	for i, v := range gray.Pix {
		if v > 127 {
			a[i] = 255
		}
	}
	soft := boxPass(boxPass(dilatePass(dilatePass(a, w, h, 1, 0), w, h, 0, 1), w, h, 1, 0), w, h, 0, 1)
	for i, v := range a {
		soft[i] = max(soft[i], v)
	}
	return soft
}

func dilatePass(a []uint8, w, h, dx, dy int) []uint8 {
	out := make([]uint8, len(a))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var m uint8
			for k := -repairDilate; k <= repairDilate && m < 255; k++ {
				nx, ny := x+k*dx, y+k*dy
				if nx >= 0 && ny >= 0 && nx < w && ny < h {
					m = max(m, a[ny*w+nx])
				}
			}
			out[y*w+x] = m
		}
	}
	return out
}

func boxPass(a []uint8, w, h, dx, dy int) []uint8 {
	out := make([]uint8, len(a))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum, n := 0, 0
			for k := -repairFeather; k <= repairFeather; k++ {
				nx, ny := x+k*dx, y+k*dy
				if nx >= 0 && ny >= 0 && nx < w && ny < h {
					sum += int(a[ny*w+nx])
					n++
				}
			}
			out[y*w+x] = uint8((sum + n/2) / n)
		}
	}
	return out
}

func repairTiles(alpha []uint8, w, h int) []repairTileBox {
	var tiles []repairTileBox
	for cy := 0; cy < h; cy += repairCore {
		for cx := 0; cx < w; cx += repairCore {
			core := image.Rect(cx, cy, min(w, cx+repairCore), min(h, cy+repairCore))
			if !alphaInRect(alpha, w, core) {
				continue
			}
			tiles = append(tiles, repairTileBox{core: core, tile: repairTileAround(core, w, h)})
		}
	}
	return tiles
}

func alphaInRect(alpha []uint8, w int, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if alpha[y*w+x] > 0 {
				return true
			}
		}
	}
	return false
}

func repairTileAround(core image.Rectangle, w, h int) image.Rectangle {
	span := func(lo, limit int) (int, int) {
		size := min(repairTile, limit)
		start := max(0, min(lo-repairMargin, limit-size))
		return start, start + size
	}
	x0, x1 := span(core.Min.X, w)
	y0, y1 := span(core.Min.Y, h)
	return image.Rect(x0, y0, x1, y1)
}

func (s *Service) validateRepairParams(p RepairParams) (image.Image, image.Image, float64, error) {
	if p.ImageBase64 == "" {
		return nil, nil, 0, errors.New("image is required")
	}
	if len(p.ImageBase64) > MaxImageBase64Len {
		return nil, nil, 0, config.ImageTooLarge("image")
	}
	if p.MaskBase64 == "" {
		return nil, nil, 0, errors.New("mask is required")
	}
	if len(p.MaskBase64) > MaxImageBase64Len {
		return nil, nil, 0, config.ImageTooLarge("mask")
	}
	strength := p.Strength
	if strength == 0 {
		strength = repairDefaultStrength
	}
	if strength < repairMinStrength || strength > repairMaxStrength {
		return nil, nil, 0, fmt.Errorf("strength must be between %.1f and %.1f", repairMinStrength, repairMaxStrength)
	}
	img, err := decodeRepairImage(p.ImageBase64, "image")
	if err != nil {
		return nil, nil, 0, err
	}
	mask, err := decodeRepairImage(p.MaskBase64, "mask")
	if err != nil {
		return nil, nil, 0, err
	}
	if !maskcomp.HasPaint(mask) {
		return nil, nil, 0, errors.New(repairEmptyMaskMsg)
	}
	return img, mask, strength, nil
}

func (s *Service) repairOneTile(src *image.RGBA, box image.Rectangle, strength float64, seed *int64) (*image.RGBA, json.RawMessage, error) {
	cw, ch := box.Dx(), box.Dy()
	crop := image.NewRGBA(image.Rect(0, 0, cw, ch))
	draw.Draw(crop, crop.Bounds(), src, box.Min, draw.Src)
	kw, kh := kleinImg2ImgSize(cw, ch)
	in := image.Image(crop)
	if kw != cw || kh != ch {
		scaled := image.NewRGBA(image.Rect(0, 0, kw, kh))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), crop, crop.Bounds(), xdraw.Src, nil)
		in = scaled
	}
	b64, err := encodePNGBase64(in)
	if err != nil {
		return nil, nil, fmt.Errorf("encode tile: %w", err)
	}
	batchSize, batchCount := 1, 1
	resp, err := s.sd.Img2Img(sd.Img2ImgRequest{
		InitImages:        []string{b64},
		Prompt:            repairPrompt,
		NegativePrompt:    s.kids.ApplyNegative(""),
		SamplerName:       kleinLookSampler,
		Scheduler:         kleinLookScheduler,
		Steps:             repairSteps,
		CfgScale:          kleinLookCfgScale,
		Width:             kw,
		Height:            kh,
		Seed:              seed,
		DenoisingStrength: &strength,
		BatchSize:         &batchSize,
		BatchCount:        &batchCount,
		DoNotSaveImages:   true,
		DoNotSaveGrid:     true,
	})
	if err != nil {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, nil, ierr
		}
		return nil, nil, err
	}
	if len(resp.Images) == 0 {
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, nil, ierr
		}
		return nil, nil, errors.New("no image generated (repair)")
	}
	data, err := base64.StdEncoding.DecodeString(resp.Images[0])
	if err != nil {
		return nil, nil, fmt.Errorf("decode repair tile: %w", err)
	}
	fixed, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("parse repair tile: %w", err)
	}
	out := image.NewRGBA(image.Rect(0, 0, cw, ch))
	fb := fixed.Bounds()
	if fb.Dx() == cw && fb.Dy() == ch {
		draw.Draw(out, out.Bounds(), fixed, fb.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(out, out.Bounds(), fixed, fb, xdraw.Src, nil)
	}
	return out, resp.Info, nil
}

func blendRepairCore(dst, src *image.RGBA, fixed *image.RGBA, t repairTileBox, alpha []uint8) {
	w := dst.Bounds().Dx()
	for y := t.core.Min.Y; y < t.core.Max.Y; y++ {
		for x := t.core.Min.X; x < t.core.Max.X; x++ {
			if alpha[y*w+x] == 0 {
				continue
			}
			a := float64(alpha[y*w+x]) / 255
			di := dst.PixOffset(x, y)
			fi := fixed.PixOffset(x-t.tile.Min.X, y-t.tile.Min.Y)
			for c := 0; c < 3; c++ {
				dst.Pix[di+c] = uint8(float64(src.Pix[di+c])*(1-a) + float64(fixed.Pix[fi+c])*a + 0.5)
			}
		}
	}
}

func (s *Service) RepairImage(params RepairParams) (*GenerateImageResult, error) {
	img, mask, strength, err := s.validateRepairParams(params)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	alpha := repairMaskAlpha(mask, w, h)
	tiles := repairTiles(alpha, w, h)
	if len(tiles) == 0 {
		return nil, errors.New(repairEmptyMaskMsg)
	}

	s.referenceMu.Lock()
	defer s.referenceMu.Unlock()

	if err := s.sd.HealthCheck(); err != nil {
		return nil, fmt.Errorf("SD is not available: %w", err)
	}
	s.StartSDPolling()
	defer s.StopSDPolling()

	s.guard.EnsureHeadroom(kleinLookModel, "repair")
	if err := s.setModel(kleinLookModel, "repair"); err != nil {
		return nil, fmt.Errorf("%s not available on SD backend: %w", kleinLookModel, err)
	}
	if err := s.sd.SetVAE("Automatic"); err != nil {
		s.log.Warn("repair: reset vae: %s", err)
	}
	if err := s.sd.SetOption(kleinReferenceOption, true); err != nil {
		if rerr := s.sd.SetOption(kleinReferenceOption, false); rerr != nil {
			s.log.Warn("repair: disable klein reference after failed enable: %s", rerr)
		}
		return nil, fmt.Errorf("enable repair reference: %w", err)
	}
	var result *GenerateImageResult
	defer func() {
		derr := s.sd.SetOption(kleinReferenceOption, false)
		if derr != nil {
			s.log.Warn("repair: disable klein reference: %s", derr)
		}
		if result != nil {
			result.PreserveIdentityResetFailed = derr != nil
		}
	}()

	seed := params.Seed
	if seed == nil || *seed < 0 {
		v := rand.Int64N(1 << 31)
		seed = &v
	}
	s.log.Info("repair: %dx%d, %d tiles, strength %.2f", w, h, len(tiles), strength)
	out := image.NewRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	var lastInfo json.RawMessage
	for i, t := range tiles {
		s.emitter.Emit("repair:progress", map[string]int{"done": i, "total": len(tiles)})
		if ierr := s.checkSDInterrupted(); ierr != nil {
			return nil, ierr
		}
		fixed, tileInfo, err := s.repairOneTile(src, t.tile, strength, seed)
		if err != nil {
			return nil, err
		}
		lastInfo = tileInfo
		blendRepairCore(out, src, fixed, t, alpha)
	}
	s.emitter.Emit("repair:progress", map[string]int{"done": len(tiles), "total": len(tiles)})

	finalB64, err := encodePNGBase64(out)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	info := patchInfoSize(lastInfo, w, h)
	result = &GenerateImageResult{
		Image:           finalB64,
		Info:            info,
		EffectivePrompt: repairPrompt,
	}
	s.sessions.AddToSession(finalB64, info, "repair", false, &preset.GenerationMeta{
		Label: repairLabel,
		Model: kleinLookModel,
	})
	return result, nil
}
