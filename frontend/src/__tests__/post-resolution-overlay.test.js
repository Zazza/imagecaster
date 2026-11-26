import { mockInstalledEditor } from './helpers/post-editor.js'
import { beforeEach, afterEach, it, expect, vi } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import PostProcessPage from '../components/PostProcessPage.vue'
import ResolutionUpscaleOverlay from '../components/ResolutionUpscaleOverlay.vue'
import { GetSettings, GetActiveSessionItem, GetSessionImage } from '../wailsjs/go/main/App.js'
import { clearEventMocks } from './mocks/runtime'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)
useFakeImage()
useFakeCanvas()
const SOURCE_ITEM = 3
beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  mockInstalledEditor()
  GetSettings.mockResolvedValue({})
  GetActiveSessionItem.mockResolvedValue({ id: SOURCE_ITEM })
  GetSessionImage.mockResolvedValue('U1JDQQ==')
})
it('keeps resolution controls in Format instead of floating over the Effects preview', async () => {
  const w = mount(PostProcessPage, { props: { initialSection: 'effect' } })
  await settle()
  const controls = w.findComponent(ResolutionUpscaleOverlay)
  expect(controls.exists()).toBe(true)
  expect(controls.element.closest('.card').querySelector('.card-num').textContent).toBe('09')
  expect(w.find('.post-preview-card .resolution-upscale').exists()).toBe(false)
  const header = w.findAll('.card-header.card-toggle').find(h => h.text().includes(t('post.finish_title')))
  await header.trigger('click')
  await settle()
  expect(controls.isVisible()).toBe(true)
  expect(controls.props('inline')).toBe(true)
})
