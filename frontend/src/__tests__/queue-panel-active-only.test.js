import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import QueuePanel from '../components/QueuePanel.vue'
import { GetQueue, IsQueuePaused } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

let seq = 0
function job(status, extra = {}) {
  seq++
  return { id: 'job-' + status + '-' + seq, type: 'generate', status, created_at: new Date(2026, 10, 11, 10, 0, seq).toISOString(), progress: 0, ...extra }
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
  vi.clearAllMocks()
  clearEventMocks()
  IsQueuePaused.mockImplementation(() => Promise.resolve(false))
})

describe('queue panel shows only current and planned work', () => {
  it.each(['completed', 'cancelled'])('does not list %s jobs', async (status) => {
    const hidden = job(status)
    const visible = job('pending')
    const w = await mountWith([hidden, visible])
    expect(w.findAll('.queue-item')).toHaveLength(1)
    expect(w.find('.status-' + status).exists()).toBe(false)
  })

  it.each(['running', 'pending', 'paused', 'failed'])('lists %s jobs', async (status) => {
    const j = job(status)
    const w = await mountWith([j])
    expect(itemFor(w, j)).toHaveLength(1)
    expect(w.find('.queue-empty').exists()).toBe(false)
  })

  it('mixed queue lists exactly running, pending, paused and failed', async () => {
    const jobs = ['running', 'pending', 'paused', 'failed', 'completed', 'cancelled', 'completed'].map(s => job(s))
    const w = await mountWith(jobs)
    const statuses = w.findAll('.queue-item').map(i => i.classes().find(c => c.startsWith('status-')))
    expect(statuses.sort()).toEqual(['status-failed', 'status-paused', 'status-pending', 'status-running'])
  })

  it('failed job keeps its own remove button', async () => {
    const j = job('failed', { error: 'boom' })
    const w = await mountWith([j])
    const item = itemFor(w, j)[0]
    expect(item.find('.qi-remove').exists()).toBe(true)
  })

  it('has no "Clear finished" button even when finished jobs exist', async () => {
    const w = await mountWith([job('completed'), job('cancelled'), job('failed')])
    expect(w.findAll('.q-btn').map(b => b.attributes('title'))).toEqual([t('queue.pause'), t('queue.cancel_all')])
  })

  it('shows empty text when queue holds only completed/cancelled jobs', async () => {
    const w = await mountWith([job('completed'), job('cancelled')])
    expect(w.findAll('.queue-item')).toHaveLength(0)
    expect(w.text()).toContain(t('queue.empty'))
  })

  it('shows empty text when queue is empty', async () => {
    const w = await mountWith([])
    expect(w.findAll('.queue-item')).toHaveLength(0)
    expect(w.text()).toContain(t('queue.empty'))
  })
})
