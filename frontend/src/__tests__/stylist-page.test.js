import { describe, it, expect, vi, beforeEach, afterEach, onTestFinished } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, StylistRegenerateSeries, StylistShoot, StylistAppearance, StylistPickGarment, StylistDescribeGarment, ReadClipboardImage,
  ListPeople, FaceSwapStatus, GetQueue, GetSessionThumb, GetSessionImage, SetStylistAppearance, StylistFrames, CancelQueueJob, StylistTranslate, StylistTranslateRu,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import StylistPage from '../components/StylistPage.vue'
import AspectTab from '../components/stylist/AspectTab.vue'
import { registerHistoryViewer } from '../historyViewer.js'
import {
  buildPlanRequest, defaultAspects, removeShot, removeSeries, regenerateRequest, stylistPeople, clampFrames,
  framesFrom, latestDone, totalShots, useStylistPlan, resetStylistPlan, setRuField, setEnField, setShotRu, setShotEn, dirtyRefs, applyTranslations, planSnapshot, sourceSubmitting,
} from '../composables/useStylistPlan.js'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

const BASE = {
  version: 1,
  themes: [{ id: 'beach', name: 'Пляж' }, { id: 'winter', name: 'Зима' }],
  outfit: [{ id: 'o1', theme: 'beach', name: 'Платье', prompt: 'white dress' }, { id: 'o2', theme: 'winter', name: 'Пальто', prompt: 'wool coat' }],
  hair: [{ id: 'h1', name: 'Пучок', prompt: 'low bun' }],
  makeup: [{ id: 'm1', name: 'Без', prompt: 'no makeup' }],
  background: [{ id: 'b1', theme: 'beach', name: 'Пляж', prompt: 'sandy beach', place: 'beach' }],
  pose: [{ id: 'p1', name: 'Анфас', prompt: 'facing camera', framing: 'close', places: ['any'], orientation: 'portrait' }],
}

const SERIES = () => [
  { outfit: 'white dress', hair: 'low bun', makeup: 'no makeup', background: 'sandy beach', place: 'beach', shots: [
    { pose: 'Анфас', prompt: 'facing camera', landscape: false },
    { pose: 'Лёжа', prompt: 'lying on sand', landscape: true },
  ] },
  { outfit: 'wool coat', hair: 'low bun', makeup: 'no makeup', background: 'sandy beach', place: 'beach', shots: [
    { pose: 'Анфас', prompt: 'facing camera', landscape: false },
  ] },
]

const ANNA = { name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman', identity: true }
const BOB = { name: 'bob', base: 'Z-Image', lora: 'bob_z', gender: 'man', identity: false }

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  sourceSubmitting.value = false
  useStylistPlan().form.frames = 3
  StylistTranslateRu.mockImplementation((texts) => Promise.resolve(texts))
  StylistPresets.mockImplementation(() => Promise.resolve(structuredClone(BASE)))
  ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }, { ...BOB }, { name: 'old', base: 'SD1.5', lora: 'x' }, { name: 'nolora', base: 'SDXL', lora: '' }]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: SERIES(), warnings: [], used_llm: true }))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  StylistFrames.mockImplementation(() => Promise.resolve([]))
})

describe('useStylistPlan helpers', () => {
  it('builds the request: keep/auto drop ids, pick keeps them, garment forces outfit keep', () => {
    const aspects = defaultAspects()
    aspects.hair = { mode: 'keep', ids: ['h1'] }
    aspects.pose = { mode: 'pick', ids: ['p1'] }
    aspects.outfit = { mode: 'pick', ids: ['o1'] }
    const req = buildPlanRequest({ person: 'anna', gender: 'woman', wish: 'sea', frames: 30, themes: ['beach'], aspects, garment: '' }, 7)
    expect(req.aspects.hair).toEqual({ mode: 'keep', ids: [], text: '' })
    expect(req.aspects.pose).toEqual({ mode: 'pick', ids: ['p1'], text: '' })
    expect(req.aspects.makeup).toEqual({ mode: 'auto', ids: [], text: '' })
    expect(req.aspects.outfit).toEqual({ mode: 'pick', ids: ['o1'], text: '' })
    expect(req.frames).toBe(24)
    expect(req.seed).toBe(7)
    expect(req.garment).toBe(false)
    const withGarment = buildPlanRequest({ aspects, frames: 4, garment: 'abc' }, 1)
    expect(withGarment.aspects.outfit).toEqual({ mode: 'keep', ids: [], text: '' })
    expect(withGarment.garment).toBe(true)
  })

  it('clamps frames to 1–24', () => {
    expect([clampFrames(0), clampFrames(5), clampFrames(99), clampFrames('x')]).toEqual([1, 5, 24, 1])
  })

  it('removes a shot and drops a series left without shots', () => {
    const s = SERIES()
    const once = removeShot(s, 0, 1)
    expect(once[0].shots.map(x => x.prompt)).toEqual(['facing camera'])
    expect(s[0].shots).toHaveLength(2)
    const gone = removeShot(once, 1, 0)
    expect(gone).toHaveLength(1)
    expect(removeSeries(s, 0)[0].outfit).toBe('wool coat')
    expect(totalShots(s)).toBe(3)
  })

  it('regenerate request keeps the series shot count', () => {
    const req = regenerateRequest({ aspects: defaultAspects(), frames: 8 }, SERIES(), 1, 5)
    expect(req.frames).toBe(8)
    expect(req.shots).toBe(1)
    expect(req.seed).toBe(5)
  })

  it('shows only people with a LoRA of a supported base', () => {
    expect(stylistPeople([ANNA, { name: 'x', base: 'SD1.5', lora: 'x' }, { name: 'y', base: 'SDXL', lora: '' }]).map(p => p.name)).toEqual(['anna'])
  })

  it('maps StylistFrames rows to my frames in job order, missing ones are gone', () => {
    const rows = [
      { job_id: 6, status: 'running', item_id: 0, error: '', shot: '' },
      { job_id: 5, status: 'completed', item_id: 77, error: '', shot: 'a', garment: 'done' },
      { job_id: 9, status: 'completed', item_id: 99, error: '', shot: '' },
    ]
    const frames = framesFrom(rows, [5, 6, 8])
    expect(frames).toEqual([
      { jobId: 5, status: 'completed', itemId: 77, error: '', shot: 'a', garment: 'done' },
      { jobId: 6, status: 'running', itemId: 0, error: '', shot: '', garment: '' },
      { jobId: 8, status: 'gone', itemId: 0, error: '', shot: '', garment: '' },
    ])
    expect(latestDone(frames).itemId).toBe(77)
    expect(latestDone([...frames, { jobId: 10, status: 'completed', itemId: 101 }]).itemId).toBe(101)
    expect(latestDone([{ jobId: 1, status: 'failed', itemId: 0 }])).toBe(null)
  })
})

