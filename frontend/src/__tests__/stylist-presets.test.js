import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, StylistAppearance, ListPeople, FaceSwapStatus, GetQueue, StylistFrames, StylistTranslate,
  StylistSavePreset, StylistRenamePreset, StylistDeletePreset,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import StylistPage from '../components/StylistPage.vue'
import AspectTab from '../components/stylist/AspectTab.vue'
import SeriesCard from '../components/stylist/SeriesCard.vue'
import SavePresetForm from '../components/stylist/SavePresetForm.vue'
import {
  presetFromField, presetFromShot, dropChoiceId, resetStylistPlan, useStylistPlan, setRuField, setEnField, setShotRu,
} from '../composables/useStylistPlan.js'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

const BASE = () => ({
  version: 1,
  themes: [{ id: 'beach', name: 'Пляж' }],
  outfit: [{ id: 'o1', theme: 'beach', name: 'Платье', prompt: 'white dress' }],
  hair: [{ id: 'h1', name: 'Пучок', prompt: 'low bun' }],
  makeup: [{ id: 'm1', name: 'Без', prompt: 'no makeup' }],
  background: [
    { id: 'b1', theme: 'beach', name: 'Пляж', prompt: 'sandy beach', place: 'beach' },
    { id: 'b2', name: 'Студия', prompt: 'grey backdrop', place: 'studio' },
  ],
  pose: [{ id: 'p1', name: 'Анфас', prompt: 'facing camera', framing: 'close', places: ['any'], orientation: 'portrait' }],
})

const USER_OUTFIT = { id: 'u3', user: true, name: 'Моё платье', prompt: 'red dress', prompt_ru: 'Красное платье' }

const withUserOutfit = () => {
  const b = BASE()
  b.outfit = [...b.outfit, { ...USER_OUTFIT }]
  return b
}

const SERIES = () => ({
  outfit: 'white dress', outfit_ru: 'Белое платье', hair: 'low bun', hair_ru: 'Пучок', makeup: 'no makeup', makeup_ru: '',
  background: 'sandy beach', background_ru: 'Пляж', place: 'beach',
  shots: [
    { pose: 'Анфас', prompt: 'facing camera', prompt_ru: 'Анфас', landscape: false, framing: 'close' },
    { pose: 'Лёжа', prompt: 'lying on sand', prompt_ru: 'Лёжа', landscape: true, framing: 'full' },
  ],
})

const ANNA = { name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman', identity: true }

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  StylistPresets.mockImplementation(() => Promise.resolve(BASE()))
  ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: [SERIES()], warnings: [], used_llm: true }))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  StylistFrames.mockImplementation(() => Promise.resolve([]))
  StylistSavePreset.mockImplementation(() => Promise.resolve('u1'))
  StylistRenamePreset.mockImplementation(() => Promise.resolve())
  StylistDeletePreset.mockImplementation(() => Promise.resolve())
})

