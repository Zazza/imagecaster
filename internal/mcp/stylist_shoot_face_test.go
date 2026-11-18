package mcp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/generation"
)

type fakeRefiner struct {
	mu       sync.Mutex
	calls    []generation.FaceRefineParams
	err      error
	broken   bool
	size     int
	stretch  float64
	returned []string
}

func (f *fakeRefiner) FaceRefine(p generation.FaceRefineParams) (string, error) {
	f.mu.Lock()
	n := len(f.calls) + 1
	f.calls = append(f.calls, p)
	f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	var out string
	if f.broken {
		out = base64.StdEncoding.EncodeToString([]byte("not a png at all"))
	} else {
		w, h := f.size, f.size
		if iw, ih := filebrowser.DecodeImageSize(p.ImageBase64); iw > 0 && ih > 0 {
			w, h = iw, ih
		}
		if f.stretch > 0 {
			w = int(float64(w) * f.stretch)
			if w < 1 {
				w = 1
			}
		}
		c := color.RGBA{R: uint8(40 * n), G: 190, B: 90, A: 255}
		out = base64.StdEncoding.EncodeToString(shootFacePNG(w, h, c))
	}
	f.mu.Lock()
	f.returned = append(f.returned, out)
	f.mu.Unlock()
	return out, nil
}

func (f *fakeRefiner) refineCalls() []generation.FaceRefineParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]generation.FaceRefineParams(nil), f.calls...)
}

func (f *fakeRefiner) hasReturned(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.returned {
		if v == s {
			return true
		}
	}
	return false
}

func shootFacePNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func shootFaceDecodeImg(b64 string) (image.Image, error) {
	if i := strings.Index(b64, ","); strings.HasPrefix(b64, "data:") && i >= 0 {
		b64 = b64[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

type landmarksFn func(imageBase64 string) ([]faceguard.Point, error)

type stubFaceEng struct {
	mu    sync.Mutex
	calls []string
	fn    landmarksFn
}

func (e *stubFaceEng) KleinEdit(string, string, *int64) (*generation.GenerateImageResult, error) {
	return nil, errors.New("klein edit is not used by stylist_shoot")
}

func (e *stubFaceEng) DescribeImage(string, string) (string, error) {
	return "stub", nil
}

func (e *stubFaceEng) FaceLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	e.mu.Lock()
	e.calls = append(e.calls, imageBase64)
	e.mu.Unlock()
	if e.fn != nil {
		return e.fn(imageBase64)
	}
	return shootFaceDefaultLandmarks(imageBase64)
}

func (e *stubFaceEng) faceCalls() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.calls...)
}

func shootFaceDefaultLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	if _, err := shootFaceDecodeImg(imageBase64); err != nil {
		return nil, errors.New("не удалось прочитать изображение")
	}
	return styFace(), nil
}

func shootFaceShiftedPts(shift float64) []faceguard.Point {
	pts := styFace()
	out := make([]faceguard.Point, len(pts))
	for i, p := range pts {
		out[i] = p
		if i%2 == 0 {
			out[i].X += shift
		}
	}
	return out
}

type shootFaceEnv struct {
	s       *Server
	people  *fakePeople
	shooter *fakeShooter
	sess    *fakeSessions
	eng     *stubFaceEng
	refiner *fakeRefiner
}

func shootFaceNewEnv(t *testing.T, eng *stubFaceEng, refiner *fakeRefiner, cfg *config.Config) *shootFaceEnv {
	t.Helper()
	env := &shootFaceEnv{
		people:  &fakePeople{people: shootPeopleAll()},
		shooter: &fakeShooter{},
		sess:    &fakeSessions{},
		eng:     eng,
		refiner: refiner,
	}
	deps := Deps{OutDir: t.TempDir(), People: env.people, Shooter: env.shooter, Sessions: env.sess, Cfg: cfg}
	if eng != nil {
		deps.Stylist = eng
	}
	if refiner != nil {
		deps.Refiner = refiner
	}
	env.s = NewServer(deps)
	RegisterTools(env.s)
	return env
}

