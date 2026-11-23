package main

import (
	"errors"

	"github.com/Zazza/imagecaster/internal/objectmask"
)

type ObjectMaskStatus struct {
	Ready      bool   `json:"ready"`
	EncoderURL string `json:"encoder_url"`
	DecoderURL string `json:"decoder_url"`
}

func (a *App) PrepareObjectMask() (*ObjectMaskStatus, error) {
	if a.ctx == nil || a.objectMask == nil {
		return nil, errors.New("object mask is unavailable before application startup")
	}
	if err := a.objectMask.Prepare(a.ctx); err != nil {
		return nil, err
	}
	return &ObjectMaskStatus{Ready: true, EncoderURL: objectmask.EncoderURL, DecoderURL: objectmask.DecoderURL}, nil
}
