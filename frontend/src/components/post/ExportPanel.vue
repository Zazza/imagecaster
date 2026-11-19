<script setup>
import { ref, computed, watch } from 'vue'
import { Pencil, X } from 'lucide-vue-next'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { batchExportSize, batchFileName } from '../../postBatch.js'

const props = defineProps({
  source: { type: String, default: '' },
  sourceName: { type: String, default: 'image' },
  imageInfo: { type: Object, required: true },
  batch: { type: Object, required: true },
  post: { type: Object, required: true },
  status: { type: Object, required: true },
  queueJobs: { type: Object, default: null },
})

const { batch: batchState, batchActive, batchBusy, batchExporting } = props.batch
const { error, successMsg } = props.status
const { upscale4k } = props.post

const exportPresets = ref([])
const selectedPresetId = ref(null)
const format = ref('png')
const width = ref(0)
const height = ref(0)
const lockRatio = ref(true)
const quality = ref(90)
const interpolation = ref('lanczos')
const filename = ref('')
const exporting = ref(false)

const editingPreset = ref(null)
const presetName = ref('')
const showPresetForm = ref(false)

const formats = [
  { value: 'png', label: 'PNG' },
  { value: 'jpeg', label: 'JPEG' },
]

const interpolations = [
  { value: 'nearest', label: 'Nearest' },
  { value: 'linear', label: 'Linear' },
  { value: 'lanczos', label: 'Lanczos' },
]

const showQuality = computed(() => format.value !== 'png')

const aspectRatio = computed(() => {
  if (!props.imageInfo.width || !props.imageInfo.height) return 1
  return props.imageInfo.width / props.imageInfo.height
})

watch(() => props.imageInfo, (info) => {
  width.value = info.width
  height.value = info.height
})

watch(width, (w) => {
  if (lockRatio.value && w > 0 && aspectRatio.value) {
    height.value = Math.round(w / aspectRatio.value)
  }
})

watch(height, (h) => {
  if (lockRatio.value && h > 0 && aspectRatio.value) {
    width.value = Math.round(h * aspectRatio.value)
  }
})

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
  interpolation.value = preset.interpolation
  quality.value = preset.quality || 90
  lockRatio.value = preset.lock_ratio

  if (preset.width > 0 || preset.height > 0) {
    if (preset.lock_ratio && preset.width > 0 && preset.height === 0) {
      width.value = preset.width
    } else if (preset.lock_ratio && preset.height > 0 && preset.width === 0) {
      height.value = preset.height
    } else {
      width.value = preset.width || props.imageInfo.width
      height.value = preset.height || props.imageInfo.height
    }
  } else {
    width.value = props.imageInfo.width
    height.value = props.imageInfo.height
  }
  updateFilename()
}

function onPresetSelect(id) {
  const p = exportPresets.value.find(ep => ep.id === Number(id))
  if (p) applyPreset(p)
}

