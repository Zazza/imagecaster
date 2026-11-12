<script setup>
import StatusMessage from './StatusMessage.vue'
import FoldBlock from './FoldBlock.vue'
import { modelLoraText } from '../modelLora.js'
import { ref, reactive, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { ArrowLeftRight, Eye, EyeOff, Image as ImageIcon, Pencil, Redo2, RotateCcw, Sparkles, Undo2, X } from 'lucide-vue-next'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { parseSeedInput } from '../seed.js'
import { useRemixQueue } from '../remixQueue.js'
import { MAX_IMAGE_SIZE } from '../constants.js'
import { usePresets } from '../composables/usePresets.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { EventsOn } from '../wailsjs/runtime/runtime'
import EditSection from './EditSection.vue'
import { EFFECT_TYPE_GROUPS } from '../effectTypes.js'
import { useMaskCanvas } from '../composables/useMaskCanvas.js'
import RemixSection from './RemixSection.vue'
import ObjectSection from './ObjectSection.vue'
import RepairSection from './RepairSection.vue'
import RecipeComposition from './RecipeComposition.vue'
import { createBatch, batchExportSize, batchFileName } from '../postBatch.js'

const props = defineProps({
  initialSection: { type: String, default: 'effect' },
  initialImage: { type: String, default: '' },
})

const emit = defineEmits(['clear-initial-image'])

const openSection = ref('effect')
const editSectionRef = ref(null)
const remixSectionRef = ref(null)
const objectSectionRef = ref(null)
const objectCompRef = ref(null)
const repairSectionRef = ref(null)
const repairCompRef = ref(null)
const effectSectionRef = ref(null)
const adjustSectionRef = ref(null)
const exportSectionRef = ref(null)
const finishSectionRef = ref(null)
const recipesSectionRef = ref(null)

function sectionEl(name) {
  if (name === 'edit') return editSectionRef.value
  if (name === 'remix') return remixSectionRef.value
  if (name === 'object') return objectSectionRef.value
  if (name === 'repair') return repairSectionRef.value
  if (name === 'adjust') return adjustSectionRef.value
  if (name === 'export') return exportSectionRef.value
  if (name === 'finish') return finishSectionRef.value
  if (name === 'recipes') return recipesSectionRef.value
  return effectSectionRef.value
}

function setOpenSection(name) {
  if (openSection.value === name) return
  openSection.value = name
  nextTick(() => {
    sectionEl(name)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  })
}

watch(() => props.initialSection, (val) => {
  if (val) setOpenSection(val)
})

const sourceB64 = ref('')
const currentB64 = ref('')
const sourceName = ref('image')
const imageInfo = ref({ width: 0, height: 0 })

const batch = ref(null)
const batchActive = computed(() => !!batch.value)
const batchIndex = ref(0)
const batchBusy = ref(false)
const batchExporting = ref(false)
const batchProgress = ref({ done: 0, total: 0 })
const batchThumbs = reactive({})
const batchMsg = ref('')
let batchCancelled = false
let batchViewReq = 0

const SESSION_PATH = 'session:'

function readBatchImage(path) {
  if (path.startsWith(SESSION_PATH)) return api.getSessionImage(Number(path.slice(SESSION_PATH.length)))
  return api.readFileAsBase64(path)
}

function readBatchThumb(path) {
  if (path.startsWith(SESSION_PATH)) return api.getSessionThumb(Number(path.slice(SESSION_PATH.length)))
  return api.readThumbnail(path)
}

async function startBatch(items) {
  if (batch.value) exitBatch(false)
  try { await api.batchReset() } catch {}
  batch.value = createBatch(items, {
    readImage: readBatchImage,
    saveImage: (b64) => api.batchSaveImage(b64),
  })
  batchMsg.value = ''
  resetAdjustAll()
  layerUndo.value = []
  layerRedo.value = []
  appliedEffects.value = []
  setOpenSection('effect')
  loadBatchThumbs()
  await showBatchItem(0)
}

async function showBatchItem(i) {
  const b = batch.value
  if (!b || !b.items[i]) return
  batchIndex.value = i
  const req = ++batchViewReq
  const item = b.items[i]
  try {
    const orig = await readBatchImage(item.path)
    const curPath = b.currentPath(i)
    const cur = curPath === item.path ? orig : await readBatchImage(curPath)
    if (req !== batchViewReq || batch.value !== b) return
    sourceB64.value = orig
    currentB64.value = cur
    sourceName.value = (item.name || 'image').replace(/\.[^.]+$/, '') || 'image'
    loadImageDimensions(cur)
    updateFilename()
  } catch (e) {
    if (req === batchViewReq) error.value = String(e)
  }
}

function loadBatchThumbs() {
  const b = batch.value
  if (!b) return
  b.items.forEach((_, i) => {
    const path = b.currentPath(i)
    if (batchThumbs[path]) return
    readBatchThumb(path).then(t => {
      if (t) batchThumbs[path] = t
    }).catch(() => {})
  })
}

function batchThumbSrc(i) {
  const b = batch.value
  return b ? b64Url(batchThumbs[b.currentPath(i)] || '') : ''
}

async function runBatchStep(op, onDone) {
  const b = batch.value
  if (!b || batchBusy.value || batchExporting.value) return
  batchBusy.value = true
  batchCancelled = false
  batchMsg.value = ''
  error.value = ''
  successMsg.value = ''
  batchProgress.value = { done: 0, total: b.items.length }
  try {
    const res = await b.applyStep(op, {
      isCancelled: () => batchCancelled,
      onProgress: (done, total) => {
        batchProgress.value = { done, total }
        loadBatchThumbs()
      },
    })
    if (res.failed > 0 && !res.cancelled) error.value = t('post.batch_step_failed', { n: res.failed })
    if (res.cancelled) batchMsg.value = t('post.pipeline_interrupted')
    if (res.changed > 0 && onDone) onDone(res)
  } finally {
    batchBusy.value = false
    if (batch.value === b) {
      loadBatchThumbs()
      await showBatchItem(batchIndex.value)
    }
  }
}

function stopBatch() {
  if (batchBusy.value) interruptBatch()
  else batchCancelled = true
}

async function interruptBatch() {
  batchCancelled = true
  for (const [id, resolve] of [...recipeJobResolvers]) {
    recipeJobResolvers.delete(id)
    journalPending.delete(id)
    try { await api.cancelQueueJob(id) } catch {}
    resolve('')
  }
}

async function removeBatchItem(i) {
  const b = batch.value
  if (!b || batchBusy.value || batchExporting.value) return
  if (!b.removeItem(i)) return
  if (b.items.length === 0) {
    exitBatch(false)
    sourceB64.value = ''
    currentB64.value = ''
    return
  }
  if (i < batchIndex.value) {
    batchIndex.value--
    return
  }
  if (i === batchIndex.value) await showBatchItem(Math.min(i, b.items.length - 1))
}

async function undoBatch() {
  const b = batch.value
  if (!b || batchBusy.value || batchExporting.value) return
  if (b.undo()) {
    batchMsg.value = ''
    await showBatchItem(batchIndex.value)
  }
}

async function resetBatch() {
  const b = batch.value
  if (!b || batchBusy.value || batchExporting.value) return
  while (b.undo()) {}
  await showBatchItem(batchIndex.value)
}

function exitBatch(keepCurrent) {
  if (!batch.value) return
  const cur = currentB64.value
  const name = sourceName.value
  interruptBatch()
  batch.value = null
  batchMsg.value = ''
  for (const k of Object.keys(batchThumbs)) delete batchThumbs[k]
  api.batchReset().catch(() => {})
  if (keepCurrent && cur) setSource(cur, name)
}

async function exportBatch() {
  const b = batch.value
  if (!b || batchBusy.value || batchExporting.value) return
  const folder = await resolveBatchFolder()
  if (!folder) {
    error.value = t('post.pipeline_needs_folder')
    return
  }
  batchExporting.value = true
  batchCancelled = false
  error.value = ''
  successMsg.value = ''
  batchProgress.value = { done: 0, total: b.items.length }
  const size = batchExportSize({
    width: width.value,
    height: height.value,
    lockRatio: lockRatio.value,
    previewWidth: imageInfo.value.width,
    previewHeight: imageInfo.value.height,
  })
  const adj = adjustParams()
  try {
    const res = await b.exportAll(
      (b64, item) => api.exportImage({ ...exportParamsFor(batchFileName(item.name, format.value), b64, folder), width: size.width, height: size.height }, adj),
      { isCancelled: () => batchCancelled, onProgress: (done, total) => { batchProgress.value = { done, total } } },
    )
    successMsg.value = t('post.batch_saved', { k: res.saved, m: res.skipped, path: folder })
    if (res.cancelled) successMsg.value += ' — ' + t('post.pipeline_interrupted')
  } finally {
    batchExporting.value = false
  }
}

const adjust = reactive({
  brightness: 0,
  contrast: 0,
  black_point: 0,
  white_point: 255,
  gamma: 1,
  saturation: 0,
  temperature: 0,
  balance_r: 0,
  balance_g: 0,
  balance_b: 0,
  sharpen: 0,
  vignette: 0,
})

const sliderMeta = {
  brightness: { min: -100, max: 100, step: 1, def: 0 },
  contrast: { min: -100, max: 100, step: 1, def: 0 },
  black_point: { min: 0, max: 254, step: 1, def: 0 },
  white_point: { min: 1, max: 255, step: 1, def: 255 },
  gamma: { min: 0.1, max: 3, step: 0.01, def: 1 },
  saturation: { min: -100, max: 100, step: 1, def: 0 },
  temperature: { min: -100, max: 100, step: 1, def: 0 },
  balance_r: { min: -100, max: 100, step: 1, def: 0 },
  balance_g: { min: -100, max: 100, step: 1, def: 0 },
  balance_b: { min: -100, max: 100, step: 1, def: 0 },
  sharpen: { min: 0, max: 100, step: 1, def: 0 },
  vignette: { min: 0, max: 100, step: 1, def: 0 },
}

const adjustGroups = [
  { labelKey: 'post.group_light', keys: ['brightness', 'contrast'] },
  { labelKey: 'post.group_levels', keys: ['black_point', 'white_point', 'gamma'] },
  { labelKey: 'post.group_color', keys: ['saturation', 'temperature', 'balance_r', 'balance_g', 'balance_b'] },
  { labelKey: 'post.group_film', keys: ['sharpen', 'vignette'] },
]

const adjustNeutral = computed(() =>
  Object.keys(sliderMeta).every(k => {
    const v = adjust[k]
    const d = sliderMeta[k].def
    return k === 'white_point' ? (v === d || v === 0) : (k === 'gamma' ? (v === d || v === 0) : v === d)
  }))

function adjustParams() {
  return { ...adjust }
}

const FINISH_RATIOS = ['1:1', '4:5', '3:4', '9:16', '16:9', '3:2']
const finish = reactive({ ratio: '', fit: 'crop', offset: 0.5, frame: '', caption: '', caption_pos: 'bottom', caption_size: 'm' })
const finishActive = computed(() => !!finish.ratio || !!finish.frame || !!finish.caption.trim())
function finishParams() {
  if (!finishActive.value) return null
  const p = {}
  if (finish.ratio) {
    p.ratio = finish.ratio
    p.fit = finish.fit
    if (finish.fit === 'crop') p.offset = finish.offset
  }
  if (finish.frame) p.frame = finish.frame
  if (finish.caption.trim()) {
    p.caption = finish.caption
    p.caption_pos = finish.caption_pos
    p.caption_size = finish.caption_size
  }
  return p
}
function resetFinish() {
  Object.assign(finish, { ratio: '', fit: 'crop', offset: 0.5, frame: '', caption: '', caption_pos: 'bottom', caption_size: 'm' })
}

function resetAdjustKey(key) {
  adjust[key] = sliderMeta[key].def
}

function resetAdjustAll() {
  for (const k of Object.keys(sliderMeta)) adjust[k] = sliderMeta[k].def
}

const autoAdjusting = ref(false)

async function applyAutoAdjust(kind) {
  if (!currentB64.value || autoAdjusting.value) return
  autoAdjusting.value = true
  error.value = ''
  try {
    const stats = await api.autoAdjust(currentB64.value)
    if (kind === 'levels') {
      adjust.black_point = stats.black_point
      adjust.white_point = stats.white_point
      adjust.gamma = stats.gamma
    } else {
      adjust.temperature = stats.temperature
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    autoAdjusting.value = false
  }
}

function formatAdjust(key) {
  const v = adjust[key]
  return sliderMeta[key].step < 1 ? Number(v).toFixed(2) : String(v)
}

const previewB64 = ref('')
const previewing = ref(false)
let previewTimer = null
let previewReqId = 0

watch(adjust, schedulePreview)
watch(finish, schedulePreview)
watch(currentB64, schedulePreview)

function schedulePreview() {
  clearTimeout(previewTimer)
  if (adjustNeutral.value && !finishActive.value) {
    previewReqId++
    previewing.value = false
    previewB64.value = currentB64.value
    return
  }
  previewTimer = setTimeout(runPreview, 150)
}

async function runPreview() {
  if (!currentB64.value) return
  const reqId = ++previewReqId
  previewing.value = true
  try {
    const fp = finishParams()
    const res = fp
      ? await api.previewPost(currentB64.value, adjustParams(), fp, 900)
      : await api.previewAdjust(currentB64.value, adjustParams(), 900)
    if (reqId !== previewReqId) return
    if (res) previewB64.value = res
  } catch (e) {
    if (reqId === previewReqId) error.value = String(e)
  } finally {
    if (reqId === previewReqId) previewing.value = false
  }
}

const splitPos = ref(50)
const splitDragging = ref(false)
const splitWrapRef = ref(null)

const ZOOM_MIN = 10
const ZOOM_MAX = 500
const ZOOM_PRESETS = [30, 50, 100]

const naturalW = ref(0)
const naturalH = ref(0)
const zoomMode = ref('fit')
const zoomPct = ref(100)
const panX = ref(0)
const panY = ref(0)
const panning = ref(false)
const zoomMenuOpen = ref(false)
const areaW = ref(0)
const areaH = ref(0)
let panLastX = 0
let panLastY = 0
let panMovedDist = 0
let suppressDbl = false
let suppressDblTimer = null

const isFit = computed(() => zoomMode.value === 'fit')
const zoomLabel = computed(() => isFit.value ? t('viewer.zoom_fit') : zoomPct.value + '%')

const fitScale = computed(() => {
  if (!naturalW.value || !naturalH.value || !areaW.value || !areaH.value) return 1
  return Math.min(areaW.value / naturalW.value, areaH.value / naturalH.value)
})

const renderedW = computed(() => naturalW.value ? (naturalW.value * zoomPct.value) / 100 : 0)
const renderedH = computed(() => naturalH.value ? (naturalH.value * zoomPct.value) / 100 : 0)

const panEnabled = computed(() => {
  if (isFit.value || !naturalW.value) return false
  return renderedW.value > areaW.value || renderedH.value > areaH.value
})

const layerStyle = computed(() => {
  if (isFit.value || !naturalW.value) return null
  const scale = zoomPct.value / 100 / fitScale.value
  return { transform: `translate(${Math.round(panX.value)}px, ${Math.round(panY.value)}px) scale(${scale})` }
})

function measureArea() {
  const el = splitWrapRef.value
  if (!el) return
  areaW.value = el.clientWidth
  areaH.value = el.clientHeight
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
  if (suppressDbl || maskMode.value) return
  if (isFit.value) enterZoom(100)
  else setFit()
}

function onAfterImgLoad(e) {
  naturalW.value = e.target.naturalWidth
  naturalH.value = e.target.naturalHeight
  measureArea()
  if (!isFit.value) clampPan()
}

function onViewWheel(e) {
  if (!e.ctrlKey) return
  e.preventDefault()
  zoomBy(e.deltaY < 0 ? 10 : -10)
}

const maskViewRef = ref(null)
const brushing = ref(false)
const brushCursor = ref({ x: 0, y: 0, visible: false })

const effectMask = useMaskCanvas()
const { brushSize: effectBrush, hasMask: effectHasMask } = effectMask
const effectMaskOn = ref(false)
const effectMaskComp = reactive({
  maskActive: computed(() => effectMaskOn.value && !!currentB64.value && !batchActive.value),
  maskCanvas: effectMask.maskCanvas,
  maskVersion: effectMask.maskVersion,
  brushSize: effectMask.brushSize,
  beginStroke: effectMask.beginStroke,
  strokeTo: effectMask.strokeTo,
  endStroke: effectMask.endStroke,
  undoMask: effectMask.undoMask,
  redoMask: effectMask.redoMask,
  canUndoMask: effectMask.hasMask,
  canRedoMask: effectMask.canRedoMask,
})
let effectMaskReset = false
watch(currentB64, v => {
  effectMask.applySource(v, !effectMaskReset)
  effectMaskReset = false
}, { immediate: true })

function effectMaskParam() {
  if (!effectMaskOn.value || batchActive.value || !effectHasMask.value) return {}
  return { mask_base64: effectMask.getMaskBase64() }
}

const maskComp = computed(() => openSection.value === 'object' ? objectCompRef.value : openSection.value === 'repair' ? repairCompRef.value : openSection.value === 'effect' ? effectMaskComp : null)
const maskMode = computed(() => !!maskComp.value?.maskActive)
const brushDiameter = computed(() => maskComp.value?.brushSize || 30)
const maskOverlayVisible = ref(true)

function maskScale() {
  const canvas = maskComp.value?.maskCanvas
  const el = splitWrapRef.value
  if (!canvas || !el || !canvas.width || !canvas.height) return 0
  const canvasFit = Math.min(el.clientWidth / canvas.width, el.clientHeight / canvas.height)
  if (isFit.value || !naturalW.value || !naturalH.value) return canvasFit
  const displayedFit = Math.min(el.clientWidth / naturalW.value, el.clientHeight / naturalH.value)
  return canvasFit * (zoomPct.value / 100) / displayedFit
}

function brushRadius() {
  const scale = maskScale()
  return scale > 0 ? brushDiameter.value / 2 / scale : 1
}

function pointerToImage(e) {
  const el = splitWrapRef.value
  const canvas = maskComp.value?.maskCanvas
  const scale = maskScale()
  if (!el || !canvas || !scale) return null
  const rect = el.getBoundingClientRect()
  const px = Math.round(panX.value)
  const py = Math.round(panY.value)
  return {
    x: (e.clientX - rect.left - rect.width / 2 - px) / scale + canvas.width / 2,
    y: (e.clientY - rect.top - rect.height / 2 - py) / scale + canvas.height / 2,
  }
}

function syncMaskView() {
  const src = maskComp.value?.maskCanvas
  const view = maskViewRef.value
  if (!src || !view) return
  if (view.width !== src.width || view.height !== src.height) {
    view.width = src.width
    view.height = src.height
  }
  const ctx = view.getContext('2d')
  ctx.clearRect(0, 0, view.width, view.height)
  ctx.drawImage(src, 0, 0)
}

watch([maskComp, () => maskComp.value?.maskVersion], () => {
  if (maskMode.value) nextTick(syncMaskView)
})
watch(maskMode, (val) => {
  if (!val) {
    brushing.value = false
    brushCursor.value.visible = false
  } else {
    nextTick(syncMaskView)
  }
})
watch(maskOverlayVisible, (val) => {
  if (val && maskMode.value) nextTick(syncMaskView)
})

function updateBrushCursor(e) {
  const el = splitWrapRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  brushCursor.value = { x: e.clientX - rect.left, y: e.clientY - rect.top, visible: true }
}

function onViewPointerDown(e) {
  zoomMenuOpen.value = false
  if (e.button !== 0) return
  if (maskMode.value) {
    const p = pointerToImage(e)
    if (!p) return
    brushing.value = true
    maskComp.value?.beginStroke(p.x, p.y, brushRadius())
    e.currentTarget.setPointerCapture(e.pointerId)
    return
  }
  if (!panEnabled.value) return
  panning.value = true
  panMovedDist = 0
  panLastX = e.clientX
  panLastY = e.clientY
  e.currentTarget.setPointerCapture(e.pointerId)
}

function onViewPointerMove(e) {
  if (maskMode.value) {
    updateBrushCursor(e)
    if (brushing.value) {
      const p = pointerToImage(e)
      if (p) maskComp.value?.strokeTo(p.x, p.y, brushRadius())
    }
    return
  }
  if (!panning.value) return
  panX.value += e.clientX - panLastX
  panY.value += e.clientY - panLastY
  panMovedDist += Math.abs(e.clientX - panLastX) + Math.abs(e.clientY - panLastY)
  panLastX = e.clientX
  panLastY = e.clientY
  clampPan()
}

function onViewPointerUp() {
  if (brushing.value) {
    brushing.value = false
    maskComp.value?.endStroke()
    return
  }
  if (panning.value && panMovedDist > 4) {
    suppressDbl = true
    clearTimeout(suppressDblTimer)
    suppressDblTimer = setTimeout(() => { suppressDbl = false }, 300)
  }
  panning.value = false
}

function onViewPointerLeave() {
  brushCursor.value.visible = false
}

function onWindowResize() {
  measureArea()
  if (!isFit.value) clampPan()
}

watch(sourceB64, () => {
  naturalW.value = 0
  naturalH.value = 0
  setFit()
})

function moveSplit(e) {
  const el = splitWrapRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  splitPos.value = Math.min(100, Math.max(0, ((e.clientX - rect.left) / rect.width) * 100))
}

function onSplitDown(e) {
  e.stopPropagation()
  splitDragging.value = true
  e.currentTarget.setPointerCapture(e.pointerId)
  moveSplit(e)
}

function onSplitMove(e) {
  if (splitDragging.value) moveSplit(e)
}

function onSplitUp() {
  splitDragging.value = false
}

function b64Url(b64) {
  if (!b64) return ''
  if (b64.startsWith('data:')) return b64
  if (b64.startsWith('/9j/')) return 'data:image/jpeg;base64,' + b64
  return 'data:image/png;base64,' + b64
}

const { presets, presetTypes, loadPresets } = usePresets()


const effectGroups = computed(() => {
  const labels = new Map()
  for (const g of EFFECT_TYPE_GROUPS) {
    const pt = presetTypes.value.find(x => (x.name || '').trim().toLowerCase() === g.type)
    if (pt) labels.set(pt.id, g.labelKey)
  }
  const byKey = new Map()
  for (const p of presets.value) {
    const labelKey = labels.get(p.type_id)
    if (!labelKey) continue
    if (!byKey.has(labelKey)) byKey.set(labelKey, [])
    byKey.get(labelKey).push(p)
  }
  return EFFECT_TYPE_GROUPS
    .filter(g => byKey.has(g.labelKey))
    .map(g => ({ labelKey: g.labelKey, presets: byKey.get(g.labelKey) }))
})

function effectLabel(name) {
  const k = 'post.effect_' + name.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
  const v = t(k)
  return v === k ? name : v
}

const selectedEffectId = ref(null)
watch(presets, list => {
  if (selectedEffectId.value && !list.some(p => p.id === selectedEffectId.value)) selectedEffectId.value = null
})
const EFFECT_GROUP_KEY = 'pp.effectGroup'
const effectGroupKey = ref((() => { try { return localStorage.getItem(EFFECT_GROUP_KEY) || '' } catch { return '' } })())
const shownEffectGroup = computed(() => effectGroups.value.find(g => g.labelKey === effectGroupKey.value) || effectGroups.value[0] || null)
function pickEffectGroup(key) {
  effectGroupKey.value = key
  try { localStorage.setItem(EFFECT_GROUP_KEY, key) } catch {}
}
const selectedEffectName = computed(() => {
  const id = selectedEffectId.value
  if (!id) return t('post.effect_none')
  for (const g of effectGroups.value) {
    const p = g.presets.find(x => x.id === id)
    if (p) return effectLabel(p.name)
  }
  return t('post.effect_none')
})
const effectStrength = ref(0.5)

function selectEffect(p) {
  selectedEffectId.value = p.id
  const d = Number(p.denoising_strength)
  if (Number.isFinite(d) && d > 0) effectStrength.value = Math.min(0.9, Math.max(0.35, d))
}
const effectKeepColor = ref(true)
const effectIsMaterial = computed(() => {
  const id = selectedEffectId.value
  return !!id && presets.value.some(p => p.id === id && p.preset_type === 'post_material')
})
const effectIsPattern = computed(() => {
  const id = selectedEffectId.value
  return !!id && presets.value.some(p => p.id === id && p.preset_type === 'post_pattern')
})
const effectSeedInput = ref('')
const applyingEffect = ref(false)
const effectJobIds = ref(new Set())
const appliedEffects = ref([])
const journalPending = new Map()

const effectLoras = ref([])
const effectLoraChoices = ref([])

function loraWeight(w) {
  if (w === '' || w === null || w === undefined) return 1
  const n = Number(w)
  return Number.isFinite(n) ? n : 1
}

function cleanLoras(list) {
  return list.filter(l => l && l.name).map(l => ({ name: String(l.name), weight: loraWeight(l.weight) }))
}

function parseLoraList(raw) {
  try {
    const list = JSON.parse(raw || '[]')
    return Array.isArray(list) ? cleanLoras(list) : []
  } catch {
    return []
  }
}

async function loadEffectModel() {
  try {
    const s = await api.getSettings()
    effectLoras.value = parseLoraList(s?.effect_loras_klein)
  } catch (e) {
    console.warn('effect model settings:', e)
  }
}

const effectModelSummary = computed(() => modelLoraText('Klein', effectLoras.value))

async function onEffectModelToggle(open) {
  if (open && !effectLoraChoices.value.length) {
    try {
      effectLoraChoices.value = (await api.getLoRAs() || []).map(l => l.name).filter(Boolean)
    } catch (e) {
      console.warn('effect loras:', e)
    }
  }
}

function saveEffectSetting(data) {
  api.updateSettings(data).catch(e => console.warn('save effect model:', e))
}

function saveEffectLoras() {
  saveEffectSetting({ effect_loras_klein: JSON.stringify(cleanLoras(effectLoras.value)) })
}

function addEffectLora() {
  effectLoras.value.push({ name: effectLoraChoices.value[0] || '', weight: 1 })
  saveEffectLoras()
}

function removeEffectLora(idx) {
  effectLoras.value.splice(idx, 1)
  saveEffectLoras()
}

function effectOverrides(presetId) {
  const preset = presets.value.find(p => p.id === presetId)
  if (!effectLoras.value.some(l => l.name)) return null
  return { loras: JSON.stringify([...parseLoraList(preset?.loras), ...cleanLoras(effectLoras.value)]) }
}

function effectJobParams(b64, seed) {
  return {
    image_base64: b64,
    mode: 'img2img',
    gen_mode: 'preset',
    preset_id: selectedEffectId.value || 0,
    compound_preset_id: 0,
    denoising_strength: effectStrength.value,
    tags: '',
    extra_negative_prompt: '',
    resolution_id: null,
    hires_profile_id: null,
    overrides: effectOverrides(selectedEffectId.value),
    preserve_identity: true,
    seed: seed ?? null,
    keep_color: effectIsMaterial.value && effectKeepColor.value,
  }
}

async function applyEffect() {
  if (applyingEffect.value || batchBusy.value || recipeRunning.value || lookBusy.value) return
  if (!currentB64.value) {
    error.value = t('export.error_no_image')
    return
  }
  if (!selectedEffectId.value) {
    error.value = t('post.error_no_effect')
    return
  }
  if (batchActive.value) {
    const seedVal = parseSeedInput(effectSeedInput.value)
    const presetId = selectedEffectId.value
    const strength = effectStrength.value
    const overrides = effectOverrides(presetId)
    await runBatchStep(async (b64, i) => {
      const seed = seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null
      const job = await trackBatchJob(api.enqueueFromImage({ ...effectJobParams(b64, seed), preset_id: presetId, denoising_strength: strength, overrides }))
      return job ? () => job.wait : ''
    })
    return
  }
  applyingEffect.value = true
  error.value = ''
  successMsg.value = ''
  effectJobIds.value = new Set()
  try {
    const seedVal = parseSeedInput(effectSeedInput.value)
    const presetId = selectedEffectId.value
    const strength = effectStrength.value
    const jobId = await api.enqueueFromImage({ ...effectJobParams(currentB64.value, seedVal), ...effectMaskParam() })
    if (jobId) {
      effectJobIds.value.add(jobId)
      journalPending.set(jobId, { preset_id: presetId, strength })
    } else {
      applyingEffect.value = false
    }
  } catch (e) {
    error.value = String(e)
    applyingEffect.value = false
  }
}

async function interruptEffect() {
  if (effectJobIds.value.size > 0) {
    const ids = [...effectJobIds.value]
    effectJobIds.value = new Set()
    for (const id of ids) {
      journalPending.delete(id)
      try { await api.cancelQueueJob(id) } catch {}
    }
  }
  applyingEffect.value = false
}

const autoStyleBusy = ref(false)
const autoStyleRecipe = ref(null)

async function runAutoStyle() {
  if (!currentB64.value || autoStyleBusy.value) return
  autoStyleBusy.value = true
  autoStyleRecipe.value = null
  error.value = ''
  successMsg.value = ''
  try {
    const desc = await api.describeForStyle(currentB64.value)
    const recipe = await api.compilePostStyle(desc)
    autoStyleRecipe.value = { vision: desc, steps: recipe.steps || [], adjust: recipe.adjust || {} }
  } catch (e) {
    error.value = String(e)
  } finally {
    autoStyleBusy.value = false
  }
}

function applyAutoStyleRecipe() {
  const r = autoStyleRecipe.value
  if (!r) return
  autoStyleRecipe.value = null
  applyRecipe({ steps: JSON.stringify(r.steps), adjust: JSON.stringify(r.adjust) })
}

function onEditApplyRecipe(payload) {
  applyRecipe({ steps: JSON.stringify(payload.steps || []), adjust: JSON.stringify(payload.adjust || {}) })
}

const lookBusy = ref(false)
const lookResultPrompt = ref('')
const lookResetFailed = ref(false)

async function onEditApplyLook({ text, strength }) {
  if (lookBusy.value || applyingEffect.value || batchBusy.value || recipeRunning.value) return
  if (!currentB64.value) {
    error.value = t('export.error_no_image')
    return
  }
  if (!text) return
  if (batchActive.value) {
    await runBatchStep(async (b64) => {
      const job = await trackBatchJob(api.enqueueLook({ image_base64: b64, text, strength, seed: null }))
      return job ? () => job.wait : ''
    })
    return
  }
  lookBusy.value = true
  lookResetFailed.value = false
  lookResultPrompt.value = ''
  error.value = ''
  successMsg.value = ''
  try {
    const res = await api.applyLookDirect(currentB64.value, text, strength, null)
    if (res?.image) {
      appliedEffects.value.push({ prompt: res.effective_prompt || '', strength })
      onToolResult(res.image)
      lookResultPrompt.value = res.effective_prompt || ''
      lookResetFailed.value = !!res.preserve_identity_reset_failed
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    lookBusy.value = false
  }
}

async function onRemixApplyBatch(params) {
  if (!batchActive.value || lookBusy.value || applyingEffect.value || batchBusy.value || recipeRunning.value) return
  await runBatchStep(async (b64) => {
    const job = await trackBatchJob(api.enqueueFromImage({ ...params, image_base64: b64 }))
    return job ? () => job.wait : ''
  })
}

async function onEditApplyBatch(params) {
  if (!batchActive.value || lookBusy.value || applyingEffect.value || batchBusy.value || recipeRunning.value) return
  await runBatchStep(async (b64) => {
    const job = await trackBatchJob(api.enqueueEdit({ ...params, image_base64: b64 }))
    return job ? () => job.wait : ''
  })
}

async function onEditSaveRecipe(payload) {
  try {
    await api.savePostRecipe({
      name: payload.name,
      steps: JSON.stringify(payload.steps || []),
      adjust: JSON.stringify(payload.adjust || {}),
    })
    await loadRecipes()
  } catch (e) {
    error.value = String(e)
  }
}

const LAYER_HISTORY_MAX = 15
const layerUndo = ref([])
const layerRedo = ref([])
const canUndoLayer = computed(() => layerUndo.value.length > 0)
const canRedoLayer = computed(() => layerRedo.value.length > 0)
const canUndoView = computed(() => batchActive.value ? batch.value.canUndo && !batchBusy.value && !batchExporting.value : maskMode.value ? !!maskComp.value?.canUndoMask : canUndoLayer.value)
const canRedoView = computed(() => batchActive.value ? false : maskMode.value ? !!maskComp.value?.canRedoMask : canRedoLayer.value)

function undoView() {
  if (batchActive.value) undoBatch()
  else if (maskMode.value) maskComp.value?.undoMask()
  else undoLayer()
}

function redoView() {
  if (batchActive.value) return
  if (maskMode.value) maskComp.value?.redoMask()
  else redoLayer()
}

function pushLayerHistory() {
  if (!currentB64.value) return
  layerUndo.value.push(currentB64.value)
  if (layerUndo.value.length > LAYER_HISTORY_MAX) layerUndo.value.shift()
  layerRedo.value = []
}

function undoLayer() {
  if (layerUndo.value.length === 0) return
  layerRedo.value.push(currentB64.value)
  currentB64.value = layerUndo.value.pop()
  loadImageDimensions(currentB64.value)
}

function redoLayer() {
  if (layerRedo.value.length === 0) return
  layerUndo.value.push(currentB64.value)
  if (layerUndo.value.length > LAYER_HISTORY_MAX) layerUndo.value.shift()
  currentB64.value = layerRedo.value.pop()
  loadImageDimensions(currentB64.value)
}

function resetLayer() {
  if (batchActive.value) {
    resetBatch()
    return
  }
  pushLayerHistory()
  currentB64.value = sourceB64.value
  appliedEffects.value = []
}

function onToolResult(b64) {
  if (!b64) return
  pushLayerHistory()
  currentB64.value = b64
  loadImageDimensions(b64)
}

function onRemixSource(b64) {
  if (!b64) return
  pushLayerHistory()
  setSource(b64, 'remix_image', true)
}

function consumeInitialImage() {
  if (!props.initialImage) return
  setSource(props.initialImage, 'dropped_image')
  emit('clear-initial-image')
}

watch(() => props.initialImage, () => consumeInitialImage())

const exportPresets = ref([])
const selectedPresetId = ref(null)
const format = ref('png')
const width = ref(0)
const height = ref(0)
const lockRatio = ref(true)
const quality = ref(90)
const interpolation = ref('lanczos')
const filename = ref('')

const exporting = ref(false)
const error = ref('')
const successMsg = ref('')

const editingPreset = ref(null)
const presetName = ref('')
const showPresetForm = ref(false)

const formats = [
  { value: 'png', label: 'PNG' },
  { value: 'jpeg', label: 'JPEG' },
]

const interpolations = [
  { value: 'nearest', label: 'Nearest' },
  { value: 'linear', label: 'Linear' },
  { value: 'lanczos', label: 'Lanczos' },
]

const showQuality = computed(() => format.value !== 'png')

const aspectRatio = computed(() => {
  if (!imageInfo.value.width || !imageInfo.value.height) return 1
  return imageInfo.value.width / imageInfo.value.height
})

watch(width, (w) => {
  if (lockRatio.value && w > 0 && aspectRatio.value) {
    height.value = Math.round(w / aspectRatio.value)
  }
})

watch(height, (h) => {
  if (lockRatio.value && h > 0 && aspectRatio.value) {
    width.value = Math.round(h * aspectRatio.value)
  }
})

watch(format, () => {
  updateFilename()
})

function updateFilename() {
  const base = sourceName.value.replace(/\.[^.]+$/, '') || 'postprocess'
  filename.value = `${base}_${new Date().toISOString().slice(0, 10)}.${format.value}`
}

function loadImageDimensions(base64) {
  const img = new Image()
  img.onload = () => {
    imageInfo.value = { width: img.naturalWidth, height: img.naturalHeight }
    width.value = img.naturalWidth
    height.value = img.naturalHeight
  }
  img.src = b64Url(base64)
}

function setSource(b64, name, keepLayerHistory = false) {
  if (batchActive.value) exitBatch(false)
  sourceB64.value = b64
  if (currentB64.value === b64) effectMask.applySource(b64, false)
  else effectMaskReset = true
  currentB64.value = b64
  sourceName.value = (name || 'image').replace(/\.[^.]+$/, '') || 'image'
  resetAdjustAll()
  if (!keepLayerHistory) {
    layerUndo.value = []
    layerRedo.value = []
    appliedEffects.value = []
  }
  error.value = ''
  successMsg.value = ''
  loadImageDimensions(b64)
  updateFilename()
}

async function loadLastImage() {
  try {
    const item = await api.getActiveSessionItem()
    if (item) {
      const image = await api.getSessionImage(item.id)
      if (image) {
        setSource(image, item.file_name || 'generated_image')
        return
      }
    }
  } catch {}
}

async function uploadImage() {
  try {
    const result = await api.readImageFile()
    if (result) setSource(result, 'uploaded_image')
  } catch {}
}

async function pasteFromClipboard() {
  try {
    const base64 = await api.readClipboardImage()
    if (base64) setSource(base64, 'pasted_image')
  } catch (e) {
    error.value = String(e)
  }
}

function handlePaste(e) {
  const items = e.clipboardData?.items
  if (!items) return
  for (const item of items) {
    if (item.type.startsWith('image/')) {
      e.preventDefault()
      const file = item.getAsFile()
      if (!file) continue
      if (file.size > MAX_IMAGE_SIZE) {
        error.value = t('fi.error_image_too_large')
        return
      }
      const reader = new FileReader()
      reader.onload = () => {
        const base64 = reader.result.split(',')[1]
        if (base64) setSource(base64, 'pasted_image')
      }
      reader.readAsDataURL(file)
      return
    }
  }
}

function onPageKeydown(e) {
  if (!(e.ctrlKey || e.metaKey)) return
  const tag = e.target?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || e.target?.isContentEditable) return
  const isZ = e.code === 'KeyZ' || e.key.toLowerCase() === 'z'
  const isY = e.code === 'KeyY' || e.key.toLowerCase() === 'y'
  if (!isZ && !isY) return
  e.preventDefault()
  const redo = isY || e.shiftKey
  if (batchActive.value) {
    if (!redo) undoBatch()
    return
  }
  if (maskMode.value) {
    if (redo) maskComp.value?.redoMask()
    else maskComp.value?.undoMask()
    return
  }
  if (redo) redoLayer()
  else undoLayer()
}

const isDragOver = ref(false)

function onDrop(e) {
  isDragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    error.value = t('fi.error_drop_image')
    return
  }
  if (file.size > MAX_IMAGE_SIZE) {
    error.value = t('fi.error_image_too_large')
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    const base64 = reader.result.split(',')[1]
    if (base64) setSource(base64, file.name)
  }
  reader.readAsDataURL(file)
}

function applyPreset(preset) {
  selectedPresetId.value = preset.id
  format.value = preset.format
  interpolation.value = preset.interpolation
  quality.value = preset.quality || 90
  lockRatio.value = preset.lock_ratio

  if (preset.width > 0 || preset.height > 0) {
    if (preset.lock_ratio && preset.width > 0 && preset.height === 0) {
      width.value = preset.width
    } else if (preset.lock_ratio && preset.height > 0 && preset.width === 0) {
      height.value = preset.height
    } else {
      width.value = preset.width || imageInfo.value.width
      height.value = preset.height || imageInfo.value.height
    }
  } else {
    width.value = imageInfo.value.width
    height.value = imageInfo.value.height
  }
  updateFilename()
}

function onPresetSelect(id) {
  const p = exportPresets.value.find(ep => ep.id === Number(id))
  if (p) applyPreset(p)
}

async function savePreset() {
  try {
    const data = {
      name: presetName.value,
      format: format.value,
      width: width.value === imageInfo.value.width ? 0 : width.value,
      height: height.value === imageInfo.value.height ? 0 : height.value,
      lock_ratio: lockRatio.value,
      quality: quality.value,
      interpolation: interpolation.value,
    }
    if (editingPreset.value) {
      data.id = editingPreset.value.id
    }
    await api.saveExportPreset(data)
    await loadExportPresets()
    editingPreset.value = null
    presetName.value = ''
    showPresetForm.value = false
  } catch (e) {
    error.value = String(e)
  }
}

function startEditPreset(preset) {
  editingPreset.value = preset
  presetName.value = preset.name
  showPresetForm.value = true
}

async function deletePreset(id) {
  try {
    await api.deleteExportPreset(id)
    if (selectedPresetId.value === id) selectedPresetId.value = null
    await loadExportPresets()
  } catch (e) {
    error.value = String(e)
  }
}

async function loadExportPresets() {
  try {
    exportPresets.value = await api.listExportPresets()
  } catch {}
}

const recipes = ref([])
const recipeRunning = ref(false)
const recipeStepIdx = ref(0)
const recipeStepTotal = ref(0)
const recipeCurrentId = ref(null)
const showRecipeForm = ref(false)
const recipeName = ref('')
const pendingDeleteRecipeId = ref(null)
let recipeDeleteTimer = null
let recipeCancelled = false
const recipeJobResolvers = new Map()

function recipeSteps(r) {
  try {
    const steps = JSON.parse(r.steps || '[]')
    return Array.isArray(steps) ? steps : []
  } catch {
    return []
  }
}

function recipeAdjustActive(r) {
  let a = null
  try { a = JSON.parse(r.adjust || '{}') } catch { return false }
  if (!a || typeof a !== 'object') return false
  return Object.keys(sliderMeta).some(k => {
    if (a[k] === undefined) return false
    const v = a[k]
    const d = sliderMeta[k].def
    if (k === 'white_point' || k === 'gamma') return v !== d && v !== 0
    return v !== d
  })
}

function recipeMeta(r) {
  const n = recipeSteps(r).length
  if (n === 0) return t('post.recipe_steps_only_adjust')
  return t('post.recipe_steps', { n }) + (recipeAdjustActive(r) ? ' ' + t('post.recipe_steps_adjust') : '')
}

async function loadRecipes() {
  try {
    recipes.value = await api.listPostRecipes() || []
  } catch {
    recipes.value = []
  }
}

function openRecipeForm() {
  recipeName.value = ''
  showRecipeForm.value = true
  setOpenSection('recipes')
}

async function saveRecipe() {
  try {
    await api.savePostRecipe({
      name: recipeName.value,
      steps: JSON.stringify(appliedEffects.value.map(e => e.preset_id
        ? { preset_id: e.preset_id, strength: e.strength }
        : { prompt: e.prompt || '', strength: e.strength })),
      adjust: JSON.stringify(adjustParams()),
    })
    recipeName.value = ''
    showRecipeForm.value = false
    await loadRecipes()
  } catch (e) {
    error.value = String(e)
  }
}

async function deleteRecipe(id) {
  if (recipeRunning.value) return
  if (pendingDeleteRecipeId.value === id) {
    clearTimeout(recipeDeleteTimer)
    pendingDeleteRecipeId.value = null
    try {
      await api.deletePostRecipe(id)
      await loadRecipes()
    } catch (e) {
      error.value = String(e)
    }
  } else {
    clearTimeout(recipeDeleteTimer)
    pendingDeleteRecipeId.value = id
    recipeDeleteTimer = setTimeout(() => { pendingDeleteRecipeId.value = null }, 3000)
  }
}

function recipeJobParams(presetId, strength, b64) {
  return {
    image_base64: b64,
    mode: 'img2img',
    gen_mode: 'preset',
    preset_id: presetId,
    compound_preset_id: 0,
    denoising_strength: strength,
    tags: '',
    extra_negative_prompt: '',
    resolution_id: null,
    hires_profile_id: null,
    overrides: null,
    preserve_identity: true,
    seed: null,
  }
}

function waitForRecipeJob(jobId) {
  return new Promise(resolve => { recipeJobResolvers.set(jobId, resolve) })
}

function enqueueRecipeStep(step, img) {
  if (step.prompt && !step.preset_id) {
    const strength = typeof step.strength === 'number' ? step.strength : 0.5
    return api.enqueueLook({ image_base64: img, text: step.prompt, strength, seed: null })
  }
  return api.enqueueFromImage(recipeJobParams(step.preset_id, step.strength, img))
}

async function trackBatchJob(enqueued) {
  const jobId = await enqueued
  if (!jobId) return null
  if (batchCancelled) {
    try { await api.cancelQueueJob(jobId) } catch {}
    return null
  }
  return { wait: waitForRecipeJob(jobId) }
}

async function runRecipeChain(img, steps) {
  const first = await trackBatchJob(enqueueRecipeStep(steps[0], img))
  if (!first) return ''
  return async () => {
    let cur = await first.wait
    for (let i = 1; i < steps.length && cur; i++) {
      if (batchCancelled) return ''
      const job = await trackBatchJob(enqueueRecipeStep(steps[i], cur))
      if (!job) return ''
      cur = await job.wait
    }
    return cur || ''
  }
}

async function applyRecipe(r) {
  if (recipeRunning.value || applyingEffect.value || batchBusy.value || lookBusy.value) return
  const steps = recipeSteps(r)
  let adjustObj = {}
  try {
    adjustObj = JSON.parse(r.adjust || '{}') || {}
  } catch {}
  if (steps.length === 0) {
    Object.assign(adjust, adjustObj)
    return
  }
  if (batchActive.value) {
    await runBatchStep(b64 => runRecipeChain(b64, steps), (res) => {
      if (!res.cancelled) Object.assign(adjust, adjustObj)
    })
    return
  }
  if (!currentB64.value) {
    error.value = t('export.error_no_image')
    return
  }
  recipeRunning.value = true
  recipeCancelled = false
  recipeCurrentId.value = r.id
  recipeStepTotal.value = steps.length
  recipeStepIdx.value = 0
  error.value = ''
  successMsg.value = ''
  let img = currentB64.value
  let aborted = 0
  try {
    for (let i = 0; i < steps.length; i++) {
      if (recipeCancelled) break
      recipeStepIdx.value = i + 1
      const step = steps[i]
      if (step.prompt && !step.preset_id) {
        const stepStrength = typeof step.strength === 'number' ? step.strength : 0.5
        let res = null
        try {
          res = await api.applyLookDirect(img, step.prompt, stepStrength, null)
        } catch {}
        if (!res?.image) {
          aborted = i + 1
          break
        }
        appliedEffects.value.push({ prompt: res.effective_prompt || '', strength: stepStrength })
        onToolResult(res.image)
        img = res.image
        continue
      }
      let jobId = 0
      try {
        jobId = await api.enqueueFromImage(recipeJobParams(step.preset_id, step.strength, img))
      } catch {}
      if (!jobId) {
        aborted = i + 1
        break
      }
      if (recipeCancelled) {
        journalPending.delete(jobId)
        try { await api.cancelQueueJob(jobId) } catch {}
        break
      }
      journalPending.set(jobId, { preset_id: step.preset_id, strength: step.strength })
      const res = await waitForRecipeJob(jobId)
      if (recipeCancelled) break
      if (!res) {
        aborted = i + 1
        break
      }
      onToolResult(res)
      img = res
    }
  } finally {
    if (aborted) error.value = t('post.recipe_aborted', { i: aborted })
    else if (!recipeCancelled) Object.assign(adjust, adjustObj)
    recipeRunning.value = false
    recipeCurrentId.value = null
  }
}

async function interruptRecipe() {
  recipeCancelled = true
  for (const [id, resolve] of [...recipeJobResolvers]) {
    recipeJobResolvers.delete(id)
    journalPending.delete(id)
    try { await api.cancelQueueJob(id) } catch {}
    resolve('')
  }
}

function resetSize() {
  width.value = imageInfo.value.width
  height.value = imageInfo.value.height
}

function openSavePreset() {
  editingPreset.value = null
  presetName.value = ''
  showPresetForm.value = true
}

function exportParamsFor(name, imageBase64, outputDir) {
  return {
    image_base64: imageBase64,
    format: format.value,
    width: width.value,
    height: height.value,
    lock_ratio: lockRatio.value,
    quality: quality.value,
    interpolation: interpolation.value,
    filename: name,
    ...(finishActive.value ? { finish: finishParams() } : {}),
    ...(outputDir ? { output_dir: outputDir } : {}),
  }
}

const batchFolder = ref('')

async function resolveBatchFolder() {
  if (batchFolder.value) return batchFolder.value
  try {
    const s = await api.getSettings()
    const p = s && s.file_browser_path
    if (p) {
      batchFolder.value = p
      return p
    }
  } catch {}
  try {
    const picked = await api.selectBrowserFolder()
    if (picked) batchFolder.value = picked
  } catch {}
  return batchFolder.value
}

async function changeBatchFolder() {
  try {
    const picked = await api.selectBrowserFolder()
    if (picked) batchFolder.value = picked
  } catch {}
}

async function doExport() {
  if (batchActive.value) {
    await exportBatch()
    return
  }
  if (!currentB64.value) {
    error.value = t('export.error_no_image')
    return
  }
  exporting.value = true
  error.value = ''
  successMsg.value = ''
  try {
    const path = await api.exportImage(exportParamsFor(filename.value, currentB64.value), adjustParams())
    if (path) {
      successMsg.value = t('export.saved_to', { path })
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    exporting.value = false
  }
}

const { postQueue, clearPostQueue } = useRemixQueue()

const { sdProgress: genProgress, preview: genPreview } = useGenerationProgress()
const sectionBusy = reactive({ edit: false, remix: false, object: false, repair: false })
const genBusy = computed(() => sectionBusy.edit || sectionBusy.remix || sectionBusy.object || sectionBusy.repair || applyingEffect.value || lookBusy.value || recipeRunning.value || batchBusy.value)
const genActive = computed(() => !!genProgress.value && genProgress.value.progress > 0 && genProgress.value.progress < 1)
const livePreview = computed(() => (genBusy.value && genActive.value && genPreview.value) || '')
watch(genBusy, (busy) => {
  if (busy) {
    genProgress.value = null
    genPreview.value = ''
  }
})
function parseQueueResult(result) {
  try {
    return typeof result === 'string' ? JSON.parse(result) : result
  } catch {
    return null
  }
}

function onQueueCompleted(data) {
  if (!data) return
  if (data.job_id && journalPending.has(data.job_id)) {
    const entry = journalPending.get(data.job_id)
    journalPending.delete(data.job_id)
    if (parseQueueResult(data.result)?.image_base64) appliedEffects.value.push(entry)
  }
  if (data.job_id && recipeJobResolvers.has(data.job_id)) {
    const resolve = recipeJobResolvers.get(data.job_id)
    recipeJobResolvers.delete(data.job_id)
    resolve(parseQueueResult(data.result)?.image_base64 || '')
  }
  if (data.job_id && effectJobIds.value.has(data.job_id)) {
    effectJobIds.value.delete(data.job_id)
    const r = parseQueueResult(data.result)
    if (r?.image_base64) {
      pushLayerHistory()
      currentB64.value = r.image_base64
      loadImageDimensions(r.image_base64)
    }
    if (effectJobIds.value.size === 0) applyingEffect.value = false
  }
}

function onQueueFailed(data) {
  if (!data) return
  handleJobTerminal(data)
}

function onQueuePaused(data) {
  if (!data?.job_id) return
  if ((data.retry_count || 0) < (data.max_retries || 0)) return
  handleJobTerminal(data)
}

function handleJobTerminal(data) {
  if (data.job_id && journalPending.has(data.job_id)) journalPending.delete(data.job_id)
  if (data.job_id && recipeJobResolvers.has(data.job_id)) {
    const resolve = recipeJobResolvers.get(data.job_id)
    recipeJobResolvers.delete(data.job_id)
    resolve('')
  }
  if (data.job_id && effectJobIds.value.has(data.job_id)) {
    effectJobIds.value.delete(data.job_id)
    if (data.error) error.value = data.error
    if (effectJobIds.value.size === 0) applyingEffect.value = false
  }
}

async function onQueueChanged() {
  if (effectJobIds.value.size === 0 && recipeJobResolvers.size === 0) return
  let jobs
  try {
    jobs = await api.getQueue() || []
  } catch {
    return
  }
  const byId = new Map(jobs.map(j => [j.id, j]))
  for (const id of [...effectJobIds.value]) {
    const job = byId.get(id)
    if (job && job.status !== 'cancelled') continue
    effectJobIds.value.delete(id)
    journalPending.delete(id)
    if (effectJobIds.value.size === 0) applyingEffect.value = false
  }
  for (const [id, resolve] of [...recipeJobResolvers]) {
    const job = byId.get(id)
    if (job && job.status !== 'cancelled') continue
    recipeJobResolvers.delete(id)
    journalPending.delete(id)
    resolve('')
  }
}

async function consumePostQueue() {
  if (!postQueue.value.length) return
  const items = postQueue.value.slice()
  clearPostQueue()
  if (items.length > 1) {
    await startBatch(items)
    return
  }
  try {
    const b64 = await readBatchImage(items[0].path)
    if (b64) setSource(b64, items[0].name)
  } catch (e) {
    error.value = String(e)
  }
}

let pageMounted = false

watch(() => postQueue.value.length, (n) => {
  if (n > 0 && pageMounted) consumePostQueue()
})

let offCompleted = () => {}
let offFailed = () => {}
let offPaused = () => {}
let offQueueChanged = () => {}
let offSessionSelected = () => {}

onMounted(async () => {
  openSection.value = props.initialSection || 'effect'
  document.addEventListener('paste', handlePaste)
  document.addEventListener('keydown', onPageKeydown)
  window.addEventListener('resize', onWindowResize)
  measureArea()
  offCompleted = EventsOn('queue:completed', onQueueCompleted)
  offFailed = EventsOn('queue:failed', onQueueFailed)
  offPaused = EventsOn('queue:paused', onQueuePaused)
  offQueueChanged = EventsOn('queue:changed', onQueueChanged)
  offSessionSelected = EventsOn('session:selected', () => { loadLastImage() })
  await loadPresets()
  await loadEffectModel()
  await loadExportPresets()
  await loadRecipes()
  consumeInitialImage()
  await consumePostQueue()
  pageMounted = true
  if (!sourceB64.value) {
    await loadLastImage()
  }
})

onUnmounted(() => {
  if (batch.value) {
    interruptBatch()
    batch.value = null
    api.batchReset().catch(() => {})
  }
  batchCancelled = true
  recipeCancelled = true
  for (const [id, resolve] of [...recipeJobResolvers]) {
    recipeJobResolvers.delete(id)
    journalPending.delete(id)
    resolve('')
  }
  clearTimeout(previewTimer)
  clearTimeout(suppressDblTimer)
  window.removeEventListener('resize', onWindowResize)
  document.removeEventListener('paste', handlePaste)
  document.removeEventListener('keydown', onPageKeydown)
  offCompleted()
  offFailed()
  offPaused()
  offQueueChanged()
  offSessionSelected()
})
</script>

<template>
  <div class="page-enter post-page">
    <StatusMessage v-if="error" :text="error" @close="error = ''" />
    <StatusMessage v-if="successMsg" kind="success" :timeout="8000" :text="successMsg" @close="successMsg = ''" />
    <StatusMessage v-if="lookResetFailed" kind="warning" @close="lookResetFailed = false">{{ t('fi.preserve_identity_reset_failed') }}</StatusMessage>

    <div class="post-workspace">
      <div class="post-preview-col">
        <div class="card post-preview-card">
          <div class="card-header">
            <span class="card-title">{{ t('post.source') }}</span>
            <div class="card-actions">
              <button class="btn btn-sm btn-secondary" @click="loadLastImage">{{ t('export.btn_load_last') }}</button>
              <button class="btn btn-sm btn-secondary" @click="uploadImage">{{ t('export.btn_upload') }}</button>
              <button class="btn btn-sm btn-secondary" @click="pasteFromClipboard">{{ t('fi.btn_paste') }}</button>
            </div>
          </div>
          <div class="split-body">
            <div v-if="sourceB64" class="viewfinder" aria-hidden="true"><span></span><span></span><span></span><span></span></div>
            <div
              v-if="sourceB64"
              ref="splitWrapRef"
              class="split-area"
              :class="{ pannable: panEnabled && !maskMode, panning, masking: maskMode }"
              @pointerdown="onViewPointerDown"
              @pointermove="onViewPointerMove"
              @pointerup="onViewPointerUp"
              @pointercancel="onViewPointerUp"
              @pointerleave="onViewPointerLeave"
              @wheel="onViewWheel"
              @dblclick="toggleFit100"
            >
              <img
                class="split-img split-after"
                :src="b64Url(previewB64 || currentB64)"
                :style="layerStyle"
                alt="After"
                draggable="false"
                @load="onAfterImgLoad"
              />
              <div v-if="!maskMode && !finishActive" class="split-before-mask" :style="{ clipPath: `inset(0 ${100 - splitPos}% 0 0)` }">
                <img
                  class="split-img split-before"
                  :src="b64Url(sourceB64)"
                  :style="layerStyle"
                  alt="Before"
                  draggable="false"
                />
              </div>
              <canvas v-if="maskMode" ref="maskViewRef" class="split-img split-mask" :class="{ 'mask-overlay-hidden': !maskOverlayVisible }" :style="layerStyle"></canvas>
              <div
                v-if="maskMode && brushCursor.visible"
                class="brush-cursor"
                :style="{ left: brushCursor.x + 'px', top: brushCursor.y + 'px', width: brushDiameter + 'px', height: brushDiameter + 'px' }"
              ></div>
              <div v-if="!maskMode && !finishActive" class="split-divider" :style="{ left: splitPos + '%' }">
                <div
                  class="split-handle"
                  @pointerdown="onSplitDown"
                  @pointermove="onSplitMove"
                  @pointerup="onSplitUp"
                  @pointercancel="onSplitUp"
                ><ArrowLeftRight :size="14" /></div>
              </div>
              <span v-if="!maskMode && !finishActive" class="split-label split-label-before">{{ t('post.before') }}</span>
              <span v-if="!maskMode && !finishActive" class="split-label split-label-after">{{ t('post.after') }}</span>
              <span v-if="previewing" class="split-processing"><span class="spinner"></span> {{ t('post.processing') }}</span>
              <div v-if="genBusy" class="gen-overlay" data-testid="gen-overlay">
                <img v-if="livePreview" class="gen-overlay-img" :src="livePreview" alt="" draggable="false" />
                <span v-else class="spinner gen-overlay-spinner"></span>
                <div v-if="genActive" class="gen-overlay-bar">
                  <div class="gen-overlay-stats">
                    <span>{{ Math.round(genProgress.progress * 100) }}%</span>
                    <span>{{ t('progress.sd_step', { current: Math.round(genProgress.progress * genProgress.steps), total: genProgress.steps }) }}<span v-if="genProgress.etaRelative > 0"> — ~{{ Math.ceil(genProgress.etaRelative) }}s</span></span>
                  </div>
                  <div class="gen-overlay-track"><div :style="{ width: (genProgress.progress * 100) + '%' }"></div></div>
                </div>
              </div>
              <div class="split-zoom" @pointerdown.stop @click.stop @dblclick.stop @wheel.stop>
                <button
                  v-if="maskMode"
                  class="split-zoom-btn"
                  :title="maskOverlayVisible ? t('post.mask_hide') : t('post.mask_show')"
                  :aria-label="maskOverlayVisible ? t('post.mask_hide') : t('post.mask_show')"
                  @click="maskOverlayVisible = !maskOverlayVisible"
                >
                  <EyeOff v-if="maskOverlayVisible" :size="14" />
                  <Eye v-else :size="14" />
                </button>
                <button
                  class="split-zoom-btn"
                  :title="t('post.undo')"
                  :aria-label="t('post.undo')"
                  :disabled="!canUndoView"
                  @click="undoView"
                ><Undo2 :size="14" /></button>
                <button
                  class="split-zoom-btn"
                  :title="t('post.redo')"
                  :aria-label="t('post.redo')"
                  :disabled="!canRedoView"
                  @click="redoView"
                ><Redo2 :size="14" /></button>
                <button
                  class="split-zoom-btn"
                  :title="t('viewer.zoom_out')"
                  :aria-label="t('viewer.zoom_out')"
                  :disabled="isFit && fitPercent() <= ZOOM_MIN"
                  @click="zoomBy(-10)"
                >&minus;</button>
                <button
                  class="split-zoom-btn split-zoom-label"
                  :title="t('viewer.zoom_presets')"
                  :aria-expanded="zoomMenuOpen"
                  aria-haspopup="menu"
                  @click="onZoomIndicatorClick"
                >{{ zoomLabel }}</button>
                <button
                  class="split-zoom-btn"
                  :title="t('viewer.zoom_in')"
                  :aria-label="t('viewer.zoom_in')"
                  :disabled="!isFit && zoomPct >= ZOOM_MAX"
                  @click="zoomBy(10)"
                >&plus;</button>
                <div v-if="zoomMenuOpen" class="split-zoom-menu" role="menu">
                  <button :class="{ active: isFit }" @click="setFit">{{ t('viewer.zoom_fit') }}</button>
                  <button
                    v-for="p in ZOOM_PRESETS"
                    :key="p"
                    :class="{ active: !isFit && zoomPct === p }"
                    @click="enterZoom(p)"
                  >{{ p }}%</button>
                </div>
              </div>
            </div>
            <div
              v-else
              class="drop-zone"
              :class="{ 'drop-zone-active': isDragOver }"
              @dragover.prevent="isDragOver = true"
              @dragleave="isDragOver = false"
              @drop.prevent="onDrop"
              @click="uploadImage()"
            >
              <div style="color: var(--text-dim); display: flex;"><ImageIcon :size="32" :stroke-width="1.5" /></div>
              <p style="color: var(--text-dim); margin-top: 8px;">{{ t('fi.drop_image') }}</p>
            </div>
          </div>
          <div v-if="sourceB64" class="split-meta">
            <span v-if="imageInfo.width">{{ imageInfo.width }} x {{ imageInfo.height }}</span>
            <button v-if="batchActive ? batch.canUndo && !batchBusy && !batchExporting : currentB64 !== sourceB64" class="btn btn-sm btn-secondary" @click="resetLayer">{{ t('post.btn_reset_layer') }}</button>
          </div>
          <div v-if="batchActive" class="batch-panel">
            <div class="batch-bar">
              <span class="batch-title">{{ t('post.batch_title', { n: batch.items.length }) }}</span>
              <span class="batch-steps">{{ t('post.batch_steps', { n: batch.steps }) }}</span>
              <template v-if="batchBusy || batchExporting">
                <span class="spinner" style="width: 12px; height: 12px; border-width: 2px;"></span>
                <span>{{ t('post.batch_progress', { done: batchProgress.done, total: batchProgress.total }) }}</span>
                <button class="btn btn-sm btn-secondary" @click="stopBatch">{{ t('progress.btn_interrupt') }}</button>
              </template>
              <span v-else-if="batchMsg" class="batch-msg">{{ batchMsg }}</span>
              <button class="btn btn-sm btn-secondary batch-exit" :disabled="batchBusy || batchExporting" @click="exitBatch(true)">{{ t('post.batch_exit') }}</button>
            </div>
            <div class="batch-strip">
              <div
                v-for="(item, i) in batch.items"
                :key="item.path"
                class="batch-thumb"
                :class="{ active: i === batchIndex, failed: item.failed }"
                role="button"
                tabindex="0"
                :title="item.failed ? item.name + ' — ' + t('post.batch_item_failed') : item.name"
                @click="showBatchItem(i)"
                @keydown.enter.self="showBatchItem(i)"
                @keydown.space.self.prevent="showBatchItem(i)"
              >
                <img v-if="batchThumbSrc(i)" :src="batchThumbSrc(i)" :alt="item.name" />
                <span v-else class="batch-thumb-name">{{ item.name }}</span>
                <button
                  class="batch-thumb-remove"
                  :title="t('post.batch_remove')"
                  :aria-label="t('post.batch_remove')"
                  :disabled="batchBusy || batchExporting"
                  @click.stop="removeBatchItem(i)"
                ><X :size="10" /></button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="post-controls">
        <div ref="editSectionRef" class="card" :class="{ collapsed: openSection !== 'edit' }">
          <div class="card-header card-toggle" @click="setOpenSection('edit')">
            <span class="card-heading"><span class="card-num mono">01</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_edit') }}</span><span class="card-subtitle">{{ t('post.sub_edit') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'edit' }">
            <div class="card-body">
              <EditSection :source="currentB64" :batch="batchActive" :look-busy="lookBusy || batchBusy" :look-result-prompt="lookResultPrompt" @result="onToolResult" @busy="v => sectionBusy.edit = v" @apply-look="onEditApplyLook" @apply-edit="onEditApplyBatch" @apply-recipe="onEditApplyRecipe" @save-recipe="onEditSaveRecipe" />
            </div>
          </div>
        </div>

        <div ref="remixSectionRef" class="card" :class="{ collapsed: openSection !== 'remix' }">
          <div class="card-header card-toggle" @click="setOpenSection('remix')">
            <span class="card-heading"><span class="card-num mono">02</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_remix') }}</span><span class="card-subtitle">{{ t('post.sub_remix') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'remix' }">
            <div class="card-body">
              <RemixSection :source="currentB64" :batch="batchActive" :batch-busy="batchBusy || batchExporting || recipeRunning || lookBusy || applyingEffect" @apply-batch="onRemixApplyBatch" :active="openSection === 'remix'" @result="onToolResult" @busy="v => sectionBusy.remix = v" @source="onRemixSource" />
            </div>
          </div>
        </div>

        <div ref="objectSectionRef" class="card" :class="{ collapsed: openSection !== 'object' }">
          <div class="card-header card-toggle" @click="setOpenSection('object')">
            <span class="card-heading"><span class="card-num mono">03</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_object') }}</span><span class="card-subtitle">{{ t('post.sub_object') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'object' }">
            <div class="card-body">
              <div v-if="batchActive" class="form-hint">{{ t('post.batch_unavailable') }}</div>
              <ObjectSection v-else ref="objectCompRef" :source="currentB64" :active="openSection === 'object'" @result="onToolResult" @busy="v => sectionBusy.object = v" />
            </div>
          </div>
        </div>

        <div ref="repairSectionRef" class="card" :class="{ collapsed: openSection !== 'repair' }">
          <div class="card-header card-toggle" @click="setOpenSection('repair')">
            <span class="card-heading"><span class="card-num mono">04</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_repair') }}</span><span class="card-subtitle">{{ t('post.sub_repair') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'repair' }">
            <div class="card-body">
              <div v-if="batchActive" class="form-hint">{{ t('post.batch_unavailable') }}</div>
              <RepairSection v-else ref="repairCompRef" :source="currentB64" :active="openSection === 'repair'" @result="onToolResult" @busy="v => sectionBusy.repair = v" />
            </div>
          </div>
        </div>

        <div ref="effectSectionRef" class="card" :class="{ collapsed: openSection !== 'effect' }">
          <div class="card-header card-toggle" @click="setOpenSection('effect')">
            <span class="card-heading"><span class="card-num mono">05</span><span class="card-heading-text"><span class="card-title">{{ t('post.effect_title') }}</span><span class="card-subtitle">{{ t('post.sub_effect') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'effect' }">
            <div class="card-body">
            <div class="adj-actions">
              <button
                class="btn btn-sm btn-secondary"
                :disabled="!currentB64 || autoStyleBusy"
                @click="runAutoStyle"
              >
                <span v-if="autoStyleBusy" class="spinner" style="width: 11px; height: 11px; border-width: 2px;"></span>
                <Sparkles v-else :size="13" />
                {{ autoStyleBusy ? t('post.autostyle_analyzing') : t('post.autostyle') }}
              </button>
            </div>
            <RecipeComposition
              v-if="autoStyleRecipe"
              :vision="autoStyleRecipe.vision"
              :steps="autoStyleRecipe.steps"
              :adjust="autoStyleRecipe.adjust"
            >
              <button
                class="btn btn-sm btn-primary"
                :disabled="recipeRunning || applyingEffect || batchBusy || batchExporting || lookBusy"
                @click="applyAutoStyleRecipe"
              >{{ t('post.autostyle_apply') }}</button>
              <button class="btn btn-sm btn-secondary" @click="autoStyleRecipe = null">{{ t('export.btn_cancel') }}</button>
            </RecipeComposition>
            <div class="fx-grid fx-grid-none">
              <button
                class="fx-card"
                :class="{ active: !selectedEffectId }"
                @click="selectedEffectId = null"
              >
                <span class="fx-name">{{ t('post.effect_none') }}</span>
              </button>
            </div>
            <div v-if="effectGroups.length" class="form-group fx-group-pick">
              <select class="form-select" :value="shownEffectGroup?.labelKey" @change="pickEffectGroup($event.target.value)">
                <option v-for="g in effectGroups" :key="g.labelKey" :value="g.labelKey">{{ t(g.labelKey) }} ({{ g.presets.length }})</option>
              </select>
              <div class="fx-selected">{{ t('post.effect_selected', { name: selectedEffectName }) }}</div>
            </div>
            <div v-if="shownEffectGroup" class="fx-group">
              <div class="fx-grid">
                <button
                  v-for="p in shownEffectGroup.presets"
                  :key="p.id"
                  class="fx-card"
                  :class="{ active: selectedEffectId === p.id }"
                  :title="p.name"
                  @click="selectEffect(p)"
                >
                  <span class="fx-name">{{ effectLabel(p.name) }}</span>
                  <span class="fx-orig">{{ p.name }}</span>
                </button>
              </div>
            </div>

            <div v-if="effectIsMaterial" class="form-group form-inline">
              <label class="form-label" :title="t('post.effect_keep_color_hint')">
                <input type="checkbox" v-model="effectKeepColor" /> {{ t('post.effect_keep_color') }}
              </label>
            </div>

            <div v-if="!batchActive" class="fx-mask">
              <button class="btn btn-sm" :class="effectMaskOn ? 'btn-primary' : 'btn-secondary'" :disabled="!currentB64" @click="effectMaskOn = !effectMaskOn">{{ t('post.effect_mask_btn') }}</button>
              <div v-if="effectIsPattern && !(effectMaskOn && effectHasMask)" class="form-hint">{{ t('post.effect_pattern_mask_hint') }}</div>
              <template v-if="effectMaskOn">
                <div class="form-hint">{{ t('post.effect_mask_hint') }}</div>
                <label class="form-label">{{ t('fi.label_brush_size', { value: effectBrush }) }}</label>
                <div class="fx-mask-row">
                  <input type="range" class="form-range" v-model.number="effectBrush" min="5" max="150" step="1" />
                  <button class="btn btn-sm btn-secondary" :disabled="!effectHasMask" @click="effectMask.clearMask">{{ t('fi.btn_clear_mask') }}</button>
                </div>
              </template>
            </div>

            <FoldBlock class="fx-model" :title="t('model.block_title')" :summary="effectModelSummary" @toggle="onEffectModelToggle">
              <div class="fx-model-body">
                <div class="form-hint">{{ t('model.hint_klein') }}</div>
                <div v-for="(l, i) in effectLoras" :key="i" class="fx-lora-row">
                  <select class="form-select fx-lora-name" v-model="l.name" @change="saveEffectLoras">
                    <option v-if="l.name && !effectLoraChoices.includes(l.name)" :value="l.name">{{ l.name }}</option>
                    <option v-for="n in effectLoraChoices" :key="n" :value="n">{{ n }}</option>
                  </select>
                  <input class="form-input fx-lora-weight" type="number" v-model.number="l.weight" min="-2" max="2" step="0.05" @change="saveEffectLoras" />
                  <button type="button" class="btn btn-sm btn-danger-quiet fx-lora-remove" @click="removeEffectLora(i)"><X :size="14" /></button>
                </div>
                <button type="button" class="btn btn-sm btn-secondary fx-lora-add" @click="addEffectLora">{{ t('post.effect_add_lora') }}</button>
              </div>
            </FoldBlock>

            <div class="form-group">
              <label class="form-label">{{ t('fi.seed_label') }}</label>
              <input class="form-input" type="number" v-model="effectSeedInput" min="0" step="1" :placeholder="t('fi.seed_placeholder')" />
            </div>

            <div class="fx-apply-bar">
            <div class="form-group">
              <label class="form-label">{{ t('post.effect_strength', { value: effectStrength.toFixed(2) }) }}</label>
              <input type="range" class="form-range" v-model.number="effectStrength" min="0.35" max="0.9" step="0.01" />
            </div>

            <button
              v-if="!applyingEffect && !batchBusy"
              class="btn btn-primary btn-block"
              :disabled="!currentB64 || !selectedEffectId || batchBusy || batchExporting || recipeRunning || lookBusy"
              @click="applyEffect"
            >
              {{ batchActive ? t('post.btn_apply_effect_all', { n: batch.items.length }) : t('post.btn_apply_effect') }}
            </button>
            <button v-else class="btn btn-primary btn-block btn-generating" @click="batchBusy ? interruptBatch() : interruptEffect()">
              <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
              {{ t('post.applying') }}
            </button>
            </div>
            <div v-if="appliedEffects.length" class="effect-log-row">
              <span>{{ t('post.recipe_log', { n: appliedEffects.length }) }}</span>
              <button
                class="btn-icon btn-icon-danger"
                :title="t('post.recipe_log_clear')"
                :aria-label="t('post.recipe_log_clear')"
                @click="appliedEffects = []"
              ><X :size="14" /></button>
            </div>
            <div class="form-hint">{{ t('post.effect_hint') }}</div>
            </div>
          </div>
        </div>

        <div ref="adjustSectionRef" class="card" :class="{ collapsed: openSection !== 'adjust' }">
          <div class="card-header card-toggle" @click="setOpenSection('adjust')">
            <span class="card-heading"><span class="card-num mono">06</span><span class="card-heading-text"><span class="card-title">{{ t('post.adjust_title') }}</span><span class="card-subtitle">{{ t('post.sub_adjust') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'adjust' }">
            <div class="card-body">
            <div class="adj-actions">
              <button class="btn btn-sm btn-secondary" :disabled="!currentB64 || autoAdjusting" @click="applyAutoAdjust('levels')">{{ t('post.auto_levels') }}</button>
              <button class="btn btn-sm btn-secondary" :disabled="!currentB64 || autoAdjusting" @click="applyAutoAdjust('wb')">{{ t('post.auto_wb') }}</button>
              <button class="btn btn-sm btn-secondary" @click="resetAdjustAll">{{ t('post.btn_reset_all') }}</button>
            </div>
            <div class="adj-groups">
              <div v-for="g in adjustGroups" :key="g.labelKey" class="adj-group">
                <div class="adj-group-title">{{ t(g.labelKey) }}</div>
                <div
                  v-for="key in g.keys"
                  :key="key"
                  class="adj-row"
                  :title="t('post.dblclick_reset')"
                  @dblclick="resetAdjustKey(key)"
                >
                  <div class="adj-row-head">
                    <span class="adj-label">{{ t('post.' + key) }}</span>
                    <span class="adj-value-wrap">
                      <button
                        v-if="adjust[key] !== sliderMeta[key].def"
                        type="button"
                        class="adj-reset"
                        :data-testid="'adj-reset-' + key"
                        :title="t('post.adjust_reset_one')"
                        :aria-label="t('post.adjust_reset_one') + ': ' + t('post.' + key)"
                        @click="resetAdjustKey(key)"
                      ><RotateCcw :size="11" /></button>
                      <span class="adj-value">{{ formatAdjust(key) }}</span>
                    </span>
                  </div>
                  <input
                    type="range"
                    class="form-range"
                    :min="sliderMeta[key].min"
                    :max="sliderMeta[key].max"
                    :step="sliderMeta[key].step"
                    v-model.number="adjust[key]"
                  />
                </div>
              </div>
            </div>
            </div>
          </div>
        </div>

        <div ref="finishSectionRef" class="card" :class="{ collapsed: openSection !== 'finish' }">
          <div class="card-header card-toggle" @click="setOpenSection('finish')">
            <span class="card-heading"><span class="card-num mono">07</span><span class="card-heading-text"><span class="card-title">{{ t('post.finish_title') }}</span><span class="card-subtitle">{{ t('post.sub_finish') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'finish' }">
            <div class="card-body">
            <div class="form-group">
              <label class="form-label">{{ t('post.finish_ratio') }}</label>
              <select class="form-select" v-model="finish.ratio" data-testid="finish-ratio">
                <option value="">{{ t('post.finish_ratio_none') }}</option>
                <option v-for="r in FINISH_RATIOS" :key="r" :value="r">{{ t('post.finish_ratio_' + r.replace(':', '_')) }}</option>
              </select>
            </div>
            <template v-if="finish.ratio">
              <div class="form-group">
                <label class="form-label">{{ t('post.finish_fit') }}</label>
                <select class="form-select" v-model="finish.fit" data-testid="finish-fit">
                  <option value="crop">{{ t('post.finish_fit_crop') }}</option>
                  <option value="blur">{{ t('post.finish_fit_blur') }}</option>
                  <option value="white">{{ t('post.finish_fit_white') }}</option>
                  <option value="black">{{ t('post.finish_fit_black') }}</option>
                </select>
              </div>
              <div v-if="finish.fit === 'crop'" class="form-group">
                <label class="form-label">{{ t('post.finish_offset') }}</label>
                <input type="range" class="form-range" min="0" max="1" step="0.01" v-model.number="finish.offset" data-testid="finish-offset" />
              </div>
            </template>
            <div class="form-group">
              <label class="form-label">{{ t('post.finish_frame') }}</label>
              <select class="form-select" v-model="finish.frame" data-testid="finish-frame">
                <option value="">{{ t('post.finish_frame_none') }}</option>
                <option value="white">{{ t('post.finish_frame_white') }}</option>
                <option value="black">{{ t('post.finish_frame_black') }}</option>
                <option value="polaroid">{{ t('post.finish_frame_polaroid') }}</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">{{ t('post.finish_caption') }}</label>
              <textarea class="form-control" rows="2" maxlength="200" v-model="finish.caption" data-testid="finish-caption" :placeholder="t('post.finish_caption_ph')"></textarea>
            </div>
            <div v-if="finish.caption.trim()" class="form-row">
              <div v-if="finish.frame !== 'polaroid'" class="form-group form-group-half">
                <label class="form-label">{{ t('post.finish_caption_pos') }}</label>
                <select class="form-select" v-model="finish.caption_pos" data-testid="finish-caption-pos">
                  <option value="bottom">{{ t('post.finish_pos_bottom') }}</option>
                  <option value="top">{{ t('post.finish_pos_top') }}</option>
                  <option value="center">{{ t('post.finish_pos_center') }}</option>
                </select>
              </div>
              <div class="form-group form-group-half">
                <label class="form-label">{{ t('post.finish_caption_size') }}</label>
                <select class="form-select" v-model="finish.caption_size" data-testid="finish-caption-size">
                  <option value="s">S</option>
                  <option value="m">M</option>
                  <option value="l">L</option>
                </select>
              </div>
            </div>
            <div class="form-hint">{{ t('post.finish_hint') }}</div>
            <button v-if="finishActive" class="btn btn-sm btn-secondary" @click="resetFinish">{{ t('post.finish_reset') }}</button>
            </div>
          </div>
        </div>

        <div ref="exportSectionRef" class="card" :class="{ collapsed: openSection !== 'export' }">
          <div class="card-header card-toggle" @click="setOpenSection('export')">
            <span class="card-heading"><span class="card-num mono">08</span><span class="card-heading-text"><span class="card-title">{{ t('export.export_settings') }}</span><span class="card-subtitle">{{ t('post.sub_export') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'export' }">
            <div class="card-body">
            <div class="form-group">
              <label class="form-label">{{ t('export.label_preset') }}</label>
              <div class="preset-row">
                <select class="form-select" :value="selectedPresetId" @change="onPresetSelect($event.target.value)">
                  <option :value="null">{{ t('export.custom') }}</option>
                  <option v-for="p in exportPresets" :key="p.id" :value="p.id">{{ p.name }}</option>
                </select>
                <button class="btn btn-sm btn-secondary" @click="openSavePreset">{{ t('export.btn_save_preset') }}</button>
              </div>
            </div>

            <div class="form-group">
              <label class="form-label">{{ t('export.label_format') }}</label>
              <select class="form-select" v-model="format">
                <option v-for="f in formats" :key="f.value" :value="f.value">{{ f.label }}</option>
              </select>
            </div>

            <div class="form-row">
              <div class="form-group form-group-half">
                <label class="form-label">{{ t('export.label_width') }}</label>
                <input class="form-control" type="number" v-model.number="width" min="1" />
              </div>
              <div class="form-group form-group-half">
                <label class="form-label">{{ t('export.label_height') }}</label>
                <input class="form-control" type="number" v-model.number="height" min="1" />
              </div>
            </div>

            <div class="form-group form-inline">
              <label class="form-label">
                <input type="checkbox" v-model="lockRatio" /> {{ t('export.lock_aspect') }}
              </label>
              <button class="btn btn-sm btn-secondary" @click="resetSize" v-if="imageInfo.width">{{ t('export.btn_reset') }}</button>
            </div>

            <div class="form-group" v-if="showQuality">
              <label class="form-label">{{ t('export.label_quality', { value: quality }) }}</label>
              <input type="range" min="1" max="100" v-model.number="quality" class="form-range" />
            </div>

            <div class="form-group">
              <label class="form-label">{{ t('export.label_interpolation') }}</label>
              <select class="form-select" v-model="interpolation">
                <option v-for="i in interpolations" :key="i.value" :value="i.value">{{ i.label }}</option>
              </select>
            </div>

            <template v-if="batchActive">
              <div class="pipeline-folder">
                <span class="pipeline-folder-label">{{ t('post.pipeline_folder') }}:</span>
                <span class="pipeline-folder-path" :title="batchFolder">{{ batchFolder || t('post.batch_folder_default') }}</span>
                <button class="btn btn-sm btn-secondary" @click="changeBatchFolder">{{ t('post.pipeline_change_folder') }}</button>
              </div>
              <div class="form-hint">{{ t('post.batch_save_hint') }}</div>
              <button class="btn btn-primary btn-block" :disabled="batchBusy || batchExporting" @click="doExport">
                {{ batchExporting ? t('export.exporting') : t('post.batch_save_all', { n: batch.items.length }) }}
              </button>
            </template>
            <template v-else>
            <div class="form-group">
              <label class="form-label">{{ t('export.label_filename') }}</label>
              <input class="form-control" type="text" v-model="filename" />
            </div>

            <button class="btn btn-primary btn-block" :disabled="exporting || !currentB64" @click="doExport">
              {{ exporting ? t('export.exporting') : t('post.btn_save_file') }}
            </button>
            </template>

            <div class="form-divider"></div>
            <div class="form-label">{{ t('export.saved_presets') }}</div>
            <div class="preset-list">
              <div v-if="!exportPresets.length" class="preset-empty">{{ t('export.no_presets') }}</div>
              <div
                v-for="p in exportPresets"
                :key="p.id"
                class="preset-item"
                :class="{ active: selectedPresetId === p.id }"
                @click="applyPreset(p)"
              >
                <div class="preset-info">
                  <span class="preset-name">{{ p.name }}</span>
                  <span class="preset-meta">{{ p.format.toUpperCase() }} {{ p.width || 'orig' }}x{{ p.height || 'orig' }} Q{{ p.quality }}</span>
                </div>
                <div class="preset-actions">
                  <button class="btn-icon" @click.stop="startEditPreset(p)" :title="t('presets.btn_edit')"><Pencil :size="14" /></button>
                  <button class="btn-icon btn-icon-danger" @click.stop="deletePreset(p.id)" :title="t('presets.btn_delete')"><X :size="14" /></button>
                </div>
              </div>
            </div>

            <div class="preset-form" v-if="showPresetForm">
              <div class="form-divider"></div>
              <div class="form-group">
                <input class="form-control" type="text" v-model="presetName" :placeholder="t('export.placeholder_preset_name')" />
              </div>
              <div class="preset-form-actions">
                <button class="btn btn-sm btn-primary" @click="savePreset" :disabled="!presetName">
                  {{ editingPreset ? t('export.btn_update') : t('export.btn_save') }}
                </button>
                <button class="btn btn-sm btn-secondary" @click="showPresetForm = false; editingPreset = null">{{ t('export.btn_cancel') }}</button>
              </div>
            </div>
            </div>
          </div>
        </div>

        <div ref="recipesSectionRef" class="card" :class="{ collapsed: openSection !== 'recipes' }">
          <div class="card-header card-toggle" @click="setOpenSection('recipes')">
            <span class="card-heading"><span class="card-num mono">09</span><span class="card-heading-text"><span class="card-title">{{ t('post.recipes_title') }}</span><span class="card-subtitle">{{ t('post.sub_recipes') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'recipes' }">
            <div class="card-body">
            <div class="adj-actions">
              <button class="btn btn-sm btn-secondary" :disabled="recipeRunning" @click="openRecipeForm">{{ t('post.recipe_save') }}</button>
            </div>
            <div class="preset-list">
              <div v-if="!recipes.length" class="preset-empty">{{ t('post.recipes_empty') }}</div>
              <div
                v-for="r in recipes"
                :key="r.id"
                class="preset-item"
                :class="{ active: recipeCurrentId === r.id }"
                @click="applyRecipe(r)"
              >
                <div class="preset-info">
                  <span class="preset-name">{{ r.name }}</span>
                  <span class="preset-meta">{{ recipeMeta(r) }}</span>
                </div>
                <div class="preset-actions">
                  <button
                    class="btn-icon btn-icon-danger"
                    :class="{ 'btn-confirm': pendingDeleteRecipeId === r.id }"
                    :title="pendingDeleteRecipeId === r.id ? t('post.recipe_delete_confirm') : t('post.recipe_delete')"
                    @click.stop="deleteRecipe(r.id)"
                  ><X :size="14" /></button>
                </div>
              </div>
            </div>

            <div class="pipeline-progress" v-if="recipeRunning">
              <span class="spinner" style="width: 12px; height: 12px; border-width: 2px;"></span>
              {{ t('post.recipe_progress', { i: recipeStepIdx, n: recipeStepTotal }) }}
            </div>
            <button v-if="recipeRunning" class="btn btn-secondary btn-block" @click="interruptRecipe">
              {{ t('progress.btn_interrupt') }}
            </button>

            <div class="preset-form" v-if="showRecipeForm && !recipeRunning">
              <div class="form-divider"></div>
              <div class="form-group">
                <input class="form-control" type="text" v-model="recipeName" :placeholder="t('post.recipe_name')" />
              </div>
              <div class="preset-form-actions">
                <button class="btn btn-sm btn-primary" @click="saveRecipe" :disabled="!recipeName">
                  {{ t('export.btn_save') }}
                </button>
                <button class="btn btn-sm btn-secondary" @click="showRecipeForm = false">{{ t('export.btn_cancel') }}</button>
              </div>
            </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.post-page {
  padding: 12px;
  height: 100%;
  overflow-y: auto;
}

.post-workspace {
  display: flex;
  gap: 10px;
  height: calc(100vh - 124px - var(--titlebar-h, 32px) - var(--frame-pad, 6px));
  min-height: 480px;
}

.post-preview-col {
  flex: 1;
  min-width: 0;
  display: flex;
}

.post-preview-card {
  flex: 1;
  padding: 0;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

.post-controls {
  width: 360px;
  flex-shrink: 0;
  align-self: flex-start;
  max-height: 100%;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  min-height: 0;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.post-controls > .card {
  flex-shrink: 0;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: 0;
  box-shadow: none;
}

.post-controls > .card + .card {
  border-top: 1px solid var(--border);
}

.post-controls .card-header {
  padding: 9px 12px;
  border-bottom: none;
}

.post-controls > .card.collapsed .card-header:hover {
  background: var(--surface-2);
}

.post-controls > .card:not(.collapsed) {
  border-color: var(--accent);
}

.post-controls > .card:not(.collapsed) .card-num {
  color: var(--accent);
}

.fx-mask {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  margin: 10px 0;
}

.fx-mask-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.fx-mask-row .form-range {
  flex: 1;
}

.fx-model-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}

.fx-lora-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.fx-lora-row .fx-lora-name {
  flex: 1;
  min-width: 0;
}

.fx-lora-row .fx-lora-weight {
  width: 72px;
}

.post-controls .card-collapse > .card-body {
  padding: 10px 12px 12px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 9px 12px;
  border-bottom: 1px solid var(--border);
}

.card.collapsed .card-header {
  border-bottom: none;
}

.card-heading {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}

.card-num {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-dim);
}

.card-heading-text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.card-subtitle {
  font-size: 12px;
  font-weight: normal;
  color: var(--text-dim);
}

.card-toggle {
  cursor: pointer;
  user-select: none;
}

.card-toggle .card-title {
  font-weight: 600;
}

.card-collapse {
  display: grid;
  grid-template-rows: 1fr;
  transition: grid-template-rows 0.2s ease;
}

.card-collapse.collapsed {
  grid-template-rows: 0fr;
}

.card-collapse > .card-body {
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  overflow: clip;
  transition: padding 0.2s ease;
}

.card-collapse.collapsed > .card-body {
  padding-top: 0;
  padding-bottom: 0;
}

.card-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-bright);
}

.card-actions {
  display: flex;
  gap: 6px;
}

.card-actions .btn {
  white-space: nowrap;
}

.post-preview-card .card-header {
  flex-wrap: wrap;
  gap: 8px;
}

.card-body {
  padding: 12px;
}

.drop-zone {
  flex: 1;
  border: 2px dashed var(--border);
  border-radius: var(--radius-sm);
  padding: 24px;
  text-align: center;
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
  min-height: 120px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.drop-zone:hover,
.drop-zone-active {
  border-color: var(--accent);
  background: var(--surface-2);
}

.split-body {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  padding: 10px;
}

.viewfinder {
  position: absolute;
  inset: 6px;
  pointer-events: none;
}

.viewfinder span {
  position: absolute;
  width: 18px;
  height: 18px;
  border: 0 solid var(--accent);
}

.viewfinder span:nth-child(1) { top: 0; left: 0; border-top-width: 2px; border-left-width: 2px; }
.viewfinder span:nth-child(2) { top: 0; right: 0; border-top-width: 2px; border-right-width: 2px; }
.viewfinder span:nth-child(3) { bottom: 0; left: 0; border-bottom-width: 2px; border-left-width: 2px; }
.viewfinder span:nth-child(4) { bottom: 0; right: 0; border-bottom-width: 2px; border-right-width: 2px; }

.split-area {
  position: relative;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: #000;
  touch-action: none;
  user-select: none;
}

.gen-overlay {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.55);
  pointer-events: none;
}

.gen-overlay-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.gen-overlay-spinner {
  width: 32px;
  height: 32px;
  border-width: 3px;
}

.gen-overlay-bar {
  position: absolute;
  left: 50%;
  bottom: 16px;
  transform: translateX(-50%);
  width: min(320px, calc(100% - 32px));
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  background: rgba(0, 0, 0, 0.65);
  color: #fff;
  font-size: 12px;
}

.gen-overlay-stats {
  display: flex;
  justify-content: space-between;
  margin-bottom: 6px;
}

.gen-overlay-track {
  height: 4px;
  border-radius: 2px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.2);
}

.gen-overlay-track > div {
  height: 100%;
  background: var(--accent);
  transition: width 0.3s;
}

.split-area.pannable {
  cursor: grab;
}

.split-area.panning {
  cursor: grabbing;
}

.split-img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  object-position: center;
  pointer-events: none;
  transform-origin: center;
}

.split-before-mask {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}

.split-mask {
  position: absolute;
  inset: 0;
}

.split-mask.mask-overlay-hidden {
  visibility: hidden;
}

.split-area.masking {
  cursor: none;
}

.brush-cursor {
  position: absolute;
  border: 1px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.5);
  border-radius: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
}

.split-before {
  position: absolute;
  inset: 0;
  height: 100%;
}

.split-after {
  position: relative;
}

.split-divider {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: #fff;
  box-shadow: 0 0 4px rgba(0, 0, 0, 0.6);
  transform: translateX(-1px);
  pointer-events: none;
}

.split-handle {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #fff;
  color: #000;
  border-radius: 50%;
  font-size: 14px;
  line-height: 1;
  pointer-events: auto;
  cursor: ew-resize;
  touch-action: none;
}

.split-label {
  position: absolute;
  top: 8px;
  padding: 2px 8px;
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.55);
  color: #fff;
  font-size: 11px;
  pointer-events: none;
}

.split-label-before {
  left: 8px;
}

.split-label-after {
  right: 8px;
}

.split-processing {
  position: absolute;
  bottom: 8px;
  left: 50%;
  transform: translateX(-50%);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.55);
  color: #fff;
  font-size: 11px;
  pointer-events: none;
}

