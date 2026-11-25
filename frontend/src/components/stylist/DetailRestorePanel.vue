<script setup>
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { b64Url } from '../../imageDataUrl.js'
import { useMaskCanvas } from '../../composables/useMaskCanvas.js'
import PostViewer from '../post/PostViewer.vue'

const BRUSH_MIN = 4
const BRUSH_MAX = 160
const props = defineProps({
  frame: { type: Object, required: true },
  target: { type: String, default: '' },
  restore: { type: Function, required: true },
})
const emit = defineEmits(['queued'])
const stage = ref('base')
const source = ref('')
const aligned = ref(false)
const painting = ref(false)
const busy = ref(false)
const error = ref('')
const dimensionsMatch = ref(false)
const mask = useMaskCanvas()
const maskAdapter = reactive({ ...mask, maskActive: true, canUndoMask: mask.hasMask })
let epoch = 0

const frameReady = computed(() => props.frame?.status === 'completed' && !!props.frame?.jobId && !!props.target)
const canApply = computed(() => frameReady.value && !!source.value && dimensionsMatch.value && mask.hasMask.value && aligned.value && !busy.value)

function reset() {
  epoch++
  source.value = ''
  aligned.value = false
  painting.value = false
  busy.value = false
  error.value = ''
  dimensionsMatch.value = false
  mask.discardMask()
}

watch([() => props.frame?.jobId, () => props.frame?.itemId, () => props.frame?.status, () => props.target, stage], reset, { flush: 'sync' })
onUnmounted(() => { epoch++ })

function imageSize(value) {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => {
      if (!image.naturalWidth || !image.naturalHeight) reject(new Error('image'))
      else resolve({ width: image.naturalWidth, height: image.naturalHeight })
    }
    image.onerror = () => reject(new Error('image'))
    image.src = b64Url(value)
  })
}

async function loadSource() {
  if (!frameReady.value || busy.value) return
  reset()
  const requestEpoch = epoch
  busy.value = true
  try {
    const original = await api.stylistDetailSource(props.frame.jobId, stage.value)
    if (requestEpoch !== epoch) return
    if (!original) {
      error.value = t('stylist.detail_cache_missing')
      return
    }
    const [before, after] = await Promise.all([imageSize(original), imageSize(props.target)])
    if (requestEpoch !== epoch) return
    if (before.width !== after.width || before.height !== after.height) {
      error.value = t('stylist.detail_dimension_mismatch')
      return
    }
    source.value = original
    dimensionsMatch.value = true
    mask.applySource(props.target, false)
  } catch {
    if (requestEpoch === epoch) error.value = t('stylist.detail_cache_missing')
  } finally {
    if (requestEpoch === epoch) busy.value = false
  }
}

async function apply() {
  if (!canApply.value) return
  const requestEpoch = epoch
  busy.value = true
  error.value = ''
  painting.value = false
  try {
    const id = await props.restore({ job_id: props.frame.jobId, stage: stage.value, mask_base64: mask.getMaskBase64(), aligned: true })
    if (requestEpoch !== epoch) return
    aligned.value = false
    mask.clearMask()
    emit('queued', id)
  } catch {
    if (requestEpoch === epoch) error.value = t('stylist.detail_restore_error')
  } finally {
    if (requestEpoch === epoch) busy.value = false
  }
}
</script>

<template>
  <section class="detail-restore" data-testid="detail-restore-panel">
    <strong>{{ t('stylist.detail_title') }}</strong>
    <p class="form-hint">{{ t('stylist.detail_warning') }}</p>
    <div class="detail-controls">
      <label>{{ t('stylist.detail_source_stage') }}
        <select v-model="stage" class="form-select" data-testid="detail-source-stage" :disabled="busy">
          <option value="base">{{ t('stylist.detail_source_base') }}</option>
          <option value="before_face">{{ t('stylist.detail_source_before_face') }}</option>
        </select>
      </label>
      <button type="button" class="btn btn-secondary" data-testid="detail-load-source" :disabled="!frameReady || busy" @click="loadSource">{{ t('stylist.detail_load_source') }}</button>
    </div>
    <p class="form-hint">{{ t('stylist.detail_retention') }}</p>
    <template v-if="source">
      <div class="detail-viewer" :class="{ 'detail-busy': busy }">
        <PostViewer :source="source" :after="target" :mask-comp="painting && !busy ? maskAdapter : null" />
      </div>
      <div class="detail-controls">
        <button type="button" class="btn btn-secondary" data-testid="detail-paint-toggle" :aria-pressed="painting" :disabled="busy" @click="painting = !painting">{{ t(painting ? 'stylist.detail_compare' : 'stylist.detail_paint') }}</button>
        <label>{{ t('stylist.detail_brush') }} <input v-model.number="maskAdapter.brushSize" type="range" :min="BRUSH_MIN" :max="BRUSH_MAX" :disabled="busy" /></label>
        <button type="button" class="btn btn-secondary" data-testid="detail-clear-mask" :disabled="busy || !maskAdapter.hasMask" @click="mask.clearMask()">{{ t('stylist.detail_clear') }}</button>
      </div>
    </template>
    <label><input v-model="aligned" type="checkbox" data-testid="detail-aligned" :disabled="busy || !source" /> {{ t('stylist.detail_aligned') }}</label>
    <button type="button" class="btn btn-primary" data-testid="detail-apply" :disabled="!canApply" @click="apply">{{ t('stylist.detail_apply') }}</button>
    <p v-if="error" class="text-danger" role="alert" data-testid="detail-error">{{ error }}</p>
  </section>
</template>

<style scoped>
.detail-restore { display: flex; flex-direction: column; gap: 10px; }
.detail-controls { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.detail-viewer { display: flex; height: 420px; min-height: 240px; }
.detail-busy { pointer-events: none; }
</style>
