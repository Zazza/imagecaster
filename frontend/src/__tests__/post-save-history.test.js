import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).createAppMock({
  PreviewAdjust: vi.fn(() => Promise.resolve('UFJFVkFESg==')),
  PreviewPost: vi.fn(() => Promise.resolve('UFJFVlBPU1Q=')),
  PreviewPostRetouch: vi.fn(() => Promise.resolve('UFJFVlJU')),
  PostFaceLandmarks: vi.fn(() => Promise.resolve({ points: [] })),
  PostBodyPoints: vi.fn(() => Promise.resolve({ points: [] })),
  EnqueuePostSave: vi.fn(() => Promise.resolve(0)),
  SavePostToHistory: vi.fn(() => Promise.resolve(1)),
}))

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, ReadFileAsBase64, BatchSaveImage, BatchReset, ExportImage,
  PreviewAdjust, PreviewPost, PreviewPostRetouch, PostFaceLandmarks, PostBodyPoints,
  EnqueuePostSave, ListPostRecipes, SavePostToHistory,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const IMG_A = 'QUFBQQ=='
const IMG_B = 'QkJCQg=='
const PTS_A = [{ X: 0.1, Y: 0.2 }]
const BODY_A = [{ X: 0.5, Y: 0.2 }, { X: 0.5, Y: 0.25 }]
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
  PostFaceLandmarks.mockImplementation(() => Promise.resolve({ points: PTS_A }))
  PostBodyPoints.mockImplementation(() => Promise.resolve({ points: BODY_A }))
  EnqueuePostSave.mockImplementation(() => Promise.resolve(41))
  let id = 100
  SavePostToHistory.mockImplementation(() => Promise.resolve(++id))
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

function saveBtn(wrapper) {
  return field(wrapper, 'post-save-history')
}

async function clickSave(wrapper) {
  await saveBtn(wrapper).trigger('click')
  await settle()
}

async function setBrightness(wrapper, value) {
  await openSection(wrapper, t('post.adjust_title'))
  const row = wrapper.findAll('.adj-row').find(r => r.find('.adj-label').text() === t('post.brightness'))
  expect(row).toBeTruthy()
  await row.find('input[type="range"]').setValue(value)
  await settlePreview()
  return row
}

function lastCall() {
  return SavePostToHistory.mock.calls[SavePostToHistory.mock.calls.length - 1][0]
}

function emptyFinish(f) {
  return f === undefined || f === null || (typeof f === 'object' && Object.keys(f).length === 0)
}

