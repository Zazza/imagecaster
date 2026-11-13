<script setup>
import { Check } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { api } from '../api.js'
import { defaultDownloads, runDefaultPlan } from '../modelDownloads.js'
import {
  serverURL, serverModels, serverModelsKnown, modelActionLoading, activeDownload, activeLabel, modelPresets,
  onDownloadProgress, resetActiveDownload, hasServerFile, syncPresets, loadServerModels,
  downloadFile, downloadModel, deleteModel, pullLLM, deleteLLMModel,
} from '../serverModels.js'
import {
  mainFilename, fitVariant, variantFits, installedFile, installedVariant, variantSwitchSteps, filterSDModels, modelBases,
} from '../catalogFilters.js'
import { t, locale } from '../i18n/index.js'
import DownloadProgress from './DownloadProgress.vue'

const props = defineProps({ active: { type: Boolean, default: true } })

const modelCatalog = ref(null)
const modelCatalogError = ref('')
const catalogNotice = ref('')
const activeTab = ref('images')
const vramMB = ref(0)
const filters = reactive({ query: '', base: '', installed: 'all', fits: false })
const chosen = reactive({})
const defaultPlan = computed(() => defaultDownloads.plan)
const defaultLabel = computed(() => (defaultDownloads.steps
  ? t('settings.download_file_of', { n: defaultDownloads.step, total: defaultDownloads.steps, name: defaultDownloads.file })
  : ''))
const llmSections = [
  { key: 'llm_generate', title: 'settings.catalog_llm_prompt' },
  { key: 'llm_vision', title: 'settings.catalog_llm_vision' },
]
const vramGB = computed(() => Math.round(vramMB.value / 1024 * 10) / 10)
const sdModels = computed(() => modelCatalog.value?.sd_models || [])
const bases = computed(() => modelBases(sdModels.value))
const visibleSDModels = computed(() => filterSDModels(sdModels.value, filters, { isInstalled: isSDModelPresent, vramMB: vramMB.value }))
const visibleNames = computed(() => new Set(visibleSDModels.value.map(m => m.name)))
const filtersActive = computed(() => !!(filters.query || filters.base || filters.installed !== 'all' || filters.fits))

function resetFilters() {
  filters.query = ''
  filters.base = ''
  filters.installed = 'all'
  filters.fits = false
}

function catalogText(m, field) {
  return (locale.value === 'ru' && m[field + '_ru']) || m[field]
}

function isCatalogInstalled(type, name) {
  if (type === 'llm') return serverModels.llm.some(i => i.name === name)
  return serverModels[type].some(i => i.name === name + '.safetensors')
}

function missingModules(m) {
  return (m.modules || []).filter(mod => serverModelsKnown[mod.type] !== false && !hasServerFile(mod.type, mod.filename))
}

function unverifiedModules(m) {
  return (m.modules || []).filter(mod => serverModelsKnown[mod.type] === false)
}

function isSDModelPresent(m) {
  return !!installedFile(m, serverModels.sd)
}

function isSDModelInstalled(m) {
  return isSDModelPresent(m) && missingModules(m).length === 0 && unverifiedModules(m).length === 0
}

function isSDModelUnverified(m) {
  return isSDModelPresent(m) && missingModules(m).length === 0 && unverifiedModules(m).length > 0
}

function hasVariants(m) {
  return Array.isArray(m.variants) && m.variants.length > 0
}

function currentVariant(m) {
  return installedVariant(m, serverModels.sd).variant
}

function isOtherVersion(m) {
  return hasVariants(m) && isSDModelPresent(m) && !currentVariant(m)
}

function selectedVariant(m) {
  if (!hasVariants(m)) return null
  const id = chosen[m.name]
  return m.variants.find(v => v.id === id) || currentVariant(m) || fitVariant(m, vramMB.value)
}

function needsReplace(m) {
  const v = selectedVariant(m)
  return !!v && isSDModelPresent(m) && currentVariant(m)?.id !== v.id
}

function selectVariant(m, id) {
  chosen[m.name] = id
}

function modelSize(m) {
  return selectedVariant(m)?.size_gb ?? m.size_gb
}

function variantOption(v) {
  const text = `${v.label} — ${v.size_gb} ${t('settings.unit_gb')}, ${t('settings.catalog_vram_need', { gb: v.vram_gb })}`
  return variantFits(v, vramMB.value) ? text : `${text} — ${t('settings.catalog_no_fit')}`
}

