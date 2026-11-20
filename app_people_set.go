package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/people"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

var errPeopleNoDB = errors.New("база приложения недоступна")

const personSetAddTimeout = 2 * time.Minute

func (a *App) photoSet() (*people.PhotoSet, error) {
	if a.presets == nil || a.serverClient == nil {
		return nil, errPeopleNoDB
	}
	local := func() bool {
		u := a.serverClient.GetBaseURL()
		return u != "" && lanonly.IsLocalURL(u) && !lanonly.ViaProxy(u)
	}
	return people.NewPhotoSet(a.presets, a.serverClient, filepath.Join(a.dataDir, "people"), local), nil
}

func (a *App) PersonSetAdd(req people.SetAddRequest) (*people.SetAddResult, error) {
	s, err := a.photoSet()
	if err != nil {
		return nil, err
	}
	ctx, cancel := a.peopleCtx(personSetAddTimeout)
	defer cancel()
	res, err := s.Add(ctx, req)
	if err != nil {
		a.log.Warn("people set: add %q: %s", filepath.Base(req.Path), err)
	}
	return res, err
}

func (a *App) PersonSetList(key string) ([]preset.PersonPhoto, error) {
	s, err := a.photoSet()
	if err != nil {
		return nil, err
	}
	list, err := s.List(key)
	if list == nil {
		list = []preset.PersonPhoto{}
	}
	return list, err
}

func (a *App) PersonSetThumb(id int64) (string, error) {
	s, err := a.photoSet()
	if err != nil {
		return "", err
	}
	return s.Thumb(id)
}

func (a *App) PersonSetRemove(id int64) error {
	s, err := a.photoSet()
	if err != nil {
		return err
	}
	return s.Remove(id)
}

func (a *App) PersonSetSummary() ([]people.SetSummary, error) {
	s, err := a.photoSet()
	if err != nil {
		return nil, err
	}
	out, err := s.Summary()
	if out == nil {
		out = []people.SetSummary{}
	}
	return out, err
}

func (a *App) PersonSetRename(oldKey, newKey string) error {
	if a.presets == nil {
		return errPeopleNoDB
	}
	newKey = people.NormalizeSetKey(newKey)
	if newKey == "" {
		return people.ErrSetNoName
	}
	return a.presets.RenamePersonPhotos(people.NormalizeSetKey(oldKey), newKey)
}

func (a *App) withSetFolder(key string, fn func(folder string) error) error {
	s, err := a.photoSet()
	if err != nil {
		return err
	}
	tmp := filepath.Join(a.dataDir, "people", "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	dir, cleanup, err := s.Folder(key, tmp)
	if err != nil {
		return err
	}
	defer cleanup()
	return fn(dir)
}

func (a *App) TrainPersonFromSet(key, name, gender, base, mode string, hold, replace bool) (*serverclient.TrainJob, error) {
	key = people.NormalizeSetKey(key)
	if key == "" {
		return nil, people.ErrSetNoName
	}
	if a.serverClient == nil || a.serverClient.GetBaseURL() == "" {
		return nil, errors.New("сервер Imagecaster не настроен")
	}
	s, err := a.photoSet()
	if err != nil {
		return nil, err
	}
	var since *time.Time
	switch mode {
	case "continue":
		p, err := a.findPerson(name)
		if err != nil {
			return nil, err
		}
		since = p.PhotosAt
		if since == nil {
			since = p.CreatedAt
		}
	case "retrain":
	default:
		if err := a.claimTechKey(key); err != nil {
			return nil, err
		}
	}
	tmp := filepath.Join(a.dataDir, "people", "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}
	collected := time.Now().UTC()
	oldDir, newDir, cleanup, err := s.SplitFolder(key, tmp, since)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	ctx, cancel := a.peopleCtx(0)
	defer cancel()
	job, err := a.peopleSvc().TrainSet(ctx, people.TrainSetRequest{
		Folder: oldDir, NewFolder: newDir, Name: name, Display: key, Gender: gender, Base: base,
		Mode: mode, Hold: hold, Replace: replace, PhotosAt: &collected,
	})
	if err != nil {
		a.log.Warn("people: train %q from set (%s): %s", name, mode, err)
	}
	return job, err
}

func (a *App) findPerson(name string) (*serverclient.Person, error) {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	list, err := a.peopleSvc().List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("LoRA %q не найдена на сервере", name)
}

func (a *App) RollbackPerson(name string) (*serverclient.Person, error) {
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	p, err := a.peopleSvc().Rollback(ctx, name)
	if err != nil {
		a.log.Warn("people: rollback %q: %s", name, err)
	}
	return p, err
}

type displaySetter interface {
	List(ctx context.Context) ([]serverclient.Person, error)
	SetDisplay(ctx context.Context, name, display string, replace bool) (*serverclient.Person, error)
}

func (a *App) claimTechKey(key string) error {
	if a.serverClient == nil || a.serverClient.GetBaseURL() == "" {
		return nil
	}
	ctx, cancel := a.peopleCtx(peopleCallTimeout)
	defer cancel()
	return claimTechKey(ctx, a.peopleSvc(), key)
}

func claimTechKey(ctx context.Context, svc displaySetter, key string) error {
	list, err := svc.List(ctx)
	if err != nil {
		return err
	}
	for _, p := range list {
		if strings.TrimSpace(p.DisplayName) == "" && strings.EqualFold(p.Name, key) {
			_, err := svc.SetDisplay(ctx, p.Name, key, false)
			return err
		}
	}
	return nil
}

func (a *App) AddPersonIdentityFromSet(key, name string) (*serverclient.Person, error) {
	var p *serverclient.Person
	err := a.withSetFolder(key, func(dir string) error {
		var err error
		p, err = a.AddPersonIdentity(dir, name)
		return err
	})
	return p, err
}
