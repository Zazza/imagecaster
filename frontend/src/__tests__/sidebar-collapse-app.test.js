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
import { t, setLocale, locale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const stubs = { SettingsPage: true, UnifiedGeneratePage: true, AppFooter: true, WelcomeModal: true, ServerCheckModal: true }
const KEY = 'imagecaster_sidebar'

async function settle() {
  for (let i = 0; i < 5; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

async function mountApp() {
  const w = mount(App, { global: { stubs } })
  await settle()
  return w
}

const sidebar = w => w.find('aside.sidebar')
const footerButtons = w => w.findAll('aside.sidebar .sidebar-footer button')
const isCollapsed = w => sidebar(w).classes().includes('collapsed')

beforeEach(() => {
  vi.clearAllMocks()
  vi.restoreAllMocks()
  localStorage.clear()
  setLocale('ru')
})

describe('sidebar footer layout (condition 1)', () => {
  it('collapse button first, then theme, language, reset', async () => {
    const w = await mountApp()
    const btns = footerButtons(w)
    expect(btns.length).toBe(4)
    expect(btns[0].classes()).toContain('sidebar-collapse-btn')
    expect(btns[1].classes()).not.toContain('sidebar-collapse-btn')
    expect(btns[1].classes()).not.toContain('sidebar-reset-btn')
    expect(btns[2].text().trim()).toMatch(/^(EN|RU)$/)
    expect(btns[3].classes()).toContain('sidebar-reset-btn')
  })

  it('expanded: title and aria-label are app.sidebar_collapse', async () => {
    const w = await mountApp()
    const b = w.find('.sidebar-footer .sidebar-collapse-btn')
    expect(b.attributes('title')).toBe(t('app.sidebar_collapse'))
    expect(b.attributes('aria-label')).toBe(t('app.sidebar_collapse'))
  })

  it('collapsed: title and aria-label are app.sidebar_expand', async () => {
    localStorage.setItem(KEY, '1')
    const w = await mountApp()
    const b = w.find('.sidebar-footer .sidebar-collapse-btn')
    expect(b.attributes('title')).toBe(t('app.sidebar_expand'))
    expect(b.attributes('aria-label')).toBe(t('app.sidebar_expand'))
  })

  it('collapse/expand labels are distinct translated strings, not raw keys', () => {
    expect(t('app.sidebar_collapse')).not.toBe('app.sidebar_collapse')
    expect(t('app.sidebar_expand')).not.toBe('app.sidebar_expand')
    expect(t('app.sidebar_collapse')).not.toBe(t('app.sidebar_expand'))
  })

  it('label follows the toggle', async () => {
    const w = await mountApp()
    const b = w.find('.sidebar-collapse-btn')
    await b.trigger('click')
    expect(b.attributes('title')).toBe(t('app.sidebar_expand'))
    expect(b.attributes('aria-label')).toBe(t('app.sidebar_expand'))
    await b.trigger('click')
    expect(b.attributes('title')).toBe(t('app.sidebar_collapse'))
  })
})

describe('collapse toggle (condition 2)', () => {
  it('click toggles collapsed class and stores 1 / 0', async () => {
    const w = await mountApp()
    expect(isCollapsed(w)).toBe(false)
    await w.find('.sidebar-collapse-btn').trigger('click')
    expect(isCollapsed(w)).toBe(true)
    expect(localStorage.getItem(KEY)).toBe('1')
    await w.find('.sidebar-collapse-btn').trigger('click')
    expect(isCollapsed(w)).toBe(false)
    expect(localStorage.getItem(KEY)).toBe('0')
  })

  it('starting collapsed, click expands and stores 0', async () => {
    localStorage.setItem(KEY, '1')
    const w = await mountApp()
    await w.find('.sidebar-collapse-btn').trigger('click')
    expect(isCollapsed(w)).toBe(false)
    expect(localStorage.getItem(KEY)).toBe('0')
  })
})

describe('initial state from localStorage (condition 3)', () => {
  it.each([
    ['1', true],
    ['0', false],
    [null, false],
  ])('stored %j -> collapsed=%s', async (val, expected) => {
    if (val !== null) localStorage.setItem(KEY, val)
    const w = await mountApp()
    expect(isCollapsed(w)).toBe(expected)
  })

  it('getItem throws: app mounts, menu expanded', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('denied') })
    const w = await mountApp()
    expect(sidebar(w).exists()).toBe(true)
    expect(isCollapsed(w)).toBe(false)
    expect(w.find('.sidebar-collapse-btn').attributes('title')).toBe(t('app.sidebar_collapse'))
  })
})

describe('other footer buttons still work (condition 4)', () => {
  it('language button switches locale both ways', async () => {
    const w = await mountApp()
    const lang = () => footerButtons(w)[2]
    expect(locale.value).toBe('ru')
    await lang().trigger('click')
    expect(locale.value).toBe('en')
    await lang().trigger('click')
    expect(locale.value).toBe('ru')
  })

  it('language button works while collapsed', async () => {
    localStorage.setItem(KEY, '1')
    const w = await mountApp()
    await footerButtons(w)[2].trigger('click')
    expect(locale.value).toBe('en')
  })

  it('reset button asks for confirmation first', async () => {
    const w = await mountApp()
    const reset = w.find('.sidebar-footer .sidebar-reset-btn')
    expect(reset.text()).not.toContain(t('app.confirm'))
    await reset.trigger('click')
    expect(reset.text()).toContain(t('app.confirm'))
  })

  it('theme button toggles data-theme', async () => {
    const w = await mountApp()
    const before = document.documentElement.getAttribute('data-theme')
    await footerButtons(w)[1].trigger('click')
    const after = document.documentElement.getAttribute('data-theme')
    expect(after).toMatch(/^(dark|light)$/)
    expect(after).not.toBe(before)
    await footerButtons(w)[1].trigger('click')
    expect(document.documentElement.getAttribute('data-theme')).not.toBe(after)
  })
})
