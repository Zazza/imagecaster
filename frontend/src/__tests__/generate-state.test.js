import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { reactive } from 'vue'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import SavedDescriptionsModal from '../components/SavedDescriptionsModal.vue'
import {
  GetSettings, UpdateSettings, ListPresets, ListPresetTypes, ListCompoundPresets, ListDescriptions, ListResolutions,
  CreateDescriptionFull, DeleteDescription, EnqueueTxt2Img, EnqueueCompound, GetQueue, GetActiveSessionItem,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 4, name: 'general' }]
const PRESETS = [
  { id: 1, name: 'Alpha', preset_type: 'general', type_id: 4, loras: '' },
  { id: 2, name: 'Beta', preset_type: 'general', type_id: 4, loras: '' },
]
const WORKFLOW = { id: 10, name: 'Flow', steps: [{ preset_id: 2, denoising_strength: 0 }] }
const DESCS = [
  { id: 5, name: 'Cat', text: 'a cat', negative_prompt: 'dogs' },
  { id: 6, name: '', text: 'a long description of a quiet harbour at dawn with boats', negative_prompt: '' },
  { id: 7, name: 'Empty', text: '   ', negative_prompt: '' },
]

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  ListPresetTypes.mockImplementation(() => Promise.resolve(TYPES))
  ListCompoundPresets.mockImplementation(() => Promise.resolve([structuredClone(WORKFLOW)]))
  ListDescriptions.mockImplementation(() => Promise.resolve(DESCS.map(d => ({ ...d }))))
  ListResolutions.mockImplementation(() => Promise.resolve([{ id: 21, name: 'A', is_builtin: true }, { id: 22, name: 'B', is_builtin: true }]))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  GetActiveSessionItem.mockImplementation(() => Promise.resolve(null))
  EnqueueTxt2Img.mockImplementation(() => Promise.resolve(101))
  EnqueueCompound.mockImplementation(() => Promise.resolve(201))
  CreateDescriptionFull.mockImplementation(() => Promise.resolve({ id: 9 }))
  DeleteDescription.mockImplementation(() => Promise.resolve())
})

async function mountPage(s, shared) {
  GetSettings.mockImplementation(() => Promise.resolve(s))
  const global = shared ? { provide: { sharedGenState: shared } } : {}
  const w = mount(GeneratePage, { attachTo: document.body, global })
  await settle()
  return w
}

const textarea = (w, key) => w.findAll('textarea').find(ta => ta.attributes('placeholder') === t(key))
const modal = w => w.findComponent(SavedDescriptionsModal)
const lastSave = () => UpdateSettings.mock.calls.map(c => c[0]).filter(o => 'gen_description' in o).at(-1)

const FULL = {
  gen_mode: 'preset', gen_preset_id: '2', gen_type_id: '4', gen_description: 'a fox', gen_negative: 'rain',
  gen_extra_prompt: 'xp', gen_extra_negative: 'xn', gen_resolution_id: '3', gen_hires_profile_id: '8', gen_count: '250',
}

