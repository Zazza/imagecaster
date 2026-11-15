import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes, ListHiresProfiles,
  EnqueueFromImage, ReadImageFile, ListPostRecipes, GetQueue,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle, waitUntil } from './helpers/async.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]
const SRC = 'U1JDQQ=='
const RESULT = 'UkVTVUxU'

useFakeImage()
useFakeCanvas()

let queue

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  queue = []
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  GetQueue.mockImplementation(() => Promise.resolve(queue.map(j => ({ ...j }))))
  EnqueueFromImage.mockImplementation(() => {
    queue.push({ id: 55, type: 'from_image', status: 'pending' })
    return Promise.resolve(55)
  })
  ListPostRecipes.mockImplementation(() => Promise.resolve([{
    id: 7, name: 'Soft Recipe', steps: JSON.stringify([{ preset_id: 10, strength: 0.5 }]), adjust: '{}',
  }]))
})

async function mountPage(section = 'effect') {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve({ id: 3 }))
  GetSessionImage.mockImplementation(id => Promise.resolve(id === 3 ? SRC : RESULT))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  const wrapper = mount(PostProcessPage, { props: { initialSection: section }, attachTo: document.body })
  await settle()
  return wrapper
}

const buttonWith = (w, text) => w.findAll('button').find(b => b.text().includes(text))

async function applyRecipe(w) {
  const item = w.findAll('.preset-item').find(i => i.text().includes('Soft Recipe'))
  expect(item).toBeTruthy()
  await item.trigger('click')
  await settle()
  expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
}

describe('PostProcessPage queue jobs', () => {
  it('upload dialog error is shown to the user', async () => {
    ReadImageFile.mockImplementation(() => Promise.reject(new Error('cannot open file')))
    const w = await mountPage()
    await buttonWith(w, t('export.btn_upload')).trigger('click')
    await settle()
    expect(w.text()).toContain('cannot open file')
  })

  it('recipe step failure shows the step and the job error', async () => {
    const w = await mountPage('recipes')
    await applyRecipe(w)
    EventsEmit('queue:failed', { job_id: 55, error: 'out of memory' })
    await settle()
    expect(w.text()).toContain(t('post.recipe_aborted', { i: 1, reason: 'out of memory' }))
  })

  it('recipe step cancelled from the queue panel says it was cancelled', async () => {
    const w = await mountPage('recipes')
    await applyRecipe(w)
    queue = queue.map(j => ({ ...j, status: 'cancelled' }))
    EventsEmit('queue:changed')
    await settle()
    expect(w.text()).toContain(t('post.recipe_aborted', { i: 1, reason: t('post.recipe_reason_cancelled') }))
  })

  it('effect result is loaded by session item id', async () => {
    const w = await mountPage('effect')
    await w.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus').trigger('click')
    await settle()
    await buttonWith(w, t('post.btn_apply_effect')).trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    EventsEmit('queue:completed', { job_id: 55, result: { session_item_id: 99 } })
    await settle()
    expect(GetSessionImage).toHaveBeenCalledWith(99)
    await waitUntil(() => (w.find('.split-img.split-after').attributes('src') || '').includes(RESULT))
  })
})
