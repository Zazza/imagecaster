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
export const PRESET_TEXT_MAX = 1000

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

let garmentEn = { from: '', en: '' }

export function rememberGarmentEnglish(shown, en) {
  garmentEn = { from: shown, en }
}

export async function garmentOutfitEnglish(text) {
  const v = (text || '').trim()
  if (!v || !CYRILLIC.test(v)) return v
  if (garmentEn.from === v && garmentEn.en) return garmentEn.en
  const [en] = (await api.stylistTranslate([v])) || []
  if (!en || !en.trim()) throw new Error(t('stylist.translate_count', { got: 0, want: 1 }))
  garmentEn = { from: v, en: en.trim() }
  return garmentEn.en
}

export async function garmentOutfitShown(en) {
  if (locale.value !== 'ru' || !en) return en
  const [ru] = (await api.stylistTranslateRu([en])) || []
  if (!ru || !ru.trim()) return en
  rememberGarmentEnglish(ru.trim(), en)
  return ru.trim()
}

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
  return { ...buildPlanRequest(form, seed), shots }
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

export const FRAME_SIZES = ['draft', 'normal', 'large']

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

export function setShotWear(s, j, value) {
  const v = String(value || '').trim()
  return {
    ...s,
    shots: s.shots.map((sh, i) => {
      if (i !== j) return sh
      const base = withoutKey(withoutKey(withoutKey(sh, 'wear_dirty'), 'wear_ru'), 'wear')
      if (!v) return base
      return CYRILLIC.test(v) ? { ...base, wear_ru: v, wear_dirty: true } : { ...base, wear: v }
    }),
  }
}

export function shotWearText(sh, english = false) {
  if (english) return sh.wear || ''
  return sh.wear_ru || sh.wear || ''
}

