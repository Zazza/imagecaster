import { modelBaseFor } from './fastMode.js'

export const PEOPLE_BASE = 'Z-Image'

function styleModel(preset, overrides) {
  return overrides?.model_name || preset?.model_name || ''
}

export function personOverride(catalog, preset, overrides, people, selectedName) {
  if (!selectedName || peopleState(catalog, 'preset', preset, overrides, people) !== 'ok') return null
  const p = people.find(p => p.name === selectedName)
  return p ? { name: p.name, trigger: p.trigger } : null
}

export function peopleState(catalog, mode, preset, overrides, people) {
  if (!Array.isArray(people) || people.length === 0) return 'no_people'
  if (mode !== 'preset') return 'not_style'
  if (!preset) return 'no_style'
  if (!Array.isArray(catalog?.sd_models)) return 'no_catalog'
  if (modelBaseFor(catalog, styleModel(preset, overrides)) !== PEOPLE_BASE) return 'wrong_model'
  return 'ok'
}
