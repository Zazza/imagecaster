package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	_ "image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Zazza/imagecaster/internal/adjust"
	"github.com/Zazza/imagecaster/internal/batch"
	"github.com/Zazza/imagecaster/internal/clipboard"
	"github.com/Zazza/imagecaster/internal/config"
	"github.com/Zazza/imagecaster/internal/filebrowser"
	"github.com/Zazza/imagecaster/internal/finish"
	"github.com/Zazza/imagecaster/internal/generation"
	"github.com/Zazza/imagecaster/internal/imgcodec"
	"github.com/Zazza/imagecaster/internal/imgformat"
	"github.com/Zazza/imagecaster/internal/importexport"
	"github.com/Zazza/imagecaster/internal/kids"
	"github.com/Zazza/imagecaster/internal/llm"
	"github.com/Zazza/imagecaster/internal/logger"
	"github.com/Zazza/imagecaster/internal/postpreview"
	"github.com/Zazza/imagecaster/internal/preset"
	"github.com/Zazza/imagecaster/internal/queue"
	"github.com/Zazza/imagecaster/internal/sd"
	"github.com/Zazza/imagecaster/internal/serverclient"
	"github.com/Zazza/imagecaster/internal/session"
	"github.com/Zazza/imagecaster/internal/settings"
)

type appEmitter struct {
	ctx *context.Context
}

func (e *appEmitter) Emit(event string, data ...any) {
	if *e.ctx != nil {
		runtime.EventsEmit(*e.ctx, event, data...)
	}
}

type App struct {
	ctx          context.Context
	presets      *preset.DB
	llm          llm.Service
	sd           sd.Service
	log          *logger.Logger
	config       *config.Config
	dataDir      string
	serverClient *serverclient.Client

	kidsMgr     *kids.Manager
	sessions    *session.Service
	settingsSvc *settings.Service
	ieSvc       *importexport.Service
	gen         *generation.Service
	queueSvc    *queue.Service
	batch       *batch.Store
	preview     *postpreview.Service
	emitter     appEmitter
}

func (a *App) saveWithDialog(defaultFilename string, data []byte) (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select folder to save",
	})
	if err != nil || dir == "" {
		return "", err
	}
	path := filepath.Join(dir, defaultFilename)
	return path, os.WriteFile(path, data, 0o644)
}

func NewApp(presets *preset.DB, llmClient llm.Service, sdClient sd.Service, srvClient *serverclient.Client, cfg *config.Config) *App {
	a := &App{
		presets:      presets,
		llm:          llmClient,
		sd:           sdClient,
		log:          logger.New(nil),
		config:       cfg,
		dataDir:      filepath.Dir(cfg.DBPath),
		kidsMgr:      kids.NewManager(presets),
		serverClient: srvClient,
	}
	a.emitter = appEmitter{ctx: &a.ctx}
	a.sessions = session.New(presets, a.dataDir, &a.emitter)
	a.settingsSvc = settings.New(presets, llmClient, sdClient, cfg, a.log, srvClient)
	a.ieSvc = importexport.New(presets, a.log)
	a.gen = generation.New(
		presets, llmClient, sdClient, cfg,
		a.dataDir,
		&a.emitter, a.kidsMgr, a.sessions, a.log,
	)
	queueStore := queue.NewStore(presets.DB())
	scoper, _ := sdClient.(sd.QueueScoper)
	queueProc := queue.NewProcessor(a.gen, scoper, queueStore, a.dataDir, &a.emitter)
	a.queueSvc = queue.NewService(queueStore, queueProc, &a.emitter)
	a.queueSvc.Handle(queue.JobStylistFrame, stylistFrameHandler(a.sessions, cfg))
	a.queueSvc.Handle(queue.JobRetouch, retouchHandler(a.sessions, cfg))
	a.queueSvc.Handle(queue.JobPostSave, postSaveHandler(a.ieSvc, cfg))
	a.queueSvc.Handle(queue.JobUpscale4K, post4KHandler(cfg))
	if scoper == nil {
		a.queueSvc.SetInterruptFn(func() { _ = a.gen.InterruptGeneration() })
	} else {
		a.queueSvc.SetJobCleanup(func(id int64) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := scoper.CleanupQueueJob(ctx, id); err != nil {
					a.log.Warn("queue: clean up server jobs of %d: %s", id, err)
				}
			}()
		})
	}
	a.batch = batch.New(filepath.Join(a.dataDir, "batch"))
	a.preview = postpreview.New(postpreview.DefaultCacheSize)
	return a
}

