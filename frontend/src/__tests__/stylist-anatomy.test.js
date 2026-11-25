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

describe('Stylist anatomy warnings', () => {
  it.each(['ru', 'en'])('restores warnings after restart in %s', async (locale) => {
    setLocale(locale)
    StylistLastShoot.mockImplementation(() => Promise.resolve([7, 8]))
    StylistFrames.mockImplementation(() => Promise.resolve([
      { job_id: 7, status: 'completed', item_id: 107, shot: 'Front', anatomy_warning: 'base_defect' },
      { job_id: 8, status: 'completed', item_id: 108, shot: 'Side' },
    ]))
    const w = mount(StylistPage)
    await settle()
    expect(w.findAll(tid('stylist-anatomy-badge'))).toHaveLength(1)
    await tiles(w)[0].trigger('click')
    await settle()
    const warning = w.find(tid('stylist-anatomy-warning'))
    expect(warning.exists()).toBe(true)
    expect(warning.text().length).toBeGreaterThan(10)
    expect(warning.text()).not.toContain('base_defect')
    expect(warning.text()).not.toContain('http')
    await tiles(w)[1].trigger('click')
    await settle()
    expect(w.find(tid('stylist-anatomy-warning')).exists()).toBe(false)
  })
})
