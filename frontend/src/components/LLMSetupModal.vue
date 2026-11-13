<script setup>
import { ref, computed } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import DownloadProgress from './DownloadProgress.vue'

const props = defineProps({ setup: { type: Object, required: true } })
const emit = defineEmits(['close'])

const generate = ref(props.setup.generate_selected || '')
const vision = ref(props.setup.vision_selected || '')
const busy = ref(false)
const error = ref('')
const progress = ref({ name: '', text: '', percent: null })

function find(list, name) {
  return (list || []).find(c => c.name === name)
}

function optionText(c) {
  if (c.installed) return `${c.name} — ${t('llm_setup.installed')}`
  return `${c.name} — ${t('llm_setup.download', { size: c.size_gb })}`
}

const toDownload = computed(() => {
  const names = []
  for (const [list, name] of [[props.setup.generate, generate.value], [props.setup.vision, vision.value]]) {
    const c = find(list, name)
    if (c && !c.installed && !names.includes(name)) names.push(name)
  }
  return names
})

const canSave = computed(() => !!generate.value && !!vision.value && !busy.value)

async function save() {
  if (!canSave.value) return
  busy.value = true
  error.value = ''
  try {
    for (const name of toDownload.value) {
      progress.value = { name, text: '', percent: null }
      await api.pullServerLLMModelStream('', name, (text, percent) => {
        progress.value = { name, text, percent }
      })
    }
    await api.saveLLMChoice(generate.value, vision.value)
    emit('close')
  } catch (e) {
    error.value = t('llm_setup.error', { error: e?.message || String(e) })
  } finally {
    busy.value = false
    progress.value = { name: '', text: '', percent: null }
  }
}
</script>

<template>
  <div class="modal-overlay">
    <div class="modal llm-setup-modal" role="dialog" aria-modal="true">
      <h2 class="welcome-title">{{ t('llm_setup.title') }}</h2>
      <p class="welcome-lead">{{ t('llm_setup.lead') }}</p>

      <label class="form-label" for="llm-setup-generate">{{ t('llm_setup.generate') }}</label>
      <select id="llm-setup-generate" v-model="generate" class="form-select" :disabled="busy">
        <option value="" disabled>{{ t('llm_setup.choose') }}</option>
        <option v-for="c in setup.generate" :key="c.name" :value="c.name">{{ optionText(c) }}</option>
      </select>

      <label class="form-label" for="llm-setup-vision">{{ t('llm_setup.vision') }}</label>
      <select id="llm-setup-vision" v-model="vision" class="form-select" :disabled="busy">
        <option value="" disabled>{{ t('llm_setup.choose') }}</option>
        <option v-for="c in setup.vision" :key="c.name" :value="c.name">{{ optionText(c) }}</option>
      </select>

      <DownloadProgress v-if="busy && progress.name" wide :percent="progress.percent" :text="progress.text" :label="progress.name" />
      <p v-if="error" class="welcome-lead err">{{ error }}</p>

      <div class="welcome-actions">
        <span class="welcome-spacer"></span>
        <button class="btn btn-secondary" :disabled="busy" @click="emit('close')">{{ t('llm_setup.later') }}</button>
        <button class="btn btn-primary" :disabled="!canSave" @click="save">
          {{ toDownload.length ? t('llm_setup.download_save') : t('llm_setup.save') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style>
.llm-setup-modal { width: min(560px, 92vw); max-width: 560px; display: flex; flex-direction: column; gap: 10px; }
.llm-setup-modal .form-select { width: 100%; }
</style>