const (
	externalSessionPoll = 2 * time.Second
	queueStopTimeout    = 5 * time.Second
)

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.log.SetContext(ctx)
	a.log.InstallBridge()
	if err := a.queueSvc.ClearCompleted(); err != nil {
		a.log.Warn("queue: clear completed: %s", err)
	}
	a.queueSvc.Start(ctx)
	go a.sessions.WatchExternal(ctx, externalSessionPoll)
	if err := a.batch.Reset(); err != nil {
		a.log.Warn("batch: reset: %s", err)
	}
	a.restoreWindowLayout()
}

func (a *App) shutdown(ctx context.Context) {
	a.queueSvc.Stop(queueStopTimeout)
}

func (a *App) restoreWindowLayout() {
	if v, err := a.presets.GetSetting("window_maximised"); err == nil && v == "true" {
		runtime.WindowMaximise(a.ctx)
		return
	}
	if w, err := a.presets.GetSetting("window_width"); err == nil && w != "" {
		if h, err2 := a.presets.GetSetting("window_height"); err2 == nil && h != "" {
			wi, _ := strconv.Atoi(w)
			hi, _ := strconv.Atoi(h)
			if wi > 0 && hi > 0 {
				runtime.WindowSetSize(a.ctx, wi, hi)
			}
		}
	}
	if x, err := a.presets.GetSetting("window_x"); err == nil && x != "" {
		if y, err2 := a.presets.GetSetting("window_y"); err2 == nil && y != "" {
			xi, _ := strconv.Atoi(x)
			yi, _ := strconv.Atoi(y)
			runtime.WindowSetPosition(a.ctx, xi, yi)
		}
	}
}

func (a *App) SaveWindowLayout(footerHeight int) error {
	maximised := runtime.WindowIsMaximised(a.ctx)
	if maximised {
		a.presets.SetSetting("window_maximised", "true")
	} else {
		a.presets.SetSetting("window_maximised", "false")
		w, h := runtime.WindowGetSize(a.ctx)
		a.presets.SetSetting("window_width", fmt.Sprintf("%d", w))
		a.presets.SetSetting("window_height", fmt.Sprintf("%d", h))
		x, y := runtime.WindowGetPosition(a.ctx)
		a.presets.SetSetting("window_x", fmt.Sprintf("%d", x))
		a.presets.SetSetting("window_y", fmt.Sprintf("%d", y))
	}
	if footerHeight > 0 {
		a.presets.SetSetting("footer_height", fmt.Sprintf("%d", footerHeight))
	}
	return nil
}

func (a *App) GetFooterHeight() int {
	if v, err := a.presets.GetSetting("footer_height"); err == nil && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 40
}

func (a *App) Version() string {
	return version
}

// --- Kids Mode ---

type KidsCategoryInfo = kids.CategoryInfo

func (a *App) IsKidsModeActive() bool {
	return a.kidsMgr.IsActive()
}

func (a *App) SetKidsMode(enabled bool, pin string) error {
	return a.kidsMgr.SetKidsMode(enabled, pin)
}

func (a *App) GetKidsCategories() ([]KidsCategoryInfo, error) {
	return a.kidsMgr.GetCategories()
}

func (a *App) SetKidsCategory(name string, enabled bool, pin string) error {
	return a.kidsMgr.SetCategory(name, enabled, pin)
}

// --- Presets ---

func (a *App) ListPresets() ([]preset.Preset, error) {
	presets, err := a.presets.List()
	if err != nil {
		return nil, err
	}
	if presets == nil {
		presets = []preset.Preset{}
	}
	return presets, nil
}

