import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit, EventsOn } from './mocks/runtime'

import AppFooter from '../components/AppFooter.vue'
import {
  GetSettings, GetServerStatus, GetLogEntries, ListSessions, GetSessionItems, GetQueue, GetQueueActiveCount,
  SwitchSession, DeleteSessionItem,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const SESSIONS = [
  { id: 1, name: 'One', item_count: 1, is_active: true },
  { id: 2, name: 'Two', item_count: 0, is_active: false },
]

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('IntersectionObserver', undefined)
  GetSettings.mockImplementation(() => Promise.resolve({}))
  GetServerStatus.mockImplementation(() => Promise.resolve({ health: {}, models: {} }))
  GetLogEntries.mockImplementation(() => Promise.resolve([]))
  ListSessions.mockImplementation(() => Promise.resolve(SESSIONS.map(s => ({ ...s }))))
  GetSessionItems.mockImplementation(() => Promise.resolve([{ id: 5, source: 'generate', created_at: '2026-01-01 10:20:00' }]))
  GetQueueActiveCount.mockImplementation(() => Promise.resolve(3))
})

afterEach(() => vi.unstubAllGlobals())

const tabButton = (w, key) => w.findAll('.footer-tab-btn').find(b => b.text().startsWith(t(key)))

async function mountFooter() {
  const w = mount(AppFooter, { attachTo: document.body })
  await settle()
  return w
}

async function openHistory(w) {
  await tabButton(w, 'footer.session').trigger('click')
  await settle()
}

describe('footer events', () => {
  it('unmounting the footer keeps other components session:added listeners', async () => {
    const other = vi.fn()
    EventsOn('session:added', other)
    const w = await mountFooter()
    w.unmount()
    EventsEmit('session:added')
    expect(other).toHaveBeenCalledTimes(1)
  })

  it('result saved into another folder does not reload or unmark the open folder', async () => {
    GetSessionItems.mockImplementation(() => Promise.resolve([{ id: 5, source: 'generate', created_at: '2026-01-01 10:20:00', is_active: true }]))
    const w = await mountFooter()
    await openHistory(w)
    GetSessionItems.mockClear()
    ListSessions.mockClear()
    EventsEmit('session:added', { id: 9, session_id: 2 })
    EventsEmit('session:active', { id: 9 })
    await settle()
    expect(ListSessions).toHaveBeenCalled()
    expect(GetSessionItems).not.toHaveBeenCalled()
    expect(w.find('.si-card[data-item-id="5"]').classes()).toContain('si-active')
  })

  it('result saved into the open folder reloads it', async () => {
    const w = await mountFooter()
    await openHistory(w)
    GetSessionItems.mockClear()
    EventsEmit('session:added', { id: 9, session_id: 1 })
    await settle()
    expect(GetSessionItems).toHaveBeenCalled()
  })

  it('queue counter comes from the active-count binding, not the full queue', async () => {
    const w = await mountFooter()
    expect(tabButton(w, 'footer.processing').text()).toContain('(3)')
    GetQueueActiveCount.mockImplementation(() => Promise.resolve(0))
    EventsEmit('queue:changed')
    await settle()
    expect(tabButton(w, 'footer.processing').text()).not.toContain('(')
    expect(GetQueue).not.toHaveBeenCalled()
  })

  it('queue listeners are removed on unmount and restored on remount', async () => {
    const w = await mountFooter()
    w.unmount()
    GetQueueActiveCount.mockClear()
    EventsEmit('queue:changed')
    await settle()
    expect(GetQueueActiveCount).not.toHaveBeenCalled()

    const again = await mountFooter()
    GetQueueActiveCount.mockClear()
    GetQueueActiveCount.mockImplementation(() => Promise.resolve(7))
    EventsEmit('queue:completed', { job_id: 1 })
    await settle()
    expect(GetQueueActiveCount).toHaveBeenCalledTimes(1)
    expect(tabButton(again, 'footer.processing').text()).toContain('(7)')
  })

  it('history thumbnails are lazy images by URL without IntersectionObserver', async () => {
    const w = await mountFooter()
    await openHistory(w)
    const img = w.find('.si-card[data-item-id="5"] .si-thumb img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('/api/thumb/5.jpg')
    expect(img.attributes('loading')).toBe('lazy')
  })

  it('session switch failure is shown', async () => {
    SwitchSession.mockImplementation(() => Promise.reject(new Error('db locked')))
    const w = await mountFooter()
    await openHistory(w)
    await w.find('select.session-select').setValue('2')
    await settle()
    expect(w.text()).toContain('db locked')
  })

  it('history item delete failure is shown', async () => {
    DeleteSessionItem.mockImplementation(() => Promise.reject(new Error('cannot delete')))
    const w = await mountFooter()
    await openHistory(w)
    await w.find('.si-del-btn').trigger('click')
    await settle()
    expect(w.text()).toContain('cannot delete')
    expect(w.find('.si-card[data-item-id="5"]').exists()).toBe(true)
  })

  it('log filter keeps entries matched by id after the ring buffer shifts', async () => {
    const w = await mountFooter()
    await tabButton(w, 'footer.log').trigger('click')
    await settle()
    for (let id = 1; id <= 502; id++) {
      EventsEmit('log:entry', JSON.stringify({ id, level: id % 2 ? 'error' : 'info', message: 'm' + id, timestamp: '' }))
    }
    await settle()
    const msgs = w.findAll('.log-entry .log-msg').map(e => e.text())
    expect(msgs.length).toBe(500)
    expect(msgs[0]).toBe('m3')
    expect(msgs.at(-1)).toBe('m502')
    await w.findAll('.filter-btn').find(b => b.text() === t('footer.filter_errors')).trigger('click')
    await settle()
    const errs = w.findAll('.log-entry .log-msg').map(e => e.text())
    expect(errs.length).toBe(250)
    expect(errs.every(m => Number(m.slice(1)) % 2 === 1)).toBe(true)
  })
})
