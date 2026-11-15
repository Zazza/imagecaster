import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import ImageViewer from '../components/ImageViewer.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes, ListHiresProfiles,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

useFakeImage()
useFakeCanvas()

const IMG = 'aW1n'
const IMG2 = 'aW1nMg=='

function setSize(el, w, h) {
  Object.defineProperty(el, 'clientWidth', { value: w, configurable: true })
  Object.defineProperty(el, 'clientHeight', { value: h, configurable: true })
}

function setNatural(el, w, h) {
  Object.defineProperty(el, 'naturalWidth', { value: w, configurable: true })
  Object.defineProperty(el, 'naturalHeight', { value: h, configurable: true })
}

function translate(el) {
  const m = /translate\((-?[\d.]+)px, (-?[\d.]+)px\)/.exec(el.style.transform)
  return m ? [Number(m[1]), Number(m[2])] : null
}

function scale(el) {
  const m = /scale\(([\d.]+)\)/.exec(el.style.transform)
  return m ? Number(m[1]) : null
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve([]))
  ListPresetTypes.mockReturnValue(Promise.resolve([]))
})

async function mountPost(area = [500, 500], img = [1000, 1000]) {
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  const areaEl = wrapper.find('.split-area')
  expect(areaEl.exists()).toBe(true)
  setSize(areaEl.element, ...area)
  window.dispatchEvent(new Event('resize'))
  const after = wrapper.find('img.split-after')
  setNatural(after.element, ...img)
  await after.trigger('load')
  await nextTick()
  return wrapper
}

const postLabel = w => w.find('.split-zoom .split-zoom-label')
const postBtn = (w, key) => w.find(`.split-zoom [aria-label="${t(key)}"]`)