func (a *App) CreatePreset(p preset.Preset) (*preset.Preset, error) {
	if err := a.presets.Create(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (a *App) UpdatePreset(p preset.Preset) (*preset.Preset, error) {
	if err := a.presets.Update(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (a *App) DeletePreset(id int64) error {
	return a.presets.Delete(id)
}

func (a *App) ListPresetTypes() ([]preset.PresetType, error) {
	items, err := a.presets.ListPresetTypes()
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []preset.PresetType{}
	}
	return items, nil
}

// --- Generation (delegates) ---

type SDProgressEvent = generation.SDProgressEvent
type GenerateSDPromptParams = generation.GenerateSDPromptParams
type GenerateSDPromptResult = generation.GenerateSDPromptResult
type GenerateImageParams = generation.GenerateImageParams
type GenerateImageResult = generation.GenerateImageResult
type AnalyzePrompts = generation.AnalyzePrompts
type UpscaleImageParams = generation.UpscaleImageParams
type GenerateCompoundImageParams = generation.GenerateCompoundImageParams
type GenerateFromImageParams = generation.GenerateFromImageParams
type ApplyLookParams = generation.ApplyLookParams
type EditImageParams = generation.EditImageParams
type ObjectEditParams = generation.ObjectEditParams
type RepairParams = generation.RepairParams
type PresetOverrides = generation.PresetOverrides

func (a *App) GenerateSDPrompt(params GenerateSDPromptParams) (*GenerateSDPromptResult, error) {
	return a.gen.GenerateSDPrompt(params)
}

func (a *App) GetDefaultPromptInstruction() string {
	return a.gen.GetDefaultPromptInstruction()
}

func (a *App) GetDefaultPromptInstructionProse() string {
	return a.gen.GetDefaultPromptInstructionProse()
}

func (a *App) AnalyzeImage(imageBase64, mode string) (string, error) {
	return a.gen.AnalyzeImage(imageBase64, mode)
}

func (a *App) GetDefaultEditGarmentPrompt() string {
	return config.DefaultEditGarmentPrompt
}

func (a *App) GetDefaultAnalyzePrompts() *AnalyzePrompts {
	return a.gen.GetDefaultAnalyzePrompts()
}

func (a *App) GenerateImage(params GenerateImageParams) (*GenerateImageResult, error) {
	return a.gen.GenerateImage(params)
}

func (a *App) UpscaleImage(params UpscaleImageParams) (*GenerateImageResult, error) {
	return a.gen.UpscaleImage(params)
}

func (a *App) InterruptGeneration() error {
	return a.gen.InterruptGeneration()
}

func (a *App) GenerateCompoundImage(params GenerateCompoundImageParams) (*GenerateImageResult, error) {
	return a.gen.GenerateCompoundImage(params)
}

func (a *App) EditImage(params EditImageParams) (*GenerateImageResult, error) {
	return a.gen.EditImage(params)
}

func (a *App) EditObject(params ObjectEditParams) (*GenerateImageResult, error) {
	return a.gen.EditObject(params)
}

func (a *App) RepairImage(params RepairParams) (*GenerateImageResult, error) {
	return a.gen.RepairImage(params)
}

func (a *App) ApplyLookDirect(imageBase64, text string, strength float64, seed *int64) (*GenerateImageResult, error) {
	return a.gen.ApplyLookDirect(imageBase64, text, strength, seed)
}

// --- File/Clipboard (Wails runtime) ---

func (a *App) ReadImageFile() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if len(data) > generation.MaxImageBytes {
		return "", config.ImageTooLarge("image")
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) ReadClipboardImage() (string, error) {
	return clipboard.ReadImage()
}

func (a *App) SaveImage(base64Data, defaultName string) (string, error) {
	if base64Data == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(imgcodec.StripDataURL(base64Data))
	if err != nil {
		return "", err
	}
	format, ok := imgformat.Detect(data)
	if !ok {
		return "", errors.New("unsupported image format")
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: saveImageName(defaultName, format),
		Filters:         []runtime.FileFilter{saveImageFilter(format)},
	})
	if err != nil || path == "" {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}

	a.log.UserAction("Image saved: %s", path)
	return path, nil
}

var imageNameExts = []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}

func saveImageName(defaultName string, format imgformat.Format) string {
	name := strings.TrimSpace(defaultName)
	if name == "" {
		name = "imagecaster-image"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if slices.Contains(imageNameExts, ext) {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name + format.Ext
}

func saveImageFilter(format imgformat.Format) runtime.FileFilter {
	patterns := make([]string, 0, 2)
	for _, e := range format.Exts() {
		patterns = append(patterns, "*"+e)
	}
	return runtime.FileFilter{DisplayName: format.Name + " Image", Pattern: strings.Join(patterns, ";")}
}

// --- SD Info ---

func (a *App) GetSDModels() ([]sd.SDModel, error) {
	return a.sd.GetModels()
}

func (a *App) GetSDSamplers() ([]sd.Sampler, error) {
	return a.sd.GetSamplers()
}

func (a *App) GetSDSchedulers() ([]sd.Scheduler, error) {
	return a.sd.GetSchedulers()
}

func (a *App) GetSDUpscalers() ([]sd.Upscaler, error) {
	return a.sd.GetUpscalers()
}

func (a *App) GetSDVAEs() ([]sd.VAE, error) {
	return a.sd.GetVAEs()
}

func (a *App) GetSDLoRAs() ([]sd.LoRA, error) {
	return a.sd.GetLoRAs()
}

// --- LLM Info ---

// --- Settings ---

func (a *App) GetSettings() (map[string]string, error) {
	return a.settingsSvc.GetSettings()
}

func (a *App) UpdateSettings(data map[string]string) error {
	return a.settingsSvc.UpdateSettings(data)
}

// --- Saved Descriptions ---

func (a *App) ListDescriptions() ([]preset.SavedDescription, error) {
	items, err := a.presets.ListDescriptions()
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []preset.SavedDescription{}
	}
	return items, nil
}

func (a *App) CreateDescriptionFull(s preset.SavedDescription) (*preset.SavedDescription, error) {
	s.Text = strings.TrimSpace(s.Text)
	if s.Text == "" {
		return nil, nil
	}
	return a.presets.CreateDescriptionFull(&s)
}

func (a *App) UpdateDescription(s preset.SavedDescription) error {
	return a.presets.UpdateDescription(&s)
}

func (a *App) DeleteDescription(id int64) error {
	return a.presets.DeleteDescription(id)
}

// --- Saved Prompts ---

// --- Preset Export/Import ---

type PresetExportFile = importexport.ExportFile
type PresetData = importexport.PresetData
type ImportPreview = importexport.ImportPreview

func (a *App) PreparePresetsExport(ids []int64) (string, error) {
	presets, err := a.ieSvc.PrepareExportData(ids)
	if err != nil {
		return "", err
	}
	jsonBytes, err := a.ieSvc.BuildExportFile(presets)
	if err != nil {
		return "", err
	}
	return a.saveWithDialog("imagecaster-presets.json", jsonBytes)
}

func (a *App) OpenImportFile() (*ImportPreview, error) {
	paths, err := a.openJSONFiles()
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return a.ieSvc.PreviewImportFiles(paths)
}

func (a *App) openJSONFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
}

func (a *App) ImportPresets(items []PresetData) ([]preset.Preset, error) {
	return a.ieSvc.ImportItems(items)
}

// --- Compound Preset Export/Import ---

type CompoundExportData = importexport.CompoundExportData
type CompoundImportPreview = importexport.CompoundImportPreview

func (a *App) PrepareCompoundPresetsExport(ids []int64) (string, error) {
	pipelines, err := a.ieSvc.PrepareCompoundExportData(ids)
	if err != nil {
		return "", err
	}
	jsonBytes, err := a.ieSvc.BuildCompoundExportFile(pipelines)
	if err != nil {
		return "", err
	}
	return a.saveWithDialog("imagecaster-pipelines.json", jsonBytes)
}

func (a *App) OpenImportCompoundFile() (*CompoundImportPreview, error) {
	paths, err := a.openJSONFiles()
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return a.ieSvc.PreviewCompoundImportFiles(paths)
}

func (a *App) ImportCompoundPresets(items []CompoundExportData) ([]preset.CompoundPreset, error) {
	return a.ieSvc.ImportCompoundItems(items)
}

// --- Compound Presets ---

func (a *App) ListCompoundPresets() ([]preset.CompoundPreset, error) {
	return a.presets.ListCompoundPresetsFull()
}

func (a *App) CreateCompoundPreset(cp preset.CompoundPreset) (*preset.CompoundPreset, error) {
	if cp.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(cp.Steps) == 0 {
		return nil, fmt.Errorf("at least one step is required")
	}
	if err := a.presets.CreateCompoundPreset(&cp); err != nil {
		return nil, err
	}
	return a.presets.GetCompoundPresetFull(cp.ID)
}

func (a *App) UpdateCompoundPreset(cp preset.CompoundPreset) (*preset.CompoundPreset, error) {
	if cp.ID <= 0 {
		return nil, fmt.Errorf("id is required")
	}
	if cp.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(cp.Steps) == 0 {
		return nil, fmt.Errorf("at least one step is required")
	}
	if err := a.presets.UpdateCompoundPreset(&cp); err != nil {
		return nil, err
	}
	return a.presets.GetCompoundPresetFull(cp.ID)
}

func (a *App) DeleteCompoundPreset(id int64) error {
	return a.presets.DeleteCompoundPreset(id)
}

// --- Export Image ---

type ExportImageParams = importexport.ExportImageParams

func (a *App) ExportImage(params ExportImageParams) (string, error) {
	processed, err := a.ieSvc.ProcessExportImage(params)
	if err != nil {
		return "", err
	}
	return a.saveProcessedImage(processed, params.OutputDir)
}

func (a *App) saveProcessedImage(processed *importexport.ProcessedImage, outputDir string) (string, error) {
	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return "", err
		}
		path := filepath.Join(outputDir, filepath.Base(processed.Filename))
		return importexport.WriteImageToPath(processed, path)
	}

	ext := "." + processed.Format
	name := strings.ToUpper(processed.Format) + " Image"
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: filepath.Base(processed.Filename),
		Filters:         []runtime.FileFilter{{DisplayName: name, Pattern: "*" + ext}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ext) {
		path += ext
	}
	return path, os.WriteFile(path, processed.Data, 0o644)
}

