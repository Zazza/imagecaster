import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).createAppMock({
  ListHistory: vi.fn(),
}))

import ImageViewer from '../components/ImageViewer.vue'
import GalleryPage from '../components/GalleryPage.vue'
import FileBrowserPage from '../components/FileBrowserPage.vue'
import RemixSection from '../components/RemixSection.vue'
import {
  DescribePhoto, UpdateSettings, IsKidsModeActive, ListHistory, GetSessionImage, GetSessionThumb,
  GetSettings, BrowseDirectory, ReadFileAsBase64, ReadThumbnail,
} from '../wailsjs/go/main/App.js'
import { describeToGenerate } from '../describeToGenerate.js'
import { useKidsMode } from '../composables/useKidsMode.js'
import { setLocale } from '../i18n/index.js'
import { settle, waitUntil } from './helpers/async.js'

enableAutoUnmount(afterEach)

const IMG = 'QUFBQQ=='
const IMG2 = 'QkJCQg=='
const TEXT = 'Рыжий кот сидит на подоконнике.'
const TAGS = 'cat, ginger, window'

const $ = (sel) => document.querySelector(sel)
const tid = (id) => $(`[data-testid="${id}"]`)
const valueOf = (el) => (el && 'value' in el && el.value !== undefined ? el.value : el?.textContent || '').trim()
const buttonsByText = (root, text) => [...root.querySelectorAll('button')].filter(b => b.textContent.trim() === text)

async function click(el) {
  expect(el, 'element exists').toBeTruthy()
  el.click()
  await settle()
}

function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

let clipboardWrite

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  localStorage.clear()
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  DescribePhoto.mockImplementation((img, mode) => Promise.resolve(mode === 'quick' ? TAGS : TEXT))
  UpdateSettings.mockImplementation(() => Promise.resolve())
  IsKidsModeActive.mockImplementation(() => Promise.resolve(false))
  clipboardWrite = vi.fn(() => Promise.resolve())
  Object.defineProperty(navigator, 'clipboard', { value: { writeText: clipboardWrite }, configurable: true })
})

afterEach(() => {
  useKidsMode().kidsModeActive.value = false
  vi.unstubAllGlobals()
  setLocale('ru')
})

async function mountViewer(props = {}) {
  const w = mount(ImageViewer, { props: { imageBase64: IMG, describable: true, ...props }, attachTo: document.body })
  await settle()
  return w
}

async function openPanel() {
  await click(tid('viewer-describe'))
  await waitUntil(() => tid('describe-panel'), { message: 'describe panel' })
}

describe('1. ImageViewer: «Описать» opens the panel and describes as text', () => {
  it('describable viewer shows «Описать»', async () => {
    await mountViewer()
    const btn = tid('viewer-describe')
    expect(btn).toBeTruthy()
    expect(btn.textContent).toContain('Описать')
  })

  it('«Описать» is on by default; describable=false hides it', async () => {
    const w2 = mount(ImageViewer, { props: { imageBase64: IMG }, attachTo: document.body })
    await settle()
    expect(w2.find('[data-testid="viewer-describe"]').exists()).toBe(true)
    w2.unmount()
    await mountViewer({ describable: false })
    expect(tid('viewer-describe')).toBeNull()
  })

  it('nothing is described until the button is pressed', async () => {
    await mountViewer()
    expect(DescribePhoto).not.toHaveBeenCalled()
    expect(tid('describe-panel')).toBeNull()
  })

  it('click opens the panel, calls DescribePhoto(image, describe, false) and shows the text', async () => {
    await mountViewer()
    await openPanel()
    expect(DescribePhoto).toHaveBeenCalledTimes(1)
    expect(DescribePhoto).toHaveBeenCalledWith(IMG, 'describe', false)
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT, { message: 'text shown' })
  })

  it('panel has «Текст» and «Теги» mode buttons', async () => {
    await mountViewer()
    await openPanel()
    const panel = tid('describe-panel')
    expect(buttonsByText(panel, 'Текст').length).toBeGreaterThan(0)
    expect(buttonsByText(panel, 'Теги').length).toBeGreaterThan(0)
    expect(tid('describe-mode-quick')).toBeTruthy()
  })
})

