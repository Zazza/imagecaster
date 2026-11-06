const KEYS = ['date', 'name']
const DIRS = ['asc', 'desc']

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

function byName(a, b) {
  return collator.compare(a.name || '', b.name || '')
}

function byDate(a, b) {
  const da = a.mod_time || ''
  const db = b.mod_time || ''
  return da < db ? -1 : da > db ? 1 : 0
}

export function normalizeSort(key, dir) {
  if (!KEYS.includes(key) || !DIRS.includes(dir)) return { key: 'date', dir: 'desc' }
  return { key, dir }
}

export function sortEntries(entries, key, dir) {
  const s = normalizeSort(key, dir)
  const sign = s.dir === 'asc' ? 1 : -1
  return [...entries].sort((a, b) => {
    if (!!a.is_dir !== !!b.is_dir) return a.is_dir ? -1 : 1
    if (s.key === 'name') return sign * byName(a, b)
    return sign * byDate(a, b) || byName(a, b)
  })
}
