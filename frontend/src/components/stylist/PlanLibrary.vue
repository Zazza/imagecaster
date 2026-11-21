<script setup>
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { ChevronDown, Pencil, Trash2, Save } from 'lucide-vue-next'
import { api } from '../../api.js'
import { t, locale } from '../../i18n/index.js'
import {
  templateSnapshot, applyTemplate, planDirty, markPlanSaved, planSummary, currentPlanName,
} from '../../composables/useStylistPlan.js'

const props = defineProps({
  themes: { type: Array, default: () => [] },
  selectedThemes: { type: Array, default: () => [] },
  canSave: { type: Boolean, default: false },
  locked: { type: Boolean, default: false },
})

const plans = ref([])
const open = ref(false)
const saving = ref(false)
const name = ref('')
const replaceAsk = ref(false)
const busy = ref(false)
const error = ref('')
const renaming = ref(0)
const renameText = ref('')
const root = ref(null)
const nameInput = ref(null)

const keyOf = s => String(s || '').trim().split(/\s+/).join(' ').toLowerCase()

function shortDate(iso) {
  const d = iso ? new Date(iso) : new Date()
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString(locale.value === 'ru' ? 'ru-RU' : 'en-US', { day: 'numeric', month: 'short' }).replace('.', '')
}

function plural(n, forms) {
  if (locale.value !== 'ru') return n === 1 ? forms[0] : forms[1]
  const m10 = n % 10, m100 = n % 100
  if (m10 === 1 && m100 !== 11) return forms[0]
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return forms[1]
  return forms[2]
}

function meta(p) {
  const s = planSummary(p.plan)
  const series = `${s.series} ${plural(s.series, t('stylist.plans_series').split('|'))}`
  const frames = `${s.frames} ${plural(s.frames, t('stylist.plans_frames').split('|'))}`
  return [series, frames, shortDate(p.updated_at)].filter(Boolean).join(' · ')
}

const defaultName = computed(() => {
  if (currentPlanName.value) return currentPlanName.value
  const byId = Object.fromEntries(props.themes.map(th => [th.id, th.name]))
  const theme = props.selectedThemes.map(id => byId[id]).filter(Boolean).join(', ') || t('stylist.plans_default')
  return `${theme} · ${shortDate()}`
})

const existing = computed(() => plans.value.find(p => keyOf(p.name) === keyOf(name.value)))

