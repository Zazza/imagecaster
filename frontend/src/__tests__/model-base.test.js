import { describe, it, expect } from 'vitest'
import { modelBaseFor } from '../fastMode.js'

const ZIMAGE = {
  name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf',
  variants: [
    { filename: 'z_image_turbo-Q4_K.gguf' },
    { filename: 'z_image_turbo-Q8_0.gguf' },
    { filename: 'z_image_turbo_bf16.safetensors' },
  ],
}
const FLUX2 = {
  name: 'flux-2-klein-4b', base: 'Flux.2',
  variants: [
    { filename: 'flux-2-klein-4b-Q4_0.gguf' },
    { filename: 'flux-2-klein-4b.safetensors' },
  ],
}
const SDXL = { name: 'juggernautXL_ragnarokBy', base: 'SDXL' }
const NOBASE = { name: 'mystery-model', filename: 'mystery-model.gguf' }

const catalog = { sd_models: [ZIMAGE, FLUX2, SDXL, NOBASE] }

describe('modelBaseFor', () => {
  it.each([
    ['main filename', 'z-image-turbo-fp8-aio.gguf', 'Z-Image'],
    ['stem without extension', 'z-image-turbo-fp8-aio', 'Z-Image'],
    ['upper case', 'Z-IMAGE-TURBO-FP8-AIO.GGUF', 'Z-Image'],
    ['path prefix', 'models/z-image-turbo-fp8-aio.gguf', 'Z-Image'],
    ['hash suffix', 'z-image-turbo-fp8-aio.gguf [abc123]', 'Z-Image'],
    ['gguf variant', 'z_image_turbo-Q4_K.gguf', 'Z-Image'],
    ['safetensors variant', 'z_image_turbo_bf16.safetensors', 'Z-Image'],
    ['variant stem', 'z_image_turbo-Q8_0', 'Z-Image'],
    ['name without filename', 'flux-2-klein-4b.safetensors', 'Flux.2'],
    ['variant of model without filename', 'flux-2-klein-4b-Q4_0.gguf', 'Flux.2'],
    ['sdxl by name', 'juggernautXL_ragnarokBy.safetensors', 'SDXL'],
    ['model without base', 'mystery-model.gguf', ''],
    ['unknown model', 'unknown-model.gguf', ''],
    ['similar but longer name', 'z_image_turbo-Q4_K-old.gguf', ''],
    ['empty model', '', ''],
    ['null model', null, ''],
  ])('%s: %s → %s', (_, model, want) => {
    expect(modelBaseFor(catalog, model)).toBe(want)
  })

  it.each([
    ['null catalog', null],
    ['undefined catalog', undefined],
    ['empty catalog', {}],
    ['empty model list', { sd_models: [] }],
  ])('%s → empty base', (_, cat) => {
    expect(modelBaseFor(cat, 'z-image-turbo-fp8-aio.gguf')).toBe('')
  })
})
