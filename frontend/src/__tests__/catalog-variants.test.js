import { describe, it, expect } from 'vitest'
import { fitVariant, installedVariant, variantSwitchSteps, filterSDModels, modelBases } from '../catalogFilters.js'

const ZIMAGE = {
  name: 'z-image-turbo-fp8-aio',
  base: 'Z-Image',
  description: 'Fast photoreal turbo model',
  description_ru: 'Быстрая фотореалистичная модель',
  filename: 'z-image-turbo-fp8-aio.gguf',
  url: 'https://h/q4.gguf',
  size_gb: 3.86,
  modules: [
    { type: 'text_encoder', filename: 'Qwen3-4B-Q8_0.gguf', url: 'https://h/te.gguf', size_gb: 4.28 },
    { type: 'vae', filename: 'ae.safetensors', url: 'https://h/ae.safetensors', size_gb: 0.34 },
  ],
  variants: [
    { id: 'q8', label: 'Q8_0', filename: 'z_image_turbo-Q8_0.gguf', url: 'https://h/q8.gguf', size_gb: 6.58, vram_gb: 12.7, save_as: 'z-image-turbo-fp8-aio.gguf' },
    { id: 'q2', label: 'Q2_K', filename: 'z_image_turbo-Q2_K.gguf', url: 'https://h/q2.gguf', size_gb: 2.59, vram_gb: 8.7, save_as: 'z-image-turbo-fp8-aio.gguf' },
    { id: 'bf16', label: 'BF16', filename: 'z_image_turbo_bf16.safetensors', url: 'https://h/bf16.safetensors', size_gb: 12.31, vram_gb: 18.4, save_as: 'z-image-turbo-fp8-aio.safetensors' },
    { id: 'q4', label: 'Q4_K', filename: 'z_image_turbo-Q4_K.gguf', url: 'https://h/q4.gguf', size_gb: 3.86, vram_gb: 10.0, default: true, save_as: 'z-image-turbo-fp8-aio.gguf' },
  ],
}

const CHROMA = {
  name: 'Chroma1-HD',
  base: 'Chroma',
  description: 'Uncensored artistic model',
  filename: 'Chroma1-HD.gguf',
  size_gb: 7.65,
  modules: [],
  variants: [
    { id: 'q8', filename: 'Chroma1-HD-Q8_0.gguf', url: 'https://h/c8.gguf', size_gb: 9.74, vram_gb: 15.6, save_as: 'Chroma1-HD.gguf' },
    { id: 'q6', filename: 'Chroma1-HD-Q6_K.gguf', url: 'https://h/c6.gguf', size_gb: 7.65, vram_gb: 13.5, default: true, save_as: 'Chroma1-HD.gguf' },
  ],
}

const SDXL = { name: 'juggernautXL', base: 'SDXL', description: 'Realistic SDXL', description_ru: 'Реализм', size_gb: 6.6 }
const DREAM = { name: 'dreamshaper_8', base: 'SD 1.5', description: 'Versatile', size_gb: 2.0 }
const MODELS = [ZIMAGE, CHROMA, SDXL, DREAM]

const ids = (list) => list.map(m => m.name)
const GB = 1e9

describe('fitVariant', () => {
  it.each([
    ['16 GB card picks largest fitting', 16376, 'q8'],
    ['24 GB card picks largest', 24576, 'bf16'],
    ['exact boundary fits', 10240, 'q4'],
    ['one MB below boundary', 10239, 'q2'],
    ['too small picks smallest', 4000, 'q2'],
    ['unknown vram picks default', 0, 'q4'],
  ])('%s', (_, vram, want) => {
    expect(fitVariant(ZIMAGE, vram).id).toBe(want)
  })

  it('model without variants gives null', () => {
    expect(fitVariant(SDXL, 16376)).toBeNull()
    expect(fitVariant({ ...SDXL, variants: [] }, 0)).toBeNull()
  })
})

