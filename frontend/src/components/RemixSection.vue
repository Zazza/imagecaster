<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { parseSeedInput } from '../seed.js'
import { parseLoras } from '../loraOverride.js'
import { t } from '../i18n/index.js'
import { useRemixQueue } from '../remixQueue.js'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { useKidsMode } from '../composables/useKidsMode.js'
import { usePresets } from '../composables/usePresets.js'
import PresetOverridesPanel from './PresetOverridesPanel.vue'
import ResolutionSelector from './ResolutionSelector.vue'
import HiresProfileSelector from './HiresProfileSelector.vue'

const props = defineProps({
  source: { type: String, default: '' },
  active: { type: Boolean, default: false },
})

const emit = defineEmits(['result', 'source'])

const { presets, presetTypes, compoundPresets, loadPresets } = usePresets()
const selectedTypeId = ref(null)
const selectedPresetId = ref(null)
const selectedCompoundPresetId = ref(null)
const genMode = ref('preset')
const mode = ref('img2img')
const denoisingStrength = ref(0.5)
const seedInput = ref('')
const extraNegativePrompt = ref('')
const tags = ref('')
const selectedResolutionId = ref(null)
const selectedHiresProfileId = ref(null)
const hiresProfiles = ref([])
const loadHiresFailed = ref(false)
const qualityMode = ref(false)
const preserveIdentity = ref(true)
const preserveIdentityResetFailed = ref(false)

const genInfo = ref(null)
const qualityUpscaleSkipped = ref(false)
const qualityRedenoiseSkipped = ref(false)

const analyzing = ref(false)
const analyzeMode = ref('quick')
const analyzeElapsed = ref(0)
let analyzeTimer = null

