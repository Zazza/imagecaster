import { mockInstalledEditor } from './helpers/post-editor.js'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, EnqueueFromImage,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'
import { effectNameKey } from '../effectNames.js'

enableAutoUnmount(afterEach)

const MORE_FILE = resolve(process.cwd(), '..', 'data/presets/effects-more.json')
const MATERIAL = 'post.group_effect_material'
const LIGHT = 'post.group_effect_light'
const WEATHER = 'post.group_effect_weather'
const PATTERN = 'post.group_effect_pattern'
const HAIR = 'post.group_effect_hair'
const BACKGROUND = 'post.group_effect_background'
const HINT = 'post.effect_pattern_mask_hint'

const TYPES = [
  { id: 1, name: 'post' },
  { id: 2, name: 'post_material' },
  { id: 3, name: 'post_skin' },
  { id: 4, name: 'post_light' },
  { id: 5, name: 'post_film' },
  { id: 6, name: 'post_transform' },
  { id: 7, name: 'post_weather' },
  { id: 8, name: 'post_pattern' },
  { id: 9, name: 'post_hair' },
  { id: 10, name: 'post_background' },
]
const PRESETS = [
  { id: 20, name: 'Leather Qx', preset_type: 'post_material', type_id: 2, denoising_strength: null },
  { id: 40, name: 'Golden Hour Qx', preset_type: 'post_light', type_id: 4, denoising_strength: null },
  { id: 70, name: 'Fog Qx', preset_type: 'post_weather', type_id: 7, denoising_strength: null },
  { id: 80, name: 'Polka Qx', preset_type: 'post_pattern', type_id: 8, denoising_strength: null },
  { id: 81, name: 'Florals Qx', preset_type: 'post_pattern', type_id: 8, denoising_strength: null },
  { id: 90, name: 'Curls Qx', preset_type: 'post_hair', type_id: 9, denoising_strength: null },
  { id: 100, name: 'Beach Qx', preset_type: 'post_background', type_id: 10, denoising_strength: null },
]
const GROUP_OF = {
  'Leather Qx': MATERIAL,
  'Golden Hour Qx': LIGHT,
  'Fog Qx': WEATHER,
  'Polka Qx': PATTERN,
  'Florals Qx': PATTERN,
  'Curls Qx': HAIR,
  'Beach Qx': BACKGROUND,
}
const IMG = 'aW1n'

useFakeImage()
useFakeCanvas({ trackPaint: true, layout: true })

beforeEach(() => {
  vi.clearAllMocks()
  mockInstalledEditor()
  clearEventMocks()
  localStorage.clear()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  let jobs = 0
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
})

async function mountPage() {
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

function groupSelect(wrapper) {
  const select = wrapper.find('.fx-group-pick select')
  expect(select.exists()).toBe(true)
  return select
}

function optionValues(wrapper) {
  return groupSelect(wrapper).findAll('option').map(o => o.attributes('value'))
}

async function pickGroup(wrapper, key) {
  await groupSelect(wrapper).setValue(key)
  await settle()
}

function cardNames(wrapper) {
  return wrapper.findAll('.fx-card').filter(c => c.attributes('title')).map(c => c.attributes('title'))
}

async function selectEffect(wrapper, name) {
  await pickGroup(wrapper, GROUP_OF[name])
  const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === name)
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
}

