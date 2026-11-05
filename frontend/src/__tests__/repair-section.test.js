import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, RepairImage: vi.fn(() => Promise.resolve(undefined)) }
})

import RepairSection from '../components/RepairSection.vue'
import ObjectSection from '../components/ObjectSection.vue'
import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, RepairImage, InterruptGeneration, GetActiveSessionItem, GetSessionImage,
  ListPresets, ListPresetTypes, ListHiresProfiles,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'

const SOURCE = 'c291cmNl'
const PREVIEW = 'UFJFVklFVw=='
const PREVIEW_URL = 'data:image/png;base64,' + PREVIEW

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

function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const OrigImage = globalThis.Image
let getContextSpy
let toDataURLSpy

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

async function mountSection({ settings = {}, props = { source: SOURCE, active: true } } = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(RepairSection, { props, attachTo: document.body })
  await settle()
  return wrapper
}

async function paintMask(wrapper) {
  wrapper.vm.beginStroke(10, 10, 5)
  wrapper.vm.strokeTo(12, 12, 5)
  wrapper.vm.endStroke()
  await settle()
}

const runBtn = w => w.find('[data-testid="repair-run"]')
const strength = w => w.find('[data-testid="repair-strength"]')
const progress = w => w.find('[data-testid="repair-progress"]')
const clearBtn = w => w.find('[data-testid="repair-clear"]')
const busyValues = w => (w.emitted('busy') || []).map(e => e[0])
const interruptButton = w => w.findAll('button').find(b => b.text().includes(t('progress.btn_interrupt')))

async function startRun(wrapper, pending) {
  RepairImage.mockReturnValue(pending.promise)
  expect(runBtn(wrapper).element.disabled).toBe(false)
  await runBtn(wrapper).trigger('click')
  await settle()
}

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  if (ListHiresProfiles) ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
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

describe('RepairSection: controls', () => {
  it('exposes mask API for painting on the main image; maskActive with source', async () => {
    const wrapper = await mountSection()
    const vm = wrapper.vm
    for (const fn of ['beginStroke', 'strokeTo', 'endStroke', 'undoMask', 'redoMask']) {
      expect(typeof vm[fn]).toBe('function')
    }
    for (const key of ['maskCanvas', 'maskVersion', 'brushSize', 'canUndoMask', 'canRedoMask']) {
      expect(key in vm).toBe(true)
    }
    expect(vm.maskActive).toBe(true)
  })

  it('maskActive false without source', async () => {
    const wrapper = await mountSection({ props: { source: '', active: true } })
    expect(wrapper.vm.maskActive).toBe(false)
  })

  it('has a brush size slider, run, clear and strength controls', async () => {
    const wrapper = await mountSection()
    expect(runBtn(wrapper).exists()).toBe(true)
    expect(clearBtn(wrapper).exists()).toBe(true)
    expect(strength(wrapper).exists()).toBe(true)
    const ranges = wrapper.findAll('input[type="range"]')
    expect(ranges.length).toBeGreaterThanOrEqual(2)
  })

  it('strength slider: range 0.3..0.9, default 0.8', async () => {
    const wrapper = await mountSection()
    const el = strength(wrapper).element
    expect(el.type).toBe('range')
    expect(Number(el.min)).toBeCloseTo(0.3)
    expect(Number(el.max)).toBeCloseTo(0.9)
    expect(Number(el.value)).toBeCloseTo(0.8)
  })

  it('run disabled without source', async () => {
    const wrapper = await mountSection({ props: { source: '', active: true } })
    expect(runBtn(wrapper).element.disabled).toBe(true)
  })

  it('run disabled without mask, enabled after painting', async () => {
    const wrapper = await mountSection()
    expect(runBtn(wrapper).element.disabled).toBe(true)
    await paintMask(wrapper)
    expect(runBtn(wrapper).element.disabled).toBe(false)
  })

  it('clear mask: disabled without mask; after clearing, run is disabled again', async () => {
    const wrapper = await mountSection()
    expect(clearBtn(wrapper).element.disabled).toBe(true)
    await paintMask(wrapper)
    expect(clearBtn(wrapper).element.disabled).toBe(false)
    await clearBtn(wrapper).trigger('click')
    await settle()
    expect(runBtn(wrapper).element.disabled).toBe(true)
  })

  it('undo after painting removes the mask, redo brings it back', async () => {
    const wrapper = await mountSection()
    await paintMask(wrapper)
    expect(wrapper.vm.canUndoMask).toBe(true)
    wrapper.vm.undoMask()
    await settle()
    expect(runBtn(wrapper).element.disabled).toBe(true)
    expect(wrapper.vm.canRedoMask).toBe(true)
    wrapper.vm.redoMask()
    await settle()
    expect(runBtn(wrapper).element.disabled).toBe(false)
  })
})

