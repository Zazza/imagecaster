package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
)

const styFakeDescription = "FAKE-VISION: person with medium brown hair, no glasses"

type styEditCall struct {
	image       string
	instruction string
	seed        *int64
}

type styDescribeCall struct {
	image  string
	prompt string
}

type fakeStylist struct {
	mu        sync.Mutex
	edits     []styEditCall
	describes []styDescribeCall
	results   []string
	failOn    map[int]bool
	editPNG   func(n int) []byte
	faceErr   error
	faceFailV int
	failFrom  int
	faces     []string
	rawEdit   bool
}

func (f *fakeStylist) KleinEdit(imageBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var seedCopy *int64
	if seed != nil {
		v := *seed
		seedCopy = &v
	}
	f.edits = append(f.edits, styEditCall{image: imageBase64, instruction: instruction, seed: seedCopy})
	n := len(f.edits)
	if f.failOn[n] {
		return nil, fmt.Errorf("fake edit failure on call %d", n)
	}
	pngBytes := styPNG(n)
	if f.editPNG != nil {
		pngBytes = f.editPNG(n)
	}
	if !f.rawEdit {
		pngBytes = styKeepBackground(imageBase64, pngBytes)
	}
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	f.results = append(f.results, b64)
	return &generation.GenerateImageResult{Image: b64, Info: json.RawMessage(`{"seed":1}`)}, nil
}

func styKeepBackground(inputB64 string, edit []byte) []byte {
	if i := strings.Index(inputB64, ","); strings.HasPrefix(inputB64, "data:") && i >= 0 {
		inputB64 = inputB64[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(inputB64)
	if err != nil {
		return edit
	}
	in, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return edit
	}
	out, _, err := image.Decode(bytes.NewReader(edit))
	if err != nil {
		return edit
	}
	ob, ib := out.Bounds(), in.Bounds()
	pts := styFace()
	browMin := pts[17].Y
	for _, p := range pts[17:27] {
		browMin = min(browMin, p.Y)
	}
	img := image.NewNRGBA(ob)
	for y := ob.Min.Y; y < ob.Max.Y; y++ {
		for x := ob.Min.X; x < ob.Max.X; x++ {
			ny := (float64(y-ob.Min.Y) + 0.5) / float64(ob.Dy())
			if ny >= browMin {
				img.Set(x, y, out.At(x, y))
				continue
			}
			sx := ib.Min.X + (x-ob.Min.X)*ib.Dx()/ob.Dx()
			sy := ib.Min.Y + (y-ob.Min.Y)*ib.Dy()/ob.Dy()
			img.Set(x, y, in.At(sx, sy))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (f *fakeStylist) DescribeImage(imageBase64, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.describes = append(f.describes, styDescribeCall{image: imageBase64, prompt: prompt})
	return styFakeDescription, nil
}

func (f *fakeStylist) FaceLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faces = append(f.faces, imageBase64)
	if f.faceErr != nil {
		return nil, f.faceErr
	}
	if k := f.faceFailV; k > 0 && len(f.results) >= k && imageBase64 == f.results[k-1] {
		return nil, fmt.Errorf("no face on result %d", k)
	}
	if k := f.failFrom; k > 0 {
		for j := k - 1; j < len(f.results); j++ {
			if imageBase64 == f.results[j] {
				return nil, fmt.Errorf("no face on result %d", j+1)
			}
		}
	}
	return styFace(), nil
}

func (f *fakeStylist) faceCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.faces...)
}

func styFace() []faceguard.Point {
	pts := make([]faceguard.Point, 0, 70)
	for i := 0; i < 17; i++ {
		t := math.Pi - math.Pi*float64(i)/16
		pts = append(pts, faceguard.Point{X: 0.5 + 0.25*math.Cos(t), Y: 0.40 + 0.38*math.Sin(t)})
	}
	for i := 0; i < 5; i++ {
		pts = append(pts, faceguard.Point{X: 0.30 + 0.0375*float64(i), Y: 0.33})
	}
	for i := 0; i < 5; i++ {
		pts = append(pts, faceguard.Point{X: 0.55 + 0.0375*float64(i), Y: 0.33})
	}
	for i := 0; i < 4; i++ {
		pts = append(pts, faceguard.Point{X: 0.5, Y: 0.41 + 0.035*float64(i)})
	}
	for i := 0; i < 5; i++ {
		pts = append(pts, faceguard.Point{X: 0.46 + 0.02*float64(i), Y: 0.55})
	}
	for _, cx := range []float64{0.38, 0.62} {
		pts = append(pts,
			faceguard.Point{X: cx - 0.05, Y: 0.40}, faceguard.Point{X: cx - 0.02, Y: 0.385},
			faceguard.Point{X: cx + 0.02, Y: 0.385}, faceguard.Point{X: cx + 0.05, Y: 0.40},
			faceguard.Point{X: cx + 0.02, Y: 0.415}, faceguard.Point{X: cx - 0.02, Y: 0.415})
	}
	ring := func(rx, ry float64, n int) {
		for i := 0; i < n; i++ {
			t := math.Pi + 2*math.Pi*float64(i)/float64(n)
			pts = append(pts, faceguard.Point{X: 0.5 + rx*math.Cos(t), Y: 0.64 + ry*math.Sin(t)})
		}
	}
	ring(0.09, 0.045, 12)
	ring(0.06, 0.005, 8)
	pts = append(pts, faceguard.Point{X: 0.38, Y: 0.40}, faceguard.Point{X: 0.62, Y: 0.40})
	return pts
}

func (f *fakeStylist) editCalls() []styEditCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]styEditCall(nil), f.edits...)
}

