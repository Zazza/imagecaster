import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import PresetForm from '../components/PresetForm.vue'
import { GetSDModels, GetSDLoRAs, GetSDSamplers } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  GetSDSamplers.mockImplementation(() => Promise.resolve(['Euler']))
  GetSDModels.mockImplementation(() => Promise.resolve([]))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
})

describe('PresetForm list loading', () => {
  it('shows an error when the model list fails to load', async () => {
    GetSDModels.mockImplementation(() => Promise.reject(new Error('server down')))
    const w = mount(PresetForm)
    await settle()
    expect(w.text()).toContain(t('preset.error_load_lists', { error: 'Error: server down' }))
  })

  it('shows an error when the LoRA list fails to load', async () => {
    GetSDLoRAs.mockImplementation(() => Promise.reject(new Error('no loras')))
    const w = mount(PresetForm)
    await settle()
    expect(w.text()).toContain(t('preset.error_load_lists', { error: 'Error: no loras' }))
  })

  it('no error when all lists load', async () => {
    const w = mount(PresetForm)
    await settle()
    expect(w.text()).not.toContain(t('preset.error_load_lists', { error: '' }).split(':')[0])
  })
})
