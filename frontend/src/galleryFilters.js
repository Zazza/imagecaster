export const PERIODS = ['all', 'today', 'week', 'month', 'custom']

function startOfLocalDay(d) {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

function parseLocalDate(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(s || ''))
  if (!m) return null
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
}

export function periodRange(period, custom = {}, now = new Date()) {
  const today = startOfLocalDay(now)
  const addDays = (d, n) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n)
  let from = null
  let to = null
  if (period === 'today') from = today
  else if (period === 'week') from = addDays(today, -6)
  else if (period === 'month') from = addDays(today, -29)
  else if (period === 'custom') {
    from = parseLocalDate(custom.from)
    const end = parseLocalDate(custom.to)
    to = end ? addDays(end, 1) : null
  }
  return { date_from: from ? from.toISOString() : '', date_to: to ? to.toISOString() : '' }
}

export function parseCreatedAt(s) {
  const v = String(s || '').trim()
  if (!v) return null
  const iso = /[zZ]|[+-]\d{2}:?\d{2}$/.test(v) ? v.replace(' ', 'T') : v.replace(' ', 'T') + 'Z'
  const d = new Date(iso)
  return isNaN(d.getTime()) ? null : d
}

export function dayKey(createdAt) {
  const d = parseCreatedAt(createdAt)
  if (!d) return ''
  const p = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

export function groupByDay(items) {
  const groups = []
  for (const it of items || []) {
    const key = dayKey(it.created_at)
    const last = groups[groups.length - 1]
    if (last && last.key === key) last.items.push(it)
    else groups.push({ key, items: [it] })
  }
  return groups
}

export function dayTitle(key, { today, yesterday, locale = 'ru' } = {}, now = new Date()) {
  if (!key) return ''
  const d = parseLocalDate(key)
  const t0 = startOfLocalDay(now)
  const diff = Math.round((t0 - d) / 86400000)
  if (diff === 0 && today) return today
  if (diff === 1 && yesterday) return yesterday
  const opts = { day: 'numeric', month: 'long' }
  if (d.getFullYear() !== now.getFullYear()) opts.year = 'numeric'
  return d.toLocaleDateString(locale === 'ru' ? 'ru-RU' : 'en-US', opts)
}
