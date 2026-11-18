package mcp

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/preset"
)

func TestRunRetouch_FacePointsFollowUpscale(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k", Retouch: &faceguard.Retouch{EyeColor: "blue"}})

	require.Equal(t, "done", res.Retouch)
	outs := eng.outputs()
	require.NotEmpty(t, outs)
	before, after := edDecode(t, outs[len(outs)-1]), edDecode(t, sess.idCalls()[0].image)
	b := after.Bounds()
	var sx, sy, n float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r1, g1, b1, _ := before.At(x, y).RGBA()
			r2, g2, b2, _ := after.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				sx += float64(x)
				sy += float64(y)
				n++
			}
		}
	}
	require.Positive(t, n, "eye colour must change some pixels")
	assert.InDelta(t, 0.5, sx/n/float64(b.Dx()), 0.15, "changes centred between the eyes")
	assert.InDelta(t, 0.37, sy/n/float64(b.Dy()), 0.12, "changes at eye height")
}

func TestRunRetouch_ResizeFallbackMarked(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{resized: true}
	sess := erItemSess()

	res := erRun(t, eng, sess, RetouchJobParams{ItemID: erGenItem, Size: "4k"})

	assert.True(t, strings.HasPrefix(res.Upscale, "done:"), res.Upscale)
	assert.Contains(t, res.Upscale, "ресайз")
}

func TestRunRetouch_2xCapped(t *testing.T) {
	t.Parallel()
	eng := &fakeRetoucher{}
	src := base64.StdEncoding.EncodeToString(edSolidPNG(6000, 400, erGray))

	erRun(t, eng, &fakeSessions{}, RetouchJobParams{ImageBase64: src, Size: "2x"})

	calls := eng.upCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, 7680, calls[0].TargetLong)
}

func TestEditorRetouch_TooLargeRejected(t *testing.T) {
	t.Parallel()
	env := erNewEnv(t, nil)
	env.sess.put(&preset.SessionItem{ID: 77, SessionID: erSession, Source: "import", Width: 9000, Height: 6000}, styPhotoPNG(10, 10, erGray))
	photo := edWritePhoto(t, edSolidPNG(8200, 10, erGray))

	for _, args := range []map[string]any{{"item": "77"}, {"photo_path": photo}} {
		out, err := styExec(env.s, "editor_retouch", args)
		require.Error(t, err, out)
	}
	assert.Zero(t, env.jobs(t))
}
