import { describe, it, expect } from 'vitest'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'

const sources = import.meta.glob(['../**/*.{vue,js}', '!../__tests__/**', '!../wailsjs/**', '!../i18n/**'], {
  query: '?raw', import: 'default', eager: true,
})

function usedKeys() {
  const keys = new Set()
  for (const src of Object.values(sources)) {
    for (const m of src.matchAll(/\bt\(\s*['"]([a-zA-Z_]+\.[a-zA-Z0-9_.]*[a-zA-Z0-9])['"]/g)) keys.add(m[1])
  }
  return [...keys].sort()
}

describe('i18n dictionaries', () => {
  it('ru and en have identical key sets', () => {
    expect(Object.keys(ru).sort()).toEqual(Object.keys(en).sort())
  })

  it('no empty values in ru and en', () => {
    const empty = [
      ...Object.entries(ru).filter(([, v]) => typeof v !== 'string' || !v.trim()).map(([k]) => 'ru:' + k),
      ...Object.entries(en).filter(([, v]) => typeof v !== 'string' || !v.trim()).map(([k]) => 'en:' + k),
    ]
    expect(empty).toEqual([])
  })

  it('every literal key passed to t() in the app exists in the dictionaries', () => {
    const keys = usedKeys()
    expect(keys.length).toBeGreaterThan(100)
    expect(keys.filter(k => !(k in ru))).toEqual([])
  })
})
