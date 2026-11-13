<script setup>
import { ArrowRight, ChevronDown, ChevronUp, X, Pencil, Copy, Trash2, Workflow } from 'lucide-vue-next'
import StatusMessage from './StatusMessage.vue'
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { withPresetStatuses } from '../composables/usePresets.js'
import { hiddenPresetIds } from '../presetAvailability.js'
import { t } from '../i18n/index.js'
import PipelineImportModal from './PipelineImportModal.vue'
import SearchableSelect from './SearchableSelect.vue'
import { filterPipelines, pipelineModels, pipelineModelCounts } from '../pipelineFilters.js'

const compounds = ref([])
const presets = ref([])
const hiddenIds = ref(new Set())
const presetTypes = ref([])
const editing = ref(null)
const showForm = ref(false)
const error = ref('')

const formName = ref('')
const formDescription = ref('')
const formSteps = ref([])

const selectedIds = ref(new Set())
const selectMode = ref(false)
const showDeleteConfirm = ref(false)
const pendingDeleteId = ref(null)
let deleteTimer = null
const filterSearch = ref('')
const filterModels = ref([])
const showImport = ref(false)
const importPipelines = ref([])

const selectedCount = computed(() => selectedIds.value.size)

const presetItems = computed(() => presets.value.filter(p => !hiddenIds.value.has(p.id)).map(p => {
  const pt = presetTypes.value.find(pt => pt.id === p.type_id)
  const item = { value: p.id, label: p.name }
  if (pt) item.hint = pt.name
  return item
}))

function toggleSelect(id) {
  const s = new Set(selectedIds.value)
  if (s.has(id)) { s.delete(id) } else { s.add(id) }
  selectedIds.value = s
}

const presetsById = computed(() => new Map(presets.value.map(p => [p.id, p])))
const filters = computed(() => ({ search: filterSearch.value, models: filterModels.value }))
const filteredCompounds = computed(() => filterPipelines(compounds.value, presetsById.value, filters.value))
const modelFacets = computed(() => pipelineModelCounts(compounds.value, presetsById.value, filters.value))
const filtersActive = computed(() => Boolean(filterSearch.value.trim() || filterModels.value.length))
const allVisibleSelected = computed(() =>
  filteredCompounds.value.length > 0 && filteredCompounds.value.every(c => selectedIds.value.has(c.id)))
const selectedCompounds = computed(() => filteredCompounds.value.filter(c => selectedIds.value.has(c.id)))

watch(filteredCompounds, list => {
  const visible = new Set(list.map(c => c.id))
  const kept = [...selectedIds.value].filter(id => visible.has(id))
  if (kept.length !== selectedIds.value.size) selectedIds.value = new Set(kept)
})

function toggleSelectAll() {
  selectedIds.value = allVisibleSelected.value ? new Set() : new Set(filteredCompounds.value.map(c => c.id))
}

function toggleSelectMode() {
  selectMode.value = !selectMode.value
  if (!selectMode.value) selectedIds.value = new Set()
}

function toggleModel(label) {
  const cur = filterModels.value
  filterModels.value = cur.includes(label) ? cur.filter(m => m !== label) : [...cur, label]
}

function pickModel(label) {
  if (!filterModels.value.includes(label)) filterModels.value = [...filterModels.value, label]
}

function resetFilters() {
  filterSearch.value = ''
  filterModels.value = []
}

function modelText(label) {
  return label || t('presets.model_any')
}

function stepMissing(step) {
  return !presetsById.value.has(step.preset_id) || hiddenIds.value.has(step.preset_id)
}

function hasMissingStep(cp) {
  return (cp.steps || []).some(stepMissing)
}

async function loadData() {
  try {
    await withPresetStatuses(async statuses => {
      const [c, p, pt] = await Promise.all([api.listCompoundPresets(), api.listPresets(), api.listPresetTypes()])
      compounds.value = c || []
      presets.value = p || []
      presetTypes.value = pt || []
      hiddenIds.value = hiddenPresetIds(statuses)
    })
  } catch (e) {
    error.value = String(e)
  }
}

