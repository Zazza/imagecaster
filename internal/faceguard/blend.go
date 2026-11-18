package faceguard

import (
	"errors"
	"image"
	"image/draw"
	"image/png"
	"math"

	"github.com/Zazza/imagecaster/internal/imgcodec"
)

const blendEllipseSegments = 64

func ellipseMask(w, h int) *plane {
	cx := (float64(w) - 1) / 2
	cy := (float64(h) - 1) / 2
	rx := math.Max(1, cx)
	ry := math.Max(1, cy)
	poly := make([]fpt, 0, blendEllipseSegments)
	for i := 0; i < blendEllipseSegments; i++ {
		t := 2 * math.Pi * float64(i) / blendEllipseSegments
		poly = append(poly, fpt{x: cx + rx*math.Cos(t), y: cy + ry*math.Sin(t)})
	}
	return fill(w, h, poly)
}

func BlendRegion(orig, region image.Image, rect image.Rectangle, feather float64) (*image.RGBA, error) {
	ob := orig.Bounds()
	if ob.Dx() <= 0 || ob.Dy() <= 0 {
		return nil, errors.New("empty original image")
	}
	rect = rect.Intersect(ob).Sub(ob.Min)
	w, h := rect.Dx(), rect.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("blend rect is empty or outside the image")
	}
	scaled := toRGBA(region, w, h)
	out := image.NewRGBA(image.Rect(0, 0, ob.Dx(), ob.Dy()))
	draw.Draw(out, out.Bounds(), orig, ob.Min, draw.Src)
	mask := ellipseMask(w, h)
	if feather > 0 {
		mask = mask.blur(feather * math.Min(float64(w), float64(h)))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := mask.v[y*w+x]
			if a <= 0 {
				continue
			}
			oi := out.PixOffset(rect.Min.X+x, rect.Min.Y+y)
			ri := scaled.PixOffset(x, y)
			for k := 0; k < 3; k++ {
				v := float64(scaled.Pix[ri+k])*a + float64(out.Pix[oi+k])*(1-a)
				out.Pix[oi+k] = uint8(math.Round(math.Max(0, math.Min(255, v))))
			}
			out.Pix[oi+3] = 255
		}
	}
	return out, nil
}

func BlendRegionBase64(origB64, regionB64 string, rect image.Rectangle, feather float64) (string, error) {
	o, err := imgcodec.Decode(origB64, imgcodec.Options{Name: "original", DataURL: true, MaxDim: 8192})
	if err != nil {
		return "", err
	}
	r, err := imgcodec.Decode(regionB64, imgcodec.Options{Name: "region", DataURL: true, MaxDim: 8192})
	if err != nil {
		return "", err
	}
	res, err := BlendRegion(o, r, rect, feather)
	if err != nil {
		return "", err
	}
	return imgcodec.EncodePNG(res, png.DefaultCompression)
}
