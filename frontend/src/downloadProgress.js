export function formatBytes(b) {
  if (b >= 1073741824) return (b / 1073741824).toFixed(1) + ' GB'
  if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MB'
  if (b >= 1024) return (b / 1024).toFixed(1) + ' KB'
  return b + ' B'
}

function clamp(n) {
  return Math.max(0, Math.min(100, n))
}

function parseJSON(data) {
  if (!data.startsWith('{')) return null
  try {
    const p = JSON.parse(data)
    if (typeof p?.downloaded !== 'number') return null
    const total = Number(p.total) || 0
    if (total <= 0) return { percent: null, text: `${formatBytes(p.downloaded)} / ???` }
    const pct = Number.isFinite(p.percent) ? p.percent : (p.downloaded / total) * 100
    return { percent: clamp(pct), text: `${formatBytes(p.downloaded)} / ${formatBytes(total)}` }
  } catch {
    return null
  }
}

export function parseProgress(data) {
  const s = typeof data === 'string' ? data.trim() : ''
  if (!s) return { percent: null, text: '' }
  const json = parseJSON(s)
  if (json) return json
  const matches = [...s.matchAll(/(\d+(?:\.\d+)?)\s*%/g)]
  const last = matches.length ? parseFloat(matches[matches.length - 1][1]) : null
  return { percent: last === null ? null : clamp(last), text: s }
}
