import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return {
    ...base,
    PreviewAdjust: vi.fn(() => Promise.resolve('UFJFVkFESg==')),
    PreviewPost: vi.fn(() => Promise.resolve('UFJFVlBPU1Q=')),
  }
})

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, ReadFileAsBase64, BatchSaveImage, BatchReset, ExportImage,
  PreviewAdjust, PreviewPost,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]
const IMG_A = 'QUFBQQ=='
const IMG_B = 'QkJCQg=='
const ONE = [{ path: '/photos/a.png', name: 'a.png' }]
const TWO = [{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }]

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

async function settlePreview() {
  await settle()
  await new Promise(r => setTimeout(r, 250))
  await settle()
}

function fakeContext(canvas) {
  return {
    canvas,
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    lineCap: '',
    lineJoin: '',
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

const OrigImage = globalThis.Image
let getContextSpy
let toDataURLSpy

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  const files = { '/photos/a.png': IMG_A, '/photos/b.png': IMG_B }
  ReadFileAsBase64.mockImplementation(p => Promise.resolve(files[p] || ''))
  let saved = 0
  BatchSaveImage.mockImplementation(b64 => {
    saved++
    const p = '/batch/v' + saved + '.png'
    files[p] = String(b64).replace(/^data:[^,]*,/, '')
    return Promise.resolve(p)
  })
  BatchReset.mockImplementation(() => Promise.resolve())
  ExportImage.mockImplementation(() => Promise.resolve('/out/x.png'))
  PreviewAdjust.mockImplementation(() => Promise.resolve('UFJFVkFESg=='))
  PreviewPost.mockImplementation(() => Promise.resolve('UFJFVlBPU1Q='))
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

async function mountPage({ items = ONE, settings = {} } = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  useRemixQueue().setPostQueue(items.map(i => ({ ...i })))
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' } })
  await settle()
  return wrapper
}

function strip(b64) {
  return String(b64 || '').replace(/^data:[^,]*,/, '')
}

function buttonWith(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().includes(text))
}

async function openSection(wrapper, title) {
  const header = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(title))
  expect(header).toBeTruthy()
  await header.trigger('click')
  await settle()
  return header.element.closest('.card')
}

function field(wrapper, id) {
  const el = wrapper.find(`[data-testid="${id}"]`)
  expect(el.exists(), id).toBe(true)
  return el
}

async function set(wrapper, id, value) {
  await field(wrapper, id).setValue(value)
  await settle()
}

async function saveSingle(wrapper) {
  await openSection(wrapper, t('export.export_settings'))
  const btn = buttonWith(wrapper, t('post.btn_save_file'))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
  expect(ExportImage).toHaveBeenCalledTimes(1)
  return ExportImage.mock.calls[0][0]
}

function emptyFinish(f) {
  return f === undefined || f === null || (typeof f === 'object' && Object.keys(f).length === 0)
}

