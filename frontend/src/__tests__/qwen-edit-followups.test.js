import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, EditImage: vi.fn(() => Promise.resolve(undefined)), SyncPresets: vi.fn(() => Promise.resolve(null)) }
})

import EditSection from '../components/EditSection.vue'
import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, UpdateSettings, ListPresets, EditImage, GetSDLoRAs, GetActiveSessionItem, GetSessionImage,
  ListPresetTypes, ListHiresProfiles, EnqueueFromImage, SyncPresets, GetServerModels,
} from '../wailsjs/go/main/App.js'
import { serverModels, serverModelsKnown } from '../serverModels.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const QWEN = 'qwen_image_2.1-Q8_0'
const KLEIN = 'flux-2-klein-4b'
const MMPROJ = 'mmproj-Qwen3VL-8B-Instruct-F16.gguf'
const SOURCE = 'c291cmNl'
const IMG = 'aW1n'

const KLEIN_STYLE = { id: 2, name: 'Klein Film', model_name: KLEIN, prompt: 'klein tags', loras: '' }
const QWEN_STYLE = { id: 4, name: 'Film (Qwen-Image)', model_name: QWEN, prompt: 'qwen tags', loras: '' }

const TYPES = [{ id: 1, name: 'post' }, { id: 4, name: 'post_light' }]
const KLEIN_FX = { id: 40, name: 'Golden Hour', preset_type: 'post_light', type_id: 4, model_name: KLEIN, denoising_strength: null, loras: '' }
const QWEN_FX = { id: 41, name: 'Golden Hour (Qwen-Image)', preset_type: 'post_light', type_id: 4, model_name: QWEN, denoising_strength: null, loras: '' }

const KLEIN_LORAS = '[{"name":"KleinL","weight":0.5}]'
const QWEN_LORAS = '[{"name":"QwenL","weight":0.8}]'

let textEncoders = null

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

function setTextEncoders(list) {
  textEncoders = list
  if (list === null) {
    serverModels.text_encoder = []
    serverModelsKnown.text_encoder = false
  } else {
    serverModels.text_encoder = list.map(name => ({ name }))
    serverModelsKnown.text_encoder = true
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  useRemixQueue().clearPostQueue()
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
  EditImage.mockImplementation(() => Promise.resolve({ image: 'UkVT' }))
  GetSettings.mockReturnValue(Promise.resolve({}))
  SyncPresets.mockImplementation(() => Promise.resolve(null))
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  let jobs = 0
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  GetServerModels.mockImplementation(type => {
    if (type === 'text_encoder') {
      if (textEncoders === null) return Promise.reject(new Error('unknown'))
      return Promise.resolve(textEncoders.map(name => ({ name })))
    }
    return Promise.resolve([])
  })
  setTextEncoders(null)
  globalThis.Image = FakeImage
  window.Image = FakeImage
})

afterEach(() => {
  globalThis.Image = OrigImage
  window.Image = OrigImage
  setTextEncoders(null)
})

async function mountEdit(settings = {}) {
  ListPresets.mockReturnValue(Promise.resolve([KLEIN_STYLE, QWEN_STYLE]))
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(EditSection, { props: { source: SOURCE, batch: false } })
  await settle()
  return wrapper
}

async function selectModel(wrapper, model) {
  const select = wrapper.find('[data-testid="edit-model-select"]')
  expect(select.exists()).toBe(true)
  await select.setValue(model)
  await settle()
}

function loraNames(wrapper) {
  return wrapper.findAll('[data-testid="edit-lora-name"]').map(i => i.element.value)
}

function generateButton(wrapper) {
  const btn = wrapper.findAll('button').find(b => b.text().includes(t('edit.btn_generate')))
  expect(btn).toBeTruthy()
  return btn
}

async function setInstruction(wrapper) {
  await wrapper.find('[data-testid="edit-instruction"]').setValue('make hair red')
  await settle()
}

