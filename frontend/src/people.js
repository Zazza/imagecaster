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
  const p = peopleForStyle(catalog, preset, overrides, people).find(p => p.name === selectedName)
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
