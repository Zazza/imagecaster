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

const GB = 1e9

function resetShared() {
  serverURL.value = ''
  for (const k of Object.keys(serverModels)) serverModels[k] = []
  for (const k of Object.keys(serverModelsKnown)) delete serverModelsKnown[k]
  for (const k of Object.keys(modelActionLoading)) delete modelActionLoading[k]
  resetActiveDownload()
}

async function settle() {
  for (let i = 0; i < 8; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

const ZIMAGE = {
  name: 'z-image-turbo-fp8-aio', base: 'Z-Image', category: 'Realistic', description: 'Fast photoreal turbo',
  filename: 'z-image-turbo-fp8-aio.gguf', url: 'https://h/q4.gguf', size_gb: 3.86,
  modules: [
    { type: 'text_encoder', filename: 'Qwen3-4B-Q8_0.gguf', url: 'https://h/te.gguf', size_gb: 4.28 },
    { type: 'vae', filename: 'ae.safetensors', url: 'https://h/ae.safetensors', size_gb: 0.34 },
  ],
  variants: [
    { id: 'q2', label: 'Q2_K', filename: 'z_image_turbo-Q2_K.gguf', url: 'https://h/q2.gguf', size_gb: 2.59, vram_gb: 8.7, save_as: 'z-image-turbo-fp8-aio.gguf' },
    { id: 'q4', label: 'Q4_K', filename: 'z_image_turbo-Q4_K.gguf', url: 'https://h/q4.gguf', size_gb: 3.86, vram_gb: 10.0, default: true, save_as: 'z-image-turbo-fp8-aio.gguf' },
    { id: 'q8', label: 'Q8_0', filename: 'z_image_turbo-Q8_0.gguf', url: 'https://h/q8.gguf', size_gb: 6.58, vram_gb: 12.7, save_as: 'z-image-turbo-fp8-aio.gguf' },
    { id: 'bf16', label: 'BF16', filename: 'z_image_turbo_bf16.safetensors', url: 'https://h/bf16.safetensors', size_gb: 12.31, vram_gb: 18.4, save_as: 'z-image-turbo-fp8-aio.safetensors' },
  ],
}

const CHROMA = {
  name: 'Chroma1-HD', base: 'Chroma', category: 'Art', description: 'Artistic model',
  filename: 'Chroma1-HD.gguf', url: 'https://h/c6.gguf', size_gb: 7.65,
  variants: [
    { id: 'c6', label: 'Q6_K', filename: 'Chroma1-HD-Q6_K.gguf', url: 'https://h/c6.gguf', size_gb: 7.65, vram_gb: 13.5, default: true, save_as: 'Chroma1-HD.gguf' },
    { id: 'c8', label: 'Q8_0', filename: 'Chroma1-HD-Q8_0.gguf', url: 'https://h/c8.gguf', size_gb: 9.74, vram_gb: 15.6, save_as: 'Chroma1-HD.gguf' },
  ],
}

const SDXL = { name: 'juggernautXL', base: 'SDXL', category: 'Realistic', description: 'Realistic SDXL', size_gb: 6.6, url: 'https://h/j.safetensors' }

const CATALOG = {
  llm_generate: [{ name: 'gen-a', description: 'gen desc', size_gb: 5.2 }],
  llm_vision: [],
  sd_models: [ZIMAGE, CHROMA, SDXL],
  lora: [{ name: 'lora-a', base: 'SDXL', category: 'Style', description: 'lora desc', size_mb: 217, url: 'https://h/l' }],
}

let serverFiles
let ops

function setServer({ vramMB, sd = [], text_encoder = [], vae = [] }) {
  serverFiles = { sd, text_encoder, vae, lora: [] }
  GetServerStatus.mockImplementation(() => Promise.resolve(vramMB ? { gpu: { memory_total_mb: vramMB, available: true } } : {}))
}

beforeEach(() => {
  vi.clearAllMocks()
  resetShared()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  ops = []
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('confirm', vi.fn(() => false))
  vi.stubGlobal('alert', () => {})
  setServer({ vramMB: 16376, sd: [{ name: 'juggernautXL.safetensors', size: 6.6 * GB }] })
  GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetModelCatalog.mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(CATALOG))))
  GetServerModels.mockImplementation((type) => Promise.resolve(JSON.parse(JSON.stringify(serverFiles[type] || []))))
  GetServerLLMModels.mockImplementation(() => Promise.resolve([]))
  vi.spyOn(api, 'downloadServerModel').mockImplementation((type, url, filename) => {
    ops.push({ op: 'download', type, filename, url })
    return Promise.resolve()
  })
  vi.spyOn(api, 'downloadServerModelStream').mockImplementation((_s, type, url, filename) => {
    ops.push({ op: 'download', type, filename, url })
    return Promise.resolve()
  })
  vi.spyOn(api, 'deleteServerModel').mockImplementation((type, filename) => {
    ops.push({ op: 'delete', type, filename })
    return Promise.resolve()
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function openCatalog() {
  serverURL.value = 'http://srv:1'
  const w = mount(CatalogPage, { props: { active: false }, attachTo: document.body })
  await settle()
  await w.setProps({ active: true })
  await settle()
  return w
}

const tid = (id) => `[data-testid=${id}]`

function rowOf(w, name) {
  return w.findAll('.cat-row').find(r => r.find('.cat-name').exists() && r.find('.cat-name').text().trim() === name)
}

function visibleNames(w) {
  return w.findAll('.cat-row').filter(r => r.isVisible()).map(r => r.find('.cat-name').text().trim())
}

function variantSelect(w, name) {
  return rowOf(w, name).find('select' + tid('catalog-variant'))
}

describe('catalog page: tabs and image filters', () => {
  it('images tab is active by default; other tabs are rendered but hidden', async () => {
    const w = await openCatalog()
    expect(visibleNames(w)).toEqual(['z-image-turbo-fp8-aio', 'Chroma1-HD', 'juggernautXL'])
    expect(rowOf(w, 'gen-a').exists()).toBe(true)
    expect(rowOf(w, 'gen-a').isVisible()).toBe(false)
    expect(rowOf(w, 'lora-a').isVisible()).toBe(false)

    await w.find(tid('catalog-tab-ai')).trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['gen-a'])

    await w.find(tid('catalog-tab-lora')).trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['lora-a'])

    await w.find(tid('catalog-tab-images')).trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['z-image-turbo-fp8-aio', 'Chroma1-HD', 'juggernautXL'])
  })

  it('search, family and reset', async () => {
    const w = await openCatalog()
    await w.find(tid('catalog-search')).setValue('CHROMA')
    await settle()
    expect(visibleNames(w)).toEqual(['Chroma1-HD'])

    await w.find(tid('catalog-search')).setValue('zzz-nothing')
    await settle()
    expect(visibleNames(w)).toEqual([])
    expect(w.find(tid('catalog-no-match')).exists()).toBe(true)
    expect(w.find(tid('catalog-no-match')).isVisible()).toBe(true)

    await w.find(tid('catalog-filter-reset')).trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['z-image-turbo-fp8-aio', 'Chroma1-HD', 'juggernautXL'])

    await w.find(tid('catalog-base')).setValue('Z-Image')
    await settle()
    expect(visibleNames(w)).toEqual(['z-image-turbo-fp8-aio'])
  })

  it('installed chips', async () => {
    const w = await openCatalog()
    await w.find(tid('catalog-installed-yes')).trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['juggernautXL'])
    await w.find(tid('catalog-installed-no')).trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['z-image-turbo-fp8-aio', 'Chroma1-HD'])
    await w.find(tid('catalog-installed-all')).trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['z-image-turbo-fp8-aio', 'Chroma1-HD', 'juggernautXL'])
  })

  it('fits chip filters by server VRAM', async () => {
    setServer({ vramMB: 8500, sd: [] })
    const w = await openCatalog()
    const fits = w.find(tid('catalog-fits'))
    expect(fits.exists()).toBe(true)
    await fits.trigger('click')
    await settle()
    expect(visibleNames(w)).toEqual(['juggernautXL'])
  })

  it('fits chip is not shown when VRAM is unknown', async () => {
    setServer({ vramMB: 0, sd: [] })
    const w = await openCatalog()
    const fits = w.find(tid('catalog-fits'))
    expect(fits.exists() && fits.isVisible()).toBe(false)
  })
})

