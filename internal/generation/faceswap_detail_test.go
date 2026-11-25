package generation

import (
	"context"
	"math"
	"testing"

	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFaceSwapWithRestoreDetailContract(t *testing.T) {
	for _, blend := range []float64{0, 0.3, 0.6, 1} {
		t.Run(string(rune('a'+int(blend*10))), func(t *testing.T) {
			calls := 0
			svc := newTestService(t, openTestDB(t), &mockLLM{}, &mockSD{faceSwap: func(_ context.Context, img, person string, got float64) (*sd.FaceSwapResult, error) {
				calls++
				assert.Equal(t, "original", img)
				assert.Equal(t, "anna", person)
				assert.Equal(t, blend, got)
				return &sd.FaceSwapResult{Status: sd.FaceSwapDone, Image: "restored"}, nil
			}})
			svc.cfg.SDUrl = "http://127.0.0.1:7861"
			img, status := svc.FaceSwapWithRestore("original", "anna", blend)
			require.Equal(t, 1, calls)
			assert.Equal(t, "restored", img)
			assert.Equal(t, FaceSwapDone, status)
		})
	}
}

func TestFaceSwapWithRestoreDetailRejectsInvalidBlend(t *testing.T) {
	for _, blend := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		svc := newTestService(t, openTestDB(t), &mockLLM{}, &mockSD{faceSwap: func(context.Context, string, string, float64) (*sd.FaceSwapResult, error) {
			t.Fatal("invalid restore strength must not reach SD")
			return nil, nil
		}})
		svc.cfg.SDUrl = "http://127.0.0.1:7861"
		img, status := svc.FaceSwapWithRestore("original", "anna", blend)
		assert.Equal(t, "original", img)
		assert.Contains(t, status, "skipped:")
	}
}

func TestFaceSwapWithRestoreDetailRemoteServerKeepsOriginal(t *testing.T) {
	svc := newTestService(t, openTestDB(t), &mockLLM{}, &mockSD{faceSwap: func(context.Context, string, string, float64) (*sd.FaceSwapResult, error) {
		t.Fatal("remote server must not receive frame")
		return nil, nil
	}})
	svc.cfg.SDUrl = "https://sd.example.com"
	img, status := svc.FaceSwapWithRestore("original", "anna", 0.3)
	assert.Equal(t, "original", img)
	assert.Equal(t, FaceSwapSkipped("not_local"), status)
}

func TestFaceSwapDetailLegacyRestoreStrength(t *testing.T) {
	calls := 0
	svc := newTestService(t, openTestDB(t), &mockLLM{}, &mockSD{faceSwap: func(_ context.Context, _, _ string, blend float64) (*sd.FaceSwapResult, error) {
		calls++
		assert.Equal(t, 0.6, blend)
		return &sd.FaceSwapResult{Status: sd.FaceSwapDone, Image: "restored"}, nil
	}})
	svc.cfg.SDUrl = "http://127.0.0.1:7861"
	img, status := svc.FaceSwap("original", "anna")
	assert.Equal(t, "restored", img)
	assert.Equal(t, FaceSwapDone, status)
	assert.Equal(t, 1, calls)
}
