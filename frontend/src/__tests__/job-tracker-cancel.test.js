import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks, EventsEmit } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import FloatingGenerationPanel from '../components/FloatingGenerationPanel.vue'
import RemixSection from '../components/RemixSection.vue'
import {
  GetSettings, GetQueue, ListPresets, GenerateSDPrompt, EnqueueTxt2Img, EnqueueFromImage, GetSessionImage,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const PRESETS = [{ id: 1, name: 'Alpha', preset_type: 'general', loras: '' }]

let queue = []

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  queue = []
  GetQueue.mockImplementation(() => Promise.resolve(queue.map(j => ({ ...j }))))
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  GenerateSDPrompt.mockImplementation(() => Promise.resolve({ prompt: 'p', negative_prompt: '' }))
})

function cancelFromPanel(id) {
  queue = queue.map(j => (j.id === id ? { ...j, status: 'cancelled' } : j))
  EventsEmit('queue:changed')
}

const generatingText = () => t('generate.generating_image')

function expectGenerating(w, active) {
  const panel = w.findComponent(FloatingGenerationPanel)
  expect(panel.exists()).toBe(true)
  expect(panel.props('active')).toBe(active)
  expect(panel.isVisible()).toBe(active)
  if (active) expect(panel.text()).toContain(generatingText())
}

async function mountGenerate() {
  GetSettings.mockImplementation(() => Promise.resolve({ gen_mode: 'preset', gen_preset_id: '1' }))
  const w = mount(GeneratePage, { attachTo: document.body })
  await settle()
  return w
}

describe('GeneratePage: job cancelled from the queue panel', () => {
  it('pending job enqueued by Generate and cancelled in the panel resets "generating"', async () => {
    EnqueueTxt2Img.mockImplementation(() => {
      queue.push({ id: 101, type: 'txt2img', status: 'pending' })
      return Promise.resolve(101)
    })
    const w = await mountGenerate()
    await w.findAll('textarea').find(ta => ta.attributes('placeholder') === t('generate.placeholder_description')).setValue('a cat')
    await w.find('.btn-generate').trigger('click')
    await settle()
    expect(EnqueueTxt2Img).toHaveBeenCalledTimes(1)
    expectGenerating(w, true)

    cancelFromPanel(101)
    await settle()
    expectGenerating(w, false)
  })

  it('jobs restored on mount: cancelling all of them in the panel resets "generating"', async () => {
    queue = [
      { id: 7, type: 'txt2img', status: 'pending' },
      { id: 8, type: 'txt2img', status: 'pending' },
    ]
    const w = await mountGenerate()
    expectGenerating(w, true)

    cancelFromPanel(7)
    await settle()
    expectGenerating(w, true)

    queue = queue.filter(j => j.id !== 8)
    EventsEmit('queue:changed')
    await settle()
    expectGenerating(w, false)
  })

  it('a running job in the panel keeps "generating" on queue:changed', async () => {
    queue = [{ id: 9, type: 'txt2img', status: 'pending' }]
    const w = await mountGenerate()
    queue = [{ id: 9, type: 'txt2img', status: 'running' }]
    EventsEmit('queue:changed')
    await settle()
    expectGenerating(w, true)
  })

  it('completed job shows the image by session item URL', async () => {
    queue = [{ id: 5, type: 'txt2img', status: 'pending' }]
    const w = await mountGenerate()
    EventsEmit('queue:completed', { job_id: 5, result: { session_item_id: 42, info: '{"seed":1}' } })
    await settle()
    expect(w.find('img[alt="Generated"]').attributes('src')).toBe('/api/img/42.jpg')
    expectGenerating(w, false)
  })
})

async function mountRemix() {
  GetSettings.mockImplementation(() => Promise.resolve({ fi_gen_mode: 'preset', fi_preset_id: '1' }))
  const w = mount(RemixSection)
  await settle()
  await w.setProps({ source: 'SRC' })
  await settle()
  return w
}

function remixGenerateButton(w) {
  return w.findAll('button').find(b => b.text().includes(t('fi.btn_generate')))
}

describe('RemixSection: job cancelled from the queue panel', () => {
  it('pending remix job cancelled in the panel resets "generating"', async () => {
    EnqueueFromImage.mockImplementation(() => {
      queue.push({ id: 301, type: 'from_image', status: 'pending' })
      return Promise.resolve(301)
    })
    const w = await mountRemix()
    await remixGenerateButton(w).trigger('click')
    await settle()
    expect(EnqueueFromImage).toHaveBeenCalledTimes(1)
    expect(w.find('.remix-progress').exists()).toBe(true)

    cancelFromPanel(301)
    await settle()
    expect(w.find('.remix-progress').exists()).toBe(false)
    expect(w.emitted('busy').at(-1)).toEqual([false])
  })

  it('completed remix job emits the image loaded by session item id', async () => {
    EnqueueFromImage.mockImplementation(() => Promise.resolve(302))
    GetSessionImage.mockImplementation(() => Promise.resolve('FULLB64'))
    const w = await mountRemix()
    await remixGenerateButton(w).trigger('click')
    await settle()
    EventsEmit('queue:completed', { job_id: 302, result: { session_item_id: 77 } })
    await settle()
    expect(GetSessionImage).toHaveBeenCalledWith(77)
    expect(w.emitted('result').at(-1)).toEqual(['FULLB64'])
    expect(w.find('.remix-progress').exists()).toBe(false)
  })
})
