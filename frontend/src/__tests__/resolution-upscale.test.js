import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import ResolutionUpscaleOverlay from '../components/ResolutionUpscaleOverlay.vue'
import { EnqueueResolutionUpscale, EnqueueImageResolutionUpscale, GetQueue, GetQueueJobResult, GetSessionImage } from '../wailsjs/go/main/App.js'
import { EventsEmit, clearEventMocks } from './mocks/runtime'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
const ITEM_A = 31
const ITEM_B = 32
const QUEUE_ID = 91
let dimensions
beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  dimensions = { width: 800, height: 1200 }
  vi.stubGlobal('Image', class {
    set src(_) { Object.assign(this, { naturalWidth: dimensions.width, naturalHeight: dimensions.height }); queueMicrotask(() => this.onload?.()) }
  })
  GetQueue.mockResolvedValue([{ id: QUEUE_ID, status: 'running' }])
  GetQueueJobResult.mockResolvedValue({ id: QUEUE_ID, status: 'running' })
})
afterEach(() => vi.unstubAllGlobals())
function deferred() { let resolve; const promise = new Promise(r => { resolve = r }); return { resolve, promise } }
async function overlay() { const w = mount(ResolutionUpscaleOverlay, { props: { image: 'AAA', itemId: ITEM_A } }); await settle(); return w }
const button = (w, label) => w.findAll('button').find(b => b.text().startsWith(label))
describe('explicit resolution upscale', () => {
  it('does not enqueue on mount or selection change; disables sizes at or below the original', async () => {
    dimensions = { width: 2560, height: 1920 }
    const w = await overlay()
    expect(button(w, '1080p').attributes('disabled')).toBeDefined()
    expect(button(w, '2K').attributes('disabled')).toBeDefined()
    expect(button(w, '4K').attributes('disabled')).toBeUndefined()
    await w.setProps({ image: 'BBB', itemId: ITEM_B })
    await settle()
    expect(EnqueueResolutionUpscale).not.toHaveBeenCalled()
  })
  it('file source uses its exact snapshot only after an explicit click', async () => {
    EnqueueImageResolutionUpscale.mockResolvedValue(QUEUE_ID)
    const w = mount(ResolutionUpscaleOverlay, { props: { image: 'FILE' } })
    await settle()
    expect(EnqueueImageResolutionUpscale).not.toHaveBeenCalled()
    await button(w, '2K').trigger('click')
    await settle()
    expect(EnqueueImageResolutionUpscale).toHaveBeenCalledWith('FILE', 2560)
    expect(EnqueueResolutionUpscale).not.toHaveBeenCalled()
  })
  it('loads the completed image from the event when the queue panel has no result', async () => {
    const RESULT_ITEM = 99
    const result = { session_item_id: RESULT_ITEM, info: '{"width":1920}' }
    EnqueueResolutionUpscale.mockResolvedValue(QUEUE_ID)
    GetSessionImage.mockResolvedValue('UPSCALED')
    const w = await overlay()
    await button(w, '1080p').trigger('click')
    await settle()
    GetQueue.mockResolvedValue([{ id: QUEUE_ID, status: 'completed', result: '' }])
    EventsEmit('queue:completed', { job_id: QUEUE_ID, result })
    await settle()
    expect(GetSessionImage).toHaveBeenCalledWith(RESULT_ITEM)
    expect(w.emitted('completed')).toEqual([[{ ...result, image_base64: 'UPSCALED' }]])
  })
  it('recovers a completion emitted before the enqueue reply using the addressed job result', async () => {
    const RESULT_ITEM = 99
    const result = { session_item_id: RESULT_ITEM, info: '{}' }
    const queued = deferred()
    EnqueueResolutionUpscale.mockReturnValue(queued.promise)
    GetSessionImage.mockResolvedValue('UPSCALED')
    const w = await overlay()
    await button(w, '1080p').trigger('click')
    GetQueue.mockResolvedValue([{ id: QUEUE_ID, status: 'completed' }])
    GetQueueJobResult.mockResolvedValue({ id: QUEUE_ID, status: 'completed', result: JSON.stringify(result) })
    EventsEmit('queue:completed', { job_id: QUEUE_ID, result })
    await settle()
    queued.resolve(QUEUE_ID)
    await settle()
    expect(GetQueueJobResult).toHaveBeenCalledWith(QUEUE_ID)
    expect(w.emitted('completed')).toEqual([[{ ...result, image_base64: 'UPSCALED' }]])
  })
  it('selection changed while enqueue waits never receives the old result', async () => {
    const queued = deferred()
    EnqueueResolutionUpscale.mockReturnValue(queued.promise)
    const w = await overlay()
    await button(w, '1080p').trigger('click')
    expect(EnqueueResolutionUpscale).toHaveBeenCalledWith(ITEM_A, 1920)
    await w.setProps({ image: 'BBB', itemId: ITEM_B })
    GetQueueJobResult.mockResolvedValue({ id: QUEUE_ID, status: 'completed', result: { session_item_id: 99, image_base64: 'OLD' } })
    queued.resolve(QUEUE_ID)
    await settle()
    EventsEmit('queue:completed', { job_id: QUEUE_ID })
    await settle()
    expect(w.emitted('completed')).toBeUndefined()
  })
  it('selection changed during result image fetch never receives the old result', async () => {
    EnqueueResolutionUpscale.mockResolvedValue(QUEUE_ID)
    const resultImage = deferred()
    GetSessionImage.mockReturnValue(resultImage.promise)
    const w = await overlay()
    await button(w, '1080p').trigger('click')
    await settle()
    GetQueueJobResult.mockResolvedValue({ id: QUEUE_ID, status: 'completed', result: { session_item_id: 99 } })
    EventsEmit('queue:completed', { job_id: QUEUE_ID })
    await settle()
    expect(GetSessionImage).toHaveBeenCalledWith(99)
    await w.setProps({ image: 'BBB', itemId: ITEM_B })
    resultImage.resolve('OLD')
    await settle()
    expect(w.emitted('completed')).toBeUndefined()
  })
})