describe('installedVariant', () => {
  it.each([
    ['exact size', [{ name: 'z-image-turbo-fp8-aio.gguf', size: 6.58 * GB }], true, 'q8'],
    ['within 3 percent', [{ name: 'z-image-turbo-fp8-aio.gguf', size: 6.77 * GB }], true, 'q8'],
    ['other extension, other case', [{ name: 'Z-Image-Turbo-FP8-AIO.safetensors', size: 12.31 * GB }], true, 'bf16'],
    ['aio size matches nothing', [{ name: 'z-image-turbo-fp8-aio.safetensors', size: 10345190658 }], true, null],
    ['just outside 3 percent', [{ name: 'z-image-turbo-fp8-aio.gguf', size: 6.8 * GB }], true, null],
    ['no file', [{ name: 'dreamshaper_8.safetensors', size: 6.58 * GB }], false, null],
    ['prefix is not the model', [{ name: 'z-image-turbo-fp8-aio-old.gguf', size: 6.58 * GB }], false, null],
    ['empty list', [], false, null],
  ])('%s', (_, files, present, id) => {
    const r = installedVariant(ZIMAGE, files)
    expect(r.present).toBe(present)
    if (id === null) expect(r.variant).toBeNull()
    else expect(r.variant.id).toBe(id)
  })
})

