import { computed, inject, onMounted, onUnmounted, provide, ref, watch } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { hasServerFile, loadServerModels, serverModels, serverModelsKnown, serverModelsLoading } from '../serverModels.js'
import { mainFilename, modelStem } from '../catalogFilters.js'
import { normalizeLoras, parseLoras } from '../loraOverride.js'

export const POST_EDITOR_KEY = Symbol('post-editor')
export const MAX_EDITOR_LORAS = 8
export const DEFAULT_EDITOR_WEIGHT = 0.6
export const MIN_EDITOR_WEIGHT = 0
export const MAX_EDITOR_WEIGHT = 2
export const EDITOR_WEIGHT_STEP = 0.05
const SAVE_DELAY_MS = 250
const FAMILY_ORDER = ['klein', 'flux3', 'qwen']

export function editorFamily(name) {
  const n = String(name || '').toLowerCase()
  if (/flux[-_. ]?3.*klein/.test(n)) return 'flux3'
  if (/flux[-_. ]?2.*klein.*4b/.test(n)) return 'klein'
  if (/qwen[-_ ]image[-_ ]2[._]1/.test(n)) return 'qwen'
  return ''
}

export function createPostEditor({ settingsPrefix = 'post_editor', restoreLegacy = true } = {}) {
  const modelKey = `${settingsPrefix}_model`
  const lorasKey = `${settingsPrefix}_loras`
  const model = ref('')
  const lorasByFamily = ref({ klein: [], flux3: [], qwen: [] })
  const catalog = ref([])
  const loaded = ref(false)
  const error = ref('')
  let disposed = false
  let restoring = false
  let changed = false
  let timer
  let pending = null
  let saving = false
  const family = computed(() => editorFamily(model.value))
  const rows = computed({ get: () => lorasByFamily.value[family.value] || [], set: value => { if (family.value) lorasByFamily.value[family.value] = value } })
  const known = computed(() => loaded.value && ['sd', 'text_encoder', 'vae'].every(type => serverModelsKnown[type] === true && !serverModelsLoading[type]))
  const options = computed(() => {
    const result = []
    for (const item of catalog.value) {
      const f = editorFamily(item.name)
      if (!f) continue
      const names = [mainFilename(item), ...(item.variants || []).flatMap(v => [v.filename, v.save_as]).filter(Boolean)]
      for (const file of serverModels.sd || []) {
        if (!names.some(name => modelStem(name) === modelStem(file.name)) || result.some(o => o.value === file.name)) continue
        const missing = (item.modules || []).filter(m => !hasServerFile(m.type, m.filename)).map(m => m.filename)
        result.push({ value: file.name, label: file.name, family: f, missing, available: known.value && missing.length === 0 })
      }
    }
    result.sort((a, b) => FAMILY_ORDER.indexOf(a.family) - FAMILY_ORDER.indexOf(b.family))
    if (model.value && !result.some(o => o.value === model.value)) result.push({ value: model.value, label: model.value, available: false, missing: [] })
    return result
  })
  const selected = computed(() => options.value.find(o => o.value === model.value))
  const ready = computed(() => known.value && !!selected.value?.available)
  function snapshot() {
    if (!ready.value) throw new Error('Post editor unavailable')
    return { model: model.value, loras: normalizeLoras(rows.value).slice(0, MAX_EDITOR_LORAS).map(l => ({ ...l })) }
  }
  async function flush() {
    if (saving || !pending) return
    saving = true
    while (pending) {
      const value = pending
      pending = null
      try { await api.updateSettings(value); error.value = '' } catch (e) { error.value = String(e) }
    }
    saving = false
  }
  function scheduleSave() {
    if (disposed || restoring) return
    changed = true
    if (!loaded.value) return
    pending = { [modelKey]: model.value, [lorasKey]: JSON.stringify(Object.fromEntries(FAMILY_ORDER.map(f => [f, normalizeLoras(lorasByFamily.value[f]).slice(0, MAX_EDITOR_LORAS)]))) }
    clearTimeout(timer)
    timer = setTimeout(flush, SAVE_DELAY_MS)
  }
  watch([model, lorasByFamily], scheduleSave, { deep: true, flush: 'sync' })
  async function load() {
    if (disposed) return
    try {
      const [settings, data] = await Promise.all([api.getSettings(), api.getModelCatalog(), loadServerModels()])
      if (disposed) return
      catalog.value = Array.isArray(data) ? data : data?.sd_models || data?.models || data?.sd || []
      restoring = true
      if (!changed) {
        let saved = {}
        if (settings?.[lorasKey]) {
          try { saved = JSON.parse(settings[lorasKey]) || {} } catch (e) { error.value = String(e) }
        }
        for (const f of FAMILY_ORDER) {
          const oldKey = `edit_loras_${f}`
          const restored = Object.hasOwn(saved, f) ? normalizeLoras(saved[f]) : restoreLegacy && Object.hasOwn(settings || {}, oldKey) ? parseLoras(settings[oldKey]) : restoreLegacy && f === 'klein' ? parseLoras(settings?.effect_loras_klein) : []
          lorasByFamily.value[f] = restored.slice(0, MAX_EDITOR_LORAS)
        }
        model.value = settings?.[modelKey] || ''
      }
      loaded.value = true
      if (!known.value) {
        restoring = false
        error.value = t('post_editor.inventory_error')
        return
      }
      if (!model.value) model.value = options.value.find(o => o.available)?.value || ''
      restoring = false
      scheduleSave()
    } catch (e) {
      if (disposed) return
      error.value = String(e)
      loaded.value = true
      restoring = false
    }
  }
  function dispose() { disposed = true; clearTimeout(timer); flush() }
  return { model, family, rows, options, selected, ready, known, error, loaded, snapshot, load, dispose }
}

export function providePostEditor(options) {
  const editor = createPostEditor(options)
  provide(POST_EDITOR_KEY, editor)
  onMounted(editor.load)
  onUnmounted(editor.dispose)
  return editor
}

export function usePostEditor() {
  return inject(POST_EDITOR_KEY, null) || { model: ref('klein'), family: ref('klein'), ready: ref(true), snapshot: () => ({ model: 'klein', loras: [] }) }
}
