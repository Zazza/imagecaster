import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref, nextTick } from 'vue'
import { useZoomPan, ZOOM_MIN, ZOOM_MAX, ZOOM_PRESETS } from '../composables/useZoomPan.js'
import { t } from '../i18n/index.js'

function area(w, h) {
  const el = {
    clientWidth: w,
    clientHeight: h,
    setPointerCapture: vi.fn(),
    releasePointerCapture: vi.fn(),
    hasPointerCapture: vi.fn(() => true),
    getBoundingClientRect: () => ({ left: 0, top: 0, width: w, height: h, right: w, bottom: h }),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }
  return ref(el)
}

function setup(img = [1000, 1000], box = [500, 500]) {
  const areaRef = area(...box)
  const z = useZoomPan(areaRef)
  z.onImgLoad({ target: { naturalWidth: img[0], naturalHeight: img[1] } })
  return { z, areaRef }
}

function ptr(x, y, el) {
  return {
    clientX: x, clientY: y, pointerId: 1, button: 0, buttons: 1, isPrimary: true,
    pointerType: 'mouse',
    target: el, currentTarget: el,
    preventDefault: vi.fn(), stopPropagation: vi.fn(),
  }
}

function translate(style) {
  const m = /translate\(\s*(-?[\d.]+)px,\s*(-?[\d.]+)px\)/.exec(style.transform)
  return m ? [Number(m[1]), Number(m[2])] : null
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
})
afterEach(() => vi.unstubAllGlobals())

describe('useZoomPan constants', () => {
  it('exports bounds and presets', () => {
    expect(ZOOM_MIN).toBe(10)
    expect(ZOOM_MAX).toBe(500)
    expect(ZOOM_PRESETS).toEqual([30, 50, 100])
  })
})

