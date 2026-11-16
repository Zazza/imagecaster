import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

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

const IDEA_JOB = 101

function completeJob(id, result) {
  EventsEmit('queue:completed', { job_id: id, result: JSON.stringify({ image_base64: 'IMG', ...result }) })
}

const noIdea = call => !call.idea && !call.idea_negative

describe('generate: style mode (description goes into the job)', () => {
  it('a dirty prompt is not generated before enqueue: the job carries the description as the idea', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await textarea(w, 'generate.placeholder_exclude').setValue('dogs')
    await clickGenerate(w)

    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img).toHaveBeenCalledWith({
      preset_id: 1,
      extra_prompt: '',
      extra_negative_prompt: '',
      idea: 'a cat',
      idea_negative: 'dogs',
      resolution_id: 3,
      hires_profile_id: 4,
      overrides: null,
      seed: null,
    })
    expect(EnqueueCompound).not.toHaveBeenCalled()
  })

  it('count 3 with seed 42 enqueues three jobs with the same idea and seeds 42, 43, 44', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1', gen_count: '3' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await textarea(w, 'generate.placeholder_exclude').setValue('dogs')
    await w.find(`input[placeholder="${t('generate.seed_placeholder')}"]`).setValue('42')
    await clickGenerate(w)
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    const calls = EnqueueTxt2Img.mock.calls.map(c => c[0])
    expect(calls.map(c => c.seed)).toEqual([42, 43, 44])
    for (const c of calls) {
      expect(c).toMatchObject({ idea: 'a cat', idea_negative: 'dogs', extra_prompt: '', extra_negative_prompt: '' })
    }
  })

  it('the completed job fills the SD prompt; the next unchanged run sends that prompt and no idea', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await textarea(w, 'generate.placeholder_exclude').setValue('dogs')
    await clickGenerate(w)
    completeJob(IDEA_JOB, { idea_prompt: 'job positive', idea_negative_prompt: 'job negative' })
    await settle()

    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).toBe('job positive')
    expect(textarea(w, 'generate.placeholder_negative_prompt').element.value).toBe('job negative')

    await clickGenerate(w)
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(2)
    const second = EnqueueTxt2Img.mock.calls[1][0]
    expect(second).toMatchObject({ extra_prompt: 'job positive', extra_negative_prompt: 'job negative' })
    expect(noIdea(second)).toBe(true)
  })

  it('description edited while the job runs: completion does not overwrite the prompt, next run sends the idea again', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    await textarea(w, 'generate.placeholder_description').setValue('a dog')
    completeJob(IDEA_JOB, { idea_prompt: 'cat prompt', idea_negative_prompt: 'cat negative' })
    await settle()

    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).not.toBe('cat prompt')
    expect(textarea(w, 'generate.placeholder_negative_prompt').element.value).not.toBe('cat negative')

    await clickGenerate(w)
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(EnqueueTxt2Img.mock.calls[1][0]).toMatchObject({ idea: 'a dog', extra_prompt: '' })
  })

  it('negative edited while the job runs: completion does not overwrite the prompt', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await textarea(w, 'generate.placeholder_exclude').setValue('dogs')
    await clickGenerate(w)
    await textarea(w, 'generate.placeholder_exclude').setValue('birds')
    completeJob(IDEA_JOB, { idea_prompt: 'cat prompt', idea_negative_prompt: 'cat negative' })
    await settle()

    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).not.toBe('cat prompt')
    await clickGenerate(w)
    expect(EnqueueTxt2Img.mock.calls[1][0]).toMatchObject({ idea: 'a cat', idea_negative: 'birds' })
  })

  it.each([
    ['SD prompt', 'generate.placeholder_positive_prompt', 'job positive'],
    ['SD negative', 'generate.placeholder_negative_prompt', 'job negative'],
  ])('text typed into the %s while the job runs is not overwritten on completion', async (_, field, jobValue) => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    await textarea(w, field).setValue('typed by hand')
    completeJob(IDEA_JOB, { idea_prompt: 'job positive', idea_negative_prompt: 'job negative' })
    await settle()

    expect(textarea(w, field).element.value).toBe('typed by hand')
    expect(textarea(w, field).element.value).not.toBe(jobValue)
  })

  it('completion of a job not enqueued by this click leaves the prompt fields alone', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    completeJob(999, { idea_prompt: 'foreign prompt', idea_negative_prompt: 'foreign negative' })
    await settle()

    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).not.toBe('foreign prompt')
    expect(textarea(w, 'generate.placeholder_negative_prompt').element.value).not.toBe('foreign negative')
  })

  it('a job result without idea prompts does not clear the SD prompt', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    completeJob(IDEA_JOB, { idea_prompt: 'job positive', idea_negative_prompt: 'job negative' })
    await settle()
    EnqueueTxt2Img.mockImplementation(() => Promise.resolve(102))
    await clickGenerate(w)
    completeJob(102, {})
    await settle()
    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).toBe('job positive')
  })

  it('a hand-edited SD prompt with an unchanged description is sent as is, without an idea', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    completeJob(IDEA_JOB, { idea_prompt: 'job positive', idea_negative_prompt: 'job negative' })
    await settle()
    await textarea(w, 'generate.placeholder_positive_prompt').setValue('my own prompt')
    await textarea(w, 'generate.placeholder_negative_prompt').setValue('my own negative')
    await clickGenerate(w)

    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    const second = EnqueueTxt2Img.mock.calls[1][0]
    expect(second).toMatchObject({ extra_prompt: 'my own prompt', extra_negative_prompt: 'my own negative' })
    expect(noIdea(second)).toBe(true)
  })

  it('an edited description after a completed job is sent as a new idea', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('a cat')
    await clickGenerate(w)
    completeJob(IDEA_JOB, { idea_prompt: 'job positive', idea_negative_prompt: '' })
    await settle()
    await textarea(w, 'generate.placeholder_description').setValue('a dog')
    await clickGenerate(w)
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(EnqueueTxt2Img.mock.calls[1][0]).toMatchObject({ idea: 'a dog', extra_prompt: '', extra_negative_prompt: '' })
  })

  it('a description of exactly 4000 characters still goes into the job', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('я'.repeat(4000))
    await clickGenerate(w)
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(EnqueueTxt2Img.mock.calls[0][0].idea).toBe('я'.repeat(4000))
  })
})