async function load() {
  try {
    plans.value = (await api.stylistPlans()) || []
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

function toggle() {
  open.value = !open.value
  error.value = ''
  renaming.value = 0
  if (open.value) load()
}

async function startSave() {
  open.value = false
  error.value = ''
  replaceAsk.value = false
  name.value = defaultName.value
  saving.value = true
  load()
  await nextTick()
  nameInput.value?.select()
}

async function confirmSave() {
  const n = name.value.trim()
  if (!n || busy.value) return
  if (existing.value && !replaceAsk.value) {
    replaceAsk.value = true
    return
  }
  busy.value = true
  error.value = ''
  try {
    const saved = await api.stylistSaveNamedPlan(n, templateSnapshot(), !!existing.value)
    markPlanSaved(saved?.name || n)
    saving.value = false
    await load()
  } catch (e) {
    error.value = String(e?.message || e)
  } finally {
    busy.value = false
  }
}

function pick(p) {
  if (props.locked) return
  if (planDirty() && !window.confirm(t('stylist.plans_replace_unsaved', { name: p.name }))) return
  if (!applyTemplate(p.plan, p.name)) {
    error.value = t('stylist.plans_broken')
    return
  }
  open.value = false
}

function startRename(p) {
  renaming.value = p.id
  renameText.value = p.name
}

function cancelRename(p) {
  renameText.value = p.name
  renaming.value = 0
}

async function finishRename(p) {
  if (renaming.value !== p.id) return
  const n = renameText.value.trim()
  if (!n || n === p.name) {
    renaming.value = 0
    return
  }
  try {
    const saved = await api.stylistRenamePlan(p.id, n)
    if (keyOf(currentPlanName.value) === keyOf(p.name)) currentPlanName.value = saved?.name || n
    renaming.value = 0
    await load()
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

async function remove(p) {
  if (!window.confirm(t('stylist.plans_delete_confirm', { name: p.name }))) return
  try {
    await api.stylistDeletePlan(p.id)
    if (keyOf(currentPlanName.value) === keyOf(p.name)) currentPlanName.value = ''
    await load()
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

function onOutside(e) {
  if (open.value && root.value && !root.value.contains(e.target)) open.value = false
}

onMounted(() => document.addEventListener('pointerdown', onOutside))
onUnmounted(() => document.removeEventListener('pointerdown', onOutside))
</script>

<template>
  <div ref="root" class="plan-lib">
    <button class="btn btn-sm btn-secondary" data-testid="plans-save" :disabled="!canSave || locked" :title="t('stylist.plans_save_hint')" @click="startSave"><Save :size="14" /> {{ t('stylist.plans_save') }}</button>
    <button class="btn btn-sm btn-secondary" data-testid="plans-open" :aria-expanded="open" @click="toggle">{{ t('stylist.plans_my') }} <ChevronDown :size="14" /></button>

    <div v-if="open" class="plan-lib-menu" data-testid="plans-menu">
      <div v-if="error" class="plan-lib-error" data-testid="plans-error">{{ error }}</div>
      <div v-if="!plans.length" class="plan-lib-empty" data-testid="plans-empty">{{ t('stylist.plans_empty') }}</div>
      <div v-for="p in plans" :key="p.id" class="plan-lib-row" :class="{ current: keyOf(p.name) === keyOf(currentPlanName) }" data-testid="plans-row">
        <template v-if="renaming === p.id">
          <input class="form-input plan-lib-rename" v-model="renameText" maxlength="80" data-testid="plans-rename-input"
            @keydown.enter.prevent="finishRename(p)" @keydown.esc.stop.prevent="cancelRename(p)" @blur="finishRename(p)" />
        </template>
        <button v-else class="plan-lib-pick" :disabled="locked" data-testid="plans-pick" @click="pick(p)">
          <span class="plan-lib-name">{{ p.name }}</span>
          <span class="plan-lib-meta" data-testid="plans-meta">{{ meta(p) }}</span>
        </button>
        <div class="plan-lib-tools">
          <button class="styles-icon-btn" :title="t('stylist.plans_rename')" :aria-label="t('stylist.plans_rename')" data-testid="plans-rename" @click="startRename(p)"><Pencil :size="14" /></button>
          <button class="styles-icon-btn danger" :title="t('stylist.plans_delete')" :aria-label="t('stylist.plans_delete')" data-testid="plans-delete" @click="remove(p)"><Trash2 :size="14" /></button>
        </div>
      </div>
    </div>

    <div v-if="saving" class="modal-overlay" @pointerdown.self="saving = false">
      <div class="modal-content plan-lib-dialog" role="dialog" aria-modal="true" data-testid="plans-dialog" @keydown.esc.stop.prevent="saving = false">
        <div class="card-title">{{ t('stylist.plans_save_title') }}</div>
        <label class="form-label" for="plan-lib-name">{{ t('stylist.plans_name') }}</label>
        <input id="plan-lib-name" ref="nameInput" class="form-input" v-model="name" maxlength="80" data-testid="plans-name"
          @input="replaceAsk = false" @keydown.enter.prevent="confirmSave" />
        <div v-if="replaceAsk" class="plan-lib-ask" data-testid="plans-replace-ask">{{ t('stylist.plans_exists', { name: existing?.name || name }) }}</div>
        <div v-if="error" class="plan-lib-error" data-testid="plans-error">{{ error }}</div>
        <div class="plan-lib-actions">
          <button class="btn btn-primary" :disabled="!name.trim() || busy" data-testid="plans-confirm" @click="confirmSave">{{ replaceAsk ? t('stylist.plans_replace') : t('stylist.plans_save') }}</button>
          <button class="btn btn-secondary" data-testid="plans-cancel" @click="saving = false">{{ t('stylist.plans_cancel') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.plan-lib { position: relative; display: flex; gap: 6px; margin-left: auto; }
.plan-lib-menu {
  position: absolute; top: calc(100% + 6px); right: 0; z-index: 50;
  width: 340px; max-height: 420px; overflow-y: auto; padding: 6px;
  background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
}
.plan-lib-empty { padding: 12px; color: var(--text-dim); font-size: var(--fs-sm); line-height: 1.4; }
.plan-lib-row { display: flex; align-items: center; gap: 4px; border-radius: calc(var(--radius) - 2px); }
.plan-lib-row:hover { background: var(--surface-2); }
.plan-lib-row.current .plan-lib-name { color: var(--accent); }
.plan-lib-pick {
  flex: 1; min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 2px;
  padding: 8px 10px; background: none; border: 0; color: var(--text); text-align: left; cursor: pointer;
}
.plan-lib-pick:disabled { cursor: default; opacity: 0.6; }
.plan-lib-name { font-weight: 600; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.plan-lib-meta { font-size: var(--fs-sm); color: var(--text-dim); }
.plan-lib-tools { display: flex; gap: 2px; padding-right: 6px; opacity: 0.6; }
.plan-lib-row:hover .plan-lib-tools { opacity: 1; }
.plan-lib-rename { flex: 1; margin: 4px; }
.plan-lib-dialog { width: 380px; max-width: calc(100vw - 32px); display: flex; flex-direction: column; gap: 8px; }
.plan-lib-ask { font-size: var(--fs-sm); color: var(--warning); }
.plan-lib-error { font-size: var(--fs-sm); color: var(--danger); padding: 4px 8px; }
.plan-lib-actions { display: flex; gap: 8px; margin-top: 4px; }
</style>
