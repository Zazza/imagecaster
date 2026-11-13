import { ref, reactive, computed } from 'vue'
import { api } from './api.js'
import { loadDefaultPlan } from './modelDownloads.js'
import { t } from './i18n/index.js'

export const serverURL = ref('')
export const serverModels = reactive({ sd: [], lora: [], vae: [], text_encoder: [], llm: [] })
export const serverModelsLoading = reactive({ sd: false, lora: false, vae: false, text_encoder: false, llm: false })
export const serverModelsKnown = reactive({})
export const modelActionLoading = reactive({})
export const activeDownload = reactive({ type: '', name: '', progress: '', percent: null, model: '', step: 0, steps: 0 })
export const activeLabel = computed(() => (activeDownload.steps > 1
  ? t('settings.download_file_of', { n: activeDownload.step, total: activeDownload.steps, name: activeDownload.name })
  : ''))
export const modelPresets = ref({})

export function onDownloadProgress(text, percent) {
  activeDownload.progress = text
  activeDownload.percent = percent
}

export function resetActiveDownload() {
  activeDownload.type = ''
  activeDownload.name = ''
  activeDownload.progress = ''
  activeDownload.percent = null
  activeDownload.model = ''
  activeDownload.step = 0
  activeDownload.steps = 0
}

export function hasServerFile(type, filename) {
  return (serverModels[type] || []).some(i => i.name === filename)
}

export async function loadModelPresets() {
  try {
    modelPresets.value = (await api.getModelPresets()) || {}
  } catch (e) {
    console.warn('catalog model presets', e)
    modelPresets.value = {}
  }
}

export async function syncPresets() {
  try {
    const r = await api.syncPresets()
    return r?.added || 0
  } catch (e) {
    console.warn('presets sync', e)
    return 0
  }
}

export async function loadServerModels() {
  for (const type of ['sd', 'lora', 'vae', 'text_encoder']) {
    serverModelsLoading[type] = true
    try {
      serverModels[type] = await api.getServerModels(type) || []
      serverModelsKnown[type] = true
    } catch (e) {
      console.warn('server models', type, e)
      serverModels[type] = []
      serverModelsKnown[type] = false
    }
    finally { serverModelsLoading[type] = false }
  }
  await loadDefaultPlan()
  await loadModelPresets()
  serverModelsLoading.llm = true
  try {
    serverModels.llm = await api.getServerLLMModels() || []
  } catch (e) {
    console.warn('server llm models', e)
    serverModels.llm = []
  }
  finally { serverModelsLoading.llm = false }
}

export async function downloadFile(type, url, filename) {
  if (serverURL.value) {
    await api.downloadServerModelStream(serverURL.value, type, url, filename, onDownloadProgress)
  } else {
    await api.downloadServerModel(type, url, filename)
  }
}

export async function downloadModel(type, url, filename) {
  const key = `dl_${type}`
  modelActionLoading[key] = true
  activeDownload.type = type
  activeDownload.name = filename
  onDownloadProgress('', null)
  try {
    await downloadFile(type, url, filename)
    await loadServerModels()
  } catch (e) {
    alert(t('settings.err_download', { error: String(e) }))
  } finally {
    delete modelActionLoading[key]
    resetActiveDownload()
  }
}

export async function deleteModel(type, filename) {
  if (!confirm(t('settings.confirm_delete_model', { name: filename }))) return
  const key = `del_${type}_${filename}`
  modelActionLoading[key] = true
  try {
    await api.deleteServerModel(type, filename)
    await loadServerModels()
  } catch (e) {
    alert(t('settings.err_delete', { error: String(e) }))
  } finally {
    delete modelActionLoading[key]
  }
}

export async function pullLLM(name) {
  if (!name) return false
  modelActionLoading['pull_llm'] = true
  activeDownload.type = 'llm'
  activeDownload.name = name
  onDownloadProgress('', null)
  try {
    if (serverURL.value) {
      await api.pullServerLLMModelStream(serverURL.value, name, onDownloadProgress)
    } else {
      await api.pullServerLLMModel(name)
    }
    await loadServerModels()
    return true
  } catch (e) {
    alert(t('settings.err_pull', { error: String(e) }))
    return false
  } finally {
    delete modelActionLoading['pull_llm']
    resetActiveDownload()
  }
}

export async function deleteLLMModel(name) {
  if (!confirm(t('settings.confirm_delete_model', { name }))) return
  modelActionLoading[`del_llm_${name}`] = true
  try {
    await api.deleteServerLLMModel(name)
    await loadServerModels()
  } catch (e) {
    alert(t('settings.err_delete', { error: String(e) }))
  } finally {
    delete modelActionLoading[`del_llm_${name}`]
  }
}
