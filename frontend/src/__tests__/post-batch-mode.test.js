import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base }
})

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, EnqueueFromImage, ReadFileAsBase64, BatchSaveImage, BatchReset, ExportImage,
  EnqueueLook, ApplyLookDirect, CancelQueueJob, ReadImageFile, ListPostRecipes,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]

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

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  useRemixQueue().clearQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  files = { '/photos/a.png': IMG_A, '/photos/b.png': IMG_B }
  savedCount = 0
  jobCount = 0
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

async function mountBatch({ section = 'effect', settings = {}, items = TWO } = {}) {
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

async function startEffectStep(wrapper) {
  const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
  const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect_all', { n: 2 })))
  expect(apply).toBeTruthy()
  expect(apply.element.disabled).toBe(false)
  await apply.trigger('click')
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

function redoButton(wrapper) {
  return wrapper.findAll('button').find(b => b.attributes('title') === t('post.redo'))
}

describe('PostProcessPage batch mode', () => {
  it('K1: two photos from My Images open the batch with two thumbnails and reset the batch store', async () => {
    const wrapper = await mountBatch()
    expect(wrapper.find('.batch-panel').exists()).toBe(true)
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(2)
    expect(BatchReset).toHaveBeenCalled()
  })

  it('K1: one photo stays in normal mode', async () => {
    const wrapper = await mountBatch({ items: [TWO[0]] })
    expect(wrapper.find('.batch-panel').exists()).toBe(false)
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(0)
  })

  it('K2: apply effect to all enqueues every photo, stores each result as a version, does not save files', async () => {
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)

    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    expect(strip(EnqueueFromImage.mock.calls[0][0].image_base64)).toBe(IMG_A)
    expect(strip(EnqueueFromImage.mock.calls[1][0].image_base64)).toBe(IMG_B)

    EventsEmit('queue:completed', { job_id: 'job-1', result: JSON.stringify({ image_base64: RES_A }) })
    await settle()
    EventsEmit('queue:completed', { job_id: 'job-2', result: JSON.stringify({ image_base64: RES_B }) })
    await settle()

    expect(BatchSaveImage).toHaveBeenCalledTimes(2)
    expect(BatchSaveImage.mock.calls.map(c => strip(c[0])).sort()).toEqual([RES_A, RES_B].sort())
    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
    expect(ExportImage).not.toHaveBeenCalled()
  })

  it('K3: single-photo tools show the batch hint, Edit offers only the look mode, no pipeline section', async () => {
    const wrapper = await mountBatch()
    for (const key of ['post.section_remix', 'post.section_object', 'post.section_repair']) {
      const header = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(t(key)))
      expect(header, key).toBeTruthy()
      const cardEl = header.element.closest('.card')
      expect(cardEl.textContent, key).toContain(t('post.batch_unavailable'))
    }
    const editHeader = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(t('post.section_edit')))
    expect(editHeader.element.closest('.card').textContent).toContain(t('edit.batch_look_only'))
    expect(wrapper.text()).not.toContain('Конвейер')
  })

  it('K4: save all writes every photo to the default folder under its own name with untouched size', async () => {
    const wrapper = await mountBatch({ settings: { file_browser_path: '/out' } })
    await openSection(wrapper, t('export.export_settings'))
    const btn = buttonWith(wrapper, t('post.batch_save_all', { n: 2 }))
    expect(btn).toBeTruthy()
    expect(btn.element.disabled).toBe(false)
    await btn.trigger('click')
    await settle()

    expect(ExportImage).toHaveBeenCalledTimes(2)
    const calls = ExportImage.mock.calls.map(c => c[0])
    expect(calls.map(c => c.filename)).toEqual(['a.png', 'b.png'])
    for (const c of calls) {
      expect(c.output_dir).toBe('/out')
      expect(c.format).toBe('png')
      expect(c.width).toBe(0)
      expect(c.height).toBe(0)
    }
    expect(strip(calls[0].image_base64)).toBe(IMG_A)
    expect(strip(calls[1].image_base64)).toBe(IMG_B)
  })

  it('K5: exit batch returns to normal mode and clears the batch store', async () => {
    const wrapper = await mountBatch()
    const resetsBefore = BatchReset.mock.calls.length
    const exit = buttonWith(wrapper, t('post.batch_exit'))
    expect(exit).toBeTruthy()
    await exit.trigger('click')
    await settle()
    expect(wrapper.find('.batch-panel').exists()).toBe(false)
    expect(BatchReset.mock.calls.length).toBeGreaterThan(resetsBefore)
  })

  it('K6: every photo of the step is enqueued before the first job completes', async () => {
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)

    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    expect(EnqueueFromImage.mock.calls.map(c => strip(c[0].image_base64))).toEqual([IMG_A, IMG_B])
    expect(BatchSaveImage).not.toHaveBeenCalled()
  })

  it('K7: free-text look in batch enqueues a look job per photo and never calls the direct look', async () => {
    let lookCount = 0
    EnqueueLook.mockImplementation(() => {
      lookCount++
      return Promise.resolve('look-' + lookCount)
    })
    const wrapper = await mountBatch({ section: 'edit' })
    const editCard = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(t('post.section_edit'))).element.closest('.card')
    const freeBtn = wrapper.findAll('button').find(b => b.text() === t('edit.look_mode_free') && editCard.contains(b.element))
    expect(freeBtn).toBeTruthy()
    await freeBtn.trigger('click')
    await settle()
    await wrapper.find('[data-testid="edit-look-text"]').setValue('warm sunset glow')
    await settle()
    const apply = wrapper.findAll('button').find(b => b.text().includes(t('edit.look_apply')) && editCard.contains(b.element))
    expect(apply).toBeTruthy()
    expect(apply.element.disabled).toBe(false)
    await apply.trigger('click')
    await settle()

    expect(EnqueueLook).toHaveBeenCalledTimes(2)
    const calls = EnqueueLook.mock.calls.map(c => c[0])
    expect(calls.map(c => strip(c.image_base64))).toEqual([IMG_A, IMG_B])
    for (const c of calls) {
      expect(c.text).toBe('warm sunset glow')
      expect(typeof c.strength).toBe('number')
    }
    expect(ApplyLookDirect).not.toHaveBeenCalled()

    await completeJob('look-1', RES_A)
    await completeJob('look-2', RES_B)
    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
    expect(ApplyLookDirect).not.toHaveBeenCalled()
  })

  it('K8: loading a new photo during a step leaves the batch and cancels the unfinished jobs', async () => {
    const IMG_NEW = 'TkVXTg=='
    ReadImageFile.mockImplementation(() => Promise.resolve(IMG_NEW))
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)

    expect(buttonWith(wrapper, t('post.batch_exit')).element.disabled).toBe(true)

    await buttonWith(wrapper, t('export.btn_upload')).trigger('click')
    await settle()

    expect(wrapper.find('.batch-panel').exists()).toBe(false)
    const cancelled = CancelQueueJob.mock.calls.map(c => c[0])
    expect(cancelled).toEqual(expect.arrayContaining(['job-1', 'job-2']))
    expect(previewSrc(wrapper)).toContain(IMG_NEW)
  })

  it('K8: a job finished before the exit is not cancelled', async () => {
    ReadImageFile.mockImplementation(() => Promise.resolve('TkVXTg=='))
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)
    await completeJob('job-1', RES_A)

    await buttonWith(wrapper, t('export.btn_upload')).trigger('click')
    await settle()

    const cancelled = CancelQueueJob.mock.calls.map(c => c[0])
    expect(cancelled).toContain('job-2')
    expect(cancelled).not.toContain('job-1')
  })

  it('K9: undo button returns the batch to its sources, redo stays unavailable', async () => {
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)
    await completeJob('job-1', RES_A)
    await completeJob('job-2', RES_B)
    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
    expect(previewSrc(wrapper)).toContain(RES_A)

    const undo = undoButton(wrapper)
    expect(undo).toBeTruthy()
    expect(undo.element.disabled).toBe(false)
    await undo.trigger('click')
    await settle()

    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 0 }))
    expect(previewSrc(wrapper)).toContain(IMG_A)
    expect(redoButton(wrapper).element.disabled).toBe(true)
  })

  it('K9: Ctrl+Z undoes the batch step', async () => {
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)
    await completeJob('job-1', RES_A)
    await completeJob('job-2', RES_B)

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'z', code: 'KeyZ', ctrlKey: true, bubbles: true }))
    await settle()

    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 0 }))
    expect(previewSrc(wrapper)).toContain(IMG_A)
    expect(redoButton(wrapper).element.disabled).toBe(true)
  })

  it('K10: a failed job marks its thumbnail and reports one failed photo', async () => {
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)
    await completeJob('job-1', RES_A)
    EventsEmit('queue:failed', { job_id: 'job-2', error: 'sd crashed' })
    await settle()

    const thumbs = wrapper.findAll('.batch-thumb')
    expect(thumbs[0].classes()).not.toContain('failed')
    expect(thumbs[1].classes()).toContain('failed')
    expect(wrapper.text()).toContain(t('post.batch_step_failed', { n: 1 }))
    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
  })

  it('K11: clicking the second thumbnail shows the second photo', async () => {
    const wrapper = await mountBatch()
    expect(previewSrc(wrapper)).toContain(IMG_A)

    await wrapper.findAll('.batch-thumb')[1].trigger('click')
    await settle()

    expect(previewSrc(wrapper)).toContain(IMG_B)
    expect(wrapper.findAll('.batch-thumb')[1].classes()).toContain('active')
  })

  it('K12: correction is applied to every photo on save all', async () => {
    const wrapper = await mountBatch({ settings: { file_browser_path: '/out' } })
    await openSection(wrapper, t('post.adjust_title'))
    const row = wrapper.findAll('.adj-row').find(r => r.find('.adj-label').text() === t('post.brightness'))
    expect(row).toBeTruthy()
    await row.find('input[type="range"]').setValue(25)
    await settle()

    await openSection(wrapper, t('export.export_settings'))
    await buttonWith(wrapper, t('post.batch_save_all', { n: 2 })).trigger('click')
    await settle()

    expect(ExportImage).toHaveBeenCalledTimes(2)
    for (const [params] of ExportImage.mock.calls) {
      expect(params.adjust).toBeTruthy()
      expect(params.adjust.brightness).toBe(25)
    }
  })

  it('K13: two-effect recipe chains each photo through two jobs and records one step', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{
      id: 7,
      name: 'Double Soft',
      steps: JSON.stringify([{ preset_id: 10, strength: 0.5 }, { preset_id: 10, strength: 0.6 }]),
      adjust: '{}',
    }]))
    const wrapper = await mountBatch({ section: 'recipes' })
    const item = wrapper.findAll('.preset-item').find(i => i.text().includes('Double Soft'))
    expect(item).toBeTruthy()
    await item.trigger('click')
    await settle()

    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    expect(EnqueueFromImage.mock.calls.map(c => strip(c[0].image_base64))).toEqual([IMG_A, IMG_B])

    await completeJob('job-1', RES_A)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(3)
    expect(strip(EnqueueFromImage.mock.calls[2][0].image_base64)).toBe(RES_A)

    await completeJob('job-2', RES_B)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(4)
    expect(strip(EnqueueFromImage.mock.calls[3][0].image_base64)).toBe(RES_B)

    await completeJob('job-3', 'RklOQQ==')
    await completeJob('job-4', 'RklOQg==')

    expect(EnqueueFromImage).toHaveBeenCalledTimes(4)
    expect(wrapper.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
    expect(previewSrc(wrapper)).toContain('RklOQQ==')
  })

  it('K14: a job id arriving after the exit is cancelled and normal mode apply is not blocked', async () => {
    ReadImageFile.mockImplementation(() => Promise.resolve('TkVXTg=='))
    let release
    EnqueueFromImage.mockImplementation(() => {
      jobCount++
      if (jobCount === 2) return new Promise(r => { release = r })
      return Promise.resolve('job-' + jobCount)
    })
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    expect(release).toBeTypeOf('function')

    await buttonWith(wrapper, t('export.btn_upload')).trigger('click')
    await settle()
    expect(wrapper.find('.batch-panel').exists()).toBe(false)

    release('job-2')
    await settle()

    const cancelled = CancelQueueJob.mock.calls.map(c => c[0])
    expect(cancelled).toEqual(expect.arrayContaining(['job-1', 'job-2']))

    const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
    if (card && !wrapper.findAll('.fx-apply-bar button').length) {
      await card.trigger('click')
      await settle()
    }
    const buttons = wrapper.findAll('.fx-apply-bar button')
    expect(buttons.some(b => b.text().includes(t('post.applying')))).toBe(false)
    const apply = buttons.find(b => b.text().includes(t('post.btn_apply_effect')))
    expect(apply).toBeTruthy()
    expect(apply.element.disabled).toBe(false)
  })
})

