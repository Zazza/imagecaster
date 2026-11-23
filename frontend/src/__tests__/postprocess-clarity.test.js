import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import RemixSection from '../components/RemixSection.vue'
import EditSection from '../components/EditSection.vue'
import HiresProfileSelector from '../components/HiresProfileSelector.vue'
import {
  GetSettings, UpdateSettings, GetSDLoRAs, GetSDModels, GetActiveSessionItem, GetSessionImage,
  ListPresets, ListPresetTypes, ListHiresProfiles, EnqueueFromImage,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t, setLocale } from '../i18n/index.js'
import { modelLoraText, activeLoraCount } from '../modelLora.js'
import { useFakeImage } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const IMG = 'aW1n'
const SEP = ' · '
const summary = (model, n) => model + SEP + (n > 0 ? t('model.lora_count', { n }) : t('model.no_lora'))
const TYPES = [{ id: 1, name: 'post' }]
const EFFECT = { id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null, loras: '' }
const STYLE_PLAIN = { id: 21, name: 'Plain Style', preset_type: 'post', type_id: 1, model_name: 'sdxl-base', prompt: 'p', loras: '' }
const STYLE_LORAS = {
  id: 22, name: 'Two Lora Style', preset_type: 'post', type_id: 1, model_name: 'sdxl-base', prompt: 'p',
  loras: '[{"name":"StyleA","weight":0.6},{"name":"StyleB","weight":0.8}]',
}

useFakeImage()

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
    const analyze = wrapper.findAll('button').find(b => b.text().includes(t('describe.again')))
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
    const analyze = wrapper.findAll('button').find(b => b.text().includes(t('describe.again')))
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
    expect(headText(block(wrapper))).toContain(summary(t('overrides.as_preset'), 0))
  })

  it('summary counts the style LoRAs: "Как в стиле · 2 LoRA"', async () => {
    const wrapper = await mountRemixPreset()
    await pickStyle(wrapper, STYLE_LORAS)
    expect(headText(block(wrapper))).toContain(summary(t('overrides.as_preset'), 2))
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
    expect(head).toContain(summary('juggernaut-xl', 0))
    expect(head).not.toContain(t('overrides.as_preset'))
  })

  it('an added LoRA row with an empty name is not counted', async () => {
    const wrapper = await mountRemixPreset()
    await pickStyle(wrapper, STYLE_PLAIN)
    await toggleFold(block(wrapper))
    const add = block(wrapper).findAll('button').find(b => b.text() === t('overrides.loras_add'))
    expect(add).toBeTruthy()
    await add.trigger('click')
    await settle()
    expect(headText(block(wrapper))).toContain(summary(t('overrides.as_preset'), 0))
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
    expect(modelLoraText('Klein', [])).toBe(summary('Klein', 0))
  })

  it('modelLoraText: "Moss · 2 LoRA" for two named rows', () => {
    expect(modelLoraText('Moss', [{ name: 'A' }, { name: 'B' }])).toBe(summary('Moss', 2))
  })

  it('modelLoraText: unnamed rows only → без LoRA', () => {
    expect(modelLoraText('Moss', [{ name: '' }, { name: ' ' }])).toBe(summary('Moss', 0))
  })

  it('modelLoraText: non-array rows → без LoRA', () => {
    expect(modelLoraText('Klein', null)).toBe(summary('Klein', 0))
  })

  it('modelLoraText follows the UI language', () => {
    setLocale('en')
    expect(modelLoraText('Klein', [])).toBe('Klein · no LoRA')
    expect(modelLoraText('Moss', [{ name: 'A' }, { name: 'B' }])).toBe('Moss · 2 LoRA')
    setLocale('ru')
  })
})