describe('2. Mode «Теги» and «Описать заново»', () => {
  it('«Теги» calls DescribePhoto(image, quick, false) and shows tags', async () => {
    await mountViewer()
    await openPanel()
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT)
    await click(tid('describe-mode-quick'))
    expect(DescribePhoto).toHaveBeenLastCalledWith(IMG, 'quick', false)
    await waitUntil(() => valueOf(tid('describe-text')) === TAGS, { message: 'tags shown' })
  })

  it('«Описать заново» calls with fresh=true and shows the new answer', async () => {
    await mountViewer()
    await openPanel()
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT)
    DescribePhoto.mockImplementation(() => Promise.resolve('Новое описание'))
    const again = tid('describe-again')
    expect(again.textContent).toContain('Описать заново')
    await click(again)
    expect(DescribePhoto).toHaveBeenLastCalledWith(IMG, 'describe', true)
    await waitUntil(() => valueOf(tid('describe-text')) === 'Новое описание')
  })

  it('«Описать заново» in tag mode re-describes tags with fresh=true', async () => {
    await mountViewer()
    await openPanel()
    await click(tid('describe-mode-quick'))
    await waitUntil(() => valueOf(tid('describe-text')) === TAGS)
    await click(tid('describe-again'))
    expect(DescribePhoto).toHaveBeenLastCalledWith(IMG, 'quick', true)
  })
})

describe('3. «Копировать»', () => {
  it('writes the shown text to the clipboard', async () => {
    await mountViewer()
    await openPanel()
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT)
    const copy = tid('describe-copy')
    expect(copy.textContent).toContain('Копировать')
    await click(copy)
    expect(clipboardWrite).toHaveBeenCalledWith(TEXT)
  })
})

describe('4. «В Генерацию»', () => {
  it('ImageViewer «В Генерацию» saves the text, navigates via the app and closes itself', async () => {
    const navigate = vi.fn()
    const w = mount(ImageViewer, { props: { imageBase64: IMG }, global: { provide: { navigate } }, attachTo: document.body })
    await settle()
    await openPanel()
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT)
    const btn = tid('describe-to-generate')
    expect(btn.textContent).toContain('В Генерацию')
    await click(btn)
    expect(UpdateSettings.mock.calls.at(-1)[0]).toMatchObject({ gen_description: TEXT })
    expect(navigate).toHaveBeenCalledWith({ page: 'generate' })
    expect(w.emitted('close')).toBeTruthy()
  })

  it('describeToGenerate saves gen_description, then navigates to generate', async () => {
    const d = deferred()
    UpdateSettings.mockImplementation(() => d.promise)
    const navigate = vi.fn()
    const done = describeToGenerate(TEXT, navigate)
    await settle()
    expect(UpdateSettings).toHaveBeenCalledTimes(1)
    expect(UpdateSettings.mock.calls[0][0]).toMatchObject({ gen_description: TEXT })
    expect(navigate).not.toHaveBeenCalled()
    d.resolve()
    await done
    await settle()
    expect(navigate).toHaveBeenCalledTimes(1)
    expect(navigate.mock.calls[0][0]).toMatchObject({ page: 'generate' })
  })

  async function sendFromPage(wrapper) {
    await openPanel()
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT)
    const d = deferred()
    UpdateSettings.mockImplementation(() => d.promise)
    await click(tid('describe-to-generate'))
    expect(UpdateSettings).toHaveBeenCalled()
    expect(UpdateSettings.mock.calls.at(-1)[0]).toMatchObject({ gen_description: TEXT })
    expect(wrapper.navigate).not.toHaveBeenCalled()
    d.resolve()
    await settle()
    expect(wrapper.navigate).toHaveBeenCalledWith({ page: 'generate' })
  }

  it('GalleryPage: viewer has «Описать» and «В Генерацию» saves the text and navigates', async () => {
    ListHistory.mockImplementation(() => Promise.resolve({
      items: [{ id: 5, session_id: 1, session_name: 'P', file_name: '5.png', thumb_name: '5_t.jpg', source: 'generate',
        prompt: 'p', width: 512, height: 512, created_at: '2026-11-20 10:00:00', kind: 'generate', model_name: 'm', label: 'L', shape: 'square' }],
      total: 1,
      facets: { sessions: [], models: [], labels: [], kinds: [], shapes: [] },
    }))
    GetSessionImage.mockImplementation(() => Promise.resolve(IMG))
    GetSessionThumb.mockImplementation(() => Promise.resolve(IMG))
    const navigate = vi.fn()
    const w = mount(GalleryPage, { global: { provide: { navigate } }, attachTo: document.body })
    w.navigate = navigate
    await settle()
    await click(document.querySelector('[data-testid="gallery-item"]'))
    await waitUntil(() => tid('viewer-describe'), { message: 'gallery viewer describe button' })
    await sendFromPage(w)
    expect(DescribePhoto.mock.calls[0][0]).toBe(IMG)
  })

  it('FileBrowserPage: viewer has «Описать» and «В Генерацию» saves the text and navigates', async () => {
    GetSettings.mockImplementation(() => Promise.resolve({ file_browser_path: '/pics' }))
    BrowseDirectory.mockImplementation(() => Promise.resolve([
      { name: 'one.png', path: '/pics/one.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
    ]))
    ReadFileAsBase64.mockImplementation(() => Promise.resolve(IMG))
    ReadThumbnail.mockImplementation(() => Promise.resolve(IMG))
    const navigate = vi.fn()
    const w = mount(FileBrowserPage, { global: { provide: { navigate } }, attachTo: document.body })
    w.navigate = navigate
    await settle()
    const card = w.findAll('.fb-card').find(c => !c.classes().includes('fb-card-dir'))
    expect(card).toBeTruthy()
    await card.trigger('click')
    await settle()
    await waitUntil(() => tid('viewer-describe'), { message: 'file browser viewer describe button' })
    await sendFromPage(w)
    expect(DescribePhoto.mock.calls[0][0]).toBe(IMG)
  })
})

