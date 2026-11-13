<script setup>
import { Check } from 'lucide-vue-next'
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '../api.js'
import { defaultDownloads, runDefaultPlan } from '../modelDownloads.js'
import {
  serverURL, serverModels, serverModelsKnown, modelActionLoading, activeDownload, activeLabel, modelPresets,
  onDownloadProgress, resetActiveDownload, hasServerFile, syncPresets, loadServerModels,
  downloadFile, downloadModel, deleteModel, pullLLM, deleteLLMModel,
} from '../serverModels.js'
import { t, locale } from '../i18n/index.js'
import DownloadProgress from './DownloadProgress.vue'

const props = defineProps({ active: { type: Boolean, default: true } })

const modelCatalog = ref(null)
const modelCatalogError = ref('')
const catalogNotice = ref('')
const defaultPlan = computed(() => defaultDownloads.plan)
const defaultLabel = computed(() => (defaultDownloads.steps
  ? t('settings.download_file_of', { n: defaultDownloads.step, total: defaultDownloads.steps, name: defaultDownloads.file })
  : ''))
const llmSections = [
  { key: 'llm_generate', title: 'settings.catalog_llm_prompt' },
  { key: 'llm_vision', title: 'settings.catalog_llm_vision' },
]

function catalogText(m, field) {
  return (locale.value === 'ru' && m[field + '_ru']) || m[field]
}

function isCatalogInstalled(type, name) {
  if (type === 'llm') return serverModels.llm.some(i => i.name === name)
  return serverModels[type].some(i => i.name === name + '.safetensors')
}

function sdMainFilename(m) {
  return m.filename || m.name + '.safetensors'
}

function missingModules(m) {
  return (m.modules || []).filter(mod => serverModelsKnown[mod.type] !== false && !hasServerFile(mod.type, mod.filename))
}

function unverifiedModules(m) {
  return (m.modules || []).filter(mod => serverModelsKnown[mod.type] === false)
}

function isSDModelInstalled(m) {
  return hasServerFile('sd', sdMainFilename(m)) && missingModules(m).length === 0 && unverifiedModules(m).length === 0
}

function isSDModelUnverified(m) {
  return hasServerFile('sd', sdMainFilename(m)) && missingModules(m).length === 0 && unverifiedModules(m).length > 0
}

async function downloadDefaultPlan() {
  const plan = defaultPlan.value
  if (!plan || !plan.items.length) return
  if (!confirm(t('settings.confirm_download_all', { count: plan.items.length, size: plan.total_gb }))) return
  catalogNotice.value = ''
  const failed = await runDefaultPlan(serverURL.value)
  await syncPresets()
  await loadServerModels()
  if (failed.length) alert(t('settings.err_download', { error: failed.join('\n') }))
  else catalogNotice.value = t('settings.download_all_done')
}

async function downloadSDModel(m) {
  modelActionLoading.dl_sd = true
  activeDownload.model = m.name
  catalogNotice.value = ''
  try {
    const files = (m.modules || []).filter(mod => !hasServerFile(mod.type, mod.filename))
      .map(mod => ({ type: mod.type, url: mod.url, filename: mod.filename }))
    if (!hasServerFile('sd', sdMainFilename(m))) files.push({ type: 'sd', url: m.url, filename: sdMainFilename(m) })
    activeDownload.steps = files.length
    for (const [i, f] of files.entries()) {
      activeDownload.type = f.type
      activeDownload.name = f.filename
      activeDownload.step = i + 1
      onDownloadProgress('', null)
      await downloadFile(f.type, f.url, f.filename)
    }
    const added = await syncPresets()
    if (added) catalogNotice.value = t('settings.catalog_presets_added', { model: m.name, count: added })
  } catch (e) {
    alert(t('settings.err_download', { error: String(e) }))
  } finally {
    await loadServerModels()
    delete modelActionLoading.dl_sd
    resetActiveDownload()
  }
}

watch(() => props.active, (on) => { if (on) loadServerModels() })

