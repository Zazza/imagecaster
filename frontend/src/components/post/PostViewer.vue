<script setup>
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { ArrowLeftRight, Eye, EyeOff, Image as ImageIcon, Redo2, Undo2 } from 'lucide-vue-next'
import { t } from '../../i18n/index.js'
import { b64Url } from '../../imageDataUrl.js'
import { useZoomPan } from '../../composables/useZoomPan.js'
import ZoomControls from '../ZoomControls.vue'

const props = defineProps({
  source: { type: String, default: '' },
  after: { type: String, default: '' },
  previewing: { type: Boolean, default: false },
  finishActive: { type: Boolean, default: false },
  maskComp: { type: Object, default: null },
  genBusy: { type: Boolean, default: false },
  genActive: { type: Boolean, default: false },
  genProgress: { type: Object, default: null },
  livePreview: { type: String, default: '' },
  canUndo: { type: Boolean, default: false },
  canRedo: { type: Boolean, default: false },
})

const emit = defineEmits(['undo', 'redo', 'upload', 'drop'])

const splitPos = ref(50)
const splitDragging = ref(false)
const splitWrapRef = ref(null)
const isDragOver = ref(false)

const zoom = useZoomPan(splitWrapRef, { observe: false })
const { naturalW, naturalH, zoomPct, panX, panY, isFit, panEnabled, panning, layerStyle } = zoom

const maskViewRef = ref(null)
const brushing = ref(false)
const brushCursor = ref({ x: 0, y: 0, visible: false })
const maskMode = computed(() => !!props.maskComp?.maskActive)
const pointSelection = computed(() => maskMode.value && !!props.maskComp?.pointSelection)
const brushDiameter = computed(() => props.maskComp?.brushSize || 30)
const maskOverlayVisible = ref(true)

function maskScale() {
  const canvas = props.maskComp?.maskCanvas
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
  const canvas = props.maskComp?.maskCanvas
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
  const src = props.maskComp?.maskCanvas
  const view = maskViewRef.value
  if (!view) return
  if (!src) {
    view.getContext('2d')?.clearRect(0, 0, view.width, view.height)
    return
  }
  if (view.width !== src.width || view.height !== src.height) {
    view.width = src.width
    view.height = src.height
  }
  const ctx = view.getContext('2d')
  ctx.clearRect(0, 0, view.width, view.height)
  ctx.drawImage(src, 0, 0)
}

