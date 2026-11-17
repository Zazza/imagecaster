import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PeoplePage from '../components/PeoplePage.vue'
import { TrainingStatus, ListTrainingJobs, ListPeople, CancelTraining, TrainPerson, PickPeopleFolder } from '../wailsjs/go/main/App.js'
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
    expect(TrainPerson).toHaveBeenCalledWith('/photos/anna', 'anna', 'woman')
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
