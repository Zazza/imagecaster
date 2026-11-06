package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
)

const skPhotoGray = 120

func skGrayPNG(v uint8) []byte {
	return styPhotoPNG(200, 300, color.NRGBA{R: v, G: v, B: v, A: 255})
}

func skWritePhoto(t *testing.T, v uint8) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "skin.png")
	require.NoError(t, os.WriteFile(path, skGrayPNG(v), 0o600))
	return path
}

func skDecode(t *testing.T, b64 string) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(styDecodeB64(t, b64)))
	require.NoError(t, err)
	return img
}

func skLumAt(img image.Image, nx, ny float64) float64 {
	b := img.Bounds()
	cx, cy := b.Min.X+int(nx*float64(b.Dx())), b.Min.Y+int(ny*float64(b.Dy()))
	var sum float64
	n := 0
	for y := max(b.Min.Y, cy-3); y <= min(b.Max.Y-1, cy+3); y++ {
		for x := max(b.Min.X, cx-3); x <= min(b.Max.X-1, cx+3); x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			sum += 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
			n++
		}
	}
	return sum / float64(n)
}

var skPercent = regexp.MustCompile(`(-?\d+)\s*%`)

func TestStylistTry_SkinReport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		edit     uint8
		retouch  map[string]any
		wantSkin bool
		wantPct  int
	}{
		{name: "SK4 edit 20% lighter is reported", edit: 144, wantSkin: true, wantPct: 20},
		{name: "SK4 edit 3% lighter is not reported", edit: 124},
		{name: "edit darker is not reported", edit: 96},
		{name: "SK4 skin_tone original: no report", edit: 144, retouch: map[string]any{"skin_tone": "original"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{editPNG: func(int) []byte { return skGrayPNG(tt.edit) }, rawEdit: true}
			s, _ := styNewServer(t, eng, &fakeSessions{})
			args := map[string]any{"photo_path": skWritePhoto(t, skPhotoGray), "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(2)}
			if tt.retouch != nil {
				args["retouch"] = tt.retouch
			}

			m, out := styCall(t, s, "stylist_try", args)

			variants := styVariants(t, m)
			require.Len(t, variants, 2)
			for i, v := range variants {
				if !tt.wantSkin {
					assert.NotContains(t, v, "skin", "variant %d: %s", i, out)
					continue
				}
				skin, ok := v["skin"].(string)
				require.True(t, ok, "variant %d must have string skin: %s", i, out)
				assert.Contains(t, skin, "светлее", "variant %d skin: %s", i, skin)
				mm := skPercent.FindStringSubmatch(skin)
				require.NotNil(t, mm, "skin must contain N%%: %s", skin)
				n, err := strconv.Atoi(mm[1])
				require.NoError(t, err)
				assert.InDelta(t, tt.wantPct, n, 1, "variant %d skin: %s", i, skin)
			}
		})
	}
}

func TestStylistTry_SkinToneAndLightness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		edit    uint8
		retouch map[string]any
		want    float64
	}{
		{name: "SK5 skin_tone original returns cheek tone", edit: 144, retouch: map[string]any{"skin_tone": "original"}, want: skPhotoGray},
		{name: "SK6 skin_lightness -10 darkens frame x0.9", edit: 150, retouch: map[string]any{"skin_lightness": float64(-10)}, want: 135},
		{name: "original tone then +10", edit: 144, retouch: map[string]any{"skin_tone": "original", "skin_lightness": float64(10)}, want: 132},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{editPNG: func(int) []byte { return skGrayPNG(tt.edit) }, rawEdit: true}
			sess := &fakeSessions{}
			s, _ := styNewServer(t, eng, sess)

			_, out := styCall(t, s, "stylist_try", map[string]any{
				"photo_path": skWritePhoto(t, skPhotoGray), "look": map[string]any{"outfit": "a navy suit"},
				"variants": float64(1), "retouch": tt.retouch,
			})

			named := sess.namedCalls()
			require.Len(t, named, 1, out)
			img := skDecode(t, named[0].image)
			for _, p := range [][2]float64{{0.35, 0.51}, {0.65, 0.51}, {0.05, 0.05}, {0.95, 0.95}} {
				assert.InDelta(t, tt.want, skLumAt(img, p[0], p[1]), 2, "saved image at %v", p)
			}
		})
	}
}

