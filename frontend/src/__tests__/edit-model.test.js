import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, EditImage: vi.fn(() => Promise.resolve(undefined)) }
})

import EditSection from '../components/EditSection.vue'
import { GetSettings, UpdateSettings, ListPresets, EditImage, GetSDLoRAs } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const SOURCE = 'c291cmNl'
const MOSS_STYLE = { id: 1, name: 'Moss Film', model_name: 'moss_image_sft_bf16', prompt: 'moss tags', loras: '' }
const KLEIN_STYLE = { id: 2, name: 'Klein Film', model_name: 'flux-2-klein-4b', prompt: 'klein tags', loras: '' }
const Z_STYLE = { id: 3, name: 'Z Photo', model_name: 'z-image-turbo', prompt: 'z tags', loras: '' }
const PRESETS = [MOSS_STYLE, KLEIN_STYLE, Z_STYLE]

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
  EditImage.mockImplementation(() => Promise.resolve({ image: 'UkVT' }))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  GetSettings.mockReturnValue(Promise.resolve({}))
})

async function mountEdit({ settings = {}, batch = false } = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(EditSection, { props: { source: SOURCE, batch } })
  await settle()
  return wrapper
}

function modelSelect(wrapper) {
  const sel = wrapper.find('[data-testid="edit-model-select"]')
  expect(sel.exists(), 'edit-model-select must exist').toBe(true)
  return sel
}

async function chooseModel(wrapper, value) {
  await modelSelect(wrapper).setValue(value)
  await settle()
}

async function setInstruction(wrapper, text = 'make hair red') {
  await wrapper.find('[data-testid="edit-instruction"]').setValue(text)
  await settle()
}

