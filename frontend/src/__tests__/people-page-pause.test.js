import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PeoplePage from '../components/PeoplePage.vue'
import { TrainingStatus, ListTrainingJobs, ListPeople, CancelTraining, PauseTraining, ResumeTraining, RenamePerson, TrainingBases, TrainPerson, PickPeopleFolder } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)
afterEach(() => vi.unstubAllGlobals())

const RUNNING = { id: 't1', name: 'anna', status: 'running', progress: { step: 120, steps: 2000 } }
const PAUSED = { id: 't2', name: 'boris', status: 'paused', progress: { step: 750, steps: 2000 } }
const ANNA = { name: 'anna', trigger: 'anna_woman', gender: 'woman', base: 'Z-Image', lora: 'anna', steps: 2000 }

function button(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text() === text)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  ListPeople.mockImplementation(() => Promise.resolve([]))
  TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
})

describe('people page: pause and resume', () => {
  it('running job: Pause calls PauseTraining, Stop still calls CancelTraining', async () => {
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...RUNNING }]))
    PauseTraining.mockImplementation(() => Promise.resolve({ ...RUNNING, status: 'paused' }))
    CancelTraining.mockImplementation(() => Promise.resolve({ ...RUNNING, status: 'cancelled' }))
    const w = mount(PeoplePage)
    await settle()
    const job = w.find('[data-testid="people-job"]')
    const pause = button(job, t('people.pause'))
    expect(pause).toBeTruthy()
    await pause.trigger('click')
    await settle()
    expect(PauseTraining).toHaveBeenCalledWith('t1')
    expect(CancelTraining).not.toHaveBeenCalled()

    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...RUNNING }]))
    const w2 = mount(PeoplePage)
    await settle()
    await button(w2.find('[data-testid="people-job"]'), t('people.cancel')).trigger('click')
    await settle()
    expect(CancelTraining).toHaveBeenCalledWith('t1')
    expect(PauseTraining).toHaveBeenCalledTimes(1)
  })

  it('paused job: card with step text, Resume and Stop, no Pause', async () => {
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...PAUSED }]))
    ResumeTraining.mockImplementation(() => Promise.resolve({ ...PAUSED, status: 'queued' }))
    const w = mount(PeoplePage)
    await settle()
    const job = w.find('[data-testid="people-job"]')
    expect(job.exists()).toBe(true)
    expect(job.text()).toContain(t('people.status_paused', { step: 750, steps: 2000 }))
    expect(button(job, t('people.pause'))).toBeUndefined()
    expect(button(job, t('people.cancel'))).toBeTruthy()
    const resume = button(job, t('people.resume'))
    expect(resume).toBeTruthy()
    await resume.trigger('click')
    await settle()
    expect(ResumeTraining).toHaveBeenCalledWith('t2')
    expect(CancelTraining).not.toHaveBeenCalled()
  })

  it('paused job: Stop calls CancelTraining', async () => {
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...PAUSED }]))
    CancelTraining.mockImplementation(() => Promise.resolve({ ...PAUSED, status: 'cancelled' }))
    const w = mount(PeoplePage)
    await settle()
    await button(w.find('[data-testid="people-job"]'), t('people.cancel')).trigger('click')
    await settle()
    expect(CancelTraining).toHaveBeenCalledWith('t2')
    expect(ResumeTraining).not.toHaveBeenCalled()
  })

  it('Pause error is shown', async () => {
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...RUNNING }]))
    PauseTraining.mockImplementation(() => Promise.reject(new Error('job is not running')))
    const w = mount(PeoplePage)
    await settle()
    await button(w.find('[data-testid="people-job"]'), t('people.pause')).trigger('click')
    await settle()
    expect(w.find('[data-testid="people-error"]').text()).toContain('job is not running')
  })

  it('only paused jobs: no polling every 3 s', async () => {
    vi.useFakeTimers()
    ListTrainingJobs.mockImplementation(() => Promise.resolve([{ ...PAUSED }]))
    mount(PeoplePage)
    await vi.advanceTimersByTimeAsync(0)
    const calls = ListTrainingJobs.mock.calls.length
    expect(calls).toBeGreaterThan(0)
    await vi.advanceTimersByTimeAsync(7000)
    expect(ListTrainingJobs.mock.calls.length).toBe(calls)
    vi.useRealTimers()
  })
})

