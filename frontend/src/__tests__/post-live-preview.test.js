import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, EditImage: vi.fn(() => Promise.resolve(undefined)) }
})

import PostProcessPage from '../components/PostProcessPage.vue'
import RemixSection from '../components/RemixSection.vue'
import EditSection from '../components/EditSection.vue'
import ObjectSection from '../components/ObjectSection.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListCompoundPresets, ListHiresProfiles, EnqueueFromImage, EditImage, EditObject,
  ReadFileAsBase64,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const SOURCE = 'c291cmNl'
const PREVIEW = 'UFJFVklFVw=='
const PREVIEW_URL = 'data:image/png;base64,' + PREVIEW
const OLD_PREVIEW = 'T0xEUFJFVg=='

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]

function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
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
  useRemixQueue().clearQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
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

function busyValues(wrapper) {
  return (wrapper.emitted('busy') || []).map(e => e[0])
}

function progressPreviewImgs(wrapper) {
  return wrapper.findAll('img').filter(img => [...img.element.classList].some(c => c.endsWith('-progress-preview')))
}

function interruptButton(wrapper) {
  return wrapper.findAll('button').find(b => b.text().includes(t('progress.btn_interrupt')))
}

function emitProgress(data) {
  EventsEmit('sd:progress', data)
}

async function mountRemix() {
  GetSettings.mockReturnValue(Promise.resolve({ fi_mode: 'img2img', fi_gen_mode: 'compound', fi_compound_preset_id: '3' }))
  ListCompoundPresets.mockReturnValue(Promise.resolve([{ id: 3, name: 'Pipeline', steps: [] }]))
  const wrapper = mount(RemixSection, { props: { source: '', active: true } })
  await settle()
  await wrapper.setProps({ source: SOURCE })
  await settle()
  return wrapper
}

async function startRemix(wrapper, jobId = 'remix-1') {
  EnqueueFromImage.mockReturnValue(Promise.resolve(jobId))
  const btn = wrapper.findAll('button').find(b => b.text().includes(t('fi.btn_generate')))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

async function mountEdit() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  ListPresets.mockReturnValue(Promise.resolve([]))
  const wrapper = mount(EditSection, { props: { source: SOURCE } })
  await settle()
  await wrapper.find('[data-testid="edit-instruction"]').setValue('make the sky red')
  await settle()
  return wrapper
}

async function startEdit(wrapper, pending) {
  EditImage.mockReturnValue(pending.promise)
  const btn = wrapper.findAll('button').find(b => b.text().includes(t('edit.btn_generate')))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

async function mountObject() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  const wrapper = mount(ObjectSection, { props: { source: SOURCE, active: true }, attachTo: document.body })
  await settle()
  wrapper.vm.beginStroke(10, 10, 5)
  wrapper.vm.strokeTo(12, 12, 5)
  wrapper.vm.endStroke()
  await settle()
  return wrapper
}

