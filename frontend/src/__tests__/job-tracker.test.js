import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

import { useJobTracker, parseQueueResult } from '../composables/useJobTracker.js'
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

describe('parseQueueResult idea prompts', () => {
  it.each([
    ['JSON string', JSON.stringify({ session_item_id: 7, idea_prompt: 'tags, cat', idea_negative_prompt: 'dogs' })],
    ['object', { session_item_id: 7, idea_prompt: 'tags, cat', idea_negative_prompt: 'dogs' }],
  ])('%s: returns the prompt the job generated from the idea', (_, raw) => {
    expect(parseQueueResult(raw)).toMatchObject({ sessionItemId: 7, ideaPrompt: 'tags, cat', ideaNegativePrompt: 'dogs' })
  })

  it('a result without idea fields gives empty strings', () => {
    expect(parseQueueResult({ image_base64: 'IMG' })).toMatchObject({ ideaPrompt: '', ideaNegativePrompt: '' })
  })

  it('the queue:completed handler receives the idea prompts', async () => {
    const onCompleted = vi.fn()
    const tracker = mountTracker({ onCompleted })
    tracker.track(9)
    EventsEmit('queue:completed', { job_id: 9, result: JSON.stringify({ image_base64: 'IMG', idea_prompt: 'p', idea_negative_prompt: 'n' }) })
    await settle()
    expect(onCompleted).toHaveBeenCalledTimes(1)
    expect(onCompleted.mock.calls[0][1]).toMatchObject({ ideaPrompt: 'p', ideaNegativePrompt: 'n' })
  })
})
