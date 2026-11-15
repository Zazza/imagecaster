<script setup>
import { ref, inject } from 'vue'
import { X } from 'lucide-vue-next'
import StatusMessage from '../StatusMessage.vue'
import DownloadProgress from '../DownloadProgress.vue'
import { api } from '../../api.js'
import {
  serverURL, serverModels, serverModelsLoading, modelActionLoading, activeDownload, activeLabel,
  loadServerModels, downloadModel, deleteModel, pullLLM, deleteLLMModel,
} from '../../serverModels.js'
import { t } from '../../i18n/index.js'

const { memoryForm, serverModelDownload, llmPullName } = inject('settingsState')

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
    <div class="card" style="margin-bottom: 16px;">
      <h3 style="color: var(--text-bright); margin-bottom: 12px;">{{ t('settings.section_memory_guard') }}</h3>
      <StatusMessage v-if="memorySaved" kind="success" :timeout="8000" style="margin-bottom: 12px;" @close="memorySaved = false">{{ t('settings.saved_immediate') }}</StatusMessage>
      <StatusMessage v-if="memoryError" style="margin-bottom: 12px;" @close="memoryError = ''">{{ memoryError }}</StatusMessage>
      <label style="display: flex; align-items: center; gap: 6px; cursor: pointer; margin-bottom: 12px;">
        <input data-testid="settings-llm-auto-unload" type="checkbox" v-model="memoryForm.llm_auto_unload" style="accent-color: var(--accent);" />
        <span style="font-size: 13px;">{{ t('settings.llm_auto_unload') }}</span>
      </label>
      <div style="font-size: 11px; color: var(--text-dim); margin-bottom: 12px;">{{ t('settings.llm_auto_unload_hint') }}</div>
      <button class="btn btn-primary btn-sm" @click="saveMemoryGuard">{{ t('settings.btn_save_params') }}</button>
    </div>
      <div v-if="!serverURL" style="color: var(--text-dim); padding: 20px; text-align: center;">{{ t('settings.server_not_configured') }}</div>
      <template v-else>
        <div class="card" style="margin-bottom: 16px;">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
            <h3 style="color: var(--text-bright);">{{ t('settings.title_sd_models') }}</h3>
            <button class="btn btn-secondary btn-sm" @click="loadServerModels">{{ t('settings.btn_refresh') }}</button>
          </div>
          <div v-if="serverModelsLoading.sd" style="text-align: center; padding: 20px;"><span class="spinner"></span></div>
          <div v-else-if="serverModels.sd.length === 0" style="color: var(--text-dim);">{{ t('settings.no_models') }}</div>
          <div v-for="m in serverModels.sd" :key="m.name" style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border);">
            <div>
              <div style="color: var(--text-bright); font-size: 13px;">{{ m.name }}</div>
              <div style="color: var(--text-dim); font-size: 11px;">{{ formatBytes(m.size) }}</div>
            </div>
            <button class="btn btn-sm btn-danger-quiet" @click="deleteModel('sd', m.name)" :disabled="modelActionLoading[`del_sd_${m.name}`]">{{ t('settings.btn_delete_model') }}</button>
          </div>
          <div style="display: flex; gap: 8px; margin-top: 12px; align-items: flex-end;">
            <div class="form-group" style="flex: 2; margin: 0;">
              <input class="form-input" v-model="serverModelDownload.url" placeholder="Download URL" style="font-size: 12px;" />
            </div>
            <div class="form-group" style="flex: 1; margin: 0;">
              <input class="form-input" v-model="serverModelDownload.filename" placeholder="filename.safetensors" style="font-size: 12px;" />
            </div>
            <button class="btn btn-primary btn-sm" @click="downloadModel('sd', serverModelDownload.url, serverModelDownload.filename)" :disabled="!serverModelDownload.url || !serverModelDownload.filename || modelActionLoading['dl_sd']">
              {{ modelActionLoading['dl_sd'] ? t('settings.btn_downloading') : t('settings.btn_download_model') }}
            </button>
          </div>
          <DownloadProgress v-if="activeDownload.type === 'sd' && !activeDownload.model" wide style="margin-top: 8px;" :percent="activeDownload.percent" :text="activeDownload.progress" :label="activeLabel" />
        </div>

        <div class="card" style="margin-bottom: 16px;">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
            <h3 style="color: var(--text-bright);">LoRA</h3>
          </div>
          <div v-if="serverModelsLoading.lora" style="text-align: center; padding: 20px;"><span class="spinner"></span></div>
          <div v-else-if="serverModels.lora.length === 0" style="color: var(--text-dim);">{{ t('settings.no_loras') }}</div>
          <div v-else style="display: flex; flex-wrap: wrap; gap: 6px;">
            <span v-for="m in serverModels.lora" :key="m.name" style="display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 4px;">
              <span style="font-size: 12px; color: var(--text);">{{ m.name }}</span>
              <button class="chip-remove" :title="t('settings.btn_delete_model')" :aria-label="t('settings.btn_delete_model')" @click="deleteModel('lora', m.name)"><X :size="12" /></button>
            </span>
          </div>
        </div>

        <div class="card">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
            <h3 style="color: var(--text-bright);">{{ t('settings.title_llm_models') }}</h3>
          </div>
          <div v-if="serverModelsLoading.llm" style="text-align: center; padding: 20px;"><span class="spinner"></span></div>
          <div v-else-if="serverModels.llm.length === 0" style="color: var(--text-dim);">{{ t('settings.no_models') }}</div>
          <div v-for="m in serverModels.llm" :key="m.name" style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border);">
            <span style="color: var(--text-bright); font-size: 13px;">{{ m.name }}</span>
            <button class="btn btn-sm btn-danger-quiet" @click="deleteLLMModel(m.name)" :disabled="modelActionLoading[`del_llm_${m.name}`]">{{ t('settings.btn_delete_model') }}</button>
          </div>
          <div style="display: flex; gap: 8px; margin-top: 12px;">
            <input class="form-input" v-model="llmPullName" placeholder="e.g. qwen3:8b" style="flex: 1; font-size: 12px;" :disabled="modelActionLoading['pull_llm']" />
            <button class="btn btn-primary btn-sm" @click="pullFromForm" :disabled="!llmPullName || modelActionLoading['pull_llm']">
              {{ modelActionLoading['pull_llm'] ? t('settings.btn_pulling') : t('settings.btn_pull_llm') }}
            </button>
          </div>
          <DownloadProgress v-if="activeDownload.type === 'llm' && !activeDownload.model" wide style="margin-top: 8px;" :percent="activeDownload.percent" :text="activeDownload.progress" :label="activeLabel" />
        </div>
      </template>
  </div>
</template>