async function runEdit(wrapper, { batch = false } = {}) {
  await setInstruction(wrapper)
  const label = batch ? t('edit.btn_apply_all') : t('edit.btn_generate')
  const btn = wrapper.findAll('button').find(b => b.text().includes(label))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

function lastEditParams() {
  expect(EditImage).toHaveBeenCalled()
  return EditImage.mock.calls[EditImage.mock.calls.length - 1][0]
}

function loraNames(wrapper) {
  return wrapper.findAll('[data-testid="edit-lora-name"]').map(i => i.element.value)
}

async function addLora(wrapper, name, weight) {
  await wrapper.find('[data-testid="edit-lora-add"]').trigger('click')
  await settle()
  const names = wrapper.findAll('[data-testid="edit-lora-name"]')
  const weights = wrapper.findAll('[data-testid="edit-lora-weight"]')
  await names[names.length - 1].setValue(name)
  await weights[weights.length - 1].setValue(String(weight))
  await settle()
}

function styleOptionValues(wrapper) {
  return wrapper.find('[data-testid="edit-style-select"]').findAll('option').map(o => Number(o.element.value))
}

function mergedSettingsUpdates() {
  return UpdateSettings.mock.calls.reduce((acc, [data]) => ({ ...acc, ...(data || {}) }), {})
}

function expectMossModel(params) {
  expect(params.model === undefined || params.model === 'moss', `model=${params.model}`).toBe(true)
}

describe('T8: Moss is the default edit model', () => {
  it('model select exists with moss and klein options, moss selected by default', async () => {
    const wrapper = await mountEdit()
    const sel = modelSelect(wrapper)
    const values = sel.findAll('option').map(o => o.element.value)
    expect(values).toContain('moss')
    expect(values).toContain('klein')
    expect(sel.element.value).toBe('moss')
  })

  it('direct edit without choosing sends no model or model=moss', async () => {
    const wrapper = await mountEdit()
    modelSelect(wrapper)
    await runEdit(wrapper)
    expectMossModel(lastEditParams())
  })

  it('Moss LoRAs come from edit_loras setting and go into the request', async () => {
    const wrapper = await mountEdit({ settings: { edit_loras: '[{"name":"MossL","weight":0.8}]' } })
    expect(modelSelect(wrapper).element.value).toBe('moss')
    expect(loraNames(wrapper)).toEqual(['MossL'])
    await runEdit(wrapper)
    const params = lastEditParams()
    expectMossModel(params)
    expect(params.loras).toEqual([{ name: 'MossL', weight: 0.8 }])
  })

  it('Moss ignores edit_loras_klein', async () => {
    const wrapper = await mountEdit({ settings: { edit_loras_klein: '[{"name":"KleinL","weight":0.5}]' } })
    expect(modelSelect(wrapper).element.value).toBe('moss')
    expect(loraNames(wrapper)).toEqual([])
    await runEdit(wrapper)
    const params = lastEditParams()
    expectMossModel(params)
    expect(params.loras || []).toEqual([])
  })
})

describe('T9: Klein choice reaches the request and is persisted', () => {
  it('direct edit with Klein sends model=klein and Klein LoRAs only', async () => {
    const wrapper = await mountEdit({ settings: {
      edit_loras: '[{"name":"MossL","weight":0.8}]',
      edit_loras_klein: '[{"name":"KleinL","weight":0.5}]',
    } })
    await chooseModel(wrapper, 'klein')
    expect(loraNames(wrapper)).toEqual(['KleinL'])
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.loras).toEqual([{ name: 'KleinL', weight: 0.5 }])
  })

  it('batch edit with Klein emits apply-edit with model=klein and Klein LoRAs', async () => {
    const wrapper = await mountEdit({ batch: true, settings: { edit_loras_klein: '[{"name":"KleinL","weight":0.5}]' } })
    await chooseModel(wrapper, 'klein')
    await runEdit(wrapper, { batch: true })
    const events = wrapper.emitted('apply-edit')
    expect(events).toBeTruthy()
    const params = events[events.length - 1][0]
    expect(params.model).toBe('klein')
    expect(params.loras).toEqual([{ name: 'KleinL', weight: 0.5 }])
    expect(EditImage).not.toHaveBeenCalled()
  })

  it('batch edit with Moss emits apply-edit without model or with model=moss', async () => {
    const wrapper = await mountEdit({ batch: true })
    modelSelect(wrapper)
    await runEdit(wrapper, { batch: true })
    const events = wrapper.emitted('apply-edit')
    expect(events).toBeTruthy()
    expectMossModel(events[events.length - 1][0])
  })

  it('quality mode with Klein sends model=klein together with output_mode=quality', async () => {
    const wrapper = await mountEdit({ settings: { edit_quality_mode: 'true', edit_resolution_id: '5' } })
    await chooseModel(wrapper, 'klein')
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.output_mode).toBe('quality')
    expect(params.resolution_id).toBe(5)
  })

  it('chosen model and Klein LoRAs are saved to settings', async () => {
    const wrapper = await mountEdit()
    await chooseModel(wrapper, 'klein')
    await addLora(wrapper, 'KleinNew', 0.7)
    wrapper.unmount()
    await settle()
    const saved = mergedSettingsUpdates()
    expect(saved.edit_model).toBe('klein')
    expect(JSON.parse(saved.edit_loras_klein)).toEqual([{ name: 'KleinNew', weight: 0.7 }])
    expect(JSON.parse(saved.edit_loras || '[]')).toEqual([])
  })

  it('switching back to Moss saves edit_model=moss', async () => {
    const wrapper = await mountEdit({ settings: { edit_model: 'klein' } })
    await chooseModel(wrapper, 'moss')
    wrapper.unmount()
    await settle()
    expect(mergedSettingsUpdates().edit_model).toBe('moss')
  })

  it('saved Klein choice and LoRAs are restored on mount', async () => {
    const wrapper = await mountEdit({ settings: {
      edit_model: 'klein',
      edit_loras: '[{"name":"MossL","weight":0.8}]',
      edit_loras_klein: '[{"name":"KleinL","weight":0.5}]',
    } })
    expect(modelSelect(wrapper).element.value).toBe('klein')
    expect(loraNames(wrapper)).toEqual(['KleinL'])
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.loras).toEqual([{ name: 'KleinL', weight: 0.5 }])
  })
})

