<script setup>
import { ref, computed, watch } from 'vue'
import { api } from '../api.js'
import { t, locale } from '../i18n/index.js'
import { splitHighlight } from '../highlight.js'
import { CornerDownLeft, FileText, Pencil, Trash2, X } from 'lucide-vue-next'
import { clampPerIdea, IDEAS_BATCH_MAX_PER_IDEA } from '../ideasBatch.js'

const props = defineProps({
  visible: { type: Boolean, default: false },
  descriptions: { type: Array, default: () => [] },
  createMode: { type: Boolean, default: false },
  createText: { type: String, default: '' },
  createNegative: { type: String, default: '' },
  batchTarget: { type: String, default: '' },
  batchRunning: { type: Boolean, default: false },
  batchProgress: { type: Object, default: null },
  batchDisabled: { type: String, default: '' },
})
const emit = defineEmits(['close', 'use', 'create', 'update', 'delete', 'batch', 'batch-stop'])

const search = ref('')
const typeFilter = ref('')
const sortBy = ref('new')
const editingId = ref(null)
const editName = ref('')
const editType = ref('')
const editNegative = ref('')
const editText = ref('')
const newText = ref('')
const newNegative = ref('')
const newName = ref('')
const newType = ref('')
const showCreate = ref(false)
const selected = ref(new Set())
const perIdea = ref(1)

watch(() => props.descriptions, (list) => {
  const ids = new Set((list || []).map(d => d.id))
  const next = new Set([...selected.value].filter(id => ids.has(id)))
  if (next.size !== selected.value.size) selected.value = next
})

const types = computed(() => {
  const set = new Set(props.descriptions.map(d => d.type).filter(Boolean))
  return [...set].sort()
})

function cmpTime(a, b) {
  const ca = a.created_at || ''
  const cb = b.created_at || ''
  if (ca !== cb) return ca < cb ? -1 : 1
  return (a.id || 0) - (b.id || 0)
}

const filtered = computed(() => {
  let list = props.descriptions
  const q = search.value.trim().toLowerCase()
  if (q) {
    list = list.filter(d =>
      d.text.toLowerCase().includes(q) ||
      (d.name || '').toLowerCase().includes(q) ||
      (d.type || '').toLowerCase().includes(q) ||
      (d.negative_prompt || '').toLowerCase().includes(q))
  }
  if (typeFilter.value) {
    list = list.filter(d => d.type === typeFilter.value)
  }
  const sorted = [...list]
  if (sortBy.value === 'name') {
    sorted.sort((a, b) => {
      const an = (a.name || '').trim()
      const bn = (b.name || '').trim()
      if (!an && bn) return 1
      if (an && !bn) return -1
      const c = an.localeCompare(bn, locale.value)
      return c !== 0 ? c : cmpTime(b, a)
    })
  } else {
    sorted.sort((a, b) => (sortBy.value === 'old' ? cmpTime(a, b) : cmpTime(b, a)))
  }
  return sorted
})

function displayName(desc) {
  if (desc.name) return desc.name
  const text = desc.text || ''
  return text.length > 40 ? text.slice(0, 40) + '…' : text
}

function fmtDate(iso) {
  if (!iso) return ''
  const d = new Date(String(iso).replace(' ', 'T'))
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString(locale.value === 'en' ? 'en-US' : 'ru-RU', { day: 'numeric', month: 'short' })
}

const selectedCount = computed(() => selected.value.size)
const allVisibleSelected = computed(() => filtered.value.length > 0 && filtered.value.every(d => selected.value.has(d.id)))
const perIdeaValue = computed(() => clampPerIdea(perIdea.value))
const batchTotal = computed(() => selectedCount.value * perIdeaValue.value)
const showBatchPanel = computed(() => selectedCount.value > 0 || props.batchRunning || (props.batchProgress?.errors?.length > 0) || !!props.batchProgress?.message)
const canRunBatch = computed(() => selectedCount.value > 0 && !props.batchRunning && !props.batchDisabled)

function toggleSelect(desc) {
  const next = new Set(selected.value)
  if (next.has(desc.id)) next.delete(desc.id)
  else next.add(desc.id)
  selected.value = next
}

