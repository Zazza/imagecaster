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
	MinPhotos         = 10
	MaxPhotos         = 50
	MinIdentityPhotos = 5
)

var (
	ErrNotLocal      = errors.New("сервер не в локальной сети: фото людей отправляются только на локальный сервер")
	ErrPhotoCount    = errors.New("в папке должно быть от 10 до 50 фото (jpg, png, webp)")
	ErrIdentityCount = errors.New("в папке должно быть от 5 до 50 фото (jpg, png, webp)")
	ErrTooFewFaces   = errors.New("на фото мало чётко видных лиц: нужно хотя бы несколько фото, где лицо крупно и смотрит в камеру")
	ErrNoFaceModels  = errors.New("на сервере не установлены модели подмены лица: скачайте их в каталоге, раздел «Подмена лица»")
)

var photoExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

type API interface {
	SubmitTraining(ctx context.Context, r serverclient.TrainRequest) (*serverclient.TrainJob, error)
	TrainingJobs(ctx context.Context, client string) ([]serverclient.TrainJob, error)
	CancelTraining(ctx context.Context, id string) (*serverclient.TrainJob, error)
	PauseTraining(ctx context.Context, id string) (*serverclient.TrainJob, error)
	ResumeTraining(ctx context.Context, id string) (*serverclient.TrainJob, error)
	RenamePerson(ctx context.Context, oldName, newName string) (*serverclient.Person, error)
	People(ctx context.Context) ([]serverclient.Person, error)
	DeletePerson(ctx context.Context, name string) error
	UploadIdentity(ctx context.Context, name string, photos []string) (*serverclient.Person, error)
	TrainingBases(ctx context.Context) ([]string, error)
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
	out, err := listPhotos(dir)
	if err != nil {
		return nil, err
	}
	if len(out) < MinPhotos || len(out) > MaxPhotos {
		return nil, fmt.Errorf("%w: найдено %d", ErrPhotoCount, len(out))
	}
	return out, nil
}

func listPhotos(dir string) ([]string, error) {
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
	return out, nil
}

func (s *Service) local() bool {
	u := s.serverURL()
	return lanonly.IsLocalURL(u) && !lanonly.ViaProxy(u)
}

func (s *Service) Train(ctx context.Context, folder, name, gender, base string) (*serverclient.TrainJob, error) {
	if !s.local() {
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
		Base:      base,
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

func (s *Service) Pause(ctx context.Context, id string) (*serverclient.TrainJob, error) {
	return s.api.PauseTraining(ctx, id)
}

func (s *Service) Resume(ctx context.Context, id string) (*serverclient.TrainJob, error) {
	return s.api.ResumeTraining(ctx, id)
}

func (s *Service) Rename(ctx context.Context, oldName, newName string) error {
	_, err := s.api.RenamePerson(ctx, oldName, strings.ToLower(strings.TrimSpace(newName)))
	return err
}

func (s *Service) List(ctx context.Context) ([]serverclient.Person, error) {
	return s.api.People(ctx)
}

func (s *Service) Delete(ctx context.Context, name string) error {
	return s.api.DeletePerson(ctx, name)
}

func (s *Service) Bases(ctx context.Context) ([]string, error) {
	return s.api.TrainingBases(ctx)
}

func (s *Service) AddIdentity(ctx context.Context, folder, name string) (*serverclient.Person, error) {
	if !s.local() {
		return nil, ErrNotLocal
	}
	photos, err := listPhotos(folder)
	if err != nil {
		return nil, err
	}
	if len(photos) < MinIdentityPhotos || len(photos) > MaxPhotos {
		return nil, fmt.Errorf("%w: найдено %d", ErrIdentityCount, len(photos))
	}
	p, err := s.api.UploadIdentity(ctx, name, photos)
	var apiErr *serverclient.TrainAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "too_few_faces":
			return nil, ErrTooFewFaces
		case "face_models_missing":
			return nil, ErrNoFaceModels
		}
	}
	return p, err
}
