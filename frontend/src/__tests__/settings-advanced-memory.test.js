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
import { GetSettings, UpdateSettings } from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

const UNLOAD_LABEL = 'Выгружать ИИ-помощника, если не хватает видеопамяти'

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
  setStored({ heavy_models: 'flux,moss', prose_models: 'flux,z-image', llm_auto_unload: 'true' })
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

function memoryCard(w) {
  const heading = w.findAll('h1,h2,h3,h4').find(h => h.text().trim() === 'Видеопамять')
  expect(heading, 'section heading "Видеопамять"').toBeTruthy()
  let el = heading.element.parentElement
  while (el && !el.querySelector('button')) el = el.parentElement
  return el
}

function unloadCheckbox(card) {
  const label = Array.from(card.querySelectorAll('label')).find(l => l.textContent.includes(UNLOAD_LABEL))
  expect(label, 'auto-unload label').toBeTruthy()
  const input = label.querySelector('input[type="checkbox"]')
  expect(input, 'auto-unload checkbox').toBeTruthy()
  return input
}

function saveButton(card) {
  const btn = Array.from(card.querySelectorAll('button')).find(b => /Сохранить/.test(b.textContent))
  expect(btn, 'save button in memory section').toBeTruthy()
  return btn
}

describe('advanced tab: memory section without model lists', () => {
  it('has no heavy/prose model fields', async () => {
    const w = await openAdvanced()
    expect(w.find('[data-testid="settings-heavy-models"]').exists()).toBe(false)
    expect(w.find('[data-testid="settings-prose-models"]').exists()).toBe(false)
    const text = w.text()
    expect(text).not.toContain('Тяжёлые модели')
    expect(text).not.toContain('Модели с прозой')
    expect(text).not.toContain('CSV')
    expect(text).not.toContain('Подстроки')
    const values = w.findAll('input, textarea').map(i => i.element.value)
    expect(values).not.toContain('flux,moss')
    expect(values).not.toContain('flux,z-image')
  })

  it('shows "Видеопамять" heading and the auto-unload checkbox', async () => {
    const w = await openAdvanced()
    const card = memoryCard(w)
    expect(card.textContent).toContain(UNLOAD_LABEL)
    expect(unloadCheckbox(card).checked).toBe(true)
  })

  it('save sends only llm_auto_unload', async () => {
    const w = await openAdvanced()
    const card = memoryCard(w)
    UpdateSettings.mockClear()
    saveButton(card).click()
    await settle()
    expect(UpdateSettings).toHaveBeenCalledTimes(1)
    expect(UpdateSettings.mock.calls[0][0]).toEqual({ llm_auto_unload: 'true' })
  })

  it('unchecked box saves llm_auto_unload=false, still no model lists', async () => {
    const w = await openAdvanced()
    const card = memoryCard(w)
    const box = unloadCheckbox(card)
    box.click()
    await settle()
    expect(box.checked).toBe(false)
    UpdateSettings.mockClear()
    saveButton(card).click()
    await settle()
    expect(UpdateSettings).toHaveBeenCalledTimes(1)
    const data = UpdateSettings.mock.calls[0][0]
    expect(data).toEqual({ llm_auto_unload: 'false' })
    expect(data).not.toHaveProperty('heavy_models')
    expect(data).not.toHaveProperty('prose_models')
  })

  it('stored llm_auto_unload=false shows the box unchecked', async () => {
    setStored({ llm_auto_unload: 'false', heavy_models: 'flux' })
    const w = await openAdvanced()
    expect(unloadCheckbox(memoryCard(w)).checked).toBe(false)
  })
})
