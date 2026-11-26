<script setup>
import { usePostEditor } from '../composables/usePostEditor.js'
const editor = usePostEditor()
import StatusMessage from './StatusMessage.vue'
import GenProgress from './GenProgress.vue'
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

const op = ref('remove')
const replacement = ref('')
const objectText = ref('')
const seedInput = ref('')
const running = ref(false)
watch(running, v => emit('busy', v))
const stage = ref('')
const error = ref('')
const detectedObject = ref('')
const effectivePrompt = ref('')
const moreOpen = ref(false)
const { llmStatus, sdProgress, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress({ acceptJob: () => false })
let reqCounter = 0
let lastEmitted = null

const maskActive = computed(() => !!props.source)

const canRun = computed(() => {
  if (!editor.ready.value || running.value || !props.source || !hasMask.value) return false
  if (op.value === 'replace' && !replacement.value.trim()) return false
  return true
})

const progressLabel = computed(() => {
  if (stage.value === 'analyzing' || llmStatus.value === 'thinking') return t('object.stage_analyzing')
  return t('object.stage_generating')
})

watch([error, objectText], ([e, o]) => {
  if (e || o) moreOpen.value = true
})

watch(() => props.source, (val) => {
  if (val !== lastEmitted) {
    objectText.value = ''
    detectedObject.value = ''
    effectivePrompt.value = ''
    error.value = ''
    mask.applySource(val, false)
  } else {
    mask.applySource(val, true)
  }
}, { immediate: true })

async function run() {
  if (!canRun.value) {
    if (!hasMask.value) error.value = t('object.error_no_mask')
    else if (op.value === 'replace' && !replacement.value.trim()) error.value = t('object.error_no_replacement')
    return
  }
  running.value = true
  stage.value = ''
  error.value = ''
  resetProgress()
  const reqId = ++reqCounter
  const runSource = props.source
  try {
    const result = await api.editObject({
      editor: editor.snapshot(),
      image_base64: props.source,
      mask_base64: mask.getMaskBase64(),
      op: op.value,
      replacement: op.value === 'replace' ? replacement.value.trim() : '',
      object: objectText.value.trim(),
      seed: parseSeedInput(seedInput.value),
    })
    if (reqId !== reqCounter || props.source !== runSource) return
    if (result?.image) {
      detectedObject.value = result.detected_object || ''
      objectText.value = ''
      effectivePrompt.value = result.effective_prompt || ''
      lastEmitted = result.image
      emit('result', result.image)
    }
  } catch (e) {
    if (reqId === reqCounter && props.source === runSource) error.value = String(e)
  } finally {
    if (reqId === reqCounter) {
      running.value = false
      stage.value = ''
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

let offStage = () => {}
let settingsRestored = false

onMounted(async () => {
  document.addEventListener('keydown', onKeydown)
  offStage = EventsOn('object:stage', (s) => { stage.value = s })
  try {
    const s = await api.getSettings()
    if (s?.object_op === 'replace' || s?.object_op === 'remove') op.value = s.object_op
    if (s?.object_brush) brushSize.value = Math.max(5, Math.min(150, Number(s.object_brush) || 40))
    settingsRestored = true
  } catch (e) {
    console.warn('ObjectSection.onMounted:', e)
  }
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  offStage()
  if (settingsRestored) {
    api.updateSettings({ object_op: op.value, object_brush: String(brushSize.value) }).catch(e => console.warn('ObjectSection.onUnmounted:', e))
  }
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

    <div class="object-op-row">
      <button data-testid="object-op-remove" class="btn btn-sm" :class="op === 'remove' ? 'btn-primary' : 'btn-secondary'" @click="op = 'remove'">{{ t('object.op_remove') }}</button>
      <button data-testid="object-op-replace" class="btn btn-sm" :class="op === 'replace' ? 'btn-primary' : 'btn-secondary'" @click="op = 'replace'">{{ t('object.op_replace') }}</button>
    </div>

    <div class="object-hint">{{ t('object.hint') }}</div>

    <div class="object-mask-controls">
      <div class="object-brush-head">
        <label class="form-label">{{ t('fi.label_brush_size', { value: brushSize }) }}</label>
        <span v-if="hasMask" class="object-mask-ready">{{ t('object.mask_ready') }}</span>
      </div>
      <div class="object-brush-row">
        <input type="range" class="form-range" v-model.number="brushSize" min="5" max="150" step="1" />
        <button class="btn btn-sm btn-secondary" @click="mask.clearMask" :disabled="!hasMask">{{ t('fi.btn_clear_mask') }}</button>
      </div>
    </div>

    <div v-if="op === 'replace'" class="form-group mt-3">
      <label class="form-label">{{ t('object.label_replacement') }}</label>
      <textarea data-testid="object-replacement" class="form-textarea" v-model="replacement" rows="2" maxlength="500" :placeholder="t('object.placeholder_replacement')"></textarea>
    </div>

    <button data-testid="object-run" class="btn btn-primary object-run" :class="{ 'btn-generating': running }" @click="run" :disabled="!canRun">
      <span v-if="running" class="inline-center">
        <span class="spinner spinner-sm"></span>
        {{ progressLabel }}
      </span>
      <span v-else>{{ op === 'replace' ? t('object.btn_replace') : t('object.btn_remove') }}</span>
    </button>

    <GenProgress v-if="running && sdProgress && sdProgress.progress > 0" class="object-progress" :progress="sdProgress.progress" :preview-available="sdProgress.preview_available" block @interrupt="interruptGeneration" />

    <div v-if="detectedObject && !running" class="object-detected">
      {{ t('object.detected', { object: detectedObject }) }}
    </div>

    <details class="object-more" :open="moreOpen" @toggle="moreOpen = $event.target.open">
      <summary>{{ t('object.more') }}</summary>
      <div class="form-group object-gap">
        <label class="form-label">{{ t('object.label_object') }}</label>
        <input data-testid="object-object" class="form-input" v-model="objectText" maxlength="200" :placeholder="t('object.placeholder_object')" />
        <div class="field-note">{{ t('object.object_hint') }}</div>
      </div>
      <div class="form-group">
        <label class="form-label">{{ t('fi.seed_label') }}</label>
        <input class="form-input" type="number" v-model="seedInput" min="0" step="1" :placeholder="t('fi.seed_placeholder')" />
      </div>
    </details>

    <details v-if="effectivePrompt" class="gen-info">
      <summary>{{ t('fi.technical_details') }}</summary>
      <pre class="pre-wrap">{{ effectivePrompt }}</pre>
    </details>
  </div>
</template>

<style scoped>
.object-detected {
  margin-top: 10px;
  font-size: var(--fs-sm);
  color: var(--text-dim);
}

.object-gap {
  margin-top: 10px;
}

.object-mask-controls {
  padding: 12px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.object-op-row {
  display: flex;
  gap: 8px;
  margin: 12px 0;
}
.object-hint {
  font-size: var(--fs-xs);
  color: var(--text-dim);
  margin-bottom: 10px;
}
.object-brush-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
}
.object-mask-ready {
  font-size: var(--fs-xs);
  color: var(--accent);
}
.object-brush-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.object-brush-row .btn {
  flex-shrink: 0;
  white-space: nowrap;
}
.object-run {
  width: 100%;
  justify-content: center;
  padding: 12px;
  margin-top: 12px;
}
.object-more {
  margin-top: 12px;
  font-size: var(--fs-sm);
}
.object-more summary {
  cursor: pointer;
  user-select: none;
  color: var(--text-dim);
}
.object-progress {
  margin-top: 10px;
  text-align: center;
}
</style>
