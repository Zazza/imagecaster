<script setup>
import { Check, Sparkles, TriangleAlert, Maximize2, SlidersHorizontal } from 'lucide-vue-next'
import StatusMessage from './StatusMessage.vue'
import GenProgress from './GenProgress.vue'
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { parseSeedInput } from '../seed.js'
import { b64Url } from '../imageDataUrl.js'
import { parseLoras } from '../loraOverride.js'
import { newCustomStep, customStepsPayload, customStepLoras } from '../customSteps.js'
import { t } from '../i18n/index.js'
import { isEffectType } from '../effectTypes.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { useKidsMode } from '../composables/useKidsMode.js'
import { usePresets } from '../composables/usePresets.js'
import { useZoomPan, ZOOM_MIN, ZOOM_MAX, ZOOM_PRESETS } from '../composables/useZoomPan.js'
import { useJobTracker, resultImageUrl } from '../composables/useJobTracker.js'
import { useGenStatePersistence } from '../composables/useGenStatePersistence.js'
import { useSavedDescriptions } from '../composables/useSavedDescriptions.js'
import { useIdeasBatch } from '../composables/useIdeasBatch.js'
import SavedDescriptionsModal from './SavedDescriptionsModal.vue'
import ImageViewer from './ImageViewer.vue'
import { openInHistoryViewer } from '../historyViewer.js'
import ResolutionSelector from './ResolutionSelector.vue'
import HiresProfileSelector from './HiresProfileSelector.vue'
import PresetOverridesPanel from './PresetOverridesPanel.vue'
import CustomPipelineEditor from './CustomPipelineEditor.vue'

const { presets, presetTypes, compoundPresets, loadPresets } = usePresets()
watch(presets, list => {
  if (selectedPresetId.value && !list.some(p => p.id === selectedPresetId.value)) selectedPresetId.value = null
})

const props = defineProps({
  resetting: { type: Boolean, default: false },
})
const emit = defineEmits(['navigate'])
const selectedTypeId = ref(null)
const selectedPresetId = ref(null)
const selectedCompoundPresetId = ref(null)
const genMode = ref('preset')
const description = ref('')
const negative = ref('')
const extraPrompt = ref('')
const extraNegativePrompt = ref('')
const generatedImage = ref('')
const genInfo = ref(null)
const sourceImage = ref('')
const sourceGenInfo = ref(null)
const hiresSkipped = ref(false)
const upscalingX2 = ref(false)
const effectivePrompt = ref('')
const effectiveNegative = ref('')

