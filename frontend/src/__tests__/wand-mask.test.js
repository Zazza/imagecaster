import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useWandMask } from '../composables/useWandMask.js'

const WIDTH = 8
const HEIGHT = 4
const scopes = []
let images = []
class ManualImage {
  constructor() { images.push(this) }
  set src(value) { this.url = value }
  load(width = WIDTH, height = HEIGHT) { Object.assign(this, { naturalWidth: width, naturalHeight: height, width, height }); this.onload?.() }
}
const settle = async () => { await Promise.resolve(); await nextTick(); await Promise.resolve() }
function region(...indices) { const data = new Uint8Array(WIDTH * HEIGHT); indices.forEach(index => { data[index] = 255 }); return { width: WIDTH, height: HEIGHT, data } }
function deferred() { let resolve; const promise = new Promise(yes => { resolve = yes }); return { promise, resolve } }
async function setup(predict = vi.fn().mockResolvedValue(region(9))) {
  const source = ref('PHOTO_A'), predictor = { predict, dispose: vi.fn() }, scope = effectScope()
  scopes.push(scope)
  const mask = scope.run(() => useWandMask(source, { createPredictor: () => predictor }))
  await settle(); images.at(-1)?.load(); await settle()
  return { source, predictor, scope, mask }
}
function selected(mask) {
  const canvas = mask.maskCanvas.value
  if (!canvas) return []
  const data = canvas.getContext('2d').getImageData().data
  return Array.from({ length: canvas.width * canvas.height }, (_, index) => index).filter(index => data[index * 4 + 3] > 0)
}
beforeEach(() => {
  images = []; vi.stubGlobal('Image', ManualImage)
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function () {
    if (!this.ctx) this.ctx = {
      createImageData: (width, height) => ({ width, height, data: new Uint8ClampedArray(width * height * 4) }),
      putImageData(image) { this.image = image },
      getImageData() { return this.image || { data: new Uint8ClampedArray(WIDTH * HEIGHT * 4) } },
      clearRect() { this.image = null }, drawImage() {},
    }
    return this.ctx
  })
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,MASK')
})
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('click selected regions', () => {
  it('replaces on a plain click and ORs on Shift, sending each point independently', async () => {
    const predict = vi.fn().mockResolvedValueOnce(region(1, 2)).mockResolvedValueOnce(region(2, 7)).mockResolvedValueOnce(region(19))
    const { mask } = await setup(predict)
    await mask.selectAt(1, 1); expect(selected(mask)).toEqual([1, 2])
    await mask.selectAt(6, 2, true); expect(selected(mask)).toEqual([1, 2, 7])
    await mask.selectAt(3, 2); expect(selected(mask)).toEqual([19])
    expect(predict.mock.calls.map(call => call[1])).toEqual([[{ x: 1, y: 1, label: 1 }], [{ x: 6, y: 2, label: 1 }], [{ x: 3, y: 2, label: 1 }]])
    expect([mask.maskCanvas.value.width, mask.maskCanvas.value.height]).toEqual([WIDTH, HEIGHT])
    expect(mask.getMaskBase64()).toBe('MASK')
  })
  it('preserves a pending plain selection and subsequent quick Shift additions', async () => {
    const first = deferred(), second = deferred(), third = deferred()
    const predict = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise)
    const { mask } = await setup(predict)
    const requests = [mask.selectAt(1, 1), mask.selectAt(2, 1, true), mask.selectAt(3, 1, true)]
    await settle(); expect(mask.busy.value).toBe(true); expect(mask.getMaskBase64()).toBe('')
    first.resolve(region(1)); await settle(); second.resolve(region(2)); await settle(); third.resolve(region(3))
    await Promise.all(requests)
    expect(selected(mask)).toEqual([1, 2, 3]); expect(mask.busy.value).toBe(false)
  })
  it('a newer plain click invalidates every older pending addition', async () => {
    const old = deferred()
    const { mask } = await setup(vi.fn().mockReturnValueOnce(old.promise).mockResolvedValue(region(8)))
    const stale = mask.selectAt(1, 1); await settle()
    const addition = mask.selectAt(2, 1, true)
    const replacement = mask.selectAt(4, 1)
    old.resolve(region(1)); await Promise.all([stale, addition, replacement]); await settle()
    expect(selected(mask)).toEqual([8])
  })
  it('undo and redo restore bitmaps without running the model', async () => {
    const predict = vi.fn().mockResolvedValueOnce(region(1)).mockResolvedValueOnce(region(7))
    const { mask } = await setup(predict)
    await mask.selectAt(1, 1); await mask.selectAt(6, 1, true)
    expect(mask.canUndoMask.value).toBe(true)
    mask.undoMask(); expect(selected(mask)).toEqual([1]); expect(mask.canRedoMask.value).toBe(true)
    mask.redoMask(); expect(selected(mask)).toEqual([1, 7]); expect(predict).toHaveBeenCalledTimes(2)
  })
  it('can clear an empty selection that still has redo history', async () => {
    const { mask, predictor } = await setup()
    await mask.selectAt(1, 1)
    mask.undoMask()
    expect(mask.hasMask.value).toBe(false)
    expect(mask.canRedoMask.value).toBe(true)
    expect(mask.canClear.value).toBe(true)
    mask.clearMask()
    await settle(); images.at(-1)?.load(); await settle()
    expect(mask.hasMask.value).toBe(false)
    expect(mask.canRedoMask.value).toBe(false)
    expect(mask.canClear.value).toBe(false)
    expect(predictor.predict).toHaveBeenCalledTimes(1)
    expect(mask.getMaskBase64()).toBe('')
  })
  it.each(['clear', 'source', 'dispose', 'undo'])('discards in-flight selection after %s', async action => {
    const pending = deferred()
    const { mask, source, scope } = await setup(vi.fn().mockResolvedValueOnce(region(1)).mockReturnValueOnce(pending.promise))
    await mask.selectAt(1, 1)
    const request = mask.selectAt(2, 1, true); await settle()
    if (action === 'clear') mask.clearMask()
    if (action === 'source') { source.value = 'PHOTO_B'; await settle(); images.at(-1)?.load() }
    if (action === 'dispose') scope.stop()
    if (action === 'undo') mask.undoMask()
    pending.resolve(region(7)); await request; await settle()
    expect(selected(mask)).not.toContain(7)
    expect(mask.getMaskBase64()).toBe('')
  })
  it.each(['empty', 'error'])('blocks export after a new %s result and recovers on another click', async kind => {
    const predict = vi.fn().mockResolvedValueOnce(region(1))
    if (kind === 'empty') predict.mockResolvedValueOnce(region())
    else predict.mockRejectedValueOnce(new Error('decoder failed'))
    predict.mockResolvedValueOnce(region(7))
    const { mask } = await setup(predict)
    await mask.selectAt(1, 1); await mask.selectAt(2, 1)
    expect(mask.getMaskBase64()).toBe(''); expect(mask.hasMask.value).toBe(false)
    await mask.selectAt(3, 1); expect(mask.hasMask.value).toBe(true); expect(mask.error.value).toBe('')
  })
  it('ignores invalid coordinates without inference or history changes', async () => {
    const { mask, predictor } = await setup()
    for (const [x, y] of [[-1, 1], [1, -1], [WIDTH, 1], [1, HEIGHT], [NaN, 1], [Infinity, 1]]) await mask.selectAt(x, y)
    expect(predictor.predict).not.toHaveBeenCalled(); expect(mask.hasMask.value).toBe(false); expect(mask.canUndoMask.value).toBe(false)
  })
})
