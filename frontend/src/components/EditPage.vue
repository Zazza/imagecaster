<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { parseSeedInput } from '../seed.js'
import { t } from '../i18n/index.js'
import { MAX_IMAGE_SIZE } from '../constants.js'
import { parseLoras, sanitizeWeight } from '../loraOverride.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import ImageViewer from './ImageViewer.vue'
import ResolutionSelector from './ResolutionSelector.vue'

const uploadedImage = ref('')
const uploadedImageMime = ref('image/png')
const instruction = ref('')
const seedInput = ref('')
const loraRows = ref([])
const serverLoras = ref([])
const lorasLoaded = ref(false)
const loraLoadError = ref(false)
const allPresets = ref([])
const stylesLoadError = ref(false)
const stylePresetId = ref(0)
const denoiseInput = ref(0.65)
const qualityMode = ref(false)
const selectedResolutionId = ref(null)
const generatingImage = ref(false)
const generatedImage = ref('')
const qualityUpscaleSkipped = ref(false)
const qualityRedenoiseSkipped = ref(false)
const error = ref('')
const isDragOver = ref(false)
const showViewer = ref(false)
const { sdProgress, preview, interrupt: interruptGeneration, reset: resetProgress } = useGenerationProgress()

const stylePresets = computed(() => {
  const list = Array.isArray(allPresets.value) ? allPresets.value : []
  return list.filter((p) => (p?.model_name || '').toLowerCase().includes('moss'))
})

const selectedStyle = computed(() => stylePresets.value.find((p) => p.id === stylePresetId.value) || null)

const selectedStyleLoras = computed(() => parseLoras(selectedStyle.value?.loras || ''))

const imageSrc = computed(() => {
  if (!generatedImage.value) return ''
  if (generatedImage.value.startsWith('/api/')) return generatedImage.value
  return 'data:image/png;base64,' + generatedImage.value
})

const canGenerate = computed(() => !!uploadedImage.value && !!instruction.value.trim() && !generatingImage.value)

function addLora() {
  if (loraRows.value.length >= 8) return
  loraRows.value.push({ name: '', weight: 0.6 })
}

function removeLora(idx) {
  loraRows.value.splice(idx, 1)
}

async function loadLoras() {
  if (lorasLoaded.value) return
  lorasLoaded.value = true
  loraLoadError.value = false
  try {
    const lor = await api.getLoRAs()
    serverLoras.value = lor || []
  } catch (e) {
    loraLoadError.value = true
    lorasLoaded.value = false
  }
}

function onLorasToggle(e) {
  if (e.target.open) loadLoras()
}

function editLorasPayload() {
  return loraRows.value
    .filter((r) => r && typeof r.name === 'string' && r.name.trim())
    .map((r) => ({ name: r.name.trim(), weight: sanitizeWeight(r.weight) }))
}

function setSource(base64, mime) {
  uploadedImage.value = base64
  uploadedImageMime.value = mime || 'image/png'
  error.value = ''
}

async function uploadImage() {
  try {
    const base64 = await api.readImageFile()
    if (base64) setSource(base64, 'image/png')
  } catch (e) {
    error.value = String(e)
  }
}

let lastImageReqId = 0

async function useLastImage() {
  const reqId = ++lastImageReqId
  try {
    const item = await api.getActiveSessionItem()
    if (reqId !== lastImageReqId) return
    if (item) {
      const b64 = await api.getSessionImage(item.id)
      if (reqId !== lastImageReqId) return
      if (b64) {
        setSource(b64, 'image/png')
      } else {
        error.value = t('fi.error_no_session_image')
      }
    } else {
      error.value = t('fi.error_no_active_item')
    }
  } catch (e) {
    if (reqId === lastImageReqId) error.value = String(e)
  }
}

async function pasteFromClipboard() {
  try {
    const base64 = await api.readClipboardImage()
    if (base64) setSource(base64, 'image/png')
  } catch (e) {
    error.value = String(e)
  }
}

