package postpreview

import (
	"fmt"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/faceguard"
	"github.com/Zazza/imagecaster/internal/finish"
	"github.com/Zazza/imagecaster/internal/imgcodec"
)

func (s *Service) PostRetouch(imageBase64 string, p adjust.Params, r faceguard.Retouch, pts []faceguard.Point, f finish.Params, maxW int) (string, error) {
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
	res, err := finish.Apply(out, f)
	if err != nil {
		return "", err
	}
	return imgcodec.EncodeJPEG(res, jpegQuality)
}
