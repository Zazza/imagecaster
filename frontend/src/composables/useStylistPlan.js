import { ref, reactive, computed } from 'vue'
import { api } from '../api.js'
import { TRAIN_BASES } from '../people.js'
import { parseSeedInput } from '../seed.js'
import { locale, t } from '../i18n/index.js'

export const ASPECTS = ['outfit', 'hair', 'makeup', 'background', 'pose']
export const FRAMES_MAX = 24
export const FRAMES_PER_SERIES = 4
export const PRESET_NAME_MAX = 60
export const PLACE_ANY = 'any'

export function defaultAspects() {
  return Object.fromEntries(ASPECTS.map(a => [a, { mode: 'auto', ids: [], text: '' }]))
}

export function stylistPeople(list) {
  if (!Array.isArray(list)) return []
  return list.filter(p => p?.lora && TRAIN_BASES.includes(p.base))
}

export function clampFrames(v) {
  const n = Math.round(Number(v))
  if (!Number.isFinite(n) || n < 1) return 1
  return Math.min(n, FRAMES_MAX)
}

export function randomSeed() {
  return Math.floor(Math.random() * 2147483646) + 1
}

export function buildPlanRequest(form, seed = randomSeed()) {
  const aspects = {}
  for (const a of ASPECTS) {
    const c = form.aspects?.[a] || { mode: 'auto', ids: [] }
    aspects[a] = { mode: c.mode, ids: c.mode === 'pick' ? [...(c.ids || [])] : [], text: c.mode === 'custom' ? (c.text || '') : '' }
  }
  if (form.garment) aspects.outfit = { mode: 'keep', ids: [], text: '' }
  return {
    person: form.person || '',
    gender: form.gender || '',
    wish: form.wish || '',
    frames: clampFrames(form.frames),
    themes: [...(form.themes || [])],
    aspects,
    garment: !!form.garment,
    seed,
    locale: locale.value,
  }
}

const CYRILLIC = /[А-Яа-яЁё]/

export async function translateCustom(req) {
  if (locale.value !== 'ru') return req
  const keys = Object.keys(req.aspects).filter(a => req.aspects[a].mode === 'custom' && CYRILLIC.test(req.aspects[a].text || ''))
  if (!keys.length) return req
  const english = (await api.stylistTranslate(keys.map(a => req.aspects[a].text))) || []
  if (english.length !== keys.length) throw new Error(t('stylist.translate_count', { got: english.length, want: keys.length }))
  const aspects = { ...req.aspects }
  keys.forEach((a, k) => {
    aspects[a] = { ...aspects[a], text: english[k], text_ru: req.aspects[a].text }
  })
  return { ...req, aspects }
}

async function withTranslatedCustom(req) {
  try {
    return await translateCustom(req)
  } catch (e) {
    error.value = t('stylist.custom_translate_failed', { error: errText(e) })
    return null
  }
}

export function regenerateRequest(form, series, index, seed = randomSeed()) {
  const shots = Math.max(1, Math.min(series?.[index]?.shots?.length || FRAMES_PER_SERIES, FRAMES_PER_SERIES))
  return { ...buildPlanRequest(form, seed), frames: Math.min(index * FRAMES_PER_SERIES + shots, FRAMES_MAX) }
}

export function removeSeries(series, index) {
  return series.filter((_, i) => i !== index)
}

export function removeShot(series, index, shot) {
  const out = series.map((s, i) => i === index ? { ...s, shots: s.shots.filter((_, j) => j !== shot) } : s)
  return out.filter(s => s.shots.length > 0)
}

export function replaceSeries(series, index, next) {
  return series.map((s, i) => i === index ? next : s)
}

export const SERIES_FIELDS = ['outfit', 'hair', 'makeup', 'background']

function withoutKey(obj, key) {
  const { [key]: _, ...rest } = obj
  return rest
}

function dirtyList(s) {
  return Array.isArray(s.ru_dirty) ? s.ru_dirty : []
}

function cleanDirty(obj, list) {
  return list.length ? { ...obj, ru_dirty: list } : withoutKey(obj, 'ru_dirty')
}

function withoutAI(s, field) {
  if (!Array.isArray(s.ru_ai)) return s
  const rest = s.ru_ai.filter(f => f !== field)
  return rest.length ? { ...s, ru_ai: rest } : withoutKey(s, 'ru_ai')
}

export function setRuField(s, field, value) {
  if (!String(value).trim()) return setEnField(s, field, '')
  return { ...withoutAI(s, field), [field + '_ru']: value, ru_dirty: [...dirtyList(s).filter(f => f !== field), field] }
}

export function setEnField(s, field, value) {
  return cleanDirty({ ...withoutAI(s, field), [field]: value, [field + '_ru']: '' }, dirtyList(s).filter(f => f !== field))
}

