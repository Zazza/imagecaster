import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createObjectMaskPredictor } from '../objectMaskClient.js'
import { useFakeCanvas, useFakeImage, FAKE_IMAGE_WIDTH, FAKE_IMAGE_HEIGHT } from './helpers/dom.js'

const { prepareObjectMask } = vi.hoisted(() => ({ prepareObjectMask: vi.fn() }))
vi.mock('../api.js', () => ({ api: { prepareObjectMask } }))
useFakeImage()
useFakeCanvas()
let workers
let predictors
class FakeWorker {
  constructor() { workers.push(this) }
  postMessage(message) {
    this.message = message
    queueMicrotask(() => this.onmessage?.({ data: {
      id: message.id,
      width: FAKE_IMAGE_WIDTH,
      height: FAKE_IMAGE_HEIGHT,
      data: new Uint8Array(FAKE_IMAGE_WIDTH * FAKE_IMAGE_HEIGHT).fill(255),
    } }))
  }
  terminate = vi.fn()
}
function makePredictor() { const predictor = createObjectMaskPredictor(); predictors.push(predictor); return predictor }
beforeEach(() => {
  workers = []
  predictors = []
  prepareObjectMask.mockReset()
  vi.stubGlobal('Worker', FakeWorker)
})
afterEach(() => { predictors.forEach(predictor => predictor.dispose()); vi.unstubAllGlobals() })

describe('object model preparation lifecycle', () => {
  it('retries a failed model download and sends no photo to the preparation RPC', async () => {
    const models = { encoder: '/api/object-mask/encoder.onnx', decoder: '/api/object-mask/decoder.onnx' }
    prepareObjectMask.mockRejectedValueOnce(new Error('network unavailable')).mockResolvedValueOnce(models)
    const predictor = makePredictor()
    const hints = [{ x: 2, y: 1, label: 1 }]
    await expect(predictor.predict('PRIVATE_PHOTO', hints)).rejects.toThrow('network unavailable')
    expect(workers).toHaveLength(0)
    const mask = await predictor.predict('PRIVATE_PHOTO', hints)
    expect(prepareObjectMask).toHaveBeenCalledTimes(2)
    expect(prepareObjectMask.mock.calls).toEqual([[], []])
    expect(workers).toHaveLength(1)
    expect(workers[0].message.models).toEqual(models)
    expect(mask.width).toBe(FAKE_IMAGE_WIDTH)
    expect(mask.height).toBe(FAKE_IMAGE_HEIGHT)
    expect(mask.data.some(Boolean)).toBe(true)
  })
  it('does not create a worker after disposal during model preparation', async () => {
    let finishPreparation
    prepareObjectMask.mockReturnValue(new Promise(resolve => { finishPreparation = resolve }))
    const predictor = makePredictor()
    const prediction = predictor.predict('PRIVATE_PHOTO', [{ x: 2, y: 1, label: 1 }])
    await Promise.resolve()
    expect(prepareObjectMask.mock.calls).toEqual([[]])
    predictor.dispose()
    const rejection = expect(prediction).rejects.toThrow('object_mask.cancelled')
    finishPreparation({ encoder: '/api/object-mask/encoder.onnx', decoder: '/api/object-mask/decoder.onnx' })
    await rejection
    expect(workers).toHaveLength(0)
  })
})