async function startObject(wrapper, pending) {
  EditObject.mockReturnValue(pending.promise)
  const btn = wrapper.find('[data-testid="object-run"]')
  expect(btn.exists()).toBe(true)
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

describe('sections report busy to the page', () => {
  it('Remix: busy=true on start, busy=false when the job completes', async () => {
    const wrapper = await mountRemix()
    await startRemix(wrapper, 'remix-1')
    expect(busyValues(wrapper).at(-1)).toBe(true)

    EventsEmit('queue:completed', { job_id: 'remix-1', result: JSON.stringify({ image_base64: 'cmVzdWx0' }) })
    await settle()
    expect(busyValues(wrapper)).toContain(true)
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('Remix: busy=false when the job fails', async () => {
    const wrapper = await mountRemix()
    await startRemix(wrapper, 'remix-2')
    expect(busyValues(wrapper).at(-1)).toBe(true)

    EventsEmit('queue:failed', { job_id: 'remix-2', error: 'boom' })
    await settle()
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('Remix: busy=false when enqueue itself throws', async () => {
    const wrapper = await mountRemix()
    EnqueueFromImage.mockReturnValue(Promise.reject(new Error('enqueue failed')))
    const btn = wrapper.findAll('button').find(b => b.text().includes(t('fi.btn_generate')))
    await btn.trigger('click')
    await settle()
    expect(busyValues(wrapper)).toContain(true)
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('Edit: busy=true on start, busy=false on success', async () => {
    const wrapper = await mountEdit()
    const pending = deferred()
    await startEdit(wrapper, pending)
    expect(busyValues(wrapper).at(-1)).toBe(true)

    pending.resolve({ image: 'cmVzdWx0', info: '{}' })
    await settle()
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('Edit: busy=false on error', async () => {
    const wrapper = await mountEdit()
    const pending = deferred()
    await startEdit(wrapper, pending)
    expect(busyValues(wrapper).at(-1)).toBe(true)

    pending.reject(new Error('sd down'))
    await settle()
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('Object: busy=true on start, busy=false on success', async () => {
    const wrapper = await mountObject()
    const pending = deferred()
    await startObject(wrapper, pending)
    expect(busyValues(wrapper).at(-1)).toBe(true)

    pending.resolve({ image: 'cmVzdWx0', info: '{}' })
    await settle()
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })

  it('Object: busy=false on error', async () => {
    const wrapper = await mountObject()
    const pending = deferred()
    await startObject(wrapper, pending)
    expect(busyValues(wrapper).at(-1)).toBe(true)

    pending.reject(new Error('sd down'))
    await settle()
    expect(busyValues(wrapper).at(-1)).toBe(false)
  })
})

describe('sections no longer show a small preview image', () => {
  it('Remix: no *-progress-preview img, progress row and interrupt stay', async () => {
    const wrapper = await mountRemix()
    await startRemix(wrapper, 'remix-3')
    emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    expect(progressPreviewImgs(wrapper)).toHaveLength(0)
    expect(wrapper.findAll('img').some(i => i.attributes('src') === PREVIEW_URL)).toBe(false)
    expect(wrapper.text()).toContain(t('progress.sd_step', { current: 10, total: 20 }))
    expect(interruptButton(wrapper)).toBeTruthy()
  })

  it('Edit: no *-progress-preview img, progress row and interrupt stay', async () => {
    const wrapper = await mountEdit()
    await startEdit(wrapper, deferred())
    emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    expect(progressPreviewImgs(wrapper)).toHaveLength(0)
    expect(wrapper.findAll('img').some(i => i.attributes('src') === PREVIEW_URL)).toBe(false)
    expect(wrapper.text()).toContain(t('progress.sd_step', { current: 10, total: 20 }))
    expect(interruptButton(wrapper)).toBeTruthy()
  })

  it('Object: no *-progress-preview img, interrupt stays', async () => {
    const wrapper = await mountObject()
    await startObject(wrapper, deferred())
    emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    expect(progressPreviewImgs(wrapper)).toHaveLength(0)
    expect(wrapper.findAll('img').some(i => i.attributes('src') === PREVIEW_URL)).toBe(false)
    expect(interruptButton(wrapper)).toBeTruthy()
  })
})

async function mountPage({ section = 'effect', settings = {} } = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  const wrapper = mount(PostProcessPage, { props: { initialSection: section } })
  await settle()
  await wrapper.setProps({ initialImage: SOURCE })
  await settle()
  return wrapper
}

function overlay(wrapper) {
  return wrapper.find('[data-testid="gen-overlay"]')
}

function overlayPreviewImg(wrapper) {
  return overlay(wrapper).findAll('img').find(i => i.attributes('src') === PREVIEW_URL)
}

const SECTIONS = [
  { name: 'remix', component: RemixSection },
  { name: 'edit', component: EditSection },
  { name: 'object', component: ObjectSection },
]

describe('PostProcessPage generation overlay', () => {
  it('no overlay while nothing is generating', async () => {
    const wrapper = await mountPage()
    emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    expect(overlay(wrapper).exists()).toBe(false)
  })

  for (const { name, component } of SECTIONS) {
    it(`${name}: busy + live preview covers the view area, gone after busy=false`, async () => {
      const wrapper = await mountPage({ section: name })
      const section = wrapper.findComponent(component)
      expect(section.exists()).toBe(true)

      section.vm.$emit('busy', true)
      await settle()
      emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
      await settle()

      expect(overlay(wrapper).exists()).toBe(true)
      expect(wrapper.find('.split-area [data-testid="gen-overlay"]').exists()).toBe(true)
      expect(overlayPreviewImg(wrapper)).toBeTruthy()

      section.vm.$emit('busy', false)
      await settle()
      expect(overlay(wrapper).exists()).toBe(false)
    })
  }

  it('busy without live preview: overlay with spinner, no image', async () => {
    const wrapper = await mountPage({ section: 'remix' })
    wrapper.findComponent(RemixSection).vm.$emit('busy', true)
    await settle()
    expect(overlay(wrapper).exists()).toBe(true)
    expect(overlay(wrapper).findAll('img')).toHaveLength(0)
    expect(overlay(wrapper).find('.spinner').exists()).toBe(true)
  })

  it('progress 0 with preview: spinner, not the preview', async () => {
    const wrapper = await mountPage({ section: 'remix' })
    wrapper.findComponent(RemixSection).vm.$emit('busy', true)
    await settle()
    emitProgress({ progress: 0, steps: 20, preview: PREVIEW })
    await settle()
    expect(overlay(wrapper).exists()).toBe(true)
    expect(overlayPreviewImg(wrapper)).toBeFalsy()
    expect(overlay(wrapper).find('.spinner').exists()).toBe(true)
  })

  it('progress 1 with preview: spinner, not the preview', async () => {
    const wrapper = await mountPage({ section: 'remix' })
    wrapper.findComponent(RemixSection).vm.$emit('busy', true)
    await settle()
    emitProgress({ progress: 1, steps: 20, preview: PREVIEW })
    await settle()
    expect(overlay(wrapper).exists()).toBe(true)
    expect(overlayPreviewImg(wrapper)).toBeFalsy()
  })

  it('overlay shows percent and step', async () => {
    const wrapper = await mountPage({ section: 'edit' })
    wrapper.findComponent(EditSection).vm.$emit('busy', true)
    await settle()
    emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    const text = overlay(wrapper).text()
    expect(text).toContain('50%')
    expect(text).toContain(t('progress.sd_step', { current: 10, total: 20 }))
  })

  it('overlay shows ETA from the backend sd:progress payload (eta_relative)', async () => {
    const wrapper = await mountPage({ section: 'edit' })
    wrapper.findComponent(EditSection).vm.$emit('busy', true)
    await settle()
    emitProgress({ progress: 0.5, steps: 20, eta_relative: 12.3, preview: PREVIEW })
    await settle()
    expect(overlay(wrapper).text()).toMatch(/1[23]/)
  })

  it('previous generation preview is not shown at the start of a new one', async () => {
    const wrapper = await mountPage({ section: 'remix' })
    const section = wrapper.findComponent(RemixSection)

    section.vm.$emit('busy', true)
    await settle()
    emitProgress({ progress: 0.5, steps: 20, preview: OLD_PREVIEW })
    await settle()
    section.vm.$emit('busy', false)
    await settle()

    section.vm.$emit('busy', true)
    await settle()
    expect(overlay(wrapper).exists()).toBe(true)
    expect(overlay(wrapper).findAll('img')).toHaveLength(0)

    emitProgress({ progress: 0.2, steps: 20, preview: PREVIEW })
    await settle()
    expect(overlayPreviewImg(wrapper)).toBeTruthy()
  })

  it('applying an effect + sd:progress shows the overlay; it disappears when the job completes', async () => {
    const wrapper = await mountPage({ section: 'effect' })
    EnqueueFromImage.mockReturnValue(Promise.resolve('fx-1'))
    const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
    await card.trigger('click')
    await settle()
    const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
    expect(apply.element.disabled).toBe(false)
    await apply.trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalled()

    emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    expect(overlay(wrapper).exists()).toBe(true)
    expect(overlayPreviewImg(wrapper)).toBeTruthy()

    EventsEmit('queue:completed', { job_id: 'fx-1', result: JSON.stringify({ image_base64: 'cmVzdWx0' }) })
    await settle()
    expect(overlay(wrapper).exists()).toBe(false)
  })

  it('folder pipeline run + sd:progress: no overlay', async () => {
    useRemixQueue().setPostQueue([
      { path: '/in/a.png', name: 'a.png' },
      { path: '/in/b.png', name: 'b.png' },
    ])
    ReadFileAsBase64.mockReturnValue(Promise.resolve(SOURCE))
    EnqueueFromImage.mockReturnValue(Promise.resolve('pipe-1'))
    GetSettings.mockReturnValue(Promise.resolve({ file_browser_path: '/out' }))
    GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
    GetSessionImage.mockReturnValue(Promise.resolve(''))
    ListPresets.mockReturnValue(Promise.resolve(PRESETS))
    ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
    const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' } })
    await settle()

    const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
    await card.trigger('click')
    await settle()

    const run = wrapper.findAll('button').find(b => b.text().includes(t('post.pipeline_run', { n: 2 })))
    expect(run).toBeTruthy()
    expect(run.element.disabled).toBe(false)
    await run.trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalled()
    expect(wrapper.text()).toContain(t('post.pipeline_running', { done: 0, total: 2 }))

    emitProgress({ progress: 0.5, steps: 20, preview: PREVIEW })
    await settle()
    expect(overlay(wrapper).exists()).toBe(false)
  })
})