function openCreate() {
  editing.value = null
  formName.value = ''
  formDescription.value = ''
  formSteps.value = [{ preset_id: null, denoising_strength: 0.5 }]
  showForm.value = true
  error.value = ''
}

function openEdit(cp) {
  editing.value = cp.id
  formName.value = cp.name
  formDescription.value = cp.description
  formSteps.value = cp.steps.map(s => ({
    preset_id: s.preset_id,
    denoising_strength: s.denoising_strength || 0.5,
    loras: s.loras ?? null,
  }))
  showForm.value = true
  error.value = ''
}

function cancelForm() {
  showForm.value = false
  editing.value = null
  error.value = ''
}

function addStep() {
  formSteps.value.push({ preset_id: null, denoising_strength: 0.5 })
}

function removeStep(idx) {
  formSteps.value.splice(idx, 1)
}

function moveStepUp(idx) {
  if (idx <= 0) return
  const tmp = formSteps.value[idx]
  formSteps.value[idx] = formSteps.value[idx - 1]
  formSteps.value[idx - 1] = tmp
  formSteps.value = [...formSteps.value]
}

function moveStepDown(idx) {
  if (idx >= formSteps.value.length - 1) return
  const tmp = formSteps.value[idx]
  formSteps.value[idx] = formSteps.value[idx + 1]
  formSteps.value[idx + 1] = tmp
  formSteps.value = [...formSteps.value]
}

async function saveCompound() {
  if (!formName.value.trim()) {
    error.value = t('compound.error_name_required')
    return
  }
  if (formSteps.value.some(s => !s.preset_id)) {
    error.value = t('compound.error_step_style')
    return
  }

  const data = {
    name: formName.value.trim(),
    description: formDescription.value.trim(),
    steps: formSteps.value.map((s, i) => ({
      step_order: i + 1,
      preset_id: s.preset_id,
      denoising_strength: s.denoising_strength,
      loras: s.loras ?? null,
    })),
  }

  try {
    if (editing.value) {
      data.id = editing.value
      await api.updateCompoundPreset(data)
    } else {
      await api.createCompoundPreset(data)
    }
    showForm.value = false
    editing.value = null
    await loadData()
  } catch (e) {
    error.value = String(e)
  }
}

async function handleDuplicate(cp) {
  const { id, created_at, updated_at, steps, ...data } = cp
  try {
    await api.createCompoundPreset({
      ...data,
      name: data.name + ' (copy)',
      steps: steps.map((s, i) => ({
        step_order: i + 1,
        preset_id: s.preset_id,
        denoising_strength: s.denoising_strength,
        loras: s.loras ?? null,
      })),
    })
  } catch (e) {
    error.value = String(e)
    return
  }
  await loadData()
}

async function deleteCompound(id) {
  if (pendingDeleteId.value !== id) {
    clearTimeout(deleteTimer)
    pendingDeleteId.value = id
    deleteTimer = setTimeout(() => { pendingDeleteId.value = null }, 3000)
    return
  }
  clearTimeout(deleteTimer)
  pendingDeleteId.value = null
  try {
    await api.deleteCompoundPreset(id)
    await loadData()
  } catch (e) {
    error.value = String(e)
  }
}

function requestBatchDelete() {
  if (selectedIds.value.size === 0) return
  showDeleteConfirm.value = true
}

async function confirmBatchDelete() {
  showDeleteConfirm.value = false
  for (const id of [...selectedIds.value]) {
    try {
      await api.deleteCompoundPreset(id)
    } catch (e) {
      console.error('delete pipeline', id, e)
      error.value = String(e)
    }
  }
  selectedIds.value = new Set()
  selectMode.value = false
  await loadData()
}

onUnmounted(() => clearTimeout(deleteTimer))

