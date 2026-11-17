package people

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

const (
	MinPhotos = 10
	MaxPhotos = 50
)

var (
	ErrNotLocal   = errors.New("сервер не в локальной сети: фото людей отправляются только на локальный сервер")
	ErrPhotoCount = errors.New("в папке должно быть от 10 до 50 фото (jpg, png, webp)")
)

var photoExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

type API interface {
	SubmitTraining(ctx context.Context, r serverclient.TrainRequest) (*serverclient.TrainJob, error)
	TrainingJobs(ctx context.Context, client string) ([]serverclient.TrainJob, error)
	CancelTraining(ctx context.Context, id string) (*serverclient.TrainJob, error)
	People(ctx context.Context) ([]serverclient.Person, error)
	DeletePerson(ctx context.Context, name string) error
}

type Service struct {
	api       API
	serverURL func() string
	installID func() string
}

func New(api API, serverURL func() string, installID func() string) *Service {
	return &Service{api: api, serverURL: serverURL, installID: installID}
}

func CollectPhotos(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !e.Type().IsRegular() || !photoExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	if len(out) < MinPhotos || len(out) > MaxPhotos {
		return nil, fmt.Errorf("%w: найдено %d", ErrPhotoCount, len(out))
	}
	return out, nil
}

func (s *Service) Train(ctx context.Context, folder, name, gender string) (*serverclient.TrainJob, error) {
	u := s.serverURL()
	if !lanonly.IsLocalURL(u) || lanonly.ViaProxy(u) {
		return nil, ErrNotLocal
	}
	photos, err := CollectPhotos(folder)
	if err != nil {
		return nil, err
	}
	id := s.installID()
	name = strings.ToLower(strings.TrimSpace(name))
	return s.api.SubmitTraining(ctx, serverclient.TrainRequest{
		Name:      name,
		Gender:    gender,
		Client:    id,
		ClientRef: id + ":train:" + name,
		Photos:    photos,
	})
}

func (s *Service) Jobs(ctx context.Context) ([]serverclient.TrainJob, error) {
	return s.api.TrainingJobs(ctx, s.installID())
}

func (s *Service) Cancel(ctx context.Context, id string) (*serverclient.TrainJob, error) {
	return s.api.CancelTraining(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]serverclient.Person, error) {
	return s.api.People(ctx)
}

func (s *Service) Delete(ctx context.Context, name string) error {
	return s.api.DeletePerson(ctx, name)
}
