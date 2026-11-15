import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import SettingsPage from '../components/SettingsPage.vue'
import PresetForm from '../components/PresetForm.vue'
import { GetSettings, UpdateSettings, GetSDModels, GetSDLoRAs, GetSDSamplers } from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

function setStored(s) {
  GetSettings.mockImplementation(() => Promise.resolve({ ...s }))
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSDSamplers.mockImplementation(() => Promise.resolve(['Euler']))
  GetSDModels.mockImplementation(() => Promise.resolve([]))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
  setStored({})
})

afterEach(() => {
  vi.unstubAllGlobals()
})

async function openAdvanced() {
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  const tab = w.findAll('.tabs button').find(b => b.text().trim() === 'Дополнительно')
  expect(tab, 'advanced tab').toBeTruthy()
  await tab.trigger('click')
  await settle()
  return w
}

function fastInput(w) {
  const toggle = w.find('[data-testid="settings-fast-generation"]')
  expect(toggle.exists(), 'fast generation toggle').toBe(true)
  const input = toggle.element.matches('input') ? toggle.element : toggle.element.querySelector('input[type="checkbox"]')
  expect(input, 'toggle checkbox').toBeTruthy()
  return input
}

function fastCalls() {
  return UpdateSettings.mock.calls.map(c => c[0]).filter(d => d && 'fast_generation' in d)
}

describe('settings: fast generation toggle', () => {
  it.each([
    ['stored true', { fast_generation: 'true' }, true],
    ['stored false', { fast_generation: 'false' }, false],
    ['not stored is off', {}, false],
  ])('%s shows saved state', async (_, stored, want) => {
    setStored(stored)
    const w = await openAdvanced()
    expect(fastInput(w).checked).toBe(want)
  })

  it.each([
    ['off -> on saves true', 'false', 'true'],
    ['on -> off saves false', 'true', 'false'],
  ])('%s', async (_, stored, want) => {
    setStored({ fast_generation: stored })
    const w = await openAdvanced()
    UpdateSettings.mockClear()
    const input = fastInput(w)
    input.click()
    await settle()
    const calls = fastCalls()
    expect(calls.length).toBe(1)
    expect(calls[0].fast_generation).toBe(want)
    expect(input.checked).toBe(want === 'true')
  })

  it('has a hint next to the toggle', async () => {
    const w = await openAdvanced()
    const toggle = w.find('[data-testid="settings-fast-generation"]').element
    let el = toggle.parentElement
    while (el && !el.querySelector('.form-hint')) el = el.parentElement
    expect(el, 'hint near toggle').toBeTruthy()
    expect(el.querySelector('.form-hint').textContent.trim().length).toBeGreaterThan(0)
  })

  it('toggle area has no inline styles', async () => {
    const w = await openAdvanced()
    const toggle = w.find('[data-testid="settings-fast-generation"]').element
    const section = toggle.closest('section, .card, .settings-section') || toggle.parentElement.parentElement
    expect(section.hasAttribute('style')).toBe(false)
    expect(section.querySelectorAll('[style]').length).toBe(0)
  })
})

const basePreset = {
  id: 7,
  name: 'Fast style',
  preset_type: 'txt2img',
  prompt: 'cat',
  negative_prompt: '',
  sampler: 'Euler',
  steps: 20,
  cfg_scale: 7,
  model_name: '',
  loras: '[]',
}

async function mountForm(preset) {
  const w = mount(PresetForm, { props: { preset }, attachTo: document.body })
  await settle()
  return w
}

describe('preset form: fast mode select', () => {
  it('offers inherit, on and off', async () => {
    const w = await mountForm({ ...basePreset, fast_mode: '' })
    const values = w.find('[data-testid="preset-fast-mode"]').findAll('option').map(o => o.element.value)
    expect(values).toEqual(['', 'on', 'off'])
  })

  it.each([
    ['on', 'on'],
    ['off', 'off'],
    ['', ''],
    [undefined, ''],
  ])('saved fast_mode %s is shown as %s', async (stored, want) => {
    const p = { ...basePreset }
    if (stored !== undefined) p.fast_mode = stored
    const w = await mountForm(p)
    expect(w.find('[data-testid="preset-fast-mode"]').element.value).toBe(want)
  })

  it.each([
    ['', 'on'],
    ['on', 'off'],
    ['off', ''],
    ['on', 'on'],
  ])('from %s choosing %s emits it on save', async (stored, choice) => {
    const w = await mountForm({ ...basePreset, fast_mode: stored })
    await w.find('[data-testid="preset-fast-mode"]').setValue(choice)
    await w.find('form').trigger('submit')
    await settle()
    const saves = w.emitted('save')
    expect(saves, 'save emitted').toBeTruthy()
    expect(saves[saves.length - 1][0].fast_mode).toBe(choice)
  })

  it('select has no inline styles', async () => {
    const w = await mountForm({ ...basePreset, fast_mode: '' })
    const group = w.find('[data-testid="preset-fast-mode"]').element.parentElement
    expect(group.hasAttribute('style')).toBe(false)
    expect(group.querySelectorAll('[style]').length).toBe(0)
  })
})