function toggleSelectAll() {
  const next = new Set(selected.value)
  if (allVisibleSelected.value) filtered.value.forEach(d => next.delete(d.id))
  else filtered.value.forEach(d => next.add(d.id))
  selected.value = next
}

function onCardClick(desc, e) {
  if (editingId.value === desc.id) return
  if (e?.target?.closest?.('input, button, textarea, select, a')) return
  toggleSelect(desc)
}

function runBatch() {
  if (!canRunBatch.value) return
  const ids = sortedSelectedIds()
  emit('batch', { ids, perIdea: perIdeaValue.value })
}

function sortedSelectedIds() {
  const visible = filtered.value.filter(d => selected.value.has(d.id)).map(d => d.id)
  const hidden = props.descriptions.filter(d => selected.value.has(d.id) && !visible.includes(d.id)).map(d => d.id)
  return [...visible, ...hidden]
}

function useDesc(desc) {
  emit('use', desc)
  emit('close')
}

function searchEnter() {
  if (filtered.value.length) useDesc(filtered.value[0])
}

function startEdit(desc) {
  editingId.value = desc.id
  editText.value = desc.text || ''
  editName.value = desc.name || ''
  editType.value = desc.type || ''
  editNegative.value = desc.negative_prompt || ''
}

function saveEdit(desc) {
  emit('update', { ...desc, text: editText.value, name: editName.value, type: editType.value, negative_prompt: editNegative.value })
  editingId.value = null
}

function handleCreate() {
  if (!newText.value.trim()) return
  emit('create', {
    text: newText.value,
    name: newName.value,
    negative_prompt: newNegative.value,
    type: newType.value,
  })
  newText.value = ''
  newNegative.value = ''
  newName.value = ''
  newType.value = ''
  showCreate.value = false
}

function handleDelete(id) {
  emit('delete', id)
}
</script>

