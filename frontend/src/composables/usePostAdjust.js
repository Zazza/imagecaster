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

export const EYE_COLORS = ['blue', 'light blue', 'green', 'grey', 'hazel', 'brown', 'amber', 'violet']

export const RETOUCH_DEFAULTS = { under_eyes: false, teeth: false, eye_whites: false, eye_color: '', light: false, face_width: 0 }

export const FACE_WIDTH_MAX = 10

export const BODY_WIDTH_MAX = 10

export const BODY_HEIGHT_MAX = 7

const BODY_DEFAULTS = { width: 0, height: 0 }

const FACE_CACHE_MAX = 4

export function retouchObjectActive(r) {
  if (!r || typeof r !== 'object') return false
  return Object.keys(RETOUCH_DEFAULTS).some(k => r[k] !== undefined && r[k] !== RETOUCH_DEFAULTS[k])
}

export function retouchObjectLocal(r) {
  if (!r || typeof r !== 'object') return false
  return !!(r.under_eyes || r.teeth || r.eye_whites || (r.eye_color || '').trim() || (r.face_width || 0) !== 0)
}

export function splitRecipeAdjust(obj) {
  const { retouch, upscale_4k: upscale4k, body, ...adjust } = (obj && typeof obj === 'object') ? obj : {}
  return { adjust, extras: { retouch, upscale4k, body } }
}

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
  const retouch = reactive({ ...RETOUCH_DEFAULTS })
  const upscale4k = ref(false)
  const face = reactive({ status: 'idle', skip: '', error: '' })
  const faceCache = new Map()
  const body = reactive({ ...BODY_DEFAULTS })
  const bodyState = reactive({ status: 'idle', skip: '', error: '' })
  const bodyCache = new Map()

  const adjustNeutral = computed(() =>
    Object.keys(SLIDER_META).every(k => {
      const v = adjust[k]
      const d = SLIDER_META[k].def
      return k === 'white_point' ? (v === d || v === 0) : (k === 'gamma' ? (v === d || v === 0) : v === d)
    }))

  const finishActive = computed(() => !!finish.ratio || !!finish.frame || !!finish.caption.trim())
  const retouchActive = computed(() => retouchObjectActive(retouch))
  const retouchLocal = computed(() => retouchObjectLocal(retouch))
  const bodyActive = computed(() => (body.width || 0) !== 0 || (body.height || 0) !== 0)
  const postRetouchActive = computed(() => retouchActive.value || bodyActive.value)

  function bodyParams() {
    return bodyActive.value ? { width: body.width, height: body.height } : null
  }

  function resetBody() {
    Object.assign(body, BODY_DEFAULTS)
  }

  function clampBody(v, max) {
    return Math.max(-max, Math.min(max, Number(v) || 0))
  }

  function retouchParams() {
    if (!retouchActive.value) return null
    const p = {}
    for (const k of Object.keys(RETOUCH_DEFAULTS)) {
      if (retouch[k] !== RETOUCH_DEFAULTS[k]) p[k] = retouch[k]
    }
    return p
  }

  function resetRetouch() {
    Object.assign(retouch, RETOUCH_DEFAULTS)
  }

  function recipeExtras() {
    return { retouch: retouchParams() || {}, body: bodyParams() || {}, upscale_4k: upscale4k.value }
  }

  function applyRecipeExtras(extras) {
    if (!extras) return
    if (extras.retouch !== undefined) {
      const r = extras.retouch && typeof extras.retouch === 'object' ? extras.retouch : {}
      const next = { ...RETOUCH_DEFAULTS }
      for (const k of Object.keys(RETOUCH_DEFAULTS)) {
        if (r[k] !== undefined) next[k] = r[k]
      }
      next.face_width = Math.max(-FACE_WIDTH_MAX, Math.min(FACE_WIDTH_MAX, Number(next.face_width) || 0))
      Object.assign(retouch, next)
    }
    if (extras.upscale4k !== undefined) upscale4k.value = !!extras.upscale4k
    if (extras.body !== undefined) {
      const b = extras.body && typeof extras.body === 'object' ? extras.body : {}
      Object.assign(body, { width: clampBody(b.width, BODY_WIDTH_MAX), height: clampBody(b.height, BODY_HEIGHT_MAX) })
    }
  }

  function bodyPoints(b64) {
    if (!b64) return Promise.resolve({ points: [], skip: '' })
    let p = bodyCache.get(b64)
    if (!p) {
      p = Promise.resolve(api.postBodyPoints(b64))
        .then(r => ({ points: r?.points || [], skip: r?.skip || '', error: r?.error || '' }))
        .catch(e => ({ points: [], skip: 'unavailable', error: String(e) }))
      bodyCache.set(b64, p)
      if (bodyCache.size > FACE_CACHE_MAX) bodyCache.delete(bodyCache.keys().next().value)
    }
    return p
  }

  async function currentBody() {
    const cur = currentB64.value
    bodyState.status = 'loading'
    const r = await bodyPoints(cur)
    if (cur === currentB64.value) {
      bodyState.status = r.skip ? 'skip' : 'ok'
      bodyState.skip = r.skip
      bodyState.error = r.error || ''
    }
    return r
  }

  function retryBody() {
    bodyCache.delete(currentB64.value)
    bodyState.status = 'idle'
    schedulePreview()
  }

  function facePoints(b64) {
    if (!b64) return Promise.resolve({ points: [], skip: '' })
    let p = faceCache.get(b64)
    if (!p) {
      p = Promise.resolve(api.postFaceLandmarks(b64))
        .then(r => ({ points: r?.points || [], skip: r?.skip || '', error: r?.error || '' }))
        .catch(e => ({ points: [], skip: 'unavailable', error: String(e) }))
      faceCache.set(b64, p)
      if (faceCache.size > FACE_CACHE_MAX) faceCache.delete(faceCache.keys().next().value)
    }
    return p
  }

  async function currentFace() {
    const cur = currentB64.value
    face.status = 'loading'
    const r = await facePoints(cur)
    if (cur === currentB64.value) {
      face.status = r.skip ? 'skip' : 'ok'
      face.skip = r.skip
      face.error = r.error || ''
    }
    return r
  }

  function retryFace() {
    faceCache.delete(currentB64.value)
    face.status = 'idle'
    schedulePreview()
  }

  async function retouchFor(b64) {
    const out = {}
    const r = retouchParams()
    if (r) {
      out.retouch = r
      if (retouchLocal.value) out.face_points = (await facePoints(b64)).points
    }
    const b = bodyParams()
    if (b) {
      out.body = b
      out.body_points = (await bodyPoints(b64)).points
    }
    return out
  }

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
    if (adjustNeutral.value && !finishActive.value && !postRetouchActive.value) {
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
      let res
      if (postRetouchActive.value) {
        const cur = currentB64.value
        const pts = retouchLocal.value ? (await currentFace()).points : []
        const bodyPts = bodyActive.value ? (await currentBody()).points : []
        if (reqId !== previewReqId) return
        res = await api.previewPostRetouch(cur, adjustParams(), retouchParams() || {}, pts, bodyParams() || {}, bodyPts, fp || {}, 900)
      } else {
        res = fp
          ? await api.previewPost(currentB64.value, adjustParams(), fp, 900)
          : await api.previewAdjust(currentB64.value, adjustParams(), 900)
      }
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
  watch(retouch, schedulePreview)
  watch(body, schedulePreview)
  watch(currentB64, schedulePreview)

  return {
    sliderMeta: SLIDER_META, adjustGroups: ADJUST_GROUPS, finishRatios: FINISH_RATIOS,
    adjust, finish, adjustNeutral, finishActive, autoAdjusting, previewB64, previewing,
    adjustParams, finishParams, resetFinish, resetAdjustKey, resetAdjustAll, formatAdjust, applyAutoAdjust, stopPreview,
    retouch, upscale4k, face, retouchActive, retouchLocal, eyeColors: EYE_COLORS, faceWidthMax: FACE_WIDTH_MAX,
    retouchParams, resetRetouch, recipeExtras, applyRecipeExtras, facePoints, retryFace, retouchFor,
    body, bodyState, bodyActive, bodyWidthMax: BODY_WIDTH_MAX, bodyHeightMax: BODY_HEIGHT_MAX, bodyParams, resetBody, retryBody, bodyPoints,
  }
}
