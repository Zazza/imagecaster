package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/finish"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
)

const edFakeDescription = "FAKE-EDITOR-VISION: warm evening light, golden tones, calm mood"

type edKleinCall struct {
	image    string
	prompt   string
	strength float64
}

type fakeEditor struct {
	mu        sync.Mutex
	describes []string
	faces     []string
	kleins    []edKleinCall
	faceErr   error
	facePts   []faceguard.Point
	presets   []preset.Preset
	kleinPNG  []byte
}

func (f *fakeEditor) DescribeImage(imageBase64, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.describes = append(f.describes, imageBase64)
	return edFakeDescription, nil
}

func (f *fakeEditor) FaceLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faces = append(f.faces, imageBase64)
	if f.faceErr != nil {
		return nil, f.faceErr
	}
	if f.facePts != nil {
		return f.facePts, nil
	}
	return styFace(), nil
}

func (f *fakeEditor) KleinEffect(imageBase64, prompt string, strength float64) (*generation.GenerateImageResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kleins = append(f.kleins, edKleinCall{image: imageBase64, prompt: prompt, strength: strength})
	data := f.kleinPNG
	if data == nil {
		raw, err := base64.StdEncoding.DecodeString(imageBase64)
		if err != nil {
			return nil, err
		}
		data = raw
	}
	return &generation.GenerateImageResult{Image: base64.StdEncoding.EncodeToString(data), Info: json.RawMessage(`{"seed":1}`)}, nil
}

func (f *fakeEditor) EffectPresets() ([]preset.Preset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]preset.Preset(nil), f.presets...), nil
}

func (f *fakeEditor) kleinCalls() []edKleinCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]edKleinCall(nil), f.kleins...)
}

func (f *fakeEditor) describeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.describes...)
}

func edPresets() []preset.Preset {
	return []preset.Preset{
		{ID: 1, Name: "Golden Hour", PresetType: "post_light", Prompt: "golden hour light"},
		{ID: 2, Name: "Film Grain", PresetType: "post_film", Prompt: "film grain"},
		{ID: 3, Name: "Rain", PresetType: "post_weather", Prompt: "rain"},
		{ID: 4, Name: "Soft Focus", PresetType: "post", Prompt: "soft focus"},
		{ID: 5, Name: "Studio Backdrop", PresetType: "post_background", Prompt: "studio backdrop"},
		{ID: 6, Name: "Anime Style", PresetType: "style", Prompt: "anime"},
		{ID: 7, Name: "Forest Recipe", PresetType: "recipe", Prompt: "forest"},
	}
}

func edColumnsPNG(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(x / 256 * 60), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func edWritePhoto(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "photo.png")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func edNewServer(t *testing.T, eng EditorEngine, sess AppSessions, cfg *config.Config) *Server {
	t.Helper()
	deps := Deps{OutDir: t.TempDir(), Cfg: cfg}
	if eng != nil {
		deps.Editor = eng
	}
	if sess != nil {
		deps.Sessions = sess
	}
	s := NewServer(deps)
	RegisterTools(s)
	return s
}

func edDecode(t *testing.T, b64 string) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(styDecodeB64(t, b64)))
	require.NoError(t, err)
	return img
}

func edPix(img image.Image) []color.NRGBA {
	b := img.Bounds()
	out := make([]color.NRGBA, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out = append(out, color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA))
		}
	}
	return out
}

func edExtras(t *testing.T, out string) string {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	for _, k := range []string{"id", "applied", "photo", "note", "session", "session_id", "source_item"} {
		delete(m, k)
	}
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	return strings.ToLower(string(raw))
}

func edMentionsFace(s string) bool {
	return strings.Contains(s, "лиц") || strings.Contains(s, "face")
}

func edAssertNoLeak(t *testing.T, out string, photo string) {
	t.Helper()
	assert.NotContains(t, out, "iVBORw0KGgo", "response must not contain images")
	assert.NotContains(t, out, "base64", "response must not contain images")
	if photo != "" {
		assert.NotContains(t, out, photo)
		assert.NotContains(t, out, filepath.Dir(photo))
	}
	assert.NotContains(t, strings.ToLower(out), ".png")
}

