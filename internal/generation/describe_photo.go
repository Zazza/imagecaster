package generation

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/lanonly"
)

var ErrVisionNotLocal = errors.New("фото описывает только локальная модель ИИ: адрес ИИ в настройках не в локальной сети, фото не отправлено")

func (s *Service) visionLocal() error {
	if s.cfg == nil || !lanonly.IsLocalURL(s.cfg.LLMUrl) || lanonly.ViaProxy(s.cfg.LLMUrl) {
		return ErrVisionNotLocal
	}
	return nil
}

func imageHash(imageBase64 string) string {
	raw := imageBase64
	if i := strings.Index(raw, ","); i >= 0 && strings.HasPrefix(raw, "data:") {
		raw = raw[i+1:]
	}
	sum := sha256.New()
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil {
		sum.Write(b)
	} else {
		sum.Write([]byte(raw))
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func (s *Service) describeCacheMode(mode string) string {
	locale, _ := s.db.GetSetting("ui_locale")
	if locale != "en" {
		locale = "ru"
	}
	return mode + ":" + locale
}

func (s *Service) DescribePhoto(imageBase64, mode string, fresh bool) (string, error) {
	if mode != "quick" && mode != "describe" {
		return "", fmt.Errorf("unknown describe mode %q", mode)
	}
	if imageBase64 == "" {
		return "", errors.New("image is required")
	}
	if len(imageBase64) > MaxImageBase64Len {
		return "", config.ImageTooLarge("image")
	}
	if err := s.visionLocal(); err != nil {
		return "", err
	}
	key, cacheMode := imageHash(imageBase64), s.describeCacheMode(mode)
	if !fresh {
		text, ok, err := s.db.GetImageDescription(key, cacheMode)
		if err != nil {
			s.log.Warn("describe cache read: %v", err)
		} else if ok {
			return s.kids.FilterOutput(text), nil
		}
	}
	text, err := s.AnalyzeImage(imageBase64, mode)
	if err != nil {
		return "", err
	}
	if text != "" && !s.kids.IsActive() {
		if err := s.db.SaveImageDescription(key, cacheMode, text); err != nil {
			s.log.Warn("describe cache write: %v", err)
		}
	}
	return text, nil
}