// --- Post-processing ---

type AdjustParams = adjust.Params

func (a *App) PreviewAdjust(imageBase64 string, p AdjustParams, maxW int) (string, error) {
	return a.preview.Adjust(imageBase64, p, maxW)
}

type FinishParams = finish.Params

func (a *App) PreviewPost(imageBase64 string, p AdjustParams, f FinishParams, maxW int) (string, error) {
	return a.preview.Post(imageBase64, p, f, maxW)
}

func (a *App) AutoAdjust(imageBase64 string) (adjust.AutoStats, error) {
	return a.preview.Auto(imageBase64)
}

func (a *App) ListExportPresets() ([]preset.ExportPreset, error) {
	return a.ieSvc.ListExportPresets()
}

func (a *App) SaveExportPreset(ep preset.ExportPreset) (*preset.ExportPreset, error) {
	return a.ieSvc.SaveExportPreset(ep)
}

func (a *App) DeleteExportPreset(id int64) error {
	return a.ieSvc.DeleteExportPreset(id)
}

func (a *App) ListPostRecipes() ([]preset.PostRecipe, error) {
	return a.presets.ListPostRecipes()
}

func (a *App) SavePostRecipe(r preset.PostRecipe) (*preset.PostRecipe, error) {
	return a.presets.SavePostRecipe(r)
}

