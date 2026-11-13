import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return {
    ...base,
    GetDefaultEditGarmentPrompt: vi.fn(() => Promise.resolve('')),
    GetSDModels: vi.fn(() => Promise.resolve([])),
    GetSDLoRAs: vi.fn(() => Promise.resolve([])),
    GetLLMModels: vi.fn(() => Promise.resolve([])),
  }
})

import SettingsPage from '../components/SettingsPage.vue'
import { GetSettings, UpdateSettings, GetServerStatus, GetServerProcessLogs } from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const PROCESSES = {
  llm: { name: 'llama-test-proc', status: 'running', category: 'llm' },
  sdcpp: { name: 'sdcpp-test-proc', status: 'running', category: 'sd' },
}

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

async function tick(ms) {
  await vi.advanceTimersByTimeAsync(ms)
  await nextTick()
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSettings.mockImplementation(() => Promise.resolve({ server_url: 'http://127.0.0.1:18080' }))
  GetServerStatus.mockImplementation(() => Promise.resolve({ processes: PROCESSES, health: {}, models: {} }))
  GetServerProcessLogs.mockImplementation(() => Promise.resolve(['log-line-one', 'log-line-two']))
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

async function mountSettings(props = {}) {
  const w = mount(SettingsPage, { attachTo: document.body, props })
  await settle()
  vi.useFakeTimers()
  return w
}

function logsButton(w, procName) {
  const label = w.findAll('span').find(s => s.text().trim() === procName)
  expect(label, `process row ${procName}`).toBeTruthy()
  let el = label.element.parentElement
  let btn = null
  while (el && !btn) {
    btn = [...el.querySelectorAll('button')].find(b => b.textContent.trim() === t('settings.btn_logs'))
    el = el.parentElement
  }
  expect(btn, `logs button for ${procName}`).toBeTruthy()
  return btn
}

async function clickLogs(w, procName) {
  logsButton(w, procName).click()
  await tick(0)
}

describe('settings process logs go through the app binding', () => {
  it.each([
    ['llm process', 'llama-test-proc', 'llm'],
    ['sd process', 'sdcpp-test-proc', 'sdcpp'],
  ])('%s: click calls GetServerProcessLogs(name, 50) and shows lines', async (_, procName, key) => {
    const w = await mountSettings()
    await clickLogs(w, procName)
    expect(GetServerProcessLogs).toHaveBeenCalledTimes(1)
    expect(GetServerProcessLogs).toHaveBeenCalledWith(key, 50)
    expect(w.text()).toContain('log-line-one')
    expect(w.text()).toContain('log-line-two')
  })

  it('does not use fetch from the window', async () => {
    const fetchSpy = vi.fn(() => Promise.reject(new Error('fetch must not be used')))
    vi.stubGlobal('fetch', fetchSpy)
    const w = await mountSettings()
    await clickLogs(w, 'llama-test-proc')
    await tick(4000)
    const logCalls = fetchSpy.mock.calls.filter(c => String(c[0]).includes('/logs'))
    expect(logCalls).toHaveLength(0)
    expect(GetServerProcessLogs).toHaveBeenCalled()
  })
})

describe('settings process logs polling', () => {
  it.each([
    ['2 s', 2000, 2],
    ['4 s', 4000, 3],
    ['10 s', 10000, 6],
    ['1.9 s', 1900, 1],
  ])('while open, refreshes every 2 s (after %s)', async (_, ms, calls) => {
    const w = await mountSettings()
    await clickLogs(w, 'llama-test-proc')
    await tick(ms)
    expect(GetServerProcessLogs).toHaveBeenCalledTimes(calls)
    for (const c of GetServerProcessLogs.mock.calls) expect(c).toEqual(['llm', 50])
  })

  it('shows refreshed lines on the next poll', async () => {
    const w = await mountSettings()
    await clickLogs(w, 'llama-test-proc')
    GetServerProcessLogs.mockImplementation(() => Promise.resolve(['fresh-line-after-poll']))
    await tick(2000)
    expect(w.text()).toContain('fresh-line-after-poll')
  })

  it.each([
    ['second click on the button', async w => { await clickLogs(w, 'llama-test-proc') }],
    ['active becomes false', async w => { await w.setProps({ active: false }); await tick(0) }],
    ['unmount', async w => { w.unmount(); await tick(0) }],
  ])('stops polling on %s', async (_, stop) => {
    const w = await mountSettings()
    await clickLogs(w, 'llama-test-proc')
    await tick(2000)
    expect(GetServerProcessLogs).toHaveBeenCalledTimes(2)
    await stop(w)
    const after = GetServerProcessLogs.mock.calls.length
    await tick(10000)
    expect(GetServerProcessLogs.mock.calls.length).toBe(after)
  })

  it('does not poll before the journal is opened', async () => {
    await mountSettings()
    await tick(10000)
    expect(GetServerProcessLogs).not.toHaveBeenCalled()
  })

  it('does not poll when mounted inactive and journal is not open', async () => {
    await mountSettings({ active: false })
    await tick(10000)
    expect(GetServerProcessLogs).not.toHaveBeenCalled()
  })
})

describe('settings process logs error', () => {
  it.each([
    ['string rejection', 'process not found: llm', 'process not found: llm'],
    ['Error rejection', new Error('server unreachable xyz'), 'server unreachable xyz'],
  ])('%s shows the error text', async (_, rejection, text) => {
    GetServerProcessLogs.mockImplementation(() => Promise.reject(rejection))
    const w = await mountSettings()
    await clickLogs(w, 'llama-test-proc')
    expect(w.text()).toContain(text)
    expect(w.text()).not.toContain('Failed to load logs')
    if (typeof rejection === 'string') {
      expect(w.text()).toContain(t('settings.logs_error', { error: text }))
    }
  })
})
