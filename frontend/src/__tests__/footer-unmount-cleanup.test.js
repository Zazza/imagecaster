import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

import AppFooter from '../components/AppFooter.vue'
import { GetSettings, GetServerStatus, GetLogEntries, ListSessions } from '../wailsjs/go/main/App.js'
import { settle } from './helpers/index.js'

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetSettings.mockImplementation(() => Promise.resolve({}))
  GetServerStatus.mockImplementation(() => Promise.resolve({ health: {}, models: {} }))
  GetLogEntries.mockImplementation(() => Promise.resolve([]))
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('footer cleanup on unmount', () => {
  it('stops reacting to session events after unmount', async () => {
    const w = mount(AppFooter)
    await settle()
    EventsEmit('session:added')
    await settle()
    const whileMounted = ListSessions.mock.calls.length
    expect(whileMounted).toBeGreaterThan(1)

    w.unmount()
    EventsEmit('session:added')
    EventsEmit('session:switched')
    await settle()
    expect(ListSessions.mock.calls.length).toBe(whileMounted)
  })

  it('stops the 30 s service check after unmount', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    const w = mount(AppFooter)
    await settle()
    vi.advanceTimersByTime(30000)
    await settle()
    const whileMounted = GetServerStatus.mock.calls.length
    expect(whileMounted).toBeGreaterThan(1)

    w.unmount()
    vi.advanceTimersByTime(90000)
    await settle()
    expect(GetServerStatus.mock.calls.length).toBe(whileMounted)
  })
})
