import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, StylistAppearance, StylistPickGarment, StylistDescribeGarment,
  ListPeople, FaceSwapStatus, GetQueue, StylistFrames, StylistTranslateRu, StylistLoadPlan, StylistSavePlan,
  StylistPlans, StylistSaveNamedPlan, StylistRenamePlan, StylistDeletePlan,
} from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import StylistPage from '../components/StylistPage.vue'
import { useStylistPlan, resetStylistPlan, planSnapshot } from '../composables/useStylistPlan.js'

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

const MOUNTAIN_SERIES = () => [
  { outfit: 'silk gown', hair: 'braid', makeup: 'red lips', background: 'snowy peaks', place: 'mountain', shots: [
    { pose: 'Сбоку', prompt: 'side view on a ridge', landscape: true },
  ] },
]

const ANNA = { name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman', identity: true }
const BOB = { name: 'bob', base: 'Z-Image', lora: 'bob_z', gender: 'man', identity: false }

const APPEARANCE = { anna: 'a woman, 160 cm', bob: 'a man, 190 cm' }

const GARMENT_B64 = 'iVBORw0KGgoAAAAUNIQUEGARMENT'

function planJson({ series = MOUNTAIN_SERIES(), form = {} } = {}) {
  return JSON.stringify({ v: 1, form: { themes: ['winter'], wish: 'горы', frames: 1, ...form }, series })
}

function row(id, name, plan = planJson(), date = '2026-11-20 10:00:00') {
  return { id, name, plan, created_at: date, updated_at: date }
}

function deepKeys(value, out = new Set()) {
  if (Array.isArray(value)) value.forEach(v => deepKeys(v, out))
  else if (value && typeof value === 'object') {
    for (const [k, v] of Object.entries(value)) { out.add(k); deepKeys(v, out) }
  }
  return out
}

function deepStrings(value, out = []) {
  if (typeof value === 'string') out.push(value)
  else if (Array.isArray(value)) value.forEach(v => deepStrings(v, out))
  else if (value && typeof value === 'object') Object.values(value).forEach(v => deepStrings(v, out))
  return out
}

let savedPlans

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  useStylistPlan().form.frames = 3
  savedPlans = []
  StylistTranslateRu.mockImplementation((texts) => Promise.resolve(texts))
  StylistPresets.mockImplementation(() => Promise.resolve(structuredClone(BASE)))
  ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }, { ...BOB }]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation((name) => Promise.resolve(APPEARANCE[name] || ''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: SERIES(), warnings: [], used_llm: true }))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  StylistFrames.mockImplementation(() => Promise.resolve([]))
  StylistLoadPlan.mockImplementation(() => Promise.resolve(''))
  StylistSavePlan.mockImplementation(() => Promise.resolve())
  StylistPlans.mockImplementation(() => Promise.resolve(structuredClone(savedPlans)))
  StylistSaveNamedPlan.mockImplementation((name, plan) => Promise.resolve({ id: 100, name, plan, created_at: '2026-11-21 12:00:00', updated_at: '2026-11-21 12:00:00' }))
  StylistRenamePlan.mockImplementation(() => Promise.resolve())
  StylistDeletePlan.mockImplementation(() => Promise.resolve())
})

afterEach(() => {
  delete window.confirm
})

async function mountPage() {
  const w = mount(StylistPage)
  await settle()
  return w
}

async function makePlan(w) {
  await w.find(tid('stylist-plan')).trigger('click')
  await settle()
}

async function openSave(w) {
  await w.find(tid('plans-save')).trigger('click')
  await settle()
}

async function openList(w) {
  await w.find(tid('plans-open')).trigger('click')
  await settle()
}

function rowByName(w, name) {
  return w.findAll(tid('plans-row')).find(r => r.find(tid('plans-pick')).text().includes(name))
}

async function pick(w, name) {
  if (!w.find(tid('plans-menu')).exists()) await openList(w)
  const r = rowByName(w, name)
  expect(r, `plan row «${name}»`).toBeTruthy()
  await r.find(tid('plans-pick')).trigger('click')
  await settle()
}

