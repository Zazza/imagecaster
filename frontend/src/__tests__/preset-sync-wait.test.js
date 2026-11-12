import { describe, it, expect, vi, afterEach } from 'vitest'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, SyncPresets: vi.fn(), ListPresets: vi.fn() }
})

import { SyncPresets, ListPresets } from '../wailsjs/go/main/App.js'
import { withPresetStatuses, listAvailablePresets, PRESET_SYNC_WAIT_MS } from '../composables/usePresets.js'

afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('preset sync wait', () => {
  it('applies statuses once when the server answers in time', async () => {
    SyncPresets.mockResolvedValue({ statuses: [{ id: 1, installed: false }], added: 0 })
    const apply = vi.fn()
    await withPresetStatuses(apply)
    expect(apply.mock.calls).toEqual([[[{ id: 1, installed: false }]]])
  })

  it('shows everything after the wait, then applies the late answer', async () => {
    vi.useFakeTimers()
    let answer
    SyncPresets.mockReturnValue(new Promise(r => { answer = r }))
    const apply = vi.fn()
    const pending = withPresetStatuses(apply)
    await vi.advanceTimersByTimeAsync(PRESET_SYNC_WAIT_MS)
    await pending
    expect(apply.mock.calls).toEqual([[null]])
    answer({ statuses: [{ id: 2, installed: false }], added: 1 })
    await vi.runAllTimersAsync()
    expect(apply.mock.calls).toEqual([[null], [[{ id: 2, installed: false }]]])
  })

  it('late failure keeps the unfiltered list', async () => {
    vi.useFakeTimers()
    let fail
    SyncPresets.mockReturnValue(new Promise((_, r) => { fail = r }))
    const apply = vi.fn()
    const pending = withPresetStatuses(apply)
    await vi.advanceTimersByTimeAsync(PRESET_SYNC_WAIT_MS)
    await pending
    fail(new Error('down'))
    await vi.runAllTimersAsync()
    expect(apply.mock.calls).toEqual([[null]])
  })

  it('sync error means unknown, nothing hidden', async () => {
    SyncPresets.mockRejectedValue(new Error('server down'))
    ListPresets.mockResolvedValue([{ id: 1 }, { id: 2 }])
    const onList = vi.fn()
    await listAvailablePresets(onList)
    expect(onList.mock.calls[0][0].map(p => p.id)).toEqual([1, 2])
  })

  it('hides presets the sync marks unavailable', async () => {
    SyncPresets.mockResolvedValue({ statuses: [{ id: 1, installed: false }, { id: 2, installed: true }], added: 0 })
    ListPresets.mockResolvedValue([{ id: 1 }, { id: 2 }])
    const onList = vi.fn()
    await listAvailablePresets(onList)
    expect(onList.mock.calls[0][0].map(p => p.id)).toEqual([2])
  })
})