func (f *fakeStylist) resultImages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.results...)
}

func (f *fakeStylist) describeCalls() []styDescribeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]styDescribeCall(nil), f.describes...)
}

type styNamedCall struct {
	name      string
	image     string
	info      json.RawMessage
	source    string
	isPreview bool
	meta      *preset.GenerationMeta
	returned  int64
}

type fakeSessions struct {
	mu     sync.Mutex
	calls  []styNamedCall
	zero   bool
	zeroOn map[int]bool
}

func (f *fakeSessions) AddToNamedSession(name, imageBase64 string, info json.RawMessage, source string, isPreview bool, meta *preset.GenerationMeta) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.calls) + 1
	id := int64(100 + n)
	if f.zero || f.zeroOn[n] {
		id = 0
	}
	f.calls = append(f.calls, styNamedCall{name: name, image: imageBase64, info: info, source: source, isPreview: isPreview, meta: meta, returned: id})
	return id
}

func (f *fakeSessions) namedCalls() []styNamedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]styNamedCall(nil), f.calls...)
}

func styTex(seed int64, x, y int) int {
	h := uint64(seed)*0x9E3779B97F4A7C15 ^ uint64(int64(x))*0xBF58476D1CE4E5B9 ^ uint64(int64(y))*0x94D049BB133111EB
	h ^= h >> 31
	h *= 0xD6E8FEB86659FD93
	h ^= h >> 32
	return int(h%17) - 8
}

func styTint(c color.NRGBA, d int) color.NRGBA {
	f := func(v uint8) uint8 { return uint8(max(0, min(255, int(v)+d))) }
	return color.NRGBA{R: f(c.R), G: f(c.G), B: f(c.B), A: c.A}
}

func styTextured(w, h int, c color.NRGBA, seed int64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, styTint(c, styTex(seed, x, y)))
		}
	}
	return img
}

func styPNG(n int) []byte {
	img := styTextured(48, 48, color.NRGBA{R: uint8(40 * n), G: uint8(255 - 30*n), B: uint8(17 * n), A: 255}, 1)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func styDecodeB64(t *testing.T, s string) []byte {
	t.Helper()
	if i := strings.Index(s, ","); strings.HasPrefix(s, "data:") && i >= 0 {
		s = s[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	return raw
}

func styWritePhoto(t *testing.T) (string, []byte) {
	t.Helper()
	data := styPNG(0)
	path := filepath.Join(t.TempDir(), "face.png")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path, data
}

func styNewServer(t *testing.T, eng StylistEngine, sess AppSessions) (*Server, string) {
	t.Helper()
	outDir := t.TempDir()
	deps := Deps{OutDir: outDir}
	if eng != nil {
		deps.Stylist = eng
	}
	if sess != nil {
		deps.Sessions = sess
	}
	s := NewServer(deps)
	RegisterTools(s)
	return s, outDir
}

func styExec(s *Server, tool string, args map[string]any) (string, error) {
	t, ok := s.tools[tool]
	if !ok {
		return "", fmt.Errorf("tool %q not registered", tool)
	}
	return s.execTool(t, args)
}

func styCall(t *testing.T, s *Server, tool string, args map[string]any) (map[string]any, string) {
	t.Helper()
	out, err := styExec(s, tool, args)
	require.NoError(t, err, "tool %s", tool)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m), "tool %s output: %s", tool, out)
	return m, out
}

func styVariants(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["variants"].([]any)
	require.True(t, ok, "variants must be an array: %v", m)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		obj, ok := v.(map[string]any)
		require.True(t, ok)
		out = append(out, obj)
	}
	return out
}

func styIDs(variants []map[string]any) []int64 {
	out := make([]int64, 0, len(variants))
	for _, v := range variants {
		f, _ := v["id"].(float64)
		out = append(out, int64(f))
	}
	return out
}

func styHasFailure(m map[string]any) bool {
	for _, k := range []string{"failed", "error"} {
		if v, ok := m[k]; ok && v != nil && fmt.Sprint(v) != "" && fmt.Sprint(v) != "0" && fmt.Sprint(v) != "false" {
			return true
		}
	}
	return false
}

func TestAgentSessions_WritesToAgentSession(t *testing.T) {
	t.Parallel()
	fake := &fakeSessions{}
	agent := AgentSessions{Svc: fake}
	info := json.RawMessage(`{"seed":42}`)
	meta := &preset.GenerationMeta{Label: "generate"}

	id := agent.AddToSession("IMGB64", info, "sd", true, meta)

	calls := fake.namedCalls()
	require.Len(t, calls, 1)
	c := calls[0]
	assert.Equal(t, "ИИ-агент", c.name)
	assert.Equal(t, "IMGB64", c.image)
	assert.JSONEq(t, string(info), string(c.info))
	assert.Equal(t, "sd", c.source)
	assert.True(t, c.isPreview)
	assert.Same(t, meta, c.meta)
	assert.Equal(t, c.returned, id)
}

