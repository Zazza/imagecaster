<script setup>
import { X } from 'lucide-vue-next'
import StatusMessage from './StatusMessage.vue'
import GenProgress from './GenProgress.vue'
import { b64Url } from '../imageDataUrl.js'
import { describeErrorText } from '../describeToGenerate.js'
import FoldBlock from './FoldBlock.vue'
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { parseSeedInput } from '../seed.js'
import { parseLoras } from '../loraOverride.js'
import { t } from '../i18n/index.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { useKidsMode } from '../composables/useKidsMode.js'
import { useJobTracker, resultImageBase64 } from '../composables/useJobTracker.js'
import { usePresets } from '../composables/usePresets.js'
import PresetOverridesPanel from './PresetOverridesPanel.vue'
import ResolutionSelector from './ResolutionSelector.vue'
import HiresProfileSelector from './HiresProfileSelector.vue'

const props = defineProps({
  source: { type: String, default: '' },
  active: { type: Boolean, default: false },
  batch: { type: Boolean, default: false },
  batchBusy: { type: Boolean, default: false },
})

const emit = defineEmits(['result', 'source', 'busy', 'apply-batch'])

const { presets, presetTypes, compoundPresets, loadPresets } = usePresets()
watch(presets, list => {
  if (selectedPresetId.value && !list.some(p => p.id === selectedPresetId.value)) selectedPresetId.value = null
})
const selectedTypeId = ref(null)
const selectedPresetId = ref(null)
const selectedCompoundPresetId = ref(null)
const genMode = ref('preset')
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

const genInfo = ref(null)
const qualityUpscaleSkipped = ref(false)
const qualityRedenoiseSkipped = ref(false)

const analyzing = ref(false)
const analyzeElapsed = ref(0)
let analyzeTimer = null

