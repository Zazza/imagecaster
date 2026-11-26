import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

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
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const IMG_A = 'QUFBQQ=='
const IMG_B = 'QkJCQg=='
const PTS_A = [{ X: 0.1, Y: 0.2 }]
const PTS_B = [{ X: 0.7, Y: 0.8 }]
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
  PostFaceLandmarks.mockImplementation(b64 => Promise.resolve({ points: strip(b64) === IMG_B ? PTS_B : PTS_A }))
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

async function check(wrapper, key, value = true) {
  await field(wrapper, 'retouch-' + key).setValue(value)
  await settlePreview()
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

describe('PostProcessPage face retouch', () => {
  it('places the retouch card right after the correction card', async () => {
    const wrapper = await mountPage()
    const titles = wrapper.findAll('.post-controls .card-title').map(x => x.text())
    const i = titles.indexOf(t('post.adjust_title'))
    expect(titles[i + 1]).toBe(t('post.retouch_title'))
  })

  it('defaults change nothing: no face request, no retouch preview, no retouch in export', async () => {
    const wrapper = await mountPage()
    await settlePreview()
    await saveSingle(wrapper)

    expect(PostFaceLandmarks).not.toHaveBeenCalled()
    expect(PreviewPostRetouch).not.toHaveBeenCalled()
    const params = ExportImage.mock.calls[0][0]
    expect(params.retouch).toBeUndefined()
    expect(params.face_points).toBeUndefined()
  })

  it('a retouch option previews with its params and the face points', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')

    expect(PreviewPostRetouch).toHaveBeenCalled()
    const [img, , retouch, pts] = lastPreview()
    expect(strip(img)).toBe(IMG_A)
    expect(retouch).toEqual({ teeth: true })
    expect(pts).toEqual(PTS_A)
  })

  it('face points are requested once per image', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')
    await check(wrapper, 'eye_whites')
    await field(wrapper, 'retouch-face_width').setValue(5)
    await settlePreview()

    expect(PostFaceLandmarks).toHaveBeenCalledTimes(1)
    expect(lastPreview()[2]).toEqual({ teeth: true, eye_whites: true, face_width: 5 })

    useRemixQueue().setPostQueue([{ path: '/photos/b.png', name: 'b.png' }])
    await settle()
    await check(wrapper, 'teeth')

    expect(PostFaceLandmarks).toHaveBeenCalledTimes(2)
    expect(strip(PostFaceLandmarks.mock.calls[1][0])).toBe(IMG_B)
    expect(lastPreview()[3]).toEqual(PTS_B)
  })

  it('light alone needs no face points', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'light')

    expect(PostFaceLandmarks).not.toHaveBeenCalled()
    expect(lastPreview()[2]).toEqual({ light: true })
    expect(lastPreview()[3]).toEqual([])
  })

  it.each([
    ['not_local', 'post.retouch_face_not_local'],
    ['no_face', 'post.retouch_face_none'],
  ])('skip %s shows the reason and the rest keeps working', async (skip, key) => {
    PostFaceLandmarks.mockImplementation(() => Promise.resolve({ points: [], skip }))
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'under_eyes')

    expect(field(wrapper, 'retouch-face-note').text()).toContain(t(key))
    expect(lastPreview()[3]).toEqual([])
    expect(wrapper.find('[data-testid="retouch-retry"]').exists()).toBe(false)
  })

  it('unavailable server shows the error and retry asks again', async () => {
    PostFaceLandmarks.mockImplementationOnce(() => Promise.resolve({ points: [], skip: 'unavailable', error: 'status 500' }))
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')

    const note = field(wrapper, 'retouch-face-note')
    expect(note.text()).toBe(t('post.retouch_face_unavailable'))
    expect(note.attributes('title')).toBe('status 500')
    await field(wrapper, 'retouch-retry').trigger('click')
    await settlePreview()

    expect(PostFaceLandmarks).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="retouch-face-note"]').exists()).toBe(false)
  })

  it('save sends the retouch params and the face points to the export', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')
    await field(wrapper, 'retouch-eye_color').setValue('green')
    await settle()
    await saveSingle(wrapper)

    const params = ExportImage.mock.calls[0][0]
    expect(params.retouch).toEqual({ teeth: true, eye_color: 'green' })
    expect(params.face_points).toEqual(PTS_A)
    expect(EnqueuePostSave).not.toHaveBeenCalled()
  })

  it('normal export carries retouch and finish together', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')
    await openSection(wrapper, t('post.finish_title'))
    await field(wrapper, 'finish-ratio').setValue('4:5')
    await saveSingle(wrapper)

    expect(EnqueuePostSave).not.toHaveBeenCalled()
    const params = ExportImage.mock.calls[0][0]
    expect(strip(params.image_base64)).toBe(IMG_A)
    expect(params.width).toBe(0)
    expect(params.height).toBe(0)
    expect(params.retouch).toEqual({ teeth: true })
    expect(params.face_points).toEqual(PTS_A)
    expect(params.finish.ratio).toBe('4:5')
  })

  it('recipe saves the retouch', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'eye_whites')
    await openSection(wrapper, t('export.export_settings'))
    await openSection(wrapper, t('post.recipes_title'))
    await buttonWith(wrapper, t('post.recipe_save')).trigger('click')
    await settle()
    await wrapper.find('input[placeholder="' + t('post.recipe_name') + '"]').setValue('Face')
    await wrapper.find('.preset-form .btn-primary').trigger('click')
    await settle()

    expect(SavePostRecipe).toHaveBeenCalledTimes(1)
    const adjust = JSON.parse(SavePostRecipe.mock.calls[0][0].adjust)
    expect(adjust.retouch).toEqual({ eye_whites: true })
    expect(adjust.brightness).toBe(0)
  })

  it('legacy 4K recipe restores retouch without restoring the removed checkbox', async () => {
    const adjust = { brightness: 0, retouch: { eye_whites: true, face_width: -4 }, upscale_4k: true }
    ListPostRecipes.mockImplementation(() => Promise.resolve([{ id: 1, name: 'Face', steps: '[]', adjust: JSON.stringify(adjust) }]))
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.recipes_title'))
    await wrapper.findAll('.preset-item').find(r => r.text().includes('Face')).trigger('click')
    await settlePreview()

    expect(field(wrapper, 'retouch-eye_whites').element.checked).toBe(true)
    expect(Number(field(wrapper, 'retouch-face_width').element.value)).toBe(-4)
    expect(wrapper.find('[data-testid="post-upscale-4k"]').exists()).toBe(false)
    expect(lastPreview()[2]).toEqual({ eye_whites: true, face_width: -4 })
  })

  it('old recipes without retouch keys leave the retouch alone', async () => {
    ListPostRecipes.mockImplementation(() => Promise.resolve([{ id: 2, name: 'Old', steps: '[]', adjust: JSON.stringify({ brightness: 10 }) }]))
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')
    await openSection(wrapper, t('post.recipes_title'))
    await wrapper.findAll('.preset-item').find(r => r.text().includes('Old')).trigger('click')
    await settle()

    expect(field(wrapper, 'retouch-teeth').element.checked).toBe(true)
  })

  it('batch save gives every photo its own face points', async () => {
    const wrapper = await mountPage({ items: TWO, settings: { file_browser_path: '/out' } })
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')
    await openSection(wrapper, t('export.export_settings'))
    await buttonWith(wrapper, t('post.batch_save_all', { n: 2 })).trigger('click')
    await settle()

    expect(ExportImage).toHaveBeenCalledTimes(2)
    const byImage = Object.fromEntries(ExportImage.mock.calls.map(c => [strip(c[0].image_base64), c[0]]))
    expect(byImage[IMG_A].face_points).toEqual(PTS_A)
    expect(byImage[IMG_B].face_points).toEqual(PTS_B)
    expect(byImage[IMG_A].retouch).toEqual({ teeth: true })
    expect(byImage[IMG_B].retouch).toEqual({ teeth: true })
  })

  it('batch normal export preserves finish and each photo’s face points', async () => {
    const wrapper = await mountPage({ items: TWO, settings: { file_browser_path: '/out' } })
    await openSection(wrapper, t('post.retouch_title'))
    await check(wrapper, 'teeth')
    await openSection(wrapper, t('export.export_settings'))
    await openSection(wrapper, t('post.finish_title'))
    await field(wrapper, 'finish-ratio').setValue('4:5')
    await buttonWith(wrapper, t('post.batch_save_all', { n: 2 })).trigger('click')
    await settle()

    expect(EnqueuePostSave).not.toHaveBeenCalled()
    expect(ExportImage).toHaveBeenCalledTimes(2)
    const byImage = Object.fromEntries(ExportImage.mock.calls.map(c => [strip(c[0].image_base64), c[0]]))
    expect(byImage[IMG_A].face_points).toEqual(PTS_A)
    expect(byImage[IMG_B].face_points).toEqual(PTS_B)
    expect(byImage[IMG_B].output_dir).toBe('/out')
    expect(byImage[IMG_A].finish.ratio).toBe('4:5')
    expect(byImage[IMG_B].finish.ratio).toBe('4:5')
  })
})