describe('PostProcessPage save to history', () => {
  it('button with its label is placed after the last card', async () => {
    const wrapper = await mountPage()
    const btn = saveBtn(wrapper)
    expect(btn.text()).toContain(t('post.save_history'))
    const cards = wrapper.findAll('.post-controls .card')
    expect(cards.length).toBeGreaterThan(0)
    const last = cards[cards.length - 1].element
    expect(last.contains(btn.element)).toBe(false)
    expect(last.compareDocumentPosition(btn.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('is enabled with a single image', async () => {
    const wrapper = await mountPage()
    expect(saveBtn(wrapper).element.disabled).toBe(false)
  })

  it('is disabled without an image', async () => {
    const wrapper = await mountPage({ items: [] })
    const btn = wrapper.find('[data-testid="post-save-history"]')
    if (btn.exists()) {
      expect(btn.element.disabled).toBe(true)
      await btn.trigger('click')
      await settle()
    }
    expect(SavePostToHistory).not.toHaveBeenCalled()
  })

  it('is disabled in batch mode', async () => {
    const wrapper = await mountPage({ items: TWO })
    const btn = wrapper.find('[data-testid="post-save-history"]')
    if (btn.exists()) {
      expect(btn.element.disabled).toBe(true)
      await btn.trigger('click')
      await settle()
    }
    expect(SavePostToHistory).not.toHaveBeenCalled()
  })

  it('is disabled while saving and enabled again after', async () => {
    let resolve
    SavePostToHistory.mockImplementation(() => new Promise(r => { resolve = r }))
    const wrapper = await mountPage()
    await saveBtn(wrapper).trigger('click')
    await settle()
    expect(saveBtn(wrapper).element.disabled).toBe(true)
    await saveBtn(wrapper).trigger('click')
    await settle()
    expect(SavePostToHistory).toHaveBeenCalledTimes(1)
    resolve(7)
    await settle()
    expect(saveBtn(wrapper).element.disabled).toBe(false)
  })

  it('unchanged image saves the current picture as full-size png with adjust', async () => {
    const wrapper = await mountPage()
    await settlePreview()
    await clickSave(wrapper)

    expect(SavePostToHistory).toHaveBeenCalledTimes(1)
    const p = lastCall()
    expect(strip(p.image_base64)).toBe(IMG_A)
    expect(p.format).toBe('png')
    expect(p.width).toBe(0)
    expect(p.height).toBe(0)
    expect(p.adjust).toBeTruthy()
    expect(p.retouch).toBeUndefined()
    expect(p.face_points).toBeUndefined()
    expect(p.body).toBeUndefined()
    expect(p.body_points).toBeUndefined()
    expect(emptyFinish(p.finish)).toBe(true)
    expect(ExportImage).not.toHaveBeenCalled()
    expect(EnqueuePostSave).not.toHaveBeenCalled()
  })

  it.each(['jpeg', 'png'])('history stays full-size PNG when file export uses %s', async format => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('export.export_settings'))
    const select = field(wrapper, 'export-format')
    expect(select.element.value).toBe('jpeg')
    await select.setValue(format)
    await settle()
    await clickSave(wrapper)

    expect(SavePostToHistory).toHaveBeenCalledTimes(1)
    const p = lastCall()
    expect(p.format).toBe('png')
    expect(p.width).toBe(0)
    expect(p.height).toBe(0)
    expect(strip(p.image_base64)).toBe(IMG_A)
    expect(ExportImage).not.toHaveBeenCalled()
    expect(EnqueuePostSave).not.toHaveBeenCalled()
  })

  it('saves the image currently shown', async () => {
    const wrapper = await mountPage()
    useRemixQueue().setPostQueue([{ path: '/photos/b.png', name: 'b.png' }])
    await settlePreview()
    await clickSave(wrapper)
    expect(strip(lastCall().image_base64)).toBe(IMG_B)
  })

  it('sends the correction values', async () => {
    const wrapper = await mountPage()
    await setBrightness(wrapper, 25)
    await clickSave(wrapper)
    expect(lastCall().adjust.brightness).toBe(25)
  })

  it('sends retouch with face points when retouch is on', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await field(wrapper, 'retouch-teeth').setValue(true)
    await settlePreview()
    await clickSave(wrapper)

    const p = lastCall()
    expect(p.retouch).toEqual({ teeth: true })
    expect(p.face_points).toEqual(PTS_A)
  })

  it('retouch on without face points saves without face points', async () => {
    PostFaceLandmarks.mockImplementation(() => Promise.resolve({ points: [], skip: 'no_face' }))
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.retouch_title'))
    await field(wrapper, 'retouch-teeth').setValue(true)
    await settlePreview()
    await clickSave(wrapper)

    expect(SavePostToHistory).toHaveBeenCalledTimes(1)
    const p = lastCall()
    expect(p.face_points === undefined || (Array.isArray(p.face_points) && p.face_points.length === 0)).toBe(true)
  })

  it('sends body with body points when the figure is changed', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.body_title'))
    await field(wrapper, 'body-width').setValue(-5)
    await settlePreview()
    await clickSave(wrapper)

    const p = lastCall()
    expect(Number(p.body?.width || 0)).toBe(-5)
    expect(Number(p.body?.height || 0)).toBe(0)
    expect(p.body_points).toEqual(BODY_A)
  })

  it('sends finish when format or frame is set', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('post.finish_title'))
    await field(wrapper, 'finish-frame').setValue('white')
    await settle()
    await field(wrapper, 'finish-caption').setValue('Hi')
    await settlePreview()
    await clickSave(wrapper)

    const p = lastCall()
    expect(p.finish).toBeTruthy()
    expect(p.finish.frame).toBe('white')
    expect(p.finish.caption).toBe('Hi')
  })

  it('shows the success message', async () => {
    const wrapper = await mountPage()
    await clickSave(wrapper)
    expect(wrapper.text()).toContain(t('post.saved_history'))
  })

  it('each click is a new save', async () => {
    const wrapper = await mountPage()
    await clickSave(wrapper)
    await clickSave(wrapper)
    expect(SavePostToHistory).toHaveBeenCalledTimes(2)
    expect(strip(SavePostToHistory.mock.calls[0][0].image_base64)).toBe(IMG_A)
    expect(strip(SavePostToHistory.mock.calls[1][0].image_base64)).toBe(IMG_A)
  })

  it('page state stays the same after saving', async () => {
    const wrapper = await mountPage()
    const row = await setBrightness(wrapper, 25)
    await openSection(wrapper, t('post.retouch_title'))
    await field(wrapper, 'retouch-teeth').setValue(true)
    await settlePreview()
    await clickSave(wrapper)
    await settlePreview()

    expect(Number(row.find('input[type="range"]').element.value)).toBe(25)
    expect(field(wrapper, 'retouch-teeth').element.checked).toBe(true)
    expect(saveBtn(wrapper).element.disabled).toBe(false)

    await clickSave(wrapper)
    const p = lastCall()
    expect(strip(p.image_base64)).toBe(IMG_A)
    expect(p.adjust.brightness).toBe(25)
    expect(p.retouch).toEqual({ teeth: true })
  })

  it('shows the binding error', async () => {
    SavePostToHistory.mockImplementation(() => Promise.reject(new Error('no active history folder')))
    const wrapper = await mountPage()
    await clickSave(wrapper)
    expect(wrapper.text()).toContain('no active history folder')
    expect(wrapper.text()).not.toContain(t('post.saved_history'))
    expect(saveBtn(wrapper).element.disabled).toBe(false)
  })
})