async function runEdit(wrapper) {
  await setInstruction(wrapper)
  const btn = generateButton(wrapper)
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

function lastEditParams() {
  expect(EditImage).toHaveBeenCalled()
  return EditImage.mock.calls[EditImage.mock.calls.length - 1][0]
}

function sentLoraNames(params) {
  return (params.loras || []).map(l => l.name)
}

function mergedUpdatesFrom(index) {
  return UpdateSettings.mock.calls.slice(index).reduce((acc, [data]) => ({ ...acc, ...(data || {}) }), {})
}

describe('Edit: LoRA rows are kept per model', () => {
  it('Klein shows rows from edit_loras_klein', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS, edit_loras_qwen: QWEN_LORAS })
    expect(wrapper.find('[data-testid="edit-model-select"]').element.value).toBe('klein')
    expect(loraNames(wrapper)).toEqual(['KleinL'])
  })

  it('switching to Qwen shows rows from edit_loras_qwen', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS, edit_loras_qwen: QWEN_LORAS })
    await selectModel(wrapper, 'qwen')
    expect(loraNames(wrapper)).toEqual(['QwenL'])
  })

  it('switching to Qwen with edit_loras_qwen unset shows no rows', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS })
    await selectModel(wrapper, 'qwen')
    expect(loraNames(wrapper)).toEqual([])
  })

  it('Qwen edit request never carries the Klein LoRAs (Qwen list unset)', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS })
    await selectModel(wrapper, 'qwen')
    await runEdit(wrapper)
    const p = lastEditParams()
    expect(p.model).toBe('qwen')
    expect(sentLoraNames(p)).not.toContain('KleinL')
    expect(p.loras || []).toEqual([])
  })

  it('Qwen edit request carries only the Qwen LoRAs', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS, edit_loras_qwen: QWEN_LORAS })
    await selectModel(wrapper, 'qwen')
    await runEdit(wrapper)
    const p = lastEditParams()
    expect(p.model).toBe('qwen')
    expect(p.loras).toEqual([{ name: 'QwenL', weight: 0.8 }])
  })

  it('switching back to Klein restores the Klein rows and request', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS, edit_loras_qwen: QWEN_LORAS })
    await selectModel(wrapper, 'qwen')
    await selectModel(wrapper, 'klein')
    expect(loraNames(wrapper)).toEqual(['KleinL'])
    await runEdit(wrapper)
    const p = lastEditParams()
    expect(p.model).toBe('klein')
    expect(p.loras).toEqual([{ name: 'KleinL', weight: 0.5 }])
  })

  it('a row added under Qwen does not appear under Klein', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS })
    await selectModel(wrapper, 'qwen')
    await wrapper.find('[data-testid="edit-lora-add"]').trigger('click')
    await settle()
    const names = wrapper.findAll('[data-testid="edit-lora-name"]')
    await names[names.length - 1].setValue('QwenNew')
    await settle()
    await selectModel(wrapper, 'klein')
    expect(loraNames(wrapper)).toEqual(['KleinL'])
    await selectModel(wrapper, 'qwen')
    expect(loraNames(wrapper)).toEqual(['QwenNew'])
  })

  it('on unmount both lists are saved, each with its own rows', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS, edit_loras_qwen: QWEN_LORAS })
    await selectModel(wrapper, 'qwen')
    const before = UpdateSettings.mock.calls.length
    wrapper.unmount()
    await settle()
    const saved = mergedUpdatesFrom(before)
    expect(saved).toHaveProperty('edit_loras_klein')
    expect(saved).toHaveProperty('edit_loras_qwen')
    expect(JSON.parse(saved.edit_loras_klein)).toEqual([{ name: 'KleinL', weight: 0.5 }])
    expect(JSON.parse(saved.edit_loras_qwen)).toEqual([{ name: 'QwenL', weight: 0.8 }])
  })

  it('on unmount with Qwen unset, edit_loras_qwen is saved empty and Klein keeps its list', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: KLEIN_LORAS })
    await selectModel(wrapper, 'qwen')
    const before = UpdateSettings.mock.calls.length
    wrapper.unmount()
    await settle()
    const saved = mergedUpdatesFrom(before)
    expect(JSON.parse(saved.edit_loras_klein)).toEqual([{ name: 'KleinL', weight: 0.5 }])
    expect(JSON.parse(saved.edit_loras_qwen || '[]')).toEqual([])
    expect(saved).toHaveProperty('edit_loras_qwen')
  })
})