export function keepWear(prev, next) {
  if (!prev?.shots || !next?.shots) return next
  return {
    ...next,
    shots: next.shots.map((sh, j) => {
      const old = prev.shots[j]
      if (!old) return sh
      const out = { ...sh }
      for (const k of ['wear', 'wear_ru', 'wear_dirty']) if (old[k]) out[k] = old[k]
      return out
    }),
  }
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
      const wear = String(sh.wear_ru || '').trim()
      if (sh.wear_dirty && wear) out.push({ series: i, field: 'wear', shot: j, text: wear })
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
      if (r.field === 'wear') {
        if (!sh?.wear_dirty || !sameText(sh.wear_ru, r.text)) return
        s.shots[r.shot] = withoutKey({ ...sh, wear: english[k] }, 'wear_dirty')
        return
      }
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

export function presetFromText(aspect, text) {
  const v = String(text || '').trim()
  const ru = CYRILLIC.test(v)
  return { aspect, prompt: v, prompt_ru: ru ? v : '', dirty: ru, place: '', framing: 'full', places: [PLACE_ANY], orientation: 'portrait' }
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
  target: '',
  targetKind: 'base',
  presetGroup: null,
  faceSwap: true,
  overrides: null,
  frameSize: 'normal',
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
    aspects: defaultAspects(), garment: '', garmentOutfit: '', seed: '', target: '', targetKind: 'base', presetGroup: null, faceSwap: true, overrides: null, frameSize: 'normal',
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
  restored = false
  planRestored = false
  lastSavedGarment = ''
  garmentEn = { from: '', en: '' }
  clearTimeout(planTimer)
  planTimer = null
}

export async function refreshFrames() {
  const ids = [...jobIds.value]
  if (!ids.length) return
  const seq = ++framesSeq
  try {
    const list = await api.stylistFrames(ids)
    if (seq !== framesSeq) return
    const all = framesFrom(list, ids)
    const deleted = new Set(all.filter(f => f.status === 'deleted').map(f => f.jobId))
    frames.value = all.filter(f => !deleted.has(f.jobId))
    if (deleted.size) jobIds.value = jobIds.value.filter(id => !deleted.has(id))
  } catch (e) {
    console.warn('stylistFrames:', e)
  }
}

let restored = false
let planRestored = false
let planTimer = null
let lastSavedGarment = ''
const PLAN_VERSION = 1
const PLAN_FORM_KEYS = ['person', 'gender', 'appearance', 'wish', 'frames', 'themes', 'aspects', 'garmentOutfit', 'seed', 'target', 'targetKind', 'presetGroup', 'faceSwap', 'frameSize']

export function planSnapshot() {
  const f = {}
  for (const k of PLAN_FORM_KEYS) f[k] = form[k]
  return JSON.stringify({ v: PLAN_VERSION, form: f, series: series.value })
}

export function applyPlanSnapshot(raw) {
  let data
  try {
    data = JSON.parse(raw)
  } catch (e) {
    console.warn('stylist plan:', e)
    return false
  }
  if (!data || data.v !== PLAN_VERSION || typeof data.form !== 'object') return false
  for (const k of PLAN_FORM_KEYS) {
    if (k in data.form) form[k] = data.form[k]
  }
  form.aspects = { ...defaultAspects(), ...(data.form.aspects || {}) }
  series.value = Array.isArray(data.series) ? data.series : []
  return true
}

export async function restorePlan() {
  if (planRestored) return
  try {
    const [raw, garment] = await Promise.all([api.stylistLoadPlan(), api.stylistLoadGarment()])
    if (planRestored) return
    planRestored = true
    if (raw && !series.value.length && applyPlanSnapshot(raw) && garment && !form.garment) {
      form.garment = garment
      lastSavedGarment = garment
    }
  } catch (e) {
    console.warn('stylistLoadPlan:', e)
  }
}

export function savePlanSoon(delay = 800) {
  if (!planRestored) return
  clearTimeout(planTimer)
  planTimer = setTimeout(() => {
    planTimer = null
    api.stylistSavePlan(planSnapshot()).catch(e => { error.value = t('stylist.plan_save_failed', { error: errText(e) }) })
    if (form.garment !== lastSavedGarment) {
      const g = form.garment
      api.stylistSaveGarment(g).then(() => { lastSavedGarment = g }).catch(e => { error.value = t('stylist.plan_save_failed', { error: errText(e) }) })
    }
  }, delay)
}

export async function reshootFrame(jobId) {
  const newId = await api.stylistReshoot(jobId, form.frameSize || 'normal')
  if (!newId) return 0
  framesSeq++
  jobIds.value = jobIds.value.map(id => (id === jobId ? newId : id))
  frames.value = frames.value.map(f => (f.jobId === jobId ? { jobId: newId, status: 'pending', itemId: 0, error: '', shot: f.shot, garment: '' } : f))
  refreshFrames()
  return newId
}

export async function restoreLastShoot() {
  if (restored || jobIds.value.length) return
  try {
    const ids = (await api.stylistLastShoot()) || []
    if (restored || jobIds.value.length) return
    restored = true
    if (!ids.length) return
    jobIds.value = [...ids]
    frames.value = ids.map(id => ({ jobId: id, status: 'pending', itemId: 0, error: '', shot: '' }))
    if (!series.value.length) view.value = 'frames'
    await refreshFrames()
  } catch (e) {
    console.warn('stylistLastShoot:', e)
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
      const slot = series.value[index]?.slot
      const res = await api.stylistRegenerateSeries(req, Number.isInteger(slot) ? slot : index)
      if (res?.series) {
        series.value = replaceSeries(series.value, index, keepWear(series.value[index], res.series))
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
    const ru = locale.value === 'ru'
    const pending = () => dirtyRefs(series.value, !!form.garment).filter(r => ru || r.field === 'wear')
    const refs = pending()
    if (!refs.length) return true
    translating.value = true
    try {
      const english = (await api.stylistTranslate(refs.map(r => r.text))) || []
      if (english.length !== refs.length) throw new Error(t('stylist.translate_count', { got: english.length, want: refs.length }))
      series.value = applyTranslations(series.value, refs, english)
      if (pending().length) {
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

  async function shoot(canFaceSwap, lora) {
    if (shooting.value || planning.value || regenerating.value >= 0 || !series.value.length) return []
    shooting.value = true
    error.value = ''
    notice.value = ''
    try {
      if (!(await translateDirty())) return []
      let outfitEn = ''
      if (form.garment) {
        translating.value = true
        try {
          outfitEn = await garmentOutfitEnglish(form.garmentOutfit)
        } catch (e) {
          error.value = t('stylist.garment_translate_failed', { error: errText(e) })
          return []
        } finally {
          translating.value = false
        }
      }
      const seed = parseSeedInput(form.seed)
      const req = {
        person: lora || form.person,
        appearance: form.appearance,
        garment: form.garment,
        garment_outfit: outfitEn,
        series: series.value,
        face_swap: !!canFaceSwap,
        size: form.frameSize || 'normal',
      }
      if (form.target.startsWith('pipe:')) req.pipeline = Number(form.target.slice(5))
      else if (form.target.startsWith('preset:')) req.preset = Number(form.target.slice(7))
      else if (form.target.startsWith('base:')) req.base = form.target.slice(5)
      if (form.overrides && !form.target.startsWith('pipe:')) req.overrides = form.overrides
      if (seed != null) req.seed = seed
      const ids = (await api.stylistShoot(req)) || []
      if (ids.length) {
        framesSeq++
        jobIds.value = [...ids]
        frames.value = ids.map(id => ({ jobId: id, status: 'pending', itemId: 0, error: '', shot: '' }))
      }
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
