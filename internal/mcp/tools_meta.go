package mcp

import (
	"strings"
)

func registerMetaTools(s *Server) {
	s.Register(Tool{
		Name:        "sd_meta",
		Description: "Метаданные SD WebUI одним ответом: models, samplers, schedulers, vae, loras, upscalers. include — CSV-фильтр, например \"models,samplers\"; пусто — все.",
		InputSchema: props(map[string]any{
			"include": prop("CSV-список секций: models,samplers,schedulers,vae,loras,upscalers", "string"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			want := map[string]bool{
				"models": true, "samplers": true, "schedulers": true,
				"vae": true, "loras": true, "upscalers": true,
			}
			if inc := argString(args, "include"); inc != "" {
				for k := range want {
					want[k] = false
				}
				for _, part := range strings.Split(inc, ",") {
					part = strings.TrimSpace(strings.ToLower(part))
					if _, ok := want[part]; ok {
						want[part] = true
					}
				}
			}
			res := map[string]any{}
			if want["models"] {
				if v, err := s.deps.SD.GetModels(); err != nil {
					res["models"] = map[string]string{"error": err.Error()}
				} else {
					out := make([]string, 0, len(v))
					for _, m := range v {
						out = append(out, m.Title)
					}
					res["models"] = out
				}
			}
			if want["samplers"] {
				if v, err := s.deps.SD.GetSamplers(); err != nil {
					res["samplers"] = map[string]string{"error": err.Error()}
				} else {
					out := make([]string, 0, len(v))
					for _, m := range v {
						out = append(out, m.Name)
					}
					res["samplers"] = out
				}
			}
			if want["schedulers"] {
				if v, err := s.deps.SD.GetSchedulers(); err != nil {
					res["schedulers"] = map[string]string{"error": err.Error()}
				} else {
					out := make([]string, 0, len(v))
					for _, m := range v {
						out = append(out, m.Name)
					}
					res["schedulers"] = out
				}
			}
			if want["vae"] {
				if v, err := s.deps.SD.GetVAEs(); err != nil {
					res["vae"] = map[string]string{"error": err.Error()}
				} else {
					out := make([]string, 0, len(v))
					for _, m := range v {
						out = append(out, m.ModelName)
					}
					res["vae"] = out
				}
			}
			if want["loras"] {
				if v, err := s.deps.SD.GetLoRAs(); err != nil {
					res["loras"] = map[string]string{"error": err.Error()}
				} else {
					out := make([]string, 0, len(v))
					for _, m := range v {
						out = append(out, m.Name)
					}
					res["loras"] = out
				}
			}
			if want["upscalers"] {
				if v, err := s.deps.SD.GetUpscalers(); err != nil {
					res["upscalers"] = map[string]string{"error": err.Error()}
				} else {
					out := make([]string, 0, len(v))
					for _, m := range v {
						out = append(out, m.Name)
					}
					res["upscalers"] = out
				}
			}
			return toJSON(res), nil
		},
	})

	s.Register(Tool{
		Name:        "interrupt",
		Description: "Прервать генерации SD, запущенные этим MCP-сервером. Генерации десктоп-приложения не затрагиваются.",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			if err := s.deps.Gen.InterruptGeneration(); err != nil {
				return "", err
			}
			return "прервано", nil
		},
	})
}
