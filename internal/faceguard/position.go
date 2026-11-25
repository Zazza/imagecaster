package faceguard

import (
	"errors"
	"fmt"
	"math"
)

func MeasurePosition(origPts, editPts []Point, w, h int) (Fit, error) {
	if len(origPts) < minPoints || len(editPts) < minPoints {
		return Fit{}, fmt.Errorf("need %d face points", minPoints)
	}
	if w <= 0 || h <= 0 {
		return Fit{}, errors.New("invalid image dimensions")
	}
	n := min(len(origPts), len(editPts))
	original, edited := toPixels(origPts[:n], w, h), toPixels(editPts[:n], w, h)
	for i := range original {
		for _, p := range []fpt{original[i], edited[i]} {
			if math.IsNaN(p.x) || math.IsNaN(p.y) || math.IsInf(p.x, 0) || math.IsInf(p.y, 0) {
				return Fit{}, errors.New("invalid face coordinates")
			}
		}
	}
	faceW := faceWidth(original)
	if faceW < 1 || faceWidth(edited) < 1 {
		return Fit{}, errors.New("face too small")
	}
	residuals := make([]float64, n)
	for i := range original {
		residuals[i] = math.Hypot(original[i].x-edited[i].x, original[i].y-edited[i].y)
	}
	return measure(residuals, faceW), nil
}