func TestFaceCropBox(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		pts    []faceguard.Point
		w, h   int
		margin float64
		wantOK bool
		check  func(t *testing.T, rect image.Rectangle)
	}{
		{
			name:   "box contains all points and frame",
			pts:    []faceguard.Point{{X: 0.3, Y: 0.2}, {X: 0.6, Y: 0.7}},
			w:      200,
			h:      100,
			margin: 0.5,
			wantOK: true,
			check: func(t *testing.T, rect image.Rectangle) {
				for i, p := range []faceguard.Point{{X: 0.3, Y: 0.2}, {X: 0.6, Y: 0.7}} {
					x, y := p.X*200, p.Y*100
					assert.True(t, float64(rect.Min.X) <= x && x <= float64(rect.Max.X), "point %d x", i)
					assert.True(t, float64(rect.Min.Y) <= y && y <= float64(rect.Max.Y), "point %d y", i)
				}
				assert.GreaterOrEqual(t, rect.Min.X, 0)
				assert.GreaterOrEqual(t, rect.Min.Y, 0)
				assert.LessOrEqual(t, rect.Max.X, 200)
				assert.LessOrEqual(t, rect.Max.Y, 100)
			},
		},
		{
			name:   "pad equals margin times longest bbox side",
			pts:    []faceguard.Point{{X: 0.4, Y: 0.4}, {X: 0.6, Y: 0.45}},
			w:      200,
			h:      100,
			margin: 0.5,
			wantOK: true,
			check: func(t *testing.T, rect image.Rectangle) {
				bw, bh := 0.6*200-0.4*200, 0.45*100-0.4*100
				pad := 0.5 * max(bw, bh)
				assert.InDelta(t, bw+2*pad, float64(rect.Dx()), 2, "width")
				assert.InDelta(t, bh+2*pad, float64(rect.Dy()), 2, "height")
			},
		},
		{
			name:   "clamps to frame",
			pts:    []faceguard.Point{{X: 0.05, Y: 0.05}, {X: 0.2, Y: 0.2}},
			w:      100,
			h:      100,
			margin: 1.0,
			wantOK: true,
			check: func(t *testing.T, rect image.Rectangle) {
				assert.GreaterOrEqual(t, rect.Min.X, 0)
				assert.GreaterOrEqual(t, rect.Min.Y, 0)
				assert.LessOrEqual(t, rect.Max.X, 100)
				assert.LessOrEqual(t, rect.Max.Y, 100)
				x, y := 0.05*100, 0.05*100
				assert.True(t, float64(rect.Min.X) <= x && x <= float64(rect.Max.X))
				assert.True(t, float64(rect.Min.Y) <= y && y <= float64(rect.Max.Y))
			},
		},
		{
			name:   "face too small",
			pts:    []faceguard.Point{{X: 0.48, Y: 0.48}, {X: 0.52, Y: 0.52}},
			w:      48,
			h:      48,
			margin: 0.5,
			wantOK: false,
		},
		{
			name:   "no points",
			pts:    nil,
			w:      100,
			h:      100,
			margin: 0.5,
			wantOK: false,
		},
		{
			name:   "zero frame",
			pts:    []faceguard.Point{{X: 0.5, Y: 0.5}},
			w:      0,
			h:      0,
			margin: 0.5,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rect, ok := faceCropBox(tt.pts, tt.w, tt.h, tt.margin)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK && tt.check != nil {
				tt.check(t, rect)
			}
		})
	}
}

