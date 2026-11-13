<script setup>
import { Check, Sparkles, TriangleAlert } from 'lucide-vue-next'
import StatusMessage from './StatusMessage.vue'
import { ref, computed, onMounted, onUnmounted, watch, inject } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { parseSeedInput } from '../seed.js'
import { parseLoras } from '../loraOverride.js'
import { newCustomStep, restoreCustomSteps, persistCustomSteps, customStepsPayload, customStepLoras } from '../customSteps.js'
import { t } from '../i18n/index.js'
import { isEffectType } from '../effectTypes.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { useKidsMode } from '../composables/useKidsMode.js'
import { usePresets } from '../composables/usePresets.js'
import { useZoomPan, ZOOM_MIN, ZOOM_MAX, ZOOM_PRESETS } from '../composables/useZoomPan.js'
import { runIdeasBatch } from '../ideasBatch.js'
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

const showFastSaveModal = ref(false)
const fastSaveFilename = ref('')
const fastSaveFormat = ref('jpg')
const fastSaveLoading = ref(false)

const generatingImage = ref(false)
const { llmStatus, sdProgress, preview, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress()
const generationStage = ref('')
const error = ref('')
const successMsg = ref('')
let promptDirty = true
let currentDescId = null
let lastUsedDescText = ''
let lastUsedDescNegative = ''

const { kidsModeActive, loadKidsMode } = useKidsMode()

const selectedResolutionId = ref(null)
const selectedHiresProfileId = ref(null)

const shared = inject('sharedGenState', null)

const genCount = ref(1)
const seedInput = ref('')
const enqueuedJobIds = ref(new Set())
const batchTotal = ref(0)
const batchDone = ref(0)

const savedDescs = ref([])
const showSavedDescs = ref(false)
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

const ideasBatchRunning = ref(false)
const ideasBatchProgress = ref(null)
let ideasBatchStopped = false

const ideasBatchTarget = computed(() => {
  if (genMode.value === 'preset') {
    const p = presets.value.find(x => x.id === selectedPresetId.value)
    return p ? t('descriptions.batch_target_preset', { name: p.name }) : ''
  }
  if (genMode.value === 'custom') return customValid.value ? t('descriptions.batch_target_custom', { n: customSteps.value.length }) : ''
  const c = compoundPresets.value.find(x => x.id === selectedCompoundPresetId.value)
  return c ? t('descriptions.batch_target_workflow', { name: c.name }) : ''
})

const ideasBatchDisabled = computed(() => {
  if (genMode.value === 'preset') return selectedPresetId.value ? '' : t('descriptions.batch_no_style')
  if (genMode.value === 'custom') return customValid.value ? '' : t('descriptions.batch_bad_custom')
  return selectedCompoundPresetId.value ? '' : t('descriptions.batch_no_workflow')
})

function ideaDisplayName(desc) {
  if (desc.name) return desc.name
  const text = desc.text || ''
  return text.length > 40 ? text.slice(0, 40) + '…' : text
}

async function runIdeasBatchFromModal({ ids, perIdea }) {
  if (ideasBatchRunning.value || ideasBatchDisabled.value) return
  const byId = new Map(savedDescs.value.map(d => [d.id, d]))
  const ideas = (ids || []).map(id => byId.get(id)).filter(Boolean)
  if (!ideas.length) return

  const mode = genMode.value
  const presetId = selectedPresetId.value
  const compoundId = selectedCompoundPresetId.value
  const ov = presetOverrides.value
  const customPayload = customStepsPayload(customSteps.value, presets.value)
  const resId = selectedResolutionId.value
  const hiresId = selectedHiresProfileId.value

  saveGenState()
  if (enqueuedJobIds.value.size === 0) {
    resetProgress()
    batchTotal.value = 0
    batchDone.value = 0
  }
  ideasBatchStopped = false
  ideasBatchRunning.value = true
  ideasBatchProgress.value = { done: 0, total: ideas.length, errors: [] }
  error.value = ''
  successMsg.value = ''

  try {
    const res = await runIdeasBatch({
      ideas,
      perIdea,
      isStopped: () => ideasBatchStopped,
      generatePrompt: (idea) => ({ prompt: idea.text || '', negative_prompt: idea.negative_prompt || '' }),
      enqueue: async ({ prompt, negative_prompt }) => {
        const common = { extra_prompt: '', extra_negative_prompt: '', idea: prompt, idea_negative: negative_prompt, resolution_id: resId, hires_profile_id: hiresId }
        let jobId
        if (mode === 'compound') jobId = await api.enqueueCompound({ compound_preset_id: compoundId, ...common })
        else if (mode === 'custom') jobId = await api.enqueueCompound({ steps: customPayload, ...common, seed: null })
        else jobId = await api.enqueueTxt2Img({ preset_id: presetId, ...common, overrides: ov, seed: null })
        if (jobId) {
          enqueuedJobIds.value.add(jobId)
          batchTotal.value++
          generatingImage.value = true
          generationStage.value = 'image'
        }
        return jobId
      },
      onProgress: ({ done, total }) => {
        ideasBatchProgress.value = { ...ideasBatchProgress.value, done, total }
      },
    })
    ideasBatchProgress.value = {
      ...ideasBatchProgress.value,
      errors: res.errors.map(e => ({ name: ideaDisplayName(e.idea), error: e.error === 'empty prompt' ? t('descriptions.batch_empty_idea') : e.error })),
    }
    if (res.jobIds.length) {
      successMsg.value = t('descriptions.batch_done', { n: res.jobIds.length })
      ideasBatchProgress.value = { ...ideasBatchProgress.value, message: successMsg.value }
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    ideasBatchRunning.value = false
    if (enqueuedJobIds.value.size === 0) {
      generatingImage.value = false
      generationStage.value = ''
    }
  }
}

watch(showSavedDescs, (open) => {
  if (open && !ideasBatchRunning.value) ideasBatchProgress.value = null
})

function stopIdeasBatch() {
  ideasBatchStopped = true
}

const imageSrc = computed(() => {
  if (!generatedImage.value) return ''
  if (generatedImage.value.startsWith('/api/')) return generatedImage.value
  return 'data:image/png;base64,' + generatedImage.value
})

const formattedGenInfo = computed(() => {
  if (!genInfo.value) return ''
  try {
    const parsed = typeof genInfo.value === 'string' ? JSON.parse(genInfo.value) : genInfo.value
    return JSON.stringify(parsed, null, 2)
  } catch {
    return String(genInfo.value)
  }
})

watch([description, negative, selectedPresetId, selectedCompoundPresetId, customFirstPresetId, () => presetOverrides.value?.model_name], () => {
  promptDirty = true
})

watch(description, (val) => {
  if (currentDescId !== null && val !== lastUsedDescText) {
    if (negative.value === lastUsedDescNegative) negative.value = ''
    currentDescId = null
    lastUsedDescText = ''
    lastUsedDescNegative = ''
  }
})

watch(selectedTypeId, () => {
  const filtered = filteredPresets.value
  if (selectedPresetId.value && !filtered.find(p => p.id === selectedPresetId.value)) {
    selectedPresetId.value = null
  }
  api.updateSettings({ gen_type_id: String(selectedTypeId.value || '') }).catch(() => {})
})

watch(selectedPresetId, () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

watch([genMode, kidsModeActive], () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

watch([selectedResolutionId, selectedHiresProfileId], () => {
  api.updateSettings({
    gen_resolution_id: String(selectedResolutionId.value || ''),
    gen_hires_profile_id: String(selectedHiresProfileId.value || ''),
  }).catch(() => {})
})

function onResize() {
  isDesktop.value = window.innerWidth > 1024
}

function saveGenState() {
  api.updateSettings({
    gen_preset_id: String(selectedPresetId.value || ''),
    gen_type_id: String(selectedTypeId.value || ''),
    gen_description: description.value || '',
    gen_negative: negative.value || '',
    gen_desc_id: String(currentDescId || ''),
    gen_extra_prompt: extraPrompt.value || '',
    gen_extra_negative: extraNegativePrompt.value || '',
    gen_mode: genMode.value,
    gen_compound_preset_id: String(selectedCompoundPresetId.value || ''),
    gen_custom_steps: JSON.stringify(persistCustomSteps(customSteps.value, presets.value)),
    gen_resolution_id: String(selectedResolutionId.value || ''),
    gen_hires_profile_id: String(selectedHiresProfileId.value || ''),
    gen_count: String(genCount.value || 1),
  }).catch(() => {})
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
  enqueuedJobIds.value = new Set()
  batchTotal.value = 0
  batchDone.value = 0

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
      if (jobId) enqueuedJobIds.value.add(jobId)
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

async function loadSavedDescs() {
  try {
    savedDescs.value = await api.listDescriptions() || []
  } catch (e) {
    console.error(e)
  }
}

async function saveDescription() {
  if (!description.value.trim()) return
  try {
    const created = await api.createDescriptionFull({
      text: description.value.trim(),
      name: '',
      negative_prompt: negative.value || '',
      type: '',
    })
    if (created && created.id) {
      currentDescId = created.id
      lastUsedDescText = description.value.trim()
      lastUsedDescNegative = negative.value || ''
    }
    await loadSavedDescs()
  } catch (e) {
    error.value = t('generate.error_save_desc', { error: String(e) })
  }
}

async function deleteDescription(id) {
  try {
    await api.deleteDescription(id)
    if (currentDescId === id) {
      currentDescId = null
      lastUsedDescText = ''
      lastUsedDescNegative = ''
    }
    await loadSavedDescs()
  } catch (e) {
    error.value = t('generate.error_delete_desc', { error: String(e) })
  }
}

function useDescription(desc) {
  currentDescId = desc.id
  lastUsedDescText = desc.text
  lastUsedDescNegative = desc.negative_prompt || ''
  description.value = desc.text
  if (desc.negative_prompt) negative.value = desc.negative_prompt
  showSavedDescs.value = false
}

async function handleCreateDesc(data) {
  try {
    await api.createDescriptionFull(data)
    await loadSavedDescs()
  } catch (e) {
    error.value = t('generate.error_create', { error: String(e) })
  }
}

async function handleUpdateDesc(data) {
  try {
    await api.updateDescription(data)
    await loadSavedDescs()
    if (currentDescId === data.id) {
      const negLoaded = negative.value === lastUsedDescNegative
      lastUsedDescText = data.text
      lastUsedDescNegative = data.negative_prompt || ''
      description.value = data.text
      if (negLoaded) negative.value = data.negative_prompt || ''
    }
  } catch (e) {
    error.value = t('generate.error_update', { error: String(e) })
  }
}

async function resolveImageBase64() {
  if (!generatedImage.value) return null
  if (!generatedImage.value.startsWith('/api/')) return generatedImage.value
  const match = generatedImage.value.match(/\/api\/img\/(\d+)\.jpg/)
  if (match) {
    try { return await api.getSessionImage(Number(match[1])) } catch { return null }
  }
  return generatedImage.value
}

async function downloadImage() {
  if (!generatedImage.value) return
  try {
    const b64 = await resolveImageBase64()
    if (!b64) return
    const defaultName = 'imagecaster-' + Date.now() + '.png'
    const savedPath = await api.saveImage(b64, defaultName)
    if (savedPath) {
      error.value = ''
    }
  } catch (e) {
    error.value = t('generate.error_save', { error: String(e) })
  }
}

function openFastSaveModal() {
  fastSaveFilename.value = 'imagecaster-' + Date.now()
  showFastSaveModal.value = true
}

async function confirmFastSave() {
  if (!fastSaveFilename.value.trim() || !generatedImage.value) return
  fastSaveLoading.value = true
  try {
    const b64 = await resolveImageBase64()
    if (!b64) return
    const savedPath = await api.fastSaveImage(b64, fastSaveFilename.value.trim(), fastSaveFormat.value)
    if (savedPath) {
      showFastSaveModal.value = false
      error.value = ''
    }
  } catch (e) {
    error.value = t('generate.error_quick_save', { error: String(e) })
  } finally {
    fastSaveLoading.value = false
  }
}

function copyPrompt() {
  const parts = []
  if (effectivePrompt.value) parts.push(effectivePrompt.value)
  if (effectiveNegative.value) parts.push('Negative: ' + effectiveNegative.value)
  if (parts.length) navigator.clipboard.writeText(parts.join('\n'))
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

function onQueueCompleted(data) {
  if (!data) return

  if (data.job_id && enqueuedJobIds.value.has(data.job_id)) {
    enqueuedJobIds.value.delete(data.job_id)
    batchDone.value++
  }

  if (data.result) {
    try {
      const r = typeof data.result === 'string' ? JSON.parse(data.result) : data.result
      if (r.image_base64) {
        generatedImage.value = r.image_base64
        shownItemId.value = r.session_item_id || null
        sourceImage.value = r.image_base64
        let info = null
        if (r.info) {
          try { info = typeof r.info === 'string' ? JSON.parse(r.info) : r.info } catch { info = r.info }
        }
        genInfo.value = info
        sourceGenInfo.value = info
        hiresSkipped.value = false
      }
    } catch {}
  }

  if (enqueuedJobIds.value.size === 0) {
    generatingImage.value = false
    generationStage.value = ''
  }
}

function onQueueFailed(data) {
  if (!data) return

  if (data.job_id && enqueuedJobIds.value.has(data.job_id)) {
    enqueuedJobIds.value.delete(data.job_id)
    batchDone.value++
  }

  if (data.error) error.value = data.error

  if (enqueuedJobIds.value.size === 0) {
    generatingImage.value = false
    generationStage.value = ''
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
  }
}

async function interruptBatch() {
  ideasBatchStopped = true
  if (enqueuedJobIds.value.size > 0) {
    try { await api.cancelQueue() } catch {}
    enqueuedJobIds.value = new Set()
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
  } catch {}
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

let offCompleted = () => {}
let offFailed = () => {}
let offSessionAdded = () => {}
let offPaused = () => {}

onMounted(async () => {
  const isReset = props.resetting
  await loadPresets()
  loadKidsMode()
  await loadSavedDescs()
  let resolutionLoaded = false
  try {
    const s = await api.getSettings()
    if (s.gen_type_id) {
      const tid = Number(s.gen_type_id) || null
      if (tid && genTypes.value.find(pt => pt.id === tid)) {
        selectedTypeId.value = tid
      }
    }
    if (s.gen_extra_prompt) extraPrompt.value = s.gen_extra_prompt
    if (s.gen_extra_negative) extraNegativePrompt.value = s.gen_extra_negative
    if (s.gen_preset_id) {
      const pid = Number(s.gen_preset_id)
      if (presets.value.find(p => p.id === pid)) {
        selectedPresetId.value = pid
      }
    }
    if (s.gen_description) description.value = s.gen_description
    const rid = Number(s.gen_desc_id) || null
    const rdesc = rid && savedDescs.value.find(d => d.id === rid)
    let bindingRestored = false
    if (rdesc && rdesc.text === description.value) {
      currentDescId = rdesc.id
      lastUsedDescText = rdesc.text
      lastUsedDescNegative = rdesc.negative_prompt || ''
      bindingRestored = true
    }
    if (s.gen_negative && (bindingRestored || !rid)) negative.value = s.gen_negative
    if (s.gen_mode === 'preset' || s.gen_mode === 'compound' || s.gen_mode === 'custom') genMode.value = s.gen_mode
    if (s.gen_compound_preset_id) {
      const cpid = Number(s.gen_compound_preset_id)
      if (compoundPresets.value.find(c => c.id === cpid)) {
        selectedCompoundPresetId.value = cpid
      }
    }
    if (s.gen_custom_steps) {
      try {
        customSteps.value = restoreCustomSteps(JSON.parse(s.gen_custom_steps), presets.value)
      } catch {}
    }
    if (s.gen_resolution_id) {
      selectedResolutionId.value = Number(s.gen_resolution_id)
      resolutionLoaded = true
    }
    if (s.gen_hires_profile_id) selectedHiresProfileId.value = Number(s.gen_hires_profile_id)
    if (s.gen_count) genCount.value = Math.max(1, Math.min(100, Number(s.gen_count) || 1))
  } catch {}
  if (shared) {
    if (shared.selectedPresetId) {
      const pid = Number(shared.selectedPresetId)
      if (presets.value.find(p => p.id === pid)) {
        selectedPresetId.value = pid
      }
    }
    if (shared.selectedCompoundPresetId) {
      const cpid = Number(shared.selectedCompoundPresetId)
      if (compoundPresets.value.find(c => c.id === cpid)) {
        selectedCompoundPresetId.value = cpid
      }
    }
    if (shared.genMode === 'preset' || shared.genMode === 'compound' || shared.genMode === 'custom') genMode.value = shared.genMode
    if (shared.description) description.value = shared.description
    if (shared.negative) negative.value = shared.negative
    if (shared.selectedResolutionId) {
      selectedResolutionId.value = shared.selectedResolutionId
      resolutionLoaded = true
    }
    if (shared.selectedHiresProfileId !== undefined) selectedHiresProfileId.value = shared.selectedHiresProfileId
  }
  if (!resolutionLoaded) {
    try {
      const resolutions = await api.listResolutions()
      if (resolutions && resolutions.length > 0) selectedResolutionId.value = resolutions[0].id
    } catch {}
  }
  if (!isReset) {
    try {
      const item = await api.getActiveSessionItem()
      if (item) {
        generatedImage.value = api.sessionImageUrl(item.id)
        shownItemId.value = item.id
    shownItemId.value = item.id
        genInfo.value = item.info || null
        sourceImage.value = generatedImage.value
        sourceGenInfo.value = item.info || null
      }
    } catch {}
  }

  try {
    const queue = await api.getQueue()
    const activeJobs = (queue || []).filter(j => (j.type === 'txt2img' || j.type === 'compound') && (j.status === 'pending' || j.status === 'running'))
    if (activeJobs.length > 0) {
      generatingImage.value = true
      generationStage.value = 'image'
      enqueuedJobIds.value = new Set(activeJobs.map(j => j.id))
      batchTotal.value = activeJobs.length
      batchDone.value = 0
    }
  } catch {}

  offCompleted = EventsOn('queue:completed', onQueueCompleted)
  offFailed = EventsOn('queue:failed', onQueueFailed)
  offSessionAdded = EventsOn('session:added', onSessionAdded)
  offPaused = EventsOn('queue:paused', onQueuePaused)
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', onResize)
})

onUnmounted(() => {
  ideasBatchStopped = true
  offCompleted()
  offFailed()
  offSessionAdded()
  offPaused()
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', onResize)
  saveGenState()
  if (shared) {
    shared.selectedPresetId = selectedPresetId.value
    shared.selectedCompoundPresetId = selectedCompoundPresetId.value
    shared.genMode = genMode.value === 'custom' ? 'preset' : genMode.value
    shared.description = description.value
    shared.negative = negative.value
    shared.selectedResolutionId = selectedResolutionId.value
    shared.selectedHiresProfileId = selectedHiresProfileId.value
  }
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
          <div v-if="generatingImage && !generatedImage" style="text-align: center; padding: 24px;">
            <img v-if="preview && sdProgress && sdProgress.progress > 0 && sdProgress.progress < 1" :src="preview" alt="preview" style="max-width: 100%; border-radius: var(--radius-sm); opacity: 0.6; image-rendering: pixelated;" />
            <span v-else class="spinner" style="width: 32px; height: 32px; border-width: 3px;"></span>
            <p style="margin-top: 12px; color: var(--text-dim);">{{ batchTotal > 1 ? (batchDone + 1) + ' / ' + batchTotal + ' — ' : '' }}{{ llmStatus === 'thinking' ? t('generate.generating_prompt') : upscalingX2 ? t('generate.upscaling_x2') : t('generate.generating_image') }}</p>
            <div v-if="sdProgress && sdProgress.progress > 0" style="margin-top: 12px; max-width: 300px; margin-left: auto; margin-right: auto;">
              <div style="display: flex; justify-content: space-between; margin-bottom: 4px;">
                <span style="color: var(--text-dim); font-size: 12px;">{{ Math.round(sdProgress.progress * 100) }}%</span>
                <span style="color: var(--text-dim); font-size: 12px;">{{ t('progress.sd_step', { current: Math.round(sdProgress.progress * sdProgress.steps), total: sdProgress.steps }) }}
                  <span v-if="sdProgress.etaRelative > 0"> — ~{{ Math.ceil(sdProgress.etaRelative) }}s</span>
                </span>
              </div>
              <div style="background: var(--surface-2); border-radius: 4px; overflow: hidden; height: 6px;">
                <div :style="{ width: (sdProgress.progress * 100) + '%', background: 'var(--accent)', height: '100%', transition: 'width 0.3s' }"></div>
              </div>
              <button class="btn btn-sm btn-secondary" @click="interruptBatch" style="margin-top: 8px; font-size: 11px;">{{ t('progress.btn_interrupt') }}</button>
            </div>
          </div>
          <div v-else-if="generatingImage && generatedImage && enqueuedJobIds.size > 0" style="width: 100%; padding: 12px; position: relative;">
            <img :src="imageSrc" alt="Generating..." style="border-radius: var(--radius-sm); opacity: 0.4; width: 100%;" />
            <div style="position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%); text-align: center; background: rgba(0,0,0,0.6); padding: 16px 24px; border-radius: 8px;">
              <img v-if="preview && sdProgress && sdProgress.progress > 0 && sdProgress.progress < 1" :src="preview" alt="preview" style="max-width: 200px; border-radius: var(--radius-sm); opacity: 0.8; image-rendering: pixelated;" />
              <span v-else class="spinner" style="width: 24px; height: 24px; border-width: 2px;"></span>
              <p style="margin-top: 8px; color: var(--text-dim); font-size: 13px;">{{ (batchDone + 1) + ' / ' + batchTotal }}</p>
              <div v-if="sdProgress && sdProgress.progress > 0" style="margin-top: 8px; min-width: 180px;">
                <div style="display: flex; justify-content: space-between; margin-bottom: 4px;">
                  <span style="color: var(--text-dim); font-size: 11px;">{{ Math.round(sdProgress.progress * 100) }}%</span>
                  <span style="color: var(--text-dim); font-size: 11px;">
                    <span v-if="sdProgress.etaRelative > 0">~{{ Math.ceil(sdProgress.etaRelative) }}s</span>
                  </span>
                </div>
                <div style="background: var(--surface-2); border-radius: 4px; overflow: hidden; height: 4px;">
                  <div :style="{ width: (sdProgress.progress * 100) + '%', background: 'var(--accent)', height: '100%', transition: 'width 0.3s' }"></div>
                </div>
                <button class="btn btn-sm btn-secondary" @click="interruptBatch" style="margin-top: 8px; font-size: 11px;">{{ t('progress.btn_interrupt') }}</button>
              </div>
            </div>
          </div>
          <div v-else-if="generatedImage" class="gen-result">
            <StatusMessage v-if="hiresSkipped" kind="warning" style="margin-bottom: 8px; text-align: center;" @close="hiresSkipped = false">{{ t('generate.hires_skipped') }}</StatusMessage>
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
              <button v-if="generatedImage" class="btn btn-primary btn-sm" @click="openFastSaveModal">{{ t('generate.btn_quick_save') }}</button>
              <button v-if="generatedImage" class="btn btn-sm btn-secondary" @click="upscaleImageX2" :disabled="upscalingX2">{{ t('generate.btn_upscale_x2') }}</button>
              <button class="btn btn-sm btn-secondary" @click="downloadImage">{{ t('generate.btn_download') }}</button>
              <button class="btn btn-sm btn-secondary" @click="copyPrompt">{{ t('generate.btn_copy') }}</button>
              <button class="btn btn-sm btn-secondary" @click="generateImage">{{ t('generate.btn_regenerate') }}</button>
              <button class="btn btn-sm btn-secondary" @click="toPostProcess" :disabled="!generatedImage">{{ t('post.btn_send') }}</button>
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
          <pre style="white-space: pre-wrap; word-break: break-word; overflow-wrap: break-word;">{{ formattedGenInfo }}</pre>
        </details>

        <details v-if="effectivePrompt" class="gen-info card">
          <summary>{{ t('generate.final_prompt') }}</summary>
          <div style="margin-bottom: 8px;">
            <div style="color: var(--text-dim); font-size: 11px; margin-bottom: 4px;">{{ t('generate.positive') }}</div>
            <div style="font-size: 12px; line-height: 1.5; word-break: break-word;">{{ effectivePrompt }}</div>
          </div>
          <div v-if="effectiveNegative" style="margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--border);">
            <div style="color: var(--text-dim); font-size: 11px; margin-bottom: 4px;">{{ t('generate.negative') }}</div>
            <div style="font-size: 12px; line-height: 1.5; word-break: break-word;">{{ effectiveNegative }}</div>
          </div>
        </details>
      </div>

      <div class="gen-controls">
        <div class="card" :class="{ collapsed: openSection !== 'main' }">
          <div class="card-header card-toggle" @click="setOpenSection('main')">
            <span class="card-heading"><span class="card-num mono">01</span><span class="card-heading-text"><span class="card-title">{{ t('gen.section_main') }}</span><span class="card-subtitle">{{ t('gen.sub_main') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'main' }">
            <div class="card-body">
              <div class="form-group" style="display: flex; gap: 8px;">
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
                <div style="display: flex; justify-content: flex-end; margin-top: 8px;">
                  <button class="btn btn-sm btn-secondary" :disabled="!customValid" @click="openSavePipelineModal">{{ t('generate.save_pipeline') }}</button>
                </div>
              </div>

              <div class="form-group">
                <label class="form-label">{{ t('generate.label_description') }}</label>
                <textarea class="form-textarea" v-model="description" rows="4" :placeholder="t('generate.placeholder_description')"></textarea>
                <div style="display: flex; gap: 8px; margin-top: 6px;">
                  <button class="btn btn-sm btn-secondary" @click="saveDescription" :disabled="!description.trim()">{{ t('generate.btn_save') }}</button>
                  <button class="btn btn-sm btn-secondary" @click="showSavedDescs = !showSavedDescs">
                    {{ t('generate.saved_ideas') }} {{ savedDescs.length ? '(' + savedDescs.length + ')' : '' }}
                  </button>
                </div>
              </div>

              <div style="display: flex; gap: 8px; width: 100%;">
                <button class="btn btn-primary btn-generate" :class="{ 'btn-generating': generatingImage }" style="justify-content: center;" @click="generateImage" :disabled="generateDisabled">
                  <span v-if="generatingImage" style="display: inline-flex; align-items: center; gap: 6px;">
                    <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
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
                <div style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">{{ genMode === 'custom' ? t('generate.seed_hint_custom') : t('generate.seed_hint') }}</div>
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
              <button class="btn btn-secondary" style="width: 100%; justify-content: center;" @click="sendToSD" :disabled="generateDisabled || !extraPrompt">
                {{ t('generate.btn_generate_directly') }}
              </button>
            </div>
          </div>
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

    <div v-if="showFastSaveModal" class="modal-overlay">
      <div class="modal-content" style="max-width: 400px;">
        <h3 style="margin: 0 0 12px;">{{ t('generate.quick_save_title') }}</h3>
        <div class="form-group">
          <label class="form-label">{{ t('generate.quick_save_filename') }}</label>
          <input class="form-input" v-model="fastSaveFilename" @keydown.enter="confirmFastSave" :disabled="fastSaveLoading" autofocus />
        </div>
        <div class="form-group" style="margin-top: 8px;">
          <label class="form-label">{{ t('generate.quick_save_format') }}</label>
          <div style="display: flex; gap: 8px;">
            <button class="btn btn-sm" :class="fastSaveFormat === 'jpg' ? 'btn-primary' : 'btn-secondary'" @click="fastSaveFormat = 'jpg'">JPG</button>
            <button class="btn btn-sm" :class="fastSaveFormat === 'png' ? 'btn-primary' : 'btn-secondary'" @click="fastSaveFormat = 'png'">PNG</button>
          </div>
        </div>
        <div style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 12px;">
          <button class="btn btn-secondary" @click="showFastSaveModal = false" :disabled="fastSaveLoading">{{ t('generate.quick_save_cancel') }}</button>
          <button class="btn btn-primary" @click="confirmFastSave" :disabled="!fastSaveFilename.trim() || fastSaveLoading">
            <span v-if="fastSaveLoading" class="spinner" style="width: 14px; height: 14px; border-width: 2px; display: inline-block; vertical-align: middle; margin-right: 4px;"></span>
            {{ t('generate.quick_save_btn') }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="showSavePipelineModal" class="modal-overlay">
      <div class="modal-content" style="max-width: 400px;">
        <h3 style="margin: 0 0 12px;">{{ t('generate.save_pipeline_title') }}</h3>
        <div class="form-group">
          <label class="form-label">{{ t('generate.save_pipeline_name') }}</label>
          <input class="form-input" v-model="pipelineName" :placeholder="t('generate.save_pipeline_placeholder_name')" @keydown.enter="confirmSavePipeline" :disabled="savingPipeline" autofocus />
        </div>
        <div class="form-group" style="margin-top: 8px;">
          <label class="form-label">{{ t('generate.save_pipeline_description') }}</label>
          <input class="form-input" v-model="pipelineDescription" :placeholder="t('generate.save_pipeline_placeholder_description')" :disabled="savingPipeline" />
        </div>
        <div style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 12px;">
          <button class="btn btn-secondary" @click="showSavePipelineModal = false" :disabled="savingPipeline">{{ t('compound.btn_cancel') }}</button>
          <button class="btn btn-primary" @click="confirmSavePipeline" :disabled="!pipelineName.trim() || savingPipeline">
            <span v-if="savingPipeline" class="spinner" style="width: 14px; height: 14px; border-width: 2px; display: inline-block; vertical-align: middle; margin-right: 4px;"></span>
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

.gen-controls > .card {
  flex-shrink: 0;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: 0;
  box-shadow: none;
}

.gen-controls > .card + .card {
  border-top: 1px solid var(--border);
}

.gen-controls .card-header {
  padding: 9px 12px;
  border-bottom: none;
}

.gen-controls > .card.collapsed .card-header:hover {
  background: var(--surface-2);
}

.gen-controls > .card:not(.collapsed) {
  border-color: var(--accent);
}

.gen-controls > .card:not(.collapsed) .card-num {
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
</style>
<style scoped>
.gen-controls .btn-generate {
  padding: 8px 16px;
  font-size: 13px;
  font-weight: 600;
}
</style>
<style scoped>
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
  padding: 0 10px 10px;
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
</style>