describe('preset helpers', () => {
  it('presetFromField takes the English prompt and, in ru, the Russian text', () => {
    const s = SERIES()
    expect(presetFromField(s, 'outfit', true)).toEqual({ aspect: 'outfit', prompt: 'white dress', prompt_ru: 'Белое платье', place: '', dirty: false })
    expect(presetFromField(s, 'outfit', false)).toEqual({ aspect: 'outfit', prompt: 'white dress', prompt_ru: '', place: '', dirty: false })
    expect(presetFromField(s, 'makeup', true)).toEqual({ aspect: 'makeup', prompt: 'no makeup', prompt_ru: '', place: '', dirty: false })
    expect(presetFromField(s, 'background', true)).toEqual({ aspect: 'background', prompt: 'sandy beach', prompt_ru: 'Пляж', place: 'beach', dirty: false })
    expect(presetFromField({ ...s, place: '' }, 'background', false).place).toBe('')
  })

  it('presetFromField marks a Russian edit not yet translated as dirty, only in ru', () => {
    const s = setRuField(SERIES(), 'hair', 'Коса')
    expect(presetFromField(s, 'hair', true)).toMatchObject({ prompt: 'low bun', prompt_ru: 'Коса', dirty: true })
    expect(presetFromField(s, 'hair', false).dirty).toBeFalsy()
    expect(presetFromField(s, 'outfit', true).dirty).toBe(false)
  })

  it('presetFromShot takes prompt, framing, orientation and the series place', () => {
    const s = SERIES()
    expect(presetFromShot(s, 1, true)).toEqual({
      aspect: 'pose', prompt: 'lying on sand', prompt_ru: 'Лёжа', framing: 'full', orientation: 'landscape', places: ['beach'], dirty: false,
    })
    expect(presetFromShot(s, 0, false)).toEqual({
      aspect: 'pose', prompt: 'facing camera', prompt_ru: '', framing: 'close', orientation: 'portrait', places: ['beach'], dirty: false,
    })
    expect(presetFromShot({ ...s, place: '' }, 0, true).places).toEqual(['any'])
    const noFraming = { ...s, shots: [{ prompt: 'x', landscape: false }] }
    expect(presetFromShot(noFraming, 0, true).framing).toBe('')
  })

  it('presetFromShot marks an untranslated Russian edit of the shot as dirty', () => {
    const s = setShotRu(SERIES(), 1, 'Лёжа на боку')
    expect(presetFromShot(s, 1, true)).toMatchObject({ prompt: 'lying on sand', prompt_ru: 'Лёжа на боку', dirty: true })
    expect(presetFromShot(s, 1, false).dirty).toBe(false)
    expect(presetFromShot(s, 0, true).dirty).toBe(false)
  })

  it('dropChoiceId removes the id from every aspect without mutating input', () => {
    const aspects = {
      outfit: { mode: 'pick', ids: ['o1', 'u3'] },
      hair: { mode: 'pick', ids: ['u3'] },
      makeup: { mode: 'auto', ids: [] },
      background: { mode: 'keep', ids: ['b1'] },
      pose: { mode: 'pick', ids: ['p1', 'u30'] },
    }
    const copy = structuredClone(aspects)
    const out = dropChoiceId(aspects, 'u3')
    expect(out.outfit).toEqual({ mode: 'pick', ids: ['o1'] })
    expect(out.hair).toEqual({ mode: 'pick', ids: [] })
    expect(out.makeup).toEqual({ mode: 'auto', ids: [] })
    expect(out.background).toEqual({ mode: 'keep', ids: ['b1'] })
    expect(out.pose).toEqual({ mode: 'pick', ids: ['p1', 'u30'] })
    expect(aspects).toEqual(copy)
    expect(dropChoiceId(aspects, 'u999')).toEqual(copy)
  })
})

describe('savePreset', () => {
  const payload = (over = {}) => ({
    aspect: 'hair', name: 'Коса', prompt: 'low bun', prompt_ru: 'Коса', place: '', framing: '', places: [], orientation: '', dirty: false, ...over,
  })

  it('saves without translating when not dirty and returns the id', async () => {
    StylistSavePreset.mockImplementation(() => Promise.resolve('u5'))
    const id = await useStylistPlan().savePreset(payload())
    expect(id).toBe('u5')
    expect(StylistTranslate).not.toHaveBeenCalled()
    expect(StylistSavePreset).toHaveBeenCalledTimes(1)
    const sent = StylistSavePreset.mock.calls[0][0]
    expect(sent).toMatchObject({ aspect: 'hair', name: 'Коса', prompt: 'low bun', prompt_ru: 'Коса' })
    expect(sent).not.toHaveProperty('dirty')
  })

  it('translates a dirty Russian text first and saves the translation as the prompt', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve(['braid']))
    const id = await useStylistPlan().savePreset(payload({ dirty: true }))
    expect(id).toBe('u1')
    expect(StylistTranslate).toHaveBeenCalledWith(['Коса'])
    expect(StylistTranslate.mock.invocationCallOrder[0]).toBeLessThan(StylistSavePreset.mock.invocationCallOrder[0])
    const sent = StylistSavePreset.mock.calls[0][0]
    expect(sent).toMatchObject({ prompt: 'braid', prompt_ru: 'Коса' })
    expect(sent).not.toHaveProperty('dirty')
  })

  it('dirty but empty Russian text: no translation', async () => {
    await useStylistPlan().savePreset(payload({ dirty: true, prompt_ru: '' }))
    expect(StylistTranslate).not.toHaveBeenCalled()
    expect(StylistSavePreset.mock.calls[0][0].prompt).toBe('low bun')
  })

  it('a save error returns an empty id and sets the error', async () => {
    StylistSavePreset.mockImplementation(() => Promise.reject(new Error('название до 60 символов')))
    const plan = useStylistPlan()
    const id = await plan.savePreset(payload())
    expect(id).toBe('')
    expect(plan.error.value).toContain('название до 60 символов')
  })

  it('a translation error: nothing saved', async () => {
    StylistTranslate.mockImplementation(() => Promise.reject(new Error('нет связи')))
    const plan = useStylistPlan()
    const id = await plan.savePreset(payload({ dirty: true }))
    expect(id).toBe('')
    expect(StylistSavePreset).not.toHaveBeenCalled()
    expect(plan.error.value).not.toBe('')
  })
})