func (a *App) DeletePostRecipe(id int64) error {
	return a.presets.DeletePostRecipe(id)
}

type CompiledRecipe = generation.CompiledRecipe

func (a *App) CompilePostStyle(text string) (*CompiledRecipe, error) {
	return a.gen.CompilePostStyle(text)
}

func (a *App) DescribeForStyle(imageBase64 string) (string, error) {
	return a.gen.DescribeForStyle(imageBase64)
}

// --- File Browser ---

type FileEntry = filebrowser.FileEntry

func (a *App) BrowseDirectory(dirPath string) ([]FileEntry, error) {
	return filebrowser.BrowseDirectory(dirPath)
}

func (a *App) ReadFileAsBase64(filePath string) (string, error) {
	return filebrowser.ReadFileAsBase64(filePath)
}

func (a *App) ReadThumbnail(filePath string) (string, error) {
	return filebrowser.NewThumbCache(filepath.Join(a.dataDir, "browser-thumbs")).Read(filePath)
}

func (a *App) BatchSaveImage(imageBase64 string) (string, error) {
	return a.batch.Save(imageBase64)
}

func (a *App) BatchReset() error {
	return a.batch.Reset()
}

func (a *App) SelectBrowserFolder() (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Image Folder",
	})
	if err != nil {
		return "", err
	}
	if path != "" {
		_ = a.presets.SetSetting("file_browser_path", path)
	}
	return path, nil
}

