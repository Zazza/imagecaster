import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import CatalogPage from '../components/CatalogPage.vue'
import { serverVersions, localVariant } from '../catalogFilters.js'
import { fastCacheFor } from '../fastMode.js'
import { serverURL, serverModels, serverModelsKnown, modelActionLoading, resetActiveDownload } from '../serverModels.js'
import {
  GetSettings, UpdateSettings, GetModelCatalog, GetServerStatus, GetServerModels, GetServerLLMModels,
} from '../wailsjs/go/main/App.js'
import { setLocale, ru } from '../i18n/index.js'
import { api } from '../api.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const GB = 1e9
const MAIN = 'qwen_image_2.1-Q8_0.gguf'
const Q4 = 'qwen_image_2.1-Q4_K.gguf'
const Q6 = 'qwen_image_2.1-Q6_K.gguf'

const QWEN = {
  name: 'qwen_image_2.1-Q8_0', base: 'Qwen-Image', category: 'Versatile', description: 'Qwen image',
  filename: MAIN, url: 'https://h/q8.gguf', size_gb: 7.69, fast_cache: 'easycache',
  modules: [
    { type: 'text_encoder', filename: 'Qwen3VL-8B-Instruct-Q4_K_M.gguf', url: 'https://h/te.gguf', size_gb: 5.0 },
    { type: 'vae', filename: 'qwen_image_2.1_vae_bf16.safetensors', url: 'https://h/vae.safetensors', size_gb: 0.68 },
  ],
  variants: [
    { id: 'q4_k', label: 'Q4_K', filename: Q4, url: 'https://h/q4.gguf', size_gb: 4.2, vram_gb: 6.4, save_as: MAIN },
    { id: 'q6_k', label: 'Q6_K', filename: Q6, url: 'https://h/q6.gguf', size_gb: 6.0, vram_gb: 8.2, save_as: MAIN },
    { id: 'q8_0', label: 'Q8_0', filename: MAIN, url: 'https://h/q8.gguf', size_gb: 7.69, vram_gb: 9.9, default: true, save_as: MAIN },
  ],
}

const CHROMA = {
  name: 'Chroma1-HD', base: 'Chroma', category: 'Art', description: 'Artistic model',
  filename: 'Chroma1-HD.gguf', url: 'https://h/c6.gguf', size_gb: 7.65, fast_cache: 'dbcache',
  variants: [
    { id: 'q4_k_s', label: 'Q4_K_S', filename: 'Chroma1-HD-Q4_K_S.gguf', url: 'https://h/c4.gguf', size_gb: 5.4, vram_gb: 11, save_as: 'Chroma1-HD.gguf' },
    { id: 'q6_k', label: 'Q6_K', filename: 'Chroma1-HD-Q6_K.gguf', url: 'https://h/c6.gguf', size_gb: 7.65, vram_gb: 13.5, default: true, save_as: 'Chroma1-HD.gguf' },
  ],
}

const KREA = {
  name: 'Krea-2-Turbo-Q4_K_M', base: 'Krea', category: 'Art', description: 'Krea',
  filename: 'Krea-2-Turbo-Q4_K_M.gguf', url: 'https://h/k4.gguf', size_gb: 5,
  variants: [
    { id: 'q4_k_m', label: 'Q4_K_M', filename: 'Krea-2-Turbo-Q4_K_M.gguf', url: 'https://h/k4.gguf', size_gb: 5, default: true, save_as: 'Krea-2-Turbo-Q4_K_M.gguf' },
    { id: 'q6_k', label: 'Q6_K', filename: 'Krea-2-Turbo-Q6_K.gguf', url: 'https://h/k6.gguf', size_gb: 7, save_as: 'Krea-2-Turbo-Q4_K_M.gguf' },
  ],
}

const SDXL = { name: 'juggernautXL', base: 'SDXL', category: 'Realistic', description: 'Realistic SDXL', size_gb: 6.6, url: 'https://h/j.safetensors' }

const fname = (f) => (typeof f === 'string' ? f : f?.name)

