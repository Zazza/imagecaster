import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import {
  ListPresets, ListPresetTypes, ListCompoundPresets, GetModelCatalog, ListPeople, FaceSwapStatus,
  StylistPresets, StylistPlan, StylistShoot, StylistAppearance, GetQueue, StylistFrames,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { loadPeople } from '../peopleStore.js'
import { resetStylistPlan, useStylistPlan } from '../composables/useStylistPlan.js'
import StylistPage from '../components/StylistPage.vue'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

const CATALOG = {
  sd_models: [
    { name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' },
    { name: 'moss_image_sft_bf16', base: 'Moss-Image', filename: 'moss_image_sft_bf16.safetensors' },
    { name: 'juggernautXL_ragnarokBy', base: 'SDXL', filename: 'juggernautXL_ragnarokBy.safetensors' },
  ],
}
const PRESETS = [
  { id: 1, type_id: 10, name: 'Фото Z', preset_type: 'general', loras: '', model_name: 'z-image-turbo-fp8-aio.gguf' },
  { id: 2, type_id: 11, name: 'Плёнка Moss', preset_type: 'general', loras: '[{"name":"film","weight":0.5}]', model_name: 'moss_image_sft_bf16.safetensors' },
  { id: 3, type_id: 12, name: 'Джаггернаут', preset_type: 'general', loras: '', model_name: 'juggernautXL_ragnarokBy.safetensors' },
  { id: 4, type_id: 13, name: 'Эффект Z', preset_type: 'post_light', loras: '', model_name: 'z-image-turbo-fp8-aio.gguf' },
]
const PIPE = { id: 12, name: 'Z → Moss', steps: [{ preset_id: 1, denoising_strength: 0 }, { preset_id: 2, denoising_strength: 0.4 }] }

const NAT_Z = { name: 'nat_z', gender: 'woman', base: 'Z-Image', lora: 'nat_z', display_name: 'Наташа' }
const NAT_MOSS = { name: 'nat_moss', gender: 'woman', base: 'Moss-Image', lora: 'nat_moss', display_name: 'Наташа' }

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

async function usePeople(list) {
  ListPeople.mockImplementation(() => Promise.resolve(list.map(p => ({ ...p }))))
  await loadPeople()
}

beforeEach(async () => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  useStylistPlan().form.frames = 1
  GetModelCatalog.mockImplementation(() => Promise.resolve(structuredClone(CATALOG)))
  ListPresets.mockImplementation(() => Promise.resolve(structuredClone(PRESETS)))
  ListPresetTypes.mockImplementation(() => Promise.resolve([
    { id: 10, name: 'Фото' }, { id: 11, name: 'Плёнка' }, { id: 12, name: 'SDXL' }, { id: 13, name: 'post_light' }, { id: 14, name: 'Пустая' },
  ]))
  ListCompoundPresets.mockImplementation(() => Promise.resolve([structuredClone(PIPE)]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('not_installed'))
  StylistPresets.mockImplementation(() => Promise.resolve(structuredClone(BASE)))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: SERIES(), warnings: [], used_llm: true }))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  StylistFrames.mockImplementation(() => Promise.resolve([]))
  StylistShoot.mockImplementation(() => Promise.resolve([1]))
  await usePeople([NAT_Z, NAT_MOSS])
})

const options = (w) => w.find(tid('stylist-target')).findAll('option').map(o => o.text())

async function pickKind(w, kind) {
  await w.find(tid('stylist-kind-' + kind)).trigger('click')
  await settle()
}

async function shootReq(w) {
  await w.find(tid('stylist-plan')).trigger('click')
  await settle()
  await w.find(tid('stylist-shoot')).trigger('click')
  await settle()
  expect(StylistShoot).toHaveBeenCalledTimes(1)
  return StylistShoot.mock.calls[0][0]
}

async function openOverrides(w) {
  await w.find(`${tid('stylist-overrides')} .fold-head`).trigger('click')
  await settle()
}

describe('Stylist «Чем снимать»: Модель / Стиль / Рабочий процесс', () => {
  it('shows three kinds; each lists only what fits the person\'s LoRAs', async () => {
    const w = mount(StylistPage)
    await settle()
    expect(['base', 'preset', 'pipe'].map(k => w.find(tid('stylist-kind-' + k)).text())).toEqual([t('stylist.kind_base'), t('stylist.kind_preset'), t('stylist.kind_pipe')])
    expect(options(w)).toHaveLength(2)
    await pickKind(w, 'preset')
    expect(options(w)).toEqual(['Фото Z', 'Плёнка Moss'])
    await pickKind(w, 'pipe')
    expect(options(w)).toEqual(['Z → Moss'])
  })

  it('a style sends preset id and the person\'s LoRA for the style\'s model', async () => {
    const w = mount(StylistPage)
    await settle()
    await pickKind(w, 'preset')
    await w.find(tid('stylist-target')).setValue('preset:2')
    await settle()
    const req = await shootReq(w)
    expect(req.preset).toBe(2)
    expect(req.person).toBe('nat_moss')
    expect(req.base).toBeUndefined()
    expect(req.pipeline).toBeUndefined()
    expect(req.overrides).toBeUndefined()
  })

  it('kind without items is disabled with a reason', async () => {
    await usePeople([NAT_Z])
    ListCompoundPresets.mockImplementation(() => Promise.resolve([]))
    const w = mount(StylistPage)
    await settle()
    const pipe = w.find(tid('stylist-kind-pipe'))
    expect(pipe.attributes('disabled')).toBeDefined()
    expect(pipe.attributes('title')).toBe(t('stylist.kind_none_pipe'))
    await pickKind(w, 'preset')
    expect(options(w)).toEqual(['Фото Z'])
  })
})

