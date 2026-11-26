package generation

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cropFixturePNG(t *testing.T, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func cropCoordinateSource(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y), B: uint8(64 + x/256*32), A: uint8(90 + (x+y)%166)})
		}
	}
	return cropFixturePNG(t, img)
}

func cropRegionsMask(t *testing.T, width, height int, regions []image.Rectangle) string {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, width, height))
	for _, region := range regions {
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return cropFixturePNG(t, img)
}

func TestGenerateFromImage_MaskedEffectCrop_ContextCoordinatesAndStrictPixels(t *testing.T) {
	const width, height = 320, 240
	tests := []struct {
		name      string
		regions   []image.Rectangle
		maskScale int
		compact   bool
	}{
		{name: "interior", regions: []image.Rectangle{image.Rect(144, 96, 176, 128)}, maskScale: 1, compact: true},
		{name: "one pixel", regions: []image.Rectangle{image.Rect(170, 130, 171, 131)}, maskScale: 1, compact: true},
		{name: "top left edge", regions: []image.Rectangle{image.Rect(0, 0, 16, 24)}, maskScale: 1, compact: true},
		{name: "bottom right edge", regions: []image.Rectangle{image.Rect(304, 216, 320, 240)}, maskScale: 1, compact: true},
		{name: "disconnected", regions: []image.Rectangle{image.Rect(112, 80, 128, 96), image.Rect(176, 144, 192, 160)}, maskScale: 1},
		{name: "different mask size", regions: []image.Rectangle{image.Rect(144, 96, 176, 128)}, maskScale: 2, compact: true},
		{name: "whole frame", regions: []image.Rectangle{image.Rect(0, 0, width, height)}, maskScale: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := emSetup(t, nil)
			e.sd.paint = func(t *testing.T, _, _ int) string {
				e.sd.mu.Lock()
				input := e.sd.img2img[len(e.sd.img2img)-1].InitImages[0]
				e.sd.mu.Unlock()
				crop := decodePNGBase64(t, input)
				painted := image.NewNRGBA(crop.Bounds())
				for y := 0; y < crop.Bounds().Dy(); y++ {
					for x := 0; x < crop.Bounds().Dx(); x++ {
						pixel := emPixel(crop, x, y)
						pixel.B, pixel.A = 255, 255
						painted.SetNRGBA(x, y, pixel)
					}
				}
				return cropFixturePNG(t, painted)
			}
			source := cropCoordinateSource(t, width, height)
			p := e.params(source)
			maskRegions := make([]image.Rectangle, len(tt.regions))
			selection := image.Rectangle{}
			for i, region := range tt.regions {
				selection = selection.Union(region)
				maskRegions[i] = image.Rect(region.Min.X/tt.maskScale, region.Min.Y/tt.maskScale, region.Max.X/tt.maskScale, region.Max.Y/tt.maskScale)
			}
			p.MaskBase64 = cropRegionsMask(t, width/tt.maskScale, height/tt.maskScale, maskRegions)
			p.MaskStrict = true
			result, err := e.svc.GenerateFromImage(p)
			require.NoError(t, err)
			require.NotNil(t, result)

			e.sd.mu.Lock()
			require.Len(t, e.sd.img2img, 1)
			req := e.sd.img2img[0]
			e.sd.mu.Unlock()
			require.Len(t, req.InitImages, 1)
			crop := decodePNGBase64(t, req.InitImages[0])
			corner := emPixel(crop, 0, 0)
			xOffset := int(corner.R) + (int(corner.B)-64)/32*256
			yOffset := int(corner.G)
			cropRect := image.Rect(xOffset, yOffset, xOffset+crop.Bounds().Dx(), yOffset+crop.Bounds().Dy())
			assert.True(t, selection.In(cropRect), "crop %v must include all selected regions %v", cropRect, selection)
			assert.True(t, cropRect.In(image.Rect(0, 0, width, height)), "crop must remain within source")
			if selection == image.Rect(0, 0, width, height) {
				assert.Equal(t, image.Rect(0, 0, width, height), cropRect)
			} else {
				assert.Less(t, cropRect.Dx()*cropRect.Dy(), width*height, "a local selection must not send the full image")
				if selection.Min.X > 0 {
					assert.Less(t, cropRect.Min.X, selection.Min.X, "include context before selection")
				}
				if selection.Max.X < width {
					assert.Greater(t, cropRect.Max.X, selection.Max.X, "include context after selection")
				}
				if selection.Min.Y > 0 {
					assert.Less(t, cropRect.Min.Y, selection.Min.Y, "include context above selection")
				}
				if selection.Max.Y < height {
					assert.Greater(t, cropRect.Max.Y, selection.Max.Y, "include context below selection")
				}
				if tt.compact {
					assert.LessOrEqual(t, cropRect.Dx()*cropRect.Dy(), width*height/4, "small selections need bounded context")
				}
			}

			original := decodePNGBase64(t, source)
			for y := 0; y < crop.Bounds().Dy(); y++ {
				for x := 0; x < crop.Bounds().Dx(); x++ {
					require.Equal(t, emPixel(original, xOffset+x, yOffset+y), emPixel(crop, x, y), "crop source %d,%d", x, y)
				}
			}
			out := decodePNGBase64(t, result.Image)
			require.Equal(t, original.Bounds(), out.Bounds())
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					selected := false
					for _, region := range tt.regions {
						selected = selected || image.Pt(x, y).In(region)
					}
					want := emPixel(original, x, y)
					if selected {
						want.B, want.A = 255, 255
					}
					require.Equal(t, want, emPixel(out, x, y), "output pixel %d,%d", x, y)
				}
			}
			e.sessions.mu.Lock()
			defer e.sessions.mu.Unlock()
			require.Len(t, e.sessions.items, 1)
			assert.Equal(t, result.Image, e.sessions.items[0].imageBase64)
		})
	}
}

