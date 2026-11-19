package postpreview

import (
	"fmt"
	"image"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/bodyshape"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/finish"
	"github.com/Zazza/imagecaster/internal/imgcodec"
)

func (s *Service) PostRetouch(imageBase64 string, p adjust.Params, r faceguard.Retouch, pts []faceguard.Point, body bodyshape.Params, bodyPts []bodyshape.Point, f finish.Params, maxW int) (string, error) {
	img, err := s.scaled(imageBase64, maxW)
	if err != nil {
		return "", err
	}
	out := adjust.Apply(img, p)
	if eff := r.ForPoints(len(pts)); !eff.IsZero() {
		if err := r.Validate(); err != nil {
			return "", fmt.Errorf("retouch: %w", err)
		}
		rt, err := faceguard.ApplyRetouch(out, pts, eff)
		if err != nil {
			return "", fmt.Errorf("retouch: %w", err)
		}
		out = rt
	}
	if !body.IsZero() {
		if err := body.Validate(); err != nil {
			return "", fmt.Errorf("body: %w", err)
		}
		bd, err := bodyshape.ApplyIfFound(out, bodyPts, body)
		if err != nil {
			return "", fmt.Errorf("body: %w", err)
		}
		return encodeFinished(bd, f)
	}
	return encodeFinished(out, f)
}

func encodeFinished(img image.Image, f finish.Params) (string, error) {
	res, err := finish.Apply(img, f)
	if err != nil {
		return "", err
	}
	return imgcodec.EncodeJPEG(res, jpegQuality)
}
