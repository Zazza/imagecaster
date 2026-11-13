import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

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
  GetDefaultEditGarmentPrompt: vi.fn(() => Promise.resolve('')),
  GetSDModels: vi.fn(() => Promise.resolve([])),
  GetSDLoRAs: vi.fn(() => Promise.resolve([])),
  GetLLMModels: vi.fn(() => Promise.resolve([])),
}))

import App from '../App.vue'
import SettingsPage from '../components/SettingsPage.vue'
import CatalogPage from '../components/CatalogPage.vue'
import {
  serverURL, serverModels, serverModelsKnown, modelActionLoading, activeDownload, resetActiveDownload,
} from '../serverModels.js'
import {
  GetSettings, UpdateSettings, GetModelCatalog, GetServerStatus, GetServerModels, GetServerLLMModels,
} from '../wailsjs/go/main/App.js'
import { setLocale, ru, en } from '../i18n/index.js'
import { api } from '../api.js'

enableAutoUnmount(afterEach)

const appStubs = { SettingsPage: true, UnifiedGeneratePage: true, AppFooter: true, WelcomeModal: true, ServerCheckModal: true }

const CATALOG = {
  llm_generate: [],
  llm_vision: [],
  sd_models: [
    { name: 'sd-b', base: 'Flux', category: 'Realistic', description: 'b', size_gb: 12, url: 'http://x/sd-b.safetensors' },
  ],
  lora: [{ name: 'lora-a', base: 'SDXL', category: 'Style', description: 'l', size_mb: 217, url: 'http://x/lora-a.safetensors' }],
}

let installed

function resetShared() {
  serverURL.value = ''
  for (const k of Object.keys(serverModels)) serverModels[k] = []
  for (const k of Object.keys(serverModelsKnown)) delete serverModelsKnown[k]
  for (const k of Object.keys(modelActionLoading)) delete modelActionLoading[k]
  resetActiveDownload()
}

async function settle() {
  for (let i = 0; i < 6; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

function pendingDownloads() {
  const pending = []
  vi.spyOn(api, 'downloadServerModelStream').mockImplementation((_s, type, _u, filename, onProgress) => new Promise(resolve => {
    onProgress('1 MB / 2 MB', 50)
    pending.push({
      type, filename, resolve: () => { installed[type].push({ name: filename }); resolve() },
    })
  }))
  return pending
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  resetShared()
  localStorage.clear()
  localStorage.setItem('imagecaster_welcome', JSON.stringify({ dismissed: true, seenVersion: '9.9.9' }))
  setLocale('ru')
  installed = { sd: [], lora: [], vae: [], text_encoder: [] }
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('confirm', () => false)
  vi.stubGlobal('alert', () => {})
  GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetModelCatalog.mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(CATALOG))))
  GetServerStatus.mockImplementation(() => Promise.resolve({}))
  GetServerModels.mockImplementation((type) => Promise.resolve([...(installed[type] || [])]))
  GetServerLLMModels.mockImplementation(() => Promise.resolve([]))
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  setLocale('ru')
})

async function mountApp() {
  const w = mount(App, { attachTo: document.body, global: { stubs: appStubs } })
  await settle()
  return w
}

function link(w, key) {
  return w.findAll('.sidebar-link').find(l => l.attributes('title') === ru[key])
}

async function mountCatalog() {
  serverURL.value = 'http://srv:1'
  const w = mount(CatalogPage, { props: { active: false }, attachTo: document.body })
  await settle()
  await w.setProps({ active: true })
  await settle()
  return w
}

async function mountSettingsAdvanced() {
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  await w.findAll('.tabs button').find(b => b.text().trim() === ru['settings.tab_advanced']).trigger('click')
  await settle()
  return w
}

function catRow(w, name) {
  return w.findAll('.cat-row').find(r => r.find('.cat-name').text().trim() === name)
}

function rowButton(row, key) {
  return row.findAll('button').find(b => b.text().trim() === ru[key])
}

async function fillSettingsSDForm(w, url, filename) {
  await w.find('input[placeholder="Download URL"]').setValue(url)
  await w.find('input[placeholder="filename.safetensors"]').setValue(filename)
}

