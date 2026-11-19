import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes, ListHiresProfiles,
  EnqueueFromImage, EnqueuePost4K, ListPostRecipes, SavePostRecipe, GetQueue, CancelQueueJob,
  ReadFileAsBase64, BatchSaveImage, BatchReset,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle, waitUntil } from './helpers/async.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]
const SRC = 'U1JDQQ=='
const BIG = 'QklHNEs='
const FX = 'RlhSRVM='
const IMG_A = 'QUFBQQ=='
const IMG_B = 'QkJCQg=='
const BIG_A = 'QklHQQ=='
const BIG_B = 'QklHQg=='

useFakeImage()
useFakeCanvas()

let queue
let jobId
let files

function strip(b64) {
  return String(b64 || '').replace(/^data:[^,]*,/, '')
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  queue = []
  jobId = 40
  files = { '/photos/a.png': IMG_A, '/photos/b.png': IMG_B }
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  GetQueue.mockImplementation(() => Promise.resolve(queue.map(j => ({ ...j }))))
  EnqueuePost4K.mockImplementation(() => {
    jobId++
    queue.push({ id: jobId, type: 'upscale_4k', status: 'pending' })
    return Promise.resolve(jobId)
  })
  EnqueueFromImage.mockImplementation(() => {
    jobId++
    queue.push({ id: jobId, type: 'from_image', status: 'pending' })
    return Promise.resolve(jobId)
  })
  ListPostRecipes.mockImplementation(() => Promise.resolve([]))
  SavePostRecipe.mockImplementation(r => Promise.resolve({ id: 1, ...r }))
  ReadFileAsBase64.mockImplementation(p => Promise.resolve(files[p] || ''))
  let saved = 0
  BatchSaveImage.mockImplementation(b64 => {
    saved++
    const p = '/batch/v' + saved + '.png'
    files[p] = strip(b64)
    return Promise.resolve(p)
  })
  BatchReset.mockImplementation(() => Promise.resolve())
})

