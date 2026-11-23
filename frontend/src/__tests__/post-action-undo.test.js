import { mockInstalledEditor } from './helpers/post-editor.js'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, EnqueueFromImage,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PRESETS = [{ id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null }]
const IMG = 'aW1n'

useFakeImage()
useFakeCanvas({ trackPaint: true, layout: true })

beforeEach(() => {
  vi.clearAllMocks()
  mockInstalledEditor()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
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

function buttonWith(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().includes(text))
}

function titledButton(wrapper, title) {
  return wrapper.findAll('button').find(b => b.attributes('title') === title)
}

function maskToggle(wrapper) {
  return buttonWith(wrapper, t('post.effect_mask_btn'))
}

async function toggleMask(wrapper) {
  const btn = maskToggle(wrapper)
  expect(btn).toBeTruthy()
  await btn.trigger('click')
  await settle()
  const mode = wrapper.find('[data-testid="object-mask-mode"]')
  if (mode.exists()) { await mode.setValue('exact'); await settle() }
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

function shownImage(wrapper) {
  return wrapper.get('.split-after').attributes('src').replace(/^data:[^,]*,/, '')
}

function expectHistory(wrapper, undo, redo) {
  expect(titledButton(wrapper, t('post.undo')).element.disabled).toBe(!undo)
  expect(titledButton(wrapper, t('post.redo')).element.disabled).toBe(!redo)
}

const RESULT = 'UkVT'

async function completedEffect(wrapper) {
  await applyEffect(wrapper)
  EventsEmit('queue:completed', { job_id: 'job-1', result: JSON.stringify({ image_base64: RESULT }) })
  await settle()
  expect(shownImage(wrapper)).toBe(RESULT)
}

async function shortcut(key, modifiers = {}) {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, ctrlKey: true, bubbles: true, ...modifiers }))
  await settle()
}

describe('Postprocessing operation history with an active selection', () => {
  it('painting alone leaves global Undo and Redo disabled', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    expectHistory(wrapper, false, false)
    expect(shownImage(wrapper)).toBe(IMG)
    expect(wrapper.get('[data-testid="mask-undo"]').element.disabled).toBe(false)
    await shortcut('z')
    expectHistory(wrapper, false, false)
    expect(wrapper.get('[data-testid="mask-undo"]').element.disabled).toBe(false)
  })

  it('toolbar Undo and Redo restore photos while selection mode remains active', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await completedEffect(wrapper)
    expectHistory(wrapper, true, false)
    await titledButton(wrapper, t('post.undo')).trigger('click')
    await settle()
    expect(shownImage(wrapper)).toBe(IMG)
    expectHistory(wrapper, false, true)
    expect(wrapper.get('.split-area').classes()).toContain('masking')
    await titledButton(wrapper, t('post.redo')).trigger('click')
    await settle()
    expect(shownImage(wrapper)).toBe(RESULT)
    expectHistory(wrapper, true, false)
  })

  it.each([
    ['Ctrl+Z/Y', {}, 'y', {}],
    ['Ctrl+Z/Shift+Z', {}, 'z', { shiftKey: true }],
    ['Cmd+Z/Shift+Z', { ctrlKey: false, metaKey: true }, 'z', { ctrlKey: false, metaKey: true, shiftKey: true }],
  ])('%s controls photos with an active selection', async (_label, undoModifiers, redoKey, redoModifiers) => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await completedEffect(wrapper)
    await shortcut('z', undoModifiers)
    expect(shownImage(wrapper)).toBe(IMG)
    expectHistory(wrapper, false, true)
    await shortcut(redoKey, redoModifiers)
    expect(shownImage(wrapper)).toBe(RESULT)
    expectHistory(wrapper, true, false)
  })

  it('separate labeled selection controls preserve photo and operation redo history', async () => {
    const wrapper = await mountSingle()
    await toggleMask(wrapper)
    await paint(wrapper)
    await completedEffect(wrapper)
    await titledButton(wrapper, t('post.undo')).trigger('click')
    await settle()
    const maskUndo = wrapper.get('[data-testid="mask-undo"]')
    const maskRedo = wrapper.get('[data-testid="mask-redo"]')
    expect(maskUndo.text()).toContain(t('post.mask_undo'))
    expect(maskRedo.text()).toContain(t('post.mask_redo'))
    await maskUndo.trigger('click')
    await settle()
    expect(shownImage(wrapper)).toBe(IMG)
    expectHistory(wrapper, false, true)
    expect(maskUndo.element.disabled).toBe(true)
    expect(maskRedo.element.disabled).toBe(false)
    await maskRedo.trigger('click')
    await settle()
    expect(shownImage(wrapper)).toBe(IMG)
    expectHistory(wrapper, false, true)
    expect(maskUndo.element.disabled).toBe(false)
    await titledButton(wrapper, t('post.redo')).trigger('click')
    await settle()
    expect(shownImage(wrapper)).toBe(RESULT)
  })
})
