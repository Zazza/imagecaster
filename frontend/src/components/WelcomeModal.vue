<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { t, locale } from '../i18n/index.js'
import { RELEASE_NOTES } from '../releaseNotes.js'
import { defaultDownloads, loadDefaultPlan, runDefaultPlan } from '../modelDownloads.js'
import DownloadProgress from './DownloadProgress.vue'

const props = defineProps({ tab: { type: String, default: 'about' } })
const emit = defineEmits(['close', 'settings'])

const cur = ref(props.tab === 'news' ? 'news' : 'about')
watch(() => props.tab, (v) => { cur.value = v === 'news' ? 'news' : 'about' })
const dontShow = ref(false)
const notes = RELEASE_NOTES[0]
const newsItems = computed(() => (notes && (notes[locale.value] || notes.ru)) || [])

const serverURL = ref('')
const serverOk = ref(null)
const planLoading = ref(false)
const plan = computed(() => defaultDownloads.plan)

async function refreshPlan() {
  planLoading.value = true
  try {
    const settings = await api.getSettings()
    serverURL.value = (settings && settings.server_url) || ''
  } catch {
    serverURL.value = ''
  }
  try {
    await api.getServerModels('sd')
    serverOk.value = true
  } catch {
    serverOk.value = false
  }
  if (serverOk.value) await loadDefaultPlan()
  planLoading.value = false
}

function onKey(e) { if (e.key === 'Escape') close() }
onMounted(() => {
  refreshPlan()
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => window.removeEventListener('keydown', onKey))

async function downloadAll() {
  await runDefaultPlan(serverURL.value)
}

const close = () => emit('close', dontShow.value)
const openSettings = () => emit('settings', dontShow.value)
</script>

<template>
  <div class="modal-overlay">
    <div class="modal welcome-modal" role="dialog" aria-modal="true">
      <div class="modal-header welcome-head">
        <img class="welcome-logo" src="../assets/logo.png" alt="" width="28" height="28">
        <div class="welcome-tabs">
          <button class="btn btn-secondary btn-sm" :class="{ on: cur === 'about' }" @click="cur = 'about'">{{ t('welcome.tab_about') }}</button>
          <button v-if="notes" class="btn btn-secondary btn-sm" :class="{ on: cur === 'news' }" @click="cur = 'news'">{{ t('welcome.tab_news') }} · {{ notes.version }}</button>
        </div>
        <button class="modal-close" :title="t('welcome.close')" @click="close">&times;</button>
      </div>

      <div v-if="cur === 'about'" class="welcome-body">
        <h2 class="welcome-title">{{ t('welcome.title') }}</h2>
        <p class="welcome-lead">{{ t('welcome.about_what') }}</p>
        <p class="welcome-lead">{{ t('welcome.about_how') }}</p>
        <p class="welcome-lead">{{ t('welcome.about_server') }}</p>
        <div class="welcome-models">
          <h3 class="welcome-subtitle">{{ t('welcome.models_title') }}</h3>
          <p class="welcome-lead">{{ t('welcome.models_lead') }}</p>
          <p v-if="planLoading" class="welcome-lead dim">{{ t('welcome.models_checking') }}</p>
          <template v-else-if="serverOk === false">
            <p class="welcome-lead">{{ t('welcome.models_no_server') }}</p>
            <button class="btn btn-primary btn-sm" @click="openSettings">{{ t('welcome.open_settings') }}</button>
          </template>
          <template v-else-if="plan">
            <DownloadProgress v-if="defaultDownloads.running" wide class="welcome-progress" :percent="defaultDownloads.percent" :text="defaultDownloads.progress"
              :label="defaultDownloads.steps ? t('settings.download_file_of', { n: defaultDownloads.step, total: defaultDownloads.steps, name: defaultDownloads.file }) : ''" />
            <template v-else-if="plan.items.length">
              <p class="welcome-lead">{{ t('welcome.models_missing', { count: plan.items.length, size: plan.total_gb }) }}</p>
              <button class="btn btn-primary btn-sm" @click="downloadAll">{{ t('settings.btn_download_all') }}</button>
            </template>
            <p v-else class="welcome-lead ok">{{ t('settings.download_all_complete') }}</p>
            <p v-if="defaultDownloads.finished && defaultDownloads.failed.length" class="welcome-lead err">
              {{ t('settings.err_download', { error: defaultDownloads.failed.join('; ') }) }}
            </p>
            <p class="welcome-lead dim">{{ t('welcome.models_optional') }}</p>
          </template>
        </div>
      </div>

      <div v-else class="welcome-body">
        <h2 class="welcome-title">{{ t('welcome.news_title', { v: notes.version }) }}</h2>
        <ul class="welcome-news">
          <li v-for="(item, i) in newsItems" :key="i">{{ item }}</li>
        </ul>
      </div>

      <div class="welcome-actions">
        <label class="welcome-dont"><input v-model="dontShow" type="checkbox"> {{ t('welcome.dont_show') }}</label>
        <span class="welcome-spacer"></span>
        <button class="btn btn-primary" @click="close">{{ t('welcome.ok') }}</button>
      </div>
    </div>
  </div>
</template>

<style>
.welcome-modal { width: min(620px, 92vw); max-width: 620px; display: flex; flex-direction: column; gap: 14px; }
.modal-header.welcome-head { gap: 10px; margin-bottom: 0; }
.welcome-logo { width: 28px; height: 28px; flex: none; }
.welcome-tabs { display: flex; gap: 6px; flex: 1; min-width: 0; }
.welcome-tabs .on { color: var(--text-bright); border-color: var(--accent); }
.welcome-body { max-height: 65vh; overflow-y: auto; }
.welcome-title { margin: 0 0 8px; font-size: 18px; font-weight: 600; color: var(--text-bright); }
.welcome-lead { margin: 0 0 12px; line-height: 1.55; }
.welcome-lead.dim { color: var(--text-dim); font-size: 13px; }
.welcome-lead.ok { color: var(--success); }
.welcome-lead.err { color: var(--danger); font-size: 12px; }
.welcome-progress { margin: 0 0 12px; }
.welcome-news { margin: 0; padding-left: 20px; display: flex; flex-direction: column; gap: 6px; line-height: 1.45; }
.welcome-models { margin-top: 4px; padding-top: 12px; border-top: 1px solid var(--border); }
.welcome-subtitle { margin: 0 0 6px; font-size: 15px; font-weight: 600; color: var(--text-bright); }
.welcome-actions { display: flex; align-items: center; gap: 8px; }
.welcome-actions .btn { min-width: 104px; }
.welcome-spacer { flex: 1; }
.welcome-dont { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; color: var(--text-dim); font-size: 13px; }
</style>
