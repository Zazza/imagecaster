import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'

const materialsPath = resolve(__dirname, '../../../data/presets/materials.json')
const materials = JSON.parse(readFileSync(materialsPath, 'utf8'))

const keyOf = (name) => 'post.effect_' + name.toLowerCase().replace(/[^a-z0-9]/g, '_')

describe('materials.json effect names', () => {
  it('contains presets', () => {
    expect(materials.presets.length).toBeGreaterThan(0)
  })

  for (const p of materials.presets) {
    it(`${p.name} has ru and en names`, () => {
      const key = keyOf(p.name)
      expect(typeof ru[key], `ru ${key}`).toBe('string')
      expect(ru[key].trim(), `ru ${key}`).not.toBe('')
      expect(typeof en[key], `en ${key}`).toBe('string')
      expect(en[key].trim(), `en ${key}`).not.toBe('')
    })
  }
})
