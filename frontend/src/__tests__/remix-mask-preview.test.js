import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import PostProcessPage from '../components/PostProcessPage.vue'
import { GetSettings, GetActiveSessionItem, GetSessionImage } from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)

function pageProps(section) {
  return section ? { initialSection: section } : {}
}

async function mountPage(settings = {}, section = '') {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  const wrapper = mount(PostProcessPage, { props: pageProps(section) })
  await flushPromises()
  return wrapper
}

describe('Remix mask mode on post-process preview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
  })

  it('remix section open + inpaint mode: preview switches to mask painting', async () => {
    const wrapper = await mountPage({ fi_mode: 'inpaint' }, 'remix')
    await wrapper.setProps({ initialImage: 'aW1n' })
    await flushPromises()

    const area = wrapper.find('.split-area')
    expect(area.classes()).toContain('masking')
    expect(wrapper.find('canvas.split-mask').exists()).toBe(true)

    await area.trigger('pointermove', { clientX: 40, clientY: 30 })
    expect(wrapper.find('.brush-cursor').exists()).toBe(true)
  })

  it('remix section open + img2img mode: no mask layer, panning behaviour kept', async () => {
    const wrapper = await mountPage({ fi_mode: 'img2img' }, 'remix')
    await wrapper.setProps({ initialImage: 'aW1n' })
    await flushPromises()

    const area = wrapper.find('.split-area')
    expect(area.classes()).not.toContain('masking')
    expect(wrapper.find('canvas.split-mask').exists()).toBe(false)
  })

  it('other section open: mask layer hidden even in inpaint mode', async () => {
    const wrapper = await mountPage({ fi_mode: 'inpaint' }, 'edit')
    await wrapper.setProps({ initialImage: 'aW1n' })
    await flushPromises()

    expect(wrapper.find('.split-area').classes()).not.toContain('masking')
    expect(wrapper.find('canvas.split-mask').exists()).toBe(false)
  })
})
