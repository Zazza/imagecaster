import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, EditImage: vi.fn(() => Promise.resolve(undefined)), EnqueueEdit: vi.fn(() => Promise.resolve(undefined)) }
})

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes, ListCompoundPresets,
  ListHiresProfiles, EnqueueFromImage, EnqueueEdit, EditImage, AnalyzeImage, ReadFileAsBase64,
  BatchSaveImage, BatchReset, ExportImage, CancelQueueJob, ReadImageFile,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]
const COMPOUND = [{ id: 3, name: 'Pipeline', steps: [] }]
const REMIX_SETTINGS = { fi_mode: 'img2img', fi_gen_mode: 'compound', fi_compound_preset_id: '3', fi_count: '3' }

const IMG_A = 'QUFBQQ=='
const IMG_B = 'QkJCQg=='
const RES_A = 'UkVTQQ=='
const RES_B = 'UkVTQg=='

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
let files
let savedCount
let jobCount
let editCount

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  ListCompoundPresets.mockImplementation(() => Promise.resolve(COMPOUND))
  files = { '/photos/a.png': IMG_A, '/photos/b.png': IMG_B }
  savedCount = 0
  jobCount = 0
  editCount = 0
  ReadFileAsBase64.mockImplementation(p => Promise.resolve(files[p] || ''))
  BatchSaveImage.mockImplementation(b64 => {
    savedCount++
    const p = '/batch/v' + savedCount + '.png'
    files[p] = String(b64).replace(/^data:[^,]*,/, '')
    return Promise.resolve(p)
  })
  BatchReset.mockImplementation(() => Promise.resolve())
  EnqueueFromImage.mockImplementation(() => {
    jobCount++
    return Promise.resolve('job-' + jobCount)
  })
  EnqueueEdit.mockImplementation(() => {
    editCount++
    return Promise.resolve('edit-' + editCount)
  })
  EditImage.mockImplementation(() => Promise.resolve({ image: RES_A }))
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

const TWO = [{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }]

async function mountPage({ section = 'remix', settings = REMIX_SETTINGS, items = TWO } = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  useRemixQueue().setPostQueue(items.map(i => ({ ...i })))
  const wrapper = mount(PostProcessPage, { props: { initialSection: section } })
  await settle()
  return wrapper
}

function strip(b64) {
  return String(b64 || '').replace(/^data:[^,]*,/, '')
}

function card(wrapper, key) {
  const header = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(t(key)))
  expect(header, key).toBeTruthy()
  return header.element.closest('.card')
}

function buttonIn(wrapper, cardEl, text) {
  return wrapper.findAll('button').find(b => b.text().includes(text) && cardEl.contains(b.element))
}

