package finish

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	CaptionMaxLen = 200
	MaxSide       = 8192
)

var Ratios = []string{"1:1", "4:5", "3:4", "9:16", "16:9", "3:2"}

var (
	fits         = []string{"crop", "blur", "white", "black"}
	frames       = []string{"white", "black", "polaroid"}
	captionPos   = []string{"bottom", "top", "center"}
	captionSizes = map[string]float64{"s": 0.045, "m": 0.065, "l": 0.095}
	polaroidBg   = color.RGBA{247, 245, 240, 255}
	polaroidInk  = color.RGBA{34, 34, 34, 255}
)

type Params struct {
	Ratio       string   `json:"ratio,omitempty"`
	Fit         string   `json:"fit,omitempty"`
	Offset      *float64 `json:"offset,omitempty"`
	Frame       string   `json:"frame,omitempty"`
	Caption     string   `json:"caption,omitempty"`
	CaptionPos  string   `json:"caption_pos,omitempty"`
	CaptionSize string   `json:"caption_size,omitempty"`
}

func (p Params) IsNeutral() bool {
	return p.Ratio == "" && p.Frame == "" && strings.TrimSpace(p.Caption) == ""
}

func oneOf(v string, list []string) bool {
	for _, x := range list {
		if v == x {
			return true
		}
	}
	return false
}

func (p Params) Validate() error {
	if p.Ratio != "" && !oneOf(p.Ratio, Ratios) {
		return fmt.Errorf("формат %q не поддерживается: %s", p.Ratio, strings.Join(Ratios, ", "))
	}
	if p.Fit != "" && !oneOf(p.Fit, fits) {
		return fmt.Errorf("способ %q не поддерживается: %s", p.Fit, strings.Join(fits, ", "))
	}
	if p.Offset != nil && (*p.Offset < 0 || *p.Offset > 1 || math.IsNaN(*p.Offset)) {
		return errors.New("сдвиг кадра — от 0 до 1")
	}
	if p.Frame != "" && !oneOf(p.Frame, frames) {
		return fmt.Errorf("рамка %q не поддерживается: %s", p.Frame, strings.Join(frames, ", "))
	}
	if p.CaptionPos != "" && !oneOf(p.CaptionPos, captionPos) {
		return fmt.Errorf("место подписи %q не поддерживается: %s", p.CaptionPos, strings.Join(captionPos, ", "))
	}
	if _, ok := captionSizes[p.CaptionSize]; p.CaptionSize != "" && !ok {
		return fmt.Errorf("размер подписи %q не поддерживается: s, m, l", p.CaptionSize)
	}
	if utf8.RuneCountInString(p.Caption) > CaptionMaxLen {
		return fmt.Errorf("подпись длиннее %d символов", CaptionMaxLen)
	}
	return nil
}

