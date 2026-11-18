package mcp

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/generation"
)

var _ StylistGarmentEngine = (*generation.Service)(nil)

type garmentCall struct {
	image       string
	garment     string
	instruction string
	seed        *int64
}

type garmentStylist struct {
	*fakeStylist
	gmu    sync.Mutex
	plain  int
	tryOns []garmentCall
}

func (g *garmentStylist) KleinEdit(imageBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	g.gmu.Lock()
	g.plain++
	g.gmu.Unlock()
	return g.fakeStylist.KleinEdit(imageBase64, instruction, seed)
}

func (g *garmentStylist) KleinTryOn(imageBase64, garmentBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	g.gmu.Lock()
	var seedCopy *int64
	if seed != nil {
		v := *seed
		seedCopy = &v
	}
	g.tryOns = append(g.tryOns, garmentCall{image: imageBase64, garment: garmentBase64, instruction: instruction, seed: seedCopy})
	g.gmu.Unlock()
	return g.fakeStylist.KleinEdit(imageBase64, instruction, seed)
}

func (g *garmentStylist) calls() (int, []garmentCall) {
	g.gmu.Lock()
	defer g.gmu.Unlock()
	return g.plain, append([]garmentCall(nil), g.tryOns...)
}

type garmentRetryStylist struct {
	*retryStylist
	gmu      sync.Mutex
	plain    int
	garments []string
}

func (g *garmentRetryStylist) KleinEdit(imageBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	g.gmu.Lock()
	g.plain++
	g.gmu.Unlock()
	return g.retryStylist.KleinEdit(imageBase64, instruction, seed)
}

func (g *garmentRetryStylist) KleinTryOn(imageBase64, garmentBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	g.gmu.Lock()
	g.garments = append(g.garments, garmentBase64)
	g.gmu.Unlock()
	return g.retryStylist.KleinEdit(imageBase64, instruction, seed)
}

func garmentWrite(t *testing.T) (string, []byte) {
	t.Helper()
	data := styPNG(5)
	path := filepath.Join(t.TempDir(), "garment.png")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path, data
}

func TestStylistTry_GarmentVariants(t *testing.T) {
	t.Parallel()
	eng := &garmentStylist{fakeStylist: &fakeStylist{}}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, data := styWritePhoto(t)
	garment, gdata := garmentWrite(t)

	m, out := styCall(t, s, "stylist_try", map[string]any{
		"photo_path":   photo,
		"garment_path": garment,
		"look":         map[string]any{"hair_cut": "short bob"},
		"variants":     float64(3),
	})

	plain, tries := eng.calls()
	assert.Zero(t, plain, "KleinEdit must not be used when garment_path is given")
	require.Len(t, tries, 3, out)
	seeds := map[int64]bool{}
	for i, c := range tries {
		assert.Equal(t, data, styDecodeB64(t, c.image), "call %d edits the original photo", i)
		assert.Equal(t, gdata, styDecodeB64(t, c.garment), "call %d gets the garment photo", i)
		assert.Contains(t, c.instruction, "short bob", "call %d keeps the look instruction", i)
		require.NotNil(t, c.seed, "call %d seed", i)
		seeds[*c.seed] = true
	}
	assert.Len(t, seeds, 3, "seeds must be pairwise different")

	named := sess.namedCalls()
	require.Len(t, named, 3)
	for _, c := range named {
		assert.Equal(t, "Стилист", c.name)
		assert.Equal(t, "stylist", c.source)
	}
	assert.Equal(t, "Стилист", m["session"])
	variants := styVariants(t, m)
	require.Len(t, variants, 3)
	assert.Equal(t, styIDs(variants), func() []int64 {
		var ids []int64
		for _, c := range named {
			ids = append(ids, c.returned)
		}
		return ids
	}())
}

func TestStylistTry_GarmentWithoutLook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		look any
	}{
		{name: "look missing", look: nil},
		{name: "look empty", look: map[string]any{}},
		{name: "look fields empty", look: map[string]any{"hair_cut": "", "outfit": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &garmentStylist{fakeStylist: &fakeStylist{}}
			sess := &fakeSessions{}
			s, _ := styNewServer(t, eng, sess)
			photo, _ := styWritePhoto(t)
			garment, gdata := garmentWrite(t)
			args := map[string]any{"photo_path": photo, "garment_path": garment, "variants": float64(1)}
			if tt.look != nil {
				args["look"] = tt.look
			}

			m, out := styCall(t, s, "stylist_try", args)

			plain, tries := eng.calls()
			assert.Zero(t, plain)
			require.Len(t, tries, 1, out)
			assert.Equal(t, gdata, styDecodeB64(t, tries[0].garment))
			assert.Len(t, styVariants(t, m), 1)
			assert.Len(t, sess.namedCalls(), 1)
		})
	}
}

