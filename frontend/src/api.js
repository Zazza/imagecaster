import {
  ListPresets, ListPresetsByType, CreatePreset, UpdatePreset, DeletePreset,
  GenerateSDPrompt, GenerateImage, UpscaleImage, SaveImage, FastSaveImage,
  GetSDModels, GetSDSamplers, GetSDSchedulers, GetSDUpscalers, GetSDVAEs, GetLLMModels,
  GetSettings, UpdateSettings, CheckServices,
  ListDescriptions, CreateDescription, CreateDescriptionFull, UpdateDescription, DeleteDescription,
  SetKidsMode, IsKidsModeActive, GetKidsCategories, SetKidsCategory,
  ExportPresets, PreparePresetsExport, OpenImportFile, ImportPresets,
  ExportCompoundPresets, PrepareCompoundPresetsExport, OpenImportCompoundFile, ImportCompoundPresets,
  AnalyzeImage, ReadImageFile, ReadClipboardImage,
  ListPresetTypes, GetPresetType, CreatePresetType, UpdatePresetType, DeletePresetType,
  GetAllTags, GetSDLoRAs, ValidateImportModels,
  GetDefaultPromptInstruction, GetDefaultPromptInstructionProse, GetDefaultAnalyzePrompts, GetDefaultEditGarmentPrompt,
  ListCompoundPresets, GetCompoundPreset, CreateCompoundPreset, UpdateCompoundPreset, DeleteCompoundPreset,
  GenerateCompoundImage,
  EditImage, EditObject, RepairImage,
  ClearLastImage,
  ListExportPresets, SaveExportPreset, DeleteExportPreset, ExportImage,
  ListPostRecipes, SavePostRecipe, DeletePostRecipe,
  BrowseDirectory, ReadFileAsBase64, ReadThumbnail, SelectBrowserFolder, SetLastImage, BatchSaveImage, BatchReset,
  CreateSession, ListSessions, SwitchSession, RenameSession, DeleteSession,
  GetSessionItems, GetActiveSessionItem, SetActiveSessionItem, DeleteSessionItem,
  ClearSession, GetSessionImage, GetSessionThumb,
  InterruptGeneration,
  DiscoverServers, GetServerStatus, GetServerModels, GetServerLLMModels,
  DownloadServerModel, DeleteServerModel, PullServerLLMModel, DeleteServerLLMModel,
  GetModelCatalog,
  InstallPresetDeps,
  StartServerProcess, StopServerProcess, RestartServerProcess,
  Version,
  ListResolutions, CreateResolution, UpdateResolution, DeleteResolution,
  ListHiresProfiles, CreateHiresProfile, UpdateHiresProfile, DeleteHiresProfile,
  PreviewAdjust, AutoAdjust, CompilePostStyle, DescribeForStyle, ApplyLookDirect,
  EnqueueTxt2Img, EnqueueFromImage, EnqueueLook, EnqueueCompound,
  GetQueue, RemoveQueueJob, CancelQueueJob, PauseQueue, ResumeQueue, CancelQueue,
  IsQueuePaused, ClearCompletedQueueJobs, ResumePausedQueueJobs,
  SaveWindowLayout, GetFooterHeight,
} from './wailsjs/go/main/App.js'
import * as AppBindings from './wailsjs/go/main/App.js'
import { parseProgress } from './downloadProgress.js'