func (a *App) SetLastImage(base64Data string) error {
	if base64Data == "" {
		return nil
	}
	if len(base64Data) > generation.MaxImageBase64Len {
		return config.ImageTooLarge("image")
	}
	itemID := a.sessions.AddToSession(base64Data, nil, "file-browser", nil)
	if itemID != 0 {
		a.emitter.Emit("session:selected", map[string]int64{"id": itemID})
	}
	return nil
}

// --- Session Management ---

func (a *App) CreateSession(name string) (*preset.SessionInfo, error) {
	return a.sessions.CreateSession(name)
}

func (a *App) ListSessions() ([]preset.SessionInfo, error) {
	return a.sessions.ListSessions()
}

func (a *App) SwitchSession(id int64) error {
	return a.sessions.SwitchSession(id)
}

func (a *App) RenameSession(id int64, name string) error {
	return a.sessions.RenameSession(id, name)
}

func (a *App) DeleteSession(id int64) error {
	return a.sessions.DeleteSession(id)
}

func (a *App) ListHistory(f preset.HistoryFilter) (*preset.HistoryPage, error) {
	return a.sessions.ListHistory(f)
}

func (a *App) DeleteHistoryItems(ids []int64) (int, error) {
	return a.sessions.DeleteSessionItems(ids)
}

func (a *App) MoveSessionItems(ids []int64, sessionID int64) (int, error) {
	return a.sessions.MoveSessionItems(ids, sessionID)
}

func (a *App) OpenInHistory(id int64) error {
	return a.sessions.OpenInHistory(id)
}

func (a *App) GetSessionItems() ([]preset.SessionItem, error) {
	return a.sessions.GetSessionItems()
}

func (a *App) GetActiveSessionItem() (*preset.SessionItem, error) {
	return a.sessions.GetActiveSessionItem()
}

func (a *App) SetActiveSessionItem(id int64) error {
	return a.sessions.SetActiveSessionItem(id)
}

func (a *App) DeleteSessionItem(id int64) error {
	return a.sessions.DeleteSessionItem(id)
}

func (a *App) ClearSession() error {
	return a.sessions.ClearSession()
}

func (a *App) GetSessionImage(id int64) (string, error) {
	return a.sessions.GetSessionImage(id)
}

func (a *App) GetSessionThumb(id int64) (string, error) {
	return a.sessions.GetSessionThumb(id)
}

// --- Server Mode ---

type DiscoveredServer = serverclient.DiscoveredServer
type ServerStatus = serverclient.ServerStatus
type ServerModelInfo = serverclient.ModelInfo
type ServerLLMModelInfo = serverclient.LLMModelInfo
type ModelCatalog = serverclient.Catalog