describe('catalog page: model variants', () => {
  it('not installed: the select shows the fitting variant, non-fitting options are marked', async () => {
    const w = await openCatalog()
    const sel = variantSelect(w, 'z-image-turbo-fp8-aio')
    expect(sel.exists()).toBe(true)
    expect(sel.element.value).toBe('q8')
    const opts = Object.fromEntries(sel.findAll('option').map(o => [o.element.value, o.text()]))
    expect(opts.bf16).toContain(ru['settings.catalog_no_fit'])
    for (const id of ['q2', 'q4', 'q8']) expect(opts[id]).not.toContain(ru['settings.catalog_no_fit'])
    expect(variantSelect(w, 'Chroma1-HD').element.value).toBe('c8')
    expect(rowOf(w, 'juggernautXL').find('select' + tid('catalog-variant')).exists()).toBe(false)
  })

  it('server status unknown: the select shows the default variant', async () => {
    setServer({ vramMB: 0, sd: [] })
    const w = await openCatalog()
    expect(variantSelect(w, 'z-image-turbo-fp8-aio').element.value).toBe('q4')
    expect(variantSelect(w, 'Chroma1-HD').element.value).toBe('c6')
  })

  it('installed: the select shows the installed variant by file size, no replace button, no other-version badge', async () => {
    setServer({ vramMB: 16376, sd: [{ name: 'z-image-turbo-fp8-aio.gguf', size: 2.59 * GB }] })
    const w = await openCatalog()
    const row = rowOf(w, 'z-image-turbo-fp8-aio')
    expect(variantSelect(w, 'z-image-turbo-fp8-aio').element.value).toBe('q2')
    expect(row.find(tid('catalog-replace')).exists()).toBe(false)
    expect(row.find(tid('catalog-other-version')).exists()).toBe(false)
  })

  it('file with the model name but unknown size: other-version badge and replace offer', async () => {
    setServer({ vramMB: 16376, sd: [{ name: 'z-image-turbo-fp8-aio.safetensors', size: 10345190658 }] })
    const w = await openCatalog()
    const row = rowOf(w, 'z-image-turbo-fp8-aio')
    expect(row.find(tid('catalog-other-version')).exists()).toBe(true)
    expect(row.find(tid('catalog-replace')).exists()).toBe(true)
    expect(rowOf(w, 'Chroma1-HD').find(tid('catalog-other-version')).exists()).toBe(false)
  })

  it('replace is cancelled when the user does not confirm', async () => {
    setServer({ vramMB: 16376, sd: [{ name: 'z-image-turbo-fp8-aio.gguf', size: 2.59 * GB }] })
    const w = await openCatalog()
    await variantSelect(w, 'z-image-turbo-fp8-aio').setValue('q8')
    await settle()
    await rowOf(w, 'z-image-turbo-fp8-aio').find(tid('catalog-replace')).trigger('click')
    await settle()
    expect(window.confirm).toHaveBeenCalled()
    expect(ops).toEqual([])
  })

  it('replace with the same file name: missing modules, then delete, then download', async () => {
    setServer({ vramMB: 16376, sd: [{ name: 'z-image-turbo-fp8-aio.gguf', size: 2.59 * GB }], vae: [{ name: 'ae.safetensors', size: 0.34 * GB }] })
    window.confirm.mockImplementation(() => true)
    const w = await openCatalog()
    await variantSelect(w, 'z-image-turbo-fp8-aio').setValue('q8')
    await settle()
    await rowOf(w, 'z-image-turbo-fp8-aio').find(tid('catalog-replace')).trigger('click')
    await settle()
    expect(window.confirm).toHaveBeenCalled()
    expect(ops).toEqual([
      { op: 'download', type: 'text_encoder', filename: 'Qwen3-4B-Q8_0.gguf', url: 'https://h/te.gguf' },
      { op: 'delete', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf' },
      { op: 'download', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf', url: 'https://h/q8.gguf' },
    ])
  })

  it('replace with another file name: download first, then delete the old file', async () => {
    setServer({
      vramMB: 24576,
      sd: [{ name: 'z-image-turbo-fp8-aio.gguf', size: 6.58 * GB }],
      text_encoder: [{ name: 'Qwen3-4B-Q8_0.gguf', size: 4.28 * GB }],
      vae: [{ name: 'ae.safetensors', size: 0.34 * GB }],
    })
    window.confirm.mockImplementation(() => true)
    const w = await openCatalog()
    await variantSelect(w, 'z-image-turbo-fp8-aio').setValue('bf16')
    await settle()
    await rowOf(w, 'z-image-turbo-fp8-aio').find(tid('catalog-replace')).trigger('click')
    await settle()
    expect(ops).toEqual([
      { op: 'download', type: 'sd', filename: 'z-image-turbo-fp8-aio.safetensors', url: 'https://h/bf16.safetensors' },
      { op: 'delete', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf' },
    ])
  })
})
