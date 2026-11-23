import { mockInstalledEditor } from './helpers/post-editor.js'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import PostProcessPage from '../components/PostProcessPage.vue'
import {
  GetSettings, UpdateSettings, GetSDLoRAs, GetActiveSessionItem, GetSessionImage, ListPresets,
  ListPresetTypes, ListHiresProfiles, EnqueueFromImage,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'post' }]
const PLAIN = { id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, denoising_strength: null, loras: '' }
const WITH_LORA = { id: 11, name: 'Lora Look', preset_type: 'post', type_id: 1, denoising_strength: null, loras: '[{"name":"PresetL","weight":1}]' }
const IMG = 'aW1n'

useFakeImage()

beforeEach(() => {
  vi.clearAllMocks()
  mockInstalledEditor()
  clearEventMocks()
  localStorage.clear()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
  let jobs = 0
  EnqueueFromImage.mockImplementation(() => Promise.resolve('job-' + (++jobs)))
  GetSettings.mockReturnValue(Promise.resolve({}))
  UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([
    { name: 'LoraA', path: '/loras/LoraA.safetensors' },
    { name: 'LoraB', path: '/loras/LoraB.safetensors' },
  ]))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(null))
  GetSessionImage.mockReturnValue(Promise.resolve(''))
  ListPresets.mockReturnValue(Promise.resolve([PLAIN, WITH_LORA]))
  ListPresetTypes.mockReturnValue(Promise.resolve(TYPES))
})

afterEach(() => {
  vi.restoreAllMocks()
})

async function mountPage() {
  const wrapper = mount(PostProcessPage, { props: { initialSection: 'effect' }, attachTo: document.body })
  await settle()
  await wrapper.setProps({ initialImage: IMG })
  await settle()
  return wrapper
}

describe('effects use the shared editor controls', () => {
  it('shared LoRA picker uses server names and preserves zero weight without hidden preset LoRAs', async () => {
    const w = await mountPage()
    expect(w.find('.fx-model').exists()).toBe(false)
    await w.find('[data-testid="post-editor-lora-add"]').trigger('click')
    await settle()
    expect(w.findAll('#post-editor-loras option').map(o => o.attributes('value'))).toEqual(['LoraA', 'LoraB'])
    await w.find('[data-testid="post-editor-lora-name"]').setValue('LoraA')
    await w.find('[data-testid="post-editor-lora-weight"]').setValue('0')
    await w.findAll('.fx-card').find(c => c.attributes('title') === WITH_LORA.name).trigger('click')
    const apply = () => w.findAll('.fx-apply-bar button').find(b => b.text().includes(t('post.btn_apply_effect')))
    await apply().trigger('click')
    await settle()
    expect(EnqueueFromImage.mock.calls[0][0].editor.loras).toEqual([{ name: 'LoraA', weight: 0 }])
    expect(EnqueueFromImage.mock.calls[0][0].editor.loras.map(l => l.name)).not.toContain('PresetL')
  })
})
