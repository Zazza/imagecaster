import { ref, computed, watch, onScopeDispose } from 'vue'
import { createObjectMaskPredictor } from '../objectMaskClient.js'
import { MAX_OBJECT_MASK_PIXELS } from '../objectMaskLimits.js'
import { b64Url } from '../imageDataUrl.js'

const OVERLAY_ALPHA = 150
const OPAQUE_ALPHA = 255
const CHANNELS = 4
const HISTORY_LIMIT = 30
const HISTORY_BYTE_LIMIT = 64 * 1024 * 1024
const INCLUDE_LABEL = 1

export function useWandMask(source, { createPredictor = createObjectMaskPredictor } = {}) {
  const maskCanvas = ref(null)
  const maskVersion = ref(0)
  const busy = ref(false)
  const error = ref('')
  const accepted = ref(false)
  const history = ref([])
  const redo = ref([])
  const hasMask = computed(() => accepted.value && !busy.value && !error.value)
  const canUndoMask = computed(() => history.value.length > 0)
  const canRedoMask = computed(() => redo.value.length > 0)
  const canClear = computed(() => accepted.value || busy.value || Boolean(error.value) || history.value.length > 0 || redo.value.length > 0)
  let selected = null
  let predictor = null
  let epoch = 0
  let pending = 0
  let queue = Promise.resolve()
  let disposed = false

  function invalidate() {
    epoch++
    pending = 0
    busy.value = false
    error.value = ''
    queue = Promise.resolve()
  }

  function render() {
    const canvas = maskCanvas.value
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    const pixels = ctx.createImageData(canvas.width, canvas.height)
    if (selected) {
      for (let i = 0; i < selected.length; i++) {
        if (!selected[i]) continue
        const offset = i * CHANNELS
        pixels.data.fill(OPAQUE_ALPHA, offset, offset + CHANNELS)
        pixels.data[offset + CHANNELS - 1] = OVERLAY_ALPHA
      }
    }
    ctx.putImageData(pixels, 0, 0)
    maskVersion.value++
  }

  function trimHistory() {
    const byteLength = maskCanvas.value ? maskCanvas.value.width * maskCanvas.value.height : 0
    while (history.value.length + redo.value.length > HISTORY_LIMIT ||
      [...history.value, ...redo.value].reduce((sum, bitmap) => sum + (bitmap ? byteLength : 0), 0) > HISTORY_BYTE_LIMIT) {
      if (history.value.length) history.value.shift()
      else redo.value.shift()
    }
  }

  function clearMask() {
    invalidate()
    predictor?.dispose()
    predictor = null
    selected = null
    accepted.value = false
    history.value = []
    redo.value = []
    maskCanvas.value = null
    maskVersion.value++
    if (disposed || !source.value) return
    const request = epoch
    const img = new Image()
    img.onload = () => {
      if (disposed || request !== epoch) return
      const width = img.naturalWidth
      const height = img.naturalHeight
      if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0 || width * height > MAX_OBJECT_MASK_PIXELS) {
        error.value = 'object_mask.image_size'
        return
      }
      const canvas = document.createElement('canvas')
      canvas.width = width
      canvas.height = height
      maskCanvas.value = canvas
      maskVersion.value++
    }
    img.onerror = () => { if (!disposed && request === epoch) error.value = 'object_mask.image_failed' }
    img.src = b64Url(source.value)
  }

  function selectAt(x, y, additive = false) {
    const canvas = maskCanvas.value
    if (disposed || !source.value || !canvas || !Number.isFinite(x) || !Number.isFinite(y) ||
      x < 0 || y < 0 || x >= canvas.width || y >= canvas.height) return Promise.resolve()
    if (!additive) invalidate()
    const request = epoch
    const image = source.value
    const width = canvas.width
    const height = canvas.height
    error.value = ''
    pending++
    busy.value = true
    const operation = queue.then(async () => {
      if (disposed || request !== epoch) return
      try {
        predictor ||= createPredictor()
        const result = await predictor.predict(image, [{ x, y, label: INCLUDE_LABEL }])
        if (disposed || request !== epoch || image !== source.value) return
        if (!result || result.width !== width || result.height !== height || !result.data || result.data.length !== width * height) {
          throw new Error('object_mask.invalid_result')
        }
        const bitmap = new Uint8Array(width * height)
        let nonempty = false
        for (let i = 0; i < bitmap.length; i++) {
          bitmap[i] = result.data[i] ? 1 : 0
          if (bitmap[i]) nonempty = true
        }
        if (!nonempty) throw new Error('wand_mask.empty')
        if (additive && selected) {
          for (let i = 0; i < bitmap.length; i++) bitmap[i] |= selected[i]
        }
        history.value.push(selected)
        redo.value = []
        selected = bitmap
        accepted.value = true
        error.value = ''
        trimHistory()
        render()
      } catch (e) {
        if (!disposed && request === epoch) error.value = String(e.message || e)
      } finally {
        if (request === epoch) {
          pending--
          busy.value = pending > 0
        }
      }
    })
    queue = operation
    return operation
  }

  function undoMask() {
    invalidate()
    if (!history.value.length) return
    redo.value.push(selected)
    selected = history.value.pop()
    accepted.value = Boolean(selected)
    trimHistory()
    render()
  }

  function redoMask() {
    invalidate()
    if (!redo.value.length) return
    history.value.push(selected)
    selected = redo.value.pop()
    accepted.value = Boolean(selected)
    trimHistory()
    render()
  }

  function getMaskBase64() {
    if (!hasMask.value || !selected || !maskCanvas.value) return ''
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

  watch(source, clearMask, { flush: 'sync', immediate: true })
  onScopeDispose(() => { disposed = true; clearMask() })

  return { maskCanvas, maskVersion, hasMask, busy, error, canUndoMask, canRedoMask, canClear,
    selectAt, clearMask, undoMask, redoMask, getMaskBase64 }
}
