import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import PostProcessPage from '../components/PostProcessPage.vue'
import { GetSettings, GetActiveSessionItem, GetSessionImage } from '../wailsjs/go/main/App.js'
import { MAX_IMAGE_SIZE } from '../constants.js'
import { en } from '../i18n/en.js'
import { ru } from '../i18n/ru.js'

enableAutoUnmount(afterEach)

const MB = 1024 * 1024
const TOO_LARGE_PATTERNS = [/too large/i, /слишком больш/i, /16 MB/, /16 МБ/, /128 MB/, /128 МБ/]

async function mountPage() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  const wrapper = mount(PostProcessPage, { attachTo: document.body })
  await flushPromises()
  return wrapper
}

function pngFileOfSize(size) {
  const file = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], 'photo.png', { type: 'image/png' })
  Object.defineProperty(file, 'size', { value: size })
  return file
}

async function dropFile(wrapper, file) {
  const zone = wrapper.find('.drop-zone')
  expect(zone.exists()).toBe(true)
  await zone.trigger('drop', { dataTransfer: { files: [file], items: [], types: ['Files'] } })
  await flushPromises()
  await new Promise(r => setTimeout(r, 20))
  await flushPromises()
}

function sizeErrorShown() {
  const text = document.body.textContent || ''
  return TOO_LARGE_PATTERNS.some(re => re.test(text))
}


describe('image size limit', () => {
  beforeEach(() => { vi.clearAllMocks(); clearEventMocks(); document.body.innerHTML = '' })

  it('MAX_IMAGE_SIZE is 128 MB', () => {
    expect(MAX_IMAGE_SIZE).toBe(128 * 1024 * 1024)
  })

  it('i18n too-large message names 128 MB', () => {
    expect(en['fi.error_image_too_large']).toContain('max 128 MB')
    expect(ru['fi.error_image_too_large']).toContain('макс. 128 МБ')
    expect(en['fi.error_image_too_large']).not.toContain('16')
    expect(ru['fi.error_image_too_large']).not.toContain('16')
  })

  it('dropping a 20 MB file in postprocess does not show a size error', async () => {
    const wrapper = await mountPage()
    await dropFile(wrapper, pngFileOfSize(20 * MB))
    expect(sizeErrorShown()).toBe(false)
  })

  it('dropping a file over MAX_IMAGE_SIZE shows the size error', async () => {
    const wrapper = await mountPage()
    await dropFile(wrapper, pngFileOfSize(MAX_IMAGE_SIZE + 1))
    expect(sizeErrorShown()).toBe(true)
  })
})
