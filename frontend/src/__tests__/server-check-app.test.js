import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'

vi.mock('../wailsjs/runtime/runtime', async () => ({
  ...(await import('./mocks/runtime')),
  BrowserOpenURL: vi.fn(),
  WindowSetSystemDefaultTheme: () => {}, WindowSetDarkTheme: () => {}, WindowSetLightTheme: () => {},
  Environment: () => Promise.reject(new Error('no wails')),
  WindowIsMaximised: () => Promise.resolve(false), WindowIsFullscreen: () => Promise.resolve(false),
}))
vi.mock('../wailsjs/go/main/App.js', async () => ({
  ...(await import('./mocks/wails-app')).default,
  CheckServer: vi.fn(() => Promise.resolve({ state: 'ok', url: 'http://s:1', version: '2', missing: [], error: '' })),
  DiscoverServers: vi.fn(() => Promise.resolve([])),
}))

import App from '../App.vue'
import ServerCheckModal from '../components/ServerCheckModal.vue'
import WelcomeModal from '../components/WelcomeModal.vue'
import { CheckServer } from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const stubs = { SettingsPage: true, UnifiedGeneratePage: true, AppFooter: true, WelcomeModal: true, ServerCheckModal: true }

async function settle() {
  for (let i = 0; i < 5; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

async function mountApp(state) {
  CheckServer.mockResolvedValue({ state, url: 'http://s:1', version: '1', missing: [], error: '' })
  const w = mount(App, { global: { stubs } })
  await settle()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  setLocale('ru')
})

describe('App startup server check (condition 4)', () => {
  it('ok: checks the server, no server modal, no banner, welcome flow continues', async () => {
    const w = await mountApp('ok')
    expect(CheckServer).toHaveBeenCalled()
    expect(w.findComponent(ServerCheckModal).exists()).toBe(false)
    expect(w.find('.server-banner').exists()).toBe(false)
    expect(w.findComponent(WelcomeModal).exists()).toBe(true)
  })

  it.each(['not_found', 'not_configured', 'outdated'])('%s: server modal is shown on startup', async (state) => {
    const w = await mountApp(state)
    expect(w.findComponent(ServerCheckModal).exists()).toBe(true)
    expect(w.find('.server-banner').exists()).toBe(false)
  })

  it.each([
    ['not_found', 'server.banner_offline'],
    ['not_configured', 'server.banner_offline'],
    ['outdated', 'server.banner_outdated'],
  ])('%s: after close a banner with %s stays and its button reopens the modal', async (state, key) => {
    const w = await mountApp(state)
    w.findComponent(ServerCheckModal).vm.$emit('close')
    await settle()
    expect(w.findComponent(ServerCheckModal).exists()).toBe(false)
    const banner = w.find('.server-banner')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain(t(key))
    const other = key === 'server.banner_offline' ? 'server.banner_outdated' : 'server.banner_offline'
    expect(banner.text()).not.toContain(t(other))
    await banner.find('button').trigger('click')
    await settle()
    expect(w.findComponent(ServerCheckModal).exists()).toBe(true)
  })

  it('ready from the modal hides it and shows no banner once state is ok', async () => {
    const w = await mountApp('not_found')
    CheckServer.mockResolvedValue({ state: 'ok', url: 'http://s:1', version: '2', missing: [], error: '' })
    const { checkServer } = await import('../serverState.js')
    await checkServer()
    w.findComponent(ServerCheckModal).vm.$emit('ready')
    await settle()
    expect(w.findComponent(ServerCheckModal).exists()).toBe(false)
    expect(w.find('.server-banner').exists()).toBe(false)
  })
})