describe('generate: style mode (old path, LLM before enqueue)', () => {
  it.each([
    ['description over 4000 characters', 'я'.repeat(4001), ''],
    ['negative over 4000 characters', 'a cat', 'н'.repeat(4001)],
  ])('%s: the LLM runs first, the job gets its prompt and no idea', async (_, description, negative) => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue(description)
    await textarea(w, 'generate.placeholder_exclude').setValue(negative)
    await clickGenerate(w)

    expect(GenerateSDPrompt).toHaveBeenCalledTimes(1)
    expect(GenerateSDPrompt).toHaveBeenCalledWith({ preset_id: 1, description, negative, overrides: null })
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    const call = EnqueueTxt2Img.mock.calls[0][0]
    expect(call).toMatchObject({ extra_prompt: 'llm positive', extra_negative_prompt: 'llm negative' })
    expect(noIdea(call)).toBe(true)
    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).toBe('llm positive')
  })

  it('empty description with a dirty prompt: the LLM runs before enqueue', async () => {
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    expect(noIdea(EnqueueTxt2Img.mock.calls[0][0])).toBe(true)
  })

  it('LLM failure on a long description: nothing is enqueued and the prompt error is shown', async () => {
    GenerateSDPrompt.mockImplementation(() => Promise.reject(new Error('llm down')))
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await textarea(w, 'generate.placeholder_description').setValue('я'.repeat(4001))
    await clickGenerate(w)
    expect(EnqueueTxt2Img).not.toHaveBeenCalled()
    expect(w.text()).toContain(t('generate.error_prompt_gen', { error: 'Error: llm down' }))
  })

  it.each([
    ['empty LLM answer', () => Promise.resolve({ prompt: '' }), () => t('generate.error_empty_llm')],
    ['LLM failure', () => Promise.reject(new Error('llm down')), () => t('generate.error_prompt_gen', { error: 'Error: llm down' })],
  ])('empty description, %s: nothing is enqueued and the error is shown', async (_, impl, message) => {
    GenerateSDPrompt.mockImplementation(impl)
    const w = await mountPage(settings({ gen_mode: 'preset', gen_preset_id: '1' }))
    await clickGenerate(w)
    expect(EnqueueTxt2Img).not.toHaveBeenCalled()
    expect(w.text()).toContain(message())
  })
})

