import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import LLMSetupModal from '../components/LLMSetupModal.vue'
import { GetSettings, SaveLLMChoice } from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { api } from '../api.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 6; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

function setupData(overrides = {}) {
  return {
    needed: true,
    generate: [
      { name: 'text-rec:8b', installed: true, recommended: true },
      { name: 'mytext:3b', installed: true, recommended: false },
      { name: 'text-new:4b', size_gb: 2.5, installed: false, recommended: false },
    ],
    vision: [
      { name: 'vis-rec:8b', installed: true, recommended: true },
      { name: 'vis-new:4b', size_gb: 3.3, installed: false, recommended: false },
      { name: 'vis-new2:9b', size_gb: 6.6, installed: false, recommended: false },
    ],
    generate_selected: '',
    vision_selected: '',
    ...overrides,
  }
}

let saveSpy
let pullSpy

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('confirm', () => false)
  vi.stubGlobal('alert', () => {})
  GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
  SaveLLMChoice.mockImplementation(() => Promise.resolve(undefined))
  saveSpy = vi.spyOn(api, 'saveLLMChoice').mockImplementation(() => Promise.resolve(undefined))
  pullSpy = vi.spyOn(api, 'pullServerLLMModelStream').mockImplementation(() => Promise.resolve(undefined))
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function mountModal(data) {
  const w = mount(LLMSetupModal, { props: { visible: true, setup: JSON.parse(JSON.stringify(data)) }, attachTo: document.body })
  await settle()
  return w
}

function selects(w) {
  const all = w.findAll('select')
  expect(all.length).toBeGreaterThanOrEqual(2)
  return { generate: all[0], vision: all[1] }
}

function optionOf(select, name) {
  const opt = select.findAll('option').find(o => o.text().includes(name))
  expect(opt, `option ${name}`).toBeTruthy()
  return opt
}

async function pick(select, name) {
  await select.setValue(optionOf(select, name).element.value)
  await settle()
}

function laterButton(w) {
  const b = w.findAll('button').find(x => x.text().includes('Позже'))
  expect(b, 'button «Позже»').toBeTruthy()
  return b
}

function saveButton(w) {
  const candidates = w.findAll('button').filter(b => {
    const t = b.text().trim()
    return t.length > 1 && !t.includes('Позже')
  })
  if (candidates.length > 1) {
    const primary = candidates.find(b => /сохран|скачать|продолж|готово|ok|применить/i.test(b.text()))
    if (primary) return primary
  }
  expect(candidates.length, 'save button').toBe(1)
  return candidates[0]
}

function isDisabled(b) {
  return b.element.disabled === true || b.attributes('disabled') !== undefined
}