async function click(btn) {
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

async function completeJob(id, b64) {
  EventsEmit('queue:completed', { job_id: id, result: JSON.stringify({ image_base64: b64 }) })
  await settle()
}

function previewSrc(wrapper) {
  return wrapper.find('.split-img.split-after').attributes('src') || ''
}

function undoButton(wrapper) {
  return wrapper.findAll('button').find(b => b.attributes('title') === t('post.undo'))
}

async function applyRemixToAll(wrapper) {
  const remix = card(wrapper, 'post.section_remix')
  await click(buttonIn(wrapper, remix, t('fi.btn_apply_all')))
}

async function applyEditToAll(wrapper, instruction) {
  const edit = card(wrapper, 'post.section_edit')
  await click(buttonIn(wrapper, edit, t('edit.mode_instruction')))
  const input = wrapper.findAll('[data-testid="edit-instruction"]').find(i => edit.contains(i.element))
  expect(input).toBeTruthy()
  await input.setValue(instruction)
  await settle()
  await click(buttonIn(wrapper, edit, t('edit.btn_apply_all')))
}

function withoutImage(params) {
  const { image_base64, ...rest } = params
  return rest
}

describe('batch remix', () => {
  it('BR3: remix in batch has no unavailable hint and offers apply to all', async () => {
    const wrapper = await mountPage()
    const remix = card(wrapper, 'post.section_remix')
    expect(remix.textContent).not.toContain(t('post.batch_unavailable'))
    expect(buttonIn(wrapper, remix, t('fi.btn_apply_all'))).toBeTruthy()
  })

  it('BR3: apply to all enqueues one from_image job per photo with that photo and the same settings, ignoring variant count 3', async () => {
    const wrapper = await mountPage()
    await applyRemixToAll(wrapper)

    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    const calls = EnqueueFromImage.mock.calls.map(c => c[0])
    expect(calls.map(c => strip(c.image_base64))).toEqual([IMG_A, IMG_B])
    expect(withoutImage(calls[0])).toEqual(withoutImage(calls[1]))
    expect(calls[0].compound_preset_id).toBe(3)
  })

  it('BR3: completing remix jobs does not enqueue more variants', async () => {
    const wrapper = await mountPage()
    await applyRemixToAll(wrapper)
    await completeJob('job-1', RES_A)
    await completeJob('job-2', RES_B)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
  })

  it('R3: remix in batch shows the describe hint and describes only the open photo', async () => {
    AnalyzeImage.mockImplementation(() => Promise.resolve('a photo'))
    const wrapper = await mountPage()
    const remix = card(wrapper, 'post.section_remix')
    expect(remix.textContent).toContain(t('fi.batch_describe_hint'))

    await click(buttonIn(wrapper, remix, t('fi.btn_analyze')))
    expect(AnalyzeImage).toHaveBeenCalledTimes(1)
    expect(strip(AnalyzeImage.mock.calls[0][0])).toBe(IMG_A)
  })

  it('BR5: completed remix jobs give one batch step, undo returns both photos to their previous versions', async () => {
    const wrapper = await mountPage()
    await applyRemixToAll(wrapper)
    await completeJob('job-1', RES_A)
    await completeJob('job-2', RES_B)

    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
    expect(previewSrc(wrapper)).toContain(RES_A)
    await wrapper.findAll('.batch-thumb')[1].trigger('click')
    await settle()
    expect(previewSrc(wrapper)).toContain(RES_B)

    await click(undoButton(wrapper))

    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 0 }))
    expect(previewSrc(wrapper)).toContain(IMG_B)
    await wrapper.findAll('.batch-thumb')[0].trigger('click')
    await settle()
    expect(previewSrc(wrapper)).toContain(IMG_A)
  })

  it('BR5: a failed remix job keeps that photo and marks its thumbnail', async () => {
    const wrapper = await mountPage()
    await applyRemixToAll(wrapper)
    await completeJob('job-1', RES_A)
    EventsEmit('queue:failed', { job_id: 'job-2', error: 'sd crashed' })
    await settle()

    const thumbs = wrapper.findAll('.batch-thumb')
    expect(thumbs[0].classes()).not.toContain('failed')
    expect(thumbs[1].classes()).toContain('failed')
    await thumbs[1].trigger('click')
    await settle()
    expect(previewSrc(wrapper)).toContain(IMG_B)
    expect(previewSrc(wrapper)).not.toContain(RES_B)
  })

  it('R8: leaving the batch during a remix step cancels its unfinished jobs', async () => {
    ReadImageFile.mockImplementation(() => Promise.resolve('TkVXTg=='))
    const wrapper = await mountPage()
    await applyRemixToAll(wrapper)
    await completeJob('job-1', RES_A)

    await wrapper.findAll('button').find(b => b.text().includes(t('export.btn_upload'))).trigger('click')
    await settle()

    const cancelled = CancelQueueJob.mock.calls.map(c => c[0])
    expect(cancelled).toContain('job-2')
    expect(cancelled).not.toContain('job-1')
  })
})

