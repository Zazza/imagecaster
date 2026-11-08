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
  ListHiresProfiles, EnqueueFromImage, ReadFileAsBase64, BatchSaveImage, BatchReset,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const TYPES = [
  { id: 1, name: 'post' },
  { id: 2, name: 'post_material' },
  { id: 3, name: 'post_light' },
]
const PRESETS = [
  { id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null },
  { id: 20, name: 'Leather', preset_type: 'post_material', type_id: 2, denoising_strength: null },
  { id: 30, name: 'Golden Hour', preset_type: 'post_light', type_id: 3, denoising_strength: null },
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
  localStorage.clear()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  const files = { '/photos/a.png': 'QUFBQQ==', '/photos/b.png': 'QkJCQg==' }
  ReadFileAsBase64.mockImplementation(p => Promise.resolve(files[p] || ''))
  BatchSaveImage.mockImplementation(() => Promise.resolve('/batch/v.png'))
  BatchReset.mockImplementation(() => Promise.resolve())
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
  globalThis.Image = OrigImage
  window.Image = OrigImage
})

async function mountSingle() {
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

async function mountBatch() {
  useRemixQueue().setPostQueue([{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }])
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  return wrapper
}

const GROUP_OF = {
  'Soft Focus': 'post.group_effect_base',
  'Leather': 'post.group_effect_material',
  'Golden Hour': 'post.group_effect_light',
}

async function pickGroup(wrapper, key) {
  const select = wrapper.find('.fx-group-pick select')
  expect(select.exists()).toBe(true)
  await select.setValue(key)
  await settle()
}

async function selectEffect(wrapper, name) {
  await pickGroup(wrapper, GROUP_OF[name])
  const card = wrapper.findAll('.fx-card').find(c => c.attributes('title') === name)
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
}

async function apply(wrapper, text) {
  const btn = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(text))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

function keepToggle(wrapper) {
  const text = t('post.effect_keep_color')
  for (const label of wrapper.findAll('label').filter(l => l.text().includes(text))) {
    const inner = label.find('input[type="checkbox"]')
    if (inner.exists()) return { kind: 'checkbox', el: inner }
    const id = label.attributes('for')
    if (id) {
      const byId = wrapper.find(`input#${id}`)
      if (byId.exists()) return { kind: 'checkbox', el: byId }
    }
  }
  const btn = wrapper.findAll('button').find(b => b.text().includes(text))
  if (btn) return { kind: 'button', el: btn }
  return null
}

function toggleShown(wrapper) {
  const tg = keepToggle(wrapper)
  return !!tg && tg.el.isVisible()
}

function toggleOn(tg) {
  if (tg.kind === 'checkbox') return tg.el.element.checked
  return tg.el.attributes('aria-pressed') === 'true' || tg.el.classes().includes('active')
}

async function turnOff(wrapper) {
  const tg = keepToggle(wrapper)
  expect(tg).toBeTruthy()
  if (tg.kind === 'checkbox') await tg.el.setValue(false)
  else await tg.el.trigger('click')
  await settle()
  expect(toggleOn(keepToggle(wrapper))).toBe(false)
}

function lastParams() {
  expect(EnqueueFromImage).toHaveBeenCalled()
  return EnqueueFromImage.mock.calls[EnqueueFromImage.mock.calls.length - 1][0]
}

describe('Keep color toggle for material effects', () => {
  it('i18n key exists', () => {
    expect(t('post.effect_keep_color')).not.toBe('post.effect_keep_color')
  })

  it('material effect selected: toggle shown and on by default, job gets keep_color=true', async () => {
    const wrapper = await mountSingle()
    await selectEffect(wrapper, 'Leather')
    expect(toggleShown(wrapper)).toBe(true)
    expect(toggleOn(keepToggle(wrapper))).toBe(true)
    await apply(wrapper, t('post.btn_apply_effect'))
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastParams().keep_color).toBe(true)
  })

  it('material effect, toggle turned off: job has keep_color false or absent', async () => {
    const wrapper = await mountSingle()
    await selectEffect(wrapper, 'Leather')
    await turnOff(wrapper)
    await apply(wrapper, t('post.btn_apply_effect'))
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastParams().keep_color).toBeFalsy()
  })

  it.each(['Soft Focus', 'Golden Hour'])('non-material effect %s: no toggle, keep_color not true', async name => {
    const wrapper = await mountSingle()
    await selectEffect(wrapper, name)
    expect(toggleShown(wrapper)).toBe(false)
    await apply(wrapper, t('post.btn_apply_effect'))
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(lastParams().keep_color).not.toBe(true)
  })

  it('no effect selected: toggle hidden', async () => {
    const wrapper = await mountSingle()
    expect(toggleShown(wrapper)).toBe(false)
  })

  it('switching from material to another group hides the toggle and drops keep_color', async () => {
    const wrapper = await mountSingle()
    await selectEffect(wrapper, 'Leather')
    expect(toggleShown(wrapper)).toBe(true)
    await selectEffect(wrapper, 'Golden Hour')
    expect(toggleShown(wrapper)).toBe(false)
    await apply(wrapper, t('post.btn_apply_effect'))
    expect(lastParams().keep_color).not.toBe(true)
  })

  it('batch mode with material effect: every job gets keep_color=true', async () => {
    const wrapper = await mountBatch()
    expect(wrapper.find('.batch-panel').exists()).toBe(true)
    await selectEffect(wrapper, 'Leather')
    expect(toggleShown(wrapper)).toBe(true)
    expect(toggleOn(keepToggle(wrapper))).toBe(true)
    await apply(wrapper, t('post.btn_apply_effect_all', { n: 2 }))
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    for (const call of EnqueueFromImage.mock.calls) {
      expect(call[0].keep_color).toBe(true)
    }
  })

  it('batch mode with non-material effect: no job gets keep_color=true', async () => {
    const wrapper = await mountBatch()
    await selectEffect(wrapper, 'Golden Hour')
    expect(toggleShown(wrapper)).toBe(false)
    await apply(wrapper, t('post.btn_apply_effect_all', { n: 2 }))
    expect(EnqueueFromImage).toHaveBeenCalledTimes(2)
    for (const call of EnqueueFromImage.mock.calls) {
      expect(call[0].keep_color).not.toBe(true)
    }
  })
})