func TestCropBase64_Aspect(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	tests := []struct {
		name       string
		rect       image.Rectangle
		targetLong int
		wantW      int
		wantH      int
		wantEmpty  bool
	}{
		{name: "upscale keeps long side and aspect", rect: image.Rect(50, 20, 150, 85), targetLong: 512, wantW: 512, wantH: 333},
		{name: "downscale keeps long side and aspect", rect: image.Rect(0, 0, 200, 100), targetLong: 100, wantW: 100, wantH: 50},
		{name: "empty rect gives empty string", rect: image.Rect(10, 10, 10, 40), targetLong: 512, wantEmpty: true},
		{name: "zero target gives empty string", rect: image.Rect(0, 0, 50, 50), targetLong: 0, wantEmpty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cropBase64(src, tt.rect, tt.targetLong)
			if tt.wantEmpty {
				assert.Empty(t, got)
				return
			}
			require.NotEmpty(t, got)
			img, err := shootFaceDecodeImg(got)
			require.NoError(t, err)
			b := img.Bounds()
			assert.Equal(t, tt.wantW, b.Dx())
			assert.Equal(t, tt.wantH, b.Dy())
			cw, ch := tt.rect.Dx(), tt.rect.Dy()
			assert.InDelta(t, float64(cw)/float64(ch), float64(b.Dx())/float64(b.Dy()), 0.02, "aspect must be preserved")
		})
	}
}

func TestRemapPtsToCrop(t *testing.T) {
	t.Parallel()
	rect := image.Rect(10, 20, 110, 80)
	tests := []struct {
		name string
		pt   faceguard.Point
		want faceguard.Point
	}{
		{name: "crop min corner maps to zero", pt: faceguard.Point{X: 0.05, Y: 0.2}, want: faceguard.Point{X: 0, Y: 0}},
		{name: "crop max corner maps to one", pt: faceguard.Point{X: 0.55, Y: 0.8}, want: faceguard.Point{X: 1, Y: 1}},
		{name: "crop center maps to half", pt: faceguard.Point{X: 0.3, Y: 0.5}, want: faceguard.Point{X: 0.5, Y: 0.5}},
		{name: "outside crop is not clamped", pt: faceguard.Point{X: 0, Y: 0}, want: faceguard.Point{X: -0.1, Y: -1.0 / 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := remapPtsToCrop([]faceguard.Point{tt.pt}, rect, 200, 100)
			require.Len(t, got, 1)
			assert.InDelta(t, tt.want.X, got[0].X, 1e-9)
			assert.InDelta(t, tt.want.Y, got[0].Y, 1e-9)
		})
	}
}

func TestStylistShoot_FacePassDone(t *testing.T) {
	t.Parallel()
	env := shootFaceNewEnv(t, &stubFaceEng{}, &fakeRefiner{size: 256}, nil)

	m, out := styCall(t, env.s, "stylist_shoot", shootArgs("anna", "front full body", "side view"))

	variants := styVariants(t, m)
	require.Len(t, variants, 2, out)
	raw := env.shooter.genImages()
	require.Len(t, raw, 2)
	named := env.sess.namedCalls()
	require.Len(t, named, 2)
	calls := env.refiner.refineCalls()
	require.Len(t, calls, 2, "one refine per frame")
	assert.Len(t, env.eng.faceCalls(), 4, "landmarks of frame and of result for each shot")

	for i := range variants {
		assert.Equal(t, shootFaceDone, variants[i]["face_pass"], "variant %d", i)
		assert.Equal(t, "Стилист", named[i].name)
		assert.NotEqual(t, raw[i], named[i].image, "saved frame %d must be the blended result, not the raw generation", i)
		img, err := shootFaceDecodeImg(named[i].image)
		require.NoError(t, err, "saved frame %d must be a valid image", i)
		frame, err := shootFaceDecodeImg(raw[i])
		require.NoError(t, err)
		assert.Equal(t, frame.Bounds(), img.Bounds(), "blend keeps the frame size")

		p := calls[i]
		assert.Equal(t, variants[i]["prompt"], p.Prompt, "refine prompt %d must match the generation prompt", i)
		assert.Contains(t, p.Prompt, "<lora:anna_lora_v1:1>", "refine prompt %d keeps the lora tag", i)
		assert.Equal(t, shootNegative, p.NegativePrompt, "refine %d", i)
		assert.Equal(t, "z-image-turbo-fp8-aio", p.Model, "refine %d", i)
		assert.Equal(t, "euler", p.Sampler, "refine %d", i)
		assert.Equal(t, 8, p.Steps, "refine %d", i)
		assert.InDelta(t, 1.0, p.CfgScale, 1e-9, "refine %d", i)
		assert.InDelta(t, 0.4, p.DenoisingStrength, 1e-9, "refine %d", i)
		require.NotNil(t, p.Seed, "refine %d seed must come from the frame", i)
		assert.Equal(t, int64(i+1), *p.Seed, "refine %d seed", i)

		crop, err := shootFaceDecodeImg(p.ImageBase64)
		require.NoError(t, err, "refine input %d must be a valid image", i)
		cb := crop.Bounds()
		assert.Equal(t, shootFaceLong, max(cb.Dx(), cb.Dy()), "refine input %d long side", i)
	}
}

