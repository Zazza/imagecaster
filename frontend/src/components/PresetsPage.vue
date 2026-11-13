<script setup>
import { LayoutGrid, X, Pencil, Copy, Trash2 } from 'lucide-vue-next'
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'
import { withPresetStatuses } from '../composables/usePresets.js'
import { filterAvailable } from '../presetAvailability.js'
import { t } from '../i18n/index.js'
import { effectTypeLabel, isEffectType } from '../effectTypes.js'
import { presetSource, modelLabel, filterPresets, facetCounts, hasActiveFilters, GROUP_NONE } from '../presetFilters.js'
import PresetForm from './PresetForm.vue'
import ImportModal from './ImportModal.vue'

const props = defineProps({
  mode: { type: String, default: 'styles' },
})

const allPresets = ref([])
const presetTypes = ref([])
const loading = ref(true)
const showForm = ref(false)
const editingPreset = ref(null)

const selectMode = ref(false)
const selectedIds = ref(new Set())

const showImport = ref(false)
const filterSearch = ref('')
const filterSource = ref('')
const filterModels = ref([])
const filterGroup = ref('')
const importPresets = ref([])

const installStatuses = ref(null)
const showUnavailable = ref(false)
const installingId = ref(null)

const pendingDeleteId = ref(null)
let deleteTimer = null

const showDeleteConfirm = ref(false)

const selectedCount = computed(() => selectedIds.value.size)

function getStatus(presetId) {
  return (installStatuses.value || []).find(s => s.id === presetId)
}

async function installDeps(presetId) {
  installingId.value = presetId
  try {
    await api.installPresetDeps(presetId)
    await load()
  } catch (e) {
    alert('Install failed: ' + e)
  } finally {
    installingId.value = null
  }
}