describe('AspectTab «Мои»', () => {
  const items = [...BASE().outfit, { ...USER_OUTFIT }]
  const mountTab = (choice = { mode: 'pick', ids: [] }, list = items) =>
    mount(AspectTab, { props: { aspect: 'outfit', choice, items: list, themes: BASE().themes } })

  it('user presets are a separate «Мои» group, only in «Выбрать»', async () => {
    const w = mountTab({ mode: 'auto', ids: [] })
    expect(w.find(tid('user-group')).exists()).toBe(false)
    await w.setProps({ choice: { mode: 'keep', ids: [] } })
    expect(w.find(tid('user-group')).exists()).toBe(false)
    await w.setProps({ choice: { mode: 'pick', ids: [] } })
    const group = w.find(tid('user-group'))
    expect(group.exists()).toBe(true)
    expect(group.text()).toContain(t('stylist.user_group'))
    expect(t('stylist.user_group')).toBe('Мои')
    expect(group.find(tid('pick-u3')).exists()).toBe(true)
    expect(group.find(tid('pick-o1')).exists()).toBe(false)
    expect(w.find(tid('pick-o1')).exists()).toBe(true)
  })

  it('no user presets → no «Мои» group; built-ins have no rename/delete', () => {
    const w = mountTab({ mode: 'pick', ids: [] }, BASE().outfit)
    expect(w.find(tid('user-group')).exists()).toBe(false)
    expect(w.find(tid('user-rename-o1')).exists()).toBe(false)
    expect(w.find(tid('user-delete-o1')).exists()).toBe(false)
  })

  it('a user preset is picked like a built-in one', async () => {
    const w = mountTab({ mode: 'pick', ids: ['o1'] })
    await w.find(tid('pick-u3')).trigger('click')
    expect(w.emitted('update:choice').at(-1)[0]).toEqual({ mode: 'pick', ids: ['o1', 'u3'] })
    await w.setProps({ choice: { mode: 'pick', ids: ['o1', 'u3'] } })
    expect(w.find(tid('pick-u3')).classes()).toContain('active')
  })

  it('rename: the name field replaces the chip, Enter emits the new name', async () => {
    const w = mountTab()
    await w.find(tid('user-rename-u3')).trigger('click')
    const input = w.find(tid('user-name-u3'))
    expect(input.exists()).toBe(true)
    expect(input.element.value).toBe('Моё платье')
    expect(w.find(tid('pick-u3')).exists()).toBe(false)
    await input.setValue('Красное')
    await input.trigger('keydown', { key: 'Enter' })
    expect(w.emitted('rename')).toEqual([['u3', 'Красное']])
  })

  it('rename: Esc cancels without emitting', async () => {
    const w = mountTab()
    await w.find(tid('user-rename-u3')).trigger('click')
    const input = w.find(tid('user-name-u3'))
    await input.setValue('Другое')
    await input.trigger('keydown', { key: 'Escape' })
    expect(w.find(tid('user-name-u3')).exists()).toBe(false)
    expect(w.find(tid('pick-u3')).exists()).toBe(true)
    expect(w.find(tid('pick-u3')).text()).toBe('Моё платье')
    expect(w.emitted('rename')).toBeUndefined()
  })

  it('delete needs a second click; the first one changes the button text', async () => {
    const w = mountTab()
    const btn = () => w.find(tid('user-delete-u3'))
    const before = btn().text()
    await btn().trigger('click')
    expect(w.emitted('delete')).toBeUndefined()
    expect(btn().text()).not.toBe(before)
    await btn().trigger('click')
    expect(w.emitted('delete')).toEqual([['u3']])
  })
})

