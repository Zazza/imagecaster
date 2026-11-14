import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import {
  GetSettings, ListPresets, ListCompoundPresets, GenerateSDPrompt, EnqueueTxt2Img, EnqueueCompound,
  GenerateImage, GenerateCompoundImage,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const PRESETS = [
  { id: 1, name: 'Alpha', preset_type: 'general', loras: '' },
  { id: 2, name: 'Beta', preset_type: 'general', loras: '[{"name":"detail","weight":0.6}]' },
]
const WORKFLOW = { id: 10, name: 'Flow', steps: [{ preset_id: 2, denoising_strength: 0 }, { preset_id: 1, denoising_strength: 0.4 }] }
const CUSTOM_STEPS = [
  { preset_id: 2, denoising_strength: 0.5 },
  { preset_id: 1, denoising_strength: 0.3, loraRows: [{ name: 'ink', weight: 0.8 }], loraIgnoreAll: false },
]
const LLM = { prompt: 'llm positive', negative_prompt: 'llm negative' }

function settings(extra = {}) {
  return { gen_resolution_id: '3', gen_hires_profile_id: '4', ...extra }
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  ListCompoundPresets.mockImplementation(() => Promise.resolve([structuredClone(WORKFLOW)]))
  GenerateSDPrompt.mockImplementation(() => Promise.resolve({ ...LLM }))
  EnqueueTxt2Img.mockImplementation(() => Promise.resolve(101))
  EnqueueCompound.mockImplementation(() => Promise.resolve(201))
  GenerateImage.mockImplementation(() => Promise.resolve({ image: 'IMG', info: '{}' }))
  GenerateCompoundImage.mockImplementation(() => Promise.resolve({ image: 'IMG', info: '{}' }))
})

async function mountPage(s) {
  GetSettings.mockImplementation(() => Promise.resolve(s))
  const w = mount(GeneratePage, { attachTo: document.body })
  await settle()
  return w
}

const textarea = (w, placeholderKey) => w.findAll('textarea').find(ta => ta.attributes('placeholder') === t(placeholderKey))
const generateButton = w => w.find('.btn-generate')
const directButton = w => w.findAll('button').find(b => b.text() === t('generate.btn_generate_directly'))

async function clickGenerate(w) {
  await generateButton(w).trigger('click')
  await settle()
}

async function pressCtrlEnter() {
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
  await settle()
}

describe('generate: style mode', () => {
  it('asks the LLM with the description, then enqueues exactly the LLM prompt for the style', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await textarea(w, 'generate.placeholder_exclude').setValue('dogs')
    await clickGenerate(w)

    expect(GenerateSDPrompt).toHaveBeenCalledTimes(1)
    expect(GenerateSDPrompt).toHaveBeenCalledWith({ preset_id: 1, description: 'a cat', negative: 'dogs', overrides: null })
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img).toHaveBeenCalledWith({
      preset_id: 1,
      extra_prompt: 'llm positive',
      extra_negative_prompt: 'llm negative',
      resolution_id: 3,
      hires_profile_id: 4,
      overrides: null,
      seed: null,
    })
    expect(EnqueueCompound).not.toHaveBeenCalled()
    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).toBe('llm positive')
  })

  it('unchanged description: the second run skips the LLM and reuses the prompt', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(2)
    expect(EnqueueTxt2Img.mock.calls[1][0].extra_prompt).toBe('llm positive')
  })

  it('a hand-edited SD prompt is sent as is without a new LLM call', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    await textarea(w, 'generate.placeholder_positive_prompt').setValue('my own prompt')
    await textarea(w, 'generate.placeholder_negative_prompt').setValue('my own negative')
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img.mock.calls[1][0]).toMatchObject({ extra_prompt: 'my own prompt', extra_negative_prompt: 'my own negative' })
  })

  it('an edited description asks the LLM again', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    GenerateSDPrompt.mockImplementation(() => Promise.resolve({ prompt: 'second', negative_prompt: '' }))
    await textarea(w, 'generate.placeholder_description').setValue('a dog')
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledTimes(2)
    expect(GenerateSDPrompt.mock.calls[1][0].description).toBe('a dog')
    expect(EnqueueTxt2Img.mock.calls[1][0]).toMatchObject({ extra_prompt: 'second', extra_negative_prompt: '' })
  })

  it('count 3 with seed 42 enqueues three jobs with seeds 42, 43, 44', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1', gen_count: '3' }))
    await w.find(`input[placeholder="${t('generate.seed_placeholder')}"]`).setValue('42')
    await clickGenerate(w)
    expect(EnqueueTxt2Img.mock.calls.map(c => c[0].seed)).toEqual([42, 43, 44])
  })

  it.each([
    ['empty LLM answer', () => Promise.resolve({ prompt: '' }), () => t('generate.error_empty_llm')],
    ['LLM failure', () => Promise.reject(new Error('llm down')), () => t('generate.error_prompt_gen', { error: 'Error: llm down' })],
  ])('%s: nothing is enqueued and the error is shown', async (_, impl, message) => {
    GenerateSDPrompt.mockImplementation(impl)
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await clickGenerate(w)
    expect(EnqueueTxt2Img).not.toHaveBeenCalled()
    expect(w.text()).toContain(message())
  })
})