func TestStylistShoot_FacePassParamsByBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		person  string
		base    string
		model   string
		sampler string
		steps   int
		cfg     float64
		denoise float64
	}{
		{person: "anna", base: "Z-Image", model: "z-image-turbo-fp8-aio", sampler: "euler", steps: 8, cfg: 1, denoise: 0.4},
		{person: "boris", base: "Moss-Image", model: "moss_image_sft_bf16", sampler: "euler", steps: 30, cfg: 3, denoise: 0.5},
		{person: "vera", base: "SDXL", model: "juggernautXL_ragnarokBy", sampler: "dpm++2m_sde", steps: 30, cfg: 6, denoise: 0.5},
		{person: "gleb", base: "Flux.2", model: "flux-2-klein-4b", sampler: "euler", steps: 4, cfg: 1, denoise: 0.6},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			t.Parallel()
			env := shootFaceNewEnv(t, &stubFaceEng{}, &fakeRefiner{size: 256}, nil)

			m, out := styCall(t, env.s, "stylist_shoot", shootArgs(tt.person, "front full body"))

			assert.Equal(t, tt.base, m["base"])
			calls := env.refiner.refineCalls()
			require.Len(t, calls, 1, out)
			p := calls[0]
			assert.Equal(t, tt.model, p.Model)
			assert.Equal(t, tt.sampler, p.Sampler)
			assert.Equal(t, tt.steps, p.Steps)
			assert.InDelta(t, tt.cfg, p.CfgScale, 1e-9)
			assert.InDelta(t, tt.denoise, p.DenoisingStrength, 1e-9, "face denoise of %s", tt.base)
			assert.Contains(t, p.Prompt, "<lora:")
		})
	}
}

func TestStylistShoot_FacePassSkips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		refiner    func() *fakeRefiner
		engFn      func(refiner *fakeRefiner) landmarksFn
		wantReason string
		wantCalls  int
	}{
		{
			name: "frame landmarks fail",
			engFn: func(*fakeRefiner) landmarksFn {
				return func(string) ([]faceguard.Point, error) { return nil, errors.New("нет лица") }
			},
			wantReason: "лицо не найдено",
			wantCalls:  0,
		},
		{
			name:       "refine error",
			refiner:    func() *fakeRefiner { return &fakeRefiner{err: errors.New("sd down")} },
			wantReason: "второй проход не удался",
			wantCalls:  2,
		},
		{
			name:    "result landmarks fail",
			refiner: func() *fakeRefiner { return &fakeRefiner{size: 256} },
			engFn: func(refiner *fakeRefiner) landmarksFn {
				return func(b64 string) ([]faceguard.Point, error) {
					if refiner.hasReturned(b64) {
						return nil, errors.New("нет лица на результате")
					}
					return shootFaceDefaultLandmarks(b64)
				}
			},
			wantReason: "лицо на результате не найдено",
			wantCalls:  2,
		},
		{
			name:       "result png broken",
			refiner:    func() *fakeRefiner { return &fakeRefiner{broken: true, size: 256} },
			wantReason: "лицо на результате не найдено",
			wantCalls:  2,
		},
		{
			name:    "measure gate fails on shifted result points",
			refiner: func() *fakeRefiner { return &fakeRefiner{size: 256} },
			engFn: func(refiner *fakeRefiner) landmarksFn {
				return func(b64 string) ([]faceguard.Point, error) {
					if refiner.hasReturned(b64) {
						return shootFaceShiftedPts(0.25), nil
					}
					return shootFaceDefaultLandmarks(b64)
				}
			},
			wantReason: "лицо сместилось",
			wantCalls:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			refiner := &fakeRefiner{size: 256}
			if tt.refiner != nil {
				refiner = tt.refiner()
			}
			eng := &stubFaceEng{}
			if tt.engFn != nil {
				eng.fn = tt.engFn(refiner)
			}
			env := shootFaceNewEnv(t, eng, refiner, nil)

			m, out := styCall(t, env.s, "stylist_shoot", shootArgs("anna", "front full body", "side view"))

			variants := styVariants(t, m)
			require.Len(t, variants, 2, "series must survive a skipped face pass: %s", out)
			raw := env.shooter.genImages()
			require.Len(t, raw, 2)
			named := env.sess.namedCalls()
			require.Len(t, named, 2)
			assert.Len(t, refiner.refineCalls(), tt.wantCalls)
			for i := range variants {
				pass, _ := variants[i]["face_pass"].(string)
				assert.True(t, strings.HasPrefix(pass, "skipped:"), "variant %d face_pass: %q", i, pass)
				assert.Contains(t, pass, tt.wantReason, "variant %d face_pass", i)
				assert.Equal(t, raw[i], named[i].image, "frame %d must be saved as generated", i)
			}
		})
	}
}

