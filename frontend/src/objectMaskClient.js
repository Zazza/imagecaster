import { api } from './api.js'
import { b64Url } from './imageDataUrl.js'
import { MAX_OBJECT_MASK_PIXELS } from './objectMaskLimits.js'

const ENCODER_SIDE = 1024

export function createObjectMaskPredictor() {
  let worker = null
  let sequence = 0
  let disposed = false
  let prepared = null
  const pending = new Map()
  let queue = Promise.resolve()

  function dispose() {
    disposed = true
    worker?.terminate()
    worker = null
    for (const request of pending.values()) request.reject(new Error('object_mask.cancelled'))
    pending.clear()
  }

  async function execute(source, hints) {
    if (disposed) throw new Error('object_mask.cancelled')
    prepared ||= api.prepareObjectMask()
    let models
    try {
      models = await prepared
    } catch (e) {
      prepared = null
      throw e
    }
    if (disposed) throw new Error('object_mask.cancelled')
    if (!worker) {
      worker = new Worker(new URL('./objectMask.worker.js', import.meta.url), { type: 'module' })
      worker.onmessage = ({ data }) => {
        const request = pending.get(data.id)
        if (!request) return
        pending.delete(data.id)
        if (data.error) request.reject(new Error(data.error))
        else request.resolve(data)
      }
      worker.onerror = event => {
        for (const request of pending.values()) request.reject(new Error(event.message || 'object_mask.worker_failed'))
        pending.clear()
        worker?.terminate()
        worker = null
      }
    }
    const img = new Image()
    await new Promise((resolve, reject) => {
      img.onload = resolve
      img.onerror = () => reject(new Error('object_mask.image_failed'))
      img.src = b64Url(source)
    })
    if (disposed) throw new Error('object_mask.cancelled')
    const width = img.naturalWidth
    const height = img.naturalHeight
    if (!width || !height || width * height > MAX_OBJECT_MASK_PIXELS) throw new Error('object_mask.image_size')
    const scale = ENCODER_SIDE / Math.max(width, height)
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(width * scale))
    canvas.height = Math.max(1, Math.round(height * scale))
    const ctx = canvas.getContext('2d')
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
    const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data
    const id = ++sequence
    return new Promise((resolve, reject) => {
      pending.set(id, { resolve, reject })
      try {
        worker.postMessage({ id, source, width, height, scaledWidth: canvas.width, scaledHeight: canvas.height,
          pixels: pixels.buffer, hints, models }, [pixels.buffer])
      } catch (e) {
        pending.delete(id)
        reject(e)
      }
    })
  }

  function predict(source, hints) {
    const operation = queue.then(() => execute(source, hints))
    queue = operation.catch(e => { if (!disposed) console.warn('Object selection:', e.message) })
    return operation
  }

  return { predict, dispose }
}
