<script setup>
import { ref, computed, watch, toRef } from 'vue'
import { Sparkles, X } from 'lucide-vue-next'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { parseSeedInput } from '../../seed.js'
import { EFFECT_TYPE_GROUPS } from '../../effectTypes.js'
import { effectLabel } from '../../effectNames.js'
import { usePresets } from '../../composables/usePresets.js'
import { useMaskCanvas } from '../../composables/useMaskCanvas.js'
import { useObjectMask } from '../../composables/useObjectMask.js'
import { useWandMask } from '../../composables/useWandMask.js'
import { jobImage } from '../../composables/usePostBatch.js'
import { usePostEditor } from '../../composables/usePostEditor.js'
import RecipeComposition from '../RecipeComposition.vue'

const props = defineProps({
  source: { type: String, default: '' },
  batch: { type: Object, required: true },
  queueJobs: { type: Object, required: true },
  status: { type: Object, required: true },
  blocked: { type: Boolean, default: false },
  appliedCount: { type: Number, default: 0 },
})

const editor = usePostEditor()
const editorReady = editor.ready

const emit = defineEmits(['busy', 'applied', 'apply-recipe', 'clear-log'])

const { batch: batchState, batchActive, batchBusy, batchExporting } = props.batch
const { error, successMsg } = props.status
const { presets, presetTypes } = usePresets()

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

