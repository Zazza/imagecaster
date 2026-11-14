import { describe, it, expect } from 'vitest'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'
import { effectNameKey } from '../effectNames.js'
import { isEffectType } from '../effectTypes.js'

const presetFiles = import.meta.glob(['../../../data/presets/*.json', '../../../internal/serverclient/catalog_presets/*.json'], {
  eager: true, import: 'default',
})

function effectPresets() {
  const out = []
  for (const [file, doc] of Object.entries(presetFiles)) {
    const list = Array.isArray(doc) ? doc : doc.presets || []
    for (const p of list) {
      if (isEffectType(p.preset_type)) out.push({ file: file.replace(/^(\.\.\/)+/, ''), name: p.name })
    }
  }
  return out
}

const effects = effectPresets()

describe('effect preset names are translated', () => {
  it('finds the shipped effect presets', () => {
    expect(effects.length).toBeGreaterThan(50)
    expect(effects.map(e => e.name)).toEqual(expect.arrayContaining(['Portra 400', 'Super 8']))
  })

  it.each([['ru', ru], ['en', en]])('every effect has a %s translation', (_, dict) => {
    const missing = effects.filter(e => !dict[effectNameKey(e.name)]).map(e => `${e.file}: ${e.name} -> ${effectNameKey(e.name)}`)
    expect(missing).toEqual([])
  })
})
