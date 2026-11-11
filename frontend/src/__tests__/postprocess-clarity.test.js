import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, EditImage: vi.fn(() => Promise.resolve(undefined)) }
})

import PostProcessPage from '../components/PostProcessPage.vue'
import RemixSection from '../components/RemixSection.vue'
import EditSection from '../components/EditSection.vue'
import HiresProfileSelector from '../components/HiresProfileSelector.vue'
import {
  GetSettings, UpdateSettings, GetSDLoRAs, GetSDModels, GetActiveSessionItem, GetSessionImage,
  ListPresets, ListPresetTypes, ListHiresProfiles, EnqueueFromImage,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t, setLocale, ru, en } from '../i18n/index.js'
import { modelLoraText, activeLoraCount } from '../modelLora.js'

enableAutoUnmount(afterEach)

const IMG = 'aW1n'
const SEP = ' · '
const TYPES = [{ id: 1, name: 'post' }]
const EFFECT = { id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null, loras: '' }
const STYLE_PLAIN = { id: 21, name: 'Plain Style', preset_type: 'post', type_id: 1, model_name: 'sdxl-base', prompt: 'p', loras: '' }
const STYLE_LORAS = {
  id: 22, name: 'Two Lora Style', preset_type: 'post', type_id: 1, model_name: 'sdxl-base', prompt: 'p',
  loras: '[{"name":"StyleA","weight":0.6},{"name":"StyleB","weight":0.8}]',
}

const SUB_KEYS = ['edit', 'remix', 'object', 'repair', 'effect', 'adjust', 'finish', 'export', 'recipes'].map(s => 'post.sub_' + s)

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

class FakeImage {
  constructor() {
    this.onload = null
    this.onerror = null
    this.naturalWidth = 64
    this.naturalHeight = 48
    this.width = 64
    this.height = 48
    this.complete = false
    this._src = ''
  }
  set src(v) {
    this._src = v
    setTimeout(() => {
      this.complete = true
      if (this.onload) this.onload()
    }, 0)
  }
  get src() {
    return this._src
  }
  decode() {
    return Promise.resolve()
  }
}

const OrigImage = globalThis.Image

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-1'))
  GetSettings.mockReturnValue(Promise.resolve({}))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([
    { name: 'LoraA', path: '/loras/LoraA.safetensors' },
    { name: 'LoraB', path: '/loras/LoraB.safetensors' },
  ]))
  GetSDModels.mockImplementation(() => Promise.resolve([{ title: 'juggernaut-xl' }]))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve([EFFECT, STYLE_PLAIN, STYLE_LORAS]))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  globalThis.Image = FakeImage
  window.Image = FakeImage
})

afterEach(() => {
  globalThis.Image = OrigImage
  window.Image = OrigImage
})

function foldByTitle(wrapper, title) {
  const fold = wrapper.findAll('.fold').find(f => f.find('.fold-title').exists() && f.find('.fold-title').text() === title)
  expect(fold, `fold "${title}" must exist`).toBeTruthy()
  return fold
}

function isOpen(fold) {
  return fold.classes().includes('fold-open')
}

function headText(fold) {
  return fold.find('.fold-head').text()
}

async function toggleFold(fold) {
  await fold.find('.fold-head').trigger('click')
  await settle()
}

function before(a, b) {
  return Boolean(a.element.compareDocumentPosition(b.element) & Node.DOCUMENT_POSITION_FOLLOWING)
}

