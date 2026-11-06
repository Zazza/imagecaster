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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/generation"
)

const (
	rtyW = 200
	rtyH = 300
)

var (
	rtyBG   = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	rtySkin = color.NRGBA{R: 200, G: 170, B: 150, A: 255}
)

type rtyAttempt struct {
	png      []byte
	mismatch bool
}

type retryStylist struct {
	mu      sync.Mutex
	photo   []faceguard.Point
	script  func(n int) rtyAttempt
	edits   []styEditCall
	results map[string]bool
}

func (f *retryStylist) KleinEdit(imageBase64, instruction string, seed *int64) (*generation.GenerateImageResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var seedCopy *int64
	if seed != nil {
		v := *seed
		seedCopy = &v
	}
	f.edits = append(f.edits, styEditCall{image: imageBase64, instruction: instruction, seed: seedCopy})
	a := f.script(len(f.edits))
	b64 := base64.StdEncoding.EncodeToString(a.png)
	if f.results == nil {
		f.results = map[string]bool{}
	}
	f.results[b64] = a.mismatch
	return &generation.GenerateImageResult{Image: b64, Info: json.RawMessage(`{"seed":1}`)}, nil
}

func (f *retryStylist) DescribeImage(imageBase64, prompt string) (string, error) {
	return styFakeDescription, nil
}

func (f *retryStylist) FaceLandmarks(imageBase64 string) ([]faceguard.Point, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.results[imageBase64] {
		return rtyFlip(f.photo), nil
	}
	return append([]faceguard.Point(nil), f.photo...), nil
}

func (f *retryStylist) editCalls() []styEditCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]styEditCall(nil), f.edits...)
}

func rtyScale(pts []faceguard.Point, k, cx, cy float64) []faceguard.Point {
	out := make([]faceguard.Point, len(pts))
	for i, p := range pts {
		out[i] = faceguard.Point{X: cx + k*(p.X-0.5), Y: cy + k*(p.Y-0.5)}
	}
	return out
}

func rtySmallFace() []faceguard.Point { return rtyScale(styFace(), 0.5, 0.5, 0.6) }

func rtyCloseUp() []faceguard.Point { return rtyScale(styFace(), 1.8, 0.5, 0.45) }

func rtyFlip(pts []faceguard.Point) []faceguard.Point {
	minY, maxY := pts[0].Y, pts[0].Y
	for _, p := range pts {
		minY = min(minY, p.Y)
		maxY = max(maxY, p.Y)
	}
	out := make([]faceguard.Point, len(pts))
	for i, p := range pts {
		out[i] = faceguard.Point{X: p.X, Y: minY + maxY - p.Y}
	}
	return out
}

const (
	rtyPersonTex = int64(1)
	rtyOtherTex  = int64(2)
)

func rtyImage(pts []faceguard.Point, outside color.NRGBA, tag int) []byte {
	return rtyImageTex(pts, outside, tag, rtyPersonTex)
}

