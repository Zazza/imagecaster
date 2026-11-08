package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

const mkEffectText = "garments made of soft premium leather with natural grain"

var (
	mkBlue    = color.NRGBA{R: 0, G: 0, B: 255, A: 255}
	mkRed     = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	mkYellow  = color.NRGBA{R: 255, G: 255, B: 0, A: 255}
	mkYellow2 = color.NRGBA{R: 195, G: 195, B: 0, A: 255}
)

type mkRec struct {
	mu      sync.Mutex
	vision  int
	img2img []sd.Img2ImgRequest
	outputs []string
}

func (r *mkRec) visionCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.vision
}

func (r *mkRec) sdCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.img2img)
}

type mkEnv struct {
	svc      *Service
	rec      *mkRec
	sessions *mockSessions
	preset   *preset.Preset
}

func mkSetup(t *testing.T, presetType string, vision func() (string, error), paint func(t *testing.T, w, h int) string) *mkEnv {
	t.Helper()
	rec := &mkRec{}
	visionCall := func() (string, error) {
		rec.mu.Lock()
		rec.vision++
		rec.mu.Unlock()
		if vision == nil {
			return "", fmt.Errorf("vision not expected")
		}
		return vision()
	}
	llmSvc := &mockLLM{
		genSDPromptFn: func(systemPrompt, userMessage, presetType, model string, maxTokens int) (string, error) {
			return `{"prompt":"llm-scene-tags","negative_prompt":"llm-neg"}`, nil
		},
		chatFn: func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
			return "llm-chat", nil
		},
		chatVisionFn: func(model, systemPrompt, userText, imageBase64 string, temperature float64, maxTokens int) (string, error) {
			return visionCall()
		},
		analyzeImageFn: func(model, systemPrompt, imageBase64 string, maxTokens int) (string, error) {
			return visionCall()
		},
		describeFn: func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
			return visionCall()
		},
	}
	sdSvc := &mockSD{
		img2img: func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
			out := solidPNGBase64(t, req.Width, req.Height, mkYellow)
			if paint != nil {
				out = paint(t, req.Width, req.Height)
			}
			rec.mu.Lock()
			rec.img2img = append(rec.img2img, req)
			rec.outputs = append(rec.outputs, out)
			rec.mu.Unlock()
			return &sd.Txt2ImgResponse{Images: []string{out}}, nil
		},
		getUpscalers: func() ([]sd.Upscaler, error) { return nil, nil },
		upscale: func(in, upscaler string, scale float64) (string, error) {
			return in, nil
		},
	}
	db := openTestDB(t)
	pr := makeTestPreset(t, db, &preset.Preset{
		Name:         "Leather",
		PresetType:   presetType,
		Prompt:       mkEffectText,
		Sampler:      "Euler",
		ScheduleType: "Simple",
		Steps:        4,
		CfgScale:     1.0,
		ModelName:    "flux-2-klein-4b",
	})
	svc := newTestService(t, db, llmSvc, sdSvc)
	svc.ctx = context.Background()
	sessions := &mockSessions{}
	svc.sessions = sessions
	return &mkEnv{svc: svc, rec: rec, sessions: sessions, preset: pr}
}

func (e *mkEnv) params(src string) GenerateFromImageParams {
	return GenerateFromImageParams{
		ImageBase64:       src,
		GenMode:           "preset",
		Mode:              "img2img",
		PresetID:          e.preset.ID,
		DenoisingStrength: 0.6,
	}
}

func (e *mkEnv) onlyPrompt(t *testing.T) string {
	t.Helper()
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	require.Len(t, e.rec.img2img, 1)
	return e.rec.img2img[0].Prompt
}