func Apply(img image.Image, p Params) (image.Image, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.IsNeutral() {
		return img, nil
	}
	out := toRGBA(img)
	if p.Ratio != "" {
		out = reframe(out, p)
	}
	caption := strings.TrimSpace(p.Caption)
	if caption != "" && p.Frame != "polaroid" {
		if err := drawCaption(out, out.Bounds(), caption, p, color.White, true); err != nil {
			return nil, err
		}
	}
	switch p.Frame {
	case "white":
		out = border(out, color.White)
	case "black":
		out = border(out, color.Black)
	case "polaroid":
		var strip image.Rectangle
		out, strip = polaroid(out)
		if caption != "" {
			if err := drawCaption(out, strip, caption, Params{CaptionPos: "center", CaptionSize: p.CaptionSize}, polaroidInk, false); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func toRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

func ratioValue(r string) float64 {
	a, b, _ := strings.Cut(r, ":")
	x, _ := strconv.ParseFloat(a, 64)
	y, _ := strconv.ParseFloat(b, 64)
	return x / y
}

func reframe(img *image.RGBA, p Params) *image.RGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	r := ratioValue(p.Ratio)
	fit := p.Fit
	if fit == "" {
		fit = "crop"
	}
	if fit == "crop" {
		off := 0.5
		if p.Offset != nil {
			off = *p.Offset
		}
		rect := cropRect(w, h, r, off)
		dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
		return dst
	}
	cw, ch := w, h
	if float64(w)/float64(h) > r {
		ch = max(h, int(math.Round(float64(w)/r)))
	} else {
		cw = max(w, int(math.Round(float64(h)*r)))
	}
	if big := max(cw, ch); big > MaxSide {
		k := float64(MaxSide) / float64(big)
		w, h = max(1, int(float64(w)*k)), max(1, int(float64(h)*k))
		small := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), xdraw.Src, nil)
		img = small
		cw, ch = max(1, int(float64(cw)*k)), max(1, int(float64(ch)*k))
		cw, ch = max(cw, w), max(ch, h)
	}
	dst := image.NewRGBA(image.Rect(0, 0, cw, ch))
	switch fit {
	case "white":
		draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	case "black":
		draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	default:
		blurredCover(dst, img)
	}
	at := image.Pt((cw-w)/2, (ch-h)/2)
	draw.Draw(dst, image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}, img, image.Point{}, draw.Src)
	return dst
}

func cropRect(w, h int, r, off float64) image.Rectangle {
	if float64(w)/float64(h) > r {
		nw := max(1, min(w, int(math.Round(float64(h)*r))))
		x := int(math.Round(float64(w-nw) * off))
		return image.Rect(x, 0, x+nw, h)
	}
	nh := max(1, min(h, int(math.Round(float64(w)/r))))
	y := int(math.Round(float64(h-nh) * off))
	return image.Rect(0, y, w, y+nh)
}

func blurredCover(dst, src *image.RGBA) {
	cw, ch := dst.Bounds().Dx(), dst.Bounds().Dy()
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	rect := cropRect(sw, sh, float64(cw)/float64(ch), 0.5)
	small := image.NewRGBA(image.Rect(0, 0, max(1, cw/24), max(1, ch/24)))
	xdraw.ApproxBiLinear.Scale(small, small.Bounds(), src, rect, xdraw.Src, nil)
	tiny := image.NewRGBA(image.Rect(0, 0, max(1, small.Bounds().Dx()/2), max(1, small.Bounds().Dy()/2)))
	xdraw.BiLinear.Scale(tiny, tiny.Bounds(), small, small.Bounds(), xdraw.Src, nil)
	xdraw.BiLinear.Scale(small, small.Bounds(), tiny, tiny.Bounds(), xdraw.Src, nil)
	xdraw.BiLinear.Scale(dst, dst.Bounds(), small, small.Bounds(), xdraw.Src, nil)
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i] = uint8(float64(dst.Pix[i]) * 0.8)
		dst.Pix[i+1] = uint8(float64(dst.Pix[i+1]) * 0.8)
		dst.Pix[i+2] = uint8(float64(dst.Pix[i+2]) * 0.8)
		dst.Pix[i+3] = 255
	}
}

func border(img *image.RGBA, c color.Color) *image.RGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	b := max(2, int(math.Round(0.04*float64(min(w, h)))))
	dst := image.NewRGBA(image.Rect(0, 0, w+2*b, h+2*b))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(b, b, b+w, b+h), img, image.Point{}, draw.Src)
	return dst
}

func polaroid(img *image.RGBA) (*image.RGBA, image.Rectangle) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	side := max(2, int(math.Round(0.06*float64(w))))
	bottom := max(side*3, int(math.Round(0.24*float64(w))))
	dst := image.NewRGBA(image.Rect(0, 0, w+2*side, h+side+bottom))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(polaroidBg), image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(side, side, side+w, side+h), img, image.Point{}, draw.Src)
	return dst, image.Rect(side, side+h, side+w, side+h+bottom)
}