describe('serverVersions', () => {
  it('main file and a variant file: two entries, main first', () => {
    const files = [{ name: Q4, size: 4.2 * GB }, { name: MAIN, size: 7.69 * GB }]
    const r = serverVersions(QWEN, files)
    expect(r).toHaveLength(2)
    expect(fname(r[0].file)).toBe(MAIN)
    expect(r[0].main).toBe(true)
    expect(r[0].variant.id).toBe('q8_0')
    expect(fname(r[1].file)).toBe(Q4)
    expect(r[1].main).toBe(false)
    expect(r[1].variant.id).toBe('q4_k')
  })

  it('only the variant file: one entry, not main', () => {
    const r = serverVersions(QWEN, [{ name: Q4, size: 4.2 * GB }])
    expect(r).toHaveLength(1)
    expect(fname(r[0].file)).toBe(Q4)
    expect(r[0].main).toBe(false)
    expect(r[0].variant.id).toBe('q4_k')
  })

  it('main file with unknown size: variant is null', () => {
    const r = serverVersions(QWEN, [{ name: MAIN, size: 1.23 * GB }])
    expect(r).toHaveLength(1)
    expect(r[0].main).toBe(true)
    expect(r[0].variant).toBeNull()
  })

  it('variant file name in other case and extension is recognised', () => {
    const r = serverVersions(QWEN, [{ name: 'QWEN_IMAGE_2.1-q4_k.safetensors', size: 4.2 * GB }])
    expect(r).toHaveLength(1)
    expect(r[0].main).toBe(false)
    expect(r[0].variant.id).toBe('q4_k')
  })

  it('foreign files are not included', () => {
    const files = [
      { name: 'Chroma1-HD-Q4_K_S.gguf', size: 5.4 * GB },
      { name: 'qwen_image_2.1-Q4_K-old.gguf', size: 4.2 * GB },
      { name: 'juggernautXL.safetensors', size: 6.6 * GB },
      { name: Q6, size: 6.0 * GB },
    ]
    const r = serverVersions(QWEN, files)
    expect(r.map(v => fname(v.file))).toEqual([Q6])
  })

  it.each([
    ['empty list', []],
    ['no list', undefined],
  ])('%s gives empty array', (_, files) => {
    expect(serverVersions(QWEN, files)).toEqual([])
  })

  it('model without variants: only the main file', () => {
    const r = serverVersions(SDXL, [{ name: 'juggernautXL.safetensors', size: 6.6 * GB }, { name: Q4, size: 4.2 * GB }])
    expect(r).toHaveLength(1)
    expect(r[0].main).toBe(true)
    expect(fname(r[0].file)).toBe('juggernautXL.safetensors')
  })
})

describe('localVariant', () => {
  const q4 = QWEN.variants[0]
  const q6 = QWEN.variants[1]
  const q8 = QWEN.variants[2]

  it.each([
    ['variant file present', q4, [{ name: Q4, size: 4.2 * GB }], Q4],
    ['other case and extension', q4, [{ name: 'Qwen_Image_2.1-Q4_K.safetensors', size: 4.2 * GB }], 'Qwen_Image_2.1-Q4_K.safetensors'],
    ['other variant on server', q6, [{ name: Q4, size: 4.2 * GB }], null],
    ['variant named like the main file is not local', q8, [{ name: MAIN, size: 7.69 * GB }], null],
    ['empty list', q4, [], null],
  ])('%s', (_, variant, files, want) => {
    const r = localVariant(QWEN, variant, files)
    if (want === null) expect(r).toBeNull()
    else expect(fname(r)).toBe(want)
  })
})

describe('fastCacheFor with variant files', () => {
  const catalog = { sd_models: [QWEN, CHROMA, KREA, SDXL] }

  it.each([
    [Q4, 'easycache'],
    ['qwen_image_2.1-Q4_K', 'easycache'],
    ['QWEN_IMAGE_2.1-Q6_K.GGUF', 'easycache'],
    [MAIN, 'easycache'],
    ['Chroma1-HD-Q4_K_S.gguf', 'dbcache'],
    ['Krea-2-Turbo-Q6_K.gguf', ''],
    ['juggernautXL.safetensors', ''],
    ['unknown-model.gguf', ''],
    ['qwen_image_2.1-Q4_K-old.gguf', ''],
    ['', ''],
  ])('%s → %s', (model, want) => {
    expect(fastCacheFor(catalog, model)).toBe(want)
  })
})

function resetShared() {
  serverURL.value = ''
  for (const k of Object.keys(serverModels)) serverModels[k] = []
  for (const k of Object.keys(serverModelsKnown)) delete serverModelsKnown[k]
  for (const k of Object.keys(modelActionLoading)) delete modelActionLoading[k]
  resetActiveDownload()
}

const CATALOG = { llm_generate: [], llm_vision: [], sd_models: [QWEN, CHROMA, SDXL], lora: [] }

let serverFiles
let ops

