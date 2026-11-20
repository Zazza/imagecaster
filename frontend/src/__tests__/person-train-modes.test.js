import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PeoplePage from '../components/PeoplePage.vue'
import {
  PersonSetSummary, PersonSetList, PersonSetThumb, TrainPersonFromSet, RollbackPerson,
  ListPeople, TrainingStatus, FaceSwapStatus, TrainingBases, ListTrainingJobs,
} from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const TID = id => `[data-testid="${id}"]`

const T0 = '2026-11-10T10:00:00Z'
const BEFORE = ['2026-11-01T10:00:00Z', '2026-11-05T10:00:00Z']
const AFTER = ['2026-11-12T10:00:00Z', '2026-11-15T10:00:00Z', '2026-11-18T10:00:00Z']

function set(key, count, added = []) {
  return { key, count, front: count, half: 0, profile: 0, last_added: added[added.length - 1] || '', added }
}

function lora(over) {
  return {
    name: 'kira_z', base: 'Z-Image', gender: 'woman', display_name: 'Кира', steps: 2000,
    created_at: '2026-11-01T00:00:00Z', mode: 'full', rounds: 0, photos_at: T0, previous: false,
    ...over,
  }
}

let confirmSpy

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  setLocale('ru')
  document.body.innerHTML = ''
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image', 'SDXL']))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  ListPeople.mockImplementation(() => Promise.resolve([]))
  PersonSetSummary.mockImplementation(() => Promise.resolve([]))
  PersonSetList.mockImplementation(() => Promise.resolve([]))
  PersonSetThumb.mockImplementation(() => Promise.resolve('QUJD'))
  RollbackPerson.mockImplementation(() => Promise.resolve())
  TrainPersonFromSet.mockImplementation((key, name, gender, base) => Promise.resolve({ id: 'j1', name, base, status: 'queued', progress: { step: 0, steps: 2000 } }))
  confirmSpy = vi.fn(() => true)
  window.confirm = confirmSpy
})

afterEach(() => {
  delete window.confirm
})

async function mountPeople() {
  const w = mount(PeoplePage, { attachTo: document.body })
  await settle()
  return w
}

function cardOf(w, key) {
  return w.findAll(TID('set-card')).find(c => c.text().includes(key))
}

function rowsOf(w, key) {
  const card = cardOf(w, key)
  expect(card, `set card «${key}»`).toBeTruthy()
  return card.findAll(TID('set-lora'))
}

function rowOf(w, key, base) {
  const row = rowsOf(w, key).find(r => r.text().includes(base))
  expect(row, `LoRA row ${base} in «${key}»`).toBeTruthy()
  return row
}

function norm(s) {
  return s.replace(/\s+/g, ' ').trim()
}