describe('generate: workflow mode', () => {
  it('LLM uses the first workflow step style; the job carries only the workflow id and prompts', async () => {
    const w = await mountPage(settings({ gen_mode: 'compound', gen_compound_preset_id: '10' }))
    await textarea(w, 'generate.placeholder_description').setValue('castle')
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledWith({ preset_id: 2, description: 'castle', negative: '', overrides: null })
    expect(EnqueueCompound).toHaveBeenCalledTimes(1)
    expect(EnqueueCompound).toHaveBeenCalledWith({
      compound_preset_id: 10,
      extra_prompt: 'llm positive',
      extra_negative_prompt: 'llm negative',
      resolution_id: 3,
      hires_profile_id: 4,
    })
    expect(EnqueueTxt2Img).not.toHaveBeenCalled()
  })
})

describe('generate: custom pipeline mode', () => {
  it('sends the restored steps with LoRA changes only where edited, seed per run', async () => {
    const w = await mountPage(settings({ gen_mode: 'custom', gen_custom_steps: JSON.stringify(CUSTOM_STEPS), gen_count: '2' }))
    await w.find(`input[placeholder="${t('generate.seed_placeholder')}"]`).setValue('7')
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledTimes(1)
    expect(GenerateSDPrompt.mock.calls[0][0]).toMatchObject({ preset_id: 2, overrides: null })
    const steps = [
      { preset_id: 2, denoising_strength: 0.5 },
      { preset_id: 1, denoising_strength: 0.3, loras: '[{"name":"ink","weight":0.8}]' },
    ]
    expect(EnqueueCompound.mock.calls.map(c => c[0])).toEqual([
      { steps, extra_prompt: 'llm positive', extra_negative_prompt: 'llm negative', resolution_id: 3, hires_profile_id: 4, seed: 7 },
      { steps, extra_prompt: 'llm positive', extra_negative_prompt: 'llm negative', resolution_id: 3, hires_profile_id: 4, seed: 8 },
    ])
  })
})

describe('generate: nothing to generate with', () => {
  it.each([
    ['style mode without a style', settings({ gen_mode: 'preset' }), 'generate.error_select_style'],
    ['workflow mode without a workflow', settings({ gen_mode: 'compound' }), 'generate.error_select_workflow'],
    ['custom mode with a step without style', settings({ gen_mode: 'custom' }), 'generate.error_custom_step_style'],
  ])('%s: button disabled, Ctrl+Enter enqueues nothing and shows why', async (_, s, errorKey) => {
    const w = await mountPage(s)
    expect(generateButton(w).attributes('disabled')).toBeDefined()
    await pressCtrlEnter()
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(EnqueueTxt2Img).not.toHaveBeenCalled()
    expect(EnqueueCompound).not.toHaveBeenCalled()
    expect(w.text()).toContain(t(errorKey))
  })
})

describe('generate directly (without the LLM)', () => {
  it('style mode sends the current prompts straight to GenerateImage', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '2', gen_extra_prompt: 'ready prompt', gen_extra_negative: 'ready neg' }))
    await w.find(`input[placeholder="${t('generate.seed_placeholder')}"]`).setValue('5')
    await directButton(w).trigger('click')
    await settle()
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(GenerateImage).toHaveBeenCalledWith({
      preset_id: 2,
      extra_prompt: 'ready prompt',
      extra_negative_prompt: 'ready neg',
      resolution_id: 3,
      hires_profile_id: 4,
      overrides: null,
      seed: 5,
    })
    expect(w.find('img[alt="Generated"]').attributes('src')).toBe('data:image/png;base64,IMG')
  })

  it('workflow mode sends the workflow id to GenerateCompoundImage', async () => {
    const w = await mountPage(settings({ gen_mode: 'compound', gen_compound_preset_id: '10', gen_extra_prompt: 'ready prompt' }))
    await directButton(w).trigger('click')
    await settle()
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(GenerateCompoundImage).toHaveBeenCalledWith({
      compound_preset_id: 10,
      extra_prompt: 'ready prompt',
      extra_negative_prompt: '',
      resolution_id: 3,
      hires_profile_id: 4,
    })
  })

  it('custom mode sends the steps and seed to GenerateCompoundImage', async () => {
    const w = await mountPage(settings({ gen_mode: 'custom', gen_custom_steps: JSON.stringify(CUSTOM_STEPS), gen_extra_prompt: 'ready prompt' }))
    await directButton(w).trigger('click')
    await settle()
    expect(GenerateCompoundImage).toHaveBeenCalledWith({
      steps: [
        { preset_id: 2, denoising_strength: 0.5 },
        { preset_id: 1, denoising_strength: 0.3, loras: '[{"name":"ink","weight":0.8}]' },
      ],
      extra_prompt: 'ready prompt',
      extra_negative_prompt: '',
      resolution_id: 3,
      hires_profile_id: 4,
      seed: null,
    })
  })

  it('no image in the answer shows an error', async () => {
    GenerateImage.mockImplementation(() => Promise.resolve({ image: '' }))
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1', gen_extra_prompt: 'p' }))
    await directButton(w).trigger('click')
    await settle()
    expect(w.text()).toContain(t('generate.error_no_image'))
  })

  it('disabled while the SD prompt is empty', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    expect(directButton(w).attributes('disabled')).toBeDefined()
  })
})