const selectedEffectId = ref(null)
watch(presets, list => {
  if (selectedEffectId.value && !list.some(p => p.id === selectedEffectId.value)) selectedEffectId.value = null
})
const EFFECT_GROUP_KEY = 'pp.effectGroup'
const effectGroupKey = ref((() => { try { return localStorage.getItem(EFFECT_GROUP_KEY) || '' } catch (e) { console.warn('PostProcessPage.effectGroupKey:', e); return '' } })())
const shownEffectGroup = computed(() => effectGroups.value.find(g => g.labelKey === effectGroupKey.value) || effectGroups.value[0] || null)
function pickEffectGroup(key) {
  effectGroupKey.value = key
  try { localStorage.setItem(EFFECT_GROUP_KEY, key) } catch (e) { console.warn('PostProcessPage.pickEffectGroup:', e) }
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
const EFFECT_STRENGTH_MIN = 0.35
const EFFECT_STRENGTH_MAX = 1
const EFFECT_STRENGTH_DEFAULT = EFFECT_STRENGTH_MAX
const EFFECT_STRENGTH_STEP = 0.01
const effectStrength = ref(EFFECT_STRENGTH_DEFAULT)

function selectEffect(p) {
  selectedEffectId.value = p.id
  const d = Number(p.denoising_strength)
  if (Number.isFinite(d) && d > 0) effectStrength.value = Math.min(EFFECT_STRENGTH_MAX, Math.max(EFFECT_STRENGTH_MIN, d))
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

function setApplying(v) {
  applyingEffect.value = v
  emit('busy', v)
}

const effectMask = useMaskCanvas()
const maskMode = ref('object')
const effectMaskOn = ref(false)
const objectMask = useObjectMask(toRef(props, 'source'))
const wandSource = computed(() => maskMode.value === 'wand' && effectMaskOn.value && !batchActive.value ? props.source : '')
const wandMask = useWandMask(wandSource)
const activeMask = computed(() => maskMode.value === 'wand' ? wandMask : maskMode.value === 'object' ? objectMask : effectMask)
const effectBrush = computed({ get: () => activeMask.value.brushSize?.value ?? effectMask.brushSize.value, set: v => { if (activeMask.value.brushSize) activeMask.value.brushSize.value = v } })
const effectHasMask = computed(() => activeMask.value.hasMask.value)
const objectBusy = objectMask.busy
const objectExclude = objectMask.exclude
const objectHasPaint = objectMask.hasPaint
const selectionBusy = computed(() => maskMode.value === 'wand' ? wandMask.busy.value : objectBusy.value)
const selectionError = computed(() => maskMode.value === 'wand' ? wandMask.error.value : objectMask.error.value)
const canClearMask = computed(() => maskMode.value === 'wand' ? wandMask.canClear.value : maskMode.value === 'object' ? objectMask.canUndoMask.value || objectBusy.value : effectMask.hasMask.value)
const objectSelectionRequired = computed(() => effectMaskOn.value && maskMode.value !== 'exact' && !batchActive.value)
const maskReady = computed(() => !objectSelectionRequired.value || (effectHasMask.value && !selectionBusy.value))
watch(maskMode, () => { effectMask.clearMask(); objectMask.clearMask(); wandMask.clearMask() })
watch(effectMaskOn, enabled => { if (!enabled) { objectMask.clearMask(); wandMask.clearMask() } })
const maskActive = computed(() => effectMaskOn.value && !!props.source && !batchActive.value)
let effectMaskReset = false
watch(() => props.source, v => {
  effectMask.applySource(v, !effectMaskReset)
  effectMaskReset = false
}, { immediate: true })

function onNewSource(b64, sameAsCurrent) {
  objectMask.clearMask()
  wandMask.clearMask()
  if (sameAsCurrent) effectMask.applySource(b64, false)
  else effectMaskReset = true
}

function effectMaskParam() {
  if (!effectMaskOn.value || batchActive.value || !effectHasMask.value) return {}
  return { mask_base64: activeMask.value.getMaskBase64(), ...(maskMode.value !== 'exact' ? { mask_strict: true } : {}) }
}

async function onEffectJobDone(jobId, outcome, entry) {
  if (!effectJobIds.value.has(jobId)) return
  const b64 = await jobImage(outcome)
  if (!effectJobIds.value.has(jobId)) return
  effectJobIds.value.delete(jobId)
  const up = entry?.upscale_4k ? outcome.result?.info?.upscale : ''
  if (b64 && up === 'skipped') {
    successMsg.value = t('post.fx4k_skipped')
  } else if (b64) {
    emit('applied', b64, entry)
    if (up === 'resized') successMsg.value = t('post.fx4k_resized')
  } else if (outcome.error) {
    error.value = outcome.error
  }
  if (effectJobIds.value.size === 0) setApplying(false)
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
    overrides: null,
    preserve_identity: true,
    seed: seed ?? null,
    keep_color: effectIsMaterial.value && effectKeepColor.value,
  }
}

async function applyEffect() {
  if (!editor.ready.value || applyingEffect.value || batchBusy.value || props.blocked || !maskReady.value) return
  if (!props.source) {
    error.value = t('export.error_no_image')
    return
  }
  if (!selectedEffectId.value) {
    error.value = t('post.error_no_effect')
    return
  }
  const snapshot = editor.snapshot()
  const baseParams = { ...effectJobParams(props.source, parseSeedInput(effectSeedInput.value)), editor: snapshot }
  if (batchActive.value) {
    const seedVal = parseSeedInput(effectSeedInput.value)
    const presetId = selectedEffectId.value
    const strength = effectStrength.value
    await props.batch.runBatchStep(async (b64, i) => {
      const seed = seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null
      const job = await props.batch.trackBatchJob(api.enqueueFromImage({ ...baseParams, image_base64: b64, seed, preset_id: presetId, denoising_strength: strength }))
      return job ? () => job.wait : ''
    })
    return
  }
  setApplying(true)
  error.value = ''
  successMsg.value = ''
  effectJobIds.value = new Set()
  try {
    const seedVal = parseSeedInput(effectSeedInput.value)
    const presetId = selectedEffectId.value
    const strength = effectStrength.value
    const jobId = await api.enqueueFromImage({ ...baseParams, ...effectMaskParam() })
    if (jobId) {
      effectJobIds.value.add(jobId)
      props.queueJobs.track(jobId).then(outcome => onEffectJobDone(jobId, outcome, { preset_id: presetId, strength }))
    } else {
      setApplying(false)
    }
  } catch (e) {
    error.value = String(e)
    setApplying(false)
  }
}

async function apply4K() {
  if (applyingEffect.value || batchBusy.value || batchExporting.value || props.blocked || !props.source) return
  if (batchActive.value) {
    await props.batch.runBatchStep(async (b64) => {
      const job = await props.batch.trackBatchJob(api.enqueuePost4K(b64))
      return job ? () => job.wait : ''
    })
    return
  }
  setApplying(true)
  error.value = ''
  successMsg.value = ''
  effectJobIds.value = new Set()
  try {
    const jobId = await api.enqueuePost4K(props.source)
    if (jobId) {
      effectJobIds.value.add(jobId)
      props.queueJobs.track(jobId).then(outcome => onEffectJobDone(jobId, outcome, { upscale_4k: true }))
    } else {
      setApplying(false)
    }
  } catch (e) {
    error.value = String(e)
    setApplying(false)
  }
}

async function interruptEffect() {
  if (effectJobIds.value.size > 0) {
    const ids = [...effectJobIds.value]
    effectJobIds.value = new Set()
    for (const id of ids) {
      props.queueJobs.untrack(id)
      try {
        await api.cancelQueueJob(id)
      } catch (e) {
        console.error('cancel queue job error:', e)
      }
    }
  }
  setApplying(false)
}

const autoStyleBusy = ref(false)
const autoStyleRecipe = ref(null)

async function runAutoStyle() {
  if (!props.source || autoStyleBusy.value) return
  autoStyleBusy.value = true
  autoStyleRecipe.value = null
  error.value = ''
  successMsg.value = ''
  try {
    const desc = await api.describeForStyle(props.source)
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
  emit('apply-recipe', { steps: JSON.stringify(r.steps), adjust: JSON.stringify(r.adjust) })
}

defineExpose({
  maskActive,
  maskCanvas: computed(() => activeMask.value.maskCanvas.value),
  maskVersion: computed(() => activeMask.value.maskVersion.value),
  brushSize: effectBrush,
  pointSelection: computed(() => maskMode.value === 'wand'),
  selectAt: (x, y, additive) => wandMask.selectAt(x, y, additive),
  beginStroke: (...args) => activeMask.value.beginStroke?.(...args),
  strokeTo: (...args) => activeMask.value.strokeTo?.(...args),
  endStroke: () => activeMask.value.endStroke?.(),
  undoMask: () => activeMask.value.undoMask(),
  redoMask: () => activeMask.value.redoMask(),
  canUndoMask: computed(() => maskMode.value === 'exact' ? effectMask.hasMask.value : activeMask.value.canUndoMask.value),
  canRedoMask: computed(() => activeMask.value.canRedoMask.value),
  onNewSource,
  load: () => {},
})
</script>

<template>
  <div class="adj-actions">
    <button
      class="btn btn-sm btn-secondary"
      :disabled="!source || autoStyleBusy"
      @click="runAutoStyle"
    >
      <span v-if="autoStyleBusy" class="spinner spinner-sm"></span>
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
      :disabled="blocked || applyingEffect || batchBusy || batchExporting"
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
    <button class="btn btn-sm" :class="effectMaskOn ? 'btn-primary' : 'btn-secondary'" :disabled="!source" @click="effectMaskOn = !effectMaskOn">{{ t('post.effect_mask_btn') }}</button>
    <div v-if="effectIsPattern && !(effectMaskOn && effectHasMask)" class="form-hint">{{ t('post.effect_pattern_mask_hint') }}</div>
    <template v-if="effectMaskOn">
      <select class="form-select" v-model="maskMode" data-testid="object-mask-mode" :aria-label="t('object_mask.mode')">
        <option value="object">{{ t('object_mask.object') }}</option>
        <option value="exact">{{ t('object_mask.exact') }}</option>
        <option value="wand">{{ t('wand_mask.mode') }}</option>
      </select>
      <div class="form-hint">{{ t(maskMode === 'wand' ? 'wand_mask.hint' : maskMode === 'object' ? 'object_mask.hint' : 'post.effect_mask_hint') }}</div>
      <template v-if="maskMode === 'object'">
        <div class="fx-mask-row">
          <button class="btn btn-sm" :class="!objectExclude ? 'btn-primary' : 'btn-secondary'" @click="objectExclude = false">{{ t('object_mask.add') }}</button>
          <button class="btn btn-sm" :class="objectExclude ? 'btn-primary' : 'btn-secondary'" @click="objectExclude = true">{{ t('object_mask.exclude') }}</button>
        </div>
        <button class="btn btn-sm btn-secondary" data-testid="object-mask-detect" :disabled="!objectHasPaint || objectBusy" @click="objectMask.detect">{{ t('object_mask.detect') }}</button>
      </template>
      <template v-if="maskMode !== 'exact'">
        <div v-if="selectionBusy" class="form-hint" role="status"><span class="spinner spinner-sm"></span> {{ t('object_mask.busy') }}</div>
        <div v-else-if="selectionError" class="form-hint" role="alert">{{ t(selectionError.startsWith('object_mask.') || selectionError.startsWith('wand_mask.') ? selectionError : 'object_mask.failed') }}<span v-if="!selectionError.startsWith('object_mask.') && !selectionError.startsWith('wand_mask.')"> {{ selectionError }}</span></div>
        <div v-else-if="effectHasMask" class="form-hint">{{ t(maskMode === 'wand' ? 'wand_mask.preview' : 'object_mask.preview') }}</div>
      </template>
      <label v-if="maskMode !== 'wand'" class="form-label">{{ t('fi.label_brush_size', { value: effectBrush }) }}</label>
      <div class="fx-mask-row">
        <input v-if="maskMode !== 'wand'" type="range" class="form-range" v-model.number="effectBrush" min="5" max="150" step="1" />
        <button class="btn btn-sm btn-secondary" :disabled="!canClearMask" @click="activeMask.clearMask">{{ t('fi.btn_clear_mask') }}</button>
      </div>
    </template>
  </div>

  <div class="form-group">
    <label class="form-label">{{ t('fi.seed_label') }}</label>
    <input class="form-input" type="number" v-model="effectSeedInput" min="0" step="1" :placeholder="t('fi.seed_placeholder')" />
  </div>

  <div class="fx-apply-bar">
  <div class="form-group">
    <label class="form-label">{{ t('post.effect_strength', { value: effectStrength.toFixed(2) }) }}</label>
    <input type="range" class="form-range" v-model.number="effectStrength" :min="EFFECT_STRENGTH_MIN" :max="EFFECT_STRENGTH_MAX" :step="EFFECT_STRENGTH_STEP" />
  </div>

  <button
    v-if="!applyingEffect && !batchBusy"
    class="btn btn-primary btn-block"
    :disabled="!editorReady || !source || !selectedEffectId || batchBusy || batchExporting || blocked || !maskReady"
    @click="applyEffect"
  >
    {{ batchActive ? t('post.btn_apply_effect_all', { n: batchState.items.length }) : t('post.btn_apply_effect') }}
  </button>
  <button v-else class="btn btn-primary btn-block btn-generating" @click="batchBusy ? batch.interruptBatch() : interruptEffect()">
    <span class="spinner spinner-sm"></span>
    {{ t('post.applying') }}
  </button>
  </div>
  <div v-if="appliedCount" class="effect-log-row">
    <span>{{ t('post.recipe_log', { n: appliedCount }) }}</span>
    <button
      class="btn-icon btn-icon-danger"
      :title="t('post.recipe_log_clear')"
      :aria-label="t('post.recipe_log_clear')"
      @click="emit('clear-log')"
    ><X :size="14" /></button>
  </div>
  <div class="form-hint">{{ t('post.effect_hint') }}</div>
  <div class="fx-4k">
    <button
      class="btn btn-secondary btn-block"
      data-testid="fx-4k"
      :disabled="!source || applyingEffect || batchBusy || batchExporting || blocked"
      @click="apply4K"
    >{{ batchActive ? t('post.fx4k_btn_all', { n: batchState.items.length }) : t('post.fx4k_btn') }}</button>
    <div class="form-hint">{{ t('post.fx4k_hint') }}</div>
  </div>
</template>

<style scoped src="./post-shared.css"></style>

<style scoped>
.fx-4k {
  margin-top: 12px;
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
  font-size: var(--fs-sm);
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
  font-size: var(--fs-sm);
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fx-orig {
  max-width: 100%;
  font-size: var(--fs-2xs);
  line-height: 1.2;
  color: var(--text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.effect-log-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 8px;
  font-size: var(--fs-xs);
  color: var(--text-dim);
}
</style>
