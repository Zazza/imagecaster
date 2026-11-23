import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import EditSection from '../components/EditSection.vue'
import { GetSettings, UpdateSettings, ListPresets, EditImage, GetSDLoRAs } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const SOURCE = 'c291cmNl'
const MOSS_STYLE = { id: 1, name: 'Moss Film', model_name: 'moss_image_sft_bf16', prompt: 'moss tags', loras: '' }
const KLEIN_STYLE = { id: 2, name: 'Klein Film', model_name: 'flux-2-klein-4b', prompt: 'klein tags', loras: '' }
const Z_STYLE = { id: 3, name: 'Z Photo', model_name: 'z-image-turbo', prompt: 'z tags', loras: '' }
const PRESETS = [MOSS_STYLE, KLEIN_STYLE, Z_STYLE]

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

function styleOptionValues(wrapper) {
  return wrapper.find('[data-testid="edit-style-select"]').findAll('option').map(o => Number(o.element.value))
}

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
    expect(params.editor).toEqual({ model: 'klein', loras: [] })
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