const generatingImage = ref(false)
const { llmStatus, sdProgress, preview, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress()
const generationStage = ref('')
const error = ref('')
const successMsg = ref('')
let promptDirty = true

const { kidsModeActive, loadKidsMode } = useKidsMode()

const selectedResolutionId = ref(null)
const selectedHiresProfileId = ref(null)

const genCount = ref(1)
const seedInput = ref('')
const batchTotal = ref(0)
const jobs = useJobTracker({
  onCompleted: onQueueCompleted,
  onFailed: onQueueFailed,
  onIdle: () => {
    generatingImage.value = false
    generationStage.value = ''
  },
})
const enqueuedJobIds = jobs.pending
const batchDone = jobs.done

const showViewer = ref(false)
const shownItemId = ref(null)

const openSection = ref('main')

const genStageEl = ref(null)
const genZoom = useZoomPan(genStageEl)
const { isFit: genZoomFit, zoomPct: genZoomPct, zoomLabel: genZoomLabel, layerStyle: genLayerStyle, panEnabled: genPanEnabled, panning: genPanning, zoomMenuOpen: genZoomMenuOpen } = genZoom
watch(generatedImage, () => genZoom.setFit())
function onResultClick() {
  if (genZoom.wasDragged() || !genZoomFit.value) return
  openGeneratedViewer()
}

function setOpenSection(name) {
  if (openSection.value === name) return
  openSection.value = name
}

const presetOverrides = ref(null)
const overridesPanel = ref(null)

const customSteps = ref([newCustomStep()])

const showSavePipelineModal = ref(false)
const pipelineName = ref('')
const pipelineDescription = ref('')
const savingPipeline = ref(false)
const pipelineSavedMsg = ref('')

const isDesktop = ref(window.innerWidth > 1024)

const genTypes = computed(() => presetTypes.value.filter(pt => !isEffectType(pt.name)))

const filteredPresets = computed(() => {
  const ids = new Set(genTypes.value.map(pt => pt.id))
  const list = presets.value.filter(p => !isEffectType(p.preset_type) && (!p.type_id || ids.has(p.type_id)))
  if (!selectedTypeId.value) return list
  return list.filter(p => p.type_id === selectedTypeId.value)
})

const selectedPresetLoras = computed(() => {
  const p = presets.value.find(p => p.id === selectedPresetId.value)
  return p ? parseLoras(p.loras) : []
})

const customValid = computed(() => customSteps.value.length > 0 && customSteps.value.every(s => s.preset_id))

const customFirstPresetId = computed(() => (customSteps.value.length && customSteps.value[0].preset_id) || null)

const generateDisabled = computed(() => {
  if (ideasBatchRunning.value) return true
  if (genMode.value === 'preset') return !selectedPresetId.value
  if (genMode.value === 'custom') return !customValid.value
  return !selectedCompoundPresetId.value
})

const descs = useSavedDescriptions({ description, negative, error })
const {
  savedDescs, showSavedDescs, loadSavedDescs, saveDescription, deleteDescription, useDescription,
  handleCreateDesc, handleUpdateDesc,
} = descs

const { saveGenState, restoreGenState, persistGenState } = useGenStatePersistence({
  selectedTypeId, selectedPresetId, selectedCompoundPresetId, genMode, description, negative,
  extraPrompt, extraNegativePrompt, customSteps, selectedResolutionId, selectedHiresProfileId, genCount,
  presets, compoundPresets, genTypes, descs,
})

const {
  ideasBatchRunning, ideasBatchProgress, ideasBatchTarget, ideasBatchDisabled, runIdeasBatchFromModal, stopIdeasBatch,
} = useIdeasBatch({
  savedDescs, showSavedDescs, genMode, presets, compoundPresets, selectedPresetId, selectedCompoundPresetId,
  presetOverrides, customSteps, customValid, selectedResolutionId, selectedHiresProfileId,
  jobs, batchTotal, generatingImage, generationStage, error, successMsg, resetProgress, saveGenState,
})

const imageSrc = computed(() => b64Url(generatedImage.value))

const formattedGenInfo = computed(() => {
  if (!genInfo.value) return ''
  try {
    const parsed = typeof genInfo.value === 'string' ? JSON.parse(genInfo.value) : genInfo.value
    return JSON.stringify(parsed, null, 2)
  } catch (e) {
    console.warn('GeneratePage.formattedGenInfo:', e)
    return String(genInfo.value)
  }
})

watch([description, negative, selectedPresetId, selectedCompoundPresetId, customFirstPresetId, () => presetOverrides.value?.model_name], () => {
  promptDirty = true
})

watch(selectedTypeId, () => {
  const filtered = filteredPresets.value
  if (selectedPresetId.value && !filtered.find(p => p.id === selectedPresetId.value)) {
    selectedPresetId.value = null
  }
  api.updateSettings({ gen_type_id: String(selectedTypeId.value || '') }).catch(e => console.warn('GeneratePage.watch:', e))
})

watch(selectedPresetId, () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

watch([genMode, kidsModeActive], () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

function onResize() {
  isDesktop.value = window.innerWidth > 1024
}

async function sendToSD() {
  if (genMode.value === 'compound' && !selectedCompoundPresetId.value) {
    error.value = t('generate.error_select_workflow')
    return
  }
  if (genMode.value === 'custom' && !customValid.value) {
    error.value = t('generate.error_custom_step_style')
    return
  }
  if (genMode.value === 'preset' && !selectedPresetId.value) {
    error.value = t('generate.error_select_style')
    return
  }
  saveGenState()
  generationStage.value = 'image'
  generatingImage.value = true
  generatedImage.value = ''
  shownItemId.value = null
  genInfo.value = null
  hiresSkipped.value = false
  effectivePrompt.value = ''
  effectiveNegative.value = ''
  try {
    let result
    if (genMode.value === 'compound') {
      result = await api.generateCompoundImage({
        compound_preset_id: selectedCompoundPresetId.value,
        extra_prompt: extraPrompt.value,
        extra_negative_prompt: extraNegativePrompt.value,
        resolution_id: selectedResolutionId.value,
        hires_profile_id: selectedHiresProfileId.value,
      })
    } else if (genMode.value === 'custom') {
      result = await api.generateCompoundImage({
        steps: customStepsPayload(customSteps.value, presets.value),
        extra_prompt: extraPrompt.value,
        extra_negative_prompt: extraNegativePrompt.value,
        resolution_id: selectedResolutionId.value,
        hires_profile_id: selectedHiresProfileId.value,
        seed: parseSeedInput(seedInput.value),
      })
    } else {
      const seedVal = parseSeedInput(seedInput.value)
      result = await api.generateImage(selectedPresetId.value, extraPrompt.value, extraNegativePrompt.value, selectedResolutionId.value, selectedHiresProfileId.value, presetOverrides.value, seedVal)
    }
    if (!result || !result.image) {
      error.value = t('generate.error_no_image')
    } else {
      error.value = ''
      generatedImage.value = result.image
      shownItemId.value = result.session_item_id || null
      genInfo.value = result.info
      sourceImage.value = result.image
      sourceGenInfo.value = result.info
      hiresSkipped.value = result.hires_fix_skipped || false
      effectivePrompt.value = result.effective_prompt || ''
      effectiveNegative.value = result.effective_negative_prompt || ''
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    generatingImage.value = false
    generationStage.value = ''
  }
}

async function generateImage() {
  if (ideasBatchRunning.value) return
  if (genMode.value === 'compound' && !selectedCompoundPresetId.value) {
    error.value = t('generate.error_select_workflow')
    return
  }
  if (genMode.value === 'custom' && !customValid.value) {
    error.value = t('generate.error_custom_step_style')
    return
  }
  if (genMode.value === 'preset' && !selectedPresetId.value) {
    error.value = t('generate.error_select_style')
    return
  }
  saveGenState()
  resetProgress()
  generatingImage.value = true
  generatedImage.value = ''
  shownItemId.value = null
  genInfo.value = null
  hiresSkipped.value = false
  effectivePrompt.value = ''
  effectiveNegative.value = ''
  jobs.reset()
  batchTotal.value = 0

  const mode = genMode.value
  const presetId = selectedPresetId.value
  const compoundId = selectedCompoundPresetId.value
  const ov = presetOverrides.value
  const customPayload = customStepsPayload(customSteps.value, presets.value)

  if (promptDirty) {
    let llmPresetId = presetId
    if (mode === 'compound') {
      const cp = compoundPresets.value.find(c => c.id === compoundId)
      if (cp && cp.steps && cp.steps.length > 0) {
        llmPresetId = cp.steps[0].preset_id
      }
    } else if (mode === 'custom' && customPayload.length > 0) {
      llmPresetId = customPayload[0].preset_id
    }
    if (llmPresetId && (mode === 'preset' || mode === 'compound' || mode === 'custom')) {
      generationStage.value = 'prompt'
      error.value = ''
      try {
        const promptResult = await api.generateSdPrompt({
          preset_id: llmPresetId,
          description: description.value,
          negative: negative.value,
          overrides: mode === 'preset' ? ov : null,
        })
        if (promptResult && promptResult.prompt) {
          extraPrompt.value = promptResult.prompt
          extraNegativePrompt.value = promptResult.negative_prompt || ''
          promptDirty = false
        } else {
          error.value = t('generate.error_empty_llm')
          generatingImage.value = false
          generationStage.value = ''
          return
        }
      } catch (e) {
        error.value = t('generate.error_prompt_gen', { error: String(e) })
        generatingImage.value = false
        generationStage.value = ''
        return
      }
      generatingImage.value = false
      generationStage.value = ''
    }
  }

  generatingImage.value = true
  generationStage.value = 'image'
  const extraP = extraPrompt.value
  const extraN = extraNegativePrompt.value
  const resId = selectedResolutionId.value
  const hiresId = selectedHiresProfileId.value
  const seedVal = parseSeedInput(seedInput.value)

  try {
    const count = Math.max(1, Math.min(100, genCount.value || 1))
    batchTotal.value = count
    for (let i = 0; i < count; i++) {
      let jobId
      if (mode === 'compound') {
        jobId = await api.enqueueCompound({
          compound_preset_id: compoundId,
          extra_prompt: extraP,
          extra_negative_prompt: extraN,
          resolution_id: resId,
          hires_profile_id: hiresId,
        })
      } else if (mode === 'custom') {
        jobId = await api.enqueueCompound({
          steps: customPayload,
          extra_prompt: extraP,
          extra_negative_prompt: extraN,
          resolution_id: resId,
          hires_profile_id: hiresId,
          seed: seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null,
        })
      } else {
        jobId = await api.enqueueTxt2Img({
          preset_id: presetId,
          extra_prompt: extraP,
          extra_negative_prompt: extraN,
          resolution_id: resId,
          hires_profile_id: hiresId,
          overrides: ov,
          seed: seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null,
        })
      }
      if (jobId) jobs.track(jobId)
    }
    error.value = ''
  } catch (e) {
    error.value = String(e)
    if (enqueuedJobIds.value.size === 0) {
      generatingImage.value = false
      generationStage.value = ''
    }
  }
}

async function upscaleImageX2() {
  if (!generatedImage.value) return
  const b64 = await resolveImageBase64()
  if (!b64) return
  upscalingX2.value = true
  error.value = ''
  try {
    const result = await api.upscaleImage(b64, genInfo.value, selectedPresetId.value)
    if (!result || !result.image) {
      error.value = t('generate.error_upscale_x2')
    } else {
      generatedImage.value = result.image
      shownItemId.value = result.session_item_id || null
      genInfo.value = result.info
      sourceImage.value = result.image
      sourceGenInfo.value = result.info
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    upscalingX2.value = false
  }
}

async function resolveImageBase64() {
  if (!generatedImage.value) return null
  if (!generatedImage.value.startsWith('/api/')) return generatedImage.value
  const match = generatedImage.value.match(/\/api\/img\/(\d+)\.jpg/)
  if (match) {
    try { return await api.getSessionImage(Number(match[1])) } catch (e) { console.warn('GeneratePage.resolveImageBase64:', e); return null }
  }
  return generatedImage.value
}

async function toPostProcess() {
  if (!generatedImage.value) return
  try {
    const item = await api.getActiveSessionItem()
    if (!item) {
      error.value = t('fi.error_no_active_item')
      return
    }
    await api.setActiveSessionItem(item.id)
    emit('navigate', { page: 'postprocess' })
  } catch (e) {
    error.value = String(e)
  }
}

function onQueueCompleted(data, result) {
  if (!result?.hasImage) return
  generatedImage.value = resultImageUrl(result)
  shownItemId.value = result.sessionItemId
  sourceImage.value = generatedImage.value
  genInfo.value = result.info
  sourceGenInfo.value = result.info
  hiresSkipped.value = false
}

function onQueueFailed(data, { tracked, paused }) {
  if (paused && !tracked) return
  if (data.error) error.value = data.error
}

async function interruptBatch() {
  stopIdeasBatch()
  if (enqueuedJobIds.value.size > 0) {
    try {
      await api.cancelQueue()
    } catch (e) {
      error.value = String(e)
    }
    jobs.clear()
    generatingImage.value = false
    generationStage.value = ''
    return
  }
  await interruptGeneration()
}

async function onSessionAdded() {
  if (!generatingImage.value && generatedImage.value) return
  try {
    const item = await api.getActiveSessionItem()
    if (!item) return
    generatedImage.value = api.sessionImageUrl(item.id)
    shownItemId.value = item.id
    sourceImage.value = generatedImage.value
    genInfo.value = item.info || null
    sourceGenInfo.value = item.info || null
    if (enqueuedJobIds.value.size === 0) {
      generatingImage.value = false
      generationStage.value = ''
    }
  } catch (e) {
    console.warn('GeneratePage.onSessionAdded:', e)
  }
}

async function openGeneratedViewer() {
  if (await openInHistoryViewer(shownItemId.value)) return
  showViewer.value = true
}

function openSavePipelineModal() {
  if (!customValid.value) {
    error.value = t('generate.error_custom_step_style')
    return
  }
  pipelineName.value = ''
  pipelineDescription.value = ''
  showSavePipelineModal.value = true
}

async function confirmSavePipeline() {
  if (!pipelineName.value.trim() || savingPipeline.value) return
  savingPipeline.value = true
  const name = pipelineName.value.trim()
  try {
    await api.createCompoundPreset({
      name,
      description: pipelineDescription.value.trim(),
      steps: customSteps.value.map((s, i) => ({
        step_order: i + 1,
        preset_id: s.preset_id,
        denoising_strength: s.denoising_strength,
        loras: customStepLoras(s, presets.value),
      })),
    })
    showSavePipelineModal.value = false
    error.value = ''
    pipelineSavedMsg.value = t('generate.save_pipeline_success', { name })
    await loadPresets()
  } catch (e) {
    error.value = t('generate.error_save_pipeline', { error: String(e) })
  } finally {
    savingPipeline.value = false
  }
}

let offSessionAdded = () => {}

onMounted(async () => {
  const isReset = props.resetting
  await loadPresets()
  loadKidsMode()
  await loadSavedDescs()
  await restoreGenState()
  if (!isReset) {
    try {
      const item = await api.getActiveSessionItem()
      if (item) {
        generatedImage.value = api.sessionImageUrl(item.id)
        shownItemId.value = item.id
        genInfo.value = item.info || null
        sourceImage.value = generatedImage.value
        sourceGenInfo.value = item.info || null
      }
    } catch (e) {
      console.warn('GeneratePage.onMounted:', e)
    }
  }

  try {
    const queue = await api.getQueue()
    const activeJobs = (queue || []).filter(j => (j.type === 'txt2img' || j.type === 'compound') && (j.status === 'pending' || j.status === 'running'))
    if (activeJobs.length > 0) {
      generatingImage.value = true
      generationStage.value = 'image'
      jobs.reset()
      activeJobs.forEach(j => jobs.track(j.id))
      batchTotal.value = activeJobs.length
    }
  } catch (e) {
    console.warn('GeneratePage.onMounted queue:', e)
  }

  offSessionAdded = EventsOn('session:added', onSessionAdded)
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', onResize)
})

onUnmounted(() => {
  stopIdeasBatch()
  offSessionAdded()
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', onResize)
  persistGenState()
})

function onKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !generatingImage.value && !ideasBatchRunning.value) {
    e.preventDefault()
    generateImage()
  }
}
</script>

<template>
  <div class="page-enter gen-page">

    <div v-if="kidsModeActive" class="service-status">
      <div class="status-badge status-ok">
        &#9679; {{ t('generate.kids_mode') }}
      </div>
    </div>

    <StatusMessage v-if="error && error !== 'interrupted'" @close="error = ''"><TriangleAlert :size="14" class="inline-icon" /> {{ error }}</StatusMessage>
    <StatusMessage v-if="error === 'interrupted'" kind="warning" @close="error = ''">{{ error }}</StatusMessage>
    <StatusMessage v-if="successMsg && !error" kind="success" :timeout="8000" :text="successMsg" @close="successMsg = ''" />
    <StatusMessage v-if="pipelineSavedMsg" kind="ok" :timeout="8000" @close="pipelineSavedMsg = ''"><Check :size="14" class="inline-icon" /> {{ pipelineSavedMsg }}</StatusMessage>

    <div class="gen-workspace">
      <div class="gen-preview-col">
        <div class="generate-image-area" :class="{ 'loading': generatingImage && !generatedImage, 'empty': !generatedImage && !generatingImage }">
          <div v-if="generatingImage && !generatedImage" class="gen-loading">
            <img v-if="preview && sdProgress && sdProgress.progress > 0 && sdProgress.progress < 1" :src="preview" alt="preview" class="gen-loading-preview" />
            <span v-else class="spinner gen-spinner-lg"></span>
            <p class="gen-loading-text">{{ batchTotal > 1 ? (batchDone + 1) + ' / ' + batchTotal + ' — ' : '' }}{{ llmStatus === 'thinking' ? t('generate.generating_prompt') : upscalingX2 ? t('generate.upscaling_x2') : t('generate.generating_image') }}</p>
            <GenProgress v-if="sdProgress && sdProgress.progress > 0" class="gen-loading-progress" :progress="sdProgress.progress" :steps="sdProgress.steps" :eta="sdProgress.etaRelative" stats @interrupt="interruptBatch" />
          </div>
          <div v-else-if="generatingImage && generatedImage && enqueuedJobIds.size > 0" class="gen-batch">
            <img :src="imageSrc" alt="Generating..." class="gen-batch-img" />
            <div class="gen-batch-overlay">
              <img v-if="preview && sdProgress && sdProgress.progress > 0 && sdProgress.progress < 1" :src="preview" alt="preview" class="gen-batch-preview" />
              <span v-else class="spinner gen-spinner-md"></span>
              <p class="gen-batch-count">{{ (batchDone + 1) + ' / ' + batchTotal }}</p>
              <GenProgress v-if="sdProgress && sdProgress.progress > 0" class="gen-batch-progress" :progress="sdProgress.progress" :steps="sdProgress.steps" :eta="sdProgress.etaRelative" stats small :show-step="false" @interrupt="interruptBatch" />
            </div>
          </div>
          <div v-else-if="generatedImage" class="gen-result">
            <StatusMessage v-if="hiresSkipped" kind="warning" class="gen-hires-warning" @close="hiresSkipped = false">{{ t('generate.hires_skipped') }}</StatusMessage>
            <div class="gen-stage-wrap">
              <div class="viewfinder" aria-hidden="true"><span></span><span></span><span></span><span></span></div>
              <div
                ref="genStageEl"
                class="gen-stage"
                :class="{ fit: genZoomFit, pannable: genPanEnabled, panning: genPanning }"
                @pointerdown="genZoom.onPointerDown"
                @pointermove="genZoom.onPointerMove"
                @pointerup="genZoom.onPointerUp"
                @pointercancel="genZoom.onPointerUp"
                @wheel="genZoom.onWheel"
              >
                <img :src="imageSrc" alt="Generated" class="img-fade-in gen-stage-img" :style="genLayerStyle" draggable="false" @load="genZoom.onImgLoad" @click="onResultClick" />
                <div class="split-zoom" @pointerdown.stop @click.stop @dblclick.stop @wheel.stop>
                  <button class="split-zoom-btn" :title="t('viewer.zoom_out')" :aria-label="t('viewer.zoom_out')" :disabled="genZoomFit && genZoom.fitPercent() <= ZOOM_MIN" @click="genZoom.zoomBy(-10)">&minus;</button>
                  <button class="split-zoom-btn split-zoom-label" :title="t('viewer.zoom_presets')" :aria-expanded="genZoomMenuOpen" aria-haspopup="menu" @click="genZoom.onZoomIndicatorClick">{{ genZoomLabel }}</button>
                  <button class="split-zoom-btn" :title="t('viewer.zoom_in')" :aria-label="t('viewer.zoom_in')" :disabled="!genZoomFit && genZoomPct >= ZOOM_MAX" @click="genZoom.zoomBy(10)">&plus;</button>
                  <div v-if="genZoomMenuOpen" class="split-zoom-menu" role="menu">
                    <button :class="{ active: genZoomFit }" @click="genZoom.setFit">{{ t('viewer.zoom_fit') }}</button>
                    <button v-for="p in ZOOM_PRESETS" :key="p" :class="{ active: !genZoomFit && genZoomPct === p }" @click="genZoom.enterZoom(p)">{{ p }}%</button>
                  </div>
                </div>
              </div>
            </div>
            <div class="action-row gen-result-actions">
              <button class="btn btn-sm btn-secondary" @click="upscaleImageX2" :disabled="upscalingX2">
                <span v-if="upscalingX2" class="spinner gen-action-spinner"></span>
                <Maximize2 v-else :size="14" />
                {{ t('generate.btn_upscale_x2') }}
              </button>
              <button class="btn btn-sm btn-primary" @click="toPostProcess">
                <SlidersHorizontal :size="14" />
                {{ t('post.btn_send') }}
              </button>
            </div>
          </div>
          <div v-else class="generate-placeholder">
            <div class="generate-placeholder-icon"><Sparkles :size="56" :stroke-width="1.25" /></div>
            <div class="generate-placeholder-title">{{ t('generate.placeholder_image') }}</div>
            <div class="generate-placeholder-hint">{{ t('generate.placeholder_image_hint') }}</div>
          </div>
        </div>

        <details v-if="genInfo" class="gen-info card">
          <summary>{{ t('generate.technical_details') }}</summary>
          <pre class="gen-info-pre">{{ formattedGenInfo }}</pre>
        </details>

        <details v-if="effectivePrompt" class="gen-info card">
          <summary>{{ t('generate.final_prompt') }}</summary>
          <div class="gen-prompt-block">
            <div class="gen-prompt-label">{{ t('generate.positive') }}</div>
            <div class="gen-prompt-text">{{ effectivePrompt }}</div>
          </div>
          <div v-if="effectiveNegative" class="gen-prompt-negative">
            <div class="gen-prompt-label">{{ t('generate.negative') }}</div>
            <div class="gen-prompt-text">{{ effectiveNegative }}</div>
          </div>
        </details>
      </div>

      <div class="gen-controls">
        <div class="gen-sections">
          <div class="card" :class="{ collapsed: openSection !== 'main' }">
            <div class="card-header card-toggle" @click="setOpenSection('main')">
              <span class="card-heading"><span class="card-num mono">01</span><span class="card-heading-text"><span class="card-title">{{ t('gen.section_main') }}</span><span class="card-subtitle">{{ t('gen.sub_main') }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'main' }">
              <div class="card-body">
                <div class="form-group gen-mode-row">
                  <button class="btn btn-sm" :class="genMode === 'preset' ? 'btn-primary' : 'btn-secondary'" :aria-pressed="genMode === 'preset'" @click="genMode = 'preset'">{{ t('generate.btn_style') }}</button>
                  <button class="btn btn-sm" :class="genMode === 'compound' ? 'btn-primary' : 'btn-secondary'" :aria-pressed="genMode === 'compound'" @click="genMode = 'compound'">{{ t('generate.btn_workflow') }}</button>
                  <button class="btn btn-sm" :class="genMode === 'custom' ? 'btn-primary' : 'btn-secondary'" :aria-pressed="genMode === 'custom'" @click="genMode = 'custom'">{{ t('generate.btn_custom') }}</button>
                </div>

                <div v-if="genMode === 'preset'" class="form-group">
                  <div class="form-group">
                    <label class="form-label">{{ t('generate.label_type') }}</label>
                    <select class="form-select" v-model="selectedTypeId">
                      <option :value="null">{{ t('generate.all_types') }}</option>
                      <option v-for="pt in genTypes" :key="pt.id" :value="pt.id">{{ pt.name }}</option>
                    </select>
                  </div>
                  <div class="form-group">
                    <label class="form-label">{{ t('generate.label_style') }}</label>
                    <select class="form-select" v-model="selectedPresetId">
                      <option :value="null" disabled>{{ t('generate.select_style') }}</option>
                      <option v-for="p in filteredPresets" :key="p.id" :value="p.id">{{ p.name }}</option>
                    </select>
                  </div>
                </div>

                <div v-if="genMode === 'compound'" class="form-group">
                  <label class="form-label">{{ t('generate.label_workflow') }}</label>
                  <select class="form-select" v-model="selectedCompoundPresetId">
                    <option :value="null" disabled>{{ t('generate.select_workflow') }}</option>
                    <option v-for="c in compoundPresets" :key="c.id" :value="c.id">{{ c.name }} ({{ t('common.steps_n', { count: c.steps.length }) }})</option>
                  </select>
                </div>

                <div v-if="genMode === 'custom'" class="form-group">
                  <CustomPipelineEditor v-model="customSteps" :presets="presets" :disable-loras="kidsModeActive" />
                  <div class="gen-pipeline-actions">
                    <button class="btn btn-sm btn-secondary" :disabled="!customValid" @click="openSavePipelineModal">{{ t('generate.save_pipeline') }}</button>
                  </div>
                </div>

                <div class="form-group">
                  <label class="form-label">{{ t('generate.label_description') }}</label>
                  <textarea class="form-textarea" v-model="description" rows="4" :placeholder="t('generate.placeholder_description')"></textarea>
                  <div class="gen-desc-actions">
                    <button class="btn btn-sm btn-secondary" @click="saveDescription" :disabled="!description.trim()">{{ t('generate.btn_save') }}</button>
                    <button class="btn btn-sm btn-secondary" @click="showSavedDescs = !showSavedDescs">
                      {{ t('generate.saved_ideas') }} {{ savedDescs.length ? '(' + savedDescs.length + ')' : '' }}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="card" :class="{ collapsed: openSection !== 'params' }">
            <div class="card-header card-toggle" @click="setOpenSection('params')">
              <span class="card-heading"><span class="card-num mono">02</span><span class="card-heading-text"><span class="card-title">{{ t('gen.section_params') }}</span><span class="card-subtitle">{{ t('gen.sub_params') }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'params' }">
              <div class="card-body">
                <div class="form-group">
                  <label class="form-label">{{ t('generate.label_exclude') }}</label>
                  <textarea class="form-textarea" v-model="negative" rows="2" :placeholder="t('generate.placeholder_exclude')"></textarea>
                </div>
                <ResolutionSelector v-model="selectedResolutionId" />
                <HiresProfileSelector v-model="selectedHiresProfileId" />
                <div v-if="genMode === 'preset' || genMode === 'custom'" class="form-group">
                  <label class="form-label">{{ t('generate.seed_label') }}</label>
                  <input class="form-input" type="number" v-model="seedInput" min="0" step="1" :placeholder="t('generate.seed_placeholder')" />
                  <div class="gen-hint">{{ genMode === 'custom' ? t('generate.seed_hint_custom') : t('generate.seed_hint') }}</div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="genMode === 'preset' && !kidsModeActive" class="card" :class="{ collapsed: openSection !== 'overrides' }">
            <div class="card-header card-toggle" @click="setOpenSection('overrides')">
              <span class="card-heading"><span class="card-num mono">03</span><span class="card-heading-text"><span class="card-title">{{ t('gen.section_overrides') }}</span><span class="card-subtitle">{{ t('gen.sub_overrides') }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'overrides' }">
              <div class="card-body">
                <PresetOverridesPanel ref="overridesPanel" v-model="presetOverrides" :preset-loras="selectedPresetLoras" />
              </div>
            </div>
          </div>

          <div v-if="!kidsModeActive" class="card" :class="{ collapsed: openSection !== 'prompts' }">
            <div class="card-header card-toggle" @click="setOpenSection('prompts')">
              <span class="card-heading"><span class="card-num mono">04</span><span class="card-heading-text"><span class="card-title">{{ t('gen.section_prompts') }}</span><span class="card-subtitle">{{ t('gen.sub_prompts') }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'prompts' }">
              <div class="card-body">
                <div class="form-group">
                  <label class="form-label">{{ t('generate.label_positive_prompt') }}</label>
                  <textarea class="form-textarea" v-model="extraPrompt" rows="4" :placeholder="t('generate.placeholder_positive_prompt')"></textarea>
                </div>
                <div class="form-group">
                  <label class="form-label">{{ t('generate.label_negative_prompt') }}</label>
                  <textarea class="form-textarea" v-model="extraNegativePrompt" rows="2" :placeholder="t('generate.placeholder_negative_prompt')"></textarea>
                </div>
                <button class="btn btn-secondary gen-direct-btn" @click="sendToSD" :disabled="generateDisabled || !extraPrompt">
                  {{ t('generate.btn_generate_directly') }}
                </button>
              </div>
            </div>
          </div>
        </div>
        <div class="gen-actions">
          <div class="gen-actions-row">
            <button class="btn btn-primary btn-generate" :class="{ 'btn-generating': generatingImage }" @click="generateImage" :disabled="generateDisabled">
              <span v-if="generatingImage" class="gen-btn-inner">
                <span class="spinner gen-action-spinner"></span>
                {{ generationStage === 'prompt' ? t('generate.generating_prompt') : t('generate.generating_image') }}
              </span>
              <span v-else>{{ genCount <= 1 ? t('generate.btn_generate') : t('generate.btn_generate_n', { count: genCount }) }}</span>
            </button>
            <label class="gen-count" :title="t('generate.count_title')">
              <input class="form-input" type="number" v-model.number="genCount" min="1" max="100" :aria-label="t('generate.count_title')" @change="genCount = Math.max(1, Math.min(100, genCount || 1))" />
              <span class="gen-count-unit">{{ t('generate.count_unit') }}</span>
            </label>
          </div>
          <div v-if="isDesktop" class="shortcut-hint">{{ t('generate.shortcut_hint') }}</div>
        </div>
      </div>
    </div>

    <SavedDescriptionsModal
      :visible="showSavedDescs"
      :descriptions="savedDescs"
      @close="showSavedDescs = false"
      @use="useDescription"
      @create="handleCreateDesc"
      @update="handleUpdateDesc"
      @delete="deleteDescription"
      :batch-target="ideasBatchTarget"
      :batch-running="ideasBatchRunning"
      :batch-progress="ideasBatchProgress"
      :batch-disabled="ideasBatchDisabled"
      @batch="runIdeasBatchFromModal"
      @batch-stop="stopIdeasBatch"
    />

    <ImageViewer v-if="showViewer" :image-base64="generatedImage" @close="showViewer = false" />

    <div v-if="showSavePipelineModal" class="modal-overlay">
      <div class="modal-content gen-modal">
        <h3 class="gen-modal-title">{{ t('generate.save_pipeline_title') }}</h3>
        <div class="form-group">
          <label class="form-label">{{ t('generate.save_pipeline_name') }}</label>
          <input class="form-input" v-model="pipelineName" :placeholder="t('generate.save_pipeline_placeholder_name')" @keydown.enter="confirmSavePipeline" :disabled="savingPipeline" autofocus />
        </div>
        <div class="form-group gen-modal-field">
          <label class="form-label">{{ t('generate.save_pipeline_description') }}</label>
          <input class="form-input" v-model="pipelineDescription" :placeholder="t('generate.save_pipeline_placeholder_description')" :disabled="savingPipeline" />
        </div>
        <div class="gen-modal-actions">
          <button class="btn btn-secondary" @click="showSavePipelineModal = false" :disabled="savingPipeline">{{ t('compound.btn_cancel') }}</button>
          <button class="btn btn-primary" @click="confirmSavePipeline" :disabled="!pipelineName.trim() || savingPipeline">
            <span v-if="savingPipeline" class="spinner gen-action-spinner gen-spinner-inline"></span>
            {{ t('generate.save_pipeline') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.gen-page {
  padding: 12px;
  height: 100%;
  overflow-y: auto;
}

.gen-workspace {
  display: flex;
  gap: 10px;
  height: calc(100vh - 124px - var(--titlebar-h, 32px) - var(--frame-pad, 6px));
  min-height: 480px;
}

.gen-preview-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
  overflow-y: auto;
}

.gen-controls {
  width: 360px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.gen-sections {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.gen-actions {
  flex-shrink: 0;
  padding: 10px 12px;
  border-top: 1px solid var(--border);
}

.gen-actions-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.gen-sections > .card {
  flex-shrink: 0;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: 0;
  box-shadow: none;
}

.gen-sections > .card + .card {
  border-top: 1px solid var(--border);
}

.gen-controls .card-header {
  padding: 9px 12px;
  border-bottom: none;
}

.gen-sections > .card.collapsed .card-header:hover {
  background: var(--surface-2);
}

.gen-sections > .card:not(.collapsed) {
  border-color: var(--accent);
}

.gen-sections > .card:not(.collapsed) .card-num {
  color: var(--accent);
}

.gen-controls .card-collapse > .card-body {
  padding: 10px 12px 12px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 9px 12px;
  border-bottom: 1px solid var(--border);
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
  transition: grid-template-rows 0.15s ease;
}

.card-collapse > .card-body {
  overflow: hidden;
  min-height: 0;
}

@media (max-width: 900px) {
  .gen-workspace {
    flex-direction: column;
    height: auto;
  }
  .gen-controls {
    width: auto;
  }
}
.gen-controls .btn-generate {
  justify-content: center;
  padding: 8px 16px;
  font-size: 13px;
  font-weight: 600;
}
.card-collapse.collapsed > .card-body {
  padding-top: 0;
  padding-bottom: 0;
  overflow: clip;
}

.gen-preview-col .generate-image-area {
  flex: 1;
  min-height: 0;
  flex-direction: column;
  align-items: stretch;
}

.gen-result {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  width: 100%;
}
.gen-result-actions {
  padding: 2px 10px 12px;
  gap: 8px;
}
.gen-result-actions .btn {
  gap: 6px;
}
.gen-action-spinner {
  width: 14px;
  height: 14px;
  border-width: 2px;
}
.gen-stage-wrap {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  padding: 10px;
}
.gen-stage {
  position: relative;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: #000;
  touch-action: none;
  user-select: none;
}
.gen-stage.fit .gen-stage-img {
  cursor: zoom-in;
}
.gen-stage.pannable {
  cursor: grab;
}
.gen-stage.panning {
  cursor: grabbing;
}
.gen-stage-img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  object-position: center;
  transform-origin: center;
}
.gen-stage:not(.fit) .gen-stage-img {
  pointer-events: none;
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
.gen-loading {
  text-align: center;
  padding: 24px;
}
.gen-loading-preview {
  max-width: 100%;
  border-radius: var(--radius-sm);
  opacity: 0.6;
  image-rendering: pixelated;
}
.gen-spinner-lg {
  width: 32px;
  height: 32px;
  border-width: 3px;
}
.gen-loading-text {
  margin-top: 12px;
  color: var(--text-dim);
}
.gen-loading-progress {
  margin-top: 12px;
  max-width: 300px;
  margin-left: auto;
  margin-right: auto;
}
.gen-batch {
  width: 100%;
  padding: 12px;
  position: relative;
}
.gen-batch-img {
  border-radius: var(--radius-sm);
  opacity: 0.4;
  width: 100%;
}
.gen-batch-overlay {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  text-align: center;
  background: rgba(0,0,0,0.6);
  padding: 16px 24px;
  border-radius: 8px;
}
.gen-batch-preview {
  max-width: 200px;
  border-radius: var(--radius-sm);
  opacity: 0.8;
  image-rendering: pixelated;
}
.gen-spinner-md {
  width: 24px;
  height: 24px;
  border-width: 2px;
}
.gen-batch-count {
  margin-top: 8px;
  color: var(--text-dim);
  font-size: 13px;
}
.gen-batch-progress {
  margin-top: 8px;
  min-width: 180px;
}
.gen-hires-warning {
  margin-bottom: 8px;
  text-align: center;
}
.gen-info-pre {
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: break-word;
}
.gen-prompt-block {
  margin-bottom: 8px;
}
.gen-prompt-label {
  color: var(--text-dim);
  font-size: 11px;
  margin-bottom: 4px;
}
.gen-prompt-text {
  font-size: 12px;
  line-height: 1.5;
  word-break: break-word;
}
.gen-prompt-negative {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}
.gen-mode-row {
  display: flex;
  gap: 8px;
}
.gen-pipeline-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 8px;
}
.gen-desc-actions {
  display: flex;
  gap: 8px;
  margin-top: 6px;
}
.gen-hint {
  font-size: 11px;
  color: var(--text-dim);
  margin-top: 4px;
}
.gen-direct-btn {
  width: 100%;
  justify-content: center;
}
.gen-btn-inner {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.gen-modal {
  max-width: 400px;
}
.gen-modal-title {
  margin: 0 0 12px;
}
.gen-modal-field {
  margin-top: 8px;
}
.gen-modal-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 12px;
}
.gen-spinner-inline {
  display: inline-block;
  vertical-align: middle;
  margin-right: 4px;
}
</style>