describe('T10: LoRA lists are separate per model', () => {
  it('LoRA added under Klein does not appear under Moss, and Moss list stays intact', async () => {
    const wrapper = await mountEdit({ settings: { edit_loras: '[{"name":"MossL","weight":0.8}]' } })
    await chooseModel(wrapper, 'klein')
    expect(loraNames(wrapper)).toEqual([])
    await addLora(wrapper, 'KleinOnly', 0.4)
    expect(loraNames(wrapper)).toEqual(['KleinOnly'])

    await chooseModel(wrapper, 'moss')
    expect(loraNames(wrapper)).toEqual(['MossL'])
    await runEdit(wrapper)
    const params = lastEditParams()
    expectMossModel(params)
    expect(params.loras).toEqual([{ name: 'MossL', weight: 0.8 }])

    await chooseModel(wrapper, 'klein')
    expect(loraNames(wrapper)).toEqual(['KleinOnly'])
  })

  it('removing a Moss LoRA does not touch the Klein list', async () => {
    const wrapper = await mountEdit({ settings: {
      edit_loras: '[{"name":"MossL","weight":0.8}]',
      edit_loras_klein: '[{"name":"KleinL","weight":0.5}]',
    } })
    await wrapper.find('[data-testid="edit-lora-remove"]').trigger('click')
    await settle()
    expect(loraNames(wrapper)).toEqual([])
    await chooseModel(wrapper, 'klein')
    expect(loraNames(wrapper)).toEqual(['KleinL'])
  })

  it('both lists are saved to their own settings keys', async () => {
    const wrapper = await mountEdit({ settings: { edit_loras: '[{"name":"MossL","weight":0.8}]' } })
    await chooseModel(wrapper, 'klein')
    await addLora(wrapper, 'KleinOnly', 0.4)
    wrapper.unmount()
    await settle()
    const saved = mergedSettingsUpdates()
    expect(JSON.parse(saved.edit_loras)).toEqual([{ name: 'MossL', weight: 0.8 }])
    expect(JSON.parse(saved.edit_loras_klein)).toEqual([{ name: 'KleinOnly', weight: 0.4 }])
  })
})

describe('T11: style list follows the chosen model', () => {
  it('Moss shows only moss styles, Klein only klein styles', async () => {
    const wrapper = await mountEdit()
    modelSelect(wrapper)
    expect(styleOptionValues(wrapper)).toEqual([0, MOSS_STYLE.id])
    await chooseModel(wrapper, 'klein')
    expect(styleOptionValues(wrapper)).toEqual([0, KLEIN_STYLE.id])
  })

  it('switching Moss -> Klein clears a selected moss style and the request has no style', async () => {
    const wrapper = await mountEdit()
    modelSelect(wrapper)
    await wrapper.find('[data-testid="edit-style-select"]').setValue(MOSS_STYLE.id)
    await settle()
    await chooseModel(wrapper, 'klein')
    expect(Number(wrapper.find('[data-testid="edit-style-select"]').element.value)).toBe(0)
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.style_preset_id || 0).toBe(0)
  })

  it('a klein style chosen under Klein is sent with model=klein', async () => {
    const wrapper = await mountEdit()
    await chooseModel(wrapper, 'klein')
    await wrapper.find(`[data-testid="edit-style-select"] option[value="${KLEIN_STYLE.id}"]`).setSelected()
    await settle()
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.style_preset_id).toBe(KLEIN_STYLE.id)
  })

  it('a saved klein style is restored together with the saved Klein choice', async () => {
    const wrapper = await mountEdit({ settings: { edit_model: 'klein', edit_style_preset_id: String(KLEIN_STYLE.id) } })
    expect(modelSelect(wrapper).element.value).toBe('klein')
    expect(Number(wrapper.find('[data-testid="edit-style-select"]').element.value)).toBe(KLEIN_STYLE.id)
  })
})