func rtyImageTex(pts []faceguard.Point, outside color.NRGBA, tag int, tex int64) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, rtyW, rtyH))
	minX, maxX, minY := pts[0].X, pts[0].X, pts[0].Y
	for _, p := range pts {
		minX, maxX, minY = min(minX, p.X), max(maxX, p.X), min(minY, p.Y)
	}
	for y := 0; y < rtyH; y++ {
		for x := 0; x < rtyW; x++ {
			nx, ny := (float64(x)+0.5)/rtyW, (float64(y)+0.5)/rtyH
			c := outside
			if nx >= minX-0.05 && nx < maxX+0.05 && ny >= minY-0.05 {
				c = rtyBG
			}
			if nx >= minX && nx < maxX && ny >= minY {
				c = styTint(rtySkin, 2*styTex(tex, x, y))
			}
			img.Set(x, y, c)
		}
	}
	img.Set(rtyW/2, rtyH-2, color.NRGBA{R: uint8(tag), G: 1, B: 2, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func rtyWritePhoto(t *testing.T, pts []faceguard.Point) []byte {
	t.Helper()
	return rtyImage(pts, rtyBG, 0)
}

func rtyRun(t *testing.T, eng *retryStylist, sess *fakeSessions, look map[string]any) (map[string]any, string, []byte) {
	t.Helper()
	s, _ := styNewServer(t, eng, sess)
	data := rtyWritePhoto(t, eng.photo)
	path := filepath.Join(t.TempDir(), "photo.png")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": path, "look": look, "variants": float64(1)})
	return m, out, data
}

func rtyOne(t *testing.T, m map[string]any, out string) map[string]any {
	t.Helper()
	v := styVariants(t, m)
	require.Len(t, v, 1, out)
	return v[0]
}

func rtyLookCalls(t *testing.T, edits []styEditCall, data []byte) []styEditCall {
	t.Helper()
	var out []styEditCall
	for _, c := range edits {
		assert.NotContains(t, c.instruction, rtyGreenMask, "no hair-mask pass")
		assert.Equal(t, data, styDecodeB64(t, c.image), "every attempt edits the original photo")
		out = append(out, c)
	}
	return out
}

func rtyDistinctSeeds(t *testing.T, calls []styEditCall) {
	t.Helper()
	seen := map[int64]bool{}
	for i, c := range calls {
		require.NotNil(t, c.seed, "attempt %d seed", i)
		assert.False(t, seen[*c.seed], "attempt %d repeats seed %d", i, *c.seed)
		seen[*c.seed] = true
	}
}

var rtyOutfit = map[string]any{"outfit": "a navy suit"}

const rtyGreenMask = "bright green"

func TestStylistTry_RetryFaceMismatch(t *testing.T) {
	t.Parallel()

	t.Run("RT10 first attempt face mismatch, second ok", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImage(face, rtyBG, n), mismatch: n == 1}
		}}
		sess := &fakeSessions{}

		m, out, data := rtyRun(t, eng, sess, rtyOutfit)

		calls := rtyLookCalls(t, eng.editCalls(), data)
		require.Len(t, calls, 2, out)
		rtyDistinctSeeds(t, calls)
		v := rtyOne(t, m, out)
		assert.Equal(t, float64(2), v["attempts"], out)
		assert.NotContains(t, v, "problem", out)
		assert.Equal(t, "original", styFace0(t, v), out)
		assert.Len(t, sess.namedCalls(), 1, "one saved image per variant")
	})

	t.Run("RT10 with hair_cut: no mask pass, two attempts", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImage(face, rtyBG, n), mismatch: n == 1}
		}}
		sess := &fakeSessions{}

		m, out, data := rtyRun(t, eng, sess, map[string]any{"hair_cut": "short bob"})

		calls := rtyLookCalls(t, eng.editCalls(), data)
		require.Len(t, calls, 2, "2 attempts, no hair mask: %s", out)
		rtyDistinctSeeds(t, calls)
		v := rtyOne(t, m, out)
		assert.Equal(t, float64(2), v["attempts"], out)
		assert.NotContains(t, v, "problem", out)
		assert.Equal(t, "original", styFace0(t, v), out)
		assert.Len(t, sess.namedCalls(), 1)
	})

	t.Run("RT11 all three attempts mismatch", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImage(face, rtyBG, n), mismatch: true}
		}}
		sess := &fakeSessions{}

		m, out, data := rtyRun(t, eng, sess, rtyOutfit)

		calls := rtyLookCalls(t, eng.editCalls(), data)
		require.Len(t, calls, 3, out)
		rtyDistinctSeeds(t, calls)
		v := rtyOne(t, m, out)
		assert.Equal(t, float64(3), v["attempts"], out)
		problem, ok := v["problem"].(string)
		require.True(t, ok, "problem must be a string: %s", out)
		assert.NotEmpty(t, problem)
		assert.NotContains(t, problem, "фон", "background is the same, problem is the face: %s", problem)
		assert.Len(t, sess.namedCalls(), 1, "one saved image per variant")
	})
}

