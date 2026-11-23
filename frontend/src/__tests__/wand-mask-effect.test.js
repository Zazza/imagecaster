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
function apply(wrapper) { return wrapper.findAll('.fx-apply-bar button').find(button => button.text().includes(t('post.btn_apply_effect')) || button.text().includes(t('post.btn_apply_effect_all', { n: 2 }))) }
async function enableMask(wrapper, mode = 'wand') {
  await wrapper.findAll('button').find(button => button.text().includes(t('post.effect_mask_btn'))).trigger('click')
  await settle()
  await wrapper.find('[data-testid="object-mask-mode"]').setValue(mode)
  await settle()
}
async function clickPhoto(wrapper, shiftKey = false) {
  const area = wrapper.find('.split-area')
  const event = { clientX: 20, clientY: 20, button: 0, buttons: 1, pointerId: 1, shiftKey }
  await area.trigger('pointerdown',event)
  await area.trigger('pointerup',{ ...event, buttons: 0 })
  await settle()
}
describe('wand selection in effect application', () => {
  it('offers the third mode and selects immediately with one positive point per Shift click', async () => {
    const first = prediction(false), second = prediction(false)
    first.data[1] = 255; second.data[7] = 255
    predict.mockResolvedValueOnce(first).mockResolvedValueOnce(second)
    const wrapper = await prepare(); await enableMask(wrapper)
    const values = wrapper.find('[data-testid="object-mask-mode"]').findAll('option').map(option => option.element.value)
    expect(values).toContain('object'); expect(values).toContain('exact'); expect(values).toContain('wand')
    expect(wrapper.find('[data-testid="object-mask-detect"]').exists()).toBe(false)
    expect(apply(wrapper).element.disabled).toBe(true)
    await clickPhoto(wrapper)
    expect(predict).toHaveBeenCalledTimes(1)
    expect(predict.mock.calls[0][1]).toEqual([{ x: expect.any(Number), y: expect.any(Number), label: 1 }])
    await clickPhoto(wrapper, true)
    expect(predict).toHaveBeenCalledTimes(2)
    expect(predict.mock.calls[1][1]).toHaveLength(1)
    const rendered = HTMLCanvasElement.prototype.getContext.mock.results.flatMap(result => result.value?.putImageData?.mock?.calls || []).map(call => call[0]?.data).filter(Boolean)
    expect(rendered.some(data => data[1 * 4 + 3] > 0 && data[7 * 4 + 3] > 0 && data[4 * 4 + 3] === 0)).toBe(true)
    expect(apply(wrapper).element.disabled).toBe(false)
    await apply(wrapper).trigger('click'); await settle()
    expect(EnqueueFromImage.mock.calls[0][0]).toMatchObject({ mask_base64: 'TUFTSw==', mask_strict: true })
  })
  it('does not infer on pointer movement or a second pointer sample while dragging', async () => {
    const wrapper = await prepare(); await enableMask(wrapper)
    const area = wrapper.find('.split-area')
    const event = { clientX: 20, clientY: 20, button: 0, buttons: 1, pointerId: 1 }
    await area.trigger('pointerdown', event)
    await area.trigger('pointermove', { ...event, clientX: 30 })
    await area.trigger('pointermove', { ...event, clientX: 35 })
    await area.trigger('pointerup', { ...event, clientX: 35, buttons: 0 }); await settle()
    expect(predict).toHaveBeenCalledTimes(1)
  })
  it('ignores clicks outside the photo', async () => {
    const wrapper = await prepare(); await enableMask(wrapper)
    const area = wrapper.find('.split-area')
    const event = { clientX: FAKE_IMAGE_WIDTH + 10, clientY: 20, button: 0, buttons: 1, pointerId: 1 }
    await area.trigger('pointerdown', event)
    await area.trigger('pointerup', { ...event, buttons: 0 }); await settle()
    expect(predict).not.toHaveBeenCalled(); expect(apply(wrapper).element.disabled).toBe(true)
  })
  it('blocks pending selection, then discards it when the user switches mode', async () => {
    const pending = deferred(); predict.mockReturnValue(pending.promise)
    const wrapper = await prepare(); await enableMask(wrapper); await clickPhoto(wrapper)
    expect(predict).toHaveBeenCalledTimes(1)
    expect(apply(wrapper).element.disabled).toBe(true)
    await apply(wrapper).trigger('click'); expect(EnqueueFromImage).not.toHaveBeenCalled()
    await wrapper.find('[data-testid="object-mask-mode"]').setValue('exact'); await settle()
    pending.resolve(prediction(true)); await settle()
    await wrapper.find('[data-testid="object-mask-mode"]').setValue('wand'); await settle()
    expect(apply(wrapper).element.disabled).toBe(true)
  })
  it.each(['empty', 'error'])('does not enqueue after an %s prediction', async kind => {
    if (kind === 'empty') predict.mockResolvedValue(prediction(false))
    else predict.mockRejectedValue(new Error('decoder failed'))
    const wrapper = await prepare(); await enableMask(wrapper); await clickPhoto(wrapper)
    expect(predict).toHaveBeenCalledTimes(1); expect(apply(wrapper).element.disabled).toBe(true)
    await apply(wrapper).trigger('click'); expect(EnqueueFromImage).not.toHaveBeenCalled()
  })
})
