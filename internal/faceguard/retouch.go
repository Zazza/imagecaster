package faceguard

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Zazza/imagecaster/internal/adjust"
)

const (
	cheekDown      = 1.0
	underStrength  = 0.5
	irisRatio      = 0.21
	pupilRatio     = 0.2
	pupilDark      = 0.45
	irisFeather    = 0.15
	irisBlend      = 0.75
	irisLift       = 0.45
	teethMinArea   = 0.002
	teethYellowCut = 0.7
	teethLift      = 1.06
	whitesDesat    = 0.6
	whitesLift     = 1.04
	pupilPoints    = 70
)

type Retouch struct {
	UnderEyes bool   `json:"under_eyes,omitempty"`
	Teeth     bool   `json:"teeth,omitempty"`
	EyeWhites bool   `json:"eye_whites,omitempty"`
	EyeColor  string `json:"eye_color,omitempty"`
	Light     bool   `json:"light,omitempty"`

	SkinTone      string  `json:"skin_tone,omitempty"`
	SkinLightness float64 `json:"skin_lightness,omitempty"`
	FaceWidth     float64 `json:"face_width,omitempty"`
}

const (
	SkinToneOriginal = "original"
	maxSkinLightness = 30
	maxFaceWidth     = 10
)

func (r Retouch) Local() bool {
	return r.UnderEyes || r.Teeth || r.EyeWhites || strings.TrimSpace(r.EyeColor) != "" || r.FaceWidth != 0
}

func (r Retouch) Skin() bool { return r.SkinTone != "" || r.SkinLightness != 0 }

func (r Retouch) IsZero() bool { return !r.Local() && !r.Light && !r.Skin() }

func (r Retouch) Validate() error {
	if r.SkinTone != "" && r.SkinTone != SkinToneOriginal {
		return fmt.Errorf("skin_tone: только %q", SkinToneOriginal)
	}
	if math.Abs(r.SkinLightness) > maxSkinLightness {
		return fmt.Errorf("skin_lightness: от −%d до +%d", maxSkinLightness, maxSkinLightness)
	}
	if math.Abs(r.FaceWidth) > maxFaceWidth {
		return fmt.Errorf("face_width: от −%d до +%d", maxFaceWidth, maxFaceWidth)
	}
	return nil
}

type Stats struct {
	UnderEye    float64
	Gamma       float64
	Temperature float64
}

var irisColors = map[string][3]float64{
	"blue": {82, 128, 180}, "light blue": {120, 165, 205}, "green": {90, 140, 95},
	"grey": {130, 140, 150}, "gray": {130, 140, 150}, "hazel": {140, 110, 60},
	"brown": {95, 60, 35}, "amber": {175, 120, 40}, "violet": {120, 90, 160},
	"синий": {82, 128, 180}, "голубой": {120, 165, 205}, "зелёный": {90, 140, 95},
	"зеленый": {90, 140, 95}, "серый": {130, 140, 150}, "ореховый": {140, 110, 60},
	"карий": {95, 60, 35}, "янтарный": {175, 120, 40}, "фиолетовый": {120, 90, 160},
}

func EyeColorRGB(name string) ([3]float64, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if c, ok := irisColors[n]; ok {
		return c, nil
	}
	if len(n) == 7 && n[0] == '#' {
		v, err := strconv.ParseUint(n[1:], 16, 32)
		if err == nil {
			return [3]float64{float64(v >> 16), float64(v >> 8 & 0xff), float64(v & 0xff)}, nil
		}
	}
	return [3]float64{}, fmt.Errorf("unknown eye color %q: use blue, light blue, green, grey, hazel, brown, amber, violet or #rrggbb", name)
}

func lum(r, g, b float64) float64 { return 0.299*r + 0.587*g + 0.114*b }

