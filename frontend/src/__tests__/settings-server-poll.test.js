import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import SettingsPage from '../components/SettingsPage.vue'
import {
  GetSettings, GetServerStatus, UpdateSettings, StopServerProcess, StartServerProcess,
} from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

let status = 'running'

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  status = 'running'
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetSettings.mockImplementation(() => Promise.resolve({ server_url: 'http://127.0.0.1:18080' }))
  GetServerStatus.mockImplementation(() => Promise.resolve({
    processes: { sdcpp: { name: 'sd-cpp', status, category: 'sd' } },
    health: {},
    models: {},
  }))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  StopServerProcess.mockImplementation(() => { status = 'stopped'; return Promise.resolve(undefined) })
  StartServerProcess.mockImplementation(() => { status = 'running'; return Promise.resolve(undefined) })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

async function mountPage() {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  return w
}

async function click(w, id) {
  await w.find(`[data-testid="process-row-sdcpp"] [data-testid="${id}"]`).trigger('click')
  await settle()
}

async function tick(ms) {
  vi.advanceTimersByTime(ms)
  await settle()
}

describe('settings: server status polling', () => {
  it('polling after stop refreshes status without writing settings', async () => {
    const w = await mountPage()
    UpdateSettings.mockClear()
    const before = GetServerStatus.mock.calls.length
    await click(w, 'process-stop')
    await tick(2000 * 10)
    expect(GetServerStatus.mock.calls.length - before).toBe(10)
    expect(UpdateSettings).not.toHaveBeenCalled()
  })

  it('two quick actions run one poll, not two in parallel', async () => {
    const w = await mountPage()
    const before = GetServerStatus.mock.calls.length
    await click(w, 'process-stop')
    await click(w, 'process-stop')
    await tick(2000)
    expect(GetServerStatus.mock.calls.length - before).toBe(1)
  })

  it('poll stops on unmount', async () => {
    const w = await mountPage()
    await click(w, 'process-stop')
    await tick(2000)
    w.unmount()
    const after = GetServerStatus.mock.calls.length
    await tick(2000 * 10)
    expect(GetServerStatus.mock.calls.length).toBe(after)
  })
})