var (
	fontOnce sync.Once
	fontData *opentype.Font
	fontErr  error
)

func captionFont() (*opentype.Font, error) {
	fontOnce.Do(func() { fontData, fontErr = opentype.Parse(gobold.TTF) })
	return fontData, fontErr
}

func drawCaption(dst *image.RGBA, area image.Rectangle, text string, p Params, ink color.Color, shadow bool) error {
	f, err := captionFont()
	if err != nil {
		return fmt.Errorf("шрифт подписи: %w", err)
	}
	k := captionSizes["m"]
	if v, ok := captionSizes[p.CaptionSize]; ok {
		k = v
	}
	aw, ah := area.Dx(), area.Dy()
	size := k * float64(min(aw, ah))
	if !shadow {
		size = k * 4.3 * float64(ah)
	}
	maxW := int(float64(aw) * 0.9)
	maxH := int(float64(ah) * 0.8)
	if !shadow {
		maxH = int(float64(ah) * 0.7)
	}
	var face font.Face
	var lines []string
	var lineH int
	for {
		face, err = opentype.NewFace(f, &opentype.FaceOptions{Size: math.Max(size, 6), DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			return fmt.Errorf("шрифт подписи: %w", err)
		}
		lines = wrap(face, text, maxW)
		lineH = face.Metrics().Height.Ceil()
		if (lineH*len(lines) <= maxH && widest(face, lines) <= maxW) || size <= 6 {
			break
		}
		face.Close()
		size *= 0.85
	}
	defer face.Close()
	m := face.Metrics()
	blockH := lineH * len(lines)
	margin := int(float64(ah) * 0.05)
	top := area.Min.Y + (ah-blockH)/2
	switch p.CaptionPos {
	case "top":
		top = area.Min.Y + margin
	case "center":
	default:
		if shadow {
			top = area.Max.Y - margin - blockH
		}
	}
	if shadow {
		scrim(dst, area, top-lineH/2, top+blockH+lineH/2)
	}
	d := &font.Drawer{Dst: dst, Face: face}
	off := max(1, int(math.Round(size*0.06)))
	for i, line := range lines {
		lw := d.MeasureString(line).Ceil()
		x := area.Min.X + (aw-lw)/2
		y := top + i*lineH + m.Ascent.Ceil()
		if shadow {
			d.Src = image.NewUniform(color.RGBA{0, 0, 0, 150})
			for _, o := range [][2]int{{off, off}, {-off / 2, off}, {off, -off / 2}, {0, off * 3 / 2}} {
				d.Dot = fixed.P(x+o[0], y+o[1])
				d.DrawString(line)
			}
		}
		d.Src = image.NewUniform(ink)
		d.Dot = fixed.P(x, y)
		d.DrawString(line)
	}
	return nil
}

func scrim(dst *image.RGBA, area image.Rectangle, y0, y1 int) {
	fade := max(1, (y1-y0)/2)
	for y := max(area.Min.Y, y0-fade); y < min(area.Max.Y, y1+fade); y++ {
		a := 0.38
		if y < y0 {
			a *= 1 - float64(y0-y)/float64(fade)
		} else if y >= y1 {
			a *= 1 - float64(y-y1+1)/float64(fade)
		}
		k := 1 - a
		row := dst.Pix[y*dst.Stride:]
		for x := area.Min.X; x < area.Max.X; x++ {
			i := x * 4
			row[i] = uint8(float64(row[i]) * k)
			row[i+1] = uint8(float64(row[i+1]) * k)
			row[i+2] = uint8(float64(row[i+2]) * k)
		}
	}
}

func widest(face font.Face, lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, font.MeasureString(face, l).Ceil())
	}
	return w
}

func wrap(face font.Face, text string, maxW int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if font.MeasureString(face, line+" "+w).Ceil() <= maxW {
				line += " " + w
				continue
			}
			lines = append(lines, line)
			line = w
		}
		lines = append(lines, line)
	}
	return lines
}
