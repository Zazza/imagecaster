<script setup>
import { ref, inject } from 'vue'
import { X } from 'lucide-vue-next'
import StatusMessage from '../StatusMessage.vue'
import ToggleSwitch from '../ToggleSwitch.vue'
import DownloadProgress from '../DownloadProgress.vue'
import { api } from '../../api.js'
import {
  serverURL, serverModels, serverModelsLoading, modelActionLoading, activeDownload, activeLabel,
  loadServerModels, downloadModel, deleteModel, pullLLM, deleteLLMModel,
} from '../../serverModels.js'
import { t } from '../../i18n/index.js'

const { memoryForm, fastGeneration, serverModelDownload, llmPullName } = inject('settingsState')

const fastError = ref('')

async function onFastToggle(on) {
  const prev = fastGeneration.value
  fastGeneration.value = on
  fastError.value = ''
  try {
    await api.updateSettings({ fast_generation: on ? 'true' : 'false' })
  } catch (e) {
    console.error('save fast_generation:', e)
    fastGeneration.value = prev
    fastError.value = String(e)
  }
}

const memorySaved = ref(false)
const memoryError = ref('')

async function saveMemoryGuard() {
  memorySaved.value = false
  memoryError.value = ''
  try {
    await api.updateSettings({
      llm_auto_unload: memoryForm.llm_auto_unload ? 'true' : 'false',
    })
    memorySaved.value = true
  } catch (e) {
    memoryError.value = String(e)
  }
}

