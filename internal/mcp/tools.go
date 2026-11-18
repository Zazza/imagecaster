package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/sd"
)

func RegisterTools(s *Server) {
	registerStatusTools(s)
	registerPresetTools(s)
	registerCompoundTools(s)
	registerGenerateTools(s)
	registerImageTools(s)
	registerMetaTools(s)
	registerEditTools(s)
	registerStylistTools(s)
	registerShootTools(s)
	registerShootQueueTools(s)
	registerRetouchTools(s)
	registerEditorTools(s)
}

func registerStatusTools(s *Server) {
	s.Register(Tool{
		Name:        "status",
		Description: "Статус Imagecaster: доступность SD/LLM, текущая модель SD, прогресс генерации, kids mode, путь к БД и папке артефактов.",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			res := map[string]any{
				"db":        s.deps.Cfg.DBPath,
				"out_dir":   s.deps.OutDir,
				"kids_mode": s.deps.Kids.IsActive(),
			}
			if err := s.deps.SD.HealthCheck(); err != nil {
				res["sd"] = map[string]any{"available": false, "error": err.Error()}
			} else {
				m := map[string]any{"available": true, "url": s.deps.Cfg.SDUrl}
				if st, err := s.deps.SD.State(); err == nil {
					m["model"] = st.LoadedCheckpoint
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				jobs, err := s.deps.SD.ListJobs(ctx, "")
				cancel()
				if err == nil {
					for _, j := range jobs {
						if j.Status != sd.JobRunning {
							continue
						}
						progress := 0.0
						if j.Progress.Steps > 0 {
							progress = float64(j.Progress.Step) / float64(j.Progress.Steps)
						}
						m["progress"] = map[string]any{
							"progress": progress,
							"job":      j.ID,
							"steps":    j.Progress.Steps,
						}
						break
					}
				}
				res["sd"] = m
			}
			if err := s.deps.LLM.HealthCheck(); err != nil {
				res["llm"] = map[string]any{"available": false, "error": err.Error()}
			} else {
				res["llm"] = map[string]any{"available": true, "url": s.deps.Cfg.LLMUrl}
			}
			return toJSON(res), nil
		},
	})
}

func infoSeed(info json.RawMessage) int64 {
	if len(info) == 0 {
		return 0
	}
	var v struct {
		Seed int64 `json:"seed"`
	}
	if err := json.Unmarshal(info, &v); err != nil {
		return 0
	}
	return v.Seed
}

func hasTag(tagsCSV, tag string) bool {
	for _, t := range strings.Split(tagsCSV, ",") {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return true
		}
	}
	return false
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "-", "\t", "-").Replace(s)
	return filebrowser.SanitizeFilename(s)
}