func TestEditorDescribe(t *testing.T) {
	t.Parallel()

	t.Run("photo_path: local vision text and code measures", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets()}
		data := edColumnsPNG(400, 200)
		photo := edWritePhoto(t, data)
		s := edNewServer(t, eng, &fakeSessions{}, nil)

		m, out := styCall(t, s, "editor_describe", map[string]any{"photo_path": photo})

		assert.Equal(t, edFakeDescription, m["description"])
		calls := eng.describeCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, data, styDecodeB64(t, calls[0]))
		measures, ok := m["photo"].(map[string]any)
		require.True(t, ok, "photo measures: %s", out)
		assert.EqualValues(t, 400, measures["width"])
		assert.EqualValues(t, 200, measures["height"])
		for _, k := range []string{"orientation", "exposure", "color_cast", "contrast", "saturation"} {
			assert.Contains(t, measures, k, "measure %s", k)
		}
		face, ok := m["face"].(map[string]any)
		require.True(t, ok, "face: %s", out)
		assert.Equal(t, true, face["found"])
		edAssertNoLeak(t, out, photo)
	})

	t.Run("face not found is reported, describe still works", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets(), faceErr: errors.New("no face")}
		photo := edWritePhoto(t, edColumnsPNG(200, 300))
		s := edNewServer(t, eng, &fakeSessions{}, nil)

		m, out := styCall(t, s, "editor_describe", map[string]any{"photo_path": photo})

		assert.Equal(t, edFakeDescription, m["description"])
		face, ok := m["face"].(map[string]any)
		require.True(t, ok, "face: %s", out)
		assert.NotEqual(t, true, face["found"])
	})

	t.Run("item active", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets()}
		sess, data := styItemSess()
		s := edNewServer(t, eng, sess, nil)

		m, out := styCall(t, s, "editor_describe", map[string]any{"item": "active"})

		assert.Equal(t, edFakeDescription, m["description"])
		calls := eng.describeCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, data, styDecodeB64(t, calls[0]))
		edAssertNoLeak(t, out, "")
	})

	bad := []struct {
		name string
		args map[string]any
	}{
		{name: "no photo", args: map[string]any{}},
		{name: "missing file", args: map[string]any{"photo_path": filepath.Join(t.TempDir(), "nope.png")}},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeEditor{presets: edPresets()}
			s := edNewServer(t, eng, &fakeSessions{}, nil)

			_, err := styExec(s, "editor_describe", tt.args)

			require.Error(t, err)
			assert.Empty(t, eng.describeCalls())
		})
	}
}

func TestEditorDescribe_EffectsOnlyPostGroups(t *testing.T) {
	t.Parallel()
	eng := &fakeEditor{presets: edPresets()}
	photo := edWritePhoto(t, edColumnsPNG(100, 100))
	s := edNewServer(t, eng, &fakeSessions{}, nil)

	_, out := styCall(t, s, "editor_describe", map[string]any{"photo_path": photo})

	for _, name := range []string{"Golden Hour", "Film Grain", "Rain", "Soft Focus", "Studio Backdrop"} {
		assert.Contains(t, out, name)
	}
	assert.NotContains(t, out, "Anime Style", "non post-processing presets must not be offered")
	assert.NotContains(t, out, "Forest Recipe", "recipes are not used in MCP")
}

func TestEditorApply_SavesToEditorSession(t *testing.T) {
	t.Parallel()
	eng := &fakeEditor{presets: edPresets()}
	sess := &fakeSessions{}
	photo := edWritePhoto(t, edColumnsPNG(1000, 1000))
	s := edNewServer(t, eng, sess, nil)

	m, out := styCall(t, s, "editor_apply", map[string]any{
		"photo_path": photo,
		"adjust":     map[string]any{"brightness": float64(10), "contrast": float64(5)},
		"finish":     map[string]any{"ratio": "4:5", "fit": "crop"},
	})

	calls := sess.namedCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, EditorSessionName, calls[0].name)
	assert.Equal(t, "Фоторедактор", EditorSessionName)
	assert.Equal(t, calls[0].returned, styNum(t, m, "id"))
	img := edDecode(t, calls[0].image)
	assert.Equal(t, 800, img.Bounds().Dx())
	assert.Equal(t, 1000, img.Bounds().Dy())
	measures, ok := m["photo"].(map[string]any)
	require.True(t, ok, "result measures: %s", out)
	assert.EqualValues(t, 800, measures["width"])
	assert.EqualValues(t, 1000, measures["height"])
	assert.Empty(t, eng.kleinCalls(), "no effect requested — no neural redraw")
	edAssertNoLeak(t, out, photo)
}