describe('AspectTab', () => {
  const items = BASE.outfit

  it('switches modes and multi-selects presets grouped by theme', async () => {
    const w = mount(AspectTab, { props: { aspect: 'outfit', choice: { mode: 'auto', ids: [] }, items, themes: BASE.themes } })
    expect(w.find(tid('pick-o1')).exists()).toBe(false)
    await w.find(tid('mode-keep')).trigger('click')
    expect(w.emitted('update:choice').at(-1)[0]).toEqual({ mode: 'keep', ids: [] })
    await w.setProps({ choice: { mode: 'pick', ids: ['o1'] } })
    expect(w.text()).toContain('Пляж')
    expect(w.text()).toContain('Зима')
    expect(w.find(tid('pick-o1')).classes()).toContain('active')
    await w.find(tid('pick-o2')).trigger('click')
    expect(w.emitted('update:choice').at(-1)[0]).toEqual({ mode: 'pick', ids: ['o1', 'o2'] })
    await w.find(tid('pick-o1')).trigger('click')
    expect(w.emitted('update:choice').at(-1)[0]).toEqual({ mode: 'pick', ids: [] })
  })

})

describe('StylistPage', () => {
  async function mountPage() {
    const w = mount(StylistPage)
    await settle()
    return w
  }

  it('offers source photo variants without a saved person and hides the new-shoot plan action', async () => {
    ListPeople.mockResolvedValue([])
    const w = await mountPage()
    expect(useStylistPlan().form.person).toBe('')
    await w.get(tid('stylist-source-mode')).trigger('click')
    await settle()
    expect(w.get(tid('source-photo-pick')).isVisible()).toBe(true)
    await w.findAll('[role="button"]').find(header => header.text().includes(t('source.task'))).trigger('click')
    expect(w.get(tid('source-plan')).isVisible()).toBe(true)
    expect(w.findAll(tid('stylist-plan')).filter(button => button.isVisible())).toHaveLength(0)
    expect(StylistPlan).not.toHaveBeenCalled()
  })

  it('keeps ordinary shoot actions and mode switching locked after remount during a source enqueue', async () => {
    const first = await mountPage()
    await first.get(tid('stylist-plan')).trigger('click')
    await settle()
    first.unmount()
    sourceSubmitting.value = true
    const second = await mountPage()
    expect(second.get(tid('stylist-plan')).element.disabled).toBe(true)
    expect(second.get(tid('stylist-shoot')).element.disabled).toBe(true)
    expect(second.get(tid('stylist-source-mode')).element.disabled).toBe(true)
    await second.get(tid('stylist-plan')).trigger('click')
    await second.get(tid('stylist-shoot')).trigger('click')
    expect(StylistPlan).toHaveBeenCalledTimes(1)
    expect(StylistShoot).not.toHaveBeenCalled()
    sourceSubmitting.value = false
    await settle()
    expect(second.get(tid('stylist-plan')).element.disabled).toBe(false)
    expect(second.get(tid('stylist-shoot')).element.disabled).toBe(false)
    expect(second.get(tid('stylist-source-mode')).element.disabled).toBe(false)
  })

  it('lists only supported people; face swap is on by default for a person with a face', async () => {
    const w = await mountPage()
    const opts = w.findAll(`${tid('stylist-person')} option`).map(o => o.element.value)
    expect(opts).toEqual(['anna', 'bob'])
    expect(w.find(tid('stylist-face-swap')).text()).toBe(t('stylist.face_swap_on'))
    expect(w.find(tid('stylist-face-swap-toggle')).element.checked).toBe(true)
    expect(w.find(tid('stylist-face-pass')).exists()).toBe(false)
    await w.find(tid('stylist-person')).setValue('bob')
    await settle()
    expect(w.find(tid('stylist-face-swap')).text()).toBe(t('stylist.face_swap_no_face'))
    expect(w.find(tid('stylist-face-swap-toggle')).exists()).toBe(false)
  })

  it('face swap can be turned off: the shoot goes without it, the note says so, the choice is kept in the plan', async () => {
    const w = await mountPage()
    await w.find(tid('stylist-face-swap-toggle')).setValue(false)
    await settle()
    expect(w.find(tid('stylist-face-swap')).text()).toBe(t('stylist.face_swap_off'))
    expect(JSON.parse(planSnapshot()).form.faceSwap).toBe(false)
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot.mock.calls[0][0].face_swap).toBe(false)
  })

  it('frame size: normal by default, draft goes to the shoot and is kept in the plan', async () => {
    const w = await mountPage()
    expect(w.find(tid('stylist-frame-size')).element.value).toBe('normal')
    await w.find(tid('stylist-frame-size')).setValue('draft')
    await settle()
    expect(JSON.parse(planSnapshot()).form.frameSize).toBe('draft')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot.mock.calls[0][0].size).toBe('draft')
  })

  it('garment comes from the paste button and is described', async () => {
    ReadClipboardImage.mockImplementation(() => Promise.resolve('UEFTVEVE'))
    StylistDescribeGarment.mockImplementation(() => Promise.resolve('a red dress'))
    const w = await mountPage()
    await w.find(tid('stylist-garment-paste')).trigger('click')
    await settle()
    expect(StylistPickGarment).not.toHaveBeenCalled()
    expect(StylistDescribeGarment).toHaveBeenCalledWith('UEFTVEVE')
    expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('a red dress')
  })

  describe('«Что на фото» in Russian', () => {
    async function withGarment(w) {
      await w.find(tid('stylist-garment-paste')).trigger('click')
      await settle()
    }
    async function shootNow(w) {
      await w.find(tid('stylist-plan')).trigger('click')
      await settle()
      await w.find(tid('stylist-shoot')).trigger('click')
      await settle()
    }
    beforeEach(() => {
      ReadClipboardImage.mockImplementation(() => Promise.resolve('UEFTVEVE'))
      StylistDescribeGarment.mockImplementation(() => Promise.resolve('a blazer and a blouse'))
      StylistTranslateRu.mockImplementation(() => Promise.resolve(['пиджак и блузка']))
      StylistTranslate.mockImplementation(() => Promise.resolve(['an unbuttoned blazer and a blouse']))
    })

    it('names are shown in Russian; untouched they go to the shoot in the original English', async () => {
      const w = await mountPage()
      await withGarment(w)
      expect(StylistTranslateRu).toHaveBeenCalledWith(['a blazer and a blouse'])
      expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('пиджак и блузка')
      await shootNow(w)
      expect(StylistTranslate).not.toHaveBeenCalled()
      expect(StylistShoot.mock.calls[0][0].garment_outfit).toBe('a blazer and a blouse')
    })

    it('a Russian edit is translated to English before the shoot', async () => {
      const w = await mountPage()
      await withGarment(w)
      await w.find(tid('stylist-garment-outfit')).setValue('пиджак расстёгнут и блузка')
      await shootNow(w)
      expect(StylistTranslate).toHaveBeenCalledWith(['пиджак расстёгнут и блузка'])
      expect(StylistShoot.mock.calls[0][0].garment_outfit).toBe('an unbuttoned blazer and a blouse')
    })

    it('translation failure before the shoot stops it, no Cyrillic goes out', async () => {
      StylistTranslate.mockImplementation(() => Promise.reject(new Error('LLM busy')))
      const w = await mountPage()
      await withGarment(w)
      await w.find(tid('stylist-garment-outfit')).setValue('пиджак расстёгнут')
      await shootNow(w)
      expect(StylistShoot).not.toHaveBeenCalled()
      expect(w.text()).toContain('LLM busy')
    })

    it('no Russian translation: English names stay with a note', async () => {
      StylistTranslateRu.mockImplementation(() => Promise.reject(new Error('LLM off')))
      const w = await mountPage()
      await withGarment(w)
      expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('a blazer and a blouse')
      expect(w.text()).toContain('LLM off')
    })

    it('English interface: no translation at all', async () => {
      setLocale('en')
      const w = await mountPage()
      await withGarment(w)
      expect(StylistTranslateRu).not.toHaveBeenCalled()
      expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('a blazer and a blouse')
      setLocale('ru')
    })
  })

  it('empty clipboard leaves the garment unset', async () => {
    ReadClipboardImage.mockImplementation(() => Promise.resolve(''))
    const w = await mountPage()
    await w.find(tid('stylist-garment-paste')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-garment-outfit')).exists()).toBe(false)
    expect(StylistDescribeGarment).not.toHaveBeenCalled()
  })

  const imageFile = () => new File([new Uint8Array([137, 80, 78, 71])], 'dress.png', { type: 'image/png' })

  it('garment comes from a dropped file', async () => {
    StylistDescribeGarment.mockImplementation(() => Promise.resolve('a top'))
    const w = await mountPage()
    const drop = w.find(tid('stylist-garment-drop'))
    await drop.trigger('dragover')
    expect(drop.classes()).toContain('drop-zone-active')
    await drop.trigger('drop', { dataTransfer: { files: [imageFile()] } })
    await vi.waitFor(() => expect(StylistDescribeGarment).toHaveBeenCalledWith('iVBORw=='))
    await settle()
    expect(drop.classes()).not.toContain('drop-zone-active')
    expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('a top')
  })

  it('dropped non-image is rejected with an error', async () => {
    const w = await mountPage()
    await w.find(tid('stylist-garment-drop')).trigger('drop', { dataTransfer: { files: [new File(['x'], 'a.txt', { type: 'text/plain' })] } })
    await settle()
    expect(StylistDescribeGarment).not.toHaveBeenCalled()
    expect(w.text()).toContain(t('fi.error_drop_image'))
  })

  it('Ctrl+V of an image sets the garment; text paste is untouched', async () => {
    StylistDescribeGarment.mockImplementation(() => Promise.resolve('a coat'))
    const w = await mountPage()
    const text = new Event('paste', { cancelable: true })
    text.clipboardData = { items: [{ type: 'text/plain', getAsFile: () => null }] }
    document.dispatchEvent(text)
    expect(text.defaultPrevented).toBe(false)
    const img = new Event('paste', { cancelable: true })
    img.clipboardData = { items: [{ type: 'image/png', getAsFile: imageFile }] }
    document.dispatchEvent(img)
    expect(img.defaultPrevented).toBe(true)
    await vi.waitFor(() => expect(StylistDescribeGarment).toHaveBeenCalledWith('iVBORw=='))
    w.unmount()
    vi.clearAllMocks()
    const again = new Event('paste', { cancelable: true })
    again.clipboardData = { items: [{ type: 'image/png', getAsFile: imageFile }] }
    document.dispatchEvent(again)
    expect(again.defaultPrevented).toBe(false)
  })

  it('garment photo turns the outfit tab into «what is in the photo» and sends it', async () => {
    StylistPickGarment.mockImplementation(() => Promise.resolve('iVBORw0KGgoAAAA'))
    StylistShoot.mockImplementation(() => Promise.resolve([1]))
    const w = await mountPage()
    expect(w.find(tid('garment-outfit-tab')).exists()).toBe(false)
    expect(w.find(tid('mode-auto')).exists()).toBe(true)
    await w.find(tid('stylist-garment-pick')).trigger('click')
    await settle()
    expect(w.find(tid('garment-outfit-tab')).exists()).toBe(true)
    expect(w.find(tid('mode-auto')).exists()).toBe(false)
    expect(w.find(tid('garment-outfit-tab')).text()).toBe(t('stylist.garment_outfit_moved'))
    expect(w.find(tid('stylist-section-task')).find(tid('stylist-garment-outfit')).exists()).toBe(true)
    await w.find(tid('stylist-garment-outfit')).setValue('a top and a skirt')
    await w.find(tid('tab-hair')).trigger('click')
    expect(w.find(tid('garment-outfit-tab')).exists()).toBe(false)
    expect(w.find(tid('stylist-garment-outfit')).exists()).toBe(true)
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistPlan.mock.calls[0][0].aspects.outfit.mode).toBe('keep')
    expect(StylistPlan.mock.calls[0][0].garment).toBe(true)
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot.mock.calls[0][0]).toMatchObject({ garment: 'iVBORw0KGgoAAAA', garment_outfit: 'a top and a skirt' })
    await w.find(tid('tab-outfit')).trigger('click')
    await w.find(tid('stylist-garment-clear')).trigger('click')
    expect(w.find(tid('garment-outfit-tab')).exists()).toBe(false)
  })

  it('restores and saves appearance per person, ignoring a late answer for another person', async () => {
    let resolveAnna
    StylistAppearance.mockImplementation((name) => name === 'anna'
      ? new Promise(r => { resolveAnna = r })
      : Promise.resolve('a man, 190 cm'))
    const w = await mountPage()
    await w.find(tid('stylist-person')).setValue('bob')
    await settle()
    resolveAnna('a woman, 160 cm')
    await settle()
    expect(w.find(tid('stylist-appearance')).element.value).toBe('a man, 190 cm')
    await w.find(tid('stylist-appearance')).setValue('a man, 185 cm')
    await w.find(tid('stylist-appearance')).trigger('change')
    await settle()
    expect(SetStylistAppearance).toHaveBeenCalledWith('bob', 'a man, 185 cm')
  })

  it('shows a planning error', async () => {
    StylistPlan.mockImplementation(() => Promise.reject(new Error('кадров от 1 до 24')))
    const w = await mountPage()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-error')).text()).toContain('кадров от 1 до 24')
    expect(w.findAll(tid('series-card'))).toHaveLength(0)
  })

  it('no «no people» flash while loading; a server error is shown as an error', async () => {
    let resolvePeople
    ListPeople.mockImplementation(() => new Promise(r => { resolvePeople = r }))
    let resolveBase
    StylistPresets.mockImplementation(() => new Promise(r => { resolveBase = r }))
    const w = mount(StylistPage)
    await settle()
    expect(w.find(tid('stylist-no-people')).exists()).toBe(false)
    expect(w.find(tid('stylist-people-loading')).exists()).toBe(true)
    expect(w.find(tid('stylist-plan')).element.disabled).toBe(true)
    expect(w.find(tid('stylist-plan-blocked')).text()).toContain('Загружаю')
    resolveBase(structuredClone(BASE))
    await settle()
    expect(w.find(tid('stylist-no-people')).exists()).toBe(false)
    resolvePeople([])
    await settle()
    expect(w.find(tid('stylist-no-people')).exists()).toBe(true)

    ListPeople.mockImplementation(() => Promise.reject(new Error('connection refused')))
    StylistPresets.mockImplementation(() => Promise.resolve(structuredClone(BASE)))
    const w2 = await mountPage()
    expect(w2.find(tid('stylist-people-error')).text()).toContain('connection refused')
    expect(w2.find(tid('stylist-no-people')).exists()).toBe(false)
  })

  it('unreachable server or unknown face swap status: the note says so, not «Каталог»', async () => {
    for (const status of ['unreachable', 'no_server']) {
      FaceSwapStatus.mockImplementation(() => Promise.resolve(status))
      const w = await mountPage()
      expect(w.find(tid('stylist-face-swap')).text()).toBe(t('stylist.face_swap_unknown'))
      w.unmount()
    }
    FaceSwapStatus.mockImplementation(() => Promise.reject(new Error('down')))
    const w = await mountPage()
    expect(w.find(tid('stylist-face-swap')).text()).toBe(t('stylist.face_swap_unknown'))
  })

  it('server without face swap: note says so and the shoot goes without swap', async () => {
    FaceSwapStatus.mockImplementation(() => Promise.resolve('not_installed'))
    const w = await mountPage()
    expect(w.find(tid('stylist-face-swap')).text()).toBe(t('stylist.face_swap_no_server'))
  })

  it('plans, deletes a shot, regenerates a series and shoots', async () => {
    StylistRegenerateSeries.mockImplementation(() => Promise.resolve({
      series: { outfit: 'red gown', hair: 'braid', makeup: 'glossy lips', background: 'sandy beach', place: 'beach', shots: [{ pose: 'Сбоку', prompt: 'side view', landscape: false }] },
      warnings: ['ИИ не дал идей — взяты случайные заготовки по темам'], used_llm: false,
    }))
    StylistShoot.mockImplementation(() => Promise.resolve([41, 42]))
    const w = await mountPage()
    await w.find(tid('mode-keep')).trigger('click')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistPlan.mock.calls[0][0]).toMatchObject({ person: 'anna', gender: 'woman', frames: 3 })
    expect(StylistPlan.mock.calls[0][0].aspects.outfit.mode).toBe('keep')
    expect(w.findAll(tid('series-card'))).toHaveLength(2)
    expect(w.findAll(tid('series-shot'))).toHaveLength(3)

    await w.findAll(tid('shot-delete'))[1].trigger('click')
    expect(w.findAll(tid('series-shot'))).toHaveLength(2)
    expect(w.find(tid('stylist-shoot')).text()).toContain('2')

    await w.findAll(tid('series-regenerate'))[1].trigger('click')
    await settle()
    expect(StylistRegenerateSeries).toHaveBeenCalledTimes(1)
    expect(StylistRegenerateSeries.mock.calls[0][1]).toBe(1)
    expect(StylistRegenerateSeries.mock.calls[0][0]).toMatchObject({ frames: 2, shots: 1 })
    const cards = w.findAll(tid('series-card'))
    expect(cards[1].find(tid('series-outfit')).element.value).toBe('red gown')
    expect(cards[0].find(tid('series-outfit')).element.value).toBe('white dress')
    expect(w.findAll(tid('stylist-warning'))).toHaveLength(1)

    await w.findAll(tid('series-delete'))[0].trigger('click')
    expect(w.findAll(tid('series-card'))).toHaveLength(1)

    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    const req = StylistShoot.mock.calls[0][0]
    expect(req).toMatchObject({ person: 'anna', face_swap: true, garment: '' })
    expect(req.series).toHaveLength(1)
    expect(req.series[0].outfit).toBe('red gown')
    expect(w.find(tid('stylist-shot-notice')).text()).toContain('2')
  })

  it('shows finished frames of my jobs as thumbnails although the queue list has no result', async () => {
    StylistShoot.mockImplementation(() => Promise.resolve([41]))
    GetSessionThumb.mockImplementation(() => Promise.resolve('/9j/abc'))
    GetSessionImage.mockImplementation(() => Promise.resolve('/9j/full'))
    const w = await mountPage()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistFrames).toHaveBeenLastCalledWith([41])
    GetQueue.mockImplementation(() => Promise.resolve([{ id: 41, type: 'stylist_frame', status: 'completed', progress: 1, error: '' }, { id: 40, type: 'stylist_frame', status: 'completed', error: '' }]))
    StylistFrames.mockImplementation((ids) => Promise.resolve(ids.map(id => ({ job_id: id, status: 'completed', item_id: id === 41 ? 300 : 0, error: '', shot: 'Анфас', face_swap: '' }))))
    EventsEmit('queue:changed')
    await settle()
    expect(StylistFrames).toHaveBeenLastCalledWith([41])
    expect(GetSessionThumb).toHaveBeenCalledWith(300)
    expect(GetSessionThumb).not.toHaveBeenCalledWith(1)
    const imgs = w.find(tid('stylist-results')).findAll('img')
    expect(imgs).toHaveLength(1)
    expect(imgs[0].attributes('src')).toBe('data:image/jpeg;base64,/9j/abc')
    const caption = imgs[0].element.parentElement.querySelector('[data-testid="stylist-frame-caption"]')
    expect(caption.textContent).toBe('1 · ' + t('stylist.frame_completed'))
    expect(caption.classList.contains('on-image')).toBe(true)
    expect(GetSessionImage).toHaveBeenCalledWith(300)
    expect(w.find(tid('stylist-big')).attributes('src')).toBe('data:image/jpeg;base64,/9j/full')
  })

  it('picking a garment photo fills «Что на фото» from the local model; «Определить» asks again', async () => {
    StylistPickGarment.mockImplementation(() => Promise.resolve('iVBORw0KGgoAAAA'))
    StylistDescribeGarment.mockImplementation(() => Promise.resolve('a top and a skirt'))
    const w = await mountPage()
    await w.find(tid('stylist-garment-pick')).trigger('click')
    await settle()
    expect(StylistDescribeGarment).toHaveBeenCalledWith('iVBORw0KGgoAAAA')
    expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('a top and a skirt')
    await w.find(tid('stylist-garment-outfit')).setValue('a blouse and a skirt')
    StylistDescribeGarment.mockImplementation(() => Promise.resolve('a top, a skirt and a belt'))
    await w.find(tid('stylist-garment-describe')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('a top, a skirt and a belt')
  })

  it('a remote model or an error leaves the field for the user with the reason', async () => {
    StylistPickGarment.mockImplementation(() => Promise.resolve('iVBORw0KGgoAAAA'))
    StylistDescribeGarment.mockImplementation(() => Promise.reject(new Error('определить вещи можно только локальной моделью ИИ')))
    const w = await mountPage()
    await w.find(tid('stylist-garment-pick')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('')
    expect(w.find(tid('stylist-garment-note')).text()).toContain('только локальной моделью')
    expect(w.find(tid('stylist-error')).exists()).toBe(false)
  })

  it('with a garment photo «Снять» waits for the item names', async () => {
    StylistPickGarment.mockImplementation(() => Promise.resolve('iVBORw0KGgoAAAA'))
    StylistShoot.mockImplementation(() => Promise.resolve([1]))
    const w = await mountPage()
    await w.find(tid('stylist-garment-pick')).trigger('click')
    await settle()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-shoot')).element.disabled).toBe(true)
    expect(w.find(tid('stylist-garment-unnamed')).text()).toBe(t('stylist.need_garment_outfit'))
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot).not.toHaveBeenCalled()
    await w.find(tid('tab-outfit')).trigger('click')
    await w.find(tid('stylist-garment-outfit')).setValue('  ')
    expect(w.find(tid('stylist-shoot')).element.disabled).toBe(true)
    await w.find(tid('stylist-garment-outfit')).setValue('a top and a skirt')
    expect(w.find(tid('stylist-shoot')).element.disabled).toBe(false)
    expect(w.find(tid('stylist-garment-unnamed')).exists()).toBe(false)
  })

  it('a frame where the garment was not put on is marked with the reason', async () => {
    StylistShoot.mockImplementation(() => Promise.resolve([51, 52]))
    StylistFrames.mockImplementation(() => Promise.resolve([
      { job_id: 51, status: 'completed', item_id: 0, error: '', shot: 'Анфас', face_swap: '', garment: 'done' },
      { job_id: 52, status: 'completed', item_id: 0, error: '', shot: 'Сбоку', face_swap: '', garment: 'skipped:примерка не вернула картинку' },
    ]))
    const w = await mountPage()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    EventsEmit('queue:changed')
    await settle()
    const tiles = w.findAll(tid('stylist-frame'))
    expect(tiles).toHaveLength(2)
    expect(tiles[0].find(tid('stylist-frame-no-garment')).exists()).toBe(false)
    expect(tiles[1].find(tid('stylist-frame-no-garment')).text()).toBe(t('stylist.frame_no_garment'))
    expect(tiles[1].find(tid('stylist-frame-caption')).text()).toBe('2 · ' + t('stylist.frame_completed'))
    expect(tiles[1].attributes('title')).toBe(t('stylist.frame_no_garment_title', { reason: 'примерка не вернула картинку' }))
  })

  it('switches between plan and frames: plan after planning, frames after shooting', async () => {
    StylistShoot.mockImplementation(() => Promise.resolve([41, 42]))
    const w = await mountPage()
    expect(w.find(tid('stylist-plan-empty')).exists()).toBe(true)
    expect(w.find(tid('stylist-shoot')).exists(), '«Сгенерировать» lives under the plan').toBe(false)
    await w.find(tid('stylist-view-frames')).trigger('click')
    expect(w.find(tid('stylist-frames-empty')).text()).toContain('Кадров пока нет')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-view-plan')).attributes('aria-pressed')).toBe('true')
    expect(w.findAll(tid('series-card'))).toHaveLength(2)
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-view-frames')).attributes('aria-pressed')).toBe('true')
    expect(w.findAll(tid('series-card'))).toHaveLength(0)
    expect(w.findAll(tid('stylist-frame'))).toHaveLength(2)
    await w.find(tid('stylist-view-plan')).trigger('click')
    expect(w.findAll(tid('series-card'))).toHaveLength(2)
  })

  it('strip shows status tiles, the error on hover and progress of the running frame; interrupt cancels it', async () => {
    StylistShoot.mockImplementation(() => Promise.resolve([41, 42, 43, 44]))
    const w = await mountPage()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    StylistFrames.mockImplementation(() => Promise.resolve([
      { job_id: 41, status: 'failed', item_id: 0, error: 'sd: out of memory', shot: '' },
      { job_id: 42, status: 'running', item_id: 0, error: '', shot: '' },
      { job_id: 43, status: 'pending', item_id: 0, error: '', shot: '' },
    ]))
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    const tiles = w.findAll(tid('stylist-frame'))
    expect(tiles).toHaveLength(4)
    expect(tiles[0].attributes('title')).toBe('sd: out of memory')
    expect(tiles[0].text()).toContain('не получился')
    expect(tiles[1].text()).toContain('снимается')
    expect(tiles[2].text()).toContain('ждёт')
    expect(tiles[3].text()).toContain('нет в очереди')
    expect(w.find(tid('stylist-progress')).text()).toContain('Кадр 2 из 4')
    EventsEmit('sd:progress', { progress: 0.5, steps: 20, eta_relative: 3 })
    await settle()
    expect(w.find(tid('stylist-progress')).text()).toContain('50%')
    await w.find(`${tid('stylist-progress')} .gp-interrupt`).trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(42)
    expect(w.find(tid('stylist-big')).exists()).toBe(false)
    expect(w.find(tid('stylist-frames-empty')).text()).toContain('в очереди')
  })

  it('clicking a finished tile shows it large and opens the viewer', async () => {
    const opener = vi.fn(() => true)
    onTestFinished(registerHistoryViewer(opener))
    StylistShoot.mockImplementation(() => Promise.resolve([41, 42]))
    GetSessionThumb.mockImplementation((id) => Promise.resolve('/9j/t' + id))
    GetSessionImage.mockImplementation((id) => Promise.resolve('/9j/f' + id))
    StylistFrames.mockImplementation(() => Promise.resolve([
      { job_id: 41, status: 'completed', item_id: 300, error: '', shot: '' },
      { job_id: 42, status: 'completed', item_id: 301, error: '', shot: '' },
    ]))
    const w = await mountPage()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-big')).attributes('src')).toBe('data:image/jpeg;base64,/9j/f301')
    await w.findAll(tid('stylist-frame'))[0].trigger('click')
    await settle()
    expect(w.find(tid('stylist-big')).attributes('src')).toBe('data:image/jpeg;base64,/9j/f300')
    expect(w.findComponent({ name: 'ImageViewer' }).exists()).toBe(false)
    await w.find(tid('stylist-big')).trigger('click')
    await settle()
    expect(opener).toHaveBeenCalledWith(300)
    expect(w.findComponent({ name: 'ImageViewer' }).exists()).toBe(false)
  })
})

