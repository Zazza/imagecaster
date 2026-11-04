import { ref, computed } from 'vue'

const HISTORY_LIMIT = 30

function b64Url(b64) {
  if (!b64) return ''
  if (b64.startsWith('data:')) return b64
  if (b64.startsWith('/9j/')) return 'data:image/jpeg;base64,' + b64
  return 'data:image/png;base64,' + b64
}

export function useMaskCanvas() {
  const brushSize = ref(40)
  const maskCanvas = ref(null)
  const maskHistory = ref([])
  const maskRedo = ref([])
  const maskVersion = ref(0)
  let strokeActive = false
  let sourceReqId = 0

  const hasMask = computed(() => maskHistory.value.length > 0)
  const canRedoMask = computed(() => maskRedo.value.length > 0)

  function discardMask() {
    sourceReqId++
    maskCanvas.value = null
    maskHistory.value = []
    maskRedo.value = []
    maskVersion.value++
  }

  function applySource(source, keepIfSameSize) {
    if (!source) {
      discardMask()
      return
    }
    const reqId = ++sourceReqId
    const img = new Image()
    img.onload = () => {
      if (reqId !== sourceReqId) return
      const w = img.naturalWidth || 1
      const h = img.naturalHeight || 1
      if (keepIfSameSize && maskCanvas.value && maskCanvas.value.width === w && maskCanvas.value.height === h) {
        maskVersion.value++
        return
      }
      const canvas = document.createElement('canvas')
      canvas.width = w
      canvas.height = h
      maskCanvas.value = canvas
      maskHistory.value = []
      maskRedo.value = []
      maskVersion.value++
    }
    img.src = b64Url(source)
  }

  function paintCircle(x, y, radius) {
    const canvas = maskCanvas.value
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    ctx.globalCompositeOperation = 'source-over'
    ctx.fillStyle = 'rgba(255, 255, 255, 0.6)'
    ctx.beginPath()
    ctx.arc(x, y, radius, 0, Math.PI * 2)
    ctx.fill()
  }

  function beginStroke(x, y, radius) {
    const canvas = maskCanvas.value
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    maskHistory.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height))
    if (maskHistory.value.length > HISTORY_LIMIT) maskHistory.value.shift()
    maskRedo.value = []
    strokeActive = true
    paintCircle(x, y, radius)
    maskVersion.value++
  }

  function strokeTo(x, y, radius) {
    if (!strokeActive) return
    paintCircle(x, y, radius)
    maskVersion.value++
  }

  function endStroke() {
    strokeActive = false
  }

  function clearMask() {
    const canvas = maskCanvas.value
    if (!canvas) return
    canvas.getContext('2d').clearRect(0, 0, canvas.width, canvas.height)
    maskHistory.value = []
    maskRedo.value = []
    maskVersion.value++
  }

  function undoMask() {
    if (maskHistory.value.length === 0) return
    const canvas = maskCanvas.value
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    maskRedo.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height))
    ctx.putImageData(maskHistory.value.pop(), 0, 0)
    maskVersion.value++
  }

  function redoMask() {
    if (maskRedo.value.length === 0) return
    const canvas = maskCanvas.value
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    maskHistory.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height))
    ctx.putImageData(maskRedo.value.pop(), 0, 0)
    maskVersion.value++
  }

  function getMaskBase64() {
    const canvas = maskCanvas.value
    if (!canvas) return ''
    const w = canvas.width
    const h = canvas.height
    const src = canvas.getContext('2d').getImageData(0, 0, w, h)
    const out = document.createElement('canvas')
    out.width = w
    out.height = h
    const outCtx = out.getContext('2d')
    const data = outCtx.createImageData(w, h)
    for (let i = 0; i < src.data.length; i += 4) {
      if (src.data[i + 3] > 10) {
        data.data[i] = 255
        data.data[i + 1] = 255
        data.data[i + 2] = 255
        data.data[i + 3] = 255
      }
    }
    outCtx.putImageData(data, 0, 0)
    return out.toDataURL('image/png').split(',')[1] || ''
  }

  return {
    brushSize,
    maskCanvas,
    maskHistory,
    maskVersion,
    hasMask,
    canRedoMask,
    applySource,
    discardMask,
    beginStroke,
    strokeTo,
    endStroke,
    clearMask,
    undoMask,
    redoMask,
    getMaskBase64,
  }
}