async function mountPage({ section = 'effect', image = SRC } = {}) {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(image ? { id: 3 } : null))
  GetSessionImage.mockImplementation(id => Promise.resolve(id === 3 ? image : ''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  const wrapper = mount(PostProcessPage, { props: { initialSection: section }, attachTo: document.body })
  await settle()
  return wrapper
}

async function mountBatch({ section = 'effect' } = {}) {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  useRemixQueue().setPostQueue([{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }])
  const wrapper = mount(PostProcessPage, { props: { initialSection: section } })
  await settle()
  return wrapper
}

const fx4k = w => w.find('[data-testid="fx-4k"]')
const buttonWith = (w, text) => w.findAll('button').find(b => b.text().includes(text))
const previewSrc = w => w.find('.split-img.split-after').attributes('src') || ''
const undoButton = w => w.findAll('button').find(b => b.attributes('title') === t('post.undo'))

async function complete(id, image, upscale) {
  const result = { image_base64: image }
  if (upscale) result.info = JSON.stringify({ upscale })
  queue = queue.map(j => (j.id === id ? { ...j, status: 'completed' } : j))
  EventsEmit('queue:completed', { job_id: id, result: JSON.stringify(result) })
  await settle()
}

async function run4K(w, upscale = 'done', image = BIG) {
  const btn = fx4k(w)
  expect(btn.exists()).toBe(true)
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
  expect(EnqueuePost4K).toHaveBeenCalledTimes(1)
  const id = await EnqueuePost4K.mock.results[0].value
  await complete(id, image, upscale)
  return id
}

describe('Post-processing: 4K without redrawing as an effect', () => {
  it('the button is shown with its label in the effects card', async () => {
    const w = await mountPage()
    expect(fx4k(w).exists()).toBe(true)
    expect(fx4k(w).text()).toContain(t('post.fx4k_btn'))
  })

  it('the button is disabled without an image', async () => {
    const w = await mountPage({ image: '' })
    expect(fx4k(w).exists()).toBe(true)
    expect(fx4k(w).element.disabled).toBe(true)
    await fx4k(w).trigger('click')
    await settle()
    expect(EnqueuePost4K).not.toHaveBeenCalled()
  })

  it('click enqueues one 4K job with the current image and nothing else', async () => {
    const w = await mountPage()
    await fx4k(w).trigger('click')
    await settle()
    expect(EnqueuePost4K).toHaveBeenCalledTimes(1)
    expect(strip(EnqueuePost4K.mock.calls[0][0])).toBe(SRC)
    expect(EnqueueFromImage).not.toHaveBeenCalled()
  })

  it('the job result becomes the current layer, undo returns the source', async () => {
    const w = await mountPage()
    await run4K(w)
    await waitUntil(() => previewSrc(w).includes(BIG))
    const undo = undoButton(w)
    expect(undo).toBeTruthy()
    expect(undo.element.disabled).toBe(false)
    await undo.trigger('click')
    await settle()
    expect(previewSrc(w)).toContain(SRC)
  })

  it('the step goes into the applied effects log', async () => {
    const w = await mountPage()
    expect(w.text()).not.toContain(t('post.recipe_log', { n: 1 }))
    await run4K(w)
    expect(w.text()).toContain(t('post.recipe_log', { n: 1 }))
  })

  it('already 4K says so', async () => {
    const w = await mountPage()
    await run4K(w, 'skipped', SRC)
    expect(w.text()).toContain(t('post.fx4k_skipped'))
    expect(w.text()).not.toContain(t('post.recipe_log', { n: 1 }))
    expect(previewSrc(w)).toContain(SRC)
  })

  it('no ESRGAN reports a plain resize', async () => {
    const w = await mountPage()
    await run4K(w, 'resized')
    expect(w.text()).toContain(t('post.fx4k_resized'))
    await waitUntil(() => previewSrc(w).includes(BIG))
  })

  it('ESRGAN result shows neither the skipped nor the resized message', async () => {
    const w = await mountPage()
    await run4K(w, 'done')
    expect(w.text()).not.toContain(t('post.fx4k_skipped'))
    expect(w.text()).not.toContain(t('post.fx4k_resized'))
  })

  it('a second click while the job runs does not enqueue again', async () => {
    const w = await mountPage()
    await fx4k(w).trigger('click')
    await settle()
    const btn = fx4k(w)
    if (btn.exists()) {
      expect(btn.element.disabled).toBe(true)
      await btn.trigger('click')
      await settle()
    }
    expect(EnqueuePost4K).toHaveBeenCalledTimes(1)
  })

  it('the button is disabled while an effect is running', async () => {
    const w = await mountPage()
    await w.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus').trigger('click')
    await settle()
    await buttonWith(w, t('post.btn_apply_effect')).trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    const btn = fx4k(w)
    expect(!btn.exists() || btn.element.disabled).toBe(true)
  })

  it('the button is disabled while a recipe is running', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{
      id: 7, name: 'Soft Recipe', steps: JSON.stringify([{ preset_id: 10, strength: 0.5 }]), adjust: '{}',
    }]))
    const w = await mountPage({ section: 'recipes' })
    await w.findAll('.preset-item').find(i => i.text().includes('Soft Recipe')).trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    const btn = fx4k(w)
    expect(!btn.exists() || btn.element.disabled).toBe(true)
  })

  it('interrupt during the job cancels it in the queue', async () => {
    const w = await mountPage()
    await fx4k(w).trigger('click')
    await settle()
    const id = await EnqueuePost4K.mock.results[0].value
    const stop = buttonWith(w, t('post.applying'))
    expect(stop).toBeTruthy()
    await stop.trigger('click')
    await settle()
    expect(CancelQueueJob.mock.calls.map(c => c[0])).toContain(id)
  })

  it('saving a recipe after it stores the 4K step, not the save-time 4K option', async () => {
    const w = await mountPage()
    await run4K(w)
    const header = w.findAll('.card-header.card-toggle').find(h => h.text().includes(t('post.recipes_title')))
    await header.trigger('click')
    await settle()
    await buttonWith(w, t('post.recipe_save')).trigger('click')
    await settle()
    await w.find('input[placeholder="' + t('post.recipe_name') + '"]').setValue('Big')
    await w.find('.preset-form .btn-primary').trigger('click')
    await settle()
    expect(SavePostRecipe).toHaveBeenCalledTimes(1)
    const rec = SavePostRecipe.mock.calls[0][0]
    expect(JSON.parse(rec.steps)).toEqual([{ upscale_4k: true }])
    expect(JSON.parse(rec.adjust || '{}').upscale_4k).toBeFalsy()
  })

  it('applying a recipe with the 4K step enqueues a 4K job, not a from-image job', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{
      id: 8, name: 'Big Recipe', steps: JSON.stringify([{ upscale_4k: true }]), adjust: '{}',
    }]))
    const w = await mountPage({ section: 'recipes' })
    await w.findAll('.preset-item').find(i => i.text().includes('Big Recipe')).trigger('click')
    await settle()
    expect(EnqueuePost4K).toHaveBeenCalledTimes(1)
    expect(strip(EnqueuePost4K.mock.calls[0][0])).toBe(SRC)
    expect(EnqueueFromImage).not.toHaveBeenCalled()
    const id = await EnqueuePost4K.mock.results[0].value
    await complete(id, BIG, 'done')
    await waitUntil(() => previewSrc(w).includes(BIG))
    expect(EnqueueFromImage).not.toHaveBeenCalled()
  })

  it('a recipe chains an effect into the 4K step with the effect result', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{
      id: 9, name: 'Soft Big', steps: JSON.stringify([{ preset_id: 10, strength: 0.5 }, { upscale_4k: true }]), adjust: '{}',
    }]))
    const w = await mountPage({ section: 'recipes' })
    await w.findAll('.preset-item').find(i => i.text().includes('Soft Big')).trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(EnqueuePost4K).not.toHaveBeenCalled()
    const fxId = await EnqueueFromImage.mock.results[0].value
    await complete(fxId, FX)
    expect(EnqueuePost4K).toHaveBeenCalledTimes(1)
    expect(strip(EnqueuePost4K.mock.calls[0][0])).toBe(FX)
    const id = await EnqueuePost4K.mock.results[0].value
    await complete(id, BIG, 'done')
    await waitUntil(() => previewSrc(w).includes(BIG))
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
  })

  it('a failed 4K step aborts the recipe with the job error', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{
      id: 8, name: 'Big Recipe', steps: JSON.stringify([{ upscale_4k: true }]), adjust: '{}',
    }]))
    const w = await mountPage({ section: 'recipes' })
    await w.findAll('.preset-item').find(i => i.text().includes('Big Recipe')).trigger('click')
    await settle()
    const id = await EnqueuePost4K.mock.results[0].value
    EventsEmit('queue:failed', { job_id: id, error: 'not local' })
    await settle()
    expect(w.text()).toContain(t('post.recipe_aborted', { i: 1, reason: 'not local' }))
  })
})

