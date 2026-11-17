import { describe, it, expect } from 'vitest'
import { personOverride, peopleState } from '../people.js'

const ZIMAGE = {
  name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf',
  variants: [{ filename: 'z_image_turbo-Q4_K.gguf' }, { filename: 'z_image_turbo_bf16.safetensors' }],
}
const FLUX2 = {
  name: 'flux-2-klein-4b', base: 'Flux.2',
  variants: [{ filename: 'flux-2-klein-4b-Q4_0.gguf' }],
}
const SDXL = { name: 'juggernautXL_ragnarokBy', base: 'SDXL' }

const catalog = { sd_models: [ZIMAGE, FLUX2, SDXL] }

const PEOPLE = [
  { name: 'zkv', trigger: 'zkv_man', gender: 'man', base: 'Z-Image', lora: 'zkv', steps: 1200 },
  { name: 'anna', trigger: 'anna_woman', gender: 'woman', base: 'Z-Image', lora: 'anna', steps: 1200 },
]

const zPreset = { id: 1, name: 'Portrait', model_name: 'z-image-turbo-fp8-aio.gguf' }
const zVariantPreset = { id: 2, name: 'Portrait Q4', model_name: 'z_image_turbo-Q4_K.gguf' }
const fluxPreset = { id: 3, name: 'Flux', model_name: 'flux-2-klein-4b-Q4_0.gguf' }
const sdxlPreset = { id: 4, name: 'XL', model_name: 'juggernautXL_ragnarokBy.safetensors' }
const noModelPreset = { id: 5, name: 'Default', model_name: '' }
const unknownPreset = { id: 6, name: 'Unknown', model_name: 'unknown-model.gguf' }

describe('personOverride', () => {
  it('z-image style and known person → name and trigger', () => {
    expect(personOverride(catalog, zPreset, {}, PEOPLE, 'zkv')).toEqual({ name: 'zkv', trigger: 'zkv_man' })
  })

  it('returns only name and trigger of the selected person', () => {
    const r = personOverride(catalog, zPreset, {}, PEOPLE, 'anna')
    expect(r).toEqual({ name: 'anna', trigger: 'anna_woman' })
    expect(Object.keys(r).sort()).toEqual(['name', 'trigger'])
  })

  it('z-image variant model → override', () => {
    expect(personOverride(catalog, zVariantPreset, {}, PEOPLE, 'zkv')).toEqual({ name: 'zkv', trigger: 'zkv_man' })
  })

  it('flux style overridden to z-image → override', () => {
    const overrides = { model_name: 'z_image_turbo_bf16.safetensors' }
    expect(personOverride(catalog, fluxPreset, overrides, PEOPLE, 'zkv')).toEqual({ name: 'zkv', trigger: 'zkv_man' })
  })

  it.each([
    ['flux style', fluxPreset, {}, PEOPLE, 'zkv'],
    ['sdxl style', sdxlPreset, {}, PEOPLE, 'zkv'],
    ['style without model', noModelPreset, {}, PEOPLE, 'zkv'],
    ['unknown model', unknownPreset, {}, PEOPLE, 'zkv'],
    ['z-image style overridden to flux', zPreset, { model_name: 'flux-2-klein-4b-Q4_0.gguf' }, PEOPLE, 'zkv'],
    ['person not in list', zPreset, {}, PEOPLE, 'ivan'],
    ['nothing selected', zPreset, {}, PEOPLE, ''],
    ['null selection', zPreset, {}, PEOPLE, null],
    ['empty people', zPreset, {}, [], 'zkv'],
    ['null people', zPreset, {}, null, 'zkv'],
    ['selection by trigger is not a name', zPreset, {}, PEOPLE, 'zkv_man'],
  ])('%s → null', (_, preset, overrides, people, selected) => {
    expect(personOverride(catalog, preset, overrides, people, selected)).toBeNull()
  })

  it('does not mutate inputs', () => {
    const people = structuredClone(PEOPLE)
    const overrides = { model_name: '' }
    const preset = { ...zPreset }
    personOverride(catalog, preset, overrides, people, 'zkv')
    expect(people).toEqual(PEOPLE)
    expect(overrides).toEqual({ model_name: '' })
    expect(preset).toEqual(zPreset)
  })
})

describe('peopleState', () => {
  it.each([
    ['z-image style', 'preset', zPreset, {}, PEOPLE, 'ok'],
    ['z-image variant', 'preset', zVariantPreset, {}, PEOPLE, 'ok'],
    ['flux overridden to z-image', 'preset', fluxPreset, { model_name: 'z-image-turbo-fp8-aio.gguf' }, PEOPLE, 'ok'],
    ['no people', 'preset', zPreset, {}, [], 'no_people'],
    ['null people', 'preset', zPreset, {}, null, 'no_people'],
    ['no people outranks mode', 'compound', null, null, [], 'no_people'],
    ['compound mode', 'compound', zPreset, {}, PEOPLE, 'not_style'],
    ['custom mode', 'custom', zPreset, {}, PEOPLE, 'not_style'],
    ['no style picked', 'preset', null, {}, PEOPLE, 'no_style'],
    ['flux style', 'preset', fluxPreset, {}, PEOPLE, 'wrong_model'],
    ['sdxl style', 'preset', sdxlPreset, {}, PEOPLE, 'wrong_model'],
    ['style without model', 'preset', noModelPreset, {}, PEOPLE, 'wrong_model'],
    ['z-image overridden to sdxl', 'preset', zPreset, { model_name: 'juggernautXL_ragnarokBy.safetensors' }, PEOPLE, 'wrong_model'],
  ])('%s → %s', (_, mode, preset, overrides, people, want) => {
    expect(peopleState(catalog, mode, preset, overrides, people)).toBe(want)
  })

  it.each([
    ['null catalog', null],
    ['empty catalog', {}],
  ])('%s → no_catalog', (_, cat) => {
    expect(peopleState(cat, 'preset', zPreset, {}, PEOPLE)).toBe('no_catalog')
  })
})
