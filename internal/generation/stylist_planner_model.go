package generation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/lanonly"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/stylist"
)

const (
	plannerIdleReleaseTimeout   = 75 * time.Second
	plannerMemoryRefreshWait    = 6 * time.Second
	plannerCaptionPollInterval  = 250 * time.Millisecond
	plannerCaptionRetryInterval = time.Second
)

type plannerIdleUnloader interface {
	UnloadIdleModel(context.Context) error
	IdleModelServerURL() string
}

type plannerCaptionUnloader interface {
	UnloadModel(context.Context, string) error
	IdleModelServerURL() string
}

func (s *Service) StylistPlannerModel() (string, error) {
	resolve := func() (string, error) {
		return stylist.ResolvePlannerModelWithCapacity(s.getGenerateModel(), s.llm.GetModels, s.plannerCapacity)
	}
	model, err := resolve()
	if !errors.Is(err, stylist.ErrPlannerCapacity) {
		return model, err
	}
	if releaseErr := s.releaseIdlePlannerModel(); releaseErr != nil {
		return "", fmt.Errorf("%w; не удалось освободить память SD/ИИ: %v", err, releaseErr)
	}
	return resolve()
}

func (s *Service) plannerCapacity() (stylist.PlannerCapacity, error) {
	stats, err := s.sd.MemoryInfo()
	if err != nil {
		return stylist.PlannerCapacity{}, err
	}
	if stats == nil || stats.CUDA == nil {
		return stylist.PlannerCapacity{}, errors.New("данные свободной видеопамяти недоступны")
	}
	loaded, err := s.llm.ListLoaded()
	if err != nil {
		return stylist.PlannerCapacity{}, err
	}
	return stylist.PlannerCapacity{FreeVRAM: stats.CUDA.System.Free, FreeRAM: stats.RAM.Free, Loaded: loaded}, nil
}

func (s *Service) releaseIdlePlannerModel() error {
	unloader, ok := s.sd.(plannerIdleUnloader)
	if !ok || s.db == nil || s.db.DB() == nil {
		return errors.New("проверка простоя SD недоступна")
	}
	state, err := s.sd.State()
	if err != nil {
		return err
	}
	if state == nil || state.RunningJob != "" {
		return errors.New("SD занята или её состояние недоступно")
	}
	serverURL, err := s.db.GetSetting("server_url")
	if err != nil {
		return err
	}
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || !lanonly.IsLocalURL(serverURL) || lanonly.ViaProxy(serverURL) {
		return errors.New("адрес сервера для проверки обучения недоступен")
	}
	if strings.TrimRight(serverURL, "/") != unloader.IdleModelServerURL() {
		return errors.New("адрес SD не совпадает с сервером проверки обучения")
	}
	ctx, cancel := context.WithTimeout(context.Background(), plannerIdleReleaseTimeout)
	defer cancel()
	var active int
	if err := s.db.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM job_queue WHERE status IN ('pending', 'running')").Scan(&active); err != nil {
		return fmt.Errorf("проверка очереди приложения: %w", err)
	}
	if active != 0 {
		return errors.New("в очереди приложения есть незавершённые задачи")
	}
	jobs, err := s.sd.ListJobs(ctx, "")
	if err != nil {
		return fmt.Errorf("проверка очереди SD: %w", err)
	}
	if jobs == nil {
		return errors.New("ответ очереди SD не содержит список задач")
	}
	for _, job := range jobs {
		if !job.Terminal() {
			return errors.New("в очереди SD есть незавершённые задачи")
		}
	}
	server := serverclient.NewClient()
	server.SetBaseURL(serverURL)
	training, err := server.TrainingJobs(ctx, "")
	if err != nil {
		return fmt.Errorf("проверка обучения: %w", err)
	}
	for _, job := range training {
		switch job.Status {
		case "completed", "failed", "cancelled", "paused":
		default:
			return errors.New("на сервере есть незавершённое обучение")
		}
	}
	if err := s.releasePlannerCaption(ctx, serverURL); err != nil {
		return err
	}
	if strings.TrimSpace(state.LoadedCheckpoint) != "" {
		if err := unloader.UnloadIdleModel(ctx); err != nil {
			return err
		}
	}
	timer := time.NewTimer(plannerMemoryRefreshWait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) releasePlannerCaption(ctx context.Context, serverURL string) error {
	catalog, err := serverclient.LoadCatalog()
	if err != nil {
		return err
	}
	configured := s.getGenerateModel()
	if !slices.ContainsFunc(catalog.LLMVision, func(model serverclient.CatalogModel) bool {
		return model.Name == configured && model.CaptionOnly
	}) {
		return nil
	}
	loaded, err := s.llm.ListLoaded()
	if err != nil {
		return err
	}
	isCaption := func(model llm.LoadedModel) bool { return model.Name == configured }
	if !slices.ContainsFunc(loaded, isCaption) {
		return nil
	}
	unloader, ok := s.llm.(plannerCaptionUnloader)
	if !ok {
		return errors.New("выгрузка модели описания фотографий недоступна")
	}
	if strings.TrimRight(serverURL, "/") != unloader.IdleModelServerURL() {
		return errors.New("адрес ИИ не совпадает с сервером проверки обучения")
	}
	if err := unloader.UnloadModel(ctx, configured); err != nil {
		return err
	}
	nextRetry := time.Now().Add(plannerCaptionRetryInterval)
	ticker := time.NewTicker(plannerCaptionPollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("модель описания фотографий не освободила память: %w", err)
		}
		loaded, err = s.llm.ListLoaded()
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(loaded, isCaption) {
			return nil
		}
		if !time.Now().Before(nextRetry) {
			if err := unloader.UnloadModel(ctx, configured); err != nil {
				return err
			}
			nextRetry = time.Now().Add(plannerCaptionRetryInterval)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("модель описания фотографий не освободила память: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
