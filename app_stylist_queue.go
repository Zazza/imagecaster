package main

import (
	"errors"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/queue"
)

var errStylistFrameNotLocal = errors.New("кадр стилиста: сервер не в локальной сети или запросы идут через прокси — фото не могут покидать LAN")

func stylistFrameHandler(sessions mcp.AppSessions, cfg *config.Config) queue.Handler {
	frame := mcp.NewShootFrameHandler(sessions)
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if cfg != nil && (!lanonly.IsLocalURL(cfg.SDUrl) || lanonly.ViaProxy(cfg.SDUrl)) {
			return nil, errStylistFrameNotLocal
		}
		return frame(gen, job)
	}
}
