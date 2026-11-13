import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => ({
  ...(await import('./mocks/wails-app')).default,
  ListHistory: vi.fn(),
  OpenInHistory: vi.fn(() => Promise.resolve()),
  DeleteHistoryItems: vi.fn(() => Promise.resolve(0)),
}))

import GalleryPage from '../components/GalleryPage.vue'
import { ListHistory, OpenInHistory, DeleteHistoryItems, GetSessionImage, GetSessionThumb, SaveImage } from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const utc = (d) => d.toISOString().slice(0, 19).replace('T', ' ')
const now = new Date()
const dayAt = (daysAgo, h) => new Date(now.getFullYear(), now.getMonth(), now.getDate() - daysAgo, h, 0, 0)

function mk(id, extra = {}) {
  return {
    id, session_id: 1, session_name: 'Портреты', file_name: `${id}.png`, thumb_name: `${id}_thumb.jpg`,
    source: 'generate', prompt: `prompt ${id}`, width: 768, height: 1024, created_at: utc(dayAt(0, 12)),
    kind: 'generate', model_name: 'flux', label: 'Neon', shape: 'portrait', ...extra,
  }
}

const FACETS = {
  sessions: [{ value: '1', name: 'Портреты', count: 3 }, { value: '2', name: 'Pets', count: 2 }],
  models: [{ value: 'flux', count: 4 }, { value: 'sdxl', count: 1 }],
  labels: [{ value: 'Neon', count: 3 }, { value: 'Flow', count: 2, pipeline: true }],
  kinds: [{ value: 'generate', count: 3 }, { value: 'pipeline', count: 2 }],
  shapes: [{ value: 'portrait', count: 5 }],
}

let store
let facets

function fakeList(filter = {}) {
  const off = filter.offset || 0
  const lim = filter.limit || 120
  const items = store.slice(off, off + lim).map(x => ({ ...x }))
  return Promise.resolve({ items, total: store.length, facets: JSON.parse(JSON.stringify(facets)) })
}

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

