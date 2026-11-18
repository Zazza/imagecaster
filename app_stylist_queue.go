package main

import (
	"errors"

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

func stylistFrameHandler(sessions mcp.AppSessions, cfg *config.Config) queue.Handler {
	frame := mcp.NewShootFrameHandler(sessions)
	return func(gen *generation.Service, job *queue.Job) (*queue.JobResult, error) {
		if sdNotLocal(cfg) {
			return nil, errStylistFrameNotLocal
		}
		return frame(gen, job)
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