describe('5. Errors', () => {
  it('a DescribePhoto failure is shown in the panel', async () => {
    DescribePhoto.mockImplementation(() => Promise.reject(new Error('vision down')))
    await mountViewer()
    await openPanel()
    const err = await waitUntil(() => tid('describe-error'), { message: 'describe-error' })
    expect(tid('describe-panel').contains(err)).toBe(true)
    expect(err.textContent.trim()).not.toBe('')
  })
})

describe('6. Kids mode', () => {
  it('text field is editable normally', async () => {
    await mountViewer()
    await openPanel()
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT)
    const f = tid('describe-text')
    expect(Boolean(f.readOnly || f.disabled)).toBe(false)
  })

  it('text field is read-only in kids mode', async () => {
    IsKidsModeActive.mockImplementation(() => Promise.resolve(true))
    useKidsMode().kidsModeActive.value = true
    await mountViewer()
    await openPanel()
    await waitUntil(() => valueOf(tid('describe-text')) === TEXT)
    const f = tid('describe-text')
    expect(Boolean(f.readOnly || f.disabled)).toBe(true)
  })
})

describe('7. RemixSection: automatic description', () => {
  async function mountRemix(props = {}) {
    GetSettings.mockImplementation(() => Promise.resolve({}))
    const w = mount(RemixSection, { props: { source: IMG, active: true, ...props }, attachTo: document.body })
    await settle()
    return w
  }
  const descArea = (w) => w.findAll('textarea')[0]

  it('has no «Текст»/«Теги» mode buttons', async () => {
    const w = await mountRemix()
    const labels = w.findAll('button').map(b => b.text().trim())
    expect(labels).not.toContain('Текст')
    expect(labels).not.toContain('Теги')
    expect(w.find('[data-testid="describe-mode-quick"]').exists()).toBe(false)
  })

  it('active with a source and empty description: describes as text and fills the description', async () => {
    const w = await mountRemix()
    expect(DescribePhoto).toHaveBeenCalledTimes(1)
    expect(DescribePhoto).toHaveBeenCalledWith(IMG, 'describe', false)
    await waitUntil(() => descArea(w).element.value === TEXT, { message: 'description filled' })
  })

  it('text typed before the result arrives is not overwritten', async () => {
    const d = deferred()
    DescribePhoto.mockImplementation(() => d.promise)
    const w = await mountRemix()
    expect(DescribePhoto).toHaveBeenCalled()
    await descArea(w).setValue('моё описание')
    d.resolve(TEXT)
    await settle()
    expect(descArea(w).element.value).toBe('моё описание')
  })

  it('a new source clears the description and describes the new photo', async () => {
    const w = await mountRemix()
    await waitUntil(() => descArea(w).element.value === TEXT)
    const d = deferred()
    DescribePhoto.mockImplementation(() => d.promise)
    await w.setProps({ source: IMG2 })
    await settle()
    expect(DescribePhoto).toHaveBeenLastCalledWith(IMG2, 'describe', false)
    expect(descArea(w).element.value).toBe('')
    d.resolve('Второе фото')
    await settle()
    expect(descArea(w).element.value).toBe('Второе фото')
  })

  it('«Описать заново» calls with fresh=true and replaces the text', async () => {
    const w = await mountRemix()
    await waitUntil(() => descArea(w).element.value === TEXT)
    await descArea(w).setValue('правка')
    DescribePhoto.mockImplementation(() => Promise.resolve('Свежее описание'))
    const again = w.find('[data-testid="remix-describe-again"]')
    expect(again.exists()).toBe(true)
    expect(again.text()).toContain('Описать заново')
    await again.trigger('click')
    await settle()
    expect(DescribePhoto).toHaveBeenLastCalledWith(IMG, 'describe', true)
    expect(descArea(w).element.value).toBe('Свежее описание')
  })

  it('not active: no DescribePhoto call', async () => {
    await mountRemix({ active: false })
    expect(DescribePhoto).not.toHaveBeenCalled()
  })

  it('active without a source: no DescribePhoto call', async () => {
    await mountRemix({ source: '' })
    expect(DescribePhoto).not.toHaveBeenCalled()
  })
})
