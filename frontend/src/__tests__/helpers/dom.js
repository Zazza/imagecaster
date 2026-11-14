import { vi, beforeEach, afterEach } from 'vitest'

export const FAKE_IMAGE_WIDTH = 64
export const FAKE_IMAGE_HEIGHT = 48
export const FAKE_DATA_URL = 'data:image/png;base64,TUFTSw=='

export class FakeImage {
  constructor() {
    this.onload = null
    this.onerror = null
    this.naturalWidth = FAKE_IMAGE_WIDTH
    this.naturalHeight = FAKE_IMAGE_HEIGHT
    this.width = FAKE_IMAGE_WIDTH
    this.height = FAKE_IMAGE_HEIGHT
    this.complete = false
    this._src = ''
  }
  set src(v) {
    this._src = v
    setTimeout(() => {
      this.complete = true
      if (this.onload) this.onload()
    }, 0)
  }
  get src() {
    return this._src
  }
  decode() {
    return Promise.resolve()
  }
}

function blankImageData(w, h) {
  return { data: new Uint8ClampedArray(Math.max(1, (w || 1) * (h || 1) * 4)), width: w, height: h }
}

export function fakeCanvasContext(canvas, { trackPaint = false } = {}) {
  const ctx = {
    canvas,
    painted: false,
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    lineCap: '',
    lineJoin: '',
    globalCompositeOperation: 'source-over',
    getImageData: (x, y, w, h) => {
      const img = blankImageData(w, h)
      if (trackPaint && ctx.painted) img.data.fill(255)
      return img
    },
    putImageData: vi.fn(img => {
      if (trackPaint) ctx.painted = !!(img && img.data && img.data.some(v => v > 0))
    }),
    createImageData: (w, h) => blankImageData(w, h),
    clearRect: vi.fn(() => { if (trackPaint) ctx.painted = false }),
    fillRect: vi.fn(),
    beginPath: vi.fn(),
    closePath: vi.fn(),
    arc: vi.fn(() => { if (trackPaint) ctx.painted = true }),
    fill: vi.fn(() => { if (trackPaint) ctx.painted = true }),
    stroke: vi.fn(() => { if (trackPaint) ctx.painted = true }),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    drawImage: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
  }
  return ctx
}

export function installFakeImage() {
  const origGlobal = globalThis.Image
  const origWindow = window.Image
  globalThis.Image = FakeImage
  window.Image = FakeImage
  return () => {
    globalThis.Image = origGlobal
    window.Image = origWindow
  }
}

export function installFakeCanvas({ trackPaint = false, layout = false } = {}) {
  const spies = [
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function () {
      if (!this.__ctx) this.__ctx = fakeCanvasContext(this, { trackPaint })
      return this.__ctx
    }),
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockImplementation(() => FAKE_DATA_URL),
  ]
  let addedCapture = false
  if (layout) {
    spies.push(
      vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(() => ({
        x: 0, y: 0, left: 0, top: 0,
        right: FAKE_IMAGE_WIDTH, bottom: FAKE_IMAGE_HEIGHT,
        width: FAKE_IMAGE_WIDTH, height: FAKE_IMAGE_HEIGHT,
        toJSON() {},
      })),
      vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => FAKE_IMAGE_WIDTH),
      vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(() => FAKE_IMAGE_HEIGHT),
    )
    if (!Element.prototype.setPointerCapture) {
      Element.prototype.setPointerCapture = function () {}
      Element.prototype.releasePointerCapture = function () {}
      addedCapture = true
    }
  }
  return () => {
    spies.forEach(s => s.mockRestore())
    if (addedCapture) {
      delete Element.prototype.setPointerCapture
      delete Element.prototype.releasePointerCapture
    }
  }
}

export function useFakeImage() {
  let restore = null
  beforeEach(() => { restore = installFakeImage() })
  afterEach(() => { restore?.(); restore = null })
}

export function useFakeCanvas(options = {}) {
  let restore = null
  beforeEach(() => { restore = installFakeCanvas(options) })
  afterEach(() => { restore?.(); restore = null })
}
