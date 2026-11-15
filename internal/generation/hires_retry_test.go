package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hiresTestService(t *testing.T, firstErr error) (*Service, *atomic.Int32, int64) {
	t.Helper()
	db := openTestDB(t)
	makeTestPreset(t, db, nil)
	hp := &preset.HiresProfile{Name: "hr", Upscale: 2.0, DenoisingStrength: 0.5, Upscaler: "Latent"}
	require.NoError(t, db.CreateHiresProfile(hp))
	var calls atomic.Int32
	sdSvc := &mockSD{
		txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			if calls.Add(1) == 1 {
				return nil, firstErr
			}
			return &sd.Txt2ImgResponse{Images: []string{"retry-img"}, Info: json.RawMessage(`{}`)}, nil
		},
	}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	svc.ctx = context.Background()
	return svc, &calls, hp.ID
}

func TestHiresFallbackRetryDecision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		cancelCtx bool
		wantCalls int32
		wantErr   bool
	}{
		{name: "sd_failed job error retries", err: fmt.Errorf("sd job failed: %w", &sd.JobError{Code: "sd_failed", Message: "CUDA out of memory"}), wantCalls: 2},
		{name: "unknown error retries", err: errors.New("connection reset"), wantCalls: 2},
		{name: "model not found does not retry", err: fmt.Errorf("sd job failed: %w", &sd.JobError{Code: "model_not_found", Message: "x"}), wantCalls: 1, wantErr: true},
		{name: "invalid params api error does not retry", err: fmt.Errorf("submit sd job: %w", &sd.APIError{Status: 400, Code: "invalid_params"}), wantCalls: 1, wantErr: true},
		{name: "any 4xx does not retry", err: &sd.APIError{Status: 422}, wantCalls: 1, wantErr: true},
		{name: "queue full does not retry", err: &sd.APIError{Status: 503, Code: "queue_full"}, wantCalls: 1, wantErr: true},
		{name: "user cancel does not retry", err: fmt.Errorf("%w: %w", context.Canceled, sd.ErrUserCancel), wantCalls: 1, wantErr: true},
		{name: "job cancelled does not retry", err: sd.ErrJobCancelled, wantCalls: 1, wantErr: true},
		{name: "queue job context cancelled does not retry", err: fmt.Errorf("sd job failed: %w", &sd.JobError{Code: "sd_failed"}), cancelCtx: true, wantCalls: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, calls, hiresID := hiresTestService(t, tt.err)
			if tt.cancelCtx {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				svc.ctx = ctx
			}
			res, err := svc.GenerateImage(GenerateImageParams{PresetID: 1, HiresProfileID: &hiresID})
			assert.Equal(t, tt.wantCalls, calls.Load())
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.True(t, res.HiresFixSkipped || res.HiresFixManual)
		})
	}
}

func TestHiresFallbackNotUsedWithoutHires(t *testing.T) {
	t.Parallel()
	svc, calls, _ := hiresTestService(t, errors.New("boom"))
	_, err := svc.GenerateImage(GenerateImageParams{PresetID: 1})
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func fromImageParams(t *testing.T, mode string, hiresID *int64) GenerateFromImageParams {
	return GenerateFromImageParams{
		ImageBase64:    makePNGBase64(t, 64, 64),
		Mode:           mode,
		GenMode:        "preset",
		PresetID:       1,
		HiresProfileID: hiresID,
	}
}

func TestGenerateFromImageTxt2ImgSharesGenerateBehavior(t *testing.T) {
	t.Parallel()
	t.Run("hires fallback", func(t *testing.T) {
		t.Parallel()
		svc, calls, hiresID := hiresTestService(t, fmt.Errorf("sd job failed: %w", &sd.JobError{Code: "sd_failed", Message: "oom"}))
		res, err := svc.GenerateFromImage(fromImageParams(t, "txt2img", &hiresID))
		require.NoError(t, err)
		assert.Equal(t, int32(2), calls.Load())
		assert.True(t, res.HiresFixSkipped || res.HiresFixManual)
		assert.Equal(t, int64(1), res.SessionItemID)
	})
	t.Run("empty response reason", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		makeTestPreset(t, db, nil)
		sdSvc := &mockSD{txt2img: func(sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			return &sd.Txt2ImgResponse{Info: json.RawMessage(`{"reason":"nsfw filtered"}`)}, nil
		}}
		svc := newTestService(t, db, &mockLLM{}, sdSvc)
		_, err := svc.GenerateFromImage(fromImageParams(t, "txt2img", nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no image generated: nsfw filtered")
	})
	t.Run("hires default denoise", func(t *testing.T) {
		t.Parallel()
		db := openTestDB(t)
		makeTestPreset(t, db, nil)
		hp := &preset.HiresProfile{Name: "hr", Upscale: 2.0, DenoisingStrength: 0.35, Upscaler: "Latent"}
		require.NoError(t, db.CreateHiresProfile(hp))
		var got sd.Txt2ImgRequest
		sdSvc := &mockSD{txt2img: func(req sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
			got = req
			return &sd.Txt2ImgResponse{Images: []string{"img"}}, nil
		}}
		svc := newTestService(t, db, &mockLLM{}, sdSvc)
		_, err := svc.GenerateFromImage(fromImageParams(t, "txt2img", &hp.ID))
		require.NoError(t, err)
		require.NotNil(t, got.DenoisingStrength)
		assert.Equal(t, 0.35, *got.DenoisingStrength)
		require.NotNil(t, got.BatchSize)
		assert.Equal(t, 1, *got.BatchSize)
	})
}

func TestGenerateFromImageImg2ImgSetsSessionItemID(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	makeTestPreset(t, db, nil)
	sdSvc := &mockSD{img2img: func(sd.Img2ImgRequest) (*sd.Txt2ImgResponse, error) {
		return &sd.Txt2ImgResponse{Images: []string{"img2img-result"}, Info: json.RawMessage(`{}`)}, nil
	}}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	sessions := &mockSessions{items: make([]sessionItem, 4)}
	svc.sessions = sessions
	res, err := svc.GenerateFromImage(fromImageParams(t, "img2img", nil))
	require.NoError(t, err)
	assert.Equal(t, int64(5), res.SessionItemID)
	assert.Equal(t, "from-image", sessions.items[4].source)
}
