import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import {
  GetSettings, ListPresets, ListCompoundPresets, EnqueueTxt2Img, EnqueueCompound, GetModelCatalog, ListPeople, FaceSwapStatus,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { loadPeople } from '../peopleStore.js'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`
const PRESETS = [
  { id: 1, name: 'Photo', preset_type: 'general', loras: '', model_name: 'z-image-turbo-fp8-aio.gguf' },
  { id: 2, name: 'Dev', preset_type: 'general', loras: '', model_name: 'flux1-dev-fp8.gguf' },
  { id: 3, name: 'XL', preset_type: 'general', loras: '', model_name: 'juggernautXL.safetensors' },
  { id: 4, name: 'Chroma', preset_type: 'general', loras: '', model_name: 'Chroma1-HD.gguf' },
]
const CATALOG = {
  sd_models: [
    { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' },
    { name: 'flux1-dev-fp8', base: 'Flux.1', filename: 'flux1-dev-fp8.gguf' },
    { name: 'juggernautXL', base: 'SDXL', filename: 'juggernautXL.safetensors' },
    { name: 'Chroma1-HD', base: 'Chroma', filename: 'Chroma1-HD.gguf' },
  ],
}
const NAT_Z = { name: 'natasha_curated', trigger: 'nat_woman', gender: 'woman', base: 'Z-Image', lora: 'natasha_curated', steps: 2000, display_name: 'Наташа' }
const NAT_F1 = { name: 'natasha_curated_flux1', trigger: 'nat_woman', gender: 'woman', base: 'Flux.1', lora: 'natasha_curated_flux1', steps: 1500, display_name: 'Наташа' }
const Z_SDXL = { id: 10, name: 'ZX', steps: [{ preset_id: 1, denoising_strength: 0 }, { preset_id: 3, denoising_strength: 0.4 }] }
const SDXL_CHROMA = { id: 11, name: 'XC', steps: [{ preset_id: 3, denoising_strength: 0 }, { preset_id: 4, denoising_strength: 0.4 }] }

async function usePeople(list) {
  ListPeople.mockImplementation(() => Promise.resolve(list.map(p => ({ ...p }))))
  await loadPeople()
}

beforeEach(async () => {
  vi.clearAllMocks()
  clearEventMocks()
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  ListCompoundPresets.mockImplementation(() => Promise.resolve([structuredClone(Z_SDXL), structuredClone(SDXL_CHROMA)]))
  GetModelCatalog.mockImplementation(() => Promise.resolve(structuredClone(CATALOG)))
  EnqueueTxt2Img.mockImplementation(() => Promise.resolve(101))
  EnqueueCompound.mockImplementation(() => Promise.resolve(201))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('not_installed'))
  await usePeople([NAT_Z, NAT_F1])
})

async function mountPage(s) {
  GetSettings.mockImplementation(() => Promise.resolve(s))
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

function nameOptions(w, text) {
  return w.find(tid('gen-person-select')).findAll('option').filter(o => o.text().includes(text))
}

async function pickName(w, text) {
  const opts = nameOptions(w, text).filter(o => !o.element.disabled)
  expect(opts).toHaveLength(1)
  await w.find(tid('gen-person-select')).setValue(opts[0].element.value)
  await settle()
}

describe('generate style mode: person by Name', () => {
  it('two LoRAs with one Name on different bases → one option, no LoRA names listed', async () => {
    const w = await mountPage({ gen_mode: 'preset', gen_preset_id: '1' })
    expect(nameOptions(w, 'Наташа')).toHaveLength(1)
    const texts = w.find(tid('gen-person-select')).findAll('option').map(o => o.text())
    expect(texts.some(x => x.includes('natasha_curated'))).toBe(false)
  })

  it('Z-Image style: Name → Z-Image LoRA in overrides.person', async () => {
    const w = await mountPage({ gen_mode: 'preset', gen_preset_id: '1' })
    await pickName(w, 'Наташа')
    await generate(w)
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides.person.name).toBe('natasha_curated')
  })

  it('Flux.1 style: Name → Flux.1 LoRA in overrides.person', async () => {
    const w = await mountPage({ gen_mode: 'preset', gen_preset_id: '2' })
    expect(nameOptions(w, 'Наташа')).toHaveLength(1)
    await pickName(w, 'Наташа')
    await generate(w)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides.person.name).toBe('natasha_curated_flux1')
  })

  it('SDXL style: Name without an SDXL LoRA is not selectable, reason names SDXL', async () => {
    await usePeople([NAT_Z, NAT_F1, { name: 'bob', trigger: 'bob_man', gender: 'man', base: 'SDXL', lora: 'bob', steps: 1000 }])
    const w = await mountPage({ gen_mode: 'preset', gen_preset_id: '3' })
    const nat = nameOptions(w, 'Наташа')
    expect(nat).toHaveLength(1)
    expect(nat[0].element.disabled).toBe(true)
    expect(nat[0].text()).toContain('SDXL')
  })
})

describe('generate pipeline mode: person by Name', () => {
  it('workflow Z-Image→SDXL, Name has only Z-Image LoRA: available, missing names step 2, person per base', async () => {
    const w = await mountPage({ gen_mode: 'compound', gen_compound_preset_id: '10' })
    const card = w.find(tid('gen-person'))
    expect(card.classes()).not.toContain('card-inactive')
    await pickName(w, 'Наташа')
    const missing = w.find(tid('gen-person-missing'))
    expect(missing.exists()).toBe(true)
    expect(missing.text()).toContain('2')
    expect(missing.text()).not.toMatch(/\b1\b/)
    await generate(w)
    expect(EnqueueCompound).toHaveBeenCalledTimes(1)
    const person = EnqueueCompound.mock.calls[0][0].person
    expect(person.name).toBe('natasha_curated')
    expect(person.loras).toEqual({ 'Z-Image': 'natasha_curated' })
    expect(person.face_swap).toBeFalsy()
  })

  it('workflow without a pick: no person in the job', async () => {
    const w = await mountPage({ gen_mode: 'compound', gen_compound_preset_id: '10' })
    await generate(w)
    expect(EnqueueCompound.mock.calls[0][0].person).toBeFalsy()
  })

  it('face swap on, identity on the Z-Image LoRA: checkbox adds face_swap and face_swap_name', async () => {
    FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
    await usePeople([{ ...NAT_Z, identity: true }, NAT_F1])
    const w = await mountPage({ gen_mode: 'compound', gen_compound_preset_id: '10' })
    await pickName(w, 'Наташа')
    const box = w.find(tid('gen-face-swap-input'))
    expect(box.exists()).toBe(true)
    await box.setValue(true)
    await generate(w)
    const person = EnqueueCompound.mock.calls[0][0].person
    expect(person.face_swap).toBe(true)
    expect(person.face_swap_name).toBe('natasha_curated')
  })

  it('workflow with no step having the Name LoRA: card inactive or no selectable option', async () => {
    const w = await mountPage({ gen_mode: 'compound', gen_compound_preset_id: '11' })
    const card = w.find(tid('gen-person'))
    const sel = card.find(tid('gen-person-select'))
    const selectable = sel.exists() ? nameOptions(w, 'Наташа').filter(o => !o.element.disabled) : []
    expect(card.classes().includes('card-inactive') || selectable.length === 0).toBe(true)
    await generate(w)
    expect(EnqueueCompound.mock.calls[0][0].person).toBeFalsy()
  })

  it('custom pipeline Z-Image→SDXL: person per base with step 2 missing', async () => {
    const steps = [{ preset_id: 1, denoising_strength: 0.5 }, { preset_id: 3, denoising_strength: 0.3 }]
    const w = await mountPage({ gen_mode: 'custom', gen_custom_steps: JSON.stringify(steps) })
    expect(w.find(tid('gen-person')).classes()).not.toContain('card-inactive')
    await pickName(w, 'Наташа')
    expect(w.find(tid('gen-person-missing')).text()).toContain('2')
    await generate(w)
    const person = EnqueueCompound.mock.calls[0][0].person
    expect(person.loras).toEqual({ 'Z-Image': 'natasha_curated' })
  })
})
