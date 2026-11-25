package generation

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/maskcomp"
	"github.com/Zazza/imagecaster/internal/stylisttrace"
)

const detailMaxSide = 8192
const detailMaskThreshold = 127
const detailImageByteLimit = 16 << 20

func (s *Service) StylistDetailSource(itemID int64, stage string) (string, error) {
	if s.dataDir == "" {
		return "", errors.New("исходник кадра недоступен")
	}
	return stylisttrace.ReadFrame(filepath.Join(s.dataDir, stylisttrace.Directory), itemID, stage)
}

func RestoreStylistDetails(target, source, mask string) (string, error) {
	images := make([]image.Image, 0, 3)
	for _, encoded := range []string{target, source, mask} {
		if len(encoded) > MaxImageBase64Len {
			return "", config.ImageTooLarge("detail image")
		}
		if len(encoded) > detailImageByteLimit*2 {
			return "", errors.New("изображение превышает допустимый размер")
		}
		img, err := imgcodec.Decode(encoded, imgcodec.Options{DataURL: true, MaxDim: detailMaxSide})
		if err != nil {
			return "", err
		}
		images = append(images, img)
	}
	if images[0].Bounds().Size() != images[1].Bounds().Size() || images[0].Bounds().Size() != images[2].Bounds().Size() {
		return "", errors.New("размеры исходника, результата и маски должны совпадать")
	}
	selected := false
	for y := images[2].Bounds().Min.Y; y < images[2].Bounds().Max.Y; y++ {
		for x := images[2].Bounds().Min.X; x < images[2].Bounds().Max.X; x++ {
			p := color.NRGBAModel.Convert(images[2].At(x, y)).(color.NRGBA)
			if p.A > detailMaskThreshold && (int(p.R)+int(p.G)+int(p.B))/3 > detailMaskThreshold {
				selected = true
				break
			}
		}
		if selected {
			break
		}
	}
	if !selected {
		return "", errors.New("выделите область для восстановления")
	}
	return imgcodec.EncodePNG(maskcomp.BlendStrict(images[0], images[1], images[2]), png.DefaultCompression)
}