func TestEditorApply_ItemSavesToItsSession(t *testing.T) {
	t.Parallel()
	eng := &fakeEditor{presets: edPresets()}
	sess, _ := styItemSess()
	s := edNewServer(t, eng, sess, nil)

	m, out := styCall(t, s, "editor_apply", map[string]any{
		"item":   "active",
		"adjust": map[string]any{"saturation": float64(20)},
	})

	assert.Empty(t, sess.namedCalls(), "item result must not go to the editor session")
	calls := sess.idCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, styItemSession, calls[0].sessionID)
	assert.Equal(t, calls[0].returned, styNum(t, m, "id"))
	edAssertNoLeak(t, out, "")
}

func TestEditorApply_InvalidAdjust(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		adjust map[string]any
	}{
		{name: "brightness above range", adjust: map[string]any{"brightness": float64(500)}},
		{name: "contrast below range", adjust: map[string]any{"contrast": float64(-101)}},
		{name: "gamma below range", adjust: map[string]any{"gamma": float64(0.01)}},
		{name: "sharpen negative", adjust: map[string]any{"sharpen": float64(-1)}},
		{name: "white point above 255", adjust: map[string]any{"white_point": float64(300)}},
		{name: "not a number", adjust: map[string]any{"brightness": "bright"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeEditor{presets: edPresets()}
			sess := &fakeSessions{}
			photo := edWritePhoto(t, edColumnsPNG(100, 100))
			s := edNewServer(t, eng, sess, nil)

			_, err := styExec(s, "editor_apply", map[string]any{"photo_path": photo, "adjust": tt.adjust})

			require.Error(t, err)
			assert.Empty(t, sess.namedCalls(), "nothing must be saved")
			assert.Empty(t, eng.kleinCalls())
		})
	}
}

func TestEditorApply_InvalidFinish(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		finish map[string]any
	}{
		{name: "bad ratio", finish: map[string]any{"ratio": "5:1", "fit": "crop"}},
		{name: "bad frame", finish: map[string]any{"frame": "gold"}},
		{name: "offset out of range", finish: map[string]any{"ratio": "1:1", "fit": "crop", "offset": float64(1.5)}},
		{name: "caption too long", finish: map[string]any{"caption": strings.Repeat("a", finish.CaptionMaxLen+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeEditor{presets: edPresets()}
			sess := &fakeSessions{}
			photo := edWritePhoto(t, edColumnsPNG(100, 100))
			s := edNewServer(t, eng, sess, nil)

			_, err := styExec(s, "editor_apply", map[string]any{"photo_path": photo, "finish": tt.finish})

			require.Error(t, err)
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func edLeftFace() []faceguard.Point {
	pts := styFace()
	out := make([]faceguard.Point, len(pts))
	for i, p := range pts {
		out[i] = faceguard.Point{X: (p.X-0.5)*0.2 + 0.1, Y: p.Y}
	}
	return out
}

func TestEditorApply_FocusFace(t *testing.T) {
	t.Parallel()
	data := edColumnsPNG(1000, 500)
	src, _, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	center, err := finish.Apply(src, finish.Params{Ratio: "1:1", Fit: "crop"})
	require.NoError(t, err)

	t.Run("no face found: center crop and a note", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets(), faceErr: errors.New("no face")}
		sess := &fakeSessions{}
		s := edNewServer(t, eng, sess, nil)

		_, out := styCall(t, s, "editor_apply", map[string]any{
			"photo_path": edWritePhoto(t, data),
			"finish":     map[string]any{"ratio": "1:1", "fit": "crop", "focus": "face"},
		})

		calls := sess.namedCalls()
		require.Len(t, calls, 1)
		got := edDecode(t, calls[0].image)
		require.Equal(t, center.Bounds().Size(), got.Bounds().Size())
		assert.Equal(t, edPix(center), edPix(got), "without a face the crop must be centered")
		assert.True(t, edMentionsFace(edExtras(t, out)), "response must note that no face was found: %s", out)
	})

	t.Run("face on the left: crop keeps the face", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets(), facePts: edLeftFace()}
		sess := &fakeSessions{}
		s := edNewServer(t, eng, sess, nil)

		styCall(t, s, "editor_apply", map[string]any{
			"photo_path": edWritePhoto(t, data),
			"finish":     map[string]any{"ratio": "1:1", "fit": "crop", "focus": "face"},
		})

		calls := sess.namedCalls()
		require.Len(t, calls, 1)
		got := edDecode(t, calls[0].image)
		require.Equal(t, 500, got.Bounds().Dx())
		require.Equal(t, 500, got.Bounds().Dy())
		first := color.NRGBAModel.Convert(got.At(got.Bounds().Min.X, got.Bounds().Min.Y+250)).(color.NRGBA)
		srcX := int(first.R) + int(first.G)/60*256
		assert.LessOrEqual(t, srcX, 100-50, "face (source x 50..150) must be inside the crop, crop starts at %d", srcX)
	})
}

