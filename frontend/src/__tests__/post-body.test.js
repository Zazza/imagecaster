import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { defineComponent, ref, nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).createAppMock({
  PreviewAdjust: vi.fn(() => Promise.resolve('UFJFVkFESg==')),
  PreviewPost: vi.fn(() => Promise.resolve('UFJFVlBPU1Q=')),
  PreviewPostRetouch: vi.fn(() => Promise.resolve('UFJFVlJU')),
  PostFaceLandmarks: vi.fn(() => Promise.resolve({ points: [] })),
  PostBodyPoints: vi.fn(() => Promise.resolve({ points: [] })),
  EnqueuePostSave: vi.fn(() => Promise.resolve(0)),
}))

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, ReadFileAsBase64, BatchSaveImage, BatchReset, ExportImage,
  PreviewAdjust, PreviewPost, PreviewPostRetouch, PostFaceLandmarks, PostBodyPoints, EnqueuePostSave,
  SavePostRecipe, ListPostRecipes,
} from '../wailsjs/go/main/App.js'
import { usePostAdjust, splitRecipeAdjust, BODY_WIDTH_MAX, BODY_HEIGHT_MAX } from '../composables/usePostAdjust.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const IMG_A = 'QUFBQQ=='
const IMG_B = 'QkJCQg=='
const BODY_A = [{ X: 0.5, Y: 0.2 }, { X: 0.5, Y: 0.25 }]
const BODY_B = [{ X: 0.4, Y: 0.3 }, { X: 0.4, Y: 0.35 }]
const ONE = [{ path: '/photos/a.png', name: 'a.png' }]
const TWO = [{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }]

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
  const files = { '/photos/a.png': IMG_A, '/photos/b.png': IMG_B }
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
  PostFaceLandmarks.mockImplementation(() => Promise.resolve({ points: [] }))
  PostBodyPoints.mockImplementation(b64 => Promise.resolve({ points: strip(b64) === IMG_B ? BODY_B : BODY_A }))
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
  await field(wrapper, 'body-' + key).setValue(value)
  await settlePreview()
}

async function openBody(wrapper) {
  await openSection(wrapper, t('post.body_title'))
}

async function saveSingle(wrapper) {
  await openSection(wrapper, t('export.export_settings'))
  const btn = buttonWith(wrapper, t('post.btn_save_file'))
  await btn.trigger('click')
  await settle()
  return btn
}

function lastPreview() {
  return PreviewPostRetouch.mock.calls[PreviewPostRetouch.mock.calls.length - 1]
}

function bodyOf(b) {
  return { width: Number(b?.width || 0), height: Number(b?.height || 0) }
}