describe('condition 1: LoRA rows in the set card', () => {
  it('one row per LoRA of that person (display_name match, case/spaces ignored, name fallback); others excluded', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, AFTER)]))
    ListPeople.mockImplementation(() => Promise.resolve([
      lora({ name: 'kira_z', base: 'Z-Image', display_name: '  кира ' }),
      lora({ name: 'кира', base: 'SDXL', display_name: '' }),
      lora({ name: 'olya_z', base: 'Z-Image', display_name: 'Оля' }),
    ]))
    const w = await mountPeople()
    const rows = rowsOf(w, 'Кира')
    expect(rows).toHaveLength(2)
    expect(rows.some(r => r.text().includes('Z-Image'))).toBe(true)
    expect(rows.some(r => r.text().includes('SDXL'))).toBe(true)
  })

  it('no LoRAs for the person — no rows', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12)]))
    ListPeople.mockImplementation(() => Promise.resolve([lora({ display_name: 'Оля' })]))
    const w = await mountPeople()
    expect(rowsOf(w, 'Кира')).toHaveLength(0)
  })

  it.each([
    ['quick, rounds 0', { mode: 'quick', rounds: 0 }, 'начальная (600 шагов)'],
    ['quick, rounds 3', { mode: 'quick', rounds: 3 }, 'дообучений: 3'],
    ['full, rounds 2', { mode: 'full', rounds: 2 }, 'дообучений: 2'],
    ['full, rounds 0', { mode: 'full', rounds: 0 }, 'полная'],
    ['empty mode (old LoRA)', { mode: '', rounds: 0 }, 'полная'],
  ])('state: %s', async (_, over, text) => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12)]))
    ListPeople.mockImplementation(() => Promise.resolve([lora(over)]))
    const w = await mountPeople()
    const state = rowOf(w, 'Кира', 'Z-Image').find(TID('set-lora-state'))
    expect(state.exists()).toBe(true)
    expect(norm(state.text())).toContain(text)
  })

  it('new photos: counts only `added` later than photos_at', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, [...BEFORE, ...AFTER])]))
    ListPeople.mockImplementation(() => Promise.resolve([lora({ photos_at: T0 })]))
    const w = await mountPeople()
    const el = rowOf(w, 'Кира', 'Z-Image').find(TID('set-lora-new'))
    expect(norm(el.text())).toContain('новых фото: 3')
  })

  it('new photos: none later than photos_at → «новых фото нет»', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, BEFORE)]))
    ListPeople.mockImplementation(() => Promise.resolve([lora({ photos_at: T0 })]))
    const w = await mountPeople()
    const el = rowOf(w, 'Кира', 'Z-Image').find(TID('set-lora-new'))
    expect(norm(el.text())).toContain('новых фото нет')
  })

  it('new photos: empty `added` → «новых фото нет»', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, [])]))
    ListPeople.mockImplementation(() => Promise.resolve([lora()]))
    const w = await mountPeople()
    expect(norm(rowOf(w, 'Кира', 'Z-Image').find(TID('set-lora-new')).text())).toContain('новых фото нет')
  })

  it('new photos: without photos_at falls back to created_at', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, [...BEFORE, ...AFTER])]))
    ListPeople.mockImplementation(() => Promise.resolve([lora({ photos_at: '', created_at: '2026-11-03T00:00:00Z' })]))
    const w = await mountPeople()
    expect(norm(rowOf(w, 'Кира', 'Z-Image').find(TID('set-lora-new')).text())).toContain('новых фото: 4')
  })

  it('new photos are counted per LoRA (each against its own photos_at)', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, [...BEFORE, ...AFTER])]))
    ListPeople.mockImplementation(() => Promise.resolve([
      lora({ name: 'kira_z', base: 'Z-Image', photos_at: T0 }),
      lora({ name: 'kira_x', base: 'SDXL', photos_at: '2026-11-20T00:00:00Z' }),
    ]))
    const w = await mountPeople()
    expect(norm(rowOf(w, 'Кира', 'Z-Image').find(TID('set-lora-new')).text())).toContain('новых фото: 3')
    expect(norm(rowOf(w, 'Кира', 'SDXL').find(TID('set-lora-new')).text())).toContain('новых фото нет')
  })
})

