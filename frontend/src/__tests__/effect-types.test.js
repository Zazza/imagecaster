import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import GeneratePage from '../components/GeneratePage.vue'
import { ListPresetTypes, ListPresets, ListPresetsByType } from '../wailsjs/go/main/App.js'
import { isEffectType, effectTypeLabel } from '../effectTypes.js'
import { t, setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

describe('isEffectType (ET1)', () => {
  it.each(['post', 'post_material', 'post_transform'])('%s is an effect type', name => {
    expect(isEffectType(name)).toBe(true)
  })
  it.each(['portrait', 'poster', ''])('%s is not an effect type', name => {
    expect(isEffectType(name)).toBe(false)
  })
})

describe('effectTypeLabel (ET2)', () => {
  beforeEach(() => setLocale('ru'))

  it('labels known effect group in ru', () => {
    expect(effectTypeLabel('post_skin', t)).toBe('Эффекты: Кожа')
  })
  it.each([
    ['post', 'Эффекты: Базовые'],
    ['post_material', 'Эффекты: Материалы'],
    ['post_light', 'Эффекты: Свет'],
    ['post_film', 'Эффекты: Плёнка'],
    ['post_transform', 'Эффекты: Преобразование'],
  ])('%s -> %s', (name, label) => {
    expect(effectTypeLabel(name, t)).toBe(label)
  })
  it('keeps non-effect type name unchanged', () => {
    expect(effectTypeLabel('portrait', t)).toBe('portrait')
  })
  it('keeps unknown post_* name unchanged', () => {
    expect(effectTypeLabel('post_unknown', t)).toBe('post_unknown')
  })
})

describe('GeneratePage hides effect types (ET3)', () => {
  const types = [
    { id: 1, name: 'portrait', description: '', sort_order: 1, created_at: '' },
    { id: 2, name: 'post_film', description: '', sort_order: 2, created_at: '' },
  ]
  const presets = [
    { id: 10, name: 'PortraitPresetZ', preset_type: 'portrait', type_id: 1, prompt: 'p' },
    { id: 11, name: 'FilmEffectPresetZ', preset_type: 'post_film', type_id: 2, prompt: 'f' },
  ]

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    ListPresetTypes.mockResolvedValue(types)
    ListPresets.mockResolvedValue(presets)
    ListPresetsByType.mockImplementation(name => Promise.resolve(presets.filter(p => p.preset_type === name)))
  })

  it('shows only non-effect types and presets with "all types"', async () => {
    const wrapper = mount(GeneratePage)
    await flushPromises()
    const html = wrapper.html()
    expect(html).toContain('portrait')
    expect(html).not.toContain('post_film')
    expect(html).toContain('PortraitPresetZ')
    expect(html).not.toContain('FilmEffectPresetZ')
  })
})