<template>
  <div v-if="visible" class="modal-overlay">
    <div class="modal" style="max-width: 860px;">
      <div class="modal-header">
        <h2 class="modal-title">{{ t('descriptions.title') }}</h2>
        <button class="modal-close" @click="$emit('close')"><X :size="16" /></button>
      </div>

      <div class="sdm-toolbar">
        <input class="form-input" v-model="search" :placeholder="t('descriptions.placeholder_search')"
          style="flex: 1; min-width: 160px;" @keydown.enter="searchEnter" />
        <span v-if="descriptions.length" class="sdm-count">
          {{ t('descriptions.found_of', { found: filtered.length, total: descriptions.length }) }}
        </span>
        <select class="form-select sdm-sort" v-model="sortBy">
          <option value="new">{{ t('descriptions.sort_new') }}</option>
          <option value="old">{{ t('descriptions.sort_old') }}</option>
          <option value="name">{{ t('descriptions.sort_name') }}</option>
        </select>
        <button class="btn btn-primary btn-sm" @click="showCreate = !showCreate">{{ showCreate ? t('descriptions.btn_cancel') : t('descriptions.btn_new') }}</button>
      </div>

      <div v-if="filtered.length" class="sdm-select-row">
        <label class="sdm-select-all">
          <input data-testid="sdm-select-all" type="checkbox" :checked="allVisibleSelected" @change="toggleSelectAll" />
          {{ t('descriptions.select_all') }}
        </label>
        <span class="sdm-count">{{ t('descriptions.selected') }} <span data-testid="sdm-selected-count">{{ selectedCount }}</span></span>
      </div>

      <div v-if="types.length > 0" class="style-markers" style="margin-bottom: 12px;">
        <span class="style-chip" :class="{ active: !typeFilter }" @click="typeFilter = ''">{{ t('descriptions.all') }}</span>
        <span v-for="ty in types" :key="ty" class="style-chip" :class="{ active: typeFilter === ty }" @click="typeFilter = ty">{{ ty }}</span>
      </div>

      <div v-if="showCreate" style="background: var(--surface-2); padding: 12px; border-radius: var(--radius-sm); margin-bottom: 12px;">
        <div class="form-group">
          <input class="form-input" v-model="newText" :placeholder="t('descriptions.placeholder_text')" />
        </div>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px;">
          <input class="form-input" v-model="newName" :placeholder="t('descriptions.placeholder_name')" />
          <input class="form-input" v-model="newType" :placeholder="t('descriptions.placeholder_tag')" />
        </div>
        <div class="form-group" style="margin-top: 8px;">
          <textarea class="form-textarea" v-model="newNegative" :placeholder="t('descriptions.placeholder_exclude')" rows="2"></textarea>
        </div>
        <button class="btn btn-primary btn-sm" @click="handleCreate" :disabled="!newText.trim()">{{ t('descriptions.btn_save') }}</button>
      </div>

      <div class="saved-modal-list">
        <div v-for="desc in filtered" :key="desc.id" class="saved-modal-item" :class="{ 'sdm-selected': selected.has(desc.id) }" @click="onCardClick(desc, $event)">
          <input data-testid="sdm-check" type="checkbox" class="sdm-check" :checked="selected.has(desc.id)" :aria-label="t('descriptions.select') + ': ' + displayName(desc)" @change="toggleSelect(desc)" />
          <div class="sdm-main">
            <div class="sdm-head">
              <span class="sdm-name">
                <template v-for="(seg, i) in splitHighlight(displayName(desc), search)" :key="i">
                  <mark v-if="seg.hit" class="sdm-mark">{{ seg.text }}</mark>
                  <template v-else>{{ seg.text }}</template>
                </template>
              </span>
              <span v-if="desc.type" class="preset-type">{{ desc.type }}</span>
              <span v-if="fmtDate(desc.created_at)" class="sdm-date">{{ fmtDate(desc.created_at) }}</span>
              <span class="sdm-actions">
                <button v-if="editingId !== desc.id" data-testid="sdm-use" class="btn btn-secondary btn-sm" :title="t('descriptions.btn_use')" @click.stop="useDesc(desc)">
                  <CornerDownLeft :size="12" /> {{ t('descriptions.btn_use') }}
                </button>
                <button v-if="editingId !== desc.id" class="btn btn-secondary btn-sm" :aria-label="t('descriptions.btn_edit')"
                  :title="t('descriptions.btn_edit')" @click.stop="startEdit(desc)">
                  <Pencil :size="12" />
                </button>
                <button class="btn btn-danger-quiet btn-sm" :aria-label="t('descriptions.btn_del')"
                  :title="t('descriptions.btn_del')" @click.stop="handleDelete(desc.id)">
                  <Trash2 :size="12" />
                </button>
              </span>
            </div>
            <div v-if="editingId !== desc.id" class="sdm-text">
              <template v-for="(seg, i) in splitHighlight(desc.text, search)" :key="i">
                <mark v-if="seg.hit" class="sdm-mark">{{ seg.text }}</mark>
                <template v-else>{{ seg.text }}</template>
              </template>
            </div>
            <div v-else class="sdm-edit" @click.stop>
              <textarea class="form-textarea" v-model="editText" :placeholder="t('descriptions.placeholder_text')" rows="3"></textarea>
              <input class="form-input" v-model="editName" :placeholder="t('descriptions.placeholder_name_edit')" />
              <input class="form-input" v-model="editType" :placeholder="t('descriptions.placeholder_type_edit')" />
              <textarea class="form-textarea" v-model="editNegative" :placeholder="t('descriptions.placeholder_exclude_edit')" rows="2"></textarea>
              <div style="display: flex; gap: 6px;">
                <button class="btn btn-primary btn-sm" @click.stop="saveEdit(desc)">{{ t('descriptions.btn_save') }}</button>
                <button class="btn btn-secondary btn-sm" @click.stop="editingId = null">{{ t('descriptions.btn_cancel') }}</button>
              </div>
            </div>
            <div v-if="desc.negative_prompt && editingId !== desc.id" class="sdm-neg">
              {{ t('descriptions.neg_label') }} {{ desc.negative_prompt }}
            </div>
          </div>
        </div>
        <div v-if="filtered.length === 0" class="empty-state">
          <div class="empty-state-icon"><FileText :size="40" :stroke-width="1.5" /></div>
          <div>{{ t('descriptions.no_saved') }}</div>
        </div>
      </div>

      <div v-if="showBatchPanel" class="sdm-batch">
        <div class="sdm-batch-row">
          <label class="sdm-batch-label">
            {{ t('descriptions.batch_per_idea') }}
            <input data-testid="sdm-per-idea" class="form-input sdm-per-idea" type="number" v-model.number="perIdea" min="1" :max="IDEAS_BATCH_MAX_PER_IDEA" step="1" :disabled="batchRunning" />
          </label>
          <span class="sdm-batch-total" data-testid="sdm-batch-total">{{ t('descriptions.batch_total', { n: batchTotal }) }}</span>
          <span v-if="batchTarget" class="sdm-batch-target" :title="batchTarget">{{ batchTarget }}</span>
          <button v-if="!batchRunning" data-testid="sdm-batch-run" class="btn btn-primary btn-sm sdm-batch-btn" :disabled="!canRunBatch" @click="runBatch">{{ t('descriptions.batch_run') }}</button>
          <button v-else data-testid="sdm-batch-stop" class="btn btn-secondary btn-sm sdm-batch-btn" @click="$emit('batch-stop')">{{ t('descriptions.batch_stop') }}</button>
        </div>
        <div v-if="batchDisabled && !batchRunning" class="sdm-batch-hint">{{ batchDisabled }}</div>
        <div v-if="batchRunning && batchProgress" class="sdm-batch-hint" data-testid="sdm-batch-progress">
          <span class="spinner" style="width: 11px; height: 11px; border-width: 2px;"></span>
          {{ t('descriptions.batch_progress', { done: batchProgress.done, total: batchProgress.total }) }}
        </div>
        <div v-if="!batchRunning && batchProgress?.message" class="sdm-batch-hint sdm-batch-ok">{{ batchProgress.message }}</div>
        <div v-if="batchProgress?.errors?.length" class="sdm-batch-errors" data-testid="sdm-batch-errors">
          <div>{{ t('descriptions.batch_errors') }}</div>
          <div v-for="(e, i) in batchProgress.errors" :key="i">{{ e.name }}: {{ e.error }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.saved-modal-list {
  max-height: 55vh;
}

.saved-modal-item {
  cursor: pointer;
  padding: 10px 8px;
  border-radius: var(--radius-sm);
}

.saved-modal-item.sdm-selected {
  background: var(--accent-bg);
}

.sdm-check {
  margin-top: 3px;
  flex-shrink: 0;
  accent-color: var(--accent);
  cursor: pointer;
}

.sdm-select-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}

