import { describe, it, expect } from 'vitest'
import { presetSource, modelLabel, filterPresets, facetCounts, hasActiveFilters } from '../presetFilters.js'

const NONE = { search: '', source: '', models: [], group: '' }
const f = (over = {}) => ({ ...NONE, models: [], ...over })

const list = () => [
  { id: 1, name: 'beta Portrait', prompt: 'Soft LIGHT portrait', is_bundled: true, model_name: 'flux/flux1-dev.safetensors', preset_type: 'general' },
  { id: 2, name: 'Alpha Anime', prompt: 'anime girl', is_bundled: false, model_name: 'sdxl/animagine.gguf', preset_type: 'anime' },
  { id: 3, name: 'Gamma Any', prompt: 'any model landscape', is_bundled: true, model_name: '', preset_type: '' },
  { id: 4, name: 'delta Photo', prompt: 'photo of a cat', is_bundled: false, model_name: 'flux1-dev.safetensors', preset_type: 'general' },
  { id: 5, name: 'Epsilon Old', prompt: 'old style', is_bundled: true, model_name: 'old/v1-5.ckpt', preset_type: 'anime' },
]

const ids = (arr) => arr.map(p => p.id)
const modelCount = (fc, label) => fc.models.find(m => m.label === label)?.count
const groupCount = (fc, value) => fc.groups.find(g => g.value === value)?.count

describe('presetSource', () => {
  it.each([
    [true, 'bundled'],
    [false, 'custom'],
    [undefined, 'custom'],
  ])('is_bundled=%s -> %s', (v, want) => {
    expect(presetSource({ is_bundled: v })).toBe(want)
  })
})

describe('modelLabel', () => {
  it.each([
    ['flux1-dev.safetensors', 'flux1-dev'],
    ['flux/flux1-dev.safetensors', 'flux1-dev'],
    ['a/b/c/klein-4b.gguf', 'klein-4b'],
    ['old/v1-5.ckpt', 'v1-5'],
    ['upscale.pt', 'upscale'],
    ['model.sft', 'model'],
    ['plain-name', 'plain-name'],
    ['', ''],
    [undefined, ''],
    [null, ''],
  ])('%s -> %s', (name, want) => {
    expect(modelLabel(name)).toBe(want)
  })
})

describe('filterPresets', () => {
  it('no filters returns all, sorted by name case-insensitively', () => {
    expect(ids(filterPresets(list(), f()))).toEqual([2, 1, 4, 5, 3])
  })

  it('empty list returns empty array', () => {
    expect(filterPresets([], f({ search: 'x' }))).toEqual([])
  })

  it('does not mutate the input list order', () => {
    const src = list()
    filterPresets(src, f())
    expect(ids(src)).toEqual([1, 2, 3, 4, 5])
  })

  it('search matches name ignoring case', () => {
    expect(ids(filterPresets(list(), f({ search: 'PORTRAIT' })))).toEqual([1])
  })

  it('search matches prompt ignoring case', () => {
    expect(ids(filterPresets(list(), f({ search: 'soft light' })))).toEqual([1])
    expect(ids(filterPresets(list(), f({ search: 'CAT' })))).toEqual([4])
  })

  it('search with no match returns empty', () => {
    expect(filterPresets(list(), f({ search: 'zzzz-nothing' }))).toEqual([])
  })

  it.each([
    ['bundled', [1, 5, 3]],
    ['custom', [2, 4]],
    ['', [2, 1, 4, 5, 3]],
  ])('source=%s', (source, want) => {
    expect(ids(filterPresets(list(), f({ source })))).toEqual(want)
  })

  it('one model label matches presets regardless of directory', () => {
    expect(ids(filterPresets(list(), f({ models: ['flux1-dev'] })))).toEqual([1, 4])
  })

  it('several models are combined with OR', () => {
    expect(ids(filterPresets(list(), f({ models: ['animagine', 'v1-5'] })))).toEqual([2, 5])
  })

  it('empty model label selects presets without model', () => {
    expect(ids(filterPresets(list(), f({ models: [''] })))).toEqual([3])
  })

  it('group filters by preset_type', () => {
    expect(ids(filterPresets(list(), f({ group: 'anime' })))).toEqual([2, 5])
  })

  it('group __none__ selects presets with empty preset_type', () => {
    expect(ids(filterPresets(list(), f({ group: '__none__' })))).toEqual([3])
  })

  it('unknown group returns empty', () => {
    expect(filterPresets(list(), f({ group: 'missing' }))).toEqual([])
  })

  it('filters combine with AND', () => {
    expect(ids(filterPresets(list(), f({ source: 'custom', models: ['flux1-dev'], group: 'general', search: 'photo' })))).toEqual([4])
    expect(filterPresets(list(), f({ source: 'bundled', models: ['animagine'] }))).toEqual([])
  })
})

