import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import SettingsPage from '../components/SettingsPage.vue'
import {
  GetSettings, UpdateSettings, ListPresets, ListCompoundPresets, GetModelCatalog,
  GetSDModels, GetSDLoRAs, GetSDSamplers, GetSDSchedulers,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const SDXL = 'juggernautXL_ragnarokBy.safetensors'
const FLUX = 'flux1-dev-fp8.safetensors'

const CATALOG = {
  llm_generate: [],
  llm_vision: [],
  lora: [],
  sd_models: [
    { name: 'juggernautXL_ragnarokBy', base: 'SDXL' },
    { name: 'flux1-dev-fp8', base: 'Flux.1', fast_cache: 'easycache' },
    { name: 'moss_image_sft_bf16', base: 'Moss-Image', fast_cache: 'dbcache' },
  ],
}

const PRESETS = [
  { id: 1, name: 'Xl', preset_type: 'general', loras: '', model_name: SDXL },
  { id: 2, name: 'Fl', preset_type: 'general', loras: '', model_name: FLUX },
]
const WORKFLOW = { id: 10, name: 'Flow', steps: [{ preset_id: 1, denoising_strength: 0 }, { preset_id: 2, denoising_strength: 0.4 }] }

let stored

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  stored = {}
  GetSettings.mockImplementation(() => Promise.resolve({ ...stored }))
  UpdateSettings.mockImplementation(d => { Object.assign(stored, d); return Promise.resolve(undefined) })
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  ListCompoundPresets.mockImplementation(() => Promise.resolve([structuredClone(WORKFLOW)]))
  GetModelCatalog.mockImplementation(() => Promise.resolve(structuredClone(CATALOG)))
  GetSDModels.mockImplementation(() => Promise.resolve([{ title: SDXL }, { title: FLUX }]))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
  GetSDSamplers.mockImplementation(() => Promise.resolve(['Euler']))
  GetSDSchedulers.mockImplementation(() => Promise.resolve([]))
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function mountGenerate(s) {
  Object.assign(stored, s)
  const w = mount(GeneratePage, { attachTo: document.body })
  await settle()
  return w
}

async function mountAdvanced() {
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  const tab = w.findAll('.tabs button').find(b => b.text().trim() === 'Дополнительно')
  expect(tab, 'advanced tab').toBeTruthy()
  await tab.trigger('click')
  await settle()
  return w
}

function fastButton(w) {
  const b = w.find('[data-testid="gen-fast"]')
  expect(b.exists(), 'fast button').toBe(true)
  return b
}

function pressed(w) {
  return fastButton(w).attributes('aria-pressed') === 'true'
}

function settingsInput(w) {
  const toggle = w.find('[data-testid="settings-fast-generation"]')
  expect(toggle.exists(), 'settings toggle').toBe(true)
  return toggle.element.matches('input') ? toggle.element : toggle.element.querySelector('input[type="checkbox"]')
}

function fastCalls() {
  return UpdateSettings.mock.calls.map(c => c[0]).filter(d => d && 'fast_generation' in d)
}

const FLUX_STYLE = { gen_mode: 'preset', gen_preset_id: '2' }
const SDXL_STYLE = { gen_mode: 'preset', gen_preset_id: '1' }

describe('generate: fast button placement', () => {
  it('sits in the same row as the generate button', async () => {
    const w = await mountGenerate(FLUX_STYLE)
    const gen = w.find('.btn-generate')
    expect(gen.exists()).toBe(true)
    expect(fastButton(w).text()).toContain(t('fast.button'))
    expect(gen.element.parentElement.contains(fastButton(w).element)).toBe(true)
  })
})

describe('generate: fast button shows and saves the shared setting', () => {
  it.each([
    ['stored true', { fast_generation: 'true' }, true],
    ['stored false', { fast_generation: 'false' }, false],
    ['not stored', {}, false],
  ])('%s', async (_, s, want) => {
    const w = await mountGenerate({ ...FLUX_STYLE, ...s })
    expect(pressed(w)).toBe(want)
  })

  it.each([
    ['off -> on saves true', 'false', 'true', true],
    ['on -> off saves false', 'true', 'false', false],
  ])('%s', async (_, from, want, after) => {
    const w = await mountGenerate({ ...FLUX_STYLE, fast_generation: from })
    UpdateSettings.mockClear()
    await fastButton(w).trigger('click')
    await settle()
    const calls = fastCalls()
    expect(calls.length).toBe(1)
    expect(calls[0].fast_generation).toBe(want)
    expect(pressed(w)).toBe(after)
  })

  it('save error rolls back and shows a message', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    const w = await mountGenerate({ ...FLUX_STYLE, fast_generation: 'false' })
    UpdateSettings.mockImplementation(() => Promise.reject(new Error('disk full')))
    await fastButton(w).trigger('click')
    await settle()
    expect(pressed(w)).toBe(false)
    expect(w.text()).toContain('disk full')
    expect(err).toHaveBeenCalled()
  })
})

