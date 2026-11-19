import { describe, it, expect, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'

import App from '../App.vue'
import { t, setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

describe('sidebar works when collapsed to icons', () => {
  it('every nav link has an icon, a label span and a matching tooltip', async () => {
    setLocale('ru')
    localStorage.setItem('imagecaster_welcome', JSON.stringify({ dismissed: true, seenVersion: '9.9.9' }))
    const w = mount(App, { shallow: false, global: { stubs: { SettingsPage: true, UnifiedGeneratePage: true, AppFooter: true, WelcomeModal: true } } })
    await flushPromises()
    const links = w.findAll('.sidebar-link')
    expect(links).toHaveLength(9)
    const keys = ['app.nav_generate', 'app.nav_stylist', 'app.nav_postprocess', 'app.nav_gallery', 'app.nav_my_images', 'app.nav_styles', 'app.nav_catalog', 'app.nav_people', 'app.nav_settings']
    links.forEach((l, i) => {
      expect(l.find('svg').exists()).toBe(true)
      expect(l.find('.sidebar-label').text()).toBe(t(keys[i]))
      expect(l.attributes('title')).toBe(t(keys[i]))
    })
    const reset = w.find('.sidebar-reset-btn')
    expect(reset.attributes('title')).toBe(t('app.reset_all'))
    expect(reset.find('.sidebar-label').text()).toBe(t('app.reset_all'))
    await reset.trigger('click')
    expect(reset.attributes('title')).toBe(t('app.confirm'))
  })
})
