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

func (a *App) TrainPerson(folder, name, gender, base string, hold bool) (*serverclient.TrainJob, error) {
	if a.serverClient.GetBaseURL() == "" {
		return nil, errors.New("сервер Imagecaster не настроен")
	}
	ctx, cancel := a.peopleCtx(0)
	defer cancel()
	job, err := a.peopleSvc().Train(ctx, folder, name, gender, base, hold)
	if err != nil {
		a.log.Warn("people: train %q: %s", name, err)
	}
	return job, err
}

func (a *App) TrainingBases() ([]string, error) {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	bases, err := a.peopleSvc().Bases(ctx)
	if err != nil {
		a.log.Warn("people: bases: %s", err)
	}
	return bases, err
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

func (a *App) PauseTraining(id string) (*serverclient.TrainJob, error) {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return a.peopleSvc().Pause(ctx, id)
}

func (a *App) ResumeTraining(id string) (*serverclient.TrainJob, error) {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return a.peopleSvc().Resume(ctx, id)
}

func (a *App) RenamePerson(oldName, newName string) error {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return a.peopleSvc().Rename(ctx, oldName, newName)
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

func (a *App) AddPersonIdentity(folder, name string) (*serverclient.Person, error) {
	if a.serverClient.GetBaseURL() == "" {
		return nil, errors.New("сервер Imagecaster не настроен")
	}
	ctx, cancel := a.peopleCtx(0)
	defer cancel()
	p, err := a.peopleSvc().AddIdentity(ctx, folder, name)
	if err != nil {
		a.log.Warn("people: identity %q: %s", name, err)
	}
	return p, err
}

func (a *App) FaceSwapStatus() string {
	if a.serverClient.GetBaseURL() == "" {
		return "no_server"
	}
	info, err := serverclient.Probe(a.serverClient.GetBaseURL())
	if err != nil {
		return "unreachable"
	}
	for _, f := range info.Features {
		if f == "face_swap" {
			return "ok"
		}
	}
	return "not_installed"
}