func TestStylistTry_SkinRetouchRejected(t *testing.T) {
	t.Parallel()
	look := map[string]any{"outfit": "a navy suit"}
	tests := []struct {
		name    string
		look    map[string]any
		retouch map[string]any
	}{
		{name: "SK6 skin_lightness 40", look: look, retouch: map[string]any{"skin_lightness": float64(40)}},
		{name: "skin_lightness -31", look: look, retouch: map[string]any{"skin_lightness": float64(-31)}},
		{name: "skin_tone unknown", look: look, retouch: map[string]any{"skin_tone": "dark"}},
		{name: "SK7 face_width 15", look: look, retouch: map[string]any{"face_width": float64(15)}},
		{name: "face_width -11 without look", retouch: map[string]any{"face_width": float64(-11)}},
		{name: "SK6 skin_lightness without look", retouch: map[string]any{"skin_lightness": float64(-10)}},
		{name: "skin_tone original without look", retouch: map[string]any{"skin_tone": "original"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := &fakeStylist{editPNG: styMossPortrait}
			sess, _ := styItemSess()
			s, _ := styNewServer(t, eng, sess)
			args := map[string]any{"item": "active", "retouch": tt.retouch}
			if tt.look != nil {
				args["look"] = tt.look
			}

			_, err := styExec(s, "stylist_try", args)

			require.Error(t, err)
			assert.Empty(t, eng.editCalls(), "no generation on invalid retouch")
			assert.Empty(t, sess.idCalls())
			assert.Empty(t, sess.namedCalls())
		})
	}
}

func TestStylistTry_FaceWidthOnly(t *testing.T) {
	t.Parallel()

	t.Run("face found: one variant without Klein", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{}
		sess, _ := styItemSess()
		sess.put(&preset.SessionItem{ID: styItemID, SessionID: styItemSession, Source: "sd", Width: 200, Height: 300}, styRetouchPNG())
		s, _ := styNewServer(t, eng, sess)

		m, out := styCall(t, s, "stylist_try", map[string]any{"item": "active", "retouch": map[string]any{"face_width": float64(-8)}})

		assert.Empty(t, eng.editCalls(), "face_width alone must not call Klein")
		assert.Len(t, styVariants(t, m), 1, out)
		require.Len(t, sess.idCalls(), 1)
	})

	t.Run("SK8 no face: error ретушь не сделана", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{faceErr: fmt.Errorf("no face found")}
		sess, _ := styItemSess()
		s, _ := styNewServer(t, eng, sess)

		_, err := styExec(s, "stylist_try", map[string]any{"item": "active", "retouch": map[string]any{"face_width": float64(-8)}})

		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "ретушь не сделана"), "error: %v", err)
		assert.Empty(t, eng.editCalls())
		assert.Empty(t, sess.idCalls())
	})
}

func TestStylistTry_FaceWidthWithLook(t *testing.T) {
	t.Parallel()
	eng := &fakeStylist{editPNG: styMossPortrait}
	sess := &fakeSessions{}
	s, _ := styNewServer(t, eng, sess)
	photo, _ := styWritePortrait(t)

	m, out := styCall(t, s, "stylist_try", map[string]any{"photo_path": photo, "look": map[string]any{"outfit": "a navy suit"}, "variants": float64(2), "retouch": map[string]any{"face_width": float64(5)}})

	assert.Len(t, eng.editCalls(), 2, out)
	assert.Len(t, styVariants(t, m), 2)
	assert.Len(t, sess.namedCalls(), 2)
}

