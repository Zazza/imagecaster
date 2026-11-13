import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import AppFooter from '../components/AppFooter.vue'
import { t, setLocale } from '../i18n/index.js'
import { GetSettings, GetServerStatus, GetLogEntries } from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetSettings.mockImplementation(() => Promise.resolve({}))
  GetServerStatus.mockImplementation(() => Promise.resolve({ health: {}, models: {} }))
  GetLogEntries.mockImplementation(() => Promise.resolve([]))
})

function entry(id, message, level = 'info') {
  return { id, level, message, timestamp: '12:00:00' }
}

async function openJournal(w) {
  const btn = w.findAll('button').find(b => b.text().includes(t('footer.log')))
  expect(btn).toBeTruthy()
  await btn.trigger('click')
  await flushPromises()
}

function occurrences(w, text) {
  return w.text().split(text).length - 1
}

describe('footer journal history', () => {
  it.each([
    ['single entry before mount', [entry(1, 'history-only-alpha')], ['history-only-alpha']],
    ['several entries before mount', [entry(1, 'history-first-beta'), entry(2, 'history-second-gamma', 'error')], ['history-first-beta', 'history-second-gamma']],
  ])('%s', async (_, history, visible) => {
    GetLogEntries.mockImplementation(() => Promise.resolve(history))
    const w = mount(AppFooter)
    await flushPromises()
    await openJournal(w)
    expect(GetLogEntries).toHaveBeenCalled()
    for (const msg of visible) expect(occurrences(w, msg)).toBe(1)
  })

  it('empty history renders without entries and still shows later events', async () => {
    const w = mount(AppFooter)
    await flushPromises()
    EventsEmit('log:entry', JSON.stringify(entry(1, 'after-load-delta')))
    await flushPromises()
    await openJournal(w)
    expect(occurrences(w, 'after-load-delta')).toBe(1)
  })
})

describe('footer journal: events during history load', () => {
  it.each([
    ['event also present in history (same id)', [entry(1, 'old-epsilon'), entry(2, 'race-zeta')], entry(2, 'race-zeta'), ['old-epsilon', 'race-zeta']],
    ['event not in history', [entry(1, 'old-eta')], entry(2, 'fresh-theta'), ['old-eta', 'fresh-theta']],
    ['event while history is empty', [], entry(5, 'lone-iota'), ['lone-iota']],
  ])('%s', async (_, history, live, visible) => {
    let resolveHistory
    GetLogEntries.mockImplementation(() => new Promise(r => { resolveHistory = r }))
    const w = mount(AppFooter)
    await nextTick()
    await nextTick()
    expect(GetLogEntries).toHaveBeenCalled()
    EventsEmit('log:entry', JSON.stringify(live))
    resolveHistory(history)
    await flushPromises()
    await openJournal(w)
    for (const msg of visible) expect(occurrences(w, msg)).toBe(1)
  })
})
