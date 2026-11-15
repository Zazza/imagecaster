import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import AppFooter from '../components/AppFooter.vue'
import { GetSettings, GetServerStatus, GetLogEntries, GetFooterHeight, SaveWindowLayout } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  GetSettings.mockImplementation(() => Promise.resolve({}))
  GetServerStatus.mockImplementation(() => Promise.resolve({ health: {}, models: {} }))
  GetLogEntries.mockImplementation(() => Promise.resolve([]))
  GetFooterHeight.mockImplementation(() => Promise.resolve(50))
  SaveWindowLayout.mockImplementation(() => Promise.resolve(undefined))
  vi.stubGlobal('innerHeight', 1000)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

async function expandedFooter() {
  const w = mount(AppFooter, { attachTo: document.body })
  await settle()
  await w.findAll('.footer-tab-btn').find(b => b.text().startsWith(t('footer.log'))).trigger('click')
  await settle()
  return w
}

describe('footer panel resize', () => {
  it('collapsed footer has no height style and no resize handle', async () => {
    const w = mount(AppFooter, { attachTo: document.body })
    await settle()
    expect(w.find('.footer-resize-handle').exists()).toBe(false)
    expect(w.find('.app-footer').attributes('style') || '').not.toContain('vh')
  })

  it('expanded footer uses the saved height', async () => {
    const w = await expandedFooter()
    expect(w.find('.app-footer').element.style.flex).toBe('0 0 50vh')
  })

  it('saved height 0 keeps the default 40vh', async () => {
    GetFooterHeight.mockImplementation(() => Promise.resolve(0))
    const w = await expandedFooter()
    expect(w.find('.app-footer').element.style.flex).toBe('0 0 40vh')
  })

  it('dragging up grows the panel and saves the height 500 ms after release', async () => {
    const w = await expandedFooter()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    await w.find('.footer-resize-handle').trigger('mousedown', { clientY: 500 })
    expect(document.body.style.cursor).toBe('ns-resize')
    document.dispatchEvent(new MouseEvent('mousemove', { clientY: 400 }))
    await w.vm.$nextTick()
    expect(w.find('.app-footer').element.style.flex).toBe('0 0 60vh')
    document.dispatchEvent(new MouseEvent('mouseup'))
    expect(document.body.style.cursor).toBe('')
    vi.advanceTimersByTime(499)
    expect(SaveWindowLayout).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(SaveWindowLayout).toHaveBeenCalledWith(60)
  })

  it.each([
    ['above 85', 0, 85],
    ['below 15', 1400, 15],
  ])('height is clamped %s', async (_, clientY, vh) => {
    const w = await expandedFooter()
    await w.find('.footer-resize-handle').trigger('mousedown', { clientY: 500 })
    document.dispatchEvent(new MouseEvent('mousemove', { clientY }))
    await w.vm.$nextTick()
    expect(w.find('.app-footer').element.style.flex).toBe(`0 0 ${vh}vh`)
    document.dispatchEvent(new MouseEvent('mouseup'))
  })

  it('moving the mouse after release does not resize', async () => {
    const w = await expandedFooter()
    await w.find('.footer-resize-handle').trigger('mousedown', { clientY: 500 })
    document.dispatchEvent(new MouseEvent('mouseup'))
    document.dispatchEvent(new MouseEvent('mousemove', { clientY: 100 }))
    await w.vm.$nextTick()
    expect(w.find('.app-footer').element.style.flex).toBe('0 0 50vh')
  })

  it('saves the height before the window closes, not after unmount', async () => {
    const w = await expandedFooter()
    window.dispatchEvent(new Event('beforeunload'))
    expect(SaveWindowLayout).toHaveBeenCalledWith(50)
    w.unmount()
    SaveWindowLayout.mockClear()
    window.dispatchEvent(new Event('beforeunload'))
    expect(SaveWindowLayout).not.toHaveBeenCalled()
  })
})