func (a *App) DiscoverServers() ([]DiscoveredServer, error) {
	return a.settingsSvc.DiscoverServers(a.ctx)
}

func (a *App) GetServerStatus() (*ServerStatus, error) {
	return a.settingsSvc.GetServerStatus()
}

func (a *App) StartServerProcess(name string) error {
	return a.settingsSvc.StartServerProcess(name)
}

func (a *App) StopServerProcess(name string) error {
	return a.settingsSvc.StopServerProcess(name)
}

func (a *App) RestartServerProcess(name string) error {
	return a.settingsSvc.RestartServerProcess(name)
}

func (a *App) GetServerModels(modelType string) ([]ServerModelInfo, error) {
	return a.settingsSvc.GetServerModels(modelType)
}

func (a *App) GetServerLLMModels() ([]ServerLLMModelInfo, error) {
	return a.settingsSvc.GetServerLLMModels()
}

func (a *App) DownloadServerModel(modelType, url, filename string) error {
	return a.settingsSvc.DownloadServerModel(modelType, url, filename)
}

func (a *App) CheckServer() settings.ServerCheck {
	return a.settingsSvc.CheckServer()
}

func (a *App) DownloadServerModelStream(jobID, modelType, url, filename string) error {
	return a.settingsSvc.DownloadServerModelStream(modelType, url, filename, func(data string) {
		runtime.EventsEmit(a.ctx, "server:download:progress:"+jobID, data)
	})
}

func (a *App) PullServerLLMModelStream(jobID, name string) error {
	return a.settingsSvc.PullServerLLMModelStream(name, func(data string) {
		runtime.EventsEmit(a.ctx, "server:download:progress:"+jobID, data)
	})
}

func (a *App) DeleteServerModel(modelType, filename string) error {
	return a.settingsSvc.DeleteServerModel(modelType, filename)
}

func (a *App) PullServerLLMModel(name string) error {
	return a.settingsSvc.PullServerLLMModel(name)
}

func (a *App) GetServerProcessLogs(name string, lines int) ([]string, error) {
	return a.settingsSvc.GetServerProcessLogs(name, lines)
}

func (a *App) GetLLMSetup() (*settings.LLMSetup, error) {
	return a.settingsSvc.GetLLMSetup()
}

func (a *App) SaveLLMChoice(generate, vision string) error {
	return a.settingsSvc.SaveLLMChoice(generate, vision)
}

func (a *App) GetLogEntries() []logger.Entry {
	return a.log.Entries()
}

func (a *App) DeleteServerLLMModel(name string) error {
	return a.settingsSvc.DeleteServerLLMModel(name)
}

func (a *App) GetModelCatalog() (*ModelCatalog, error) {
	return a.settingsSvc.GetModelCatalog()
}

func (a *App) GetDefaultDownloadPlan() (*settings.DownloadPlan, error) {
	return a.settingsSvc.GetDefaultDownloadPlan()
}

func (a *App) SyncPresets() (*settings.PresetSync, error) {
	return a.settingsSvc.SyncPresets()
}

func (a *App) GetModelPresets() (map[string][]string, error) {
	return a.settingsSvc.ModelPresets()
}

func (a *App) InstallPresetDeps(presetID int64) error {
	return a.settingsSvc.InstallPresetDeps(presetID)
}

// --- Resolutions ---

func (a *App) ListResolutions() ([]preset.Resolution, error) {
	items, err := a.presets.ListResolutions()
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []preset.Resolution{}
	}
	return items, nil
}