const generatingImage = ref(false)
const { llmStatus, sdProgress, preview, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress()
const generationStage = ref('')
const error = ref('')
const enqueuedJobIds = ref(new Set())
const genCount = ref(1)
const batchTotal = ref(0)
const batchDone = ref(0)

const { kidsModeActive, loadKidsMode } = useKidsMode()

const presetOverrides = ref(null)
const overridesPanel = ref(null)

const removeStage = ref('')
const brushSize = ref(30)
const maskBlur = ref(4)
const maskPadding = ref(8)
const maskFeather = ref(8)
const inpaintFill = ref(1)
const inpaintFullRes = ref(true)
const invertMask = ref(false)
const maskCanvas = ref(null)
const maskHistory = ref([])
const maskRedo = ref([])
const maskVersion = ref(0)
let maskStrokeActive = false
let maskSourceReqId = 0
let lastEmittedSource = null

const { queue: remixQueue, clearQueue: clearRemixQueue } = useRemixQueue()
const folderQueue = ref([])
let folderCancelled = false

const filteredPresets = computed(() => {
  if (!selectedTypeId.value) return presets.value
  return presets.value.filter(p => p.type_id === selectedTypeId.value)
})

const selectedPresetLoras = computed(() => {
  const p = presets.value.find(p => p.id === selectedPresetId.value)
  return p ? parseLoras(p.loras) : []
})

const kleinPresetSelected = computed(() => {
  const p = presets.value.find(p => p.id === selectedPresetId.value)
  return !!p?.model_name?.toLowerCase().includes('klein')
})

const hasMask = computed(() => maskHistory.value.length > 0)
const canRedoMask = computed(() => maskRedo.value.length > 0)

const maskActive = computed(() => mode.value === 'inpaint' || mode.value === 'remove')

const progressLabel = computed(() => {
  if (llmStatus.value === 'thinking') return t('progress.llm_thinking')
  if (removeStage.value === 'analyzing') return t('fi.analyzing_context')
  if (generationStage.value === 'analyzing') return t('fi.analyzing_image')
  return t('generate.generating_image')
})

function b64Url(b64) {
  if (!b64) return ''
  if (b64.startsWith('data:')) return b64
  if (b64.startsWith('/9j/')) return 'data:image/jpeg;base64,' + b64
  return 'data:image/png;base64,' + b64
}

async function analyzeImage() {
  if (!props.source) return
  analyzing.value = true
  error.value = ''
  analyzeElapsed.value = 0
  analyzeTimer = setInterval(() => { analyzeElapsed.value++ }, 1000)

  try {
    const result = await api.analyzeImage(props.source, analyzeMode.value)
    tags.value = result || ''
  } catch (e) {
    error.value = t('fi.error_analysis', { error: String(e) })
  } finally {
    analyzing.value = false
    if (analyzeTimer) {
      clearInterval(analyzeTimer)
      analyzeTimer = null
    }
  }
}

function applySourceToMask(keepIfSameSize) {
  if (mode.value !== 'inpaint' && mode.value !== 'remove') {
    discardMask()
    return
  }
  if (!props.source) {
    discardMask()
    return
  }
  const reqId = ++maskSourceReqId
  const img = new Image()
  img.onload = () => {
    if (reqId !== maskSourceReqId) return
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
  img.src = b64Url(props.source)
}

function discardMask() {
  maskSourceReqId++
  maskCanvas.value = null
  maskHistory.value = []
  maskRedo.value = []
  maskVersion.value++
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
  if (maskHistory.value.length > 30) {
    maskHistory.value.shift()
  }
  maskRedo.value = []
  maskStrokeActive = true
  paintCircle(x, y, radius)
  maskVersion.value++
}

function strokeTo(x, y, radius) {
  if (!maskStrokeActive) return
  paintCircle(x, y, radius)
  maskVersion.value++
}

function endStroke() {
  maskStrokeActive = false
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
  const prev = maskHistory.value.pop()
  ctx.putImageData(prev, 0, 0)
  maskVersion.value++
}

function redoMask() {
  if (maskRedo.value.length === 0) return
  const canvas = maskCanvas.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  maskHistory.value.push(ctx.getImageData(0, 0, canvas.width, canvas.height))
  const next = maskRedo.value.pop()
  ctx.putImageData(next, 0, 0)
  maskVersion.value++
}

function getMaskBase64() {
  const canvas = maskCanvas.value
  if (!canvas) return ''
  const w = canvas.width
  const h = canvas.height
  const ctx = canvas.getContext('2d')
  const src = ctx.getImageData(0, 0, w, h)

  const outCanvas = document.createElement('canvas')
  outCanvas.width = w
  outCanvas.height = h
  const outCtx = outCanvas.getContext('2d')
  const maskData = outCtx.createImageData(w, h)

  for (let i = 0; i < src.data.length; i += 4) {
    const alpha = src.data[i + 3]
    const isMasked = alpha > 10
    const fill = invertMask.value ? !isMasked : isMasked
    if (fill) {
      maskData.data[i] = 255
      maskData.data[i + 1] = 255
      maskData.data[i + 2] = 255
      maskData.data[i + 3] = 255
    }
  }
  outCtx.putImageData(maskData, 0, 0)

  if (maskPadding.value > 0) {
    const dilated = document.createElement('canvas')
    dilated.width = w
    dilated.height = h
    const dCtx = dilated.getContext('2d')
    dCtx.filter = `blur(${maskPadding.value}px)`
    dCtx.drawImage(outCanvas, 0, 0)
    dCtx.filter = 'none'

    const dData = dCtx.getImageData(0, 0, w, h)
    for (let i = 0; i < dData.data.length; i += 4) {
      if (dData.data[i + 3] > 0) {
        dData.data[i] = 255
        dData.data[i + 1] = 255
        dData.data[i + 2] = 255
        dData.data[i + 3] = 255
      }
    }
    dCtx.putImageData(dData, 0, 0)

    outCtx.clearRect(0, 0, w, h)
    outCtx.drawImage(dilated, 0, 0)
  }

  if (maskFeather.value > 0) {
    const feathered = document.createElement('canvas')
    feathered.width = w
    feathered.height = h
    const fCtx = feathered.getContext('2d')
    fCtx.filter = `blur(${maskFeather.value}px)`
    fCtx.drawImage(outCanvas, 0, 0)

    outCtx.clearRect(0, 0, w, h)
    outCtx.drawImage(feathered, 0, 0)
  }

  const dataUrl = outCanvas.toDataURL('image/png')
  return dataUrl.split(',')[1] || ''
}

watch(mode, (newMode) => {
  if (newMode !== 'img2img') folderQueue.value = []
  if (newMode === 'inpaint' || newMode === 'remove') {
    applySourceToMask(false)
  } else {
    discardMask()
  }
  if (newMode === 'remove') {
    denoisingStrength.value = 0.6
    maskBlur.value = 8
    inpaintFill.value = 1
    inpaintFullRes.value = true
  }
})

watch(() => props.source, (val) => {
  if (val !== lastEmittedSource) {
    folderQueue.value = []
    tags.value = ''
    error.value = ''
  }
  applySourceToMask(true)
})

watch(selectedPresetId, () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

watch([genMode, mode, kidsModeActive], () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

async function generate() {
  if (generatingImage.value) return
  if (!props.source) {
    error.value = t('fi.error_upload_first')
    return
  }
  if (genMode.value === 'preset' && !selectedPresetId.value && mode.value !== 'remove') {
    error.value = t('fi.error_select_style')
    return
  }
  if (genMode.value === 'compound' && !selectedCompoundPresetId.value && mode.value !== 'remove') {
    error.value = t('fi.error_select_workflow')
    return
  }
  if (folderQueue.value.length > 0 && mode.value !== 'img2img') {
    error.value = t('fi.folder_requires_img2img')
    return
  }
  if ((mode.value === 'inpaint' || mode.value === 'remove') && !hasMask.value) {
    error.value = t('fi.error_draw_mask')
    return
  }

  generatingImage.value = true
  generationStage.value = 'generating'
  genInfo.value = null
  qualityUpscaleSkipped.value = false
  qualityRedenoiseSkipped.value = false
  preserveIdentityResetFailed.value = false
  error.value = ''
  resetProgress()
  enqueuedJobIds.value = new Set()
  batchTotal.value = 0
  batchDone.value = 0
  folderCancelled = false

  try {
    const maskB64 = (mode.value === 'inpaint' || mode.value === 'remove') ? getMaskBase64() : ''
    const params = {
      image_base64: props.source,
      mode: mode.value === 'remove' ? 'inpaint' : mode.value,
      gen_mode: genMode.value,
      preset_id: selectedPresetId.value || 0,
      compound_preset_id: selectedCompoundPresetId.value || 0,
      denoising_strength: denoisingStrength.value,
      tags: mode.value === 'remove' ? '' : tags.value,
      extra_negative_prompt: extraNegativePrompt.value,
      resolution_id: selectedResolutionId.value || null,
      hires_profile_id: (genMode.value === 'compound' && mode.value !== 'remove') ? (selectedHiresProfileId.value || null) : null,
      remove_object: mode.value === 'remove',
      overrides: (genMode.value === 'preset' && mode.value !== 'remove') ? presetOverrides.value : null,
    }
    if (genMode.value === 'preset' && mode.value === 'img2img') {
      params.output_mode = (qualityMode.value && selectedResolutionId.value) ? 'quality' : ''
    }
    if (genMode.value === 'preset' && mode.value === 'img2img' && kleinPresetSelected.value && preserveIdentity.value) {
      params.preserve_identity = true
    }
    if (mode.value === 'inpaint' || mode.value === 'remove') {
      params.mask_base64 = maskB64
      params.mask_blur = maskBlur.value
      params.inpaint_fill = inpaintFill.value
      params.inpaint_full_res = inpaintFullRes.value
    }
    const seedVal = parseSeedInput(seedInput.value)
    if (folderQueue.value.length > 0) {
      const items = folderQueue.value
      let skipped = 0
      batchTotal.value = items.length
      for (let i = 0; i < items.length; i++) {
        if (folderCancelled) break
        let b64 = ''
        try {
          b64 = await api.readFileAsBase64(items[i].path)
        } catch {
          skipped++
          continue
        }
        if (!b64) {
          skipped++
          continue
        }
        try {
          const jobId = await api.enqueueFromImage({ ...params, image_base64: b64, seed: seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null })
          if (jobId) enqueuedJobIds.value.add(jobId)
        } catch {
          skipped++
        }
      }
      folderQueue.value = []
      batchTotal.value = batchDone.value + enqueuedJobIds.value.size
      if (folderCancelled) {
        error.value = 'interrupted'
        generatingImage.value = false
        generationStage.value = ''
        removeStage.value = ''
      } else if (enqueuedJobIds.value.size === 0) {
        error.value = t('fi.folder_skipped', { n: items.length, m: items.length })
        generatingImage.value = false
        generationStage.value = ''
        removeStage.value = ''
      } else if (skipped > 0) {
        error.value = t('fi.folder_skipped', { n: skipped, m: items.length })
      }
      return
    }
    const count = Math.max(1, Math.min(100, genCount.value || 1))
    batchTotal.value = count
    for (let i = 0; i < count; i++) {
      const jobId = await api.enqueueFromImage({ ...params, seed: seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null })
      if (jobId) enqueuedJobIds.value.add(jobId)
    }
    error.value = ''
  } catch (e) {
    error.value = String(e)
    if (enqueuedJobIds.value.size === 0) {
      generatingImage.value = false
      generationStage.value = ''
      removeStage.value = ''
    }
  }
}

async function onQueueCompleted(data) {
  if (!data?.job_id || !enqueuedJobIds.value.has(data.job_id)) return

  enqueuedJobIds.value.delete(data.job_id)
  batchDone.value++

  let gotResult = false
  if (data.result) {
    try {
      const r = typeof data.result === 'string' ? JSON.parse(data.result) : data.result
      if (r.image_base64) {
        gotResult = true
        lastEmittedSource = r.image_base64
        emit('result', r.image_base64)
        let info = null
        if (r.info) {
          try { info = typeof r.info === 'string' ? JSON.parse(r.info) : r.info } catch { info = r.info }
        }
        genInfo.value = info
        qualityUpscaleSkipped.value = r.quality_upscale_skipped || false
        qualityRedenoiseSkipped.value = r.quality_redenoise_skipped || false
        preserveIdentityResetFailed.value = r.preserve_identity_reset_failed || false
      }
    } catch {}
  }
  if (!gotResult) {
    try {
      const item = await api.getActiveSessionItem()
      if (item) {
        const b64 = await api.getSessionImage(item.id)
        if (b64) {
          lastEmittedSource = b64
          emit('result', b64)
        }
        genInfo.value = item.info || null
      }
    } catch {}
  }

  if (enqueuedJobIds.value.size === 0) {
    generatingImage.value = false
    generationStage.value = ''
    removeStage.value = ''
  }
}

function onQueueFailed(data) {
  if (!data?.job_id || !enqueuedJobIds.value.has(data.job_id)) return

  enqueuedJobIds.value.delete(data.job_id)
  batchDone.value++
  if (data.error) error.value = data.error

  if (enqueuedJobIds.value.size === 0) {
    generatingImage.value = false
    generationStage.value = ''
    removeStage.value = ''
  }
}

function onQueuePaused(data) {
  if (!data?.job_id || !enqueuedJobIds.value.has(data.job_id)) return
  if ((data.retry_count || 0) < (data.max_retries || 0)) return

  enqueuedJobIds.value.delete(data.job_id)
  batchDone.value++
  if (data.error) error.value = data.error

  if (enqueuedJobIds.value.size === 0) {
    generatingImage.value = false
    generationStage.value = ''
    removeStage.value = ''
  }
}

async function interruptBatch() {
  folderCancelled = true
  if (enqueuedJobIds.value.size > 0) {
    const ids = [...enqueuedJobIds.value]
    enqueuedJobIds.value = new Set()
    for (const id of ids) {
      try { await api.cancelQueueJob(id) } catch {}
    }
    generatingImage.value = false
    generationStage.value = ''
    removeStage.value = ''
    return
  }
  await interruptGeneration()
}

let offRemoveStage = () => {}
let offCompleted = () => {}
let offFailed = () => {}
let offPaused = () => {}
let settingsRestored = false

onMounted(async () => {
  document.addEventListener('keydown', onKeydown)
  offRemoveStage = EventsOn("remove:stage", (stage) => {
    removeStage.value = stage
  })
  offCompleted = EventsOn('queue:completed', onQueueCompleted)
  offFailed = EventsOn('queue:failed', onQueueFailed)
  offPaused = EventsOn('queue:paused', onQueuePaused)

  await loadPresets()
  loadKidsMode()
  try {
    hiresProfiles.value = await api.listHiresProfiles() || []
  } catch {
    loadHiresFailed.value = true
  }
  try {
    const s = await api.getSettings()
    if (s.fi_mode) mode.value = s.fi_mode
    if (s.fi_preset_id) {
      const pid = Number(s.fi_preset_id)
      if (presets.value.find(p => p.id === pid)) {
        selectedPresetId.value = pid
      }
    }
    if (s.fi_compound_preset_id) {
      const cpid = Number(s.fi_compound_preset_id)
      if (compoundPresets.value.find(c => c.id === cpid)) {
        selectedCompoundPresetId.value = cpid
      }
    }
    if (s.fi_gen_mode) genMode.value = s.fi_gen_mode
    if (s.fi_hires_profile_id && !loadHiresFailed.value) {
      const hid = Number(s.fi_hires_profile_id)
      if (hiresProfiles.value.find(h => h.id === hid)) {
        selectedHiresProfileId.value = hid
      }
    }
    if (s.fi_denoising) denoisingStrength.value = Number(s.fi_denoising)
    if (s.fi_quality_mode) qualityMode.value = s.fi_quality_mode === 'true'
    if (s.fi_preserve_identity) preserveIdentity.value = s.fi_preserve_identity === 'true'
    if (s.fi_extra_negative) extraNegativePrompt.value = s.fi_extra_negative
    if (s.fi_analyze_mode) analyzeMode.value = s.fi_analyze_mode === 'describe' ? 'describe' : 'quick'
    if (s.fi_mask_padding) maskPadding.value = Number(s.fi_mask_padding)
    if (s.fi_mask_feather) maskFeather.value = Number(s.fi_mask_feather)
    if (s.fi_count) genCount.value = Math.max(1, Math.min(100, Number(s.fi_count) || 1))
    settingsRestored = true
  } catch {}
  if (remixQueue.value.length > 0) {
    folderQueue.value = remixQueue.value
    clearRemixQueue()
    try {
      const b64 = await api.readFileAsBase64(folderQueue.value[0].path)
      if (b64) {
        tags.value = ''
        error.value = ''
        mode.value = 'img2img'
        lastEmittedSource = b64
        emit('source', b64)
      }
    } catch (e) {
      error.value = String(e)
    }
  }
})

onUnmounted(() => {
  folderCancelled = true
  document.removeEventListener('keydown', onKeydown)
  offRemoveStage()
  offCompleted()
  offFailed()
  offPaused()
  saveFIState()
})

function saveFIState() {
  if (!settingsRestored) return
  const state = {
    fi_mode: mode.value,
    fi_preset_id: String(selectedPresetId.value || ''),
    fi_compound_preset_id: String(selectedCompoundPresetId.value || ''),
    fi_gen_mode: genMode.value,
    fi_denoising: String(denoisingStrength.value || ''),
    fi_quality_mode: qualityMode.value ? 'true' : 'false',
    fi_preserve_identity: preserveIdentity.value ? 'true' : 'false',
    fi_extra_negative: extraNegativePrompt.value || '',
    fi_analyze_mode: analyzeMode.value || '',
    fi_mask_padding: String(maskPadding.value || ''),
    fi_mask_feather: String(maskFeather.value || ''),
    fi_count: String(genCount.value || 1),
  }
  if (!loadHiresFailed.value) {
    state.fi_hires_profile_id = selectedHiresProfileId.value ? String(selectedHiresProfileId.value) : ''
  }
  api.updateSettings(state).catch(() => {})
}

function onKeydown(e) {
  if (!props.active) return
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !generatingImage.value) {
    e.preventDefault()
    generate()
  }
}

defineExpose({
  maskActive,
  maskCanvas,
  maskVersion,
  brushSize,
  beginStroke,
  strokeTo,
  endStroke,
  undoMask,
  redoMask,
  canUndoMask: hasMask,
  canRedoMask,
})
</script>

<template>
  <div>
    <div v-if="kidsModeActive" class="service-status">
      <div class="status-badge status-ok">
        &#9679; {{ t('fi.kids_mode') }}
      </div>
    </div>

    <div v-if="error" class="status" :class="error === 'interrupted' ? 'status-warning' : 'status-error'">{{ error }}</div>

    <div v-if="folderQueue.length" class="fi-folder-badge">
      {{ t('fi.folder_queue', { n: folderQueue.length }) }}
    </div>

    <div style="display: flex; gap: 8px; margin-top: 12px; margin-bottom: 12px; flex-wrap: wrap;">
      <button class="btn btn-sm" :class="mode === 'img2img' ? 'btn-primary' : 'btn-secondary'" @click="mode = 'img2img'">{{ t('fi.mode_img2img') }}</button>
      <button class="btn btn-sm" :class="mode === 'inpaint' ? 'btn-primary' : 'btn-secondary'" @click="mode = 'inpaint'">{{ t('fi.mode_inpaint') }}</button>
      <button class="btn btn-sm" :class="mode === 'remove' ? 'btn-primary' : 'btn-secondary'" @click="mode = 'remove'">{{ t('fi.mode_remove') }}</button>
    </div>

    <div v-if="mode === 'img2img' || mode === 'inpaint'" class="form-group">
      <label class="form-label">{{ t('fi.label_denoising', { value: denoisingStrength.toFixed(2) }) }}</label>
      <input type="range" class="form-range" v-model.number="denoisingStrength" min="0.05" max="1.0" step="0.05" />
      <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--text-dim);">
        <span>{{ t('fi.keep_original') }}</span>
        <span>{{ t('fi.full_redraw') }}</span>
      </div>
    </div>

    <div v-if="mode !== 'remove'" class="form-group" style="margin-top: 4px;">
      <label class="form-label" :style="{ opacity: genMode === 'compound' ? 0.5 : 1 }">{{ t('fi.seed_label') }}</label>
      <input data-testid="fi-seed-input" class="form-input" type="number" v-model="seedInput" min="0" step="1" :disabled="genMode === 'compound'" :placeholder="t('fi.seed_placeholder')" :style="{ opacity: genMode === 'compound' ? 0.5 : 1 }" />
      <div v-if="genMode === 'compound'" style="font-size: 11px; color: var(--accent); margin-top: 4px;">{{ t('fi.preset_only') }}</div>
      <div v-else style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">{{ t('fi.seed_hint') }}</div>
    </div>

    <div v-if="mode !== 'remove'" class="form-group" style="margin-top: 4px;">
      <ResolutionSelector v-model="selectedResolutionId" :none-label="t('fi.resolution_original')" />
      <div v-if="mode === 'img2img'" style="margin-top: 6px;">
        <label :style="{ display: 'flex', alignItems: 'center', gap: '6px', cursor: (genMode === 'compound' || !selectedResolutionId) ? 'not-allowed' : 'pointer', opacity: (genMode === 'compound' || !selectedResolutionId) ? 0.5 : 1 }">
          <input data-testid="fi-quality-checkbox" type="checkbox" v-model="qualityMode" :disabled="genMode === 'compound' || !selectedResolutionId" style="accent-color: var(--accent);" />
          <span style="font-size: 12px;">{{ t('fi.quality_mode') }}</span>
        </label>
        <div v-if="genMode === 'preset'" style="font-size: 11px; color: var(--text-dim); margin-top: 2px;">{{ t('fi.quality_hint') }}</div>
        <div v-if="genMode === 'compound'" style="font-size: 11px; color: var(--accent); margin-top: 2px;">{{ t('fi.preset_only') }}</div>
        <div v-else-if="!selectedResolutionId" style="font-size: 11px; color: var(--accent); margin-top: 2px;">{{ t('fi.quality_needs_resolution') }}</div>
      </div>
      <div v-if="mode === 'img2img' && genMode === 'preset' && kleinPresetSelected" style="margin-top: 6px;">
        <label style="display: flex; align-items: center; gap: 6px; cursor: pointer;">
          <input data-testid="fi-preserve-identity-checkbox" type="checkbox" v-model="preserveIdentity" style="accent-color: var(--accent);" />
          <span style="font-size: 12px;">{{ t('fi.preserve_identity') }}</span>
        </label>
        <div style="font-size: 11px; color: var(--text-dim); margin-top: 2px;">{{ t('fi.preserve_identity_hint') }}</div>
      </div>
    </div>

    <div v-if="genMode === 'compound' && mode !== 'remove'" class="form-group" style="margin-top: 4px;">
      <HiresProfileSelector v-model="selectedHiresProfileId" />
      <div style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">{{ t('fi.hires_hint') }}</div>
    </div>

    <div v-if="mode === 'remove'" class="form-group" style="margin-top: 4px;">
      <div style="font-size: 11px; color: var(--text-dim);">
        {{ t('fi.remove_params', { denoising: denoisingStrength.toFixed(2), blur: maskBlur }) }}
      </div>
    </div>

    <div v-if="mode === 'inpaint' || mode === 'remove'" class="inpaint-controls">
      <div class="form-group">
        <label class="form-label">{{ t('fi.label_brush_size', { value: brushSize }) }}</label>
        <input type="range" class="form-range" v-model.number="brushSize" min="5" max="100" step="1" />
      </div>
      <div class="fi-btn-row">
        <button class="btn btn-sm btn-secondary" @click="clearMask" :disabled="!hasMask">{{ t('fi.btn_clear_mask') }}</button>
        <button class="btn btn-sm btn-secondary" @click="undoMask" :disabled="!hasMask">{{ t('fi.btn_undo') }}</button>
        <button class="btn btn-sm btn-secondary" @click="redoMask" :disabled="!canRedoMask">{{ t('fi.btn_redo') }}</button>
      </div>
      <div v-if="mode === 'inpaint'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-top: 8px;">
        <div class="form-group">
          <label class="form-label">{{ t('fi.label_mask_blur', { value: maskBlur }) }}</label>
          <input type="range" class="form-range" v-model.number="maskBlur" min="0" max="64" step="1" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('fi.label_inpaint_fill') }}</label>
          <select class="form-select" v-model="inpaintFill">
            <option :value="0">{{ t('fi.fill_fill') }}</option>
            <option :value="1">{{ t('fi.fill_original') }}</option>
            <option :value="2">{{ t('fi.fill_latent_noise') }}</option>
          </select>
        </div>
      </div>
      <div v-if="mode === 'inpaint'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-top: 4px;">
        <div class="form-group">
          <label class="form-label">{{ t('fi.label_mask_padding', { value: maskPadding }) }}</label>
          <input type="range" class="form-range" v-model.number="maskPadding" min="0" max="64" step="1" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('fi.label_mask_feather', { value: maskFeather }) }}</label>
          <input type="range" class="form-range" v-model.number="maskFeather" min="0" max="64" step="1" />
        </div>
      </div>
      <div v-if="mode === 'inpaint'" class="form-group" style="margin-top: 4px;">
        <label style="display: flex; align-items: center; gap: 6px; cursor: pointer;">
          <input type="checkbox" v-model="inpaintFullRes" style="accent-color: var(--accent);" />
          <span style="font-size: 12px;">{{ t('fi.inpaint_full_res') }}</span>
        </label>
        <label style="display: flex; align-items: center; gap: 6px; cursor: pointer; margin-top: 4px;">
          <input type="checkbox" v-model="invertMask" style="accent-color: var(--accent);" />
          <span style="font-size: 12px;">{{ t('fi.invert_mask') }}</span>
        </label>
      </div>
      <div v-if="hasMask" style="font-size: 11px; color: var(--accent); margin-top: 4px;">
        {{ t('fi.mask_drawn', { count: maskHistory.length, inverted: invertMask ? t('fi.mask_inverted') : '' }) }}
      </div>
      <div v-else style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">
        {{ t('fi.paint_areas') }}
      </div>
    </div>

    <div v-if="mode !== 'remove'" style="display: flex; gap: 8px; margin-bottom: 12px;">
      <button class="btn btn-sm" :class="genMode === 'preset' ? 'btn-primary' : 'btn-secondary'" @click="genMode = 'preset'">{{ t('fi.btn_style') }}</button>
      <button class="btn btn-sm" :class="genMode === 'compound' ? 'btn-primary' : 'btn-secondary'" @click="genMode = 'compound'">{{ t('fi.btn_workflow') }}</button>
    </div>

    <div v-if="mode !== 'remove' && genMode === 'preset'" style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px;">
      <div class="form-group">
        <label class="form-label">{{ t('fi.label_type') }}</label>
        <select class="form-select" v-model="selectedTypeId">
          <option :value="null">{{ t('fi.all_types') }}</option>
          <option v-for="t in presetTypes" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
      </div>
      <div class="form-group">
        <label class="form-label">{{ t('fi.label_style') }}</label>
        <select class="form-select" v-model="selectedPresetId">
          <option :value="null" disabled>{{ t('fi.select_style') }}</option>
          <option v-for="p in filteredPresets" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </div>
    </div>

    <div v-if="mode !== 'remove' && genMode === 'compound'" class="form-group">
      <label class="form-label">{{ t('fi.label_workflow') }}</label>
      <select class="form-select" v-model="selectedCompoundPresetId">
        <option :value="null" disabled>{{ t('fi.select_workflow') }}</option>
        <option v-for="c in compoundPresets" :key="c.id" :value="c.id">{{ c.name }} ({{ c.steps.length }} steps)</option>
      </select>
    </div>

    <PresetOverridesPanel v-if="mode !== 'remove' && genMode === 'preset' && !kidsModeActive" ref="overridesPanel" v-model="presetOverrides" :preset-loras="selectedPresetLoras" />

    <div v-if="mode !== 'remove'" class="form-group" style="margin-top: 12px;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px;">
        <label class="form-label" style="margin-bottom: 0;">{{ t('fi.extracted_tags') }}</label>
        <div style="display: flex; gap: 6px;">
          <button class="btn btn-sm" :class="analyzeMode === 'quick' ? 'btn-primary' : 'btn-secondary'" @click="analyzeMode = 'quick'" style="font-size: 11px; padding: 2px 8px;">{{ t('fi.tags') }}</button>
          <button class="btn btn-sm" :class="analyzeMode === 'describe' ? 'btn-primary' : 'btn-secondary'" @click="analyzeMode = 'describe'" style="font-size: 11px; padding: 2px 8px;">{{ t('fi.describe') }}</button>
        </div>
      </div>
      <div style="display: flex; gap: 8px; margin-bottom: 6px;">
        <button class="btn btn-sm btn-secondary" @click="analyzeImage" :disabled="analyzing || !source">
          <template v-if="analyzing">
            <span>{{ t('fi.analyzing') }} ({{ analyzeElapsed }}s)</span>
          </template>
          <span v-else>{{ t('fi.btn_analyze') }}</span>
        </button>
        <button v-if="!kidsModeActive" class="btn btn-sm btn-secondary" @click="tags = ''" :disabled="!tags" title="Clear tags">&#10005;</button>
      </div>
      <textarea class="form-textarea" v-model="tags" rows="4" :placeholder="t('fi.placeholder_tags')" :disabled="kidsModeActive"></textarea>
      <div v-if="kidsModeActive" style="margin-top: 4px; padding: 6px; background: var(--surface-2); border-radius: 4px; text-align: center; font-size: 11px; color: var(--text-dim);">
        {{ t('fi.kids_tags_restricted') }}
      </div>
    </div>

    <div v-if="!kidsModeActive" class="form-group">
      <label class="form-label">{{ t('fi.label_extra_exclude') }}</label>
      <textarea class="form-textarea" v-model="extraNegativePrompt" rows="2" :placeholder="t('fi.placeholder_extra_exclude')"></textarea>
    </div>

    <div style="display: flex; gap: 8px; width: 100%;">
      <button class="btn btn-primary" :class="{ 'btn-generating': generatingImage }" style="flex: 1; justify-content: center; padding: 12px;" @click="generate" :disabled="generatingImage || !source || ((mode === 'inpaint' || mode === 'remove') && !hasMask) || (mode !== 'remove' && (genMode === 'preset' ? !selectedPresetId : !selectedCompoundPresetId))">
        <span v-if="generatingImage" style="display: inline-flex; align-items: center; gap: 6px;">
          <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
          {{ progressLabel }}
        </span>
        <span v-else-if="folderQueue.length">{{ t('fi.folder_generate', { n: folderQueue.length }) }}</span>
        <span v-else>{{ genCount <= 1 ? t('fi.btn_generate') : t('fi.btn_generate_n', { count: genCount }) }}</span>
      </button>
      <input class="form-input" type="number" v-model.number="genCount" min="1" max="100" style="width: 70px; text-align: center;" :disabled="folderQueue.length > 0" @change="genCount = Math.max(1, Math.min(100, genCount || 1))" />
    </div>

    <div v-if="generatingImage" class="remix-progress">
      <img v-if="preview && sdProgress && sdProgress.progress > 0 && sdProgress.progress < 1" :src="preview" alt="preview" class="remix-progress-preview" />
      <div v-else class="remix-progress-row">
        <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
        <span class="remix-progress-label">{{ batchTotal > 1 ? (batchDone + 1) + ' / ' + batchTotal + ' — ' : '' }}{{ progressLabel }}</span>
      </div>
      <div v-if="sdProgress && sdProgress.progress > 0" class="remix-progress-info">
        <div class="remix-progress-row remix-progress-stats">
          <span>{{ Math.round(sdProgress.progress * 100) }}%</span>
          <span>{{ t('progress.sd_step', { current: Math.round(sdProgress.progress * sdProgress.steps), total: sdProgress.steps }) }}<span v-if="sdProgress.etaRelative > 0"> — ~{{ Math.ceil(sdProgress.etaRelative) }}s</span></span>
        </div>
        <div class="remix-progress-bar">
          <div :style="{ width: (sdProgress.progress * 100) + '%' }"></div>
        </div>
        <button class="btn btn-sm btn-secondary btn-block" @click="interruptBatch">{{ t('progress.btn_interrupt') }}</button>
      </div>
    </div>

    <div v-if="qualityUpscaleSkipped" class="status status-warning" style="margin-top: 8px;">
      {{ t('fi.quality_upscale_skipped') }}
    </div>
    <div v-if="qualityRedenoiseSkipped" class="status status-warning" style="margin-top: 8px;">
      {{ t('fi.quality_redenoise_skipped') }}
    </div>
    <div v-if="preserveIdentityResetFailed" class="status status-warning" style="margin-top: 8px;">
      {{ t('fi.preserve_identity_reset_failed') }}
    </div>

    <details v-if="genInfo" class="gen-info">
      <summary>{{ t('fi.technical_details') }}</summary>
      <pre style="white-space: pre-wrap; word-break: break-word; overflow-wrap: break-word;">{{ typeof genInfo === 'string' ? genInfo : JSON.stringify(genInfo, null, 2) }}</pre>
    </details>
  </div>