export function setShotRu(s, j, value) {
  if (!String(value).trim()) return setShotEn(s, j, '')
  return { ...s, shots: s.shots.map((sh, i) => i === j ? { ...withoutKey(sh, 'framing'), prompt_ru: value, ru_dirty: true } : sh) }
}

export function setShotEn(s, j, value) {
  return { ...s, shots: s.shots.map((sh, i) => i === j ? withoutKey(withoutKey({ ...sh, prompt: value, prompt_ru: '' }, 'ru_dirty'), 'framing') : sh) }
}

export function dirtyRefs(series, skipOutfit = false) {
  const out = []
  ;(series || []).forEach((s, i) => {
    for (const f of dirtyList(s)) {
      if (skipOutfit && f === 'outfit') continue
      const text = String(s[f + '_ru'] || '').trim()
      if (text) out.push({ series: i, field: f, shot: -1, text })
    }
    ;(s.shots || []).forEach((sh, j) => {
      const text = String(sh.prompt_ru || '').trim()
      if (sh.ru_dirty && text) out.push({ series: i, field: 'prompt', shot: j, text })
    })
  })
  return out
}

function sameText(v, text) {
  return String(v || '').trim() === text
}

export function applyTranslations(series, refs, english) {
  const out = series.map(s => ({ ...s, shots: [...(s.shots || [])] }))
  refs.forEach((r, k) => {
    const s = out[r.series]
    if (!s) return
    if (r.shot >= 0) {
      const sh = s.shots[r.shot]
      if (!sh?.ru_dirty || !sameText(sh.prompt_ru, r.text)) return
      s.shots[r.shot] = withoutKey({ ...sh, prompt: english[k] }, 'ru_dirty')
      return
    }
    if (!dirtyList(s).includes(r.field) || !sameText(s[r.field + '_ru'], r.text)) return
    out[r.series] = cleanDirty({ ...s, [r.field]: english[k] }, dirtyList(s).filter(f => f !== r.field))
  })
  return out
}

export function presetFromField(series, field, ru) {
  return {
    aspect: field,
    prompt: series[field] || '',
    prompt_ru: ru ? (series[field + '_ru'] || '') : '',
    place: field === 'background' ? (series.place || '') : '',
    dirty: !!ru && dirtyList(series).includes(field),
  }
}

export function presetFromShot(series, j, ru) {
  const shot = series.shots[j]
  return {
    aspect: 'pose',
    prompt: shot.prompt || '',
    prompt_ru: ru ? (shot.prompt_ru || '') : '',
    framing: shot.framing || '',
    orientation: shot.landscape ? 'landscape' : 'portrait',
    places: [series.place || PLACE_ANY],
    dirty: !!ru && !!shot.ru_dirty,
  }
}

export function dropChoiceId(aspects, id) {
  return Object.fromEntries(Object.entries(aspects).map(([a, c]) => [a, { ...c, ids: (c.ids || []).filter(x => x !== id) }]))
}

export function totalShots(series) {
  return (series || []).reduce((n, s) => n + (s.shots?.length || 0), 0)
}

export function framesFrom(list, ids) {
  const byId = new Map((list || []).map(f => [f.job_id, f]))
  return ids.map(id => {
    const f = byId.get(id)
    return { jobId: id, status: f?.status || 'gone', itemId: f?.item_id || 0, error: f?.error || '', shot: f?.shot || '', garment: f?.garment || '' }
  })
}

export function latestDone(frames) {
  for (let i = frames.length - 1; i >= 0; i--) {
    if (frames[i].status === 'completed' && frames[i].itemId) return frames[i]
  }
  return null
}

const form = reactive({
  person: '',
  gender: '',
  appearance: '',
  wish: '',
  frames: 8,
  themes: [],
  aspects: defaultAspects(),
  garment: '',
  garmentOutfit: '',
  seed: '',
})
const series = ref([])
const warnings = ref([])
const usedLLM = ref(false)
const planning = ref(false)
const regenerating = ref(-1)
const shooting = ref(false)
const translating = ref(false)
const error = ref('')
const notice = ref('')
const jobIds = ref([])
const frames = ref([])
const view = ref('plan')
const planEpoch = ref(0)
let framesSeq = 0

export function resetStylistPlan() {
  Object.assign(form, {
    person: '', gender: '', appearance: '', wish: '', frames: 8, themes: [],
    aspects: defaultAspects(), garment: '', garmentOutfit: '', seed: '',
  })
  series.value = []
  warnings.value = []
  usedLLM.value = false
  planning.value = false
  regenerating.value = -1
  shooting.value = false
  translating.value = false
  error.value = ''
  notice.value = ''
  jobIds.value = []
  frames.value = []
  view.value = 'plan'
  framesSeq++
}