describe('fast setting: settings page and button stay in sync', () => {
  it('settings toggle changes the button without remount', async () => {
    const g = await mountGenerate({ ...FLUX_STYLE, fast_generation: 'false' })
    const s = await mountAdvanced()
    expect(settingsInput(s).checked).toBe(false)
    settingsInput(s).click()
    await settle()
    expect(stored.fast_generation).toBe('true')
    expect(pressed(g)).toBe(true)
  })

  it('button changes the settings toggle without remount', async () => {
    const g = await mountGenerate({ ...FLUX_STYLE, fast_generation: 'true' })
    const s = await mountAdvanced()
    expect(settingsInput(s).checked).toBe(true)
    await fastButton(g).trigger('click')
    await settle()
    expect(stored.fast_generation).toBe('false')
    expect(settingsInput(s).checked).toBe(false)
  })
})

describe('generate: fast button depends on the model', () => {
  it('style on a model without speed-up (SDXL): disabled with a hint', async () => {
    const w = await mountGenerate({ ...SDXL_STYLE, fast_generation: 'true' })
    const b = fastButton(w)
    expect(b.attributes('disabled')).toBeDefined()
    expect(b.attributes('title')).toBe(t('fast.unsupported'))
  })

  it('style on Flux.1: enabled', async () => {
    const w = await mountGenerate({ ...FLUX_STYLE, fast_generation: 'true' })
    const b = fastButton(w)
    expect(b.attributes('disabled')).toBeUndefined()
    expect(b.attributes('title')).not.toBe(t('fast.unsupported'))
    expect(pressed(w)).toBe(true)
  })

  it.each([
    ['compound', { gen_mode: 'compound', gen_compound_preset_id: '10', gen_preset_id: '1' }],
    ['custom chain', { gen_mode: 'custom', gen_preset_id: '1' }],
  ])('%s mode: enabled even if a step uses SDXL', async (_, s) => {
    const w = await mountGenerate(s)
    const label = s.gen_mode === 'compound' ? t('generate.btn_workflow') : t('generate.btn_custom')
    expect(w.findAll('button').find(b => b.text() === label).attributes('aria-pressed')).toBe('true')
    expect(fastButton(w).attributes('disabled')).toBeUndefined()
  })

  it('model override to Flux.1 on an SDXL style enables the button', async () => {
    const w = await mountGenerate(SDXL_STYLE)
    expect(fastButton(w).attributes('disabled')).toBeDefined()
    await w.find('.preset-overrides .fold-head').trigger('click')
    await settle()
    const select = w.findAll('.preset-overrides select').find(sel => sel.findAll('option').some(o => o.element.value === FLUX))
    expect(select, 'model override select').toBeTruthy()
    await select.setValue(FLUX)
    await settle()
    expect(fastButton(w).attributes('disabled')).toBeUndefined()
  })

  it('model override to SDXL on a Flux.1 style disables the button', async () => {
    const w = await mountGenerate(FLUX_STYLE)
    expect(fastButton(w).attributes('disabled')).toBeUndefined()
    await w.find('.preset-overrides .fold-head').trigger('click')
    await settle()
    const select = w.findAll('.preset-overrides select').find(sel => sel.findAll('option').some(o => o.element.value === SDXL))
    await select.setValue(SDXL)
    await settle()
    expect(fastButton(w).attributes('disabled')).toBeDefined()
  })
})
