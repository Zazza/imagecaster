import { ref } from 'vue'
import { api } from '../api.js'
import { filterAvailable } from '../presetAvailability.js'

const presets = ref([])
const presetTypes = ref([])
const compoundPresets = ref([])

export const PRESET_SYNC_WAIT_MS = 1500

export async function withPresetStatuses(apply, waitMs = PRESET_SYNC_WAIT_MS) {
  const sync = Promise.resolve().then(() => api.syncPresets()).then(r => r?.statuses || null).catch(e => {
    console.warn('presets sync', e)
    return null
  })
  let timer
  let timedOut = false
  const timeout = new Promise(resolve => {
    timer = setTimeout(() => { timedOut = true; resolve(null) }, waitMs)
  })
  const first = await Promise.race([sync, timeout])
  clearTimeout(timer)
  await apply(first)
  if (!timedOut) return
  sync.then(late => {
    if (!late) return
    return Promise.resolve().then(() => apply(late)).catch(e => console.warn('presets late sync', e))
  })
}

export async function listAvailablePresets(onList) {
  await withPresetStatuses(async statuses => {
    onList(filterAvailable((await api.listPresets()) || [], statuses))
  })
}

export function usePresets() {
  async function loadPresets() {
    try {
      await withPresetStatuses(async statuses => {
        const [p, t, c] = await Promise.all([api.listPresets(), api.listPresetTypes(), api.listCompoundPresets()])
        presets.value = filterAvailable(p || [], statuses)
        presetTypes.value = t || []
        compoundPresets.value = c || []
      })
    } catch (e) {
      console.error(e)
    }
  }

  return { presets, presetTypes, compoundPresets, loadPresets }
}