.split-processing .spinner {
  width: 10px;
  height: 10px;
  border-width: 2px;
}

.split-zoom {
  position: absolute;
  bottom: 8px;
  right: 8px;
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: default;
}

.split-zoom-btn {
  background: rgba(0, 0, 0, 0.55);
  border: none;
  color: #fff;
  height: 24px;
  min-width: 24px;
  padding: 0 8px;
  border-radius: 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
}

.split-zoom-btn:hover {
  background: rgba(0, 0, 0, 0.75);
}

.split-zoom-btn:disabled {
  opacity: 0.4;
  cursor: default;
}

.split-zoom-label {
  min-width: 44px;
  font-variant-numeric: tabular-nums;
}

.split-zoom-menu {
  position: absolute;
  bottom: 30px;
  right: 0;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
}

.split-zoom-menu button {
  background: none;
  border: none;
  color: var(--text);
  padding: 6px 14px;
  font-size: 12px;
  cursor: pointer;
  text-align: left;
  min-width: 70px;
}

.split-zoom-menu button:hover {
  background: var(--surface-2);
}

.split-zoom-menu button.active {
  color: var(--accent);
  font-weight: 600;
}

.split-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 16px 12px;
  font-size: 12px;
  color: var(--text-dim);
}

.fx-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.fx-apply-bar {
  position: sticky;
  bottom: 0;
  z-index: 2;
  margin: 0 -16px;
  padding: 10px 16px 12px;
  background: var(--surface);
  border-top: 1px solid var(--border);
}

