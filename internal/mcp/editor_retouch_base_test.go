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

func TestRunRetouch_DetailByBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model  string
		detail bool
	}{
		{model: "juggernautXL_ragnarokBy", detail: true},
		{model: "z-image-turbo-fp8-aio", detail: true},
		{model: "flux-2-klein-4b", detail: false},
		{model: "moss_image_sft_bf16", detail: false},
		{model: "photon_v1", detail: false},
		{model: "my_custom_merge", detail: false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			eng := &fakeRetoucher{}
			sess := newFakeItemSessions()
			sess.put(&preset.SessionItem{ID: 5, SessionID: 1, Source: "sd", Width: 100, Height: 150, Prompt: "a woman <lora:x:1>",
				Meta: &preset.GenerationMeta{Model: tt.model, Label: "кадр"}}, styPhotoPNG(100, 150, erGray))

			res := erRun(t, eng, sess, RetouchJobParams{ItemID: 5, Size: "4k", Detail: 0.25})

			calls := eng.detailCalls()
			require.Len(t, calls, 1)
			if tt.detail {
				assert.InDelta(t, 0.25, calls[0].Detail, 1e-9)
				assert.Equal(t, tt.model, calls[0].Model)
				return
			}
			assert.Zero(t, calls[0].Detail)
			assert.True(t, strings.HasPrefix(res.Detail, "skipped:"), res.Detail)
			assert.Equal(t, "done", res.Upscale)
		})
	}
}

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

	calls := eng.detailCalls()
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
