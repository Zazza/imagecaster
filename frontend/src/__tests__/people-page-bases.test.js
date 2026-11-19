import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PeoplePage from '../components/PeoplePage.vue'
import { TrainingStatus, ListTrainingJobs, ListPeople, TrainPerson, PickPeopleFolder, TrainingBases } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  ListPeople.mockImplementation(() => Promise.resolve([]))
  PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/kira'))
  TrainPerson.mockImplementation(() => Promise.resolve({ id: 't1', name: 'kira', status: 'queued' }))
})

const options = (w) => w.find('[data-testid="people-base"]').findAll('option').map(o => ({ value: o.element.value, text: o.text().trim() }))

describe('people page: SDXL and Flux.2 bases', () => {
  it('all four bases: values are base names, Flux.2 labelled Flux.2 Klein 4B', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image', 'Moss-Image', 'SDXL', 'Flux.2']))
    const w = mount(PeoplePage)
    await settle()
    expect(options(w)).toEqual([
      { value: 'Z-Image', text: 'Z-Image' },
      { value: 'Moss-Image', text: 'Moss-Image' },
      { value: 'SDXL', text: 'SDXL' },
      { value: 'Flux.2', text: 'Flux.2 Klein 4B' },
    ])
  })

  it('picked Flux.2 goes to TrainPerson as Flux.2', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve(['SDXL', 'Flux.2']))
    const w = mount(PeoplePage)
    await settle()
    await w.findAll('button').find(b => b.text() === t('people.folder_pick')).trigger('click')
    await settle()
    await w.find('[data-testid="people-name"]').setValue('kira')
    await w.find('[data-testid="people-gender"]').setValue('woman')
    await w.find('[data-testid="people-base"]').setValue('Flux.2')
    await w.find('[data-testid="people-train"]').trigger('click')
    await settle()
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    expect(TrainPerson).toHaveBeenCalledWith('/photos/kira', 'kira', 'woman', 'Flux.2', false)
  })

  it('only SDXL on the server: default base is SDXL', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve(['SDXL']))
    const w = mount(PeoplePage)
    await settle()
    expect(w.find('[data-testid="people-base"]').element.value).toBe('SDXL')
  })

  it('server has no bases: hint about the catalog, training disabled', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve([]))
    const w = mount(PeoplePage)
    await settle()
    const hint = w.find('[data-testid="people-no-bases"]')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toBe(t('people.no_bases'))
    expect(t('people.no_bases')).not.toBe('people.no_bases')
    await w.findAll('button').find(b => b.text() === t('people.folder_pick')).trigger('click')
    await settle()
    await w.find('[data-testid="people-name"]').setValue('kira')
    await w.find('[data-testid="people-gender"]').setValue('woman')
    await settle()
    expect(w.find('[data-testid="people-train"]').attributes('disabled')).toBeDefined()
  })

  it('bases present: no hint', async () => {
    TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
    const w = mount(PeoplePage)
    await settle()
    expect(w.find('[data-testid="people-no-bases"]').exists()).toBe(false)
  })
})
