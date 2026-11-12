import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base }
})

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, UpdateSettings, GetSDLoRAs, GetActiveSessionItem, GetSessionImage, ListPresets,
  ListPresetTypes, ListHiresProfiles, EnqueueFromImage,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PLAIN = { id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null, loras: '' }
const WITH_LORA = { id: 11, name: 'Lora Look', preset_type: 'post', type_id: 1, denoising_strength: null, loras: '[{"name":"PresetL","weight":1}]' }
const IMG = 'aW1n'

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

class FakeImage {
  constructor() {
    this.onload = null
    this.onerror = null
    this.naturalWidth = 64
    this.naturalHeight = 48
    this.width = 64
    this.height = 48
    this.complete = false
    this._src = ''
  }
  set src(v) {
    this._src = v
    setTimeout(() => {
      this.complete = true
      if (this.onload) this.onload()
    }, 0)
  }
  get src() {
    return this._src
  }
  decode() {
    return Promise.resolve()
  }
}

const OrigImage = globalThis.Image

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  let jobs = 0
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
  GetSettings.mockReturnValue(Promise.resolve({}))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([
    { name: 'LoraA', path: '/loras/LoraA.safetensors' },
    { name: 'LoraB', path: '/loras/LoraB.safetensors' },
  ]))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve([PLAIN, WITH_LORA]))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  globalThis.Image = FakeImage
  window.Image = FakeImage
})

afterEach(() => {
  vi.restoreAllMocks()
  globalThis.Image = OrigImage
  window.Image = OrigImage
})

async function mountPage() {
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

async function openModel(wrapper) {
  const toggle = wrapper.find('.fx-model .fold-head')
  expect(toggle.exists()).toBe(true)
  await toggle.trigger('click')
  await settle()
  expect(wrapper.find('.fx-model').classes()).toContain('fold-open')
}

function loraRows(wrapper) {
  return wrapper.findAll('.fx-model-body .fx-lora-row')
}

async function addLora(wrapper, name, weight) {
  const before = loraRows(wrapper).length
  const add = wrapper.find('.fx-model-body .fx-lora-add')
  expect(add.exists()).toBe(true)
  await add.trigger('click')
  await settle()
  const rows = loraRows(wrapper)
  expect(rows.length).toBe(before + 1)
  const row = rows[rows.length - 1]
  await row.find('select.fx-lora-name').setValue(name)
  await settle()
  await row.find('input.fx-lora-weight').setValue(String(weight))
  await row.find('input.fx-lora-weight').trigger('change')
  await settle()
}

async function applyEffect(wrapper, presetName = 'Soft Focus') {
  const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === presetName)
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
  const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
  expect(apply).toBeTruthy()
  expect(apply.element.disabled).toBe(false)
  await apply.trigger('click')
  await settle()
}

function lastParams() {
  expect(EnqueueFromImage).toHaveBeenCalled()
  return EnqueueFromImage.mock.calls[EnqueueFromImage.mock.calls.length - 1][0]
}

function sentLoras(params) {
  expect(params.overrides).toBeTruthy()
  expect(typeof params.overrides.loras).toBe('string')
  return JSON.parse(params.overrides.loras)
}

function settingsCallsWith(key) {
  return UpdateSettings.mock.calls.map(c => c[0]).filter(arg => arg && Object.prototype.hasOwnProperty.call(arg, key))
}

