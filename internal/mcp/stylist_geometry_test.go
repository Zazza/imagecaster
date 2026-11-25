package mcp

import (
	"encoding/base64"
	"image/color"
	"math"
	"testing"

	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefineShootFace_PreservesCropGeometry(t *testing.T) {
	const frameWidth, frameHeight = 240, 360
	const displacedCropFraction = 0.1
	const enlargedFaceScale = 1.3
	const rotatedFaceRadians = 20 * math.Pi / 180
	const minorJitter = 0.001
	frame := base64.StdEncoding.EncodeToString(shootFacePNG(frameWidth, frameHeight, color.RGBA{R: 90, A: 255}))
	original := styFace()
	rect, ok := faceCropBox(original, frameWidth, frameHeight, shootFaceMargin)
	require.True(t, ok)
	cropPoints := remapPtsToCrop(original, rect, frameWidth, frameHeight)
	for _, tc := range []struct {
		name                string
		shift, scale, angle float64
		accept              bool
	}{
		{name: "identical", scale: 1, accept: true},
		{name: "minor jitter", shift: minorJitter, scale: 1, accept: true},
		{name: "translated", shift: displacedCropFraction, scale: 1},
		{name: "enlarged", scale: enlargedFaceScale},
		{name: "rotated", scale: 1, angle: rotatedFaceRadians},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refiner := &fakeRefiner{}
			eng := &stubFaceEng{fn: func(input string) ([]faceguard.Point, error) {
				if input == frame {
					return original, nil
				}
				points := make([]faceguard.Point, len(cropPoints))
				// Rotate in pixel coordinates: a rectangular crop has different X/Y scales.
				for i, p := range cropPoints {
					x, y := (p.X-0.5)*float64(rect.Dx()), (p.Y-0.5)*float64(rect.Dy())
					points[i] = faceguard.Point{X: 0.5 + tc.scale*(x*math.Cos(tc.angle)-y*math.Sin(tc.angle))/float64(rect.Dx()) + tc.shift, Y: 0.5 + tc.scale*(x*math.Sin(tc.angle)+y*math.Cos(tc.angle))/float64(rect.Dy())}
				}
				return points, nil
			}}
			out, status, interrupted := refineShootFace(eng, refiner, frame, "prompt", "negative", shootModels["Z-Image"], nil)
			require.Len(t, refiner.refineCalls(), 1)
			assert.False(t, interrupted)
			if tc.accept {
				assert.Equal(t, shootFaceDone, status)
				assert.True(t, frame != out, "accepted refinement must change the frame")
			} else {
				assert.Contains(t, status, "skipped:")
				assert.True(t, frame == out, "rejected refinement must return the exact original frame")
			}
		})
	}
}