func Analyze(img image.Image, pts []Point) (Stats, error) {
	auto := adjust.AnalyzeAuto(img)
	st := Stats{Gamma: auto.Gamma, Temperature: auto.Temperature}
	if len(pts) < minPoints {
		return st, nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba := toRGBA(img, w, h)
	px := toPixels(pts, w, h)
	eyes := eyesMask(px, w, h)
	var under, cheek float64
	for _, idx := range underLids {
		u := meanLum(rgba, underEyeBand(px, idx, w, h, 0), eyes)
		c := meanLum(rgba, underEyeBand(px, idx, w, h, cheekDown), eyes)
		under += u
		cheek += c
	}
	if cheek > 0 {
		st.UnderEye = math.Max(0, 1-under/cheek)
	}
	return st, nil
}

func meanLum(img *image.RGBA, m, exclude *plane) float64 {
	var sum, n float64
	for i, v := range m.v {
		a := v * (1 - exclude.v[i])
		if a <= 0 {
			continue
		}
		p := img.Pix[i*4 : i*4+3]
		sum += a * lum(float64(p[0]), float64(p[1]), float64(p[2]))
		n += a
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func ApplyRetouch(img image.Image, pts []Point, r Retouch) (*image.RGBA, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("empty image")
	}
	var eyeTarget [3]float64
	if c := strings.TrimSpace(r.EyeColor); c != "" {
		t, err := EyeColorRGB(c)
		if err != nil {
			return nil, err
		}
		if len(pts) < pupilPoints {
			return nil, errors.New("no pupil points: eye color needs 70 face points")
		}
		eyeTarget = t
	}
	if r.Local() && len(pts) < minPoints {
		return nil, fmt.Errorf("need %d face points", minPoints)
	}
	src := img
	if r.Light {
		a := adjust.AnalyzeAuto(img)
		src = adjust.Apply(img, adjust.Params{BlackPoint: a.BlackPoint, WhitePoint: a.WhitePoint, Gamma: math.Max(1, a.Gamma)})
	}
	out := toRGBA(src, w, h)
	if !r.Local() {
		return out, nil
	}
	px := toPixels(pts, w, h)
	faceW := faceWidth(px)
	if faceW < 1 {
		return nil, errors.New("face too small")
	}
	if r.UnderEyes {
		liftUnderEyes(out, px, w, h, faceW)
	}
	if r.Teeth {
		whitenTeeth(out, px, w, h, faceW)
	}
	if r.EyeWhites || r.EyeColor != "" {
		eyes := eyesMask(px, w, h)
		iris := newPlane(w, h)
		if len(px) >= pupilPoints {
			for k, idx := range eyeContours {
				m := irisMask(px, idx, px[68+k], w, h)
				for i := range iris.v {
					iris.v[i] = math.Max(iris.v[i], m.v[i])
				}
			}
		}
		if r.EyeWhites {
			clearWhites(out, eyes, iris)
		}
		if strings.TrimSpace(r.EyeColor) != "" {
			recolorIris(out, eyes, iris, eyeTarget)
		}
	}
	if r.FaceWidth != 0 {
		out = widenFace(out, px, r.FaceWidth)
	}
	return out, nil
}

func liftUnderEyes(img *image.RGBA, px []fpt, w, h int, faceW float64) {
	eyes := eyesMask(px, w, h)
	ch := channels(img)
	sigma := faceW * toneBlurRatio
	var blurred [3][]float64
	for k := range 3 {
		blurred[k] = blurValues(ch[k], w, h, sigma)
	}
	for _, idx := range underLids {
		core := underEyeBand(px, idx, w, h, 0)
		cheek := underEyeBand(px, idx, w, h, cheekDown)
		var ref [3]float64
		var n float64
		for i, v := range cheek.v {
			if v <= 0 {
				continue
			}
			for k := range 3 {
				ref[k] += v * blurred[k][i]
			}
			n += v
		}
		if n == 0 {
			continue
		}
		for k := range 3 {
			ref[k] /= n
		}
		mask := core.blur(faceW * underBlur)
		for i, a := range mask.v {
			a *= (1 - eyes.v[i]) * underStrength
			if a <= 0.001 {
				continue
			}
			for k := range 3 {
				d := ref[k] - blurred[k][i]
				if d <= 0 {
					continue
				}
				j := i*4 + k
				img.Pix[j] = clamp8(float64(img.Pix[j]) + d*a)
			}
		}
	}
}

func whitenTeeth(img *image.RGBA, px []fpt, w, h int, faceW float64) {
	mouth := fill(w, h, pick(px, innerLips))
	var lums []float64
	for i, v := range mouth.v {
		if v > 0.5 {
			p := img.Pix[i*4 : i*4+3]
			lums = append(lums, lum(float64(p[0]), float64(p[1]), float64(p[2])))
		}
	}
	if float64(len(lums)) < teethMinArea*faceW*faceW {
		return
	}
	sort.Float64s(lums)
	p90 := lums[len(lums)*9/10]
	lo, hi := 0.5*p90, 0.75*p90
	mouth = mouth.blur(faceW * lipsBlurRatio)
	for i, m := range mouth.v {
		if m <= 0.001 {
			continue
		}
		p := img.Pix[i*4 : i*4+3]
		r, g, b := float64(p[0]), float64(p[1]), float64(p[2])
		if r-g > 40 {
			continue
		}
		a := m * smooth((lum(r, g, b)-lo)/(hi-lo))
		if a <= 0 {
			continue
		}
		avg := (r + g) / 2
		nb := b + (avg-b)*teethYellowCut
		p[0] = clamp8(r + (r*teethLift-r)*a)
		p[1] = clamp8(g + (g*teethLift-g)*a)
		p[2] = clamp8(b + (nb*teethLift-b)*a)
	}
}

func irisMask(px []fpt, contour []int, pupil fpt, w, h int) *plane {
	eye := fill(w, h, pick(px, contour))
	c0, c1 := px[contour[0]], px[contour[3]]
	radius := math.Hypot(c1.x-c0.x, c1.y-c0.y) * irisRatio
	m := newPlane(w, h)
	x0, x1 := max(0, int(pupil.x-radius-1)), min(w-1, int(pupil.x+radius+1))
	y0, y1 := max(0, int(pupil.y-radius-1)), min(h-1, int(pupil.y+radius+1))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			d := math.Hypot(float64(x)+0.5-pupil.x, float64(y)+0.5-pupil.y) / radius
			if d >= 1 || d < pupilRatio {
				continue
			}
			i := y*w + x
			m.v[i] = math.Min(1, (1-d)/irisFeather) * math.Min(1, (d-pupilRatio)/irisFeather) * eye.v[i]
		}
	}
	return m
}

