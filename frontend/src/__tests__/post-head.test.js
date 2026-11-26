import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).createAppMock({
  PreviewAdjust: vi.fn(() => Promise.resolve('UFJFVkFESg==')),
  PreviewPost: vi.fn(() => Promise.resolve('UFJFVlBPU1Q=')),
  PreviewPostRetouch: vi.fn(() => Promise.resolve('UFJFVlJU')),
  PostFaceLandmarks: vi.fn(() => Promise.resolve({ points: [] })),
  EnqueuePostSave: vi.fn(() => Promise.resolve(0)),
}))

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, ReadFileAsBase64, BatchSaveImage, BatchReset, ExportImage,
  PreviewAdjust, PreviewPost, PreviewPostRetouch, PostFaceLandmarks, EnqueuePostSave,
  SavePostRecipe, ListPostRecipes,
} from '../wailsjs/go/main/App.js'
import { HEAD_SIZE_MAX, HEAD_SHIFT_MAX, RETOUCH_DEFAULTS } from '../composables/usePostAdjust.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const IMG_A = 'QUFBQQ=='
const PTS_A = [{ X: 0.1, Y: 0.2 }]
const ONE = [{ path: '/photos/a.png', name: 'a.png' }]

useFakeImage()
useFakeCanvas()

function strip(b64) {
  return String(b64 || '').replace(/^data:[^,]*,/, '')
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  ListPostRecipes.mockImplementation(() => Promise.resolve([]))
  const files = { '/photos/a.png': IMG_A }
  ReadFileAsBase64.mockImplementation(p => Promise.resolve(files[p] || ''))
  let saved = 0
  BatchSaveImage.mockImplementation(b64 => {
    saved++
    const p = '/batch/v' + saved + '.png'
    files[p] = strip(b64)
    return Promise.resolve(p)
  })
  BatchReset.mockImplementation(() => Promise.resolve())
  ExportImage.mockImplementation(() => Promise.resolve('/out/x.png'))
  PreviewAdjust.mockImplementation(() => Promise.resolve('UFJFVkFESg=='))
  PreviewPost.mockImplementation(() => Promise.resolve('UFJFVlBPU1Q='))
  PreviewPostRetouch.mockImplementation(() => Promise.resolve('UFJFVlJU'))
  PostFaceLandmarks.mockImplementation(() => Promise.resolve({ points: PTS_A }))
  EnqueuePostSave.mockImplementation(() => Promise.resolve(41))
  SavePostRecipe.mockImplementation(r => Promise.resolve({ id: 1, ...r }))
})

async function settlePreview() {
  await settle()
  await new Promise(r => setTimeout(r, 250))
  await settle()
}

async function mountPage({ items = ONE, settings = {} } = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve([]))
  ListPresetTypes.mockReturnValue(Promise.resolve([]))
  useRemixQueue().setPostQueue(items.map(i => ({ ...i })))
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' } })
  await settle()
  return wrapper
}

function buttonWith(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().includes(text))
}

async function openSection(wrapper, title) {
  const header = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(title))
  expect(header).toBeTruthy()
  await header.trigger('click')
  await settle()
}

function field(wrapper, id) {
  const el = wrapper.find(`[data-testid="${id}"]`)
  expect(el.exists(), id).toBe(true)
  return el
}

async function slide(wrapper, key, value) {
  await field(wrapper, 'retouch-' + key).setValue(value)
  await settlePreview()
}

async function openRetouch(wrapper) {
  await openSection(wrapper, t('post.retouch_title'))
}

async function saveSingle(wrapper) {
  await openSection(wrapper, t('export.export_settings'))
  await buttonWith(wrapper, t('post.btn_save_file')).trigger('click')
  await settle()
}

function lastPreview() {
  return PreviewPostRetouch.mock.calls[PreviewPostRetouch.mock.calls.length - 1]
}

function recipeWith(adjust) {
  ListPostRecipes.mockImplementation(() => Promise.resolve([{ id: 1, name: 'Head', steps: '[]', adjust: JSON.stringify(adjust) }]))
}

async function applyRecipe(wrapper) {
  await openSection(wrapper, t('post.recipes_title'))
  await wrapper.findAll('.preset-item').find(r => r.text().includes('Head')).trigger('click')
  await settlePreview()
}

