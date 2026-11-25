package main

import (
	"errors"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
)

const stylistDetailAncestryLimit = 16
const stylistDetailShot = "Детали из исходника"

type StylistRestoreDetailsParams struct {
	JobID      int64  `json:"job_id"`
	Stage      string `json:"stage"`
	MaskBase64 string `json:"mask_base64"`
	Aligned    bool   `json:"aligned"`
}
type stylistDetailReader interface {
	StylistDetailSource(int64, string) (string, error)
}

func detailSource(reader stylistDetailReader, items stylistEditItems, itemID int64, stage string) (string, error) {
	if stage != "base" && stage != "before_face" {
		return "", errors.New("неверный этап исходника")
	}
	var lastErr error
	var sessionID int64
	for depth := 0; depth < stylistDetailAncestryLimit && itemID > 0; depth++ {
		item, err := items.SessionItem(itemID)
		if err != nil {
			return "", err
		}
		if item == nil || item.ID != itemID || item.SessionID <= 0 || (sessionID > 0 && item.SessionID != sessionID) {
			return "", errors.New("исходный кадр удалён или перемещён")
		}
		sessionID = item.SessionID
		source, err := reader.StylistDetailSource(itemID, stage)
		if err == nil {
			return source, nil
		}
		lastErr = err
		if item.Meta == nil || item.Meta.SourceItem == itemID {
			break
		}
		itemID = item.Meta.SourceItem
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("исходник кадра недоступен")
}
func (a *App) StylistDetailSource(jobID int64, stage string) (string, error) {
	if a.presets == nil || a.sessions == nil || a.gen == nil {
		return "", errors.New("история недоступна")
	}
	lastShootMu.Lock()
	defer lastShootMu.Unlock()
	for _, frame := range loadLastShoot(a.presets) {
		if frame.JobID == jobID && frame.Status == string(queue.StatusCompleted) {
			return detailSource(a.gen, a.sessions, frame.ItemID, stage)
		}
	}
	return "", errors.New("выберите готовый кадр текущей фотосессии")
}
func (a *App) StylistRestoreDetails(params StylistRestoreDetailsParams) (int64, error) {
	if !params.Aligned {
		return 0, errors.New("подтвердите совпадение положения выбранной области")
	}
	if a.gen == nil || a.sessions == nil || a.presets == nil || a.queueSvc == nil {
		return 0, errors.New("история или очередь недоступна")
	}
	return enqueueStylistEditParams(a.presets, a.queueSvc, a.sessions, sdNotLocal(a.config), StylistEditParams{JobID: params.JobID, Instruction: stylistDetailShot, RestoreStage: params.Stage, RestoreMaskBase64: params.MaskBase64, RestoreAligned: params.Aligned, validateSource: func(item *preset.SessionItem) error {
		source, err := detailSource(a.gen, a.sessions, item.ID, params.Stage)
		if err != nil {
			return err
		}
		target, err := a.sessions.GetSessionImage(item.ID)
		if err != nil {
			return err
		}
		_, err = generation.RestoreStylistDetails(target, source, params.MaskBase64)
		return err
	}})
}
func runStylistRestoreDetails(reader stylistDetailReader, sessions mcp.AppSessionItems, p mcp.ShootFrameParams, stopped func() bool) (*mcp.ShootFrameResult, error) {
	if !p.RestoreAligned {
		return nil, errors.New("не подтверждено совпадение области")
	}
	if stopped != nil && stopped() {
		return nil, generation.ErrInterrupted
	}
	item, err := sessions.SessionItem(p.SourceItemID)
	if err != nil {
		return nil, err
	}
	if item == nil || item.SessionID != p.SourceSessionID {
		return nil, errors.New("исходный кадр удалён или перемещён")
	}
	source, err := detailSource(reader, sessions, item.ID, p.RestoreStage)
	if err != nil {
		return nil, err
	}
	target, err := sessions.GetSessionImage(item.ID)
	if err != nil {
		return nil, err
	}
	result, err := generation.RestoreStylistDetails(target, source, p.RestoreMaskBase64)
	if err != nil {
		return nil, err
	}
	if stopped != nil && stopped() {
		return nil, generation.ErrInterrupted
	}
	current, err := sessions.SessionItem(item.ID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.SessionID != p.SourceSessionID {
		return nil, errors.New("исходный кадр удалён или перемещён")
	}
	id := sessions.AddToSessionID(item.SessionID, result, nil, stylistPageSource, &preset.GenerationMeta{Label: stylistDetailShot, SourceItem: item.ID})
	if id <= 0 {
		return nil, errors.New("вариант не сохранён")
	}
	return &mcp.ShootFrameResult{ID: id, Shot: stylistDetailShot, FaceSwap: generation.FaceSwapOff, FacePass: generation.FaceSwapOff}, nil
}
