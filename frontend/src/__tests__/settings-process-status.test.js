import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import SettingsPage from '../components/SettingsPage.vue'
import { GetSettings, GetServerStatus, StopServerProcess, RestartServerProcess, StartServerProcess } from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

let status = 'running'

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetSettings.mockImplementation(() => Promise.resolve({ server_url: 'http://127.0.0.1:18080' }))
  GetServerStatus.mockImplementation(() => Promise.resolve({
    processes: { sdcpp: { name: 'sd-cpp', status, category: 'sd' } },
    health: {},
    models: {},
  }))
  for (const fn of [StopServerProcess, RestartServerProcess, StartServerProcess]) fn.mockImplementation(() => Promise.resolve(undefined))
})

afterEach(() => vi.unstubAllGlobals())

async function processRow(procStatus) {
  status = procStatus
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  const row = w.find('[data-testid="process-row-sdcpp"]')
  expect(row.exists()).toBe(true)
  return row
}

describe('settings: server process status', () => {
  it.each([
    ['running', 'settings.process_running', { start: false, stop: true, restart: true }],
    ['unhealthy', 'settings.process_unhealthy', { start: false, stop: true, restart: true }],
    ['starting', 'settings.process_starting', { start: false, stop: false, restart: false }],
    ['crashed', 'settings.process_crashed', { start: true, stop: false, restart: false }],
    ['stopped', 'settings.process_stopped', { start: true, stop: false, restart: false }],
  ])('%s: shows %s and the matching buttons', async (procStatus, labelKey, buttons) => {
    const row = await processRow(procStatus)
    expect(row.find('[data-testid="process-status"]').text()).toBe(t(labelKey))
    expect(row.find('[data-testid="process-start"]').exists()).toBe(buttons.start)
    expect(row.find('[data-testid="process-stop"]').exists()).toBe(buttons.stop)
    expect(row.find('[data-testid="process-restart"]').exists()).toBe(buttons.restart)
  })

  it('unhealthy process can be stopped and restarted', async () => {
    const row = await processRow('unhealthy')
    await row.find('[data-testid="process-restart"]').trigger('click')
    await settle()
    expect(RestartServerProcess).toHaveBeenCalledWith('sdcpp')
    await row.find('[data-testid="process-stop"]').trigger('click')
    await settle()
    expect(StopServerProcess).toHaveBeenCalledWith('sdcpp')
    expect(StartServerProcess).not.toHaveBeenCalled()
  })
})