func TestStylistTry_WithoutGarmentUsesKleinEdit(t *testing.T) {
	t.Parallel()
	eng := &garmentStylist{fakeStylist: &fakeStylist{}}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePhoto(t)
	look := map[string]any{"outfit": "a navy suit"}

	m, _ := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": look, "variants": float64(2)})

	plain, tries := eng.calls()
	assert.Equal(t, 2, plain)
	assert.Empty(t, tries)
	want := buildLookInstruction(Look{Outfit: "a navy suit"})
	for _, c := range eng.editCalls() {
		assert.Equal(t, want, c.instruction)
	}
	assert.Equal(t, want, m["instruction"])
	assert.Len(t, styVariants(t, m), 2)
}

func TestStylistTry_WithoutGarmentLookStillRequired(t *testing.T) {
	t.Parallel()
	eng := &garmentStylist{fakeStylist: &fakeStylist{}}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePhoto(t)

	_, err := styExec(s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{}})

	require.Error(t, err)
	plain, tries := eng.calls()
	assert.Zero(t, plain)
	assert.Empty(t, tries)
	assert.Empty(t, sess.namedCalls())
}

func TestStylistTry_GarmentBadInput(t *testing.T) {
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
			eng := &garmentStylist{fakeStylist: &fakeStylist{}}
			sess := &fakeSessions{}
			s, _ := styNewServer(t, eng, sess)
			photo, _ := styWritePhoto(t)

			_, err := styExec(s, "stylist_try", map[string]any{
				"photo_path":   photo,
				"garment_path": tt.garment,
				"look":         map[string]any{"hair_cut": "bob"},
			})

			require.Error(t, err)
			plain, tries := eng.calls()
			assert.Zero(t, plain)
			assert.Empty(t, tries)
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestStylistTry_GarmentEngineWithoutTryOn(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePhoto(t)
	garment, _ := garmentWrite(t)

	_, err := styExec(s, "stylist_try", map[string]any{
		"photo_path":   photo,
		"garment_path": garment,
		"look":         map[string]any{"hair_cut": "bob"},
	})

	require.Error(t, err)
	assert.Empty(t, eng.editCalls(), "garment must not be silently dropped")
	assert.Empty(t, sess.namedCalls())
}

func TestStylistTry_GarmentRetryFaceMismatch(t *testing.T) {
	t.Parallel()
	face := rtySmallFace()
	eng := &garmentRetryStylist{retryStylist: &retryStylist{photo: face, script: func(n int) rtyAttempt {
		return rtyAttempt{png: rtyImage(face, rtyBG, n), mismatch: n == 1}
	}}}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	data := rtyWritePhoto(t, face)
	photo := filepath.Join(t.TempDir(), "photo.png")
	require.NoError(t, os.WriteFile(photo, data, 0o600))
	garment, gdata := garmentWrite(t)

	m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "garment_path": garment, "variants": float64(1)})

	calls := rtyLookCalls(t, eng.editCalls(), data)
	require.Len(t, calls, 2, out)
	rtyDistinctSeeds(t, calls)
	eng.gmu.Lock()
	plain, garments := eng.plain, append([]string(nil), eng.garments...)
	eng.gmu.Unlock()
	assert.Zero(t, plain)
	require.Len(t, garments, 2)
	for _, g := range garments {
		assert.Equal(t, gdata, styDecodeB64(t, g))
	}
	v := rtyOne(t, m, out)
	assert.Equal(t, float64(2), v["attempts"], out)
	assert.NotContains(t, v, "problem", out)
	assert.Len(t, sess.namedCalls(), 1)
}

func TestStylistTry_GarmentPrivacy(t *testing.T) {
	t.Parallel()
	eng := &garmentStylist{fakeStylist: &fakeStylist{}}
	s, outDir := styNewServer(t, eng, &fakeSessions{})
	photo, data := styWritePhoto(t)
	garment, gdata := garmentWrite(t)
	missing := filepath.Join(filepath.Dir(garment), "missing.png")

	var texts []string
	collect := func(out string, err error) {
		if err != nil {
			texts = append(texts, err.Error())
		}
		if out != "" {
			texts = append(texts, out)
		}
	}
	collect(styExec(s, "stylist_try", map[string]any{"photo_path": photo, "garment_path": garment, "variants": float64(2)}))
	collect(styExec(s, "stylist_try", map[string]any{"photo_path": photo, "garment_path": missing, "look": map[string]any{"outfit": "a navy suit"}}))

	require.Len(t, texts, 2)
	forbidden := []string{".png", outDir, photo, garment, missing, filepath.Dir(garment), filepath.Dir(photo),
		"iVBORw0KGgo", base64.StdEncoding.EncodeToString(data), base64.StdEncoding.EncodeToString(gdata)}
	forbidden = append(forbidden, eng.resultImages()...)
	for i, text := range texts {
		lower := strings.ToLower(text)
		for _, f := range forbidden {
			assert.NotContains(t, lower, strings.ToLower(f), "text %d leaks %q: %s", i, f, text)
		}
	}
}
