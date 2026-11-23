package queue

import (
	"encoding/json"
	"testing"

	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/stretchr/testify/require"
)

func TestEnqueueStrictMaskValidationAndPersistence(t *testing.T) {
	for _, labeled := range []bool{false, true} {
		for _, withMask := range []bool{false, true} {
			name := "plain"
			if labeled {
				name = "labeled"
			}
			if withMask {
				name += "/with mask"
			} else {
				name += "/missing mask"
			}
			t.Run(name, func(t *testing.T) {
				svc, emitter := newLookTestService(t)
				params := generation.GenerateFromImageParams{ImageBase64: editPNGBase64(t), Mode: "img2img", GenMode: "preset", PresetID: 1, DenoisingStrength: 0.6, MaskStrict: true}
				if withMask {
					params.MaskBase64 = editPNGBase64(t)
				}
				var id int64
				var err error
				if labeled {
					id, err = svc.EnqueueLabeled(JobFromImage, params, "post", "effect")
				} else {
					id, err = svc.Enqueue(JobFromImage, params, "post")
				}
				if !withMask {
					require.ErrorContains(t, err, "strict mask requires mask")
					require.Zero(t, countJobs(t, svc))
					require.False(t, emitter.hasEvent(EventQueueChanged))
					return
				}
				require.NoError(t, err)
				job, err := svc.store.Get(id)
				require.NoError(t, err)
				var restored generation.GenerateFromImageParams
				require.NoError(t, json.Unmarshal([]byte(job.Params), &restored))
				require.True(t, restored.MaskStrict)
				require.Equal(t, params.MaskBase64, restored.MaskBase64)
			})
		}
	}
}
