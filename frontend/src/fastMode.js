import { ref } from 'vue'
import { api } from './api.js'

export const fastGeneration = ref(false)

export function applyFastSetting(settings) {
  fastGeneration.value = settings?.fast_generation === 'true'
}

export async function loadFastGeneration() {
  try {
    applyFastSetting(await api.getSettings())
  } catch (e) {
    console.error('load fast_generation:', e)
  }
}

export async function saveFastGeneration(on) {
  const prev = fastGeneration.value
  fastGeneration.value = on
  try {
    await api.updateSettings({ fast_generation: on ? 'true' : 'false' })
  } catch (e) {
    console.error('save fast_generation:', e)
    fastGeneration.value = prev
    throw e
  }
}

function modelStem(name) {
  let base = String(name || '').trim().split(/[\\/]/).pop()
  const i = base.indexOf(' [')
  if (i >= 0) base = base.slice(0, i)
  return base.replace(/\.(safetensors|ckpt|gguf)$/i, '').toLowerCase()
}

function catalogModel(catalog, model) {
  const stem = modelStem(model)
  if (!stem) return null
  const models = catalog?.sd_models || []
  return models.find(m => modelStem(m.filename || `${m.name}.safetensors`) === stem || String(m.name || '').toLowerCase() === stem)
    || models.find(m => (m.variants || []).some(v => modelStem(v.filename) === stem))
    || null
}

export function fastCacheFor(catalog, model) {
  return catalogModel(catalog, model)?.fast_cache || ''
}

export function modelBaseFor(catalog, model) {
  return catalogModel(catalog, model)?.base || ''
}