function setServer({
  vramMB = 16376, sd = [],
  text_encoder = [{ name: 'Qwen3VL-8B-Instruct-Q4_K_M.gguf', size: 5.0 * GB }],
  vae = [{ name: 'qwen_image_2.1_vae_bf16.safetensors', size: 0.68 * GB }],
}) {
  serverFiles = {
    sd,
    text_encoder,
    vae,
    lora: [],
  }
  GetServerStatus.mockImplementation(() => Promise.resolve({ gpu: { memory_total_mb: vramMB, available: true } }))
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
  setServer({ sd: [] })
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

function versionItems(w, name) {
  return rowOf(w, name).findAll(tid('catalog-server-version'))
}

function versionItem(w, name, label) {
  return versionItems(w, name).find(i => i.text().includes(label))
}

function hasDownloadButton(row) {
  return row.findAll('button').some(b => b.text().trim() === ru['settings.btn_download_model'])
}

async function selectVariant(w, name, id) {
  await rowOf(w, name).find('select' + tid('catalog-variant')).setValue(id)
  await settle()
}

describe('catalog page: versions on the server', () => {
  it('main file and variant file: the row lists both, main first, with size and delete button', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }, { name: Q4, size: 4.2 * GB }] })
    const w = await openCatalog()
    const row = rowOf(w, 'qwen_image_2.1-Q8_0')
    expect(row.find(tid('catalog-server-versions')).exists()).toBe(true)
    const items = versionItems(w, 'qwen_image_2.1-Q8_0')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('Q8_0')
    expect(items[0].text()).toMatch(/7[.,]69?/)
    expect(items[1].text()).toContain('Q4_K')
    expect(items[1].text()).toMatch(/4[.,]2/)
    expect(items[0].find(tid('catalog-version-delete')).exists()).toBe(false)
    expect(items[1].find(tid('catalog-version-delete')).exists()).toBe(true)
  })

  it('only the variant file: the row lists just it', async () => {
    setServer({ sd: [{ name: Q4, size: 4.2 * GB }] })
    const w = await openCatalog()
    const items = versionItems(w, 'qwen_image_2.1-Q8_0')
    expect(items).toHaveLength(1)
    expect(items[0].text()).toContain('Q4_K')
    expect(items[0].find(tid('catalog-version-delete')).exists()).toBe(true)
  })

  it('nothing of the model on the server: no versions row', async () => {
    setServer({ sd: [{ name: 'Chroma1-HD-Q4_K_S.gguf', size: 5.4 * GB }] })
    const w = await openCatalog()
    const row = rowOf(w, 'qwen_image_2.1-Q8_0')
    expect(row.find(tid('catalog-server-versions')).exists()).toBe(false)
    expect(versionItems(w, 'Chroma1-HD')).toHaveLength(1)
  })

  it('delete confirmed: deletes exactly the variant file', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }, { name: Q4, size: 4.2 * GB }] })
    window.confirm.mockImplementation(() => true)
    const w = await openCatalog()
    await versionItem(w, 'qwen_image_2.1-Q8_0', 'Q4_K').find(tid('catalog-version-delete')).trigger('click')
    await settle()
    expect(window.confirm).toHaveBeenCalled()
    expect(api.deleteServerModel).toHaveBeenCalledWith('sd', Q4)
    expect(ops.filter(o => o.op === 'delete')).toEqual([{ op: 'delete', type: 'sd', filename: Q4 }])
    expect(ops.filter(o => o.op === 'download')).toEqual([])
  })

  it('main file is still deleted by the card delete button', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }, { name: Q4, size: 4.2 * GB }] })
    window.confirm.mockImplementation(() => true)
    const w = await openCatalog()
    const row = rowOf(w, 'qwen_image_2.1-Q8_0')
    const del = row.findAll('button').find(b => b.text().trim() === ru['settings.btn_delete_model'] && !b.attributes('data-testid'))
    expect(del).toBeTruthy()
    await del.trigger('click')
    await settle()
    expect(ops.filter(o => o.op === 'delete' && o.filename === MAIN)).toHaveLength(1)
    expect(ops.filter(o => o.op === 'delete' && o.filename === Q4)).toEqual([])
  })

  it('delete declined: nothing is deleted', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }, { name: Q4, size: 4.2 * GB }] })
    const w = await openCatalog()
    await versionItem(w, 'qwen_image_2.1-Q8_0', 'Q4_K').find(tid('catalog-version-delete')).trigger('click')
    await settle()
    expect(window.confirm).toHaveBeenCalled()
    expect(api.deleteServerModel).not.toHaveBeenCalled()
    expect(ops).toEqual([])
  })
})