describe('useZoomPan', () => {
  it('1: starts in fit mode with the fit label', () => {
    const z = useZoomPan(area(500, 500))
    expect(z.isFit.value).toBe(true)
    expect(z.zoomLabel.value).toBe(t('viewer.zoom_fit'))
  })

  it('2: onImgLoad records the natural image size', () => {
    const { z } = setup([1200, 800])
    expect(z.naturalW.value).toBe(1200)
    expect(z.naturalH.value).toBe(800)
  })

  it('2: fit percent uses the area size from areaRef', () => {
    const { z } = setup([1000, 1000], [500, 500])
    expect(Math.round(z.fitPercent())).toBe(50)
    const { z: z2 } = setup([1000, 1000], [250, 250])
    expect(Math.round(z2.fitPercent())).toBe(25)
  })

  it('3: zoomBy(10) from fit starts at the fit percent floored to tens', async () => {
    const { z } = setup([1000, 1000], [500, 500])
    z.zoomBy(10)
    await nextTick()
    expect(z.isFit.value).toBe(false)
    expect(z.zoomPct.value).toBe(60)
    expect(z.zoomLabel.value).toBe('60%')
  })

  it('3: fit percent not on a ten is floored before stepping', () => {
    const { z } = setup([1000, 1000], [570, 570])
    z.zoomBy(10)
    expect(z.zoomPct.value).toBe(60)
    const { z: z2 } = setup([1000, 1000], [570, 570])
    z2.zoomBy(-10)
    expect(z2.zoomPct.value).toBe(40)
  })

  it('4: enterZoom clamps to [10, 500]', () => {
    const { z } = setup()
    z.enterZoom(5)
    expect(z.zoomPct.value).toBe(10)
    z.enterZoom(9999)
    expect(z.zoomPct.value).toBe(500)
    z.enterZoom(0)
    expect(z.zoomPct.value).toBe(10)
    z.enterZoom(120)
    expect(z.zoomPct.value).toBe(120)
    expect(z.zoomLabel.value).toBe('120%')
  })

  it('4: zoomBy does not go past the bounds', () => {
    const { z } = setup()
    z.enterZoom(500)
    z.zoomBy(10)
    expect(z.zoomPct.value).toBe(500)
    z.enterZoom(10)
    z.zoomBy(-10)
    expect(z.zoomPct.value).toBe(10)
  })

  it('4: setFit returns to fit and resets pan', () => {
    const { z, areaRef } = setup()
    z.enterZoom(100)
    z.onPointerDown(ptr(100, 100, areaRef.value))
    z.onPointerMove(ptr(150, 130, areaRef.value))
    z.onPointerUp(ptr(150, 130, areaRef.value))
    expect(translate(z.layerStyle.value)).not.toEqual([0, 0])
    z.setFit()
    expect(z.isFit.value).toBe(true)
    expect(z.zoomLabel.value).toBe(t('viewer.zoom_fit'))
    expect(z.layerStyle.value).toBeNull()
    z.enterZoom(100)
    expect(translate(z.layerStyle.value)).toEqual([0, 0])
  })

  it('5: indicator click in fit mode toggles the presets menu', () => {
    const { z } = setup()
    expect(z.zoomMenuOpen.value).toBe(false)
    z.onZoomIndicatorClick()
    expect(z.zoomMenuOpen.value).toBe(true)
    expect(z.isFit.value).toBe(true)
    z.onZoomIndicatorClick()
    expect(z.zoomMenuOpen.value).toBe(false)
  })

  it('5: indicator click in percent mode returns to fit', () => {
    const { z } = setup()
    z.enterZoom(100)
    z.onZoomIndicatorClick()
    expect(z.isFit.value).toBe(true)
    expect(z.zoomLabel.value).toBe(t('viewer.zoom_fit'))
  })

  it('6: layerStyle is null in fit and a translate+scale transform in percent mode', () => {
    const { z } = setup()
    expect(z.layerStyle.value).toBeNull()
    z.enterZoom(100)
    const s = z.layerStyle.value
    expect(s).toBeTruthy()
    expect(s.transform).toMatch(/translate\(/)
    expect(s.transform).toMatch(/scale\(/)
  })

  it('7: panEnabled only when the zoomed image is larger than the area', () => {
    const { z } = setup([1000, 1000], [500, 500])
    expect(z.panEnabled.value).toBe(false)
    z.enterZoom(30)
    expect(z.panEnabled.value).toBe(false)
    z.enterZoom(100)
    expect(z.panEnabled.value).toBe(true)
  })

  it('7: drag shifts the image', () => {
    const { z, areaRef } = setup()
    z.enterZoom(100)
    z.onPointerDown(ptr(200, 200, areaRef.value))
    z.onPointerMove(ptr(240, 170, areaRef.value))
    z.onPointerUp(ptr(240, 170, areaRef.value))
    expect(translate(z.layerStyle.value)).toEqual([40, -30])
    expect(z.wasDragged()).toBe(true)
  })

  it('7: shift is clamped so the image never leaves the area', () => {
    const { z, areaRef } = setup([1000, 1000], [500, 500])
    z.enterZoom(100)
    z.onPointerDown(ptr(0, 0, areaRef.value))
    z.onPointerMove(ptr(5000, -5000, areaRef.value))
    z.onPointerUp(ptr(5000, -5000, areaRef.value))
    expect(translate(z.layerStyle.value)).toEqual([250, -250])
  })

  it('7: no pan when the image fits the area', () => {
    const { z, areaRef } = setup([1000, 1000], [500, 500])
    z.enterZoom(30)
    z.onPointerDown(ptr(0, 0, areaRef.value))
    z.onPointerMove(ptr(100, 100, areaRef.value))
    z.onPointerUp(ptr(100, 100, areaRef.value))
    expect(translate(z.layerStyle.value)).toEqual([0, 0])
  })

  it('7: wasDragged is false after a plain click and after a move of 4px or less', () => {
    const { z, areaRef } = setup()
    z.enterZoom(100)
    z.onPointerDown(ptr(100, 100, areaRef.value))
    z.onPointerUp(ptr(100, 100, areaRef.value))
    expect(z.wasDragged()).toBe(false)
    z.onPointerDown(ptr(100, 100, areaRef.value))
    z.onPointerMove(ptr(103, 100, areaRef.value))
    z.onPointerUp(ptr(103, 100, areaRef.value))
    expect(z.wasDragged()).toBe(false)
  })

  it('8: wheel with Ctrl prevents default and zooms', () => {
    const { z } = setup()
    const e = { ctrlKey: true, deltaY: -100, deltaX: 0, clientX: 250, clientY: 250, preventDefault: vi.fn() }
    z.onWheel(e)
    expect(e.preventDefault).toHaveBeenCalled()
    expect(z.isFit.value).toBe(false)
    expect(z.zoomPct.value).toBeGreaterThan(50)
    const pct = z.zoomPct.value
    z.onWheel({ ...e, deltaY: 100, preventDefault: vi.fn() })
    expect(z.zoomPct.value).toBeLessThan(pct)
  })

  it('8: wheel without Ctrl does nothing', () => {
    const { z } = setup()
    const e = { ctrlKey: false, deltaY: -100, deltaX: 0, clientX: 250, clientY: 250, preventDefault: vi.fn() }
    z.onWheel(e)
    expect(e.preventDefault).not.toHaveBeenCalled()
    expect(z.isFit.value).toBe(true)
    z.enterZoom(100)
    z.onWheel(e)
    expect(z.zoomPct.value).toBe(100)
  })
})
