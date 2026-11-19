import { modelBaseFor } from './fastMode.js'

export const TRAIN_BASES = ['Z-Image', 'Moss-Image', 'SDXL', 'Flux.2', 'Flux.1']

const BASE_LABELS = { 'Flux.2': 'Flux.2 Klein 4B', 'Flux.1': 'Flux.1 dev' }

export function trainBaseLabel(base) {
  return BASE_LABELS[base] || base
}

function styleModel(preset, overrides) {
  return overrides?.model_name || preset?.model_name || ''
}

function personBase(p) {
  return p?.base || 'Z-Image'
}

function styleBase(catalog, preset, overrides) {
  return modelBaseFor(catalog, styleModel(preset, overrides))
}

export function peopleForStyle(catalog, preset, overrides, people) {
  if (!preset || !Array.isArray(people)) return []
  const base = styleBase(catalog, preset, overrides)
  if (!TRAIN_BASES.includes(base)) return []
  return people.filter(p => personBase(p) === base)
}

export function canFaceSwap(person, featureOn) {
  return !!(featureOn && person?.identity)
}

export function personOverride(catalog, preset, overrides, people, selectedName, faceSwap = false) {
  if (!selectedName || peopleState(catalog, 'preset', preset, overrides, people) !== 'ok') return null
  const p = peopleForStyle(catalog, preset, overrides, people).find(p => sameKey(personKey(p), selectedName))
  if (!p) return null
  return faceSwap && p.identity ? { name: p.name, face_swap: true } : { name: p.name }
}

export function peopleState(catalog, mode, preset, overrides, people) {
  if (!Array.isArray(people) || people.length === 0) return 'no_people'
  if (mode !== 'preset') return 'not_style'
  if (!preset) return 'no_style'
  if (!Array.isArray(catalog?.sd_models)) return 'no_catalog'
  if (!TRAIN_BASES.includes(styleBase(catalog, preset, overrides))) return 'wrong_model'
  if (peopleForStyle(catalog, preset, overrides, people).length === 0) return 'no_people_base'
  return 'ok'
}

export function peopleStyleBase(catalog, preset, overrides) {
  return styleBase(catalog, preset, overrides)
}

export function sameKey(a, b) {
  return String(a || '').toLocaleLowerCase() === String(b || '').toLocaleLowerCase()
}

export function personKey(p) {
  const d = String(p?.display_name || '').trim()
  return d || p?.name || ''
}

export function peopleGroups(people) {
  const groups = []
  const byKey = new Map()
  for (const p of Array.isArray(people) ? people : []) {
    if (!TRAIN_BASES.includes(personBase(p))) continue
    const key = personKey(p)
    if (!key) continue
    let g = byKey.get(key.toLocaleLowerCase())
    if (!g) {
      g = { key, loras: [] }
      byKey.set(key.toLocaleLowerCase(), g)
      groups.push(g)
    }
    g.loras.push(p)
  }
  return groups
}

export function groupLoraForBase(group, base) {
  if (!group || !base) return null
  return group.loras.find(p => personBase(p) === base) || null
}

export function groupBases(group) {
  return group ? [...new Set(group.loras.map(personBase))] : []
}

export function pipelinePerson(catalog, stepModels, group) {
  const loras = (stepModels || []).map(m => groupLoraForBase(group, modelBaseFor(catalog, m))?.name || '')
  const missing = loras.map((l, i) => (l ? 0 : i + 1)).filter(Boolean)
  return { loras, missing, available: loras.some(Boolean) }
}

export function pipelineOverride(catalog, stepModels, group, faceSwap = false) {
  const pp = pipelinePerson(catalog, stepModels, group)
  if (!pp.available) return null
  const map = {}
  let swap = ''
  ;(stepModels || []).forEach(m => {
    const base = modelBaseFor(catalog, m)
    const p = groupLoraForBase(group, base)
    if (!p) return
    map[base] = p.name
    if (!swap && p.identity) swap = p.name
  })
  const out = { name: pp.loras.find(Boolean), loras: map }
  if (faceSwap && swap) {
    out.face_swap = true
    out.face_swap_name = swap
  }
  return out
}