function settingsSDButton(w) {
  return w.findAll('button.btn-primary').find(b => [ru['settings.btn_download_model'], ru['settings.btn_downloading']].includes(b.text().trim()))
}

describe('sidebar menu (M1, M2)', () => {
  it('has no group labels', async () => {
    const w = await mountApp()
    expect(w.findAll('.sidebar-group-label')).toHaveLength(0)
  })

  it('links go in order generate, postprocess, gallery, my images | styles, catalog | settings', async () => {
    const w = await mountApp()
    const nav = w.find('.sidebar-nav')
    const items = nav.element.children
    const seq = [...items].map(el => (el.classList.contains('sidebar-divider') ? '|' : el.getAttribute('title')))
    expect(seq).toEqual([
      ru['app.nav_generate'], ru['app.nav_postprocess'], ru['app.nav_gallery'], ru['app.nav_my_images'], '|',
      ru['app.nav_styles'], ru['app.nav_catalog'], '|',
      ru['app.nav_settings'],
    ])
    expect(w.findAll('.sidebar-divider')).toHaveLength(2)
  })

  it('clicking the catalog link shows the catalog page, marks it active and hides the generate page', async () => {
    const w = await mountApp()
    const catalog = w.findComponent(CatalogPage)
    expect(catalog.exists()).toBe(true)
    expect(catalog.isVisible()).toBe(false)
    expect(w.findComponent({ name: 'UnifiedGeneratePage' }).exists()).toBe(true)

    await link(w, 'app.nav_catalog').trigger('click')
    await settle()

    expect(w.findComponent(CatalogPage).isVisible()).toBe(true)
    expect(w.findComponent(CatalogPage).find('.page-title').text()).toBe(ru['app.nav_catalog'])
    expect(w.findComponent({ name: 'UnifiedGeneratePage' }).exists()).toBe(false)
    expect(link(w, 'app.nav_catalog').classes()).toContain('active')
    expect(link(w, 'app.nav_generate').classes()).not.toContain('active')
  })

  it('opening the catalog page reloads server models', async () => {
    const w = await mountApp()
    const before = GetServerModels.mock.calls.length
    await link(w, 'app.nav_catalog').trigger('click')
    await settle()
    expect(GetServerModels.mock.calls.length).toBeGreaterThan(before)
  })

  it('download progress survives leaving and returning to the catalog page', async () => {
    serverURL.value = 'http://srv:1'
    const pending = pendingDownloads()
    const w = await mountApp()
    await link(w, 'app.nav_catalog').trigger('click')
    await settle()
    const page = w.findComponent(CatalogPage)
    await rowButton(catRow(page, 'lora-a'), 'settings.btn_download_model').trigger('click')
    await settle()
    expect(catRow(page, 'lora-a').find('.dl-progress').exists()).toBe(true)

    await link(w, 'app.nav_settings').trigger('click')
    await settle()
    await link(w, 'app.nav_catalog').trigger('click')
    await settle()

    const row = catRow(w.findComponent(CatalogPage), 'lora-a')
    expect(row.find('.dl-progress').exists()).toBe(true)
    expect(row.find('.dl-progress-pct').text()).toBe('50%')
    pending[0].resolve()
    await settle()
  })
})

describe('settings tabs (M2, M5)', () => {
  it.each([['ru', ru], ['en', en]])('%s: no catalog tab among settings tabs', async (locale) => {
    setLocale(locale)
    const w = mount(SettingsPage, { attachTo: document.body })
    await settle()
    const tabs = w.findAll('.tabs button').map(b => b.text().trim())
    expect(tabs.length).toBeGreaterThan(0)
    for (const label of ['Каталог', 'Catalog']) expect(tabs).not.toContain(label)
  })

  it('removed i18n keys are gone from both locales, catalog nav key exists in both', () => {
    for (const dict of [ru, en]) {
      expect(dict['settings.tab_catalog']).toBeUndefined()
      expect(Object.keys(dict).filter(k => k.startsWith('app.group_'))).toEqual([])
      expect(dict['app.nav_catalog']).toBeTruthy()
    }
  })
})

