import { mockInstalledEditor } from './helpers/post-editor.js'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).createAppMock({ SyncPresets: vi.fn(() => Promise.resolve(null)) }))

import EditSection from '../components/EditSection.vue'
import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, UpdateSettings, ListPresets, EditImage, GetSDLoRAs, GetActiveSessionItem, GetSessionImage,
  ListPresetTypes, ListHiresProfiles, EnqueueFromImage, SyncPresets,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const QWEN = 'qwen_image_2.1-Q8_0'
const KLEIN = 'flux-2-klein-4b'
const SOURCE = 'c291cmNl'
const IMG = 'aW1n'

const KLEIN_STYLE = { id: 2, name: 'Klein Film', model_name: KLEIN, prompt: 'klein tags', loras: '' }
const Z_STYLE = { id: 3, name: 'Z Photo', model_name: 'z-image-turbo', prompt: 'z tags', loras: '' }
const QWEN_STYLE = { id: 4, name: 'Film (Qwen-Image)', model_name: QWEN, prompt: 'qwen tags', loras: '' }

const TYPES = [{ id: 1, name: 'post' }, { id: 4, name: 'post_light' }]
const KLEIN_FX = { id: 40, name: 'Golden Hour', preset_type: 'post_light', type_id: 4, model_name: KLEIN, denoising_strength: null, loras: '' }
const QWEN_FX = { id: 41, name: 'Golden Hour (Qwen-Image)', preset_type: 'post_light', type_id: 4, model_name: QWEN, denoising_strength: null, loras: '' }

useFakeImage()

beforeEach(() => {
  vi.clearAllMocks()
  mockInstalledEditor()
  clearEventMocks()
  localStorage.clear()
  useRemixQueue().clearPostQueue()
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
  EditImage.mockImplementation(() => Promise.resolve({ image: 'UkVT' }))
  GetSettings.mockReturnValue(Promise.resolve({}))
  SyncPresets.mockImplementation(() => Promise.resolve(null))
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  let jobs = 0
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
})

async function mountEdit(presets, settings = {}) {
  ListPresets.mockReturnValue(Promise.resolve(presets))
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(EditSection, { props: { source: SOURCE, batch: false } })
  await settle()
  return wrapper
}

async function runEdit(wrapper) {
  await wrapper.find('[data-testid="edit-instruction"]').setValue('make hair red')
  await settle()
  const btn = wrapper.findAll('button').find(b => b.text().includes(t('edit.btn_generate')))
  expect(btn).toBeTruthy()
  expect(btn.element.disabled).toBe(false)
  await btn.trigger('click')
  await settle()
}

function lastEditParams() {
  expect(EditImage).toHaveBeenCalled()
  return EditImage.mock.calls[EditImage.mock.calls.length - 1][0]
}

async function mountPost(settings = {}) {
  ListPresets.mockReturnValue(Promise.resolve([KLEIN_FX, QWEN_FX]))
  GetSettings.mockReturnValue(Promise.resolve(settings))
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

async function showLightGroup(wrapper) {
  const select = wrapper.find('.fx-group-pick select')
  if (select.exists()) {
    const light = select.findAll('option').map(o => o.attributes('value')).find(v => v && v.includes('light'))
    if (light) {
      await select.setValue(light)
      await settle()
    }
  }
}

function effectCard(wrapper, preset) {
  return wrapper.findAll('.fx-card').find(c => c.attributes('title') === preset.name)
}

async function applyEffect(wrapper, preset) {
  await showLightGroup(wrapper)
  const card = effectCard(wrapper, preset)
  expect(card).toBeTruthy()
  await card.trigger('click')
  await settle()
  const apply = wrapper.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
  expect(apply).toBeTruthy()
  expect(apply.element.disabled).toBe(false)
  await apply.trigger('click')
  await settle()
}

function lastEnqueue() {
  expect(EnqueueFromImage).toHaveBeenCalled()
  return EnqueueFromImage.mock.calls[EnqueueFromImage.mock.calls.length - 1][0]
}

function sentLoraNames(params) {
  return params.editor.loras.map(l => l.name)
}

describe('Effects: Qwen-Image presets', () => {
  it('Qwen effect card shows the translated base label and the Qwen-Image mark, next to the Klein card', async () => {
    const wrapper = await mountPost()
    await showLightGroup(wrapper)
    const qwen = effectCard(wrapper, QWEN_FX)
    const klein = effectCard(wrapper, KLEIN_FX)
    expect(qwen).toBeTruthy()
    expect(klein).toBeTruthy()
    const label = t('post.effect_golden_hour')
    expect(label).not.toBe('post.effect_golden_hour')
    expect(qwen.text()).toContain(label)
    expect(qwen.text()).toContain('Qwen-Image')
    expect(klein.text()).toContain(label)
    expect(klein.text()).not.toContain('Qwen')
  })

  it('shared Klein editor LoRAs apply to a Qwen effect description', async () => {
    const wrapper = await mountPost({ effect_loras_klein: '[{"name":"LoraA","weight":0.7}]' })
    await applyEffect(wrapper, QWEN_FX)
    const p = lastEnqueue()
    expect(p.preset_id).toBe(QWEN_FX.id)
    expect(sentLoraNames(p)).toEqual(['LoraA'])
  })

  it('Klein effect LoRAs are still sent with a Klein effect', async () => {
    const wrapper = await mountPost({ effect_loras_klein: '[{"name":"LoraA","weight":0.7}]' })
    await applyEffect(wrapper, KLEIN_FX)
    const p = lastEnqueue()
    expect(p.preset_id).toBe(KLEIN_FX.id)
    expect(sentLoraNames(p)).toContain('LoraA')
  })
})