const generatingImage = ref(false)
watch(generatingImage, v => emit('busy', v))
const { llmStatus, sdProgress, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress({ acceptJob: id => enqueuedJobIds.value.has(id) })
const generationStage = ref('')
const error = ref('')
const genCount = ref(1)
const batchTotal = ref(0)
const jobs = useJobTracker({
  onCompleted: onQueueCompleted,
  onFailed: onQueueFailed,
  onCancelled: stopIfIdle,
})
const enqueuedJobIds = jobs.pending
defineExpose({ ownsJob: id => enqueuedJobIds.value.has(id) })
const batchDone = jobs.done

const { kidsModeActive, loadKidsMode } = useKidsMode()

const presetOverrides = ref(null)
const overridesPanel = ref(null)

let lastEmittedSource = null


const filteredPresets = computed(() => {
  if (!selectedTypeId.value) return presets.value
  return presets.value.filter(p => p.type_id === selectedTypeId.value)
})

const selectedPresetLoras = computed(() => {
  const p = presets.value.find(p => p.id === selectedPresetId.value)
  return p ? parseLoras(p.loras) : []
})

const moreSummary = computed(() => {
  const parts = []
  const seed = String(seedInput.value ?? '').trim()
  if (seed !== '' && genMode.value !== 'compound') parts.push(t('remix.more_seed', { seed }))
  if (selectedResolutionId.value) parts.push(qualityMode.value && genMode.value !== 'compound' ? t('remix.more_size_quality') : t('remix.more_size'))
  if (genMode.value === 'compound' && selectedHiresProfileId.value) parts.push(t('remix.more_hires'))
  if (!kidsModeActive.value && String(extraNegativePrompt.value || '').trim()) parts.push(t('remix.more_exclude'))
  return parts.length ? parts.join(', ') : t('remix.more_default')
})

const kleinPresetSelected = computed(() => {
  const p = presets.value.find(p => p.id === selectedPresetId.value)
  return !!p?.model_name?.toLowerCase().includes('klein')
})

const progressLabel = computed(() => {
  if (llmStatus.value === 'thinking') return t('progress.llm_thinking')
  if (generationStage.value === 'analyzing') return t('fi.analyzing_image')
  return t('generate.generating_image')
})

let describeToken = 0

async function analyzeImage(fresh = true) {
  if (!props.source) return
  const my = ++describeToken
  const auto = !fresh
  analyzing.value = true
  error.value = ''
  analyzeElapsed.value = 0
  if (analyzeTimer) clearInterval(analyzeTimer)
  analyzeTimer = setInterval(() => { analyzeElapsed.value++ }, 1000)

  try {
    const result = await api.describePhoto(props.source, 'describe', fresh)
    if (my === describeToken && (!auto || !tags.value)) tags.value = result || ''
  } catch (e) {
    if (my === describeToken) error.value = t('fi.error_analysis', { error: describeErrorText(e, t) })
  } finally {
    if (my === describeToken) {
      analyzing.value = false
      if (analyzeTimer) {
        clearInterval(analyzeTimer)
        analyzeTimer = null
      }
    }
  }
}

function autoDescribe() {
  if (props.active && !props.batch && props.source && !tags.value && !analyzing.value) analyzeImage(false)
}

watch(() => props.source, (val) => {
  if (val !== lastEmittedSource) {
    describeToken++
    analyzing.value = false
    if (analyzeTimer) {
      clearInterval(analyzeTimer)
      analyzeTimer = null
    }
    tags.value = ''
    error.value = ''
  }
  autoDescribe()
})

watch(() => props.active, autoDescribe)

watch(selectedPresetId, () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

watch([genMode, kidsModeActive], () => {
  presetOverrides.value = null
  overridesPanel.value?.reset()
})

function remixParams() {
  const params = {
    mode: 'img2img',
    gen_mode: genMode.value,
    preset_id: selectedPresetId.value || 0,
    compound_preset_id: selectedCompoundPresetId.value || 0,
    denoising_strength: denoisingStrength.value,
    tags: tags.value,
    extra_negative_prompt: extraNegativePrompt.value,
    resolution_id: selectedResolutionId.value || null,
    hires_profile_id: genMode.value === 'compound' ? (selectedHiresProfileId.value || null) : null,
    overrides: genMode.value === 'preset' ? presetOverrides.value : null,
  }
  if (genMode.value === 'preset') {
    params.output_mode = (qualityMode.value && selectedResolutionId.value) ? 'quality' : ''
  }
  if (genMode.value === 'preset' && kleinPresetSelected.value && preserveIdentity.value) {
    params.preserve_identity = true
  }
  return params
}

async function generate() {
  if (generatingImage.value) return
  if (!props.source) {
    error.value = t('fi.error_upload_first')
    return
  }
  if (genMode.value === 'preset' && !selectedPresetId.value) {
    error.value = t('fi.error_select_style')
    return
  }
  if (genMode.value === 'compound' && !selectedCompoundPresetId.value) {
    error.value = t('fi.error_select_workflow')
    return
  }

  if (props.batch) {
    if (props.batchBusy) return
    emit('apply-batch', { ...remixParams(), seed: parseSeedInput(seedInput.value) })
    return
  }

  generatingImage.value = true
  generationStage.value = 'generating'
  genInfo.value = null
  qualityUpscaleSkipped.value = false
  qualityRedenoiseSkipped.value = false
  error.value = ''
  resetProgress()
  jobs.reset()
  batchTotal.value = 0
  try {
    const params = { ...remixParams(), image_base64: props.source }
    const seedVal = parseSeedInput(seedInput.value)
    const count = Math.max(1, Math.min(100, genCount.value || 1))
    batchTotal.value = count
    for (let i = 0; i < count; i++) {
      const jobId = await api.enqueueFromImage({ ...params, seed: seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null })
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

function stopIfIdle() {
  if (enqueuedJobIds.value.size === 0) {
    generatingImage.value = false
    generationStage.value = ''
  }
}

async function onQueueCompleted(data, result, tracked) {
  if (!tracked) return
  if (result?.hasImage) {
    try {
      const b64 = await resultImageBase64(result)
      if (b64) {
        lastEmittedSource = b64
        emit('result', b64)
      }
    } catch (e) {
      error.value = String(e)
    }
    genInfo.value = result.info
    qualityUpscaleSkipped.value = result.qualityUpscaleSkipped
    qualityRedenoiseSkipped.value = result.qualityRedenoiseSkipped
  }
  stopIfIdle()
}

function onQueueFailed(data, { tracked }) {
  if (!tracked) return
  if (data.error) error.value = data.error
  stopIfIdle()
}

async function interruptBatch() {
  if (enqueuedJobIds.value.size > 0) {
    for (const id of [...enqueuedJobIds.value]) {
      try {
        await api.cancelQueueJob(id)
        jobs.untrack(id)
      } catch (e) {
        let finished = false
        try {
          const job = await api.getQueueJobResult(id)
          finished = !!job && ['completed', 'failed', 'cancelled'].includes(job.status)
        } catch (readError) {
          console.warn('cancelled job status unavailable:', readError)
        }
        if (finished) jobs.untrack(id)
        else error.value = String(e)
      }
    }
    if (!enqueuedJobIds.value.size) {
      generatingImage.value = false
      generationStage.value = ''
    }
    return
  }
  await interruptGeneration()
}

async function interruptCurrentImage() {
  try {
    const id = sdProgress.value?.queue_job_id
    if (id && enqueuedJobIds.value.has(id)) await api.cancelQueueJob(id)
    else if (!enqueuedJobIds.value.size) await interruptGeneration()
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

let settingsRestored = false

onMounted(async () => {
  document.addEventListener('keydown', onKeydown)

  await loadPresets()
  loadKidsMode()
  try {
    hiresProfiles.value = await api.listHiresProfiles() || []
  } catch (e) {
    console.warn('RemixSection.onMounted:', e)
    loadHiresFailed.value = true
  }
  try {
    const s = await api.getSettings()
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
    if (s.fi_count) genCount.value = Math.max(1, Math.min(100, Number(s.fi_count) || 1))
    settingsRestored = true
  } catch (e) {
    console.warn('RemixSection.onMounted:', e)
  }
  autoDescribe()
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  saveFIState()
})

function saveFIState() {
  if (!settingsRestored) return
  const state = {
    fi_preset_id: String(selectedPresetId.value || ''),
    fi_compound_preset_id: String(selectedCompoundPresetId.value || ''),
    fi_gen_mode: genMode.value,
    fi_denoising: String(denoisingStrength.value || ''),
    fi_quality_mode: qualityMode.value ? 'true' : 'false',
    fi_preserve_identity: preserveIdentity.value ? 'true' : 'false',
    fi_extra_negative: extraNegativePrompt.value || '',
    fi_count: String(genCount.value || 1),
  }
  if (!loadHiresFailed.value) {
    state.fi_hires_profile_id = selectedHiresProfileId.value ? String(selectedHiresProfileId.value) : ''
  }
  api.updateSettings(state).catch(e => console.warn('RemixSection.saveFIState:', e))
}

function onKeydown(e) {
  if (!props.active) return
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !generatingImage.value) {
    e.preventDefault()
    generate()
  }
}

</script>

<template>
  <div>
    <div v-if="kidsModeActive" class="service-status">
      <div class="status-badge status-ok">
        &#9679; {{ t('fi.kids_mode') }}
      </div>
    </div>

    <StatusMessage v-if="error" :kind="error === 'interrupted' ? 'warning' : 'error'" :text="error" @close="error = ''" />

    <div class="form-group">
      <div class="flex-between remix-desc-head">
        <label class="form-label m-0">{{ t('remix.description_label') }}</label>
      </div>
      <div class="flex gap-2 remix-desc-head">
        <button data-testid="remix-describe-again" class="btn btn-sm btn-secondary" @click="analyzeImage(true)" :disabled="analyzing || !source">
          <template v-if="analyzing">
            <span>{{ t('fi.analyzing') }} ({{ analyzeElapsed }}s)</span>
          </template>
          <span v-else>{{ t('describe.again') }}</span>
        </button>
        <button v-if="!kidsModeActive" class="btn btn-sm btn-secondary" @click="tags = ''" :disabled="!tags" title="Clear tags"><X :size="14" /></button>
      </div>
      <div v-if="!kidsModeActive" class="remix-hint">{{ t('remix.analyze_hint') }}</div>
      <textarea class="form-textarea" v-model="tags" rows="4" :placeholder="t('fi.placeholder_tags')" :disabled="kidsModeActive"></textarea>
      <div v-if="batch" class="form-hint">{{ t('fi.batch_describe_hint') }}</div>
      <div v-if="kidsModeActive" class="remix-kids-note">
        {{ t('fi.kids_tags_restricted') }}
      </div>
    </div>

    <div class="flex gap-2 mb-3">
      <button class="btn btn-sm" :class="genMode === 'preset' ? 'btn-primary' : 'btn-secondary'" @click="genMode = 'preset'">{{ t('fi.btn_style') }}</button>
      <button class="btn btn-sm" :class="genMode === 'compound' ? 'btn-primary' : 'btn-secondary'" @click="genMode = 'compound'">{{ t('fi.btn_workflow') }}</button>
    </div>

    <div v-if="genMode === 'preset'">
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

    <div v-if="genMode === 'compound'" class="form-group">
      <label class="form-label">{{ t('fi.label_workflow') }}</label>
      <select class="form-select" v-model="selectedCompoundPresetId">
        <option :value="null" disabled>{{ t('fi.select_workflow') }}</option>
        <option v-for="c in compoundPresets" :key="c.id" :value="c.id">{{ c.name }} ({{ t('common.steps_n', { count: c.steps.length }) }})</option>
      </select>
    </div>

    <div v-if="genMode === 'preset' && kleinPresetSelected" class="form-group">
      <label class="check-label">
        <input data-testid="fi-preserve-identity-checkbox" type="checkbox" v-model="preserveIdentity" class="accent-check" />
        <span class="fs-sm">{{ t('fi.preserve_identity') }}</span>
      </label>
      <div class="field-note">{{ t('fi.preserve_identity_hint') }}</div>
    </div>

    <div class="form-group">
      <label class="form-label">{{ t('fi.label_denoising', { value: denoisingStrength.toFixed(2) }) }}</label>
      <input type="range" class="form-range" v-model.number="denoisingStrength" min="0.05" max="1.0" step="0.05" />
      <div class="flex-between fs-xs text-dim">
        <span>{{ t('fi.keep_original') }}</span>
        <span>{{ t('fi.full_redraw') }}</span>
      </div>
    </div>

    <PresetOverridesPanel :title="t('model.block_title')" v-if="genMode === 'preset' && !kidsModeActive" ref="overridesPanel" v-model="presetOverrides" :preset-loras="selectedPresetLoras" />

    <FoldBlock class="remix-more" :title="t('remix.more_title')" :summary="moreSummary">
      <div class="form-group">
        <label class="form-label" :style="{ opacity: genMode === 'compound' ? 0.5 : 1 }">{{ t('fi.seed_label') }}</label>
        <input data-testid="fi-seed-input" class="form-input" type="number" v-model="seedInput" min="0" step="1" :disabled="genMode === 'compound'" :placeholder="t('fi.seed_placeholder')" :style="{ opacity: genMode === 'compound' ? 0.5 : 1 }" />
        <div v-if="genMode === 'compound'" class="field-note field-note-accent">{{ t('fi.preset_only') }}</div>
        <div v-else class="field-note">{{ t('fi.seed_hint') }}</div>
      </div>

      <div class="form-group">
        <ResolutionSelector v-model="selectedResolutionId" :none-label="t('fi.resolution_original')" />
        <div class="remix-quality">
          <label :style="{ display: 'flex', alignItems: 'center', gap: '6px', cursor: (genMode === 'compound' || !selectedResolutionId) ? 'not-allowed' : 'pointer', opacity: (genMode === 'compound' || !selectedResolutionId) ? 0.5 : 1 }">
            <input data-testid="fi-quality-checkbox" type="checkbox" v-model="qualityMode" :disabled="genMode === 'compound' || !selectedResolutionId" class="accent-check" />
            <span class="fs-sm">{{ t('fi.quality_mode') }}</span>
          </label>
          <div v-if="genMode === 'compound'" class="field-note field-note-accent">{{ t('fi.preset_only') }}</div>
          <div v-else-if="!selectedResolutionId" class="field-note">{{ t('fi.quality_needs_resolution') }}</div>
          <div v-else class="field-note">{{ t('fi.quality_hint') }}</div>
        </div>
      </div>

      <div v-if="genMode === 'compound'" class="form-group">
        <HiresProfileSelector v-model="selectedHiresProfileId" />
        <div class="field-note">{{ t('fi.hires_hint') }}</div>
      </div>

      <div v-if="!kidsModeActive" class="form-group">
        <label class="form-label">{{ t('fi.label_extra_exclude') }}</label>
        <textarea class="form-textarea" v-model="extraNegativePrompt" rows="2" :placeholder="t('fi.placeholder_extra_exclude')"></textarea>
      </div>
    </FoldBlock>

    <div class="flex gap-2 remix-actions">
      <button class="btn btn-primary remix-generate" :class="{ 'btn-generating': generatingImage }" @click="generate" :disabled="generatingImage || (batch && batchBusy) || !source || (genMode === 'preset' ? !selectedPresetId : !selectedCompoundPresetId)">
        <span v-if="generatingImage" class="inline-center">
          <span class="spinner spinner-sm"></span>
          {{ progressLabel }}
        </span>
        <span v-else-if="batch">{{ t('fi.btn_apply_all') }}</span>
        <span v-else>{{ genCount <= 1 ? t('fi.btn_generate') : t('fi.btn_generate_n', { count: genCount }) }}</span>
      </button>
      <input v-if="!batch" class="form-input remix-count" type="number" v-model.number="genCount" min="1" max="100" @change="genCount = Math.max(1, Math.min(100, genCount || 1))" />
    </div>

    <div v-if="generatingImage" class="remix-progress">
      <div class="remix-progress-row">
        <span class="spinner spinner-sm"></span>
        <span class="remix-progress-label">{{ batchTotal > 1 ? (batchDone + 1) + ' / ' + batchTotal + ' — ' : '' }}{{ progressLabel }}</span>
      </div>
      <button v-if="batchTotal > 1 && enqueuedJobIds.size > 0" class="btn btn-sm btn-secondary" @click="interruptBatch">{{ t('progress.stop_series') }}</button>
      <GenProgress v-if="sdProgress && sdProgress.progress > 0" class="remix-progress-info" :progress="sdProgress.progress" :steps="sdProgress.steps" :eta="sdProgress.etaRelative" :preview-available="sdProgress.preview_available" stats block @interrupt="interruptCurrentImage" />
    </div>

    <StatusMessage v-if="qualityUpscaleSkipped" kind="warning" class="mt-2" @close="qualityUpscaleSkipped = false">{{ t('fi.quality_upscale_skipped') }}</StatusMessage>
    <StatusMessage v-if="qualityRedenoiseSkipped" kind="warning" class="mt-2" @close="qualityRedenoiseSkipped = false">{{ t('fi.quality_redenoise_skipped') }}</StatusMessage>

    <details v-if="genInfo" class="gen-info">
      <summary>{{ t('fi.technical_details') }}</summary>
      <pre class="pre-wrap">{{ typeof genInfo === 'string' ? genInfo : JSON.stringify(genInfo, null, 2) }}</pre>
    </details>
  </div>
</template>

<style scoped>
.remix-desc-head {
  margin-bottom: 6px;
}

.remix-gap-6 {
  gap: 6px;
}

.remix-kids-note {
  margin-top: var(--sp-1);
  padding: 6px;
  background: var(--surface-2);
  border-radius: 4px;
  text-align: center;
  font-size: var(--fs-xs);
  color: var(--text-dim);
}

.remix-quality {
  margin-top: 6px;
}

.remix-actions {
  width: 100%;
}

.remix-generate {
  flex: 1;
  justify-content: center;
  padding: var(--sp-3);
}

.remix-count {
  width: 70px;
  text-align: center;
}

.remix-hint {
  margin-bottom: 6px;
  font-size: var(--fs-xs);
  color: var(--text-dim);
}

.remix-progress {
  margin-top: 10px;
  text-align: center;
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
  font-size: var(--fs-sm);
}
.remix-progress-info {
  margin-top: 8px;
}
</style>