describe('PostProcessPage format, frame, caption', () => {
  it('PE7: ratio with crop and offset go to the export params', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-ratio', '4:5')
    await set(wrapper, 'finish-fit', 'crop')
    await set(wrapper, 'finish-offset', 0)

    const params = await saveSingle(wrapper)

    expect(strip(params.image_base64)).toBe(IMG_A)
    expect(params.finish).toBeTruthy()
    expect(params.finish.ratio).toBe('4:5')
    expect(params.finish.fit).toBe('crop')
    expect(params.finish.offset).toBe(0)
  })

  it('PE7: crop offset defaults to the center', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-ratio', '1:1')

    const params = await saveSingle(wrapper)

    expect(params.finish.ratio).toBe('1:1')
    expect(params.finish.fit).toBe('crop')
    expect(params.finish.offset).toBe(0.5)
  })

  it.each(['blur', 'white', 'black'])('PE7: fit %s goes to the export params', async (fit) => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-ratio', '9:16')
    await set(wrapper, 'finish-fit', fit)

    const params = await saveSingle(wrapper)

    expect(params.finish.ratio).toBe('9:16')
    expect(params.finish.fit).toBe(fit)
  })

  it.each(['white', 'black', 'polaroid'])('PE7: frame %s goes to the export params', async (frame) => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-frame', frame)

    const params = await saveSingle(wrapper)

    expect(params.finish).toBeTruthy()
    expect(params.finish.frame).toBe(frame)
    expect(params.finish.ratio || '').toBe('')
  })

  it('PE7: caption with place and size goes to the export params', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-caption', 'Лето 2026')
    await set(wrapper, 'finish-caption-pos', 'top')
    await set(wrapper, 'finish-caption-size', 'l')

    const params = await saveSingle(wrapper)

    expect(params.finish.caption).toBe('Лето 2026')
    expect(params.finish.caption_pos).toBe('top')
    expect(params.finish.caption_size).toBe('l')
  })

  it('PE7: "Unchanged" sends no finish params', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-ratio', '4:5')
    await set(wrapper, 'finish-ratio', '')

    const params = await saveSingle(wrapper)

    expect(emptyFinish(params.finish)).toBe(true)
  })

  it('PE7: untouched card sends no finish params', async () => {
    const wrapper = await mountPage()

    const params = await saveSingle(wrapper)

    expect(emptyFinish(params.finish)).toBe(true)
  })

  it('PE6/PE7: batch save all sends the same finish params for every photo', async () => {
    const wrapper = await mountPage({ items: TWO, settings: { file_browser_path: '/out' } })
    expect(wrapper.find('.batch-panel').exists()).toBe(true)
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-ratio', '4:5')
    await set(wrapper, 'finish-frame', 'polaroid')
    await set(wrapper, 'finish-caption', 'Hello')

    await openSection(wrapper, t('export.export_settings'))
    await buttonWith(wrapper, t('post.batch_save_all', { n: 2 })).trigger('click')
    await settle()

    expect(ExportImage).toHaveBeenCalledTimes(2)
    const finishes = ExportImage.mock.calls.map(c => c[0].finish)
    for (const f of finishes) {
      expect(f).toBeTruthy()
      expect(f.ratio).toBe('4:5')
      expect(f.frame).toBe('polaroid')
      expect(f.caption).toBe('Hello')
    }
    expect(finishes[0]).toEqual(finishes[1])
  })

  it('PE7: preview uses previewPost with the finish params when finish is active', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-ratio', '16:9')
    await set(wrapper, 'finish-fit', 'white')
    await settlePreview()

    expect(PreviewPost).toHaveBeenCalled()
    const last = PreviewPost.mock.calls[PreviewPost.mock.calls.length - 1]
    expect(strip(last[0])).toBe(IMG_A)
    expect(last[2]).toBeTruthy()
    expect(last[2].ratio).toBe('16:9')
    expect(last[2].fit).toBe('white')
    expect(last[3]).toBe(900)
  })

  it('PE7: without finish the preview does not call previewPost', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-frame', 'white')
    await set(wrapper, 'finish-frame', '')
    PreviewPost.mockClear()
    await settlePreview()

    expect(PreviewPost).not.toHaveBeenCalled()
  })

  it('PE7: correction without finish previews through previewAdjust', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.adjust_title'))
    const row = wrapper.findAll('.adj-row').find(r => r.find('.adj-label').text() === t('post.brightness'))
    expect(row).toBeTruthy()
    await row.find('input[type="range"]').setValue(25)
    await settlePreview()

    expect(PreviewAdjust).toHaveBeenCalled()
    expect(PreviewPost).not.toHaveBeenCalled()
  })

  it('PE7: correction with finish previews through previewPost with both', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.adjust_title'))
    const row = wrapper.findAll('.adj-row').find(r => r.find('.adj-label').text() === t('post.brightness'))
    await row.find('input[type="range"]').setValue(25)
    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-frame', 'black')
    PreviewAdjust.mockClear()
    PreviewPost.mockClear()
    await set(wrapper, 'finish-caption', 'Hi')
    await settlePreview()

    expect(PreviewPost).toHaveBeenCalled()
    const last = PreviewPost.mock.calls[PreviewPost.mock.calls.length - 1]
    expect(last[1].brightness).toBe(25)
    expect(last[2].frame).toBe('black')
    expect(last[2].caption).toBe('Hi')
    expect(PreviewAdjust).not.toHaveBeenCalled()
  })

  it('PE7: before/after comparison is hidden while finish is active', async () => {
    const wrapper = await mountPage()
    const beforeLabel = () => wrapper.findAll('span').filter(s => s.text() === t('post.before'))
    expect(beforeLabel().length).toBeGreaterThan(0)

    await openSection(wrapper, t('post.finish_title'))
    await set(wrapper, 'finish-frame', 'polaroid')
    expect(beforeLabel()).toHaveLength(0)

    await set(wrapper, 'finish-frame', '')
    expect(beforeLabel().length).toBeGreaterThan(0)
  })
})
