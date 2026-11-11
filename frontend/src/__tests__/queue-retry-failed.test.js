import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, RetryQueueJob: vi.fn(() => Promise.resolve(undefined)) }
})

import QueuePanel from '../components/QueuePanel.vue'
import { api } from '../api.js'
import { GetQueue, IsQueuePaused, RetryQueueJob } from '../wailsjs/go/main/App.js'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'

enableAutoUnmount(afterEach)

let seq = 0
function job(status, extra = {}) {
  seq++
  return { id: 1000 + seq, type: 'txt2img', status, created_at: new Date(2026, 10, 11, 10, 0, seq).toISOString(), progress: 0, ...extra }
}

async function mountWith(jobs) {
  GetQueue.mockImplementation(() => Promise.resolve(jobs))
  const w = mount(QueuePanel)
  await flushPromises()
  return w
}

function itemFor(w, j) {
  return w.findAll('.queue-item').filter(i => i.classes().includes('status-' + j.status))
}

beforeEach(() => {
  clearEventMocks()
  GetQueue.mockReset()
  RetryQueueJob.mockReset()
  RetryQueueJob.mockImplementation(() => Promise.resolve(undefined))
  IsQueuePaused.mockImplementation(() => Promise.resolve(false))
})

describe('api.retryQueueJob', () => {
  it('calls RetryQueueJob binding with the job id', async () => {
    await api.retryQueueJob(42)
    expect(RetryQueueJob).toHaveBeenCalledTimes(1)
    expect(RetryQueueJob).toHaveBeenCalledWith(42)
  })

  it('propagates binding rejection to the caller', async () => {
    RetryQueueJob.mockImplementation(() => Promise.reject(new Error('job is not failed')))
    await expect(api.retryQueueJob(7)).rejects.toThrow('job is not failed')
  })
})

describe('queue panel retry button for failed jobs', () => {
  it('failed job has a .qi-retry button', async () => {
    const j = job('failed', { error: 'sd timeout' })
    const w = await mountWith([j])
    expect(itemFor(w, j)[0].find('.qi-retry').exists()).toBe(true)
  })

  it.each(['pending', 'running', 'paused'])('%s job has no .qi-retry button', async (status) => {
    const j = job(status)
    const w = await mountWith([j])
    expect(itemFor(w, j)).toHaveLength(1)
    expect(itemFor(w, j)[0].find('.qi-retry').exists()).toBe(false)
  })

  it('in a mixed queue only failed jobs get .qi-retry', async () => {
    const failedA = job('failed')
    const failedB = job('failed')
    const w = await mountWith([job('running'), job('pending'), failedA, job('paused'), failedB])
    const retryItems = w.findAll('.queue-item').filter(i => i.find('.qi-retry').exists())
    expect(retryItems).toHaveLength(2)
    retryItems.forEach(i => expect(i.classes()).toContain('status-failed'))
  })

  it('failed job keeps its remove button next to retry', async () => {
    const j = job('failed', { error: 'boom' })
    const w = await mountWith([j])
    const item = itemFor(w, j)[0]
    expect(item.find('.qi-retry').exists()).toBe(true)
    expect(item.find('.qi-remove').exists()).toBe(true)
  })

  it('click calls RetryQueueJob with the job id', async () => {
    const j = job('failed')
    const w = await mountWith([job('pending'), j])
    await itemFor(w, j)[0].find('.qi-retry').trigger('click')
    await flushPromises()
    expect(RetryQueueJob).toHaveBeenCalledTimes(1)
    expect(RetryQueueJob).toHaveBeenCalledWith(j.id)
  })

  it('click refreshes the queue after retry resolves', async () => {
    const j = job('failed')
    const w = await mountWith([j])
    const callsBefore = GetQueue.mock.calls.length
    GetQueue.mockImplementation(() => Promise.resolve([{ ...j, status: 'pending', error: '' }]))

    await itemFor(w, j)[0].find('.qi-retry').trigger('click')
    await flushPromises()

    expect(GetQueue.mock.calls.length).toBeGreaterThan(callsBefore)
    const lastGetQueueOrder = GetQueue.mock.invocationCallOrder.at(-1)
    expect(lastGetQueueOrder).toBeGreaterThan(RetryQueueJob.mock.invocationCallOrder[0])
    expect(w.find('.status-failed').exists()).toBe(false)
    expect(w.find('.status-pending').exists()).toBe(true)
    expect(w.find('.qi-retry').exists()).toBe(false)
  })

  it('retry error does not break the panel', async () => {
    RetryQueueJob.mockImplementation(() => Promise.reject(new Error('job is not failed')))
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const j = job('failed')
    const other = job('pending')
    const w = await mountWith([j, other])

    await itemFor(w, j)[0].find('.qi-retry').trigger('click')
    await flushPromises()

    expect(RetryQueueJob).toHaveBeenCalledWith(j.id)
    expect(w.findAll('.queue-item')).toHaveLength(2)
    expect(itemFor(w, j)[0].find('.qi-retry').exists()).toBe(true)
    expect(itemFor(w, j)[0].find('.qi-remove').exists()).toBe(true)
    errSpy.mockRestore()
  })
})

describe('i18n queue.retry', () => {
  it('ru label is «Повторить»', () => {
    expect(ru['queue.retry']).toBe('Повторить')
  })

  it('en label is «Retry»', () => {
    expect(en['queue.retry']).toBe('Retry')
  })

  it('retry button shows the localized label as text or title', async () => {
    const j = job('failed')
    const w = await mountWith([j])
    const btn = itemFor(w, j)[0].find('.qi-retry')
    const label = (btn.text() + ' ' + (btn.attributes('title') || '') + ' ' + (btn.attributes('aria-label') || ''))
    expect([ru['queue.retry'], en['queue.retry']].some(l => l && label.includes(l))).toBe(true)
  })
})
