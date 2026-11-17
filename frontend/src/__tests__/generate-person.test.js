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
]
const CATALOG = {
  sd_models: [
    { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' },
    { name: 'flux-2-klein-4b', base: 'Flux.2' },
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

  it('non-Z-Image style: section inactive, no picker, reason shown, no person in the job', async () => {
    const w = await mountPage(2)
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