.sdm-select-all {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  cursor: pointer;
  user-select: none;
}

.sdm-select-all input {
  accent-color: var(--accent);
}

.sdm-batch {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}

.sdm-batch-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.sdm-batch-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.sdm-per-idea {
  width: 64px;
}

.sdm-batch-total {
  font-size: 12px;
  color: var(--text-dim);
}

.sdm-batch-target {
  font-size: 12px;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1;
}

.sdm-batch-btn {
  margin-left: auto;
}

.sdm-batch-hint {
  margin-top: 6px;
  font-size: 11px;
  color: var(--text-dim);
  display: flex;
  align-items: center;
  gap: 6px;
}

.sdm-batch-ok {
  color: var(--success, var(--accent));
}

.sdm-batch-errors {
  margin-top: 6px;
  font-size: 11px;
  color: var(--danger, #e5484d);
  max-height: 80px;
  overflow-y: auto;
}

.sdm-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.sdm-count {
  font-size: 11px;
  color: var(--text-dim);
  white-space: nowrap;
}

.sdm-sort {
  width: auto;
  min-width: 110px;
}

.sdm-main {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}

.sdm-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.sdm-name {
  font-weight: 600;
  color: var(--text-bright);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sdm-date {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-dim);
  white-space: nowrap;
}

.sdm-actions {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}

.sdm-text {
  font-size: 13px;
  line-height: 1.5;
  color: var(--text);
  margin-top: 3px;
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.sdm-main:hover .sdm-text {
  color: var(--accent);
}

.sdm-neg {
  font-size: 11px;
  color: var(--text-dim);
  margin-top: 3px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sdm-edit {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
  cursor: default;
}

.sdm-mark {
  background: var(--accent-bg);
  color: inherit;
  border-radius: 2px;
}
</style>
