import { ref, computed, onUnmounted, watch } from 'vue'
import { t } from '../i18n/index.js'

export const ZOOM_MIN = 10
export const ZOOM_MAX = 500
export const ZOOM_PRESETS = [30, 50, 100]

export function useZoomPan(areaRef) {
  const naturalW = ref(0)
  const naturalH = ref(0)
  const areaW = ref(0)
  const areaH = ref(0)
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
    areaW.value = el.clientWidth
    areaH.value = el.clientHeight
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
    panning.value = false
  }

  function wasDragged() {
    return moved > 4
  }

  watch(areaRef, el => {
    observer?.disconnect()
    observer = null
    if (!el) return
    if (typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(measure)
      observer.observe(el)
    }
    measure()
  })
  onUnmounted(() => observer?.disconnect())

  return {
    naturalW, naturalH, zoomPct, panning, zoomMenuOpen, isFit, zoomLabel, panEnabled, layerStyle,
    fitPercent, enterZoom, setFit, zoomBy, onZoomIndicatorClick, onImgLoad, onWheel,
    onPointerDown, onPointerMove, onPointerUp, wasDragged,
  }
}