function handlePaste(e) {
  const items = e.clipboardData?.items
  if (!items) return
  for (const item of items) {
    if (item.type.startsWith('image/')) {
      e.preventDefault()
      const file = item.getAsFile()
      if (!file) continue
      if (file.size > MAX_IMAGE_SIZE) {
        error.value = t('fi.error_image_too_large')
        return
      }
      const reader = new FileReader()
      reader.onload = () => {
        const base64 = reader.result.split(',')[1]
        const mime = reader.result.split(':')[1]?.split(';')[0] || 'image/png'
        if (base64) setSource(base64, mime)
      }
      reader.readAsDataURL(file)
      return
    }
  }
}

function onDrop(e) {
  isDragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    error.value = t('fi.error_drop_image')
    return
  }
  if (file.size > MAX_IMAGE_SIZE) {
    error.value = t('fi.error_image_too_large')
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    const base64 = reader.result.split(',')[1]
    const mime = reader.result.split(':')[1]?.split(';')[0] || 'image/png'
    if (base64) setSource(base64, mime)
  }
  reader.readAsDataURL(file)
}

function clearImage() {
  uploadedImage.value = ''
  uploadedImageMime.value = 'image/png'
  generatedImage.value = ''
  error.value = ''
}

let genReqId = 0

async function generate() {
  if (generatingImage.value) return
  if (!uploadedImage.value) {
    error.value = t('edit.error_no_image')
    return
  }
  const instr = instruction.value.trim()
  if (!instr) {
    error.value = t('edit.error_no_instruction')
    return
  }
  generatingImage.value = true
  generatedImage.value = ''
  qualityUpscaleSkipped.value = false
  qualityRedenoiseSkipped.value = false
  error.value = ''
  resetProgress()
  const reqId = ++genReqId
  try {
    const seedVal = parseSeedInput(seedInput.value)
    const params = {
      image_base64: uploadedImage.value,
      instruction: instr,
      seed: seedVal,
    }
    if (stylePresetId.value) params.style_preset_id = stylePresetId.value
    const dn = Number(denoiseInput.value)
    if (Number.isFinite(dn) && dn > 0) params.denoising_strength = dn
    const loras = editLorasPayload()
    if (loras.length) params.loras = loras
    if (qualityMode.value && selectedResolutionId.value) {
      params.output_mode = 'quality'
      params.resolution_id = selectedResolutionId.value
    }
    const result = await api.editImage(params)
    if (reqId !== genReqId) return
    if (result?.image) {
      generatedImage.value = result.image
      qualityUpscaleSkipped.value = !!result.quality_upscale_skipped
      qualityRedenoiseSkipped.value = !!result.quality_redenoise_skipped
    }
  } catch (e) {
    if (reqId === genReqId) error.value = String(e)
  } finally {
    if (reqId === genReqId) generatingImage.value = false
  }
}

async function downloadImage() {
  if (!generatedImage.value) return
  try {
    const defaultName = 'sd-studio-edit-' + Date.now() + '.png'
    if (generatedImage.value.startsWith('/api/')) {
      const resp = await fetch(generatedImage.value)
      const blob = await resp.blob()
      const reader = new FileReader()
      reader.onload = async () => {
        const b64 = reader.result.split(',')[1]
        await api.saveImage(b64, defaultName)
      }
      reader.readAsDataURL(blob)
    } else {
      await api.saveImage(generatedImage.value, defaultName)
    }
  } catch (e) {
    error.value = t('fi.error_save', { error: String(e) })
  }
}

function useResult() {
  if (!generatedImage.value) return
  setSource(generatedImage.value, 'image/png')
  generatedImage.value = ''
}

function onKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && canGenerate.value) {
    e.preventDefault()
    generate()
  }
}

let settingsRestored = false
let offSessionSelected = null

