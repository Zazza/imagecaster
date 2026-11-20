import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'
import {
  StylistPresets, StylistPlan, StylistShoot, StylistAppearance, ListPeople, FaceSwapStatus, GetQueue,
  GetSessionThumb, StylistFrames, StylistLastShoot,
} from '../wailsjs/go/main/App.js'
import { setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { resetStylistPlan } from '../composables/useStylistPlan.js'
import { failedTrainings } from '../people.js'
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

let deleted
const done = (ids) => Promise.resolve(ids.map(id => ({ job_id: id, status: deleted.has(id) ? 'deleted' : 'completed', item_id: id + 100, error: '', shot: 'Анфас' })))

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  deleted = new Set()
  StylistPresets.mockImplementation(() => Promise.resolve(structuredClone(BASE)))
  ListPeople.mockImplementation(() => Promise.resolve([{ name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman' }]))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  StylistPlan.mockImplementation(() => Promise.resolve({ series: SERIES(), warnings: [], used_llm: true }))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  GetSessionThumb.mockImplementation(() => Promise.resolve('/9j/abc'))
  StylistFrames.mockImplementation(done)
  StylistLastShoot.mockImplementation(() => Promise.resolve([]))
})

const tiles = (w) => w.findAll(tid('stylist-frame'))

async function shoot(w, ids) {
  StylistShoot.mockImplementation(() => Promise.resolve(ids))
  await w.find(tid('stylist-plan')).trigger('click')
  await settle()
  await w.find(tid('stylist-shoot')).trigger('click')
  await settle()
}

describe('Stylist «Готовые кадры»: only the last shoot', () => {
  it('a new «Сгенерировать» replaces the strip', async () => {
    const w = mount(StylistPage)
    await settle()
    await shoot(w, [1, 2, 3])
    expect(tiles(w)).toHaveLength(3)
    await shoot(w, [4, 5])
    expect(tiles(w)).toHaveLength(2)
    expect(StylistFrames).toHaveBeenLastCalledWith([4, 5])
  })

  it('a frame deleted in History disappears; a moved one stays', async () => {
    const w = mount(StylistPage)
    await settle()
    await shoot(w, [1, 2, 3])
    StylistFrames.mockClear()
    EventsEmit('session:removed', { ids: [102], moved_to: 9 })
    await settle()
    expect(StylistFrames).not.toHaveBeenCalled()
    deleted.add(2)
    EventsEmit('session:removed', { id: 102 })
    await settle()
    expect(tiles(w)).toHaveLength(2)
    expect(StylistFrames).toHaveBeenLastCalledWith([1, 2, 3])
    EventsEmit('queue:changed')
    await settle()
    expect(StylistFrames).toHaveBeenLastCalledWith([1, 3])
    expect(tiles(w)).toHaveLength(2)
  })

  it.each(['session:deleted', 'session:cleared'])('%s re-checks the frames', async (ev) => {
    const w = mount(StylistPage)
    await settle()
    await shoot(w, [1, 2])
    deleted = new Set([1, 2])
    EventsEmit(ev, { session_id: 3 })
    await settle()
    expect(tiles(w)).toHaveLength(0)
  })

  it('restores the last shoot after the app restarts', async () => {
    StylistLastShoot.mockImplementation(() => Promise.resolve([7, 8]))
    const w = mount(StylistPage)
    await settle()
    expect(StylistFrames).toHaveBeenCalledWith([7, 8])
    expect(w.find(tid('stylist-view-frames')).attributes('aria-pressed')).toBe('true')
    expect(tiles(w)).toHaveLength(2)
    expect(w.find(tid('stylist-results')).findAll('img')).toHaveLength(2)
  })

  it('a failed restore is retried on the next visit', async () => {
    StylistLastShoot.mockImplementationOnce(() => Promise.reject(new Error('busy')))
    const first = mount(StylistPage)
    await settle()
    expect(tiles(first)).toHaveLength(0)
    first.unmount()
    StylistLastShoot.mockImplementation(() => Promise.resolve([7]))
    const w = mount(StylistPage)
    await settle()
    expect(tiles(w)).toHaveLength(1)
  })

  it('does not restore over the current page state', async () => {
    const w = mount(StylistPage)
    await settle()
    await shoot(w, [1])
    w.unmount()
    StylistLastShoot.mockImplementation(() => Promise.resolve([7, 8]))
    const again = mount(StylistPage)
    await settle()
    expect(StylistLastShoot).toHaveBeenCalledTimes(1)
    expect(tiles(again)).toHaveLength(1)
  })
})

describe('People «Неудачные обучения»', () => {
  const now = Date.parse('2026-11-20T12:00:00Z')
  const job = (name, status, finished) => ({ id: name + status + finished, name, status, finished_at: finished })

  it('shows only fresh unsuccessful trainings, newest first', () => {
    const got = failedTrainings([
      job('anna', 'completed', '2026-11-19T10:00:00Z'),
      job('bob', 'failed', '2026-11-19T10:00:00Z'),
      job('eve', 'cancelled', '2026-11-20T09:00:00Z'),
      job('max', 'lost', '2026-11-18T09:00:00Z'),
      job('old', 'failed', '2026-11-12T09:00:00Z'),
    ], now)
    expect(got.map(j => j.name)).toEqual(['eve', 'bob', 'max'])
  })

  it('hides a failure once the same person trained successfully later', () => {
    const got = failedTrainings([
      job('anna', 'failed', '2026-11-19T10:00:00Z'),
      job('anna', 'completed', '2026-11-19T12:00:00Z'),
      job('bob', 'completed', '2026-11-18T12:00:00Z'),
      job('bob', 'failed', '2026-11-19T12:00:00Z'),
    ], now)
    expect(got.map(j => j.name)).toEqual(['bob'])
  })

  it('handles empty input and keeps at most five', () => {
    expect(failedTrainings(null, now)).toEqual([])
    const many = Array.from({ length: 8 }, (_, i) => job('p' + i, 'failed', `2026-11-20T0${i}:00:00Z`))
    expect(failedTrainings(many, now)).toHaveLength(5)
  })
})