func TestStylistTry_RetryBackground(t *testing.T) {
	t.Parallel()
	other := color.NRGBA{R: 20, G: 60, B: 200, A: 255}

	t.Run("RT12 first attempt changes background, second keeps it", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			bg := rtyBG
			if n == 1 {
				bg = other
			}
			return rtyAttempt{png: rtyImage(face, bg, n)}
		}}
		sess := &fakeSessions{}

		m, out, data := rtyRun(t, eng, sess, rtyOutfit)

		calls := rtyLookCalls(t, eng.editCalls(), data)
		require.Len(t, calls, 2, out)
		rtyDistinctSeeds(t, calls)
		v := rtyOne(t, m, out)
		assert.Equal(t, float64(2), v["attempts"], out)
		assert.NotContains(t, v, "problem", out)
		named := sess.namedCalls()
		require.Len(t, named, 1)
		saved := skDecode(t, named[0].image)
		assert.InDelta(t, 128, skLumAt(saved, 0.03, 0.03), 3, "saved variant is the one with the kept background")
	})

	t.Run("RT12 all three change background: problem about background", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImage(face, other, n)}
		}}
		sess := &fakeSessions{}

		m, out, data := rtyRun(t, eng, sess, rtyOutfit)

		calls := rtyLookCalls(t, eng.editCalls(), data)
		require.Len(t, calls, 3, out)
		v := rtyOne(t, m, out)
		assert.Equal(t, float64(3), v["attempts"], out)
		problem, ok := v["problem"].(string)
		require.True(t, ok, "problem must be a string: %s", out)
		assert.Contains(t, problem, "фон", out)
		assert.Equal(t, "original", styFace0(t, v), "face paste succeeded: %s", out)
		assert.Len(t, sess.namedCalls(), 1)
	})

	t.Run("У56 best attempt: paste beats no paste", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImage(face, other, n), mismatch: n != 2}
		}}
		sess := &fakeSessions{}

		m, out, _ := rtyRun(t, eng, sess, rtyOutfit)

		require.Len(t, eng.editCalls(), 3, out)
		v := rtyOne(t, m, out)
		assert.Equal(t, "original", styFace0(t, v), "the attempt with a pasted face is chosen: %s", out)
		problem, ok := v["problem"].(string)
		require.True(t, ok, "problem must be a string: %s", out)
		assert.Contains(t, problem, "фон", out)
	})

	t.Run("У56 У58 best attempt: same face texture, smaller background shift", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		gray := func(v uint8) color.NRGBA { return color.NRGBA{R: v, G: v, B: v, A: 255} }
		bgs := map[int]color.NRGBA{1: gray(220), 2: gray(184), 3: gray(204)}
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImage(face, bgs[n], n)}
		}}
		sess := &fakeSessions{}

		m, out, _ := rtyRun(t, eng, sess, rtyOutfit)

		require.Len(t, eng.editCalls(), 3, out)
		v := rtyOne(t, m, out)
		assert.Contains(t, v, "problem", out)
		named := sess.namedCalls()
		require.Len(t, named, 1)
		assert.InDelta(t, 184, skLumAt(skDecode(t, named[0].image), 0.03, 0.03), 3, "face region is identical in all attempts (equal TextureMatch), so attempt 2 with the smallest background shift wins")
	})

	t.Run("У58 best attempt: higher TextureMatch beats smaller background shift", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			if n == 2 {
				return rtyAttempt{png: rtyImageTex(face, other, n, rtyPersonTex)}
			}
			return rtyAttempt{png: rtyImageTex(face, rtyBG, n, rtyOtherTex+int64(n))}
		}}
		sess := &fakeSessions{}

		m, out, _ := rtyRun(t, eng, sess, rtyOutfit)

		require.Len(t, eng.editCalls(), 3, out)
		v := rtyOne(t, m, out)
		assert.Contains(t, v, "problem", out)
		named := sess.namedCalls()
		require.Len(t, named, 1)
		got := skDecode(t, named[0].image)
		assert.Less(t, skLumAt(got, 0.03, 0.03), 100.0, "attempt 2 (same face texture, other background) is chosen: %s", out)
	})

	t.Run("RT13 close-up: background not measurable, no retry", func(t *testing.T) {
		t.Parallel()
		face := rtyCloseUp()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImage(face, other, n)}
		}}
		sess := &fakeSessions{}

		m, out, _ := rtyRun(t, eng, sess, rtyOutfit)

		require.Len(t, eng.editCalls(), 1, out)
		v := rtyOne(t, m, out)
		assert.NotContains(t, v, "attempts", out)
		assert.NotContains(t, v, "problem", out)
		assert.Equal(t, "original", styFace0(t, v), out)
		assert.Len(t, sess.namedCalls(), 1)
	})
}

