<script setup>
import StatusMessage from './StatusMessage.vue'
import GenProgress from './GenProgress.vue'
import { usePostEditor, editorFamily } from '../composables/usePostEditor.js'
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { listAvailablePresets } from '../composables/usePresets.js'
import { parseSeedInput } from '../seed.js'
import { b64Url } from '../imageDataUrl.js'
import { t } from '../i18n/index.js'
import { readImageFile } from '../imageFile.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import ResolutionSelector from './ResolutionSelector.vue'
import RecipeComposition from './RecipeComposition.vue'

const props = defineProps({
  source: { type: String, default: '' },
  lookBusy: { type: Boolean, default: false },
  lookResultPrompt: { type: String, default: '' },
  batch: { type: Boolean, default: false },
})

const emit = defineEmits(['result', 'apply-look', 'apply-edit', 'apply-recipe', 'save-recipe', 'busy'])

const editLookMode = ref(false)
const lookWay = ref('effects')
const lookText = ref('')
const lookChangeScene = ref(false)
const LOOK_STRENGTH_MIN = 0.35
const LOOK_STRENGTH_MAX = 1
const LOOK_STRENGTH_DEFAULT = LOOK_STRENGTH_MAX
const LOOK_STRENGTH_STEP = 0.01
const lookStrength = ref(LOOK_STRENGTH_DEFAULT)
const lookCompiling = ref(false)
const lookRecipe = ref(null)
const lookRecipeName = ref('')

