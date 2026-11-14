import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, EnqueueFromImage, ReadFileAsBase64, BatchSaveImage, BatchReset,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]
const IMG = 'aW1n'
const IMG_A = 'QUFBQQ=='
const IMG_B = 'QkJCQg=='

useFakeImage()
useFakeCanvas({ trackPaint: true, layout: true })

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  const files = { '/photos/a.png': IMG_A, '/photos/b.png': IMG_B }
  ReadFileAsBase64.mockImplementation(p => Promise.resolve(files[p] || ''))
  BatchSaveImage.mockImplementation(() => Promise.resolve('/batch/v.png'))
  BatchReset.mockImplementation(() => Promise.resolve())
  let jobs = 0
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
})

async function mountSingle() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

async function mountBatch() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  useRemixQueue().setPostQueue([{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }])
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  return wrapper
}

function buttonWith(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().includes(text))
}

function titledButton(wrapper, title) {
  return wrapper.findAll('button').find(b => b.attributes('title') === title)
}

function maskToggle(wrapper) {
  return buttonWith(wrapper, t('post.effect_mask_btn'))
}

function effectCard(wrapper) {
  const btn = maskToggle(wrapper)
  expect(btn).toBeTruthy()
  return wrapper.findAll('.card').filter(c => c.element.contains(btn.element)).pop()
}

function hasBrushLabel(wrapper) {
  const text = effectCard(wrapper).text()
  const prefixes = ['fi.label_brush_size', 'fi.brush'].map(k => t(k, { value: '\u0000' }).split('\u0000')[0].trim())
  return prefixes.some(p => p && text.includes(p))
}

function clearMaskBtn(wrapper) {
  return effectCard(wrapper).findAll('button').find(b => b.text().includes(t('fi.btn_clear_mask')))
}

async function toggleMask(wrapper) {
  const btn = maskToggle(wrapper)
  expect(btn).toBeTruthy()
  await btn.trigger('click')
  await settle()
}

async function paint(wrapper) {
  const area = wrapper.find('.split-area')
  expect(area.exists()).toBe(true)
  const opts = { clientX: 20, clientY: 20, button: 0, buttons: 1, pointerId: 1 }
  await area.trigger('pointerdown', opts)
  await area.trigger('pointermove', { ...opts, clientX: 30, clientY: 25 })
  await area.trigger('pointermove', { ...opts, clientX: 40, clientY: 30 })
  await area.trigger('pointerup', { ...opts, clientX: 40, clientY: 30, buttons: 0 })
  await settle()
}

async function applyEffect(wrapper) {
  const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
  const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
  expect(apply).toBeTruthy()
  expect(apply.element.disabled).toBe(false)
  await apply.trigger('click')
  await settle()
}

function lastEnqueueParams() {
  expect(EnqueueFromImage).toHaveBeenCalled()
  return EnqueueFromImage.mock.calls[EnqueueFromImage.mock.calls.length - 1][0]
}

describe('Effect mask in post-process', () => {
  it('single image: mask toggle is shown in the effects section', async () => {
    const wrapper = await mountSingle()
    expect(maskToggle(wrapper)).toBeTruthy()
  })

  it('toggle on: brush size and clear mask controls appear; toggle off hides them', async () => {
    const wrapper = await mountSingle()
    expect(clearMaskBtn(wrapper)).toBeFalsy()
    expect(hasBrushLabel(wrapper)).toBe(false)
    await toggleMask(wrapper)
    expect(clearMaskBtn(wrapper)).toBeTruthy()
    expect(hasBrushLabel(wrapper)).toBe(true)
    expect(wrapper.find('.split-area').classes()).toContain('masking')
    await toggleMask(wrapper)
    expect(clearMaskBtn(wrapper)).toBeFalsy()
    expect(hasBrushLabel(wrapper)).toBe(false)
    expect(wrapper.find('.split-area').classes()).not.toContain('masking')
  })

  it('toggle off: apply sends no mask_base64', async () => {
    const wrapper = await mountSingle()
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastEnqueueParams().mask_base64).toBeFalsy()
  })

  it('toggle on but nothing painted: apply sends no mask_base64', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastEnqueueParams().mask_base64).toBeFalsy()
  })

  it('toggle on and painted: apply sends mask_base64', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    const p = lastEnqueueParams()
    expect(typeof p.mask_base64).toBe('string')
    expect(p.mask_base64.length).toBeGreaterThan(0)
  })

  it('mask survives applying the effect: second apply on the new image sends the mask again', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    const first = lastEnqueueParams().mask_base64
    expect(first).toBeTruthy()
    EventsEmit('queue:completed', { job_id: 'job-1', result: JSON.stringify({ image_base64: 'UkVT' }) })
    await settle()
    expect(maskToggle(wrapper)).toBeTruthy()
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    const second = EnqueueFromImage.mock.calls[1][0]
    expect(second.image_base64.replace(/^data:[^,]*,/, '')).toBe('UkVT')
    expect(second.mask_base64).toBeTruthy()
  })

  it('EM6: opening another photo of the same size clears the mask, toggle stays on', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await wrapper.setProps({ initialImage: 'aW1nMg==' })
    await settle()
    expect(clearMaskBtn(wrapper)).toBeTruthy()
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    const p = lastEnqueueParams()
    expect(p.image_base64.replace(/^data:[^,]*,/, '')).toBe('aW1nMg==')
    expect(p.mask_base64).toBeFalsy()
  })

  it('EM8: undo of the only mask stroke empties the mask, no mask_base64', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    const undo = titledButton(wrapper, t('post.undo'))
    expect(undo).toBeTruthy()
    expect(undo.element.disabled).toBe(false)
    await undo.trigger('click')
    await settle()
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastEnqueueParams().mask_base64).toBeFalsy()
  })

  it('EM8: undo then redo of the mask stroke brings the mask back', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await titledButton(wrapper, t('post.undo')).trigger('click')
    await settle()
    const redo = titledButton(wrapper, t('post.redo'))
    expect(redo).toBeTruthy()
    expect(redo.element.disabled).toBe(false)
    await redo.trigger('click')
    await settle()
    await applyEffect(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastEnqueueParams().mask_base64).toBeTruthy()
  })

  it('painted, then toggle off: apply sends no mask_base64', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await toggleMask(wrapper)
    await applyEffect(wrapper)
    expect(lastEnqueueParams().mask_base64).toBeFalsy()
  })

  it('painted, then cleared: apply sends no mask_base64', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await clearMaskBtn(wrapper).trigger('click')
    await settle()
    await applyEffect(wrapper)
    expect(lastEnqueueParams().mask_base64).toBeFalsy()
  })

  it('batch mode: no mask toggle, batch jobs carry no mask_base64', async () => {
    const wrapper = await mountBatch()
    expect(wrapper.find('.batch-panel').exists()).toBe(true)
    expect(maskToggle(wrapper)).toBeFalsy()
    const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === 'Soft Focus')
    await card.trigger('click')
    await settle()
    const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect_all', { n: 2 })))
    expect(apply).toBeTruthy()
    await apply.trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    for (const call of EnqueueFromImage.mock.calls) {
      expect(call[0].mask_base64).toBeFalsy()
    }
  })
})
