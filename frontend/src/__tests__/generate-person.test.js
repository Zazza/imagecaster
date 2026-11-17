import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import { GetSettings, ListPresets, EnqueueTxt2Img, GetModelCatalog, ListPeople } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { loadPeople } from '../peopleStore.js'
import { useKidsMode } from '../composables/useKidsMode.js'

enableAutoUnmount(afterEach)

const PRESETS = [
  { id: 1, name: 'Photo', preset_type: 'general', loras: '', model_name: 'z-image-turbo-fp8-aio.gguf' },
  { id: 2, name: 'Flux', preset_type: 'general', loras: '', model_name: 'flux-2-klein-4b' },
  { id: 3, name: 'Moss', preset_type: 'general', loras: '', model_name: 'moss_image_sft_bf16.safetensors' },
  { id: 4, name: 'Chroma', preset_type: 'general', loras: '', model_name: 'Chroma1-HD.gguf' },
]
const CATALOG = {
  sd_models: [
    { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' },
    { name: 'flux-2-klein-4b', base: 'Flux.2' },
    { name: 'moss_image_sft_bf16', base: 'Moss-Image', filename: 'moss_image_sft_bf16.safetensors' },
    { name: 'Chroma1-HD', base: 'Chroma', filename: 'Chroma1-HD.gguf' },
  ],
}
const PEOPLE = [{ name: 'anna', trigger: 'anna_woman', gender: 'woman', base: 'Z-Image', lora: 'anna', steps: 2000 }]

beforeEach(async () => {
  vi.clearAllMocks()
  clearEventMocks()
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  GetModelCatalog.mockImplementation(() => Promise.resolve(structuredClone(CATALOG)))
  ListPeople.mockImplementation(() => Promise.resolve(PEOPLE.map(p => ({ ...p }))))
  EnqueueTxt2Img.mockImplementation(() => Promise.resolve(101))
  await loadPeople()
})

async function mountPage(presetId) {
  GetSettings.mockImplementation(() => Promise.resolve({ gen_mode: 'preset', gen_preset_id: String(presetId) }))
  const w = mount(GeneratePage, { attachTo: document.body })
  await settle()
  return w
}

async function generate(w) {
  const ta = w.findAll('textarea').find(x => x.attributes('placeholder') === t('generate.placeholder_description'))
  await ta.setValue('a woman in a cafe')
  await w.find('.btn-generate').trigger('click')
  await settle()
}

describe('generate: person picker', () => {
  it('Z-Image style: picked person goes into overrides.person', async () => {
    const w = await mountPage(1)
    const sel = w.find('[data-testid="gen-person-select"]')
    expect(sel.exists()).toBe(true)
    await sel.setValue('anna')
    await generate(w)
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides).toMatchObject({ person: { name: 'anna', trigger: 'anna_woman' } })
  })

  it('Z-Image style without a pick: no person in overrides', async () => {
    const w = await mountPage(1)
    await generate(w)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides?.person).toBeUndefined()
  })

  it('kids mode turned on after a pick: no person in the job', async () => {
    const w = await mountPage(1)
    await w.find('[data-testid="gen-person-select"]').setValue('anna')
    await w.findAll('textarea').find(x => x.attributes('placeholder') === t('generate.placeholder_description')).setValue('a woman in a cafe')
    const { kidsModeActive } = useKidsMode()
    kidsModeActive.value = true
    await settle()
    expect(w.find('[data-testid="gen-person"]').exists()).toBe(false)
    await w.find('.btn-generate').trigger('click')
    await settle()
    kidsModeActive.value = false
    const calls = EnqueueTxt2Img.mock.calls
    expect(calls).toHaveLength(1)
    expect(calls[0][0].overrides?.person).toBeUndefined()
  })

  it('Z-Image style: section active with the picker', async () => {
    const w = await mountPage(1)
    const card = w.find('[data-testid="gen-person"]')
    expect(card.classes()).not.toContain('card-inactive')
    expect(card.find('.card-title').text()).toBe(t('gen.section_people'))
    expect(card.find('[data-testid="gen-person-off"]').exists()).toBe(false)
  })

  it('style on a model without training: section inactive, no picker, reason shown, no person in the job', async () => {
    const w = await mountPage(4)
    const card = w.find('[data-testid="gen-person"]')
    expect(card.exists()).toBe(true)
    expect(card.classes()).toContain('card-inactive')
    expect(card.find('[data-testid="gen-person-select"]').exists()).toBe(false)
    expect(card.find('[data-testid="gen-person-off"]').text()).toBe(t('people.off_wrong_model'))
    expect(card.find('.card-subtitle').text()).toBe(t('gen.sub_people_wrong_model'))
    await generate(w)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides?.person).toBeUndefined()
  })

  it('no people: section inactive with a pointer to the People page', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([]))
    await loadPeople()
    const w = await mountPage(1)
    const card = w.find('[data-testid="gen-person"]')
    expect(card.classes()).toContain('card-inactive')
    expect(card.find('[data-testid="gen-person-off"]').text()).toBe(t('people.off_no_people'))
  })

  it('section sits between style tweaks and direct prompts', async () => {
    const w = await mountPage(1)
    const titles = w.findAll('.gen-sections .card-title').map(x => x.text())
    const nums = w.findAll('.gen-sections .card-num').map(x => x.text())
    const i = titles.indexOf(t('gen.section_people'))
    expect(titles[i - 1]).toBe(t('gen.section_overrides'))
    expect(titles[i + 1]).toBe(t('gen.section_prompts'))
    expect(nums).toEqual(['01', '02', '03', '04', '05'])
  })
})