func TestStylistShoot_FacePassDepsMissing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		eng     *stubFaceEng
		refiner *fakeRefiner
	}{
		{name: "no stylist engine", eng: nil, refiner: &fakeRefiner{size: 256}},
		{name: "no refiner", eng: &stubFaceEng{}, refiner: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := shootFaceNewEnv(t, tt.eng, tt.refiner, nil)

			m, out := styCall(t, env.s, "stylist_shoot", shootArgs("anna", "front full body", "side view"))

			variants := styVariants(t, m)
			require.Len(t, variants, 2, out)
			raw := env.shooter.genImages()
			require.Len(t, raw, 2)
			named := env.sess.namedCalls()
			require.Len(t, named, 2)
			for i := range variants {
				assert.Equal(t, shootFacePassMissing, variants[i]["face_pass"], "variant %d", i)
				assert.Equal(t, raw[i], named[i].image, "frame %d must be saved as generated", i)
			}
			if tt.refiner != nil {
				assert.Empty(t, tt.refiner.refineCalls())
			}
		})
	}
}

func TestStylistShoot_FacePassLocalOnly(t *testing.T) {
	t.Parallel()
	env := shootFaceNewEnv(t, &stubFaceEng{}, &fakeRefiner{size: 256}, &config.Config{
		LLMUrl: "http://localhost:1234",
		SDUrl:  "http://8.8.8.8:7860",
	})

	_, err := styExec(env.s, "stylist_shoot", shootArgs("anna", "front full body"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "не в локальной сети")
	assert.Empty(t, env.refiner.refineCalls(), "cloud SD must not leak frames to the refiner")
	assert.Empty(t, env.shooter.genCalls())
	assert.Empty(t, env.sess.namedCalls())
}

func TestStylistShoot_FacePassAspect(t *testing.T) {
	t.Parallel()
	env := shootFaceNewEnv(t, &stubFaceEng{}, &fakeRefiner{size: 256, stretch: 1.5}, nil)

	m, out := styCall(t, env.s, "stylist_shoot", shootArgs("anna", "front full body", "side view"))

	variants := styVariants(t, m)
	require.Len(t, variants, 2, out)
	raw := env.shooter.genImages()
	require.Len(t, raw, 2)
	named := env.sess.namedCalls()
	require.Len(t, named, 2)
	for i := range variants {
		assert.Equal(t, facePassSkipped("аспект результата изменился"), variants[i]["face_pass"], "variant %d", i)
		assert.Equal(t, raw[i], named[i].image, "frame %d must be saved as generated", i)
	}
}

func TestStylistShoot_Interrupted(t *testing.T) {
	t.Parallel()
	env := shootFaceNewEnv(t, &stubFaceEng{}, &fakeRefiner{err: generation.ErrInterrupted}, nil)

	m, out := styCall(t, env.s, "stylist_shoot", shootArgs("anna", "front full body", "side view"))

	variants := styVariants(t, m)
	require.Len(t, variants, 1, "series must stop after the interrupted frame: %s", out)
	assert.Equal(t, facePassSkipped("прервано"), variants[0]["face_pass"])
	assert.Equal(t, true, m["interrupted"], "result must carry the interrupted flag: %s", out)
	raw := env.shooter.genImages()
	named := env.sess.namedCalls()
	require.Len(t, named, 1)
	assert.Equal(t, raw[0], named[0].image, "interrupted frame must be saved as generated")
	require.Len(t, env.refiner.refineCalls(), 1)
}

func TestFaceSkipReason_StripsURL(t *testing.T) {
	t.Parallel()
	inner := errors.New("connection refused")
	err := &url.Error{Op: "Post", URL: "http://192.168.1.184:8080/api/sd/img2img", Err: inner}

	reason := faceSkipReason(err)

	assert.NotContains(t, reason, "http", "LAN URL must not leak into the tool response")
	assert.NotContains(t, reason, "192.168", "LAN host must not leak into the tool response")
	assert.Contains(t, reason, "connection refused")
}

func TestRefineShootFace_SkipsLargeFace(t *testing.T) {
	t.Parallel()
	big := base64.StdEncoding.EncodeToString(shootFacePNG(1024, 1536, color.RGBA{R: 90, G: 90, B: 90, A: 255}))
	wide := make([]faceguard.Point, 0, 6)
	for _, p := range []faceguard.Point{{X: 0.2, Y: 0.15}, {X: 0.8, Y: 0.15}, {X: 0.2, Y: 0.55}, {X: 0.8, Y: 0.55}, {X: 0.5, Y: 0.3}, {X: 0.5, Y: 0.6}} {
		wide = append(wide, p)
	}
	eng := &stubFaceEng{fn: func(string) ([]faceguard.Point, error) { return wide, nil }}
	refiner := &fakeRefiner{size: 256}
	m := shootModels["Z-Image"]

	out, pass, interrupted := refineShootFace(eng, refiner, big, "prompt", "negative", m, nil)

	assert.False(t, interrupted)
	assert.Equal(t, facePassSkipped("лицо уже крупное"), pass)
	assert.Empty(t, refiner.refineCalls(), "large face must not be re-rendered")
	assert.Equal(t, big, out, "frame must stay untouched")
}

func TestStylistShoot_FacePassDisabled(t *testing.T) {
	t.Parallel()
	env := shootFaceNewEnv(t, &stubFaceEng{}, &fakeRefiner{size: 256}, nil)

	m, out := styCall(t, env.s, "stylist_shoot", func() map[string]any {
		a := shootArgs("anna", "front full body", "side view")
		a["face_pass"] = false
		return a
	}())

	variants := styVariants(t, m)
	require.Len(t, variants, 2, out)
	raw := env.shooter.genImages()
	named := env.sess.namedCalls()
	require.Len(t, named, 2)
	for i := range variants {
		assert.Equal(t, "off", variants[i]["face_pass"], "variant %d", i)
		assert.Equal(t, raw[i], named[i].image, "frame %d must be saved as generated", i)
	}
	assert.Empty(t, env.refiner.refineCalls())
}

func TestStylistShoot_FacePassDefaultOff(t *testing.T) {
	t.Parallel()
	env := shootFaceNewEnv(t, &stubFaceEng{}, &fakeRefiner{size: 256}, nil)
	args := shootArgs("anna", "front full body", "side view")
	delete(args, "face_pass")

	m, out := styCall(t, env.s, "stylist_shoot", args)

	variants := styVariants(t, m)
	require.Len(t, variants, 2, out)
	raw := env.shooter.genImages()
	named := env.sess.namedCalls()
	require.Len(t, named, 2)
	for i := range variants {
		assert.Equal(t, "off", variants[i]["face_pass"], "variant %d", i)
		assert.Equal(t, raw[i], named[i].image, "frame %d must be saved as generated", i)
	}
	assert.Empty(t, env.refiner.refineCalls(), "face pass is off by default")
}
