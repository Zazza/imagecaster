export const GROUP_NONE = '__none__'

const MODEL_EXT = /\.(safetensors|gguf|ckpt|pt|sft)$/i

export function presetSource(p) {
  return p && p.is_bundled ? 'bundled' : 'custom'
}

export function modelLabel(name) {
  const s = String(name || '').trim()
  if (!s) return ''
  const base = s.split(/[\\/]/).pop()
  return base.replace(MODEL_EXT, '')
}

function groupOf(p) {
  const g = String(p.preset_type || '').trim()
  return g || GROUP_NONE
}

function matchSearch(p, q) {
  if (!q) return true
  return String(p.name || '').toLowerCase().includes(q) || String(p.prompt || '').toLowerCase().includes(q)
}

function matches(p, f, skip) {
  const q = String(f.search || '').trim().toLowerCase()
  if (!matchSearch(p, q)) return false
  if (f.source && presetSource(p) !== f.source) return false
  if (skip !== 'models' && f.models && f.models.length > 0 && !f.models.includes(modelLabel(p.model_name))) return false
  if (skip !== 'group' && f.group && groupOf(p) !== f.group) return false
  return true
}

function byName(a, b) {
  return String(a.name || '').localeCompare(String(b.name || ''), undefined, { sensitivity: 'base' })
}

export function filterPresets(list, f) {
  return (list || []).filter(p => matches(p, f || {})).sort(byName)
}

export function facetCounts(list, f) {
  const filters = f || {}
  const models = new Map()
  const groups = new Map()
  for (const p of list || []) {
    if (matches(p, filters, 'models')) {
      const m = modelLabel(p.model_name)
      models.set(m, (models.get(m) || 0) + 1)
    }
    if (matches(p, filters, 'group')) {
      const g = groupOf(p)
      groups.set(g, (groups.get(g) || 0) + 1)
    }
  }
  for (const m of filters.models || []) {
    if (!models.has(m)) models.set(m, 0)
  }
  if (filters.group && !groups.has(filters.group)) groups.set(filters.group, 0)
  return {
    models: [...models].map(([label, count]) => ({ label, count }))
      .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label)),
    groups: [...groups].map(([value, count]) => ({ value, count }))
      .sort((a, b) => a.value.localeCompare(b.value)),
  }
}

export function hasActiveFilters(f) {
  if (!f) return false
  return Boolean(String(f.search || '').trim() || f.source || (f.models && f.models.length > 0) || f.group)
}