describe('PostProcessPage zoom/pan', () => {
  it('starts in fit; plus zooms from the fit percent; label returns to fit', async () => {
    const w = await mountPost([500, 500], [1000, 1000])
    expect(postLabel(w).text()).toBe(t('viewer.zoom_fit'))
    expect(w.find('img.split-after').attributes('style')).toBeFalsy()
    await postBtn(w, 'viewer.zoom_in').trigger('click')
    expect(postLabel(w).text()).toBe('60%')
    await postLabel(w).trigger('click')
    expect(postLabel(w).text()).toBe(t('viewer.zoom_fit'))
    expect(w.find('.split-zoom-menu').exists()).toBe(false)
  })

  it('label in fit opens the preset menu; a preset sets zoom and closes the menu', async () => {
    const w = await mountPost()
    await postLabel(w).trigger('click')
    const menu = w.find('.split-zoom-menu')
    expect(menu.exists()).toBe(true)
    expect(menu.attributes('role')).toBe('menu')
    expect(menu.findAll('button').map(b => b.text().trim())).toEqual([t('viewer.zoom_fit'), '30%', '50%', '100%'])
    expect(menu.find('button.active').text()).toBe(t('viewer.zoom_fit'))
    await menu.findAll('button').find(b => b.text().trim() === '30%').trigger('click')
    expect(postLabel(w).text()).toBe('30%')
    expect(w.find('.split-zoom-menu').exists()).toBe(false)
  })

  it('minus is disabled in fit when fit percent is at the minimum; plus is disabled at 500%', async () => {
    const w = await mountPost([100, 100], [2000, 2000])
    expect(postBtn(w, 'viewer.zoom_out').attributes('disabled')).toBeDefined()
    await postBtn(w, 'viewer.zoom_in').trigger('click')
    expect(postLabel(w).text()).toBe('10%')
    expect(postBtn(w, 'viewer.zoom_out').attributes('disabled')).toBeUndefined()
    await postBtn(w, 'viewer.zoom_out').trigger('click')
    expect(postLabel(w).text()).toBe('10%')
    for (let i = 0; i < 60; i++) await postBtn(w, 'viewer.zoom_in').trigger('click')
    expect(postLabel(w).text()).toBe('500%')
    expect(postBtn(w, 'viewer.zoom_in').attributes('disabled')).toBeDefined()
  })

  it('wheel zooms only with ctrl, in steps of 10, bounded 10..500', async () => {
    const w = await mountPost([500, 500], [1000, 1000])
    const area = w.find('.split-area')
    await area.trigger('wheel', { deltaY: -100 })
    expect(postLabel(w).text()).toBe(t('viewer.zoom_fit'))
    await area.trigger('wheel', { deltaY: -100, ctrlKey: true })
    expect(postLabel(w).text()).toBe('60%')
    await area.trigger('wheel', { deltaY: 100, ctrlKey: true })
    expect(postLabel(w).text()).toBe('50%')
    for (let i = 0; i < 10; i++) await area.trigger('wheel', { deltaY: 100, ctrlKey: true })
    expect(postLabel(w).text()).toBe('10%')
    for (let i = 0; i < 60; i++) await area.trigger('wheel', { deltaY: -100, ctrlKey: true })
    expect(postLabel(w).text()).toBe('500%')
  })

  it('double click toggles fit and 100%', async () => {
    const w = await mountPost()
    await w.find('.split-area').trigger('dblclick')
    expect(postLabel(w).text()).toBe('100%')
    await w.find('.split-area').trigger('dblclick')
    expect(postLabel(w).text()).toBe(t('viewer.zoom_fit'))
  })

  it('zoomed image is a scaled layer; drag pans within bounds and suppresses the next double click', async () => {
    const w = await mountPost([500, 500], [1000, 1000])
    const area = w.find('.split-area')
    expect(area.classes()).not.toContain('pannable')
    await w.find('.split-area').trigger('dblclick')
    expect(area.classes()).toContain('pannable')
    const after = w.find('img.split-after').element
    const before = w.find('img.split-before').element
    expect(scale(after)).toBe(2)
    expect(translate(after)).toEqual([0, 0])
    await area.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100, pointerId: 1 })
    expect(area.classes()).toContain('panning')
    await area.trigger('pointermove', { clientX: 400, clientY: 50, pointerId: 1 })
    expect(translate(after)).toEqual([250, -50])
    expect(translate(before)).toEqual([250, -50])
    await area.trigger('pointerup', { pointerId: 1 })
    expect(area.classes()).not.toContain('panning')
    await area.trigger('dblclick')
    expect(postLabel(w).text()).toBe('100%')
  })

  it('pan does not start in fit mode or with a non-left button', async () => {
    const w = await mountPost([500, 500], [1000, 1000])
    const area = w.find('.split-area')
    await area.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100, pointerId: 1 })
    expect(area.classes()).not.toContain('panning')
    await area.trigger('dblclick')
    await area.trigger('pointerdown', { button: 2, clientX: 100, clientY: 100, pointerId: 1 })
    expect(area.classes()).not.toContain('panning')
  })

  it('pointer down on the view closes the zoom menu', async () => {
    const w = await mountPost()
    await postLabel(w).trigger('click')
    expect(w.find('.split-zoom-menu').exists()).toBe(true)
    await w.find('.split-area').trigger('pointerdown', { button: 0, clientX: 1, clientY: 1, pointerId: 1 })
    expect(w.find('.split-zoom-menu').exists()).toBe(false)
  })

  it('a new source image returns the view to fit', async () => {
    const w = await mountPost()
    await w.find('.split-area').trigger('dblclick')
    expect(postLabel(w).text()).toBe('100%')
    await w.setProps({ initialImage: IMG2 })
    await settle()
    expect(postLabel(w).text()).toBe(t('viewer.zoom_fit'))
  })

  it('double click does not zoom while the effect mask is being painted', async () => {
    const w = await mountPost()
    await w.findAll('button').find(b => b.text().trim() === t('post.effect_mask_btn')).trigger('click')
    await nextTick()
    expect(w.find('.split-area').classes()).toContain('masking')
    await w.find('.split-area').trigger('dblclick')
    expect(postLabel(w).text()).toBe(t('viewer.zoom_fit'))
  })
})

