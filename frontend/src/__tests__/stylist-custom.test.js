import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, StylistRegenerateSeries, StylistAppearance, ListPeople, FaceSwapStatus, GetQueue, StylistFrames, StylistTranslate,
} from '../wailsjs/go/main/App.js'
import { setLocale, t, ru, en } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import StylistPage from '../components/StylistPage.vue'
import AspectTab from '../components/stylist/AspectTab.vue'
import { buildPlanRequest, defaultAspects, resetStylistPlan, useStylistPlan } from '../composables/useStylistPlan.js'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

const BASE = () => ({
  version: 1,
  themes: [{ id: 'beach', name: 'Пляж' }],
  outfit: [{ id: 'o1', theme: 'beach', name: 'Платье', prompt: 'white dress' }],
  hair: [{ id: 'h1', name: 'Пучок', prompt: 'low bun' }],
  makeup: [{ id: 'm1', name: 'Без', prompt: 'no makeup' }],
  background: [{ id: 'b1', theme: 'beach', name: 'Пляж', prompt: 'sandy beach', place: 'beach' }],
  pose: [{ id: 'p1', name: 'Анфас', prompt: 'facing camera', framing: 'close', places: ['any'], orientation: 'portrait' }],
})

const SERIES = () => [
  { outfit: 'white dress', hair: 'low bun', makeup: 'no makeup', background: 'sandy beach', place: 'beach', shots: [
    { pose: 'Анфас', prompt: 'facing camera', landscape: false },
    { pose: 'Анфас', prompt: 'facing camera', landscape: false },
  ] },
  { outfit: 'white dress', hair: 'low bun', makeup: 'no makeup', background: 'sandy beach', place: 'beach', shots: [
    { pose: 'Анфас', prompt: 'facing camera', landscape: false },
  ] },
]

const ANNA = { name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman', identity: true }

const CUSTOM_ASPECTS = ['outfit', 'hair', 'makeup', 'background']

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  useStylistPlan().form.frames = 3
  StylistPresets.mockImplementation(() => Promise.resolve(BASE()))
  ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: SERIES(), warnings: [], used_llm: true }))
  StylistRegenerateSeries.mockImplementation(() => Promise.resolve({ series: SERIES()[1], warnings: [], used_llm: true }))
  StylistTranslate.mockImplementation((texts) => Promise.resolve(texts.map(x => 'en:' + x)))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  StylistFrames.mockImplementation(() => Promise.resolve([]))
})

const mountTab = (aspect, choice = { mode: 'auto', ids: [], text: '' }) =>
  mount(AspectTab, { props: { aspect, choice, items: BASE()[aspect], themes: BASE().themes } })

