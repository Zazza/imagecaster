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
  ListExportPresets, SaveExportPreset, ListHiresProfiles, ReadFileAsBase64, BatchSaveImage, BatchReset, ExportImage,
  PreviewAdjust, PreviewPost, PreviewPostRetouch, PostFaceLandmarks, EnqueuePostSave,
  SavePostRecipe, ListPostRecipes,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas, FakeImage } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
afterEach(() => vi.unstubAllGlobals())

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

describe('post export keeps actual image geometry', () => {
  it('defaults to JPEG quality 90 and requests no resize', async () => {
    const wrapper = await mountPage()
    await saveSingle(wrapper)
    expect(ExportImage).toHaveBeenCalledTimes(1)
    expect(ExportImage.mock.calls[0][0]).toMatchObject({ width: 0, height: 0, format: 'jpeg', quality: 90 })
    expect(strip(ExportImage.mock.calls[0][0].image_base64)).toBe(IMG_A)
    expect(EnqueuePostSave).not.toHaveBeenCalled()
  })

  it('offers PNG explicitly and removes size, interpolation and 4K controls', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('export.export_settings'))
    const card = wrapper.findAll('.card-header.card-toggle').find(h => h.text().includes(t('export.export_settings'))).element.closest('.card')
    expect(card.querySelector('input[type="number"]')).toBeNull()
    expect(card.querySelector('input[type="checkbox"]')).toBeNull()
    expect(wrapper.find('[data-testid="post-upscale-4k"]').exists()).toBe(false)
    await field(wrapper, 'export-format').setValue('png')
    await saveSingle(wrapper)
    expect(ExportImage.mock.calls[0][0]).toMatchObject({ format: 'png', width: 0, height: 0 })
  })

  it('legacy preset preserves PNG quality but never its resize settings', async () => {
    ListExportPresets.mockResolvedValue([{ id: 17, name: 'Legacy', format: 'png', quality: 73, width: 120, height: 80, lock_ratio: false, interpolation: 'nearest' }])
    const wrapper = await mountPage()
    await openSection(wrapper, t('export.export_settings'))
    await field(wrapper, 'export-preset').setValue('17')
    await saveSingle(wrapper)
    expect(ExportImage.mock.calls[0][0]).toMatchObject({ format: 'png', quality: 73, width: 0, height: 0 })
    expect(ExportImage.mock.calls[0][0].interpolation).not.toBe('nearest')
  })

  it('new preset stores compatibility geometry without an export resize', async () => {
    const wrapper = await mountPage()
    await openSection(wrapper, t('export.export_settings'))
    await field(wrapper, 'export-save-preset').trigger('click')
    await field(wrapper, 'export-preset-name').setValue('Native JPEG')
    await wrapper.findAll('.preset-form .btn-primary').at(-1).trigger('click')
    await settle()
    expect(SaveExportPreset).toHaveBeenCalledTimes(1)
    expect(SaveExportPreset.mock.calls[0][0]).toMatchObject({ width: 0, height: 0, lock_ratio: true, interpolation: 'lanczos', format: 'jpeg', quality: 90 })
  })

  it('mixed batch exports each source without borrowing preview dimensions or bytes', async () => {
    class MixedSizeImage extends FakeImage {
      set src(value) {
        const portrait = String(value).includes(IMG_B)
        this.naturalWidth = this.width = portrait ? 800 : 1600
        this.naturalHeight = this.height = portrait ? 1200 : 900
        super.src = value
      }
      get src() { return super.src }
    }
    vi.stubGlobal('Image', MixedSizeImage)
    const wrapper = await mountPage({ items: TWO, settings: { file_browser_path: '/out' } })
    await wrapper.findAll('.batch-thumb')[1].trigger('click')
    await settle()
    await openSection(wrapper, t('export.export_settings'))
    await buttonWith(wrapper, t('post.batch_save_all', { n: 2 })).trigger('click')
    await settle()
    expect(ExportImage).toHaveBeenCalledTimes(2)
    expect(ExportImage.mock.calls.map(([p]) => strip(p.image_base64))).toEqual([IMG_A, IMG_B])
    for (const [params] of ExportImage.mock.calls) expect(params).toMatchObject({ width: 0, height: 0, format: 'jpeg', quality: 90 })
  })
})
