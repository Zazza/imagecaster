import { describe, it, expect } from 'vitest'
import {
  MAX_CUSTOM_STEPS, newCustomStep, normalizeCustomStep, customStepsPayload,
  persistCustomSteps, restoreCustomSteps,
} from '../customSteps.js'

const PRESETS = [
  { id: 1, name: 'Plain', loras: '' },
  { id: 2, name: 'Detail', loras: '[{"name":"detail","weight":0.6}]' },
  { id: 3, name: 'Two', loras: '[{"name":"a","weight":0.5},{"name":"b","weight":1}]' },
]

describe('normalizeCustomStep strength clamp', () => {
  it.each([
    [0.3, 0.3],
    [-0.5, 0],
    [0, 0],
    [1, 1],
    [1.7, 1],
    ['0.4', 0.4],
    ['abc', 0.5],
    [undefined, 0.5],
  ])('denoising_strength %j -> %j', (raw, want) => {
    expect(normalizeCustomStep({ preset_id: 1, denoising_strength: raw }).denoising_strength).toBe(want)
  })

  it.each([
    [{ preset_id: 0 }, null],
    [{ preset_id: -3 }, null],
    [{ preset_id: 'x' }, null],
    [{ preset_id: '4' }, 4],
    [null, null],
  ])('preset_id of %j -> %j', (raw, want) => {
    expect(normalizeCustomStep(raw).preset_id).toBe(want)
  })

  it('drops LoRA rows without a name and trims names', () => {
    const s = normalizeCustomStep({ preset_id: 1, loraRows: [{ name: ' x ', weight: 0.7 }, { name: '  ', weight: 1 }, null] })
    expect(s.loraRows).toEqual([{ name: 'x', weight: 0.7 }])
  })
})

describe('restoreCustomSteps', () => {
  it.each([
    ['first step keeps a strength below 0.05', [{ preset_id: 1, denoising_strength: 0.01 }], [0.01]],
    ['later steps get at least 0.05', [{ preset_id: 1, denoising_strength: 0 }, { preset_id: 2, denoising_strength: 0.01 }, { preset_id: 3, denoising_strength: 0.2 }], [0, 0.05, 0.2]],
    ['strength above 1 is clamped', [{ preset_id: 1, denoising_strength: 1 }, { preset_id: 1, denoising_strength: 9 }], [1, 1]],
  ])('%s', (_, raw, strengths) => {
    expect(restoreCustomSteps(raw, PRESETS).map(s => s.denoising_strength)).toEqual(strengths)
  })

  it(`keeps at most ${MAX_CUSTOM_STEPS} steps`, () => {
    const raw = Array.from({ length: MAX_CUSTOM_STEPS + 4 }, () => ({ preset_id: 1, denoising_strength: 0.5 }))
    expect(MAX_CUSTOM_STEPS).toBe(8)
    expect(restoreCustomSteps(raw, PRESETS)).toHaveLength(8)
  })

  it('drops steps whose style was deleted or never chosen', () => {
    const raw = [{ preset_id: 1 }, { preset_id: 99 }, { preset_id: null }, { preset_id: 2 }]
    expect(restoreCustomSteps(raw, PRESETS).map(s => s.preset_id)).toEqual([1, 2])
  })

  it.each([
    ['not an array', 'oops'],
    ['all styles deleted', [{ preset_id: 50 }, { preset_id: 51 }]],
    ['empty list', []],
  ])('%s -> one empty step', (_, raw) => {
    expect(restoreCustomSteps(raw, PRESETS)).toEqual([newCustomStep()])
  })

  it('a step saved without LoRA rows takes the style LoRAs', () => {
    const [s] = restoreCustomSteps([{ preset_id: 3, denoising_strength: 0.5 }], PRESETS)
    expect(s.loraRows).toEqual([{ name: 'a', weight: 0.5 }, { name: 'b', weight: 1 }])
    expect(s.loraIgnoreAll).toBe(false)
  })

  it('a step saved with edited LoRA rows keeps them and the ignore flag', () => {
    const [s] = restoreCustomSteps([{ preset_id: 2, loraRows: [{ name: 'mine', weight: 0.3 }], loraIgnoreAll: true }], PRESETS)
    expect(s.loraRows).toEqual([{ name: 'mine', weight: 0.3 }])
    expect(s.loraIgnoreAll).toBe(true)
  })

  it('persist then restore keeps an edited step as is', () => {
    const steps = [
      { preset_id: 2, denoising_strength: 0.5, loraRows: [{ name: 'other', weight: 0.9 }], loraIgnoreAll: false },
      { preset_id: 1, denoising_strength: 0.35, loraRows: [], loraIgnoreAll: true },
    ]
    const restored = restoreCustomSteps(JSON.parse(JSON.stringify(persistCustomSteps(steps, PRESETS))), PRESETS)
    expect(restored).toEqual(steps)
  })
})

describe('customStepsPayload (what goes to EnqueueCompound steps)', () => {
  it.each([
    ['untouched style LoRAs are not sent', { preset_id: 2, denoising_strength: 0.5, loraRows: [{ name: 'detail', weight: 0.6 }], loraIgnoreAll: false }, { preset_id: 2, denoising_strength: 0.5 }],
    ['ignore all sends an empty list', { preset_id: 2, denoising_strength: 0.4, loraRows: [{ name: 'detail', weight: 0.6 }], loraIgnoreAll: true }, { preset_id: 2, denoising_strength: 0.4, loras: '[]' }],
    ['edited weight sends the full list as JSON', { preset_id: 2, denoising_strength: 0.5, loraRows: [{ name: 'detail', weight: 1.2 }], loraIgnoreAll: false }, { preset_id: 2, denoising_strength: 0.5, loras: '[{"name":"detail","weight":1.2}]' }],
    ['removed style LoRA sends an empty list', { preset_id: 2, denoising_strength: 0.5, loraRows: [], loraIgnoreAll: false }, { preset_id: 2, denoising_strength: 0.5, loras: '[]' }],
    ['style without LoRAs and no rows sends nothing extra', { preset_id: 1, denoising_strength: 0.5, loraRows: [], loraIgnoreAll: false }, { preset_id: 1, denoising_strength: 0.5 }],
  ])('%s', (_, step, want) => {
    expect(customStepsPayload([step], PRESETS)).toEqual([want])
  })

  it('keeps step order', () => {
    const steps = [3, 1, 2].map(id => ({ ...newCustomStep(), preset_id: id, loraRows: [] }))
    expect(customStepsPayload(steps, []).map(s => s.preset_id)).toEqual([3, 1, 2])
  })
})
