import { ref, computed, watch, onScopeDispose } from 'vue'
import { createObjectMaskPredictor } from '../objectMaskClient.js'
import { b64Url } from '../imageDataUrl.js'
import { MAX_OBJECT_MASK_PIXELS } from '../objectMaskLimits.js'

const DEFAULT_BRUSH_SIZE = 40
const POINT_SPACING = 24
const MAX_HINTS = 64
const OVERLAY_ALPHA = 150
const OPAQUE_ALPHA = 255
const CHANNELS = 4
const HISTORY_LIMIT = 30
const HISTORY_BYTE_LIMIT = 64 * 1024 * 1024
const INCLUDE_LABEL = 1
const EXCLUDE_LABEL = 0

export function useObjectMask(source, { createPredictor = createObjectMaskPredictor } = {}) {
  const maskCanvas = ref(null)
  const maskVersion = ref(0)
  const brushSize = ref(DEFAULT_BRUSH_SIZE)
  const busy = ref(false)
  const error = ref('')
  const hasMask = ref(false)
  const hasPaint = ref(false)
  const exclude = ref(false)
  const hints = ref([])
  const history = ref([])
  const redo = ref([])
  const canUndoMask = computed(() => history.value.length > 0)
  const canRedoMask = computed(() => redo.value.length > 0)
  let paint = new Uint8Array(0)
  let selected = null
  let predictor = null
  let revision = 0
  let stroke = null
  let disposed = false

  function render(bitmap = paint) {
    const canvas = maskCanvas.value
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    const pixels = ctx.createImageData(canvas.width, canvas.height)
    for (let i = 0; i < bitmap.length; i++) {
      if (!bitmap[i]) continue
      const p = i * CHANNELS
      pixels.data[p] = OPAQUE_ALPHA
      pixels.data[p + 1] = OPAQUE_ALPHA
      pixels.data[p + 2] = OPAQUE_ALPHA
      pixels.data[p + 3] = OVERLAY_ALPHA
    }
    ctx.putImageData(pixels, 0, 0)
    maskVersion.value++
  }

  function invalidate() {
    revision++
    hasMask.value = false
    selected = null
    busy.value = false
    error.value = ''
  }

  function clearMask() {
    invalidate()
    maskCanvas.value = null
    maskVersion.value++
    predictor?.dispose()
    predictor = null
    paint = new Uint8Array(0)
    hasPaint.value = false
    hints.value = []
    history.value = []
    redo.value = []
    stroke = null
    if (!source.value || disposed) return
    const request = revision
    const img = new Image()
    img.onload = () => {
      if (disposed || request !== revision) return
      if (!img.naturalWidth || !img.naturalHeight || img.naturalWidth * img.naturalHeight > MAX_OBJECT_MASK_PIXELS) {
        error.value = 'object_mask.image_size'
        return
      }
      const canvas = document.createElement('canvas')
      canvas.width = img.naturalWidth
      canvas.height = img.naturalHeight
      paint = new Uint8Array(canvas.width * canvas.height)
      maskCanvas.value = canvas
      maskVersion.value++
    }
    img.onerror = () => { if (request === revision) error.value = 'object_mask.image_failed' }
    img.src = b64Url(source.value)
  }

  watch(source, clearMask, { flush: 'sync', immediate: true })
  onScopeDispose(() => { disposed = true; clearMask() })

  function promptHints(bitmap, width) {
    const points = hints.value.filter(p => Boolean(bitmap[Math.floor(p.y) * width + Math.floor(p.x)]) === (p.label === INCLUDE_LABEL))
    if (!points.some(p => p.label === INCLUDE_LABEL)) {
      const first = bitmap.findIndex(Boolean)
      if (first >= 0) points.unshift({ x: first % width, y: Math.floor(first / width), label: INCLUDE_LABEL })
    }
    return points.slice(0, MAX_HINTS).map(p => ({ ...p }))
  }

  async function detect() {
    if (busy.value || stroke || !hasPaint.value || !source.value || disposed) return
    invalidate()
    render()
    const request = revision
    const image = source.value
    const width = maskCanvas.value.width
    const height = maskCanvas.value.height
    const painted = paint.slice()
    busy.value = true
    try {
      predictor ||= createPredictor()
      const result = await predictor.predict(image, promptHints(painted, width))
      if (disposed || request !== revision || image !== source.value) return
      if (result.width !== width || result.height !== height || result.data.length !== painted.length) throw new Error('object_mask.invalid_result')
      const clipped = new Uint8Array(painted.length)
      for (let i = 0; i < clipped.length; i++) clipped[i] = painted[i] && result.data[i] ? 1 : 0
      if (!clipped.some(Boolean)) throw new Error('object_mask.empty')
      selected = clipped
      hasMask.value = true
      render(selected)
    } catch (e) {
      if (request === revision && !disposed) error.value = String(e.message || e)
    } finally {
      if (request === revision) busy.value = false
    }
  }

  function snapshot() {
    return { paint: paint.slice(), hints: hints.value.map(p => ({ ...p })) }
  }

  function remember() {
    history.value.push(snapshot())
    while (history.value.length > HISTORY_LIMIT || history.value.length * paint.byteLength > HISTORY_BYTE_LIMIT) history.value.shift()
    redo.value = []
  }

  function insideImage(x, y) {
    const canvas = maskCanvas.value
    return canvas && Number.isFinite(x) && Number.isFinite(y) && x >= 0 && y >= 0 && x < canvas.width && y < canvas.height
  }

  function paintCircle(x, y, radius) {
    const canvas = maskCanvas.value
    const included = stroke.label === INCLUDE_LABEL
    const r = Number.isFinite(radius) && radius > 0 ? radius : brushSize.value / 2
    const left = Math.max(0, Math.floor(x - r))
    const right = Math.min(canvas.width - 1, Math.ceil(x + r))
    const top = Math.max(0, Math.floor(y - r))
    const bottom = Math.min(canvas.height - 1, Math.ceil(y + r))
    for (let py = top; py <= bottom; py++) {
      for (let px = left; px <= right; px++) {
        if ((px - x) ** 2 + (py - y) ** 2 <= r * r) paint[py * canvas.width + px] = included ? 1 : 0
      }
    }
    const ctx = canvas.getContext('2d')
    ctx.globalCompositeOperation = included ? 'source-over' : 'destination-out'
    ctx.fillStyle = included ? `rgba(255,255,255,${OVERLAY_ALPHA / OPAQUE_ALPHA})` : 'white'
    ctx.beginPath()
    ctx.arc(x, y, r, 0, Math.PI * 2)
    ctx.fill()
    ctx.globalCompositeOperation = 'source-over'
    maskVersion.value++
  }

  function beginStroke(x, y, radius) {
    if (disposed || !source.value || !insideImage(x, y)) return
    remember()
    invalidate()
    render()
    stroke = { label: exclude.value ? EXCLUDE_LABEL : INCLUDE_LABEL, points: [{ x, y }], last: { x, y } }
    paintCircle(x, y, radius)
  }

  function strokeTo(x, y, radius) {
    if (!stroke || !insideImage(x, y)) return
    const distance = Math.hypot(x - stroke.last.x, y - stroke.last.y)
    const step = Math.max(1, (radius || brushSize.value / 2) / 2)
    const count = Math.max(1, Math.ceil(distance / step))
    for (let i = 1; i <= count; i++) paintCircle(stroke.last.x + (x - stroke.last.x) * i / count, stroke.last.y + (y - stroke.last.y) * i / count, radius)
    const last = stroke.points[stroke.points.length - 1]
    if (Math.hypot(x - last.x, y - last.y) >= POINT_SPACING && stroke.points.length < MAX_HINTS) stroke.points.push({ x, y })
    stroke.last = { x, y }
  }

  function endStroke() {
    if (!stroke) return
    hints.value = [...hints.value, ...stroke.points.map(p => ({ ...p, label: stroke.label }))].slice(-MAX_HINTS)
    stroke = null
    hasPaint.value = paint.some(Boolean)
    render()
  }

  function restore(state) {
    invalidate()
    paint = state.paint.slice()
    hints.value = state.hints.map(p => ({ ...p }))
    stroke = null
    hasPaint.value = paint.some(Boolean)
    render()
  }

  function undoMask() {
    if (!history.value.length) return
    redo.value.push(snapshot())
    restore(history.value.pop())
  }

  function redoMask() {
    if (!redo.value.length) return
    history.value.push(snapshot())
    restore(redo.value.pop())
  }

  function getMaskBase64() {
    if (!hasMask.value || busy.value || !selected || !maskCanvas.value) return ''
    const output = document.createElement('canvas')
    output.width = maskCanvas.value.width
    output.height = maskCanvas.value.height
    const ctx = output.getContext('2d')
    const pixels = ctx.createImageData(output.width, output.height)
    for (let i = 0; i < selected.length; i++) {
      if (selected[i]) pixels.data.fill(OPAQUE_ALPHA, i * CHANNELS, (i + 1) * CHANNELS)
    }
    ctx.putImageData(pixels, 0, 0)
    return output.toDataURL('image/png').split(',')[1] || ''
  }

  return { maskCanvas, maskVersion, brushSize, busy, error, hasMask, hasPaint, exclude, hints, canUndoMask, canRedoMask,
    beginStroke, strokeTo, endStroke, clearMask, undoMask, redoMask, getMaskBase64, detect }
}
