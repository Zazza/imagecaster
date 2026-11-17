import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import CatalogPage from '../components/CatalogPage.vue'
import { serverURL, serverModels, serverModelsKnown, modelActionLoading, resetActiveDownload, loadServerModels } from '../serverModels.js'
import {
  GetSettings, UpdateSettings, GetModelCatalog, GetServerStatus, GetServerModels, GetServerLLMModels,
} from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { api } from '../api.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const GB = 1e9

const ZFILES = [
  { type: 'train', filename: 'z_image_turbo_bf16.safetensors', url: 'https://h/z_bf16.safetensors', size_gb: 12.3 },
  { type: 'train', filename: 'zimage_turbo_training_adapter_v2.safetensors', url: 'https://h/adapter.safetensors', size_gb: 0.2 },
  { type: 'text_encoder', filename: 'qwen_3_4b.safetensors', url: 'https://h/qwen.safetensors', size_gb: 8.1 },
  { type: 'vae', filename: 'ae.safetensors', url: 'https://h/ae.safetensors', size_gb: 0.3 },
]
const KFILES = [
  { type: 'sd', filename: 'flux-2-klein-4b.safetensors', url: 'https://h/klein.safetensors', size_gb: 7.8 },
  { type: 'text_encoder', filename: 'qwen_3_4b.safetensors', url: 'https://h/qwen.safetensors', size_gb: 8.1 },
  { type: 'vae', filename: 'flux2-klein-vae.safetensors', url: 'https://h/kvae.safetensors', size_gb: 0.4 },
]
const XFILES = [
  { type: 'sd', filename: 'juggernautXL_ragnarokBy.safetensors', url: 'https://h/jugg.safetensors', size_gb: 7.1 },
]

const CATALOG = {
  llm_generate: [],
  llm_vision: [],
  sd_models: [{ name: 'sd-a', base: 'SDXL', category: 'Realistic', description: 'a', size_gb: 6.6 }],
  lora: [],
  train: [
    { base: 'Z-Image', files: ZFILES },
    { base: 'SDXL', files: XFILES },
    { base: 'Flux.2', files: KFILES },
  ],
}

let serverFiles
let downloads

function setServer(files) {
  serverFiles = { sd: [], lora: [], vae: [], text_encoder: [], train: [], ...files }
}

function resetShared() {
  serverURL.value = ''
  for (const k of Object.keys(serverModels)) serverModels[k] = []
  for (const k of Object.keys(serverModelsKnown)) delete serverModelsKnown[k]
  for (const k of Object.keys(modelActionLoading)) delete modelActionLoading[k]
  resetActiveDownload()
}

beforeEach(() => {
  vi.clearAllMocks()
  resetShared()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  downloads = []
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.stubGlobal('confirm', vi.fn(() => false))
  vi.stubGlobal('alert', () => {})
  setServer({})
  GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetModelCatalog.mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(CATALOG))))
  GetServerStatus.mockImplementation(() => Promise.resolve({}))
  GetServerModels.mockImplementation((type) => Promise.resolve(JSON.parse(JSON.stringify(serverFiles[type] || []))))
  GetServerLLMModels.mockImplementation(() => Promise.resolve([]))
  const record = (type, url, filename) => {
    downloads.push({ type, url, filename })
    if (serverFiles[type]) serverFiles[type].push({ name: filename, size: GB })
    return Promise.resolve()
  }
  vi.spyOn(api, 'downloadServerModel').mockImplementation((type, url, filename) => record(type, url, filename))
  vi.spyOn(api, 'downloadServerModelStream').mockImplementation((_s, type, url, filename) => record(type, url, filename))
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const tid = (id) => `[data-testid=${id}]`

async function openTrainTab() {
  serverURL.value = 'http://srv:1'
  const w = mount(CatalogPage, { props: { active: false }, attachTo: document.body })
  await settle()
  await w.setProps({ active: true })
  await settle()
  const tab = w.find(tid('catalog-tab-train'))
  expect(tab.exists()).toBe(true)
  await tab.trigger('click')
  await settle()
  return w
}

function block(w, base) {
  return w.findAll(tid('catalog-train-base')).find(b => b.text().includes(base))
}

function fileRow(b, filename) {
  return b.findAll(tid('catalog-train-file')).find(f => f.text().includes(filename))
}

describe('serverModels: train type', () => {
  it('loads train files from the server like other types', async () => {
    setServer({ train: [{ name: 'z_image_turbo_bf16.safetensors', size: 12 * GB }] })
    await loadServerModels()
    expect(GetServerModels).toHaveBeenCalledWith('train')
    expect(serverModels.train.map(f => f.name)).toEqual(['z_image_turbo_bf16.safetensors'])
    expect(serverModelsKnown.train).toBe(true)
  })
})

