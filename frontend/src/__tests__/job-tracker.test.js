import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

import { useJobTracker } from '../composables/useJobTracker.js'
import { GetQueue } from '../wailsjs/go/main/App.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

function mountTracker(handlers = {}) {
  let tracker
  const Host = defineComponent({
    setup() {
      tracker = useJobTracker(handlers)
      return () => h('div')
    },
  })
  mount(Host)
  return tracker
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
})

describe('useJobTracker queue:changed', () => {
  it('a job tracked while the queue request is in flight is not closed by its stale answer', async () => {
    let answer
    GetQueue.mockImplementation(() => new Promise(r => { answer = r }))
    const onCancelled = vi.fn()
    const tracker = mountTracker({ onCancelled })
    tracker.track(1)
    EventsEmit('queue:changed')
    tracker.track(2)
    answer([{ id: 1, status: 'pending' }])
    await settle()
    expect([...tracker.pending.value]).toEqual([1, 2])
    expect(onCancelled).not.toHaveBeenCalled()
  })

  it('a job cancelled in the panel is closed and resolves as cancelled', async () => {
    GetQueue.mockImplementation(() => Promise.resolve([{ id: 1, status: 'cancelled' }]))
    const onIdle = vi.fn()
    const tracker = mountTracker({ onIdle })
    const outcome = tracker.track(1)
    EventsEmit('queue:changed')
    await settle()
    await expect(outcome).resolves.toMatchObject({ status: 'cancelled' })
    expect(tracker.pending.value.size).toBe(0)
    expect(tracker.done.value).toBe(1)
    expect(onIdle).toHaveBeenCalledTimes(1)
  })
})
