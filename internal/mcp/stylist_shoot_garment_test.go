package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/generation"
)

var sgTryOnColor = color.RGBA{R: 12, G: 24, B: 230, A: 255}

func sgSolidPNG(w, h int, c color.RGBA) []byte {
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

func sgDecode(b64 string) (image.Image, error) {
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

func sgSize(b64 string) (int, int) {
	img, err := sgDecode(b64)
	if err != nil {
		return 48, 48
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

type sgTryOnCall struct {
	image       string
	garment     string
	instruction string
}

type sgEngine struct {
	mu      sync.Mutex
	tryOns  []sgTryOnCall
	results []string
	errOn   map[int]error
	edits   int
}

func (e *sgEngine) KleinEdit(string, string, *int64) (*generation.GenerateImageResult, error) {
	e.mu.Lock()
	e.edits++
	e.mu.Unlock()
	return nil, errors.New("klein edit is not expected in stylist_shoot")
}

func (e *sgEngine) DescribeImage(string, string) (string, error) {
	return "stub", nil
}

func (e *sgEngine) FaceLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	if _, err := sgDecode(imageBase64); err != nil {
		return nil, errors.New("не удалось прочитать изображение")
	}
	return styFace(), nil
}

func (e *sgEngine) KleinTryOn(imageBase64, garmentBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tryOns = append(e.tryOns, sgTryOnCall{image: imageBase64, garment: garmentBase64, instruction: instruction})
	n := len(e.tryOns)
	if err := e.errOn[n]; err != nil {
		return nil, err
	}
	w, h := sgSize(imageBase64)
	c := sgTryOnColor
	c.R = uint8(int(c.R) + n)
	out := base64.StdEncoding.EncodeToString(sgSolidPNG(w, h, c))
	e.results = append(e.results, out)
	return &generation.GenerateImageResult{Image: out, Info: json.RawMessage(`{"seed":1}`)}, nil
}

func (e *sgEngine) calls() []sgTryOnCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sgTryOnCall(nil), e.tryOns...)
}

func (e *sgEngine) tryOnResults() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.results...)
}

type sgRefiner struct {
	mu    sync.Mutex
	calls []generation.FaceRefineParams
}

func (r *sgRefiner) FaceRefine(p generation.FaceRefineParams) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, p)
	r.mu.Unlock()
	w, h := sgSize(p.ImageBase64)
	return base64.StdEncoding.EncodeToString(sgSolidPNG(w, h, color.RGBA{R: 200, G: 180, B: 150, A: 255})), nil
}

func (r *sgRefiner) refineCalls() []generation.FaceRefineParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]generation.FaceRefineParams(nil), r.calls...)
}

type sgEnv struct {
	s       *Server
	outDir  string
	shooter *fakeShooter
	sess    *fakeSessions
	eng     *sgEngine
}

func sgNewEnv(t *testing.T, eng StylistEngine, refiner FaceRefiner) *sgEnv {
	t.Helper()
	env := &sgEnv{outDir: t.TempDir(), shooter: &fakeShooter{}, sess: &fakeSessions{}}
	if e, ok := eng.(*sgEngine); ok {
		env.eng = e
	}
	deps := Deps{OutDir: env.outDir, People: &fakePeople{people: shootPeopleAll()}, Shooter: env.shooter, Sessions: env.sess, Stylist: eng}
	if refiner != nil {
		deps.Refiner = refiner
	}
	env.s = NewServer(deps)
	RegisterTools(env.s)
	return env
}

func sgArgs(garment string, shots ...string) map[string]any {
	args := shootArgs("anna", shots...)
	if garment != "" {
		args["garment_path"] = garment
	}
	return args
}

func TestStylistShootGarment_TryOnEachFrame(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sgNewEnv(t, eng, nil)
	garment, gdata := garmentWrite(t)

	m, out := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front full body", "side view", "back view"))

	raw := env.shooter.genImages()
	require.Len(t, raw, 3, out)
	tries := eng.calls()
	require.Len(t, tries, 3, "one try-on per frame: %s", out)
	for i, c := range tries {
		assert.Equal(t, raw[i], c.image, "try-on %d must dress the generated frame", i)
		assert.Equal(t, gdata, styDecodeB64(t, c.garment), "try-on %d gets the garment photo", i)
		assert.Contains(t, c.instruction, "a red silk midi dress with puff sleeves", "try-on %d keeps look.outfit", i)
	}
	assert.Zero(t, eng.edits)

	results := eng.tryOnResults()
	require.Len(t, results, 3)
	named := env.sess.namedCalls()
	require.Len(t, named, 3)
	variants := styVariants(t, m)
	require.Len(t, variants, 3)
	for i := range named {
		assert.Equal(t, "Стилист", named[i].name)
		assert.Equal(t, results[i], named[i].image, "frame %d must be saved as the try-on result", i)
		assert.NotEqual(t, raw[i], named[i].image, "frame %d must not be saved as the raw generation", i)
		assert.Equal(t, "done", variants[i]["garment"], "variant %d", i)
	}
}

