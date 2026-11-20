<script setup>
import { ref, computed, nextTick } from 'vue'
import { Pencil, X, Plus } from 'lucide-vue-next'
import SavePresetForm from './SavePresetForm.vue'
import { t } from '../../i18n/index.js'
import { PRESET_NAME_MAX } from '../../composables/useStylistPlan.js'

const props = defineProps({
  aspect: { type: String, required: true },
  choice: { type: Object, required: true },
  items: { type: Array, default: () => [] },
  themes: { type: Array, default: () => [] },
  places: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:choice', 'rename', 'delete', 'save-preset'])
const adding = ref(false)

function onAdd(payload) {
  adding.value = false
  emit('save-preset', payload)
}

const MODES = ['auto', 'keep', 'pick', 'custom']
const POSE_MODES = ['auto', 'keep', 'pick']
const CUSTOM_TEXT_MAX = 500

const groups = computed(() => {
  const out = new Map()
  const themeName = Object.fromEntries(props.themes.map(th => [th.id, th.name]))
  for (const it of props.items) {
    if (it.user) continue
    let key = ''
    if (props.aspect === 'pose') key = t('stylist.framing_' + it.framing)
    else if (it.theme) key = themeName[it.theme] || it.theme
    if (!out.has(key)) out.set(key, [])
    out.get(key).push(it)
  }
  return [...out.entries()].map(([name, items]) => ({ name, items }))
})

const modes = computed(() => props.aspect === 'pose' ? POSE_MODES : MODES)
const userItems = computed(() => props.items.filter(it => it.user))
const editing = ref('')
const editName = ref('')
const confirmDelete = ref('')

async function startRename(it) {
  confirmDelete.value = ''
  editing.value = it.id
  editName.value = it.name
  await nextTick()
  document.querySelector(`[data-testid="user-name-${it.id}"]`)?.focus()
}

function finishRename() {
  const id = editing.value
  const name = editName.value.trim()
  editing.value = ''
  if (id && name) emit('rename', id, name)
}

function cancelRename() {
  editing.value = ''
}

function askDelete(id) {
  if (confirmDelete.value === id) {
    confirmDelete.value = ''
    emit('delete', id)
    return
  }
  confirmDelete.value = id
}

function setMode(mode) {
  emit('update:choice', { mode, ids: props.choice.ids || [], text: props.choice.text })
}

function setText(text) {
  emit('update:choice', { mode: 'custom', ids: props.choice.ids || [], text })
}

function toggle(id) {
  const ids = props.choice.ids || []
  const next = ids.includes(id) ? ids.filter(x => x !== id) : [...ids, id]
  emit('update:choice', { mode: 'pick', ids: next, text: props.choice.text })
}
</script>

<template>
  <div class="aspect-tab" :data-testid="'aspect-' + aspect">
      <div class="aspect-modes">
        <button v-for="m in modes" :key="m" type="button" class="btn btn-sm" :class="choice.mode === m ? 'btn-primary' : 'btn-secondary'" :aria-pressed="choice.mode === m ? 'true' : 'false'" :data-testid="'mode-' + m" @click="setMode(m)">{{ t('stylist.mode_' + m) }}</button>
      </div>
      <div class="form-hint">{{ t('stylist.mode_hint_' + (aspect === 'pose' ? 'pose_' : '') + choice.mode) }}</div>
      <input v-if="choice.mode === 'custom'" class="form-input" type="text" :value="choice.text || ''" :maxlength="CUSTOM_TEXT_MAX" :placeholder="t('stylist.custom_placeholder_' + aspect)" data-testid="custom-text" @change="setText($event.target.value)" />
      <div v-if="choice.mode === 'pick'" class="aspect-groups">
        <div v-for="g in groups" :key="g.name" class="aspect-group">
          <div v-if="g.name" class="styles-filter-label">{{ g.name }}</div>
          <div class="styles-chips">
            <button v-for="it in g.items" :key="it.id" type="button" class="styles-chip" :class="{ active: (choice.ids || []).includes(it.id) }" :title="it.prompt" :data-testid="'pick-' + it.id" @click="toggle(it.id)">{{ it.name }}</button>
          </div>
        </div>
        <div class="aspect-group" data-testid="user-group">
          <div class="user-head">
            <span class="styles-filter-label">{{ t('stylist.user_group') }}</span>
            <button v-if="!adding" type="button" class="btn btn-secondary btn-xs" data-testid="user-add" @click="adding = true"><Plus :size="12" /> {{ t('stylist.user_add') }}</button>
          </div>
          <SavePresetForm v-if="adding" :key="'add-' + aspect" :preset="{ aspect, places: [] }" :places="places" with-text @save="onAdd" @cancel="adding = false" />
          <div v-if="!userItems.length && !adding" class="form-hint" data-testid="user-empty">{{ t('stylist.user_empty') }}</div>
          <div class="styles-chips">
            <span v-for="it in userItems" :key="it.id" class="user-chip">
              <input v-if="editing === it.id" v-model="editName" class="form-input user-name" type="text" :maxlength="PRESET_NAME_MAX" :data-testid="'user-name-' + it.id" @keydown.enter.prevent="finishRename" @keydown.esc.prevent="cancelRename" @blur="cancelRename" />
              <button v-else type="button" class="styles-chip" :class="{ active: (choice.ids || []).includes(it.id) }" :title="it.prompt" :data-testid="'pick-' + it.id" @click="toggle(it.id)">{{ it.name }}</button>
              <button type="button" class="btn btn-secondary btn-xs" :title="t('stylist.preset_rename')" :aria-label="t('stylist.preset_rename')" :data-testid="'user-rename-' + it.id" @click="startRename(it)"><Pencil :size="12" /></button>
              <button type="button" class="btn btn-danger-quiet btn-xs" :title="t('stylist.preset_delete')" :data-testid="'user-delete-' + it.id" @click="askDelete(it.id)"><template v-if="confirmDelete === it.id">{{ t('stylist.preset_delete_confirm') }}</template><X v-else :size="12" :aria-label="t('stylist.preset_delete')" /></button>
            </span>
          </div>
        </div>
      </div>
  </div>
</template>

<style scoped>
.aspect-tab { display: flex; flex-direction: column; gap: 8px; }
.aspect-modes { display: flex; gap: 6px; flex-wrap: wrap; }
.aspect-groups { display: flex; flex-direction: column; gap: 8px; }
.aspect-group { display: flex; flex-direction: column; gap: 4px; }
.user-head { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
.user-head .styles-filter-label { margin: 0; }
.user-chip { display: inline-flex; gap: 2px; align-items: center; }
.user-name { width: 160px; padding: 2px 6px; }
</style>