const IMG_C = 'Q0NDQw=='
const THREE = [...TWO, { path: '/photos/c.png', name: 'c.png' }]

function removeButtons(wrapper) {
  return wrapper.findAll('.batch-thumb .batch-thumb-remove')
}

async function removeAt(wrapper, i) {
  const btn = removeButtons(wrapper)[i]
  expect(btn).toBeTruthy()
  expect(btn.attributes('aria-label')).toBe(t('post.batch_remove'))
  await btn.trigger('click')
  await settle()
}

async function selectEffect(wrapper) {
  const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
}

describe('PostProcessPage batch remove', () => {
  beforeEach(() => {
    files['/photos/c.png'] = IMG_C
  })

  it('K15: every thumbnail has a remove button', async () => {
    const wrapper = await mountBatch({ items: THREE })
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(3)
    expect(removeButtons(wrapper)).toHaveLength(3)
    for (const b of removeButtons(wrapper)) expect(b.element.disabled).toBe(false)
  })

  it('K15: removing the second photo leaves two thumbnails and updates the counters', async () => {
    const wrapper = await mountBatch({ items: THREE, settings: { file_browser_path: '/out' } })
    expect(wrapper.text()).toContain(t('post.batch_title', { n: 3 }))

    await removeAt(wrapper, 1)

    expect(wrapper.findAll('.batch-thumb')).toHaveLength(2)
    expect(wrapper.find('.batch-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain(t('post.batch_title', { n: 2 }))
    expect(wrapper.text()).not.toContain(t('post.batch_title', { n: 3 }))

    await selectEffect(wrapper)
    const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect_all', { n: 2 })))
    expect(apply).toBeTruthy()

    await openSection(wrapper, t('export.export_settings'))
    expect(buttonWith(wrapper, t('post.batch_save_all', { n: 2 }))).toBeTruthy()
    expect(buttonWith(wrapper, t('post.batch_save_all', { n: 3 }))).toBeFalsy()
  })

  it('K15: removing a photo that is not selected keeps the selected photo on the preview', async () => {
    const wrapper = await mountBatch({ items: THREE })
    expect(previewSrc(wrapper)).toContain(IMG_A)

    await removeAt(wrapper, 1)

    expect(previewSrc(wrapper)).toContain(IMG_A)
    expect(wrapper.findAll('.batch-thumb')[0].classes()).toContain('active')
  })

  it('K15: removing an earlier photo keeps a later selected photo selected', async () => {
    const wrapper = await mountBatch({ items: THREE })
    await wrapper.findAll('.batch-thumb')[2].trigger('click')
    await settle()
    expect(previewSrc(wrapper)).toContain(IMG_C)

    await removeAt(wrapper, 0)

    expect(previewSrc(wrapper)).toContain(IMG_C)
    const thumbs = wrapper.findAll('.batch-thumb')
    expect(thumbs).toHaveLength(2)
    expect(thumbs[1].classes()).toContain('active')
  })

  it('K15: removing the selected photo switches the preview to a neighbour', async () => {
    const wrapper = await mountBatch({ items: THREE })
    await wrapper.findAll('.batch-thumb')[1].trigger('click')
    await settle()
    expect(previewSrc(wrapper)).toContain(IMG_B)

    await removeAt(wrapper, 1)

    const src = previewSrc(wrapper)
    expect(src).not.toContain(IMG_B)
    expect(src.includes(IMG_A) || src.includes(IMG_C)).toBe(true)
    expect(wrapper.findAll('.batch-thumb.active')).toHaveLength(1)
  })

  it('K15: removing a photo does not touch files or history', async () => {
    const wrapper = await mountBatch({ items: THREE })
    await removeAt(wrapper, 1)
    expect(ExportImage).not.toHaveBeenCalled()
    expect(files['/photos/b.png']).toBe(IMG_B)
  })

  it('K16: removing every photo leaves the batch and resets the batch store', async () => {
    const wrapper = await mountBatch()
    const resetsBefore = BatchReset.mock.calls.length

    await removeAt(wrapper, 0)
    if (removeButtons(wrapper).length) await removeAt(wrapper, 0)

    expect(wrapper.find('.batch-panel').exists()).toBe(false)
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(0)
    expect(BatchReset.mock.calls.length).toBeGreaterThan(resetsBefore)
  })

  it('K17: remove buttons are disabled while a step is running', async () => {
    const wrapper = await mountBatch()
    await startEffectStep(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)

    const btns = removeButtons(wrapper)
    expect(btns).toHaveLength(2)
    for (const b of btns) expect(b.element.disabled).toBe(true)

    await btns[0].trigger('click')
    await settle()
    expect(wrapper.findAll('.batch-thumb')).toHaveLength(2)

    await completeJob('job-1', RES_A)
    await completeJob('job-2', RES_B)
    for (const b of removeButtons(wrapper)) expect(b.element.disabled).toBe(false)
  })

  it('K17: remove buttons are disabled while saving all', async () => {
    let finish
    ExportImage.mockImplementation(() => new Promise(r => { finish = r }))
    const wrapper = await mountBatch({ settings: { file_browser_path: '/out' } })
    await openSection(wrapper, t('export.export_settings'))
    await buttonWith(wrapper, t('post.batch_save_all', { n: 2 })).trigger('click')
    await settle()
    expect(ExportImage).toHaveBeenCalled()

    for (const b of removeButtons(wrapper)) expect(b.element.disabled).toBe(true)
    expect(removeButtons(wrapper)).toHaveLength(2)

    finish('/out/a.png')
    await settle()
    if (finish) finish('/out/b.png')
    await settle()
  })
  it('K18: Enter on a remove button removes the photo without selecting it, space selects a thumbnail', async () => {
    const wrapper = await mountBatch({ items: THREE })
    expect(previewSrc(wrapper)).toContain(IMG_A)

    const btn = removeButtons(wrapper)[2]
    await btn.trigger('keydown', { key: 'Enter' })
    await settle()
    expect(wrapper.findAll('.batch-thumb')[0].classes()).toContain('active')
    expect(wrapper.findAll('.batch-thumb')[2].classes()).not.toContain('active')
    await btn.trigger('click')
    await settle()

    const thumbs = wrapper.findAll('.batch-thumb')
    expect(thumbs).toHaveLength(2)
    expect(thumbs[0].classes()).toContain('active')
    expect(previewSrc(wrapper)).toContain(IMG_A)

    await thumbs[1].trigger('keydown', { key: ' ' })
    await settle()
    expect(wrapper.findAll('.batch-thumb')[1].classes()).toContain('active')
    expect(previewSrc(wrapper)).toContain(IMG_B)
  })

  it('K18: Enter on a thumbnail selects it', async () => {
    const wrapper = await mountBatch({ items: THREE })
    await wrapper.findAll('.batch-thumb')[2].trigger('keydown', { key: 'Enter' })
    await settle()
    expect(wrapper.findAll('.batch-thumb')[2].classes()).toContain('active')
    expect(previewSrc(wrapper)).toContain(IMG_C)
  })
})
