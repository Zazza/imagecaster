import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useMaskCanvas } from '../composables/useMaskCanvas.js'

let images = []

class ManualImage {
  constructor() {
    this.onload = null
    this.naturalWidth = 0
    this.naturalHeight = 0
    this.src = ''
    images.push(this)
  }
  load(w, h) {
    this.naturalWidth = w
    this.naturalHeight = h
    this.onload?.()
  }
}

function stateCanvasContext(canvas) {
  return {
    canvas,
    strokes: 0,
    globalCompositeOperation: '',
    fillStyle: '',
    beginPath() {},
    arc() {},
    fill() { this.strokes++ },
    clearRect() { this.strokes = 0 },
    getImageData(x, y, w, h) {
      return { strokes: this.strokes, data: new Uint8ClampedArray(w * h * 4), width: w, height: h }
    },
    putImageData(img) { this.strokes = img.strokes },
    createImageData(w, h) { return { data: new Uint8ClampedArray(w * h * 4), width: w, height: h } },
  }
}

const strokesOn = m => m.maskCanvas.value.getContext('2d').strokes

function draw(m, n = 1) {
  for (let i = 0; i < n; i++) {
    m.beginStroke(5, 5, 3)
    m.endStroke()
  }
}

async function readyMask(w = 10, h = 8) {
  const m = useMaskCanvas()
  m.applySource('AAAA')
  images.at(-1).load(w, h)
  return m
}

let origImage

beforeEach(() => {
  images = []
  origImage = globalThis.Image
  globalThis.Image = ManualImage
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function () {
    if (!this.__ctx) this.__ctx = stateCanvasContext(this)
    return this.__ctx
  })
})

afterEach(() => {
  globalThis.Image = origImage
  vi.restoreAllMocks()
})

describe('useMaskCanvas history', () => {
  it('undo restores the previous drawing, redo brings it back', async () => {
    const m = await readyMask()
    draw(m, 3)
    expect(strokesOn(m)).toBe(3)
    m.undoMask()
    m.undoMask()
    expect(strokesOn(m)).toBe(1)
    expect(m.canRedoMask.value).toBe(true)
    m.redoMask()
    expect(strokesOn(m)).toBe(2)
    m.undoMask()
    m.undoMask()
    expect(strokesOn(m)).toBe(0)
    expect(m.hasMask.value).toBe(false)
  })

  it('a new stroke after undo drops the redo branch', async () => {
    const m = await readyMask()
    draw(m, 2)
    m.undoMask()
    draw(m)
    expect(m.canRedoMask.value).toBe(false)
    m.redoMask()
    expect(strokesOn(m)).toBe(2)
  })

  it('keeps the last 30 steps: after 35 strokes only 30 undos are possible', async () => {
    const m = await readyMask()
    draw(m, 35)
    expect(m.maskHistory.value).toHaveLength(30)
    for (let i = 0; i < 40; i++) m.undoMask()
    expect(strokesOn(m)).toBe(5)
    expect(m.hasMask.value).toBe(false)
  })

  it('undo and redo with empty history change nothing', async () => {
    const m = await readyMask()
    const v = m.maskVersion.value
    m.undoMask()
    m.redoMask()
    expect(m.maskVersion.value).toBe(v)
  })

  it('clear wipes the drawing and the history', async () => {
    const m = await readyMask()
    draw(m, 4)
    m.clearMask()
    expect(strokesOn(m)).toBe(0)
    expect(m.hasMask.value).toBe(false)
    m.undoMask()
    expect(strokesOn(m)).toBe(0)
  })
})

describe('useMaskCanvas source loading', () => {
  it('the canvas takes the size of the loaded photo', async () => {
    const m = await readyMask(64, 48)
    expect([m.maskCanvas.value.width, m.maskCanvas.value.height]).toEqual([64, 48])
  })

  it('a slower load of an older photo is ignored', async () => {
    const m = useMaskCanvas()
    m.applySource('OLD')
    const older = images.at(-1)
    m.applySource('NEW')
    const newer = images.at(-1)
    newer.load(20, 10)
    draw(m, 2)
    older.load(300, 200)
    expect([m.maskCanvas.value.width, m.maskCanvas.value.height]).toEqual([20, 10])
    expect(strokesOn(m)).toBe(2)
    expect(m.hasMask.value).toBe(true)
  })

  it('discarding the mask cancels a pending load', async () => {
    const m = useMaskCanvas()
    m.applySource('PHOTO')
    const pending = images.at(-1)
    m.discardMask()
    pending.load(10, 10)
    expect(m.maskCanvas.value).toBeNull()
  })

  it('same-size source with keep flag keeps the drawing; another size starts clean', async () => {
    const m = await readyMask(10, 8)
    draw(m, 2)
    m.applySource('SAME', true)
    images.at(-1).load(10, 8)
    expect(strokesOn(m)).toBe(2)
    expect(m.hasMask.value).toBe(true)

    m.applySource('BIGGER', true)
    images.at(-1).load(20, 16)
    expect(strokesOn(m)).toBe(0)
    expect(m.hasMask.value).toBe(false)
  })

  it('empty source discards the mask', async () => {
    const m = await readyMask()
    draw(m)
    m.applySource('')
    expect(m.maskCanvas.value).toBeNull()
    expect(m.hasMask.value).toBe(false)
  })

  it('jpeg base64 is loaded as a jpeg data url', () => {
    const m = useMaskCanvas()
    m.applySource('/9j/abc')
    expect(images.at(-1).src).toBe('data:image/jpeg;base64,/9j/abc')
  })
})