describe('Post-processing: 4K effect in batch mode', () => {
  it('the button enqueues a 4K job per photo and records one batch step', async () => {
    const w = await mountBatch()
    const btn = fx4k(w)
    expect(btn.exists()).toBe(true)
    expect(btn.element.disabled).toBe(false)
    await btn.trigger('click')
    await settle()
    expect(EnqueuePost4K).toHaveBeenCalledTimes(2)
    expect(EnqueuePost4K.mock.calls.map(c => strip(c[0]))).toEqual([IMG_A, IMG_B])
    expect(EnqueueFromImage).not.toHaveBeenCalled()
    const ids = await Promise.all(EnqueuePost4K.mock.results.map(r => r.value))
    await complete(ids[0], BIG_A, 'done')
    await complete(ids[1], BIG_B, 'done')
    expect(BatchSaveImage.mock.calls.map(c => strip(c[0])).sort()).toEqual([BIG_A, BIG_B].sort())
    expect(w.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
  })

  it('a recipe with the 4K step runs it for every photo', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{
      id: 8, name: 'Big Recipe', steps: JSON.stringify([{ upscale_4k: true }]), adjust: '{}',
    }]))
    const w = await mountBatch({ section: 'recipes' })
    await w.findAll('.preset-item').find(i => i.text().includes('Big Recipe')).trigger('click')
    await settle()
    expect(EnqueuePost4K).toHaveBeenCalledTimes(2)
    expect(EnqueuePost4K.mock.calls.map(c => strip(c[0]))).toEqual([IMG_A, IMG_B])
    expect(EnqueueFromImage).not.toHaveBeenCalled()
    const ids = await Promise.all(EnqueuePost4K.mock.results.map(r => r.value))
    await complete(ids[0], BIG_A, 'done')
    await complete(ids[1], BIG_B, 'done')
    expect(w.find('.batch-steps').text()).toBe(t('post.batch_steps', { n: 1 }))
  })
})
