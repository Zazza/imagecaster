import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick, defineComponent, h, onMounted } from 'vue'

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
import WelcomeModal from '../components/WelcomeModal.vue'
import LLMSetupModal from '../components/LLMSetupModal.vue'
import { CheckServer } from '../wailsjs/go/main/App.js'
import { WELCOME_KEY } from '../welcome.js'
import { RELEASE_NOTES } from '../releaseNotes.js'
import { setLocale } from '../i18n/index.js'
import { api } from '../api.js'

enableAutoUnmount(afterEach)

const NEEDED = {
  needed: true,
  generate: [{ name: 'text-rec:8b', installed: false, recommended: true, size_gb: 5.2 }],
  vision: [{ name: 'vis-rec:8b', installed: false, recommended: true, size_gb: 6.2 }],
  generate_selected: 'text-rec:8b',
  vision_selected: 'vis-rec:8b',
}

let settingsMounts = 0
const SettingsStub = defineComponent({
  name: 'SettingsPage',
  setup() {
    onMounted(() => { settingsMounts++ })
    return () => h('div', { class: 'settings-stub' })
  },
})

const stubs = { SettingsPage: SettingsStub, UnifiedGeneratePage: true, AppFooter: true, WelcomeModal: true, ServerCheckModal: true, LLMSetupModal: true }

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

function deferred() {
  let resolve
  const promise = new Promise(r => { resolve = r })
  return { promise, resolve }
}

const llmModal = (w) => w.findComponent(LLMSetupModal)
const welcomeModal = (w) => w.findComponent(WelcomeModal)

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  setLocale('ru')
  settingsMounts = 0
  CheckServer.mockResolvedValue({ state: 'ok', url: 'http://s:1', version: '2', missing: [], error: '' })
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('settings page re-reads after the LLM setup window closes (condition 1)', () => {
  it('SettingsPage is mounted once at start and again after LLMSetupModal emits close', async () => {
    welcomeDismissed()
    vi.spyOn(api, 'getLLMSetup').mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(NEEDED))))
    const w = mount(App, { global: { stubs } })
    await settle()
    expect(llmModal(w).exists()).toBe(true)
    expect(settingsMounts).toBe(1)
    llmModal(w).vm.$emit('close')
    await settle()
    expect(llmModal(w).exists()).toBe(false)
    expect(settingsMounts).toBe(2)
  })

  it('no setup window: SettingsPage is mounted only once', async () => {
    welcomeDismissed()
    vi.spyOn(api, 'getLLMSetup').mockImplementation(() => Promise.resolve({ ...NEEDED, needed: false }))
    mount(App, { global: { stubs } })
    await settle()
    expect(settingsMounts).toBe(1)
  })
})

describe('startup order: welcome check before LLM setup check (condition 2)', () => {
  it('fresh install: setup window waits for the welcome check and the welcome window', async () => {
    welcomeDismissed()
    const plan = deferred()
    vi.spyOn(api, 'getDefaultDownloadPlan').mockImplementation(() => plan.promise)
    vi.spyOn(api, 'getServerModels').mockImplementation(() => Promise.resolve([]))
    const setupSpy = vi.spyOn(api, 'getLLMSetup').mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(NEEDED))))
    const w = mount(App, { global: { stubs } })
    await settle()
    expect(setupSpy).not.toHaveBeenCalled()
    expect(llmModal(w).exists()).toBe(false)

    plan.resolve({ items: [{ type: 'sd' }] })
    await settle()
    expect(welcomeModal(w).exists()).toBe(true)
    expect(llmModal(w).exists()).toBe(false)

    welcomeModal(w).vm.$emit('close')
    await settle()
    expect(welcomeModal(w).exists()).toBe(false)
    expect(llmModal(w).exists()).toBe(true)
  })

  it('welcome due: setup resolving first with needed does not show over the welcome window', async () => {
    vi.spyOn(api, 'getDefaultDownloadPlan').mockImplementation(() => Promise.resolve({ items: [{ type: 'sd' }] }))
    vi.spyOn(api, 'getServerModels').mockImplementation(() => Promise.resolve([]))
    vi.spyOn(api, 'getLLMSetup').mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(NEEDED))))
    const w = mount(App, { global: { stubs } })
    await settle()
    expect(welcomeModal(w).exists()).toBe(true)
    expect(llmModal(w).exists()).toBe(false)
    welcomeModal(w).vm.$emit('close')
    await settle()
    expect(llmModal(w).exists()).toBe(true)
  })
})