describe('1. Section headers on the post-process page have a one-line description', () => {
  it.each(SUB_KEYS)('%s exists and is non-empty in ru and en', (key) => {
    expect(typeof ru[key]).toBe('string')
    expect(ru[key].trim()).not.toBe('')
    expect(typeof en[key]).toBe('string')
    expect(en[key].trim()).not.toBe('')
    expect(ru[key]).not.toBe(key)
    expect(en[key]).not.toBe(key)
  })

  it('every section header shows its title together with its description', async () => {
    const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
    await settle()
    await wrapper.setProps({ initialImage: IMG })
    await settle()

    const pairs = [
      ['post.section_edit', 'post.sub_edit'],
      ['post.section_remix', 'post.sub_remix'],
      ['post.section_object', 'post.sub_object'],
      ['post.section_repair', 'post.sub_repair'],
      ['post.effect_title', 'post.sub_effect'],
      ['post.adjust_title', 'post.sub_adjust'],
      ['post.finish_title', 'post.sub_finish'],
      ['export.export_settings', 'post.sub_export'],
      ['post.recipes_title', 'post.sub_recipes'],
    ]
    const headers = wrapper.findAll('.card-header').map(h => h.text())
    for (const [titleKey, subKey] of pairs) {
      const header = headers.find(h => h.includes(t(titleKey)) && h.includes(t(subKey)))
      expect(header, `header for ${titleKey} must contain ${subKey}`).toBeTruthy()
    }
  })

  it('descriptions are visible in headers of collapsed sections too', async () => {
    const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
    await settle()
    const editHeader = wrapper.findAll('.card-header').find(h => h.text().includes(t('post.section_edit')))
    expect(editHeader).toBeTruthy()
    expect(editHeader.text()).toContain(t('post.sub_edit'))
    expect(editHeader.isVisible()).toBe(true)
  })
})

async function mountRemix(settings = {}) {
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(RemixSection, { props: { source: IMG, active: true }, attachTo: document.body })
  await settle()
  return wrapper
}

function moreFold(wrapper) {
  return foldByTitle(wrapper, t('remix.more_title'))
}

describe('2. Remix: analysis first, rare options folded into "Дополнительно"', () => {
  it('analysis button comes before style buttons, style select and denoising slider', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    const analyze = wrapper.findAll('button').find(b => b.text().includes(t('fi.btn_analyze')))
    const styleBtn = wrapper.findAll('button').find(b => b.text() === t('fi.btn_style'))
    const workflowBtn = wrapper.findAll('button').find(b => b.text() === t('fi.btn_workflow'))
    const styleSelect = wrapper.findAll('select').find(s => s.findAll('option').some(o => o.text() === STYLE_PLAIN.name))
    const slider = wrapper.find('input[type="range"]')
    expect(analyze).toBeTruthy()
    expect(styleBtn).toBeTruthy()
    expect(workflowBtn).toBeTruthy()
    expect(styleSelect).toBeTruthy()
    expect(slider.exists()).toBe(true)
    expect(before(analyze, styleBtn)).toBe(true)
    expect(before(analyze, workflowBtn)).toBe(true)
    expect(before(analyze, styleSelect)).toBe(true)
    expect(before(analyze, slider)).toBe(true)
  })

  it('analysis comes before the workflow select in compound mode', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'compound' })
    const analyze = wrapper.findAll('button').find(b => b.text().includes(t('fi.btn_analyze')))
    const workflowSelect = wrapper.findAll('select').find(s => s.findAll('option').some(o => o.text() === t('fi.select_workflow')))
    expect(workflowSelect).toBeTruthy()
    expect(before(analyze, workflowSelect)).toBe(true)
  })

  it('"Дополнительно" is collapsed by default and holds seed, resolution, quality and extra exclude', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    const fold = moreFold(wrapper)
    expect(isOpen(fold)).toBe(false)
    expect(fold.find('[data-testid="fi-seed-input"]').exists()).toBe(true)
    expect(fold.find('[data-testid="fi-quality-checkbox"]').exists()).toBe(true)
    expect(fold.findComponent({ name: 'ResolutionSelector' }).exists()).toBe(true)
    expect(fold.find(`textarea[placeholder="${t('fi.placeholder_extra_exclude')}"]`).exists()).toBe(true)
    expect(wrapper.find('[data-testid="fi-seed-input"]').isVisible()).toBe(false)
    expect(wrapper.find('[data-testid="fi-quality-checkbox"]').isVisible()).toBe(false)
  })

  it('opening "Дополнительно" shows the seed input', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    await toggleFold(moreFold(wrapper))
    expect(isOpen(moreFold(wrapper))).toBe(true)
    expect(wrapper.find('[data-testid="fi-seed-input"]').isVisible()).toBe(true)
  })

  it('hires profile in compound mode is inside "Дополнительно"', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'compound' })
    const hires = wrapper.findComponent(HiresProfileSelector)
    expect(hires.exists()).toBe(true)
    expect(moreFold(wrapper).element.contains(hires.element)).toBe(true)
  })

  it('summary says "по умолчанию" when nothing is set', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    expect(headText(moreFold(wrapper))).toContain(t('remix.more_default'))
    expect(t('remix.more_default')).toBe('по умолчанию')
  })

  it('summary contains the typed seed in preset mode', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    await wrapper.find('[data-testid="fi-seed-input"]').setValue('987654')
    await settle()
    const head = headText(moreFold(wrapper))
    expect(head).toContain('987654')
    expect(head).not.toContain(t('remix.more_default'))
  })

  it('summary mentions extra exclusions when the textarea is not empty', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    await moreFold(wrapper).find(`textarea[placeholder="${t('fi.placeholder_extra_exclude')}"]`).setValue('hats, glasses')
    await settle()
    const head = headText(moreFold(wrapper))
    expect(head).toContain(t('remix.more_exclude'))
    expect(head).not.toContain(t('remix.more_default'))
  })

  it('summary returns to "по умолчанию" after the seed is cleared', async () => {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    const seed = wrapper.find('[data-testid="fi-seed-input"]')
    await seed.setValue('42')
    await settle()
    await seed.setValue('')
    await settle()
    expect(headText(moreFold(wrapper))).toContain(t('remix.more_default'))
  })
})