describe('LLMSetupModal', () => {
  it('shows window title and both lists with installed and download marks', async () => {
    const w = await mountModal(setupData())
    expect(w.text()).toContain('Модели ИИ-помощника')
    expect(w.text()).toContain('Модель для промптов')
    expect(w.text()).toContain('Модель для фото')
    const { generate, vision } = selects(w)
    expect(optionOf(generate, 'mytext:3b').text()).toContain('установлена')
    expect(optionOf(generate, 'text-new:4b').text()).toMatch(/скачать.*2[.,]5\s*ГБ/)
    expect(optionOf(vision, 'vis-rec:8b').text()).toContain('установлена')
    expect(optionOf(vision, 'vis-new:4b').text()).toMatch(/скачать.*3[.,]3\s*ГБ/)
    expect(generate.findAll('option').some(o => o.text().includes('vis-new:4b'))).toBe(false)
    expect(vision.findAll('option').some(o => o.text().includes('mytext:3b'))).toBe(false)
  })

  it.each([
    ['both empty', '', ''],
    ['generate empty', '', 'vis-rec:8b'],
    ['vision empty', 'text-rec:8b', ''],
  ])('save button is disabled when selection is empty (%s)', async (_, g, v) => {
    const w = await mountModal(setupData({ generate_selected: g, vision_selected: v }))
    const btn = saveButton(w)
    expect(isDisabled(btn)).toBe(true)
    await btn.trigger('click')
    await settle()
    expect(saveSpy).not.toHaveBeenCalled()
    expect(pullSpy).not.toHaveBeenCalled()
  })

  it('button becomes active after the user picks both models', async () => {
    const w = await mountModal(setupData())
    const { generate, vision } = selects(w)
    await pick(generate, 'mytext:3b')
    expect(isDisabled(saveButton(w))).toBe(true)
    await pick(vision, 'vis-rec:8b')
    expect(isDisabled(saveButton(w))).toBe(false)
  })

  it('preselected models from setup make the button active', async () => {
    const w = await mountModal(setupData({ generate_selected: 'text-rec:8b', vision_selected: 'vis-rec:8b' }))
    expect(isDisabled(saveButton(w))).toBe(false)
    await saveButton(w).trigger('click')
    await settle()
    expect(saveSpy).toHaveBeenCalledWith('text-rec:8b', 'vis-rec:8b')
  })

  it('two installed models are saved without downloading and the window closes', async () => {
    const w = await mountModal(setupData())
    const { generate, vision } = selects(w)
    await pick(generate, 'mytext:3b')
    await pick(vision, 'vis-rec:8b')
    await saveButton(w).trigger('click')
    await settle()
    expect(pullSpy).not.toHaveBeenCalled()
    expect(saveSpy).toHaveBeenCalledTimes(1)
    expect(saveSpy).toHaveBeenCalledWith('mytext:3b', 'vis-rec:8b')
    expect(w.emitted('close')).toBeTruthy()
  })

  it('not installed model is downloaded first, then the choice is saved', async () => {
    const w = await mountModal(setupData())
    const { generate, vision } = selects(w)
    await pick(generate, 'mytext:3b')
    await pick(vision, 'vis-new:4b')
    await saveButton(w).trigger('click')
    await settle()
    expect(pullSpy).toHaveBeenCalledTimes(1)
    expect(pullSpy.mock.calls[0][1]).toBe('vis-new:4b')
    expect(typeof pullSpy.mock.calls[0][2]).toBe('function')
    expect(saveSpy).toHaveBeenCalledWith('mytext:3b', 'vis-new:4b')
    expect(pullSpy.mock.invocationCallOrder[0]).toBeLessThan(saveSpy.mock.invocationCallOrder[0])
  })

  it('preselected recommended model that is not installed is downloaded on save', async () => {
    const w = await mountModal(setupData({ generate_selected: 'text-new:4b', vision_selected: 'vis-rec:8b' }))
    await saveButton(w).trigger('click')
    await settle()
    expect(pullSpy).toHaveBeenCalledTimes(1)
    expect(pullSpy.mock.calls[0][1]).toBe('text-new:4b')
    expect(saveSpy).toHaveBeenCalledWith('text-new:4b', 'vis-rec:8b')
  })

  it('two missing models are downloaded one after another', async () => {
    const pending = []
    pullSpy.mockImplementation(() => new Promise(resolve => pending.push(resolve)))
    const w = await mountModal(setupData())
    const { generate, vision } = selects(w)
    await pick(generate, 'text-new:4b')
    await pick(vision, 'vis-new2:9b')
    await saveButton(w).trigger('click')
    await settle()
    expect(pullSpy).toHaveBeenCalledTimes(1)
    expect(saveSpy).not.toHaveBeenCalled()
    pending[0]()
    await settle()
    expect(pullSpy).toHaveBeenCalledTimes(2)
    expect(saveSpy).not.toHaveBeenCalled()
    pending[1]()
    await settle()
    expect(pullSpy.mock.calls.map(c => c[1]).sort()).toEqual(['text-new:4b', 'vis-new2:9b'])
    expect(saveSpy).toHaveBeenCalledWith('text-new:4b', 'vis-new2:9b')
  })

  it('same missing model for both lists is downloaded once', async () => {
    const data = setupData()
    data.vision.push({ name: 'text-new:4b', size_gb: 2.5, installed: false, recommended: false })
    const w = await mountModal(data)
    const { generate, vision } = selects(w)
    await pick(generate, 'text-new:4b')
    await pick(vision, 'text-new:4b')
    await saveButton(w).trigger('click')
    await settle()
    expect(pullSpy).toHaveBeenCalledTimes(1)
    expect(saveSpy).toHaveBeenCalledWith('text-new:4b', 'text-new:4b')
  })

  it('download error shows its text, settings are not saved and window stays', async () => {
    pullSpy.mockImplementation(() => Promise.reject(new Error('disk full on server')))
    const w = await mountModal(setupData())
    const { generate, vision } = selects(w)
    await pick(generate, 'mytext:3b')
    await pick(vision, 'vis-new:4b')
    await saveButton(w).trigger('click')
    await settle()
    expect(pullSpy).toHaveBeenCalled()
    expect(w.text()).toContain('disk full on server')
    expect(saveSpy).not.toHaveBeenCalled()
    expect(SaveLLMChoice).not.toHaveBeenCalled()
    expect(w.emitted('close')).toBeFalsy()
  })

  it('«Позже» closes without saving or downloading', async () => {
    const w = await mountModal(setupData({ generate_selected: 'text-new:4b', vision_selected: 'vis-rec:8b' }))
    await laterButton(w).trigger('click')
    await settle()
    expect(w.emitted('close')).toBeTruthy()
    expect(saveSpy).not.toHaveBeenCalled()
    expect(SaveLLMChoice).not.toHaveBeenCalled()
    expect(pullSpy).not.toHaveBeenCalled()
  })

  it('shows a progress bar with model name and percent while downloading', async () => {
    const progress = []
    const pending = []
    pullSpy.mockImplementation((_url, name, onProgress) => {
      progress.push(onProgress)
      return new Promise(resolve => pending.push(resolve))
    })
    const w = await mountModal(setupData())
    expect(w.find('[role="progressbar"]').exists()).toBe(false)
    const { generate, vision } = selects(w)
    await pick(generate, 'text-new:4b')
    await pick(vision, 'vis-new2:9b')
    await saveButton(w).trigger('click')
    await settle()
    expect(pullSpy).toHaveBeenCalledTimes(1)
    const first = pullSpy.mock.calls[0][1]
    progress[0]('1.0 GB / 4.0 GB', 37)
    await settle()
    let bar = w.find('[role="progressbar"]')
    expect(bar.exists()).toBe(true)
    expect(bar.text()).toContain(first)
    expect(bar.text()).toContain('37%')
    pending[0]()
    await settle()
    expect(pullSpy).toHaveBeenCalledTimes(2)
    const second = pullSpy.mock.calls[1][1]
    expect(second).not.toBe(first)
    progress[1]('0.5 GB / 6.0 GB', 8)
    await settle()
    bar = w.find('[role="progressbar"]')
    expect(bar.exists()).toBe(true)
    expect(bar.text()).toContain(second)
    expect(bar.text()).toContain('8%')
    expect(bar.text()).not.toContain('37%')
    expect(saveSpy).not.toHaveBeenCalled()
  })
})
