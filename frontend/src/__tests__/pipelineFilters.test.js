import { describe, it, expect } from 'vitest'
import { pipelineModels, filterPipelines, pipelineModelCounts } from '../pipelineFilters.js'

const STYLES = [
  { id: 1, name: 'Portrait Base', model_name: 'flux/flux1-dev.safetensors' },
  { id: 2, name: 'Anime Pass', model_name: 'sdxl/animagine.gguf' },
  { id: 3, name: 'Detail Up', model_name: 'flux1-dev.safetensors' },
  { id: 4, name: 'Free Style', model_name: '' },
]

const byId = () => new Map(STYLES.map(s => [s.id, { ...s }]))

const step = (preset_id, denoising_strength = 1) => ({ preset_id, denoising_strength })

const list = () => [
  { id: 101, name: 'Beta Flow', description: 'soft MORNING', steps: [step(1, 0.85), step(3, 0.4)] },
  { id: 102, name: 'alpha Chain', description: 'neon look', steps: [step(2), step(1, 0.6)] },
  { id: 103, name: 'Gamma Solo', description: '', steps: [step(4)] },
  { id: 104, name: 'delta Broken', description: '', steps: [step(2), step(999, 0.5)] },
]

const NONE = { search: '', models: [] }
const f = (over = {}) => ({ ...NONE, models: [], ...over })
const ids = (arr) => arr.map(p => p.id)
const count = (counts, label) => counts.find(c => c.label === label)?.count

describe('pipelineModels', () => {
  it('returns model file names without directory and extension, without repeats', () => {
    expect(pipelineModels(list()[0], byId())).toEqual(['flux1-dev'])
  })

  it('lists every model of the chain once', () => {
    expect([...pipelineModels(list()[1], byId())].sort()).toEqual(['animagine', 'flux1-dev'])
  })

  it('style without model gives empty label', () => {
    expect(pipelineModels(list()[2], byId())).toEqual([''])
  })

  it('pipeline without steps has no models', () => {
    expect(pipelineModels({ id: 1, name: 'x', description: '', steps: [] }, byId())).toEqual([])
  })

  it('tolerates null steps', () => {
    expect(pipelineModels({ id: 1, name: 'x', description: '', steps: null }, byId())).toEqual([])
  })

  it('step with deleted style does not break the list of other models', () => {
    expect(pipelineModels(list()[3], byId())).toContain('animagine')
  })
})

describe('filterPipelines', () => {
  it('no filters returns all sorted by name case-insensitive', () => {
    expect(ids(filterPipelines(list(), byId(), f()))).toEqual([102, 101, 104, 103])
  })

  it('empty list gives empty result', () => {
    expect(filterPipelines([], byId(), f({ search: 'x' }))).toEqual([])
  })

  it('search by name is case-insensitive', () => {
    expect(ids(filterPipelines(list(), byId(), f({ search: 'GAMMA' })))).toEqual([103])
  })

  it('search by description', () => {
    expect(ids(filterPipelines(list(), byId(), f({ search: 'Neon' })))).toEqual([102])
    expect(ids(filterPipelines(list(), byId(), f({ search: 'morning' })))).toEqual([101])
  })

  it('search by step style name', () => {
    expect(ids(filterPipelines(list(), byId(), f({ search: 'detail up' })))).toEqual([101])
    expect(ids(filterPipelines(list(), byId(), f({ search: 'anime pass' })))).toEqual([102, 104])
  })

  it('search with no match gives empty result', () => {
    expect(filterPipelines(list(), byId(), f({ search: 'zzz-nothing' }))).toEqual([])
  })

  it('model filter keeps pipeline if any step uses the model', () => {
    expect(ids(filterPipelines(list(), byId(), f({ models: ['flux1-dev'] })))).toEqual([102, 101])
  })

  it('several models combine with OR', () => {
    expect(ids(filterPipelines(list(), byId(), f({ models: ['animagine', ''] })))).toEqual([102, 104, 103])
  })

  it('search and model filters combine with AND', () => {
    expect(ids(filterPipelines(list(), byId(), f({ search: 'beta', models: ['animagine'] })))).toEqual([])
    expect(ids(filterPipelines(list(), byId(), f({ search: 'neon', models: ['flux1-dev'] })))).toEqual([102])
  })

  it('does not mutate the input order', () => {
    const src = list()
    filterPipelines(src, byId(), f())
    expect(ids(src)).toEqual([101, 102, 103, 104])
  })
})

describe('pipelineModelCounts', () => {
  it('counts pipelines per model', () => {
    const c = pipelineModelCounts(list(), byId(), f())
    expect(count(c, 'flux1-dev')).toBe(2)
    expect(count(c, 'animagine')).toBe(2)
    expect(count(c, '')).toBe(1)
  })

  it('counts ignore the model filter itself', () => {
    const c = pipelineModelCounts(list(), byId(), f({ models: ['animagine'] }))
    expect(count(c, 'flux1-dev')).toBe(2)
    expect(count(c, '')).toBe(1)
    expect(count(c, 'animagine')).toBe(2)
  })

  it('counts respect search', () => {
    const c = pipelineModelCounts(list(), byId(), f({ search: 'beta' }))
    expect(count(c, 'flux1-dev')).toBe(1)
    expect(count(c, 'animagine') ?? 0).toBe(0)
  })

  it('selected model with zero matches is still listed with 0', () => {
    const c = pipelineModelCounts(list(), byId(), f({ search: 'gamma', models: ['animagine'] }))
    expect(c.find(x => x.label === 'animagine')).toBeTruthy()
    expect(count(c, 'animagine')).toBe(0)
  })

  it('empty list gives no counts', () => {
    expect(pipelineModelCounts([], byId(), f())).toEqual([])
  })
})
