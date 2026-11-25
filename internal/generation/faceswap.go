package generation

import (
	"context"
	"math"

	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/sd"
)

const (
	FaceSwapDone        = "done"
	FaceSwapOff         = "off"
	faceSwapBlend       = 0.6
	faceRestoreBlendMin = 0.0
	faceRestoreBlendMax = 1.0
	faceSwapPrefix      = "skipped:"
)

var faceSwapReasons = map[string]string{
	"invalid_restore_blend": "сила восстановления лица должна быть от 0 до 1",
	"no_face":               "лицо не найдено",
	"face_small":            "лицо меньше 60 px",
	"yaw":                   "голова повёрнута больше 65°",
	"not_local":             "сервер не в локальной сети",
	"face_models_missing":   "модели подмены не установлены",
	"identity_missing":      "у человека нет лица для подмены",
	"person_not_found":      "человек не найден на сервере",
	"interrupted":           "прервано",
}

var FaceSwapInterrupted = FaceSwapSkipped("interrupted")

func FaceSwapSkipped(code string) string {
	if r, ok := faceSwapReasons[code]; ok {
		return faceSwapPrefix + r
	}
	return faceSwapPrefix + "ошибка подмены лица"
}

func SDIsLocal(url string) bool {
	return lanonly.IsLocalURL(url) && !lanonly.ViaProxy(url)
}

func (s *Service) sdLocal() bool {
	return s.cfg != nil && SDIsLocal(s.cfg.SDUrl)
}

func (s *Service) FaceSwap(imgB64, person string) (string, string) {
	return s.FaceSwapWithRestore(imgB64, person, faceSwapBlend)
}

func ValidFaceRestoreBlend(blend float64) bool {
	return !math.IsNaN(blend) && !math.IsInf(blend, 0) && blend >= faceRestoreBlendMin && blend <= faceRestoreBlendMax
}

func (s *Service) FaceSwapWithRestore(imgB64, person string, blend float64) (string, string) {
	if !ValidFaceRestoreBlend(blend) {
		return imgB64, FaceSwapSkipped("invalid_restore_blend")
	}
	if !s.sdLocal() {
		return imgB64, FaceSwapSkipped("not_local")
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	finish := s.startSDStage("faceswap", "", sdStageUnknown, sdStageUnknown, sdStageUnknown)
	res, err := s.sd.FaceSwap(ctx, imgB64, person, blend)
	stageErr := err
	if stageErr == nil && res != nil && res.Status == sd.FaceSwapSkipped {
		stageErr = errSDStageSkipped
	} else if stageErr == nil && (res == nil || res.Status != sd.FaceSwapDone || res.Image == "") {
		stageErr = errSDStageEmpty
	}
	finish(stageErr)
	if err != nil && (IsInterrupted(err) || ctx.Err() != nil) {
		return imgB64, FaceSwapInterrupted
	}
	if err != nil {
		s.log.Warn("face swap %q: %s", person, err)
		return imgB64, FaceSwapSkipped(sd.FaceSwapErrorCode(err))
	}
	if res.Status != sd.FaceSwapDone || res.Image == "" {
		s.log.Info("face swap %q skipped: %s (face %d px, yaw %d)", person, res.Skip, res.FacePx, res.Yaw)
		return imgB64, FaceSwapSkipped(res.Skip)
	}
	return res.Image, FaceSwapDone
}

func (s *Service) faceSwapOverride(ov *PresetOverrides, imgB64 string) (string, string) {
	if ov == nil || !validPerson(ov.Person) || !ov.Person.FaceSwap {
		return imgB64, ""
	}
	return s.FaceSwap(imgB64, ov.Person.Name)
}