func TestEditorApply_Effect(t *testing.T) {
	t.Parallel()

	t.Run("known effect: redraw and face warning", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets(), kleinPNG: edColumnsPNG(120, 120)}
		sess := &fakeSessions{}
		data := edColumnsPNG(100, 100)
		s := edNewServer(t, eng, sess, nil)

		_, out := styCall(t, s, "editor_apply", map[string]any{
			"photo_path": edWritePhoto(t, data),
			"effect":     map[string]any{"name": "Golden Hour", "strength": float64(0.6)},
		})

		kc := eng.kleinCalls()
		require.Len(t, kc, 1)
		assert.Equal(t, data, styDecodeB64(t, kc[0].image))
		assert.InDelta(t, 0.6, kc[0].strength, 1e-9)
		require.Len(t, sess.namedCalls(), 1)
		assert.True(t, edMentionsFace(edExtras(t, out)), "response must warn that the face may change: %s", out)
	})

	t.Run("without effect: no face warning", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets()}
		sess := &fakeSessions{}
		s := edNewServer(t, eng, sess, nil)

		_, out := styCall(t, s, "editor_apply", map[string]any{
			"photo_path": edWritePhoto(t, edColumnsPNG(100, 100)),
			"adjust":     map[string]any{"brightness": float64(5)},
		})

		assert.False(t, edMentionsFace(edExtras(t, out)), "no effect — no face warning: %s", out)
	})

	for _, name := range []string{"No Such Effect", "Anime Style", "Forest Recipe"} {
		t.Run("rejected effect "+name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeEditor{presets: edPresets()}
			sess := &fakeSessions{}
			s := edNewServer(t, eng, sess, nil)

			_, err := styExec(s, "editor_apply", map[string]any{
				"photo_path": edWritePhoto(t, edColumnsPNG(100, 100)),
				"effect":     map[string]any{"name": name},
			})

			require.Error(t, err)
			assert.Empty(t, eng.kleinCalls())
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestEditorTools_NoEngineOrSessions(t *testing.T) {
	t.Parallel()
	photo := edWritePhoto(t, edColumnsPNG(50, 50))
	tests := []struct {
		name string
		eng  EditorEngine
		sess AppSessions
	}{
		{name: "no engine", eng: nil, sess: &fakeSessions{}},
		{name: "no sessions", eng: &fakeEditor{presets: edPresets()}, sess: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := edNewServer(t, tt.eng, tt.sess, nil)
			_, err := styExec(s, "editor_describe", map[string]any{"photo_path": photo})
			assert.Error(t, err)
			_, err = styExec(s, "editor_apply", map[string]any{"photo_path": photo, "adjust": map[string]any{"brightness": float64(5)}})
			assert.Error(t, err)
		})
	}
}

func TestEditorTools_RequireLocalBackends(t *testing.T) {
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
		{name: "both local", llmURL: localLLM, sdURL: localSD},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeEditor{presets: edPresets()}
			sess := &fakeSessions{}
			photo := edWritePhoto(t, edColumnsPNG(60, 60))
			s := edNewServer(t, eng, sess, &config.Config{LLMUrl: tt.llmURL, SDUrl: tt.sdURL})

			_, errDescribe := styExec(s, "editor_describe", map[string]any{"photo_path": photo})
			_, errApply := styExec(s, "editor_apply", map[string]any{"photo_path": photo, "effect": map[string]any{"name": "Golden Hour"}})

			if tt.wantErr {
				assert.Error(t, errDescribe)
				assert.Error(t, errApply)
				assert.Empty(t, eng.describeCalls())
				assert.Empty(t, eng.kleinCalls())
				assert.Empty(t, sess.namedCalls())
				return
			}
			assert.NoError(t, errDescribe)
			assert.NoError(t, errApply)
			assert.Len(t, sess.namedCalls(), 1)
		})
	}
}