func TestStylistShootGarment_WithoutLook(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		look any
	}{
		{name: "look missing", look: nil},
		{name: "look empty", look: map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &sgEngine{}
			env := sgNewEnv(t, eng, nil)
			garment, gdata := garmentWrite(t)
			args := sgArgs(garment, "front full body")
			delete(args, "look")
			if tt.look != nil {
				args["look"] = tt.look
			}

			m, out := styCall(t, env.s, "stylist_shoot", args)

			tries := eng.calls()
			require.Len(t, tries, 1, out)
			assert.Equal(t, gdata, styDecodeB64(t, tries[0].garment))
			variants := styVariants(t, m)
			require.Len(t, variants, 1)
			assert.Equal(t, "done", variants[0]["garment"])
			named := env.sess.namedCalls()
			require.Len(t, named, 1)
			assert.Equal(t, eng.tryOnResults()[0], named[0].image)
		})
	}
}

func TestStylistShootGarment_FacePassGetsTryOnResult(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	refiner := &sgRefiner{}
	env := sgNewEnv(t, eng, refiner)
	garment, _ := garmentWrite(t)

	m, out := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front full body", "side view"))

	raw := env.shooter.genImages()
	require.Len(t, raw, 2, out)
	require.Len(t, eng.calls(), 2, out)
	results := eng.tryOnResults()
	require.Len(t, results, 2)
	calls := refiner.refineCalls()
	require.Len(t, calls, 2, "one face pass per frame: %s", out)
	for i, p := range calls {
		crop, err := sgDecode(p.ImageBase64)
		require.NoError(t, err, "refine input %d", i)
		b := crop.Bounds()
		r, g, bl, _ := crop.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
		want := sgTryOnColor
		want.R = uint8(int(want.R) + i + 1)
		assert.InDelta(t, float64(want.R), float64(r>>8), 3, "refine input %d must come from the try-on result", i)
		assert.InDelta(t, float64(want.G), float64(g>>8), 3, "refine input %d must come from the try-on result", i)
		assert.InDelta(t, float64(want.B), float64(bl>>8), 3, "refine input %d must come from the try-on result", i)
	}
	named := env.sess.namedCalls()
	require.Len(t, named, 2)
	variants := styVariants(t, m)
	require.Len(t, variants, 2)
	for i := range named {
		assert.NotEqual(t, raw[i], named[i].image, "frame %d must not be the raw generation", i)
		assert.Equal(t, "done", variants[i]["garment"], "variant %d", i)
		assert.Equal(t, "done", variants[i]["face_pass"], "variant %d", i)
	}
}

func TestStylistShootGarment_TryOnErrorSkipsFrame(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
	}{
		{name: "plain error", err: errors.New("klein try-on failed: out of memory")},
		{name: "url error", err: &url.Error{Op: "Post", URL: "http://192.168.1.184:8080/api/sd/jobs", Err: errors.New("connection refused")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &sgEngine{errOn: map[int]error{2: tt.err}}
			env := sgNewEnv(t, eng, nil)
			garment, gdata := garmentWrite(t)

			m, out := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front", "side", "back"))

			raw := env.shooter.genImages()
			require.Len(t, raw, 3, "series must continue after a failed try-on: %s", out)
			require.Len(t, eng.calls(), 3, out)
			results := eng.tryOnResults()
			require.Len(t, results, 2)
			named := env.sess.namedCalls()
			require.Len(t, named, 3, "failed frame must still be saved: %s", out)
			variants := styVariants(t, m)
			require.Len(t, variants, 3)

			assert.Equal(t, results[0], named[0].image)
			assert.Equal(t, raw[1], named[1].image, "frame without the garment is saved as generated")
			assert.Equal(t, results[1], named[2].image)

			assert.Equal(t, "done", variants[0]["garment"])
			assert.Equal(t, "done", variants[2]["garment"])
			reason, _ := variants[1]["garment"].(string)
			require.True(t, strings.HasPrefix(reason, "skipped:"), "variant 2 garment: %q", reason)
			assert.NotEmpty(t, strings.TrimSpace(strings.TrimPrefix(reason, "skipped:")), "skip must carry a reason")
			for _, f := range []string{"http://", garment, filepath.Dir(garment), env.outDir, "iVBORw0KGgo", base64.StdEncoding.EncodeToString(gdata)} {
				assert.NotContains(t, reason, f)
			}
			assert.NotEqual(t, true, m["interrupted"], out)
		})
	}
}