func mkMask(t *testing.T, w, h int, r image.Rectangle) string {
	t.Helper()
	m := image.NewGray(image.Rect(0, 0, w, h))
	r = r.Intersect(m.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			m.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, m))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func mkBlueRedPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := mkRed
			if x < w/2 {
				c = mkBlue
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func mkYellowChecker(t *testing.T, w, h int) string {
	t.Helper()
	const cell = 4
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := mkYellow
			if (x/cell+y/cell)%2 == 1 {
				c = mkYellow2
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

type mkWindowStats struct {
	mean      [3]float64
	sumSpread int
}

func mkWindow(img image.Image, cx, cy, half int) mkWindowStats {
	b := img.Bounds()
	var s mkWindowStats
	minSum, maxSum := 1<<30, -1
	n := 0
	for y := cy - half; y < cy+half; y++ {
		for x := cx - half; x < cx+half; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			s.mean[0] += float64(c.R)
			s.mean[1] += float64(c.G)
			s.mean[2] += float64(c.B)
			sum := int(c.R) + int(c.G) + int(c.B)
			minSum = min(minSum, sum)
			maxSum = max(maxSum, sum)
			n++
		}
	}
	for i := range s.mean {
		s.mean[i] /= float64(n)
	}
	s.sumSpread = maxSum - minSum
	return s
}

func TestGenerateFromImage_MaterialMask_ItemFromVisionInPrompt(t *testing.T) {
	t.Parallel()
	const w, h = 512, 512
	e := mkSetup(t, "post_material", func() (string, error) { return "  a blue denim jacket \n", nil }, nil)
	p := e.params(makePNGBase64(t, w, h))
	p.MaskBase64 = mkMask(t, w, h, image.Rect(128, 128, 384, 384))

	result, err := e.svc.GenerateFromImage(p)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, e.rec.visionCount(), "vision names the item under the mask exactly once")
	prompt := e.onlyPrompt(t)
	assert.Contains(t, prompt, "Change only the material of a blue denim jacket")
	assert.Contains(t, prompt, "keep its color exactly the same")
	assert.Contains(t, prompt, mkEffectText)
	assert.Contains(t, prompt, "Keep all other clothes, the face, the background and everything else exactly as they are.")
}

func TestGenerateFromImage_MaterialMask_VisionFailureStopsBeforeSD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		vision func() (string, error)
	}{
		{name: "vision error", vision: func() (string, error) { return "", fmt.Errorf("vision down") }},
		{name: "empty answer", vision: func() (string, error) { return "", nil }},
		{name: "whitespace answer", vision: func() (string, error) { return " \n\t ", nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			const w, h = 512, 512
			e := mkSetup(t, "post_material", tt.vision, nil)
			p := e.params(makePNGBase64(t, w, h))
			p.MaskBase64 = mkMask(t, w, h, image.Rect(128, 128, 384, 384))

			_, err := e.svc.GenerateFromImage(p)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "не удалось определить объект")
			assert.Zero(t, e.rec.sdCount(), "SD must not be called")
			e.sessions.mu.Lock()
			assert.Empty(t, e.sessions.items)
			e.sessions.mu.Unlock()
		})
	}
}

func TestGenerateFromImage_NonMaterialMask_PromptUnchangedNoVision(t *testing.T) {
	t.Parallel()
	for _, pt := range []string{"post", "post_light", "post_skin"} {
		t.Run(pt, func(t *testing.T) {
			t.Parallel()
			const w, h = 512, 512
			src := makePNGBase64(t, w, h)

			control := mkSetup(t, pt, nil, nil)
			_, err := control.svc.GenerateFromImage(control.params(src))
			require.NoError(t, err)
			want := control.onlyPrompt(t)

			e := mkSetup(t, pt, nil, nil)
			p := e.params(src)
			p.MaskBase64 = mkMask(t, w, h, image.Rect(128, 128, 384, 384))
			_, err = e.svc.GenerateFromImage(p)
			require.NoError(t, err)

			assert.Zero(t, e.rec.visionCount(), "vision must not be called for non-material effects")
			got := e.onlyPrompt(t)
			assert.Equal(t, want, got, "prompt with mask equals prompt without mask")
			assert.NotContains(t, got, "Change only the material")
		})
	}
}

