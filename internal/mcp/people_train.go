package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/serverclient"
)

const (
	peopleTrainUpload = 15 * time.Minute
	peopleTrainCall   = 20 * time.Second
)

type PeopleTrainer interface {
	Train(ctx context.Context, folder, name, display, gender, base string, hold bool) (*serverclient.TrainJob, error)
	Jobs(ctx context.Context) ([]serverclient.TrainJob, error)
	Resume(ctx context.Context, id string) (*serverclient.TrainJob, error)
	Bases(ctx context.Context) ([]string, error)
}

func (s *Server) trainer() (PeopleTrainer, error) {
	if s.deps.Trainer == nil {
		return nil, errors.New("обучение людей недоступно в этой сборке imagecaster-mcp")
	}
	return s.deps.Trainer, nil
}

func trainJobView(j serverclient.TrainJob) map[string]any {
	return map[string]any{
		"id":          j.ID,
		"name":        j.Name,
		"base":        j.Base,
		"status":      j.Status,
		"not_started": j.Status == "paused" && j.StartedAt == nil,
		"step":        j.Progress.Step,
		"steps":       j.Progress.Steps,
	}
}

func trainBases(t PeopleTrainer) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), peopleTrainCall)
	defer cancel()
	bases, err := t.Bases(ctx)
	if err != nil {
		return nil, fmt.Errorf("список моделей обучения с сервера: %w", err)
	}
	return bases, nil
}

func registerPeopleTrainTools(s *Server) {
	s.Register(Tool{
		Name: "people_train",
		Description: "Люди: обучить LoRA человека по папке с фото на этой машине (10–100 jpg/png/webp). Фото уходят только на локальный сервер, тебе не показываются. " +
			"Задание видно на странице «Люди» приложения. start=false — создать без запуска (на паузе, запустить позже people_train_start или кнопкой «Запустить»); по умолчанию запускается сразу. " +
			"base — модель из bases в people_train_jobs; какую брать, решает человек.",
		InputSchema: props(map[string]any{
			"folder":       prop("абсолютный путь к папке с фото человека", "string"),
			"name":         prop("имя LoRA: 2–32 символа, латиница, цифры, _", "string"),
			"display_name": prop("Имя человека (Наташа, Дима; до 40 символов): по нему человек выбирается на Генерации, в Стилисте и в stylist_shoot; у одного Имени одна LoRA на модель", "string"),
			"gender":       prop("man | woman | person", "string", map[string]any{"enum": []string{"man", "woman", "person"}}),
			"base":         prop("модель обучения из bases (people_train_jobs)", "string"),
			"start":        prop("запустить сразу (по умолчанию true)", "boolean"),
		}, "folder", "name", "gender", "base"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			t, err := s.trainer()
			if err != nil {
				return "", err
			}
			bases, err := trainBases(t)
			if err != nil {
				return "", err
			}
			base := strings.TrimSpace(argString(args, "base"))
			if !slices.Contains(bases, base) {
				if len(bases) == 0 {
					return "", errors.New("на сервере нет моделей для обучения — их качают в каталоге, вкладка «Для обучения»")
				}
				return "", fmt.Errorf("base %q недоступна; доступные: %s", base, strings.Join(bases, ", "))
			}
			hold := false
			if v, ok := args["start"].(bool); ok {
				hold = !v
			}
			ctx, cancel := context.WithTimeout(context.Background(), peopleTrainUpload)
			defer cancel()
			job, err := t.Train(ctx, argString(args, "folder"), argString(args, "name"), argString(args, "display_name"), argString(args, "gender"), base, hold)
			if err != nil {
				return "", err
			}
			return toJSON(map[string]any{"job": trainJobView(*job)}), nil
		},
	})
	s.Register(Tool{
		Name:        "people_train_jobs",
		Description: "Люди: задания обучения LoRA этой установки (status queued|running|paused|completed|failed|cancelled; not_started — создано без запуска) и bases — модели, доступные для обучения.",
		InputSchema: props(map[string]any{}),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			t, err := s.trainer()
			if err != nil {
				return "", err
			}
			ctx, cancel := context.WithTimeout(context.Background(), peopleTrainCall)
			defer cancel()
			jobs, err := t.Jobs(ctx)
			if err != nil {
				return "", fmt.Errorf("задания обучения с сервера: %w", err)
			}
			bases, err := trainBases(t)
			if err != nil {
				return "", err
			}
			out := make([]map[string]any, 0, len(jobs))
			for _, j := range jobs {
				out = append(out, trainJobView(j))
			}
			return toJSON(map[string]any{"jobs": out, "bases": bases}), nil
		},
	})
	s.Register(Tool{
		Name:        "people_train_start",
		Description: "Люди: запустить обучение на паузе (созданное без запуска или приостановленное) по имени. Если видеокарта занята другим обучением, встаёт в очередь.",
		InputSchema: props(map[string]any{
			"name": prop("имя задания из people_train_jobs", "string"),
		}, "name"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			t, err := s.trainer()
			if err != nil {
				return "", err
			}
			name := strings.ToLower(strings.TrimSpace(argString(args, "name")))
			ctx, cancel := context.WithTimeout(context.Background(), peopleTrainCall)
			defer cancel()
			jobs, err := t.Jobs(ctx)
			if err != nil {
				return "", fmt.Errorf("задания обучения с сервера: %w", err)
			}
			for _, j := range jobs {
				if j.Name != name || j.Status != "paused" {
					continue
				}
				job, err := t.Resume(ctx, j.ID)
				if err != nil {
					return "", err
				}
				return toJSON(map[string]any{"job": trainJobView(*job)}), nil
			}
			return "", fmt.Errorf("обучения %q на паузе нет", name)
		},
	})
}