describe('PostProcessPage body shape', () => {
  it('places the body card right after the face retouch card', async () => {
    const wrapper = await mountPage()
    const titles = wrapper.findAll('.post-controls .card-title').map(x => x.text())
    const i = titles.indexOf(t('post.retouch_title'))
    expect(i).toBeGreaterThanOrEqual(0)
    expect(titles[i + 1]).toBe(t('post.body_title'))
  })

  it('sliders have the card ranges', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    const w = field(wrapper, 'body-width')
    const h = field(wrapper, 'body-height')
    expect(Number(w.attributes('min'))).toBe(-10)
    expect(Number(w.attributes('max'))).toBe(10)
    expect(Number(h.attributes('min'))).toBe(-7)
    expect(Number(h.attributes('max'))).toBe(7)
    expect(Number(w.element.value)).toBe(0)
    expect(Number(h.element.value)).toBe(0)
  })

  it('defaults change nothing: no body request, no retouch preview, no body in export', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await settlePreview()
    await saveSingle(wrapper)

    expect(PostBodyPoints).not.toHaveBeenCalled()
    expect(PreviewPostRetouch).not.toHaveBeenCalled()
    const params = ExportImage.mock.calls[0][0]
    expect(params.body).toBeUndefined()
    expect(params.body_points).toBeUndefined()
  })

  it('a slider alone previews through the retouch preview with body and its points', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)

    expect(PostBodyPoints).toHaveBeenCalledTimes(1)
    expect(strip(PostBodyPoints.mock.calls[0][0])).toBe(IMG_A)
    expect(PostFaceLandmarks).not.toHaveBeenCalled()
    expect(PreviewPostRetouch).toHaveBeenCalled()
    const call = lastPreview()
    expect(call).toHaveLength(8)
    expect(strip(call[0])).toBe(IMG_A)
    expect(bodyOf(call[4])).toEqual({ width: -5, height: 0 })
    expect(call[5]).toEqual(BODY_A)
  })

  it('body points are requested once per image', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)
    await slide(wrapper, 'height', 3)
    await slide(wrapper, 'width', 4)

    expect(PostBodyPoints).toHaveBeenCalledTimes(1)
    expect(bodyOf(lastPreview()[4])).toEqual({ width: 4, height: 3 })

    useRemixQueue().setPostQueue([{ path: '/photos/b.png', name: 'b.png' }])
    await settle()
    await slide(wrapper, 'height', 2)

    expect(PostBodyPoints).toHaveBeenCalledTimes(2)
    expect(strip(PostBodyPoints.mock.calls[1][0])).toBe(IMG_B)
    expect(lastPreview()[5]).toEqual(BODY_B)
  })

  it('a new image resets the body', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)
    await slide(wrapper, 'height', 3)

    useRemixQueue().setPostQueue([{ path: '/photos/b.png', name: 'b.png' }])
    await settlePreview()

    expect(Number(field(wrapper, 'body-width').element.value)).toBe(0)
    expect(Number(field(wrapper, 'body-height').element.value)).toBe(0)
  })

  it('double click on a slider sets it to 0', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)
    await slide(wrapper, 'height', 3)
    await field(wrapper, 'body-width').trigger('dblclick')
    await settlePreview()

    expect(Number(field(wrapper, 'body-width').element.value)).toBe(0)
    expect(Number(field(wrapper, 'body-height').element.value)).toBe(3)
    expect(bodyOf(lastPreview()[4])).toEqual({ width: 0, height: 3 })

    await field(wrapper, 'body-height').trigger('dblclick')
    await settlePreview()
    expect(Number(field(wrapper, 'body-height').element.value)).toBe(0)
  })

  it('reset button zeroes both sliders and the export gets no body', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)
    await slide(wrapper, 'height', 3)
    await field(wrapper, 'body-reset').trigger('click')
    await settlePreview()

    expect(Number(field(wrapper, 'body-width').element.value)).toBe(0)
    expect(Number(field(wrapper, 'body-height').element.value)).toBe(0)
    await saveSingle(wrapper)
    const params = ExportImage.mock.calls[0][0]
    expect(params.body).toBeUndefined()
    expect(params.body_points).toBeUndefined()
  })

  it('shows "looking for a person" while points are loading', async () => {
    let resolve
    PostBodyPoints.mockImplementation(() => new Promise(r => { resolve = r }))
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)

    expect(field(wrapper, 'body-note').text()).toContain(t('post.body_loading'))
    resolve({ points: BODY_A })
    await settlePreview()
    expect(wrapper.find('[data-testid="body-note"]').exists()).toBe(false)
  })

  it.each([
    ['not_local', 'post.body_not_local'],
    ['no_body', 'post.body_none'],
  ])('skip %s shows the reason without retry', async (skip, key) => {
    PostBodyPoints.mockImplementation(() => Promise.resolve({ points: [], skip }))
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)

    expect(field(wrapper, 'body-note').text()).toContain(t(key))
    expect(wrapper.find('[data-testid="body-retry"]').exists()).toBe(false)
    expect(lastPreview()[5]).toEqual([])
  })

  it('unavailable server shows the error and retry asks again', async () => {
    PostBodyPoints.mockImplementationOnce(() => Promise.resolve({ points: [], skip: 'unavailable', error: 'status 500' }))
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'height', 4)

    const note = field(wrapper, 'body-note')
    expect(note.text()).toContain(t('post.body_unavailable'))
    expect(note.attributes('title')).toBe('status 500')
    await field(wrapper, 'body-retry').trigger('click')
    await settlePreview()

    expect(PostBodyPoints).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="body-note"]').exists()).toBe(false)
    expect(lastPreview()[5]).toEqual(BODY_A)
  })

  it('body and face retouch go together into one preview', async () => {
    PostFaceLandmarks.mockImplementation(() => Promise.resolve({ points: [{ X: 0.1, Y: 0.1 }] }))
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await field(wrapper, 'retouch-teeth').setValue(true)
    await settlePreview()
    await openBody(wrapper)
    await slide(wrapper, 'width', 6)

    const call = lastPreview()
    expect(call[2]).toEqual({ teeth: true })
    expect(call[3]).toEqual([{ X: 0.1, Y: 0.1 }])
    expect(bodyOf(call[4])).toEqual({ width: 6, height: 0 })
    expect(call[5]).toEqual(BODY_A)
  })

  it('save sends body and body points to the export', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)
    await slide(wrapper, 'height', 3)
    await saveSingle(wrapper)

    const params = ExportImage.mock.calls[0][0]
    expect(bodyOf(params.body)).toEqual({ width: -5, height: 3 })
    expect(params.body_points).toEqual(BODY_A)
  })

  it('normal export carries body and finish', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'height', 3)
    await openSection(wrapper, t('export.export_settings'))
    await openSection(wrapper, t('post.finish_title'))
    await field(wrapper, 'finish-ratio').setValue('4:5')
    await saveSingle(wrapper)

    expect(EnqueuePostSave).not.toHaveBeenCalled()
    expect(ExportImage).toHaveBeenCalledTimes(1)
    expect(ExportImage.mock.calls[0][0].finish.ratio).toBe('4:5')
    const params = ExportImage.mock.calls[0][0]
    expect(bodyOf(params.body)).toEqual({ width: 0, height: 3 })
    expect(params.body_points).toEqual(BODY_A)
  })

  it('batch save gives every photo its own body points', async () => {
    const wrapper = await mountPage({ items: TWO, settings: { file_browser_path: '/out' } })
    await openBody(wrapper)
    await slide(wrapper, 'width', -4)
    await openSection(wrapper, t('export.export_settings'))
    await buttonWith(wrapper, t('post.batch_save_all', { n: 2 })).trigger('click')
    await settle()

    expect(ExportImage).toHaveBeenCalledTimes(2)
    const byImage = Object.fromEntries(ExportImage.mock.calls.map(c => [strip(c[0].image_base64), c[0]]))
    expect(byImage[IMG_A].body_points).toEqual(BODY_A)
    expect(byImage[IMG_B].body_points).toEqual(BODY_B)
    expect(bodyOf(byImage[IMG_A].body)).toEqual({ width: -4, height: 0 })
    expect(bodyOf(byImage[IMG_B].body)).toEqual({ width: -4, height: 0 })
  })

  it('recipe saves the body under the body key', async () => {
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)
    await slide(wrapper, 'height', 3)
    await openSection(wrapper, t('post.recipes_title'))
    await buttonWith(wrapper, t('post.recipe_save')).trigger('click')
    await settle()
    await wrapper.find('input[placeholder="' + t('post.recipe_name') + '"]').setValue('Body')
    await wrapper.find('.preset-form .btn-primary').trigger('click')
    await settle()

    expect(SavePostRecipe).toHaveBeenCalledTimes(1)
    const adjust = JSON.parse(SavePostRecipe.mock.calls[0][0].adjust)
    expect(adjust.body).toEqual({ width: -5, height: 3 })
    expect(adjust.brightness).toBe(0)
  })

  it('applying a recipe restores the body clamped to the ranges', async () => {
    const adjust = { brightness: 0, body: { width: -15, height: 3 } }
    ListPostRecipes.mockImplementation(() => Promise.resolve([{ id: 1, name: 'Slim', steps: '[]', adjust: JSON.stringify(adjust) }]))
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.recipes_title'))
    await wrapper.findAll('.preset-item').find(r => r.text().includes('Slim')).trigger('click')
    await settlePreview()
    await openBody(wrapper)

    expect(Number(field(wrapper, 'body-width').element.value)).toBe(-10)
    expect(Number(field(wrapper, 'body-height').element.value)).toBe(3)
    expect(bodyOf(lastPreview()[4])).toEqual({ width: -10, height: 3 })
  })

  it('old recipes without the body key leave the body alone', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{ id: 2, name: 'Old', steps: '[]', adjust: JSON.stringify({ brightness: 10 }) }]))
    const wrapper = await mountPage()
    await openBody(wrapper)
    await slide(wrapper, 'width', -5)
    await openSection(wrapper, t('post.recipes_title'))
    await wrapper.findAll('.preset-item').find(r => r.text().includes('Old')).trigger('click')
    await settle()

    expect(Number(field(wrapper, 'body-width').element.value)).toBe(-5)
  })
})