func TestGenerateFromImage_MaterialNoMask_PromptAsBeforeNoVision(t *testing.T) {
	t.Parallel()
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep_color=%v", keep), func(t *testing.T) {
			t.Parallel()
			const w, h = 512, 512
			src := makePNGBase64(t, w, h)

			control := mkSetup(t, "post", nil, nil)
			_, err := control.svc.GenerateFromImage(control.params(src))
			require.NoError(t, err)
			want := control.onlyPrompt(t)

			e := mkSetup(t, "post_material", nil, nil)
			p := e.params(src)
			p.KeepColor = keep
			_, err = e.svc.GenerateFromImage(p)
			require.NoError(t, err)

			assert.Zero(t, e.rec.visionCount(), "vision must not be called without a mask")
			got := e.onlyPrompt(t)
			assert.Equal(t, want, got, "material effect without mask sends the same prompt as any other effect")
			assert.NotContains(t, got, "Change only the material")
		})
	}
}

func TestGenerateFromImage_MaterialKeepColor_LargeColorFromSourceDetailFromKlein(t *testing.T) {
	t.Parallel()
	const w, h = 768, 512

	tests := []struct {
		name string
		mask bool
	}{
		{name: "no mask", mask: false},
		{name: "whole frame mask", mask: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := mkSetup(t, "post_material", func() (string, error) { return "a dress", nil }, mkYellowChecker)
			p := e.params(mkBlueRedPNG(t, w, h))
			p.KeepColor = true
			if tt.mask {
				p.MaskBase64 = mkMask(t, w, h, image.Rect(0, 0, w, h))
			}

			result, err := e.svc.GenerateFromImage(p)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, e.rec.sdCount())

			e.sessions.mu.Lock()
			require.Len(t, e.sessions.items, 1)
			saved := e.sessions.items[0].imageBase64
			e.sessions.mu.Unlock()
			assert.Equal(t, result.Image, saved, "session and response must hold the same image")

			img := decodePNGBase64(t, result.Image)
			require.Equal(t, w, img.Bounds().Dx(), "result width = source width")
			require.Equal(t, h, img.Bounds().Dy(), "result height = source height")

			left := mkWindow(img, w/4, h/2, 16)
			assert.Less(t, left.mean[0], 80.0, "left half R: %v", left.mean)
			assert.Less(t, left.mean[1], 80.0, "left half G: %v", left.mean)
			assert.Greater(t, left.mean[2], 175.0, "left half stays blue: %v", left.mean)

			right := mkWindow(img, w*3/4, h/2, 16)
			assert.Greater(t, right.mean[0], 175.0, "right half stays red: %v", right.mean)
			assert.Less(t, right.mean[1], 80.0, "right half G: %v", right.mean)
			assert.Less(t, right.mean[2], 80.0, "right half B: %v", right.mean)

			assert.Greater(t, left.sumSpread, 15, "fine Klein pattern visible on the left")
			assert.Greater(t, right.sumSpread, 15, "fine Klein pattern visible on the right")
		})
	}
}

func TestGenerateFromImage_KeepColorOffOrNotMaterial_ResultIsSDOutput(t *testing.T) {
	t.Parallel()
	const w, h = 1024, 1024

	tests := []struct {
		name       string
		presetType string
		keep       bool
	}{
		{name: "material, keep_color off", presetType: "post_material", keep: false},
		{name: "light effect, keep_color on", presetType: "post_light", keep: true},
		{name: "base effect, keep_color on", presetType: "post", keep: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := mkSetup(t, tt.presetType, nil, mkYellowChecker)
			p := e.params(mkBlueRedPNG(t, w, h))
			p.KeepColor = tt.keep

			result, err := e.svc.GenerateFromImage(p)
			require.NoError(t, err)
			require.NotNil(t, result)

			e.rec.mu.Lock()
			require.Len(t, e.rec.outputs, 1)
			out := e.rec.outputs[0]
			e.rec.mu.Unlock()
			require.True(t, strings.TrimSpace(out) != "")
			assert.Equal(t, out, result.Image, "result must be SD output byte for byte")
			assert.Zero(t, e.rec.visionCount())
		})
	}
}