func TestBuildLookInstruction_OutfitReplacesClothing(t *testing.T) {
	t.Parallel()
	got := buildLookInstruction(Look{Outfit: "a navy suit"})
	assert.Contains(t, got, "Replace the current clothing with")
	assert.Contains(t, got, "a navy suit")
}

func TestStylistDescribe(t *testing.T) {
	t.Parallel()

	t.Run("returns local vision text and ask_user", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{}
		s, _ := styNewServer(t, eng, &fakeSessions{})
		photo, data := styWritePhoto(t)

		m, _ := styCall(t, s, "stylist_describe", map[string]any{"photo_path": photo})

		assert.Equal(t, styFakeDescription, m["description"])
		require.Contains(t, m, "ask_user")
		assert.NotEmpty(t, m["ask_user"])
		calls := eng.describeCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, data, styDecodeB64(t, calls[0].image))
	})

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "photo_path missing", args: map[string]any{}},
		{name: "photo_path empty", args: map[string]any{"photo_path": ""}},
		{name: "file does not exist", args: map[string]any{"photo_path": filepath.Join(t.TempDir(), "nope.png")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			s, _ := styNewServer(t, eng, &fakeSessions{})

			_, err := styExec(s, "stylist_describe", tt.args)

			require.Error(t, err)
			assert.Empty(t, eng.describeCalls())
		})
	}
}

func TestStylistTry_ThreeVariants(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, data := styWritePhoto(t)
	look := map[string]any{"outfit": "a navy suit", "makeup": "red lipstick"}

	m, _ := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": look, "variants": float64(3)})

	edits := eng.editCalls()
	require.Len(t, edits, 3)
	seeds := map[int64]bool{}
	want := buildLookInstruction(Look{Outfit: "a navy suit", Makeup: "red lipstick"})
	for i, c := range edits {
		assert.Equal(t, data, styDecodeB64(t, c.image), "call %d must use the original photo", i)
		assert.Equal(t, want, c.instruction)
		require.NotNil(t, c.seed, "call %d seed", i)
		seeds[*c.seed] = true
	}
	assert.Len(t, seeds, 3, "seeds must be pairwise different")

	named := sess.namedCalls()
	require.Len(t, named, 3)
	require.Len(t, eng.resultImages(), 3)
	var returned []int64
	for _, c := range named {
		assert.Equal(t, "Стилист", c.name)
		assert.Equal(t, "stylist", c.source)
		assert.False(t, c.isPreview)
		assert.NotEmpty(t, c.image)
		require.NotNil(t, c.meta)
		assert.NotEmpty(t, c.meta.Label)
		returned = append(returned, c.returned)
	}

	assert.Equal(t, "Стилист", m["session"])
	assert.Equal(t, want, m["instruction"])
	variants := styVariants(t, m)
	require.Len(t, variants, 3)
	assert.Equal(t, returned, styIDs(variants))
	for _, v := range variants {
		check, _ := v["check"].(string)
		assert.Contains(t, check, styFakeDescription)
	}
}

func TestStylistTry_VariantsCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		variants  any
		wantErr   bool
		wantCount int
	}{
		{name: "default is 2", variants: nil, wantCount: 2},
		{name: "1 allowed", variants: float64(1), wantCount: 1},
		{name: "4 allowed", variants: float64(4), wantCount: 4},
		{name: "0 rejected", variants: float64(0), wantErr: true},
		{name: "5 rejected", variants: float64(5), wantErr: true},
		{name: "negative rejected", variants: float64(-1), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess := &fakeSessions{}
			s, _ := styNewServer(t, eng, sess)
			photo, _ := styWritePhoto(t)
			args := map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}}
			if tt.variants != nil {
				args["variants"] = tt.variants
			}

			out, err := styExec(s, "stylist_try", args)

			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, eng.editCalls())
				assert.Empty(t, sess.namedCalls())
				return
			}
			require.NoError(t, err)
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &m))
			assert.Len(t, styVariants(t, m), tt.wantCount)
			assert.Len(t, eng.editCalls(), tt.wantCount)
		})
	}
}