describe('shared server models and download state (M4)', () => {
  it('SD download started in catalog disables the SD download button in settings while it runs', async () => {
    const pending = pendingDownloads()
    const settings = await mountSettingsAdvanced()
    const catalog = await mountCatalog()
    await fillSettingsSDForm(settings, 'http://x/other.safetensors', 'other.safetensors')
    await settle()
    expect(settingsSDButton(settings).attributes('disabled')).toBeUndefined()

    await rowButton(catRow(catalog, 'sd-b'), 'settings.btn_download_model').trigger('click')
    await settle()

    expect(modelActionLoading.dl_sd).toBeTruthy()
    expect(activeDownload.model).toBe('sd-b')
    expect(settingsSDButton(settings).attributes('disabled')).toBeDefined()
    expect(settingsSDButton(settings).text().trim()).toBe(ru['settings.btn_downloading'])

    pending[0].resolve()
    await settle()
    expect(modelActionLoading.dl_sd).toBeFalsy()
    expect(activeDownload.model).toBe('')
    expect(settingsSDButton(settings).attributes('disabled')).toBeUndefined()
  })

  it('SD download started in settings disables the SD download button in catalog while it runs', async () => {
    const pending = pendingDownloads()
    const settings = await mountSettingsAdvanced()
    const catalog = await mountCatalog()
    const btn = rowButton(catRow(catalog, 'sd-b'), 'settings.btn_download_model')
    expect(btn.attributes('disabled')).toBeUndefined()
    await fillSettingsSDForm(settings, 'http://x/other.safetensors', 'other.safetensors')
    await settle()
    await settingsSDButton(settings).trigger('click')
    await settle()

    expect(activeDownload.type).toBe('sd')
    expect(rowButton(catRow(catalog, 'sd-b'), 'settings.btn_download_model').attributes('disabled')).toBeDefined()

    pending[0].resolve()
    await settle()
    expect(rowButton(catRow(catalog, 'sd-b'), 'settings.btn_download_model').attributes('disabled')).toBeUndefined()
  })

  it('LoRA download started in catalog is reflected in the shared download state', async () => {
    const pending = pendingDownloads()
    const catalog = await mountCatalog()
    await rowButton(catRow(catalog, 'lora-a'), 'settings.btn_download_model').trigger('click')
    await settle()
    expect(modelActionLoading.dl_lora).toBeTruthy()
    expect(activeDownload.type).toBe('lora')
    expect(activeDownload.name).toBe('lora-a.safetensors')
    expect(activeDownload.percent).toBe(50)
    pending[0].resolve()
    await settle()
    expect(modelActionLoading.dl_lora).toBeFalsy()
    expect(activeDownload.type).toBe('')
  })

  it('LoRA downloaded in catalog appears in the settings LoRA list', async () => {
    const pending = pendingDownloads()
    const settings = await mountSettingsAdvanced()
    const catalog = await mountCatalog()
    expect(settings.text()).not.toContain('lora-a.safetensors')
    expect(settings.text()).toContain(ru['settings.no_loras'])

    await rowButton(catRow(catalog, 'lora-a'), 'settings.btn_download_model').trigger('click')
    await settle()
    pending[0].resolve()
    await settle()

    expect(settings.text()).toContain('lora-a.safetensors')
    expect(catRow(catalog, 'lora-a').findAll('.badge-success').map(b => b.text().trim())).toContain(ru['settings.installed'])
  })

  it('SD model downloaded in settings shows as installed in catalog', async () => {
    const pending = pendingDownloads()
    const settings = await mountSettingsAdvanced()
    const catalog = await mountCatalog()
    expect(catRow(catalog, 'sd-b').findAll('.badge-success')).toHaveLength(0)

    await fillSettingsSDForm(settings, 'http://x/sd-b.safetensors', 'sd-b.safetensors')
    await settle()
    await settingsSDButton(settings).trigger('click')
    await settle()
    pending[0].resolve()
    await settle()

    const row = catRow(catalog, 'sd-b')
    expect(row.findAll('.badge-success').map(b => b.text().trim())).toContain(ru['settings.installed'])
    expect(rowButton(row, 'settings.btn_delete_model')).toBeTruthy()
  })
})
