import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base }
})

import AppFooter from '../components/AppFooter.vue'
import PostProcessPage from '../components/PostProcessPage.vue'
import ImageViewer from '../components/ImageViewer.vue'
import {
  ListSessions, SwitchSession, GetActiveSessionItem, GetSessionItems, GetSessionImage, GetSessionThumb,
  GetSettings, ListPresets, ListPresetTypes, ListHiresProfiles, ReadFileAsBase64, ReadThumbnail,
  BatchSaveImage, BatchReset, ExportImage, EnqueueFromImage, CancelQueueJob,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

function buttonExact(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().trim() === text)
}

function buttonWith(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().includes(text))
}

function strip(b64) {
  return String(b64 || '').replace(/^data:[^,]*,/, '')
}

describe('AppFooter history selection', () => {
  const SESSIONS = [
    { id: 11, name: 'Первая', item_count: 1, created_at: '', updated_at: '', is_active: false },
    { id: 22, name: 'Вторая', item_count: 3, created_at: '', updated_at: '', is_active: true },
  ]
  let activeSession = 22
  const ITEMS = {
    22: [
      { id: 101, source: 'generate', created_at: '2026-11-06T10:00:00Z', is_active: false },
      { id: 102, source: 'generate', created_at: '2026-11-06T10:01:00Z', is_active: false },
      { id: 103, source: 'generate', created_at: '2026-11-06T10:02:00Z', is_active: false },
    ],
    11: [
      { id: 201, source: 'generate', created_at: '2026-11-06T09:00:00Z', is_active: false },
    ],
  }
  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
    useRemixQueue().clearPostQueue()
    activeSession = 22
    ListSessions.mockImplementation(() => Promise.resolve(SESSIONS.map(s => ({ ...s, is_active: s.id === activeSession }))))
    SwitchSession.mockImplementation(id => {
      activeSession = Number(id)
      return Promise.resolve()
    })
    GetActiveSessionItem.mockImplementation(() => Promise.resolve(null))
    GetSessionItems.mockImplementation(() => Promise.resolve((ITEMS[activeSession] || []).map(i => ({ ...i }))))
  })

  async function mountOpen() {
    const wrapper = mount(AppFooter)
    await settle()
    const label = t('footer.session')
    const btn = wrapper.findAll('button').find(b => b.text().startsWith(label))
    expect(btn, `button "${label}"`).toBeTruthy()
    await btn.trigger('click')
    await settle()
    return wrapper
  }

  function cards(wrapper) {
    return wrapper.findAll('.si-card')
  }

  function cardIds(wrapper) {
    return cards(wrapper).map(c => Number(c.attributes('data-item-id')))
  }

  async function enterSelect(wrapper) {
    const btn = buttonExact(wrapper, t('footer.select_mode'))
    expect(btn, 'select mode button').toBeTruthy()
    await btn.trigger('click')
    await settle()
  }

  function postQueueValue() {
    return useRemixQueue().postQueue.value
  }

  function lastQueued() {
    return postQueueValue()
  }

  it('H1: select button is shown when the session has photos', async () => {
    const wrapper = await mountOpen()
    expect(cards(wrapper)).toHaveLength(3)
    expect(buttonExact(wrapper, t('footer.select_mode'))).toBeTruthy()
  })

  it('H1: select button is hidden when the session is empty', async () => {
    GetSessionItems.mockImplementation(() => Promise.resolve([]))
    const wrapper = await mountOpen()
    expect(cards(wrapper)).toHaveLength(0)
    expect(buttonExact(wrapper, t('footer.select_mode'))).toBeFalsy()
  })

  it('H1: in select mode clicks mark photos without opening the viewer and send them to post-processing', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    const ids = cardIds(wrapper)

    await cards(wrapper)[0].trigger('click')
    await settle()
    await cards(wrapper)[2].trigger('click')
    await settle()

    expect(wrapper.findComponent(ImageViewer).exists()).toBe(false)
    expect(cards(wrapper)[0].classes()).toContain('si-selected')
    expect(cards(wrapper)[1].classes()).not.toContain('si-selected')
    expect(cards(wrapper)[2].classes()).toContain('si-selected')
    expect(wrapper.text()).toContain(t('footer.selected_count', { n: 2 }))

    const send = buttonWith(wrapper, t('footer.send_to_postprocess', { n: 2 }))
    expect(send).toBeTruthy()
    expect(send.element.disabled).toBe(false)
    await send.trigger('click')
    await settle()

    expect(lastQueued()).toEqual([
      { path: 'session:' + ids[0], name: 'history_' + ids[0] },
      { path: 'session:' + ids[2], name: 'history_' + ids[2] },
    ])
    const nav = wrapper.emitted('navigate') || []
    expect(nav.some(e => e[0] && e[0].page === 'postprocess')).toBe(true)
    expect(buttonWith(wrapper, t('footer.send_to_postprocess', { n: 2 }))).toBeFalsy()
    expect(wrapper.findAll('.si-card.si-selected')).toHaveLength(0)
  })

  it('H1: queue follows feed order, not click order', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    const ids = cardIds(wrapper)

    await cards(wrapper)[2].trigger('click')
    await settle()
    await cards(wrapper)[0].trigger('click')
    await settle()

    await buttonWith(wrapper, t('footer.send_to_postprocess', { n: 2 })).trigger('click')
    await settle()

    expect(lastQueued().map(i => i.path)).toEqual(['session:' + ids[0], 'session:' + ids[2]])
  })

  it('H1: outside select mode a click opens the viewer', async () => {
    const wrapper = await mountOpen()
    await cards(wrapper)[0].trigger('click')
    await settle()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(true)
    expect(wrapper.findAll('.si-card.si-selected')).toHaveLength(0)
  })

  it('H2: second click unmarks the photo', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)

    await cards(wrapper)[1].trigger('click')
    await settle()
    expect(cards(wrapper)[1].classes()).toContain('si-selected')
    expect(wrapper.text()).toContain(t('footer.selected_count', { n: 1 }))

    await cards(wrapper)[1].trigger('click')
    await settle()
    expect(cards(wrapper)[1].classes()).not.toContain('si-selected')
    expect(wrapper.text()).toContain(t('footer.selected_count', { n: 0 }))
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(false)
  })

  it('H2: send button is disabled with nothing selected', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)

    const send = buttonWith(wrapper, t('footer.send_to_postprocess', { n: 0 }))
    expect(send).toBeTruthy()
    expect(send.element.disabled).toBe(true)
    await send.trigger('click')
    await settle()
    expect(postQueueValue()).toEqual([])
    expect((wrapper.emitted('navigate') || []).some(e => e[0] && e[0].page === 'postprocess')).toBe(false)
  })

  it('H2: cancel clears marks and leaves select mode', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    await cards(wrapper)[0].trigger('click')
    await settle()
    await cards(wrapper)[1].trigger('click')
    await settle()
    expect(wrapper.findAll('.si-card.si-selected')).toHaveLength(2)

    const cancel = buttonExact(wrapper, t('footer.cancel_select'))
    expect(cancel).toBeTruthy()
    await cancel.trigger('click')
    await settle()

    expect(wrapper.findAll('.si-card.si-selected')).toHaveLength(0)
    expect(wrapper.text()).not.toContain(t('footer.selected_count', { n: 2 }))
    expect(buttonWith(wrapper, t('footer.send_to_postprocess', { n: 2 }))).toBeFalsy()
    expect(buttonExact(wrapper, t('footer.select_mode'))).toBeTruthy()
    expect(postQueueValue()).toEqual([])

    await cards(wrapper)[0].trigger('click')
    await settle()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(true)
  })

  it('H2: switching session resets the selection and select mode', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    await cards(wrapper)[0].trigger('click')
    await settle()
    expect(wrapper.text()).toContain(t('footer.selected_count', { n: 1 }))

    const select = wrapper.find('select')
    expect(select.exists()).toBe(true)
    await select.setValue('11')
    await settle()

    expect(cardIds(wrapper)).toEqual([201])
    expect(wrapper.findAll('.si-card.si-selected')).toHaveLength(0)
    expect(wrapper.text()).not.toContain(t('footer.selected_count', { n: 1 }))
    expect(buttonWith(wrapper, t('footer.send_to_postprocess', { n: 1 }))).toBeFalsy()

    await cards(wrapper)[0].trigger('click')
    await settle()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(true)
  })
  it('H7: in select mode a marked photo shows a check, an unmarked one does not', async () => {
    const wrapper = await mountOpen()
    await enterSelect(wrapper)
    await cards(wrapper)[1].trigger('click')
    await settle()
    expect(cards(wrapper)[1].find('.si-check').exists()).toBe(true)
    expect(cards(wrapper)[0].find('.si-check').exists()).toBe(false)
    expect(cards(wrapper)[2].find('.si-check').exists()).toBe(false)
  })
})