describe('head size and height in face retouch', () => {
  it('limits and defaults come from the contract', () => {
    expect(HEAD_SIZE_MAX).toBe(15)
    expect(HEAD_SHIFT_MAX).toBe(20)
    expect(RETOUCH_DEFAULTS.head_size).toBe(0)
    expect(RETOUCH_DEFAULTS.head_shift).toBe(0)
  })

  it('both sliders exist with their ranges and start at 0', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)

    const size = field(wrapper, 'retouch-head_size')
    expect(size.attributes('type')).toBe('range')
    expect(Number(size.attributes('min'))).toBe(-HEAD_SIZE_MAX)
    expect(Number(size.attributes('max'))).toBe(HEAD_SIZE_MAX)
    expect(Number(size.element.value)).toBe(0)

    const shift = field(wrapper, 'retouch-head_shift')
    expect(shift.attributes('type')).toBe('range')
    expect(Number(shift.attributes('min'))).toBe(-HEAD_SHIFT_MAX)
    expect(Number(shift.attributes('max'))).toBe(HEAD_SHIFT_MAX)
    expect(Number(shift.element.value)).toBe(0)

    expect(wrapper.find('[data-testid="retouch-reset"]').exists()).toBe(false)
  })

  it('labels show the slider values', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, 'head_size', 7)
    await slide(wrapper, 'head_shift', -12)

    expect(t('post.retouch_head_size', { value: 7 })).not.toBe('post.retouch_head_size')
    expect(t('post.retouch_head_shift', { value: -12 })).not.toBe('post.retouch_head_shift')
    expect(wrapper.text()).toContain(t('post.retouch_head_size', { value: 7 }))
    expect(wrapper.text()).toContain(t('post.retouch_head_shift', { value: -12 }))
  })

  it('moving the sliders previews with head_size and head_shift and the face points', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, 'head_size', 7)

    expect(PostFaceLandmarks).toHaveBeenCalled()
    expect(lastPreview()[2]).toEqual({ head_size: 7 })
    expect(lastPreview()[3]).toEqual(PTS_A)

    await slide(wrapper, 'head_shift', -12)
    expect(lastPreview()[2]).toEqual({ head_size: 7, head_shift: -12 })
  })

  it.each([
    ['head_size', 5],
    ['head_size', -5],
    ['head_shift', 10],
    ['head_shift', -10],
  ])('nonzero %s=%s alone activates the retouch', async (key, value) => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, key, value)

    expect(field(wrapper, 'retouch-reset').exists()).toBe(true)
    expect(PostFaceLandmarks).toHaveBeenCalledTimes(1)
    expect(lastPreview()[2]).toEqual({ [key]: value })
  })

  it('save sends head_size and head_shift to the export', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, 'head_size', -6)
    await slide(wrapper, 'head_shift', 8)
    await saveSingle(wrapper)

    const params = ExportImage.mock.calls[0][0]
    expect(params.retouch).toEqual({ head_size: -6, head_shift: 8 })
    expect(params.face_points).toEqual(PTS_A)
  })

  it('normal export carries head changes and finish', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, 'head_size', 4)
    await slide(wrapper, 'head_shift', -3)
    await openSection(wrapper, t('export.export_settings'))
    await openSection(wrapper, t('post.finish_title'))
    await field(wrapper, 'finish-ratio').setValue('4:5')
    await saveSingle(wrapper)

    expect(EnqueuePostSave).not.toHaveBeenCalled()
    expect(ExportImage).toHaveBeenCalledTimes(1)
    expect(ExportImage.mock.calls[0][0].finish.ratio).toBe('4:5')
    expect(ExportImage.mock.calls[0][0].retouch).toEqual({ head_size: 4, head_shift: -3 })
  })

  it('double click resets each slider to 0 on its own', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, 'head_size', 7)
    await slide(wrapper, 'head_shift', -12)

    await field(wrapper, 'retouch-head_size').trigger('dblclick')
    await settlePreview()
    expect(Number(field(wrapper, 'retouch-head_size').element.value)).toBe(0)
    expect(Number(field(wrapper, 'retouch-head_shift').element.value)).toBe(-12)
    expect(lastPreview()[2]).toEqual({ head_shift: -12 })

    await field(wrapper, 'retouch-head_shift').trigger('dblclick')
    await settlePreview()
    expect(Number(field(wrapper, 'retouch-head_shift').element.value)).toBe(0)
    expect(wrapper.find('[data-testid="retouch-reset"]').exists()).toBe(false)
  })

  it('reset button zeroes both and the export gets no retouch', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, 'head_size', 7)
    await slide(wrapper, 'head_shift', -12)
    await field(wrapper, 'retouch-reset').trigger('click')
    await settlePreview()

    expect(Number(field(wrapper, 'retouch-head_size').element.value)).toBe(0)
    expect(Number(field(wrapper, 'retouch-head_shift').element.value)).toBe(0)
    await saveSingle(wrapper)
    expect(ExportImage.mock.calls[0][0].retouch).toBeUndefined()
  })

  it('recipe saves head_size and head_shift in retouch', async () => {
    const wrapper = await mountPage()
    await openRetouch(wrapper)
    await slide(wrapper, 'head_size', -9)
    await slide(wrapper, 'head_shift', 14)
    await openSection(wrapper, t('post.recipes_title'))
    await buttonWith(wrapper, t('post.recipe_save')).trigger('click')
    await settle()
    await wrapper.find('input[placeholder="' + t('post.recipe_name') + '"]').setValue('Head')
    await wrapper.find('.preset-form .btn-primary').trigger('click')
    await settle()

    const adjust = JSON.parse(SavePostRecipe.mock.calls[0][0].adjust)
    expect(adjust.retouch).toEqual({ head_size: -9, head_shift: 14 })
  })

  it('applying a recipe restores in-range values', async () => {
    recipeWith({ brightness: 0, retouch: { head_size: 5, head_shift: -10 } })
    const wrapper = await mountPage()
    await applyRecipe(wrapper)
    await openRetouch(wrapper)

    expect(Number(field(wrapper, 'retouch-head_size').element.value)).toBe(5)
    expect(Number(field(wrapper, 'retouch-head_shift').element.value)).toBe(-10)
    expect(lastPreview()[2]).toEqual({ head_size: 5, head_shift: -10 })
  })

  it.each([
    [{ head_size: 40, head_shift: -50 }, { head_size: 15, head_shift: -20 }],
    [{ head_size: -16, head_shift: 21 }, { head_size: -15, head_shift: 20 }],
  ])('applying a recipe clamps %o to the ranges', async (stored, want) => {
    recipeWith({ brightness: 0, retouch: stored })
    const wrapper = await mountPage()
    await applyRecipe(wrapper)
    await openRetouch(wrapper)

    expect(Number(field(wrapper, 'retouch-head_size').element.value)).toBe(want.head_size)
    expect(Number(field(wrapper, 'retouch-head_shift').element.value)).toBe(want.head_shift)
    expect(lastPreview()[2]).toEqual(want)

    await saveSingle(wrapper)
    expect(ExportImage.mock.calls[0][0].retouch).toEqual(want)
  })
})
