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
  ListPresetTypes, ListHiresProfiles, EnqueueFromImage, SyncPresets,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const QWEN = 'qwen_image_2.1-Q8_0'
const KLEIN = 'flux-2-klein-4b'
const SOURCE = 'c291cmNl'
const IMG = 'aW1n'

const KLEIN_STYLE = { id: 2, name: 'Klein Film', model_name: KLEIN, prompt: 'klein tags', loras: '' }
const Z_STYLE = { id: 3, name: 'Z Photo', model_name: 'z-image-turbo', prompt: 'z tags', loras: '' }
const QWEN_STYLE = { id: 4, name: 'Film (Qwen-Image)', model_name: QWEN, prompt: 'qwen tags', loras: '' }

const TYPES = [{ id: 1, name: 'post' }, { id: 4, name: 'post_light' }]
const KLEIN_FX = { id: 40, name: 'Golden Hour', preset_type: 'post_light', type_id: 4, model_name: KLEIN, denoising_strength: null, loras: '' }
const QWEN_FX = { id: 41, name: 'Golden Hour (Qwen-Image)', preset_type: 'post_light', type_id: 4, model_name: QWEN, denoising_strength: null, loras: '' }

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
  globalThis.Image = FakeImage
  window.Image = FakeImage
})

afterEach(() => {
  globalThis.Image = OrigImage
  window.Image = OrigImage
})

async function mountEdit(presets, settings = {}) {
  ListPresets.mockReturnValue(Promise.resolve(presets))
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(EditSection, { props: { source: SOURCE, batch: false } })
  await settle()
  return wrapper
}

async function runEdit(wrapper) {
  await wrapper.find('[data-testid="edit-instruction"]').setValue('make hair red')
  await settle()
  const btn = wrapper.findAll('button').find(b => b.text().includes(t('edit.btn_generate')))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

function lastEditParams() {
  expect(EditImage).toHaveBeenCalled()
  return EditImage.mock.calls[EditImage.mock.calls.length - 1][0]
}

describe('Edit: Qwen-Image model choice', () => {
  it('no Qwen preset: no model select, request goes with model klein', async () => {
    const wrapper = await mountEdit([KLEIN_STYLE, Z_STYLE])
    expect(wrapper.find('[data-testid="edit-model-select"]').exists()).toBe(false)
    await runEdit(wrapper)
    expect(lastEditParams().model).toBe('klein')
  })

  it('empty preset list: no model select', async () => {
    const wrapper = await mountEdit([])
    expect(wrapper.find('[data-testid="edit-model-select"]').exists()).toBe(false)
  })

  it('Qwen preset available: select offers klein and qwen, klein by default', async () => {
    const wrapper = await mountEdit([KLEIN_STYLE, QWEN_STYLE])
    const select = wrapper.find('[data-testid="edit-model-select"]')
    expect(select.exists()).toBe(true)
    const values = select.findAll('option').map(o => o.attributes('value'))
    expect(values).toEqual(expect.arrayContaining(['klein', 'qwen']))
    expect(values.length).toBe(2)
    expect(select.element.value).toBe('klein')
    await runEdit(wrapper)
    expect(lastEditParams().model).toBe('klein')
  })

  it('Qwen preset name without model match does not enable Qwen', async () => {
    const fake = { id: 5, name: 'Qwen-Image look', model_name: KLEIN, prompt: 'x', loras: '' }
    const wrapper = await mountEdit([KLEIN_STYLE, fake])
    expect(wrapper.find('[data-testid="edit-model-select"]').exists()).toBe(false)
  })

  it('qwen-image spelled with a dash also enables Qwen', async () => {
    const dashed = { ...QWEN_STYLE, id: 6, model_name: 'qwen-image-2512-Q4_K_M' }
    const wrapper = await mountEdit([KLEIN_STYLE, dashed])
    expect(wrapper.find('[data-testid="edit-model-select"]').exists()).toBe(true)
  })

  it('choosing Qwen sends model qwen', async () => {
    const wrapper = await mountEdit([KLEIN_STYLE, QWEN_STYLE])
    await wrapper.find('[data-testid="edit-model-select"]').setValue('qwen')
    await settle()
    await runEdit(wrapper)
    expect(lastEditParams().model).toBe('qwen')
  })

  it('Qwen preset hidden as not installed: no model select', async () => {
    SyncPresets.mockImplementation(() => Promise.resolve({ statuses: [
      { id: KLEIN_STYLE.id, installed: true, missing_sd: [], missing_lora: [] },
      { id: QWEN_STYLE.id, installed: false, missing_sd: [QWEN], missing_lora: [] },
    ] }))
    const wrapper = await mountEdit([KLEIN_STYLE, QWEN_STYLE])
    expect(wrapper.find('[data-testid="edit-model-select"]').exists()).toBe(false)
  })
})

async function mountPost(settings = {}) {
  ListPresets.mockReturnValue(Promise.resolve([KLEIN_FX, QWEN_FX]))
  GetSettings.mockReturnValue(Promise.resolve(settings))
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

function effectCard(wrapper, preset) {
  return wrapper.findAll('.fx-card').find(c => c.attributes('title') === preset.name)
}

async function applyEffect(wrapper, preset) {
  await showLightGroup(wrapper)
  const card = effectCard(wrapper, preset)
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
  const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
  expect(apply).toBeTruthy()
  expect(apply.element.disabled).toBe(false)
  await apply.trigger('click')
  await settle()
}

function lastEnqueue() {
  expect(EnqueueFromImage).toHaveBeenCalled()
  return EnqueueFromImage.mock.calls[EnqueueFromImage.mock.calls.length - 1][0]
}

function sentLoraNames(params) {
  if (!params.overrides || !params.overrides.loras) return []
  return JSON.parse(params.overrides.loras).map(l => l.name)
}

describe('Effects: Qwen-Image presets', () => {
  it('Qwen effect card shows the translated base label and the Qwen-Image mark, next to the Klein card', async () => {
    const wrapper = await mountPost()
    await showLightGroup(wrapper)
    const qwen = effectCard(wrapper, QWEN_FX)
    const klein = effectCard(wrapper, KLEIN_FX)
    expect(qwen).toBeTruthy()
    expect(klein).toBeTruthy()
    const label = t('post.effect_golden_hour')
    expect(label).not.toBe('post.effect_golden_hour')
    expect(qwen.text()).toContain(label)
    expect(qwen.text()).toContain('Qwen-Image')
    expect(klein.text()).toContain(label)
    expect(klein.text()).not.toContain('Qwen')
  })

  it('Klein effect LoRAs are not sent with a Qwen effect', async () => {
    const wrapper = await mountPost({ effect_loras_klein: '[{"name":"LoraA","weight":0.7}]' })
    await applyEffect(wrapper, QWEN_FX)
    const p = lastEnqueue()
    expect(p.preset_id).toBe(QWEN_FX.id)
    expect(sentLoraNames(p)).not.toContain('LoraA')
  })

  it('Klein effect LoRAs are still sent with a Klein effect', async () => {
    const wrapper = await mountPost({ effect_loras_klein: '[{"name":"LoraA","weight":0.7}]' })
    await applyEffect(wrapper, KLEIN_FX)
    const p = lastEnqueue()
    expect(p.preset_id).toBe(KLEIN_FX.id)
    expect(sentLoraNames(p)).toContain('LoraA')
  })
})