export async function refreshFrames() {
  const ids = [...jobIds.value]
  if (!ids.length) return
  const seq = ++framesSeq
  try {
    const list = await api.stylistFrames(ids)
    if (seq === framesSeq) frames.value = framesFrom(list, ids)
  } catch (e) {
    console.warn('stylistFrames:', e)
  }
}

function errText(e) {
  return String(e?.message || e || '')
}

export function useStylistPlan() {
  const frameCount = computed(() => totalShots(series.value))
  const outfitLocked = computed(() => !!form.garment)

  async function makePlan() {
    if (planning.value || shooting.value) return
    planning.value = true
    error.value = ''
    notice.value = ''
    try {
      const req = await withTranslatedCustom(buildPlanRequest(form))
      if (!req) return
      const res = await api.stylistPlan(req)
      series.value = res?.series || []
      planEpoch.value++
      warnings.value = res?.warnings || []
      usedLLM.value = !!res?.used_llm
      view.value = 'plan'
    } catch (e) {
      error.value = errText(e)
    } finally {
      planning.value = false
    }
  }

  async function regenerate(index) {
    if (regenerating.value >= 0 || shooting.value) return
    regenerating.value = index
    error.value = ''
    try {
      const req = await withTranslatedCustom(regenerateRequest(form, series.value, index))
      if (!req) return
      const res = await api.stylistRegenerateSeries(req, index)
      if (res?.series) {
        series.value = replaceSeries(series.value, index, res.series)
        planEpoch.value++
      }
      warnings.value = res?.warnings || []
      usedLLM.value = !!res?.used_llm
    } catch (e) {
      error.value = errText(e)
    } finally {
      regenerating.value = -1
    }
  }

  function deleteSeries(index) {
    if (shooting.value) return
    series.value = removeSeries(series.value, index)
    planEpoch.value++
  }

  function deleteShot(index, shot) {
    if (shooting.value) return
    series.value = removeShot(series.value, index, shot)
    planEpoch.value++
  }

  function updateSeries(index, next) {
    series.value = replaceSeries(series.value, index, next)
  }

  async function translateDirty() {
    if (locale.value !== 'ru') return true
    const refs = dirtyRefs(series.value, !!form.garment)
    if (!refs.length) return true
    translating.value = true
    try {
      const english = (await api.stylistTranslate(refs.map(r => r.text))) || []
      if (english.length !== refs.length) throw new Error(t('stylist.translate_count', { got: english.length, want: refs.length }))
      series.value = applyTranslations(series.value, refs, english)
      if (dirtyRefs(series.value, !!form.garment).length) {
        error.value = t('stylist.plan_changed')
        return false
      }
      return true
    } catch (e) {
      error.value = t('stylist.translate_failed', { error: errText(e) })
      return false
    } finally {
      translating.value = false
    }
  }

  async function shoot(canFaceSwap) {
    if (shooting.value || planning.value || regenerating.value >= 0 || !series.value.length) return []
    shooting.value = true
    error.value = ''
    notice.value = ''
    try {
      if (!(await translateDirty())) return []
      const seed = parseSeedInput(form.seed)
      const req = {
        person: form.person,
        appearance: form.appearance,
        garment: form.garment,
        garment_outfit: form.garment ? form.garmentOutfit : '',
        series: series.value,
        face_swap: !!canFaceSwap,
      }
      if (seed != null) req.seed = seed
      const ids = (await api.stylistShoot(req)) || []
      jobIds.value = [...jobIds.value, ...ids]
      frames.value = [...frames.value, ...ids.map(id => ({ jobId: id, status: 'pending', itemId: 0, error: '', shot: '' }))]
      if (ids.length) view.value = 'frames'
      refreshFrames()
      return ids
    } catch (e) {
      error.value = errText(e)
      return []
    } finally {
      shooting.value = false
    }
  }

  async function savePreset(payload) {
    error.value = ''
    const { dirty, ...preset } = payload
    const ru = String(preset.prompt_ru || '').trim()
    try {
      if (dirty && ru) {
        const english = (await api.stylistTranslate([ru])) || []
        if (english.length !== 1 || !String(english[0] || '').trim()) throw new Error(t('stylist.translate_count', { got: english.length, want: 1 }))
        preset.prompt = english[0]
      }
    } catch (e) {
      error.value = t('stylist.preset_translate_failed', { error: errText(e) })
      return ''
    }
    try {
      return (await api.stylistSavePreset(preset)) || ''
    } catch (e) {
      error.value = errText(e)
      return ''
    }
  }

  return {
    savePreset, planEpoch,
    form, series, warnings, usedLLM, planning, regenerating, shooting, translating, error, notice, jobIds, frames, view,
    frameCount, outfitLocked, refreshFrames,
    makePlan, regenerate, deleteSeries, deleteShot, updateSeries, shoot,
  }
}