describe('3a. Model block in Edit', () => {
  async function mountEdit(settings = {}) {
    GetSettings.mockReturnValue(Promise.resolve(settings))
    GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
    const wrapper = mount(EditSection, { props: { source: IMG, batch: false }, attachTo: document.body })
    await settle()
    return wrapper
  }

  function block(wrapper) {
    return foldByTitle(wrapper, t('model.block_title'))
  }

  it('is collapsed by default and contains the model select and LoRA rows', async () => {
    const wrapper = await mountEdit({ edit_loras: '[{"name":"MossL","weight":0.8}]' })
    const fold = block(wrapper)
    expect(t('model.block_title')).toBe('Модель и LoRA')
    expect(isOpen(fold)).toBe(false)
    expect(fold.find('[data-testid="edit-model-select"]').exists()).toBe(true)
    expect(fold.findAll('[data-testid="edit-lora-name"]').length).toBe(1)
    expect(wrapper.find('[data-testid="edit-model-select"]').isVisible()).toBe(false)
  })

  it('header summary "Moss · без LoRA" by default', async () => {
    const wrapper = await mountEdit()
    expect(headText(block(wrapper))).toContain('Moss' + SEP + 'без LoRA')
  })

  it('header summary counts saved LoRA rows: "Moss · 2 LoRA"', async () => {
    const wrapper = await mountEdit({ edit_loras: '[{"name":"M1","weight":0.8},{"name":"M2","weight":0.5}]' })
    expect(headText(block(wrapper))).toContain('Moss' + SEP + '2 LoRA')
  })

  it('header summary shows Klein after switching the model', async () => {
    const wrapper = await mountEdit({ edit_loras_klein: '[{"name":"K1","weight":0.5}]' })
    await toggleFold(block(wrapper))
    await wrapper.find('[data-testid="edit-model-select"]').setValue('klein')
    await settle()
    expect(headText(block(wrapper))).toContain('Klein' + SEP + '1 LoRA')
  })

  it('a LoRA row with an empty name is not counted', async () => {
    const wrapper = await mountEdit()
    await toggleFold(block(wrapper))
    await wrapper.find('[data-testid="edit-lora-add"]').trigger('click')
    await settle()
    expect(wrapper.findAll('[data-testid="edit-lora-name"]').length).toBe(1)
    expect(headText(block(wrapper))).toContain('Moss' + SEP + 'без LoRA')
    await wrapper.find('[data-testid="edit-lora-name"]').setValue('   ')
    await settle()
    expect(headText(block(wrapper))).toContain('Moss' + SEP + 'без LoRA')
    await wrapper.find('[data-testid="edit-lora-name"]').setValue('Real')
    await settle()
    expect(headText(block(wrapper))).toContain('Moss' + SEP + '1 LoRA')
  })

  it('opened block shows the Moss hint for Moss and the Klein hint for Klein', async () => {
    const wrapper = await mountEdit()
    await toggleFold(block(wrapper))
    expect(isOpen(block(wrapper))).toBe(true)
    const body = () => block(wrapper).find('.fold-body')
    expect(body().isVisible()).toBe(true)
    expect(body().text()).toContain(t('model.hint_moss'))
    expect(body().text()).not.toContain(t('model.hint_klein'))
    await wrapper.find('[data-testid="edit-model-select"]').setValue('klein')
    await settle()
    expect(body().text()).toContain(t('model.hint_klein'))
    expect(body().text()).not.toContain(t('model.hint_moss'))
  })
})

