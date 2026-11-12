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

describe('T8: Edit has no model choice, Klein only', () => {
  it('there is no model select and no Moss option', async () => {
    const wrapper = await mountEdit()
    expect(wrapper.find('[data-testid="edit-model-select"]').exists()).toBe(false)
    expect(wrapper.findAll('option').map(o => o.text())).not.toContain('Moss')
    expect(wrapper.text()).not.toContain('Moss')
  })

  it('direct edit sends model=klein', async () => {
    const wrapper = await mountEdit()
    await runEdit(wrapper)
    expect(lastEditParams().model).toBe('klein')
  })

  it('saved edit_model=moss does not bring Moss back', async () => {
    const wrapper = await mountEdit({ settings: { edit_model: 'moss' } })
    expect(wrapper.text()).not.toContain('Moss')
    await runEdit(wrapper)
    expect(lastEditParams().model).toBe('klein')
  })

  it('batch edit emits apply-edit with model=klein and Klein LoRAs', async () => {
    const wrapper = await mountEdit({ batch: true, settings: { edit_loras_klein: '[{"name":"KleinL","weight":0.5}]' } })
    await runEdit(wrapper, { batch: true })
    const events = wrapper.emitted('apply-edit')
    expect(events).toBeTruthy()
    const params = events[events.length - 1][0]
    expect(params.model).toBe('klein')
    expect(params.loras).toEqual([{ name: 'KleinL', weight: 0.5 }])
    expect(EditImage).not.toHaveBeenCalled()
  })

  it('quality mode sends model=klein together with output_mode=quality', async () => {
    const wrapper = await mountEdit({ settings: { edit_quality_mode: 'true', edit_resolution_id: '5' } })
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.output_mode).toBe('quality')
    expect(params.resolution_id).toBe(5)
  })
})

describe('T9: Klein LoRAs', () => {
  it('LoRAs come from edit_loras_klein; the old Moss list edit_loras is ignored', async () => {
    const wrapper = await mountEdit({ settings: {
      edit_loras: '[{"name":"MossL","weight":0.8}]',
      edit_loras_klein: '[{"name":"KleinL","weight":0.5}]',
    } })
    expect(loraNames(wrapper)).toEqual(['KleinL'])
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.loras).toEqual([{ name: 'KleinL', weight: 0.5 }])
  })

  it('only the old Moss list saved: no LoRAs shown or sent', async () => {
    const wrapper = await mountEdit({ settings: { edit_loras: '[{"name":"MossL","weight":0.8}]' } })
    expect(loraNames(wrapper)).toEqual([])
    await runEdit(wrapper)
    expect(lastEditParams().loras || []).toEqual([])
  })

  it('added LoRA is saved to edit_loras_klein; edit_model and edit_loras are not written', async () => {
    const wrapper = await mountEdit()
    await addLora(wrapper, 'KleinNew', 0.7)
    wrapper.unmount()
    await settle()
    const saved = mergedSettingsUpdates()
    expect(JSON.parse(saved.edit_loras_klein)).toEqual([{ name: 'KleinNew', weight: 0.7 }])
    expect(saved).not.toHaveProperty('edit_model')
    expect(saved).not.toHaveProperty('edit_loras')
  })

  it('removing a LoRA removes it from the request', async () => {
    const wrapper = await mountEdit({ settings: { edit_loras_klein: '[{"name":"KleinL","weight":0.5}]' } })
    await wrapper.find('[data-testid="edit-lora-remove"]').trigger('click')
    await settle()
    expect(loraNames(wrapper)).toEqual([])
    await runEdit(wrapper)
    expect(lastEditParams().loras || []).toEqual([])
  })
})

describe('T10: style list has only Klein styles', () => {
  it('only klein styles are offered', async () => {
    const wrapper = await mountEdit()
    expect(styleOptionValues(wrapper)).toEqual([0, KLEIN_STYLE.id])
  })

  it('a klein style is sent with model=klein', async () => {
    const wrapper = await mountEdit()
    await wrapper.find(`[data-testid="edit-style-select"] option[value="${KLEIN_STYLE.id}"]`).setSelected()
    await settle()
    await runEdit(wrapper)
    const params = lastEditParams()
    expect(params.model).toBe('klein')
    expect(params.style_preset_id).toBe(KLEIN_STYLE.id)
  })

  it('a saved klein style is restored', async () => {
    const wrapper = await mountEdit({ settings: { edit_style_preset_id: String(KLEIN_STYLE.id) } })
    expect(Number(wrapper.find('[data-testid="edit-style-select"]').element.value)).toBe(KLEIN_STYLE.id)
  })

  it('a saved moss style is not restored and not sent', async () => {
    const wrapper = await mountEdit({ settings: { edit_style_preset_id: String(MOSS_STYLE.id) } })
    expect(Number(wrapper.find('[data-testid="edit-style-select"]').element.value)).toBe(0)
    await runEdit(wrapper)
    expect(lastEditParams().style_preset_id || 0).toBe(0)
  })
})
