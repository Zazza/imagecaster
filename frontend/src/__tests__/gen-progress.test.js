import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

import RemixSection from '../components/RemixSection.vue'
import EditSection from '../components/EditSection.vue'
import ObjectSection from '../components/ObjectSection.vue'
import RepairSection from '../components/RepairSection.vue'
import GeneratePage from '../components/GeneratePage.vue'
import GenProgress from '../components/GenProgress.vue'
import FloatingGenerationPanel from '../components/FloatingGenerationPanel.vue'
import {
  GetSettings, ListPresets, ListCompoundPresets, ListHiresProfiles, EnqueueFromImage, EditImage, EditObject,
  RepairImage, InterruptGeneration, CancelQueueJob, CancelQueue, GetQueue, GetActiveSessionItem,
} from '../wailsjs/go/main/App.js'
import { useRemixQueue } from '../remixQueue.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
useFakeImage()
useFakeCanvas()

const SOURCE = 'c291cmNl'
const STEP_TEXT = () => t('progress.sd_step', { current: 10, total: 20 })

function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  useRemixQueue().clearPostQueue()
  ListHiresProfiles.mockImplementation(() => Promise.resolve([]))
})

const barWidths = w => w.findAll('[style]').map(e => e.element.style.width).filter(Boolean)
const interruptButton = w => w.findAll('button').find(b => b.text().includes(t('progress.btn_interrupt')))

async function emitProgress(data) {
  EventsEmit('sd:progress', data)
  await settle()
}

async function startEdit() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  ListPresets.mockReturnValue(Promise.resolve([]))
  const w = mount(EditSection, { props: { source: SOURCE } })
  await settle()
  await w.find('[data-testid="edit-instruction"]').setValue('make the sky red')
  EditImage.mockReturnValue(deferred().promise)
  await w.findAll('button').find(b => b.text().includes(t('edit.btn_generate'))).trigger('click')
  await settle()
  return w
}

async function startRemix() {
  GetSettings.mockReturnValue(Promise.resolve({ fi_mode: 'img2img', fi_gen_mode: 'compound', fi_compound_preset_id: '3' }))
  ListCompoundPresets.mockReturnValue(Promise.resolve([{ id: 3, name: 'Pipeline', steps: [] }]))
  const w = mount(RemixSection, { props: { source: '', active: true } })
  await settle()
  await w.setProps({ source: SOURCE })
  await settle()
  EnqueueFromImage.mockReturnValue(Promise.resolve('remix-p'))
  await w.findAll('button').find(b => b.text().includes(t('fi.btn_generate'))).trigger('click')
  await settle()
  return w
}

async function paint(w) {
  w.vm.beginStroke(10, 10, 5)
  w.vm.strokeTo(12, 12, 5)
  w.vm.endStroke()
  await settle()
}

async function startObject() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  const w = mount(ObjectSection, { props: { source: SOURCE, active: true }, attachTo: document.body })
  await settle()
  await paint(w)
  EditObject.mockReturnValue(deferred().promise)
  await w.find('[data-testid="object-run"]').trigger('click')
  await settle()
  return w
}

async function startRepair() {
  GetSettings.mockReturnValue(Promise.resolve({}))
  const w = mount(RepairSection, { props: { source: SOURCE, active: true }, attachTo: document.body })
  await settle()
  await paint(w)
  RepairImage.mockReturnValue(deferred().promise)
  await w.find('[data-testid="repair-run"]').trigger('click')
  await settle()
  return w
}

async function mountGenerating({ withImage }) {
  GetSettings.mockReturnValue(Promise.resolve({}))
  ListPresets.mockReturnValue(Promise.resolve([]))
  ListCompoundPresets.mockReturnValue(Promise.resolve([]))
  GetActiveSessionItem.mockReturnValue(Promise.resolve(withImage ? { id: 7, info: '{}' } : null))
  GetQueue.mockReturnValue(Promise.resolve([{ id: 55, type: 'txt2img', status: 'running' }]))
  const w = mount(GeneratePage, { attachTo: document.body })
  await settle()
  return w
}