describe('variantSwitchSteps', () => {
  const withModules = (sd) => ({
    sd,
    text_encoder: [{ name: 'Qwen3-4B-Q8_0.gguf', size: 4.28 * GB }],
    vae: [{ name: 'ae.safetensors', size: 0.34 * GB }],
  })
  const q8 = ZIMAGE.variants.find(v => v.id === 'q8')
  const bf16 = ZIMAGE.variants.find(v => v.id === 'bf16')

  it('same file name: delete first, then download', () => {
    const steps = variantSwitchSteps(ZIMAGE, q8, withModules([{ name: 'z-image-turbo-fp8-aio.gguf', size: 3.86 * GB }]))
    expect(steps).toHaveLength(2)
    expect(steps[0]).toMatchObject({ op: 'delete', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf' })
    expect(steps[1]).toEqual({ op: 'download', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf', url: q8.url })
  })

  it('different file name: download first, then delete the old file', () => {
    const steps = variantSwitchSteps(ZIMAGE, bf16, withModules([{ name: 'z-image-turbo-fp8-aio.gguf', size: 3.86 * GB }]))
    expect(steps).toHaveLength(2)
    expect(steps[0]).toEqual({ op: 'download', type: 'sd', filename: 'z-image-turbo-fp8-aio.safetensors', url: bf16.url })
    expect(steps[1]).toMatchObject({ op: 'delete', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf' })
  })

  it('unknown installed file under the model name is replaced too', () => {
    const steps = variantSwitchSteps(ZIMAGE, q8, withModules([{ name: 'z-image-turbo-fp8-aio.safetensors', size: 10345190658 }]))
    expect(steps.map(s => s.op)).toEqual(['download', 'delete'])
    expect(steps[1]).toMatchObject({ type: 'sd', filename: 'z-image-turbo-fp8-aio.safetensors' })
  })

  it('nothing installed: only download', () => {
    const steps = variantSwitchSteps(ZIMAGE, q8, withModules([{ name: 'dreamshaper_8.safetensors', size: 2 * GB }]))
    expect(steps).toEqual([{ op: 'download', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf', url: q8.url }])
  })

  it('missing modules are downloaded first', () => {
    const files = { sd: [{ name: 'z-image-turbo-fp8-aio.gguf', size: 3.86 * GB }], text_encoder: [], vae: [] }
    const steps = variantSwitchSteps(ZIMAGE, q8, files)
    expect(steps).toEqual([
      { op: 'download', type: 'text_encoder', filename: 'Qwen3-4B-Q8_0.gguf', url: 'https://h/te.gguf' },
      { op: 'download', type: 'vae', filename: 'ae.safetensors', url: 'https://h/ae.safetensors' },
      expect.objectContaining({ op: 'delete', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf' }),
      { op: 'download', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf', url: q8.url },
    ])
  })

  it('present module is not downloaded again', () => {
    const files = { sd: [], text_encoder: [{ name: 'Qwen3-4B-Q8_0.gguf', size: 1 }], vae: [] }
    const steps = variantSwitchSteps(ZIMAGE, q8, files)
    expect(steps).toEqual([
      { op: 'download', type: 'vae', filename: 'ae.safetensors', url: 'https://h/ae.safetensors' },
      { op: 'download', type: 'sd', filename: 'z-image-turbo-fp8-aio.gguf', url: q8.url },
    ])
  })
})

describe('filterSDModels', () => {
  const base = { query: '', base: '', installed: 'all', fits: false }
  const installedNames = new Set(['juggernautXL', 'Chroma1-HD'])
  const ctx = (vramMB) => ({ isInstalled: (m) => installedNames.has(m.name), vramMB })

  it('no filters keeps everything in order', () => {
    expect(ids(filterSDModels(MODELS, base, ctx(16376)))).toEqual(ids(MODELS))
  })

  it.each([
    ['name, case-insensitive', 'CHROMA', ['Chroma1-HD']],
    ['description', 'versatile', ['dreamshaper_8']],
    ['russian description', 'фотореалист', ['z-image-turbo-fp8-aio']],
    ['base', 'sdxl', ['juggernautXL']],
    ['nothing found', 'zzz-nothing', []],
  ])('query by %s', (_, query, want) => {
    expect(ids(filterSDModels(MODELS, { ...base, query }, ctx(0)))).toEqual(want)
  })

  it('base filter', () => {
    expect(ids(filterSDModels(MODELS, { ...base, base: 'Chroma' }, ctx(0)))).toEqual(['Chroma1-HD'])
  })

  it.each([
    ['yes', ['Chroma1-HD', 'juggernautXL']],
    ['no', ['z-image-turbo-fp8-aio', 'dreamshaper_8']],
    ['all', ids(MODELS)],
  ])('installed = %s', (installed, want) => {
    expect(ids(filterSDModels(MODELS, { ...base, installed }, ctx(0)))).toEqual(want)
  })

  it('fits keeps models whose smallest variant fits, models without variants by size + 1.5', () => {
    expect(ids(filterSDModels(MODELS, { ...base, fits: true }, ctx(9000)))).toEqual(['z-image-turbo-fp8-aio', 'juggernautXL', 'dreamshaper_8'])
    expect(ids(filterSDModels(MODELS, { ...base, fits: true }, ctx(8000)))).toEqual(['dreamshaper_8'])
    expect(ids(filterSDModels(MODELS, { ...base, fits: true }, ctx(3584)))).toEqual(['dreamshaper_8'])
    expect(ids(filterSDModels(MODELS, { ...base, fits: true }, ctx(3583)))).toEqual([])
    expect(ids(filterSDModels(MODELS, { ...base, fits: true }, ctx(13824)))).toEqual(ids(MODELS))
  })

  it('fits is ignored when vram is unknown', () => {
    expect(ids(filterSDModels(MODELS, { ...base, fits: true }, ctx(0)))).toEqual(ids(MODELS))
  })

  it('filters combine', () => {
    const f = { query: 'e', base: '', installed: 'no', fits: true }
    expect(ids(filterSDModels(MODELS, f, ctx(4000)))).toEqual(['dreamshaper_8'])
  })

  it('empty model list', () => {
    expect(filterSDModels([], base, ctx(16376))).toEqual([])
  })
})

describe('modelBases', () => {
  it('sorted unique bases', () => {
    expect(modelBases([...MODELS, { ...SDXL, name: 'other' }])).toEqual(['Chroma', 'SD 1.5', 'SDXL', 'Z-Image'])
  })

  it('empty list', () => {
    expect(modelBases([])).toEqual([])
  })
})

describe('review fixes', () => {
  it('same name differing only in case is deleted before download', () => {
    const q8 = ZIMAGE.variants.find(v => v.id === 'q8')
    const files = {
      sd: [{ name: 'Z-Image-Turbo-FP8-AIO.gguf', size: 3.86 * GB }],
      text_encoder: [{ name: 'Qwen3-4B-Q8_0.gguf' }],
      vae: [{ name: 'ae.safetensors' }],
    }
    expect(variantSwitchSteps(ZIMAGE, q8, files).map(s => s.op)).toEqual(['delete', 'download'])
  })

  it('fits for a model without versions counts its encoder and VAE', () => {
    const anima = {
      name: 'anima', base: 'Anima', size_gb: 4.18,
      modules: [
        { type: 'text_encoder', filename: 'te.safetensors', size_gb: 6.0 },
        { type: 'vae', filename: 'vae.safetensors', size_gb: 0.25 },
      ],
    }
    const f = { query: '', base: '', installed: 'all', fits: true }
    const ctx = (vramMB) => ({ isInstalled: () => false, vramMB })
    expect(filterSDModels([anima], f, ctx(7988)).length).toBe(1)
    expect(filterSDModels([anima], f, ctx(7987)).length).toBe(0)
  })
})