</template>

<style scoped>
.form-range {
  width: 100%;
  accent-color: var(--accent);
}
.fi-btn-row {
  display: flex;
  gap: 8px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.fi-folder-badge {
  display: inline-block;
  margin-top: 12px;
  padding: 4px 10px;
  background: var(--surface-2);
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  color: var(--text);
  font-size: 12px;
}
.inpaint-controls {
  margin-top: 12px;
  padding: 12px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.remix-progress {
  margin-top: 10px;
  text-align: center;
}
.remix-progress-preview {
  max-width: 100%;
  border-radius: var(--radius-sm);
  opacity: 0.6;
  image-rendering: pixelated;
}
.remix-progress-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  margin-top: 8px;
}
.remix-progress-label {
  color: var(--text-dim);
  font-size: 12px;
}
.remix-progress-stats {
  justify-content: space-between;
  margin-bottom: 4px;
}
.remix-progress-stats span {
  color: var(--text-dim);
  font-size: 12px;
}
.remix-progress-info {
  margin-top: 8px;
}
.remix-progress-bar {
  background: var(--surface-2);
  border-radius: 4px;
  overflow: hidden;
  height: 6px;
}
.remix-progress-bar div {
  background: var(--accent);
  height: 100%;
  transition: width 0.3s;
}
.remix-progress-info .btn {
  margin-top: 8px;
  font-size: 11px;
}
.btn-block {
  width: 100%;
}
.gen-info {
  margin-top: 10px;
  font-size: 11px;
  color: var(--text-dim);
}
.gen-info summary {
  cursor: pointer;
  user-select: none;
}
</style>
