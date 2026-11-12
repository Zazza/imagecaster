package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/mcp"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/session"
	"github.com/Zazza/imagecaster/internal/settings"
)

func env(name string) string {
	if v := os.Getenv("IMAGECASTER_MCP_" + name); v != "" {
		return v
	}
	return os.Getenv("SD_MCP_" + name)
}

func main() {
	log.SetFlags(0)

	dbPath := flag.String("db", "", "путь к presets.db (по умолчанию env IMAGECASTER_MCP_DB, затем config.Load)")
	flag.Parse()

	cfg := config.Load()
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	} else if v := env("DB"); v != "" {
		cfg.DBPath = v
	}

	db, err := preset.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	llmClient := llm.New(cfg.LLMUrl)
	sdClient := sd.New(cfg.SDUrl)
	srvClient := serverclient.NewClient()

	settings.Restore(db, cfg, llmClient, sdClient, srvClient)

	logSvc := logger.New(nil)
	kidsMgr := kids.NewManager(db)
	settingsSvc := settings.New(db, llmClient, sdClient, cfg, logSvc, srvClient)
	dataDir := filepath.Dir(cfg.DBPath)
	appSessions := session.New(db, dataDir, mcp.NoopEmitter{})
	gen := generation.New(
		db, llmClient, sdClient, cfg,
		dataDir,
		mcp.NoopEmitter{}, kidsMgr, mcp.AgentSessions{Svc: appSessions}, settingsSvc, nil, logSvc,
	)
	go gen.ResetReferences()

	outDir := env("OUT")
	if outDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("home dir: %v", err)
		}
		outDir = filepath.Join(home, "imagecaster-mcp-out")
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		log.Fatalf("create out dir: %v", err)
	}

	srv := mcp.NewServer(mcp.Deps{
		DB:       db,
		Gen:      gen,
		SD:       sdClient,
		LLM:      llmClient,
		Settings: settingsSvc,
		Kids:     kidsMgr,
		Cfg:      cfg,
		OutDir:   outDir,
		Stylist:  gen,
		Editor:   mcp.EditorAdapter{Gen: gen, DB: db},
		Sessions: appSessions,

		NoImageBytes: env("NO_IMAGE_BYTES") == "1",
	})
	mcp.RegisterTools(srv)

	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		log.Fatalf("mcp serve: %v", err)
	}
}