async function apply(wrapper) {
  const btn = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

function lastParams() {
  expect(EnqueueFromImage).toHaveBeenCalled()
  return EnqueueFromImage.mock.calls[EnqueueFromImage.mock.calls.length - 1][0]
}

function keepToggleShown(wrapper) {
  const text = t('post.effect_keep_color')
  const labels = wrapper.findAll('label').filter(l => l.text().includes(text) && l.isVisible())
  const buttons = wrapper.findAll('button').filter(b => b.text().includes(text) && b.isVisible())
  return labels.length + buttons.length > 0
}

function hintShown(wrapper) {
  const text = t(HINT)
  const holders = wrapper.findAll('*').filter(el => {
    if (!el.text().includes(text)) return false
    return !Array.from(el.element.children).some(ch => ch.textContent.includes(text))
  })
  return holders.some(el => el.isVisible())
}

function maskToggle(wrapper) {
  return wrapper.findAll('button').find(b => b.text().includes(t('post.effect_mask_btn')))
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

describe('EM3: ru/en names for new effects and groups', () => {
  const file = JSON.parse(readFileSync(MORE_FILE, 'utf8'))

  it('file has effects', () => {
    expect(Array.isArray(file.presets)).toBe(true)
    expect(file.presets.length).toBeGreaterThan(0)
  })

  it('every effect in effects-more.json has ru and en name', () => {
    const missing = []
    for (const p of file.presets) {
      const key = effectNameKey(p.name)
      if (typeof ru[key] !== 'string' || !ru[key].trim()) missing.push('ru:' + key)
      if (typeof en[key] !== 'string' || !en[key].trim()) missing.push('en:' + key)
    }
    expect(missing).toEqual([])
  })
})

describe('EM5: new effect groups in Post-Process', () => {
  it('dropdown lists new groups after existing ones when DB has their effects', async () => {
    const wrapper = await mountPage()
    expect(optionValues(wrapper)).toEqual([MATERIAL, LIGHT, WEATHER, PATTERN, HAIR, BACKGROUND])
    const options = groupSelect(wrapper).findAll('option')
    for (const [i, key] of [MATERIAL, LIGHT, WEATHER, PATTERN, HAIR, BACKGROUND].entries()) {
      expect(options[i].text()).toContain(t(key))
    }
    expect(options[3].text()).toContain('2')
  })

  it('new groups without effects in DB are not listed', async () => {
    ListPresets.mockReturnValue(Promise.resolve(PRESETS.filter(p => p.preset_type === 'post_material' || p.preset_type === 'post_hair')))
    const wrapper = await mountPage()
    expect(optionValues(wrapper)).toEqual([MATERIAL, HAIR])
  })

  it('choosing the pattern group shows only its cards', async () => {
    const wrapper = await mountPage()
    await pickGroup(wrapper, PATTERN)
    expect(cardNames(wrapper).sort()).toEqual(['Florals Qx', 'Polka Qx'])
  })

  it('pattern effect: keep color toggle hidden and job has no keep_color=true', async () => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, 'Polka Qx')
    expect(keepToggleShown(wrapper)).toBe(false)
    await apply(wrapper)
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastParams().preset_id).toBe(80)
    expect(lastParams().keep_color).not.toBe(true)
  })

  it('switching from material to pattern hides keep color and drops keep_color', async () => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, 'Leather Qx')
    expect(keepToggleShown(wrapper)).toBe(true)
    await selectEffect(wrapper, 'Polka Qx')
    expect(keepToggleShown(wrapper)).toBe(false)
    await apply(wrapper)
    expect(lastParams().keep_color).not.toBe(true)
  })

  it('pattern without mask: hint is shown', async () => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, 'Polka Qx')
    expect(hintShown(wrapper)).toBe(true)
  })

  it('pattern, mask mode on but nothing painted: hint is still shown', async () => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, 'Polka Qx')
    await toggleMask(wrapper)
    expect(hintShown(wrapper)).toBe(true)
  })

  it('pattern with painted mask: hint hidden, job carries the mask', async () => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, 'Polka Qx')
    await toggleMask(wrapper)
    await paint(wrapper)
    expect(hintShown(wrapper)).toBe(false)
    await apply(wrapper)
    expect(lastParams().mask_base64).toBeTruthy()
  })

  it('pattern, mask painted then mask button turned off: hint shown, job has no mask', async () => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, 'Polka Qx')
    await toggleMask(wrapper)
    await paint(wrapper)
    expect(hintShown(wrapper)).toBe(false)
    await toggleMask(wrapper)
    expect(hintShown(wrapper)).toBe(true)
    await apply(wrapper)
    expect(lastParams().mask_base64).toBeFalsy()
  })

  it.each(['Leather Qx', 'Golden Hour Qx', 'Fog Qx', 'Curls Qx', 'Beach Qx'])('non-pattern effect %s without mask: no hint', async name => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, name)
    expect(hintShown(wrapper)).toBe(false)
  })

  it('no effect selected: no hint', async () => {
    const wrapper = await mountPage()
    expect(hintShown(wrapper)).toBe(false)
  })

  it('switching from pattern to another effect hides the hint', async () => {
    const wrapper = await mountPage()
    await selectEffect(wrapper, 'Polka Qx')
    expect(hintShown(wrapper)).toBe(true)
    await selectEffect(wrapper, 'Curls Qx')
    expect(hintShown(wrapper)).toBe(false)
  })
})
