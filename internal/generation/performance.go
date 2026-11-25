package generation

import (
	"errors"
	"path"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/sd"
)

const sdStageUnknown = 0

var errSDStageEmpty = errors.New("SD stage returned no image")
var errSDStageSkipped = errors.New("SD stage skipped")

func (s *Service) startSDStage(stage, model string, width, height, steps int) func(error) {
	if s.log == nil || !s.diagnosticsConfig().VerboseLogs {
		return func(error) {}
	}
	if model != "" {
		model = path.Base(strings.ReplaceAll(model, `\`, "/"))
	}
	start := time.Now()
	s.log.Info("generation_sd stage=%s status=start model=%q width=%d height=%d steps=%d elapsed_ms=0", stage, model, width, height, steps)
	return func(err error) {
		outcome := "ok"
		if errors.Is(err, errSDStageSkipped) {
			outcome = "skipped"
		} else if err != nil {
			outcome = "error"
		}
		s.log.Info("generation_sd stage=%s status=end model=%q width=%d height=%d steps=%d elapsed_ms=%d outcome=%s", stage, model, width, height, steps, time.Since(start).Milliseconds(), outcome)
	}
}

func finishSDImageStage(finish func(error), result *sd.Txt2ImgResponse, err error) {
	if err == nil && (result == nil || len(result.Images) == 0 || result.Images[0] == "") {
		err = errSDStageEmpty
	}
	finish(err)
}

func (s *Service) measuredTxt2Img(req sd.Txt2ImgRequest) (result *sd.Txt2ImgResponse, err error) {
	finish := s.startSDStage("txt2img", req.Model.Checkpoint, req.Width, req.Height, req.Steps)
	defer func() { finishSDImageStage(finish, result, err) }()
	return s.sd.Txt2Img(req)
}

func (s *Service) measuredImg2Img(req sd.Img2ImgRequest) (result *sd.Txt2ImgResponse, err error) {
	finish := s.startSDStage("img2img", req.Model.Checkpoint, req.Width, req.Height, req.Steps)
	defer func() { finishSDImageStage(finish, result, err) }()
	return s.sd.Img2Img(req)
}

func (s *Service) measuredUpscale(img, upscaler string, scale float64) (result string, err error) {
	finish := s.startSDStage("upscale", "", sdStageUnknown, sdStageUnknown, sdStageUnknown)
	defer func() {
		stageErr := err
		if stageErr == nil && result == "" {
			stageErr = errSDStageEmpty
		}
		finish(stageErr)
	}()
	return s.sd.UpscaleImage(img, upscaler, scale)
}
