<script setup>
import StatusMessage from './StatusMessage.vue'
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { parseSeedInput } from '../seed.js'
import { t } from '../i18n/index.js'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { useMaskCanvas } from '../composables/useMaskCanvas.js'

const props = defineProps({
  source: { type: String, default: '' },
  active: { type: Boolean, default: false },
})

const emit = defineEmits(['result', 'busy'])

const mask = useMaskCanvas()
const { brushSize, maskCanvas, maskVersion, hasMask, canRedoMask } = mask

const strength = ref(0.8)
const seedInput = ref('')
const running = ref(false)
watch(running, v => emit('busy', v))
const error = ref('')
const moreOpen = ref(false)
const tileDone = ref(0)
const tileTotal = ref(0)
const { sdProgress, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress()
let reqCounter = 0
let lastEmitted = null

const maskActive = computed(() => !!props.source)
const canRun = computed(() => !running.value && !!props.source && hasMask.value)
const tileCurrent = computed(() => Math.min(tileDone.value + 1, tileTotal.value))
const overallProgress = computed(() => {
  if (!tileTotal.value) return 0
  const inTile = sdProgress.value?.progress > 0 && sdProgress.value.progress < 1 ? sdProgress.value.progress : 0
  return Math.min(1, (tileDone.value + inTile) / tileTotal.value)
})

watch(error, (e) => {
  if (e) moreOpen.value = true
})

watch(() => props.source, (val) => {
  if (val !== lastEmitted) {
    error.value = ''
    mask.applySource(val, false)
  } else {
    mask.applySource(val, true)
  }
}, { immediate: true })

async function run() {
  if (!canRun.value) {
    if (!hasMask.value) error.value = t('repair.error_no_mask')
    return
  }
  running.value = true
  error.value = ''
  tileDone.value = 0
  tileTotal.value = 0
  resetProgress()
  const reqId = ++reqCounter
  const runSource = props.source
  try {
    const result = await api.repairImage({
      image_base64: props.source,
      mask_base64: mask.getMaskBase64(),
      strength: strength.value,
      seed: parseSeedInput(seedInput.value),
    })
    if (reqId !== reqCounter || props.source !== runSource) return
    if (result?.image) {
      lastEmitted = result.image
      emit('result', result.image)
    }
  } catch (e) {
    if (reqId === reqCounter && props.source === runSource) error.value = String(e)
  } finally {
    if (reqId === reqCounter) {
      running.value = false
      tileDone.value = 0
      tileTotal.value = 0
    }
  }
}

function onKeydown(e) {
  if (!props.active) return
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !running.value) {
    e.preventDefault()
    run()
  }
}

let offProgress = () => {}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  offProgress = EventsOn('repair:progress', (p) => {
    if (!running.value || !p) return
    tileDone.value = Number(p.done) || 0
    tileTotal.value = Number(p.total) || 0
  })
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  offProgress()
})

defineExpose({
  maskActive,
  maskCanvas,
  maskVersion,
  brushSize,
  beginStroke: mask.beginStroke,
  strokeTo: mask.strokeTo,
  endStroke: mask.endStroke,
  undoMask: mask.undoMask,
  redoMask: mask.redoMask,
  canUndoMask: hasMask,
  canRedoMask,
})
</script>

<template>
  <div>
    <StatusMessage v-if="error" :text="error" @close="error = ''" />

    <div class="repair-hint">{{ t('repair.hint') }}</div>

    <div class="repair-mask-controls">
      <div class="repair-brush-head">
        <label class="form-label">{{ t('fi.label_brush_size', { value: brushSize }) }}</label>
        <span v-if="hasMask" class="repair-mask-ready">{{ t('repair.mask_ready') }}</span>
      </div>
      <div class="repair-brush-row">
        <input type="range" class="form-range" v-model.number="brushSize" min="5" max="150" step="1" />
        <button data-testid="repair-clear" class="btn btn-sm btn-secondary" @click="mask.clearMask" :disabled="!hasMask">{{ t('fi.btn_clear_mask') }}</button>
      </div>
    </div>

    <button data-testid="repair-run" class="btn btn-primary repair-run" :class="{ 'btn-generating': running }" @click="run" :disabled="!canRun">
      <span v-if="running" style="display: inline-flex; align-items: center; gap: 6px;">
        <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
        {{ t('repair.stage_running') }}
      </span>
      <span v-else>{{ t('repair.btn_run') }}</span>
    </button>

    <div v-if="running && tileTotal > 0" data-testid="repair-progress" class="repair-progress">
      <div class="repair-progress-label">{{ t('repair.progress', { n: tileCurrent, m: tileTotal }) }}</div>
      <div class="repair-progress-bar"><div :style="{ width: (overallProgress * 100) + '%' }"></div></div>
      <button class="btn btn-sm btn-secondary" style="width: 100%; margin-top: 8px; font-size: 11px;" @click="interruptGeneration">{{ t('progress.btn_interrupt') }}</button>
    </div>


    <details class="repair-more" :open="moreOpen" @toggle="moreOpen = $event.target.open">
      <summary>{{ t('repair.more') }}</summary>
      <div class="form-group" style="margin-top: 10px;">
        <label class="form-label">{{ t('repair.label_strength', { value: strength.toFixed(2) }) }}</label>
        <input data-testid="repair-strength" type="range" class="form-range" v-model.number="strength" min="0.3" max="0.9" step="0.05" />
        <div style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">{{ t('repair.strength_hint') }}</div>
      </div>
      <div class="form-group">
        <label class="form-label">{{ t('fi.seed_label') }}</label>
        <input class="form-input" type="number" v-model="seedInput" min="0" step="1" :placeholder="t('fi.seed_placeholder')" />
      </div>
    </details>
  </div>
</template>

<style scoped>
.form-range {
  width: 100%;
  accent-color: var(--accent);
}
.repair-hint {
  font-size: 11px;
  color: var(--text-dim);
  margin: 12px 0 10px;
}
.repair-mask-controls {
  padding: 12px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.repair-brush-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
}
.repair-mask-ready {
  font-size: 11px;
  color: var(--accent);
}
.repair-brush-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.repair-brush-row .btn {
  flex-shrink: 0;
  white-space: nowrap;
}
.repair-run {
  width: 100%;
  justify-content: center;
  padding: 12px;
  margin-top: 12px;
}
.repair-progress {
  margin-top: 10px;
  text-align: center;
}
.repair-progress-label {
  font-size: 12px;
  color: var(--text-dim);
}
.repair-progress-bar {
  background: var(--surface-2);
  border-radius: 4px;
  overflow: hidden;
  height: 6px;
  margin-top: 6px;
}
.repair-progress-bar div {
  background: var(--accent);
  height: 100%;
  transition: width 0.3s;
}
.repair-more {
  margin-top: 12px;
  font-size: 12px;
}
.repair-more summary {
  cursor: pointer;
  user-select: none;
  color: var(--text-dim);
}
</style>