.fx-apply-bar .form-group {
  margin-bottom: 8px;
}

.fx-grid-none {
  margin-bottom: 12px;
}

.fx-group {
  margin-bottom: 12px;
}

.fx-group-pick {
  margin-bottom: 8px;
}

.fx-selected {
  font-size: 12px;
  color: var(--text-dim);
  margin-top: 6px;
}


.fx-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 1px;
  min-height: 46px;
  padding: 6px 10px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  cursor: pointer;
  transition: all 0.15s;
  text-align: left;
}

.fx-card:hover {
  border-color: var(--accent);
  color: var(--text-bright);
}

.fx-card.active {
  background: var(--accent-bg);
  border-color: var(--accent);
  color: var(--accent);
}

.fx-name {
  max-width: 100%;
  font-size: 12px;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fx-orig {
  max-width: 100%;
  font-size: 10px;
  line-height: 1.2;
  color: var(--text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.form-range {
  width: 100%;
  accent-color: var(--accent);
}

.form-hint {
  margin-top: 8px;
  font-size: 11px;
  color: var(--text-dim);
}

.effect-log-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 8px;
  font-size: 11px;
  color: var(--text-dim);
}

.adj-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 12px;
}

.adj-groups {
  display: grid;
  grid-template-columns: 1fr;
  gap: 12px;
}

.adj-group {
  min-width: 0;
}

.adj-group-title {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-dim);
  margin-bottom: 8px;
}