describe('condition 2: «Дообучить» / «Переучить с нуля»', () => {
  const LORA = lora({ name: 'kira_z', base: 'Z-Image', gender: 'woman', mode: 'quick', rounds: 0 })

  describe.each([
    ['set-continue', 'continue'],
    ['set-retrain', 'retrain'],
  ])('%s', (testid, mode) => {
    it(`asks confirmation; OK → TrainPersonFromSet(set, lora, gender, base, '${mode}', false, false)`, async () => {
      PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, AFTER)]))
      ListPeople.mockImplementation(() => Promise.resolve([LORA]))
      const w = await mountPeople()
      const btn = rowOf(w, 'Кира', 'Z-Image').find(TID(testid))
      expect(btn.exists()).toBe(true)
      expect(btn.element.disabled).toBe(false)
      await btn.trigger('click')
      await settle()
      expect(confirmSpy).toHaveBeenCalledTimes(1)
      expect(TrainPersonFromSet).toHaveBeenCalledTimes(1)
      expect(TrainPersonFromSet).toHaveBeenCalledWith('Кира', 'kira_z', 'woman', 'Z-Image', mode, false, false)
    })

    it('Cancel → nothing called', async () => {
      confirmSpy.mockImplementation(() => false)
      PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, AFTER)]))
      ListPeople.mockImplementation(() => Promise.resolve([LORA]))
      const w = await mountPeople()
      await rowOf(w, 'Кира', 'Z-Image').find(TID(testid)).trigger('click')
      await settle()
      expect(confirmSpy).toHaveBeenCalledTimes(1)
      expect(TrainPersonFromSet).not.toHaveBeenCalled()
      expect(RollbackPerson).not.toHaveBeenCalled()
    })

    it('disabled when the set has fewer than 10 photos', async () => {
      PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 9, AFTER)]))
      ListPeople.mockImplementation(() => Promise.resolve([LORA]))
      const w = await mountPeople()
      const btn = rowOf(w, 'Кира', 'Z-Image').find(TID(testid))
      expect(btn.element.disabled).toBe(true)
      await btn.trigger('click')
      await settle()
      expect(TrainPersonFromSet).not.toHaveBeenCalled()
    })

    it('enabled at exactly 10 photos', async () => {
      PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 10, AFTER)]))
      ListPeople.mockImplementation(() => Promise.resolve([LORA]))
      const w = await mountPeople()
      expect(rowOf(w, 'Кира', 'Z-Image').find(TID(testid)).element.disabled).toBe(false)
    })

    it.each(['queued', 'running', 'paused'])('disabled with set-lora-busy while a job for that LoRA is %s', async status => {
      PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, AFTER)]))
      ListPeople.mockImplementation(() => Promise.resolve([LORA]))
      ListTrainingJobs.mockImplementation(() => Promise.resolve([{ id: 'j9', name: 'kira_z', base: 'Z-Image', status, progress: { step: 10, steps: 600 } }]))
      const w = await mountPeople()
      const row = rowOf(w, 'Кира', 'Z-Image')
      expect(row.find(TID('set-lora-busy')).exists()).toBe(true)
      expect(row.find(TID(testid)).element.disabled).toBe(true)
    })
  })

  it.each(['done', 'failed', 'cancelled'])('a %s job for that LoRA does not block', async status => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, AFTER)]))
    ListPeople.mockImplementation(() => Promise.resolve([LORA]))
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ id: 'j9', name: 'kira_z', base: 'Z-Image', status, progress: { step: 600, steps: 600 } }]))
    const w = await mountPeople()
    const row = rowOf(w, 'Кира', 'Z-Image')
    expect(row.find(TID('set-lora-busy')).exists()).toBe(false)
    expect(row.find(TID('set-continue')).element.disabled).toBe(false)
    expect(row.find(TID('set-retrain')).element.disabled).toBe(false)
  })

  it('a running job of another LoRA blocks only that LoRA', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12, AFTER)]))
    ListPeople.mockImplementation(() => Promise.resolve([
      LORA,
      lora({ name: 'kira_x', base: 'SDXL' }),
    ]))
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ id: 'j9', name: 'kira_x', base: 'SDXL', status: 'running', progress: { step: 10, steps: 2000 } }]))
    const w = await mountPeople()
    const z = rowOf(w, 'Кира', 'Z-Image')
    expect(z.find(TID('set-lora-busy')).exists()).toBe(false)
    expect(z.find(TID('set-continue')).element.disabled).toBe(false)
    const x = rowOf(w, 'Кира', 'SDXL')
    expect(x.find(TID('set-lora-busy')).exists()).toBe(true)
    expect(x.find(TID('set-continue')).element.disabled).toBe(true)
  })
})

describe('condition 3: «Вернуть прошлую»', () => {
  it('hidden when previous is false', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12)]))
    ListPeople.mockImplementation(() => Promise.resolve([lora({ previous: false })]))
    const w = await mountPeople()
    expect(rowOf(w, 'Кира', 'Z-Image').find(TID('set-rollback')).exists()).toBe(false)
  })

  it('shown when previous is true; confirm OK → RollbackPerson(loraName)', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12)]))
    ListPeople.mockImplementation(() => Promise.resolve([lora({ name: 'kira_z', previous: true })]))
    const w = await mountPeople()
    const btn = rowOf(w, 'Кира', 'Z-Image').find(TID('set-rollback'))
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    await settle()
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    expect(RollbackPerson).toHaveBeenCalledTimes(1)
    expect(RollbackPerson).toHaveBeenCalledWith('kira_z')
    expect(TrainPersonFromSet).not.toHaveBeenCalled()
  })

  it('confirm Cancel → RollbackPerson not called', async () => {
    confirmSpy.mockImplementation(() => false)
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12)]))
    ListPeople.mockImplementation(() => Promise.resolve([lora({ previous: true })]))
    const w = await mountPeople()
    await rowOf(w, 'Кира', 'Z-Image').find(TID('set-rollback')).trigger('click')
    await settle()
    expect(RollbackPerson).not.toHaveBeenCalled()
  })

  it('per LoRA: shown only on the row that has previous', async () => {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12)]))
    ListPeople.mockImplementation(() => Promise.resolve([
      lora({ name: 'kira_z', base: 'Z-Image', previous: true }),
      lora({ name: 'kira_x', base: 'SDXL', previous: false }),
    ]))
    const w = await mountPeople()
    expect(rowOf(w, 'Кира', 'Z-Image').find(TID('set-rollback')).exists()).toBe(true)
    expect(rowOf(w, 'Кира', 'SDXL').find(TID('set-rollback')).exists()).toBe(false)
  })
})