func TestStylistTry_RetryOtherPerson(t *testing.T) {
	t.Parallel()

	t.Run("RT16 first attempt has other skin detail, second keeps it", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			tex := rtyPersonTex
			if n == 1 {
				tex = rtyOtherTex
			}
			return rtyAttempt{png: rtyImageTex(face, rtyBG, n, tex)}
		}}
		sess := &fakeSessions{}

		m, out, data := rtyRun(t, eng, sess, rtyOutfit)

		calls := rtyLookCalls(t, eng.editCalls(), data)
		require.Len(t, calls, 2, out)
		rtyDistinctSeeds(t, calls)
		v := rtyOne(t, m, out)
		assert.Equal(t, float64(2), v["attempts"], out)
		assert.NotContains(t, v, "problem", out)
		assert.Equal(t, "original", styFace0(t, v), out)
		assert.Len(t, sess.namedCalls(), 1, "one saved image per variant")
	})

	t.Run("RT16 all three attempts show another person", func(t *testing.T) {
		t.Parallel()
		face := rtySmallFace()
		eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
			return rtyAttempt{png: rtyImageTex(face, rtyBG, n, rtyOtherTex+int64(n))}
		}}
		sess := &fakeSessions{}

		m, out, data := rtyRun(t, eng, sess, rtyOutfit)

		calls := rtyLookCalls(t, eng.editCalls(), data)
		require.Len(t, calls, 3, out)
		rtyDistinctSeeds(t, calls)
		v := rtyOne(t, m, out)
		assert.Equal(t, float64(3), v["attempts"], out)
		problem, ok := v["problem"].(string)
		require.True(t, ok, "problem must be a string: %s", out)
		assert.Contains(t, problem, "другого человека", out)
		assert.NotContains(t, problem, "фон", "background is the same: %s", problem)
		assert.Len(t, sess.namedCalls(), 1, "one saved image per variant")
	})
}

func TestStylistTry_NoRetryWhenOk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		look      map[string]any
		variants  int
		wantEdits int
	}{
		{name: "RT17 hair_cut, 1 variant: exactly one edit", look: map[string]any{"hair_cut": "short bob"}, variants: 1, wantEdits: 1},
		{name: "RT17 hair_color, 1 variant: exactly one edit", look: map[string]any{"hair_color": "copper"}, variants: 1, wantEdits: 1},
		{name: "RT17 headwear, 1 variant: exactly one edit", look: map[string]any{"headwear": "a cap"}, variants: 1, wantEdits: 1},
		{name: "RT17 hair_cut, 2 variants: one edit each", look: map[string]any{"hair_cut": "short bob"}, variants: 2, wantEdits: 2},
		{name: "outfit, 2 variants: one edit each", look: rtyOutfit, variants: 2, wantEdits: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			face := rtySmallFace()
			eng := &retryStylist{photo: face, script: func(n int) rtyAttempt {
				return rtyAttempt{png: rtyImage(face, rtyBG, n)}
			}}
			sess := &fakeSessions{}
			s, _ := styNewServer(t, eng, sess)
			path := filepath.Join(t.TempDir(), "photo.png")
			require.NoError(t, os.WriteFile(path, rtyWritePhoto(t, face), 0o600))

			m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": path, "look": tt.look, "variants": float64(tt.variants)})

			edits := eng.editCalls()
			require.Len(t, edits, tt.wantEdits, out)
			for i, c := range edits {
				assert.NotContains(t, c.instruction, rtyGreenMask, "edit %d is not a hair-mask pass", i)
			}
			assert.NotContains(t, m, "old_hair", out)
			variants := styVariants(t, m)
			require.Len(t, variants, tt.variants)
			for i, v := range variants {
				assert.NotContains(t, v, "attempts", "variant %d: %s", i, out)
				assert.NotContains(t, v, "problem", "variant %d: %s", i, out)
				assert.Equal(t, "original", styFace0(t, v), fmt.Sprintf("variant %d", i))
			}
			assert.Len(t, sess.namedCalls(), tt.variants)
		})
	}
}
