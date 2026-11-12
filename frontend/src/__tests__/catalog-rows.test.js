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
import { api } from '../api.js'

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
    { name: 'gen-a', description: 'gen desc', size_gb: 5.2, recommended: true },
  ],
  llm_vision: [
    { name: 'vis-b', description: 'vis desc', size_gb: 6.1 },
  ],
  sd_models: [
    { name: 'sd-a', base: 'SDXL', category: 'Realistic', description: 'sd a desc', size_gb: 6.6, filename: 'sd-a.safetensors', url: 'http://x/sd-a.safetensors' },
    { name: 'sd-b', base: 'Flux', category: 'Realistic', description: 'sd b desc', size_gb: 12, recommended: true, filename: 'sd-b.safetensors', url: 'http://x/sd-b.safetensors' },
  ],
  lora: [
    { name: 'lora-a', base: 'SDXL', category: 'Style', description: 'lora desc', size_mb: 217, filename: 'lora-a.safetensors', url: 'http://x/lora-a.safetensors' },
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
  return w.findAll('.cat-row').find(r => r.find('.cat-name').exists() && r.find('.cat-name').text().trim() === name)
}

function badgeTexts(row, cls) {
  return row.findAll('.badge' + cls).map(b => b.text().trim())
}

function buttonTexts(row) {
  return row.findAll('button').map(b => b.text().trim())
}

describe.each([['ru', ru], ['en', en]])('catalog rows (%s)', (locale, dict) => {
  it('every catalog model is rendered as a .cat-row with .cat-name', async () => {
    const w = await openCatalog(locale)
    for (const name of ['gen-a', 'vis-b', 'sd-a', 'sd-b', 'lora-a']) {
      expect(rowOf(w, name), name).toBeTruthy()
    }
  })

  it('installed model: success badge and quiet danger delete button', async () => {
    const w = await openCatalog(locale)
    for (const name of ['gen-a', 'sd-a']) {
      const row = rowOf(w, name)
      expect(row, name).toBeTruthy()
      expect(badgeTexts(row, '.badge-success'), name).toContain(dict['settings.installed'])
      const del = row.findAll('button').find(b => b.text().trim() === dict['settings.btn_delete_model'])
      expect(del, name).toBeTruthy()
      for (const c of ['btn', 'btn-sm', 'btn-danger-quiet']) expect(del.classes(), name + ' ' + c).toContain(c)
    }
  })

  it('not installed model: no installed badge, download / pull button', async () => {
    const w = await openCatalog(locale)
    for (const name of ['sd-b', 'lora-a']) {
      const row = rowOf(w, name)
      expect(row, name).toBeTruthy()
      expect(badgeTexts(row, '.badge-success'), name).not.toContain(dict['settings.installed'])
      expect(buttonTexts(row), name).toContain(dict['settings.btn_download_model'])
      expect(buttonTexts(row), name).not.toContain(dict['settings.btn_delete_model'])
    }
    const vis = rowOf(w, 'vis-b')
    expect(vis).toBeTruthy()
    expect(buttonTexts(vis)).toContain(dict['settings.btn_pull_llm'])
    expect(buttonTexts(vis)).not.toContain(dict['settings.btn_delete_model'])
  })

  it('recommended model: accent badge; not recommended: none', async () => {
    const w = await openCatalog(locale)
    for (const name of ['gen-a', 'sd-b']) {
      expect(badgeTexts(rowOf(w, name), '.badge-accent'), name).toContain(dict['settings.recommended'])
    }
    for (const name of ['vis-b', 'sd-a', 'lora-a']) {
      expect(rowOf(w, name).text(), name).not.toContain(dict['settings.recommended'])
    }
  })

  it('base is a muted badge with the bare base name, no brackets', async () => {
    const w = await openCatalog(locale)
    for (const [name, base] of [['sd-a', 'SDXL'], ['sd-b', 'Flux'], ['lora-a', 'SDXL']]) {
      const row = rowOf(w, name)
      expect(badgeTexts(row, '.badge-muted'), name).toContain(base)
      expect(row.text(), name).not.toContain('[' + base + ']')
    }
  })

  it('no inline danger color inside catalog rows', async () => {
    const w = await openCatalog(locale)
    const rows = w.findAll('.cat-row')
    expect(rows.length).toBeGreaterThan(0)
    for (const r of rows) {
      for (const el of [r, ...r.findAll('*')]) {
        expect(el.attributes('style') || '', r.text()).not.toContain('color: var(--danger)')
      }
    }
  })

  it('download in progress: row shows progress bar instead of download button', async () => {
    const spy = vi.spyOn(api, 'downloadServerModelStream').mockImplementation((...args) => {
      const onProgress = args.find(a => typeof a === 'function')
      if (onProgress) onProgress('1 MB / 2 MB', 50)
      return new Promise(() => {})
    })
    const w = await openCatalog(locale)
    const btn = rowOf(w, 'sd-b').findAll('button').find(b => b.text().trim() === dict['settings.btn_download_model'])
    expect(btn).toBeTruthy()
    await btn.trigger('click')
    await settle()
    expect(spy).toHaveBeenCalled()
    const row = rowOf(w, 'sd-b')
    expect(row.find('.dl-progress').exists()).toBe(true)
    expect(buttonTexts(row)).not.toContain(dict['settings.btn_download_model'])
    spy.mockRestore()
  })
})
