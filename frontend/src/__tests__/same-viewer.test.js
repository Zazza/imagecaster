import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import AppFooter from '../components/AppFooter.vue'
import GeneratePage from '../components/GeneratePage.vue'
import ImageViewer from '../components/ImageViewer.vue'
import { registerHistoryViewer, openInHistoryViewer } from '../historyViewer.js'
import { ListSessions, GetActiveSessionItem, GetSessionItems } from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

let cleanups = []
function register(fn) {
  const off = registerHistoryViewer(fn)
  cleanups.push(off)
  return off
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
})

afterEach(() => {
  cleanups.forEach(off => off())
  cleanups = []
  vi.unstubAllGlobals()
})

describe('historyViewer helper', () => {
  it('resolves false when no opener is registered', async () => {
    expect(await openInHistoryViewer(5)).toBe(false)
  })

  it('resolves true and passes the id when the opener opened the item', async () => {
    const fn = vi.fn(() => true)
    register(fn)
    expect(await openInHistoryViewer(5)).toBe(true)
    expect(fn).toHaveBeenCalledWith(5)
  })

  it('supports async openers', async () => {
    register(async () => true)
    expect(await openInHistoryViewer(5)).toBe(true)
  })

  it.each([[null], [undefined], [0]])('resolves false without calling opener for missing id %s', async (id) => {
    const fn = vi.fn(() => true)
    register(fn)
    expect(await openInHistoryViewer(id)).toBe(false)
    expect(fn).not.toHaveBeenCalled()
  })

  it('resolves false when the opener returns false', async () => {
    register(() => false)
    expect(await openInHistoryViewer(5)).toBe(false)
  })

  it('resolves false when the opener throws or rejects', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const off = register(() => { throw new Error('boom') })
    expect(await openInHistoryViewer(5)).toBe(false)
    off()
    register(() => Promise.reject(new Error('boom')))
    expect(await openInHistoryViewer(5)).toBe(false)
    warn.mockRestore()
  })

  it('unregister removes the opener', async () => {
    const off = register(() => true)
    off()
    expect(await openInHistoryViewer(5)).toBe(false)
  })

  it('unregister of a replaced opener does not remove the newer one', async () => {
    const offOld = register(() => true)
    const newer = vi.fn(() => true)
    register(newer)
    offOld()
    expect(await openInHistoryViewer(5)).toBe(true)
    expect(newer).toHaveBeenCalledWith(5)
  })
})

describe('AppFooter history opener', () => {
  const ITEMS = [
    { id: 301, source: 'generate', created_at: '2026-11-07T10:00:00Z', is_active: false },
    { id: 302, source: 'generate', created_at: '2026-11-07T10:01:00Z', is_active: false },
  ]

  beforeEach(() => {
    ListSessions.mockImplementation(() => Promise.resolve([
      { id: 7, name: 'S', item_count: ITEMS.length, created_at: '', updated_at: '', is_active: true },
    ]))
    GetActiveSessionItem.mockImplementation(() => Promise.resolve(null))
    GetSessionItems.mockImplementation(() => Promise.resolve(ITEMS.map(i => ({ ...i }))))
  })

  it('opens its viewer with actions for an item in the list', async () => {
    const wrapper = mount(AppFooter)
    await settle()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(false)
    const ok = await openInHistoryViewer(302)
    await settle()
    expect(ok).toBe(true)
    const v = wrapper.findComponent(ImageViewer)
    expect(v.exists()).toBe(true)
    expect(v.props('showActions')).toBe(true)
    expect(v.props('item')?.id).toBe(302)
  })

  it('reloads the list once when the item is missing, then opens it', async () => {
    const wrapper = mount(AppFooter)
    await settle()
    const before = GetSessionItems.mock.calls.length
    const fresh = { id: 303, source: 'generate', created_at: '2026-11-07T10:02:00Z', is_active: true }
    GetSessionItems.mockImplementation(() => Promise.resolve([...ITEMS.map(i => ({ ...i })), fresh]))
    const ok = await openInHistoryViewer(303)
    await settle()
    expect(ok).toBe(true)
    expect(GetSessionItems.mock.calls.length - before).toBe(1)
    const v = wrapper.findComponent(ImageViewer)
    expect(v.exists()).toBe(true)
    expect(v.props('item')?.id).toBe(303)
    expect(v.props('showActions')).toBe(true)
  })

  it('returns false and shows no viewer when the item is still absent after reload', async () => {
    const wrapper = mount(AppFooter)
    await settle()
    const before = GetSessionItems.mock.calls.length
    const ok = await openInHistoryViewer(999)
    await settle()
    expect(ok).toBe(false)
    expect(GetSessionItems.mock.calls.length - before).toBe(1)
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(false)
  })

  it('stops opening after unmount', async () => {
    const wrapper = mount(AppFooter)
    await settle()
    wrapper.unmount()
    expect(await openInHistoryViewer(302)).toBe(false)
  })
})

describe('GeneratePage result image', () => {
  async function pageWithResult(result) {
    const wrapper = mount(GeneratePage)
    await settle()
    EventsEmit('queue:completed', { job_id: 'j1', result: JSON.stringify(result) })
    await settle()
    const img = wrapper.findAll('img').find(i => i.attributes('alt') === 'Generated')
    expect(img, 'result image').toBeTruthy()
    return { wrapper, img }
  }

  it('with session_item_id opens the history viewer, not its own', async () => {
    const opener = vi.fn(() => true)
    register(opener)
    const { wrapper, img } = await pageWithResult({ image_base64: 'AAAA', session_item_id: 42 })
    await img.trigger('click')
    await settle()
    expect(opener).toHaveBeenCalledWith(42)
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(false)
  })

  it('without session_item_id opens its own viewer', async () => {
    const opener = vi.fn(() => true)
    register(opener)
    const { wrapper, img } = await pageWithResult({ image_base64: 'AAAA' })
    await img.trigger('click')
    await settle()
    expect(opener).not.toHaveBeenCalled()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(true)
  })

  it('falls back to its own viewer when the opener returns false', async () => {
    const opener = vi.fn(() => false)
    register(opener)
    const { wrapper, img } = await pageWithResult({ image_base64: 'AAAA', session_item_id: 42 })
    await img.trigger('click')
    await settle()
    expect(opener).toHaveBeenCalledWith(42)
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(true)
  })
})