describe('AspectTab mode buttons', () => {
  for (const aspect of CUSTOM_ASPECTS) {
    it(`${aspect}: four highlighted buttons, exactly one pressed`, async () => {
      const w = mountTab(aspect)
      expect(w.find('input[type=radio]').exists()).toBe(false)
      for (const m of ['auto', 'keep', 'pick', 'custom']) {
        const b = w.find(tid('mode-' + m))
        expect(b.exists()).toBe(true)
        expect(b.element.tagName).toBe('BUTTON')
        expect(b.text()).toBe(t('stylist.mode_' + m))
      }
      for (const mode of ['auto', 'keep', 'pick', 'custom']) {
        await w.setProps({ choice: { mode, ids: [], text: '' } })
        const pressed = w.findAll('[aria-pressed="true"]')
        expect(pressed).toHaveLength(1)
        expect(pressed[0].attributes('data-testid')).toBe('mode-' + mode)
        expect(pressed[0].classes()).toContain('btn-primary')
        for (const other of ['auto', 'keep', 'pick', 'custom'].filter(x => x !== mode)) {
          expect(w.find(tid('mode-' + other)).classes()).not.toContain('btn-primary')
          expect(w.find(tid('mode-' + other)).attributes('aria-pressed')).not.toBe('true')
        }
      }
    })
  }

  it('pose: three modes, no «Своё»', () => {
    const w = mountTab('pose')
    expect(w.find(tid('mode-auto')).exists()).toBe(true)
    expect(w.find(tid('mode-keep')).exists()).toBe(true)
    expect(w.find(tid('mode-pick')).exists()).toBe(true)
    expect(w.find(tid('mode-custom')).exists()).toBe(false)
    expect(w.find(tid('custom-text')).exists()).toBe(false)
  })

  it('click on a mode button emits the new mode', async () => {
    const w = mountTab('hair', { mode: 'pick', ids: ['h1'], text: '' })
    await w.find(tid('mode-custom')).trigger('click')
    expect(w.emitted('update:choice').at(-1)[0]).toMatchObject({ mode: 'custom' })
    await w.find(tid('mode-keep')).trigger('click')
    expect(w.emitted('update:choice').at(-1)[0]).toMatchObject({ mode: 'keep' })
  })

  it('«Своё» shows the text field (500 max) and no preset chips; other modes hide it', async () => {
    const w = mountTab('outfit', { mode: 'custom', ids: ['o1'], text: 'red dress' })
    const field = w.find(tid('custom-text'))
    expect(field.exists()).toBe(true)
    expect(field.attributes('maxlength')).toBe('500')
    expect(field.element.value).toBe('red dress')
    expect(field.attributes('placeholder')).toBe(t('stylist.custom_placeholder_outfit'))
    expect(w.find(tid('pick-o1')).exists()).toBe(false)
    expect(w.text()).toContain(t('stylist.mode_hint_custom'))
    for (const mode of ['auto', 'keep', 'pick']) {
      await w.setProps({ choice: { mode, ids: [], text: 'red dress' } })
      expect(w.find(tid('custom-text')).exists()).toBe(false)
    }
  })

  it('typing in «Своё» emits custom with the text', async () => {
    const w = mountTab('makeup', { mode: 'custom', ids: [], text: '' })
    const field = w.find(tid('custom-text'))
    await field.setValue('gold glitter on eyelids')
    await field.trigger('change')
    expect(w.emitted('update:choice').at(-1)[0]).toMatchObject({ mode: 'custom', text: 'gold glitter on eyelids' })
  })

  it('placeholder per aspect', () => {
    for (const aspect of CUSTOM_ASPECTS) {
      const w = mountTab(aspect, { mode: 'custom', ids: [], text: '' })
      expect(w.find(tid('custom-text')).attributes('placeholder')).toBe(t('stylist.custom_placeholder_' + aspect))
      w.unmount()
    }
  })
})

describe('custom in the plan request', () => {
  it('defaultAspects carries an empty text', () => {
    const a = defaultAspects()
    for (const k of ['outfit', 'hair', 'makeup', 'background', 'pose']) {
      expect(a[k]).toEqual({ mode: 'auto', ids: [], text: '' })
    }
  })

  it('custom keeps the text and drops ids; other modes send empty text; no text_ru', () => {
    const aspects = defaultAspects()
    aspects.outfit = { mode: 'custom', ids: ['o1'], text: 'red silk dress' }
    aspects.hair = { mode: 'pick', ids: ['h1'], text: 'leftover' }
    aspects.makeup = { mode: 'keep', ids: [], text: 'leftover' }
    const req = buildPlanRequest({ aspects, frames: 4, garment: '' }, 1)
    expect(req.aspects.outfit).toEqual({ mode: 'custom', ids: [], text: 'red silk dress' })
    expect(req.aspects.hair).toEqual({ mode: 'pick', ids: ['h1'], text: '' })
    expect(req.aspects.makeup).toEqual({ mode: 'keep', ids: [], text: '' })
    expect(req.aspects.background).toEqual({ mode: 'auto', ids: [], text: '' })
  })

  it('garment photo: outfit stays «from the photo», custom text is dropped', () => {
    const aspects = defaultAspects()
    aspects.outfit = { mode: 'custom', ids: [], text: 'red silk dress' }
    const req = buildPlanRequest({ aspects, frames: 4, garment: 'abc' }, 1)
    expect(req.aspects.outfit.mode).toBe('keep')
    expect(req.aspects.outfit.text || '').toBe('')
  })
})