async function load() {
  loading.value = true
  try {
    await withPresetStatuses(async statuses => {
      const [p, t] = await Promise.all([api.listPresets(), api.listPresetTypes()])
      installStatuses.value = statuses
      allPresets.value = p || []
      presetTypes.value = t || []
    })
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

const effectTypeIds = computed(() => new Set(
  presetTypes.value.filter(pt => isEffectType((pt.name || '').trim().toLowerCase())).map(pt => pt.id)
))

const modePresets = computed(() => {
  const wantEffects = props.mode === 'effects'
  return allPresets.value.filter(p => effectTypeIds.value.has(p.type_id) === wantEffects)
})

const presets = computed(() => {
  return filterAvailable(modePresets.value, installStatuses.value, showUnavailable.value)
})

const formDefaults = computed(() => {
  if (props.mode !== 'effects') return null
  const base = presetTypes.value.find(pt => (pt.name || '').trim().toLowerCase() === 'post')
  return base ? { type_id: base.id, preset_type: 'post' } : { preset_type: 'post' }
})

const filters = computed(() => ({
  search: filterSearch.value,
  source: filterSource.value,
  models: filterModels.value,
  group: filterGroup.value,
}))

const filteredPresets = computed(() => filterPresets(presets.value, filters.value))
const facets = computed(() => facetCounts(presets.value, filters.value))
const filtersActive = computed(() => hasActiveFilters(filters.value))

function toggleModel(label) {
  const cur = filterModels.value
  filterModels.value = cur.includes(label) ? cur.filter(m => m !== label) : [...cur, label]
}

function pickModel(label) {
  if (!filterModels.value.includes(label)) filterModels.value = [...filterModels.value, label]
}

function pickGroup(p) {
  filterGroup.value = (p.preset_type || '').trim() || GROUP_NONE
}

function resetFilters() {
  filterSearch.value = ''
  filterSource.value = ''
  filterModels.value = []
  filterGroup.value = ''
}

function groupLabel(value) {
  return value === GROUP_NONE ? t('presets.type_general') : effectTypeLabel(value, t)
}

function modelText(label) {
  return label || t('presets.model_any')
}

function openCreate() {
  editingPreset.value = null
  showForm.value = true
}

function openEdit(preset) {
  editingPreset.value = { ...preset }
  showForm.value = true
}

function closeForm() {
  showForm.value = false
  editingPreset.value = null
}

async function handleSave(data) {
  if (editingPreset.value) {
    await api.updatePreset(editingPreset.value.id, data)
  } else {
    await api.createPreset(data)
  }
  closeForm()
  await load()
}

async function handleDuplicate(preset) {
  const { id, created_at, updated_at, ...data } = preset
  await api.createPreset({ ...data, name: data.name + ' (copy)' })
  await load()
}

async function handleDelete(id) {
  if (pendingDeleteId.value === id) {
    clearTimeout(deleteTimer)
    pendingDeleteId.value = null
    await api.deletePreset(id)
    await load()
  } else {
    clearTimeout(deleteTimer)
    pendingDeleteId.value = id
    deleteTimer = setTimeout(() => { pendingDeleteId.value = null }, 3000)
  }
}

function toggleSelectMode() {
  selectMode.value = !selectMode.value
  if (!selectMode.value) {
    selectedIds.value = new Set()
  }
}

function togglePreset(id) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedIds.value = next
}

function selectAll() {
  if (selectedIds.value.size === presets.value.length) {
    selectedIds.value = new Set()
  } else {
    selectedIds.value = new Set(presets.value.map(p => p.id))
  }
}

async function handleExport() {
  if (selectedIds.value.size === 0) return
  const ids = [...selectedIds.value]
  try {
    const path = await api.preparePresetsExport(ids)
    if (path) {
      selectMode.value = false
      selectedIds.value = new Set()
      alert('Saved: ' + path)
    }
  } catch (e) {
    alert('Export failed: ' + e)
  }
}

function requestBatchDelete() {
  if (selectedIds.value.size === 0) return
  showDeleteConfirm.value = true
}

async function confirmBatchDelete() {
  showDeleteConfirm.value = false
  const ids = [...selectedIds.value]
  for (const id of ids) {
    try {
      await api.deletePreset(id)
    } catch (e) {
      console.error('Delete failed for', id, e)
    }
  }
  selectedIds.value = new Set()
  selectMode.value = false
  await load()
}

async function handleOpenImport() {
  try {
    const result = await api.openImportFile()
    if (result && result.presets && result.presets.length > 0) {
      importPresets.value = result.presets
      showImport.value = true
    }
  } catch (e) {
    if (String(e)) alert('Import failed: ' + e)
  }
}

function handleImportDone() {
  showImport.value = false
  importPresets.value = []
  load()
}

onMounted(load)
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ mode === 'effects' ? t('presets.title_effects') : t('presets.title') }}</h1>
      <div class="header-actions">
        <template v-if="selectMode">
          <button class="btn btn-secondary btn-sm" @click="selectAll">
            {{ selectedCount === presets.length ? t('presets.btn_deselect_all') : t('presets.btn_select_all') }}
          </button>
          <button class="btn btn-primary btn-sm" @click="handleExport" :disabled="selectedCount === 0">
            {{ t('presets.btn_export', { count: selectedCount }) }}
          </button>
          <button class="btn btn-danger-quiet btn-sm" @click="requestBatchDelete" :disabled="selectedCount === 0">
            {{ t('presets.btn_delete_selected', { count: selectedCount }) }}
          </button>
          <button class="btn btn-secondary btn-sm" @click="toggleSelectMode">{{ t('presets.btn_cancel') }}</button>
        </template>
        <template v-else>
          <button class="btn btn-secondary" @click="handleOpenImport">{{ t('presets.btn_import') }}</button>
          <button class="btn btn-secondary" @click="toggleSelectMode" :disabled="presets.length === 0">{{ t('presets.btn_select') }}</button>
          <button class="btn btn-primary" @click="openCreate">{{ t('presets.btn_new') }}</button>
        </template>
      </div>
    </div>

    <div v-if="loading" style="padding: 40px; text-align: center;">
      <span class="spinner"></span>
    </div>

    <div v-else-if="modePresets.length === 0" class="empty-state">
      <div class="empty-state-icon"><LayoutGrid :size="40" :stroke-width="1.5" /></div>
      <p>{{ mode === 'effects' ? t('presets.no_effects') : t('presets.no_presets') }}</p>
    </div>

    <template v-else>
      <div class="styles-filters">
        <div class="styles-filter-row">
          <input class="form-input styles-search" data-testid="filter-search" v-model="filterSearch" :placeholder="t('presets.search_presets')" />
          <div class="styles-chips">
            <span class="styles-filter-label">{{ t('presets.filter_source') }}</span>
            <button class="styles-chip" data-testid="filter-source-all" :class="{ active: filterSource === '' }" @click="filterSource = ''">{{ t('presets.filter_all') }}</button>
            <button class="styles-chip" data-testid="filter-source-bundled" :class="{ active: filterSource === 'bundled' }" @click="filterSource = 'bundled'">{{ t('presets.filter_bundled') }}</button>
            <button class="styles-chip" data-testid="filter-source-custom" :class="{ active: filterSource === 'custom' }" @click="filterSource = 'custom'">{{ t('presets.filter_custom') }}</button>
          </div>
          <select class="form-select styles-group" data-testid="filter-group" v-model="filterGroup">
            <option value="">{{ t('presets.all_groups') }}</option>
            <option v-for="g in facets.groups" :key="g.value" :value="g.value">{{ groupLabel(g.value) }} ({{ g.count }})</option>
          </select>
          <label class="styles-unavailable">
            <input type="checkbox" v-model="showUnavailable" />
            {{ t('presets.show_unavailable') }}
          </label>
          <button v-if="filtersActive" class="btn btn-secondary btn-sm" data-testid="filter-reset" @click="resetFilters">{{ t('presets.filter_reset') }}</button>
        </div>
        <div class="styles-chips styles-models">
          <span class="styles-filter-label">{{ t('presets.filter_model') }}</span>
          <button
            v-for="m in facets.models"
            :key="m.label"
            class="styles-chip"
            data-testid="filter-model"
            :data-model="m.label"
            :class="{ active: filterModels.includes(m.label) }"
            @click="toggleModel(m.label)"
          >{{ modelText(m.label) }} <span class="styles-chip-count">{{ m.count }}</span></button>
        </div>
      </div>

      <div v-if="filteredPresets.length === 0" class="styles-empty" data-testid="no-match">
        <p>{{ t('presets.no_match') }}</p>
        <button v-if="filtersActive" class="btn btn-secondary btn-sm" @click="resetFilters">{{ t('presets.filter_reset') }}</button>
      </div>

      <div v-else class="styles-table-wrap">
        <table class="styles-table">
          <thead>
            <tr>
              <th v-if="selectMode" class="col-check"></th>
              <th>{{ t('presets.col_name') }}</th>
              <th>{{ t('presets.col_source') }}</th>
              <th>{{ t('presets.col_model') }}</th>
              <th>{{ t('presets.col_group') }}</th>
              <th class="col-num">{{ t('presets.col_steps') }}</th>
              <th class="col-num">CFG</th>
              <th class="col-actions"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in filteredPresets" :key="p.id" data-testid="style-row" :class="{ 'preset-selected': selectedIds.has(p.id) }">
              <td v-if="selectMode" class="col-check">
                <input type="checkbox" :checked="selectedIds.has(p.id)" @change="togglePreset(p.id)" />
              </td>
              <td class="col-name">
                <div class="styles-name">{{ p.name }}</div>
                <div class="styles-prompt" :title="p.prompt">{{ p.prompt || t('presets.no_prompt') }}</div>
                <div v-if="getStatus(p.id) && !getStatus(p.id).installed" class="styles-missing">
                  {{ t('presets.status_missing') }} {{ [...(getStatus(p.id).missing_sd || []), ...(getStatus(p.id).missing_lora || [])].join(', ') }}
                  <button v-if="!selectMode" class="btn btn-secondary btn-sm" @click="installDeps(p.id)" :disabled="installingId === p.id">
                    {{ installingId === p.id ? t('presets.btn_installing') : t('presets.btn_install') }}
                  </button>
                </div>
              </td>
              <td>
                <button
                  class="badge styles-badge"
                  data-testid="badge-source"
                  :class="presetSource(p) === 'bundled' ? 'badge-accent' : 'badge-success'"
                  :title="t('presets.filter_by')"
                  @click="filterSource = presetSource(p)"
                >{{ presetSource(p) === 'bundled' ? t('presets.source_bundled') : t('presets.source_custom') }}</button>
              </td>
              <td>
                <button class="badge badge-muted styles-badge" data-testid="badge-model" :title="p.model_name || t('presets.model_any')" @click="pickModel(modelLabel(p.model_name))">{{ modelText(modelLabel(p.model_name)) }}</button>
              </td>
              <td>
                <button class="badge badge-muted styles-badge" data-testid="badge-group" :title="t('presets.filter_by')" @click="pickGroup(p)">{{ groupLabel((p.preset_type || '').trim() || GROUP_NONE) }}</button>
              </td>
              <td class="col-num">{{ p.steps }}</td>
              <td class="col-num">{{ p.cfg_scale }}</td>
              <td class="col-actions">
                <template v-if="!selectMode">
                  <button class="styles-icon-btn" data-testid="row-edit" :title="t('presets.btn_edit')" :aria-label="t('presets.btn_edit')" @click="openEdit(p)"><Pencil :size="14" /></button>
                  <button class="styles-icon-btn" data-testid="row-duplicate" :title="t('presets.btn_duplicate')" :aria-label="t('presets.btn_duplicate')" @click="handleDuplicate(p)"><Copy :size="14" /></button>
                  <button
                    class="styles-icon-btn"
                    data-testid="row-delete"
                    :class="{ danger: pendingDeleteId === p.id }"
                    :title="pendingDeleteId === p.id ? t('presets.btn_sure') : t('presets.btn_delete')"
                    :aria-label="t('presets.btn_delete')"
                    @click="handleDelete(p.id)"
                  ><Trash2 :size="14" /><span v-if="pendingDeleteId === p.id" class="styles-sure">{{ t('presets.btn_sure') }}</span></button>
                </template>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <PresetForm
      v-if="showForm"
      :preset="editingPreset"
      :defaults="formDefaults"
      @save="handleSave"
      @close="closeForm"
    />

    <ImportModal
      v-if="showImport"
      :presets="importPresets"
      @done="handleImportDone"
      @close="showImport = false"
    />

    <div v-if="showDeleteConfirm" class="modal-overlay">
      <div class="modal">
        <div class="modal-header">
          <h2 class="modal-title">{{ t('presets.delete_title') }}</h2>
          <button class="modal-close" @click="showDeleteConfirm = false"><X :size="16" /></button>
        </div>
        <p>{{ t('presets.delete_confirm_n', { count: selectedCount }) }}</p>
        <p style="color: var(--text-dim); font-size: 0.9em;">{{ t('presets.delete_cannot_undo') }}</p>
        <div style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 16px;">
          <button class="btn btn-secondary" @click="showDeleteConfirm = false">{{ t('presets.btn_cancel') }}</button>
          <button class="btn btn-danger" @click="confirmBatchDelete">{{ t('presets.btn_delete') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.styles-filters { display: flex; flex-direction: column; gap: 8px; margin-bottom: 14px; }
.styles-filter-row { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; }
.styles-search { flex: 1 1 200px; max-width: 280px; }
.styles-group { width: auto; min-width: 150px; }
.styles-chips { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; }
.styles-filter-label { font-size: 11px; color: var(--text-dim); margin-right: 2px; }
.styles-chip {
  height: 24px; padding: 0 9px; border-radius: 12px; font-size: 12px; cursor: pointer;
  border: 1px solid var(--border); background: transparent; color: var(--text);
  display: inline-flex; align-items: center; gap: 5px;
}
.styles-chip:hover { border-color: var(--accent); }
.styles-chip.active { background: var(--accent-bg); border-color: var(--accent); color: var(--accent); }
.styles-chip-count { font-size: 10px; color: var(--text-dim); }
.styles-chip.active .styles-chip-count { color: var(--accent); }
.styles-unavailable { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--text-dim); cursor: pointer; }
.styles-empty { color: var(--text-dim); text-align: center; padding: 24px; display: flex; flex-direction: column; align-items: center; gap: 8px; }
.styles-table-wrap { overflow-x: auto; border: 1px solid var(--border); border-radius: var(--radius); }
.styles-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.styles-table th {
  text-align: left; font-size: 11px; font-weight: 600; color: var(--text-dim);
  padding: 8px 10px; border-bottom: 1px solid var(--border); position: sticky; top: 0; background: var(--surface);
}
.styles-table td { padding: 7px 10px; border-bottom: 1px solid var(--border); vertical-align: top; }
.styles-table tbody tr:last-child td { border-bottom: none; }
.styles-table tbody tr:hover { background: var(--accent-bg); }
.col-check { width: 28px; }
.col-num { text-align: right; white-space: nowrap; width: 48px; }
.col-actions { width: 96px; white-space: nowrap; text-align: right; }
.col-name { min-width: 180px; max-width: 420px; }
.styles-name { font-weight: 600; }
.styles-prompt { font-size: 11px; color: var(--text-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 400px; }
.styles-missing { font-size: 11px; color: var(--warning); display: flex; align-items: center; gap: 6px; margin-top: 3px; flex-wrap: wrap; }
.styles-badge { cursor: pointer; font-family: inherit; max-width: 200px; overflow: hidden; text-overflow: ellipsis; }
.styles-badge:hover { border-color: var(--accent); }
.styles-badge.badge-muted { background: transparent; }
.styles-icon-btn {
  display: inline-flex; align-items: center; gap: 4px; height: 26px; min-width: 26px; padding: 0 6px;
  border: none; border-radius: var(--radius-sm); background: transparent; color: var(--text-dim); cursor: pointer;
}
.styles-icon-btn:hover { color: var(--text); background: var(--accent-bg); }
.styles-icon-btn.danger { color: var(--danger); }
.styles-sure { font-size: 11px; }
</style>
