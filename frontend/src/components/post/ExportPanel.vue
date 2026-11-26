<script setup>
import { ref, computed, watch } from 'vue'
import { Pencil, X } from 'lucide-vue-next'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { batchFileName } from '../../postBatch.js'

const props = defineProps({
  source: { type: String, default: '' },
  blocked: Boolean,
  sourceName: { type: String, default: 'image' },
  imageInfo: { type: Object, required: true },
  batch: { type: Object, required: true },
  post: { type: Object, required: true },
  status: { type: Object, required: true },
  queueJobs: { type: Object, default: null },
})

const { batch: batchState, batchActive, batchBusy, batchExporting } = props.batch
const { error, successMsg } = props.status

const exportPresets = ref([])
const selectedPresetId = ref(null)
const format = ref('jpeg')
const DEFAULT_EXPORT_QUALITY = 90
const quality = ref(DEFAULT_EXPORT_QUALITY)
const filename = ref('')
const exporting = ref(false)

const editingPreset = ref(null)
const presetName = ref('')
const showPresetForm = ref(false)

const formats = [
  { value: 'png', label: 'PNG' },
  { value: 'jpeg', label: 'JPEG' },
]

const showQuality = computed(() => format.value !== 'png')

watch(format, () => {
  updateFilename()
})

function updateFilename(name = props.sourceName) {
  const base = name.replace(/\.[^.]+$/, '') || 'postprocess'
  filename.value = `${base}_${new Date().toISOString().slice(0, 10)}.${format.value}`
}

function applyPreset(preset) {
  selectedPresetId.value = preset.id
  format.value = preset.format
  quality.value = preset.quality || DEFAULT_EXPORT_QUALITY
  updateFilename()
}

function onPresetSelect(id) {
  if (!id) { selectedPresetId.value = null; return }
  const p = exportPresets.value.find(ep => ep.id === Number(id))
  if (p) applyPreset(p)
}

async function savePreset() {
  try {
    const data = {
      name: presetName.value,
      format: format.value,
      width: 0,
      height: 0,
      lock_ratio: true,
      quality: quality.value,
      interpolation: 'lanczos',
    }
    if (editingPreset.value) {
      data.id = editingPreset.value.id
    }
    await api.saveExportPreset(data)
    await loadExportPresets()
    editingPreset.value = null
    presetName.value = ''
    showPresetForm.value = false
  } catch (e) {
    error.value = String(e)
  }
}

function startEditPreset(preset) {
  editingPreset.value = preset
  presetName.value = preset.name
  showPresetForm.value = true
}

async function deletePreset(id) {
  try {
    await api.deleteExportPreset(id)
    if (selectedPresetId.value === id) selectedPresetId.value = null
    await loadExportPresets()
  } catch (e) {
    error.value = String(e)
  }
}

async function loadExportPresets() {
  try {
    exportPresets.value = await api.listExportPresets()
  } catch (e) {
    console.warn('PostProcessPage.loadExportPresets:', e)
  }
}

function openSavePreset() {
  editingPreset.value = null
  presetName.value = ''
  showPresetForm.value = true
}

function exportParamsFor(name, imageBase64, outputDir) {
  return {
    image_base64: imageBase64,
    format: format.value,
    width: 0,
    height: 0,
    lock_ratio: true,
    quality: quality.value,
    interpolation: 'lanczos',
    filename: name,
    ...(props.post.finishActive.value ? { finish: props.post.finishParams() } : {}),
    ...(outputDir ? { output_dir: outputDir } : {}),
  }
}

const batchFolder = ref('')

async function resolveBatchFolder() {
  if (batchFolder.value) return batchFolder.value
  try {
    const s = await api.getSettings()
    const p = s && s.file_browser_path
    if (p) {
      batchFolder.value = p
      return p
    }
  } catch (e) {
    console.warn('PostProcessPage.resolveBatchFolder:', e)
  }
  try {
    const picked = await api.selectBrowserFolder()
    if (picked) batchFolder.value = picked
  } catch (e) {
    error.value = String(e)
  }
  return batchFolder.value
}

async function changeBatchFolder() {
  try {
    const picked = await api.selectBrowserFolder()
    if (picked) batchFolder.value = picked
  } catch (e) {
    error.value = String(e)
  }
}

async function withRetouch(params, b64) {
  return { ...params, ...(await props.post.retouchFor(b64)) }
}

function batchExporter(folder) {
  const adj = props.post.adjustParams()
  return async (b64, item) => api.exportImage({ ...(await withRetouch(exportParamsFor(batchFileName(item.name, format.value), b64, folder), b64)) }, adj)
}

