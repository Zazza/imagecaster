import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base }
})

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, GetActiveSessionItem, GetSessionImage, ListPresets, ListPresetTypes,
  ListHiresProfiles, EnqueueFromImage,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const STORAGE_KEY = 'pp.effectGroup'
const MATERIAL = 'post.group_effect_material'
const LIGHT = 'post.group_effect_light'
const TRANSFORM = 'post.group_effect_transform'

const TYPES = [
  { id: 1, name: 'post' },
  { id: 2, name: 'post_material' },
  { id: 3, name: 'post_skin' },
  { id: 4, name: 'post_light' },
  { id: 5, name: 'post_film' },
  { id: 6, name: 'post_transform' },
]
const PRESETS = [
  { id: 20, name: 'Leather Zq', preset_type: 'post_material', type_id: 2, denoising_strength: null },
  { id: 21, name: 'Velvet Zq', preset_type: 'post_material', type_id: 2, denoising_strength: null },
  { id: 40, name: 'Golden Hour Zq', preset_type: 'post_light', type_id: 4, denoising_strength: null },
  { id: 60, name: 'Cartoon Zq', preset_type: 'post_transform', type_id: 6, denoising_strength: 0.8 },
]
const IMG = 'aW1n'

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

class FakeImage {
  constructor() {
    this.onload = null
    this.onerror = null
    this.naturalWidth = 64
    this.naturalHeight = 48
    this.width = 64
    this.height = 48
    this.complete = false
    this._src = ''
  }
  set src(v) {
    this._src = v
    setTimeout(() => {
      this.complete = true
      if (this.onload) this.onload()
    }, 0)
  }
  get src() {
    return this._src
  }
  decode() {
    return Promise.resolve()
  }
}

const OrigImage = globalThis.Image

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  let jobs = 0
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
  GetSettings.mockReturnValue(Promise.resolve({}))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve(PRESETS))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
  globalThis.Image = FakeImage
  window.Image = FakeImage
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  globalThis.Image = OrigImage
  window.Image = OrigImage
})

async function mountPage() {
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

function groupSelect(wrapper) {
  const select = wrapper.find('.fx-group-pick select')
  expect(select.exists()).toBe(true)
  return select
}

function optionValues(wrapper) {
  return groupSelect(wrapper).findAll('option').map(o => o.attributes('value'))
}

async function pickGroup(wrapper, key) {
  await groupSelect(wrapper).setValue(key)
  await settle()
}

function namedCards(wrapper) {
  return wrapper.findAll('.fx-card').filter(c => c.attributes('title'))
}

function cardNames(wrapper) {
  return namedCards(wrapper).map(c => c.attributes('title'))
}

function noneCard(wrapper) {
  const label = t('post.effect_none')
  return wrapper.findAll('.fx-card').find(c => c.text().includes(label))
}

function selectedLine(wrapper) {
  return wrapper.find('.fx-group-pick').text()
}

async function clickCard(wrapper, name) {
  const card = namedCards(wrapper).find(c => c.attributes('title') === name)
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
  return card
}

describe('Effect group dropdown', () => {
  it('GS1: lists only non-empty groups in order, first is chosen, only its cards shown', async () => {
    const wrapper = await mountPage()
    expect(optionValues(wrapper)).toEqual([MATERIAL, LIGHT, TRANSFORM])
    expect(groupSelect(wrapper).element.value).toBe(MATERIAL)

    const options = groupSelect(wrapper).findAll('option')
    expect(options[0].text()).toContain(t(MATERIAL))
    expect(options[0].text()).toContain('2')
    expect(options[1].text()).toContain(t(LIGHT))
    expect(options[1].text()).toContain('1')

    expect(cardNames(wrapper).sort()).toEqual(['Leather Zq', 'Velvet Zq'])
    expect(noneCard(wrapper)).toBeTruthy()
  })

  it('GS1: nothing selected yet: line under the dropdown says no effect', async () => {
    const wrapper = await mountPage()
    expect(selectedLine(wrapper)).toContain(t('post.effect_selected', { name: t('post.effect_none') }))
  })

  it('GS2: choosing another group shows its cards and hides the first group cards', async () => {
    const wrapper = await mountPage()
    await pickGroup(wrapper, LIGHT)
    expect(groupSelect(wrapper).element.value).toBe(LIGHT)
    expect(cardNames(wrapper)).toEqual(['Golden Hour Zq'])
    expect(noneCard(wrapper)).toBeTruthy()

    await pickGroup(wrapper, TRANSFORM)
    expect(cardNames(wrapper)).toEqual(['Cartoon Zq'])
  })

  it('GS3: switching group keeps the chosen effect, line names it, apply sends its preset', async () => {
    const wrapper = await mountPage()
    const card = await clickCard(wrapper, 'Velvet Zq')
    const shownName = card.find('.fx-name').text()
    expect(selectedLine(wrapper)).toContain(t('post.effect_selected', { name: shownName }))

    await pickGroup(wrapper, LIGHT)
    expect(cardNames(wrapper)).not.toContain('Velvet Zq')
    expect(selectedLine(wrapper)).toContain(t('post.effect_selected', { name: shownName }))

    const btn = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
    expect(btn).toBeTruthy()
    expect(btn.element.disabled).toBe(false)
    await btn.trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(EnqueueFromImage.mock.calls[0][0].preset_id).toBe(21)
  })

  it('GS3: choosing "No effect" resets the line', async () => {
    const wrapper = await mountPage()
    await clickCard(wrapper, 'Leather Zq')
    await noneCard(wrapper).trigger('click')
    await settle()
    expect(selectedLine(wrapper)).toContain(t('post.effect_selected', { name: t('post.effect_none') }))
  })

  it('GS4: remount opens the last chosen group', async () => {
    const first = await mountPage()
    await pickGroup(first, TRANSFORM)
    first.unmount()

    const second = await mountPage()
    expect(groupSelect(second).element.value).toBe(TRANSFORM)
    expect(cardNames(second)).toEqual(['Cartoon Zq'])
  })

  it('GS4: stored group that no longer has effects falls back to the first group', async () => {
    localStorage.setItem(STORAGE_KEY, 'post.group_effect_film')
    const wrapper = await mountPage()
    expect(groupSelect(wrapper).element.value).toBe(MATERIAL)
    expect(cardNames(wrapper).sort()).toEqual(['Leather Zq', 'Velvet Zq'])
  })

  it('GS4: localStorage throws: page works, first group, switching still works', async () => {
    const deny = () => { throw new Error('denied') }
    const getSpy = vi.fn(deny)
    const setSpy = vi.fn(deny)
    vi.stubGlobal('localStorage', { getItem: getSpy, setItem: setSpy, removeItem: deny, clear: deny, key: deny, length: 0 })
    const wrapper = await mountPage()
    expect(getSpy).toHaveBeenCalled()
    expect(groupSelect(wrapper).element.value).toBe(MATERIAL)
    expect(cardNames(wrapper).sort()).toEqual(['Leather Zq', 'Velvet Zq'])

    await pickGroup(wrapper, LIGHT)
    expect(cardNames(wrapper)).toEqual(['Golden Hour Zq'])
    await clickCard(wrapper, 'Golden Hour Zq')
    const btn = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
    await btn.trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(EnqueueFromImage.mock.calls[0][0].preset_id).toBe(40)
    expect(setSpy).toHaveBeenCalled()
  })
})
