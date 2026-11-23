import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useObjectMask } from '../composables/useObjectMask.js'

let scopes = []
let images = []
class ManualImage {
  constructor() { images.push(this) }
  set src(value) { this.url = value }
  load(width = 8, height = 4) {
    this.naturalWidth = width
    this.naturalHeight = height
    this.width = width
    this.height = height
    this.onload?.()
  }
}
const settle = async () => { await Promise.resolve(); await nextTick(); await Promise.resolve() }
const result = (filled = true) => ({ width: 8, height: 4, data: new Uint8Array(32).fill(filled ? 255 : 0) })
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
async function setup(predict = vi.fn().mockResolvedValue(result())) {
  const source = ref('PHOTO_A')
  const predictor = { predict, dispose: vi.fn() }
  const scope = effectScope()
  scopes.push(scope)
  const mask = scope.run(() => useObjectMask(source, { createPredictor: () => predictor }))
  await settle()
  images.at(-1)?.load()
  await settle()
  return { mask, source, predictor, scope }
}
async function stroke(mask, x = 2, y = 1) {
  mask.beginStroke(x, y, 1)
  mask.endStroke()
  await settle()
}
beforeEach(() => {
  images = []
  vi.stubGlobal('Image', ManualImage)
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function () {
    if (!this.ctx) this.ctx = {
      createImageData: (w, h) => ({ width: w, height: h, data: new Uint8ClampedArray(w * h * 4) }),
      putImageData(image) { this.image = image },
      getImageData() { return this.image || { data: new Uint8ClampedArray(128) } },
      clearRect() { this.image = null }, drawImage() {},
      beginPath() {}, arc() {}, fill() {}, moveTo() {}, lineTo() {}, stroke() {}, save() {}, restore() {},
    }
    return this.ctx
  })
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,MASK')
})
afterEach(() => { scopes.forEach(scope => scope.stop()); scopes = []; vi.restoreAllMocks(); vi.unstubAllGlobals() })

const alpha = (mask, x, y) => mask.maskCanvas.value.getContext('2d').getImageData(0, 0, 8, 4).data[(y * 8 + x) * 4 + 3]
describe('painted object selection contract', () => {
  it('requires explicit detection after paint and history edits', async () => {
    const { mask, predictor } = await setup()
    await stroke(mask, 1, 1)
    expect(predictor.predict).not.toHaveBeenCalled()
    expect(mask.hasPaint.value).toBe(true)
    expect(mask.hasMask.value).toBe(false)
    expect(mask.getMaskBase64()).toBe('')
    await mask.detect()
    expect(mask.hasMask.value).toBe(true)
    expect(mask.getMaskBase64()).toBe('MASK')
    await stroke(mask, 6, 1)
    mask.undoMask(); mask.redoMask(); await settle()
    expect(predictor.predict).toHaveBeenCalledTimes(1)
    expect(mask.hasMask.value).toBe(false)
    expect(mask.getMaskBase64()).toBe('')
  })
  it('clips an all-ones model to disconnected paint and preserves the gap', async () => {
    const { mask } = await setup()
    await stroke(mask, 1, 1); await stroke(mask, 6, 1)
    await mask.detect()
    expect([mask.maskCanvas.value.width, mask.maskCanvas.value.height]).toEqual([8, 4])
    expect(alpha(mask, 1, 1)).toBeGreaterThan(0)
    expect(alpha(mask, 6, 1)).toBeGreaterThan(0)
    for (const [x, y] of [[3, 1], [4, 1], [0, 3], [7, 3]]) expect(alpha(mask, x, y)).toBe(0)
  })
  it('paints a continuous stroke between distant pointer samples without inference', async () => {
    const { mask, predictor } = await setup()
    mask.beginStroke(1, 1, 1)
    mask.strokeTo(6, 1, 1)
    mask.endStroke()
    await settle()
    for (let x = 1; x <= 6; x++) expect(alpha(mask, x, 1)).toBeGreaterThan(0)
    expect(alpha(mask, 3, 3)).toBe(0)
    expect(predictor.predict).not.toHaveBeenCalled()
    expect(mask.getMaskBase64()).toBe('')
    await mask.detect()
    for (let x = 1; x <= 6; x++) expect(alpha(mask, x, 1)).toBeGreaterThan(0)
    expect(alpha(mask, 3, 3)).toBe(0)
  })
  it('keeps erased pixels outside the final mask', async () => {
    const { mask } = await setup()
    mask.beginStroke(3, 2, 2); mask.endStroke(); await settle()
    mask.exclude.value = true
    await stroke(mask, 3, 2)
    await mask.detect()
    expect(alpha(mask, 3, 2)).toBe(0)
    expect(mask.hasMask.value).toBe(true)
  })
  it('re-detects from original paint instead of the previous narrow prediction', async () => {
    const narrow = result(false); narrow.data[9] = 255
    const predict = vi.fn().mockResolvedValueOnce(narrow).mockResolvedValueOnce(result())
    const { mask } = await setup(predict)
    await stroke(mask, 1, 1); await stroke(mask, 6, 1)
    await mask.detect()
    expect(alpha(mask, 6, 1)).toBe(0)
    await mask.detect()
    expect(alpha(mask, 6, 1)).toBeGreaterThan(0)
  })
  it.each(['paint', 'clear', 'source', 'dispose'])('discards an async result after %s', async edit => {
    const pending = deferred()
    const { mask, source, scope } = await setup(vi.fn().mockReturnValue(pending.promise))
    await stroke(mask)
    const detection = mask.detect(); await settle()
    expect(mask.busy.value).toBe(true)
    expect(mask.getMaskBase64()).toBe('')
    if (edit === 'paint') await stroke(mask, 6, 1)
    if (edit === 'clear') mask.clearMask()
    if (edit === 'source') { source.value = 'PHOTO_B'; await settle(); images.at(-1)?.load() }
    if (edit === 'dispose') scope.stop()
    pending.resolve(result()); await detection; await settle()
    expect(mask.hasMask.value).toBe(false)
    expect(mask.getMaskBase64()).toBe('')
  })
  it('blocks empty and failed detections and permits explicit recovery', async () => {
    const predict = vi.fn().mockResolvedValueOnce(result(false)).mockRejectedValueOnce(new Error('decoder failed')).mockResolvedValueOnce(result())
    const { mask } = await setup(predict)
    await stroke(mask)
    await mask.detect()
    expect(mask.hasMask.value).toBe(false)
    await mask.detect()
    expect(String(mask.error.value)).toContain('decoder failed')
    expect(mask.getMaskBase64()).toBe('')
    await mask.detect()
    expect(mask.hasMask.value).toBe(true)
    expect(mask.error.value).toBe('')
  })
  it('ignores strokes outside the photo', async () => {
    const { mask, predictor } = await setup()
    for (const [x, y] of [[-1, 1], [1, -1], [8, 1], [1, 4]]) await stroke(mask, x, y)
    expect(mask.hasPaint.value).toBe(false)
    expect(mask.canUndoMask.value).toBe(false)
    expect(predictor.predict).not.toHaveBeenCalled()
  })
})
