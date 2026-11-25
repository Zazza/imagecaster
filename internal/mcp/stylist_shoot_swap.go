package mcp

import (
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/stylisttrace"
)

const (
	shootSwapOff     = "off"
	shootSwapMissing = "skipped:подмена лица недоступна в этой сборке"
)

type StylistFaceSwapper interface {
	FaceSwap(imgB64, person string) (string, string)
}

type StylistFaceRestoreSwapper interface {
	FaceSwapWithRestore(imgB64, person string, blend float64) (string, string)
}

func shootFaceSwap(eng StylistEngine, out *frameOut, person string, on bool, blends ...*float64) {
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
		if len(blends) > 0 && blends[0] != nil {
			blend := *blends[0]
			if !generation.ValidFaceRestoreBlend(blend) {
				out.faceSwap = generation.FaceSwapSkipped("invalid_restore_blend")
				return
			}
			restored, ok := eng.(StylistFaceRestoreSwapper)
			if !ok {
				out.faceSwap = "skipped:настройка восстановления лица недоступна в этой сборке"
				return
			}
			out.img, out.faceSwap = restored.FaceSwapWithRestore(out.img, person, blend)
		} else {
			out.img, out.faceSwap = fs.FaceSwap(out.img, person)
		}
		if out.faceSwap == generation.FaceSwapDone {
			traceError(out.trace.Save(stylisttrace.FaceSwap, out.img))
		}
		if out.faceSwap == generation.FaceSwapInterrupted {
			out.stopped = true
		}
	}
}
