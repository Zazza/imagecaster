import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, StylistShoot, StylistAppearance, ListPeople, FaceSwapStatus, GetQueue,
  GetSessionThumb, GetSessionImage, StylistFrames, StylistLastShoot, StylistReshoot, UpscaleSessionItem, OpenInHistory,
  StylistLoadPlan, StylistSavePlan, StylistLoadGarment, StylistSaveGarment, StylistRegenerateSeries,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { resetStylistPlan, useStylistPlan, planSnapshot, applyPlanSnapshot } from '../composables/useStylistPlan.js'
import { registerHistoryViewer } from '../historyViewer.js'
import StylistPage from '../components/StylistPage.vue'

enableAutoUnmount(afterEach)

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

describe('Stylist: actions under the big frame (as in «Генерация»)', () => {
  it('shows the last finished frame with Reshoot, Upscale ×2 and «В постобработку»', async () => {
    const w = mount(StylistPage)
    await settle()
    expect(bigSrc(w)).toContain('img102')
    expect(w.find(tid('stylist-reshoot')).text()).toBe(t('stylist.reshoot'))
    expect(w.find(tid('stylist-upscale')).text()).toBe(t('generate.btn_upscale_x2'))
    expect(w.find(tid('stylist-to-post')).text()).toBe(t('post.btn_send'))
  })

  it('«В постобработку» opens the frame in History and goes to post-processing', async () => {
    const w = mount(StylistPage)
    await settle()
    await tiles(w)[0].trigger('click')
    await settle()
    await w.find(tid('stylist-to-post')).trigger('click')
    await settle()
    expect(OpenInHistory).toHaveBeenCalledWith(101)
    expect(w.emitted('navigate')).toEqual([[{ page: 'postprocess' }]])
  })

  it('upscale shows the enlarged copy; picking another frame goes back', async () => {
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-upscale')).trigger('click')
    await settle()
    expect(UpscaleSessionItem).toHaveBeenCalledWith(102, 0)
    expect(bigSrc(w)).toContain('img500')
    await w.find(tid('stylist-to-post')).trigger('click')
    await settle()
    expect(OpenInHistory).toHaveBeenCalledWith(500)
    await tiles(w)[0].trigger('click')
    await settle()
    expect(bigSrc(w)).toContain('img101')
  })

  it('upscale without a saved copy reports an error', async () => {
    UpscaleSessionItem.mockImplementation(() => Promise.resolve({ image: '' }))
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-upscale')).trigger('click')
    await settle()
    expect(w.text()).toContain(t('generate.error_upscale_x2'))
    expect(bigSrc(w)).toContain('img102')
  })

  it('clicking the big frame opens the History viewer when there is one', async () => {
    const opener = vi.fn(() => true)
    const off = registerHistoryViewer(opener)
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-big')).trigger('click')
    await settle()
    expect(opener).toHaveBeenCalledWith(102)
    off()
  })

  it('a failed frame shows why and offers only Reshoot', async () => {
    const w = mount(StylistPage)
    await settle()
    await tiles(w)[2].trigger('click')
    await settle()
    expect(w.find(tid('stylist-big')).exists()).toBe(false)
    expect(w.find(tid('stylist-frame-failed')).text()).toContain('SD упал')
    expect(w.find(tid('stylist-reshoot')).exists()).toBe(true)
    expect(w.find(tid('stylist-upscale')).exists()).toBe(false)
    expect(w.find(tid('stylist-to-post')).exists()).toBe(false)
  })

  it('Reshoot puts the new job in place of the old one', async () => {
    const w = mount(StylistPage)
    await settle()
    await tiles(w)[0].trigger('click')
    await settle()
    await w.find(tid('stylist-reshoot')).trigger('click')
    await settle()
    expect(StylistReshoot).toHaveBeenCalledWith(1, 'normal')
    expect(StylistFrames).toHaveBeenLastCalledWith([9, 2, 3])
    expect(tiles(w)).toHaveLength(3)
    expect(bigSrc(w)).toContain('img102')
  })

  it('Reshoot error is shown and the strip stays', async () => {
    StylistReshoot.mockImplementation(() => Promise.reject(new Error('этот кадр нельзя переснять')))
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-reshoot')).trigger('click')
    await settle()
    expect(w.text()).toContain('этот кадр нельзя переснять')
    expect(StylistFrames).toHaveBeenLastCalledWith([1, 2, 3])
  })
})

describe('Stylist: regenerate uses the series slot', () => {
  it('after deleting a series the next one regenerates as itself', async () => {
    StylistLastShoot.mockImplementation(() => Promise.resolve([]))
    useStylistPlan().form.frames = 3
    const three = [0, 1, 2].map(slot => ({ ...SERIES()[0], outfit: 'o' + slot, slot }))
    StylistPlan.mockImplementation(() => Promise.resolve({ series: three, warnings: [], used_llm: false }))
    StylistRegenerateSeries.mockImplementation((req, slot) => Promise.resolve({ series: { ...three[slot], outfit: 'new' + slot }, warnings: [] }))
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.findAll(tid('series-delete'))[1].trigger('click')
    await settle()
    await w.findAll(tid('series-regenerate'))[1].trigger('click')
    await settle()
    expect(StylistRegenerateSeries.mock.calls[0][1]).toBe(2)
    expect(useStylistPlan().series.value.map(s => s.outfit)).toEqual(['o0', 'new2'])
  })
})