async function loadVRAM() {
  try {
    const status = await api.getServerStatus()
    vramMB.value = status?.gpu?.memory_total_mb || 0
  } catch (e) {
    console.warn('server status for VRAM', e)
    vramMB.value = 0
  }
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

function downloadSteps(m) {
  const v = selectedVariant(m)
  if (v) return variantSwitchSteps(m, v, serverModels)
  const steps = (m.modules || []).filter(mod => !hasServerFile(mod.type, mod.filename))
    .map(mod => ({ op: 'download', type: mod.type, url: mod.url, filename: mod.filename }))
  if (!hasServerFile('sd', mainFilename(m))) steps.push({ op: 'download', type: 'sd', url: m.url, filename: mainFilename(m) })
  return steps
}

async function downloadSDModel(m) {
  if (needsReplace(m) && !confirm(t('settings.confirm_replace_version', { name: m.name, label: selectedVariant(m).label }))) return
  modelActionLoading.dl_sd = true
  activeDownload.model = m.name
  catalogNotice.value = ''
  try {
    const steps = downloadSteps(m)
    const downloads = steps.filter(s => s.op === 'download')
    activeDownload.steps = downloads.length
    for (const s of steps) {
      if (s.op === 'delete') {
        await api.deleteServerModel(s.type, s.filename)
        continue
      }
      activeDownload.type = s.type
      activeDownload.name = s.filename
      activeDownload.step = downloads.indexOf(s) + 1
      onDownloadProgress('', null)
      await downloadFile(s.type, s.url, s.filename)
    }
    delete chosen[m.name]
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

function deleteSDModel(m) {
  const file = installedFile(m, serverModels.sd)
  deleteModel('sd', file ? file.name : mainFilename(m))
}

watch(() => props.active, (on) => {
  if (!on) return
  loadServerModels()
  loadVRAM()
})

onMounted(async () => {
  loadVRAM()
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
        <div class="tabs">
          <button class="tab" data-testid="catalog-tab-images" :class="{ active: activeTab === 'images' }" @click="activeTab = 'images'">{{ t('settings.title_sd_models') }}</button>
          <button class="tab" data-testid="catalog-tab-ai" :class="{ active: activeTab === 'ai' }" @click="activeTab = 'ai'">{{ t('settings.catalog_tab_ai') }}</button>
          <button class="tab" data-testid="catalog-tab-lora" :class="{ active: activeTab === 'lora' }" @click="activeTab = 'lora'">LoRA</button>
        </div>

        <div v-show="activeTab === 'ai'">
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
        </div>

        <div v-show="activeTab === 'images'" class="card cat-card">
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
          <div class="styles-filters cat-filters">
            <div class="styles-filter-row">
              <input class="form-input styles-search" data-testid="catalog-search" v-model="filters.query" :placeholder="t('settings.catalog_search')" />
              <select class="form-select styles-group" data-testid="catalog-base" v-model="filters.base">
                <option value="">{{ t('settings.catalog_all_bases') }}</option>
                <option v-for="b in bases" :key="b" :value="b">{{ b }}</option>
              </select>
              <button class="styles-chip" data-testid="catalog-installed-all" :class="{ active: filters.installed === 'all' }" @click="filters.installed = 'all'">{{ t('settings.catalog_filter_all') }}</button>
              <button class="styles-chip" data-testid="catalog-installed-yes" :class="{ active: filters.installed === 'yes' }" @click="filters.installed = 'yes'">{{ t('settings.catalog_filter_installed') }}</button>
              <button class="styles-chip" data-testid="catalog-installed-no" :class="{ active: filters.installed === 'no' }" @click="filters.installed = 'no'">{{ t('settings.catalog_filter_missing') }}</button>
              <button v-if="vramMB > 0" class="styles-chip" data-testid="catalog-fits" :class="{ active: filters.fits }" @click="filters.fits = !filters.fits">{{ t('settings.catalog_filter_fits', { gb: vramGB }) }}</button>
              <button v-if="filtersActive" class="btn btn-secondary btn-sm" data-testid="catalog-filter-reset" @click="resetFilters">{{ t('presets.filter_reset') }}</button>
            </div>
          </div>
          <div v-if="visibleSDModels.length === 0" class="styles-empty" data-testid="catalog-no-match">{{ t('settings.catalog_nothing_found') }}</div>
          <div v-for="m in sdModels" v-show="visibleNames.has(m.name)" :key="m.name" class="cat-row">
            <div class="cat-info">
              <div class="cat-head">
                <span class="cat-name">{{ m.name }}</span>
                <span class="badge badge-muted">{{ m.base }}</span>
                <span v-if="isOtherVersion(m)" class="badge badge-warning" data-testid="catalog-other-version">{{ t('settings.catalog_other_version') }}</span>
                <span v-else-if="isSDModelInstalled(m)" class="badge badge-success">{{ currentVariant(m) ? t('settings.installed') + ': ' + currentVariant(m).label : t('settings.installed') }}</span>
                <span v-else-if="isSDModelUnverified(m)" class="badge badge-warning">{{ t('settings.installed_unverified') }}</span>
                <span v-if="m.recommended" class="badge badge-accent">{{ t('settings.recommended') }}</span>
              </div>
              <div class="cat-desc">{{ catalogText(m, 'category') }} &mdash; {{ catalogText(m, 'description') }} &mdash; {{ modelSize(m) }} {{ t('settings.unit_gb') }}</div>
              <div v-if="hasVariants(m)" class="cat-extra cat-variant">
                <span>{{ t('settings.catalog_version') }}</span>
                <select class="form-select cat-variant-select" data-testid="catalog-variant" :value="selectedVariant(m).id" @change="selectVariant(m, $event.target.value)">
                  <option v-for="v in m.variants" :key="v.id" :value="v.id">{{ variantOption(v) }}</option>
                </select>
              </div>
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
              <template v-else>
                <button v-if="needsReplace(m) && m.url" class="btn btn-secondary btn-sm" data-testid="catalog-replace" @click="downloadSDModel(m)" :disabled="modelActionLoading['dl_sd'] || defaultDownloads.running">{{ t('settings.btn_replace_version', { label: selectedVariant(m).label }) }}</button>
                <button v-if="isSDModelPresent(m)" class="btn btn-sm btn-danger-quiet" @click="deleteSDModel(m)">{{ t('settings.btn_delete_model') }}</button>
                <button v-else-if="m.url" class="btn btn-secondary btn-sm" @click="downloadSDModel(m)" :disabled="modelActionLoading['dl_sd'] || defaultDownloads.running">{{ t('settings.btn_download_model') }}</button>
              </template>
            </div>
          </div>
        </div>

        <div v-show="activeTab === 'lora'" class="card cat-card">
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