async function handleExport() {
  if (selectedIds.value.size === 0) return
  const ids = [...selectedIds.value]
  try {
    const path = await api.prepareCompoundPresetsExport(ids)
    if (path) {
      selectedIds.value = new Set()
      selectMode.value = false
      alert(t('compound.alert_saved', { path }))
    }
  } catch (e) {
    alert(t('compound.alert_export_failed', { error: String(e) }))
  }
}

async function handleOpenImport() {
  try {
    const result = await api.openImportCompoundFile()
    if (result && result.pipelines) {
      importPipelines.value = result.pipelines
      showImport.value = true
    }
  } catch (e) {
    if (String(e)) alert(t('compound.alert_import_failed', { error: String(e) }))
  }
}

function handleImportDone() {
  showImport.value = false
  importPipelines.value = []
  loadData()
}

function getPresetName(presetId) {
  const p = presetsById.value.get(presetId)
  return p ? p.name : `#${presetId}`
}

onMounted(loadData)
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ t('compound.title') }}</h1>
      <div class="header-actions">
        <template v-if="selectMode">
          <button class="btn btn-secondary btn-sm" data-testid="select-all" @click="toggleSelectAll" :disabled="filteredCompounds.length === 0">
            {{ allVisibleSelected ? t('presets.btn_deselect_all') : t('presets.btn_select_all') }}
          </button>
          <button class="btn btn-primary btn-sm" data-testid="export-selected" @click="handleExport" :disabled="selectedCount === 0">
            {{ t('compound.btn_export', { count: selectedCount }) }}
          </button>
          <button class="btn btn-danger-quiet btn-sm" data-testid="delete-selected" @click="requestBatchDelete" :disabled="selectedCount === 0">
            {{ t('presets.btn_delete_selected', { count: selectedCount }) }}
          </button>
          <button class="btn btn-secondary btn-sm" @click="toggleSelectMode">{{ t('presets.btn_cancel') }}</button>
        </template>
        <template v-else>
          <button class="btn btn-secondary" @click="handleOpenImport">{{ t('compound.btn_import') }}</button>
          <button class="btn btn-secondary" data-testid="select-mode" @click="toggleSelectMode" :disabled="compounds.length === 0 || showForm">{{ t('presets.btn_select') }}</button>
          <button class="btn btn-primary" @click="openCreate">{{ t('compound.btn_new') }}</button>
        </template>
      </div>
    </div>

    <StatusMessage v-if="error" @close="error = ''">{{ error }}</StatusMessage>

    <div v-if="showForm" class="card" style="max-width: 700px;">
      <h3 style="margin-bottom: 12px;">{{ editing ? t('compound.edit_pipeline') : t('compound.new_pipeline') }}</h3>

      <div class="form-group">
        <label class="form-label">{{ t('compound.label_name') }}</label>
        <input class="form-input" v-model="formName" :placeholder="t('compound.placeholder_name')" />
      </div>

      <div class="form-group">
        <label class="form-label">{{ t('compound.label_description') }}</label>
        <input class="form-input" v-model="formDescription" :placeholder="t('compound.placeholder_description')" />
      </div>

      <div style="margin-bottom: 12px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
          <label class="form-label" style="margin: 0;">{{ t('compound.steps_count', { count: formSteps.length }) }}</label>
          <button class="btn btn-sm btn-secondary" @click="addStep">{{ t('compound.btn_add_step') }}</button>
        </div>

        <div v-for="(step, idx) in formSteps" :key="idx" style="border: 1px solid var(--border); border-radius: 6px; padding: 12px; margin-bottom: 8px; background: var(--surface-1);">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
            <span style="font-weight: 600; font-size: 13px;">{{ t('compound.step_label', { num: idx + 1, type: idx === 0 ? t('compound.step_txt2img') : t('compound.step_img2img') }) }}</span>
            <div style="display: flex; gap: 4px;">
              <button class="btn btn-sm btn-secondary" style="padding: 2px 8px;" @click="moveStepUp(idx)" :disabled="idx === 0"><ChevronUp :size="14" /></button>
              <button class="btn btn-sm btn-secondary" style="padding: 2px 8px;" @click="moveStepDown(idx)" :disabled="idx === formSteps.length - 1"><ChevronDown :size="14" /></button>
              <button class="btn btn-sm btn-danger-quiet" style="padding: 2px 8px;" @click="removeStep(idx)" :disabled="formSteps.length <= 1"><X :size="14" /></button>
            </div>
          </div>

          <div style="display: grid; grid-template-columns: 1fr; gap: 8px;">
            <div class="form-group" style="margin: 0;">
              <label class="form-label">{{ t('compound.label_style') }}</label>
              <SearchableSelect
                v-model="step.preset_id"
                :items="presetItems"
                :placeholder="t('compound.select_style')"
                :empty-text="t('searchable.no_results')"
              />
            </div>

            <div v-if="idx > 0" class="form-group" style="margin: 0;">
              <label class="form-label">{{ t('compound.label_denoising') }}</label>
              <input class="form-input" type="number" v-model.number="step.denoising_strength" min="0" max="1" step="0.05" />
            </div>
          </div>
        </div>
      </div>

      <div style="display: flex; gap: 8px;">
        <button class="btn btn-primary" @click="saveCompound">{{ editing ? t('compound.btn_update') : t('compound.btn_create') }}</button>
        <button class="btn btn-secondary" @click="cancelForm">{{ t('compound.btn_cancel') }}</button>
      </div>
    </div>

    <div v-if="!showForm && compounds.length === 0" class="empty-state">
      <div class="empty-state-icon"><Workflow :size="40" :stroke-width="1.5" /></div>
      <p>{{ t('compound.no_pipelines') }}</p>
    </div>

    <template v-if="!showForm && compounds.length > 0">
      <div class="styles-filters">
        <div class="styles-filter-row">
          <input class="form-input styles-search" data-testid="filter-search" v-model="filterSearch" :placeholder="t('compound.search')" />
          <button v-if="filtersActive" class="btn btn-secondary btn-sm" data-testid="filter-reset" @click="resetFilters">{{ t('presets.filter_reset') }}</button>
        </div>
        <div v-if="modelFacets.length" class="styles-chips">
          <span class="styles-filter-label">{{ t('presets.filter_model') }}</span>
          <button
            v-for="m in modelFacets"
            :key="m.label"
            class="styles-chip"
            data-testid="filter-model"
            :data-model="m.label"
            :class="{ active: filterModels.includes(m.label) }"
            @click="toggleModel(m.label)"
          >{{ modelText(m.label) }} <span class="styles-chip-count">{{ m.count }}</span></button>
        </div>
      </div>

      <div v-if="filteredCompounds.length === 0" class="styles-empty" data-testid="no-match">
        <p>{{ t('compound.no_match') }}</p>
        <button class="btn btn-secondary btn-sm" @click="resetFilters">{{ t('presets.filter_reset') }}</button>
      </div>

      <div v-else class="styles-table-wrap">
        <table class="styles-table">
          <thead>
            <tr>
              <th v-if="selectMode" class="col-check">
                <input type="checkbox" data-testid="select-all-check" :checked="allVisibleSelected" :aria-label="t('presets.btn_select_all')" @change="toggleSelectAll" />
              </th>
              <th>{{ t('presets.col_name') }}</th>
              <th>{{ t('compound.col_chain') }}</th>
              <th>{{ t('presets.col_model') }}</th>
              <th class="col-actions"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="cp in filteredCompounds" :key="cp.id" data-testid="pipeline-row" :class="{ 'preset-selected': selectedIds.has(cp.id) }">
              <td v-if="selectMode" class="col-check">
                <input type="checkbox" :checked="selectedIds.has(cp.id)" @change="toggleSelect(cp.id)" />
              </td>
              <td class="col-name">
                <div class="styles-name">{{ cp.name }}</div>
                <div v-if="cp.description" class="styles-prompt" :title="cp.description">{{ cp.description }}</div>
                <div v-if="hasMissingStep(cp)" class="styles-missing" data-testid="pipeline-missing">{{ t('compound.step_missing_hint') }}</div>
              </td>
              <td>
                <div class="pipe-chain">
                  <template v-for="(step, idx) in cp.steps" :key="step.id || idx">
                    <ArrowRight v-if="idx > 0" :size="12" class="pipe-arrow" />
                    <span
                      class="badge pipe-step"
                      data-testid="step-badge"
                      :class="stepMissing(step) ? 'badge-warning' : 'badge-muted'"
                      :title="stepMissing(step) ? t('compound.step_missing') : ''"
                    >{{ getPresetName(step.preset_id) }}<span v-if="idx > 0 && step.denoising_strength != null" class="pipe-denoise">{{ step.denoising_strength }}</span></span>
                  </template>
                </div>
              </td>
              <td>
                <div class="pipe-chain">
                  <button
                    v-for="m in pipelineModels(cp, presetsById)"
                    :key="m"
                    class="badge badge-muted styles-badge"
                    data-testid="badge-model"
                    :title="t('presets.filter_by')"
                    @click="pickModel(m)"
                  >{{ modelText(m) }}</button>
                </div>
              </td>
              <td class="col-actions">
                <template v-if="!selectMode">
                  <button class="styles-icon-btn" data-testid="row-edit" :title="t('compound.btn_edit')" :aria-label="t('compound.btn_edit')" @click="openEdit(cp)"><Pencil :size="14" /></button>
                  <button class="styles-icon-btn" data-testid="row-duplicate" :title="t('presets.btn_duplicate')" :aria-label="t('presets.btn_duplicate')" @click="handleDuplicate(cp)"><Copy :size="14" /></button>
                  <button
                    class="styles-icon-btn"
                    data-testid="row-delete"
                    :class="{ danger: pendingDeleteId === cp.id }"
                    :title="pendingDeleteId === cp.id ? t('presets.btn_sure') : t('compound.btn_delete')"
                    :aria-label="t('compound.btn_delete')"
                    @click="deleteCompound(cp.id)"
                  ><Trash2 :size="14" /><span v-if="pendingDeleteId === cp.id" class="styles-sure">{{ t('presets.btn_sure') }}</span></button>
                </template>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <div v-if="showDeleteConfirm" class="modal-overlay">
      <div class="modal">
        <div class="modal-header">
          <h2 class="modal-title">{{ t('compound.delete_title') }}</h2>
          <button class="modal-close" @click="showDeleteConfirm = false"><X :size="16" /></button>
        </div>
        <p>{{ t('compound.delete_confirm_n', { count: selectedCount }) }}</p>
        <ul class="styles-delete-names" data-testid="delete-names">
          <li v-for="cp in selectedCompounds.slice(0, 10)" :key="cp.id">{{ cp.name }}</li>
          <li v-if="selectedCompounds.length > 10">{{ t('presets.delete_more', { count: selectedCompounds.length - 10 }) }}</li>
        </ul>
        <p style="color: var(--text-dim); font-size: 0.9em;">{{ t('presets.delete_cannot_undo') }}</p>
        <div style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 16px;">
          <button class="btn btn-secondary" @click="showDeleteConfirm = false">{{ t('presets.btn_cancel') }}</button>
          <button class="btn btn-danger" data-testid="confirm-delete" @click="confirmBatchDelete">{{ t('presets.btn_delete') }}</button>
        </div>
      </div>
    </div>

    <PipelineImportModal
      v-if="showImport"
      :pipelines="importPipelines"
      @done="handleImportDone"
      @close="showImport = false"
    />
  </div>
</template>

<style scoped>
.pipe-chain { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; }
.pipe-arrow { color: var(--text-dim); flex-shrink: 0; }
.pipe-step { display: inline-flex; align-items: center; gap: 5px; }
.pipe-denoise { font-size: 10px; opacity: 0.7; }
.pipe-step.badge-muted { background: transparent; }
</style>
