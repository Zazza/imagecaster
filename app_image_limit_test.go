package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/config"
)

var (
	appLargePNGOnce  sync.Once
	appLargePNG      string
	appOverLimitOnce sync.Once
	appOverLimit     string
)

func appLargeNoisePNGBase64(t *testing.T) string {
	t.Helper()
	appLargePNGOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 2400, 2200))
		rand.New(rand.NewSource(1)).Read(img.Pix)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		appLargePNG = base64.StdEncoding.EncodeToString(buf.Bytes())
	})
	require.Greater(t, len(appLargePNG), 22*1024*1024)
	require.Less(t, len(appLargePNG), config.MaxImageBase64Len)
	return appLargePNG
}

func appOverLimitBase64() string {
	appOverLimitOnce.Do(func() {
		appOverLimit = strings.Repeat("A", config.MaxImageBase64Len+1)
	})
	return appOverLimit
}

func TestPreviewAdjust_LargePNGPassesSizeCheck(t *testing.T) {
	app := &App{}
	b64 := appLargeNoisePNGBase64(t)

	_, err := app.PreviewAdjust(b64, AdjustParams{}, 512)
	if err != nil {
		assert.NotContains(t, err.Error(), "too large")
	}
	assert.NoError(t, err)
}

func TestAutoAdjust_LargePNGPassesSizeCheck(t *testing.T) {
	app := &App{}
	b64 := appLargeNoisePNGBase64(t)

	_, err := app.AutoAdjust(b64)
	if err != nil {
		assert.NotContains(t, err.Error(), "too large")
	}
	assert.NoError(t, err)
}

func TestPreviewAdjust_OverLimitRejected(t *testing.T) {
	app := &App{}

	_, err := app.PreviewAdjust(appOverLimitBase64(), AdjustParams{}, 512)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")
	assert.Contains(t, err.Error(), "128 MB")
}

func TestAutoAdjust_OverLimitRejected(t *testing.T) {
	app := &App{}

	_, err := app.AutoAdjust(appOverLimitBase64())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")
	assert.Contains(t, err.Error(), "128 MB")
}

func TestPreviewAdjust_PixelLimitUnchanged(t *testing.T) {
	app := &App{}
	b64 := base64.StdEncoding.EncodeToString(hugePNGHeaderApp(8193, 8193))

	_, err := app.PreviewAdjust(b64, AdjustParams{}, 512)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}
