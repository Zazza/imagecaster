import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import UnifiedGeneratePage from '../components/UnifiedGeneratePage.vue'
import { readImageFile } from '../imageFile.js'
import { MAX_IMAGE_SIZE } from '../constants.js'
import { t } from '../i18n/index.js'
import { settle, waitUntil } from './helpers/async.js'

enableAutoUnmount(afterEach)

function fileOf(type, size, name = 'photo.png') {
  const file = new File([new Uint8Array([1, 2, 3, 4])], name, { type })
  if (size !== undefined) Object.defineProperty(file, 'size', { value: size })
  return file
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
})

afterEach(() => vi.unstubAllGlobals())

describe('readImageFile', () => {
  it('resolves base64 without the data URL prefix', async () => {
    expect(await readImageFile(fileOf('image/png'))).toBe('AQIDBA==')
  })

  it('accepts a file of exactly the size limit', async () => {
    await expect(readImageFile(fileOf('image/png', MAX_IMAGE_SIZE))).resolves.toBeTruthy()
  })

  it('rejects a file over the size limit with the size message', async () => {
    await expect(readImageFile(fileOf('image/png', MAX_IMAGE_SIZE + 1))).rejects.toThrow(t('fi.error_image_too_large'))
  })

  it('rejects a non-image with the drop message', async () => {
    await expect(readImageFile(fileOf('text/plain', 10, 'a.txt'))).rejects.toThrow(t('fi.error_drop_image'))
  })
})

async function dropOnUnified(file) {
  const w = mount(UnifiedGeneratePage, { attachTo: document.body })
  await settle()
  await w.find('.page-enter').trigger('drop', { dataTransfer: { files: [file], types: ['Files'] } })
  await settle()
  return w
}

describe('UnifiedGeneratePage drop', () => {
  it('an image goes to post-processing', async () => {
    const w = await dropOnUnified(fileOf('image/png'))
    await waitUntil(() => w.emitted('navigate'))
    expect(w.emitted('navigate')[0]).toEqual([{ page: 'postprocess', section: 'remix', image: 'AQIDBA==' }])
  })

  it('a non-image shows the drop error', async () => {
    const w = await dropOnUnified(fileOf('text/plain', 10, 'a.txt'))
    await waitUntil(() => w.text().includes(t('fi.error_drop_image')))
    expect(w.emitted('navigate')).toBeUndefined()
  })

  it('an image over the size limit shows the size error', async () => {
    const w = await dropOnUnified(fileOf('image/png', MAX_IMAGE_SIZE + 1))
    await waitUntil(() => w.text().includes(t('fi.error_image_too_large')))
    expect(w.emitted('navigate')).toBeUndefined()
  })
})
