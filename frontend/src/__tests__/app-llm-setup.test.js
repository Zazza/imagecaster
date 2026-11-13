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
import LLMSetupModal from '../components/LLMSetupModal.vue'
import { CheckServer } from '../wailsjs/go/main/App.js'
import { WELCOME_KEY } from '../welcome.js'
import { RELEASE_NOTES } from '../releaseNotes.js'
import { setLocale } from '../i18n/index.js'
import { api } from '../api.js'

enableAutoUnmount(afterEach)

const stubs = { SettingsPage: true, UnifiedGeneratePage: true, AppFooter: true, WelcomeModal: true, ServerCheckModal: true, LLMSetupModal: true }

const NEEDED = {
  needed: true,
  generate: [{ name: 'text-rec:8b', installed: false, recommended: true, size_gb: 5.2 }],
  vision: [{ name: 'vis-rec:8b', installed: false, recommended: true, size_gb: 6.2 }],
  generate_selected: 'text-rec:8b',
  vision_selected: 'vis-rec:8b',
}

let setupSpy

async function settle() {
  for (let i = 0; i < 6; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

function welcomeDismissed() {
  localStorage.setItem(WELCOME_KEY, JSON.stringify({ dismissed: true, seenVersion: RELEASE_NOTES[0]?.version || '' }))
}

async function mountApp(state, setup) {
  CheckServer.mockResolvedValue({ state, url: 'http://s:1', version: '2', missing: [], error: '' })
  setupSpy.mockImplementation(() => (setup instanceof Error ? Promise.reject(setup) : Promise.resolve(JSON.parse(JSON.stringify(setup)))))
  const w = mount(App, { global: { stubs } })
  await settle()
  return w
}

const llmModal = (w) => w.findComponent(LLMSetupModal)

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  setLocale('ru')
  setupSpy = vi.spyOn(api, 'getLLMSetup')
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('App shows LLM setup window', () => {
  it('server ok and choice needed: window is shown with setup from getLLMSetup', async () => {
    welcomeDismissed()
    const w = await mountApp('ok', NEEDED)
    expect(w.findComponent(WelcomeModal).exists()).toBe(false)
    expect(setupSpy).toHaveBeenCalled()
    expect(llmModal(w).exists()).toBe(true)
    expect(llmModal(w).props('setup')).toEqual(NEEDED)
  })

  it('is not shown over the welcome window and appears after it is closed', async () => {
    const w = await mountApp('ok', NEEDED)
    expect(w.findComponent(WelcomeModal).exists()).toBe(true)
    expect(llmModal(w).exists()).toBe(false)
    w.findComponent(WelcomeModal).vm.$emit('close')
    await settle()
    expect(w.findComponent(WelcomeModal).exists()).toBe(false)
    expect(llmModal(w).exists()).toBe(true)
  })

  it.each([
    ['choice not needed', { ...NEEDED, needed: false }],
    ['getLLMSetup fails', new Error('server gone')],
  ])('%s: no window', async (_, setup) => {
    welcomeDismissed()
    const w = await mountApp('ok', setup)
    expect(setupSpy).toHaveBeenCalled()
    expect(llmModal(w).exists()).toBe(false)
  })

  it('choice not needed: no window after welcome is closed either', async () => {
    const w = await mountApp('ok', { ...NEEDED, needed: false })
    w.findComponent(WelcomeModal).vm.$emit('close')
    await settle()
    expect(llmModal(w).exists()).toBe(false)
  })

  it.each(['not_found', 'not_configured', 'outdated'])('%s: not shown over the server check window, and «later» does not ask for setup', async (state) => {
    welcomeDismissed()
    const w = await mountApp(state, NEEDED)
    expect(w.findComponent(ServerCheckModal).exists()).toBe(true)
    expect(llmModal(w).exists()).toBe(false)
    w.findComponent(ServerCheckModal).vm.$emit('close')
    await settle()
    expect(setupSpy).not.toHaveBeenCalled()
    expect(llmModal(w).exists()).toBe(false)
  })

  it('server becomes ok from the check window: setup is asked and window is shown', async () => {
    welcomeDismissed()
    const w = await mountApp('not_found', NEEDED)
    expect(llmModal(w).exists()).toBe(false)
    CheckServer.mockResolvedValue({ state: 'ok', url: 'http://s:1', version: '2', missing: [], error: '' })
    const { checkServer } = await import('../serverState.js')
    await checkServer()
    w.findComponent(ServerCheckModal).vm.$emit('ready')
    await settle()
    expect(w.findComponent(ServerCheckModal).exists()).toBe(false)
    expect(setupSpy).toHaveBeenCalled()
    expect(llmModal(w).exists()).toBe(true)
  })

  it('closing the window hides it', async () => {
    welcomeDismissed()
    const w = await mountApp('ok', NEEDED)
    expect(llmModal(w).exists()).toBe(true)
    llmModal(w).vm.$emit('close')
    await settle()
    expect(llmModal(w).exists()).toBe(false)
  })
})
