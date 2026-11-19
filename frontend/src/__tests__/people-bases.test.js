import { describe, it, expect } from 'vitest'
import { TRAIN_BASES, peopleForStyle, peopleState, personOverride } from '../people.js'

const ZIMAGE = { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' }
const MOSS = { name: 'moss_image_sft_bf16', base: 'Moss-Image', filename: 'moss_image_sft_bf16.safetensors' }
const SDXL_A = { name: 'juggernautXL_ragnarokBy', base: 'SDXL' }
const SDXL_B = { name: 'epicrealismXL_pureFix', base: 'SDXL' }
const FLUX2 = {
  name: 'flux-2-klein-4b', base: 'Flux.2',
  variants: [{ filename: 'flux-2-klein-4b-Q4_0.gguf' }],
}
const CHROMA = { name: 'Chroma1-HD', base: 'Chroma', filename: 'Chroma1-HD.gguf' }

const catalog = { sd_models: [ZIMAGE, MOSS, SDXL_A, SDXL_B, FLUX2, CHROMA] }

const sdxlPreset = { id: 1, name: 'XL', model_name: 'juggernautXL_ragnarokBy.safetensors' }
const sdxlOtherPreset = { id: 2, name: 'XL2', model_name: 'epicrealismXL_pureFix.safetensors' }
const fluxPreset = { id: 3, name: 'Klein', model_name: 'flux-2-klein-4b-Q4_0.gguf' }
const zPreset = { id: 4, name: 'Z', model_name: 'z-image-turbo-fp8-aio.gguf' }
const chromaPreset = { id: 5, name: 'Chroma', model_name: 'Chroma1-HD.gguf' }

const Z = { name: 'zkv', trigger: 'zkv_man', gender: 'man', base: 'Z-Image', lora: 'zkv', steps: 1200 }
const LEGACY = { name: 'old', trigger: 'old_man', gender: 'man', lora: 'old', steps: 2000 }
const M = { name: 'mila', trigger: 'mila_woman', gender: 'woman', base: 'Moss-Image', lora: 'mila', steps: 1500 }
const X1 = { name: 'xena', trigger: 'xena_woman', gender: 'woman', base: 'SDXL', lora: 'xena', steps: 1500 }
const X2 = { name: 'xavi', trigger: 'xavi_man', gender: 'man', base: 'SDXL', lora: 'xavi', steps: 1500 }
const K = { name: 'kira', trigger: 'kira_woman', gender: 'woman', base: 'Flux.2', lora: 'kira', steps: 1500 }
const ALL = [Z, LEGACY, M, X1, X2, K]

const names = (list) => list.map(p => p.name).sort()

describe('TRAIN_BASES', () => {
  it('lists the training bases, no Chroma', () => {
    expect([...TRAIN_BASES].sort()).toEqual(['Flux.1', 'Flux.2', 'Moss-Image', 'SDXL', 'Z-Image'])
    expect(TRAIN_BASES).not.toContain('Chroma')
  })
})

describe('peopleForStyle: SDXL and Flux.2', () => {
  it('SDXL style → only SDXL people', () => {
    expect(names(peopleForStyle(catalog, sdxlPreset, {}, ALL))).toEqual(['xavi', 'xena'])
  })

  it('another SDXL checkpoint → the same SDXL people', () => {
    expect(names(peopleForStyle(catalog, sdxlOtherPreset, {}, ALL))).toEqual(['xavi', 'xena'])
  })

  it('Flux.2 style → only Flux.2 people', () => {
    expect(names(peopleForStyle(catalog, fluxPreset, {}, ALL))).toEqual(['kira'])
  })

  it('Z-Image style does not get SDXL or Flux.2 people', () => {
    expect(names(peopleForStyle(catalog, zPreset, {}, ALL))).toEqual(['old', 'zkv'])
  })

  it('override to Flux.2 wins over SDXL style', () => {
    expect(names(peopleForStyle(catalog, sdxlPreset, { model_name: 'flux-2-klein-4b-Q4_0.gguf' }, ALL))).toEqual(['kira'])
  })

  it.each([
    ['SDXL style, no SDXL people', sdxlPreset, [Z, LEGACY, M, K]],
    ['Flux.2 style, no Flux.2 people', fluxPreset, [Z, LEGACY, M, X1]],
    ['Flux.2 style, only people without base', fluxPreset, [LEGACY]],
    ['SDXL style, nobody', sdxlPreset, []],
    ['Chroma style', chromaPreset, ALL],
  ])('%s → empty', (_, preset, people) => {
    expect(peopleForStyle(catalog, preset, {}, people)).toEqual([])
  })
})

describe('peopleState: SDXL and Flux.2', () => {
  it.each([
    ['SDXL style with SDXL people', sdxlPreset, {}, ALL, 'ok'],
    ['Flux.2 style with Flux.2 people', fluxPreset, {}, ALL, 'ok'],
    ['SDXL style without SDXL people', sdxlPreset, {}, [Z, M, K], 'no_people_base'],
    ['Flux.2 style without Flux.2 people', fluxPreset, {}, [Z, LEGACY, X1], 'no_people_base'],
    ['Z-Image overridden to SDXL, SDXL people', zPreset, { model_name: 'juggernautXL_ragnarokBy.safetensors' }, [X1], 'ok'],
    ['Chroma style', chromaPreset, {}, ALL, 'wrong_model'],
  ])('%s → %s', (_, preset, overrides, people, want) => {
    expect(peopleState(catalog, 'preset', preset, overrides, people)).toBe(want)
  })
})

describe('personOverride: SDXL and Flux.2', () => {
  it('SDXL style and SDXL person → override', () => {
    expect(personOverride(catalog, sdxlPreset, {}, ALL, 'xena')).toEqual({ name: 'xena' })
  })

  it('Flux.2 style and Flux.2 person → override', () => {
    expect(personOverride(catalog, fluxPreset, {}, ALL, 'kira')).toEqual({ name: 'kira' })
  })

  it.each([
    ['SDXL style, Flux.2 person', sdxlPreset, 'kira'],
    ['Flux.2 style, SDXL person', fluxPreset, 'xena'],
    ['Flux.2 style, Z-Image person', fluxPreset, 'zkv'],
    ['SDXL style, person without base', sdxlPreset, 'old'],
  ])('%s → null', (_, preset, selected) => {
    expect(personOverride(catalog, preset, {}, ALL, selected)).toBeNull()
  })
})
