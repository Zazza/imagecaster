import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PeoplePage from '../components/PeoplePage.vue'
import { TrainingStatus, ListTrainingJobs, ListPeople, ResumeTraining, TrainingBases, TrainPerson, PickPeopleFolder } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)
afterEach(() => vi.unstubAllGlobals())

const NOT_STARTED = { id: 'h1', name: 'kira', base: 'Z-Image', status: 'paused', progress: { step: 0, steps: 2000 }, started_at: null }
const PAUSED_STARTED = { id: 'h2', name: 'boris', base: 'Z-Image', status: 'paused', progress: { step: 750, steps: 2000 }, started_at: '2026-11-19T10:00:00Z' }

function button(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text() === text)
}

async function fillForm(w) {
  await button(w, t('people.folder_pick')).trigger('click')
  await settle()
  await w.find('[data-testid="people-name"]').setValue('kira')
  await w.find('[data-testid="people-gender"]').setValue('woman')
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  ListPeople.mockImplementation(() => Promise.resolve([]))
  TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
  PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/kira'))
})

describe('people page: create training without starting', () => {
  it('hold checkbox exists and is off by default', async () => {
    const w = mount(PeoplePage)
    await settle()
    const hold = w.find('[data-testid="people-hold"]')
    expect(hold.exists()).toBe(true)
    expect(hold.element.checked).toBe(false)
  })

  it('checkbox off: TrainPerson gets hold=false', async () => {
    TrainPerson.mockImplementation(() => Promise.resolve({ ...NOT_STARTED, status: 'queued' }))
    const w = mount(PeoplePage)
    await settle()
    await fillForm(w)
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    expect(TrainPerson).toHaveBeenCalledWith('/photos/kira', 'kira', 'woman', 'Z-Image', false)
    expect(w.find('[data-testid="people-hold-ignored"]').exists()).toBe(false)
  })

  it('checkbox on: TrainPerson gets hold=true, paused answer shows no warning', async () => {
    TrainPerson.mockImplementation(() => Promise.resolve({ ...NOT_STARTED }))
    const w = mount(PeoplePage)
    await settle()
    await fillForm(w)
    await w.find('[data-testid="people-hold"]').setValue(true)
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    expect(TrainPerson).toHaveBeenCalledWith('/photos/kira', 'kira', 'woman', 'Z-Image', true)
    expect(w.find('[data-testid="people-hold-ignored"]').exists()).toBe(false)
  })

  it('checkbox on but old server returned queued: warning is shown', async () => {
    TrainPerson.mockImplementation(() => Promise.resolve({ ...NOT_STARTED, status: 'queued' }))
    const w = mount(PeoplePage)
    await settle()
    await fillForm(w)
    await w.find('[data-testid="people-hold"]').setValue(true)
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    const warn = w.find('[data-testid="people-hold-ignored"]')
    expect(warn.exists()).toBe(true)
    expect(warn.isVisible()).toBe(true)
    expect(warn.text()).toContain(t('people.hold_ignored'))
  })

  it('paused job without started_at: "not started" text, Start button and hint', async () => {
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...NOT_STARTED }]))
    ResumeTraining.mockImplementation(() => Promise.resolve({ ...NOT_STARTED, status: 'queued' }))
    const w = mount(PeoplePage)
    await settle()
    const job = w.find('[data-testid="people-job"]')
    expect(job.exists()).toBe(true)
    expect(job.text()).toContain(t('people.status_not_started'))
    expect(job.text()).toContain(t('people.not_started_hint'))
    expect(button(job, t('people.resume'))).toBeUndefined()
    expect(job.text()).not.toContain(t('people.status_paused', { step: 0, steps: 2000 }))
    const start = button(job, t('people.start'))
    expect(start).toBeTruthy()
    await start.trigger('click')
    await settle()
    expect(ResumeTraining).toHaveBeenCalledWith('h1')
  })

  it('paused job with started_at: old paused text and Resume button', async () => {
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...PAUSED_STARTED }]))
    const w = mount(PeoplePage)
    await settle()
    const job = w.find('[data-testid="people-job"]')
    expect(job.text()).toContain(t('people.status_paused', { step: 750, steps: 2000 }))
    expect(button(job, t('people.resume'))).toBeTruthy()
    expect(button(job, t('people.start'))).toBeUndefined()
    expect(job.text()).not.toContain(t('people.status_not_started'))
  })

  it('only paused jobs (started and not): no polling every 3 s', async () => {
    vi.useFakeTimers()
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...NOT_STARTED }, { ...PAUSED_STARTED }]))
    mount(PeoplePage)
    await vi.advanceTimersByTimeAsync(0)
    const calls = ListTrainingJobs.mock.calls.length
    expect(calls).toBeGreaterThan(0)
    await vi.advanceTimersByTimeAsync(7000)
    expect(ListTrainingJobs.mock.calls.length).toBe(calls)
    vi.useRealTimers()
  })
})

describe('people i18n: hold', () => {
  it.each(['people.status_not_started', 'people.start', 'people.not_started_hint', 'people.hold_ignored'])('%s exists in ru and en', (key) => {
    expect(typeof ru[key], `ru ${key}`).toBe('string')
    expect(ru[key].trim()).not.toBe('')
    expect(typeof en[key], `en ${key}`).toBe('string')
    expect(en[key].trim()).not.toBe('')
  })

  it('not started hint does not mention the last save', () => {
    expect(ru['people.not_started_hint'].toLowerCase()).not.toMatch(/сохран/)
    expect(en['people.not_started_hint'].toLowerCase()).not.toMatch(/sav/)
  })
})