func TestStylistTry_BadInput(t *testing.T) {
	t.Parallel()
	photo, _ := styWritePhoto(t)

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "look missing", args: map[string]any{"photo_path": photo}},
		{name: "look empty", args: map[string]any{"photo_path": photo, "look": map[string]any{}}},
		{name: "look all fields empty", args: map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "", "outfit": ""}}},
		{name: "photo_path missing", args: map[string]any{"look": map[string]any{"hair_cut": "bob"}}},
		{name: "photo file missing", args: map[string]any{"photo_path": filepath.Join(t.TempDir(), "nope.png"), "look": map[string]any{"hair_cut": "bob"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{}
			sess := &fakeSessions{}
			s, _ := styNewServer(t, eng, sess)

			_, err := styExec(s, "stylist_try", tt.args)

			require.Error(t, err)
			assert.Empty(t, eng.editCalls())
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestStylistTry_EditFailure(t *testing.T) {
	t.Parallel()

	t.Run("second of three fails: stop, keep first", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{failOn: map[int]bool{2: true}}
		sess := &fakeSessions{}
		s, _ := styNewServer(t, eng, sess)
		photo, _ := styWritePhoto(t)

		m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(3)})

		variants := styVariants(t, m)
		require.Len(t, variants, 1)
		assert.True(t, styHasFailure(m), "response must report failed/error: %s", out)
		assert.Len(t, eng.editCalls(), 2, "must stop on the first failure")
		named := sess.namedCalls()
		require.Len(t, named, 1)
		assert.Equal(t, []int64{named[0].returned}, styIDs(variants))
	})

	t.Run("first fails: tool error", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{failOn: map[int]bool{1: true}}
		sess := &fakeSessions{}
		s, _ := styNewServer(t, eng, sess)
		photo, _ := styWritePhoto(t)

		_, err := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(3)})

		require.Error(t, err)
		assert.Empty(t, sess.namedCalls())
	})
}

func TestStylistTry_SessionWriteFailure(t *testing.T) {
	t.Parallel()

	t.Run("only variant not saved: tool error", func(t *testing.T) {
		t.Parallel()
		s, _ := styNewServer(t, &fakeStylist{}, &fakeSessions{zero: true})
		photo, _ := styWritePhoto(t)

		_, err := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(1)})

		require.Error(t, err)
	})

	t.Run("second not saved: not among successful", func(t *testing.T) {
		t.Parallel()
		sess := &fakeSessions{zeroOn: map[int]bool{2: true}}
		s, _ := styNewServer(t, &fakeStylist{}, sess)
		photo, _ := styWritePhoto(t)

		m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(2)})

		ids := styIDs(styVariants(t, m))
		assert.Equal(t, []int64{sess.namedCalls()[0].returned}, ids)
		assert.NotContains(t, ids, int64(0))
		assert.True(t, styHasFailure(m), "response must report failed/error: %s", out)
	})
}

func TestStylistTools_NoEngineOrSessions(t *testing.T) {
	t.Parallel()
	photo, _ := styWritePhoto(t)

	tests := []struct {
		name string
		eng  *fakeStylist
		sess *fakeSessions
	}{
		{name: "no engine", eng: nil, sess: &fakeSessions{}},
		{name: "no sessions", eng: &fakeStylist{}, sess: nil},
		{name: "neither", eng: nil, sess: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var eng StylistEngine
			if tt.eng != nil {
				eng = tt.eng
			}
			var sess AppSessions
			if tt.sess != nil {
				sess = tt.sess
			}
			s, _ := styNewServer(t, eng, sess)

			_, err := styExec(s, "stylist_describe", map[string]any{"photo_path": photo})
			assert.Error(t, err, "describe")
			_, err = styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(1)})
			assert.Error(t, err, "try")

			if tt.eng != nil {
				assert.Empty(t, tt.eng.editCalls())
				assert.Empty(t, tt.eng.describeCalls())
			}
			if tt.sess != nil {
				assert.Empty(t, tt.sess.namedCalls())
			}
		})
	}
}

func TestStylistTools_Privacy(t *testing.T) {
	t.Parallel()
	photo, data := styWritePhoto(t)
	missing := filepath.Join(filepath.Dir(photo), "missing.png")

	var texts []string
	var forbiddenB64 []string
	collect := func(out string, err error) {
		if err != nil {
			texts = append(texts, err.Error())
		}
		if out != "" {
			texts = append(texts, out)
		}
	}

	eng := &fakeStylist{}
	s, outDir := styNewServer(t, eng, &fakeSessions{})
	collect(styExec(s, "stylist_describe", map[string]any{"photo_path": photo}))
	collect(styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"glasses": "none"}, "variants": float64(2)}))
	collect(styExec(s, "stylist_describe", map[string]any{"photo_path": missing}))
	collect(styExec(s, "stylist_try", map[string]any{"photo_path": missing, "look": map[string]any{"outfit": "a navy suit"}}))
	forbiddenB64 = append(forbiddenB64, eng.resultImages()...)

	partial := &fakeStylist{failOn: map[int]bool{2: true}}
	s2, outDir2 := styNewServer(t, partial, &fakeSessions{})
	collect(styExec(s2, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(3)}))
	forbiddenB64 = append(forbiddenB64, partial.resultImages()...)

	failing := &fakeStylist{failOn: map[int]bool{1: true}}
	s3, outDir3 := styNewServer(t, failing, &fakeSessions{})
	collect(styExec(s3, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(1)}))

	s4, outDir4 := styNewServer(t, &fakeStylist{}, &fakeSessions{zero: true})
	collect(styExec(s4, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(1)}))

	require.GreaterOrEqual(t, len(texts), 7)
	forbidden := []string{".png", ".jpg", ".jpeg", outDir, outDir2, outDir3, outDir4, photo, missing, filepath.Dir(photo),
		"iVBORw0KGgo", base64.StdEncoding.EncodeToString(data)}
	forbidden = append(forbidden, forbiddenB64...)
	for i, text := range texts {
		lower := strings.ToLower(text)
		for _, f := range forbidden {
			assert.NotContains(t, lower, strings.ToLower(f), "text %d leaks %q: %s", i, f, text)
		}
	}
}

