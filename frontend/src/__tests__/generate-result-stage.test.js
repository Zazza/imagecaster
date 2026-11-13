import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import GeneratePage from '../components/GeneratePage.vue'
import ImageViewer from '../components/ImageViewer.vue'
import { GetActiveSessionItem } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

let job = 0
async function emitImage(data = 'AAAA') {
  job++
  EventsEmit('queue:completed', { job_id: 'j' + job, result: JSON.stringify({ image_base64: data }) })
  await settle()
}

async function pageWithResult() {
  const wrapper = mount(GeneratePage, { attachTo: document.body })
  await settle()
  await emitImage()
  const img = wrapper.find('img[alt="Generated"]')
  expect(img.exists(), 'result image').toBe(true)
  return { wrapper, img }
}

const label = w => w.find('.split-zoom .split-zoom-label')

async function choosePreset(wrapper, text) {
  await label(wrapper).trigger('click')
  const btn = wrapper.findAll('.split-zoom-menu button').find(b => b.text().trim() === text)
  expect(btn, `preset ${text}`).toBeTruthy()
  await btn.trigger('click')
  await nextTick()
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetActiveSessionItem.mockImplementation(() => Promise.resolve({ id: 1 }))
})

afterEach(() => vi.unstubAllGlobals())

describe('GeneratePage result stage', () => {
  it('9: image is inside .gen-stage within .gen-stage-wrap; viewfinder is a sibling of the stage', async () => {
    const { wrapper } = await pageWithResult()
    const wrap = wrapper.find('.gen-stage-wrap')
    expect(wrap.exists()).toBe(true)
    const stage = wrap.find('.gen-stage')
    expect(stage.exists()).toBe(true)
    expect(stage.find('img[alt="Generated"]').exists()).toBe(true)
    const vf = wrap.find('.viewfinder')
    expect(vf.exists()).toBe(true)
    expect(vf.element.parentElement).toBe(stage.element.parentElement)
    expect(stage.find('.viewfinder').exists()).toBe(false)
  })

  it('10: zoom controls are minus, label, plus', async () => {
    const { wrapper } = await pageWithResult()
    const btns = wrapper.findAll('.split-zoom > button')
    expect(btns).toHaveLength(3)
    expect(btns[0].text()).toMatch(/^[−-]$/)
    expect(btns[1].classes()).toContain('split-zoom-label')
    expect(btns[1].text()).toBe(t('viewer.zoom_fit'))
    expect(btns[2].text()).toBe('+')
  })

  it('10: label click opens menu with fit, 30%, 50%, 100%; choosing 100% shows it on the label', async () => {
    const { wrapper } = await pageWithResult()
    expect(wrapper.find('.split-zoom-menu').exists()).toBe(false)
    await label(wrapper).trigger('click')
    const menu = wrapper.find('.split-zoom-menu')
    expect(menu.exists()).toBe(true)
    const items = menu.findAll('button').map(b => b.text().trim())
    expect(items).toEqual(expect.arrayContaining([t('viewer.zoom_fit'), '30%', '50%', '100%']))
    expect(items).toHaveLength(4)
    await menu.findAll('button').find(b => b.text().trim() === '100%').trigger('click')
    await nextTick()
    expect(label(wrapper).text()).toBe('100%')
  })

  it('11: click on the image in fit mode opens the viewer', async () => {
    const { wrapper, img } = await pageWithResult()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(false)
    await img.trigger('click')
    await settle()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(true)
  })

  it('11: click on the image in zoomed mode does not open the viewer', async () => {
    const { wrapper, img } = await pageWithResult()
    await choosePreset(wrapper, '100%')
    expect(label(wrapper).text()).toBe('100%')
    await img.trigger('click')
    await settle()
    expect(wrapper.findComponent(ImageViewer).exists()).toBe(false)
  })

  it('12: a new image returns the view to fit', async () => {
    const { wrapper } = await pageWithResult()
    await choosePreset(wrapper, '100%')
    expect(label(wrapper).text()).toBe('100%')
    await emitImage('BBBB')
    expect(label(wrapper).text()).toBe(t('viewer.zoom_fit'))
  })
})