onMounted(() => {
  document.addEventListener('paste', handlePaste)
  document.addEventListener('keydown', onKeydown)
  offSessionSelected = EventsOn("session:selected", () => { useLastImage() })
  if (!uploadedImage.value) useLastImage()
  const presetsReq = api.listPresets().then(list => {
    allPresets.value = list || []
  }).catch(() => {
    stylesLoadError.value = true
  })
  api.getSettings().then(async s => {
    if (s?.edit_instruction) instruction.value = s.edit_instruction
    if (s?.edit_loras) loraRows.value = parseLoras(s.edit_loras)
    if (s?.edit_quality_mode) qualityMode.value = s.edit_quality_mode === 'true'
    if (s?.edit_denoise) {
      const dn = Number(s.edit_denoise)
      if (Number.isFinite(dn)) denoiseInput.value = Math.min(Math.max(dn, 0.3), 1)
    }
    if (s?.edit_style_preset_id) {
      const spid = Number(s.edit_style_preset_id)
      if (Number.isFinite(spid) && spid > 0) {
        await presetsReq.catch(() => {})
        if (stylePresets.value.find(p => p.id === spid)) stylePresetId.value = spid
      }
    }
    settingsRestored = true
  }).catch(() => {})
})

onUnmounted(() => {
  document.removeEventListener('paste', handlePaste)
  document.removeEventListener('keydown', onKeydown)
  if (offSessionSelected) offSessionSelected()
  if (!settingsRestored) return
  api.updateSettings({
    edit_instruction: instruction.value,
    edit_loras: JSON.stringify(editLorasPayload()),
    edit_quality_mode: qualityMode.value ? 'true' : 'false',
    edit_resolution_id: selectedResolutionId.value ? String(selectedResolutionId.value) : '',
    edit_style_preset_id: stylePresetId.value ? String(stylePresetId.value) : '',
    edit_denoise: String(denoiseInput.value),
  }).catch(() => {})
})
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ t('edit.title') }}</h1>
    </div>

    <div v-if="error === 'interrupted'" class="status status-warning">{{ t('progress.interrupted') }}</div>
    <div v-else-if="error" class="status status-error">{{ error }}</div>

    <div class="generate-layout">
      <div class="generate-section">
        <div class="card">
          <div
            v-if="!uploadedImage"
            class="drop-zone"
            :class="{ 'drop-zone-active': isDragOver }"
            @dragover.prevent="isDragOver = true"
            @dragleave="isDragOver = false"
            @drop.prevent="onDrop"
            @click="uploadImage()"
          >
            <div style="font-size: 32px; color: var(--text-dim);">&#128444;</div>
            <p style="color: var(--text-dim); margin-top: 8px;">{{ t('edit.drop_image') }}</p>
          </div>

          <div v-else class="source-container">
            <img :src="'data:' + uploadedImageMime + ';base64,' + uploadedImage" alt="Source" class="source-img" />
          </div>

          <div v-if="uploadedImage" class="fi-btn-row">
            <button class="btn btn-sm btn-secondary" @click="uploadImage">{{ t('edit.btn_change') }}</button>
            <button class="btn btn-sm btn-secondary" @click="useLastImage">{{ t('edit.btn_last') }}</button>
            <button class="btn btn-sm btn-secondary" @click="pasteFromClipboard">{{ t('edit.btn_paste') }}</button>
            <button class="btn btn-sm btn-secondary" @click="clearImage">{{ t('edit.btn_clear') }}</button>
          </div>
          <div v-else class="fi-btn-row" style="margin-top: 8px;">
            <button class="btn btn-sm btn-secondary" @click="useLastImage">{{ t('edit.btn_last') }}</button>
            <button class="btn btn-sm btn-secondary" @click="pasteFromClipboard">{{ t('edit.btn_paste') }}</button>
          </div>

          <div class="model-note">{{ t('edit.model_note') }}</div>

          <div class="form-group">
            <label class="form-label">{{ t('edit.instruction_label') }}</label>
            <textarea data-testid="edit-instruction" class="form-textarea" v-model="instruction" rows="4" maxlength="2000" :placeholder="t('edit.instruction_placeholder')"></textarea>
            <div style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">{{ t('edit.instruction_hint') }}</div>
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
            <div v-if="stylesLoadError" style="font-size: 11px; color: var(--status-warn, #e6a23c); margin-top: 4px;">{{ t('overrides.load_error') }}</div>
            <div v-if="selectedStyle" style="font-size: 11px; color: var(--text-dim); margin-top: 4px;">
              {{ t('edit.style_hint') }}
              <span v-if="selectedStyleLoras.length" style="display: inline-flex; flex-wrap: wrap; gap: 4px; margin-top: 4px;">
                <span v-for="l in selectedStyleLoras" :key="l.name + '@' + l.weight" class="style-lora-badge">{{ l.name }}@{{ l.weight }}</span>
              </span>
            </div>
          </div>

          <div class="form-group">
            <label class="form-label" for="edit-denoise-range">{{ t('edit.denoise_label') }}: <span data-testid="edit-denoise-value">{{ Number(denoiseInput).toFixed(2) }}</span></label>
            <input id="edit-denoise-range" data-testid="edit-denoise-range" type="range" v-model.number="denoiseInput" min="0.3" max="1" step="0.05" style="width: 100%; accent-color: var(--accent);" />
            <div style="font-size: 11px; color: var(--text-dim); margin-top: 2px;">{{ t('edit.denoise_hint') }}</div>
          </div>

          <details class="edit-loras" @toggle="onLorasToggle">
            <summary>{{ t('edit.loras') }}</summary>
            <div class="edit-loras-body">
              <div v-if="loraLoadError" style="font-size: 11px; color: var(--status-warn, #e6a23c);">{{ t('overrides.load_error') }}</div>
              <div class="lora-head">
                <button data-testid="edit-lora-add" type="button" class="btn btn-sm btn-secondary" @click="addLora">{{ t('edit.loras_add') }}</button>
              </div>
              <div v-for="(row, idx) in loraRows" :key="idx" class="lora-row">
                <input data-testid="edit-lora-name" class="form-input" type="text" v-model="row.name" list="edit-lora-names" :placeholder="t('edit.loras_name')" :aria-label="t('edit.loras_name')" />
                <input data-testid="edit-lora-weight" class="form-input" type="number" v-model.number="row.weight" min="0" max="2" step="0.05" :placeholder="0.6" :aria-label="t('edit.loras_weight')" />
                <button data-testid="edit-lora-remove" type="button" class="btn btn-sm btn-secondary lora-remove" @click="removeLora(idx)" :aria-label="t('edit.loras_remove')">&times;</button>
              </div>
              <div class="lora-hint">{{ t('edit.loras_hint') }}</div>
            </div>
          </details>
          <datalist id="edit-lora-names">
            <option v-for="l in serverLoras" :key="l.name" :value="l.name" />
          </datalist>

          <div class="form-group">
            <ResolutionSelector v-model="selectedResolutionId" :none-label="t('fi.resolution_original')" />
            <label :style="{ display: 'flex', alignItems: 'center', gap: '6px', cursor: selectedResolutionId ? 'pointer' : 'not-allowed', opacity: selectedResolutionId ? 1 : 0.5, marginTop: '8px' }">
              <input data-testid="edit-quality-checkbox" type="checkbox" v-model="qualityMode" :disabled="!selectedResolutionId" style="accent-color: var(--accent);" />
              <span style="font-size: 12px;">{{ t('edit.quality_mode') }}</span>
            </label>
            <div v-if="qualityMode && selectedResolutionId" style="font-size: 11px; color: var(--text-dim); margin-top: 2px;">{{ t('edit.quality_hint') }}</div>
            <div v-else-if="!selectedResolutionId" style="font-size: 11px; color: var(--accent); margin-top: 2px;">{{ t('edit.quality_needs_resolution') }}</div>
          </div>

          <button class="btn btn-primary" :class="{ 'btn-generating': generatingImage }" style="width: 100%; justify-content: center; padding: 12px;" @click="generate" :disabled="!canGenerate">
            <span v-if="generatingImage" style="display: inline-flex; align-items: center; gap: 6px;">
              <span class="spinner" style="width: 14px; height: 14px; border-width: 2px;"></span>
              {{ t('edit.generating') }}
            </span>
            <span v-else>{{ t('edit.btn_generate') }}</span>
          </button>
        </div>
      </div>

      <div class="generate-section">
        <div class="generate-image-area">
          <div v-if="generatingImage" style="text-align: center; padding: 24px;">
            <img v-if="preview && sdProgress && sdProgress.progress > 0 && sdProgress.progress < 1" :src="preview" alt="preview" style="max-width: 100%; border-radius: var(--radius-sm); opacity: 0.6; image-rendering: pixelated;" />
            <span v-else class="spinner" style="width: 32px; height: 32px; border-width: 3px;"></span>
            <p style="margin-top: 12px; color: var(--text-dim);">{{ t('edit.generating') }}</p>
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
              <button class="btn btn-sm btn-secondary" @click="interruptGeneration" style="margin-top: 8px; font-size: 11px;">{{ t('progress.btn_interrupt') }}</button>
            </div>
          </div>
          <div v-else-if="generatedImage" style="width: 100%; padding: 12px;">
            <div v-if="qualityUpscaleSkipped" class="status status-warning" style="margin-bottom: 8px; text-align: center;">
              {{ t('fi.quality_upscale_skipped') }}
            </div>
            <div v-if="qualityRedenoiseSkipped" class="status status-warning" style="margin-bottom: 8px; text-align: center;">
              {{ t('fi.quality_redenoise_skipped') }}
            </div>
            <img :src="imageSrc" alt="Edited" class="img-fade-in" style="border-radius: var(--radius-sm); cursor: zoom-in;" @click="showViewer = true" />
            <div style="display: flex; gap: 8px; margin-top: 12px; justify-content: center; flex-wrap: wrap;">
              <button class="btn btn-secondary btn-sm" @click="downloadImage">{{ t('edit.btn_download') }}</button>
              <button class="btn btn-secondary btn-sm" @click="useResult">{{ t('edit.btn_use_result') }}</button>
              <button class="btn btn-secondary btn-sm" @click="generate">{{ t('edit.btn_regenerate') }}</button>
            </div>
          </div>
          <div v-else class="generate-placeholder">
            <div class="generate-placeholder-icon">&#9744;</div>
            <p>{{ t('fi.placeholder_image') }}</p>
          </div>
        </div>
      </div>
    </div>

    <ImageViewer v-if="showViewer" :image-base64="generatedImage" @close="showViewer = false" />
  </div>