func TestStylistTools_RequireLocalBackends(t *testing.T) {
	t.Parallel()

	const localLLM = "http://localhost:1234"
	const localSD = "http://127.0.0.1:7860"
	tests := []struct {
		name    string
		llmURL  string
		sdURL   string
		wantErr bool
	}{
		{name: "cloud LLM", llmURL: "https://api.openai.com/v1", sdURL: localSD, wantErr: true},
		{name: "cloud SD", llmURL: localLLM, sdURL: "http://8.8.8.8:7860", wantErr: true},
		{name: "both local", llmURL: localLLM, sdURL: localSD, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			photo, _ := styWritePhoto(t)
			eng := &fakeStylist{}
			sess := &fakeSessions{}
			s := NewServer(Deps{OutDir: t.TempDir(), Stylist: eng, Sessions: sess, Cfg: &config.Config{LLMUrl: tt.llmURL, SDUrl: tt.sdURL}})
			RegisterTools(s)

			_, errDescribe := styExec(s, "stylist_describe", map[string]any{"photo_path": photo})
			_, errTry := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(1)})

			if tt.wantErr {
				assert.Error(t, errDescribe)
				assert.Error(t, errTry)
				assert.Empty(t, eng.editCalls())
				assert.Empty(t, eng.describeCalls())
				assert.Empty(t, sess.namedCalls())
				return
			}
			assert.NoError(t, errDescribe)
			assert.NoError(t, errTry)
			assert.Len(t, eng.editCalls(), 1)
		})
	}
}

func TestBuildLookInstruction(t *testing.T) {
	t.Parallel()

	full := Look{
		HairCut:   "pixie-cut-AAA",
		HairColor: "platinum-BBB",
		Outfit:    "navy-suit-CCC",
		Glasses:   "round-frames-DDD",
		Makeup:    "nude-makeup-EEE",
		Free:      "freckles-FFF",
	}

	t.Run("every non-empty field is in the text in fixed order", func(t *testing.T) {
		t.Parallel()
		got := buildLookInstruction(full)
		order := []string{full.HairCut, full.HairColor, full.Outfit, full.Glasses, full.Makeup, full.Free}
		prev := -1
		for _, v := range order {
			idx := strings.Index(got, v)
			require.GreaterOrEqual(t, idx, 0, "missing %q in %q", v, got)
			assert.Greater(t, idx, prev, "%q out of order in %q", v, got)
			prev = idx
		}
	})

	single := []struct {
		name  string
		look  Look
		value string
	}{
		{name: "hair_cut", look: Look{HairCut: "bob-XYZ"}, value: "bob-XYZ"},
		{name: "hair_color", look: Look{HairColor: "copper-XYZ"}, value: "copper-XYZ"},
		{name: "outfit", look: Look{Outfit: "denim-XYZ"}, value: "denim-XYZ"},
		{name: "glasses", look: Look{Glasses: "aviator-XYZ"}, value: "aviator-XYZ"},
		{name: "makeup", look: Look{Makeup: "smoky-XYZ"}, value: "smoky-XYZ"},
		{name: "free", look: Look{Free: "beard-XYZ"}, value: "beard-XYZ"},
	}
	for _, tt := range single {
		t.Run("single field "+tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildLookInstruction(tt.look)
			assert.Contains(t, got, tt.value)
		})
	}

	t.Run("empty fields give no phrases", func(t *testing.T) {
		t.Parallel()
		got := strings.ToLower(buildLookInstruction(Look{HairCut: "bob-XYZ"}))
		assert.NotContains(t, got, "glasses")
		assert.NotContains(t, got, "makeup")
	})

	t.Run("glasses none means removal", func(t *testing.T) {
		t.Parallel()
		got := strings.ToLower(buildLookInstruction(Look{Glasses: "none"}))
		assert.Contains(t, got, "glasses")
		removal := strings.Contains(got, "remov") || strings.Contains(got, "without") ||
			strings.Contains(got, "take off") || strings.Contains(got, "no glasses")
		assert.True(t, removal, "expected removal phrase in %q", got)
	})

	suffixCases := []struct {
		name string
		look Look
		last string
	}{
		{name: "full look", look: full, last: full.Free},
		{name: "only hair cut", look: Look{HairCut: "bob-XYZ"}, last: "bob-XYZ"},
		{name: "only glasses none", look: Look{Glasses: "none"}, last: ""},
	}
	for _, tt := range suffixCases {
		t.Run("preserve suffix "+tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildLookInstruction(tt.look)
			lower := strings.ToLower(got)
			lastIdx := 0
			if tt.last != "" {
				lastIdx = strings.Index(got, tt.last)
				require.GreaterOrEqual(t, lastIdx, 0)
			}
			for _, w := range []string{"face", "identity", "pose", "background"} {
				idx := strings.LastIndex(lower, w)
				require.GreaterOrEqual(t, idx, 0, "missing %q in %q", w, got)
				assert.Greater(t, idx, lastIdx, "%q must come after look phrases in %q", w, got)
			}
		})
	}
}

func TestIsLocalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want bool
	}{
		{raw: "http://localhost:1234", want: true},
		{raw: "http://127.0.0.1:7860", want: true},
		{raw: "http://192.168.1.184:8080/api/llm", want: true},
		{raw: "http://10.0.0.5", want: true},
		{raw: "http://172.16.3.4", want: true},
		{raw: "http://[::1]:11434", want: true},
		{raw: "http://[fd00::1]", want: true},
		{raw: "https://api.openai.com", want: false},
		{raw: "http://8.8.8.8", want: false},
		{raw: "http://172.32.0.1", want: false},
		{raw: "", want: false},
		{raw: "not a url", want: false},
		{raw: "http://no-such-host.invalid", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isLocalURL(tt.raw))
		})
	}
}

func TestImageResult_NoImageBytes(t *testing.T) {
	t.Parallel()
	b64 := base64.StdEncoding.EncodeToString(styPNG(1))

	t.Run("flag on: save=false writes file and hides bytes", func(t *testing.T) {
		t.Parallel()
		s := NewServer(Deps{OutDir: t.TempDir(), NoImageBytes: true})

		out, err := s.imageResult("name", "slug", &generation.GenerateImageResult{Image: b64}, false, nil)

		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &m))
		assert.NotContains(t, m, "image_base64")
		assert.NotContains(t, out, b64)
		path, ok := m["path"].(string)
		require.True(t, ok, "path must be returned")
		saved, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, styPNG(1), saved)
	})

	t.Run("flag off: save=false returns bytes as before", func(t *testing.T) {
		t.Parallel()
		s := NewServer(Deps{OutDir: t.TempDir()})

		out, err := s.imageResult("name", "slug", &generation.GenerateImageResult{Image: b64}, false, nil)

		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &m))
		assert.Equal(t, b64, m["image_base64"])
		assert.NotContains(t, m, "path")
	})
}

func TestWantSave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		args         map[string]any
		noImageBytes bool
		want         bool
	}{
		{name: "no save arg", args: map[string]any{}, want: true},
		{name: "save true", args: map[string]any{"save": true}, want: true},
		{name: "save false", args: map[string]any{"save": false}, want: false},
		{name: "save false with NoImageBytes", args: map[string]any{"save": false}, noImageBytes: true, want: true},
		{name: "no save arg with NoImageBytes", args: map[string]any{}, noImageBytes: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewServer(Deps{OutDir: t.TempDir(), NoImageBytes: tt.noImageBytes})
			assert.Equal(t, tt.want, s.wantSave(tt.args))
		})
	}
}

func TestStylistTools_RefuseEnvProxy(t *testing.T) {
	cfg := &config.Config{LLMUrl: "http://192.168.1.184:8080/api/llm", SDUrl: "http://192.168.1.184:7860"}
	proxyVars := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"}

	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{name: "HTTP_PROXY without NO_PROXY", env: map[string]string{"HTTP_PROXY": "http://proxy.example:3128"}, wantErr: true},
		{name: "lowercase http_proxy without no_proxy", env: map[string]string{"http_proxy": "http://proxy.example:3128"}, wantErr: true},
		{name: "NO_PROXY covers LAN", env: map[string]string{"HTTP_PROXY": "http://proxy.example:3128", "NO_PROXY": "192.168.0.0/16,localhost,127.0.0.1"}, wantErr: false},
		{name: "lowercase no_proxy covers LAN", env: map[string]string{"http_proxy": "http://proxy.example:3128", "no_proxy": "192.168.0.0/16,localhost,127.0.0.1"}, wantErr: false},
		{name: "no proxy at all", env: map[string]string{}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range proxyVars {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			photo, _ := styWritePhoto(t)
			eng := &fakeStylist{}
			sess := &fakeSessions{}
			s := NewServer(Deps{OutDir: t.TempDir(), Stylist: eng, Sessions: sess, Cfg: cfg})
			RegisterTools(s)

			_, errDescribe := styExec(s, "stylist_describe", map[string]any{"photo_path": photo})
			_, errTry := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(1)})

			if tt.wantErr {
				assert.Error(t, errDescribe)
				assert.Error(t, errTry)
				assert.Empty(t, eng.editCalls())
				assert.Empty(t, eng.describeCalls())
				assert.Empty(t, sess.namedCalls())
				return
			}
			assert.NoError(t, errDescribe)
			assert.NoError(t, errTry)
			assert.Len(t, eng.editCalls(), 1)
		})
	}
}

func styPhotoPNG(w, h int, c color.NRGBA) []byte {
	return styEncode(styTextured(w, h, c, 1))
}

func styEncode(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

var styNoseSpot = [2]float64{0.5, 0.48}

func styDot(img *image.NRGBA, at [2]float64, r float64, c color.NRGBA) {
	b := img.Bounds()
	cx, cy := at[0]*float64(b.Dx()), at[1]*float64(b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, c)
			}
		}
	}
}

func styWritePortrait(t *testing.T) (string, []byte) {
	t.Helper()
	img := styTextured(200, 300, color.NRGBA{R: 128, G: 128, B: 128, A: 255}, 1)
	styDot(img, styNoseSpot, 4, color.NRGBA{R: 20, G: 20, B: 20, A: 255})
	data := styEncode(img)
	path := filepath.Join(t.TempDir(), "portrait.png")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path, data
}