func TestEditorPrompt(t *testing.T) {
	t.Parallel()

	t.Run("list has stylist and photo_editor", func(t *testing.T) {
		t.Parallel()
		s, _ := styNewServer(t, nil, nil)
		m := styRPC(t, s, "prompts/list", nil)
		res, ok := m["result"].(map[string]any)
		require.True(t, ok, "%v", m)
		prompts, ok := res["prompts"].([]any)
		require.True(t, ok, "%v", res)
		var names []string
		for _, p := range prompts {
			obj, ok := p.(map[string]any)
			require.True(t, ok)
			names = append(names, obj["name"].(string))
		}
		assert.ElementsMatch(t, []string{"stylist", "photo_editor"}, names)
	})

	t.Run("get photo_editor returns text", func(t *testing.T) {
		t.Parallel()
		s, _ := styNewServer(t, nil, nil)
		m := styRPC(t, s, "prompts/get", map[string]any{"name": "photo_editor"})
		require.Nil(t, m["error"], "%v", m)
		res, ok := m["result"].(map[string]any)
		require.True(t, ok, "%v", m)
		msgs, ok := res["messages"].([]any)
		require.True(t, ok, "%v", res)
		require.NotEmpty(t, msgs)
		msg, ok := msgs[0].(map[string]any)
		require.True(t, ok)
		content, ok := msg["content"].(map[string]any)
		require.True(t, ok, "%v", msg)
		text, _ := content["text"].(string)
		assert.NotEmpty(t, strings.TrimSpace(text))
		assert.Contains(t, text, "editor_describe")
		assert.Contains(t, text, "editor_apply")
	})
}

