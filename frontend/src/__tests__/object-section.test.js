import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import ObjectSection from '../components/ObjectSection.vue'
import { GetSettings, EditObject } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'

const SOURCE = 'c291cmNl'

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

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

async function mountSection({ settings = {}, props = { source: SOURCE, active: true } } = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(ObjectSection, { props, attachTo: document.body })
  await settle()
  return wrapper
}

async function paintMask(wrapper) {
  wrapper.vm.beginStroke(10, 10, 5)
  wrapper.vm.strokeTo(12, 12, 5)
  wrapper.vm.endStroke()
  await settle()
}

const runBtn = w => w.find('[data-testid="object-run"]')
const replacement = w => w.find('[data-testid="object-replacement"]')
const objectInput = w => w.find('[data-testid="object-object"]')

enableAutoUnmount(afterEach)

describe('ObjectSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
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

  it('default op is remove: no replacement textarea, remove button label', async () => {
    const wrapper = await mountSection()
    expect(replacement(wrapper).exists()).toBe(false)
    expect(wrapper.find('[data-testid="object-op-remove"]').text()).toContain(t('object.op_remove'))
    expect(wrapper.find('[data-testid="object-op-replace"]').text()).toContain(t('object.op_replace'))
    expect(runBtn(wrapper).text()).toContain(t('object.btn_remove'))
  })

  it('settings object_op=replace makes replace the initial op', async () => {
    const wrapper = await mountSection({ settings: { object_op: 'replace' } })
    expect(replacement(wrapper).exists()).toBe(true)
    expect(runBtn(wrapper).text()).toContain(t('object.btn_replace'))
  })

  it('op buttons switch mode and toggle the replacement textarea', async () => {
    const wrapper = await mountSection()
    await wrapper.find('[data-testid="object-op-replace"]').trigger('click')
    expect(replacement(wrapper).exists()).toBe(true)
    expect(wrapper.text()).toContain(t('object.label_replacement'))
    await wrapper.find('[data-testid="object-op-remove"]').trigger('click')
    expect(replacement(wrapper).exists()).toBe(false)
  })

  it('manual object input is rendered', async () => {
    const wrapper = await mountSection()
    expect(objectInput(wrapper).exists()).toBe(true)
  })

  it('exposes mask API; maskActive true with source', async () => {
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

  it('run disabled without source', async () => {
    const wrapper = await mountSection({ props: { source: '', active: true } })
    expect(runBtn(wrapper).element.disabled).toBe(true)
  })

  it('run disabled with source but no mask, enabled after painting', async () => {
    const wrapper = await mountSection()
    expect(runBtn(wrapper).element.disabled).toBe(true)
    await paintMask(wrapper)
    expect(runBtn(wrapper).element.disabled).toBe(false)
  })

  it('replace mode: run disabled while replacement empty or whitespace', async () => {
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await wrapper.find('[data-testid="object-op-replace"]').trigger('click')
    expect(runBtn(wrapper).element.disabled).toBe(true)
    await replacement(wrapper).setValue('   ')
    expect(runBtn(wrapper).element.disabled).toBe(true)
    await replacement(wrapper).setValue('a vase')
    expect(runBtn(wrapper).element.disabled).toBe(false)
  })

  it('remove: run calls EditObject once with remove params', async () => {
    EditObject.mockResolvedValue({ image: 'NEW', detected_object: 'x', effective_prompt: 'p' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await objectInput(wrapper).setValue('  mug  ')
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(EditObject).toHaveBeenCalledTimes(1)
    const p = EditObject.mock.calls[0][0]
    expect(p.image_base64).toBe(SOURCE)
    expect(typeof p.mask_base64).toBe('string')
    expect(p.mask_base64.length).toBeGreaterThan(0)
    expect(p.op).toBe('remove')
    expect(p.replacement).toBe('')
    expect(p.object).toBe('mug')
    expect(p.seed === null || typeof p.seed === 'number').toBe(true)
  })

  it('replace: run sends trimmed replacement', async () => {
    EditObject.mockResolvedValue({ image: 'NEW', detected_object: 'x', effective_prompt: 'p' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await wrapper.find('[data-testid="object-op-replace"]').trigger('click')
    await replacement(wrapper).setValue('  a glass vase  ')
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(EditObject).toHaveBeenCalledTimes(1)
    const p = EditObject.mock.calls[0][0]
    expect(p.op).toBe('replace')
    expect(p.replacement).toBe('a glass vase')
    expect(p.object).toBe('')
  })

  it('success: emits result, shows detected object, manual field stays empty', async () => {
    EditObject.mockResolvedValue({ image: 'NEW', detected_object: 'the red mug', effective_prompt: '...' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(wrapper.emitted('result')).toBeTruthy()
    expect(wrapper.emitted('result')[0][0]).toBe('NEW')
    expect(wrapper.text()).toContain(t('object.detected', { object: 'the red mug' }))
    expect(objectInput(wrapper).element.value).toBe('')
  })

  it('success: manual object is cleared — the object is gone after the edit', async () => {
    EditObject.mockResolvedValue({ image: 'NEW', detected_object: 'the red mug', effective_prompt: '...' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await objectInput(wrapper).setValue('a cup')
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(EditObject.mock.calls[0][0].object).toBe('a cup')
    expect(objectInput(wrapper).element.value).toBe('')
  })

  it('rejection: shows error, no result emitted', async () => {
    EditObject.mockRejectedValue('server exploded')
    const wrapper = await mountSection()
    await paintMask(wrapper)
    await runBtn(wrapper).trigger('click')
    await settle()

    expect(wrapper.text()).toContain('server exploded')
    expect(wrapper.emitted('result')).toBeFalsy()
  })

  it('Ctrl+Enter triggers run when active', async () => {
    EditObject.mockResolvedValue({ image: 'NEW', detected_object: 'x', effective_prompt: 'p' })
    const wrapper = await mountSection()
    await paintMask(wrapper)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
    await settle()
    expect(EditObject).toHaveBeenCalledTimes(1)
  })

  it('Ctrl+Enter ignored when not active', async () => {
    const wrapper = await mountSection({ props: { source: SOURCE, active: false } })
    await paintMask(wrapper)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
    await settle()
    expect(EditObject).not.toHaveBeenCalled()
  })

  it('Ctrl+Enter does not run without mask', async () => {
    const wrapper = await mountSection()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
    await settle()
    expect(EditObject).not.toHaveBeenCalled()
  })
})