func TestGenerateFromImage_MaskedEffectCrop_RejectsInvalidOutputWithoutHistory(t *testing.T) {
	for _, output := range []string{"", "not-base64", base64.StdEncoding.EncodeToString([]byte("not a PNG"))} {
		t.Run(output, func(t *testing.T) {
			e := emSetup(t, func(t *testing.T, w, h int) string { return output })
			p := e.params(makePNGBase64(t, 320, 240))
			p.MaskBase64 = emMaskBase64(t, 320, 240, image.Rect(144, 96, 176, 128))
			p.MaskStrict = true
			_, err := e.svc.GenerateFromImage(p)
			require.Error(t, err)
			e.sessions.mu.Lock()
			defer e.sessions.mu.Unlock()
			assert.Empty(t, e.sessions.items)
		})
	}
}

func TestGenerateFromImage_MaskedMaterialCrop_PreservesConstructionAndRespectsColorChoice(t *testing.T) {
	for _, keepColor := range []bool{false, true} {
		name := "color may change"
		if keepColor {
			name = "keep color"
		}
		t.Run(name, func(t *testing.T) {
			e := mkSetup(t, "post_material", func() (string, error) { return "a blue denim jacket", nil }, nil)
			p := e.params(makePNGBase64(t, 320, 240))
			p.MaskBase64 = emMaskBase64(t, 320, 240, image.Rect(144, 96, 176, 128))
			p.KeepColor = keepColor
			_, err := e.svc.GenerateFromImage(p)
			require.NoError(t, err)
			prompt := strings.ToLower(e.onlyPrompt(t))
			for _, detail := range []string{"silhouette", "cut", "seams", "fasten", "folds"} {
				assert.Contains(t, prompt, detail, "material changes must preserve construction")
			}
			if keepColor {
				assert.Contains(t, prompt, "keep its color exactly the same")
			} else {
				assert.NotContains(t, prompt, "keep its color exactly the same", "color option must control the instruction")
			}
		})
	}
}