describe('SavePresetForm', () => {
  const PLACES = ['beach', 'studio']

  it('name is required and limited to 60; save emits the preset with the name', async () => {
    const preset = presetFromField(SERIES(), 'hair', true)
    const w = mount(SavePresetForm, { props: { preset, places: PLACES } })
    const name = w.find(tid('preset-name'))
    expect(name.attributes('maxlength')).toBe('60')
    expect(w.find(tid('preset-place')).exists()).toBe(false)
    expect(w.find(tid('preset-framing')).exists()).toBe(false)
    expect(w.find(tid('preset-save')).element.disabled).toBe(true)
    await name.setValue('Мой пучок')
    expect(w.find(tid('preset-save')).element.disabled).toBe(false)
    await w.find(tid('preset-save')).trigger('click')
    const out = w.emitted('save')[0][0]
    expect(out).toMatchObject({ aspect: 'hair', name: 'Мой пучок', prompt: 'low bun', prompt_ru: 'Пучок' })
  })

  it('cancel does not save', async () => {
    const w = mount(SavePresetForm, { props: { preset: presetFromField(SERIES(), 'hair', true), places: PLACES } })
    await w.find(tid('preset-name')).setValue('x')
    await w.find(tid('preset-cancel')).trigger('click')
    expect(w.emitted('save')).toBeUndefined()
  })

  it('background: place select of background places, defaults to the series place, can be changed', async () => {
    const w = mount(SavePresetForm, { props: { preset: presetFromField(SERIES(), 'background', true), places: PLACES } })
    const sel = w.find(tid('preset-place'))
    expect(sel.exists()).toBe(true)
    const opts = sel.findAll('option').map(o => o.element.value).filter(Boolean)
    expect(opts).toEqual(PLACES)
    expect(opts).not.toContain('any')
    expect(sel.element.value).toBe('beach')
    await sel.setValue('studio')
    await w.find(tid('preset-name')).setValue('Серый фон')
    await w.find(tid('preset-save')).trigger('click')
    expect(w.emitted('save')[0][0]).toMatchObject({ aspect: 'background', name: 'Серый фон', place: 'studio', prompt: 'sandy beach' })
  })

  it('pose: framing defaults to the shot, places are chips with «any», no places → save disabled', async () => {
    const w = mount(SavePresetForm, { props: { preset: presetFromShot(SERIES(), 1, true), places: PLACES } })
    const framing = w.find(tid('preset-framing'))
    expect(framing.element.value).toBe('full')
    expect(framing.findAll('option').map(o => o.element.value)).toEqual(expect.arrayContaining(['close', 'waist', 'knee', 'full']))
    for (const p of [...PLACES, 'any']) expect(w.find(tid('preset-place-' + p)).exists()).toBe(true)
    await w.find(tid('preset-name')).setValue('Лёжа')
    expect(w.find(tid('preset-save')).element.disabled).toBe(false)
    await w.find(tid('preset-place-beach')).trigger('click')
    expect(w.find(tid('preset-save')).element.disabled).toBe(true)
    await w.find(tid('preset-place-studio')).trigger('click')
    await w.find(tid('preset-place-any')).trigger('click')
    expect(w.find(tid('preset-save')).element.disabled).toBe(false)
    await framing.setValue('knee')
    await w.find(tid('preset-save')).trigger('click')
    const out = w.emitted('save')[0][0]
    expect(out).toMatchObject({ aspect: 'pose', name: 'Лёжа', framing: 'knee', orientation: 'landscape', prompt: 'lying on sand' })
    expect([...out.places].sort()).toEqual(['any', 'studio'])
  })
})