describe('Stylist — saved plans', () => {
  it('1: «Сохранить» and «Мои планы» are shown; «Сохранить» is disabled without a plan, enabled with one', async () => {
    const w = await mountPage()
    expect(w.find(tid('plans-save')).exists()).toBe(true)
    expect(w.find(tid('plans-open')).exists()).toBe(true)
    expect(w.find(tid('plans-save')).element.disabled).toBe(true)
    await makePlan(w)
    expect(w.findAll(tid('series-card')).length).toBeGreaterThan(0)
    expect(w.find(tid('plans-save')).element.disabled).toBe(false)
  })

  it('2: the dialog prefills a name from the selected theme and the date; confirm saves with replace=false', async () => {
    const w = await mountPage()
    await w.find(tid('theme-beach')).trigger('click')
    await makePlan(w)
    await openSave(w)
    expect(w.find(tid('plans-dialog')).exists()).toBe(true)
    const name = w.find(tid('plans-name')).element.value
    expect(name).toContain('Пляж')
    expect(name).toMatch(/\d/)
    await w.find(tid('plans-confirm')).trigger('click')
    await settle()
    expect(StylistSaveNamedPlan).toHaveBeenCalledTimes(1)
    const [savedName, json, replace] = StylistSaveNamedPlan.mock.calls[0]
    expect(savedName).toBe(name)
    expect(typeof json).toBe('string')
    expect(() => JSON.parse(json)).not.toThrow()
    expect(replace).toBe(false)
  })

  it('2: several selected themes all go into the prefilled name', async () => {
    const w = await mountPage()
    await w.find(tid('theme-beach')).trigger('click')
    await w.find(tid('theme-winter')).trigger('click')
    await makePlan(w)
    await openSave(w)
    const name = w.find(tid('plans-name')).element.value
    expect(name).toContain('Пляж')
    expect(name).toContain('Зима')
  })

  it('2: an empty or blank name is not saved', async () => {
    const w = await mountPage()
    await makePlan(w)
    await openSave(w)
    for (const value of ['', '   ']) {
      await w.find(tid('plans-name')).setValue(value)
      const confirm = w.find(tid('plans-confirm'))
      if (!confirm.element.disabled) await confirm.trigger('click')
      await settle()
      expect(StylistSaveNamedPlan).not.toHaveBeenCalled()
    }
  })

  it('2: cancel closes the dialog without saving', async () => {
    const w = await mountPage()
    await makePlan(w)
    await openSave(w)
    await w.find(tid('plans-cancel')).trigger('click')
    await settle()
    expect(w.find(tid('plans-dialog')).exists()).toBe(false)
    expect(StylistSaveNamedPlan).not.toHaveBeenCalled()
  })

  it('3: saved JSON carries the plan and its settings but not the person, gender, appearance or garment', async () => {
    StylistPickGarment.mockImplementation(() => Promise.resolve(GARMENT_B64))
    StylistDescribeGarment.mockImplementation(() => Promise.resolve('a red velvet dress'))
    const w = await mountPage()
    await w.find(tid('stylist-person')).setValue('anna')
    await settle()
    await w.find(tid('stylist-appearance')).setValue('a woman, 160 cm, freckles')
    await w.find(tid('stylist-wish')).setValue('прогулка у моря')
    await w.find(tid('theme-beach')).trigger('click')
    const sevenShotSeries = SERIES()
    sevenShotSeries[0].shots.push(...Array.from({ length: 4 }, (_, index) => ({ prompt: `additional pose ${index}` })))
    StylistPlan.mockResolvedValue({ series: sevenShotSeries, warnings: [], used_llm: true })
    await w.find(tid('stylist-frames')).setValue('7')
    await w.find(tid('stylist-frames')).trigger('change')
    await w.find(tid('stylist-frame-size')).setValue('large')
    await w.find(tid('stylist-garment-pick')).trigger('click')
    await settle()
    await makePlan(w)
    await openSave(w)
    await w.find(tid('plans-name')).setValue('Море')
    await w.find(tid('plans-confirm')).trigger('click')
    await settle()
    expect(StylistSaveNamedPlan).toHaveBeenCalledTimes(1)
    const json = StylistSaveNamedPlan.mock.calls[0][1]
    const parsed = JSON.parse(json)
    const keys = deepKeys(parsed)
    const strings = deepStrings(parsed)

    expect(strings).toContain('facing camera')
    expect(strings).toContain('lying on sand')
    expect(strings).toContain('wool coat')
    expect(strings).toContain('прогулка у моря')
    expect(strings).toContain('beach')
    for (const k of ['series', 'shots', 'themes', 'aspects', 'wish', 'frames', 'target', 'targetKind', 'seed', 'faceSwap', 'frameSize']) {
      expect(keys.has(k), `key ${k}`).toBe(true)
    }
    expect(json).toContain('"large"')
    expect(json).toMatch(/"frames":\s*7\b/)

    for (const k of ['person', 'gender', 'appearance', 'garmentOutfit', 'garment']) {
      expect(keys.has(k), `forbidden key ${k}`).toBe(false)
    }
    expect(strings).not.toContain('anna')
    expect(strings).not.toContain('woman')
    expect(json).not.toContain('freckles')
    expect(json).not.toContain('a woman, 160 cm')
    expect(json).not.toContain('red velvet dress')
    expect(json).not.toContain(GARMENT_B64)
  })

  it('4: an existing name (any case) asks to replace first; confirming saves with replace=true', async () => {
    savedPlans = [row(5, 'Море')]
    const w = await mountPage()
    await makePlan(w)
    await openSave(w)
    await w.find(tid('plans-name')).setValue('МОРЕ')
    await w.find(tid('plans-confirm')).trigger('click')
    await settle()
    expect(w.find(tid('plans-replace-ask')).exists()).toBe(true)
    expect(StylistSaveNamedPlan).not.toHaveBeenCalled()
    await w.find(tid('plans-confirm')).trigger('click')
    await settle()
    expect(StylistSaveNamedPlan).toHaveBeenCalledTimes(1)
    const [name, json, replace] = StylistSaveNamedPlan.mock.calls[0]
    expect(name).toBe('МОРЕ')
    expect(JSON.parse(json)).toBeTruthy()
    expect(replace).toBe(true)
  })

  it('4: a new name saves at once without the replace question', async () => {
    savedPlans = [row(5, 'Море')]
    const w = await mountPage()
    await makePlan(w)
    await openSave(w)
    await w.find(tid('plans-name')).setValue('Море и горы')
    await w.find(tid('plans-confirm')).trigger('click')
    await settle()
    expect(w.find(tid('plans-replace-ask')).exists()).toBe(false)
    expect(StylistSaveNamedPlan).toHaveBeenCalledWith('Море и горы', expect.any(String), false)
  })

  it('5: the list shows name and «N серий · M кадров · date» with Russian plurals', async () => {
    const shots = (n) => Array.from({ length: n }, (_, i) => ({ pose: 'p' + i, prompt: 'shot ' + i, landscape: false }))
    const series = (counts) => counts.map(c => ({ outfit: 'o', hair: 'h', makeup: 'm', background: 'b', place: 'any', shots: shots(c) }))
    savedPlans = [
      row(1, 'Один', planJson({ series: series([1]) })),
      row(2, 'Два', planJson({ series: series([2, 1]) })),
      row(3, 'Пять', planJson({ series: series([3, 3, 2, 2, 2]) })),
    ]
    const w = await mountPage()
    await openList(w)
    expect(w.find(tid('plans-menu')).exists()).toBe(true)
    expect(w.findAll(tid('plans-row'))).toHaveLength(3)
    const meta = (name) => rowByName(w, name).find(tid('plans-meta')).text()
    expect(meta('Один')).toMatch(/^1 серия · 1 кадр · \S/)
    expect(meta('Два')).toMatch(/^2 серии · 3 кадра · \S/)
    expect(meta('Пять')).toMatch(/^5 серий · 12 кадров · \S/)
    expect(w.find(tid('plans-empty')).exists()).toBe(false)
  })

  it('5: empty list shows a hint', async () => {
    const w = await mountPage()
    await openList(w)
    expect(w.find(tid('plans-empty')).exists()).toBe(true)
    expect(w.findAll(tid('plans-row'))).toHaveLength(0)
  })

  it('6: picking a plan loads its series, themes and wish but keeps person, appearance, gender, garment photo and its description', async () => {
    StylistPickGarment.mockImplementation(() => Promise.resolve(GARMENT_B64))
    StylistDescribeGarment.mockImplementation(() => Promise.resolve('a red velvet dress'))
    savedPlans = [row(7, 'Горы', planJson({ form: {
      themes: ['winter'], wish: 'горы', frames: 1,
      person: 'anna', gender: 'woman', appearance: 'a woman, 160 cm', garmentOutfit: 'a ski suit',
    } }))]
    const w = await mountPage()
    await w.find(tid('stylist-person')).setValue('bob')
    await settle()
    await w.find(tid('stylist-appearance')).setValue('a man, 185 cm, beard')
    await w.find(tid('stylist-garment-pick')).trigger('click')
    await settle()
    await w.find(tid('stylist-garment-outfit')).setValue('a grey hoodie')
    await w.find(tid('theme-beach')).trigger('click')
    await makePlan(w)
    const before = JSON.parse(planSnapshot())
    window.confirm = vi.fn(() => true)
    await pick(w, 'Горы')

    expect(w.find(tid('stylist-wish')).element.value).toBe('горы')
    expect(w.find(tid('theme-winter')).classes()).toContain('active')
    expect(w.find(tid('theme-beach')).classes()).not.toContain('active')
    expect(w.findAll(tid('series-card'))).toHaveLength(1)
    const after = JSON.parse(planSnapshot())
    expect(deepStrings(after.series)).toContain('side view on a ridge')
    expect(deepStrings(after.series)).not.toContain('lying on sand')

    expect(w.find(tid('stylist-person')).element.value).toBe('bob')
    expect(w.find(tid('stylist-appearance')).element.value).toBe('a man, 185 cm, beard')
    expect(w.find(tid('stylist-garment-outfit')).element.value).toBe('a grey hoodie')
    expect(w.find(tid('stylist-garment-clear')).exists()).toBe(true)
    if (before.form && 'gender' in before.form) expect(after.form.gender).toBe(before.form.gender)
  })

  it('6: a plan saved from the page round-trips through the list', async () => {
    const w = await mountPage()
    await w.find(tid('theme-beach')).trigger('click')
    await w.find(tid('stylist-wish')).setValue('закат')
    await makePlan(w)
    await openSave(w)
    await w.find(tid('plans-name')).setValue('Закат')
    await w.find(tid('plans-confirm')).trigger('click')
    await settle()
    savedPlans = [row(9, 'Закат', StylistSaveNamedPlan.mock.calls[0][1])]
    await w.find(tid('stylist-wish')).setValue('другое')
    await w.find(tid('theme-beach')).trigger('click')
    window.confirm = vi.fn(() => true)
    await pick(w, 'Закат')
    expect(w.find(tid('stylist-wish')).element.value).toBe('закат')
    expect(w.find(tid('theme-beach')).classes()).toContain('active')
    expect(w.findAll(tid('series-card'))).toHaveLength(2)
  })

  it('7: an edited unsaved plan asks before being replaced; Cancel keeps it', async () => {
    savedPlans = [row(1, 'Горы'), row(2, 'Лес', planJson({ form: { wish: 'лес', themes: [] } }))]
    const w = await mountPage()
    window.confirm = vi.fn(() => true)
    await pick(w, 'Горы')
    await w.find(tid('stylist-wish')).setValue('горы и озеро')
    await settle()
    window.confirm = vi.fn(() => false)
    await pick(w, 'Лес')
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(w.find(tid('stylist-wish')).element.value).toBe('горы и озеро')
    expect(deepStrings(JSON.parse(planSnapshot()).series)).toContain('side view on a ridge')

    window.confirm = vi.fn(() => true)
    await pick(w, 'Лес')
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(w.find(tid('stylist-wish')).element.value).toBe('лес')
  })

  it('7: right after loading a plan, picking another does not ask', async () => {
    savedPlans = [row(1, 'Горы'), row(2, 'Лес', planJson({ form: { wish: 'лес', themes: [] } }))]
    const w = await mountPage()
    window.confirm = vi.fn(() => true)
    await pick(w, 'Горы')
    window.confirm = vi.fn(() => true)
    await pick(w, 'Лес')
    expect(window.confirm).not.toHaveBeenCalled()
    expect(w.find(tid('stylist-wish')).element.value).toBe('лес')
  })

  it('7: right after saving a plan, picking another does not ask', async () => {
    savedPlans = [row(2, 'Лес', planJson({ form: { wish: 'лес', themes: [] } }))]
    const w = await mountPage()
    await w.find(tid('stylist-wish')).setValue('пляж')
    await makePlan(w)
    await openSave(w)
    await w.find(tid('plans-name')).setValue('Пляж')
    await w.find(tid('plans-confirm')).trigger('click')
    await settle()
    expect(StylistSaveNamedPlan).toHaveBeenCalledTimes(1)
    window.confirm = vi.fn(() => true)
    await pick(w, 'Лес')
    expect(window.confirm).not.toHaveBeenCalled()
    expect(w.find(tid('stylist-wish')).element.value).toBe('лес')
  })

  it('8: «Переименовать» opens an inline input; Enter renames by id', async () => {
    savedPlans = [row(11, 'Горы'), row(12, 'Лес')]
    const w = await mountPage()
    await openList(w)
    const r = rowByName(w, 'Горы')
    await r.find(tid('plans-rename')).trigger('click')
    await settle()
    const input = w.find(tid('plans-rename-input'))
    expect(input.exists()).toBe(true)
    await input.setValue('Высокие горы')
    await input.trigger('keydown', { key: 'Enter' })
    await settle()
    expect(StylistRenamePlan).toHaveBeenCalledTimes(1)
    expect(StylistRenamePlan).toHaveBeenCalledWith(11, 'Высокие горы')
  })

  it('8: Esc in rename cancels, even if blur follows', async () => {
    savedPlans = [row(11, 'Горы')]
    const w = await mountPage()
    await openList(w)
    await rowByName(w, 'Горы').find(tid('plans-rename')).trigger('click')
    await settle()
    const input = w.find(tid('plans-rename-input'))
    await input.setValue('Другое')
    await input.trigger('keydown', { key: 'Escape' })
    await input.trigger('blur')
    await settle()
    expect(StylistRenamePlan).not.toHaveBeenCalled()
    expect(w.find(tid('plans-rename-input')).exists()).toBe(false)
  })

  it('8: «Удалить» asks and deletes by id only on OK', async () => {
    savedPlans = [row(11, 'Горы'), row(12, 'Лес')]
    const w = await mountPage()
    await openList(w)
    window.confirm = vi.fn(() => false)
    await rowByName(w, 'Лес').find(tid('plans-delete')).trigger('click')
    await settle()
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(StylistDeletePlan).not.toHaveBeenCalled()
    expect(w.find(tid('plans-menu')).exists()).toBe(true)
    window.confirm = vi.fn(() => true)
    await rowByName(w, 'Лес').find(tid('plans-delete')).trigger('click')
    await settle()
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(StylistDeletePlan).toHaveBeenCalledTimes(1)
    expect(StylistDeletePlan).toHaveBeenCalledWith(12)
  })
})
