<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { formatBytes } from '../../downloadProgress.js'

const MIN_RETENTION = 1
const MAX_RETENTION = 50
const DEFAULT_RETENTION = 10
const settings = reactive({
  diagnostics_verbose_logs: false,
  diagnostics_intermediate_images: false,
  stylist_keep_originals: true,
  diagnostics_retention: String(DEFAULT_RETENTION),
})
const switches = [
  ['diagnostics_verbose_logs', 'diagnostics-verbose', 'settings.diagnostics_verbose'],
  ['diagnostics_intermediate_images', 'diagnostics-images', 'settings.diagnostics_images'],
  ['stylist_keep_originals', 'diagnostics-originals', 'settings.diagnostics_originals'],
]
const loaded = ref(false)
const busy = ref(false)
const error = ref('')
const saved = ref(false)
const confirmed = ref(false)
const storage = ref(null)
const blocked = computed(() => busy.value || !loaded.value)
const clearBlocked = computed(() => blocked.value || !storage.value || storage.value.active > 0 || !confirmed.value)

function validRetention(value) {
  const count = Number(value)
  return Number.isInteger(count) && count >= MIN_RETENTION && count <= MAX_RETENTION
}

async function loadStorage() {
  storage.value = await api.diagnosticsStorage()
}

onMounted(async () => {
  busy.value = true
  try {
    const values = await api.getSettings()
    settings.diagnostics_verbose_logs = values.diagnostics_verbose_logs === 'true'
    settings.diagnostics_intermediate_images = values.diagnostics_intermediate_images === 'true'
    settings.stylist_keep_originals = values.stylist_keep_originals !== 'false'
    settings.diagnostics_retention = validRetention(values.diagnostics_retention) ? String(Number(values.diagnostics_retention)) : String(DEFAULT_RETENTION)
    loaded.value = true
    await loadStorage()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
})

async function saveSetting(key, value) {
  if (blocked.value) return
  error.value = ''
  saved.value = false
  if (key === 'diagnostics_retention' && !validRetention(value)) {
    error.value = t('settings.diagnostics_retention_invalid')
    return
  }
  const previous = { ...settings }
  settings[key] = key === 'diagnostics_retention' ? String(Number(value)) : value
  busy.value = true
  try {
    await api.updateSettings({ [key]: String(settings[key]) })
    saved.value = true
  } catch (e) {
    Object.assign(settings, previous)
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function changeRetention(event) {
  await saveSetting('diagnostics_retention', event.target.value)
  event.target.value = settings.diagnostics_retention
}

async function refresh() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await loadStorage()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function clearStorage() {
  if (clearBlocked.value) return
  busy.value = true
  error.value = ''
  saved.value = false
  try {
    await api.clearDiagnostics(confirmed.value)
    confirmed.value = false
    await loadStorage()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function openFolder() {
  if (busy.value || !storage.value?.exists) return
  busy.value = true
  error.value = ''
  try {
    await api.openDiagnosticsFolder()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="card diagnostics-settings">
    <h3 class="text-bright">{{ t('settings.tab_diagnostics') }}</h3>
    <p class="text-dim">{{ t('settings.diagnostics_description') }}</p>
    <label v-for="[key, testid, label] in switches" :key="key" class="diagnostics-option">
      <input :data-testid="testid" type="checkbox" :checked="settings[key]" :disabled="blocked" @change="saveSetting(key, $event.target.checked)" />
      {{ t(label) }}
    </label>
    <label class="diagnostics-option">
      {{ t('settings.diagnostics_retention') }}
      <input class="form-input diagnostics-retention" data-testid="diagnostics-retention" type="number" :min="MIN_RETENTION" :max="MAX_RETENTION" :value="settings.diagnostics_retention" :disabled="blocked" @change="changeRetention" />
    </label>
    <p class="text-dim">{{ t('settings.diagnostics_preservation') }}</p>
    <p class="text-dim">{{ t('settings.diagnostics_memory_guard') }}</p>
    <p v-if="saved" role="status">{{ t('settings.diagnostics_saved') }}</p>
    <p v-if="error" data-testid="diagnostics-error" role="alert">{{ error }}</p>
    <dl v-if="storage" class="diagnostics-storage">
      <dt>{{ t('settings.diagnostics_path') }}</dt><dd>{{ storage.path }}</dd>
      <dt>{{ t('settings.diagnostics_size') }}</dt><dd>{{ formatBytes(storage.bytes) }}</dd>
      <dt>{{ t('settings.diagnostics_frames') }}</dt><dd>{{ storage.frames }}</dd>
      <dt>{{ t('settings.diagnostics_active') }}</dt><dd>{{ storage.active }}</dd>
      <template v-if="storage.orphans > 0"><dt>{{ t('settings.diagnostics_orphans') }}</dt><dd>{{ storage.orphans }}</dd></template>
    </dl>
    <div class="diagnostics-actions">
      <button data-testid="diagnostics-refresh" class="btn btn-secondary" :disabled="busy" @click="refresh">{{ t('settings.diagnostics_refresh') }}</button>
      <button data-testid="diagnostics-open" class="btn btn-secondary" :disabled="busy || !storage?.exists" @click="openFolder">{{ t('settings.diagnostics_open') }}</button>
    </div>
    <label class="diagnostics-option">
      <input data-testid="diagnostics-clear-confirm" v-model="confirmed" type="checkbox" :disabled="blocked" />
      {{ t('settings.diagnostics_clear_confirm') }}
    </label>
    <p v-if="storage?.active > 0" role="status">{{ t('settings.diagnostics_clear_active') }}</p>
    <button data-testid="diagnostics-clear" class="btn btn-danger" :disabled="clearBlocked" @click="clearStorage">{{ t('settings.diagnostics_clear') }}</button>
  </div>
</template>

<style scoped>
.diagnostics-settings { display: grid; gap: var(--sp-3); }
.diagnostics-option, .diagnostics-actions { display: flex; align-items: center; gap: var(--sp-2); }
.diagnostics-retention { width: auto; }
.diagnostics-storage { display: grid; grid-template-columns: auto 1fr; gap: var(--sp-2); }
.diagnostics-storage dd { margin: 0; overflow-wrap: anywhere; }
</style>
