import { mockInstalledEditor } from './helpers/post-editor.js'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import PostProcessPage from '../components/PostProcessPage.vue'
import { GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes, ListHiresProfiles, EnqueueFromImage, ReadFileAsBase64, BatchSaveImage, BatchReset } from '../wailsjs/go/main/App.js'
import { EventsEmit, clearEventMocks } from './mocks/runtime'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas, FAKE_IMAGE_WIDTH, FAKE_IMAGE_HEIGHT } from './helpers/dom.js'
import { settle } from './helpers/async.js'

const { predict, dispose } = vi.hoisted(() => ({ predict: vi.fn(), dispose: vi.fn() }))
vi.mock('../objectMaskClient.js', () => ({ createObjectMaskPredictor: () => ({ predict, dispose }) }))
enableAutoUnmount(afterEach)
useFakeImage()
useFakeCanvas({ trackPaint: true, layout: true })
const prediction = filled => ({ width: FAKE_IMAGE_WIDTH, height: FAKE_IMAGE_HEIGHT, data: new Uint8Array(FAKE_IMAGE_WIDTH * FAKE_IMAGE_HEIGHT).fill(filled ? 255 : 0) })
function deferred() { let resolve, reject; const promise = new Promise((yes,no) => { resolve=yes; reject=no }); return { promise, resolve, reject } }
beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  mockInstalledEditor()
  useRemixQueue().clearPostQueue()
  GetSettings.mockResolvedValue({})
  GetActiveSessionItem.mockResolvedValue(null)
  GetSessionImage.mockResolvedValue('')
  ListPresets.mockResolvedValue([{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }])
  ListPresetTypes.mockResolvedValue([{ id: 1, name: 'post' }])
  ListHiresProfiles.mockResolvedValue([])
  EnqueueFromImage.mockResolvedValue('object-job')
  ReadFileAsBase64.mockResolvedValue('aW1n')
  BatchSaveImage.mockResolvedValue('/batch/v.png')
  BatchReset.mockResolvedValue()
  predict.mockResolvedValue(prediction(true))
})
async function prepare(batch = false) {
  if (batch) useRemixQueue().setPostQueue([{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }])
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  if (!batch) { await wrapper.setProps({ initialImage: 'aW1n' }); await settle() }
  await wrapper.findAll('.fx-card').find(card => card.attributes('title') === 'Soft Focus').trigger('click')
  await settle()
  return wrapper
}
async function detect(wrapper) { await wrapper.find('[data-testid="object-mask-detect"]').trigger('click'); await settle() }
function apply(wrapper) { return wrapper.findAll('.fx-apply-bar button').find(button => button.text().includes(t('post.btn_apply_effect')) || button.text().includes(t('post.btn_apply_effect_all', { n: 2 }))) }
async function enableMask(wrapper, mode = 'object') {
  await wrapper.findAll('button').find(button => button.text().includes(t('post.effect_mask_btn'))).trigger('click')
  await settle()
  await wrapper.find('[data-testid="object-mask-mode"]').setValue(mode)
  await settle()
}
async function paint(wrapper) {
  const area = wrapper.find('.split-area')
  const event = { clientX: 20, clientY: 20, button: 0, buttons: 1, pointerId: 1 }
  await area.trigger('pointerdown',event)
  await area.trigger('pointerup',{ ...event, buttons: 0 })
  await settle()
}
describe('effect application with object mask', () => {
  it('blocks apply before a selection and while segmentation is pending', async () => {
    const pending = deferred(); predict.mockReturnValue(pending.promise)
    const wrapper = await prepare()
    await enableMask(wrapper)
    expect(apply(wrapper).element.disabled).toBe(true)
    await paint(wrapper)
    expect(predict).not.toHaveBeenCalled()
    await detect(wrapper)
    expect(predict).toHaveBeenCalled()
    expect(apply(wrapper).element.disabled).toBe(true)
    await apply(wrapper).trigger('click')
    expect(EnqueueFromImage).not.toHaveBeenCalled()
    pending.resolve(prediction(true)); await settle()
    expect(apply(wrapper).element.disabled).toBe(false)
  })
  it.each(['error','empty'])('blocks apply after %s prediction', async kind => {
    if (kind === 'error') predict.mockRejectedValue(new Error('decoder failed'))
    else predict.mockResolvedValue(prediction(false))
    const wrapper = await prepare(); await enableMask(wrapper); await paint(wrapper)
    expect(predict).not.toHaveBeenCalled()
    await detect(wrapper)
    expect(predict).toHaveBeenCalled()
    expect(apply(wrapper).element.disabled).toBe(true)
    await apply(wrapper).trigger('click')
    expect(EnqueueFromImage).not.toHaveBeenCalled()
  })
  it('enqueues the confirmed mask with its original photo and clears it for another same-sized photo', async () => {
    const wrapper = await prepare(); await enableMask(wrapper); await paint(wrapper); await detect(wrapper)
    expect(apply(wrapper).element.disabled).toBe(false)
    await apply(wrapper).trigger('click'); await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(EnqueueFromImage.mock.calls[0][0]).toMatchObject({ image_base64: expect.stringContaining('aW1n'), mask_base64: 'TUFTSw==', mask_strict: true })
    EventsEmit('queue:completed', { job_id: 'object-job', result: JSON.stringify({ image_base64: 'UkVT' }) })
    await settle()
    await wrapper.setProps({ initialImage: 'bmV3' }); await settle()
    expect(apply(wrapper).element.disabled).toBe(true)
  })
  it('preserves exact brush behavior without requiring a prediction', async () => {
    const wrapper = await prepare(); await enableMask(wrapper,'exact'); await paint(wrapper)
    expect(predict).not.toHaveBeenCalled()
    expect(apply(wrapper).element.disabled).toBe(false)
    await apply(wrapper).trigger('click'); await settle()
    expect(EnqueueFromImage.mock.calls[0][0].mask_base64).toBeTruthy()
    expect(EnqueueFromImage.mock.calls[0][0].mask_strict).not.toBe(true)
  })
  it('keeps batch without mask controls or masked jobs', async () => {
    const wrapper = await prepare(true)
    expect(wrapper.find('[data-testid="object-mask-mode"]').exists()).toBe(false)
    await apply(wrapper).trigger('click'); await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    expect(predict).not.toHaveBeenCalled()
    for (const [params] of EnqueueFromImage.mock.calls) { expect(params.mask_base64).toBeFalsy(); expect(params.mask_strict).not.toBe(true) }
  })
})