func clearWhites(img *image.RGBA, eyes, iris *plane) {
	var lums []float64
	for i, v := range eyes.v {
		if v > 0.5 && iris.v[i] == 0 {
			p := img.Pix[i*4 : i*4+3]
			lums = append(lums, lum(float64(p[0]), float64(p[1]), float64(p[2])))
		}
	}
	if len(lums) == 0 {
		return
	}
	sort.Float64s(lums)
	p90 := lums[len(lums)*9/10]
	lo, hi := 0.55*p90, 0.8*p90
	for i, v := range eyes.v {
		a := v * (1 - math.Min(1, iris.v[i]*4))
		if a <= 0 {
			continue
		}
		p := img.Pix[i*4 : i*4+3]
		r, g, b := float64(p[0]), float64(p[1]), float64(p[2])
		l := lum(r, g, b)
		a *= smooth((l - lo) / (hi - lo))
		if a <= 0 {
			continue
		}
		for k, c := range []float64{r, g, b} {
			n := (c + (l-c)*whitesDesat) * whitesLift
			p[k] = clamp8(c + (n-c)*a)
		}
	}
}

func recolorIris(img *image.RGBA, eyes, iris *plane, target [3]float64) {
	var sum, n float64
	for i, v := range iris.v {
		if v > 0.5 {
			p := img.Pix[i*4 : i*4+3]
			sum += lum(float64(p[0]), float64(p[1]), float64(p[2]))
			n++
		}
	}
	if n == 0 || sum == 0 {
		return
	}
	mean := sum / n
	for i, v := range iris.v {
		a := v * eyes.v[i] * irisBlend
		if a <= 0 {
			continue
		}
		p := img.Pix[i*4 : i*4+3]
		r, g, b := float64(p[0]), float64(p[1]), float64(p[2])
		l := lum(r, g, b)
		if l > 2*mean || l < pupilDark*mean {
			continue
		}
		s := math.Max(0.25, math.Min(1.8, l/mean))
		want := l + (lum(target[0], target[1], target[2])*s-l)*irisLift
		norm := want / math.Max(1, lum(target[0], target[1], target[2])*s)
		for k, c := range []float64{r, g, b} {
			p[k] = clamp8(c + (target[k]*s*norm-c)*a)
		}
	}
}

func smooth(t float64) float64 {
	t = math.Max(0, math.Min(1, t))
	return t * t * (3 - 2*t)
}

func clamp8(v float64) uint8 {
	return uint8(math.Round(math.Max(0, math.Min(255, v))))
}

func AnalyzeBase64(b64 string, pts []Point) (Stats, error) {
	img, err := decode(b64, "image")
	if err != nil {
		return Stats{}, err
	}
	return Analyze(img, pts)
}

func ApplyRetouchBase64(b64 string, pts []Point, r Retouch) (string, error) {
	img, err := decode(b64, "image")
	if err != nil {
		return "", err
	}
	res, err := ApplyRetouch(img, pts, r)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, res); err != nil {
		return "", fmt.Errorf("encode: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
