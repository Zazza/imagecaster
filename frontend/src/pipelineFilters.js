import { modelLabel } from './presetFilters.js'

function stepPresets(cp, presetsById) {
  return (cp.steps || []).map(s => presetsById.get(s.preset_id)).filter(Boolean)
}

export function pipelineModels(cp, presetsById) {
  const out = []
  for (const p of stepPresets(cp, presetsById)) {
    const m = modelLabel(p.model_name)
    if (!out.includes(m)) out.push(m)
  }
  return out
}

function matchSearch(cp, presetsById, q) {
  if (!q) return true
  const texts = [cp.name, cp.description, ...stepPresets(cp, presetsById).map(p => p.name)]
  return texts.some(s => String(s || '').toLowerCase().includes(q))
}

function matches(cp, presetsById, f, skipModels) {
  if (!matchSearch(cp, presetsById, String(f.search || '').trim().toLowerCase())) return false
  if (!skipModels && f.models && f.models.length > 0) {
    const ms = pipelineModels(cp, presetsById)
    if (!f.models.some(m => ms.includes(m))) return false
  }
  return true
}

export function filterPipelines(list, presetsById, f) {
  return (list || []).filter(cp => matches(cp, presetsById, f || {}, false))
    .sort((a, b) => String(a.name || '').localeCompare(String(b.name || ''), undefined, { sensitivity: 'base' }))
}

export function pipelineModelCounts(list, presetsById, f) {
  const filters = f || {}
  const counts = new Map()
  for (const cp of list || []) {
    if (!matches(cp, presetsById, filters, true)) continue
    for (const m of pipelineModels(cp, presetsById)) counts.set(m, (counts.get(m) || 0) + 1)
  }
  for (const m of filters.models || []) if (!counts.has(m)) counts.set(m, 0)
  return [...counts].map(([label, count]) => ({ label, count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
}
