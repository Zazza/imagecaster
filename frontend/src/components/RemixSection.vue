<script setup>
import StatusMessage from './StatusMessage.vue'
import FoldBlock from './FoldBlock.vue'
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { parseSeedInput } from '../seed.js'
import { parseLoras } from '../loraOverride.js'
import { t } from '../i18n/index.js'
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
const preserveIdentityResetFailed = ref(false)

const genInfo = ref(null)
const qualityUpscaleSkipped = ref(false)
const qualityRedenoiseSkipped = ref(false)

const analyzing = ref(false)
const analyzeMode = ref('quick')
const analyzeElapsed = ref(0)
let analyzeTimer = null

const generatingImage = ref(false)
watch(generatingImage, v => emit('busy', v))
const { llmStatus, sdProgress, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress()
const generationStage = ref('')
const error = ref('')
const enqueuedJobIds = ref(new Set())
const genCount = ref(1)
const batchTotal = ref(0)
const batchDone = ref(0)

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

watch(() => props.source, (val) => {
  if (val !== lastEmittedSource) {
    tags.value = ''
    error.value = ''
  }
})

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
  preserveIdentityResetFailed.value = false
  error.value = ''
  resetProgress()
  enqueuedJobIds.value = new Set()
  batchTotal.value = 0
  batchDone.value = 0
  try {
    const params = { ...remixParams(), image_base64: props.source }
    const seedVal = parseSeedInput(seedInput.value)
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
  if (enqueuedJobIds.value.size > 0) {
    const ids = [...enqueuedJobIds.value]
    enqueuedJobIds.value = new Set()
    for (const id of ids) {
      try { await api.cancelQueueJob(id) } catch {}
    }
    generatingImage.value = false
    generationStage.value = ''
    return
  }
  await interruptGeneration()
}

let offCompleted = () => {}
let offFailed = () => {}
let offPaused = () => {}
let settingsRestored = false

onMounted(async () => {
  document.addEventListener('keydown', onKeydown)
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
    if (s.fi_count) genCount.value = Math.max(1, Math.min(100, Number(s.fi_count) || 1))
    settingsRestored = true
  } catch {}
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  offCompleted()
  offFailed()
  offPaused()
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
    fi_analyze_mode: analyzeMode.value || '',
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
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px;">
        <label class="form-label" style="margin-bottom: 0;">{{ t('remix.description_label') }}</label>
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
      <div v-if="!kidsModeActive" class="remix-hint">{{ t('remix.analyze_hint') }}</div>
      <textarea class="form-textarea" v-model="tags" rows="4" :placeholder="t('fi.placeholder_tags')" :disabled="kidsModeActive"></textarea>
      <div v-if="batch" class="form-hint">{{ t('fi.batch_describe_hint') }}</div>
      <div v-if="kidsModeActive" style="margin-top: 4px; padding: 6px; background: var(--surface-2); border-radius: 4px; text-align: center; font-size: 11px; color: var(--text-dim);">
        {{ t('fi.kids_tags_restricted') }}
      </div>
    </div>

    <div style="display: flex; gap: 8px; margin-bottom: 12px;">
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
      <label style="display: flex; align-items: center; gap: 6px; cursor: pointer;">
        <input data-testid="fi-preserve-identity-checkbox" type="checkbox" v-model="preserveIdentity" style="accent-color: var(--accent);" />
        <span style="font-size: 12px;">{{ t('fi.preserve_identity') }}</span>
      </label>
      <div style="font-size: 11px; color: var(--text-dim); margin-top: 2px;">{{ t('fi.preserve_identity_hint') }}</div>
    </div>

    <div class="form-group">
      <label class="form-label">{{ t('fi.label_denoising', { value: denoisingStrength.toFixed(2) }) }}</label>
      <input type="range" class="form-range" v-model.number="denoisingStrength" min="0.05" max="1.0" step="0.05" />
      <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--text-dim);">
        <span>{{ t('fi.keep_original') }}</span>
        <span>{{ t('fi.full_redraw') }}</span>
      </div>
    </div>

    <PresetOverridesPanel :title="t('model.block_title')" v-if="genMode === 'preset' && !kidsModeActive" ref="overridesPanel" v-model="presetOverrides" :preset-loras="selectedPresetLoras" />

    <FoldBlock class="remix-more" :title="t('remix.more_title')" :summary="moreSummary">
      <div class="form-group">
        <label class="form-label" :style="{ opacity: genMode === 'compound' ? 0.5 : 1 }">{{ t('fi.seed_label') }}</label>
        <input data-testid="fi-seed-input" class="form-input" type="number" v-model="seedInput" min="0" step="1" :disabled="genMode === 'compound'" :placeholder="t('fi.seed_placeholder')" :style="{ opacity: genMode === 'compound' ? 0.5 : 1 }" />
        <div v-if="genMode === 'compound'" style="font-size: 11px; color: var(--accent); margin-top: 4px;">{{ t('fi.preset_only') }}</div>
        <div v-else style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">{{ t('fi.seed_hint') }}</div>
      </div>

      <div class="form-group">
        <ResolutionSelector v-model="selectedResolutionId" :none-label="t('fi.resolution_original')" />
        <div style="margin-top: 6px;">
          <label :style="{ display: 'flex', alignItems: 'center', gap: '6px', cursor: (genMode === 'compound' || !selectedResolutionId) ? 'not-allowed' : 'pointer', opacity: (genMode === 'compound' || !selectedResolutionId) ? 0.5 : 1 }">
            <input data-testid="fi-quality-checkbox" type="checkbox" v-model="qualityMode" :disabled="genMode === 'compound' || !selectedResolutionId" style="accent-color: var(--accent);" />
            <span style="font-size: 12px;">{{ t('fi.quality_mode') }}</span>
          </label>
          <div v-if="genMode === 'compound'" style="font-size: 11px; color: var(--accent); margin-top: 2px;">{{ t('fi.preset_only') }}</div>
          <div v-else-if="!selectedResolutionId" style="font-size: 11px; color: var(--text-dim); margin-top: 2px;">{{ t('fi.quality_needs_resolution') }}</div>
          <div v-else style="font-size: 11px; color: var(--text-dim); margin-top: 2px;">{{ t('fi.quality_hint') }}</div>
        </div>
      </div>

      <div v-if="genMode === 'compound'" class="form-group">
        <HiresProfileSelector v-model="selectedHiresProfileId" />
        <div style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">{{ t('fi.hires_hint') }}</div>
      </div>

      <div v-if="!kidsModeActive" class="form-group">
        <label class="form-label">{{ t('fi.label_extra_exclude') }}</label>
        <textarea class="form-textarea" v-model="extraNegativePrompt" rows="2" :placeholder="t('fi.placeholder_extra_exclude')"></textarea>
      </div>
    </FoldBlock>

    <div style="display: flex; gap: 8px; width: 100%;">
      <button class="btn btn-primary" :class="{ 'btn-generating': generatingImage }" style="flex: 1; justify-content: center; padding: 12px;" @click="generate" :disabled="generatingImage || (batch && batchBusy) || !source || (genMode === 'preset' ? !selectedPresetId : !selectedCompoundPresetId)">
        <span v-if="generatingImage" style="display: inline-flex; align-items: center; gap: 6px;">
          <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
          {{ progressLabel }}
        </span>
        <span v-else-if="batch">{{ t('fi.btn_apply_all') }}</span>
        <span v-else>{{ genCount <= 1 ? t('fi.btn_generate') : t('fi.btn_generate_n', { count: genCount }) }}</span>
      </button>
      <input v-if="!batch" class="form-input" type="number" v-model.number="genCount" min="1" max="100" style="width: 70px; text-align: center;" @change="genCount = Math.max(1, Math.min(100, genCount || 1))" />
    </div>

    <div v-if="generatingImage" class="remix-progress">
      <div class="remix-progress-row">
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

    <StatusMessage v-if="qualityUpscaleSkipped" kind="warning" style="margin-top: 8px;" @close="qualityUpscaleSkipped = false">{{ t('fi.quality_upscale_skipped') }}</StatusMessage>
    <StatusMessage v-if="qualityRedenoiseSkipped" kind="warning" style="margin-top: 8px;" @close="qualityRedenoiseSkipped = false">{{ t('fi.quality_redenoise_skipped') }}</StatusMessage>
    <StatusMessage v-if="preserveIdentityResetFailed" kind="warning" style="margin-top: 8px;" @close="preserveIdentityResetFailed = false">{{ t('fi.preserve_identity_reset_failed') }}</StatusMessage>

    <details v-if="genInfo" class="gen-info">
      <summary>{{ t('fi.technical_details') }}</summary>
      <pre style="white-space: pre-wrap; word-break: break-word; overflow-wrap: break-word;">{{ typeof genInfo === 'string' ? genInfo : JSON.stringify(genInfo, null, 2) }}</pre>
    </details>
  </div>
</template>

<style scoped>
.remix-hint {
  margin-bottom: 6px;
  font-size: 11px;
  color: var(--text-dim);
}

.form-range {
  width: 100%;
  accent-color: var(--accent);
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
