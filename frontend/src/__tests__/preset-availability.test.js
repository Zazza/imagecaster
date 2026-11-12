import { describe, it, expect } from 'vitest'
import { hiddenPresetIds, filterAvailable } from '../presetAvailability.js'

const status = (id, installed, missing_sd = [], missing_lora = []) => ({ id, installed, missing_sd, missing_lora })

const presets = () => [
  { id: 1, name: 'Klein' },
  { id: 2, name: 'Flux dev' },
  { id: 3, name: 'No model' },
  { id: 4, name: 'Lora missing' },
]

const statuses = () => [
  status(1, true),
  status(2, false, ['flux1-dev-fp8']),
  status(3, true),
  status(4, false, [], ['lora-b']),
]

const ids = (list) => list.map((p) => p.id)

describe('hiddenPresetIds', () => {
  it('returns ids with installed === false', () => {
    const hidden = hiddenPresetIds(statuses())
    expect(hidden).toBeInstanceOf(Set)
    expect([...hidden].sort()).toEqual([2, 4])
  })

  it.each([
    ['null', null],
    ['undefined', undefined],
    ['empty array', []],
  ])('%s statuses give empty set', (_, value) => {
    const hidden = hiddenPresetIds(value)
    expect(hidden).toBeInstanceOf(Set)
    expect(hidden.size).toBe(0)
  })

  it('all installed hides nothing', () => {
    expect(hiddenPresetIds([status(1, true), status(2, true)]).size).toBe(0)
  })
})

describe('filterAvailable', () => {
  it('hides presets missing model or lora, keeps order', () => {
    expect(ids(filterAvailable(presets(), statuses()))).toEqual([1, 3])
  })

  it('showAll keeps every preset', () => {
    expect(ids(filterAvailable(presets(), statuses(), true))).toEqual([1, 2, 3, 4])
  })

  it.each([
    ['null', null],
    ['undefined', undefined],
  ])('%s statuses show all presets', (_, value) => {
    expect(ids(filterAvailable(presets(), value))).toEqual([1, 2, 3, 4])
  })

  it('preset without status is not hidden', () => {
    const list = [...presets(), { id: 5, name: 'Fresh' }]
    expect(ids(filterAvailable(list, statuses()))).toEqual([1, 3, 5])
  })

  it('empty presets give empty list', () => {
    expect(filterAvailable([], statuses())).toEqual([])
  })

  it('does not mutate input', () => {
    const list = presets()
    filterAvailable(list, statuses())
    expect(ids(list)).toEqual([1, 2, 3, 4])
  })
})
