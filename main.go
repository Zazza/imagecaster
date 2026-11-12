package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/settings"
)

var version = "dev"

func init() {
	if version == "dev" {
		info, ok := debug.ReadBuildInfo()
		if ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					version = "dev-" + s.Value[:7]
					break
				}
			}
		}
	}
}

//go:embed all:frontend/dist
var assets embed.FS

//go:embed data/presets/*.json
var bundledPresets embed.FS

// иконка окна под Linux (Windows/macOS берут её из build/ при сборке)
//
//go:embed build/appicon.png
var appIcon []byte

//go:embed build/appicon-256.png
var windowIcon []byte

func main() {
	cfg := config.Load()

	presets, err := preset.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer presets.Close()

	llmClient := llm.New(cfg.LLMUrl)
	sdClient := sd.New(cfg.SDUrl)
	srvClient := serverclient.NewClient()

	settings.Restore(presets, cfg, llmClient, sdClient, srvClient)

	app := NewApp(presets, llmClient, sdClient, srvClient, cfg)
	app.settingsSvc.SetBundledPresets(bundledPresets)
	if err := app.settingsSvc.MigratePresetInstallLog(); err != nil {
		log.Printf("Warning: preset install log migration failed: %v", err)
	}
	imgHandler := &imageFileHandler{db: presets, dataDir: filepath.Dir(cfg.DBPath)}

	if err := wails.Run(&options.App{
		Title:            "Imagecaster",
		Width:            1280,
		Height:           800,
		Frameless:        runtime.GOOS != "darwin",
		MinWidth:         900,
		MinHeight:        600,
		MaxWidth:         7680,
		MaxHeight:        4320,
		BackgroundColour: windowBackground(),
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: imgHandler,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHidden(),
		},
		Linux: &linux.Options{
			WebviewGpuPolicy:    linux.WebviewGpuPolicyAlways,
			WindowIsTranslucent: true,
			Icon:                windowIcon,
		},
	}); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

type imageFileHandler struct {
	db      *preset.DB
	dataDir string
}

func (h *imageFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	var dir string
	var idStr string

	if strings.HasPrefix(p, "/api/img/") {
		idStr = strings.TrimPrefix(p, "/api/img/")
		dir = "sessions"
	} else if strings.HasPrefix(p, "/api/thumb/") {
		idStr = strings.TrimPrefix(p, "/api/thumb/")
		dir = "thumbs"
	} else {
		http.NotFound(w, r)
		return
	}

	idStr = strings.TrimSuffix(idStr, ".jpg")
	idStr = strings.TrimSuffix(idStr, ".png")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	item, err := h.db.GetSessionItem(id)
	if err != nil || item == nil {
		http.NotFound(w, r)
		return
	}

	var fileName string
	if dir == "sessions" {
		fileName = item.FileName
	} else {
		fileName = item.ThumbName
	}
	if fileName == "" {
		http.NotFound(w, r)
		return
	}

	filePath := filepath.Join(h.dataDir, dir, strconv.FormatInt(item.SessionID, 10), fileName)
	if _, err := os.Stat(filePath); err != nil {
		http.NotFound(w, r)
		return
	}

	contentType := "image/jpeg"
	if strings.HasSuffix(fileName, ".png") {
		contentType = "image/png"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "max-age=3600")
	http.ServeFile(w, r, filePath)
}
