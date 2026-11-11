import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

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
import GeneratePage from '../components/GeneratePage.vue'
import FoldBlock from '../components/FoldBlock.vue'
import { api } from '../api.js'
import {
  GetSettings, UpdateSettings, GetDefaultPromptInstruction, GetDefaultPromptInstructionProse,
  GetDefaultAnalyzePrompts, GetDefaultEditGarmentPrompt, GetActiveSessionItem, UpscaleImage,
} from '../wailsjs/go/main/App.js'
import { setLocale, ru, en } from '../i18n/index.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

const DEF = {
  sd_prompt_instruction: 'DEFAULT tags instruction',
  sd_prompt_instruction_prose: 'DEFAULT prose instruction',
  analyze_system_prompt: 'DEFAULT system prompt',
  analyze_prompt: 'DEFAULT tags analyze prompt',
  analyze_describe_prompt: 'DEFAULT describe prompt',
  edit_garment_prompt: 'DEFAULT garment prompt',
}

const IMAGE_KEYS = ['sd_prompt_instruction', 'sd_prompt_instruction_prose']
const PHOTO_KEYS = ['analyze_system_prompt', 'analyze_prompt', 'analyze_describe_prompt', 'edit_garment_prompt']
const PREVIEW_KEYS = ['preview_mode', 'preview_width', 'preview_height']

function stored(overrides = {}) {
  return {
    preview_mode: 'true',
    preview_width: '256',
    preview_height: '256',
    ...overrides,
  }
}

