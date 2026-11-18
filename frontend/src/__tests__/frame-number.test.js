import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).createAppMock({
  ListHistory: vi.fn(),
  OpenInHistory: vi.fn(() => Promise.resolve()),
  DeleteHistoryItems: vi.fn(() => Promise.resolve(0)),
}))

import AppFooter from '../components/AppFooter.vue'
import GalleryPage from '../components/GalleryPage.vue'
import ImageViewer from '../components/ImageViewer.vue'
import {
  ListSessions, GetActiveSessionItem, GetSessionItems, ListHistory, GetSessionImage, GetSessionThumb,
} from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const NUM = '[data-testid="viewer-item-number"]'

function titleLines(title) {
  return String(title || '').split('\n').map(s => s.trim())
}

function hintOf(el) {
  const own = el.getAttribute('title')
  if (own) return own
  const inner = el.querySelector('[title]')
  return inner ? inner.getAttribute('title') : ''
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
})

afterEach(() => {
  vi.unstubAllGlobals()
  setLocale('ru')
})

describe('ImageViewer frame number', () => {
  const make = (props = {}) => mount(ImageViewer, { props: { imageBase64: 'AAAA', ...props }, attachTo: document.body })

  it('positive itemNumber shows #<id> in the top toolbar', () => {
    const w = make({ itemNumber: 33190 })
    const el = w.find(NUM)
    expect(el.exists()).toBe(true)
    expect(el.text()).toContain('#33190')
    expect(w.find('.iv-toolbar').element.contains(el.element)).toBe(true)
  })

  it('frame number has the hint tooltip', () => {
    const w = make({ itemNumber: 33190 })
    expect(t('viewer.item_number_hint')).toBe('Номер кадра — по нему ИИ-агент найдёт этот кадр')
    expect(hintOf(w.find(NUM).element)).toContain(t('viewer.item_number_hint'))
  })

  it('itemNumber 0 shows nothing', () => {
    const w = make({ itemNumber: 0 })
    expect(w.find(NUM).exists()).toBe(false)
    expect(w.find('.iv-toolbar').text()).not.toMatch(/#\d/)
  })

  it('without itemNumber shows nothing', () => {
    const w = make()
    expect(w.find(NUM).exists()).toBe(false)
  })
})

describe('History panel', () => {
  const ITEMS = [
    { id: 33190, source: 'generate', created_at: '2026-11-07T10:00:00Z', is_active: false },
    { id: 302, source: 'generate', created_at: '2026-11-07T10:01:00Z', is_active: false },
  ]

  function setItems(items) {
    ListSessions.mockImplementation(() => Promise.resolve([
      { id: 7, name: 'S', item_count: items.length, created_at: '', updated_at: '', is_active: true },
    ]))
    GetActiveSessionItem.mockImplementation(() => Promise.resolve(null))
    GetSessionItems.mockImplementation(() => Promise.resolve(items.map(i => ({ ...i }))))
  }

  async function openHistory() {
    const w = mount(AppFooter, { attachTo: document.body })
    await settle()
    const btn = w.findAll('button').find(b => b.text().startsWith(t('footer.session')))
    expect(btn).toBeTruthy()
    await btn.trigger('click')
    await settle()
    return w
  }

  it('each thumbnail tooltip contains the line #<id>', async () => {
    setItems(ITEMS)
    const w = await openHistory()
    for (const it of ITEMS) {
      const card = w.find(`[data-item-id="${it.id}"]`)
      expect(card.exists()).toBe(true)
      expect(titleLines(card.attributes('title'))).toContain(`#${it.id}`)
    }
  })

  it('item without id has no # line in its tooltip', async () => {
    setItems([{ source: 'generate', created_at: '2026-11-07T10:00:00Z', is_active: false }])
    const w = await openHistory()
    const cards = w.findAll('.si-card')
    expect(cards).toHaveLength(1)
    const lines = titleLines(cards[0].attributes('title'))
    expect(lines.some(l => l.startsWith('#'))).toBe(false)
  })

  it('opening the viewer from History shows #<that id>', async () => {
    setItems(ITEMS)
    const w = await openHistory()
    await w.find('[data-item-id="302"]').trigger('click')
    await settle()
    const v = w.findComponent(ImageViewer)
    expect(v.exists()).toBe(true)
    const el = v.find(NUM)
    expect(el.exists()).toBe(true)
    expect(el.text()).toContain('#302')
    expect(el.text()).not.toContain('#33190')
  })
})

describe('Gallery page', () => {
  const mk = (id) => ({
    id, session_id: 1, session_name: 'P', file_name: `${id}.png`, thumb_name: `${id}_thumb.jpg`,
    source: 'generate', prompt: `prompt ${id}`, width: 768, height: 1024, created_at: '2026-11-13 12:00:00',
    kind: 'generate', model_name: 'flux', label: 'Neon', shape: 'portrait',
  })

  beforeEach(() => {
    localStorage.clear()
    const store = [5, 4, 3].map(mk)
    ListHistory.mockImplementation(() => Promise.resolve({
      items: store.map(x => ({ ...x })), total: store.length,
      facets: { sessions: [], models: [], labels: [], kinds: [], shapes: [] },
    }))
    GetSessionImage.mockImplementation((id) => Promise.resolve(`IMG${id}`))
    GetSessionThumb.mockImplementation((id) => Promise.resolve(`TH${id}`))
  })

  it('opening the viewer shows #<that id> and follows navigation', async () => {
    const w = mount(GalleryPage, { attachTo: document.body })
    await settle()
    const tiles = w.findAll('[data-testid="gallery-item"]')
    expect(tiles).toHaveLength(3)
    await tiles[1].trigger('click')
    await settle()
    let el = document.querySelector(NUM)
    expect(el).toBeTruthy()
    expect(el.textContent).toContain('#4')
    document.querySelector('.image-viewer-next').click()
    await settle()
    el = document.querySelector(NUM)
    expect(el.textContent).toContain('#3')
    expect(el.textContent).not.toContain('#4')
  })
})