describe('RepairSection: run', () => {
  it('calls RepairImage once with image, mask, default strength 0.8 and empty seed', async () => {
    RepairImage.mockResolvedValue({ image: 'NEW', info: '{}' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(RepairImage).toHaveBeenCalledTimes(1)
    const p = RepairImage.mock.calls[0][0]
    expect(p.image_base64).toBe(SOURCE)
    expect(typeof p.mask_base64).toBe('string')
    expect(p.mask_base64.length).toBeGreaterThan(0)
    expect(p.strength).toBeCloseTo(0.8)
    expect(p.seed === null || p.seed === undefined).toBe(true)
  })

  it('sends strength and seed chosen in "more"', async () => {
    RepairImage.mockResolvedValue({ image: 'NEW', info: '{}' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await strength(wrapper).setValue('0.5')
    const seedInput = wrapper.find('input[type="number"]')
    expect(seedInput.exists()).toBe(true)
    await seedInput.setValue('123')
    await runBtn(wrapper).trigger('click')
    await settle()

    const p = RepairImage.mock.calls[0][0]
    expect(p.strength).toBeCloseTo(0.5)
    expect(p.seed).toBe(123)
  })

  it('success: emits result with the new image (new layer)', async () => {
    RepairImage.mockResolvedValue({ image: 'NEW', info: '{}' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(wrapper.emitted('result')).toBeTruthy()
    expect(wrapper.emitted('result')[0][0]).toBe('NEW')
  })

  it('rejection: shows error, no result emitted', async () => {
    RepairImage.mockRejectedValue('sd exploded')
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(wrapper.text()).toContain('sd exploded')
    expect(wrapper.emitted('result')).toBeFalsy()
  })

  it('busy=true on start, busy=false on success', async () => {
    const wrapper = await mountSection()
    await paintMask(wrapper)
    const pending = deferred()
    await startRun(wrapper, pending)
    expect(busyValues(wrapper).at(-1)).toBe(true)
    pending.resolve({ image: 'NEW', info: '{}' })
    await settle()
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('busy=false on error', async () => {
    const wrapper = await mountSection()
    await paintMask(wrapper)
    const pending = deferred()
    await startRun(wrapper, pending)
    expect(busyValues(wrapper).at(-1)).toBe(true)
    pending.reject(new Error('sd down'))
    await settle()
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('Ctrl+Enter runs when active and mask painted', async () => {
    RepairImage.mockResolvedValue({ image: 'NEW', info: '{}' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
    await settle()
    expect(RepairImage).toHaveBeenCalledTimes(1)
  })

  it('Ctrl+Enter ignored when not active', async () => {
    const wrapper = await mountSection({ props: { source: SOURCE, active: false } })
    await paintMask(wrapper)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
    await settle()
    expect(RepairImage).not.toHaveBeenCalled()
  })

  it('Ctrl+Enter does nothing without mask', async () => {
    await mountSection()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
    await settle()
    expect(RepairImage).not.toHaveBeenCalled()
  })
})

describe('RepairSection: tile progress', () => {
  it('no progress block before the run', async () => {
    const wrapper = await mountSection()
    await paintMask(wrapper)
    expect(progress(wrapper).exists()).toBe(false)
  })

  it('no progress block while running until total is known', async () => {
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await startRun(wrapper, deferred())
    expect(progress(wrapper).exists()).toBe(false)
  })

  it('repair:progress shows the current tile N of M, interrupt works, block gone after finish', async () => {
    const wrapper = await mountSection()
    await paintMask(wrapper)
    const pending = deferred()
    await startRun(wrapper, pending)

    EventsEmit('repair:progress', { done: 4, total: 7 })
    await settle()
    const block = progress(wrapper)
    expect(block.exists()).toBe(true)
    expect(block.text()).toContain('5')
    expect(block.text()).toContain('7')

    const stop = interruptButton(wrapper)
    expect(stop).toBeTruthy()
    await stop.trigger('click')
    await settle()
    expect(InterruptGeneration).toHaveBeenCalled()

    pending.resolve({ image: 'NEW', info: '{}' })
    await settle()
    expect(progress(wrapper).exists()).toBe(false)
  })

  it('repair:progress outside a run does not show the block', async () => {
    const wrapper = await mountSection()
    EventsEmit('repair:progress', { done: 1, total: 3 })
    await settle()
    expect(progress(wrapper).exists()).toBe(false)
  })
})

async function mountPage(section = 'effect') {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve([]))
  ListPresetTypes.mockReturnValue(Promise.resolve([]))
  const wrapper = mount(PostProcessPage, { props: { initialSection: section } })
  await settle()
  await wrapper.setProps({ initialImage: SOURCE })
  await settle()
  return wrapper
}

function sectionHeader(wrapper, title) {
  return wrapper.findAll('.card-toggle').find(h => h.text().includes(title))
}

describe('PostProcessPage: repair section', () => {
  it('repair section is in the accordion right after the object section', async () => {
    const wrapper = await mountPage()
    const headers = wrapper.findAll('.card-toggle').map(h => h.text())
    const objIdx = headers.findIndex(h => h.includes(t('post.section_object')))
    const repIdx = headers.findIndex(h => h.includes(t('post.section_repair')))
    expect(objIdx).toBeGreaterThanOrEqual(0)
    expect(repIdx).toBe(objIdx + 1)
    expect(wrapper.findComponent(RepairSection).exists()).toBe(true)
  })

  it('opening repair section switches the main image to mask painting with brush cursor', async () => {
    const wrapper = await mountPage()
    const header = sectionHeader(wrapper, t('post.section_repair'))
    expect(header).toBeTruthy()
    await header.trigger('click')
    await settle()

    expect(wrapper.findComponent(RepairSection).props('active')).toBe(true)
    const area = wrapper.find('.split-area')
    expect(area.classes()).toContain('masking')
    expect(wrapper.find('canvas.split-mask').exists()).toBe(true)
    await area.trigger('pointermove', { clientX: 40, clientY: 30 })
    expect(wrapper.find('.brush-cursor').exists()).toBe(true)
  })

  it('repair busy + live preview shows overlay over the photo, gone after busy=false', async () => {
    const wrapper = await mountPage()
    const header = sectionHeader(wrapper, t('post.section_repair'))
    await header.trigger('click')
    await settle()
    const section = wrapper.findComponent(RepairSection)

    section.vm.$emit('busy', true)
    await settle()
    EventsEmit('sd:progress', { progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    const overlay = wrapper.find('.split-area [data-testid="gen-overlay"]')
    expect(overlay.exists()).toBe(true)
    expect(overlay.findAll('img').some(i => i.attributes('src') === PREVIEW_URL)).toBe(true)

    section.vm.$emit('busy', false)
    await settle()
    expect(wrapper.find('[data-testid="gen-overlay"]').exists()).toBe(false)
  })

  it('object section still owns the mask when it is the open one', async () => {
    const wrapper = await mountPage('object')
    expect(wrapper.findComponent(ObjectSection).props('active')).toBe(true)
    expect(wrapper.findComponent(RepairSection).props('active')).toBe(false)
  })
})

describe('i18n: repair', () => {
  it('section title exists in ru and en', () => {
    expect(ru['post.section_repair']).toBeTruthy()
    expect(en['post.section_repair']).toBeTruthy()
  })

  it('repair.* keys exist and match between ru and en', () => {
    const ruKeys = Object.keys(ru).filter(k => k.startsWith('repair.')).sort()
    const enKeys = Object.keys(en).filter(k => k.startsWith('repair.')).sort()
    expect(ruKeys.length).toBeGreaterThan(0)
    expect(enKeys).toEqual(ruKeys)
    for (const k of ruKeys) {
      expect(ru[k]).toBeTruthy()
      expect(en[k]).toBeTruthy()
    }
  })

  it('photo restore effect renamed to Enhance Quality', () => {
    expect(ru['post.effect_photo_restore']).toBe('Улучшение качества')
    expect(en['post.effect_photo_restore']).toBe('Enhance Quality')
  })
})