describe('batch edit by instruction', () => {
  it('BR4: apply to all enqueues one edit job per photo with its image and the instruction, never the direct edit', async () => {
    const wrapper = await mountPage({ section: 'edit', settings: {} })
    await applyEditToAll(wrapper, 'добавь котика')

    expect(EnqueueEdit).toHaveBeenCalledTimes(2)
    const calls = EnqueueEdit.mock.calls.map(c => c[0])
    expect(calls.map(c => strip(c.image_base64))).toEqual([IMG_A, IMG_B])
    for (const c of calls) expect(c.instruction).toBe('добавь котика')
    expect(withoutImage(calls[0])).toEqual(withoutImage(calls[1]))
    expect(EditImage).not.toHaveBeenCalled()
  })

  it('BR4: edit seed is the same for every photo', async () => {
    const wrapper = await mountPage({ section: 'edit', settings: {} })
    const edit = card(wrapper, 'post.section_edit')
    await click(buttonIn(wrapper, edit, t('edit.mode_instruction')))
    const seed = wrapper.findAll('[data-testid="edit-seed-input"]').find(i => edit.contains(i.element))
    expect(seed).toBeTruthy()
    await seed.setValue('42')
    await settle()
    await applyEditToAll(wrapper, 'добавь котика')

    expect(EnqueueEdit).toHaveBeenCalledTimes(2)
    expect(EnqueueEdit.mock.calls.map(c => Number(c[0].seed))).toEqual([42, 42])
  })

  it('BR5: completed edit jobs give one batch step, undo returns the previous versions', async () => {
    const wrapper = await mountPage({ section: 'edit', settings: {} })
    await applyEditToAll(wrapper, 'добавь котика')
    await completeJob('edit-1', RES_A)
    await completeJob('edit-2', RES_B)

    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
    expect(previewSrc(wrapper)).toContain(RES_A)

    await click(undoButton(wrapper))

    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 0 }))
    expect(previewSrc(wrapper)).toContain(IMG_A)
    await wrapper.findAll('.batch-thumb')[1].trigger('click')
    await settle()
    expect(previewSrc(wrapper)).toContain(IMG_B)
  })

  it('BR5: a failed edit job keeps that photo and marks its thumbnail', async () => {
    const wrapper = await mountPage({ section: 'edit', settings: {} })
    await applyEditToAll(wrapper, 'добавь котика')
    EventsEmit('queue:failed', { job_id: 'edit-1', error: 'sd crashed' })
    await settle()
    await completeJob('edit-2', RES_B)

    const thumbs = wrapper.findAll('.batch-thumb')
    expect(thumbs[0].classes()).toContain('failed')
    expect(thumbs[1].classes()).not.toContain('failed')
    expect(previewSrc(wrapper)).toContain(IMG_A)
    expect(wrapper.text()).toContain(t('post.batch_step_failed', { n: 1 }))
  })
})

describe('single photo keeps the old behaviour', () => {
  it('BR6: edit calls the direct edit, not the queue', async () => {
    const wrapper = await mountPage({ section: 'edit', settings: {}, items: [TWO[0]] })
    expect(wrapper.find('.batch-panel').exists()).toBe(false)
    const edit = card(wrapper, 'post.section_edit')
    const input = wrapper.findAll('[data-testid="edit-instruction"]').find(i => edit.contains(i.element))
    expect(input).toBeTruthy()
    await input.setValue('добавь котика')
    await settle()
    await click(buttonIn(wrapper, edit, t('edit.btn_generate')))

    expect(EditImage).toHaveBeenCalledTimes(1)
    expect(EditImage.mock.calls[0][0].instruction).toBe('добавь котика')
    expect(strip(EditImage.mock.calls[0][0].image_base64)).toBe(IMG_A)
    expect(EnqueueEdit).not.toHaveBeenCalled()
    expect(buttonIn(wrapper, edit, t('edit.btn_apply_all'))).toBeFalsy()
  })

  it('BR6: remix enqueues N jobs for N variants', async () => {
    const wrapper = await mountPage({ items: [TWO[0]] })
    const remix = card(wrapper, 'post.section_remix')
    expect(buttonIn(wrapper, remix, t('fi.btn_apply_all'))).toBeFalsy()
    await click(buttonIn(wrapper, remix, t('fi.btn_generate_n', { count: 3 })))

    expect(EnqueueFromImage).toHaveBeenCalledTimes(3)
    for (const [p] of EnqueueFromImage.mock.calls) expect(strip(p.image_base64)).toBe(IMG_A)
  })
})

describe('batch single-photo tools', () => {
  it('BR7: Object and Repair show the unavailable hint in batch', async () => {
    const wrapper = await mountPage({ settings: {} })
    for (const key of ['post.section_object', 'post.section_repair']) {
      expect(card(wrapper, key).textContent, key).toContain(t('post.batch_unavailable'))
    }
  })
})