describe('GeneratePage: form state persistence', () => {
  it('restores fields from settings and saves the full state on unmount', async () => {
    const w = await mountPage(FULL)
    expect(textarea(w, 'generate.placeholder_description').element.value).toBe('a fox')
    expect(textarea(w, 'generate.placeholder_exclude').element.value).toBe('rain')
    expect(textarea(w, 'generate.placeholder_positive_prompt').element.value).toBe('xp')
    expect(textarea(w, 'generate.placeholder_negative_prompt').element.value).toBe('xn')
    w.unmount()
    expect(lastSave()).toEqual({
      gen_preset_id: '2', gen_type_id: '4', gen_description: 'a fox', gen_negative: 'rain', gen_desc_id: '',
      gen_extra_prompt: 'xp', gen_extra_negative: 'xn', gen_mode: 'preset', gen_compound_preset_id: '',
      gen_custom_steps: expect.any(String), gen_resolution_id: '3', gen_hires_profile_id: '8', gen_count: '100',
    })
  })

  it('unknown preset, type and workflow ids are not restored', async () => {
    const w = await mountPage({ gen_mode: 'compound', gen_preset_id: '99', gen_type_id: '77', gen_compound_preset_id: '55' })
    w.unmount()
    const s = lastSave()
    expect(s.gen_preset_id).toBe('')
    expect(s.gen_type_id).toBe('')
    expect(s.gen_compound_preset_id).toBe('')
    expect(s.gen_mode).toBe('compound')
  })

  it('restores the saved idea binding when its text matches, with the saved negative', async () => {
    const w = await mountPage({ gen_description: 'a cat', gen_negative: 'cats', gen_desc_id: '5' })
    expect(textarea(w, 'generate.placeholder_exclude').element.value).toBe('cats')
    w.unmount()
    expect(lastSave().gen_desc_id).toBe('5')
  })

  it('drops the binding and the negative when the idea text no longer matches', async () => {
    const w = await mountPage({ gen_description: 'a dog', gen_negative: 'cats', gen_desc_id: '5' })
    expect(textarea(w, 'generate.placeholder_exclude').element.value).toBe('')
    w.unmount()
    expect(lastSave().gen_desc_id).toBe('')
  })

  it('without a saved resolution picks the first resolution and stores it', async () => {
    const w = await mountPage({})
    expect(UpdateSettings).toHaveBeenCalledWith({ gen_resolution_id: '21', gen_hires_profile_id: '' })
    w.unmount()
    expect(lastSave().gen_resolution_id).toBe('21')
  })

  it('shared state wins over settings and is written back on unmount (custom mode becomes style)', async () => {
    const shared = reactive({
      description: 'shared desc', negative: 'shared neg', selectedPresetId: 1, selectedCompoundPresetId: 10,
      genMode: 'compound', selectedResolutionId: 30, selectedHiresProfileId: 31,
    })
    const w = await mountPage({ gen_mode: 'preset', gen_preset_id: '2', gen_description: 'from settings' }, shared)
    expect(textarea(w, 'generate.placeholder_description').element.value).toBe('shared desc')
    w.findAll('button').find(b => b.text() === t('generate.btn_custom')).trigger('click')
    await settle()
    await textarea(w, 'generate.placeholder_description').setValue('edited')
    w.unmount()
    expect(lastSave()).toMatchObject({ gen_preset_id: '1', gen_compound_preset_id: '10', gen_mode: 'custom', gen_description: 'edited', gen_resolution_id: '30', gen_hires_profile_id: '31' })
    expect({ ...shared }).toEqual({
      description: 'edited', negative: 'shared neg', selectedPresetId: 1, selectedCompoundPresetId: 10,
      genMode: 'preset', selectedResolutionId: 30, selectedHiresProfileId: 31,
    })
  })
})

describe('GeneratePage: saved ideas', () => {
  it('save idea creates it with trimmed text and binds it', async () => {
    const w = await mountPage({ gen_resolution_id: '3' })
    await textarea(w, 'generate.placeholder_description').setValue('  a bird  ')
    await textarea(w, 'generate.placeholder_exclude').setValue('cage')
    const calls = ListDescriptions.mock.calls.length
    await w.findAll('button').find(b => b.text() === t('generate.btn_save')).trigger('click')
    await settle()
    expect(CreateDescriptionFull).toHaveBeenCalledWith({ text: 'a bird', name: '', negative_prompt: 'cage', type: '' })
    expect(ListDescriptions.mock.calls.length).toBe(calls + 1)
    w.unmount()
    expect(lastSave().gen_desc_id).toBe('9')
  })

  it('save error is shown on the page', async () => {
    CreateDescriptionFull.mockImplementation(() => Promise.reject('db busy'))
    const w = await mountPage({ gen_resolution_id: '3' })
    await textarea(w, 'generate.placeholder_description').setValue('a bird')
    await w.findAll('button').find(b => b.text() === t('generate.btn_save')).trigger('click')
    await settle()
    expect(w.text()).toContain(t('generate.error_save_desc', { error: 'db busy' }))
  })

  it('using an idea fills description and negative and closes the list', async () => {
    const w = await mountPage({ gen_resolution_id: '3' })
    await w.findAll('button').find(b => b.text().includes(t('generate.saved_ideas'))).trigger('click')
    expect(modal(w).props('visible')).toBe(true)
    modal(w).vm.$emit('use', DESCS[0])
    await settle()
    expect(textarea(w, 'generate.placeholder_description').element.value).toBe('a cat')
    expect(textarea(w, 'generate.placeholder_exclude').element.value).toBe('dogs')
    expect(modal(w).props('visible')).toBe(false)
    w.unmount()
    expect(lastSave().gen_desc_id).toBe('5')
  })

  it('deleting the bound idea unbinds it; delete error is shown', async () => {
    const w = await mountPage({ gen_description: 'a cat', gen_desc_id: '5', gen_resolution_id: '3' })
    modal(w).vm.$emit('delete', 5)
    await settle()
    expect(DeleteDescription).toHaveBeenCalledWith(5)
    DeleteDescription.mockImplementation(() => Promise.reject('locked'))
    modal(w).vm.$emit('delete', 6)
    await settle()
    expect(w.text()).toContain(t('generate.error_delete_desc', { error: 'locked' }))
    w.unmount()
    expect(lastSave().gen_desc_id).toBe('')
  })
})