func edSolidPNG(w, h int, c color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func edMentionsFailure(s string) bool {
	s = strings.ToLower(s)
	for _, w := range []string{"не удал", "ошиб", "fail", "error"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func TestEditorDescribe_FaceErrorVsNoFace(t *testing.T) {
	t.Parallel()

	t.Run("landmarks error: found=false and face.error", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets(), faceErr: errors.New("forge timeout")}
		s := edNewServer(t, eng, &fakeSessions{}, nil)

		m, out := styCall(t, s, "editor_describe", map[string]any{"photo_path": edWritePhoto(t, edColumnsPNG(200, 300))})

		assert.Equal(t, edFakeDescription, m["description"])
		face, ok := m["face"].(map[string]any)
		require.True(t, ok, "face: %s", out)
		assert.Equal(t, false, face["found"])
		e, _ := face["error"].(string)
		assert.NotEmpty(t, strings.TrimSpace(e), "face.error must explain the failed search: %s", out)
	})

	t.Run("empty points: found=false without error", func(t *testing.T) {
		t.Parallel()
		eng := &fakeEditor{presets: edPresets(), facePts: []faceguard.Point{}}
		s := edNewServer(t, eng, &fakeSessions{}, nil)

		m, out := styCall(t, s, "editor_describe", map[string]any{"photo_path": edWritePhoto(t, edColumnsPNG(200, 300))})

		face, ok := m["face"].(map[string]any)
		require.True(t, ok, "face: %s", out)
		assert.Equal(t, false, face["found"])
		e, _ := face["error"].(string)
		assert.Empty(t, e, "no face is not an error: %s", out)
	})
}

func TestEditorApply_FocusFaceErrorVsNoFace(t *testing.T) {
	t.Parallel()
	data := edColumnsPNG(1000, 500)
	src, _, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	center, err := finish.Apply(src, finish.Params{Ratio: "1:1", Fit: "crop"})
	require.NoError(t, err)

	run := func(t *testing.T, eng *fakeEditor) string {
		t.Helper()
		sess := &fakeSessions{}
		s := edNewServer(t, eng, sess, nil)
		_, out := styCall(t, s, "editor_apply", map[string]any{
			"photo_path": edWritePhoto(t, data),
			"finish":     map[string]any{"ratio": "1:1", "fit": "crop", "focus": "face"},
		})
		calls := sess.namedCalls()
		require.Len(t, calls, 1)
		got := edDecode(t, calls[0].image)
		require.Equal(t, center.Bounds().Size(), got.Bounds().Size())
		assert.Equal(t, edPix(center), edPix(got), "focus face without a face must fall back to the center")
		return out
	}

	var errOut, noneOut string
	t.Run("search error: center and a 'search failed' note", func(t *testing.T) {
		errOut = run(t, &fakeEditor{presets: edPresets(), faceErr: errors.New("forge timeout")})
		assert.True(t, edMentionsFace(strings.ToLower(errOut)), "note must be about the face: %s", errOut)
		assert.True(t, edMentionsFailure(errOut), "note must say the face search failed: %s", errOut)
	})
	t.Run("no face: center and a 'not found' note, not a failure", func(t *testing.T) {
		noneOut = run(t, &fakeEditor{presets: edPresets(), facePts: []faceguard.Point{}})
		assert.True(t, edMentionsFace(strings.ToLower(noneOut)), "note must say no face was found: %s", noneOut)
		assert.False(t, edMentionsFailure(noneOut), "no face is not a failed search: %s", noneOut)
	})
	assert.NotEqual(t, errOut, noneOut, "failed search and no face must give different notes")
}

func TestEditorDescribe_SaturationMeasure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		c    color.NRGBA
		want float64
	}{
		{name: "gray 128", c: color.NRGBA{R: 128, G: 128, B: 128, A: 255}, want: 0},
		{name: "pure red", c: color.NRGBA{R: 255, G: 0, B: 0, A: 255}, want: 1},
		{name: "dark gray 40", c: color.NRGBA{R: 40, G: 40, B: 40, A: 255}, want: 0},
		{name: "white", c: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, want: 0},
		{name: "muted red (200,100,100)", c: color.NRGBA{R: 200, G: 100, B: 100, A: 255}, want: 100.0 / 255},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeEditor{presets: edPresets(), facePts: []faceguard.Point{}}
			s := edNewServer(t, eng, &fakeSessions{}, nil)

			m, out := styCall(t, s, "editor_describe", map[string]any{"photo_path": edWritePhoto(t, edSolidPNG(64, 48, tt.c))})

			measures, ok := m["photo"].(map[string]any)
			require.True(t, ok, "photo measures: %s", out)
			got, ok := measures["saturation"].(float64)
			require.True(t, ok, "saturation must be a number: %s", out)
			assert.InDelta(t, tt.want, got, 0.01, "%s", out)
		})
	}
}

func TestEditorPrompt_NeutralAdjustValues(t *testing.T) {
	t.Parallel()
	s, _ := styNewServer(t, nil, nil)
	m := styRPC(t, s, "prompts/get", map[string]any{"name": "photo_editor"})
	require.Nil(t, m["error"], "%v", m)
	res := m["result"].(map[string]any)
	msg := res["messages"].([]any)[0].(map[string]any)
	text, _ := msg["content"].(map[string]any)["text"].(string)
	require.NotEmpty(t, text)
	assert.Regexp(t, `gamma[^\n]{0,40}\b1(\.0)?\b`, text, "prompt must name gamma's neutral value 1")
	assert.Regexp(t, `white_point[^\n]{0,40}\b255\b`, text, "prompt must name white_point's neutral value 255")
}