func styMossPortrait(n int) []byte {
	return styPhotoPNG(200, 300, color.NRGBA{R: 230, G: uint8(10 * n), B: 20, A: 255})
}

func styMossLandscape(n int) []byte {
	return styPhotoPNG(300, 200, color.NRGBA{R: 230, G: uint8(10 * n), B: 20, A: 255})
}

func styFace0(t *testing.T, m map[string]any) string {
	t.Helper()
	face, ok := m["face"].(string)
	require.True(t, ok, "response must have string field face: %v", m)
	return face
}

func TestStylistTry_FaceProtected(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: styMossPortrait}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, data := styWritePortrait(t)

	m, _ := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(3)})

	faces := eng.faceCalls()
	require.Len(t, faces, 4, "landmarks: once on the photo and once on each moss result")
	assert.Equal(t, data, styDecodeB64(t, faces[0]), "first detection must be on the original photo")
	rawResults := eng.resultImages()
	require.Len(t, rawResults, 3)
	for i, r := range rawResults {
		assert.Contains(t, faces[1:], r, "landmarks must be detected on moss result %d", i)
	}
	assert.Equal(t, "original", styFace0(t, m))
	variants := styVariants(t, m)
	require.Len(t, variants, 3)
	for i, v := range variants {
		assert.Equal(t, "original", styFace0(t, v), "variant %d face", i)
	}

	named := sess.namedCalls()
	raw := eng.resultImages()
	require.Len(t, named, 3)
	require.Len(t, raw, 3)
	checkImages := map[string]bool{}
	for _, d := range eng.describeCalls() {
		checkImages[d.image] = true
	}
	for i, c := range named {
		assert.NotEqual(t, raw[i], c.image, "variant %d: session must get the face-restored image, not raw moss", i)
		img, err := png.Decode(bytes.NewReader(styDecodeB64(t, c.image)))
		require.NoError(t, err, "variant %d: session image must be PNG", i)
		assert.Equal(t, image.Rect(0, 0, 200, 300), img.Bounds())
		assert.True(t, checkImages[c.image], "variant %d: vision check must run on the saved image", i)
		rawImg, err := png.Decode(bytes.NewReader(styDecodeB64(t, raw[i])))
		require.NoError(t, err)
		assert.Less(t, skLumAt(img, styNoseSpot[0], styNoseSpot[1]), skLumAt(rawImg, styNoseSpot[0], styNoseSpot[1])-15,
			"variant %d: dark spot on the photo's nose must be pasted onto the edit", i)
	}
}

func TestStylistTry_FaceNotProtected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		eng     *fakeStylist
		look    map[string]any
		mayCall bool
	}{
		{name: "glasses set", eng: &fakeStylist{editPNG: styMossPortrait}, look: map[string]any{"outfit": "a navy suit", "glasses": "round frames"}, mayCall: true},
		{name: "glasses none", eng: &fakeStylist{editPNG: styMossPortrait}, look: map[string]any{"glasses": "none"}, mayCall: true},
		{name: "landmarks error", eng: &fakeStylist{editPNG: styMossPortrait, faceErr: fmt.Errorf("no face found")}, look: map[string]any{"hair_cut": "bob"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sess := &fakeSessions{}
			s, _ := styNewServer(t, tt.eng, sess)
			photo, _ := styWritePortrait(t)

			m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": tt.look, "variants": float64(2)})

			assert.True(t, strings.HasPrefix(styFace0(t, m), "not protected"), "face: %s", out)
			variants := styVariants(t, m)
			assert.Len(t, variants, 2)
			for i, v := range variants {
				assert.True(t, strings.HasPrefix(styFace0(t, v), "not protected"), "variant %d face: %s", i, out)
			}
			assert.Len(t, tt.eng.editCalls(), 2)
			assert.LessOrEqual(t, len(tt.eng.faceCalls()), 3)
			named := sess.namedCalls()
			raw := tt.eng.resultImages()
			require.Len(t, named, 2)
			for i, c := range named {
				assert.Equal(t, raw[i], c.image, "variant %d: raw moss result must be saved as is", i)
			}
			lower := strings.ToLower(out)
			for _, f := range []string{".png", ".jpg", photo, filepath.Dir(photo), "ivborw0kggo"} {
				assert.NotContains(t, lower, strings.ToLower(f))
			}
		})
	}
}

func TestBuildLookInstruction_Headwear(t *testing.T) {
	t.Parallel()
	got := buildLookInstruction(Look{Headwear: "a red beret-XYZ"})
	assert.Contains(t, got, "Put on a red beret-XYZ.")
}

func TestLook_HeadwearFromArgsAndLabel(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePhoto(t)

	m, _ := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"headwear": "straw hat-XYZ"}, "variants": float64(1)})

	edits := eng.editCalls()
	require.Len(t, edits, 1, "one variant, one Klein call, no hair-mask pass")
	assert.Contains(t, edits[0].instruction, "Put on straw hat-XYZ.")
	assert.Contains(t, m["instruction"], "Put on straw hat-XYZ.")
	named := sess.namedCalls()
	require.Len(t, named, 1)
	require.NotNil(t, named[0].meta)
	assert.Contains(t, named[0].meta.Label, "straw hat-XYZ")
}

