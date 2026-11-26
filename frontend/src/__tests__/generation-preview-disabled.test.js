import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import GenProgress from '../components/GenProgress.vue'
import { EventsEmit, clearEventMocks } from './mocks/runtime'
import { CancelQueueJob } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
beforeEach(() => { clearEventMocks(); vi.clearAllMocks() })
const QUEUE_JOB = 101
const STEPS = 30
const CURRENT_STEP = 15
function harness() {
  let state
  const wrapper = mount(defineComponent({
    setup() {
      state = useGenerationProgress()
      return () => state.sdProgress.value ? h(GenProgress, {
        progress: state.sdProgress.value.progress, steps: state.sdProgress.value.steps,
        stats: true, previewAvailable: state.sdProgress.value.preview_available,
        onInterrupt: state.interrupt,
      }) : null
    },
  }))
  return { wrapper, get state() { return state } }
}
describe('disabled process images retain progress controls', () => {
  it('suppresses unsupported warning for deliberate disabling and retains steps and stop', async () => {
    const view = harness()
    EventsEmit('sd:progress', { queue_job_id: QUEUE_JOB, job: 'server', stage: 1, progress: CURRENT_STEP / STEPS, steps: STEPS, preview_disabled: true, preview_available: false })
    await settle()
    expect(view.wrapper.text()).not.toContain(t('progress.preview_unavailable'))
    expect(view.wrapper.text()).toContain('50%')
    expect(view.wrapper.text()).toContain(t('progress.sd_step', { current: CURRENT_STEP, total: STEPS }))
    expect(view.state.sdProgress.value.progress).toBe(CURRENT_STEP / STEPS)
    expect(view.state.preview.value).toBe('')
    await view.wrapper.get('button').trigger('click')
    await settle()
    expect(CancelQueueJob).toHaveBeenCalledWith(QUEUE_JOB)
  })
  it('does not flash unsupported warning before sampling or while preparing the next stage', async () => {
    const view = harness()
    EventsEmit('sd:progress', { queue_job_id: QUEUE_JOB, job: 'first', stage: 1, progress: 0, steps: 0, preview_available: false })
    await settle()
    expect(view.wrapper.text()).not.toContain(t('progress.preview_unavailable'))
    EventsEmit('sd:progress', { queue_job_id: QUEUE_JOB, job: 'first', stage: 1, progress: 0.5, steps: STEPS, preview_available: true, preview: 'FIRST', preview_frame: 1 })
    await settle()
    EventsEmit('sd:progress', { queue_job_id: QUEUE_JOB, job: 'second', stage: 2, progress: 0, steps: 0, preview_available: false })
    await settle()
    expect(view.wrapper.text()).not.toContain(t('progress.preview_unavailable'))
    expect(view.state.preview.value).toContain('FIRST')
  })
  it('still reports unsupported preview when it was requested', async () => {
    const view = harness()
    EventsEmit('sd:progress', { queue_job_id: QUEUE_JOB, job: 'server', stage: 1, progress: 0.5, steps: STEPS, preview_disabled: false, preview_available: false })
    await settle()
    expect(view.wrapper.text()).toContain(t('progress.preview_unavailable'))
  })
  it('clears the previous image before showing a disabled next job', async () => {
    const view = harness()
    EventsEmit('sd:progress', { queue_job_id: QUEUE_JOB, job: 'previous', stage: 1, progress: 0.5, preview_available: true, preview: 'OLD_FRAME', preview_frame: 1 })
    expect(view.state.preview.value).toContain('OLD_FRAME')
    EventsEmit('queue:started', { job_id: QUEUE_JOB + 1 })
    EventsEmit('sd:progress', { queue_job_id: QUEUE_JOB + 1, job: 'next', stage: 1, progress: 0.5, steps: STEPS, preview_disabled: true, preview_available: false })
    await settle()
    expect(view.state.preview.value).toBe('')
    expect(view.wrapper.text()).not.toContain(t('progress.preview_unavailable'))
  })
})
