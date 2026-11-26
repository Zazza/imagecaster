import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { EventsEmit, clearEventMocks } from './mocks/runtime'
import { CancelQueueJob, InterruptGeneration } from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)
beforeEach(() => { clearEventMocks(); vi.clearAllMocks() })
const JOB_A = 101
const JOB_B = 202
const emit = data => EventsEmit('sd:progress', data)
function harness(options) {
  let progress
  mount(defineComponent({ setup() { progress = useGenerationProgress(options); return () => null } }))
  return progress
}
const preview = (job, stage, frame, image, pass = 1) => ({ queue_job_id: job, job: `server-${job}`, stage, preview_frame: frame, preview: image, preview_pass: pass, progress: 0.5 })

describe('preview ownership and ordering', () => {
  it('isolates queue owners even before queue:started', () => {
    const a = harness({ acceptJob: id => id === JOB_A })
    const b = harness({ acceptJob: id => id === JOB_B })
    emit(preview(JOB_A, 1, 1, 'AAA'))
    expect(a.preview.value).toContain('AAA')
    expect(b.preview.value).toBe('')
    emit(preview(JOB_B, 1, 1, 'BBB'))
    expect(a.preview.value).toContain('AAA')
    expect(b.preview.value).toContain('BBB')
  })
  it('queue start clears the prior image and rejects its late delivery', () => {
    const p = harness()
    EventsEmit('queue:started', { job_id: JOB_A })
    emit(preview(JOB_A, 1, 1, 'AAA'))
    EventsEmit('queue:started', { job_id: JOB_B })
    expect(p.preview.value).toBe('')
    emit(preview(JOB_A, 1, 2, 'LATE'))
    expect(p.preview.value).toBe('')
    emit(preview(JOB_B, 1, 1, 'BBB'))
    expect(p.preview.value).toContain('BBB')
  })
  it('retains the previous stage image until the next image and ignores late old-stage preview', () => {
    const p = harness()
    emit(preview(JOB_A, 1, 10, 'STAGE1'))
    emit({ queue_job_id: JOB_A, job: `server-${JOB_A}`, stage: 2, progress: 0.1 })
    expect(p.preview.value).toContain('STAGE1')
    emit(preview(JOB_A, 2, 11, 'STAGE2'))
    emit(preview(JOB_A, 1, 12, 'LATE'))
    expect(p.preview.value).toContain('STAGE2')
    expect(p.sdProgress.value.stage).toBe(2)
  })
  it('never rolls frame IDs backwards, including a pass switch', () => {
    const p = harness()
    emit(preview(JOB_A, 1, 10, 'NEW'))
    emit(preview(JOB_A, 1, 9, 'OLD'))
    expect(p.preview.value).toContain('NEW')
    emit(preview(JOB_A, 1, 11, 'SECOND', 2))
    emit(preview(JOB_A, 1, 10, 'LATE', 1))
    expect(p.preview.value).toContain('SECOND')
  })
  it('retains the last sampling image through decoding and progress gaps', () => {
    const p = harness()
    emit({ ...preview(JOB_A, 1, 1, 'SAMPLING'), preview_available: true })
    expect(p.preview.value).toContain('SAMPLING')
    emit({ queue_job_id: JOB_A, job: `server-${JOB_A}`, stage: 1, preview_available: true, progress: 1 })
    expect(p.preview.value).toContain('SAMPLING')
    emit({ queue_job_id: JOB_A, job: `server-${JOB_A}`, stage: 1, preview_available: true, progress: 0 })
    expect(p.preview.value).toContain('SAMPLING')
  })
  it('retains a queue-owned image across remote jobs and passes, replacing only with a new frame', () => {
    const p = harness()
    emit(preview(JOB_A, 1, 10, 'BASE'))
    emit({ queue_job_id: JOB_A, job: 'garment', stage: 2, preview_available: false, progress: 0, steps: 0 })
    expect(p.preview.value).toContain('BASE')
    emit({ queue_job_id: JOB_A, job: 'garment', stage: 2, preview_pass: 2, preview_available: true, progress: 0, steps: 20 })
    expect(p.preview.value).toContain('BASE')
    emit({ ...preview(JOB_A, 2, 1, 'GARMENT', 2), job: 'garment' })
    expect(p.preview.value).toContain('GARMENT')
    emit(preview(JOB_A, 1, 11, 'LATE_BASE'))
    expect(p.preview.value).toContain('GARMENT')
  })
  it('clears across queue owners even without a queue-start notification', () => {
    const p = harness()
    emit(preview(JOB_A, 2, 10, 'PREVIOUS'))
    emit({ queue_job_id: JOB_B, job: 'next', stage: 1, progress: 0, preview_available: false })
    expect(p.preview.value).toBe('')
    emit({ ...preview(JOB_B, 1, 1, 'NEXT'), job: 'next' })
    expect(p.preview.value).toContain('NEXT')
  })
  it('start and explicit reset discard retained images', () => {
    const p = harness()
    emit(preview(JOB_A, 1, 1, 'BEFORE_START'))
    EventsEmit('sd:progress:start')
    expect(p.preview.value).toBe('')
    emit(preview(JOB_A, 1, 2, 'BEFORE_RESET'))
    p.reset()
    expect(p.preview.value).toBe('')
    expect(p.sdProgress.value).toBeNull()
  })
  it('stop clears and interrupt targets only the current queued job', async () => {
    const p = harness()
    emit(preview(JOB_A, 1, 1, 'AAA'))
    await p.interrupt()
    expect(CancelQueueJob).toHaveBeenCalledWith(JOB_A)
    expect(InterruptGeneration).not.toHaveBeenCalled()
    EventsEmit('sd:progress:stop')
    expect(p.preview.value).toBe('')
    expect(p.sdProgress.value).toBeNull()
    emit({ progress: 0.5, job: 'direct', preview: 'DIRECT' })
    await p.interrupt()
    expect(InterruptGeneration).toHaveBeenCalledTimes(1)
  })
})
