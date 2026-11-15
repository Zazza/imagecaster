import { ref, computed, onScopeDispose, watch } from 'vue'
import { t } from '../i18n/index.js'

export const ZOOM_MIN = 10
export const ZOOM_MAX = 500
export const ZOOM_PRESETS = [30, 50, 100]

function clientSize(el) {
  return { w: el.clientWidth, h: el.clientHeight }
}

export function useZoomPan(areaRef, options = {}) {
  const measureArea = options.measure || clientSize
  const observe = options.observe !== false
  const naturalW = ref(0)
  const naturalH = ref(0)
  const areaW = ref(options.initialSize?.w || 0)
  const areaH = ref(options.initialSize?.h || 0)
  const zoomMode = ref('fit')
  const zoomPct = ref(100)
  const panX = ref(0)
  const panY = ref(0)
  const panning = ref(false)
  const zoomMenuOpen = ref(false)
  let lastX = 0
  let lastY = 0
  let moved = 0
  let observer = null
  let suppressDbl = false
  let suppressDblTimer = null

  const isFit = computed(() => zoomMode.value === 'fit')
  const zoomLabel = computed(() => isFit.value ? t('viewer.zoom_fit') : zoomPct.value + '%')
  const fitScale = computed(() => {
    if (!naturalW.value || !naturalH.value || !areaW.value || !areaH.value) return 1
    return Math.min(areaW.value / naturalW.value, areaH.value / naturalH.value)
  })
  const renderedW = computed(() => naturalW.value * zoomPct.value / 100)
  const renderedH = computed(() => naturalH.value * zoomPct.value / 100)
  const panEnabled = computed(() => !isFit.value && !!naturalW.value && (renderedW.value > areaW.value || renderedH.value > areaH.value))
  const layerStyle = computed(() => {
    if (isFit.value || !naturalW.value) return null
    const scale = zoomPct.value / 100 / fitScale.value
    return { transform: `translate(${Math.round(panX.value)}px, ${Math.round(panY.value)}px) scale(${scale})` }
  })

  function measure() {
    const el = areaRef.value
    if (!el) return
    const size = measureArea(el)
    if (size) {
      areaW.value = size.w
      areaH.value = size.h
    }
    if (!isFit.value) clampPan()
  }

  function clampPan() {
    const maxX = Math.max(0, (renderedW.value - areaW.value) / 2)
    const maxY = Math.max(0, (renderedH.value - areaH.value) / 2)
    panX.value = Math.min(maxX, Math.max(-maxX, panX.value))
    panY.value = Math.min(maxY, Math.max(-maxY, panY.value))
  }

  function fitPercent() {
    if (!naturalW.value || !naturalH.value) return 100
    return Math.floor(fitScale.value * 100)
  }

  function enterZoom(pct) {
    zoomMode.value = 'pct'
    zoomPct.value = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, pct))
    clampPan()
    zoomMenuOpen.value = false
  }

  function setFit() {
    zoomMode.value = 'fit'
    zoomPct.value = 100
    panX.value = 0
    panY.value = 0
    panning.value = false
    zoomMenuOpen.value = false
  }

  function zoomBy(delta) {
    const base = isFit.value ? Math.floor(fitPercent() / 10) * 10 : zoomPct.value
    enterZoom(base + delta)
  }

  function onZoomIndicatorClick() {
    if (!isFit.value) {
      setFit()
      return
    }
    zoomMenuOpen.value = !zoomMenuOpen.value
  }

  function toggleFit100() {
    if (suppressDbl) return
    if (isFit.value) enterZoom(100)
    else setFit()
  }

  function resetImage() {
    naturalW.value = 0
    naturalH.value = 0
    setFit()
  }

  function onImgLoad(e) {
    naturalW.value = e.target.naturalWidth
    naturalH.value = e.target.naturalHeight
    measure()
  }

  function onWheel(e) {
    if (!e.ctrlKey) return
    e.preventDefault()
    zoomBy(e.deltaY < 0 ? 10 : -10)
  }

  function onPointerDown(e) {
    zoomMenuOpen.value = false
    moved = 0
    if (e.button !== 0 || !panEnabled.value) return
    panning.value = true
    lastX = e.clientX
    lastY = e.clientY
    e.currentTarget.setPointerCapture(e.pointerId)
  }

  function onPointerMove(e) {
    if (!panning.value) return
    panX.value += e.clientX - lastX
    panY.value += e.clientY - lastY
    moved += Math.abs(e.clientX - lastX) + Math.abs(e.clientY - lastY)
    lastX = e.clientX
    lastY = e.clientY
    clampPan()
  }

  function onPointerUp() {
    if (panning.value && moved > 4) {
      suppressDbl = true
      clearTimeout(suppressDblTimer)
      suppressDblTimer = setTimeout(() => { suppressDbl = false }, 300)
    }
    panning.value = false
  }

  function wasDragged() {
    return moved > 4
  }

  watch(areaRef, el => {
    observer?.disconnect()
    observer = null
    if (!el) return
    if (observe && typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(measure)
      observer.observe(el)
    }
    measure()
  })
  onScopeDispose(() => {
    observer?.disconnect()
    clearTimeout(suppressDblTimer)
  })

  return {
    naturalW, naturalH, zoomPct, panX, panY, renderedW, renderedH, panning, zoomMenuOpen, isFit, zoomLabel, panEnabled, layerStyle,
    measure, fitPercent, enterZoom, setFit, zoomBy, onZoomIndicatorClick, toggleFit100, resetImage, onImgLoad, onWheel,
    onPointerDown, onPointerMove, onPointerUp, wasDragged,
  }
}