watch([() => props.maskComp, () => props.maskComp?.maskVersion], () => {
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

function toggleFit100() {
  if (maskMode.value) return
  zoom.toggleFit100()
}

function onViewPointerDown(e) {
  zoom.zoomMenuOpen.value = false
  if (e.button !== 0) return
  if (maskMode.value) {
    const p = pointerToImage(e)
    if (!p) return
    if (pointSelection.value) {
      void props.maskComp?.selectAt(p.x, p.y, e.shiftKey)
      return
    }
    brushing.value = true
    props.maskComp?.beginStroke(p.x, p.y, brushRadius())
    e.currentTarget.setPointerCapture(e.pointerId)
    return
  }
  zoom.onPointerDown(e)
}

function onViewPointerMove(e) {
  if (maskMode.value) {
    if (pointSelection.value) return
    updateBrushCursor(e)
    if (brushing.value) {
      const p = pointerToImage(e)
      if (p) props.maskComp?.strokeTo(p.x, p.y, brushRadius())
    }
    return
  }
  zoom.onPointerMove(e)
}

function onViewPointerUp() {
  if (brushing.value) {
    brushing.value = false
    props.maskComp?.endStroke()
    return
  }
  zoom.onPointerUp()
}

function onViewPointerLeave() {
  brushCursor.value.visible = false
}

watch(() => props.source, () => zoom.resetImage())

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

function onDrop(e) {
  isDragOver.value = false
  emit('drop', e)
}

onMounted(() => {
  window.addEventListener('resize', zoom.measure)
  zoom.measure()
})

onUnmounted(() => {
  window.removeEventListener('resize', zoom.measure)
})
</script>

<template>
  <div class="split-body">
    <div v-if="source" class="viewfinder" aria-hidden="true"><span></span><span></span><span></span><span></span></div>
    <div
      v-if="source"
      ref="splitWrapRef"
      class="split-area"
      :class="{ pannable: panEnabled && !maskMode, panning, masking: maskMode, 'point-selecting': pointSelection }"
      @pointerdown="onViewPointerDown"
      @pointermove="onViewPointerMove"
      @pointerup="onViewPointerUp"
      @pointercancel="onViewPointerUp"
      @pointerleave="onViewPointerLeave"
      @wheel="zoom.onWheel"
      @dblclick="toggleFit100"
    >
      <img
        class="split-img split-after"
        :src="b64Url(after)"
        :style="layerStyle"
        alt="After"
        draggable="false"
        @load="zoom.onImgLoad"
      />
      <div v-if="!maskMode && !finishActive" class="split-before-mask" :style="{ clipPath: `inset(0 ${100 - splitPos}% 0 0)` }">
        <img
          class="split-img split-before"
          :src="b64Url(source)"
          :style="layerStyle"
          alt="Before"
          draggable="false"
        />
      </div>
      <canvas v-if="maskMode" ref="maskViewRef" class="split-img split-mask" :class="{ 'mask-overlay-hidden': !maskOverlayVisible }" :style="layerStyle"></canvas>
      <div
        v-if="maskMode && !pointSelection && brushCursor.visible"
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
      <ZoomControls :zoom="zoom" @pointerdown.stop @click.stop @dblclick.stop @wheel.stop>
        <button
          v-if="maskMode"
          class="split-zoom-btn"
          data-testid="mask-overlay-toggle"
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
          :disabled="!canUndo"
          @click="emit('undo')"
        ><Undo2 :size="14" /></button>
        <button
          class="split-zoom-btn"
          :title="t('post.redo')"
          :aria-label="t('post.redo')"
          :disabled="!canRedo"
          @click="emit('redo')"
        ><Redo2 :size="14" /></button>
      </ZoomControls>
      <div v-if="maskMode" class="mask-history" @pointerdown.stop>
        <button class="btn btn-sm btn-secondary" data-testid="mask-undo" :title="t(pointSelection ? 'wand_mask.undo' : 'post.mask_undo')" :disabled="!maskComp?.canUndoMask" @click="maskComp?.undoMask()">{{ t(pointSelection ? 'wand_mask.undo' : 'post.mask_undo') }}</button>
        <button class="btn btn-sm btn-secondary" data-testid="mask-redo" :title="t(pointSelection ? 'wand_mask.redo' : 'post.mask_redo')" :disabled="!maskComp?.canRedoMask" @click="maskComp?.redoMask()">{{ t(pointSelection ? 'wand_mask.redo' : 'post.mask_redo') }}</button>
      </div>
    </div>
    <div
      v-else
      class="drop-zone"
      :class="{ 'drop-zone-active': isDragOver }"
      @dragover.prevent="isDragOver = true"
      @dragleave="isDragOver = false"
      @drop.prevent="onDrop"
      @click="emit('upload')"
    >
      <div class="flex text-dim"><ImageIcon :size="32" :stroke-width="1.5" /></div>
      <p class="text-dim mt-2">{{ t('fi.drop_image') }}</p>
    </div>
  </div>
</template>

<style scoped>
.mask-history {
  position: absolute;
  top: 12px;
  left: 12px;
  display: flex;
  gap: 6px;
  z-index: 3;
}

.drop-zone {
  flex: 1;
  min-height: 120px;
}

.split-body {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  padding: 10px;
}

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
  font-size: var(--fs-sm);
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

.split-area.point-selecting {
  cursor: crosshair;
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
  font-size: var(--fs-base);
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
  font-size: var(--fs-xs);
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
  font-size: var(--fs-xs);
  pointer-events: none;
}

.split-processing .spinner {
  width: 10px;
  height: 10px;
  border-width: 2px;
}

</style>
