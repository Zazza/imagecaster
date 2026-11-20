import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import { StylistPresets, ListPeople, FaceSwapStatus, StylistAppearance, GetQueue, StylistFrames } from '../wailsjs/go/main/App.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { loadPeople } from '../peopleStore.js'
import { resetStylistPlan } from '../composables/useStylistPlan.js'
import StylistPage from '../components/StylistPage.vue'

enableAutoUnmount(afterEach)

const tid = (id) => `[data-testid="${id}"]`

async function usePeople(list) {
  ListPeople.mockImplementation(() => Promise.resolve(list.map(p => ({ ...p }))))
  await loadPeople()
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  resetStylistPlan()
  StylistPresets.mockImplementation(() => Promise.resolve({ version: 1, themes: [], outfit: [], hair: [], makeup: [], background: [], pose: [] }))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  StylistAppearance.mockImplementation(() => Promise.resolve(''))
  GetQueue.mockImplementation(() => Promise.resolve([]))
  StylistFrames.mockImplementation(() => Promise.resolve([]))
})

describe('Stylist: link to «Люди»', () => {
  it('no person with a LoRA → warning and a button to «Люди», no person select', async () => {
    await usePeople([{ name: 'face_only', base: 'Z-Image', lora: '', identity: true }])
    const w = mount(StylistPage)
    await settle()
    const box = w.find(tid('stylist-no-people'))
    expect(box.classes()).toContain('status-warning')
    expect(box.text()).toContain(t('stylist.no_people'))
    expect(w.find(tid('stylist-person')).exists()).toBe(false)
    expect(w.find(tid('stylist-add-person')).exists()).toBe(false)
    await w.find(tid('stylist-no-people-go')).trigger('click')
    expect(w.emitted('navigate')).toEqual([[{ page: 'people' }]])
  })

  it('with people → «Добавить человека» next to the select leads to «Люди»', async () => {
    await usePeople([{ name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman' }])
    const w = mount(StylistPage)
    await settle()
    expect(w.find(tid('stylist-no-people')).exists()).toBe(false)
    expect(w.find(tid('stylist-person')).exists()).toBe(true)
    const add = w.find(tid('stylist-add-person'))
    expect(add.text()).toBe(t('stylist.add_person'))
    await add.trigger('click')
    expect(w.emitted('navigate')).toEqual([[{ page: 'people' }]])
  })
})
