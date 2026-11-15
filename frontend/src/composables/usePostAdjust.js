import { ref, reactive, computed, watch } from 'vue'
import { api } from '../api.js'

export const SLIDER_META = {
  brightness: { min: -100, max: 100, step: 1, def: 0 },
  contrast: { min: -100, max: 100, step: 1, def: 0 },
  black_point: { min: 0, max: 254, step: 1, def: 0 },
  white_point: { min: 1, max: 255, step: 1, def: 255 },
  gamma: { min: 0.1, max: 3, step: 0.01, def: 1 },
  saturation: { min: -100, max: 100, step: 1, def: 0 },
  temperature: { min: -100, max: 100, step: 1, def: 0 },
  balance_r: { min: -100, max: 100, step: 1, def: 0 },
  balance_g: { min: -100, max: 100, step: 1, def: 0 },
  balance_b: { min: -100, max: 100, step: 1, def: 0 },
  sharpen: { min: 0, max: 100, step: 1, def: 0 },
  vignette: { min: 0, max: 100, step: 1, def: 0 },
}

export const ADJUST_GROUPS = [
  { labelKey: 'post.group_light', keys: ['brightness', 'contrast'] },
  { labelKey: 'post.group_levels', keys: ['black_point', 'white_point', 'gamma'] },
  { labelKey: 'post.group_color', keys: ['saturation', 'temperature', 'balance_r', 'balance_g', 'balance_b'] },
  { labelKey: 'post.group_film', keys: ['sharpen', 'vignette'] },
]

export const FINISH_RATIOS = ['1:1', '4:5', '3:4', '9:16', '16:9', '3:2']

const FINISH_DEFAULTS = { ratio: '', fit: 'crop', offset: 0.5, frame: '', caption: '', caption_pos: 'bottom', caption_size: 'm' }

function defaults() {
  const out = {}
  for (const k of Object.keys(SLIDER_META)) out[k] = SLIDER_META[k].def
  return out
}

export function adjustObjectActive(a) {
  if (!a || typeof a !== 'object') return false
  return Object.keys(SLIDER_META).some(k => {
    if (a[k] === undefined) return false
    const v = a[k]
    const d = SLIDER_META[k].def
    if (k === 'white_point' || k === 'gamma') return v !== d && v !== 0
    return v !== d
  })
}

export function usePostAdjust({ currentB64, error }) {
  const adjust = reactive(defaults())
  const finish = reactive({ ...FINISH_DEFAULTS })

  const adjustNeutral = computed(() =>
    Object.keys(SLIDER_META).every(k => {
      const v = adjust[k]
      const d = SLIDER_META[k].def
      return k === 'white_point' ? (v === d || v === 0) : (k === 'gamma' ? (v === d || v === 0) : v === d)
    }))

  const finishActive = computed(() => !!finish.ratio || !!finish.frame || !!finish.caption.trim())

  function adjustParams() {
    return { ...adjust }
  }

  function finishParams() {
    if (!finishActive.value) return null
    const p = {}
    if (finish.ratio) {
      p.ratio = finish.ratio
      p.fit = finish.fit
      if (finish.fit === 'crop') p.offset = finish.offset
    }
    if (finish.frame) p.frame = finish.frame
    if (finish.caption.trim()) {
      p.caption = finish.caption
      p.caption_pos = finish.caption_pos
      p.caption_size = finish.caption_size
    }
    return p
  }

  function resetFinish() {
    Object.assign(finish, FINISH_DEFAULTS)
  }

  function resetAdjustKey(key) {
    adjust[key] = SLIDER_META[key].def
  }

  function resetAdjustAll() {
    for (const k of Object.keys(SLIDER_META)) adjust[k] = SLIDER_META[k].def
  }

  function formatAdjust(key) {
    const v = adjust[key]
    return SLIDER_META[key].step < 1 ? Number(v).toFixed(2) : String(v)
  }

  const autoAdjusting = ref(false)

  async function applyAutoAdjust(kind) {
    if (!currentB64.value || autoAdjusting.value) return
    autoAdjusting.value = true
    error.value = ''
    try {
      const stats = await api.autoAdjust(currentB64.value)
      if (kind === 'levels') {
        adjust.black_point = stats.black_point
        adjust.white_point = stats.white_point
        adjust.gamma = stats.gamma
      } else {
        adjust.temperature = stats.temperature
      }
    } catch (e) {
      error.value = String(e)
    } finally {
      autoAdjusting.value = false
    }
  }

  const previewB64 = ref('')
  const previewing = ref(false)
  let previewTimer = null
  let previewReqId = 0

  function schedulePreview() {
    clearTimeout(previewTimer)
    if (adjustNeutral.value && !finishActive.value) {
      previewReqId++
      previewing.value = false
      previewB64.value = currentB64.value
      return
    }
    previewTimer = setTimeout(runPreview, 150)
  }

  async function runPreview() {
    if (!currentB64.value) return
    const reqId = ++previewReqId
    previewing.value = true
    try {
      const fp = finishParams()
      const res = fp
        ? await api.previewPost(currentB64.value, adjustParams(), fp, 900)
        : await api.previewAdjust(currentB64.value, adjustParams(), 900)
      if (reqId !== previewReqId) return
      if (res) previewB64.value = res
    } catch (e) {
      if (reqId === previewReqId) error.value = String(e)
    } finally {
      if (reqId === previewReqId) previewing.value = false
    }
  }

  function stopPreview() {
    clearTimeout(previewTimer)
  }

  watch(adjust, schedulePreview)
  watch(finish, schedulePreview)
  watch(currentB64, schedulePreview)

  return {
    sliderMeta: SLIDER_META, adjustGroups: ADJUST_GROUPS, finishRatios: FINISH_RATIOS,
    adjust, finish, adjustNeutral, finishActive, autoAdjusting, previewB64, previewing,
    adjustParams, finishParams, resetFinish, resetAdjustKey, resetAdjustAll, formatAdjust, applyAutoAdjust, stopPreview,
  }
}
