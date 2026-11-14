import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

import QueuePanel from '../components/QueuePanel.vue'
import { useQueue } from '../composables/useQueue.js'
import {
  GetQueue, IsQueuePaused, PauseQueue, ResumeQueue, CancelQueue, CancelQueueJob,
  RemoveQueueJob, ResumePausedQueueJobs,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

let jobs = []

function job(id, status) {
  return { id, type: 'txt2img', status, created_at: '2026-11-11T10:00:00Z', progress: 0, retry_count: 0, max_retries: 3 }
}

async function mountPanel(list, { paused = false } = {}) {
  jobs = list
  IsQueuePaused.mockImplementation(() => Promise.resolve(paused))
  const w = mount(QueuePanel)
  await settle()
  return w
}

function toolbarButton(w, label) {
  return w.findAll('.queue-controls button').find(b => b.text().includes(label))
}

beforeEach(() => {
  clearEventMocks()
  vi.clearAllMocks()
  GetQueue.mockImplementation(() => Promise.resolve(jobs))
  for (const fn of [PauseQueue, ResumeQueue, CancelQueue, CancelQueueJob, RemoveQueueJob, ResumePausedQueueJobs]) {
    fn.mockImplementation(() => Promise.resolve(undefined))
  }
})

describe('queue panel toolbar', () => {
  it('pause sends PauseQueue and the button turns into resume', async () => {
    const w = await mountPanel([job(1, 'pending')])
    await toolbarButton(w, t('queue.pause')).trigger('click')
    await settle()
    expect(PauseQueue).toHaveBeenCalledTimes(1)
    expect(ResumeQueue).not.toHaveBeenCalled()
    expect(toolbarButton(w, t('queue.resume'))).toBeTruthy()
  })

  it('resume on a paused queue sends ResumeQueue and the button turns into pause', async () => {
    const w = await mountPanel([job(1, 'pending')], { paused: true })
    await toolbarButton(w, t('queue.resume')).trigger('click')
    await settle()
    expect(ResumeQueue).toHaveBeenCalledTimes(1)
    expect(PauseQueue).not.toHaveBeenCalled()
    expect(toolbarButton(w, t('queue.pause'))).toBeTruthy()
  })

  it('cancel all needs a confirming second click, then sends CancelQueue and reloads the queue', async () => {
    const w = await mountPanel([job(1, 'running'), job(2, 'pending')])
    await toolbarButton(w, t('queue.cancel_all')).trigger('click')
    await settle()
    expect(CancelQueue).not.toHaveBeenCalled()

    jobs = []
    const loadsBefore = GetQueue.mock.calls.length
    await toolbarButton(w, t('queue.confirm_cancel')).trigger('click')
    await settle()
    expect(CancelQueue).toHaveBeenCalledTimes(1)
    expect(GetQueue.mock.calls.length).toBeGreaterThan(loadsBefore)
    expect(w.find('.queue-empty').text()).toBe(t('queue.empty'))
  })

  it('resume paused jobs sends ResumePausedQueueJobs and shows the refreshed list', async () => {
    const w = await mountPanel([job(5, 'paused')])
    jobs = [job(5, 'pending')]
    await toolbarButton(w, t('queue.resume_paused')).trigger('click')
    await settle()
    expect(ResumePausedQueueJobs).toHaveBeenCalledTimes(1)
    expect(w.findAll('.queue-item.status-pending')).toHaveLength(1)
    expect(w.findAll('.queue-item.status-paused')).toHaveLength(0)
  })
})

describe('queue panel job actions', () => {
  it('remove on a pending job sends RemoveQueueJob(id) and the job disappears', async () => {
    const w = await mountPanel([job(7, 'pending'), job(8, 'pending')])
    jobs = [job(8, 'pending')]
    await w.findAll(`button[title="${t('queue.remove')}"]`)[0].trigger('click')
    await settle()
    expect(RemoveQueueJob).toHaveBeenCalledWith(7)
    expect(w.findAll('.queue-item')).toHaveLength(1)
  })

  it('cancel on the running job sends CancelQueueJob(id) and reloads', async () => {
    const w = await mountPanel([job(3, 'running')])
    jobs = []
    await w.find(`button[title="${t('queue.cancel')}"]`).trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(3)
    expect(RemoveQueueJob).not.toHaveBeenCalled()
    expect(w.findAll('.queue-item')).toHaveLength(0)
  })

  it('a failed remove is logged and the job stays listed', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const err = new Error('job is running')
    RemoveQueueJob.mockImplementation(() => Promise.reject(err))
    const w = await mountPanel([job(9, 'pending')])
    await w.find(`button[title="${t('queue.remove')}"]`).trigger('click')
    await settle()
    expect(warn).toHaveBeenCalledWith(expect.any(String), err)
    expect(w.findAll('.queue-item')).toHaveLength(1)
    warn.mockRestore()
  })

  it('a failed pause is logged and the button keeps saying pause', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const err = new Error('server down')
    PauseQueue.mockImplementation(() => Promise.reject(err))
    const w = await mountPanel([job(1, 'pending')])
    await toolbarButton(w, t('queue.pause')).trigger('click')
    await settle()
    expect(warn).toHaveBeenCalledWith(expect.any(String), err)
    expect(toolbarButton(w, t('queue.pause'))).toBeTruthy()
    warn.mockRestore()
  })
})

describe('useQueue backend subscription', () => {
  const Consumer = defineComponent({
    setup() {
      useQueue()
      return () => h('div')
    },
  })

  async function queueLoadsAfterChange() {
    const before = GetQueue.mock.calls.length
    EventsEmit('queue:changed')
    await settle()
    return GetQueue.mock.calls.length - before
  }

  it('two mounted consumers share one subscription; unmounting both unsubscribes', async () => {
    jobs = []
    const a = mount(Consumer)
    const b = mount(Consumer)
    await settle()
    expect(GetQueue).toHaveBeenCalledTimes(1)

    expect(await queueLoadsAfterChange()).toBe(1)

    a.unmount()
    expect(await queueLoadsAfterChange()).toBe(1)

    b.unmount()
    expect(await queueLoadsAfterChange()).toBe(0)
  })

  it('mounting again after everyone left subscribes anew', async () => {
    jobs = []
    mount(Consumer).unmount()
    await settle()
    const c = mount(Consumer)
    await settle()
    expect(await queueLoadsAfterChange()).toBe(1)
    c.unmount()
  })
})