describe('catalog: For training tab', () => {
  it('shows one block per training base with its label', async () => {
    const w = await openTrainTab()
    const blocks = w.findAll(tid('catalog-train-base'))
    expect(blocks).toHaveLength(3)
    expect(block(w, 'Z-Image')).toBeTruthy()
    expect(block(w, 'SDXL')).toBeTruthy()
    expect(block(w, 'Flux.2')).toBeTruthy()
  })

  it('lists every file of a base with its size', async () => {
    const w = await openTrainTab()
    const z = block(w, 'Z-Image')
    const rows = z.findAll(tid('catalog-train-file'))
    expect(rows).toHaveLength(ZFILES.length)
    for (const f of ZFILES) {
      const r = fileRow(z, f.filename)
      expect(r).toBeTruthy()
      expect(r.text()).toMatch(new RegExp(String(f.size_gb).replace('.', '[.,]')))
    }
  })

  it('marks files present on the server with class have, by type', async () => {
    setServer({
      train: [{ name: 'z_image_turbo_bf16.safetensors', size: 12 * GB }],
      vae: [{ name: 'ae.safetensors', size: 0.3 * GB }],
      text_encoder: [],
      sd: [{ name: 'zimage_turbo_training_adapter_v2.safetensors', size: GB }],
    })
    const w = await openTrainTab()
    const z = block(w, 'Z-Image')
    expect(fileRow(z, 'z_image_turbo_bf16.safetensors').classes()).toContain('have')
    expect(fileRow(z, 'ae.safetensors').classes()).toContain('have')
    expect(fileRow(z, 'qwen_3_4b.safetensors').classes()).not.toContain('have')
    expect(fileRow(z, 'zimage_turbo_training_adapter_v2.safetensors').classes()).not.toContain('have')
  })

  it('download button fetches only missing files with type, url and filename from the catalog', async () => {
    setServer({
      train: [{ name: 'z_image_turbo_bf16.safetensors', size: 12 * GB }],
      vae: [{ name: 'ae.safetensors', size: 0.3 * GB }],
    })
    const w = await openTrainTab()
    const z = block(w, 'Z-Image')
    expect(z.find(tid('catalog-train-ready')).exists()).toBe(false)
    const btn = z.find(tid('catalog-train-download'))
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    await settle()
    expect(downloads).toEqual([
      { type: 'train', url: 'https://h/adapter.safetensors', filename: 'zimage_turbo_training_adapter_v2.safetensors' },
      { type: 'text_encoder', url: 'https://h/qwen.safetensors', filename: 'qwen_3_4b.safetensors' },
    ])
  })

  it('downloads go one after another, not in parallel', async () => {
    let inFlight = 0
    let maxInFlight = 0
    const slow = (type, url, filename) => {
      inFlight++
      maxInFlight = Math.max(maxInFlight, inFlight)
      return new Promise(resolve => setTimeout(() => {
        inFlight--
        downloads.push({ type, url, filename })
        resolve()
      }, 5))
    }
    api.downloadServerModel.mockImplementation(slow)
    api.downloadServerModelStream.mockImplementation((_s, type, url, filename) => slow(type, url, filename))
    const w = await openTrainTab()
    await block(w, 'Flux.2').find(tid('catalog-train-download')).trigger('click')
    await new Promise(r => setTimeout(r, 60))
    await settle()
    expect(downloads.map(d => d.filename)).toEqual(KFILES.map(f => f.filename))
    expect(maxInFlight).toBe(1)
  })

  it('all files present: ready mark, no download button', async () => {
    setServer({ sd: [{ name: 'juggernautXL_ragnarokBy.safetensors', size: 7 * GB }] })
    const w = await openTrainTab()
    const x = block(w, 'SDXL')
    expect(x.find(tid('catalog-train-ready')).exists()).toBe(true)
    expect(x.find(tid('catalog-train-download')).exists()).toBe(false)
    expect(fileRow(x, 'juggernautXL_ragnarokBy.safetensors').classes()).toContain('have')
  })

  it('shared file counts for each base that needs it', async () => {
    setServer({
      sd: [{ name: 'flux-2-klein-4b.safetensors', size: 7 * GB }],
      text_encoder: [{ name: 'qwen_3_4b.safetensors', size: 8 * GB }],
      vae: [{ name: 'flux2-klein-vae.safetensors', size: 0.3 * GB }],
    })
    const w = await openTrainTab()
    expect(block(w, 'Flux.2').find(tid('catalog-train-ready')).exists()).toBe(true)
    const z = block(w, 'Z-Image')
    expect(fileRow(z, 'qwen_3_4b.safetensors').classes()).toContain('have')
    expect(z.find(tid('catalog-train-download')).exists()).toBe(true)
  })

  it('nothing on the server: every base offers download', async () => {
    const w = await openTrainTab()
    for (const base of ['Z-Image', 'SDXL', 'Flux.2']) {
      expect(block(w, base).find(tid('catalog-train-download')).exists()).toBe(true)
      expect(block(w, base).find(tid('catalog-train-ready')).exists()).toBe(false)
    }
  })

  it('catalog without train section: tab opens without blocks', async () => {
    const { train, ...rest } = CATALOG
    GetModelCatalog.mockImplementation(() => Promise.resolve(JSON.parse(JSON.stringify(rest))))
    const w = await openTrainTab()
    expect(w.findAll(tid('catalog-train-base'))).toHaveLength(0)
  })
})