describe('3b. Model block in Effect', () => {
  async function mountPage(settings = {}) {
    GetSettings.mockReturnValue(Promise.resolve(settings))
    const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
    await settle()
    await wrapper.setProps({ initialImage: IMG })
    await settle()
    return wrapper
  }

  function block(wrapper) {
    const fold = wrapper.find('.fx-model')
    expect(fold.exists()).toBe(true)
    return fold
  }

  it('uses the common "Модель и LoRA" title and is collapsed by default', async () => {
    const wrapper = await mountPage()
    expect(block(wrapper).find('.fold-title').text()).toBe(t('model.block_title'))
    expect(isOpen(block(wrapper))).toBe(false)
  })

  it('header summary "Klein · без LoRA" by default', async () => {
    const wrapper = await mountPage()
    expect(headText(block(wrapper))).toContain('Klein' + SEP + 'без LoRA')
  })

  it('header summary "Moss · 2 LoRA" from saved settings', async () => {
    const wrapper = await mountPage({
      effect_model: 'moss',
      effect_loras_moss: '[{"name":"LoraA","weight":0.5},{"name":"LoraB","weight":0.7}]',
    })
    expect(headText(block(wrapper))).toContain('Moss' + SEP + '2 LoRA')
  })

  it('saved rows with empty names are not counted', async () => {
    const wrapper = await mountPage({
      effect_model: 'klein',
      effect_loras_klein: '[{"name":"LoraA","weight":0.5},{"name":"","weight":0.7},{"name":"  ","weight":1}]',
    })
    expect(headText(block(wrapper))).toContain('Klein' + SEP + '1 LoRA')
  })

  it('adding a LoRA updates the summary', async () => {
    const wrapper = await mountPage()
    await toggleFold(block(wrapper))
    await wrapper.find('.fx-model-body .fx-lora-add').trigger('click')
    await settle()
    const row = wrapper.findAll('.fx-model-body .fx-lora-row').at(-1)
    await row.find('select.fx-lora-name').setValue('LoraA')
    await settle()
    expect(headText(block(wrapper))).toContain('Klein' + SEP + '1 LoRA')
  })

  it('opened block shows the hint for the selected model', async () => {
    const wrapper = await mountPage()
    await toggleFold(block(wrapper))
    const body = () => block(wrapper).find('.fold-body')
    expect(body().isVisible()).toBe(true)
    expect(body().text()).toContain(t('model.hint_klein'))
    expect(body().text()).not.toContain(t('model.hint_moss'))
    await wrapper.find('.fx-model-body .fx-model-select').setValue('moss')
    await settle()
    expect(body().text()).toContain(t('model.hint_moss'))
    expect(body().text()).not.toContain(t('model.hint_klein'))
    expect(headText(block(wrapper))).toContain('Moss')
  })
})

