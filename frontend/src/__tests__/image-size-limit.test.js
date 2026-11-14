import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import { GetSettings, GetActiveSessionItem, GetSessionImage } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const MB = 1024 * 1024

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
  return (document.body.textContent || '').includes(t('fi.error_image_too_large'))
}

describe('image size limit', () => {
  beforeEach(() => { vi.clearAllMocks(); clearEventMocks(); document.body.innerHTML = '' })

  it('dropping a 20 MB file in postprocess does not show a size error', async () => {
    const wrapper = await mountPage()
    await dropFile(wrapper, pngFileOfSize(20 * MB))
    expect(sizeErrorShown()).toBe(false)
  })

  it('dropping a file of exactly 128 MB does not show a size error', async () => {
    const wrapper = await mountPage()
    await dropFile(wrapper, pngFileOfSize(128 * MB))
    expect(sizeErrorShown()).toBe(false)
  })

  it('dropping a file over 128 MB shows the size error', async () => {
    const wrapper = await mountPage()
    await dropFile(wrapper, pngFileOfSize(128 * MB + 1))
    expect(sizeErrorShown()).toBe(true)
  })
})