function formatBytes(bytes) {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

async function pullFromForm() {
  if (await pullLLM(llmPullName.value)) llmPullName.value = ''
}
</script>

<template>
  <div>
    <div class="card mb-4">
      <h3 class="text-bright mb-3">{{ t('fast.section') }}</h3>
      <StatusMessage v-if="fastError" class="mb-3" @close="fastError = ''">{{ fastError }}</StatusMessage>
      <div class="flex gap-3">
        <ToggleSwitch data-testid="settings-fast-generation" :modelValue="fastGeneration" @update:modelValue="onFastToggle" />
        <div class="flex-1">
          <div class="text-bright">{{ fastGeneration ? t('settings.enabled') : t('settings.disabled') }}</div>
          <div class="form-hint">{{ t('fast.hint') }}</div>
        </div>
      </div>
    </div>
    <div class="card mb-4">
      <h3 class="text-bright mb-3">{{ t('settings.section_memory_guard') }}</h3>
      <StatusMessage v-if="memorySaved" kind="success" :timeout="8000" class="mb-3" @close="memorySaved = false">{{ t('settings.saved_immediate') }}</StatusMessage>
      <StatusMessage v-if="memoryError" class="mb-3" @close="memoryError = ''">{{ memoryError }}</StatusMessage>
      <label class="check-label mb-3">
        <input data-testid="settings-llm-auto-unload" type="checkbox" v-model="memoryForm.llm_auto_unload" class="accent-check" />
        <span class="fs-md">{{ t('settings.llm_auto_unload') }}</span>
      </label>
      <div class="fs-xs text-dim mb-3">{{ t('settings.llm_auto_unload_hint') }}</div>
      <button class="btn btn-primary btn-sm" @click="saveMemoryGuard">{{ t('settings.btn_save_params') }}</button>
    </div>
      <div v-if="!serverURL" class="adv-placeholder text-dim">{{ t('settings.server_not_configured') }}</div>
      <template v-else>
        <div class="card mb-4">
          <div class="flex-between mb-3">
            <h3 class="text-bright">{{ t('settings.title_sd_models') }}</h3>
            <button class="btn btn-secondary btn-sm" @click="loadServerModels">{{ t('settings.btn_refresh') }}</button>
          </div>
          <div v-if="serverModelsLoading.sd" class="adv-placeholder"><span class="spinner"></span></div>
          <div v-else-if="serverModels.sd.length === 0" class="text-dim">{{ t('settings.no_models') }}</div>
          <div v-for="m in serverModels.sd" :key="m.name" class="flex-between adv-model-row">
            <div>
              <div class="text-bright fs-md">{{ m.name }}</div>
              <div class="text-dim fs-xs">{{ formatBytes(m.size) }}</div>
            </div>
            <button class="btn btn-sm btn-danger-quiet" @click="deleteModel('sd', m.name)" :disabled="modelActionLoading[`del_sd_${m.name}`]">{{ t('settings.btn_delete_model') }}</button>
          </div>
          <div class="flex gap-2 mt-3 adv-download">
            <div class="form-group m-0 adv-download-url">
              <input class="form-input fs-sm" v-model="serverModelDownload.url" placeholder="Download URL" />
            </div>
            <div class="form-group m-0 flex-1">
              <input class="form-input fs-sm" v-model="serverModelDownload.filename" placeholder="filename.safetensors" />
            </div>
            <button class="btn btn-primary btn-sm" @click="downloadModel('sd', serverModelDownload.url, serverModelDownload.filename)" :disabled="!serverModelDownload.url || !serverModelDownload.filename || modelActionLoading['dl_sd']">
              {{ modelActionLoading['dl_sd'] ? t('settings.btn_downloading') : t('settings.btn_download_model') }}
            </button>
          </div>
          <DownloadProgress v-if="activeDownload.type === 'sd' && !activeDownload.model" wide class="mt-2" :percent="activeDownload.percent" :text="activeDownload.progress" :label="activeLabel" />
        </div>

        <div class="card mb-4">
          <div class="flex-between mb-3">
            <h3 class="text-bright">LoRA</h3>
          </div>
          <div v-if="serverModelsLoading.lora" class="adv-placeholder"><span class="spinner"></span></div>
          <div v-else-if="serverModels.lora.length === 0" class="text-dim">{{ t('settings.no_loras') }}</div>
          <div v-else class="adv-chips">
            <span v-for="m in serverModels.lora" :key="m.name" class="adv-chip">
              <span class="fs-sm">{{ m.name }}</span>
              <button class="chip-remove" :title="t('settings.btn_delete_model')" :aria-label="t('settings.btn_delete_model')" @click="deleteModel('lora', m.name)"><X :size="12" /></button>
            </span>
          </div>
        </div>

        <div class="card">
          <div class="flex-between mb-3">
            <h3 class="text-bright">{{ t('settings.title_llm_models') }}</h3>
          </div>
          <div v-if="serverModelsLoading.llm" class="adv-placeholder"><span class="spinner"></span></div>
          <div v-else-if="serverModels.llm.length === 0" class="text-dim">{{ t('settings.no_models') }}</div>
          <div v-for="m in serverModels.llm" :key="m.name" class="flex-between adv-model-row">
            <span class="text-bright fs-md">{{ m.name }}</span>
            <button class="btn btn-sm btn-danger-quiet" @click="deleteLLMModel(m.name)" :disabled="modelActionLoading[`del_llm_${m.name}`]">{{ t('settings.btn_delete_model') }}</button>
          </div>
          <div class="flex gap-2 mt-3">
            <input class="form-input flex-1 fs-sm" v-model="llmPullName" placeholder="e.g. qwen3:8b" :disabled="modelActionLoading['pull_llm']" />
            <button class="btn btn-primary btn-sm" @click="pullFromForm" :disabled="!llmPullName || modelActionLoading['pull_llm']">
              {{ modelActionLoading['pull_llm'] ? t('settings.btn_pulling') : t('settings.btn_pull_llm') }}
            </button>
          </div>
          <DownloadProgress v-if="activeDownload.type === 'llm' && !activeDownload.model" wide class="mt-2" :percent="activeDownload.percent" :text="activeDownload.progress" :label="activeLabel" />
        </div>
      </template>
  </div>
</template>

<style scoped>
.adv-placeholder {
  padding: var(--sp-5);
  text-align: center;
}

.adv-model-row {
  padding: 6px 0;
  border-bottom: 1px solid var(--border);
}

.adv-download {
  align-items: flex-end;
}

.adv-download-url {
  flex: 2;
}

.adv-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.adv-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 4px;
}
</style>