describe('condition 4: training mode in the «Из набора» form', () => {
  async function openSetForm() {
    PersonSetSummary.mockImplementation(() => Promise.resolve([set('Кира', 12)]))
    const w = await mountPeople()
    await cardOf(w, 'Кира').find(TID('set-train')).trigger('click')
    await settle()
    return w
  }

  it('select people-train-mode has quick (default) and full; texts mention 600 and 2000 steps', async () => {
    const w = await openSetForm()
    const sel = w.find(TID('people-train-mode'))
    expect(sel.exists()).toBe(true)
    const opts = Array.from(sel.element.options)
    expect(opts.map(o => o.value).sort()).toEqual(['full', 'quick'])
    expect(sel.element.value).toBe('quick')
    expect(opts.find(o => o.value === 'quick').textContent).toContain('600')
    expect(opts.find(o => o.value === 'full').textContent).toContain('2000')
  })

  it('«Обучить» with default mode → TrainPersonFromSet(..., "quick", hold, false)', async () => {
    const w = await openSetForm()
    await w.find(TID('people-name')).setValue('kira')
    await w.find(TID('people-gender')).setValue('woman')
    await w.find(TID('people-train')).trigger('click')
    await settle()
    expect(TrainPersonFromSet).toHaveBeenCalledTimes(1)
    expect(TrainPersonFromSet).toHaveBeenCalledWith('Кира', 'kira', 'woman', 'Z-Image', 'quick', false, false)
  })

  it('«Обучить» with full selected and hold → TrainPersonFromSet(..., "full", true, false)', async () => {
    const w = await openSetForm()
    await w.find(TID('people-name')).setValue('kira')
    await w.find(TID('people-gender')).setValue('woman')
    await w.find(TID('people-train-mode')).setValue('full')
    await w.find(TID('people-hold')).setValue(true)
    await w.find(TID('people-train')).trigger('click')
    await settle()
    expect(TrainPersonFromSet).toHaveBeenCalledTimes(1)
    expect(TrainPersonFromSet).toHaveBeenCalledWith('Кира', 'kira', 'woman', 'Z-Image', 'full', true, false)
  })
})

describe('old server ignores the training mode', () => {
  it('quick training on a server without modes shows a warning; a server that knows modes does not', async () => {
    const { default: PeoplePage } = await import('../components/PeoplePage.vue')
    const App = await import('../wailsjs/go/main/App.js')
    App.TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
    App.FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
    App.TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
    App.ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
    App.ListPeople.mockImplementation(() => Promise.resolve([]))
    App.PersonSetSummary.mockImplementation(() => Promise.resolve([{ key: 'Кира', count: 12, front: 12, half: 0, profile: 0, last_added: '', added: [] }]))
    for (const [jobMode, warn] of [[undefined, true], ['quick', false]]) {
      App.TrainPersonFromSet.mockImplementation(() => Promise.resolve({ id: 'j', name: 'kira', status: 'queued', mode: jobMode, progress: { step: 0, steps: 2000 } }))
      const w = mount(PeoplePage, { attachTo: document.body })
      await settle()
      await w.find('[data-testid="set-train"]').trigger('click')
      await settle()
      await w.find('[data-testid="people-name"]').setValue('kira')
      await w.find('[data-testid="people-train"]').trigger('click')
      await settle()
      expect(w.find('[data-testid="people-mode-ignored"]').exists()).toBe(warn)
      w.unmount()
    }
  })
})