describe('catalog page: selected variant already on the server', () => {
  it('main installed, Q4_K under its own name selected: no replace or download, hint shown', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }, { name: Q4, size: 4.2 * GB }] })
    const w = await openCatalog()
    await selectVariant(w, 'qwen_image_2.1-Q8_0', 'q4_k')
    const row = rowOf(w, 'qwen_image_2.1-Q8_0')
    expect(row.find(tid('catalog-replace')).exists()).toBe(false)
    expect(hasDownloadButton(row)).toBe(false)
    expect(row.find(tid('catalog-variant-local')).exists()).toBe(true)
    expect(row.find(tid('catalog-variant-local')).text()).toContain(Q4)
  })

  it('main installed, variant not on the server selected: replace offered, no hint', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }, { name: Q4, size: 4.2 * GB }] })
    const w = await openCatalog()
    await selectVariant(w, 'qwen_image_2.1-Q8_0', 'q6_k')
    const row = rowOf(w, 'qwen_image_2.1-Q8_0')
    expect(row.find(tid('catalog-replace')).exists()).toBe(true)
    expect(row.find(tid('catalog-variant-local')).exists()).toBe(false)
  })

  it('main missing, Q4_K on the server selected: no download, hint shown; other variant offers download', async () => {
    setServer({ sd: [{ name: Q4, size: 4.2 * GB }] })
    const w = await openCatalog()
    await selectVariant(w, 'qwen_image_2.1-Q8_0', 'q4_k')
    let row = rowOf(w, 'qwen_image_2.1-Q8_0')
    expect(hasDownloadButton(row)).toBe(false)
    expect(row.find(tid('catalog-replace')).exists()).toBe(false)
    expect(row.find(tid('catalog-variant-local')).exists()).toBe(true)

    await selectVariant(w, 'qwen_image_2.1-Q8_0', 'q6_k')
    row = rowOf(w, 'qwen_image_2.1-Q8_0')
    expect(hasDownloadButton(row)).toBe(true)
    expect(row.find(tid('catalog-variant-local')).exists()).toBe(false)
  })
})

describe('catalog page: installed badge for a version under its own name', () => {
  const TE = { name: 'Qwen3VL-8B-Instruct-Q4_K_M.gguf', size: 5 * GB }
  const VAE = { name: 'qwen_image_2.1_vae_bf16.safetensors', size: 0.68 * GB }

  it('variant file with all modules: installed badge names the version', async () => {
    setServer({ sd: [{ name: Q4, size: 4.2 * GB }], text_encoder: [TE], vae: [VAE] })
    const w = await openCatalog()
    const badge = rowOf(w, 'qwen_image_2.1-Q8_0').find(tid('catalog-installed-local'))
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain(ru['settings.installed'])
    expect(badge.text()).toContain('Q4_K')
  })

  it('variant file with a module missing: no installed badge', async () => {
    setServer({ sd: [{ name: Q4, size: 4.2 * GB }], text_encoder: [], vae: [VAE] })
    const w = await openCatalog()
    expect(rowOf(w, 'qwen_image_2.1-Q8_0').find(tid('catalog-installed-local')).exists()).toBe(false)
  })
})

describe('catalog page: model on the server with a module missing', () => {
  const TE = { name: 'Qwen3VL-8B-Instruct-Q4_K_M.gguf', size: 5 * GB }
  const VAE = { name: 'qwen_image_2.1_vae_bf16.safetensors', size: 0.68 * GB }

  it('main file present, VAE missing: download fetches only the VAE, the model file stays', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }], text_encoder: [TE], vae: [] })
    const w = await openCatalog()
    const btn = rowOf(w, 'qwen_image_2.1-Q8_0').find(tid('catalog-download-missing'))
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toBe(ru['settings.btn_download_model'])
    await btn.trigger('click')
    await settle()
    expect(ops).toEqual([{ op: 'download', type: 'vae', filename: VAE.name, url: 'https://h/vae.safetensors' }])
  })

  it('only a version under its own name, text encoder missing: download fetches only the encoder', async () => {
    setServer({ sd: [{ name: Q4, size: 4.2 * GB }], text_encoder: [], vae: [VAE] })
    const w = await openCatalog()
    const row = rowOf(w, 'qwen_image_2.1-Q8_0')
    await row.find(tid('catalog-download-missing')).trigger('click')
    await settle()
    expect(ops).toEqual([{ op: 'download', type: 'text_encoder', filename: TE.name, url: 'https://h/te.gguf' }])
  })

  it('all modules present: no missing-files button', async () => {
    setServer({ sd: [{ name: MAIN, size: 7.69 * GB }], text_encoder: [TE], vae: [VAE] })
    const w = await openCatalog()
    expect(rowOf(w, 'qwen_image_2.1-Q8_0').find(tid('catalog-download-missing')).exists()).toBe(false)
  })

  it('model not on the server: no missing-files button, the usual download stays', async () => {
    setServer({ sd: [], text_encoder: [], vae: [] })
    const w = await openCatalog()
    const row = rowOf(w, 'qwen_image_2.1-Q8_0')
    expect(row.find(tid('catalog-download-missing')).exists()).toBe(false)
    expect(hasDownloadButton(row)).toBe(true)
  })
})