onMounted(async () => {
  try {
    modelCatalog.value = await api.getModelCatalog()
  } catch (e) {
    console.error('getModelCatalog:', e)
    modelCatalogError.value = String(e)
  }
})
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ t('app.nav_catalog') }}</h1>
    </div>
    <div>
      <div v-if="!modelCatalog" style="color: var(--text-dim); text-align: center; padding: 20px;">
        <template v-if="modelCatalogError">{{ t('settings.catalog_error', { error: modelCatalogError }) }}</template>
        <template v-else>{{ t('settings.catalog_loading') }}</template>
      </div>
      <template v-else>
        <div v-for="sec in llmSections" :key="sec.key" class="card cat-card">
          <h3 class="cat-title">{{ t(sec.title) }}</h3>
          <div v-for="m in modelCatalog[sec.key]" :key="m.name" class="cat-row">
            <div class="cat-info">
              <div class="cat-head">
                <span class="cat-name">{{ m.name }}</span>
                <span v-if="isCatalogInstalled('llm', m.name)" class="badge badge-success">{{ t('settings.installed') }}</span>
                <span v-if="m.recommended" class="badge badge-accent">{{ t('settings.recommended') }}</span>
              </div>
              <div class="cat-desc">{{ catalogText(m, 'description') }} &mdash; {{ m.size_gb }} {{ t('settings.unit_gb') }}</div>
            </div>
            <div class="cat-action">
              <DownloadProgress v-if="activeDownload.type === 'llm' && activeDownload.name === m.name" :percent="activeDownload.percent" :text="activeDownload.progress" :label="activeLabel" />
              <button v-else-if="isCatalogInstalled('llm', m.name)" class="btn btn-sm btn-danger-quiet" @click="deleteLLMModel(m.name)">{{ t('settings.btn_delete_model') }}</button>
              <button v-else class="btn btn-secondary btn-sm" @click="pullLLM(m.name)" :disabled="modelActionLoading['pull_llm']">{{ t('settings.btn_pull_llm') }}</button>
            </div>
          </div>
        </div>

        <div class="card cat-card">
          <h3 class="cat-title">{{ t('settings.title_sd_models') }}</h3>
          <div v-if="defaultPlan" class="cat-plan">
            <div class="cat-plan-text">
              <div v-if="defaultPlan.items.length">{{ t('settings.download_all_hint', { count: defaultPlan.items.length, size: defaultPlan.total_gb }) }}</div>
              <div v-else class="cat-ok">{{ t('settings.download_all_complete') }}</div>
              <div>{{ t('settings.download_all_optional') }}</div>
            </div>
            <div class="cat-action">
              <DownloadProgress v-if="defaultDownloads.running" :percent="defaultDownloads.percent" :text="defaultDownloads.progress" :label="defaultLabel" />
              <button v-else-if="defaultPlan.items.length" class="btn btn-primary btn-sm" @click="downloadDefaultPlan" :disabled="!!modelActionLoading['dl_sd']">{{ t('settings.btn_download_all') }}</button>
            </div>
          </div>
          <div v-if="catalogNotice" class="cat-notice">{{ catalogNotice }}</div>
          <div v-for="m in modelCatalog.sd_models" :key="m.name" class="cat-row">
            <div class="cat-info">
              <div class="cat-head">
                <span class="cat-name">{{ m.name }}</span>
                <span class="badge badge-muted">{{ m.base }}</span>
                <span v-if="isSDModelInstalled(m)" class="badge badge-success">{{ t('settings.installed') }}</span>
                <span v-else-if="isSDModelUnverified(m)" class="badge badge-warning">{{ t('settings.installed_unverified') }}</span>
                <span v-if="m.recommended" class="badge badge-accent">{{ t('settings.recommended') }}</span>
              </div>
              <div class="cat-desc">{{ catalogText(m, 'category') }} &mdash; {{ catalogText(m, 'description') }} &mdash; {{ m.size_gb }} {{ t('settings.unit_gb') }}</div>
              <div v-if="m.modules && m.modules.length" class="cat-extra">
                <span>{{ t('settings.catalog_modules') }}</span>
                <span v-for="mod in m.modules" :key="mod.filename" class="cat-module" :class="{ have: hasServerFile(mod.type, mod.filename) }">
                  <Check v-if="hasServerFile(mod.type, mod.filename)" :size="12" />{{ mod.filename }} ({{ mod.size_gb }} {{ t('settings.unit_gb') }})
                </span>
              </div>
              <details v-if="modelPresets[m.name] && modelPresets[m.name].length" class="catalog-model-presets cat-extra">
                <summary>{{ t('settings.catalog_model_presets', { count: modelPresets[m.name].length }) }}</summary>
                <div>{{ modelPresets[m.name].join(', ') }}</div>
              </details>
            </div>
            <div class="cat-action">
              <DownloadProgress v-if="activeDownload.model === m.name" :percent="activeDownload.percent" :text="activeDownload.progress" :label="activeLabel" />
              <button v-else-if="isSDModelInstalled(m) || isSDModelUnverified(m)" class="btn btn-sm btn-danger-quiet" @click="deleteModel('sd', sdMainFilename(m))">{{ t('settings.btn_delete_model') }}</button>
              <button v-else-if="m.url" class="btn btn-secondary btn-sm" @click="downloadSDModel(m)" :disabled="modelActionLoading['dl_sd'] || defaultDownloads.running">{{ t('settings.btn_download_model') }}</button>
            </div>
          </div>
        </div>

        <div class="card cat-card">
          <h3 class="cat-title">LoRA</h3>
          <div v-for="m in modelCatalog.lora" :key="m.name" class="cat-row">
            <div class="cat-info">
              <div class="cat-head">
                <span class="cat-name">{{ m.name }}</span>
                <span class="badge badge-muted">{{ m.base }}</span>
                <span v-if="isCatalogInstalled('lora', m.name)" class="badge badge-success">{{ t('settings.installed') }}</span>
              </div>
              <div class="cat-desc">{{ catalogText(m, 'category') }} &mdash; {{ catalogText(m, 'description') }} &mdash; {{ m.size_mb }} {{ t('settings.unit_mb') }}</div>
            </div>
            <div class="cat-action">
              <DownloadProgress v-if="activeDownload.type === 'lora' && activeDownload.name === m.name + '.safetensors'" :percent="activeDownload.percent" :text="activeDownload.progress" :label="activeLabel" />
              <button v-else-if="isCatalogInstalled('lora', m.name)" class="btn btn-sm btn-danger-quiet" @click="deleteModel('lora', m.name + '.safetensors')">{{ t('settings.btn_delete_model') }}</button>
              <button v-else-if="m.url" class="btn btn-secondary btn-sm" @click="downloadModel('lora', m.url, m.name + '.safetensors')" :disabled="modelActionLoading['dl_lora']">{{ t('settings.btn_download_model') }}</button>
            </div>
          </div>
        </div>
      </template>
    </div>

  </div>
</template>
