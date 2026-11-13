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

import CatalogPage from '../components/CatalogPage.vue'
import { serverURL, serverModels, serverModelsKnown, modelActionLoading, resetActiveDownload } from '../serverModels.js'
import {
  GetSettings, UpdateSettings, GetModelCatalog, GetServerStatus, GetServerModels, GetServerLLMModels,
} from '../wailsjs/go/main/App.js'
import { setLocale, ru } from '../i18n/index.js'
import { api } from '../api.js'

enableAutoUnmount(afterEach)

function resetShared() {
  serverURL.value = ''
  for (const k of Object.keys(serverModels)) serverModels[k] = []
  for (const k of Object.keys(serverModelsKnown)) delete serverModelsKnown[k]
  for (const k of Object.keys(modelActionLoading)) delete modelActionLoading[k]
  resetActiveDownload()
}

async function settle() {
  for (let i = 0; i < 6; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

const CATALOG = {
  llm_generate: [],
  llm_vision: [],
  sd_models: [
    { name: 'sd-a', base: 'SDXL', category: 'Realistic', description: 'a', size_gb: 6.6 },
    { name: 'sd-multi', base: 'Flux', category: 'Realistic', description: 'b', size_gb: 9, url: 'https://x/sd-multi.gguf',
      modules: [{ type: 'vae', filename: 'multi-vae.safetensors', url: 'https://x/vae', size_gb: 0.3 }] },
  ],
  lora: [{ name: 'lora-a', base: 'SDXL', category: 'Style', description: 'l', size_mb: 217, url: 'https://x/l' }],
}

beforeEach(() => {
  vi.clearAllMocks()
  resetShared()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('confirm', () => false)
  vi.stubGlobal('alert', () => {})
  GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetModelCatalog.mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(CATALOG))))
  GetServerStatus.mockImplementation(() => Promise.resolve({}))
  GetServerModels.mockImplementation((type) => Promise.resolve(type === 'sd' ? [{ name: 'sd-a.safetensors' }] : []))
  GetServerLLMModels.mockImplementation(() => Promise.resolve([]))
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function openCatalog() {
  serverURL.value = 'http://srv:1'
  const w = mount(CatalogPage, { props: { active: false }, attachTo: document.body })
  await settle()
  expect(w.find('.page-title').text()).toBe(ru['app.nav_catalog'])
  await w.setProps({ active: true })
  await settle()
  return w
}

function rowOf(w, name) {
  const el = w.findAll('div').filter(d => d.text().includes(name) && (d.findAll('button').length > 0 || d.find('.dl-progress').exists()))
  return el[el.length - 1]
}

describe('catalog download progress', () => {
  it('shows the bar with percent and file step only in the downloading row', async () => {
    const pending = []
    vi.spyOn(api, 'downloadServerModelStream').mockImplementation((_s, _t, _u, filename, onProgress) => new Promise(resolve => {
      onProgress('1.0 GB / 4.0 GB', 25)
      pending.push({ filename, resolve })
    }))
    const w = await openCatalog()
    const btn = rowOf(w, 'sd-multi').findAll('button').find(b => b.text() === ru['settings.btn_download_model'])
    await btn.trigger('click')
    await settle()

    const row = rowOf(w, 'sd-multi')
    const bar = row.find('.dl-progress')
    expect(bar.exists()).toBe(true)
    expect(bar.find('.dl-progress-pct').text()).toBe('25%')
    expect(bar.find('.dl-progress-text').text()).toBe('1.0 GB / 4.0 GB')
    expect(bar.find('.dl-progress-label').text()).toBe('Файл 1 из 2: multi-vae.safetensors')
    expect(w.findAll('.dl-progress')).toHaveLength(1)

    pending[0].resolve()
    await settle()
    expect(rowOf(w, 'sd-multi').find('.dl-progress-label').text()).toBe('Файл 2 из 2: sd-multi.safetensors')
    expect(w.findAll('.dl-progress')).toHaveLength(1)

    pending[1].resolve()
    await settle()
    expect(w.findAll('.dl-progress')).toHaveLength(0)
  })

  it('removes the bar after a failed download', async () => {
    vi.spyOn(api, 'downloadServerModelStream').mockImplementation((_s, _t, _u, _f, onProgress) => {
      onProgress('', 10)
      return Promise.reject(new Error('boom'))
    })
    const w = await openCatalog()
    const btn = rowOf(w, 'lora-a').findAll('button').find(b => b.text() === ru['settings.btn_download_model'])
    await btn.trigger('click')
    await settle()
    expect(w.findAll('.dl-progress')).toHaveLength(0)
    expect(rowOf(w, 'lora-a').findAll('button').some(b => b.text() === ru['settings.btn_download_model'])).toBe(true)
  })
})