async function mountViewer(stage = [820, 620], img = [1600, 1200]) {
  const w = mount(ImageViewer, { props: { imageBase64: IMG }, attachTo: document.body })
  await nextTick()
  const st = w.find('.image-viewer-stage').element
  st.style.padding = '10px'
  setSize(st, ...stage)
  window.dispatchEvent(new Event('resize'))
  const im = w.find('img')
  setNatural(im.element, ...img)
  await im.trigger('load')
  await nextTick()
  return w
}

const ivLabel = w => w.find('.image-viewer-zoom-label')
const ivBtn = (w, key) => w.find(`.image-viewer-zoom [aria-label="${t(key)}"]`)

describe('ImageViewer zoom/pan', () => {
  it('fit percent uses the stage size minus padding', async () => {
    const w = await mountViewer([820, 620], [1600, 1200])
    await ivBtn(w, 'viewer.zoom_out').trigger('click')
    expect(ivLabel(w).text()).toBe('40%')
  })

  it('minus is disabled at minimum fit; plus never gets disabled', async () => {
    const w = await mountViewer([110, 110], [2000, 2000])
    expect(ivBtn(w, 'viewer.zoom_out').attributes('disabled')).toBeDefined()
    for (let i = 0; i < 60; i++) await ivBtn(w, 'viewer.zoom_in').trigger('click')
    expect(ivLabel(w).text()).toBe('500%')
    expect(ivBtn(w, 'viewer.zoom_in').attributes('disabled')).toBeUndefined()
  })

  it('ctrl+wheel zooms the stage', async () => {
    const w = await mountViewer()
    const stage = w.find('.image-viewer-stage')
    await stage.trigger('wheel', { deltaY: -100 })
    expect(ivLabel(w).text()).toBe(t('viewer.zoom_fit'))
    await stage.trigger('wheel', { deltaY: -100, ctrlKey: true })
    expect(ivLabel(w).text()).toBe('60%')
  })

  it('zoomed image gets width and translate; drag pans within bounds and suppresses double click', async () => {
    const w = await mountViewer([820, 620], [1600, 1200])
    const img = w.find('img')
    expect(img.classes()).not.toContain('zoomed')
    await img.trigger('dblclick')
    expect(ivLabel(w).text()).toBe('100%')
    expect(img.classes()).toContain('zoomed')
    expect(img.classes()).toContain('pannable')
    expect(img.element.style.width).toBe('1600px')
    expect(translate(img.element)).toEqual([0, 0])
    await img.trigger('pointerdown', { button: 0, clientX: 0, clientY: 0, pointerId: 1 })
    expect(img.classes()).toContain('panning')
    await img.trigger('pointermove', { clientX: 900, clientY: -100, pointerId: 1 })
    expect(translate(img.element)).toEqual([400, -100])
    await img.trigger('pointerup', { pointerId: 1 })
    expect(img.classes()).not.toContain('panning')
    await img.trigger('dblclick')
    expect(ivLabel(w).text()).toBe('100%')
  })

  it('pointer down on the stage closes the menu; a new image returns to fit', async () => {
    const w = await mountViewer()
    await ivLabel(w).trigger('click')
    expect(w.find('.image-viewer-zoom-menu').exists()).toBe(true)
    await w.find('.image-viewer-stage').trigger('pointerdown', { button: 0, pointerId: 1 })
    expect(w.find('.image-viewer-zoom-menu').exists()).toBe(false)
    await w.find('img').trigger('dblclick')
    expect(ivLabel(w).text()).toBe('100%')
    await w.setProps({ imageBase64: IMG2 })
    expect(ivLabel(w).text()).toBe(t('viewer.zoom_fit'))
  })
})
