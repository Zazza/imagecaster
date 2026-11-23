package main

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/queue"
)

var (
	errStylistFrameNotLocal = errors.New("кадр стилиста: сервер не в локальной сети или запросы идут через прокси — фото не могут покидать LAN")
	errRetouchNotLocal      = errors.New("ретушь: сервер не в локальной сети или запросы идут через прокси — фото не могут покидать LAN")
)

func sdNotLocal(cfg *config.Config) bool {
	return cfg != nil && (!lanonly.IsLocalURL(cfg.SDUrl) || lanonly.ViaProxy(cfg.SDUrl))
}

func stylistFrameHandler(sessions mcp.AppSessions, cfg *config.Config, shoots stylistShootStore) queue.Handler {
	frame := mcp.NewShootFrameHandler(sessions)
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if sdNotLocal(cfg) {
			return nil, errStylistFrameNotLocal
		}
		var p mcp.ShootFrameParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, err
		}
		var res *queue.JobResult
		var err error
		if p.SourceItemID != 0 || p.EditInstruction != "" {
			items, ok := sessions.(mcp.AppSessionItems)
			if !ok || gen == nil {
				return nil, errors.New("изменить кадр: генерация или история недоступна")
			}
			var edited *mcp.ShootFrameResult
			edited, err = runStylistEdit(gen, items, p, gen.Stopped)
			if err == nil {
				var raw json.RawMessage
				raw, err = json.Marshal(edited)
				res = &queue.JobResult{Frame: raw}
			}
		} else {
			res, err = frame(gen, job)
		}
		if err == nil && res != nil && shoots != nil {
			if rerr := recordShootFrame(shoots, job.ID, res.Frame); rerr != nil {
				log.Printf("stylist: last shoot: %v", rerr)
			}
		}
		return res, err
	}
}

func retouchHandler(sessions mcp.AppSessions, cfg *config.Config) queue.Handler {
	retouch := mcp.NewRetouchHandler(sessions)
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if sdNotLocal(cfg) {
			return nil, errRetouchNotLocal
		}
		return retouch(gen, job)
	}
}