describe('SeriesCard save buttons', () => {
  const mountCard = (props = {}) => mount(SeriesCard, { props: { series: SERIES(), index: 0, ...props } })

  it('each of the four fields and each shot has «Сохранить в базу»', () => {
    const w = mountCard()
    for (const f of ['outfit', 'hair', 'makeup', 'background']) expect(w.find(tid('save-preset-' + f)).exists()).toBe(true)
    expect(w.findAll(tid('shot-save'))).toHaveLength(2)
  })

  it('no outfit save button when the outfit comes from a garment photo', () => {
    const w = mountCard({ outfitLocked: true })
    expect(w.find(tid('save-preset-outfit')).exists()).toBe(false)
    expect(w.find(tid('save-preset-hair')).exists()).toBe(true)
  })

  it('click opens the form; save emits save-preset and closes it', async () => {
    const w = mountCard()
    expect(w.find(tid('preset-name')).exists()).toBe(false)
    await w.find(tid('save-preset-makeup')).trigger('click')
    expect(w.find(tid('preset-name')).exists()).toBe(true)
    await w.find(tid('preset-name')).setValue('Без макияжа')
    await w.find(tid('preset-save')).trigger('click')
    expect(w.emitted('save-preset')[0][0]).toMatchObject({ aspect: 'makeup', name: 'Без макияжа', prompt: 'no makeup' })
    expect(w.find(tid('preset-name')).exists()).toBe(false)
  })

  it('shot save opens a pose form for that shot', async () => {
    const w = mountCard()
    await w.findAll(tid('shot-save'))[1].trigger('click')
    expect(w.find(tid('preset-framing')).exists()).toBe(true)
    await w.find(tid('preset-name')).setValue('Лёжа')
    await w.find(tid('preset-save')).trigger('click')
    expect(w.emitted('save-preset')[0][0]).toMatchObject({ aspect: 'pose', prompt: 'lying on sand', orientation: 'landscape', framing: 'full', places: ['beach'] })
  })

  it('RU: with the prompt view on, a Russian edit is still saved and marked for translation', async () => {
    const w = mountCard({ series: setRuField(SERIES(), 'hair', 'Коса') })
    await w.find(tid('series-prompt-toggle')).trigger('click')
    await w.find(tid('save-preset-hair')).trigger('click')
    await w.find(tid('preset-name')).setValue('Коса')
    await w.find(tid('preset-save')).trigger('click')
    expect(w.emitted('save-preset')[0][0]).toMatchObject({ aspect: 'hair', prompt: 'low bun', prompt_ru: 'Коса', dirty: true })
  })

  it('the form saves the field text as it is at confirm time, not when opened', async () => {
    const w = mountCard()
    await w.find(tid('save-preset-hair')).trigger('click')
    await w.setProps({ series: setEnField(SERIES(), 'hair', 'french braid') })
    await w.find(tid('preset-name')).setValue('Коса')
    await w.find(tid('preset-save')).trigger('click')
    expect(w.emitted('save-preset')[0][0]).toMatchObject({ aspect: 'hair', prompt: 'french braid', prompt_ru: '' })
  })

  it('a new epoch (series/shot removed, regenerated, new plan) closes an open form', async () => {
    const w = mountCard()
    await w.findAll(tid('shot-save'))[1].trigger('click')
    expect(w.find(tid('preset-framing')).exists()).toBe(true)
    await w.setProps({ epoch: 1 })
    expect(w.find(tid('preset-framing')).exists()).toBe(false)
  })

  it('cancel closes the form without emitting', async () => {
    const w = mountCard()
    await w.find(tid('save-preset-hair')).trigger('click')
    await w.find(tid('preset-cancel')).trigger('click')
    expect(w.find(tid('preset-name')).exists()).toBe(false)
    expect(w.emitted('save-preset')).toBeUndefined()
  })
})