async function savePreset() {
  try {
    const data = {
      name: presetName.value,
      format: format.value,
      width: width.value === props.imageInfo.width ? 0 : width.value,
      height: height.value === props.imageInfo.height ? 0 : height.value,
      lock_ratio: lockRatio.value,
      quality: quality.value,
      interpolation: interpolation.value,
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

function resetSize() {
  width.value = props.imageInfo.width
  height.value = props.imageInfo.height
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
    width: width.value,
    height: height.value,
    lock_ratio: lockRatio.value,
    quality: quality.value,
    interpolation: interpolation.value,
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

function upscaleNote(info) {
  return info && info.upscale === 'resized' ? ' — ' + t('post.up4k_resized') : ''
}

function track4K(id) {
  if (!props.queueJobs) return Promise.resolve(null)
  return props.queueJobs.track(id)
}

async function enqueue4K(params, b64, adj) {
  const p = await withRetouch(params, b64)
  return api.enqueuePostSave({ ...p, width: 0, height: 0 }, adj)
}

async function export4K() {
  const id = await enqueue4K(exportParamsFor(filename.value, props.source), props.source, props.post.adjustParams())
  if (!id) return
  successMsg.value = t('post.up4k_queued')
  track4K(id).then(outcome => {
    if (!outcome) return
    if (outcome.status === 'completed') {
      const info = outcome.result?.info
      if (info?.path) successMsg.value = t('export.saved_to', { path: info.path }) + upscaleNote(info)
    } else if (outcome.status === 'failed') {
      error.value = t('post.up4k_failed', { error: outcome.error || '' })
    }
  })
}

async function exportBatch4K() {
  const ids = []
  const adj = props.post.adjustParams()
  await props.batch.exportBatch(resolveBatchFolder, (folder) => async (b64, item) => {
    const id = await enqueue4K(exportParamsFor(batchFileName(item.name, format.value), b64, folder), b64, adj)
    if (id) ids.push(id)
    return id
  })
  if (!ids.length) return
  const folder = batchFolder.value
  successMsg.value = t('post.up4k_batch_queued', { n: ids.length })
  Promise.all(ids.map(track4K)).then(outcomes => {
    if (outcomes.some(o => !o)) return
    const saved = outcomes.filter(o => o.status === 'completed').length
    successMsg.value = t('post.up4k_batch_done', { k: saved, n: ids.length, path: folder })
  })
}

function batchExporter(folder) {
  const size = batchExportSize({
    width: width.value,
    height: height.value,
    lockRatio: lockRatio.value,
    previewWidth: props.imageInfo.width,
    previewHeight: props.imageInfo.height,
  })
  const adj = props.post.adjustParams()
  return async (b64, item) => api.exportImage({ ...(await withRetouch(exportParamsFor(batchFileName(item.name, format.value), b64, folder), b64)), width: size.width, height: size.height }, adj)
}

async function doExport() {
  if (batchActive.value) {
    if (upscale4k.value) {
      await exportBatch4K()
      return
    }
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
  if (upscale4k.value) {
    try {
      await export4K()
    } catch (e) {
      error.value = String(e)
    } finally {
      exporting.value = false
    }
    return
  }
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
      <select class="form-select" :value="selectedPresetId" @change="onPresetSelect($event.target.value)">
        <option :value="null">{{ t('export.custom') }}</option>
        <option v-for="p in exportPresets" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <button class="btn btn-sm btn-secondary" @click="openSavePreset">{{ t('export.btn_save_preset') }}</button>
    </div>
  </div>

  <div class="form-group">
    <label class="form-label">{{ t('export.label_format') }}</label>
    <select class="form-select" v-model="format">
      <option v-for="f in formats" :key="f.value" :value="f.value">{{ f.label }}</option>
    </select>
  </div>

  <div class="form-row">
    <div class="form-group form-group-half">
      <label class="form-label">{{ t('export.label_width') }}</label>
      <input class="form-control" type="number" v-model.number="width" min="1" :disabled="upscale4k" />
    </div>
    <div class="form-group form-group-half">
      <label class="form-label">{{ t('export.label_height') }}</label>
      <input class="form-control" type="number" v-model.number="height" min="1" :disabled="upscale4k" />
    </div>
  </div>

  <div class="form-group form-inline">
    <label class="form-label">
      <input type="checkbox" v-model="lockRatio" /> {{ t('export.lock_aspect') }}
    </label>
    <button class="btn btn-sm btn-secondary" @click="resetSize" v-if="imageInfo.width">{{ t('export.btn_reset') }}</button>
  </div>

  <div class="form-group" v-if="showQuality">
    <label class="form-label">{{ t('export.label_quality', { value: quality }) }}</label>
    <input type="range" min="1" max="100" v-model.number="quality" class="form-range" />
  </div>

  <div class="form-group">
    <label class="form-label">{{ t('export.label_interpolation') }}</label>
    <select class="form-select" v-model="interpolation">
      <option v-for="i in interpolations" :key="i.value" :value="i.value">{{ i.label }}</option>
    </select>
  </div>

  <div class="form-group">
    <label class="form-label">
      <input type="checkbox" v-model="upscale4k" data-testid="post-upscale-4k" /> {{ t('post.up4k_label') }}
    </label>
    <div v-if="upscale4k" class="form-hint">{{ t('post.up4k_hint') }}</div>
  </div>

  <template v-if="batchActive">
    <div class="pipeline-folder">
      <span class="pipeline-folder-label">{{ t('post.pipeline_folder') }}:</span>
      <span class="pipeline-folder-path" :title="batchFolder">{{ batchFolder || t('post.batch_folder_default') }}</span>
      <button class="btn btn-sm btn-secondary" @click="changeBatchFolder">{{ t('post.pipeline_change_folder') }}</button>
    </div>
    <div class="form-hint">{{ t('post.batch_save_hint') }}</div>
    <button class="btn btn-primary btn-block" :disabled="batchBusy || batchExporting" @click="doExport">
      {{ batchExporting ? t('export.exporting') : t('post.batch_save_all', { n: batchState.items.length }) }}
    </button>
  </template>
  <template v-else>
  <div class="form-group">
    <label class="form-label">{{ t('export.label_filename') }}</label>
    <input class="form-control" type="text" v-model="filename" />
  </div>

  <button class="btn btn-primary btn-block" :disabled="exporting || !source" @click="doExport">
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
        <span class="preset-meta">{{ p.format.toUpperCase() }} {{ p.width || 'orig' }}x{{ p.height || 'orig' }} Q{{ p.quality }}</span>
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
      <input class="form-control" type="text" v-model="presetName" :placeholder="t('export.placeholder_preset_name')" />
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
