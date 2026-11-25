package settings

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strconv"
	"strings"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

type Service struct {
	db           *preset.DB
	llm          llm.Service
	sd           sd.Service
	cfg          *config.Config
	log          *logger.Logger
	serverClient *serverclient.Client
	bundledFS    fs.FS
}

type llmConfigSink interface {
	SetConfigSource(src llm.ConfigSource)
}

func New(db *preset.DB, llmSvc llm.Service, sdSvc sd.Service, cfg *config.Config, log *logger.Logger, srvClient *serverclient.Client) *Service {
	s := &Service{db: db, llm: llmSvc, sd: sdSvc, cfg: cfg, log: log, serverClient: srvClient}
	if sink, ok := llmSvc.(llmConfigSink); ok {
		sink.SetConfigSource(s.LLMConfig)
	}
	return s
}

func (s *Service) GetSettings() (map[string]string, error) {
	settings, err := s.db.GetAllSettings()
	if err != nil {
		return nil, err
	}

	defaults := map[string]string{
		config.DiagnosticsVerboseLogs: "false", config.DiagnosticsIntermediateImages: "false", config.StylistKeepOriginals: "true", config.DiagnosticsRetention: strconv.Itoa(config.DefaultDiagnosticsRetention),
		"llm_model":                   s.cfg.LLMModel,
		"sd_prompt_model":             s.cfg.SDPromptModel,
		"vision_model":                s.cfg.VisionModel,
		"llm_keep_alive":              "5m",
		"llm_max_tokens":              "256",
		"llm_generate_model":          s.cfg.SDPromptModel,
		"llm_analyze_model":           s.cfg.VisionModel,
		"llm_generate_num_ctx":        "4096",
		"llm_generate_top_p":          "0.9",
		"llm_generate_num_thread":     "0",
		"llm_analyze_num_ctx":         "4096",
		"llm_analyze_top_p":           "0.9",
		"llm_analyze_num_thread":      "0",
		"sd_prompt_instruction_prose": config.DefaultSDPromptInstructionProse,
		"llm_auto_unload":             "true",
		"kids_mode":                   "false",
		"kids_cat_violence":           "true",
		"kids_cat_horror":             "true",
		"kids_cat_weapons":            "true",
		"kids_cat_substances":         "true",
		"kids_cat_mature":             "true",
		"server_url":                  "",
	}
	for k, v := range defaults {
		if _, ok := settings[k]; !ok {
			settings[k] = v
		}
	}
	return settings, nil
}

func NormalizeServerURL(raw string) string {
	v := strings.TrimRight(strings.TrimSpace(raw), "/")
	if v != "" && !strings.Contains(v, "://") {
		v = "http://" + v
	}
	return v
}

func (s *Service) UpdateSettings(data map[string]string) error {
	if err := config.ValidateDiagnostics(data); err != nil {
		return err
	}
	if v, ok := data["server_url"]; ok {
		data["server_url"] = NormalizeServerURL(v)
	}
	urlFields := map[string]bool{"server_url": true}
	for k, v := range data {
		if urlFields[k] && v != "" {
			if _, err := url.Parse(v); err != nil {
				return fmt.Errorf("invalid %s: %w", k, err)
			}
		}
	}

	numericFields := map[string]bool{
		"llm_max_tokens": true,
	}
	for k, v := range data {
		if numericFields[k] {
			if v == "" {
				data[k] = "0"
			} else if n, err := strconv.Atoi(v); err != nil || n < 0 {
				data[k] = "0"
			}
		}
	}

	for k, v := range data {
		if !config.AllowedSettings[k] {
			continue
		}
		if err := s.db.SetSetting(k, v); err != nil {
			return err
		}
	}

	if v, ok := data["server_url"]; ok && v != "" && s.serverClient != nil {
		ApplyServerURL(v, s.cfg, s.llm, s.sd, s.serverClient)
		s.log.Info("Server: %s", v)
	}
	if v, ok := data["llm_model"]; ok {
		s.cfg.LLMModel = v
	}
	if v, ok := data["sd_prompt_model"]; ok {
		s.cfg.SDPromptModel = v
	}
	if v, ok := data["vision_model"]; ok {
		s.cfg.VisionModel = v
	}
	if v, ok := data["llm_generate_model"]; ok && v != "" {
		s.cfg.SDPromptModel = v
	}
	if v, ok := data["llm_analyze_model"]; ok && v != "" {
		s.cfg.VisionModel = v
	}

	var changed []string
	for k := range data {
		if config.AllowedSettings[k] {
			changed = append(changed, k)
		}
	}
	s.log.UserAction("Settings updated: %s", strings.Join(changed, ", "))

	return nil
}