describe('generation progress block: current behavior', () => {
  it('Edit: percent, step, ETA, bar width, interrupt calls InterruptGeneration', async () => {
    const w = await startEdit()
    expect(interruptButton(w)).toBeFalsy()
    await emitProgress({ progress: 0.5, steps: 20, eta_relative: 12.3 })
    expect(w.text()).toContain('50%')
    expect(w.text()).toMatch(new RegExp(STEP_TEXT() + '\\s+— ~13s'))
    expect(barWidths(w)).toContain('50%')
    expect(w.findComponent(GenProgress).exists()).toBe(true)
    await interruptButton(w).trigger('click')
    await settle()
    expect(InterruptGeneration).toHaveBeenCalled()
  })

  it('Edit: no ETA when eta is 0', async () => {
    const w = await startEdit()
    await emitProgress({ progress: 0.25, steps: 20, eta_relative: 0 })
    expect(w.text()).toContain('25%')
    expect(w.text()).not.toContain('~0s')
    expect(barWidths(w)).toContain('25%')
  })

  it('Remix: interrupt cancels the current queued job', async () => {
    const w = await startRemix()
    await emitProgress({ queue_job_id: 'remix-p', progress: 0.5, steps: 20, eta_relative: 4.2 })
    expect(w.text()).toContain('50%')
    expect(w.text()).toMatch(new RegExp(STEP_TEXT() + '\\s+— ~5s'))
    expect(barWidths(w)).toContain('50%')
    await interruptButton(w).trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith('remix-p')
  })

  it('Object: only the bar and interrupt, no percent text', async () => {
    const w = await startObject()
    expect(interruptButton(w)).toBeFalsy()
    await emitProgress({ progress: 0.5, steps: 20, eta_relative: 3 })
    expect(barWidths(w)).toContain('50%')
    expect(w.text()).not.toContain('50%')
    expect(w.text()).not.toContain(STEP_TEXT())
    await interruptButton(w).trigger('click')
    await settle()
    expect(InterruptGeneration).toHaveBeenCalled()
  })

  it('Repair: tile label and overall bar width', async () => {
    const w = await startRepair()
    EventsEmit('repair:progress', { done: 1, total: 4 })
    await settle()
    await emitProgress({ progress: 0.5, steps: 20 })
    const block = w.find('[data-testid="repair-progress"]')
    expect(block.text()).toContain(t('repair.progress', { n: 2, m: 4 }))
    expect(block.text()).not.toContain(STEP_TEXT())
    expect(barWidths(block)).toContain('37.5%')
    await interruptButton(w).trigger('click')
    await settle()
    expect(InterruptGeneration).toHaveBeenCalled()
  })

  it('GeneratePage uses the shared floating panel and retains collapse through a stage change', async () => {
    const w = await mountGenerating({ withImage: false })
    await emitProgress({ queue_job_id: 55, job: 'base', stage: 1, progress: 0.5, steps: 20, eta_relative: 4 })
    const panel = w.findComponent(FloatingGenerationPanel)
    expect(panel.exists()).toBe(true)
    const toggle = panel.get('[data-testid="progress-panel-toggle"]')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await emitProgress({ queue_job_id: 55, job: 'garment', stage: 2, progress: 0.1, steps: 30, eta_relative: 8 })
    expect(w.findComponent(FloatingGenerationPanel).get('[data-testid="progress-panel-toggle"]').attributes('aria-expanded')).toBe('false')
    await panel.get('.gp-interrupt').trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(55)
  })

  it.each([false, true])('GeneratePage retains its latest image through decode and next-stage preparation (previous image: %s)', async (withImage) => {
    const w = await mountGenerating({ withImage })
    const liveImage = 'data:image/png;base64,iVBORw0KGgo-stage-image'
    await emitProgress({ queue_job_id: 55, job: 'base', stage: 1, progress: 0.5, steps: 20, preview_available: true, preview: liveImage, preview_frame: 1, preview_pass: 1 })
    expect(w.find(`.generate-image-area img[src="${liveImage}"]`).exists()).toBe(true)
    await emitProgress({ queue_job_id: 55, job: 'base', stage: 1, progress: 1, steps: 20, preview_available: true })
    expect(w.find(`.generate-image-area img[src="${liveImage}"]`).exists()).toBe(true)
    await emitProgress({ queue_job_id: 55, job: 'garment', stage: 2, progress: 0, steps: 0, preview_available: false })
    expect(w.find(`.generate-image-area img[src="${liveImage}"]`).exists()).toBe(true)
  })

  it('GeneratePage without image: percent, step, ETA, interrupt cancels the current job', async () => {
    const w = await mountGenerating({ withImage: false })
    await emitProgress({ queue_job_id: 55, progress: 0.5, steps: 20, eta_relative: 12.3 })
    expect(w.text()).toContain('50%')
    expect(w.text()).toMatch(new RegExp(STEP_TEXT() + '\\s+— ~13s'))
    expect(barWidths(w)).toContain('50%')
    await interruptButton(w).trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(55)
    expect(CancelQueue).not.toHaveBeenCalled()
  })

  it('GeneratePage over an image: percent, step and ETA, interrupt cancels the current job', async () => {
    const w = await mountGenerating({ withImage: true })
    await emitProgress({ queue_job_id: 55, progress: 0.5, steps: 20, eta_relative: 12.3 })
    expect(w.text()).toContain('50%')
    expect(w.text()).toContain('~13s')
    expect(w.text()).toContain(STEP_TEXT())
    expect(w.text()).toContain('1 / 1')
    expect(barWidths(w)).toContain('50%')
    await interruptButton(w).trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(55)
    expect(CancelQueue).not.toHaveBeenCalled()
  })
})

