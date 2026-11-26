import { beforeEach, afterEach, it, expect, vi } from 'vitest'
import { ref } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import ResolutionUpscaleOverlay from '../components/ResolutionUpscaleOverlay.vue'
import { EnqueueImageResolutionUpscale, GetQueueJobResult, GetSessionImage } from '../wailsjs/go/main/App.js'
import { EventsEmit, clearEventMocks } from './mocks/runtime'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
const TARGET_2K = 2560
const QUEUE_ID = 91
const LARGE = 'LARGE'
const SMALL = 'SMALL'
const BIG_RESULT = 'UPSCALED'
const strip = value => String(value).replace(/^data:[^,]*,/, '')
const button = w => w.findAll('button').find(b => b.text().startsWith('2K'))

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('Image', class {
    set src(value) {
      const large = strip(value) === LARGE
      this.naturalWidth = large ? 4000 : 800
      this.naturalHeight = large ? 3000 : 1200
      queueMicrotask(() => this.onload?.())
    }
  })
  EnqueueImageResolutionUpscale.mockResolvedValue(QUEUE_ID)
  GetQueueJobResult.mockResolvedValue({ id: QUEUE_ID, status: 'running' })
})
afterEach(() => vi.unstubAllGlobals())

it('batch preserves already-large images and only sends smaller current images to the upscaler', async () => {
  const outputs = []
  const batch = {
    batchActive: ref(true), batchBusy: ref(false), batchExporting: ref(false),
    batch: ref({ items: [{ name: 'large' }, { name: 'small' }] }),
    trackBatchJob: vi.fn(async queued => { await queued; return { wait: Promise.resolve(BIG_RESULT) } }),
    runBatchStep: vi.fn(async op => {
      batch.batchBusy.value = true
      try {
        const results = await Promise.all([LARGE, SMALL].map(op))
        outputs.push(...await Promise.all(results.map(result => typeof result === 'function' ? result() : result)))
      } finally {
        batch.batchBusy.value = false
      }
    }),
  }
  const w = mount(ResolutionUpscaleOverlay, { props: { image: LARGE, inline: true, batch } })
  await settle()
  expect(button(w).element.disabled).toBe(false)
  await button(w).trigger('click')
  await settle()
  expect(EnqueueImageResolutionUpscale).toHaveBeenCalledTimes(1)
  expect(EnqueueImageResolutionUpscale).toHaveBeenCalledWith(SMALL, TARGET_2K)
  expect(outputs).toEqual([LARGE, BIG_RESULT])
})

it('blocked format controls do not submit even if called programmatically', async () => {
  const w = mount(ResolutionUpscaleOverlay, { props: { image: SMALL, inline: true, blocked: true } })
  await settle()
  expect(button(w).element.disabled).toBe(true)
  await button(w).trigger('click')
  await settle()
  expect(EnqueueImageResolutionUpscale).not.toHaveBeenCalled()
})


const RESULT_ITEM = 99
const RETRY_QUEUE_ID = 92
const ERROR_CASES = [
  ['completed result JSON is corrupt', () => GetQueueJobResult.mockResolvedValueOnce({ id: QUEUE_ID, status: 'completed', result: '{not-json' })],
  ['completed result has no image item', () => GetQueueJobResult.mockResolvedValueOnce({ id: QUEUE_ID, status: 'completed', result: {} })],
  ['result image lookup rejects', () => {
    GetQueueJobResult.mockResolvedValueOnce({ id: QUEUE_ID, status: 'completed', result: { session_item_id: RESULT_ITEM } })
    GetSessionImage.mockRejectedValueOnce(new Error('image read failed'))
  }],
  ['result image is empty', () => {
    GetQueueJobResult.mockResolvedValueOnce({ id: QUEUE_ID, status: 'completed', result: { session_item_id: RESULT_ITEM } })
    GetSessionImage.mockResolvedValueOnce('')
  }],
]

it.each(ERROR_CASES)('recovers controls and releases busy when %s without accepting a late old result', async (_name, arrange) => {
  arrange()
  const w = mount(ResolutionUpscaleOverlay, { props: { image: SMALL, inline: true } })
  await settle()
  await button(w).trigger('click')
  await settle()
  expect(w.find('[role="alert"]').exists()).toBe(true)
  expect(button(w).element.disabled).toBe(false)
  expect(w.emitted('busy')?.map(args => args[0])).toContain(true)
  expect(w.emitted('busy')?.at(-1)).toEqual([false])
  expect(w.emitted('completed')).toBeUndefined()
  expect(w.props('image')).toBe(SMALL)

  EventsEmit('queue:completed', { job_id: QUEUE_ID, result: { session_item_id: RESULT_ITEM, image_base64: 'LATE_OLD_RESULT' } })
  await settle()
  expect(w.emitted('completed')).toBeUndefined()
  expect(w.props('image')).toBe(SMALL)

  EnqueueImageResolutionUpscale.mockResolvedValueOnce(RETRY_QUEUE_ID)
  GetQueueJobResult.mockResolvedValueOnce({ id: RETRY_QUEUE_ID, status: 'running' })
  await button(w).trigger('click')
  await settle()
  expect(EnqueueImageResolutionUpscale).toHaveBeenCalledTimes(2)
  expect(button(w).element.disabled).toBe(true)
  EventsEmit('queue:completed', { job_id: QUEUE_ID, result: { session_item_id: RESULT_ITEM, image_base64: 'LATE_OLD_RESULT' } })
  await settle()
  expect(w.emitted('completed')).toBeUndefined()
  EventsEmit('queue:completed', { job_id: RETRY_QUEUE_ID, result: { session_item_id: RESULT_ITEM, image_base64: BIG_RESULT } })
  await settle()
  expect(w.emitted('completed')).toEqual([[{ session_item_id: RESULT_ITEM, image_base64: BIG_RESULT }]])
  expect(button(w).element.disabled).toBe(false)
  expect(w.emitted('busy')?.at(-1)).toEqual([false])
})


it('an uncertain queue lookup failure retries the same job instead of enabling another generation', async () => {
  GetQueueJobResult.mockRejectedValueOnce(new Error('state unavailable'))
  const w = mount(ResolutionUpscaleOverlay, { props: { image: SMALL, inline: true } })
  await settle()
  await button(w).trigger('click')
  await settle()
  expect(w.find('[role="alert"]').exists()).toBe(true)
  expect(button(w).element.disabled).toBe(true)
  expect(w.emitted('busy')?.at(-1)).toEqual([true])
  expect(w.emitted('completed')).toBeUndefined()
  expect(w.props('image')).toBe(SMALL)

  const checkAgain = w.findAll('button').find(candidate => /Проверить снова|Check again/.test(candidate.text()))
  expect(checkAgain).toBeTruthy()
  GetQueueJobResult.mockResolvedValueOnce({ id: QUEUE_ID, status: 'completed', result: { session_item_id: RESULT_ITEM, image_base64: BIG_RESULT } })
  await checkAgain.trigger('click')
  await settle()
  expect(GetQueueJobResult.mock.calls.map(args => args[0])).toEqual([QUEUE_ID, QUEUE_ID])
  expect(EnqueueImageResolutionUpscale).toHaveBeenCalledTimes(1)
  expect(w.emitted('completed')).toEqual([[{ session_item_id: RESULT_ITEM, image_base64: BIG_RESULT }]])
  expect(button(w).element.disabled).toBe(false)
  expect(w.emitted('busy')?.at(-1)).toEqual([false])
})