describe('GeneratePage: batch over saved ideas', () => {
  it('without a style the batch is disabled with a reason', async () => {
    const w = await mountPage({ gen_mode: 'preset', gen_resolution_id: '3' })
    expect(modal(w).props('batchDisabled')).toBe(t('descriptions.batch_no_style'))
    expect(modal(w).props('batchTarget')).toBe('')
    modal(w).vm.$emit('batch', { ids: [5], perIdea: 1 })
    await settle()
    expect(EnqueueTxt2Img).not.toHaveBeenCalled()
  })

  it('style mode enqueues each idea as is and reports progress, errors and the done message', async () => {
    const w = await mountPage({ gen_mode: 'preset', gen_preset_id: '1', gen_resolution_id: '3', gen_hires_profile_id: '8' })
    expect(modal(w).props('batchDisabled')).toBe('')
    expect(modal(w).props('batchTarget')).toBe(t('descriptions.batch_target_preset', { name: 'Alpha' }))
    UpdateSettings.mockClear()
    modal(w).vm.$emit('batch', { ids: [5, 7, 6, 404], perIdea: 2 })
    await settle()
    expect(lastSave()).toBeTruthy()
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(4)
    expect(EnqueueTxt2Img).toHaveBeenNthCalledWith(1, {
      preset_id: 1, extra_prompt: '', extra_negative_prompt: '', idea: 'a cat', idea_negative: 'dogs',
      resolution_id: 3, hires_profile_id: 8, overrides: null, seed: null,
    })
    expect(EnqueueTxt2Img.mock.calls[2][0].idea).toBe(DESCS[1].text)
    expect(modal(w).props('batchRunning')).toBe(false)
    expect(modal(w).props('batchProgress')).toEqual({
      done: 3, total: 3,
      errors: [{ name: 'Empty', error: t('descriptions.batch_empty_idea') }],
      message: t('descriptions.batch_done', { n: 4 }),
    })
    expect(w.text()).toContain(t('descriptions.batch_done', { n: 4 }))
  })

  it('workflow mode enqueues the workflow; reopening the list clears the finished progress', async () => {
    const w = await mountPage({ gen_mode: 'compound', gen_compound_preset_id: '10', gen_resolution_id: '3' })
    expect(modal(w).props('batchTarget')).toBe(t('descriptions.batch_target_workflow', { name: 'Flow' }))
    modal(w).vm.$emit('batch', { ids: [5], perIdea: 1 })
    await settle()
    expect(EnqueueCompound).toHaveBeenCalledWith({
      compound_preset_id: 10, extra_prompt: '', extra_negative_prompt: '', idea: 'a cat', idea_negative: 'dogs',
      resolution_id: 3, hires_profile_id: null,
    })
    expect(modal(w).props('batchProgress')).not.toBeNull()
    await w.findAll('button').find(b => b.text().includes(t('generate.saved_ideas'))).trigger('click')
    await settle()
    expect(modal(w).props('batchProgress')).toBeNull()
  })

  it('names an idea without a name by its first 40 characters in errors', async () => {
    EnqueueTxt2Img.mockImplementation(() => Promise.reject('queue full'))
    const w = await mountPage({ gen_mode: 'preset', gen_preset_id: '1', gen_resolution_id: '3' })
    modal(w).vm.$emit('batch', { ids: [6], perIdea: 1 })
    await settle()
    expect(modal(w).props('batchProgress').errors).toEqual([{ name: DESCS[1].text.slice(0, 40) + '…', error: 'queue full' }])
  })
})
