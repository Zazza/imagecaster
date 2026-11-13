import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import GeneratePage from '../components/GeneratePage.vue'
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

async function page() {
  const wrapper = mount(GeneratePage, { attachTo: document.body })
  await settle()
  return wrapper
}

const inside = (el, sel) => !!el.closest(sel)

function buttonByText(wrapper, key) {
  const text = t(key)
  return wrapper.findAll('button').find(b => b.text().trim().startsWith(text))
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetActiveSessionItem.mockImplementation(() => Promise.resolve({ id: 1 }))
})

afterEach(() => vi.unstubAllGlobals())

describe('GeneratePage actions block', () => {
  it('1: generate button and count input are in .gen-actions inside .gen-controls, not in .gen-sections', async () => {
    const wrapper = await page()
    const actions = wrapper.find('.gen-controls .gen-actions')
    expect(actions.exists(), '.gen-actions inside .gen-controls').toBe(true)
    expect(inside(actions.element, '.gen-sections')).toBe(false)
    for (const sel of ['.btn-generate', '.gen-count']) {
      const el = wrapper.find(sel)
      expect(el.exists(), sel).toBe(true)
      expect(inside(el.element, '.gen-actions'), `${sel} in .gen-actions`).toBe(true)
      expect(inside(el.element, '.gen-sections'), `${sel} not in .gen-sections`).toBe(false)
    }
  })

  it('2: after opening section 02 the generate button is rendered and not inside a collapsed card', async () => {
    const wrapper = await page()
    const toggle = wrapper.findAll('.card-toggle').find(h => h.find('.card-num').text().trim() === '02')
    expect(toggle, 'section 02 header').toBeTruthy()
    await toggle.trigger('click')
    await settle()
    const sec01 = wrapper.findAll('.card-toggle').find(h => h.find('.card-num').text().trim() === '01')
    expect(inside(sec01.element, '.collapsed'), 'section 01 collapsed after opening 02').toBe(true)
    const btn = wrapper.find('.btn-generate')
    expect(btn.exists()).toBe(true)
    expect(inside(btn.element, '.collapsed')).toBe(false)
    const count = wrapper.find('.gen-count')
    expect(count.exists()).toBe(true)
    expect(inside(count.element, '.collapsed')).toBe(false)
  })

  it('3: save idea and saved ideas buttons stay in section 01 inside .gen-sections, next to the textarea', async () => {
    const wrapper = await page()
    for (const key of ['generate.btn_save', 'generate.saved_ideas']) {
      const btn = buttonByText(wrapper, key)
      expect(btn, key).toBeTruthy()
      expect(inside(btn.element, '.gen-sections'), `${key} in .gen-sections`).toBe(true)
      expect(inside(btn.element, '.gen-actions'), `${key} not in .gen-actions`).toBe(false)
      const card = btn.element.closest('.gen-sections > *')
      expect(card, `${key} card`).toBeTruthy()
      expect(card.querySelector('.card-num').textContent.trim()).toBe('01')
      expect(card.querySelector('textarea'), `${key} card has textarea`).toBeTruthy()
    }
  })
})
