package mcp

import "github.com/Zazza/imagecaster/internal/generation"

const (
	shootSwapOff     = "off"
	shootSwapMissing = "skipped:подмена лица недоступна в этой сборке"
)

type StylistFaceSwapper interface {
	FaceSwap(imgB64, person string) (string, string)
}

func shootFaceSwap(eng StylistEngine, out *frameOut, person string, on bool) {
	switch {
	case !on:
		out.faceSwap = shootSwapOff
	case out.stopped:
		out.faceSwap = facePassSkipped("прервано")
	default:
		fs, ok := eng.(StylistFaceSwapper)
		if !ok {
			out.faceSwap = shootSwapMissing
			return
		}
		out.img, out.faceSwap = fs.FaceSwap(out.img, person)
		if out.faceSwap == generation.FaceSwapInterrupted {
			out.stopped = true
		}
	}
}