func TestStylistTry_SkinWithoutLandmarks(t *testing.T) {
	t.Parallel()
	glassesLook := map[string]any{"outfit": "a navy suit", "glasses": "round frames"}

	t.Run("SK9 glasses: skin_lightness -10 still applied, no skin field", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{editPNG: func(int) []byte { return skGrayPNG(150) }, rawEdit: true}
		sess := &fakeSessions{}
		s, _ := styNewServer(t, eng, sess)

		m, out := styCall(t, s, "stylist_try", map[string]any{
			"photo_path": skWritePhoto(t, skPhotoGray), "look": glassesLook,
			"variants": float64(1), "retouch": map[string]any{"skin_lightness": float64(-10)},
		})

		named := sess.namedCalls()
		require.Len(t, named, 1, out)
		img := skDecode(t, named[0].image)
		for _, p := range [][2]float64{{0.35, 0.51}, {0.5, 0.4}, {0.05, 0.05}, {0.95, 0.95}} {
			assert.InDelta(t, 135, skLumAt(img, p[0], p[1]), 2, "saved image at %v", p)
		}
		variants := styVariants(t, m)
		require.Len(t, variants, 1)
		assert.NotContains(t, variants[0], "skin", out)
	})

	t.Run("SK10 glasses: skin_tone original not returned, reason is glasses", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{editPNG: func(int) []byte { return skGrayPNG(144) }, rawEdit: true}
		s, _ := styNewServer(t, eng, &fakeSessions{})

		m, out := styCall(t, s, "stylist_try", map[string]any{
			"photo_path": skWritePhoto(t, skPhotoGray), "look": glassesLook,
			"variants": float64(1), "retouch": map[string]any{"skin_tone": "original"},
		})

		variants := styVariants(t, m)
		require.Len(t, variants, 1)
		skin, ok := variants[0]["skin"].(string)
		require.True(t, ok, "skin must be a string: %s", out)
		assert.Contains(t, skin, "не возвращён")
		assert.Contains(t, strings.ToLower(skin), "очк")
		assert.NotContains(t, skin, "не найдено")
	})

	t.Run("SK11 no landmarks on edit: skin_tone original not returned", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{editPNG: func(int) []byte { return skGrayPNG(144) }, rawEdit: true, faceFailV: 1}
		s, _ := styNewServer(t, eng, &fakeSessions{})

		m, out := styCall(t, s, "stylist_try", map[string]any{
			"photo_path": skWritePhoto(t, skPhotoGray), "look": map[string]any{"outfit": "a navy suit"},
			"variants": float64(1), "retouch": map[string]any{"skin_tone": "original"},
		})

		variants := styVariants(t, m)
		require.Len(t, variants, 1)
		skin, ok := variants[0]["skin"].(string)
		require.True(t, ok, "skin must be a string: %s", out)
		assert.Contains(t, skin, "не возвращён")
	})

	t.Run("glasses without skin fields: no skin field", func(t *testing.T) {
		t.Parallel()
		eng := &fakeStylist{editPNG: func(int) []byte { return skGrayPNG(144) }, rawEdit: true}
		s, _ := styNewServer(t, eng, &fakeSessions{})

		m, out := styCall(t, s, "stylist_try", map[string]any{
			"photo_path": skWritePhoto(t, skPhotoGray), "look": glassesLook, "variants": float64(1),
		})

		variants := styVariants(t, m)
		require.Len(t, variants, 1)
		assert.NotContains(t, variants[0], "skin", out)
	})
}

func TestStylistTry_DescriptionListsSkinFields(t *testing.T) {
	t.Parallel()
	s, _ := styNewServer(t, &fakeStylist{}, &fakeSessions{})
	var out bytes.Buffer
	require.NoError(t, s.Serve(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), &out))

	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp), out.String())
	var desc string
	for _, tl := range resp.Result.Tools {
		if tl.Name == "stylist_try" {
			desc = tl.Description
		}
	}
	require.NotEmpty(t, desc, "stylist_try must be listed: %s", out.String())
	for _, f := range []string{"face_width", "skin_tone", "skin_lightness"} {
		assert.Contains(t, desc, f, "SK12")
	}
}