func (a *App) CreateResolution(r preset.Resolution) (*preset.Resolution, error) {
	if strings.TrimSpace(r.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if r.Width < 64 || r.Width > 4096 || r.Height < 64 || r.Height > 4096 {
		return nil, fmt.Errorf("width and height must be between 64 and 4096")
	}
	if err := a.presets.CreateResolution(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (a *App) UpdateResolution(r preset.Resolution) (*preset.Resolution, error) {
	if r.ID <= 0 {
		return nil, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if r.Width < 64 || r.Width > 4096 || r.Height < 64 || r.Height > 4096 {
		return nil, fmt.Errorf("width and height must be between 64 and 4096")
	}
	if err := a.presets.UpdateResolution(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (a *App) DeleteResolution(id int64) error {
	return a.presets.DeleteResolution(id)
}

// --- Hires Profiles ---

func (a *App) ListHiresProfiles() ([]preset.HiresProfile, error) {
	items, err := a.presets.ListHiresProfiles()
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []preset.HiresProfile{}
	}
	return items, nil
}

func (a *App) CreateHiresProfile(h preset.HiresProfile) (*preset.HiresProfile, error) {
	if strings.TrimSpace(h.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if h.Upscale < 1.0 || h.Upscale > 4.0 {
		return nil, fmt.Errorf("upscale must be between 1.0 and 4.0")
	}
	if h.DenoisingStrength < 0.0 || h.DenoisingStrength > 1.0 {
		return nil, fmt.Errorf("denoising_strength must be between 0.0 and 1.0")
	}
	if err := a.presets.CreateHiresProfile(&h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (a *App) UpdateHiresProfile(h preset.HiresProfile) (*preset.HiresProfile, error) {
	if h.ID <= 0 {
		return nil, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(h.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if h.Upscale < 1.0 || h.Upscale > 4.0 {
		return nil, fmt.Errorf("upscale must be between 1.0 and 4.0")
	}
	if h.DenoisingStrength < 0.0 || h.DenoisingStrength > 1.0 {
		return nil, fmt.Errorf("denoising_strength must be between 0.0 and 1.0")
	}
	if err := a.presets.UpdateHiresProfile(&h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (a *App) DeleteHiresProfile(id int64) error {
	return a.presets.DeleteHiresProfile(id)
}

// --- Queue ---

type QueueJob = queue.Job

func (a *App) EnqueueTxt2Img(params GenerateImageParams) (int64, error) {
	return a.queueSvc.Enqueue(queue.JobTxt2Img, params, "generate")
}

func (a *App) EnqueueFromImage(params GenerateFromImageParams) (int64, error) {
	return a.queueSvc.Enqueue(queue.JobFromImage, params, "from-image")
}

func (a *App) EnqueueLook(params ApplyLookParams) (int64, error) {
	return a.queueSvc.Enqueue(queue.JobLook, params, "look")
}

func (a *App) EnqueueEdit(params EditImageParams) (int64, error) {
	return a.queueSvc.Enqueue(queue.JobEdit, params, "edit")
}

func (a *App) EnqueueCompound(params GenerateCompoundImageParams) (int64, error) {
	return a.queueSvc.Enqueue(queue.JobCompound, params, "compound")
}

func (a *App) GetQueueActiveCount() (int, error) {
	return a.queueSvc.ActiveCount()
}

func (a *App) GetQueue() ([]*QueueJob, error) {
	jobs, err := a.queueSvc.GetQueue()
	if err != nil {
		return nil, err
	}
	if jobs == nil {
		jobs = []*QueueJob{}
	}
	return jobs, nil
}

func (a *App) RemoveQueueJob(id int64) error {
	return a.queueSvc.RemoveJob(id)
}

func (a *App) RetryQueueJob(id int64) error {
	return a.queueSvc.RetryJob(id)
}

func (a *App) CancelQueueJob(id int64) error {
	return a.queueSvc.CancelJob(id)
}

func (a *App) PauseQueue() {
	a.queueSvc.PauseQueue()
}

func (a *App) ResumeQueue() {
	a.queueSvc.ResumeQueue()
}

func (a *App) CancelQueue() error {
	return a.queueSvc.CancelQueue()
}

func (a *App) IsQueuePaused() bool {
	return a.queueSvc.IsPaused()
}

func (a *App) ResumePausedQueueJobs() (int, error) {
	return a.queueSvc.ResumePausedJobs()
}
