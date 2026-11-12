package settings

import (
	"fmt"
	"log"
	"net"
	"net/url"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
)

func Restore(db *preset.DB, cfg *config.Config, llmSvc llm.Service, sdSvc sd.Service, srvClient *serverclient.Client) {
	serverURL, _ := db.GetSetting("server_url")
	if mode, _ := db.GetSetting("connection_mode"); mode != "server" {
		if found := findServerFromDirectURLs(db); found != "" {
			serverURL = found
			if err := db.SetSetting("server_url", found); err != nil {
				log.Printf("[settings] save server_url: %v", err)
			}
		}
		if err := db.SetSetting("connection_mode", "server"); err != nil {
			log.Printf("[settings] save connection_mode: %v", err)
		}
	}
	if serverURL != "" {
		ApplyServerURL(serverURL, cfg, llmSvc, sdSvc, srvClient)
	}
	if v, _ := db.GetSetting("llm_model"); v != "" {
		cfg.LLMModel = v
	}
	if v, _ := db.GetSetting("sd_prompt_model"); v != "" {
		cfg.SDPromptModel = v
	}
	if v, _ := db.GetSetting("vision_model"); v != "" {
		cfg.VisionModel = v
	}

	var backendCfg llm.BackendConfig
	if v, _ := db.GetSetting("llm_keep_alive"); v != "" {
		backendCfg.KeepAlive = v
	} else {
		backendCfg.KeepAlive = "5m"
	}
	if v, _ := db.GetSetting("llm_num_ctx"); v != "" {
		fmt.Sscanf(v, "%d", &backendCfg.NumCtx)
	} else {
		backendCfg.NumCtx = 4096
	}
	if v, _ := db.GetSetting("llm_num_gpu"); v != "" {
		fmt.Sscanf(v, "%d", &backendCfg.NumGPU)
	}
	llmSvc.SetBackendConfig(backendCfg)
}

func ApplyServerURL(serverURL string, cfg *config.Config, llmSvc llm.Service, sdSvc sd.Service, srvClient *serverclient.Client) {
	srvClient.SetBaseURL(serverURL)
	sdURL, llmURL := srvClient.ProxyURLs()
	cfg.SDUrl = sdURL
	cfg.LLMUrl = llmURL
	sdSvc.SetURL(sdURL)
	llmSvc.SetURL(llmURL)
}

func findServerFromDirectURLs(db *preset.DB) string {
	seen := map[string]bool{}
	for _, key := range []string{"sd_url", "llm_url"} {
		raw, _ := db.GetSetting(key)
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		candidate := u.Scheme + "://" + net.JoinHostPort(u.Hostname(), "8080")
		if u.Scheme != "http" && u.Scheme != "https" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := serverclient.Probe(candidate); err == nil {
			log.Printf("[settings] old direct connection moved to server %s", candidate)
			return candidate
		}
	}
	return ""
}
