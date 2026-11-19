import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import PeoplePage from '../components/PeoplePage.vue'
import StylistPage from '../components/StylistPage.vue'
import {
  GetSettings, ListPresets, ListCompoundPresets, EnqueueTxt2Img, EnqueueCompound, GetModelCatalog, ListPeople, FaceSwapStatus,
  TrainingStatus, ListTrainingJobs, TrainPerson, TrainPersonReplace, PickPeopleFolder, TrainingBases,
  StylistPresets, StylistPlan, StylistShoot, StylistAppearance, GetQueue, StylistFrames,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { loadPeople } from '../peopleStore.js'
import { peopleGroups, pipelineOverride } from '../people.js'
import { resetStylistPlan } from '../composables/useStylistPlan.js'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

const CATALOG = {
  sd_models: [
    { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' },
    { name: 'moss_image_sft_bf16', base: 'Moss-Image', filename: 'moss_image_sft_bf16.safetensors' },
    { name: 'flux1-dev-fp8', base: 'Flux.1', filename: 'flux1-dev-fp8.gguf' },
  ],
}
const PRESETS = [
  { id: 1, name: 'Photo', preset_type: 'general', loras: '', model_name: 'z-image-turbo-fp8-aio.gguf' },
  { id: 2, name: 'Moss', preset_type: 'general', loras: '', model_name: 'moss_image_sft_bf16.safetensors' },
]
const Z_MOSS = { id: 12, name: 'ZM', steps: [{ preset_id: 1, denoising_strength: 0 }, { preset_id: 2, denoising_strength: 0.4 }] }

const NAT_Z = { name: 'natasha_curated', trigger: 'nat_woman', gender: 'woman', base: 'Z-Image', lora: 'natasha_curated', steps: 2000, display_name: 'Наташа' }
const NAT_MOSS = { name: 'natasha_curated_moss', trigger: 'nat_woman', gender: 'woman', base: 'Moss-Image', lora: 'natasha_curated_moss', steps: 2000, display_name: 'наташа' }
const NAT_F1 = { name: 'natasha_curated_flux1', trigger: 'nat_woman', gender: 'woman', base: 'Flux.1', lora: 'natasha_curated_flux1', steps: 1500, display_name: 'Наташа', identity: true }

async function usePeople(list) {
  ListPeople.mockImplementation(() => Promise.resolve(list.map(p => ({ ...p }))))
  await loadPeople()
}

beforeEach(async () => {
  vi.clearAllMocks()
  vi.useRealTimers()
  clearEventMocks()
  setLocale('ru')
  GetModelCatalog.mockImplementation(() => Promise.resolve(structuredClone(CATALOG)))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('not_installed'))
  await usePeople([NAT_Z, NAT_MOSS])
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Name is case-insensitive: one person', () => {
  it('peopleGroups: «Наташа» and «наташа» are one group with both LoRAs', () => {
    const groups = peopleGroups([{ ...NAT_Z }, { ...NAT_MOSS }])
    expect(groups).toHaveLength(1)
    expect(groups[0].loras.map(p => p.name).sort()).toEqual(['natasha_curated', 'natasha_curated_moss'])
  })

  it('pipelineOverride Z-Image→Moss: both LoRAs', () => {
    const g = peopleGroups([{ ...NAT_Z }, { ...NAT_MOSS }])[0]
    const ov = pipelineOverride(CATALOG, ['z-image-turbo-fp8-aio.gguf', 'moss_image_sft_bf16.safetensors'], g, false)
    expect(ov.loras).toEqual({ 'Z-Image': 'natasha_curated', 'Moss-Image': 'natasha_curated_moss' })
  })

  describe('Generate «04 Человек»', () => {
    beforeEach(() => {
      ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
      ListCompoundPresets.mockImplementation(() => Promise.resolve([structuredClone(Z_MOSS)]))
      EnqueueTxt2Img.mockImplementation(() => Promise.resolve(101))
      EnqueueCompound.mockImplementation(() => Promise.resolve(201))
    })

    async function mountGen(s) {
      GetSettings.mockImplementation(() => Promise.resolve(s))
      const w = mount(GeneratePage, { attachTo: document.body })
      await settle()
      return w
    }

    function natOptions(w) {
      return w.find(tid('gen-person-select')).findAll('option').filter(o => /наташа/i.test(o.text()))
    }

    async function generate(w) {
      const ta = w.findAll('textarea').find(x => x.attributes('placeholder') === t('generate.placeholder_description'))
      await ta.setValue('a woman in a cafe')
      await w.find('.btn-generate').trigger('click')
      await settle()
    }

    it('Moss style: one option, picks the Moss LoRA', async () => {
      const w = await mountGen({ gen_mode: 'preset', gen_preset_id: '2' })
      const opts = natOptions(w)
      expect(opts).toHaveLength(1)
      expect(opts[0].element.disabled).toBe(false)
      await w.find(tid('gen-person-select')).setValue(opts[0].element.value)
      await settle()
      await generate(w)
      expect(EnqueueTxt2Img.mock.calls[0][0].overrides.person.name).toBe('natasha_curated_moss')
    })

    it('pipeline Z-Image→Moss: one option, both LoRAs, nothing missing', async () => {
      const w = await mountGen({ gen_mode: 'compound', gen_compound_preset_id: '12' })
      const opts = natOptions(w)
      expect(opts).toHaveLength(1)
      await w.find(tid('gen-person-select')).setValue(opts[0].element.value)
      await settle()
      expect(w.find(tid('gen-person-missing')).exists()).toBe(false)
      await generate(w)
      expect(EnqueueCompound).toHaveBeenCalledTimes(1)
      expect(EnqueueCompound.mock.calls[0][0].person.loras).toEqual({ 'Z-Image': 'natasha_curated', 'Moss-Image': 'natasha_curated_moss' })
    })
  })
})

describe('People page: training with a taken Name', () => {
  const TAKEN = () => Promise.reject(new Error('Имя уже у другой LoRA этой базы (display_taken)'))

  beforeEach(() => {
    TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
    ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
    ListPeople.mockImplementation(() => Promise.resolve([{ ...NAT_Z }]))
    TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/nat'))
    TrainPerson.mockImplementation(TAKEN)
    TrainPersonReplace.mockImplementation(() => Promise.resolve({ id: 't2', name: 'natasha_new', status: 'queued' }))
  })

  async function train(display) {
    const w = mount(PeoplePage)
    await settle()
    await w.findAll('button').find(b => b.text() === t('people.folder_pick')).trigger('click')
    await settle()
    await w.find(tid('people-name')).setValue('natasha_new')
    await w.find(tid('people-display')).setValue(display)
    await w.find(tid('people-gender')).setValue('woman')
    await w.find(tid('people-train')).trigger('click')
    await settle()
    return w
  }

  it('confirm yes → TrainPersonReplace with the same arguments', async () => {
    const confirm = vi.fn(() => true)
    vi.stubGlobal('confirm', confirm)
    await train('Наташа')
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(TrainPersonReplace).toHaveBeenCalledTimes(1)
    expect(TrainPersonReplace.mock.calls[0]).toEqual(TrainPerson.mock.calls[0])
  })

  it('confirm no → error shown, no second call', async () => {
    const confirm = vi.fn(() => false)
    vi.stubGlobal('confirm', confirm)
    const w = await train('Наташа')
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(TrainPersonReplace).not.toHaveBeenCalled()
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    const err = w.find(tid('people-error'))
    expect(err.exists()).toBe(true)
    expect(err.text()).not.toBe('')
  })

  it('empty Name + display_taken → no confirm, no replace', async () => {
    const confirm = vi.fn(() => true)
    vi.stubGlobal('confirm', confirm)
    await train('')
    expect(confirm).not.toHaveBeenCalled()
    expect(TrainPersonReplace).not.toHaveBeenCalled()
  })
})

describe('Stylist: «Снять» sends the LoRA name of the chosen model', () => {
  const BASE = {
    version: 1,
    themes: [{ id: 'beach', name: 'Пляж' }],
    outfit: [{ id: 'o1', theme: 'beach', name: 'Платье', prompt: 'white dress' }],
    hair: [{ id: 'h1', name: 'Пучок', prompt: 'low bun' }],
    makeup: [{ id: 'm1', name: 'Без', prompt: 'no makeup' }],
    background: [{ id: 'b1', theme: 'beach', name: 'Пляж', prompt: 'sandy beach', place: 'beach' }],
    pose: [{ id: 'p1', name: 'Анфас', prompt: 'facing camera', framing: 'close', places: ['any'], orientation: 'portrait' }],
  }
  const SERIES = () => [{ outfit: 'white dress', hair: 'low bun', makeup: 'no makeup', background: 'sandy beach', place: 'beach', shots: [
    { pose: 'Анфас', prompt: 'facing camera', landscape: false },
  ] }]

  beforeEach(() => {
    resetStylistPlan()
    StylistPresets.mockImplementation(() => Promise.resolve(structuredClone(BASE)))
    ListPeople.mockImplementation(() => Promise.resolve([{ ...NAT_Z }, { ...NAT_MOSS }, { ...NAT_F1 }]))
    FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
    StylistAppearance.mockImplementation(() => Promise.resolve(''))
    StylistPlan.mockImplementation(() => Promise.resolve({ series: SERIES(), warnings: [], used_llm: true }))
    GetQueue.mockImplementation(() => Promise.resolve([]))
    StylistFrames.mockImplementation(() => Promise.resolve([]))
    ListCompoundPresets.mockImplementation(() => Promise.resolve([]))
    StylistShoot.mockImplementation(() => Promise.resolve([1]))
  })

  it.each([
    ['Flux.1', 'natasha_curated_flux1'],
    ['Moss', 'natasha_curated_moss'],
  ])('model %s → person %s', async (label, lora) => {
    const w = mount(StylistPage)
    await settle()
    const persons = w.findAll(`${tid('stylist-person')} option`)
    expect(persons.filter(o => /наташа/i.test(o.text()))).toHaveLength(1)
    const target = w.find(tid('stylist-target')).findAll('option').filter(o => o.text().includes(label))
    expect(target.length).toBeGreaterThanOrEqual(1)
    await w.find(tid('stylist-target')).setValue(target[0].element.value)
    await settle()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot).toHaveBeenCalledTimes(1)
    expect(StylistShoot.mock.calls[0][0].person).toBe(lora)
  })
})
