package generation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
)

type faceSwapCall struct {
	img, person string
	blend       float64
}

func TestGenerateImage_FaceSwap(t *testing.T) {
	t.Parallel()
	identityMissing := &sd.APIError{Status: http.StatusConflict, Code: "identity_missing"}
	tests := []struct {
		name       string
		sdURL      string
		person     *PersonOverride
		swapRes    *sd.FaceSwapResult
		swapErr    error
		wantCalls  int
		wantImage  string
		wantStatus string
	}{
		{
			name:       "flag on, swapped image saved",
			sdURL:      "http://192.168.1.10:7861",
			person:     &PersonOverride{Name: "anna", FaceSwap: true},
			swapRes:    &sd.FaceSwapResult{Status: sd.FaceSwapDone, Image: "swapped"},
			wantCalls:  1,
			wantImage:  "swapped",
			wantStatus: FaceSwapDone,
		},
		{
			name:       "flag off, no swap",
			sdURL:      "http://192.168.1.10:7861",
			person:     &PersonOverride{Name: "anna"},
			wantCalls:  0,
			wantImage:  "original",
			wantStatus: "",
		},
		{
			name:       "no person, no swap",
			sdURL:      "http://192.168.1.10:7861",
			wantCalls:  0,
			wantImage:  "original",
			wantStatus: "",
		},
		{
			name:       "server skipped, original kept",
			sdURL:      "http://localhost:7861",
			person:     &PersonOverride{Name: "anna", FaceSwap: true},
			swapRes:    &sd.FaceSwapResult{Status: sd.FaceSwapSkipped, Skip: "no_face"},
			wantCalls:  1,
			wantImage:  "original",
			wantStatus: "skipped:лицо не найдено",
		},
		{
			name:       "server error, original kept",
			sdURL:      "http://localhost:7861",
			person:     &PersonOverride{Name: "anna", FaceSwap: true},
			swapErr:    identityMissing,
			wantCalls:  1,
			wantImage:  "original",
			wantStatus: "skipped:у человека нет лица для подмены",
		},
		{
			name:       "remote server, photo never sent",
			sdURL:      "https://sd.example.com",
			person:     &PersonOverride{Name: "anna", FaceSwap: true},
			wantCalls:  0,
			wantImage:  "original",
			wantStatus: "skipped:сервер не в локальной сети",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			p := makeTestPreset(t, db, &preset.Preset{Name: "style", Prompt: "photo", Sampler: "euler", Steps: 8, CfgScale: 1, ModelName: "z-image-turbo"})
			var calls []faceSwapCall
			sdSvc := &mockSD{
				txt2img: func(sd.Txt2ImgRequest) (*sd.Txt2ImgResponse, error) {
					return &sd.Txt2ImgResponse{Images: []string{"original"}, Info: json.RawMessage(`{}`), Parameters: json.RawMessage(`{}`)}, nil
				},
				faceSwap: func(_ context.Context, img, person string, blend float64) (*sd.FaceSwapResult, error) {
					calls = append(calls, faceSwapCall{img, person, blend})
					return tt.swapRes, tt.swapErr
				},
			}
			svc := newTestService(t, db, &mockLLM{}, sdSvc)
			svc.cfg.SDUrl = tt.sdURL
			res, err := svc.GenerateImage(GenerateImageParams{PresetID: p.ID, Overrides: &PresetOverrides{Person: tt.person}})
			require.NoError(t, err)
			require.Len(t, calls, tt.wantCalls)
			if tt.wantCalls > 0 {
				assert.Equal(t, faceSwapCall{"original", "anna", faceSwapBlend}, calls[0])
			}
			assert.Equal(t, tt.wantImage, res.Image)
			assert.Equal(t, tt.wantStatus, res.FaceSwap)
			items := svc.sessions.(*mockSessions).items
			require.Len(t, items, 1)
			assert.Equal(t, tt.wantImage, items[0].imageBase64)
		})
	}
}

func TestFaceSwapSkippedReasons(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code string
		want string
	}{
		{"no_face", "skipped:лицо не найдено"},
		{"face_small", "skipped:лицо меньше 60 px"},
		{"yaw", "skipped:голова повёрнута больше 65°"},
		{"not_local", "skipped:сервер не в локальной сети"},
		{"face_models_missing", "skipped:модели подмены не установлены"},
		{"identity_missing", "skipped:у человека нет лица для подмены"},
		{"", "skipped:ошибка подмены лица"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FaceSwapSkipped(tt.code), tt.code)
	}
}

func TestFaceSwap_NetworkErrorKeepsOriginal(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	sdSvc := &mockSD{faceSwap: func(context.Context, string, string, float64) (*sd.FaceSwapResult, error) {
		return nil, errors.New("connection refused")
	}}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	svc.cfg.SDUrl = "http://127.0.0.1:7861"
	img, status := svc.FaceSwap("original", "anna")
	assert.Equal(t, "original", img)
	assert.Equal(t, "skipped:ошибка подмены лица", status)
}

func TestPersonOverrideFaceSwapJSON(t *testing.T) {
	t.Parallel()
	var ov PresetOverrides
	require.NoError(t, json.Unmarshal([]byte(`{"person":{"name":"anna","face_swap":true}}`), &ov))
	require.NotNil(t, ov.Person)
	assert.True(t, ov.Person.FaceSwap)
	raw, err := json.Marshal(PresetOverrides{Person: &PersonOverride{Name: "anna"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"person":{"name":"anna"}}`, string(raw))
}

func TestFaceSwap_InterruptedReportsStop(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	sdSvc := &mockSD{faceSwap: func(context.Context, string, string, float64) (*sd.FaceSwapResult, error) {
		return nil, sd.ErrUserCancel
	}}
	svc := newTestService(t, db, &mockLLM{}, sdSvc)
	svc.cfg.SDUrl = "http://127.0.0.1:7861"
	img, status := svc.FaceSwap("original", "anna")
	assert.Equal(t, "original", img)
	assert.Equal(t, FaceSwapInterrupted, status)
}