const MIXED = [
  { name: 'anna', trigger: 'anna_woman', gender: 'woman', base: 'Z-Image', lora: 'anna', steps: 2000 },
  { name: 'old', trigger: 'old_man', gender: 'man', lora: 'old', steps: 2000 },
  { name: 'mila', trigger: 'mila_woman', gender: 'woman', base: 'Moss-Image', lora: 'mila', steps: 1500 },
]

async function usePeople(list) {
  ListPeople.mockImplementation(() => Promise.resolve(list.map(p => ({ ...p }))))
  await loadPeople()
}

function optionValues(w) {
  return w.find('[data-testid="gen-person-select"]').findAll('option').map(o => o.element.value).filter(v => v !== '').sort()
}

describe('generate: person picker by training base', () => {
  it('Moss style: select lists only Moss people', async () => {
    await usePeople(MIXED)
    const w = await mountPage(3)
    const card = w.find('[data-testid="gen-person"]')
    expect(card.classes()).not.toContain('card-inactive')
    expect(optionValues(w)).toEqual(['mila'])
  })

  it('Z-Image style: select lists Z-Image people and people without base', async () => {
    await usePeople(MIXED)
    const w = await mountPage(1)
    expect(optionValues(w)).toEqual(['anna', 'old'])
  })

  it('Moss style: picked Moss person goes into overrides.person', async () => {
    await usePeople(MIXED)
    const w = await mountPage(3)
    await w.find('[data-testid="gen-person-select"]').setValue('mila')
    await generate(w)
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides).toMatchObject({ person: { name: 'mila', trigger: 'mila_woman' } })
  })

  it('Moss style without Moss people: section inactive, reason names the model, no person in the job', async () => {
    await usePeople(MIXED.filter(p => p.base !== 'Moss-Image'))
    const w = await mountPage(3)
    const card = w.find('[data-testid="gen-person"]')
    expect(card.exists()).toBe(true)
    expect(card.classes()).toContain('card-inactive')
    expect(card.find('[data-testid="gen-person-select"]').exists()).toBe(false)
    const off = card.find('[data-testid="gen-person-off"]').text()
    expect(off).toBe(t('people.off_no_people_base', { base: 'Moss-Image' }))
    expect(off).toContain('Moss-Image')
    expect(card.find('.card-subtitle').text()).toBe(t('gen.sub_people_no_people_base'))
    await generate(w)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides?.person).toBeUndefined()
  })

  it('Z-Image style with only Moss people: section inactive, reason names Z-Image', async () => {
    await usePeople(MIXED.filter(p => p.base === 'Moss-Image'))
    const w = await mountPage(1)
    const card = w.find('[data-testid="gen-person"]')
    expect(card.classes()).toContain('card-inactive')
    expect(card.find('[data-testid="gen-person-select"]').exists()).toBe(false)
    const off = card.find('[data-testid="gen-person-off"]').text()
    expect(off).toBe(t('people.off_no_people_base', { base: 'Z-Image' }))
    expect(off).toContain('Z-Image')
  })

  it('Flux.2 style without Flux.2 people: section inactive, reason names Flux.2', async () => {
    await usePeople(MIXED)
    const w = await mountPage(2)
    const card = w.find('[data-testid="gen-person"]')
    expect(card.classes()).toContain('card-inactive')
    expect(card.find('[data-testid="gen-person-off"]').text()).toBe(t('people.off_no_people_base', { base: 'Flux.2' }))
  })

  it('Flux.2 style: select lists only Flux.2 people, pick goes into overrides.person', async () => {
    await usePeople([...MIXED, { name: 'kira', trigger: 'kira_woman', gender: 'woman', base: 'Flux.2', lora: 'kira', steps: 1500 }])
    const w = await mountPage(2)
    expect(optionValues(w)).toEqual(['kira'])
    await w.find('[data-testid="gen-person-select"]').setValue('kira')
    await generate(w)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides).toMatchObject({ person: { name: 'kira', trigger: 'kira_woman' } })
  })

  it('kids mode: section hidden for Moss style too', async () => {
    await usePeople(MIXED)
    const w = await mountPage(3)
    const { kidsModeActive } = useKidsMode()
    kidsModeActive.value = true
    await settle()
    expect(w.find('[data-testid="gen-person"]').exists()).toBe(false)
    kidsModeActive.value = false
  })
})