async function doExport() {
  if (props.blocked || exporting.value || batchBusy.value || batchExporting.value) return
  if (batchActive.value) {
    await props.batch.exportBatch(resolveBatchFolder, batchExporter)
    return
  }
  if (!props.source) {
    error.value = t('export.error_no_image')
    return
  }
  exporting.value = true
  error.value = ''
  successMsg.value = ''
  try {
    const path = await api.exportImage(await withRetouch(exportParamsFor(filename.value, props.source), props.source), props.post.adjustParams())
    if (path) {
      successMsg.value = t('export.saved_to', { path })
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    exporting.value = false
  }
}

defineExpose({ updateFilename, load: loadExportPresets })
</script>

<template>
  <div class="form-group">
    <label class="form-label">{{ t('export.label_preset') }}</label>
    <div class="preset-row">
      <select class="form-select" data-testid="export-preset" :value="selectedPresetId ?? ''" @change="onPresetSelect($event.target.value)">
        <option value="">{{ t('export.custom') }}</option>
        <option v-for="p in exportPresets" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <button class="btn btn-sm btn-secondary" data-testid="export-save-preset" @click="openSavePreset">{{ t('export.btn_save_preset') }}</button>
    </div>
  </div>

  <div class="form-group">
    <label class="form-label">{{ t('export.label_format') }}</label>
    <select class="form-select" v-model="format" data-testid="export-format">
      <option v-for="f in formats" :key="f.value" :value="f.value">{{ f.label }}</option>
    </select>
  </div>

  <p class="form-hint" data-testid="export-current-size">{{ t('export.current_size_hint') }}</p>

  <div class="form-group" v-if="showQuality">
    <label class="form-label">{{ t('export.label_quality', { value: quality }) }}</label>
    <input type="range" min="1" max="100" v-model.number="quality" class="form-range" />
  </div>

  <template v-if="batchActive">
    <div class="pipeline-folder">
      <span class="pipeline-folder-label">{{ t('post.pipeline_folder') }}:</span>
      <span class="pipeline-folder-path" :title="batchFolder">{{ batchFolder || t('post.batch_folder_default') }}</span>
      <button class="btn btn-sm btn-secondary" @click="changeBatchFolder">{{ t('post.pipeline_change_folder') }}</button>
    </div>
    <div class="form-hint">{{ t('post.batch_save_hint') }}</div>
    <button class="btn btn-primary btn-block" :disabled="blocked || batchBusy || batchExporting" @click="doExport">
      {{ batchExporting ? t('export.exporting') : t('post.batch_save_all', { n: batchState.items.length }) }}
    </button>
  </template>
  <template v-else>
  <div class="form-group">
    <label class="form-label">{{ t('export.label_filename') }}</label>
    <input class="form-control" type="text" v-model="filename" />
  </div>

  <button class="btn btn-primary btn-block" :disabled="blocked || exporting || !source" @click="doExport">
    {{ exporting ? t('export.exporting') : t('post.btn_save_file') }}
  </button>
  </template>

  <div class="form-divider"></div>
  <div class="form-label">{{ t('export.saved_presets') }}</div>
  <div class="preset-list">
    <div v-if="!exportPresets.length" class="preset-empty">{{ t('export.no_presets') }}</div>
    <div
      v-for="p in exportPresets"
      :key="p.id"
      class="preset-item"
      :class="{ active: selectedPresetId === p.id }"
      @click="applyPreset(p)"
    >
      <div class="preset-info">
        <span class="preset-name">{{ p.name }}</span>
        <span class="preset-meta">{{ p.format.toUpperCase() }} · Q{{ p.quality }}</span>
      </div>
      <div class="preset-actions">
        <button class="btn-icon" @click.stop="startEditPreset(p)" :title="t('presets.btn_edit')"><Pencil :size="14" /></button>
        <button class="btn-icon btn-icon-danger" @click.stop="deletePreset(p.id)" :title="t('presets.btn_delete')"><X :size="14" /></button>
      </div>
    </div>
  </div>

  <div class="preset-form" v-if="showPresetForm">
    <div class="form-divider"></div>
    <div class="form-group">
      <input class="form-control" type="text" v-model="presetName" data-testid="export-preset-name" :placeholder="t('export.placeholder_preset_name')" />
    </div>
    <div class="preset-form-actions">
      <button class="btn btn-sm btn-primary" @click="savePreset" :disabled="!presetName">
        {{ editingPreset ? t('export.btn_update') : t('export.btn_save') }}
      </button>
      <button class="btn btn-sm btn-secondary" @click="showPresetForm = false; editingPreset = null">{{ t('export.btn_cancel') }}</button>
    </div>
  </div>
</template>

<style scoped src="./post-shared.css"></style>

<style scoped>
.pipeline-folder {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  font-size: var(--fs-sm);
  color: var(--text-dim);
}

.pipeline-folder-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl;
  text-align: left;
  color: var(--text);
}
</style>
