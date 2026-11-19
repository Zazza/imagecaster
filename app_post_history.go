package main

import (
	"encoding/base64"
	"errors"
)

const postHistorySource = "post"

var errPostNoSession = errors.New("не удалось сохранить в историю: нет активной папки (выберите её в «Истории») или ошибка записи — подробности в журнале")

func savePostToHistory(ex postExporter, add func(imageBase64 string) int64, p ExportImageParams) (int64, error) {
	if err := checkPostImage(p.ImageBase64); err != nil {
		return 0, err
	}
	p.Format = "png"
	p.Width, p.Height = 0, 0
	p.OutputDir = ""
	processed, err := ex.ProcessExportImage(p)
	if err != nil {
		return 0, err
	}
	id := add(base64.StdEncoding.EncodeToString(processed.Data))
	if id == 0 {
		return 0, errPostNoSession
	}
	return id, nil
}

func (a *App) SavePostToHistory(p ExportImageParams) (int64, error) {
	return savePostToHistory(a.ieSvc, func(b64 string) int64 {
		return a.sessions.AddToSession(b64, nil, postHistorySource, nil)
	}, p)
}