describe('3c. Model block in Remix (style override panel)', () => {
  async function mountRemixPreset() {
    const wrapper = await mountRemix({ fi_mode: 'img2img', fi_gen_mode: 'preset' })
    return wrapper
  }

  function block(wrapper) {
    return foldByTitle(wrapper, t('model.block_title'))
  }

  async function pickStyle(wrapper, preset) {
    const sel = wrapper.findAll('select').find(s => s.findAll('option').some(o => o.text() === preset.name))
    expect(sel).toBeTruthy()
    await sel.find(`option[value="${preset.id}"]`).setSelected()
    await settle()
  }

  it('is collapsed by default and titled "Модель и LoRA"', async () => {
    const wrapper = await mountRemixPreset()
    expect(isOpen(block(wrapper))).toBe(false)
  })

  it('summary is "Как в стиле · без LoRA" without overrides and a style without LoRA', async () => {
    const wrapper = await mountRemixPreset()
    await pickStyle(wrapper, STYLE_PLAIN)
    expect(t('overrides.as_preset')).toBe('Как в стиле')
    expect(headText(block(wrapper))).toContain('Как в стиле' + SEP + 'без LoRA')
  })

  it('summary counts the style LoRAs: "Как в стиле · 2 LoRA"', async () => {
    const wrapper = await mountRemixPreset()
    await pickStyle(wrapper, STYLE_LORAS)
    expect(headText(block(wrapper))).toContain('Как в стиле' + SEP + '2 LoRA')
  })

  it('summary shows the overriding model name', async () => {
    const wrapper = await mountRemixPreset()
    await pickStyle(wrapper, STYLE_PLAIN)
    await toggleFold(block(wrapper))
    const modelSelect = block(wrapper).findAll('select').find(s => s.findAll('option').some(o => o.text() === 'juggernaut-xl'))
    expect(modelSelect).toBeTruthy()
    await modelSelect.setValue('juggernaut-xl')
    await settle()
    const head = headText(block(wrapper))
    expect(head).toContain('juggernaut-xl' + SEP + 'без LoRA')
    expect(head).not.toContain('Как в стиле')
  })

  it('an added LoRA row with an empty name is not counted', async () => {
    const wrapper = await mountRemixPreset()
    await pickStyle(wrapper, STYLE_PLAIN)
    await toggleFold(block(wrapper))
    const add = block(wrapper).findAll('button').find(b => b.text() === t('overrides.loras_add'))
    expect(add).toBeTruthy()
    await add.trigger('click')
    await settle()
    expect(headText(block(wrapper))).toContain('Как в стиле' + SEP + 'без LoRA')
  })
})

describe('modelLora helper', () => {
  it('activeLoraCount: non-array gives 0', () => {
    expect(activeLoraCount(undefined)).toBe(0)
    expect(activeLoraCount(null)).toBe(0)
    expect(activeLoraCount('LoraA')).toBe(0)
    expect(activeLoraCount({ name: 'LoraA' })).toBe(0)
    expect(activeLoraCount(3)).toBe(0)
  })

  it('activeLoraCount: empty array gives 0', () => {
    expect(activeLoraCount([])).toBe(0)
  })

  it('activeLoraCount: empty and whitespace names are not counted', () => {
    expect(activeLoraCount([{ name: '' }, { name: '   ' }, { name: 'A', weight: 1 }, { weight: 0.5 }])).toBe(1)
  })

  it('activeLoraCount: counts every named row, weight 0 included', () => {
    expect(activeLoraCount([{ name: 'A', weight: 0 }, { name: 'B', weight: 1 }])).toBe(2)
  })

  it('modelLoraText: "Klein · без LoRA" for no rows', () => {
    expect(modelLoraText('Klein', [])).toBe('Klein · без LoRA')
  })

  it('modelLoraText: "Moss · 2 LoRA" for two named rows', () => {
    expect(modelLoraText('Moss', [{ name: 'A' }, { name: 'B' }])).toBe('Moss · 2 LoRA')
  })

  it('modelLoraText: unnamed rows only → без LoRA', () => {
    expect(modelLoraText('Moss', [{ name: '' }, { name: ' ' }])).toBe('Moss · без LoRA')
  })

  it('modelLoraText: non-array rows → без LoRA', () => {
    expect(modelLoraText('Klein', null)).toBe('Klein · без LoRA')
  })

  it('modelLoraText follows the UI language', () => {
    setLocale('en')
    expect(modelLoraText('Klein', [])).toBe('Klein · no LoRA')
    expect(modelLoraText('Moss', [{ name: 'A' }, { name: 'B' }])).toBe('Moss · 2 LoRA')
    setLocale('ru')
  })
})
