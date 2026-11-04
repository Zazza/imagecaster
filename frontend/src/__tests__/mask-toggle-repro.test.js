import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import PostProcessPage from '../components/PostProcessPage.vue'
import { GetSettings, GetActiveSessionItem, GetSessionImage } from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)

async function mountPage(settings = {}, section = '') {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  const wrapper = mount(PostProcessPage, { props: section ? { initialSection: section } : {} })
  await flushPromises()
  return wrapper
}

describe('mask overlay eye toggle', () => {
  beforeEach(() => { vi.clearAllMocks(); clearEventMocks() })

  it('toggles repeatedly', async () => {
    const wrapper = await mountPage({ fi_mode: 'inpaint' }, 'remix')
    await wrapper.setProps({ initialImage: 'aW1n' })
    await flushPromises()

    const canvas = wrapper.find('canvas.split-mask')
    expect(canvas.exists()).toBe(true)
    const shown = () => !canvas.element.classList.contains('mask-overlay-hidden')

    expect(shown()).toBe(true)
    const buttons = wrapper.findAll('.split-zoom-btn')
    const eye = buttons.find(b => (b.attributes('title') || '').includes('маск') || (b.attributes('title') || '').toLowerCase().includes('mask'))

    await eye.trigger('click')
    await flushPromises()
    console.log('after hide click, display =', JSON.stringify(canvas.element.style.display), '| title now:', eye.attributes('title'))
    expect(shown()).toBe(false)

    await eye.trigger('click')
    await flushPromises()
    console.log('after show click, display =', JSON.stringify(canvas.element.style.display), '| title now:', eye.attributes('title'))
    expect(shown()).toBe(true)

    await eye.trigger('click')
    await flushPromises()
    console.log('after hide2 click, display =', JSON.stringify(canvas.element.style.display), '| title now:', eye.attributes('title'))
    expect(shown()).toBe(false)
  })
})
