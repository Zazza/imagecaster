package importexport

import (
	"fmt"
	"image"

	"github.com/Zazza/imagecaster/internal/bodyshape"
	"github.com/Zazza/imagecaster/internal/faceguard"
)

func applyRetouch(img image.Image, r *faceguard.Retouch, pts []faceguard.Point) (image.Image, error) {
	if r == nil || r.IsZero() {
		return img, nil
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("retouch: %w", err)
	}
	eff := r.ForPoints(len(pts))
	if eff.IsZero() {
		return img, nil
	}
	out, err := faceguard.ApplyRetouch(img, pts, eff)
	if err != nil {
		return nil, fmt.Errorf("retouch: %w", err)
	}
	return out, nil
}

func applyBody(img image.Image, p *bodyshape.Params, pts []bodyshape.Point) (image.Image, error) {
	if p == nil || p.IsZero() {
		return img, nil
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("body: %w", err)
	}
	out, err := bodyshape.ApplyIfFound(img, pts, *p)
	if err != nil {
		return nil, fmt.Errorf("body: %w", err)
	}
	return out, nil
}
