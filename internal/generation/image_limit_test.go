package generation

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
	"github.com/Zazza/imagecaster/internal/sd"
)

var (
	genLargePNGOnce  sync.Once
	genLargePNG      string
	genOverLimitOnce sync.Once
	genOverLimit     string
)

func genLargeNoisePNGBase64(t *testing.T) string {
	t.Helper()
	genLargePNGOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 2400, 2200))
		rand.New(rand.NewSource(1)).Read(img.Pix)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		genLargePNG = base64.StdEncoding.EncodeToString(buf.Bytes())
	})
	require.Greater(t, len(genLargePNG), 22*1024*1024)
	require.Less(t, len(genLargePNG), config.MaxImageBase64Len)
	return genLargePNG
}

func genOverLimitBase64() string {
	genOverLimitOnce.Do(func() {
		genOverLimit = strings.Repeat("A", config.MaxImageBase64Len+1)
	})
	return genOverLimit
}

func TestDescribeForStyle_LargePNGPassesSizeCheck(t *testing.T) {
	db := openTestDB(t)
	llmSvc := &mockLLM{}
	llmSvc.describeFn = func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
		return "A woman in a red coat at dusk.", nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	text, err := svc.DescribeForStyle(genLargeNoisePNGBase64(t))
	if err != nil {
		assert.NotContains(t, err.Error(), "too large")
	}
	require.NoError(t, err)
	assert.Equal(t, "A woman in a red coat at dusk.", text)
}

func TestDescribeForStyle_OverLimitRejected(t *testing.T) {
	db := openTestDB(t)
	called := false
	llmSvc := &mockLLM{}
	llmSvc.describeFn = func(model, prompt, imageBase64 string, maxTokens int) (string, error) {
		called = true
		return "x", nil
	}
	svc := newTestService(t, db, llmSvc, &mockSD{})

	_, err := svc.DescribeForStyle(genOverLimitBase64())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")
	assert.Contains(t, err.Error(), "128 MB")
	assert.False(t, called)
}

func TestApplyLookDirect_LargePNGPassesSizeCheck(t *testing.T) {
	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string

	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "cold blue neon dusk", nil
	}
	sdSvc := recordingSD(&ops, &mu, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		return &sd.Txt2ImgResponse{Images: []string{"look-b64"}, Info: []byte(`{}`)}, nil
	})
	svc := newTestService(t, db, llmSvc, sdSvc)
	svc.sessions = &mockSessions{}

	result, err := svc.ApplyLookDirect(genLargeNoisePNGBase64(t), "neon night look", 0.5, nil)
	if err != nil {
		assert.NotContains(t, err.Error(), "too large")
	}
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "look-b64", result.Image)
}

func TestApplyLookDirect_OverLimitRejected(t *testing.T) {
	db := openTestDB(t)
	var mu sync.Mutex
	var ops []string
	img2imgCalled := false

	llmSvc := &mockLLM{}
	llmSvc.chatFn = func(model, systemPrompt, userMessage string, temperature float64, maxTokens int) (string, error) {
		return "cold blue neon dusk", nil
	}
	sdSvc := recordingSD(&ops, &mu, func(req sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		img2imgCalled = true
		return &sd.Txt2ImgResponse{Images: []string{"look-b64"}}, nil
	})
	svc := newTestService(t, db, llmSvc, sdSvc)
	svc.sessions = &mockSessions{}

	_, err := svc.ApplyLookDirect(genOverLimitBase64(), "neon night look", 0.5, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image too large")
	assert.Contains(t, err.Error(), "128 MB")
	assert.False(t, img2imgCalled)
}
