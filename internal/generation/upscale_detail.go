package generation

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"

	xdraw "golang.org/x/image/draw"

	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/sdnames"
)

const (
	detailTile        = 1024
	detailOverlap     = 128
	detailMaxDim      = 8192
	detailMinEffSteps = 4
	detailMaxSteps    = 60
	detailScheduler   = "karras"
)

type DetailParams struct {
	ImageBase64    string
	TargetLong     int
	Detail         float64
	Model          string
	Prompt         string
	NegativePrompt string
	Sampler        string
	Steps          int
	CfgScale       float64
	Seed           *int64
}

type DetailResult struct {
	Image    string
	Width    int
	Height   int
	Upscaled bool
	Resized  bool
	Detailed bool
	Tiles    int
}

func (s *Service) UpscaleDetail(p DetailParams) (*DetailResult, error) {
	if p.ImageBase64 == "" {
		return nil, errors.New("image is required")
	}
	if p.Detail > 0 && p.Model == "" {
		return nil, errors.New("detail pass needs the model of the image")
	}
	if s.Stopped() {
		return nil, ErrInterrupted
	}
	src, err := imgcodec.Decode(p.ImageBase64, imgcodec.Options{Name: "image", DataURL: true, MaxDim: detailMaxDim})
	if err != nil {
		return nil, err
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	res := &DetailResult{Image: p.ImageBase64, Width: w, Height: h}
	cur := src
	if long := max(w, h); p.TargetLong > 0 && long < p.TargetLong {
		tw, th := fitLong(w, h, p.TargetLong)
		img, err := s.neuralUpscale(p.ImageBase64, float64(p.TargetLong)/float64(long))
		if err != nil {
			return nil, err
		}
		if img == nil {
			img, res.Resized = src, true
		}
		cur = resizeExact(img, tw, th)
		res.Width, res.Height, res.Upscaled = tw, th, true
	}
	if p.Detail > 0 {
		out, tiles, err := s.detailTiles(cur, p)
		if err != nil {
			return nil, err
		}
		cur, res.Detailed, res.Tiles = out, true, tiles
	}
	if res.Upscaled || res.Detailed {
		b64, err := imgcodec.EncodePNG(cur, png.BestSpeed)
		if err != nil {
			return nil, err
		}
		res.Image = b64
	}
	return res, nil
}

func fitLong(w, h, long int) (int, int) {
	if w >= h {
		return long, int(math.Round(float64(h) * float64(long) / float64(w)))
	}
	return int(math.Round(float64(w) * float64(long) / float64(h))), long
}

func resizeExact(img image.Image, w, h int) image.Image {
	if img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	return dst
}

func tileStarts(size, tile, overlap int) []int {
	if size <= tile {
		return []int{0}
	}
	var out []int
	for x := 0; x < size-tile; x += tile - overlap {
		out = append(out, x)
	}
	return append(out, size-tile)
}

func tileMask(w, h, overlap int) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	ramp := float64(overlap)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := math.Min(math.Min(float64(x), float64(w-1-x)), math.Min(float64(y), float64(h-1-y)))
			a := 1.0
			if ramp > 0 && d < ramp {
				a = (d + 1) / (ramp + 1)
			}
			m.Pix[y*m.Stride+x] = uint8(math.Round(a * 255))
		}
	}
	return m
}