func TestStylistShootGarment_Interrupted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
	}{
		{name: "interrupted", err: generation.ErrInterrupted},
		{name: "wrapped interrupted", err: fmt.Errorf("klein try-on: %w", generation.ErrInterrupted)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &sgEngine{errOn: map[int]error{2: tt.err}}
			env := sgNewEnv(t, eng, nil)
			garment, _ := garmentWrite(t)

			m, out := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front", "side", "back"))

			assert.Equal(t, true, m["interrupted"], "result must carry the interrupted flag: %s", out)
			assert.Equal(t, 2, len(env.shooter.genImages()), "series must stop at the interrupted frame: %s", out)
			assert.Len(t, eng.calls(), 2, out)
			results := eng.tryOnResults()
			require.Len(t, results, 1)
			named := env.sess.namedCalls()
			require.NotEmpty(t, named, "finished frames must be kept")
			assert.LessOrEqual(t, len(named), 2)
			assert.Equal(t, results[0], named[0].image)
			variants := styVariants(t, m)
			require.NotEmpty(t, variants)
			assert.LessOrEqual(t, len(variants), 2)
			assert.Equal(t, "done", variants[0]["garment"])
		})
	}
}

func TestStylistShootGarment_WithoutGarmentPath(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sgNewEnv(t, eng, nil)

	m, out := styCall(t, env.s, "stylist_shoot", sgArgs("", "front", "side"))

	assert.Empty(t, eng.calls(), "KleinTryOn must not be called without garment_path")
	raw := env.shooter.genImages()
	require.Len(t, raw, 2, out)
	named := env.sess.namedCalls()
	require.Len(t, named, 2)
	variants := styVariants(t, m)
	require.Len(t, variants, 2)
	for i := range variants {
		assert.NotContains(t, variants[i], "garment", "variant %d", i)
		assert.Equal(t, raw[i], named[i].image, "frame %d saved as generated", i)
	}
}

func TestStylistShootGarment_EmptyGarmentPathIsAbsent(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sgNewEnv(t, eng, nil)
	args := sgArgs("", "front")
	args["garment_path"] = ""

	m, out := styCall(t, env.s, "stylist_shoot", args)

	assert.Empty(t, eng.calls(), out)
	variants := styVariants(t, m)
	require.Len(t, variants, 1)
	assert.NotContains(t, variants[0], "garment")
}

func TestStylistShootGarment_BadGarmentPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := []struct {
		name    string
		garment string
	}{
		{name: "file does not exist", garment: filepath.Join(dir, "nope.png")},
		{name: "directory", garment: dir},
		{name: "url is not a local path", garment: "http://127.0.0.1:1/garment.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &sgEngine{}
			env := sgNewEnv(t, eng, nil)

			out, err := styExec(env.s, "stylist_shoot", sgArgs(tt.garment, "front"))

			require.Error(t, err, out)
			assert.Contains(t, err.Error(), "garment_path")
			assert.NotContains(t, err.Error(), tt.garment)
			assert.NotContains(t, err.Error(), dir)
			assert.Empty(t, env.shooter.genCalls())
			assert.Empty(t, eng.calls())
			assert.Empty(t, env.sess.namedCalls())
		})
	}
}

func TestStylistShootGarment_EngineWithoutTryOn(t *testing.T) {
	t.Parallel()
	env := sgNewEnv(t, &fakeStylist{}, nil)
	garment, _ := garmentWrite(t)

	out, err := styExec(env.s, "stylist_shoot", sgArgs(garment, "front"))

	require.Error(t, err, "garment must not be silently dropped: %s", out)
	assert.NotContains(t, err.Error(), garment)
	assert.Empty(t, env.shooter.genCalls())
	assert.Empty(t, env.sess.namedCalls())
}

func TestStylistShootGarment_Privacy(t *testing.T) {
	t.Parallel()
	eng := &sgEngine{}
	env := sgNewEnv(t, eng, nil)
	garment, gdata := garmentWrite(t)

	_, out := styCall(t, env.s, "stylist_shoot", sgArgs(garment, "front", "side"))

	forbidden := []string{garment, filepath.Dir(garment), env.outDir, ".png", "iVBORw0KGgo", base64.StdEncoding.EncodeToString(gdata)}
	forbidden = append(forbidden, eng.tryOnResults()...)
	lower := strings.ToLower(out)
	for _, f := range forbidden {
		assert.NotContains(t, lower, strings.ToLower(f))
	}
}
