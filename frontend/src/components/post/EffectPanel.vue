<script setup>
import { ref, computed, watch } from 'vue'
import { Sparkles, X } from 'lucide-vue-next'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { parseSeedInput } from '../../seed.js'
import { modelLoraText } from '../../modelLora.js'
import { parseLoras, normalizeLoras } from '../../loraOverride.js'
import { useServerLoras } from '../../composables/useServerLoras.js'
import { EFFECT_TYPE_GROUPS } from '../../effectTypes.js'
import { effectLabel } from '../../effectNames.js'
import { usePresets } from '../../composables/usePresets.js'
import { useMaskCanvas } from '../../composables/useMaskCanvas.js'
import { jobImage } from '../../composables/usePostBatch.js'
import FoldBlock from '../FoldBlock.vue'
import RecipeComposition from '../RecipeComposition.vue'

const props = defineProps({
  source: { type: String, default: '' },
  batch: { type: Object, required: true },
  queueJobs: { type: Object, required: true },
  status: { type: Object, required: true },
  blocked: { type: Boolean, default: false },
  appliedCount: { type: Number, default: 0 },
})

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
const effectStrength = ref(0.5)

function selectEffect(p) {
  selectedEffectId.value = p.id
  const d = Number(p.denoising_strength)
  if (Number.isFinite(d) && d > 0) effectStrength.value = Math.min(0.9, Math.max(0.35, d))
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
const { brushSize: effectBrush, hasMask: effectHasMask } = effectMask
const effectMaskOn = ref(false)
const maskActive = computed(() => effectMaskOn.value && !!props.source && !batchActive.value)
let effectMaskReset = false
watch(() => props.source, v => {
  effectMask.applySource(v, !effectMaskReset)
  effectMaskReset = false
}, { immediate: true })

function onNewSource(b64, sameAsCurrent) {
  if (sameAsCurrent) effectMask.applySource(b64, false)
  else effectMaskReset = true
}

function effectMaskParam() {
  if (!effectMaskOn.value || batchActive.value || !effectHasMask.value) return {}
  return { mask_base64: effectMask.getMaskBase64() }
}

async function onEffectJobDone(jobId, outcome, entry) {
  if (!effectJobIds.value.has(jobId)) return
  const b64 = await jobImage(outcome)
  if (!effectJobIds.value.has(jobId)) return
  effectJobIds.value.delete(jobId)
  if (b64) {
    emit('applied', b64, entry)
  } else if (outcome.error) {
    error.value = outcome.error
  }
  if (effectJobIds.value.size === 0) setApplying(false)
}

const EFFECT_LORA_WEIGHT = { defaultWeight: 1, min: -Infinity, max: Infinity }
const effectLoras = ref([])
const { loras: serverLoras, load: loadServerLoras } = useServerLoras()
const effectLoraChoices = computed(() => serverLoras.value.map(l => l.name).filter(Boolean))

function cleanLoras(list) {
  return normalizeLoras(list, EFFECT_LORA_WEIGHT)
}

function parseLoraList(raw) {
  return parseLoras(raw, EFFECT_LORA_WEIGHT)
}

async function loadEffectModel() {
  try {
    const s = await api.getSettings()
    effectLoras.value = parseLoraList(s?.effect_loras_klein)
  } catch (e) {
    console.warn('effect model settings:', e)
  }
}

const effectModelSummary = computed(() => modelLoraText('Klein', effectLoras.value))
const selectedEffectNonKlein = computed(() => {
  const p = presets.value.find(x => x.id === selectedEffectId.value)
  return !!(p?.model_name && !/klein/i.test(p.model_name))
})

function onEffectModelToggle(open) {
  if (open) loadServerLoras()
}

function saveEffectSetting(data) {
  api.updateSettings(data).catch(e => console.warn('save effect model:', e))
}

function saveEffectLoras() {
  saveEffectSetting({ effect_loras_klein: JSON.stringify(cleanLoras(effectLoras.value)) })
}

function addEffectLora() {
  effectLoras.value.push({ name: effectLoraChoices.value[0] || '', weight: 1 })
  saveEffectLoras()
}

function removeEffectLora(idx) {
  effectLoras.value.splice(idx, 1)
  saveEffectLoras()
}

function effectOverrides(presetId) {
  const preset = presets.value.find(p => p.id === presetId)
  if (preset?.model_name && !/klein/i.test(preset.model_name)) return null
  if (!effectLoras.value.some(l => l.name)) return null
  return { loras: JSON.stringify([...parseLoraList(preset?.loras), ...cleanLoras(effectLoras.value)]) }
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
    overrides: effectOverrides(selectedEffectId.value),
    preserve_identity: true,
    seed: seed ?? null,
    keep_color: effectIsMaterial.value && effectKeepColor.value,
  }
}