const referenceImage = ref('')
const describingGarment = ref(false)
const isRefDragOver = ref(false)
const instruction = ref('')
const seedInput = ref('')
const editor = usePostEditor()
const editModel = editor.model
const allPresets = ref([])
const stylesLoadError = ref(false)
const stylePresetId = ref(0)
const styleTags = ref(false)
const qualityMode = ref(false)
const selectedResolutionId = ref(null)
const generatingImage = ref(false)
watch(generatingImage, v => emit('busy', v))
const effectivePrompt = ref('')
const qualityUpscaleSkipped = ref(false)
const qualityRedenoiseSkipped = ref(false)
const error = ref('')
const { sdProgress, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress()

const stylePresets = computed(() => (Array.isArray(allPresets.value) ? allPresets.value : []).filter(p => editorFamily(p.model_name) === editor.family.value && !String(p.preset_type || '').startsWith('post')))
watch(editModel, () => { stylePresetId.value = 0 })

const selectedStyle = computed(() => stylePresets.value.find((p) => p.id === stylePresetId.value) || null)
watch(stylePresets, list => {
  if (stylePresetId.value && !list.some(p => p.id === stylePresetId.value)) stylePresetId.value = 0
})

const canGenerate = computed(() => !!props.source && !!instruction.value.trim() && !generatingImage.value && editor.ready.value && !(props.batch && props.lookBusy))

async function describeGarment() {
  if (!referenceImage.value || describingGarment.value) return
  describingGarment.value = true
  error.value = ''
  try {
    const text = await api.analyzeImage(referenceImage.value, 'garment')
    if (text) {
      instruction.value = t('edit.tryon_prefix') + text.trim()
    }
  } catch (e) {
    error.value = t('edit.tryon_describe_error') + ': ' + String(e)
  } finally {
    describingGarment.value = false
  }
}

function setReference(base64) {
  referenceImage.value = base64
  error.value = ''
}

const canCompileLook = computed(() => !!lookText.value.trim() && !lookCompiling.value)
const canApplyLookFree = computed(() => editor.ready.value && !!lookText.value.trim() && !props.lookBusy)

function applyLookFree() {
  if (!canApplyLookFree.value) return
  emit('apply-look', { text: lookText.value.trim(), strength: lookStrength.value, ...(lookChangeScene.value ? { change_scene: true } : {}) })
}

async function compileLook() {
  const text = lookText.value.trim()
  if (!text || lookCompiling.value) return
  lookCompiling.value = true
  error.value = ''
  try {
    const recipe = await api.compilePostStyle(text)
    lookRecipe.value = { steps: recipe.steps || [], adjust: recipe.adjust || {} }
    lookRecipeName.value = text.slice(0, 60)
  } catch (e) {
    error.value = String(e)
  } finally {
    lookCompiling.value = false
  }
}

function applyLookRecipe() {
  if (!lookRecipe.value) return
  emit('apply-recipe', { steps: lookRecipe.value.steps, adjust: lookRecipe.value.adjust })
}

function saveLookRecipe() {
  if (!lookRecipe.value) return
  const name = lookRecipeName.value.trim()
  if (!name) return
  emit('save-recipe', { name, steps: lookRecipe.value.steps, adjust: lookRecipe.value.adjust })
}

async function uploadReference() {
  try {
    const base64 = await api.readImageFile()
    if (base64) setReference(base64)
  } catch (e) {
    error.value = String(e)
  }
}

async function pasteReference() {
  try {
    const base64 = await api.readClipboardImage()
    if (base64) setReference(base64)
  } catch (e) {
    error.value = String(e)
  }
}

function onRefDrop(e) {
  isRefDragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  if (!file) return
  readImageFile(file)
    .then(setReference)
    .catch(err => { error.value = err.message })
}

function clearReference() {
  referenceImage.value = ''
}

let genReqId = 0

function editParams(instr) {
  const params = {
    instruction: instr,
    seed: parseSeedInput(seedInput.value),
    editor: editor.snapshot(),
  }
  if (referenceImage.value) params.reference_image_base64 = referenceImage.value
  if (stylePresetId.value) {
    params.style_preset_id = stylePresetId.value
    if (styleTags.value) params.style_tags = true
  }
  if (qualityMode.value && selectedResolutionId.value) {
    params.output_mode = 'quality'
    params.resolution_id = selectedResolutionId.value
  }
  return params
}

async function generate() {
  if (generatingImage.value || !editor.ready.value) return
  if (!props.source) {
    error.value = t('edit.error_no_image')
    return
  }
  const instr = instruction.value.trim()
  if (!instr) {
    error.value = t('edit.error_no_instruction')
    return
  }
  if (props.batch) {
    emit('apply-edit', editParams(instr))
    return
  }
  generatingImage.value = true
  effectivePrompt.value = ''
  qualityUpscaleSkipped.value = false
  qualityRedenoiseSkipped.value = false
  error.value = ''
  resetProgress()
  const reqId = ++genReqId
  try {
    const result = await api.editImage({ ...editParams(instr), image_base64: props.source })
    if (reqId !== genReqId) return
    if (result?.image) {
      effectivePrompt.value = result.effective_prompt || ''
      qualityUpscaleSkipped.value = !!result.quality_upscale_skipped
      qualityRedenoiseSkipped.value = !!result.quality_redenoise_skipped
      emit('result', result.image)
    }
  } catch (e) {
    if (reqId === genReqId) error.value = String(e)
  } finally {
    if (reqId === genReqId) generatingImage.value = false
  }
}

function copyEffectivePrompt() {
  if (effectivePrompt.value) navigator.clipboard.writeText(effectivePrompt.value).catch(e => { error.value = String(e) })
}

let settingsRestored = false

onMounted(() => {
  const presetsReq = listAvailablePresets(list => {
    allPresets.value = list || []
  }).catch(() => {
    stylesLoadError.value = true
  })
  api.getSettings().then(async s => {
    if (s?.edit_instruction) instruction.value = s.edit_instruction
    if (s?.edit_quality_mode) qualityMode.value = s.edit_quality_mode === 'true'
    if (s?.edit_style_tags) styleTags.value = s.edit_style_tags === 'true'
    if (s?.edit_look_mode) lookWay.value = s.edit_look_mode === 'free' ? 'free' : 'effects'
    if (s?.edit_resolution_id) {
      const rid = Number(s.edit_resolution_id)
      if (Number.isFinite(rid) && rid > 0) selectedResolutionId.value = rid
    }
    if (s?.edit_style_preset_id) {
      const spid = Number(s.edit_style_preset_id)
      if (Number.isFinite(spid) && spid > 0) {
        await presetsReq.catch(e => console.warn('EditSection.onMounted:', e))
        if (stylePresets.value.find(p => p.id === spid)) stylePresetId.value = spid
      }
    }
    settingsRestored = true
  }).catch(e => console.warn('EditSection.onMounted:', e))
})

onUnmounted(() => {
  if (!settingsRestored) return
  api.updateSettings({
    edit_instruction: instruction.value,
    edit_quality_mode: qualityMode.value ? 'true' : 'false',
    edit_resolution_id: selectedResolutionId.value ? String(selectedResolutionId.value) : '',
    edit_style_preset_id: stylePresetId.value ? String(stylePresetId.value) : '',
    edit_style_tags: styleTags.value ? 'true' : 'false',
    edit_look_mode: lookWay.value,
  }).catch(e => console.warn('EditSection.onUnmounted:', e))
})
</script>

<template>
  <div>
    <StatusMessage v-if="error === 'interrupted'" kind="warning" :text="t('progress.interrupted')" @close="error = ''" />
    <StatusMessage v-else-if="error" :text="error" @close="error = ''" />

    <div class="edit-mode-row">
      <button type="button" class="btn btn-sm" :class="editLookMode ? 'btn-secondary' : 'btn-primary'" @click="editLookMode = false">{{ t('edit.mode_instruction') }}</button>
      <button type="button" class="btn btn-sm" :class="editLookMode ? 'btn-primary' : 'btn-secondary'" @click="editLookMode = true">{{ t('edit.mode_look') }}</button>
    </div>

    <template v-if="editLookMode">
      <div class="edit-mode-row">
        <button type="button" class="btn btn-sm" :class="lookWay === 'effects' ? 'btn-primary' : 'btn-secondary'" @click="lookWay = 'effects'">{{ t('edit.look_mode_effects') }}</button>
        <button type="button" class="btn btn-sm" :class="lookWay === 'free' ? 'btn-primary' : 'btn-secondary'" @click="lookWay = 'free'">{{ t('edit.look_mode_free') }}</button>
      </div>
      <div v-if="lookWay === 'free'" class="form-group">
        <label class="form-label">
          <input data-testid="edit-look-change-scene" class="accent-check" type="checkbox" v-model="lookChangeScene" :disabled="lookBusy" />
          {{ t('edit.look_change_scene') }}
        </label>
      </div>
      <div class="field-hint look-way-hint">{{ t(lookWay === 'free' ? lookChangeScene ? 'edit.look_change_scene_hint' : 'edit.look_mode_free_hint' : 'edit.look_mode_effects_hint') }}</div>
      <div class="form-group">
        <label class="form-label">{{ t(lookWay === 'free' && lookChangeScene ? 'edit.look_scene_label' : 'edit.look_label') }}</label>
        <textarea data-testid="edit-look-text" class="form-textarea" v-model="lookText" rows="4" maxlength="2000" :placeholder="t(lookWay === 'free' && lookChangeScene ? 'edit.look_scene_placeholder' : 'edit.look_placeholder')"></textarea>
      </div>
      <template v-if="lookWay === 'effects'">
        <button class="btn btn-primary btn-generate" :class="{ 'btn-generating': lookCompiling }" :disabled="!canCompileLook" @click="compileLook">
          <span class="btn-generate-inner">
            <span v-if="lookCompiling" class="spinner spinner-sm"></span>
            <span>{{ t('edit.look_compile') }}</span>
          </span>
        </button>
        <RecipeComposition v-if="lookRecipe" :steps="lookRecipe.steps" :adjust="lookRecipe.adjust" class="look-composition">
          <button class="btn btn-sm btn-primary" :disabled="!source" @click="applyLookRecipe">{{ t('edit.look_apply') }}</button>
          <button class="btn btn-sm btn-secondary" :disabled="!lookRecipeName.trim()" @click="saveLookRecipe">{{ t('edit.look_save') }}</button>
          <input class="form-input look-name-input" type="text" v-model="lookRecipeName" :placeholder="t('edit.look_name_placeholder')" />
        </RecipeComposition>
      </template>
      <template v-else>
        <div class="form-group">
          <label class="form-label">{{ t('edit.look_strength', { value: lookStrength.toFixed(2) }) }}</label>
          <input data-testid="edit-look-strength" type="range" class="form-range" v-model.number="lookStrength" :min="LOOK_STRENGTH_MIN" :max="LOOK_STRENGTH_MAX" :step="LOOK_STRENGTH_STEP" />
        </div>
        <button class="btn btn-primary btn-generate" :class="{ 'btn-generating': lookBusy }" :disabled="!canApplyLookFree || !source" @click="applyLookFree">
          <span class="btn-generate-inner">
            <span v-if="lookBusy" class="spinner spinner-sm"></span>
            <span>{{ lookBusy ? t('post.applying') : t('edit.look_apply') }}</span>
          </span>
        </button>
        <div v-if="lookResultPrompt" data-testid="edit-look-result-prompt" class="look-prompt-line" :title="lookResultPrompt">
          <span class="look-prompt-label">{{ t('edit.look_prompt_label') }}</span> {{ lookResultPrompt }}
        </div>
      </template>
    </template>

    <template v-else>
    <div v-if="referenceImage" class="ref-container">
      <img :src="b64Url(referenceImage)" alt="Reference" class="ref-img" />
    </div>
    <div v-if="referenceImage" class="fi-btn-row">
      <button class="btn btn-sm btn-secondary" @click="describeGarment" :disabled="describingGarment">
        {{ describingGarment ? t('edit.tryon_describing') : t('edit.btn_describe_ref') }}
      </button>
      <button class="btn btn-sm btn-secondary" @click="clearReference">{{ t('edit.btn_clear_ref') }}</button>
    </div>
    <div v-else
      class="drop-zone drop-zone-compact"
      :class="{ 'drop-zone-active': isRefDragOver }"
      @dragover.prevent="isRefDragOver = true"
      @dragleave="isRefDragOver = false"
      @drop.prevent="onRefDrop"
      @click="uploadReference()"
    >
      <p class="drop-zone-hint">{{ t('edit.tryon_hint') }}</p>
    </div>
    <div v-if="!referenceImage" class="fi-btn-row fi-btn-row-tight">
      <button class="btn btn-sm btn-secondary btn-tiny" @click="pasteReference">{{ t('edit.btn_paste_ref') }}</button>
    </div>

    <div class="form-group">
      <label class="form-label">{{ t('edit.instruction_label') }}</label>
      <textarea data-testid="edit-instruction" class="form-textarea" v-model="instruction" rows="4" maxlength="2000" :placeholder="t('edit.instruction_placeholder')"></textarea>
      <div class="field-hint">{{ t('edit.instruction_hint') }}</div>
    </div>

    <div class="form-group">
      <label class="form-label">{{ t('edit.seed_label') }}</label>
      <input data-testid="edit-seed-input" class="form-input" type="number" v-model="seedInput" min="0" step="1" :placeholder="t('generate.seed_placeholder')" />
    </div>

    <div class="form-group">
      <label class="form-label" for="edit-style-select">{{ t('edit.style_label') }}</label>
      <select id="edit-style-select" data-testid="edit-style-select" class="form-select" v-model="stylePresetId">
        <option :value="0">{{ t('edit.style_none') }}</option>
        <option v-for="p in stylePresets" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <div v-if="stylesLoadError" class="field-hint field-hint-warn">{{ t('overrides.load_error') }}</div>
      <label v-if="selectedStyle" class="style-tags-label">
        <input data-testid="edit-style-tags" type="checkbox" v-model="styleTags" class="accent-check" />
        <span>{{ t('edit.style_tags') }}</span>
      </label>
      <div v-if="selectedStyle" class="field-hint">
        {{ t('edit.style_hint') }}
        {{ t('post_editor.style_prompt_only') }}
      </div>
    </div>

    <div class="form-group">
      <ResolutionSelector v-model="selectedResolutionId" :none-label="t('fi.resolution_original')" />
      <label class="quality-label" :style="{ cursor: selectedResolutionId ? 'pointer' : 'not-allowed', opacity: selectedResolutionId ? 1 : 0.5 }">
        <input data-testid="edit-quality-checkbox" type="checkbox" v-model="qualityMode" :disabled="!selectedResolutionId" class="accent-check" />
        <span>{{ t('edit.quality_mode') }}</span>
      </label>
      <div v-if="qualityMode && selectedResolutionId" class="field-hint">{{ t('edit.quality_hint') }}</div>
      <div v-else-if="!selectedResolutionId" class="field-hint">{{ t('edit.quality_needs_resolution') }}</div>
    </div>

    <button class="btn btn-primary btn-generate" :class="{ 'btn-generating': generatingImage }" @click="generate" :disabled="!canGenerate">
      <span v-if="generatingImage" class="btn-generate-inner">
        <span class="spinner spinner-sm"></span>
        {{ t('edit.generating') }}
      </span>
      <span v-else>{{ batch ? t('edit.btn_apply_all') : t('edit.btn_generate') }}</span>
    </button>
    <div v-if="batch" class="field-hint">{{ t('edit.batch_instruction_hint') }}</div>

    <div v-if="generatingImage" class="edit-progress">
      <div class="edit-progress-row">
        <span class="spinner spinner-sm"></span>
        <span class="edit-progress-label">{{ t('edit.generating') }}</span>
      </div>
      <GenProgress v-if="sdProgress && sdProgress.progress > 0" class="edit-progress-info" :progress="sdProgress.progress" :steps="sdProgress.steps" :eta="sdProgress.etaRelative" stats @interrupt="interruptGeneration" />
    </div>

    <StatusMessage v-if="qualityUpscaleSkipped" kind="warning" @close="qualityUpscaleSkipped = false">{{ t('fi.quality_upscale_skipped') }}</StatusMessage>
    <StatusMessage v-if="qualityRedenoiseSkipped" kind="warning" @close="qualityRedenoiseSkipped = false">{{ t('fi.quality_redenoise_skipped') }}</StatusMessage>
    <div v-if="effectivePrompt" data-testid="edit-effective-prompt" class="edit-effective-prompt" :title="effectivePrompt" @click="copyEffectivePrompt">{{ effectivePrompt }}</div>
    </template>
  </div>
</template>

<style scoped>
.edit-mode-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.look-composition {
  margin-top: 12px;
}
.look-way-hint {
  margin-top: -6px;
  margin-bottom: 10px;
}
.look-prompt-line {
  margin-top: 8px;
  font-size: var(--fs-xs);
  color: var(--text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.look-prompt-label {
  color: var(--accent);
}
.look-name-input {
  flex: 1;
  min-width: 120px;
  font-size: var(--fs-sm);
  padding: 4px 8px;
}
.drop-zone-compact {
  padding: 10px;
  min-height: 56px;
}
.drop-zone-hint {
  color: var(--text-dim);
  font-size: var(--fs-sm);
  margin: 0;
}
.fi-btn-row {
  display: flex;
  gap: 8px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.fi-btn-row-tight {
  margin-top: 4px;
}
.btn-tiny {
  font-size: var(--fs-xs);
}
.ref-container {
  display: flex;
  justify-content: center;
  border-radius: var(--radius-sm);
  overflow: hidden;
  background: var(--surface-2);
  border: 1px dashed var(--border);
}
.ref-img {
  max-height: 200px;
  max-width: 100%;
  display: block;
}
.field-hint {
  font-size: var(--fs-xs);
  color: var(--text-dim);
  margin-top: 4px;
}
.field-hint-warn {
  color: var(--status-warn, #e6a23c);
}
.style-tags-label {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  margin-top: 6px;
}
.style-tags-label span {
  font-size: var(--fs-xs);
}
.style-lora-list {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}
.style-lora-badge {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 1px 6px;
  font-size: var(--fs-2xs);
  color: var(--text-dim);
}
.lora-hint {
  font-size: var(--fs-xs);
  color: var(--text-dim);
}
.quality-label {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}
.quality-label span {
  font-size: var(--fs-sm);
}
.btn-generate {
  width: 100%;
  justify-content: center;
  padding: 12px;
}
.btn-generate-inner {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.edit-progress {
  margin-top: 10px;
  text-align: center;
}
.edit-progress-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  margin-top: 8px;
}
.edit-progress-label {
  color: var(--text-dim);
  font-size: var(--fs-sm);
}
.edit-progress-info {
  margin-top: 8px;
}
.edit-effective-prompt {
  margin-top: 8px;
  font-size: var(--fs-xs);
  color: var(--text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}
.edit-effective-prompt:hover {
  color: var(--text);
}
</style>
