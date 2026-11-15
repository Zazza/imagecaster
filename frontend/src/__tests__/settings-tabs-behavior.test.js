import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import SettingsPage from '../components/SettingsPage.vue'
import PinModal from '../components/PinModal.vue'
import {
  GetSettings, UpdateSettings, GetServerStatus, IsKidsModeActive, GetKidsCategories, SetKidsMode, SetKidsCategory,
  DiscoverServers, GetServerModels, GetServerLLMModels, StartServerProcess,
} from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const CATS = [
  { name: 'violence', label: 'Violence', enabled: true, alwaysOn: false },
  { name: 'nsfw', label: 'NSFW', enabled: true, alwaysOn: true },
]

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetSettings.mockImplementation(() => Promise.resolve({}))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetServerStatus.mockImplementation(() => Promise.resolve({ processes: {}, health: {}, models: {} }))
  IsKidsModeActive.mockImplementation(() => Promise.resolve(false))
  GetKidsCategories.mockImplementation(() => Promise.resolve(CATS.map(c => ({ ...c }))))
  SetKidsMode.mockImplementation(() => Promise.resolve(undefined))
  SetKidsCategory.mockImplementation(() => Promise.resolve(undefined))
  GetServerModels.mockImplementation(() => Promise.resolve([]))
  GetServerLLMModels.mockImplementation(() => Promise.resolve([]))
})

afterEach(() => vi.unstubAllGlobals())

async function mountTab(tab) {
  const w = mount(SettingsPage, { attachTo: document.body, props: { initialTab: tab } })
  await settle()
  return w
}

describe('settings: initial tab and switching', () => {
  it('initialTab prop picks the tab and later prop changes switch it', async () => {
    const w = await mountTab('safety')
    expect(w.find('.tabs .tab.active').text()).toBe(t('settings.tab_safety'))
    expect(w.text()).toContain(t('settings.section_kids'))
    await w.setProps({ initialTab: 'advanced' })
    await settle()
    expect(w.find('.tabs .tab.active').text()).toBe(t('settings.tab_advanced'))
    expect(w.text()).toContain(t('settings.section_memory_guard'))
    expect(w.text()).not.toContain(t('settings.section_kids'))
  })

  it('unsaved edits survive switching tabs', async () => {
    const w = await mountTab('prompts')
    const ta = w.find('textarea')
    await ta.setValue('edited and not saved')
    await w.findAll('.tabs .tab').find(b => b.text() === t('settings.tab_safety')).trigger('click')
    await settle()
    await w.findAll('.tabs .tab').find(b => b.text() === t('settings.tab_prompts')).trigger('click')
    await settle()
    expect(w.findAll('textarea').map(x => x.element.value)).toContain('edited and not saved')
  })
})

describe('settings: safety tab', () => {
  it('turning kids mode on asks for a new PIN and enables it with that PIN', async () => {
    const w = await mountTab('safety')
    expect(w.findComponent(PinModal).exists()).toBe(false)
    await w.find('input[type="checkbox"]').setValue(true)
    const modal = w.findComponent(PinModal)
    expect(modal.props('mode')).toBe('set')
    modal.vm.$emit('confirm', '1234')
    await settle()
    expect(SetKidsMode).toHaveBeenCalledWith(true, '1234')
    expect(w.findComponent(PinModal).exists()).toBe(false)
    expect(w.text()).toContain(t('settings.enabled'))
    expect(w.text()).toContain('Violence')
  })

  it('turning kids mode off verifies the PIN; a wrong PIN shows the error and keeps it on', async () => {
    IsKidsModeActive.mockImplementation(() => Promise.resolve(true))
    SetKidsMode.mockImplementation(() => Promise.reject(new Error('wrong pin')))
    const w = await mountTab('safety')
    await w.find('input[type="checkbox"]').setValue(false)
    const modal = w.findComponent(PinModal)
    expect(modal.props('mode')).toBe('verify')
    modal.vm.$emit('confirm', '0000')
    await settle()
    expect(SetKidsMode).toHaveBeenCalledWith(false, '0000')
    expect(w.findComponent(PinModal).props('error')).toContain('wrong pin')
    w.findComponent(PinModal).vm.$emit('cancel')
    await settle()
    expect(w.findComponent(PinModal).exists()).toBe(false)
    expect(w.text()).toContain(t('settings.enabled'))
  })

  it('category toggle asks the PIN and applies the flipped value', async () => {
    IsKidsModeActive.mockImplementation(() => Promise.resolve(true))
    const w = await mountTab('safety')
    const toggles = w.findAll('input[type="checkbox"]')
    expect(toggles).toHaveLength(3)
    expect(toggles[2].element.disabled).toBe(true)
    await toggles[1].setValue(false)
    const modal = w.findComponent(PinModal)
    expect(modal.props('mode')).toBe('verify')
    modal.vm.$emit('confirm', '4321')
    await settle()
    expect(SetKidsCategory).toHaveBeenCalledWith('violence', false, '4321')
    expect(w.findComponent(PinModal).exists()).toBe(false)
    expect(w.findAll('input[type="checkbox"]')[1].element.checked).toBe(false)
  })
})

