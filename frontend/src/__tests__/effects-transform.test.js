import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const TYPES = [
  { id: 1, name: 'post' },
  { id: 2, name: 'post_film' },
  { id: 3, name: 'post_transform' },
]

const PRESETS = [
  { id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null },
  { id: 11, name: 'Film Fade', preset_type: 'post_film', type_id: 2 },
  { id: 20, name: '3D Cartoon', preset_type: 'post_transform', type_id: 3, denoising_strength: 0.85 },
  { id: 21, name: 'Photo Enhance', preset_type: 'post_transform', type_id: 3, denoising_strength: 0.75 },
]

async function mountPage({ presets = PRESETS, types = TYPES } = {}) {
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(presets))
  ListPresetTypes.mockReturnValue(Promise.resolve(types))
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' } })
  await flushPromises()
  return wrapper
}

function effectCard(wrapper, name) {
  return wrapper.findAll('.fx-card').find(c => c.attributes('title') === name)
}

function strengthSlider(wrapper) {
  return wrapper.find('.fx-apply-bar input[type="range"]')
}

function strengthLabel(wrapper) {
  return wrapper.find('.fx-apply-bar .form-label').text()
}

function transformGroup(wrapper) {
  const label = t('post.group_effect_transform')
  return wrapper.findAll('.fx-group').find(g => g.find('.fx-group-label').text() === label)
}

describe('Transform effects in post-process', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
  })

  it('shows the post_transform group with its effects', async () => {
    const wrapper = await mountPage()
    expect(t('post.group_effect_transform')).not.toBe('post.group_effect_transform')
    const group = transformGroup(wrapper)
    expect(group).toBeTruthy()
    const names = group.findAll('.fx-card').map(c => c.attributes('title'))
    expect(names).toEqual(expect.arrayContaining(['3D Cartoon', 'Photo Enhance']))
    expect(names).not.toContain('Soft Focus')
    expect(names).not.toContain('Film Fade')
  })

  it('no post_transform presets: group is not shown', async () => {
    const wrapper = await mountPage({ presets: PRESETS.filter(p => p.type_id !== 3) })
    expect(transformGroup(wrapper)).toBeFalsy()
  })

  it('selecting an effect with denoising_strength 0.85 sets strength to 0.85', async () => {
    const wrapper = await mountPage()
    await effectCard(wrapper, '3D Cartoon').trigger('click')
    await flushPromises()
    expect(Number(strengthSlider(wrapper).element.value)).toBeCloseTo(0.85, 5)
    expect(strengthLabel(wrapper)).toContain('0.85')
  })

  it('selecting another effect with its own strength switches to it', async () => {
    const wrapper = await mountPage()
    await effectCard(wrapper, '3D Cartoon').trigger('click')
    await effectCard(wrapper, 'Photo Enhance').trigger('click')
    await flushPromises()
    expect(Number(strengthSlider(wrapper).element.value)).toBeCloseTo(0.75, 5)
    expect(strengthLabel(wrapper)).toContain('0.75')
  })

  it('selecting an effect without denoising_strength keeps the current strength', async () => {
    const wrapper = await mountPage()
    await strengthSlider(wrapper).setValue('0.42')
    await flushPromises()
    await effectCard(wrapper, 'Soft Focus').trigger('click')
    await flushPromises()
    expect(Number(strengthSlider(wrapper).element.value)).toBeCloseTo(0.42, 5)
    expect(strengthLabel(wrapper)).toContain('0.42')

    await effectCard(wrapper, 'Film Fade').trigger('click')
    await flushPromises()
    expect(Number(strengthSlider(wrapper).element.value)).toBeCloseTo(0.42, 5)
  })

  it('effect with denoising_strength 0 does not change strength', async () => {
    const presets = [
      ...PRESETS,
      { id: 30, name: 'Zero Fx', preset_type: 'post', type_id: 1, denoising_strength: 0 },
    ]
    const wrapper = await mountPage({ presets })
    await strengthSlider(wrapper).setValue('0.5')
    await effectCard(wrapper, 'Zero Fx').trigger('click')
    await flushPromises()
    expect(Number(strengthSlider(wrapper).element.value)).toBeCloseTo(0.5, 5)
  })
})
