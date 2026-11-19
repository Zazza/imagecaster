import { describe, it, expect } from 'vitest'
import { TRAIN_BASES, trainBaseLabel, peopleForStyle, peopleState, personOverride } from '../people.js'

const FLUX1 = {
  name: 'flux1-dev-fp8', base: 'Flux.1', filename: 'flux1-dev-fp8.gguf',
  variants: [{ filename: 'flux1-dev-Q4_K_S.gguf' }, { filename: 'flux1-dev-Q8_0.gguf' }],
}
const FLUX2 = { name: 'flux-2-klein-4b', base: 'Flux.2', variants: [{ filename: 'flux-2-klein-4b-Q4_0.gguf' }] }
const ZIMAGE = { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' }

const catalog = { sd_models: [FLUX1, FLUX2, ZIMAGE] }

const flux1Preset = { id: 1, name: 'Dev', model_name: 'flux1-dev-fp8' }
const flux1VariantPreset = { id: 2, name: 'Dev Q8', model_name: 'flux1-dev-Q8_0.gguf' }
const flux2Preset = { id: 3, name: 'Klein', model_name: 'flux-2-klein-4b-Q4_0.gguf' }
const zPreset = { id: 4, name: 'Z', model_name: 'z-image-turbo-fp8-aio.gguf' }

const F1 = { name: 'fiona', trigger: 'fiona_woman', gender: 'woman', base: 'Flux.1', lora: 'fiona', steps: 1500 }
const F2 = { name: 'kira', trigger: 'kira_woman', gender: 'woman', base: 'Flux.2', lora: 'kira', steps: 1500 }
const Z = { name: 'zkv', trigger: 'zkv_man', gender: 'man', base: 'Z-Image', lora: 'zkv', steps: 1200 }
const LEGACY = { name: 'old', trigger: 'old_man', gender: 'man', lora: 'old', steps: 2000 }
const ALL = [F1, F2, Z, LEGACY]

const names = (list) => list.map(p => p.name).sort()

describe('Flux.1 training base', () => {
  it('is in TRAIN_BASES', () => {
    expect(TRAIN_BASES).toContain('Flux.1')
  })

  it('is labelled Flux.1 dev', () => {
    expect(trainBaseLabel('Flux.1')).toBe('Flux.1 dev')
  })

  it('keeps the Flux.2 label', () => {
    expect(trainBaseLabel('Flux.2')).toBe('Flux.2 Klein 4B')
  })
})

describe('peopleForStyle: Flux.1', () => {
  it('Flux.1 style → only Flux.1 people', () => {
    expect(names(peopleForStyle(catalog, flux1Preset, {}, ALL))).toEqual(['fiona'])
  })

  it('Flux.1 variant file → Flux.1 people', () => {
    expect(names(peopleForStyle(catalog, flux1VariantPreset, {}, ALL))).toEqual(['fiona'])
  })

  it('Z-Image style does not get Flux.1 people', () => {
    expect(names(peopleForStyle(catalog, zPreset, {}, ALL))).toEqual(['old', 'zkv'])
  })

  it('Flux.2 style does not get Flux.1 people', () => {
    expect(names(peopleForStyle(catalog, flux2Preset, {}, ALL))).toEqual(['kira'])
  })

  it('override to Flux.1 wins over Z-Image style', () => {
    expect(names(peopleForStyle(catalog, zPreset, { model_name: 'flux1-dev-fp8' }, ALL))).toEqual(['fiona'])
  })

  it.each([
    ['Flux.1 style, no Flux.1 people', flux1Preset, [F2, Z, LEGACY]],
    ['Flux.1 style, nobody', flux1Preset, []],
  ])('%s → empty', (_, preset, people) => {
    expect(peopleForStyle(catalog, preset, {}, people)).toEqual([])
  })
})

describe('peopleState and personOverride: Flux.1', () => {
  it.each([
    ['Flux.1 style with Flux.1 people', flux1Preset, ALL, 'ok'],
    ['Flux.1 style without Flux.1 people', flux1Preset, [F2, Z, LEGACY], 'no_people_base'],
  ])('%s', (_, preset, people, want) => {
    expect(peopleState(catalog, 'preset', preset, {}, people)).toBe(want)
  })

  it('Flux.1 style and Flux.1 person → override', () => {
    expect(personOverride(catalog, flux1Preset, {}, ALL, 'fiona')).toEqual({ name: 'fiona' })
  })

  it.each([
    ['Z-Image style, Flux.1 person', zPreset, 'fiona'],
    ['Flux.2 style, Flux.1 person', flux2Preset, 'fiona'],
    ['Flux.1 style, Flux.2 person', flux1Preset, 'kira'],
  ])('%s → null', (_, preset, selected) => {
    expect(personOverride(catalog, preset, {}, ALL, selected)).toBeNull()
  })
})
