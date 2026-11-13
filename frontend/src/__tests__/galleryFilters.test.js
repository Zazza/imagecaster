import { describe, it, expect } from 'vitest'
import { periodRange, parseCreatedAt, dayKey, groupByDay, dayTitle } from '../galleryFilters.js'

const NOW = new Date(2026, 10, 13, 15, 30, 0)
const midnight = (y, m, d) => new Date(y, m, d).toISOString()
const utcString = (d) => d.toISOString().slice(0, 19).replace('T', ' ')

describe('periodRange', () => {
  it('all gives no bounds', () => {
    const r = periodRange('all', {}, NOW)
    expect(r.date_from || '').toBe('')
    expect(r.date_to || '').toBe('')
  })

  it.each([
    ['today', midnight(2026, 10, 13)],
    ['week', midnight(2026, 10, 7)],
    ['month', midnight(2026, 9, 15)],
  ])('%s starts at local midnight and covers now', (period, from) => {
    const r = periodRange(period, {}, NOW)
    expect(r.date_from).toBe(from)
    if (r.date_to) expect(new Date(r.date_to).getTime()).toBeGreaterThan(NOW.getTime())
  })

  it('week includes today plus the 6 previous days, month 30 days incl. today', () => {
    const DAY = 24 * 3600 * 1000
    const today0 = new Date(2026, 10, 13).getTime()
    expect(Math.round((today0 - new Date(periodRange('week', {}, NOW).date_from).getTime()) / DAY)).toBe(6)
    expect(Math.round((today0 - new Date(periodRange('month', {}, NOW).date_from).getTime()) / DAY)).toBe(29)
  })

  it('custom: from at local midnight, to is the next day exclusive', () => {
    const r = periodRange('custom', { from: '2026-11-01', to: '2026-11-03' }, NOW)
    expect(r.date_from).toBe(midnight(2026, 10, 1))
    expect(r.date_to).toBe(midnight(2026, 10, 4))
  })

  it('custom: same day gives a one-day range, month end rolls over', () => {
    const r = periodRange('custom', { from: '2026-11-30', to: '2026-11-30' }, NOW)
    expect(r.date_from).toBe(midnight(2026, 10, 30))
    expect(r.date_to).toBe(midnight(2026, 11, 1))
  })

  it('custom: missing bounds stay empty', () => {
    expect(periodRange('custom', { from: '2026-11-01' }, NOW)).toMatchObject({ date_from: midnight(2026, 10, 1) })
    expect(periodRange('custom', { from: '2026-11-01' }, NOW).date_to || '').toBe('')
    expect(periodRange('custom', { to: '2026-11-03' }, NOW).date_from || '').toBe('')
    expect(periodRange('custom', { to: '2026-11-03' }, NOW).date_to).toBe(midnight(2026, 10, 4))
    const empty = periodRange('custom', {}, NOW)
    expect(empty.date_from || '').toBe('')
    expect(empty.date_to || '').toBe('')
  })
})

describe('parseCreatedAt and dayKey', () => {
  it('treats the SQLite timestamp as UTC', () => {
    expect(parseCreatedAt('2026-11-12 10:20:30').getTime()).toBe(Date.UTC(2026, 10, 12, 10, 20, 30))
  })

  it('accepts RFC3339', () => {
    expect(parseCreatedAt('2026-11-12T10:20:30Z').getTime()).toBe(Date.UTC(2026, 10, 12, 10, 20, 30))
  })

  it('dayKey is the local calendar date of the UTC timestamp', () => {
    const local = new Date(2026, 10, 12, 0, 30, 0)
    const k = dayKey(utcString(local))
    expect(k).toBe(dayKey(utcString(new Date(2026, 10, 12, 23, 30, 0))))
    expect(k).not.toBe(dayKey(utcString(new Date(2026, 10, 11, 23, 30, 0))))
    expect(k).toContain('2026')
    expect(k).toContain('12')
  })
})

describe('groupByDay', () => {
  it('keeps order and groups consecutive items of the same day', () => {
    const at = (d, h) => utcString(new Date(2026, 10, d, h, 0, 0))
    const items = [
      { id: 5, created_at: at(13, 20) },
      { id: 4, created_at: at(13, 9) },
      { id: 3, created_at: at(12, 22) },
      { id: 2, created_at: at(10, 1) },
      { id: 1, created_at: at(10, 0) },
    ]
    const groups = groupByDay(items)
    expect(groups).toHaveLength(3)
    expect(groups.map(g => g.items.map(i => i.id))).toEqual([[5, 4], [3], [2, 1]])
    expect(groups[0].key).toBe(dayKey(items[0].created_at))
    expect(groups[2].key).toBe(dayKey(items[3].created_at))
  })

  it('empty list gives no groups', () => {
    expect(groupByDay([])).toEqual([])
  })
})

describe('dayTitle', () => {
  const labels = { today: 'Сегодня', yesterday: 'Вчера' }
  const key = (y, m, d) => dayKey(utcString(new Date(y, m, d, 12, 0, 0)))

  it('today and yesterday use the given words', () => {
    expect(dayTitle(key(2026, 10, 13), labels, NOW)).toBe('Сегодня')
    expect(dayTitle(key(2026, 10, 12), labels, NOW)).toBe('Вчера')
  })

  it('older days are a localized day and month', () => {
    const ru = dayTitle(key(2026, 10, 10), { ...labels, locale: 'ru' }, NOW)
    expect(ru).toContain('10')
    expect(ru).toContain('ноября')
    const en = dayTitle(key(2026, 10, 10), { today: 'Today', yesterday: 'Yesterday', locale: 'en' }, NOW)
    expect(en).toContain('10')
    expect(en).toMatch(/Nov/)
  })

  it('yesterday across a month boundary', () => {
    const first = new Date(2026, 11, 1, 9, 0, 0)
    expect(dayTitle(key(2026, 10, 30), labels, first)).toBe('Вчера')
  })
})
