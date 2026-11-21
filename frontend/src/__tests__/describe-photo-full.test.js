import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import FileBrowserPage from '../components/FileBrowserPage.vue'
import {
  DescribePhoto, GetSettings, BrowseDirectory, ReadFileAsBase64, ReadThumbnail, IsKidsModeActive,
} from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { settle, waitUntil } from './helpers/async.js'

enableAutoUnmount(afterEach)

const THUMB = 'VEhVTUI='
const FULL1 = 'RlVMTDE='
const FULL2 = 'RlVMTDI='
const tid = (id) => document.querySelector(`[data-testid="${id}"]`)

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('ru')
  IsKidsModeActive.mockImplementation(() => Promise.resolve(false))
  DescribePhoto.mockImplementation(() => Promise.resolve('описание'))
  GetSettings.mockImplementation(() => Promise.resolve({ file_browser_path: '/pics' }))
  BrowseDirectory.mockImplementation(() => Promise.resolve([
    { name: 'one.png', path: '/pics/one.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
    { name: 'two.png', path: '/pics/two.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:01' },
  ]))
  ReadThumbnail.mockImplementation(() => Promise.resolve(THUMB))
  ReadFileAsBase64.mockImplementation(p => Promise.resolve(p.endsWith('one.png') ? FULL1 : FULL2))
})

async function openViewer() {
  const w = mount(FileBrowserPage, { attachTo: document.body })
  await settle()
  const card = w.findAll('.fb-card').find(c => !c.classes().includes('fb-card-dir'))
  await card.trigger('click')
  await settle()
  await waitUntil(() => tid('viewer-describe'))
  tid('viewer-describe').click()
  await settle()
  return w
}

describe('«Мои изображения»: описание по полному фото', () => {
  it('one request per photo, with the full image, not the thumbnail', async () => {
    await openViewer()
    await new Promise(r => setTimeout(r, 500))
    await settle()
    const imgs = DescribePhoto.mock.calls.map(c => c[0])
    expect(imgs).toHaveLength(1)
    expect([FULL1, FULL2]).toContain(imgs[0])
  })

  it('flipping to the next photo describes the next full image once', async () => {
    await openViewer()
    await new Promise(r => setTimeout(r, 500))
    const nav = document.querySelector('.image-viewer-next') || document.querySelector('.image-viewer-prev')
    nav.click()
    await settle()
    await new Promise(r => setTimeout(r, 600))
    await settle()
    expect(document.querySelector('[data-testid="describe-panel"]')).toBeTruthy()
    const imgs = DescribePhoto.mock.calls.map(c => c[0])
    expect([...imgs].sort()).toEqual([FULL1, FULL2].sort())
  })
})

describe('отказ при нелокальном ИИ', () => {
  it('shows the translated refusal in English', async () => {
    const { default: ImageViewer } = await import('../components/ImageViewer.vue')
    setLocale('en')
    DescribePhoto.mockImplementation(() => Promise.reject(new Error('фото описывает только локальная модель ИИ: адрес ИИ в настройках не в локальной сети, фото не отправлено')))
    mount(ImageViewer, { props: { imageBase64: FULL1, describable: true }, attachTo: document.body })
    await settle()
    tid('viewer-describe').click()
    await settle()
    const err = await waitUntil(() => tid('describe-error'))
    expect(err.textContent).toContain('local AI model')
    setLocale('ru')
  })
})

describe('«В Генерацию» на странице «Генерация»', () => {
  it('with the Generate page open the text goes straight into its field, settings are not touched', async () => {
    const { setGenerateDescriptionSink, describeToGenerate } = await import('../describeToGenerate.js')
    const { UpdateSettings } = await import('../wailsjs/go/main/App.js')
    const got = []
    const release = setGenerateDescriptionSink(t => got.push(t))
    const navigate = vi.fn()
    await describeToGenerate('кот', navigate)
    expect(got).toEqual(['кот'])
    expect(UpdateSettings).not.toHaveBeenCalled()
    expect(navigate).toHaveBeenCalledWith({ page: 'generate' })
    release()
    await describeToGenerate('пёс', navigate)
    expect(got).toEqual(['кот'])
    expect(UpdateSettings.mock.calls.at(-1)[0]).toEqual({ gen_description: 'пёс' })
  })

  it('a history picture shown by link is described from the stored image', async () => {
    const { default: ImageViewer } = await import('../components/ImageViewer.vue')
    const { GetSessionImage } = await import('../wailsjs/go/main/App.js')
    GetSessionImage.mockImplementation(() => Promise.resolve(FULL2))
    mount(ImageViewer, { props: { imageBase64: '/api/img/42.jpg' }, attachTo: document.body })
    await settle()
    tid('viewer-describe').click()
    await waitUntil(() => DescribePhoto.mock.calls.length > 0)
    expect(GetSessionImage).toHaveBeenCalledWith(42)
    expect(DescribePhoto.mock.calls[0][0]).toBe(FULL2)
  })
})