export const api = {
  listPresets: () => ListPresets(),
  listPresetsByType: (type) => ListPresetsByType(type),
  createPreset: (data) => CreatePreset(data),
  updatePreset: (id, data) => UpdatePreset({ ...data, id }),
  deletePreset: (id) => DeletePreset(id),
  generateSdPrompt: (params) => GenerateSDPrompt(params),
  generateImage: (presetId, extraPrompt, extraNegativePrompt, resolutionId, hiresProfileId, overrides, seed) =>
    GenerateImage({ preset_id: presetId, extra_prompt: extraPrompt, extra_negative_prompt: extraNegativePrompt, resolution_id: resolutionId || null, hires_profile_id: hiresProfileId || null, overrides: overrides || null, seed: seed ?? null }),
  upscaleImage: (imageBase64, genInfo, presetId) =>
    UpscaleImage({ image_base64: imageBase64, gen_info: typeof genInfo === 'string' ? genInfo : JSON.stringify(genInfo || {}), preset_id: presetId || 0 }),
  saveImage: (base64Data, defaultName) => SaveImage(base64Data, defaultName),
  fastSaveImage: (base64Data, filename, format) => FastSaveImage(base64Data, filename, format),
  getModels: () => GetSDModels(),
  getSamplers: () => GetSDSamplers(),
  getSchedulers: () => GetSDSchedulers(),
  getUpscalers: () => GetSDUpscalers(),
  getVAEs: () => GetSDVAEs(),
  getLLMModels: () => GetLLMModels(),
  getSettings: () => GetSettings(),
  updateSettings: (data) => UpdateSettings(data),
  checkServices: () => CheckServices(),
  listDescriptions: () => ListDescriptions(),
  createDescription: (text) => CreateDescription(text),
  createDescriptionFull: (data) => CreateDescriptionFull(data),
  updateDescription: (data) => UpdateDescription(data),
  deleteDescription: (id) => DeleteDescription(id),
  setKidsMode: (enabled, pin) => SetKidsMode(enabled, pin),
  isKidsModeActive: () => IsKidsModeActive(),
  getKidsCategories: () => GetKidsCategories(),
  setKidsCategory: (name, enabled, pin) => SetKidsCategory(name, enabled, pin),
  exportPresets: (ids) => ExportPresets(ids),
  preparePresetsExport: (ids) => PreparePresetsExport(ids),
  openImportFile: () => OpenImportFile(),
  importPresets: (items) => ImportPresets(items),
  analyzeImage: (imageBase64, mode) => AnalyzeImage(imageBase64, mode),
  readImageFile: () => ReadImageFile(),
  readClipboardImage: () => ReadClipboardImage(),
  listPresetTypes: () => ListPresetTypes(),
  getPresetType: (id) => GetPresetType(id),
  createPresetType: (data) => CreatePresetType(data),
  updatePresetType: (data) => UpdatePresetType(data),
  deletePresetType: (id) => DeletePresetType(id),
  getAllTags: () => GetAllTags(),
  getLoRAs: () => GetSDLoRAs(),
  validateImportModels: (items) => ValidateImportModels(items),
  exportCompoundPresets: (ids) => ExportCompoundPresets(ids),
  prepareCompoundPresetsExport: (ids) => PrepareCompoundPresetsExport(ids),
  openImportCompoundFile: () => OpenImportCompoundFile(),
  importCompoundPresets: (items) => ImportCompoundPresets(items),
  getDefaultPromptInstruction: () => GetDefaultPromptInstruction(),
  getDefaultPromptInstructionProse: () => GetDefaultPromptInstructionProse(),
  getDefaultEditGarmentPrompt: () => GetDefaultEditGarmentPrompt(),
  getDefaultAnalyzePrompts: () => GetDefaultAnalyzePrompts(),
  listCompoundPresets: () => ListCompoundPresets(),
  getCompoundPreset: (id) => GetCompoundPreset(id),
  createCompoundPreset: (data) => CreateCompoundPreset(data),
  updateCompoundPreset: (data) => UpdateCompoundPreset(data),
  deleteCompoundPreset: (id) => DeleteCompoundPreset(id),
  generateCompoundImage: (params) => GenerateCompoundImage(params),
  editImage: (params) => EditImage(params),
  editObject: (params) => EditObject(params),
  repairImage: (params) => RepairImage(params),
  clearLastImage: () => ClearLastImage(),
  listExportPresets: () => ListExportPresets(),
  saveExportPreset: (data) => SaveExportPreset(data),
  deleteExportPreset: (id) => DeleteExportPreset(id),
  listPostRecipes: () => ListPostRecipes(),
  savePostRecipe: (data) => SavePostRecipe(data),
  deletePostRecipe: (id) => DeletePostRecipe(id),
  exportImage: (params, adjust) => ExportImage(adjust ? { ...params, adjust } : params),
  previewAdjust: (imageBase64, adjust, maxW) => PreviewAdjust(imageBase64, adjust, maxW),
  previewPost: (imageBase64, adjust, finish, maxW) => (AppBindings.PreviewPost ? AppBindings.PreviewPost(imageBase64, adjust, finish, maxW) : window['go']['main']['App']['PreviewPost'](imageBase64, adjust, finish, maxW)),
  autoAdjust: (imageBase64) => AutoAdjust(imageBase64),
  compilePostStyle: (text) => CompilePostStyle(text),
  describeForStyle: (imageBase64) => DescribeForStyle(imageBase64),
  applyLookDirect: (imageBase64, text, strength, seed) => ApplyLookDirect(imageBase64, text, strength, seed ?? null),
  browseDirectory: (path) => BrowseDirectory(path),
  readFileAsBase64: (path) => ReadFileAsBase64(path),
  readThumbnail: (path) => ReadThumbnail(path),
  selectBrowserFolder: () => SelectBrowserFolder(),
  batchSaveImage: (imageBase64) => BatchSaveImage(imageBase64),
  batchReset: () => BatchReset(),
  setLastImage: (base64) => SetLastImage(base64),

  createSession: (name) => CreateSession(name),
  listSessions: () => ListSessions(),
  switchSession: (id) => SwitchSession(id),
  renameSession: (id, name) => RenameSession(id, name),
  deleteSession: (id) => DeleteSession(id),
  getSessionItems: () => GetSessionItems(),
  getActiveSessionItem: () => GetActiveSessionItem(),
  setActiveSessionItem: (id) => SetActiveSessionItem(id),
  deleteSessionItem: (id) => DeleteSessionItem(id),
  clearSession: () => ClearSession(),
  getSessionImage: (id) => GetSessionImage(id),
  getSessionThumb: (id) => GetSessionThumb(id),
  sessionImageUrl: (id) => `/api/img/${id}.jpg`,
  sessionThumbUrl: (id) => `/api/thumb/${id}.jpg`,
  interruptGeneration: () => InterruptGeneration(),
  version: () => Version(),
  discoverServers: () => DiscoverServers(),
  getServerStatus: () => GetServerStatus(),
  getServerModels: (type) => GetServerModels(type),
  getServerLLMModels: () => GetServerLLMModels(),
  downloadServerModel: (type, url, filename) => DownloadServerModel(type, url, filename),
  downloadServerModelStream: (serverURL, type, downloadURL, filename, onProgress) => {
    return new Promise((resolve, reject) => {
      const url = `${serverURL}/api/server/models/download/stream?type=${encodeURIComponent(type)}&url=${encodeURIComponent(downloadURL)}&filename=${encodeURIComponent(filename)}`
      const es = new EventSource(url)
      es.onmessage = (e) => {
        if (e.data === '[DONE]') {
          es.close()
          resolve()
        } else if (e.data.startsWith('[ERROR]')) {
          es.close()
          reject(new Error(e.data.slice(7).trim()))
        } else {
          const p = parseProgress(e.data)
          onProgress(p.text, p.percent)
        }
      }
      es.onerror = () => {
        es.close()
        reject(new Error('Connection lost'))
      }
    })
  },
  deleteServerModel: (type, filename) => DeleteServerModel(type, filename),
  pullServerLLMModel: (name) => PullServerLLMModel(name),
  pullServerLLMModelStream: (serverURL, name, onProgress) => {
    return new Promise((resolve, reject) => {
      const url = `${serverURL}/api/server/models/llm/pull?name=${encodeURIComponent(name)}`
      const es = new EventSource(url)
      es.onmessage = (e) => {
        if (e.data === '[DONE]') {
          es.close()
          resolve()
        } else if (e.data.startsWith('[ERROR]')) {
          es.close()
          reject(new Error(e.data.slice(7).trim()))
        } else {
          const p = parseProgress(e.data)
          onProgress(p.text, p.percent)
        }
      }
      es.onerror = (e) => {
        es.close()
        reject(new Error('Connection lost'))
      }
    })
  },
  deleteServerLLMModel: (name) => DeleteServerLLMModel(name),
  getModelCatalog: () => GetModelCatalog(),
  syncPresets: () => (AppBindings.SyncPresets ? AppBindings.SyncPresets() : window['go']['main']['App']['SyncPresets']()),
  getModelPresets: () => (AppBindings.GetModelPresets ? AppBindings.GetModelPresets() : window['go']['main']['App']['GetModelPresets']()),
  installPresetDeps: (id) => InstallPresetDeps(id),
  startServerProcess: (name) => StartServerProcess(name),
  stopServerProcess: (name) => StopServerProcess(name),
  restartServerProcess: (name) => RestartServerProcess(name),

  listResolutions: () => ListResolutions(),
  createResolution: (data) => CreateResolution(data),
  updateResolution: (data) => UpdateResolution(data),
  deleteResolution: (id) => DeleteResolution(id),
  listHiresProfiles: () => ListHiresProfiles(),
  createHiresProfile: (data) => CreateHiresProfile(data),
  updateHiresProfile: (data) => UpdateHiresProfile(data),
  deleteHiresProfile: (id) => DeleteHiresProfile(id),

  enqueueTxt2Img: (params) => EnqueueTxt2Img(params),
  enqueueFromImage: (params) => EnqueueFromImage(params),
  enqueueLook: (params) => EnqueueLook(params),
  enqueueEdit: (params) => (AppBindings.EnqueueEdit ? AppBindings.EnqueueEdit(params) : window['go']['main']['App']['EnqueueEdit'](params)),
  enqueueCompound: (params) => EnqueueCompound(params),
  getQueue: () => GetQueue(),
  removeQueueJob: (id) => RemoveQueueJob(id),
  cancelQueueJob: (id) => CancelQueueJob(id),
  pauseQueue: () => PauseQueue(),
  resumeQueue: () => ResumeQueue(),
  cancelQueue: () => CancelQueue(),
  isQueuePaused: () => IsQueuePaused(),
  clearCompletedQueueJobs: () => ClearCompletedQueueJobs(),
  resumePausedQueueJobs: () => ResumePausedQueueJobs(),
  getDefaultDownloadPlan: () => (AppBindings.GetDefaultDownloadPlan ? AppBindings.GetDefaultDownloadPlan() : window['go']['main']['App']['GetDefaultDownloadPlan']()),
  retryQueueJob: (id) => (AppBindings.RetryQueueJob ? AppBindings.RetryQueueJob(id) : window['go']['main']['App']['RetryQueueJob'](id)),
  saveWindowLayout: (footerHeight) => SaveWindowLayout(footerHeight),
  getFooterHeight: () => GetFooterHeight(),
}