describe('settings: advanced tab models', () => {
  it('without a server address the model lists are replaced by a hint', async () => {
    const w = await mountTab('advanced')
    expect(w.text()).toContain(t('settings.server_not_configured'))
    expect(w.text()).not.toContain(t('settings.title_sd_models'))
  })

  it('with a server address lists SD models with sizes, LoRA and LLM models', async () => {
    GetSettings.mockImplementation(() => Promise.resolve({ server_url: 'http://127.0.0.1:18080' }))
    GetServerModels.mockImplementation((type) => Promise.resolve(
      type === 'sd' ? [{ name: 'model-a.safetensors', size: 1536 }] : type === 'lora' ? [{ name: 'lora-b' }] : [],
    ))
    GetServerLLMModels.mockImplementation(() => Promise.resolve([{ name: 'text-c:8b' }]))
    const w = await mountTab('advanced')
    expect(w.text()).not.toContain(t('settings.server_not_configured'))
    expect(w.text()).toContain('model-a.safetensors')
    expect(w.text()).toContain('1.5 KB')
    expect(w.text()).toContain('lora-b')
    expect(w.text()).toContain('text-c:8b')
  })
})

describe('settings: server discovery', () => {
  it('picking a discovered server saves its address and refreshes the status', async () => {
    DiscoverServers.mockImplementation(() => Promise.resolve([{ name: 'Box', ip_address: '10.0.0.5', port: 8080 }]))
    const w = await mountTab('connection')
    await w.findAll('button').find(b => b.text() === t('settings.btn_discover')).trigger('click')
    await settle()
    expect(w.text()).toContain('10.0.0.5:8080')
    const before = GetServerStatus.mock.calls.length
    await w.findAll('div').find(d => d.text() === 'Box').trigger('click')
    await settle()
    expect(UpdateSettings).toHaveBeenCalledWith({ server_url: 'http://10.0.0.5:8080' })
    expect(GetServerStatus.mock.calls.length).toBeGreaterThan(before)
    expect(w.find('input.form-input').element.value).toBe('http://10.0.0.5:8080')
    expect(w.text()).toContain(t('settings.section_server_status'))
  })

  it('saving the address fails: the error is shown', async () => {
    DiscoverServers.mockImplementation(() => Promise.resolve([{ name: 'Box', ip_address: '10.0.0.5', port: 8080 }]))
    UpdateSettings.mockImplementation(() => Promise.reject(new Error('disk full')))
    const w = await mountTab('connection')
    await w.findAll('button').find(b => b.text() === t('settings.btn_discover')).trigger('click')
    await settle()
    await w.findAll('div').find(d => d.text() === 'Box').trigger('click')
    await settle()
    expect(w.text()).toContain('disk full')
  })
})

describe('settings: process buttons', () => {
  it('a pending start keeps the button disabled after switching tabs and back', async () => {
    GetSettings.mockImplementation(() => Promise.resolve({ server_url: 'http://srv:8080' }))
    GetServerStatus.mockImplementation(() => Promise.resolve({ processes: { sd: { name: 'SD', status: 'stopped' } }, health: {}, models: {} }))
    let finish
    StartServerProcess.mockImplementation(() => new Promise(r => { finish = r }))
    const w = await mountTab('connection')
    await w.find('[data-testid="process-start"]').trigger('click')
    await settle()
    expect(w.find('[data-testid="process-start"]').attributes('disabled')).toBeDefined()
    await w.findAll('.tabs .tab').find(b => b.text() === t('settings.tab_safety')).trigger('click')
    await settle()
    await w.findAll('.tabs .tab').find(b => b.text() === t('settings.tab_connection')).trigger('click')
    await settle()
    expect(w.find('[data-testid="process-start"]').attributes('disabled')).toBeDefined()
    finish()
    await settle()
    expect(w.find('[data-testid="process-start"]').attributes('disabled')).toBeUndefined()
  })
})