describe('Stylist «Стиль»: group select as in «Генерация»', () => {
  it('lists only groups that have styles for the person\'s LoRAs and filters by group', async () => {
    const w = mount(StylistPage)
    await settle()
    expect(w.find(tid('stylist-preset-group')).exists()).toBe(false)
    await pickKind(w, 'preset')
    const groups = w.find(tid('stylist-preset-group')).findAll('option').map(o => o.text())
    expect(groups).toEqual([t('generate.all_types'), 'Фото', 'Плёнка'])
    await w.find(tid('stylist-preset-group')).setValue('11')
    await settle()
    expect(options(w)).toEqual(['Плёнка Moss'])
    await w.find(tid('stylist-preset-group')).setValue(null)
    await settle()
    expect(options(w)).toEqual(['Фото Z', 'Плёнка Moss'])
  })

  it('a saved group that no longer has styles falls back to all groups', async () => {
    const { useStylistPlan } = await import('../composables/useStylistPlan.js')
    useStylistPlan().form.presetGroup = 12
    useStylistPlan().form.targetKind = 'preset'
    const w = mount(StylistPage)
    await settle()
    expect(useStylistPlan().form.presetGroup).toBe(null)
    expect(options(w)).toEqual(['Фото Z', 'Плёнка Moss'])
  })

  it('choosing a group keeps the target inside it', async () => {
    const w = mount(StylistPage)
    await settle()
    await pickKind(w, 'preset')
    await w.find(tid('stylist-target')).setValue('preset:1')
    await w.find(tid('stylist-preset-group')).setValue('11')
    await settle()
    const req = await shootReq(w)
    expect(req.preset).toBe(2)
  })
})

describe('Stylist «Дополнительно»: parameters and LoRA', () => {
  it('model: overrides go into the request, no model choice in the panel', async () => {
    const w = mount(StylistPage)
    await settle()
    await openOverrides(w)
    const panel = w.find(tid('stylist-overrides'))
    expect(panel.text()).not.toContain(t('overrides.model'))
    await panel.find('input[type=number]').setValue(12)
    await settle()
    const req = await shootReq(w)
    expect(req.base).toBeTruthy()
    expect(req.overrides).toEqual({ steps: 12 })
  })

  it('style: panel starts with the style\'s LoRAs; «ignore» sends an empty list', async () => {
    const w = mount(StylistPage)
    await settle()
    await pickKind(w, 'preset')
    await w.find(tid('stylist-target')).setValue('preset:2')
    await settle()
    await openOverrides(w)
    const panel = w.find(tid('stylist-overrides'))
    expect(panel.find('.lora-row input[type=text]').element.value).toBe('film')
    await panel.find('input[type=checkbox]').setValue(true)
    await settle()
    const req = await shootReq(w)
    expect(req.preset).toBe(2)
    expect(JSON.parse(req.overrides.loras)).toEqual([])
  })

  it('changing what to shoot with resets the overrides', async () => {
    const w = mount(StylistPage)
    await settle()
    await openOverrides(w)
    await w.find(tid('stylist-overrides')).find('input[type=number]').setValue(12)
    await settle()
    await pickKind(w, 'preset')
    const req = await shootReq(w)
    expect(req.overrides).toBeUndefined()
  })

  it('workflow: no panel, a hint instead, overrides are not sent', async () => {
    const w = mount(StylistPage)
    await settle()
    await openOverrides(w)
    await w.find(tid('stylist-overrides')).find('input[type=number]').setValue(12)
    await settle()
    await pickKind(w, 'pipe')
    expect(w.find(tid('stylist-overrides')).exists()).toBe(false)
    expect(w.find(tid('stylist-overrides-pipe')).text()).toBe(t('stylist.overrides_pipe'))
    const req = await shootReq(w)
    expect(req.pipeline).toBe(12)
    expect(req.overrides).toBeUndefined()
  })
})

describe('Stylist: coming back to the page', () => {
  it('keeps the chosen style and workflow, drops overrides the panel cannot show', async () => {
    for (const [kind, id, field, want] of [['preset', 'preset:2', 'preset', 2], ['pipe', 'pipe:12', 'pipeline', 12]]) {
      StylistShoot.mockClear()
      const first = mount(StylistPage)
      await settle()
      await pickKind(first, kind)
      await first.find(tid('stylist-target')).setValue(id)
      await settle()
      if (kind === 'preset') {
        await openOverrides(first)
        await first.find(tid('stylist-overrides')).find('input[type=number]').setValue(40)
        await settle()
      }
      first.unmount()
      const w = mount(StylistPage)
      await settle()
      expect(w.find(tid('stylist-kind-' + kind)).attributes('aria-pressed')).toBe('true')
      expect(w.find(tid('stylist-target')).element.value).toBe(id)
      const req = await shootReq(w)
      expect(req[field]).toBe(want)
      expect(req.overrides).toBeUndefined()
      w.unmount()
      resetStylistPlan()
      useStylistPlan().form.frames = 1
    }
  })
})
