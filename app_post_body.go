package main

import (
	"errors"

	"github.com/Zazza/imagecaster/internal/bodyshape"
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/sd"
)

const postBodyNone = "no_body"

type PostBody struct {
	Points []bodyshape.Point `json:"points"`
	Skip   string            `json:"skip,omitempty"`
	Error  string            `json:"error,omitempty"`
}

type bodyDetector interface {
	BodyKeypoints(imageBase64 string) ([]bodyshape.Point, error)
}

func postBodyPoints(det bodyDetector, cfg *config.Config, imageBase64 string) (PostBody, error) {
	if err := checkPostImage(imageBase64); err != nil {
		return PostBody{}, err
	}
	none := []bodyshape.Point{}
	if sdNotLocal(cfg) {
		return PostBody{Points: none, Skip: postFaceNotLocal}, nil
	}
	if det == nil {
		return PostBody{Points: none, Skip: postFaceUnavailable}, nil
	}
	pts, err := det.BodyKeypoints(imageBase64)
	switch {
	case errors.Is(err, sd.ErrNoBody):
		return PostBody{Points: none, Skip: postBodyNone}, nil
	case err != nil:
		return PostBody{Points: none, Skip: postFaceUnavailable, Error: err.Error()}, nil
	}
	if pts == nil {
		pts = none
	}
	return PostBody{Points: pts}, nil
}

func (a *App) PostBodyPoints(imageBase64 string) (PostBody, error) {
	body, err := postBodyPoints(a.gen, a.config, imageBase64)
	if body.Error != "" {
		a.log.Warn("post: body keypoints: %s", body.Error)
	}
	return body, err
}