describe('StylistPage «Своё»', () => {
  async function mountPage() {
    const w = mount(StylistPage)
    await settle()
    return w
  }

  async function setCustom(w, aspect, text) {
    await w.find(tid('tab-' + aspect)).trigger('click')
    await w.find(tid('mode-custom')).trigger('click')
    const field = w.find(tid('custom-text'))
    await field.setValue(text)
    await field.trigger('change')
  }

  it('the mode buttons work on the page and «Своё» shows the field', async () => {
    const w = await mountPage()
    await w.find(tid('tab-hair')).trigger('click')
    expect(w.find(tid('custom-text')).exists()).toBe(false)
    await w.find(tid('mode-custom')).trigger('click')
    expect(w.find(tid('mode-custom')).attributes('aria-pressed')).toBe('true')
    expect(w.find(tid('custom-text')).exists()).toBe(true)
    await w.find(tid('tab-pose')).trigger('click')
    expect(w.find(tid('mode-custom')).exists()).toBe(false)
  })

  it('RU: Cyrillic texts are translated in one call before the plan; text = translation, text_ru = original', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve(['red silk dress', 'high braid']))
    const w = await mountPage()
    await setCustom(w, 'outfit', 'Красное шёлковое платье')
    await setCustom(w, 'hair', 'Высокая коса')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistTranslate).toHaveBeenCalledTimes(1)
    expect([...StylistTranslate.mock.calls[0][0]].sort()).toEqual(['Высокая коса', 'Красное шёлковое платье'].sort())
    expect(StylistPlan).toHaveBeenCalledTimes(1)
    expect(StylistTranslate.mock.invocationCallOrder[0]).toBeLessThan(StylistPlan.mock.invocationCallOrder[0])
    const req = StylistPlan.mock.calls[0][0]
    expect(req.aspects.outfit).toMatchObject({ mode: 'custom', text: 'red silk dress', text_ru: 'Красное шёлковое платье' })
    expect(req.aspects.hair).toMatchObject({ mode: 'custom', text: 'high braid', text_ru: 'Высокая коса' })
  })

  it('RU: Latin text is not translated, text_ru is empty', async () => {
    const w = await mountPage()
    await setCustom(w, 'background', 'rooftop at sunset')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistTranslate).not.toHaveBeenCalled()
    const bg = StylistPlan.mock.calls[0][0].aspects.background
    expect(bg).toMatchObject({ mode: 'custom', text: 'rooftop at sunset' })
    expect(bg.text_ru || '').toBe('')
  })

  it('RU: only the Cyrillic texts go to the translator', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve(['smoky eyes']))
    const w = await mountPage()
    await setCustom(w, 'outfit', 'black jeans')
    await setCustom(w, 'makeup', 'Дымчатые глаза')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistTranslate.mock.calls[0][0]).toEqual(['Дымчатые глаза'])
    const req = StylistPlan.mock.calls[0][0]
    expect(req.aspects.outfit.text).toBe('black jeans')
    expect(req.aspects.makeup).toMatchObject({ text: 'smoky eyes', text_ru: 'Дымчатые глаза' })
  })

  it('RU: translation error → no plan, the error is shown', async () => {
    StylistTranslate.mockImplementation(() => Promise.reject(new Error('нет связи с ИИ')))
    const w = await mountPage()
    await setCustom(w, 'outfit', 'Красное платье')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistPlan).not.toHaveBeenCalled()
    const err = w.find(tid('stylist-error'))
    expect(err.exists()).toBe(true)
    expect(err.text()).toBe(t('stylist.custom_translate_failed', { error: 'нет связи с ИИ' }))
    expect(w.findAll(tid('series-card'))).toHaveLength(0)
  })

  it('EN: no translation, the text goes as typed', async () => {
    setLocale('en')
    const w = await mountPage()
    await setCustom(w, 'outfit', 'Красное платье')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistTranslate).not.toHaveBeenCalled()
    const o = StylistPlan.mock.calls[0][0].aspects.outfit
    expect(o).toMatchObject({ mode: 'custom', text: 'Красное платье' })
    expect(o.text_ru || '').toBe('')
  })

  it('RU: regenerate translates first and sends the translation', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve(['red silk dress']))
    const w = await mountPage()
    await setCustom(w, 'outfit', 'Красное шёлковое платье')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    StylistTranslate.mockClear()
    await w.findAll(tid('series-regenerate'))[1].trigger('click')
    await settle()
    expect(StylistTranslate).toHaveBeenCalledWith(['Красное шёлковое платье'])
    expect(StylistRegenerateSeries).toHaveBeenCalledTimes(1)
    expect(StylistTranslate.mock.invocationCallOrder[0]).toBeLessThan(StylistRegenerateSeries.mock.invocationCallOrder[0])
    expect(StylistRegenerateSeries.mock.calls[0][0].aspects.outfit).toMatchObject({ mode: 'custom', text: 'red silk dress', text_ru: 'Красное шёлковое платье' })
  })

  it('RU: translation error on regenerate → series not remade, error shown', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve(['red silk dress']))
    const w = await mountPage()
    await setCustom(w, 'outfit', 'Красное шёлковое платье')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    StylistTranslate.mockImplementation(() => Promise.reject(new Error('таймаут')))
    await w.findAll(tid('series-regenerate'))[0].trigger('click')
    await settle()
    expect(StylistRegenerateSeries).not.toHaveBeenCalled()
    expect(w.find(tid('stylist-error')).text()).toBe(t('stylist.custom_translate_failed', { error: 'таймаут' }))
  })
})

