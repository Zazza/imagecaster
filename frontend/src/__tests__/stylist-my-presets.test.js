import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, ListPeople, FaceSwapStatus, StylistAppearance, GetQueue, StylistFrames, StylistSavePreset, StylistTranslate,
} from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { resetStylistPlan, useStylistPlan, presetFromText } from '../composables/useStylistPlan.js'
import StylistPage from '../components/StylistPage.vue'
import AspectTab from '../components/stylist/AspectTab.vue'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

const BASE = () => ({
  version: 1,
  themes: [{ id: 'beach', name: 'Пляж' }],
  outfit: [{ id: 'o1', theme: 'beach', name: 'Платье', prompt: 'white dress' }],
  hair: [{ id: 'h1', name: 'Пучок', prompt: 'low bun' }],
  makeup: [{ id: 'm1', name: 'Без', prompt: 'no makeup' }],
  background: [{ id: 'b1', theme: 'beach', name: 'Пляж', prompt: 'sandy beach', place: 'beach' }, { id: 'b2', name: 'Студия', prompt: 'grey', place: 'studio' }],
  pose: [{ id: 'p1', name: 'Анфас', prompt: 'facing camera', framing: 'close', places: ['any'], orientation: 'portrait' }],
})

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  useStylistPlan().form.frames = 1
  StylistPresets.mockImplementation(() => Promise.resolve(BASE()))
  ListPeople.mockImplementation(() => Promise.resolve([{ name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman' }]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: [], warnings: [], used_llm: true }))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  StylistFrames.mockImplementation(() => Promise.resolve([]))
  StylistSavePreset.mockImplementation(() => Promise.resolve('u7'))
  StylistTranslate.mockImplementation((list) => Promise.resolve(list.map(() => 'black tuxedo, white shirt')))
})

describe('presetFromText', () => {
  it('Russian text goes to translation, English is kept as is', () => {
    expect(presetFromText('outfit', ' чёрный смокинг ')).toMatchObject({ aspect: 'outfit', prompt: 'чёрный смокинг', prompt_ru: 'чёрный смокинг', dirty: true })
    expect(presetFromText('hair', 'short bob')).toMatchObject({ aspect: 'hair', prompt: 'short bob', prompt_ru: '', dirty: false })
  })
})

describe('AspectTab «Мои»', () => {
  const tab = (mode, aspect = 'outfit') => mount(AspectTab, { props: { aspect, choice: { mode, ids: [] }, items: BASE()[aspect], places: ['beach', 'studio'] } })

  it('visible in «Выбрать» only', () => {
    expect(tab('pick').find(tid('user-group')).exists()).toBe(true)
    for (const m of ['auto', 'keep', 'custom']) expect(tab(m).find(tid('user-group')).exists()).toBe(false)
  })

  it('background needs a place; pose needs places and sends orientation', async () => {
    const bg = tab('pick', 'background')
    await bg.find(tid('user-add')).trigger('click')
    await bg.find(tid('preset-name')).setValue('Крыша')
    await bg.find(tid('preset-text')).setValue('rooftop at sunset')
    expect(bg.find(tid('preset-save')).attributes('disabled')).toBeDefined()
    await bg.find(tid('preset-place')).setValue('studio')
    await bg.find(tid('preset-save')).trigger('click')
    expect(bg.emitted('save-preset')[0][0]).toMatchObject({ aspect: 'background', name: 'Крыша', prompt: 'rooftop at sunset', place: 'studio' })
    expect(bg.find(tid('save-preset-form')).exists()).toBe(false)

    const pose = tab('pick', 'pose')
    await pose.find(tid('user-add')).trigger('click')
    await pose.find(tid('preset-name')).setValue('Ступеньки')
    await pose.find(tid('preset-text')).setValue('sitting on steps')
    expect(pose.find(tid('preset-save')).attributes('disabled')).toBeDefined()
    await pose.find(tid('preset-place-any')).trigger('click')
    await pose.find(tid('preset-framing')).setValue('knee')
    await pose.find(tid('preset-orientation')).setValue('landscape')
    await pose.find(tid('preset-save')).trigger('click')
    expect(pose.emitted('save-preset')[0][0]).toMatchObject({ aspect: 'pose', framing: 'knee', places: ['any'], orientation: 'landscape', prompt: 'sitting on steps' })
  })

  it('empty text cannot be saved; cancel closes the form', async () => {
    const w = tab('pick')
    await w.find(tid('user-add')).trigger('click')
    await w.find(tid('preset-name')).setValue('Смокинг')
    expect(w.find(tid('preset-save')).attributes('disabled')).toBeDefined()
    await w.find(tid('preset-cancel')).trigger('click')
    expect(w.find(tid('save-preset-form')).exists()).toBe(false)
    expect(w.find(tid('user-empty')).exists()).toBe(true)
  })
})

describe('Stylist page: own preset without a plan', () => {
  it('Russian text is translated, saved into «Мои» and picked', async () => {
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('tab-outfit')).trigger('click')
    await w.find(tid('mode-pick')).trigger('click')
    await settle()
    await w.find(tid('user-add')).trigger('click')
    await w.find(tid('preset-name')).setValue('Смокинг')
    await w.find(tid('preset-text')).setValue('чёрный смокинг, белая рубашка')
    StylistPresets.mockClear()
    await w.find(tid('preset-save')).trigger('click')
    await settle()
    expect(StylistTranslate).toHaveBeenCalledWith(['чёрный смокинг, белая рубашка'])
    expect(StylistSavePreset).toHaveBeenCalledWith(expect.objectContaining({ aspect: 'outfit', name: 'Смокинг', prompt: 'black tuxedo, white shirt', prompt_ru: 'чёрный смокинг, белая рубашка' }))
    expect(StylistPresets).toHaveBeenCalled()
    const { form } = useStylistPlan()
    expect(form.aspects.outfit).toMatchObject({ mode: 'pick', ids: ['u7'] })
  })

  it('save error keeps nothing picked', async () => {
    StylistSavePreset.mockImplementation(() => Promise.reject(new Error('пустой текст')))
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('tab-hair')).trigger('click')
    await w.find(tid('mode-pick')).trigger('click')
    await w.find(tid('user-add')).trigger('click')
    await w.find(tid('preset-name')).setValue('Боб')
    await w.find(tid('preset-text')).setValue('short bob')
    await w.find(tid('preset-save')).trigger('click')
    await settle()
    expect(StylistTranslate).not.toHaveBeenCalled()
    const { form } = useStylistPlan()
    expect(form.aspects.hair.ids).toEqual([])
    expect(w.text()).toContain('пустой текст')
  })
})

describe('plan card button has a label', () => {
  it('«В мои» text next to the bookmark', async () => {
    StylistPlan.mockImplementation(() => Promise.resolve({ series: [{ outfit: 'white dress', hair: 'low bun', makeup: '', background: 'sandy beach', place: 'beach', shots: [{ pose: 'Анфас', prompt: 'facing camera', landscape: false, framing: 'close' }] }], warnings: [], used_llm: true }))
    const w = mount(StylistPage)
    await settle()
    await w.find(tid('stylist-plan')).trigger('click')
    await settle()
    expect(w.find(tid('save-preset-outfit')).text()).toBe(t('stylist.save_mine'))
    expect(w.find(tid('shot-save')).text()).toBe(t('stylist.save_mine'))
  })
})