async function waitFor(fn, ms = 2000) {
  const start = Date.now()
  for (;;) {
    await settle()
    if (fn()) return
    if (Date.now() - start > ms) return
    await new Promise(r => setTimeout(r, 50))
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  localStorage.clear()
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  store = [5, 4, 3, 2, 1].map(id => mk(id))
  facets = JSON.parse(JSON.stringify(FACETS))
  ListHistory.mockImplementation(fakeList)
  GetSessionImage.mockImplementation((id) => Promise.resolve(`IMG${id}`))
  GetSessionThumb.mockImplementation((id) => Promise.resolve(`TH${id}`))
  SaveImage.mockImplementation(() => Promise.resolve('/tmp/x.png'))
  OpenInHistory.mockImplementation(() => Promise.resolve())
  DeleteHistoryItems.mockImplementation((ids) => {
    store = store.filter(x => !ids.includes(x.id))
    return Promise.resolve(ids.length)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  setLocale('ru')
})

async function mountPage() {
  const w = mount(GalleryPage, { attachTo: document.body })
  await settle()
  return w
}

const lastFilter = () => ListHistory.mock.calls.at(-1)[0] || {}
const items = (w) => w.findAll('[data-testid="gallery-item"]')
const itemIds = (w) => items(w).map(i => Number(i.attributes('data-id')))
const chip = (w, hook, attr, value) => w.findAll(`[data-testid="${hook}"]`).find(e => e.attributes(attr) === String(value))
const emptyList = (v) => v == null || (Array.isArray(v) && v.length === 0)
const viewerOpen = () => document.querySelector('.image-viewer-overlay') !== null
const q = (sel) => document.querySelector(sel)

async function click(el) {
  expect(el, 'element exists').toBeTruthy()
  if (el.trigger) await el.trigger('click')
  else el.click()
  await settle()
}

describe('gallery grid (G1, G5, G6)', () => {
  it('loads history on open and shows one tile per item, newest first, with total', async () => {
    const w = await mountPage()
    expect(ListHistory).toHaveBeenCalled()
    expect(lastFilter().offset || 0).toBe(0)
    expect(itemIds(w)).toEqual([5, 4, 3, 2, 1])
    expect(w.find('[data-testid="gallery-total"]').text()).toContain('5')
  })

  it('groups tiles under day headers: today, yesterday, a dated day', async () => {
    store = [
      mk(6, { created_at: utc(dayAt(0, 15)) }),
      mk(5, { created_at: utc(dayAt(0, 9)) }),
      mk(4, { created_at: utc(dayAt(1, 12)) }),
      mk(3, { created_at: utc(new Date(2026, 9, 12, 12)) }),
    ]
    const w = await mountPage()
    const days = w.findAll('[data-testid="gallery-day"]')
    expect(days).toHaveLength(3)
    expect(days[0].text()).toContain(t('gallery.today'))
    expect(days[1].text()).toContain(t('gallery.yesterday'))
    expect(days[2].text()).toContain('12')
    expect(days[2].text()).toContain('октября')
    expect(itemIds(w)).toEqual([6, 5, 4, 3])
  })

  it('empty history shows the empty state, not the no-match message', async () => {
    store = []
    facets = { sessions: [], models: [], labels: [], kinds: [], shapes: [] }
    const w = await mountPage()
    expect(w.find('[data-testid="gallery-empty"]').exists()).toBe(true)
    expect(w.find('[data-testid="no-match"]').exists()).toBe(false)
    expect(items(w)).toHaveLength(0)
  })

  it('load more appends the next page using offset', async () => {
    store = Array.from({ length: 130 }, (_, i) => mk(1000 - i))
    const w = await mountPage()
    const first = items(w).length
    expect(first).toBeGreaterThan(0)
    expect(first).toBeLessThan(130)
    await click(w.find('[data-testid="load-more"]'))
    expect(lastFilter().offset).toBe(first)
    expect(itemIds(w)).toEqual(store.map(x => x.id))
    expect(new Set(itemIds(w)).size).toBe(130)
    expect(w.find('[data-testid="load-more"]').exists()).toBe(false)
  })

  it('refreshes when a history item is added', async () => {
    const w = await mountPage()
    const calls = ListHistory.mock.calls.length
    store = [mk(9, { created_at: utc(dayAt(0, 23)) }), ...store]
    EventsEmit('session:added', 9)
    await waitFor(() => ListHistory.mock.calls.length > calls && items(w).length === 6)
    expect(ListHistory.mock.calls.length).toBeGreaterThan(calls)
    expect(itemIds(w)[0]).toBe(9)
  })
})

describe('filters (G2)', () => {
  it('each value shows its count', async () => {
    const w = await mountPage()
    expect(chip(w, 'filter-session', 'data-session', '1').text()).toContain('Портреты')
    expect(chip(w, 'filter-session', 'data-session', '1').text()).toContain('3')
    expect(chip(w, 'filter-session', 'data-session', '2').text()).toContain('2')
    expect(chip(w, 'filter-model', 'data-model', 'flux').text()).toContain('4')
    expect(chip(w, 'filter-label', 'data-label', 'Neon').text()).toContain('3')
    expect(chip(w, 'filter-label', 'data-label', 'Flow').text()).toContain('2')
    expect(chip(w, 'filter-kind', 'data-kind', 'generate').text()).toContain('3')
    expect(chip(w, 'filter-kind', 'data-kind', 'pipeline').text()).toContain('2')
  })

  it('session chips are multi-select and send numeric ids; second click removes', async () => {
    const w = await mountPage()
    await click(chip(w, 'filter-session', 'data-session', '1'))
    expect(lastFilter().session_ids).toEqual([1])
    await click(chip(w, 'filter-session', 'data-session', '2'))
    expect([...lastFilter().session_ids].sort()).toEqual([1, 2])
    await click(chip(w, 'filter-session', 'data-session', '1'))
    expect(lastFilter().session_ids).toEqual([2])
    expect(lastFilter().offset || 0).toBe(0)
  })

  it.each([
    ['filter-model', 'data-model', 'flux', 'models', 'sdxl'],
    ['filter-label', 'data-label', 'Neon', 'labels', 'Flow'],
    ['filter-kind', 'data-kind', 'generate', 'kinds', 'pipeline'],
  ])('%s chip sends %s immediately and combines several values', async (hook, attr, v1, key, v2) => {
    const w = await mountPage()
    await click(chip(w, hook, attr, v1))
    expect(lastFilter()[key]).toEqual([v1])
    await click(chip(w, hook, attr, v2))
    expect([...lastFilter()[key]].sort()).toEqual([v1, v2].sort())
  })

  it('shape chip sends shapes', async () => {
    const w = await mountPage()
    await click(chip(w, 'filter-shape', 'data-shape', 'landscape'))
    expect(lastFilter().shapes).toEqual(['landscape'])
  })

  it('filters combine: session and kind are sent together', async () => {
    const w = await mountPage()
    await click(chip(w, 'filter-session', 'data-session', '2'))
    await click(chip(w, 'filter-kind', 'data-kind', 'pipeline'))
    expect(lastFilter()).toMatchObject({ session_ids: [2], kinds: ['pipeline'] })
  })

  it('search sends the prompt text', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-search"]').setValue('рыжий кот')
    await waitFor(() => lastFilter().search === 'рыжий кот')
    expect(lastFilter().search).toBe('рыжий кот')
  })

  it('period today sends local midnight as date_from', async () => {
    const w = await mountPage()
    await click(chip(w, 'filter-period', 'data-period', 'today'))
    expect(lastFilter().date_from).toBe(new Date(now.getFullYear(), now.getMonth(), now.getDate()).toISOString())
    await click(chip(w, 'filter-period', 'data-period', 'all'))
    expect(lastFilter().date_from || '').toBe('')
    expect(lastFilter().date_to || '').toBe('')
  })

  it('custom dates send from inclusive and the day after "to" exclusive', async () => {
    const w = await mountPage()
    await click(chip(w, 'filter-period', 'data-period', 'custom'))
    await w.find('[data-testid="filter-date-from"]').setValue('2026-11-01')
    await w.find('[data-testid="filter-date-to"]').setValue('2026-11-03')
    await settle()
    expect(lastFilter().date_from).toBe(new Date(2026, 10, 1).toISOString())
    expect(lastFilter().date_to).toBe(new Date(2026, 10, 4).toISOString())
  })

  it('reset clears every filter', async () => {
    const w = await mountPage()
    await click(chip(w, 'filter-session', 'data-session', '1'))
    await click(chip(w, 'filter-model', 'data-model', 'flux'))
    await click(chip(w, 'filter-kind', 'data-kind', 'generate'))
    await click(chip(w, 'filter-period', 'data-period', 'today'))
    await click(w.find('[data-testid="filter-reset"]'))
    const f = lastFilter()
    for (const k of ['session_ids', 'models', 'labels', 'kinds', 'shapes']) expect(emptyList(f[k]), k).toBe(true)
    expect(f.search || '').toBe('')
    expect(f.date_from || '').toBe('')
    expect(f.date_to || '').toBe('')
  })

  it('no results under filters shows the no-match message with a reset', async () => {
    const w = await mountPage()
    store = []
    await click(chip(w, 'filter-model', 'data-model', 'sdxl'))
    const nm = w.find('[data-testid="no-match"]')
    expect(nm.exists()).toBe(true)
    expect(nm.text()).toContain(t('gallery.no_match'))
    expect(w.find('[data-testid="gallery-empty"]').exists()).toBe(false)
    store = [5, 4, 3, 2, 1].map(id => mk(id))
    const reset = nm.find('[data-testid="filter-reset"]').exists() ? nm.find('[data-testid="filter-reset"]') : w.find('[data-testid="filter-reset"]')
    await click(reset)
    expect(emptyList(lastFilter().models)).toBe(true)
    expect(items(w)).toHaveLength(5)
  })
})

describe('viewer (G3)', () => {
  it('click opens the viewer and browses the filtered set with N of M', async () => {
    store = [5, 4, 3, 2, 1].map(id => mk(id, { model_name: `model-${id}` }))
    const w = await mountPage()
    const shows = (id) => q('.image-viewer-overlay').textContent.includes(`model-${id}`)
    await click(items(w)[1])
    expect(viewerOpen()).toBe(true)
    expect(shows(4)).toBe(true)
    expect(q('.iv-counter').textContent).toBe(t('viewer.counter', { n: 2, total: 5 }))
    await click(q('.image-viewer-next'))
    expect(shows(3)).toBe(true)
    expect(q('.iv-counter').textContent).toBe(t('viewer.counter', { n: 3, total: 5 }))
    await click(q('.image-viewer-prev'))
    await click(q('.image-viewer-prev'))
    expect(shows(5)).toBe(true)
    expect(q('.iv-counter').textContent).toBe(t('viewer.counter', { n: 1, total: 5 }))
    expect(q('.image-viewer-prev')).toBeNull()
  })

  it('caption shows group, model and style; size is shown', async () => {
    store = [mk(7, { session_name: 'Pets', model_name: 'sdxl-base', label: 'Retro', width: 832, height: 1216 })]
    const w = await mountPage()
    await click(items(w)[0])
    const text = q('.image-viewer-overlay').textContent
    expect(text).toContain('Pets')
    expect(text).toContain('sdxl-base')
    expect(text).toContain('Retro')
    expect(text).toMatch(/832\s*[×x]\s*1216/)
  })

  it('save calls SaveImage', async () => {
    const w = await mountPage()
    await click(items(w)[0])
    await click(q('[data-testid="viewer-save"]'))
    expect(SaveImage).toHaveBeenCalled()
    expect(SaveImage.mock.calls[0][0]).toBe('IMG5')
  })

  it('delete needs two clicks and removes only that item', async () => {
    const w = await mountPage()
    await click(items(w)[0])
    await click(q('.iv-delete'))
    expect(DeleteHistoryItems).not.toHaveBeenCalled()
    await click(q('.iv-delete'))
    expect(DeleteHistoryItems).toHaveBeenCalledTimes(1)
    expect(DeleteHistoryItems).toHaveBeenCalledWith([5])
    await settle()
    expect(itemIds(w)).not.toContain(5)
  })

  it('open in history calls OpenInHistory and goes to generate', async () => {
    const w = await mountPage()
    await click(items(w)[2])
    await click(q('[data-testid="viewer-open-history"]'))
    expect(OpenInHistory).toHaveBeenCalledWith(3)
    const nav = w.emitted('navigate')
    expect(nav).toBeTruthy()
    expect(nav.at(-1)[0]).toMatchObject({ page: 'generate' })
  })

  it('postprocess sends the image to the postprocess page', async () => {
    const w = await mountPage()
    await click(items(w)[0])
    const btn = [...q('.image-viewer-overlay').querySelectorAll('button')].find(b => b.textContent.trim() === t('viewer.postprocess'))
    await click(btn)
    const nav = w.emitted('navigate')
    expect(nav).toBeTruthy()
    expect(nav.at(-1)[0]).toMatchObject({ page: 'postprocess' })
    const handedOver = nav.at(-1)[0].image === 'IMG5' || OpenInHistory.mock.calls.some(c => c[0] === 5)
    expect(handedOver, 'image passed or item made active for postprocess').toBe(true)
  })
})

describe('selection mode (G4)', () => {
  it('clicking tiles selects them; delete happens only after confirmation', async () => {
    const w = await mountPage()
    await click(w.find('[data-testid="select-mode"]'))
    await click(items(w)[0])
    await click(items(w)[2])
    expect(viewerOpen()).toBe(false)
    const del = w.find('[data-testid="delete-selected"]')
    expect(del.text()).toContain('2')
    await click(del)
    expect(DeleteHistoryItems).not.toHaveBeenCalled()
    const confirm = q('[data-testid="confirm-delete"]')
    expect(confirm).toBeTruthy()
    await click(confirm)
    expect(DeleteHistoryItems).toHaveBeenCalledTimes(1)
    expect([...DeleteHistoryItems.mock.calls[0][0]].sort()).toEqual([3, 5])
    await settle()
    expect(itemIds(w)).toEqual([4, 2, 1])
  })

  it('select all loads the remaining pages of the filtered set and selects every item', async () => {
    store = Array.from({ length: 130 }, (_, i) => mk(1000 - i))
    const w = await mountPage()
    await click(chip(w, 'filter-kind', 'data-kind', 'generate'))
    const loaded = items(w).length
    expect(loaded).toBeLessThan(130)
    await click(w.find('[data-testid="select-mode"]'))
    await click(w.find('[data-testid="select-all"]'))
    const pageCalls = ListHistory.mock.calls.map(c => c[0] || {})
    expect(pageCalls.some(f => f.offset === loaded && emptyList(f.kinds) === false && f.kinds.includes('generate'))).toBe(true)
    expect(w.find('[data-testid="delete-selected"]').text()).toContain('130')
    await click(w.find('[data-testid="delete-selected"]'))
    await click(q('[data-testid="confirm-delete"]'))
    expect(DeleteHistoryItems).toHaveBeenCalledTimes(1)
    expect([...DeleteHistoryItems.mock.calls[0][0]].sort((a, b) => a - b)).toEqual(Array.from({ length: 130 }, (_, i) => 871 + i))
  })
})

describe('review follow-ups (G2, G3)', () => {
  it('period chips show counts from facets.periods', async () => {
    facets.periods = [{ value: 'all', count: 41 }, { value: 'today', count: 7 }, { value: 'week', count: 19 }, { value: 'month', count: 33 }]
    const w = await mountPage()
    expect(chip(w, 'filter-period', 'data-period', 'all').text()).toContain('41')
    expect(chip(w, 'filter-period', 'data-period', 'today').text()).toContain('7')
    expect(chip(w, 'filter-period', 'data-period', 'week').text()).toContain('19')
    expect(chip(w, 'filter-period', 'data-period', 'month').text()).toContain('33')
  })

  it('every ListHistory call sends period_starts for today, week and month', async () => {
    const w = await mountPage()
    await click(chip(w, 'filter-kind', 'data-kind', 'generate'))
    await click(chip(w, 'filter-period', 'data-period', 'week'))
    expect(ListHistory.mock.calls.length).toBeGreaterThanOrEqual(3)
    const today0 = new Date(now.getFullYear(), now.getMonth(), now.getDate()).toISOString()
    for (const [f] of ListHistory.mock.calls) {
      expect(Object.keys(f.period_starts || {}).sort()).toEqual(['month', 'today', 'week'])
      expect(f.period_starts.today).toBe(today0)
      expect(new Date(f.period_starts.week).getTime()).toBeLessThan(new Date(today0).getTime())
      expect(new Date(f.period_starts.month).getTime()).toBeLessThan(new Date(f.period_starts.week).getTime())
    }
  })

  it('viewer stays on the same picture when a refresh inserts a new item before it', async () => {
    store = [5, 4, 3, 2, 1].map(id => mk(id, { model_name: `model-${id}` }))
    const w = await mountPage()
    await click(items(w)[2])
    expect(q('.iv-counter').textContent).toBe(t('viewer.counter', { n: 3, total: 5 }))
    store = [mk(9, { model_name: 'model-9', created_at: utc(dayAt(0, 23)) }), ...store]
    EventsEmit('session:added', 9)
    await waitFor(() => items(w).length === 6)
    expect(viewerOpen()).toBe(true)
    expect(q('.image-viewer-overlay').textContent).toContain('model-3')
    expect(q('.image-viewer-overlay').textContent).not.toContain('model-2')
    expect(q('.iv-counter').textContent).toBe(t('viewer.counter', { n: 4, total: 6 }))
  })

  it('viewer closes when its picture disappears after a refresh', async () => {
    const w = await mountPage()
    await click(items(w)[2])
    expect(viewerOpen()).toBe(true)
    store = store.filter(x => x.id !== 3)
    EventsEmit('session:added', 0)
    await waitFor(() => items(w).length === 4)
    expect(viewerOpen()).toBe(false)
  })

  it('delete confirmation does not carry over to another picture', async () => {
    const w = await mountPage()
    await click(items(w)[0])
    await click(q('.iv-delete'))
    await click(q('.image-viewer-next'))
    await click(q('.iv-delete'))
    expect(DeleteHistoryItems).not.toHaveBeenCalled()
    await click(q('.iv-delete'))
    expect(DeleteHistoryItems).toHaveBeenCalledTimes(1)
    expect(DeleteHistoryItems).toHaveBeenCalledWith([4])
  })

  it('delete confirmation armed on X is not used after a refresh moves positions', async () => {
    store = [5, 4, 3, 2, 1].map(id => mk(id, { model_name: `model-${id}` }))
    const w = await mountPage()
    await click(items(w)[1])
    await click(q('.iv-delete'))
    store = [mk(9, { model_name: 'model-9', created_at: utc(dayAt(0, 23)) }), ...store]
    EventsEmit('session:added', 9)
    await waitFor(() => items(w).length === 6)
    await click(q('.iv-delete'))
    for (const [ids] of DeleteHistoryItems.mock.calls) expect(ids).toEqual([4])
    expect(DeleteHistoryItems.mock.calls.flat(2)).not.toContain(9)
    expect(DeleteHistoryItems.mock.calls.flat(2)).not.toContain(5)
  })

  it('viewer title contains the localized date', async () => {
    store = [mk(7, { created_at: utc(new Date(2026, 9, 12, 14, 0, 0)) })]
    const w = await mountPage()
    await click(items(w)[0])
    const title = q('.iv-title').textContent
    expect(title).toContain('12')
    expect(title).toContain('октября')
  })

  it('refresh after more than 500 loaded keeps all of them, fetching chunks of at most 500', async () => {
    store = Array.from({ length: 620 }, (_, i) => mk(10000 - i))
    const w = await mountPage()
    for (let i = 0; i < 20 && w.find('[data-testid="load-more"]').exists(); i++) {
      await click(w.find('[data-testid="load-more"]'))
    }
    expect(items(w)).toHaveLength(620)
    const before = ListHistory.mock.calls.length
    store = [mk(20000, { created_at: utc(dayAt(0, 23)) }), ...store]
    EventsEmit('session:added', 20000)
    await waitFor(() => ListHistory.mock.calls.length > before && itemIds(w)[0] === 20000, 10000)
    const after = ListHistory.mock.calls.slice(before).map(c => c[0] || {})
    expect(after.length).toBeGreaterThan(0)
    for (const f of after) {
      expect(f.limit).toBeGreaterThan(0)
      expect(f.limit).toBeLessThanOrEqual(500)
    }
    await waitFor(() => items(w).length === 621, 10000)
    expect(items(w)).toHaveLength(621)
    expect(itemIds(w)).toEqual(store.map(x => x.id))
  }, 30000)
})
