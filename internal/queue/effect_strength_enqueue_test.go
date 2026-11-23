package queue

import (
	"encoding/json"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnqueueFromImage_EffectStrengthContract(t *testing.T) {
	const (
		fullStrength        = 1.0
		legacyIdentityLimit = 0.9
		aboveFullStrength   = 1.1
		selectedLoraWeight  = 0.65
	)
	cases := []struct {
		name             string
		model            string
		strength         float64
		preserveIdentity bool
		wantError        bool
	}{
		{"flux3 full strength", "flux3-image-klein-Q4_K.gguf", fullStrength, true, false},
		{"klein full strength", "flux-2-klein-4b-Q8_0.gguf", fullStrength, true, false},
		{"qwen full strength", "qwen_image_2.1-Q8_0.gguf", fullStrength, true, false},
		{"editor above full strength", "flux3-image-klein-Q4_K.gguf", aboveFullStrength, true, true},
		{"legacy identity above limit", "", fullStrength, true, true},
		{"legacy identity at limit", "", legacyIdentityLimit, true, false},
		{"legacy nonidentity accepts clamped strength", "", aboveFullStrength, false, false},
	}
	for _, labeled := range []bool{false, true} {
		entry := "Enqueue"
		if labeled {
			entry = "EnqueueLabeled"
		}
		t.Run(entry, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					svc, emitter := newLookTestService(t)
					params := generation.GenerateFromImageParams{
						ImageBase64: editPNGBase64(t), Mode: "img2img", GenMode: "preset", PresetID: 1,
						DenoisingStrength: tc.strength, PreserveIdentity: tc.preserveIdentity,
					}
					if tc.model != "" {
						params.Editor = &generation.PostEditorParams{Model: tc.model, Loras: []preset.LoRAEntry{{Name: "SelectedDetail", Weight: selectedLoraWeight}}}
					}
					var id int64
					var err error
					if labeled {
						id, err = svc.EnqueueLabeled(JobFromImage, params, "post", "effect")
					} else {
						id, err = svc.Enqueue(JobFromImage, params, "post")
					}
					if tc.wantError {
						require.Error(t, err)
						assert.Zero(t, countJobs(t, svc))
						assert.False(t, emitter.hasEvent(EventQueueChanged))
						return
					}
					require.NoError(t, err)
					assert.Equal(t, 1, countJobs(t, svc))
					job, err := svc.store.Get(id)
					require.NoError(t, err)
					assert.Equal(t, JobFromImage, job.Type)
					assert.Equal(t, StatusPending, job.Status)
					var restored generation.GenerateFromImageParams
					require.NoError(t, json.Unmarshal([]byte(job.Params), &restored))
					assert.Equal(t, params.Editor, restored.Editor)
					assert.Equal(t, params.DenoisingStrength, restored.DenoisingStrength)
					assert.True(t, emitter.hasEvent(EventQueueChanged))
				})
			}
		})
	}
}