describe('Edit: Qwen vision file check', () => {
  it('Qwen selected, list known without mmproj: hint shown and generate disabled', async () => {
    setTextEncoders(['qwen_3_4b.safetensors'])
    const wrapper = await mountEdit()
    await selectModel(wrapper, 'qwen')
    await setInstruction(wrapper)
    expect(wrapper.find('[data-testid="edit-qwen-vision-missing"]').exists()).toBe(true)
    expect(generateButton(wrapper).element.disabled).toBe(true)
  })

  it('Qwen selected, list known and empty: hint shown and generate disabled', async () => {
    setTextEncoders([])
    const wrapper = await mountEdit()
    await selectModel(wrapper, 'qwen')
    await setInstruction(wrapper)
    expect(wrapper.find('[data-testid="edit-qwen-vision-missing"]').exists()).toBe(true)
    expect(generateButton(wrapper).element.disabled).toBe(true)
  })

  it('Qwen selected, mmproj present: no hint, generate enabled', async () => {
    setTextEncoders(['qwen_3_4b.safetensors', MMPROJ])
    const wrapper = await mountEdit()
    await selectModel(wrapper, 'qwen')
    await setInstruction(wrapper)
    expect(wrapper.find('[data-testid="edit-qwen-vision-missing"]').exists()).toBe(false)
    expect(generateButton(wrapper).element.disabled).toBe(false)
  })

  it('Qwen selected, list unknown: no hint, generate enabled', async () => {
    setTextEncoders(null)
    const wrapper = await mountEdit()
    await selectModel(wrapper, 'qwen')
    await setInstruction(wrapper)
    expect(wrapper.find('[data-testid="edit-qwen-vision-missing"]').exists()).toBe(false)
    expect(generateButton(wrapper).element.disabled).toBe(false)
  })

  it('Klein selected, list known without mmproj: no hint, generate enabled', async () => {
    setTextEncoders(['qwen_3_4b.safetensors'])
    const wrapper = await mountEdit()
    await setInstruction(wrapper)
    expect(wrapper.find('[data-testid="edit-qwen-vision-missing"]').exists()).toBe(false)
    expect(generateButton(wrapper).element.disabled).toBe(false)
  })
})

async function mountPost() {
  ListPresets.mockReturnValue(Promise.resolve([KLEIN_FX, QWEN_FX]))
  GetSettings.mockReturnValue(Promise.resolve({}))
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

async function showLightGroup(wrapper) {
  const select = wrapper.find('.fx-group-pick select')
  if (select.exists()) {
    const light = select.findAll('option').map(o => o.attributes('value')).find(v => v && v.includes('light'))
    if (light) {
      await select.setValue(light)
      await settle()
    }
  }
}

async function pickEffect(wrapper, preset) {
  await showLightGroup(wrapper)
  const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === preset.name)
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
}

function modelBlockVisible(wrapper) {
  const block = wrapper.find('.fx-model')
  return block.exists() && block.isVisible()
}

describe('Effect card: Model and LoRA block by effect model', () => {
  it('no effect selected: block shown', async () => {
    const wrapper = await mountPost()
    expect(modelBlockVisible(wrapper)).toBe(true)
  })

  it('Klein effect selected: block shown', async () => {
    const wrapper = await mountPost()
    await pickEffect(wrapper, KLEIN_FX)
    expect(modelBlockVisible(wrapper)).toBe(true)
  })

  it('Qwen effect selected: block hidden', async () => {
    const wrapper = await mountPost()
    await pickEffect(wrapper, QWEN_FX)
    expect(modelBlockVisible(wrapper)).toBe(false)
  })

  it('switching from Qwen effect back to Klein effect shows the block again', async () => {
    const wrapper = await mountPost()
    await pickEffect(wrapper, QWEN_FX)
    await pickEffect(wrapper, KLEIN_FX)
    expect(modelBlockVisible(wrapper)).toBe(true)
  })
})
