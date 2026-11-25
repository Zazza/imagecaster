import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'
import {
  StylistEdit, StylistEditWithEditor, StylistTranslate, StylistPresets, StylistPlan, StylistShoot, StylistAppearance, ListPeople, FaceSwapStatus, GetQueue,
  GetSessionThumb, GetSessionImage, StylistFrames, StylistLastShoot, StylistReshoot, UpscaleSessionItem, OpenInHistory,
  StylistLoadPlan, StylistSavePlan, StylistLoadGarment, StylistSaveGarment, StylistRegenerateSeries,
} from '../wailsjs/go/main/App.js'
import { api } from '../api.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { resetStylistPlan, useStylistPlan, editFrame, planSnapshot, applyPlanSnapshot } from '../composables/useStylistPlan.js'
import { registerHistoryViewer } from '../historyViewer.js'
import StylistPage from '../components/StylistPage.vue'

enableAutoUnmount(afterEach)

const SNAPSHOT_WEIGHT = 0.75

const tid = (id) => `[data-testid="${id}"]`

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

let state
const frameList = (ids) => Promise.resolve(ids.map(id => ({ job_id: id, item_id: 0, error: '', shot: 'Анфас', ...(state[id] || { status: 'pending' }) })))

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  useStylistPlan().form.frames = 1
  localStorage.clear()
  vi.spyOn(api, 'getSettings').mockResolvedValue({})
  vi.spyOn(api, 'updateSettings').mockResolvedValue()
  vi.spyOn(api, 'getModelCatalog').mockResolvedValue({ sd_models: [{ name: 'flux-2-klein-4b', filename: 'flux-2-klein-4b-Q8_0.gguf', modules: [] }] })
  vi.spyOn(api, 'getServerModels').mockImplementation(async type => type === 'sd' ? [{ name: 'flux-2-klein-4b-Q8_0.gguf' }] : [])
  state = {
    1: { status: 'completed', item_id: 101 },
    2: { status: 'completed', item_id: 102 },
    3: { status: 'failed', error: 'SD упал' },
  }
  StylistPresets.mockImplementation(() => Promise.resolve(structuredClone(BASE)))
  ListPeople.mockImplementation(() => Promise.resolve([{ name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman' }]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: SERIES(), warnings: [], used_llm: true }))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  GetSessionThumb.mockImplementation(() => Promise.resolve('/9j/t'))
  GetSessionImage.mockImplementation((id) => Promise.resolve('/9j/img' + id))
  StylistFrames.mockImplementation(frameList)
  StylistLastShoot.mockImplementation(() => Promise.resolve([1, 2, 3]))
  StylistEditWithEditor.mockImplementation(() => Promise.resolve(9))
  StylistTranslate.mockImplementation(texts => Promise.resolve(texts))
  StylistReshoot.mockImplementation(() => Promise.resolve(9))
  UpscaleSessionItem.mockImplementation(() => Promise.resolve({ image: '/9j/big', session_item_id: 500 }))
  StylistLoadGarment.mockImplementation(() => Promise.resolve(''))
  StylistSaveGarment.mockImplementation(() => Promise.resolve())
  OpenInHistory.mockImplementation(() => Promise.resolve())
  StylistLoadPlan.mockImplementation(() => Promise.resolve(''))
  StylistSavePlan.mockImplementation(() => Promise.resolve())
})

const tiles = (w) => w.findAll(tid('stylist-frame'))
const bigSrc = (w) => w.find(tid('stylist-big')).attributes('src')

const edit = async (w, text = 'red dress') => {
  await w.find(tid('stylist-edit-toggle')).trigger('click')
  await w.find(tid('stylist-edit-instruction')).setValue(text)
}
const completeVariant = async () => {
  state[9] = { status: 'completed', item_id: 109 }
  EventsEmit('queue:changed')
  await settle()
}

describe('Stylist: edit the selected frame', () => {
  it('opens a collapsible editor below the image, rejecting empty wishes', async () => {
    const w = mount(StylistPage)
    await settle()
    expect(w.find(tid('stylist-edit-panel')).exists()).toBe(false)
    await edit(w, '   ')
    expect(w.find(tid('stylist-edit-panel')).exists()).toBe(true)
    expect(w.find(tid('stylist-edit-submit')).element.disabled).toBe(true)
    await w.find(tid('stylist-edit-toggle')).trigger('click')
    expect(w.find(tid('stylist-edit-panel')).exists()).toBe(false)
    expect(StylistEditWithEditor).not.toHaveBeenCalled()
  })

  it.each(['pending', 'failed'])('does not offer editing for a %s frame', async status => {
    state[3] = { status, error: status === 'failed' ? 'broken' : '' }
    if (status === 'pending') StylistLastShoot.mockResolvedValue([3])
    const w = mount(StylistPage)
    await settle()
    if (status === 'failed') await tiles(w)[2].trigger('click')
    await settle()
    expect(w.find(tid('stylist-edit-toggle')).exists()).toBe(false)
  })

  it('appends a variant, preserves originals and selects its completed result', async () => {
    const w = mount(StylistPage)
    await settle()
    await edit(w)
    await w.find(tid('stylist-edit-submit')).trigger('click')
    await settle()
    expect(StylistEditWithEditor).toHaveBeenCalledWith({ job_id: 2, instruction: 'red dress', editor: { model: 'flux-2-klein-4b-Q8_0.gguf', loras: [] } })
    expect(StylistTranslate).not.toHaveBeenCalled()
    expect(StylistFrames).toHaveBeenLastCalledWith([1, 2, 3, 9])
    expect(tiles(w)).toHaveLength(4)
    await completeVariant()
    expect(bigSrc(w)).toContain('img109')
    await tiles(w)[1].trigger('click')
    await settle()
    expect(bigSrc(w)).toContain('img102')
  })

  it('captures the source before translation and preserves a later manual selection', async () => {
    let translated
    StylistTranslate.mockImplementation(() => new Promise(resolve => { translated = resolve }))
    const w = mount(StylistPage)
    await settle()
    await edit(w, 'красное платье')
    await w.find(tid('stylist-edit-submit')).trigger('click')
    await settle()
    expect(StylistTranslate).toHaveBeenCalledWith(['красное платье'])
    await tiles(w)[0].trigger('click')
    await settle()
    translated(['red dress'])
    await settle()
    expect(StylistEditWithEditor).toHaveBeenCalledWith({ job_id: 2, instruction: 'red dress', editor: { model: 'flux-2-klein-4b-Q8_0.gguf', loras: [] } })
    await completeVariant()
    expect(bigSrc(w)).toContain('img101')
  })

  it('captures editor rows before awaiting translation', async () => {
    const chosen = { model: 'flux-2-klein-4b-Q8_0.gguf', loras: [{ name: 'Chosen', weight: SNAPSHOT_WEIGHT }] }
    let translated
    StylistTranslate.mockImplementation(() => new Promise(resolve => { translated = resolve }))
    mount(StylistPage)
    await settle()
    const pending = editFrame(2, 'красное платье', chosen)
    await settle()
    chosen.model = 'qwen_image_2.1-Q8_0.gguf'
    chosen.loras[0].name = 'ChangedLater'
    translated(['red dress'])
    await pending
    expect(StylistEditWithEditor).toHaveBeenCalledWith({ job_id: 2, instruction: 'red dress', editor: { model: 'flux-2-klein-4b-Q8_0.gguf', loras: [{ name: 'Chosen', weight: SNAPSHOT_WEIGHT }] } })
    expect(StylistEdit).not.toHaveBeenCalled()
  })

  it('translation failure stays visible and does not enqueue an untranslated edit', async () => {
    StylistTranslate.mockRejectedValue(new Error('translator unavailable'))
    const w = mount(StylistPage)
    await settle()
    await edit(w, 'красное платье')
    await w.find(tid('stylist-edit-submit')).trigger('click')
    await settle()
    expect(w.text()).toContain('translator unavailable')
    expect(StylistEditWithEditor).not.toHaveBeenCalled()
    expect(tiles(w)).toHaveLength(3)
  })

  it('prevents duplicate submits while enqueue is pending', async () => {
    let enqueued
    StylistEditWithEditor.mockImplementation(() => new Promise(resolve => { enqueued = resolve }))
    const w = mount(StylistPage)
    await settle()
    await edit(w)
    await w.find(tid('stylist-edit-submit')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-edit-submit')).element.disabled).toBe(true)
    await w.find(tid('stylist-edit-submit')).trigger('click')
    expect(StylistEditWithEditor).toHaveBeenCalledTimes(1)
    enqueued(9)
    await settle()
  })

  it('shows enqueue failure and keeps the source and its draft', async () => {
    StylistEditWithEditor.mockRejectedValue(new Error('Klein unavailable'))
    const w = mount(StylistPage)
    await settle()
    await edit(w)
    await w.find(tid('stylist-edit-submit')).trigger('click')
    await settle()
    expect(w.text()).toContain('Klein unavailable')
    expect(bigSrc(w)).toContain('img102')
    expect(tiles(w)).toHaveLength(3)
    expect(w.find(tid('stylist-edit-instruction')).element.value).toBe('red dress')
  })

  it('does not reuse the previous frame draft when another frame is selected', async () => {
    const w = mount(StylistPage)
    await settle()
    await edit(w)
    await tiles(w)[0].trigger('click')
    await settle()
    if (!w.find(tid('stylist-edit-instruction')).exists()) {
      await w.find(tid('stylist-edit-toggle')).trigger('click')
    }
    expect(w.find(tid('stylist-edit-instruction')).element.value).toBe('')
  })

  it('opening edit after upscale returns to the selected original', async () => {
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-upscale')).trigger('click')
    await settle()
    expect(bigSrc(w)).toContain('img500')
    await edit(w)
    expect(bigSrc(w)).toContain('img102')
  })

  it('a late enqueue cannot append its result to a newer shoot', async () => {
    let enqueued
    StylistEditWithEditor.mockImplementation(() => new Promise(resolve => { enqueued = resolve }))
    StylistShoot.mockResolvedValue([20])
    state[20] = { status: 'completed', item_id: 120 }
    const w = mount(StylistPage)
    await settle()
    await edit(w)
    await w.find(tid('stylist-edit-submit')).trigger('click')
    await settle()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot).toHaveBeenCalledTimes(1)
    enqueued(9)
    await settle()
    expect(StylistFrames).toHaveBeenLastCalledWith([20])
    expect(tiles(w)).toHaveLength(1)
  })
})
