import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, StylistShoot, StylistAppearance, ListPeople, FaceSwapStatus, GetQueue,
  GetSessionThumb, GetSessionImage, StylistFrames, StylistLastShoot, StylistReshoot, UpscaleSessionItem, OpenInHistory,
  StylistLoadPlan, StylistSavePlan, StylistLoadGarment, StylistSaveGarment, StylistRegenerateSeries,
  CancelQueueJob,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { resetStylistPlan, useStylistPlan, planSnapshot, applyPlanSnapshot } from '../composables/useStylistPlan.js'
import { registerHistoryViewer } from '../historyViewer.js'
import StylistPage from '../components/StylistPage.vue'
import FloatingGenerationPanel from '../components/FloatingGenerationPanel.vue'

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
const LIVE_IMAGE = 'data:image/png;base64,iVBORw0KGgo-live-frame'
const emitLive = (jobId = 2) => EventsEmit('sd:progress', {
  queue_job_id: jobId, job: `remote-${jobId}`, stage: 1, preview_pass: 1,
  preview_frame: 1, preview: LIVE_IMAGE, progress: 0.5, steps: 20, eta_relative: 3,
})

describe('Stylist: full decoded live frame in the main image area', () => {
  beforeEach(() => {
    state[2] = { status: 'running', item_id: 0 }
    CancelQueueJob.mockResolvedValue(undefined)
  })

  it('Stylist uses the shared floating panel and retains collapse through a stage change', async () => {
    const w = mount(StylistPage)
    await settle()
    emitLive()
    await settle()
    const panel = w.findComponent(FloatingGenerationPanel)
    expect(panel.exists()).toBe(true)
    await panel.get('[data-testid="progress-panel-toggle"]').trigger('click')
    expect(panel.get('[data-testid="progress-panel-toggle"]').attributes('aria-expanded')).toBe('false')
    EventsEmit('sd:progress', { queue_job_id: 2, job: 'garment', stage: 2, progress: 0.1, steps: 30, preview_available: true })
    await settle()
    expect(w.findComponent(FloatingGenerationPanel).get('[data-testid="progress-panel-toggle"]').attributes('aria-expanded')).toBe('false')
    await panel.get('.gp-interrupt').trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(2)
  })

  it('replaces the previous completed image in its full image area, retaining progress and cancel', async () => {
    const w = mount(StylistPage)
    await settle()
    const finalImage = w.find(tid('stylist-big'))
    const imageArea = finalImage.element.parentElement
    const imageClasses = finalImage.classes()
    expect(finalImage.attributes('src')).toContain('img101')

    emitLive()
    await settle()
    const liveImage = w.find(`.generate-image-area img[src="${LIVE_IMAGE}"]`)
    expect(liveImage.exists()).toBe(true)
    expect(liveImage.element.parentElement).toBe(imageArea)
    expect(liveImage.classes()).toEqual(expect.arrayContaining(imageClasses))
    expect(w.find(`${tid('stylist-progress')} img`).exists()).toBe(false)
    expect(imageArea.querySelector('img[src*="img101"]')).toBeNull()
    expect(w.find(tid('stylist-upscale')).exists()).toBe(false)
    expect(w.find(tid('stylist-to-post')).exists()).toBe(false)
    expect(w.find('.resolution-upscale').exists()).toBe(false)
    expect(w.find(tid('stylist-progress')).text()).toContain('50%')
    expect(w.find(tid('stylist-progress')).text()).toContain(t('progress.sd_step', { current: 10, total: 20 }))
    await w.find(`${tid('stylist-progress')} .gp-interrupt`).trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(2)
  })

  it('ignores another job and drops the live frame when the user selects a finished frame', async () => {
    const w = mount(StylistPage)
    await settle()
    emitLive(999)
    await settle()
    expect(w.find(`img[src="${LIVE_IMAGE}"]`).exists()).toBe(false)
    expect(bigSrc(w)).toContain('img101')
    emitLive()
    await settle()
    expect(w.find(`.generate-image-area img[src="${LIVE_IMAGE}"]`).exists()).toBe(true)
    await tiles(w)[0].trigger('click')
    await settle()
    expect(w.find(`img[src="${LIVE_IMAGE}"]`).exists()).toBe(false)
    expect(bigSrc(w)).toContain('img101')
    expect(w.find(tid('stylist-upscale')).exists()).toBe(true)
  })

  it('returns to the running frame immediately from a completed frame without another progress event', async () => {
    const w = mount(StylistPage)
    await settle()
    emitLive()
    await settle()
    await tiles(w)[0].trigger('click')
    await settle()
    expect(bigSrc(w)).toContain('img101')
    expect(tiles(w)[0].classes()).toContain('active')

    await tiles(w)[1].trigger('click')
    await settle()
    expect(bigSrc(w)).toBe(LIVE_IMAGE)
    expect(tiles(w)[1].classes()).toContain('active')
    expect(tiles(w)[0].classes()).not.toContain('active')
    expect(w.find(tid('stylist-actions')).exists()).toBe(false)
    expect(w.find('.resolution-upscale').exists()).toBe(false)

    await tiles(w)[0].trigger('click')
    await settle()
    expect(bigSrc(w)).toContain('img101')
    expect(tiles(w)[0].classes()).toContain('active')
    expect(tiles(w)[1].classes()).not.toContain('active')
    expect(w.find(tid('stylist-upscale')).exists()).toBe(true)
  })

  it('does not let a pending frame capture either the finished selection or the running view', async () => {
    state[3] = { status: 'pending', item_id: 0 }
    const w = mount(StylistPage)
    await settle()
    emitLive()
    await settle()
    await tiles(w)[0].trigger('click')
    await settle()
    await tiles(w)[2].trigger('click')
    await settle()
    expect(bigSrc(w)).toContain('img101')
    expect(tiles(w)[0].classes()).toContain('active')
    expect(tiles(w)[2].classes()).not.toContain('active')

    await tiles(w)[1].trigger('click')
    await settle()
    await tiles(w)[2].trigger('click')
    await settle()
    expect(bigSrc(w)).toBe(LIVE_IMAGE)
    expect(tiles(w)[1].classes()).toContain('active')
    expect(tiles(w)[2].classes()).not.toContain('active')
  })

  it('keeps the current generated image through the garment-stage gap instead of an earlier result', async () => {
    const w = mount(StylistPage)
    await settle()
    emitLive()
    await settle()
    const progressEvents = [
      { queue_job_id: 2, job: 'remote-2', stage: 1, progress: 1, steps: 20, preview_available: true },
      { queue_job_id: 2, job: 'garment-stage', stage: 2, progress: 0, steps: 0, preview_available: false },
      { queue_job_id: 2, job: 'garment-stage', stage: 2, progress: 0, steps: 20, preview_available: true },
    ]
    for (const event of progressEvents) {
      EventsEmit('sd:progress', event)
      await settle()
      expect(bigSrc(w)).toBe(LIVE_IMAGE)
      expect(w.find(tid('stylist-actions')).exists()).toBe(false)
      expect(w.find(tid('stylist-progress')).text()).not.toContain(t('progress.preview_unavailable'))
    }
    const garmentImage = 'data:image/png;base64,iVBORw0KGgo-garment-frame'
    EventsEmit('sd:progress', { queue_job_id: 2, job: 'garment-stage', stage: 2, progress: 0.5, steps: 20, preview_available: true, preview: garmentImage, preview_frame: 1, preview_pass: 1 })
    await settle()
    expect(bigSrc(w)).toBe(garmentImage)
    await tiles(w)[0].trigger('click')
    await settle()
    expect(bigSrc(w)).toContain('img101')
    await tiles(w)[1].trigger('click')
    await settle()
    expect(bigSrc(w)).toBe(garmentImage)
  })

  it('clears the live frame when its job is no longer running', async () => {
    const w = mount(StylistPage)
    await settle()
    emitLive()
    await settle()
    expect(w.find(`.generate-image-area img[src="${LIVE_IMAGE}"]`).exists()).toBe(true)
    state[2] = { status: 'cancelled', item_id: 0 }
    EventsEmit('queue:changed')
    await settle()
    expect(w.find(`img[src="${LIVE_IMAGE}"]`).exists()).toBe(false)
    expect(bigSrc(w)).toContain('img101')
  })
})

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
