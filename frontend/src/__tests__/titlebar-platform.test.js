import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'

const rt = vi.hoisted(() => ({
  Environment: null,
  WindowIsMaximised: null,
  WindowIsFullscreen: null,
  WindowMinimise: null,
  WindowToggleMaximise: null,
  Quit: null,
}))

vi.mock('../wailsjs/runtime/runtime.js', async () => {
  const base = await import('./mocks/runtime')
  return {
    ...base,
    Environment: (...a) => rt.Environment(...a),
    WindowIsMaximised: (...a) => rt.WindowIsMaximised(...a),
    WindowIsFullscreen: (...a) => rt.WindowIsFullscreen(...a),
    WindowMinimise: (...a) => rt.WindowMinimise(...a),
    WindowToggleMaximise: (...a) => rt.WindowToggleMaximise(...a),
    Quit: (...a) => rt.Quit(...a),
  }
})
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

enableAutoUnmount(afterEach)

const html = () => document.documentElement.classList

async function flush() {
  await flushPromises()
  await flushPromises()
  await flushPromises()
}

async function mountBar(platform) {
  if (platform === null) rt.Environment.mockRejectedValue(new Error('no wails'))
  else rt.Environment.mockResolvedValue({ platform })
  vi.resetModules()
  const { default: TitleBar } = await import('../components/TitleBar.vue')
  const w = mount(TitleBar)
  await flush()
  return w
}

beforeEach(() => {
  document.documentElement.className = ''
  rt.Environment = vi.fn()
  rt.WindowIsMaximised = vi.fn().mockResolvedValue(false)
  rt.WindowIsFullscreen = vi.fn().mockResolvedValue(false)
  rt.WindowMinimise = vi.fn()
  rt.WindowToggleMaximise = vi.fn()
  rt.Quit = vi.fn()
})

describe('TitleBar platform', () => {
  it('darwin: win-mac, no own window buttons, no win-rounded', async () => {
    const w = await mountBar('darwin')
    expect(html().contains('win-mac')).toBe(true)
    expect(html().contains('win-rounded')).toBe(false)
    expect(w.findAll('.titlebar-btn')).toHaveLength(0)
  })

  it('windows: no win-mac, three buttons, no win-rounded', async () => {
    const w = await mountBar('windows')
    expect(html().contains('win-mac')).toBe(false)
    expect(html().contains('win-rounded')).toBe(false)
    expect(w.findAll('.titlebar-btn')).toHaveLength(3)
  })

  it('linux: win-rounded, three buttons, no win-mac', async () => {
    const w = await mountBar('linux')
    expect(html().contains('win-rounded')).toBe(true)
    expect(html().contains('win-mac')).toBe(false)
    expect(w.findAll('.titlebar-btn')).toHaveLength(3)
  })

  it('Environment fails (outside Wails): no platform classes, three buttons', async () => {
    const w = await mountBar(null)
    expect(html().contains('win-mac')).toBe(false)
    expect(html().contains('win-rounded')).toBe(false)
    expect(w.findAll('.titlebar-btn')).toHaveLength(3)
  })
})

describe('TitleBar window state', () => {
  it('win-fullscreen follows WindowIsFullscreen on resize', async () => {
    await mountBar('darwin')
    rt.WindowIsFullscreen.mockResolvedValue(true)
    window.dispatchEvent(new Event('resize'))
    await flush()
    expect(html().contains('win-fullscreen')).toBe(true)
    rt.WindowIsFullscreen.mockResolvedValue(false)
    window.dispatchEvent(new Event('resize'))
    await flush()
    expect(html().contains('win-fullscreen')).toBe(false)
  })

  it('blur adds win-inactive, focus removes it', async () => {
    await mountBar('windows')
    expect(html().contains('win-inactive')).toBe(false)
    window.dispatchEvent(new Event('blur'))
    await flush()
    expect(html().contains('win-inactive')).toBe(true)
    window.dispatchEvent(new Event('focus'))
    await flush()
    expect(html().contains('win-inactive')).toBe(false)
  })
})