describe('labels', () => {
  async function planned() {
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    return w
  }

  it('shoot button says «Сгенерировать (N)»', async () => {
    expect(t('stylist.shoot', { n: 3 })).toBe('Сгенерировать (3)')
    const w = await planned()
    const text = w.find(tid('stylist-shoot')).text()
    expect(text).toBe('Сгенерировать (3)')
    expect(text).not.toContain('Снять')
  })

  it('no stylist text mentions «Снять» any more', () => {
    const left = Object.entries(ru).filter(([k, v]) => k.startsWith('stylist.') && /Снять/.test(v)).map(([k]) => k)
    expect(left).toEqual([])
  })

  it('wish is «Пожелание для ИИ» with a hint under the field', async () => {
    expect(ru['stylist.wish']).toBe('Пожелание для ИИ')
    expect(en['stylist.wish']).toBe('Wish for the AI')
    const w = mount(StylistPage)
    await settle()
    expect(w.text()).toContain('Пожелание для ИИ')
    expect(w.text()).not.toContain('Что нужно')
    const help = w.find(tid('stylist-wish-help'))
    expect(help.exists()).toBe(true)
    expect(help.text()).toBe(t('stylist.wish_help'))
    expect(ru['stylist.wish_help']).toBeTruthy()
    expect(en['stylist.wish_help']).toBeTruthy()
  })

  it('custom mode texts exist in both languages', () => {
    expect(ru['stylist.mode_custom']).toBe('Своё')
    const keys = ['stylist.mode_custom', 'stylist.mode_hint_custom', 'stylist.custom_translate_failed',
      ...CUSTOM_ASPECTS.map(a => 'stylist.custom_placeholder_' + a)]
    for (const k of keys) {
      expect(ru[k], k).toBeTruthy()
      expect(en[k], k).toBeTruthy()
    }
    expect(ru['stylist.custom_translate_failed']).toContain('{error}')
  })
})
