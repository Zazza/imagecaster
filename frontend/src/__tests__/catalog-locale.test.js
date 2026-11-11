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
  }
})

import SettingsPage from '../components/SettingsPage.vue'
import {
  GetSettings, UpdateSettings, GetModelCatalog, GetServerStatus, GetServerModels, GetServerLLMModels,
} from '../wailsjs/go/main/App.js'
import { setLocale, ru, en } from '../i18n/index.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 6; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

const CATALOG = {
  llm_generate: [
    { name: 'gen-a', description: 'EN gen desc', description_ru: 'РУ описание генерации', size_gb: 5.2 },
    { name: 'gen-noru', description: 'EN only gen desc', size_gb: 1.5 },
  ],
  llm_vision: [
    { name: 'vis-a', description: 'EN vision desc', description_ru: 'РУ описание зрения', size_gb: 6.1 },
  ],
  sd_models: [
    { name: 'sd-a', base: 'SDXL', category: 'Realistic', category_ru: 'Реализм', description: 'EN sd desc', description_ru: 'РУ описание модели', size_gb: 6.6 },
    { name: 'sd-noru', base: 'SDXL', category: 'Anime', description: 'EN only sd desc', size_gb: 6.5 },
  ],
  lora: [
    { name: 'lora-a', base: 'SDXL', category: 'Style', category_ru: 'Стиль', description: 'EN lora desc', description_ru: 'РУ описание лоры', size_mb: 217 },
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('confirm', () => false)
  GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetModelCatalog.mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(CATALOG))))
  GetServerStatus.mockImplementation(() => Promise.resolve({}))
  GetServerModels.mockImplementation((type) => Promise.resolve(type === 'sd' ? [{ name: 'sd-a.safetensors' }] : []))
  GetServerLLMModels.mockImplementation(() => Promise.resolve([{ name: 'gen-a' }]))
})

afterEach(() => {
  setLocale('ru')
  vi.unstubAllGlobals()
})

async function openCatalog(locale) {
  setLocale(locale)
  const dict = locale === 'ru' ? ru : en
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  const tab = w.findAll('.tabs button').find(b => b.text().trim() === dict['settings.tab_catalog'])
  expect(tab, 'catalog tab').toBeTruthy()
  await tab.trigger('click')
  await settle()
  return w
}

function rowOf(w, name) {
  const el = w.findAll('div').filter(d => d.text().includes(name) && d.findAll('button').length > 0)
  return el[el.length - 1]
}

describe('catalog tab language', () => {
  it('catalog tab hidden when connection_mode is not server', async () => {
    GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'direct' }))
    const w = mount(SettingsPage, { attachTo: document.body })
    await settle()
    const names = w.findAll('.tabs button').map(b => b.text().trim())
    expect(names).not.toContain(ru['settings.tab_catalog'])
  })

  it('ru: shows description_ru and category_ru, not English texts', async () => {
    const w = await openCatalog('ru')
    const text = w.text()
    for (const s of ['РУ описание генерации', 'РУ описание зрения', 'РУ описание модели', 'Реализм', 'РУ описание лоры', 'Стиль']) {
      expect(text, s).toContain(s)
    }
    for (const s of ['EN gen desc', 'EN vision desc', 'EN sd desc', 'Realistic', 'EN lora desc', 'Style']) {
      expect(text, s).not.toContain(s)
    }
  })

  it('en: shows description and category, not Russian texts', async () => {
    const w = await openCatalog('en')
    const text = w.text()
    for (const s of ['EN gen desc', 'EN vision desc', 'EN sd desc', 'Realistic', 'EN lora desc', 'Style']) {
      expect(text, s).toContain(s)
    }
    for (const s of ['РУ описание', 'Реализм', 'Стиль']) {
      expect(text, s).not.toContain(s)
    }
  })

  it('ru: missing *_ru falls back to English text', async () => {
    const w = await openCatalog('ru')
    const text = w.text()
    expect(text).toContain('EN only gen desc')
    expect(text).toContain('EN only sd desc')
    expect(text).toContain('Anime')
  })

  it('ru: size units ГБ / МБ', async () => {
    const w = await openCatalog('ru')
    const text = w.text()
    expect(text).toMatch(/5\.2\s*ГБ/)
    expect(text).toMatch(/217\s*МБ/)
    expect(text).not.toMatch(/\bGB\b|\bMB\b/)
  })

  it('en: size units GB / MB', async () => {
    const w = await openCatalog('en')
    const text = w.text()
    expect(text).toMatch(/5\.2\s*GB/)
    expect(text).toMatch(/217\s*MB/)
    expect(text).not.toMatch(/ГБ|МБ/)
  })

  it.each([
    ['ru', 'Удалить'],
    ['en', 'Delete'],
  ])('%s: delete button of installed model reads "%s"', async (locale, label) => {
    const w = await openCatalog(locale)
    for (const name of ['gen-a', 'sd-a']) {
      const row = rowOf(w, name)
      expect(row, name).toBeTruthy()
      const btns = row.findAll('button').map(b => b.text().trim())
      expect(btns, name).toContain(label)
    }
    const all = w.findAll('button').map(b => b.text().trim())
    expect(all).not.toContain(locale === 'ru' ? 'Delete' : 'Удалить')
  })
})
