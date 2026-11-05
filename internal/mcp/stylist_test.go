package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
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
		return nil, fmt.Errorf("fake klein failure on call %d", n)
	}
	b64 := base64.StdEncoding.EncodeToString(styPNG(n))
	f.results = append(f.results, b64)
	return &generation.GenerateImageResult{Image: b64, Info: json.RawMessage(`{"seed":1}`)}, nil
}

func (f *fakeStylist) DescribeImage(imageBase64, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.describes = append(f.describes, styDescribeCall{image: imageBase64, prompt: prompt})
	return styFakeDescription, nil
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

func styPNG(n int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	c := color.NRGBA{R: uint8(40 * n), G: uint8(255 - 30*n), B: uint8(17 * n), A: 255}
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, c)
		}
	}
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
	look := map[string]any{"hair_cut": "short bob", "hair_color": "auburn"}

	m, _ := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": look, "variants": float64(3)})

	edits := eng.editCalls()
	require.Len(t, edits, 3)
	seeds := map[int64]bool{}
	want := buildLookInstruction(Look{HairCut: "short bob", HairColor: "auburn"})
	for i, c := range edits {
		assert.Equal(t, data, styDecodeB64(t, c.image), "call %d must use the original photo", i)
		assert.Equal(t, want, c.instruction)
		require.NotNil(t, c.seed, "call %d seed", i)
		seeds[*c.seed] = true
	}
	assert.Len(t, seeds, 3, "seeds must be pairwise different")

	named := sess.namedCalls()
	require.Len(t, named, 3)
	results := eng.resultImages()
	var returned []int64
	for i, c := range named {
		assert.Equal(t, "Стилист", c.name)
		assert.Equal(t, "stylist", c.source)
		assert.False(t, c.isPreview)
		assert.Equal(t, results[i], c.image)
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
			args := map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}}
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

func TestStylistTry_KleinFailure(t *testing.T) {
	t.Parallel()

	t.Run("second of three fails: stop, keep first", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{failOn: map[int]bool{2: true}}
		sess := &fakeSessions{}
		s, _ := styNewServer(t, eng, sess)
		photo, _ := styWritePhoto(t)

		m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(3)})

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

		_, err := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(3)})

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

		_, err := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(1)})

		require.Error(t, err)
	})

	t.Run("second not saved: not among successful", func(t *testing.T) {
		t.Parallel()
		sess := &fakeSessions{zeroOn: map[int]bool{2: true}}
		s, _ := styNewServer(t, &fakeStylist{}, sess)
		photo, _ := styWritePhoto(t)

		m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(2)})

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
			_, err = styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(1)})
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
	collect(styExec(s, "stylist_try", map[string]any{"photo_path": missing, "look": map[string]any{"hair_cut": "bob"}}))
	forbiddenB64 = append(forbiddenB64, eng.resultImages()...)

	partial := &fakeStylist{failOn: map[int]bool{2: true}}
	s2, outDir2 := styNewServer(t, partial, &fakeSessions{})
	collect(styExec(s2, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(3)}))
	forbiddenB64 = append(forbiddenB64, partial.resultImages()...)

	failing := &fakeStylist{failOn: map[int]bool{1: true}}
	s3, outDir3 := styNewServer(t, failing, &fakeSessions{})
	collect(styExec(s3, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(1)}))

	s4, outDir4 := styNewServer(t, &fakeStylist{}, &fakeSessions{zero: true})
	collect(styExec(s4, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(1)}))

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
			_, errTry := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(1)})

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
				t.Setenv(k, tt.env[k])
			}
			photo, _ := styWritePhoto(t)
			eng := &fakeStylist{}
			sess := &fakeSessions{}
			s := NewServer(Deps{OutDir: t.TempDir(), Stylist: eng, Sessions: sess, Cfg: cfg})
			RegisterTools(s)

			_, errDescribe := styExec(s, "stylist_describe", map[string]any{"photo_path": photo})
			_, errTry := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"hair_cut": "bob"}, "variants": float64(1)})

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
