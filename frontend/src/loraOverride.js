const DEFAULTS = { defaultWeight: 0.6, min: 0, max: 2 }

function opts(o) {
  return { ...DEFAULTS, ...(o || {}) }
}

export function sanitizeWeight(v, o) {
  const { defaultWeight, min, max } = opts(o)
  if (v === '' || v === null || v === undefined) return defaultWeight
  const n = Number(v)
  if (!Number.isFinite(n) || n < min) return defaultWeight
  return Math.round(Math.min(n, max) * 100) / 100
}

export function normalizeLoras(rows, o) {
  if (!Array.isArray(rows)) return []
  const out = []
  for (const r of rows) {
    if (!r || typeof r.name !== 'string' || !r.name.trim()) continue
    out.push({ name: r.name.trim(), weight: sanitizeWeight(r.weight, o) })
  }
  return out
}

export function parseLoras(jsonStr, o) {
  if (!jsonStr) return []
  try {
    return normalizeLoras(JSON.parse(jsonStr), o)
  } catch (e) {
    console.warn('loraOverride.parseLoras:', e)
    return []
  }
}

export function sameLoras(a, b) {
  const x = normalizeLoras(a)
  const y = normalizeLoras(b)
  if (x.length !== y.length) return false
  return x.every((l, i) => l.name === y[i].name && l.weight === y[i].weight)
}

export function buildLorasOverride(original, edited, ignoreAll) {
  if (ignoreAll) return '[]'
  const rows = normalizeLoras(edited)
  if (sameLoras(original, rows)) return null
  return JSON.stringify(rows)
}