describe('Stylist: the last plan survives a restart', () => {
  it('snapshot round-trip keeps form and series, not overrides', () => {
    const { form, series } = useStylistPlan()
    form.person = 'Анна'
    form.wish = 'море'
    form.garment = 'abc'
    form.aspects.hair = { mode: 'pick', ids: ['h1'] }
    form.overrides = { steps: 5 }
    series.value = SERIES()
    const raw = planSnapshot()
    expect(JSON.parse(raw).form.overrides).toBeUndefined()
    expect(JSON.parse(raw).form.garment).toBeUndefined()
    resetStylistPlan()
    expect(applyPlanSnapshot(raw)).toBe(true)
    expect(form.person).toBe('Анна')
    expect(form.wish).toBe('море')
    expect(form.garment).toBe('')
    expect(form.aspects.hair).toEqual({ mode: 'pick', ids: ['h1'] })
    expect(form.aspects.outfit.mode).toBe('auto')
    expect(series.value).toHaveLength(1)
    expect(applyPlanSnapshot('{broken')).toBe(false)
    expect(applyPlanSnapshot('{"v":99,"form":{}}')).toBe(false)
  })

  it('restores the saved plan on open and saves edits', async () => {
    StylistLastShoot.mockImplementation(() => Promise.resolve([]))
    StylistLoadPlan.mockImplementation(() => Promise.resolve(JSON.stringify({ v: 1, form: { person: 'anna', wish: 'море' }, series: SERIES() })))
    const w = mount(StylistPage)
    await settle()
    expect(w.findAll(tid('series-card'))).toHaveLength(1)
    const { form } = useStylistPlan()
    expect(form.wish).toBe('море')
    StylistSavePlan.mockClear()
    form.wish = 'горы'
    await new Promise(r => setTimeout(r, 900))
    expect(StylistSavePlan).toHaveBeenCalledTimes(1)
    expect(JSON.parse(StylistSavePlan.mock.calls[0][0]).form.wish).toBe('горы')
  })

  it('the garment photo is restored and saved separately, only when it changes', async () => {
    StylistLastShoot.mockImplementation(() => Promise.resolve([]))
    StylistLoadPlan.mockImplementation(() => Promise.resolve(JSON.stringify({ v: 1, form: { person: 'anna', garmentOutfit: 'red dress' }, series: SERIES() })))
    StylistLoadGarment.mockImplementation(() => Promise.resolve('/9j/garment'))
    mount(StylistPage)
    await settle()
    const { form } = useStylistPlan()
    expect(form.garment).toBe('/9j/garment')
    expect(form.garmentOutfit).toBe('red dress')
    form.wish = 'горы'
    await new Promise(r => setTimeout(r, 900))
    expect(StylistSavePlan).toHaveBeenCalled()
    expect(StylistSaveGarment).not.toHaveBeenCalled()
    form.garment = '/9j/other'
    await new Promise(r => setTimeout(r, 900))
    expect(StylistSaveGarment).toHaveBeenCalledWith('/9j/other')
  })

  it('a failed save is shown', async () => {
    StylistLastShoot.mockImplementation(() => Promise.resolve([]))
    StylistSavePlan.mockImplementation(() => Promise.reject(new Error('disk full')))
    const w = mount(StylistPage)
    await settle()
    useStylistPlan().form.wish = 'x'
    await new Promise(r => setTimeout(r, 900))
    await settle()
    expect(w.text()).toContain('disk full')
  })

  it('with both a saved plan and a saved shoot the page opens on the plan', async () => {
    StylistLoadPlan.mockImplementation(() => Promise.resolve(JSON.stringify({ v: 1, form: { person: 'anna' }, series: SERIES() })))
    const w = mount(StylistPage)
    await settle()
    expect(w.find(tid('stylist-view-plan')).attributes('aria-pressed')).toBe('true')
    expect(w.find(tid('stylist-view-frames')).text()).toContain('3')
  })

  it('does not save before the saved plan is loaded', async () => {
    let resolve
    StylistLoadPlan.mockImplementation(() => new Promise(r => { resolve = r }))
    const w = mount(StylistPage)
    await settle()
    const { form } = useStylistPlan()
    form.wish = 'x'
    await new Promise(r => setTimeout(r, 900))
    expect(StylistSavePlan).not.toHaveBeenCalled()
    resolve('')
    await settle()
    w.unmount()
  })
})

describe('reshoot uses the chosen frame size', () => {
  it('passes the size from «Дополнительно»', async () => {
    const { useStylistPlan, reshootFrame } = await import('../composables/useStylistPlan.js')
    const { form } = useStylistPlan()
    form.frameSize = 'large'
    StylistReshoot.mockImplementation(() => Promise.resolve(0))
    await reshootFrame(5)
    expect(StylistReshoot).toHaveBeenLastCalledWith(5, 'large')
    form.frameSize = 'normal'
  })
})