describe('Effect model block in post-process', () => {
  it('T6: block is collapsed by default, model is Klein, apply sends no overrides', async () => {
    const wrapper = await mountPage()
    expect(wrapper.find('.fx-model .fold-head').exists()).toBe(true)
    expect(wrapper.find('.fx-model').classes()).not.toContain('fold-open')

    await applyEffect(wrapper)
    const p = lastParams()
    expect(p.preset_id).toBe(10)
    expect(p.overrides == null).toBe(true)
  })

  it('T6: opened block has no model select and no Moss, only Klein LoRA rows', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    const body = wrapper.find('.fx-model-body')
    expect(body.find('.fx-model-select').exists()).toBe(false)
    expect(body.findAll('option').map(o => o.attributes('value'))).not.toContain('moss')
    expect(wrapper.find('.fx-model').text()).not.toContain('Moss')
    expect(wrapper.find('.fx-model').text()).toContain('Klein')
    expect(loraRows(wrapper).length).toBe(0)
  })

  it('T6: toggle collapses the opened block again', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    await wrapper.find('.fx-model .fold-head').trigger('click')
    await settle()
    expect(wrapper.find('.fx-model').classes()).not.toContain('fold-open')
  })

  it('T6b: LoRA name choices come from SD LoRA list', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    await wrapper.find('.fx-model-body .fx-lora-add').trigger('click')
    await settle()
    const names = loraRows(wrapper)[0].find('select.fx-lora-name').findAll('option').map(o => o.attributes('value'))
    expect(names).toEqual(expect.arrayContaining(['LoraA', 'LoraB']))
    expect(wrapper.find('.fx-model-body input.fx-lora-weight').attributes('type')).toBe('number')
  })

  it('T6b: Klein + LoraA 0.7 → overrides.loras has LoraA, model_name not set', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    await addLora(wrapper, 'LoraA', 0.7)
    await applyEffect(wrapper)
    const p = lastParams()
    expect(sentLoras(p)).toContainEqual({ name: 'LoraA', weight: 0.7 })
    expect(p.overrides.model_name).toBeFalsy()
  })

  it('T6b: Klein + LoraA on a preset with its own LoRA → both sent', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    await addLora(wrapper, 'LoraA', 0.7)
    await applyEffect(wrapper, 'Lora Look')
    const p = lastParams()
    expect(p.preset_id).toBe(11)
    const loras = sentLoras(p)
    expect(loras).toContainEqual({ name: 'PresetL', weight: 1 })
    expect(loras).toContainEqual({ name: 'LoraA', weight: 0.7 })
    expect(p.overrides.model_name).toBeFalsy()
  })

  it('T6b: removed LoRA row is not sent', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    await addLora(wrapper, 'LoraA', 0.7)
    await loraRows(wrapper)[0].find('.fx-lora-remove').trigger('click')
    await settle()
    expect(loraRows(wrapper).length).toBe(0)
    await applyEffect(wrapper)
    const p = lastParams()
    expect(p.overrides == null).toBe(true)
  })

  it('T7: Klein LoRA is saved to effect_loras_klein', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    await addLora(wrapper, 'LoraA', 0.7)
    const saves = settingsCallsWith('effect_loras_klein')
    expect(saves.length).toBeGreaterThan(0)
    expect(JSON.parse(saves[saves.length - 1].effect_loras_klein)).toContainEqual({ name: 'LoraA', weight: 0.7 })
    expect(settingsCallsWith('effect_loras_moss').length).toBe(0)
    expect(settingsCallsWith('effect_model').length).toBe(0)
  })

  it('T7b: saved effect_model=moss is treated as Klein: no model override, Moss LoRAs not sent', async () => {
    GetSettings.mockReturnValue(Promise.resolve({
      effect_model: 'moss',
      effect_loras_moss: '[{"name":"LoraB","weight":0.5}]',
    }))
    const wrapper = await mountPage()
    expect(wrapper.find('.fx-model').text()).not.toContain('Moss')
    await applyEffect(wrapper)
    const p = lastParams()
    expect(p.overrides == null || !p.overrides.model_name).toBe(true)
    expect(p.overrides == null || !(p.overrides.loras || '').includes('LoraB')).toBe(true)
  })

  it('T7b: saved effect_model=moss keeps the saved Klein LoRA list', async () => {
    GetSettings.mockReturnValue(Promise.resolve({
      effect_model: 'moss',
      effect_loras_klein: '[{"name":"LoraA","weight":0.7}]',
      effect_loras_moss: '[{"name":"LoraB","weight":0.5}]',
    }))
    const wrapper = await mountPage()
    await applyEffect(wrapper)
    const p = lastParams()
    const loras = sentLoras(p)
    expect(loras).toContainEqual({ name: 'LoraA', weight: 0.7 })
    expect(loras.map(l => l.name)).not.toContain('LoraB')
    expect(p.overrides.model_name).toBeFalsy()
  })

  it('T7b: saved Klein LoRA list from settings is used on apply', async () => {
    GetSettings.mockReturnValue(Promise.resolve({
      effect_model: 'klein',
      effect_loras_klein: '[{"name":"LoraA","weight":0.7}]',
    }))
    const wrapper = await mountPage()
    await applyEffect(wrapper)
    const p = lastParams()
    expect(sentLoras(p)).toContainEqual({ name: 'LoraA', weight: 0.7 })
    expect(p.overrides.model_name).toBeFalsy()
  })
})

describe('Effect model LoRA weight edge cases', () => {
  it('empty weight is sent as 1 and keeps the preset LoRA', async () => {
    const wrapper = await mountPage()
    await openModel(wrapper)
    await addLora(wrapper, 'LoraA', '')
    await applyEffect(wrapper, 'Lora Look')
    const loras = sentLoras(lastParams())
    expect(loras).toContainEqual({ name: 'LoraA', weight: 1 })
    expect(loras.some(l => l.name === 'PresetL')).toBe(true)
    for (const l of loras) expect(typeof l.weight).toBe('number')
  })

  it('weight 0 saved in settings stays 0', async () => {
    GetSettings.mockReturnValue(Promise.resolve({ effect_loras_klein: '[{"name":"LoraA","weight":0}]' }))
    const wrapper = await mountPage()
    await applyEffect(wrapper)
    expect(sentLoras(lastParams())).toContainEqual({ name: 'LoraA', weight: 0 })
  })
})