</template>

<style scoped>
.drop-zone {
  border: 2px dashed var(--border);
  border-radius: var(--radius-sm);
  padding: 24px;
  text-align: center;
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
  min-height: 120px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}
.drop-zone:hover {
  border-color: var(--accent);
  background: var(--surface-2);
}
.drop-zone-active {
  border-color: var(--accent);
  background: var(--surface-2);
}
.fi-btn-row {
  display: flex;
  gap: 8px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.source-container {
  display: flex;
  justify-content: center;
  border-radius: var(--radius-sm);
  overflow: hidden;
  background: #000;
}
.source-img {
  max-height: 400px;
  max-width: 100%;
  display: block;
  border-radius: var(--radius-sm);
}
.model-note {
  margin-top: 12px;
  font-size: 11px;
  color: var(--text-dim);
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 4px 8px;
  display: inline-block;
}
.edit-loras {
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.edit-loras summary {
  cursor: pointer;
  color: var(--text-dim);
  font-size: 13px;
  padding: 8px 12px;
  user-select: none;
}
.edit-loras summary:hover {
  color: var(--text);
}
.edit-loras-body {
  padding: 4px 12px 12px;
}
.lora-head {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 6px;
}
.lora-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 6px;
}
.lora-row .form-input[type='text'] {
  flex: 1;
}
.lora-row .form-input[type='number'] {
  flex: 0 0 90px;
}
.lora-remove {
  padding: 4px 10px;
}
.lora-hint {
  font-size: 11px;
  color: var(--text-dim);
}
.style-lora-badge {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 1px 6px;
  font-size: 10px;
  color: var(--text-dim);
}
</style>
