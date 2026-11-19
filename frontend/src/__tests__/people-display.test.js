import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PeoplePage from '../components/PeoplePage.vue'
import { TrainingStatus, ListTrainingJobs, ListPeople, TrainPerson, PickPeopleFolder, TrainingBases, SetPersonDisplay } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`
const NATASHA = { name: 'natasha_curated', trigger: 'natasha_woman', gender: 'woman', base: 'Z-Image', lora: 'natasha_curated', steps: 2000, display_name: 'Наташа' }
const ANNA = { name: 'anna', trigger: 'anna_woman', gender: 'woman', base: 'Z-Image', lora: 'anna', steps: 2000 }

beforeEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  ListPeople.mockImplementation(() => Promise.resolve([{ ...NATASHA }, { ...ANNA }]))
  TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
  SetPersonDisplay.mockImplementation((name, display) => Promise.resolve({ ...ANNA, display_name: display }))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function row(w, text) {
  return w.findAll(tid('people-item')).find(i => i.text().includes(text))
}

async function editName(w, rowText, value) {
  const r = row(w, rowText)
  await r.find(tid('people-display-edit')).trigger('click')
  await settle()
  await row(w, rowText).find(tid('people-display-input')).setValue(value)
  await row(w, rowText).find(tid('people-display-save')).trigger('click')
  await settle()
}

describe('people page: Name', () => {
  it('Name typed in the train form goes as the 3rd argument of TrainPerson', async () => {
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/nat'))
    TrainPerson.mockImplementation(() => Promise.resolve({ id: 't1', name: 'natasha_flux1', status: 'queued' }))
    const w = mount(PeoplePage)
    await settle()
    await w.findAll('button').find(b => b.text() === t('people.folder_pick')).trigger('click')
    await settle()
    await w.find(tid('people-name')).setValue('natasha_flux1')
    await w.find(tid('people-display')).setValue('Наташа')
    await w.find(tid('people-gender')).setValue('woman')
    await w.find(tid('people-train')).trigger('click')
    await settle()
    expect(TrainPerson).toHaveBeenCalledTimes(1)
    expect(TrainPerson.mock.calls[0][0]).toBe('/photos/nat')
    expect(TrainPerson.mock.calls[0][1]).toBe('natasha_flux1')
    expect(TrainPerson.mock.calls[0][2]).toBe('Наташа')
  })

  it('row shows display_name', async () => {
    const w = mount(PeoplePage)
    await settle()
    const r = row(w, 'natasha_curated')
    expect(r.find(tid('people-display-name')).text()).toContain('Наташа')
  })

  it('editing Name calls SetPersonDisplay(lora, name, false)', async () => {
    const w = mount(PeoplePage)
    await settle()
    await editName(w, 'anna', 'Аня')
    expect(SetPersonDisplay).toHaveBeenCalledTimes(1)
    expect(SetPersonDisplay).toHaveBeenCalledWith('anna', 'Аня', false)
  })

  it('display_taken: confirm yes → retried with replace=true', async () => {
    const confirm = vi.fn(() => true)
    vi.stubGlobal('confirm', confirm)
    SetPersonDisplay.mockImplementation((name, display, replace) => replace
      ? Promise.resolve({ ...ANNA, display_name: display })
      : Promise.reject(new Error('Имя уже у другой LoRA этой базы (display_taken)')))
    const w = mount(PeoplePage)
    await settle()
    await editName(w, 'anna', 'Наташа')
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(SetPersonDisplay).toHaveBeenCalledTimes(2)
    expect(SetPersonDisplay).toHaveBeenNthCalledWith(1, 'anna', 'Наташа', false)
    expect(SetPersonDisplay).toHaveBeenNthCalledWith(2, 'anna', 'Наташа', true)
  })

  it('display_taken: confirm no → no replace call', async () => {
    const confirm = vi.fn(() => false)
    vi.stubGlobal('confirm', confirm)
    SetPersonDisplay.mockImplementation(() => Promise.reject(new Error('Имя уже у другой LoRA этой базы (display_taken)')))
    const w = mount(PeoplePage)
    await settle()
    await editName(w, 'anna', 'Наташа')
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(SetPersonDisplay).toHaveBeenCalledTimes(1)
    expect(SetPersonDisplay).not.toHaveBeenCalledWith('anna', 'Наташа', true)
  })

  it('other server error: no confirm, no replace', async () => {
    const confirm = vi.fn(() => true)
    vi.stubGlobal('confirm', confirm)
    SetPersonDisplay.mockImplementation(() => Promise.reject(new Error('слишком длинное имя (display_invalid)')))
    const w = mount(PeoplePage)
    await settle()
    await editName(w, 'anna', 'Наташа')
    expect(confirm).not.toHaveBeenCalled()
    expect(SetPersonDisplay).toHaveBeenCalledTimes(1)
  })

  it('empty Name clears it: SetPersonDisplay with an empty string', async () => {
    SetPersonDisplay.mockImplementation(() => Promise.resolve({ ...NATASHA, display_name: '' }))
    const w = mount(PeoplePage)
    await settle()
    await editName(w, 'natasha_curated', '')
    expect(SetPersonDisplay).toHaveBeenCalledTimes(1)
    expect(SetPersonDisplay.mock.calls[0][0]).toBe('natasha_curated')
    expect(SetPersonDisplay.mock.calls[0][1]).toBe('')
  })
})
