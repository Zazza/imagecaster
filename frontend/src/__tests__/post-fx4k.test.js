import { mockInstalledEditor } from './helpers/post-editor.js'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import ResolutionUpscaleOverlay from '../components/ResolutionUpscaleOverlay.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes, ListHiresProfiles,
  EnqueueFromImage, EnqueuePost4K, EnqueueImageResolutionUpscale, GetQueueJobResult, ListPostRecipes, SavePostRecipe, GetQueue,
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
  mockInstalledEditor()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  queue = []
  jobId = 40
  files = { '/photos/a.png': IMG_A, '/photos/b.png': IMG_B }
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  GetQueue.mockImplementation(() => Promise.resolve(queue.map(j => ({ ...j }))))
  GetQueueJobResult.mockImplementation(id => Promise.resolve(queue.find(j => j.id === id)))
  EnqueueImageResolutionUpscale.mockImplementation(() => {
    jobId++
    queue.push({ id: jobId, type: 'resolution_upscale', status: 'pending' })
    return Promise.resolve(jobId)
  })
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

async function mountPage({ section = 'finish', image = SRC } = {}) {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(image ? { id: 3 } : null))
  GetSessionImage.mockImplementation(id => Promise.resolve(id === 3 ? image : ''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  const wrapper = mount(PostProcessPage, { props: { initialSection: section }, attachTo: document.body })
  await settle()
  return wrapper
}

async function mountBatch({ section = 'finish' } = {}) {
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
  const result = { image_base64: image, session_item_id: id + 100 }
  if (upscale) result.info = JSON.stringify({ upscale })
  queue = queue.map(j => (j.id === id ? { ...j, status: 'completed', result } : j))
  EventsEmit('queue:completed', { job_id: id, result: JSON.stringify(result) })
  await settle()
}

const resolution = w => w.findComponent(ResolutionUpscaleOverlay)
const resolutionButton = (w, label) => resolution(w).findAll('button').find(b => b.text().startsWith(label))

async function runResolution(w, label = '4K', image = BIG) {
  await resolutionButton(w, label).trigger('click')
  await settle()
  const id = await EnqueueImageResolutionUpscale.mock.results.at(-1).value
  await complete(id, image)
  return id
}

describe('Post-processing: resolution belongs to Format', () => {
  it('places 1080p, 2K and 4K inside card09 and removes the separate Effects4K button', async () => {
    const w = await mountPage()
    const controls = resolution(w)
    expect(controls.exists()).toBe(true)
    const card = controls.element.closest('.card')
    expect(card.querySelector('.card-num').textContent).toBe('09')
    for (const label of ['1080p', '2K', '4K']) expect(resolutionButton(w, label)).toBeTruthy()
    expect(fx4k(w).exists()).toBe(false)
    expect(controls.props('inline')).toBe(true)
  })

  it('never starts upscale without an image', async () => {
    const w = await mountPage({ image: '' })
    const controls = resolution(w)
    if (controls.exists()) {
      for (const button of controls.findAll('button')) expect(button.element.disabled).toBe(true)
    }
    expect(EnqueueImageResolutionUpscale).not.toHaveBeenCalled()
  })

  it.each([['1080p', 1920], ['2K', 2560], ['4K', 3840]])('explicit %s uses the current image and the requested long edge', async (label, target) => {
    const w = await mountPage()
    expect(EnqueueImageResolutionUpscale).not.toHaveBeenCalled()
    await resolutionButton(w, label).trigger('click')
    await settle()
    expect(EnqueueImageResolutionUpscale).toHaveBeenCalledTimes(1)
    expect(strip(EnqueueImageResolutionUpscale.mock.calls[0][0])).toBe(SRC)
    expect(EnqueueImageResolutionUpscale.mock.calls[0][1]).toBe(target)
    expect(EnqueueFromImage).not.toHaveBeenCalled()
    expect(EnqueuePost4K).not.toHaveBeenCalled()
  })

  it('shows the completed size operation and undo returns the prior current image', async () => {
    const w = await mountPage()
    await runResolution(w)
    await waitUntil(() => previewSrc(w).includes(BIG))
    await undoButton(w).trigger('click')
    await settle()
    expect(previewSrc(w)).toContain(SRC)
    expect(w.text()).not.toContain(t('post.recipe_log', { n: 1 }))
  })

  it('does not enqueue twice while a size operation is pending', async () => {
    const w = await mountPage()
    await resolutionButton(w, '2K').trigger('click')
    await settle()
    expect(resolutionButton(w, '4K').element.disabled).toBe(true)
    await resolutionButton(w, '4K').trigger('click')
    await settle()
    expect(EnqueueImageResolutionUpscale).toHaveBeenCalledTimes(1)
  })

  it('an error or cancellation leaves the source image available', async () => {
    const w = await mountPage()
    await resolutionButton(w, '4K').trigger('click')
    await settle()
    const id = await EnqueueImageResolutionUpscale.mock.results[0].value
    queue = queue.map(j => j.id === id ? { ...j, status: 'cancelled' } : j)
    EventsEmit('queue:changed')
    await settle()
    expect(previewSrc(w)).toContain(SRC)
    expect(resolutionButton(w, '2K').element.disabled).toBe(false)
  })

  it('blocks size changes while an effect is running', async () => {
    const w = await mountPage({ section: 'effect' })
    await w.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus').trigger('click')
    await settle()
    await buttonWith(w, t('post.btn_apply_effect')).trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(resolutionButton(w, '4K').element.disabled).toBe(true)
    expect(EnqueueImageResolutionUpscale).not.toHaveBeenCalled()
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

describe('Post-processing: resolution in batch mode', () => {
  it('upscales each current photo and records a single undoable batch step', async () => {
    const w = await mountBatch()
    await resolutionButton(w, '2K').trigger('click')
    await settle()
    expect(EnqueueImageResolutionUpscale).toHaveBeenCalledTimes(2)
    expect(EnqueueImageResolutionUpscale.mock.calls.map(c => strip(c[0]))).toEqual([IMG_A, IMG_B])
    expect(EnqueueImageResolutionUpscale.mock.calls.map(c => c[1])).toEqual([2560, 2560])
    const ids = await Promise.all(EnqueueImageResolutionUpscale.mock.results.map(r => r.value))
    await complete(ids[0], BIG_A)
    await complete(ids[1], BIG_B)
    await waitUntil(() => w.find('.batch-steps').text() === t('post.batch_steps', { n: 1 }))
    expect(BatchSaveImage.mock.calls.map(c => strip(c[0])).sort()).toEqual([BIG_A, BIG_B].sort())
    await undoButton(w).trigger('click')
    await settle()
    expect(previewSrc(w)).toContain(IMG_A)
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