describe('StylistPage presets', () => {
  async function mountPage() {
    const w = mount(StylistPage)
    await settle()
    return w
  }

  async function planned() {
    const w = await mountPage()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    return w
  }

  async function saveFrom(w, button, name) {
    await w.find(tid('series-card')).find(button).trigger('click')
    await w.find(tid('preset-name')).setValue(name)
    await w.find(tid('preset-save')).trigger('click')
    await settle()
  }

  it('saving a field stores it with the Russian text and reloads the base; the new preset shows in «Мои»', async () => {
    const w = await planned()
    expect(StylistPresets).toHaveBeenCalledTimes(1)
    StylistPresets.mockImplementation(() => {
      const b = BASE()
      b.hair = [...b.hair, { id: 'u1', user: true, name: 'Мой пучок', prompt: 'low bun', prompt_ru: 'Пучок' }]
      return Promise.resolve(b)
    })
    await saveFrom(w, tid('save-preset-hair'), 'Мой пучок')
    expect(StylistTranslate).not.toHaveBeenCalled()
    expect(StylistSavePreset).toHaveBeenCalledTimes(1)
    const sent = StylistSavePreset.mock.calls[0][0]
    expect(sent).toMatchObject({ aspect: 'hair', name: 'Мой пучок', prompt: 'low bun', prompt_ru: 'Пучок' })
    expect(sent).not.toHaveProperty('dirty')
    expect(StylistPresets).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain(t('stylist.preset_saved'))
    await w.find(tid('tab-hair')).trigger('click')
    await w.find(tid('mode-pick')).trigger('click')
    expect(w.find(tid('user-group')).find(tid('pick-u1')).exists()).toBe(true)
  })

  it('in English UI the Russian text is not saved', async () => {
    setLocale('en')
    const w = await planned()
    await saveFrom(w, tid('save-preset-outfit'), 'My dress')
    expect(StylistSavePreset.mock.calls[0][0]).toMatchObject({ aspect: 'outfit', prompt: 'white dress', prompt_ru: '' })
  })

  it('a Russian edit is translated before saving; English comes from the translation', async () => {
    StylistTranslate.mockImplementation(() => Promise.resolve(['braid']))
    const w = await planned()
    const hair = w.find(tid('series-card')).find(tid('series-hair'))
    await hair.setValue('Коса')
    await hair.trigger('change')
    await settle()
    await saveFrom(w, tid('save-preset-hair'), 'Коса')
    expect(StylistTranslate).toHaveBeenCalledWith(['Коса'])
    expect(StylistTranslate.mock.invocationCallOrder[0]).toBeLessThan(StylistSavePreset.mock.invocationCallOrder[0])
    expect(StylistSavePreset.mock.calls[0][0]).toMatchObject({ aspect: 'hair', prompt: 'braid', prompt_ru: 'Коса' })
  })

  it('background is saved with the series place', async () => {
    const w = await planned()
    await saveFrom(w, tid('save-preset-background'), 'Мой пляж')
    expect(StylistSavePreset.mock.calls[0][0]).toMatchObject({ aspect: 'background', place: 'beach', prompt: 'sandy beach' })
  })

  it('a shot is saved as a pose with framing, places and orientation', async () => {
    const w = await planned()
    await w.find(tid('series-card')).findAll(tid('shot-save'))[1].trigger('click')
    await w.find(tid('preset-name')).setValue('Лёжа')
    await w.find(tid('preset-save')).trigger('click')
    await settle()
    expect(StylistSavePreset.mock.calls[0][0]).toMatchObject({
      aspect: 'pose', prompt: 'lying on sand', prompt_ru: 'Лёжа', framing: 'full', places: ['beach'], orientation: 'landscape',
    })
  })

  it('deleting a series closes its open form, so the next series is not saved under that name', async () => {
    StylistPlan.mockImplementation(() => Promise.resolve({ series: [SERIES(), { ...SERIES(), hair: 'high ponytail', hair_ru: 'Хвост' }], warnings: [], used_llm: false }))
    const w = await planned()
    await w.findAll(tid('series-card'))[0].find(tid('save-preset-hair')).trigger('click')
    await w.find(tid('preset-name')).setValue('Пучок')
    await w.findAll(tid('series-card'))[0].find(tid('series-delete')).trigger('click')
    await settle()
    expect(w.findAll(tid('series-card'))).toHaveLength(1)
    expect(w.find(tid('preset-name')).exists()).toBe(false)
  })

  it('deleting a shot closes an open pose form of another shot', async () => {
    const three = { ...SERIES(), shots: [...SERIES().shots, { pose: 'Сидя', prompt: 'sitting on sand', prompt_ru: 'Сидя', landscape: false, framing: 'knee' }] }
    StylistPlan.mockImplementation(() => Promise.resolve({ series: [three], warnings: [], used_llm: false }))
    const w = await planned()
    const card = () => w.find(tid('series-card'))
    await card().findAll(tid('shot-save'))[1].trigger('click')
    expect(w.find(tid('preset-framing')).exists()).toBe(true)
    await card().findAll(tid('shot-delete'))[0].trigger('click')
    await settle()
    expect(w.find(tid('preset-framing')).exists()).toBe(false)
  })

  it('a save error is shown and the base is not reloaded', async () => {
    StylistSavePreset.mockImplementation(() => Promise.reject(new Error('название до 60 символов')))
    const w = await planned()
    await saveFrom(w, tid('save-preset-hair'), 'x')
    expect(w.find(tid('stylist-error')).text()).toContain('название до 60 символов')
    expect(StylistPresets).toHaveBeenCalledTimes(1)
  })

  it('rename calls the backend and reloads the base', async () => {
    StylistPresets.mockImplementation(() => Promise.resolve(withUserOutfit()))
    const w = await mountPage()
    await w.find(tid('mode-pick')).trigger('click')
    await w.find(tid('user-rename-u3')).trigger('click')
    const input = w.find(tid('user-name-u3'))
    await input.setValue('Красное')
    await input.trigger('keydown', { key: 'Enter' })
    await settle()
    expect(StylistRenamePreset).toHaveBeenCalledWith('u3', 'Красное')
    expect(StylistPresets).toHaveBeenCalledTimes(2)
  })

  it('delete calls the backend, reloads and drops the id from the picked ones; the plan keeps its text', async () => {
    StylistPresets.mockImplementation(() => Promise.resolve(withUserOutfit()))
    StylistPlan.mockImplementation(() => Promise.resolve({ series: [{ ...SERIES(), outfit: 'red dress', outfit_ru: 'Красное платье' }], warnings: [], used_llm: false }))
    const w = await mountPage()
    await w.find(tid('mode-pick')).trigger('click')
    await w.find(tid('pick-o1')).trigger('click')
    await w.find(tid('pick-u3')).trigger('click')
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistPlan.mock.calls[0][0].aspects.outfit).toEqual({ mode: 'pick', ids: ['o1', 'u3'], text: '' })

    StylistPresets.mockImplementation(() => Promise.resolve(BASE()))
    await w.find(tid('user-delete-u3')).trigger('click')
    await w.find(tid('user-delete-u3')).trigger('click')
    await settle()
    expect(StylistDeletePreset).toHaveBeenCalledWith('u3')
    expect(StylistPresets).toHaveBeenCalledTimes(2)
    expect(w.find(tid('pick-u3')).exists()).toBe(false)
    expect(w.find(tid('series-card')).find(tid('series-outfit')).element.value).toBe('Красное платье')

    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(StylistPlan.mock.calls[1][0].aspects.outfit).toEqual({ mode: 'pick', ids: ['o1'], text: '' })
  })
})