const RU_SERIES = () => [
  {
    outfit: 'white dress', outfit_ru: 'Белое платье', hair: 'low bun', hair_ru: 'Пучок', makeup: 'no makeup', makeup_ru: '',
    background: 'sandy beach', background_ru: 'Пляж', place: 'beach', ru_ai: ['outfit'],
    shots: [{ pose: 'Анфас', prompt: 'facing camera', prompt_ru: 'Анфас', landscape: false }, { pose: 'Лёжа', prompt: 'lying on sand', prompt_ru: 'Лёжа', landscape: true }],
  },
  {
    outfit: 'wool coat', outfit_ru: 'Пальто', hair: 'low bun', hair_ru: 'Пучок', makeup: 'no makeup', makeup_ru: 'Без макияжа',
    background: 'sandy beach', background_ru: 'Пляж', place: 'beach',
    shots: [{ pose: 'Анфас', prompt: 'facing camera', prompt_ru: 'Анфас', landscape: false }],
  },
]

describe('Russian plan text', () => {
  beforeEach(() => {
    StylistPlan.mockImplementation(() => Promise.resolve({ series: RU_SERIES(), warnings: [], used_llm: true }))
    StylistShoot.mockImplementation(() => Promise.resolve([7]))
  })

  async function planned() {
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    return w
  }

  async function edit(input, value) {
    await input.setValue(value)
    await input.trigger('change')
    await settle()
  }

  it('helpers: ru edit marks dirty, English edit clears ru, translations replace only dirty English', () => {
    const s = RU_SERIES()[0]
    const ru = setRuField(s, 'outfit', 'Красное платье')
    expect(ru.ru_dirty).toEqual(['outfit'])
    expect(ru.outfit).toBe('white dress')
    const en = setEnField(ru, 'outfit', 'red gown')
    expect(en).toMatchObject({ outfit: 'red gown', outfit_ru: '' })
    expect(en.ru_dirty).toBeUndefined()
    const series = [ru, RU_SERIES()[1]]
    const refs = dirtyRefs(series)
    expect(refs).toEqual([{ series: 0, field: 'outfit', shot: -1, text: 'Красное платье' }])
    expect(dirtyRefs(series, true)).toEqual([])
    const out = applyTranslations(series, refs, ['red dress'])
    expect(out[0]).toMatchObject({ outfit: 'red dress', outfit_ru: 'Красное платье', hair: 'low bun' })
    expect(out[0].ru_dirty).toBeUndefined()
    expect(out[1].outfit).toBe('wool coat')
  })

  it('ru locale shows Russian text, pose names in shots, English when there is no Russian', async () => {
    const w = await planned()
    const card = w.findAll(tid('series-card'))[0]
    expect(card.find(tid('series-outfit')).element.value).toBe('Белое платье')
    expect(card.find(tid('series-outfit')).attributes('data-lang')).toBe('ru')
    expect(card.find(tid('series-makeup')).element.value).toBe('no makeup')
    expect(card.find(tid('series-makeup')).attributes('data-lang')).toBe('en')
    expect(card.findAll(tid('series-shot'))[0].text()).toContain('Анфас')
    expect(card.findAll(tid('shot-prompt'))[1].element.value).toBe('Лёжа')
  })

  it('edits in Russian are translated in one call with only dirty fields before shooting; the shoot gets English', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve(['lying on the side', 'red silk dress']))
    const w = await planned()
    const cards = w.findAll(tid('series-card'))
    await edit(cards[1].find(tid('series-outfit')), 'Красное шёлковое платье')
    await edit(w.findAll(tid('series-card'))[0].findAll(tid('shot-prompt'))[1], 'Лёжа на боку')
    expect(w.findAll(tid('series-dirty'))).toHaveLength(2)
    expect(StylistTranslate).not.toHaveBeenCalled()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistTranslate).toHaveBeenCalledTimes(1)
    expect(StylistTranslate.mock.calls[0][0]).toEqual(['Лёжа на боку', 'Красное шёлковое платье'])
    const req = StylistShoot.mock.calls[0][0]
    expect(req.series[1].outfit).toBe('red silk dress')
    expect(req.series[0].shots[1].prompt).toBe('lying on the side')
    expect(req.series[0].outfit).toBe('white dress')
    expect(req.series[0].shots[0].prompt).toBe('facing camera')
    await w.find(tid('stylist-view-plan')).trigger('click')
    expect(w.findAll(tid('series-dirty'))).toHaveLength(0)
  })

  it('no edits → no translation call', async () => {
    const w = await planned()
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistTranslate).not.toHaveBeenCalled()
    expect(StylistShoot).toHaveBeenCalledTimes(1)
  })

  it('translation error: nothing is shot, the error is shown, the Russian edit stays', async () => {
    StylistTranslate.mockImplementation(() => Promise.reject(new Error('«блондинка»: в переводе есть внешность человека')))
    const w = await planned()
    await edit(w.findAll(tid('series-card'))[0].find(tid('series-hair')), 'блондинка')
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot).not.toHaveBeenCalled()
    expect(w.find(tid('stylist-error')).text()).toContain('съёмка не запущена')
    expect(w.find(tid('stylist-error')).text()).toContain('блондинка')
    expect(w.findAll(tid('series-card'))[0].find(tid('series-hair')).element.value).toBe('блондинка')
    expect(w.findAll(tid('series-dirty'))).toHaveLength(1)
  })

  it('a wrong number of translations is an error, no shoot', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve([]))
    const w = await planned()
    await edit(w.findAll(tid('series-card'))[0].find(tid('series-hair')), 'Коса')
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot).not.toHaveBeenCalled()
    expect(w.find(tid('stylist-error')).text()).toContain('0 из 1')
  })

  it('«Промпт» shows English; editing it goes to the shoot as is, without translation', async () => {
    const w = await planned()
    const card = () => w.findAll(tid('series-card'))[0]
    await card().find(tid('series-prompt-toggle')).trigger('click')
    expect(card().find(tid('series-prompt-toggle')).attributes('aria-pressed')).toBe('true')
    expect(card().find(tid('series-outfit')).element.value).toBe('white dress')
    expect(card().findAll(tid('shot-prompt'))[0].element.value).toBe('facing camera')
    await edit(card().find(tid('series-outfit')), 'black slip dress, thin straps')
    await card().find(tid('series-prompt-toggle')).trigger('click')
    expect(card().find(tid('series-outfit')).element.value).toBe('black slip dress, thin straps')
    expect(card().find(tid('series-outfit')).attributes('data-lang')).toBe('en')
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistTranslate).not.toHaveBeenCalled()
    expect(StylistShoot.mock.calls[0][0].series[0].outfit).toBe('black slip dress, thin straps')
  })

  it('the plan request carries the locale', async () => {
    await planned()
    expect(StylistPlan.mock.calls[0][0].locale).toBe('ru')
    setLocale('en')
    expect(buildPlanRequest({ aspects: defaultAspects(), frames: 4 }, 1).locale).toBe('en')
  })

  it('«перевод ИИ» marks only AI-translated fields; gone after a Russian edit and under «Промпт»', async () => {
    const w = await planned()
    const card = () => w.findAll(tid('series-card'))[0]
    expect(card().find(tid('series-ai-outfit')).text()).toContain(t('stylist.ru_ai'))
    expect(card().find(tid('series-ai-hair')).exists()).toBe(false)
    expect(card().find(tid('series-ai-background')).exists()).toBe(false)
    expect(card().find(tid('series-ai-makeup')).exists()).toBe(false)
    expect(w.findAll(tid('series-card'))[1].find(tid('series-ai-outfit')).exists()).toBe(false)
    await card().find(tid('series-prompt-toggle')).trigger('click')
    expect(card().find(tid('series-ai-outfit')).exists()).toBe(false)
    await card().find(tid('series-prompt-toggle')).trigger('click')
    await edit(card().find(tid('series-outfit')), 'Белое льняное платье')
    expect(card().find(tid('series-ai-outfit')).exists()).toBe(false)
  })

  it('«Снять» is blocked while a plan or a series is being remade, so a plan you have not seen is never shot', async () => {
    const w = await planned()
    let resolvePlan
    StylistPlan.mockImplementation(() => new Promise(r => { resolvePlan = r }))
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-shoot')).element.disabled).toBe(true)
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot).not.toHaveBeenCalled()
    resolvePlan({ series: RU_SERIES(), warnings: [], used_llm: true })
    await settle()
    expect(w.find(tid('stylist-shoot')).element.disabled).toBe(false)

    let resolveRedo
    StylistRegenerateSeries.mockImplementation(() => new Promise(r => { resolveRedo = r }))
    await w.findAll(tid('series-card'))[0].find(tid('series-regenerate')).trigger('click')
    await settle()
    expect(w.find(tid('stylist-shoot')).element.disabled).toBe(true)
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistShoot).not.toHaveBeenCalled()
    resolveRedo({ series: RU_SERIES()[0], warnings: [], used_llm: true })
    await settle()
    expect(w.find(tid('stylist-shoot')).element.disabled).toBe(false)
  })

  it('edits during the translation stay dirty, nothing is shot; plan, redo and delete are blocked meanwhile', async () => {
    let resolve
    StylistTranslate.mockImplementation(() => new Promise(r => { resolve = r }))
    const w = await planned()
    const card = (i) => w.findAll(tid('series-card'))[i]
    await edit(card(0).find(tid('series-outfit')), 'Красное платье')
    await edit(card(1).find(tid('series-hair')), 'Коса')
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistTranslate.mock.calls[0][0]).toEqual(['Красное платье', 'Коса'])
    expect(w.find(tid('stylist-plan')).element.disabled).toBe(true)
    expect(card(0).find(tid('series-regenerate')).element.disabled).toBe(true)
    expect(card(0).find(tid('series-delete')).element.disabled).toBe(true)
    expect(card(0).find(tid('shot-delete')).element.disabled).toBe(true)
    await edit(card(1).find(tid('series-hair')), 'Высокая коса')
    await edit(card(1).find(tid('series-makeup')), 'Яркая помада')
    resolve(['red dress', 'braid'])
    await settle()
    expect(StylistShoot).not.toHaveBeenCalled()
    expect(w.find(tid('stylist-error')).text()).toBe(t('stylist.plan_changed'))
    expect(card(1).find(tid('series-hair')).element.value).toBe('Высокая коса')
    await card(1).find(tid('series-prompt-toggle')).trigger('click')
    expect(card(1).find(tid('series-hair')).element.value).toBe('low bun')
    expect(card(1).find(tid('series-makeup')).element.value).toBe('no makeup')
    await card(1).find(tid('series-prompt-toggle')).trigger('click')
    StylistTranslate.mockImplementation(() => Promise.resolve(['high braid', 'bold lipstick']))
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistTranslate.mock.calls[1][0]).toEqual(['Высокая коса', 'Яркая помада'])
    const req = StylistShoot.mock.calls[0][0]
    expect([req.series[0].outfit, req.series[1].hair, req.series[1].makeup]).toEqual(['red dress', 'high braid', 'bold lipstick'])
    expect(req.series).toHaveLength(2)
  })

  it('a garment photo skips the outfit translation and keeps it dirty for later', () => {
    const s = setRuField(setRuField(RU_SERIES()[0], 'outfit', 'Красное платье'), 'hair', 'Коса')
    const refs = dirtyRefs([s], true)
    expect(refs.map(r => r.field)).toEqual(['hair'])
    const out = applyTranslations([s], refs, ['braid'])
    expect(out[0].ru_dirty).toEqual(['outfit'])
    expect(out[0].outfit).toBe('white dress')
    expect(dirtyRefs(out).map(r => r.text)).toEqual(['Красное платье'])
  })

  it('editing a shot text drops its framing so the lead cannot contradict it', async () => {
    const s = { ...RU_SERIES()[0], shots: [{ pose: 'Анфас', prompt: 'facing camera', prompt_ru: 'Анфас', framing: 'close' }] }
    expect(setShotRu(s, 0, 'Сидит, в полный рост').shots[0].framing).toBeUndefined()
    expect(setShotEn(s, 0, 'sitting, full length').shots[0].framing).toBeUndefined()
    useStylistPlan().form.frames = 2
    StylistPlan.mockImplementation(() => Promise.resolve({ series: [s, RU_SERIES()[1]], warnings: [], used_llm: true }))
    StylistTranslate.mockImplementation(() => Promise.resolve(['sitting, full length']))
    const w = await planned()
    await edit(w.findAll(tid('series-card'))[0].findAll(tid('shot-prompt'))[0], 'Сидит, в полный рост')
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    const shot = StylistShoot.mock.calls[0][0].series[0].shots[0]
    expect(shot.prompt).toBe('sitting, full length')
    expect(shot.framing).toBeUndefined()
    expect(StylistShoot.mock.calls[0][0].series[1].shots[0].prompt).toBe('facing camera')
  })

  it('en locale: English shown and edited, no «Промпт» toggle, no translation', async () => {
    setLocale('en')
    const w = await planned()
    const card = w.findAll(tid('series-card'))[0]
    expect(card.find(tid('series-prompt-toggle')).exists()).toBe(false)
    expect(card.find(tid('series-ai-outfit')).exists()).toBe(false)
    expect(StylistPlan.mock.calls[0][0].locale).toBe('en')
    expect(card.find(tid('series-outfit')).element.value).toBe('white dress')
    expect(card.findAll(tid('shot-prompt'))[0].element.value).toBe('facing camera')
    await edit(card.find(tid('series-outfit')), 'red gown')
    await w.find(tid('stylist-shoot')).trigger('click')
    await settle()
    expect(StylistTranslate).not.toHaveBeenCalled()
    expect(StylistShoot.mock.calls[0][0].series[0].outfit).toBe('red gown')
  })
})