describe('people page: rename', () => {
  it('every person has a Rename button', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }, { ...ANNA, name: 'mila', lora: 'mila', trigger: 'mila_woman' }]))
    const w = mount(PeoplePage)
    await settle()
    const items = w.findAll('[data-testid="people-item"]')
    expect(items).toHaveLength(2)
    for (const item of items) expect(button(item, t('people.rename'))).toBeTruthy()
  })

  async function openRename(w) {
    await button(w.find('[data-testid="people-item"]'), t('people.rename')).trigger('click')
    await settle()
  }

  it('inline field with warning and current name; new name sent, list reloaded', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }]))
    RenamePerson.mockImplementation(() => Promise.resolve())
    const w = mount(PeoplePage)
    await settle()
    const before = ListPeople.mock.calls.length
    ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA, name: 'maria', lora: 'maria' }]))
    await openRename(w)
    const input = w.find('[data-testid="people-rename-input"]')
    expect(input.element.value).toBe('anna')
    expect(w.find('[data-testid="people-item"]').text()).toContain(t('people.rename_prompt', { name: 'anna' }))
    await input.setValue('maria')
    await w.find('[data-testid="people-rename-save"]').trigger('click')
    await settle()
    expect(RenamePerson).toHaveBeenCalledWith('anna', 'maria')
    expect(ListPeople.mock.calls.length).toBeGreaterThan(before)
    const items = w.findAll('[data-testid="people-item"]')
    expect(items).toHaveLength(1)
    expect(items[0].text()).toContain('maria')
    expect(w.find('[data-testid="people-rename-input"]').exists()).toBe(false)
  })

  it.each([
    ['cancel button', null],
    ['same name', 'anna'],
    ['empty name', '  '],
  ])('%s: RenamePerson not called', async (_, answer) => {
    ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }]))
    const w = mount(PeoplePage)
    await settle()
    await openRename(w)
    if (answer === null) {
      await button(w.find('[data-testid="people-item"]'), t('people.rename_cancel')).trigger('click')
    } else {
      await w.find('[data-testid="people-rename-input"]').setValue(answer)
      await w.find('[data-testid="people-rename-save"]').trigger('click')
    }
    await settle()
    expect(RenamePerson).not.toHaveBeenCalled()
    expect(w.find('[data-testid="people-rename-input"]').exists()).toBe(false)
  })

  it('invalid name: readable error, RenamePerson not called', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }]))
    const w = mount(PeoplePage)
    await settle()
    await openRename(w)
    await w.find('[data-testid="people-rename-input"]').setValue('bad!name')
    await w.find('[data-testid="people-rename-save"]').trigger('click')
    await settle()
    expect(RenamePerson).not.toHaveBeenCalled()
    expect(w.find('[data-testid="people-error"]').text()).toBe(t('people.err_invalid_request'))
  })

  it('server error code is shown as readable text', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }]))
    RenamePerson.mockImplementation(() => Promise.reject('training: name "maria" is already used (name_taken)'))
    const w = mount(PeoplePage)
    await settle()
    await openRename(w)
    await w.find('[data-testid="people-rename-input"]').setValue('maria')
    await w.find('[data-testid="people-rename-save"]').trigger('click')
    await settle()
    expect(RenamePerson).toHaveBeenCalledWith('anna', 'maria')
    expect(w.find('[data-testid="people-error"]').text()).toBe(t('people.err_name_taken'))
  })

  it('unknown error is shown as is', async () => {
    ListPeople.mockImplementation(() => Promise.resolve([{ ...ANNA }]))
    RenamePerson.mockImplementation(() => Promise.reject(new Error('connection refused')))
    const w = mount(PeoplePage)
    await settle()
    await openRename(w)
    await w.find('[data-testid="people-rename-input"]').setValue('maria')
    await w.find('[data-testid="people-rename-save"]').trigger('click')
    await settle()
    expect(w.find('[data-testid="people-error"]').text()).toContain('connection refused')
  })
})

describe('people i18n: pause and rename', () => {
  it.each(['people.pause', 'people.resume', 'people.status_paused', 'people.rename', 'people.rename_prompt'])('%s exists in ru and en', (key) => {
    expect(typeof ru[key], `ru ${key}`).toBe('string')
    expect(ru[key].trim()).not.toBe('')
    expect(typeof en[key], `en ${key}`).toBe('string')
    expect(en[key].trim()).not.toBe('')
  })

  it('status_paused shows step and steps', () => {
    for (const dict of [ru, en]) {
      expect(dict['people.status_paused']).toContain('{step}')
      expect(dict['people.status_paused']).toContain('{steps}')
    }
  })

  it('ru rename prompt has the name and warns about styles with the LoRA added manually', () => {
    expect(ru['people.rename_prompt']).toContain('{name}')
    expect(ru['people.rename_prompt']).toContain('вручную')
    expect(en['people.rename_prompt']).toContain('{name}')
  })
})

describe('people page: training errors', () => {
  it('invalid_request from training (bad photo) is shown as the server text, not the name hint', async () => {
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/anna'))
    TrainPerson.mockImplementation(() => Promise.reject('training: file is not a JPEG, PNG or WebP image (invalid_request)'))
    const w = mount(PeoplePage)
    await settle()
    await button(w, t('people.folder_pick')).trigger('click')
    await settle()
    await w.find('[data-testid="people-name"]').setValue('anna')
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    const text = w.find('[data-testid="people-error"]').text()
    expect(text).toContain('file is not a JPEG')
    expect(text).not.toBe(t('people.err_invalid_request'))
  })
})
