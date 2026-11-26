import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import SettingsPage from '../components/SettingsPage.vue'
import { GetSettings, UpdateSettings } from '../wailsjs/go/main/App.js'
import { clearEventMocks } from './mocks/runtime'
import { setLocale } from '../i18n/index.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetSettings.mockResolvedValue({})
  UpdateSettings.mockResolvedValue(undefined)
})
afterEach(() => { vi.unstubAllGlobals() })
async function openAdvanced(settings = {}) {
  GetSettings.mockResolvedValue(settings)
  const wrapper = mount(SettingsPage, { props: { initialTab: 'advanced' } })
  await settle()
  return wrapper
}
const checkbox = wrapper => wrapper.get('[data-testid="settings-live-preview"] input')
describe('common generation process preference', () => {
  it('defaults to enabled and loads saved false without overwriting it', async () => {
    const defaults = await openAdvanced()
    expect(checkbox(defaults).element.checked).toBe(true)
    defaults.unmount()
    const saved = await openAdvanced({ generation_live_preview: 'false' })
    expect(checkbox(saved).element.checked).toBe(false)
    expect(UpdateSettings).not.toHaveBeenCalled()
  })
  it('saves disabling as a persisted boolean string', async () => {
    const wrapper = await openAdvanced({ generation_live_preview: 'true' })
    await checkbox(wrapper).setValue(false)
    await settle()
    expect(UpdateSettings).toHaveBeenCalledExactlyOnceWith({ generation_live_preview: 'false' })
    expect(checkbox(wrapper).element.checked).toBe(false)
  })
  it('keeps the prior enabled value and displays save errors', async () => {
    const wrapper = await openAdvanced({ generation_live_preview: 'true' })
    UpdateSettings.mockRejectedValueOnce(new Error('settings write failed'))
    await checkbox(wrapper).setValue(false)
    await settle()
    expect(wrapper.text()).toContain('settings write failed')
    expect(checkbox(wrapper).element.checked).toBe(true)
  })
  it('disables the control while save is pending and re-enables after success', async () => {
    const wrapper = await openAdvanced()
    let finish
    UpdateSettings.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    await checkbox(wrapper).setValue(false)
    await settle()
    expect(checkbox(wrapper).element.disabled).toBe(true)
    finish()
    await settle()
    expect(checkbox(wrapper).element.disabled).toBe(false)
    expect(checkbox(wrapper).element.checked).toBe(false)
  })
})