func (s *Service) LLMConfig(mode string) llm.BackendConfig {
	prefix := "llm_generate_"
	if mode == llm.ModeAnalyze {
		prefix = "llm_analyze_"
	}

	var cfg llm.BackendConfig
	if v, err := s.db.GetSetting("llm_keep_alive"); err == nil {
		cfg.KeepAlive = v
	}
	if v, err := s.db.GetSetting(prefix + "num_ctx"); err == nil && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.NumCtx = n
		}
	}
	if v, err := s.db.GetSetting(prefix + "top_p"); err == nil && v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.TopP = n
		}
	}
	if v, err := s.db.GetSetting(prefix + "num_thread"); err == nil && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.NumThread = n
		}
	}
	return cfg
}

// --- Server mode methods ---

func (s *Service) DiscoverServers(ctx context.Context) ([]serverclient.DiscoveredServer, error) {
	return serverclient.DiscoverServers(ctx, 5e9) // 5 seconds
}

func (s *Service) ensureServerClient() error {
	if s.serverClient == nil || s.serverClient.GetBaseURL() == "" {
		return fmt.Errorf("server not configured")
	}
	return nil
}

func (s *Service) GetServerStatus() (*serverclient.ServerStatus, error) {
	if err := s.ensureServerClient(); err != nil {
		return nil, err
	}
	return s.serverClient.GetStatus()
}

func (s *Service) StartServerProcess(name string) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	return s.serverClient.StartProcess(name)
}

func (s *Service) StopServerProcess(name string) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	return s.serverClient.StopProcess(name)
}

func (s *Service) RestartServerProcess(name string) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	return s.serverClient.RestartProcess(name)
}

func (s *Service) GetServerModels(modelType string) ([]serverclient.ModelInfo, error) {
	if err := s.ensureServerClient(); err != nil {
		return nil, err
	}
	return s.serverClient.GetModels(modelType)
}

func (s *Service) GetServerLLMModels() ([]serverclient.LLMModelInfo, error) {
	if err := s.ensureServerClient(); err != nil {
		return nil, err
	}
	return s.serverClient.GetLLMModels()
}

func (s *Service) DownloadServerModel(modelType, url, filename string) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	d := newDownloadLog(s.log, modelType+"/"+filename)
	err := s.serverClient.DownloadModel(modelType, url, filename)
	d.finish(err)
	return err
}

func (s *Service) DownloadServerModelStream(modelType, url, filename string, onProgress func(string)) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	d := newDownloadLog(s.log, modelType+"/"+filename)
	err := s.serverClient.DownloadModelStream(modelType, url, filename, func(data string) {
		d.progress(data)
		if onProgress != nil {
			onProgress(data)
		}
	})
	d.finish(err)
	return err
}

func (s *Service) PullServerLLMModelStream(name string, onProgress func(string)) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	d := newDownloadLog(s.log, "LLM "+name)
	err := s.serverClient.PullLLMModelStream(name, func(data string) {
		d.progress(data)
		if onProgress != nil {
			onProgress(data)
		}
	})
	d.finish(err)
	return err
}

func (s *Service) DeleteServerModel(modelType, filename string) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	return s.serverClient.DeleteModel(modelType, filename)
}

func (s *Service) PullServerLLMModel(name string) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	d := newDownloadLog(s.log, "LLM "+name)
	err := s.serverClient.PullLLMModel(name)
	d.finish(err)
	return err
}

func (s *Service) GetServerProcessLogs(name string, lines int) ([]string, error) {
	if err := s.ensureServerClient(); err != nil {
		return nil, err
	}
	return s.serverClient.ProcessLogs(name, lines)
}

func (s *Service) DeleteServerLLMModel(name string) error {
	if err := s.ensureServerClient(); err != nil {
		return err
	}
	return s.serverClient.DeleteLLMModel(name)
}

func (s *Service) GetModelCatalog() (*serverclient.Catalog, error) {
	return serverclient.LoadCatalog()
}
