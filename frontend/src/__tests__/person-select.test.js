import { describe, it, expect } from 'vitest'
import { personOverride, peopleState, peopleForStyle } from '../people.js'

const ZIMAGE = {
  name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf',
  variants: [{ filename: 'z_image_turbo-Q4_K.gguf' }, { filename: 'z_image_turbo_bf16.safetensors' }],
}
const FLUX2 = {
  name: 'flux-2-klein-4b', base: 'Flux.2',
  variants: [{ filename: 'flux-2-klein-4b-Q4_0.gguf' }],
}
const SDXL = { name: 'juggernautXL_ragnarokBy', base: 'SDXL' }
const MOSS = { name: 'moss_image_sft_bf16', base: 'Moss-Image', filename: 'moss_image_sft_bf16.safetensors' }

const catalog = { sd_models: [ZIMAGE, FLUX2, SDXL, MOSS] }

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

const mossPreset = { id: 7, name: 'Moss', model_name: 'moss_image_sft_bf16.safetensors' }

const ZPERSON = { name: 'zkv', trigger: 'zkv_man', gender: 'man', base: 'Z-Image', lora: 'zkv', steps: 1200 }
const LEGACY = { name: 'old', trigger: 'old_man', gender: 'man', lora: 'old', steps: 2000 }
const MOSS_A = { name: 'mila', trigger: 'mila_woman', gender: 'woman', base: 'Moss-Image', lora: 'mila', steps: 1500 }
const MOSS_B = { name: 'ivan', trigger: 'ivan_man', gender: 'man', base: 'Moss-Image', lora: 'ivan', steps: 1500 }
const MIXED = [ZPERSON, LEGACY, MOSS_A, MOSS_B]

const names = (list) => list.map(p => p.name).sort()

describe('peopleState: training bases', () => {
  it.each([
    ['moss style, moss people present', 'ok', 'preset', mossPreset, {}, MIXED],
    ['moss style, only z-image people', 'no_people_base', 'preset', mossPreset, {}, [ZPERSON]],
    ['moss style, only people without base', 'no_people_base', 'preset', mossPreset, {}, [LEGACY]],
    ['z-image style, only moss people', 'no_people_base', 'preset', zPreset, {}, [MOSS_A, MOSS_B]],
    ['z-image style, only people without base', 'ok', 'preset', zPreset, {}, [LEGACY]],
    ['z-image style, mixed people', 'ok', 'preset', zPreset, {}, MIXED],
    ['z-image overridden to moss, only z-image people', 'no_people_base', 'preset', zPreset, { model_name: 'moss_image_sft_bf16.safetensors' }, [ZPERSON]],
    ['flux overridden to moss, moss people', 'ok', 'preset', fluxPreset, { model_name: 'moss_image_sft_bf16.safetensors' }, [MOSS_A]],
    ['moss overridden to z-image, only moss people', 'no_people_base', 'preset', mossPreset, { model_name: 'z-image-turbo-fp8-aio.gguf' }, [MOSS_A]],
    ['flux style, mixed people', 'wrong_model', 'preset', fluxPreset, {}, MIXED],
    ['sdxl style, moss people', 'wrong_model', 'preset', sdxlPreset, {}, [MOSS_A]],
    ['unknown model, mixed people', 'wrong_model', 'preset', unknownPreset, {}, MIXED],
    ['style without model, moss people', 'wrong_model', 'preset', noModelPreset, {}, [MOSS_A]],
    ['moss style, nobody trained', 'no_people', 'preset', mossPreset, {}, []],
    ['moss style in compound mode', 'not_style', 'compound', mossPreset, {}, MIXED],
    ['custom mode, moss people', 'not_style', 'custom', mossPreset, {}, [MOSS_A]],
    ['no style, moss people', 'no_style', 'preset', null, {}, [MOSS_A]],
  ])('%s → %s', (_, want, mode, preset, overrides, people) => {
    expect(peopleState(catalog, mode, preset, overrides, people)).toBe(want)
  })

  it('moss style without catalog → no_catalog', () => {
    expect(peopleState(null, 'preset', mossPreset, {}, MIXED)).toBe('no_catalog')
  })
})

describe('peopleForStyle', () => {
  it('moss style → only moss people', () => {
    expect(names(peopleForStyle(catalog, mossPreset, {}, MIXED))).toEqual(['ivan', 'mila'])
  })

  it('z-image style → z-image people and people without base', () => {
    expect(names(peopleForStyle(catalog, zPreset, {}, MIXED))).toEqual(['old', 'zkv'])
  })

  it('z-image variant model → z-image people', () => {
    expect(names(peopleForStyle(catalog, zVariantPreset, {}, MIXED))).toEqual(['old', 'zkv'])
  })

  it('person with base Z-Image and person without base are treated alike', () => {
    expect(names(peopleForStyle(catalog, zPreset, {}, [LEGACY]))).toEqual(['old'])
    expect(peopleForStyle(catalog, mossPreset, {}, [LEGACY])).toEqual([])
  })

  it('override to moss wins over the style model', () => {
    expect(names(peopleForStyle(catalog, zPreset, { model_name: 'moss_image_sft_bf16.safetensors' }, MIXED))).toEqual(['ivan', 'mila'])
  })

  it('override to z-image wins over the moss style model', () => {
    expect(names(peopleForStyle(catalog, mossPreset, { model_name: 'z_image_turbo_bf16.safetensors' }, MIXED))).toEqual(['old', 'zkv'])
  })

  it.each([
    ['moss style, no moss people', mossPreset, {}, [ZPERSON, LEGACY]],
    ['z-image style, only moss people', zPreset, {}, [MOSS_A, MOSS_B]],
    ['flux style', fluxPreset, {}, MIXED],
    ['sdxl style', sdxlPreset, {}, MIXED],
    ['unknown model', unknownPreset, {}, MIXED],
    ['empty people', mossPreset, {}, []],
  ])('%s → empty', (_, preset, overrides, people) => {
    expect(peopleForStyle(catalog, preset, overrides, people)).toEqual([])
  })

  it('keeps person fields', () => {
    expect(peopleForStyle(catalog, mossPreset, {}, [MOSS_A])).toEqual([MOSS_A])
  })

  it('does not mutate inputs', () => {
    const people = structuredClone(MIXED)
    peopleForStyle(catalog, mossPreset, {}, people)
    expect(people).toEqual(MIXED)
  })
})

describe('personOverride: training bases', () => {
  it('moss style and moss person → override', () => {
    expect(personOverride(catalog, mossPreset, {}, MIXED, 'mila')).toEqual({ name: 'mila', trigger: 'mila_woman' })
  })

  it('z-image style and person without base → override', () => {
    expect(personOverride(catalog, zPreset, {}, MIXED, 'old')).toEqual({ name: 'old', trigger: 'old_man' })
  })

  it.each([
    ['moss style, z-image person', mossPreset, {}, 'zkv'],
    ['moss style, person without base', mossPreset, {}, 'old'],
    ['z-image style, moss person', zPreset, {}, 'mila'],
    ['z-image overridden to moss, z-image person', zPreset, { model_name: 'moss_image_sft_bf16.safetensors' }, 'zkv'],
    ['flux style, moss person', fluxPreset, {}, 'mila'],
  ])('%s → null', (_, preset, overrides, selected) => {
    expect(personOverride(catalog, preset, overrides, MIXED, selected)).toBeNull()
  })
})