function withPost(b64 = IMG_A) {
  const currentB64 = ref(b64)
  const error = ref('')
  let post
  const Host = defineComponent({
    setup() {
      post = usePostAdjust({ currentB64, error })
      return () => null
    },
  })
  mount(Host)
  return { post, currentB64, error }
}

describe('usePostAdjust body', () => {
  it('exports the ranges', () => {
    expect(BODY_WIDTH_MAX).toBe(10)
    expect(BODY_HEIGHT_MAX).toBe(7)
  })

  it('starts neutral', () => {
    const { post } = withPost()
    expect(bodyOf(post.body)).toEqual({ width: 0, height: 0 })
    expect(post.bodyActive.value).toBe(false)
    expect(post.bodyState.status).toBe('idle')
    expect(post.recipeExtras().body).toEqual({})
  })

  it('retouchFor adds body and its points only when the body is active', async () => {
    const { post } = withPost()
    const off = await post.retouchFor(IMG_A)
    expect(off.body).toBeUndefined()
    expect(off.body_points).toBeUndefined()
    expect(PostBodyPoints).not.toHaveBeenCalled()

    post.body.width = -3
    await nextTick()
    expect(post.bodyActive.value).toBe(true)
    const on = await post.retouchFor(IMG_B)
    expect(bodyOf(on.body)).toEqual({ width: -3, height: 0 })
    expect(on.body_points).toEqual(BODY_B)
    expect(on.retouch).toBeUndefined()
    await post.retouchFor(IMG_B)
    expect(PostBodyPoints).toHaveBeenCalledTimes(1)
  })

  it('recipe round trip keeps the body and clamps it', async () => {
    const { post } = withPost()
    post.body.width = 4
    post.body.height = -2
    await nextTick()
    const extras = post.recipeExtras()
    expect(extras.body).toEqual({ width: 4, height: -2 })

    const split = splitRecipeAdjust({ brightness: 5, ...extras })
    expect(split.adjust.body).toBeUndefined()
    expect(split.adjust.brightness).toBe(5)
    expect(split.extras.body).toEqual({ width: 4, height: -2 })

    const other = withPost().post
    other.applyRecipeExtras(split.extras)
    expect(bodyOf(other.body)).toEqual({ width: 4, height: -2 })

    other.applyRecipeExtras(splitRecipeAdjust({ body: { width: 25, height: -30 } }).extras)
    expect(bodyOf(other.body)).toEqual({ width: 10, height: -7 })
  })

  it('a recipe without body does not touch the body; an empty body resets it', async () => {
    const { post } = withPost()
    post.body.width = 4
    post.applyRecipeExtras(splitRecipeAdjust({ brightness: 3 }).extras)
    expect(post.body.width).toBe(4)
    post.applyRecipeExtras(splitRecipeAdjust({ body: {} }).extras)
    expect(bodyOf(post.body)).toEqual({ width: 0, height: 0 })
  })

  it('resetBody zeroes both values', async () => {
    const { post } = withPost()
    post.body.width = -6
    post.body.height = 5
    post.resetBody()
    await nextTick()
    expect(bodyOf(post.body)).toEqual({ width: 0, height: 0 })
    expect(post.bodyActive.value).toBe(false)
  })
})
