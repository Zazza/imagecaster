import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PeoplePage from '../components/PeoplePage.vue'
import { TrainingStatus, ListTrainingJobs, ListPeople, CancelTraining, TrainPerson, PickPeopleFolder, TrainingBases } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const RUNNING = { id: 't1', name: 'anna', status: 'running', progress: { step: 120, steps: 2000 } }

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  ListPeople.mockImplementation(() => Promise.resolve([]))
  TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
})

describe('people page', () => {
  it.each(['not_installed', 'no_server', 'unreachable'])('status %s: matching hint shown, training disabled', async (st) => {
    TrainingStatus.mockImplementation(() => Promise.resolve(st))
    const w = mount(PeoplePage)
    await settle()
    expect(w.find('[data-testid="people-unavailable"]').text()).toBe(t('people.unavailable_' + st))
    expect(w.find('[data-testid="people-train"]').attributes('disabled')).toBeDefined()
  })

  it('status ok: no hint', async () => {
    const w = mount(PeoplePage)
    await settle()
    expect(w.find('[data-testid="people-unavailable"]').exists()).toBe(false)
  })

  it('running job shows step N of M; Stop calls CancelTraining', async () => {
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...RUNNING }]))
    CancelTraining.mockImplementation(() => Promise.resolve({ ...RUNNING }))
    const w = mount(PeoplePage)
    await settle()
    const job = w.find('[data-testid="people-job"]')
    expect(job.text()).toContain(t('people.status_running', { step: 120, steps: 2000 }))
    await job.find('button').trigger('click')
    await settle()
    expect(CancelTraining).toHaveBeenCalledWith('t1')
  })

  it('train sends folder, name and gender', async () => {
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/anna'))
    TrainPerson.mockImplementation(() => Promise.resolve({ ...RUNNING, status: 'queued' }))
    const w = mount(PeoplePage)
    await settle()
    await w.findAll('button').find(b => b.text() === t('people.folder_pick')).trigger('click')
    await settle()
    await w.find('[data-testid="people-name"]').setValue('anna')
    await w.find('[data-testid="people-gender"]').setValue('woman')
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    expect(TrainPerson).toHaveBeenCalledWith('/photos/anna', 'anna', '', 'woman', 'Z-Image', false)
  })

  it('leaving the page while a poll is in flight stops polling', async () => {
    vi.useFakeTimers()
    let release
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...RUNNING }]))
    const w = mount(PeoplePage)
    await vi.advanceTimersByTimeAsync(0)
    ListTrainingJobs.mockImplementation(() => new Promise(r => { release = () => r([{ ...RUNNING }]) }))
    await vi.advanceTimersByTimeAsync(3000)
    const calls = ListTrainingJobs.mock.calls.length
    w.unmount()
    release()
    await vi.advanceTimersByTimeAsync(10000)
    expect(ListTrainingJobs.mock.calls.length).toBe(calls)
    vi.useRealTimers()
  })
})

async function fillAndTrain(w) {
  await w.findAll('button').find(b => b.text() === t('people.folder_pick')).trigger('click')
  await settle()
  await w.find('[data-testid="people-name"]').setValue('mila')
  await w.find('[data-testid="people-gender"]').setValue('woman')
}

describe('people page: training base', () => {
  beforeEach(() => {
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/mila'))
    TrainPerson.mockImplementation(() => Promise.resolve({ ...RUNNING, name: 'mila', status: 'queued' }))
  })

  it('base select lists bases from the server', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image', 'Moss-Image']))
    const w = mount(PeoplePage)
    await settle()
    expect(TrainingBases).toHaveBeenCalled()
    const sel = w.find('[data-testid="people-base"]')
    expect(sel.exists()).toBe(true)
    expect(sel.findAll('option').map(o => o.element.value)).toEqual(['Z-Image', 'Moss-Image'])
  })

  it('only Z-Image on the server: select has only Z-Image', async () => {
    const w = mount(PeoplePage)
    await settle()
    expect(w.find('[data-testid="people-base"]').findAll('option').map(o => o.element.value)).toEqual(['Z-Image'])
  })

  it('default base is the first one, Z-Image', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image', 'Moss-Image']))
    const w = mount(PeoplePage)
    await settle()
    expect(w.find('[data-testid="people-base"]').element.value).toBe('Z-Image')
    await fillAndTrain(w)
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    expect(TrainPerson).toHaveBeenCalledWith('/photos/mila', 'mila', '', 'woman', 'Z-Image', false)
  })

  it('picked Moss-Image goes as the 4th argument of TrainPerson', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image', 'Moss-Image']))
    const w = mount(PeoplePage)
    await settle()
    await fillAndTrain(w)
    await w.find('[data-testid="people-base"]').setValue('Moss-Image')
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    expect(TrainPerson).toHaveBeenCalledWith('/photos/mila', 'mila', '', 'woman', 'Moss-Image', false)
  })

  it('people list shows each person base', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([
      { name: 'anna', trigger: 'anna_woman', gender: 'woman', base: 'Z-Image', lora: 'anna', steps: 2000 },
      { name: 'mila', trigger: 'mila_woman', gender: 'woman', base: 'Moss-Image', lora: 'mila', steps: 1500 },
    ]))
    const w = mount(PeoplePage)
    await settle()
    const items = w.findAll('[data-testid="people-item"]')
    expect(items).toHaveLength(2)
    const anna = items.find(i => i.text().includes('anna'))
    const mila = items.find(i => i.text().includes('mila'))
    expect(anna.text()).toContain('Z-Image')
    expect(anna.text()).not.toContain('Moss-Image')
    expect(mila.text()).toContain('Moss-Image')
    expect(mila.text()).not.toContain('Z-Image')
  })

  it('person without base is shown as Z-Image', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([
      { name: 'old', trigger: 'old_man', gender: 'man', lora: 'old', steps: 2000 },
    ]))
    const w = mount(PeoplePage)
    await settle()
    expect(w.find('[data-testid="people-item"]').text()).toContain('Z-Image')
  })
})