func TestStylistTry_MergeFailureOnlyAffectsItsVariant(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: func(n int) []byte {
		if n == 2 {
			return styMossLandscape(n)
		}
		return styMossPortrait(n)
	}}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePortrait(t)

	m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(2)})

	assert.Equal(t, "original", styFace0(t, m), "top-level face must not be overwritten by a merge failure: %s", out)
	variants := styVariants(t, m)
	require.Len(t, variants, 2)
	assert.Equal(t, "original", styFace0(t, variants[0]))
	assert.NotContains(t, variants[0], "attempts", "variant 1 is not retried: %s", out)
	assert.Equal(t, float64(2), variants[1]["attempts"], "variant 2 is retried after the merge failure: %s", out)
	assert.NotContains(t, variants[1], "problem", out)
	assert.Equal(t, "original", styFace0(t, variants[1]), "variant 2 face after the retry: %s", out)
	assert.False(t, styHasFailure(m), "merge failure is not a variant failure: %s", out)

	named := sess.namedCalls()
	raw := eng.resultImages()
	require.Len(t, named, 2)
	require.Len(t, raw, 3)
	assert.NotEqual(t, raw[0], named[0].image, "variant 1 must be face-restored")
	assert.NotEqual(t, raw[1], named[1].image, "failed landscape attempt is not saved")
	img, err := png.Decode(bytes.NewReader(styDecodeB64(t, named[1].image)))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 200, 300), img.Bounds(), "variant 2 saves the retried portrait result")
	lower := strings.ToLower(out)
	for _, f := range []string{".png", ".jpg", photo, filepath.Dir(photo), "ivborw0kggo"} {
		assert.NotContains(t, lower, strings.ToLower(f))
	}
}

func TestStylistTry_NoFaceOnMossResult(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: styMossPortrait, failFrom: 2}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePortrait(t)

	m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(2)})

	assert.Len(t, eng.faceCalls(), 5, "landmarks: 1 on the photo + 1 for variant 1 + 3 attempts of variant 2")
	assert.Equal(t, "original", styFace0(t, m), "top-level face reflects the photo: %s", out)
	variants := styVariants(t, m)
	require.Len(t, variants, 2)
	assert.Equal(t, "original", styFace0(t, variants[0]))
	assert.True(t, strings.HasPrefix(styFace0(t, variants[1]), "not protected"), "variant 2 face: %s", out)

	assert.Equal(t, float64(3), variants[1]["attempts"], out)
	assert.Contains(t, variants[1], "problem", out)

	named := sess.namedCalls()
	raw := eng.resultImages()
	require.Len(t, named, 2)
	require.Len(t, raw, 4)
	assert.NotEqual(t, raw[0], named[0].image, "variant 1 must be face-restored")
	assert.Contains(t, raw[1:], named[1].image, "variant 2 must be saved as a raw moss result")
	lower := strings.ToLower(out)
	for _, f := range []string{".png", ".jpg", photo, filepath.Dir(photo), "ivborw0kggo"} {
		assert.NotContains(t, lower, strings.ToLower(f))
	}
}

func TestBuildLookInstruction_KeepSuffix(t *testing.T) {
	t.Parallel()
	const suffix = "Keep the person's face, identity, head position and angle, pose and background unchanged."
	for _, l := range []Look{{HairCut: "bob"}, {Glasses: "none"}, {Headwear: "a cap", Makeup: "nude", Free: "freckles"}} {
		got := buildLookInstruction(l)
		assert.True(t, strings.HasSuffix(got, suffix), "instruction must end with keep suffix: %q", got)
	}
}

func TestStylistTry_VariantFit(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: styMossPortrait, failFrom: 2}
	s, _ := styNewServer(t, eng, &fakeSessions{})
	photo, _ := styWritePortrait(t)

	m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(2)})

	variants := styVariants(t, m)
	require.Len(t, variants, 2)
	fit, ok := variants[0]["fit"].(map[string]any)
	require.True(t, ok, "variant 1 must have fit object: %s", out)
	for _, k := range []string{"mean", "key"} {
		v, ok := fit[k].(float64)
		require.True(t, ok, "fit.%s must be a number: %s", k, out)
		assert.InDelta(t, 0, v, 1e-3, "identical points give zero %s", k)
	}
	assert.NotContains(t, variants[1], "fit", "no fit where face was not found on the edit: %s", out)
}

func TestBuildLookInstruction_NoLooseStrands(t *testing.T) {
	t.Parallel()
	const phrase = "No loose strands of hair across the cheeks or eyes."

	got := buildLookInstruction(Look{HairCut: "bob-XYZ", HairColor: "copper", Outfit: "a coat"})
	assert.Contains(t, got, "Change the hairstyle to bob-XYZ. "+phrase)
	assert.Equal(t, 1, strings.Count(got, phrase))

	for _, l := range []Look{{HairColor: "copper"}, {Outfit: "a coat"}, {Headwear: "a cap"}, {Glasses: "none"}} {
		assert.NotContains(t, buildLookInstruction(l), phrase, "look %+v", l)
	}
}