describe('facetCounts', () => {
  it('without filters counts every model label, empty model as label ""', () => {
    const fc = facetCounts(list(), f())
    expect(modelCount(fc, 'flux1-dev')).toBe(2)
    expect(modelCount(fc, 'animagine')).toBe(1)
    expect(modelCount(fc, 'v1-5')).toBe(1)
    expect(modelCount(fc, '')).toBe(1)
  })

  it('models sorted by count desc, then by label', () => {
    const fc = facetCounts(list(), f())
    const labels = fc.models.map(m => m.label)
    expect(labels[0]).toBe('flux1-dev')
    const ones = labels.filter(l => ['animagine', 'v1-5'].includes(l))
    expect(ones).toEqual(['animagine', 'v1-5'])
    const counts = fc.models.map(m => m.count)
    expect([...counts].sort((a, b) => b - a)).toEqual(counts)
  })

  it('groups counted by preset_type, empty as __none__, sorted by value', () => {
    const fc = facetCounts(list(), f())
    expect(groupCount(fc, 'general')).toBe(2)
    expect(groupCount(fc, 'anime')).toBe(2)
    expect(groupCount(fc, '__none__')).toBe(1)
    const named = fc.groups.map(g => g.value).filter(v => v !== '__none__')
    expect(named).toEqual(['anime', 'general'])
  })

  it('model counts ignore the model filter but apply the others', () => {
    const fc = facetCounts(list(), f({ models: ['animagine'], source: 'bundled' }))
    expect(modelCount(fc, 'flux1-dev')).toBe(1)
    expect(modelCount(fc, 'v1-5')).toBe(1)
    expect(modelCount(fc, '')).toBe(1)
  })

  it('selected model with zero matches after other filters stays with count 0', () => {
    const fc = facetCounts(list(), f({ models: ['animagine'], source: 'bundled' }))
    expect(fc.models).toContainEqual({ label: 'animagine', count: 0 })
  })

  it('group counts ignore the group filter but apply the others', () => {
    const fc = facetCounts(list(), f({ group: 'general', source: 'bundled' }))
    expect(groupCount(fc, 'general')).toBe(1)
    expect(groupCount(fc, 'anime')).toBe(1)
    expect(groupCount(fc, '__none__')).toBe(1)
  })

  it('search applies to both facets', () => {
    const fc = facetCounts(list(), f({ search: 'anime' }))
    expect(modelCount(fc, 'animagine')).toBe(1)
    expect(modelCount(fc, 'flux1-dev') ?? 0).toBe(0)
    expect(groupCount(fc, 'anime')).toBe(1)
    expect(groupCount(fc, 'general') ?? 0).toBe(0)
  })

  it('group filter affects model counts, model filter affects group counts', () => {
    const fc = facetCounts(list(), f({ group: 'anime', models: ['flux1-dev'] }))
    expect(modelCount(fc, 'animagine')).toBe(1)
    expect(modelCount(fc, 'v1-5')).toBe(1)
    expect(modelCount(fc, 'flux1-dev')).toBe(0)
    expect(groupCount(fc, 'general')).toBe(2)
    expect(groupCount(fc, 'anime') ?? 0).toBe(0)
  })

  it('empty list gives empty facets', () => {
    expect(facetCounts([], f())).toEqual({ models: [], groups: [] })
  })
})

describe('hasActiveFilters', () => {
  it('false when nothing is set', () => {
    expect(hasActiveFilters(f())).toBe(false)
  })

  it.each([
    ['search', { search: 'x' }],
    ['source', { source: 'bundled' }],
    ['models', { models: ['flux1-dev'] }],
    ['empty-model chip', { models: [''] }],
    ['group', { group: 'general' }],
    ['group none', { group: '__none__' }],
  ])('true when %s is set', (_, over) => {
    expect(hasActiveFilters(f(over))).toBe(true)
  })
})
