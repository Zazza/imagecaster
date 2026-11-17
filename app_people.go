package main

import (
	"context"
	"errors"
	"time"

	"github.com/Zazza/imagecaster/internal/people"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const peopleCallTimeout = 30 * time.Second

func (a *App) peopleSvc() *people.Service {
	return people.New(a.serverClient, a.serverClient.GetBaseURL, func() string {
		id, err := a.presets.InstallID()
		if err != nil {
			a.log.Warn("people: install id: %s", err)
		}
		return id
	})
}

func (a *App) peopleCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	base := a.ctx
	if base == nil {
		base = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(base)
	}
	return context.WithTimeout(base, timeout)
}

func (a *App) TrainingStatus() string {
	if a.serverClient.GetBaseURL() == "" {
		return "no_server"
	}
	info, err := serverclient.Probe(a.serverClient.GetBaseURL())
	if err != nil {
		return "unreachable"
	}
	for _, f := range info.Features {
		if f == "lora_train" {
			return "ok"
		}
	}
	return "not_installed"
}

func (a *App) PickPeopleFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Папка с фото человека"})
}

func (a *App) TrainPerson(folder, name, gender string) (*serverclient.TrainJob, error) {
	if a.serverClient.GetBaseURL() == "" {
		return nil, errors.New("сервер Imagecaster не настроен")
	}
	ctx, cancel := a.peopleCtx(0)
	defer cancel()
	job, err := a.peopleSvc().Train(ctx, folder, name, gender)
	if err != nil {
		a.log.Warn("people: train %q: %s", name, err)
	}
	return job, err
}

func (a *App) ListTrainingJobs() ([]serverclient.TrainJob, error) {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return a.peopleSvc().Jobs(ctx)
}

func (a *App) CancelTraining(id string) (*serverclient.TrainJob, error) {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return a.peopleSvc().Cancel(ctx, id)
}

func (a *App) ListPeople() ([]serverclient.Person, error) {
	if a.serverClient.GetBaseURL() == "" {
		return []serverclient.Person{}, nil
	}
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return a.peopleSvc().List(ctx)
}

func (a *App) DeletePerson(name string) error {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return a.peopleSvc().Delete(ctx, name)
}