async function applyEffect() {
  if (applyingEffect.value || batchBusy.value || props.blocked) return
  if (!props.source) {
    error.value = t('export.error_no_image')
    return
  }
  if (!selectedEffectId.value) {
    error.value = t('post.error_no_effect')
    return
  }
  if (batchActive.value) {
    const seedVal = parseSeedInput(effectSeedInput.value)
    const presetId = selectedEffectId.value
    const strength = effectStrength.value
    const overrides = effectOverrides(presetId)
    await props.batch.runBatchStep(async (b64, i) => {
      const seed = seedVal !== null ? Math.min(seedVal + i, Number.MAX_SAFE_INTEGER) : null
      const job = await props.batch.trackBatchJob(api.enqueueFromImage({ ...effectJobParams(b64, seed), preset_id: presetId, denoising_strength: strength, overrides }))
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
    const jobId = await api.enqueueFromImage({ ...effectJobParams(props.source, seedVal), ...effectMaskParam() })
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
  maskCanvas: effectMask.maskCanvas,
  maskVersion: effectMask.maskVersion,
  brushSize: effectMask.brushSize,
  beginStroke: effectMask.beginStroke,
  strokeTo: effectMask.strokeTo,
  endStroke: effectMask.endStroke,
  undoMask: effectMask.undoMask,
  redoMask: effectMask.redoMask,
  canUndoMask: effectMask.hasMask,
  canRedoMask: effectMask.canRedoMask,
  onNewSource,
  load: loadEffectModel,
})
</script>

<template>
  <div class="adj-actions">
    <button
      class="btn btn-sm btn-secondary"
      :disabled="!source || autoStyleBusy"
      @click="runAutoStyle"
    >
      <span v-if="autoStyleBusy" class="spinner" style="width: 11px; height: 11px; border-width: 2px;"></span>
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
      <div class="form-hint">{{ t('post.effect_mask_hint') }}</div>
      <label class="form-label">{{ t('fi.label_brush_size', { value: effectBrush }) }}</label>
      <div class="fx-mask-row">
        <input type="range" class="form-range" v-model.number="effectBrush" min="5" max="150" step="1" />
        <button class="btn btn-sm btn-secondary" :disabled="!effectHasMask" @click="effectMask.clearMask">{{ t('fi.btn_clear_mask') }}</button>
      </div>
    </template>
  </div>

  <FoldBlock v-if="!selectedEffectNonKlein" class="fx-model" :title="t('model.block_title')" :summary="effectModelSummary" @toggle="onEffectModelToggle">
    <div class="fx-model-body">
      <div class="form-hint">{{ t('model.hint_klein') }}</div>
      <div v-for="(l, i) in effectLoras" :key="i" class="fx-lora-row">
        <select class="form-select fx-lora-name" v-model="l.name" @change="saveEffectLoras">
          <option v-if="l.name && !effectLoraChoices.includes(l.name)" :value="l.name">{{ l.name }}</option>
          <option v-for="n in effectLoraChoices" :key="n" :value="n">{{ n }}</option>
        </select>
        <input class="form-input fx-lora-weight" type="number" v-model.number="l.weight" min="-2" max="2" step="0.05" @change="saveEffectLoras" />
        <button type="button" class="btn btn-sm btn-danger-quiet fx-lora-remove" @click="removeEffectLora(i)"><X :size="14" /></button>
      </div>
      <button type="button" class="btn btn-sm btn-secondary fx-lora-add" @click="addEffectLora">{{ t('post.effect_add_lora') }}</button>
    </div>
  </FoldBlock>

  <div class="form-group">
    <label class="form-label">{{ t('fi.seed_label') }}</label>
    <input class="form-input" type="number" v-model="effectSeedInput" min="0" step="1" :placeholder="t('fi.seed_placeholder')" />
  </div>

  <div class="fx-apply-bar">
  <div class="form-group">
    <label class="form-label">{{ t('post.effect_strength', { value: effectStrength.toFixed(2) }) }}</label>
    <input type="range" class="form-range" v-model.number="effectStrength" min="0.35" max="0.9" step="0.01" />
  </div>

  <button
    v-if="!applyingEffect && !batchBusy"
    class="btn btn-primary btn-block"
    :disabled="!source || !selectedEffectId || batchBusy || batchExporting || blocked"
    @click="applyEffect"
  >
    {{ batchActive ? t('post.btn_apply_effect_all', { n: batchState.items.length }) : t('post.btn_apply_effect') }}
  </button>
  <button v-else class="btn btn-primary btn-block btn-generating" @click="batchBusy ? batch.interruptBatch() : interruptEffect()">
    <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
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
</template>

<style scoped src="./post-shared.css"></style>

<style scoped>
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
  font-size: 12px;
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
  font-size: 12px;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fx-orig {
  max-width: 100%;
  font-size: 10px;
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
  font-size: 11px;
  color: var(--text-dim);
}
</style>