describe('generate: workflow mode', () => {
  it('the job carries the workflow id and the description as the idea', async () => {
    const w = await mountPage(settings({ gen_mode: 'compound', gen_compound_preset_id: '10' }))
    await textarea(w, 'generate.placeholder_description').setValue('castle')
    await textarea(w, 'generate.placeholder_exclude').setValue('people')
    await clickGenerate(w)
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    expect(EnqueueCompound).toHaveBeenCalledTimes(1)
    expect(EnqueueCompound).toHaveBeenCalledWith({
      compound_preset_id: 10,
      extra_prompt: '',
      extra_negative_prompt: '',
      idea: 'castle',
      idea_negative: 'people',
      resolution_id: 3,
      hires_profile_id: 4,
    })
    expect(EnqueueTxt2Img).not.toHaveBeenCalled()
  })

  it('completion of the workflow job fills the SD prompt; next unchanged run sends no idea', async () => {
    const w = await mountPage(settings({ gen_mode: 'compound', gen_compound_preset_id: '10' }))
    await textarea(w, 'generate.placeholder_description').setValue('castle')
    await clickGenerate(w)
    completeJob(201, { idea_prompt: 'flow positive', idea_negative_prompt: 'flow negative' })
    await settle()
    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).toBe('flow positive')
    await clickGenerate(w)
    const second = EnqueueCompound.mock.calls[1][0]
    expect(second).toMatchObject({ extra_prompt: 'flow positive', extra_negative_prompt: 'flow negative' })
    expect(noIdea(second)).toBe(true)
  })

  it('old path for a long description uses the first workflow step style', async () => {
    const w = await mountPage(settings({ gen_mode: 'compound', gen_compound_preset_id: '10' }))
    await textarea(w, 'generate.placeholder_description').setValue('я'.repeat(4001))
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledWith({ preset_id: 2, description: 'я'.repeat(4001), negative: '', overrides: null })
    expect(EnqueueCompound.mock.calls[0][0]).toMatchObject({ compound_preset_id: 10, extra_prompt: 'llm positive', extra_negative_prompt: 'llm negative' })
  })
})

describe('generate: custom pipeline mode', () => {
  it('sends the restored steps with the idea, LoRA changes only where edited, seed per run', async () => {
    const w = await mountPage(settings({ gen_mode: 'custom', gen_custom_steps: JSON.stringify(CUSTOM_STEPS), gen_count: '2' }))
    await textarea(w, 'generate.placeholder_description').setValue('forest')
    await textarea(w, 'generate.placeholder_exclude').setValue('cars')
    await w.find(`input[placeholder="${t('generate.seed_placeholder')}"]`).setValue('7')
    await clickGenerate(w)
    expect(GenerateSDPrompt).not.toHaveBeenCalled()
    const steps = [
      { preset_id: 2, denoising_strength: 0.5 },
      { preset_id: 1, denoising_strength: 0.3, loras: '[{"name":"ink","weight":0.8}]' },
    ]
    const common = { steps, extra_prompt: '', extra_negative_prompt: '', idea: 'forest', idea_negative: 'cars', resolution_id: 3, hires_profile_id: 4 }
    expect(EnqueueCompound.mock.calls.map(c => c[0])).toEqual([
      { ...common, seed: 7 },
      { ...common, seed: 8 },
    ])
  })

  it('empty description: old path asks the LLM with the first step style', async () => {
    const w = await mountPage(settings({ gen_mode: 'custom', gen_custom_steps: JSON.stringify(CUSTOM_STEPS) }))
    await clickGenerate(w)
    expect(GenerateSDPrompt).toHaveBeenCalledTimes(1)
    expect(GenerateSDPrompt.mock.calls[0][0]).toMatchObject({ preset_id: 2, overrides: null })
    expect(EnqueueCompound.mock.calls[0][0]).toMatchObject({ extra_prompt: 'llm positive', extra_negative_prompt: 'llm negative' })
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