describe('PostProcessPage queue from history', () => {
  const TYPES = [{ id: 1, name: 'post' }]
  const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]
  const IMG = { 501: 'SElTVDUwMQ==', 502: 'SElTVDUwMg==', 503: 'SElTVDUwMw==', 504: 'SElTVDUwNA==' }
  const THUMB = { 501: 'VEhNQjUwMQ==', 502: 'VEhNQjUwMg==', 503: 'VEhNQjUwMw==', 504: 'VEhNQjUwNA==' }

  class FakeImage {
    constructor() {
      this.onload = null
      this.onerror = null
      this.naturalWidth = 64
      this.naturalHeight = 48
      this.width = 64
      this.height = 48
      this.complete = false
      this._src = ''
    }
    set src(v) {
      this._src = v
      setTimeout(() => {
        this.complete = true
        if (this.onload) this.onload()
      }, 0)
    }
    get src() {
      return this._src
    }
    decode() {
      return Promise.resolve()
    }
  }

  function fakeContext(canvas) {
    return {
      canvas,
      fillStyle: '',
      strokeStyle: '',
      lineWidth: 1,
      globalCompositeOperation: 'source-over',
      getImageData: (x, y, w, h) => ({ data: new Uint8ClampedArray(Math.max(1, (w || 1) * (h || 1) * 4)), width: w, height: h }),
      putImageData: vi.fn(),
      createImageData: (w, h) => ({ data: new Uint8ClampedArray(Math.max(1, (w || 1) * (h || 1) * 4)), width: w, height: h }),
      clearRect: vi.fn(),
      fillRect: vi.fn(),
      beginPath: vi.fn(),
      closePath: vi.fn(),
      arc: vi.fn(),
      fill: vi.fn(),
      stroke: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      drawImage: vi.fn(),
      save: vi.fn(),
      restore: vi.fn(),
    }
  }

  const OrigImage = globalThis.Image
  let getContextSpy
  let toDataURLSpy

  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
    useRemixQueue().clearPostQueue()
    useRemixQueue().clearQueue()
    ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
    GetSettings.mockImplementation(() => Promise.resolve({ file_browser_path: '/out' }))
    GetActiveSessionItem.mockImplementation(() => Promise.resolve(null))
    GetSessionImage.mockImplementation(id => Promise.resolve(IMG[id] || ''))
    GetSessionThumb.mockImplementation(id => Promise.resolve(THUMB[id] || ''))
    ListPresets.mockImplementation(() => Promise.resolve(PRESETS))
    ListPresetTypes.mockImplementation(() => Promise.resolve(TYPES))
    ReadFileAsBase64.mockImplementation(() => Promise.resolve(''))
    ReadThumbnail.mockImplementation(() => Promise.resolve(''))
    BatchSaveImage.mockImplementation(() => Promise.resolve('/batch/v.png'))
    BatchReset.mockImplementation(() => Promise.resolve())
    ExportImage.mockImplementation(() => Promise.resolve('/out/x.png'))
    globalThis.Image = FakeImage
    window.Image = FakeImage
    getContextSpy = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function () {
      if (!this.__ctx) this.__ctx = fakeContext(this)
      return this.__ctx
    })
    toDataURLSpy = vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockImplementation(() => 'data:image/png;base64,TUFTSw==')
  })

  afterEach(() => {
    globalThis.Image = OrigImage
    window.Image = OrigImage
    getContextSpy.mockRestore()
    toDataURLSpy.mockRestore()
  })

  function historyItems(ids) {
    return ids.map(id => ({ path: 'session:' + id, name: 'history_' + id }))
  }

  async function mountThenQueue(ids) {
    const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' } })
    await settle()
    expect(wrapper.find('.batch-panel').exists()).toBe(false)
    useRemixQueue().setPostQueue(historyItems(ids))
    await settle()
    return wrapper
  }

  function previewSrc(wrapper) {
    const img = wrapper.find('.split-img.split-after')
    return img.exists() ? (img.attributes('src') || '') : ''
  }

  function readFileCalledWithSession() {
    return ReadFileAsBase64.mock.calls.some(c => String(c[0]).startsWith('session:'))
  }

  function readThumbCalledWithSession() {
    return ReadThumbnail.mock.calls.some(c => String(c[0]).startsWith('session:'))
  }

  it('H3: two history photos queued into an open page start the batch through session reads', async () => {
    const wrapper = await mountThenQueue([501, 502])

    expect(wrapper.find('.batch-panel').exists()).toBe(true)
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(2)
    const imageIds = GetSessionImage.mock.calls.map(c => Number(c[0]))
    expect(imageIds).toContain(501)
    expect(imageIds.every(id => id === 501 || id === 502)).toBe(true)
    const thumbIds = GetSessionThumb.mock.calls.map(c => Number(c[0]))
    expect(thumbIds).toEqual(expect.arrayContaining([501, 502]))
    expect(readFileCalledWithSession()).toBe(false)
    expect(readThumbCalledWithSession()).toBe(false)
    expect(previewSrc(wrapper)).toContain(IMG[501])
  })

  it('H4: one history photo queued into an open page opens it in normal mode', async () => {
    const wrapper = await mountThenQueue([502])

    expect(wrapper.find('.batch-panel').exists()).toBe(false)
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(0)
    expect(GetSessionImage.mock.calls.map(c => Number(c[0]))).toContain(502)
    expect(readFileCalledWithSession()).toBe(false)
    expect(previewSrc(wrapper)).toContain(IMG[502])
  })

  it('H5: save all names history photos history_<id>.<format>', async () => {
    const wrapper = await mountThenQueue([501, 502])
    expect(wrapper.find('.batch-panel').exists()).toBe(true)

    const header = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(t('export.export_settings')))
    expect(header).toBeTruthy()
    await header.trigger('click')
    await settle()

    const btn = buttonWith(wrapper, t('post.batch_save_all', { n: 2 }))
    expect(btn).toBeTruthy()
    expect(btn.element.disabled).toBe(false)
    await btn.trigger('click')
    await settle()

    expect(ExportImage).toHaveBeenCalledTimes(2)
    const calls = ExportImage.mock.calls.map(c => c[0])
    expect(calls.map(c => c.filename)).toEqual(['history_501.png', 'history_502.png'])
    expect(calls.map(c => c.format)).toEqual(['png', 'png'])
    expect(strip(calls[0].image_base64)).toBe(IMG[501])
    expect(strip(calls[1].image_base64)).toBe(IMG[502])
  })
  it('H6: a new queue of two photos during a running step cancels the unfinished jobs and opens the new batch', async () => {
    let jobs = 0
    EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
    const wrapper = await mountThenQueue([501, 502])
    const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
    expect(card).toBeTruthy()
    await card.trigger('click')
    await settle()
    const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect_all', { n: 2 })))
    expect(apply).toBeTruthy()
    await apply.trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)

    useRemixQueue().setPostQueue(historyItems([503, 504]))
    await settle()

    const cancelled = CancelQueueJob.mock.calls.map(c => c[0])
    expect(cancelled).toEqual(expect.arrayContaining(['job-1', 'job-2']))
    expect(wrapper.find('.batch-panel').exists()).toBe(true)
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(2)
    expect(GetSessionThumb.mock.calls.map(c => Number(c[0]))).toEqual(expect.arrayContaining([503, 504]))
    expect(previewSrc(wrapper)).toContain(IMG[503])
  })
})
