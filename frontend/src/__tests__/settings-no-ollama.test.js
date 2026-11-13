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
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'

const { GetSettings, UpdateSettings, GetServerStatus } = App

enableAutoUnmount(afterEach)

const SERVICES = {
  llm: { name: 'llama.cpp', status: 'running', category: 'llm' },
  ollama: { name: 'Ollama', status: 'stopped', category: 'llm' },
  sd: { name: 'SD', status: 'running', category: 'sd' },
}

const UTILITIES = {
  llm: { name: 'llama.cpp', status: 'running', category: 'llm' },
  sd: { name: 'SD', status: 'running', category: 'sd' },
  git: { name: 'git-tool', status: 'stopped', category: 'utility' },
  ollama: { name: 'Ollama', status: 'stopped', category: 'utility' },
}

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

function visibleRows(w, name) {
  return w.findAll('span').filter(s => s.text().trim() === name && s.isVisible())
}

function visibleTextHas(w, needle) {
  return w.findAll('*').some(el => el.element.children.length === 0 && el.isVisible() && el.text().toLowerCase().includes(needle.toLowerCase()))
}

function settingsCalls() {
  const out = []
  for (const [name, fn] of Object.entries(App)) {
    if (!fn || !fn.mock) continue
    for (const args of fn.mock.calls) out.push({ name, args })
  }
  return out
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSettings.mockImplementation(() => Promise.resolve({ server_url: 'http://127.0.0.1:18080', llm_num_gpu: '20' }))
  GetServerStatus.mockImplementation(() => Promise.resolve({ processes: SERVICES, health: {}, models: {} }))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

async function mountSettings() {
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  return w
}

describe('settings services list without Ollama', () => {
  it.each(['ru', 'en'])('%s: llama.cpp and SD rows are shown, Ollama row is not', async (locale) => {
    setLocale(locale)
    const w = await mountSettings()
    expect(visibleRows(w, 'llama.cpp').length).toBeGreaterThan(0)
    expect(visibleRows(w, 'SD').length).toBeGreaterThan(0)
    expect(visibleRows(w, 'Ollama')).toHaveLength(0)
    expect(visibleTextHas(w, 'ollama')).toBe(false)
  })

  it('an ollama utility is not shown in the expanded utilities list', async () => {
    GetServerStatus.mockImplementation(() => Promise.resolve({ processes: UTILITIES, health: {}, models: {} }))
    const w = await mountSettings()
    const header = w.findAll('div').filter(d => d.text().trim() === t('settings.section_utilities')).at(-1)
    expect(header, 'utilities header').toBeTruthy()
    await header.trigger('click')
    await settle()
    expect(visibleRows(w, 'git-tool').length).toBeGreaterThan(0)
    expect(visibleRows(w, 'Ollama')).toHaveLength(0)
    expect(visibleTextHas(w, 'ollama')).toBe(false)
  })

  it('without an ollama process the other rows are unchanged', async () => {
    GetServerStatus.mockImplementation(() => Promise.resolve({ processes: { llm: SERVICES.llm, sd: SERVICES.sd }, health: {}, models: {} }))
    const w = await mountSettings()
    expect(visibleRows(w, 'llama.cpp').length).toBeGreaterThan(0)
    expect(visibleRows(w, 'SD').length).toBeGreaterThan(0)
  })
})

describe('settings without the num_gpu field', () => {
  it.each(['ru', 'en'])('%s: no GPU layers field on the connection tab', async (locale) => {
    setLocale(locale)
    const w = await mountSettings()
    expect(w.text()).not.toContain('num_gpu')
    expect(w.text()).not.toContain('Слоёв на видеокарте')
    expect(w.text()).not.toContain('GPU Layers')
    const numberInputs = w.findAll('input[type="number"]').filter(i => i.attributes('placeholder') === '-1' && i.attributes('max') === '999')
    expect(numberInputs).toHaveLength(0)
  })

  it('saving the connection does not send llm_num_gpu', async () => {
    const w = await mountSettings()
    const save = w.findAll('button').find(b => b.text().includes(t('settings.btn_save_server')))
    expect(save, 'save connection button').toBeTruthy()
    for (const fn of Object.values(App)) if (fn && fn.mock) fn.mockClear()
    await save.trigger('click')
    await settle()
    const calls = settingsCalls()
    const objects = calls.flatMap(c => c.args).filter(a => a && typeof a === 'object')
    expect(objects.some(o => o.server_url === 'http://127.0.0.1:18080'), 'connection was saved').toBe(true)
    for (const o of objects) expect(Object.keys(o)).not.toContain('llm_num_gpu')
  })
})

describe('i18n settings.* and server.* without Ollama', () => {
  it.each([['ru', ru], ['en', en]])('%s', (_, dict) => {
    const bad = Object.entries(dict)
      .filter(([k]) => k.startsWith('settings.') || k.startsWith('server.'))
      .filter(([, v]) => String(v).toLowerCase().includes('ollama'))
      .map(([k]) => k)
    expect(bad).toEqual([])
  })

  it.each([
    ['ru', ru, 'Модели ИИ-помощника', 'движок текста (llama.cpp)'],
    ['en', en, 'AI assistant models', 'a text engine (llama.cpp)'],
  ])('%s: AI assistant models title and text engine feature', (_, dict, title, feature) => {
    expect(dict['settings.title_llm_models']).toBe(title)
    expect(dict['server.feature_llm']).toBe(feature)
  })
})