.adj-row {
  margin-bottom: 10px;
  cursor: default;
}

.adj-row-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 2px;
}

.adj-label {
  font-size: 12px;
  color: var(--text);
}

.adj-value-wrap {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.adj-reset {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--text-dim);
  cursor: pointer;
}

.adj-reset:hover {
  color: var(--accent);
  background: var(--surface-2);
}

.adj-value {
  font-size: 11px;
  color: var(--text-dim);
  font-variant-numeric: tabular-nums;
}

.form-row {
  display: flex;
  gap: 12px;
}

.form-group-half {
  flex: 1;
}

.form-inline {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.form-inline .form-label {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 0;
}

.form-divider {
  border-top: 1px solid var(--border);
  margin: 12px 0;
}

.form-control {
  width: 100%;
  padding: 6px 10px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  font-size: 13px;
}

.preset-row {
  display: flex;
  gap: 8px;
}

.preset-row .form-select {
  flex: 1;
}

.preset-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.preset-empty {
  color: var(--text-dim);
  font-size: 12px;
  text-align: center;
  padding: 16px;
}

.preset-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 0.15s;
}

.preset-item:hover {
  background: var(--surface-2);
}

.preset-item.active {
  background: var(--accent-bg);
}

.preset-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.preset-name {
  font-size: 13px;
  color: var(--text-bright);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.preset-meta {
  font-size: 11px;
  color: var(--text-dim);
}

.preset-actions {
  display: flex;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.15s;
  flex-shrink: 0;
}

.preset-item:hover .preset-actions {
  opacity: 1;
}

.btn-icon {
  background: none;
  border: none;
  color: var(--text-dim);
  cursor: pointer;
  padding: 2px 4px;
  font-size: 14px;
  border-radius: 3px;
  line-height: 1;
}

.btn-icon:hover {
  color: var(--text-bright);
  background: var(--surface-3);
}

.btn-icon-danger:hover {
  color: var(--danger);
}

.btn-icon.btn-confirm {
  color: var(--danger);
}

.preset-form-actions {
  display: flex;
  gap: 8px;
}


.pipeline-progress {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text);
  margin-bottom: 6px;
}


