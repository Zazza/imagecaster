import { describe, it, expect } from 'vitest'
import { personKey, peopleGroups, groupLoraForBase, pipelinePerson, pipelineOverride } from '../people.js'

const ZIMAGE = { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' }
const MOSS = { name: 'moss_image_sft_bf16', base: 'Moss-Image', filename: 'moss_image_sft_bf16.safetensors' }
const SDXL = { name: 'juggernautXL_ragnarokBy', base: 'SDXL' }
const FLUX1 = {
  name: 'flux1-dev-fp8', base: 'Flux.1', filename: 'flux1-dev-fp8.gguf',
  variants: [{ filename: 'flux1-dev-Q8_0.gguf' }],
}
const CHROMA = { name: 'Chroma1-HD', base: 'Chroma', filename: 'Chroma1-HD.gguf' }
const catalog = { sd_models: [ZIMAGE, MOSS, SDXL, FLUX1, CHROMA] }

const STEPS = ['z-image-turbo-fp8-aio.gguf', 'juggernautXL_ragnarokBy', 'flux1-dev-Q8_0.gguf']

const N_Z = { name: 'natasha_curated', display_name: 'Наташа', base: '', identity: false }
const N_MOSS = { name: 'natasha_curated_moss', display_name: ' Наташа ', base: 'Moss-Image', identity: true }
const N_F1 = { name: 'natasha_curated_flux1', display_name: 'Наташа', base: 'Flux.1', identity: true }
const ANNA = { name: 'anna', base: 'Z-Image', identity: true }
const CHR = { name: 'chroma_guy', display_name: 'Хром', base: 'Chroma' }

const people = () => [N_Z, ANNA, N_MOSS, CHR, N_F1].map(p => ({ ...p }))

function group(list, key) {
  return peopleGroups(list).find(g => g.key === key)
}

describe('personKey', () => {
  it.each([
    ['display name', { name: 'natasha_curated', display_name: 'Наташа' }, 'Наташа'],
    ['trimmed', { name: 'natasha_curated', display_name: '  Наташа  ' }, 'Наташа'],
    ['no display name', { name: 'anna' }, 'anna'],
    ['empty display name', { name: 'anna', display_name: '' }, 'anna'],
    ['spaces only', { name: 'anna', display_name: '   ' }, 'anna'],
  ])('%s', (_, p, want) => {
    expect(personKey(p)).toBe(want)
  })
})

describe('peopleGroups', () => {
  it('groups by key in order of first appearance', () => {
    const groups = peopleGroups(people())
    expect(groups.map(g => g.key)).toEqual(['Наташа', 'anna'])
    expect(groups[0].loras.map(p => p.name)).toEqual(['natasha_curated', 'natasha_curated_moss', 'natasha_curated_flux1'])
    expect(groups[1].loras.map(p => p.name)).toEqual(['anna'])
  })

  it('skips loras with base outside train bases', () => {
    expect(peopleGroups(people()).map(g => g.key)).not.toContain('Хром')
  })

  it('lora name is not a separate entry when lora has display name', () => {
    expect(peopleGroups(people()).map(g => g.key)).not.toContain('natasha_curated')
  })

  it('old server without display_name: one group per lora', () => {
    const list = [{ name: 'anna', base: 'Z-Image' }, { name: 'olga', base: 'SDXL' }]
    expect(peopleGroups(list).map(g => g.key)).toEqual(['anna', 'olga'])
  })

  it('empty list', () => {
    expect(peopleGroups([])).toEqual([])
  })
})

describe('groupLoraForBase', () => {
  it.each([
    ['empty lora base is Z-Image', 'Z-Image', 'natasha_curated'],
    ['moss', 'Moss-Image', 'natasha_curated_moss'],
    ['flux1', 'Flux.1', 'natasha_curated_flux1'],
  ])('%s', (_, base, want) => {
    expect(groupLoraForBase(group(people(), 'Наташа'), base)?.name).toBe(want)
  })

  it('no lora for base', () => {
    expect(groupLoraForBase(group(people(), 'Наташа'), 'SDXL')).toBeNull()
  })
})

describe('pipelinePerson', () => {
  it('Z-Image, SDXL, Flux.1: lora on steps 1 and 3, step 2 missing', () => {
    expect(pipelinePerson(catalog, STEPS, group(people(), 'Наташа'))).toEqual({
      loras: ['natasha_curated', '', 'natasha_curated_flux1'],
      missing: [2],
      available: true,
    })
  })

  it('no step with lora: unavailable', () => {
    const steps = ['juggernautXL_ragnarokBy', 'Chroma1-HD.gguf']
    expect(pipelinePerson(catalog, steps, group(people(), 'Наташа'))).toEqual({
      loras: ['', ''],
      missing: [1, 2],
      available: false,
    })
  })

  it('model unknown to catalog counts as step without lora', () => {
    const r = pipelinePerson(catalog, ['unknown-model', 'z-image-turbo-fp8-aio.gguf'], group(people(), 'Наташа'))
    expect(r.loras).toEqual(['', 'natasha_curated'])
    expect(r.missing).toEqual([1])
    expect(r.available).toBe(true)
  })

  it('lora without display name keyed by its name', () => {
    expect(pipelinePerson(catalog, STEPS, group(people(), 'anna'))).toEqual({
      loras: ['anna', '', ''],
      missing: [2, 3],
      available: true,
    })
  })
})

describe('pipelineOverride', () => {
  it('unavailable gives null', () => {
    const steps = ['juggernautXL_ragnarokBy']
    expect(pipelineOverride(catalog, steps, group(people(), 'Наташа'), true)).toBeNull()
  })

  it('identity at second lora by step order: face swap uses it', () => {
    expect(pipelineOverride(catalog, STEPS, group(people(), 'Наташа'), true)).toEqual({
      name: 'natasha_curated',
      loras: { 'Z-Image': 'natasha_curated', 'Flux.1': 'natasha_curated_flux1' },
      face_swap: true,
      face_swap_name: 'natasha_curated_flux1',
    })
  })

  it('face swap off: no face_swap flag', () => {
    const ov = pipelineOverride(catalog, STEPS, group(people(), 'Наташа'), false)
    expect(ov.name).toBe('natasha_curated')
    expect(ov.loras).toEqual({ 'Z-Image': 'natasha_curated', 'Flux.1': 'natasha_curated_flux1' })
    expect(ov.face_swap).toBeFalsy()
  })

  it('no lora with identity on pipeline steps: no face swap', () => {
    const list = people().map(p => ({ ...p, identity: false }))
    const ov = pipelineOverride(catalog, STEPS, group(list, 'Наташа'), true)
    expect(ov).not.toBeNull()
    expect(ov.face_swap).toBeFalsy()
    expect(ov.face_swap_name).toBeFalsy()
  })

  it('identity lora on a base not in pipeline is not used', () => {
    const steps = ['z-image-turbo-fp8-aio.gguf', 'juggernautXL_ragnarokBy']
    const ov = pipelineOverride(catalog, steps, group(people(), 'Наташа'), true)
    expect(ov).toEqual({ name: 'natasha_curated', loras: { 'Z-Image': 'natasha_curated' } })
  })

  it('name is first found lora by step order', () => {
    const steps = ['juggernautXL_ragnarokBy', 'flux1-dev-Q8_0.gguf', 'z-image-turbo-fp8-aio.gguf']
    const ov = pipelineOverride(catalog, steps, group(people(), 'Наташа'), true)
    expect(ov.name).toBe('natasha_curated_flux1')
    expect(ov.face_swap_name).toBe('natasha_curated_flux1')
  })
})
