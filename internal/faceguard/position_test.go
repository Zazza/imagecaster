package faceguard

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasurePosition_UsesCropCoordinatesAndOriginalWidth(t *testing.T) {
	const cropWidth, cropHeight = 400, 600
	const shiftX, shiftY = 0.03, 0.04
	const tolerance = 1e-12
	original := fgFace()
	same, err := MeasurePosition(original, original, cropWidth, cropHeight)
	require.NoError(t, err)
	assert.Equal(t, Fit{}, same)
	moved := append([]Point(nil), original...)
	for i := range moved {
		moved[i].X += shiftX
		moved[i].Y += shiftY
	}
	fit, err := MeasurePosition(original, moved, cropWidth, cropHeight)
	require.NoError(t, err)
	originalWidth := math.Hypot((original[16].X-original[0].X)*cropWidth, (original[16].Y-original[0].Y)*cropHeight)
	want := math.Hypot(shiftX*cropWidth, shiftY*cropHeight) / originalWidth
	assert.InDelta(t, want, fit.Mean, tolerance)
	assert.InDelta(t, want, fit.Key, tolerance)
	const enlargedFaceScale = 1.3
	enlarged := append([]Point(nil), original...)
	var meanDistance, keyDistance float64
	const eyeCornerIndex = 36
	for i, point := range original {
		enlarged[i] = Point{X: 0.5 + enlargedFaceScale*(point.X-0.5), Y: 0.5 + enlargedFaceScale*(point.Y-0.5)}
		distance := math.Hypot((enlarged[i].X-point.X)*cropWidth, (enlarged[i].Y-point.Y)*cropHeight)
		meanDistance += distance
	}
	for _, index := range []int{36, 39, 42, 45, 30, 48, 54} {
		keyDistance = math.Max(keyDistance, math.Hypot((enlarged[index].X-original[index].X)*cropWidth, (enlarged[index].Y-original[index].Y)*cropHeight))
	}
	fit, err = MeasurePosition(original, enlarged, cropWidth, cropHeight)
	require.NoError(t, err)
	assert.InDelta(t, meanDistance/float64(len(original))/originalWidth, fit.Mean, tolerance)
	assert.InDelta(t, keyDistance/originalWidth, fit.Key, tolerance)
	// Moving a single eye corner must remain visible in the key-point maximum.
	keyMoved := append([]Point(nil), original...)
	keyMoved[eyeCornerIndex].X += shiftX
	fit, err = MeasurePosition(original, keyMoved, cropWidth, cropHeight)
	require.NoError(t, err)
	assert.InDelta(t, shiftX*cropWidth/originalWidth, fit.Key, tolerance)
	assert.InDelta(t, fit.Key/float64(len(original)), fit.Mean, tolerance)
}

func TestMeasurePosition_RejectsInvalidGeometry(t *testing.T) {
	const cropWidth, cropHeight = 400, 600
	cases := []struct {
		name             string
		original, edited []Point
		w, h             int
	}{
		{"missing original", nil, fgFace(), cropWidth, cropHeight},
		{"insufficient edited", fgFace(), fgFace()[:10], cropWidth, cropHeight},
		{"zero width", fgFace(), fgFace(), 0, cropHeight},
		{"negative height", fgFace(), fgFace(), cropWidth, -1},
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, originalSide := range []bool{true, false} {
			original, edited := fgFace(), fgFace()
			if originalSide {
				original[30].X = bad
			} else {
				edited[30].Y = bad
			}
			cases = append(cases, struct {
				name             string
				original, edited []Point
				w, h             int
			}{"nonfinite", original, edited, cropWidth, cropHeight})
		}
	}
	original, edited := fgFace(), fgFace()
	original[16] = original[0]
	cases = append(cases, struct {
		name             string
		original, edited []Point
		w, h             int
	}{"degenerate original width", original, edited, cropWidth, cropHeight})
	collapsed := make([]Point, len(edited))
	cases = append(cases, struct {
		name             string
		original, edited []Point
		w, h             int
	}{"collapsed edited face", fgFace(), collapsed, cropWidth, cropHeight})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := MeasurePosition(tc.original, tc.edited, tc.w, tc.h)
			assert.Error(t, err)
		})
	}
}