.pipeline-folder {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  font-size: 12px;
  color: var(--text-dim);
}

.pipeline-folder-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl;
  text-align: left;
  color: var(--text);
}


.batch-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 16px 16px;
}

.batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--text-dim);
}

.batch-title {
  color: var(--text);
  font-weight: 600;
}

.batch-exit {
  margin-left: auto;
}

.batch-strip {
  display: flex;
  gap: 10px;
  overflow-x: auto;
  padding: 8px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.batch-thumb {
  position: relative;
  flex: 0 0 auto;
  width: 64px;
  height: 64px;
  padding: 0;
  border: 2px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  overflow: hidden;
  cursor: pointer;
}

.batch-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.batch-thumb-remove {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 16px;
  height: 16px;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s;
}

.batch-thumb:hover .batch-thumb-remove,
.batch-thumb:focus-within .batch-thumb-remove {
  opacity: 1;
}

.batch-thumb-remove:disabled {
  cursor: default;
  opacity: 0.3;
}

.batch-thumb.active {
  border-color: var(--accent);
}

.batch-thumb.failed {
  border-color: var(--danger);
}

.batch-thumb-name {
  font-size: 9px;
  color: var(--text-dim);
  word-break: break-all;
}

.btn-block {
  width: 100%;
}
</style>