function setStored(s) {
  GetSettings.mockImplementation(() => Promise.resolve({ ...s }))
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetDefaultPromptInstruction.mockImplementation(() => Promise.resolve(DEF.sd_prompt_instruction))
  GetDefaultPromptInstructionProse.mockImplementation(() => Promise.resolve(DEF.sd_prompt_instruction_prose))
  GetDefaultAnalyzePrompts.mockImplementation(() => Promise.resolve({
    system_prompt: DEF.analyze_system_prompt,
    single_prompt: DEF.analyze_prompt,
    describe_prompt: DEF.analyze_describe_prompt,
  }))
  GetDefaultEditGarmentPrompt.mockImplementation(() => Promise.resolve(DEF.edit_garment_prompt))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  setStored(stored())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function tabButtons(w) {
  return w.findAll('.tabs button')
}

function tabByText(w, text) {
  return tabButtons(w).find(b => b.text().trim() === text)
}

async function mountSettings() {
  const w = mount(SettingsPage, { attachTo: document.body })
  await settle()
  return w
}

async function openPromptsTab(w) {
  const tab = tabByText(w, ru['settings.tab_prompts'])
  expect(tab, 'prompts tab').toBeTruthy()
  await tab.trigger('click')
  await settle()
}

function folds(w) {
  return w.findAllComponents(FoldBlock).map(f => {
    const ta = f.find('textarea')
    return { fold: f, textarea: ta.exists() ? ta : null, head: f.find('button') }
  })
}

function keyOfValue(value, values) {
  return Object.keys(values).find(k => values[k] === value)
}

function groupCard(w, titleKey) {
  const heading = w.findAll('*').find(el => el.element.children.length === 0 && el.text().trim() === ru[titleKey])
  expect(heading, 'group heading ' + ru[titleKey]).toBeTruthy()
  let el = heading.element.parentElement
  while (el && el.querySelectorAll('textarea').length === 0) el = el.parentElement
  return el
}

function foldsIn(container, w) {
  return folds(w).filter(f => container.contains(f.fold.element))
}

function buttonsOutsideFolds(container, w) {
  const foldEls = w.findAllComponents(FoldBlock).map(f => f.element)
  return Array.from(container.querySelectorAll('button')).filter(b => !foldEls.some(el => el.contains(b)))
}

describe('settings tabs', () => {
  it('has Connection, Prompts, Safety, Advanced and no Generation / Analyze tabs (ru)', async () => {
    const w = await mountSettings()
    const names = tabButtons(w).map(b => b.text().trim())
    expect(ru['settings.tab_prompts']).toBe('Промпты')
    expect(names).toContain('Подключение')
    expect(names).toContain('Промпты')
    expect(names).toContain('Безопасность')
    expect(names).toContain('Дополнительно')
    expect(names).not.toContain('Генерация')
    expect(names).not.toContain('Анализ')
    expect(names.filter(n => n === 'Промпты')).toHaveLength(1)
  })

  it('en locale also shows a single Prompts tab from settings.tab_prompts', async () => {
    setLocale('en')
    const w = await mountSettings()
    const names = tabButtons(w).map(b => b.text().trim())
    expect(en['settings.tab_prompts']).toBeTruthy()
    expect(names).toContain(en['settings.tab_prompts'])
    expect(names).not.toContain('Generation')
    expect(names).not.toContain('Analyze')
  })
})

describe('prompts tab: groups and fold blocks', () => {
  const custom = {
    sd_prompt_instruction: 'my tags instruction',
    sd_prompt_instruction_prose: DEF.sd_prompt_instruction_prose,
    analyze_system_prompt: DEF.analyze_system_prompt,
    analyze_prompt: 'my analyze tags',
    analyze_describe_prompt: DEF.analyze_describe_prompt,
    edit_garment_prompt: 'my garment',
  }

  it('image prompt group has two folds (tags, prose); describe photo group has four', async () => {
    setStored(stored(custom))
    const w = await mountSettings()
    await openPromptsTab(w)

    const imageGroup = groupCard(w, 'settings.group_image_prompt')
    const photoGroup = groupCard(w, 'settings.group_describe_photo')
    expect(imageGroup).not.toBe(photoGroup)

    const imgKeys = foldsIn(imageGroup, w).map(f => keyOfValue(f.textarea.element.value, custom))
    const photoKeys = foldsIn(photoGroup, w).map(f => keyOfValue(f.textarea.element.value, custom))
    expect(imgKeys.sort()).toEqual([...IMAGE_KEYS].sort())
    expect(photoKeys.sort()).toEqual([...PHOTO_KEYS].sort())
    expect(ru['settings.group_image_prompt']).toBe('Промпт для картинки')
    expect(ru['settings.group_describe_photo']).toBe('Описание фото')
  })

  it('every prompt fold is closed by default: textarea not visible', async () => {
    setStored(stored(custom))
    const w = await mountSettings()
    await openPromptsTab(w)
    const fs = folds(w)
    expect(fs).toHaveLength(6)
    for (const f of fs) {
      expect(f.textarea).toBeTruthy()
      expect(f.textarea.isVisible()).toBe(false)
      expect(f.head.attributes('aria-expanded')).toBe('false')
    }
  })

  it('click on fold header opens it and shows the textarea', async () => {
    setStored(stored(custom))
    const w = await mountSettings()
    await openPromptsTab(w)
    const f = folds(w)[0]
    await f.head.trigger('click')
    await settle()
    expect(f.textarea.isVisible()).toBe(true)
  })

  it('header shows title, hint line and state: default vs changed', async () => {
    setStored(stored(custom))
    const w = await mountSettings()
    await openPromptsTab(w)
    for (const f of folds(w)) {
      const key = keyOfValue(f.textarea.element.value, custom)
      const head = f.head.text()
      expect(f.fold.props('title')).toBeTruthy()
      expect(f.fold.props('hint')).toBeTruthy()
      expect(head).toContain(f.fold.props('title'))
      expect(head).toContain(f.fold.props('hint'))
      const isDefault = custom[key] === DEF[key]
      if (isDefault) {
        expect(head, key).toContain('по умолчанию')
        expect(head, key).not.toContain('изменён')
      } else {
        expect(head, key).toContain('изменён')
        expect(head, key).not.toContain('по умолчанию')
      }
    }
  })

  it('state switches to "изменён" after editing a default prompt', async () => {
    setStored(stored({ ...DEF }))
    const w = await mountSettings()
    await openPromptsTab(w)
    const f = folds(w).find(x => x.textarea.element.value === DEF.analyze_describe_prompt)
    expect(f.head.text()).toContain('по умолчанию')
    await f.textarea.setValue('something else')
    await settle()
    expect(f.head.text()).toContain('изменён')
  })

  it('garment fold title is "Одежда"', async () => {
    setStored(stored(custom))
    const w = await mountSettings()
    await openPromptsTab(w)
    const f = folds(w).find(x => x.textarea.element.value === custom.edit_garment_prompt)
    expect(f.fold.props('title')).toBe('Одежда')
  })
})

describe('describe photo group: save and reset include garment', () => {
  function groupButtons(w) {
    const photoGroup = groupCard(w, 'settings.group_describe_photo')
    const btns = buttonsOutsideFolds(photoGroup, w)
    const save = btns.find(b => b.textContent.trim() === 'Сохранить промпты')
    const reset = btns.find(b => b.textContent.trim() === 'Сбросить по умолчанию')
    return { photoGroup, save, reset }
  }

  it('single save button stands after all four folds, not between them', async () => {
    const w = await mountSettings()
    await openPromptsTab(w)
    const { photoGroup, save, reset } = groupButtons(w)
    expect(save, 'save button').toBeTruthy()
    expect(reset, 'reset button').toBeTruthy()
    const groupFolds = foldsIn(photoGroup, w)
    expect(groupFolds).toHaveLength(4)
    for (const f of groupFolds) {
      expect(f.fold.element.contains(save)).toBe(false)
      expect(f.fold.element.compareDocumentPosition(save) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
    const allSave = Array.from(photoGroup.querySelectorAll('button')).filter(b => b.textContent.trim() === 'Сохранить промпты')
    expect(allSave).toHaveLength(1)
  })

  it('save sends all four prompts including edited garment in one call', async () => {
    setStored(stored({
      analyze_system_prompt: 'sys A', analyze_prompt: 'tags A', analyze_describe_prompt: 'desc A', edit_garment_prompt: 'garment A',
    }))
    const w = await mountSettings()
    await openPromptsTab(w)
    const garment = folds(w).find(x => x.textarea.element.value === 'garment A')
    await garment.textarea.setValue('garment edited')
    const { save } = groupButtons(w)
    UpdateSettings.mockClear()
    save.click()
    await settle()
    expect(UpdateSettings).toHaveBeenCalledTimes(1)
    const data = UpdateSettings.mock.calls[0][0]
    expect(data).toMatchObject({
      analyze_system_prompt: 'sys A',
      analyze_prompt: 'tags A',
      analyze_describe_prompt: 'desc A',
      edit_garment_prompt: 'garment edited',
    })
    PREVIEW_KEYS.forEach(k => expect(data).not.toHaveProperty(k))
  })

  it('reset restores all four to defaults, garment from getDefaultEditGarmentPrompt', async () => {
    setStored(stored({
      analyze_system_prompt: 'sys A', analyze_prompt: 'tags A', analyze_describe_prompt: 'desc A', edit_garment_prompt: 'garment A',
    }))
    const w = await mountSettings()
    await openPromptsTab(w)
    expect(GetDefaultEditGarmentPrompt).toHaveBeenCalled()
    const { photoGroup, reset, save } = groupButtons(w)
    reset.click()
    await settle()
    const values = foldsIn(photoGroup, w).map(f => f.textarea.element.value).sort()
    expect(values).toEqual(PHOTO_KEYS.map(k => DEF[k]).sort())

    UpdateSettings.mockClear()
    save.click()
    await settle()
    expect(UpdateSettings).toHaveBeenCalledTimes(1)
    const data = UpdateSettings.mock.calls[0][0]
    PHOTO_KEYS.forEach(k => expect(data[k], k).toBe(''))
  })

  it('reset of describe group does not touch image prompt group', async () => {
    setStored(stored({ sd_prompt_instruction: 'my tags instruction', edit_garment_prompt: 'garment A' }))
    const w = await mountSettings()
    await openPromptsTab(w)
    const { reset } = groupButtons(w)
    reset.click()
    await settle()
    expect(folds(w).some(f => f.textarea.element.value === 'my tags instruction')).toBe(true)
  })

  it.each([
    ['missing', {}],
    ['empty string', { edit_garment_prompt: '' }],
  ])('garment %s in settings: field shows default and state "по умолчанию"', async (_n, extra) => {
    setStored(stored(extra))
    const w = await mountSettings()
    await openPromptsTab(w)
    const f = folds(w).find(x => x.fold.props('title') === 'Одежда')
    expect(f).toBeTruthy()
    expect(f.textarea.element.value).toBe(DEF.edit_garment_prompt)
    expect(f.head.text()).toContain('по умолчанию')
  })
})

describe('no preview mode in settings', () => {
  const PREVIEW_TEXTS = ['Генерация превью', 'Ширина превью', 'Высота превью', 'превью']

  async function visitAllTabs(w, fn) {
    const names = tabButtons(w).map(b => b.text().trim())
    for (const name of names) {
      await tabByText(w, name).trigger('click')
      await settle()
      await fn(name)
    }
  }

  it('no tab shows a preview toggle or preview width/height fields', async () => {
    const w = await mountSettings()
    await visitAllTabs(w, (name) => {
      const text = w.text().toLowerCase()
      PREVIEW_TEXTS.forEach(p => expect(text, `tab ${name}`).not.toContain(p.toLowerCase()))
    })
  })

  it('no save button on any tab sends preview_mode / preview_width / preview_height', async () => {
    const w = await mountSettings()
    await visitAllTabs(w, async () => {
      const saves = w.findAll('button').filter(b => /^Сохранить/.test(b.text().trim()))
      for (const b of saves) {
        await b.trigger('click')
        await settle()
      }
    })
    expect(UpdateSettings).toHaveBeenCalled()
    for (const [data] of UpdateSettings.mock.calls) {
      PREVIEW_KEYS.forEach(k => expect(data).not.toHaveProperty(k))
    }
  })

  it('api has no upscalePreview', () => {
    expect(api.upscalePreview).toBeUndefined()
    expect('upscalePreview' in api).toBe(false)
  })
})

describe('generate page without preview mode', () => {
  function previewButtons(w) {
    return w.findAll('button').filter(b => {
      const t = b.text().trim()
      return t === 'Апскейл до полного размера' || t === '← К превью' || t.includes('К превью')
    })
  }

  function x2(w) {
    return w.findAll('button').find(b => b.text().trim() === ru['generate.btn_upscale_x2'])
  }

  it('history item with is_preview=true still gets enabled "Апскейл x2", no preview buttons', async () => {
    GetActiveSessionItem.mockImplementation(() => Promise.resolve({
      id: 77, is_preview: true, is_active: true, info: '{"seed":1}', source: 'generate', created_at: '2025-01-01T00:00:00Z',
    }))
    const w = mount(GeneratePage)
    await settle()
    expect(w.findAll('img').some(i => i.attributes('alt') === 'Generated')).toBe(true)
    const b = x2(w)
    expect(b, 'upscale x2 button').toBeTruthy()
    expect(b.attributes('disabled')).toBeUndefined()
    expect(previewButtons(w)).toHaveLength(0)
  })

  it('generated result with is_preview=true still gets enabled "Апскейл x2", no preview buttons', async () => {
    const w = mount(GeneratePage)
    await settle()
    EventsEmit('queue:completed', { job_id: 'j1', result: JSON.stringify({ image_base64: 'AAAA', is_preview: true, info: '{}' }) })
    await settle()
    expect(w.findAll('img').some(i => i.attributes('alt') === 'Generated')).toBe(true)
    const b = x2(w)
    expect(b, 'upscale x2 button').toBeTruthy()
    expect(b.attributes('disabled')).toBeUndefined()
    expect(previewButtons(w)).toHaveLength(0)
  })

  it('click on "Апскейл x2" for an is_preview image calls the upscale binding', async () => {
    const w = mount(GeneratePage)
    await settle()
    EventsEmit('queue:completed', { job_id: 'j1', result: JSON.stringify({ image_base64: 'AAAA', is_preview: true, info: '{}' }) })
    await settle()
    await x2(w).trigger('click')
    await settle()
    expect(UpscaleImage).toHaveBeenCalledTimes(1)
  })

  it('no image shown: no preview buttons either', async () => {
    const w = mount(GeneratePage)
    await settle()
    expect(previewButtons(w)).toHaveLength(0)
  })
})