func (s *Service) detailTiles(src image.Image, p DetailParams) (image.Image, int, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := min(detailTile, w), min(detailTile, h)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	spec := modelSpec(p.Model)
	sampler, scheduler, _ := sdnames.Pair(p.Sampler, "")
	if scheduler == "" && len(spec.Modules) == 0 {
		scheduler = detailScheduler
	}
	steps := p.Steps
	if steps <= 0 {
		steps = 20
	}
	steps = min(max(steps, int(math.Ceil(detailMinEffSteps/p.Detail))), max(p.Steps, detailMaxSteps))
	denoise := p.Detail
	xs, ys := tileStarts(w, tw, detailOverlap), tileStarts(h, th, detailOverlap)
	edgeX, edgeY := len(xs) > 1, len(ys) > 1
	tiles := 0
	for _, y := range ys {
		for _, x := range xs {
			if s.Stopped() {
				return nil, tiles, ErrInterrupted
			}
			crop := image.NewRGBA(image.Rect(0, 0, tw, th))
			draw.Draw(crop, crop.Bounds(), src, image.Pt(b.Min.X+x, b.Min.Y+y), draw.Src)
			in, err := imgcodec.EncodePNG(crop, png.BestSpeed)
			if err != nil {
				return nil, tiles, err
			}
			resp, err := s.sd.Img2Img(sd.Img2ImgRequest{
				Model:             spec,
				InitImages:        []string{in},
				Prompt:            p.Prompt,
				NegativePrompt:    p.NegativePrompt,
				SamplerName:       sampler,
				Scheduler:         scheduler,
				Steps:             steps,
				CfgScale:          p.CfgScale,
				Width:             tw,
				Height:            th,
				Seed:              p.Seed,
				DenoisingStrength: &denoise,
			})
			tiles++
			if err != nil {
				if IsInterrupted(err) {
					return nil, tiles, err
				}
				return nil, tiles, fmt.Errorf("detail tile %d: %w", tiles, err)
			}
			if resp == nil || len(resp.Images) == 0 {
				return nil, tiles, fmt.Errorf("detail tile %d: no image", tiles)
			}
			got, err := imgcodec.Decode(resp.Images[0], imgcodec.Options{Name: "tile", MaxDim: detailMaxDim})
			if err != nil {
				return nil, tiles, err
			}
			got = resizeExact(got, tw, th)
			mask := tileMask(tw, th, detailOverlap)
			featherEdges(mask, x == 0 || !edgeX, x+tw >= w || !edgeX, y == 0 || !edgeY, y+th >= h || !edgeY)
			draw.DrawMask(out, image.Rect(x, y, x+tw, y+th), got, got.Bounds().Min, mask, image.Point{}, draw.Over)
		}
	}
	return out, tiles, nil
}

func featherEdges(m *image.Alpha, left, right, top, bottom bool) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			onOuter := (left && x < detailOverlap) || (right && x >= w-detailOverlap) || (top && y < detailOverlap) || (bottom && y >= h-detailOverlap)
			if !onOuter {
				continue
			}
			dx := float64(detailOverlap)
			if !left && x < detailOverlap {
				dx = float64(x)
			}
			if !right && x >= w-detailOverlap {
				dx = math.Min(dx, float64(w-1-x))
			}
			dy := float64(detailOverlap)
			if !top && y < detailOverlap {
				dy = float64(y)
			}
			if !bottom && y >= h-detailOverlap {
				dy = math.Min(dy, float64(h-1-y))
			}
			d := math.Min(dx, dy)
			a := 1.0
			if d < detailOverlap {
				a = (d + 1) / (detailOverlap + 1)
			}
			m.Pix[y*m.Stride+x] = uint8(math.Round(a * 255))
		}
	}
}

func (s *Service) neuralUpscale(b64 string, scale float64) (image.Image, error) {
	name := s.resolveQualityUpscaler()
	if name == "" {
		return nil, nil
	}
	up, err := s.sd.UpscaleImage(b64, name, scale)
	if err != nil {
		if IsInterrupted(err) {
			return nil, err
		}
		s.log.Warn("detail upscale: %s failed, resizing: %s", name, err)
		return nil, nil
	}
	img, err := imgcodec.Decode(up, imgcodec.Options{Name: "upscaled", MaxDim: detailMaxDim * 2})
	if err != nil {
		s.log.Warn("detail upscale: %s result unreadable, resizing: %s", name, err)
		return nil, nil
	}
	return img, nil
}