const SERIES_JOB_A = 101
const SERIES_JOB_B = 102
async function mountSeries() {
  GetSettings.mockResolvedValue({})
  ListPresets.mockResolvedValue([])
  ListCompoundPresets.mockResolvedValue([])
  GetActiveSessionItem.mockResolvedValue(null)
  GetQueue.mockResolvedValue([
    { id: SERIES_JOB_A, type: 'txt2img', status: 'running' },
    { id: SERIES_JOB_B, type: 'txt2img', status: 'pending' },
  ])
  const w = mount(GeneratePage)
  await settle()
  return w
}
const stopSeriesButton = w => w.findAll('button').find(b => b.text() === t('progress.stop_series'))
it('Stop series still cancels the next job when the first completed just before cancellation', async () => {
  const w = await mountSeries()
  CancelQueueJob.mockImplementation(id => id === SERIES_JOB_A ? Promise.reject(new Error('job already completed')) : Promise.resolve())
  await stopSeriesButton(w).trigger('click')
  await settle()
  expect(CancelQueueJob).toHaveBeenCalledWith(SERIES_JOB_B)
  expect(CancelQueue).not.toHaveBeenCalled()
})
it('Stop series keeps failed cancellations tracked so the user can retry', async () => {
  const w = await mountSeries()
  CancelQueueJob.mockRejectedValue(new Error('connection lost'))
  await stopSeriesButton(w).trigger('click')
  await settle()
  expect(w.text()).toContain('connection lost')
  expect(stopSeriesButton(w)).toBeTruthy()
  CancelQueueJob.mockResolvedValue(undefined)
  await stopSeriesButton(w).trigger('click')
  await settle()
  expect(CancelQueueJob.mock.calls.filter(([id]) => id === SERIES_JOB_A)).toHaveLength(2)
  expect(CancelQueueJob).toHaveBeenCalledWith(SERIES_JOB_B)
})
