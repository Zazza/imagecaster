import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return {
    ...base,
    GetDefaultEditGarmentPrompt: vi.fn(() => Promise.resolve('')),
    GetSDModels: vi.fn(() => Promise.resolve([])),
    GetSDLoRAs: vi.fn(() => Promise.resolve([])),
    GetLLMModels: vi.fn(() => Promise.resolve([])),
    GetModelPresets: vi.fn(() => Promise.resolve([])),
  }
})

import SettingsPage from '../components/SettingsPage.vue'
import * as App from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'
import { api } from '../api.js'

const { GetSettings, UpdateSettings, GetServerStatus } = App

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 6; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

const LLM = [{ name: 'text-a:8b' }, { name: 'text-b:3b' }]

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetServerStatus.mockImplementation(() => Promise.resolve({ processes: {}, health: {}, models: {} }))
  vi.spyOn(api, 'getServerModels').mockImplementation(() => Promise.resolve([]))
  vi.spyOn(api, 'getServerLLMModels').mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(LLM))))
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function mountSettings(settings) {
  GetSettings.mockImplementation(() => Promise.resolve({ server_url: 'http://127.0.0.1:18080', ...settings }))
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  return w
}

function generateSelect(w) {
  const label = w.findAll('label').find(l => l.text().trim() === t('settings.label_model_generate'))
  expect(label, 'generate model label').toBeTruthy()
  const group = label.element.closest('.form-group') || label.element.parentElement
  const sel = group.querySelector('select')
  expect(sel, 'generate model select').toBeTruthy()
  return sel
}

describe.each(['ru', 'en'])('settings prompt-model select (condition 4, %s)', (locale) => {
  beforeEach(() => setLocale(locale))

  it('empty option is disabled and reads llm_setup.choose', async () => {
    const w = await mountSettings({ llm_generate_model: 'text-a:8b' })
    const sel = generateSelect(w)
    const empty = [...sel.options].filter(o => o.value === '')
    expect(empty).toHaveLength(1)
    expect(empty[0].disabled).toBe(true)
    expect(empty[0].textContent.trim()).toBe(t('llm_setup.choose'))
  })

  it('no selectable empty "default" option', async () => {
    const w = await mountSettings({ llm_generate_model: 'text-a:8b' })
    const sel = generateSelect(w)
    const selectableEmpty = [...sel.options].filter(o => o.value === '' && !o.disabled)
    expect(selectableEmpty).toHaveLength(0)
    const selectable = [...sel.options].filter(o => !o.disabled).map(o => o.value)
    expect(selectable).toEqual(['text-a:8b', 'text-b:3b'])
  })

  it('no model saved: select shows the disabled choose option', async () => {
    const w = await mountSettings({ llm_generate_model: '' })
    const sel = generateSelect(w)
    expect(sel.value).toBe('')
    expect(sel.options[sel.selectedIndex].disabled).toBe(true)
    expect(sel.options[sel.selectedIndex].textContent.trim()).toBe(t('llm_setup.choose'))
  })

  it('no LLM models from the server: the select is not shown', async () => {
    api.getServerLLMModels.mockImplementation(() => Promise.resolve([]))
    const w = await mountSettings({})
    expect(w.findAll('label').some(l => l.text().trim() === t('settings.label_model_generate'))).toBe(false)
  })
})
