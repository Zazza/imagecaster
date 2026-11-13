<script setup>
import { X } from 'lucide-vue-next'
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { t, locale } from '../i18n/index.js'
import { BrowserOpenURL } from '../wailsjs/runtime/runtime'
import { serverState, checkServer, useServer, SERVER_INSTALL_URL } from '../serverState.js'

const emit = defineEmits(['close', 'ready'])

const found = ref([])
const searching = ref(false)
const searched = ref(false)
const address = ref(serverState.url || '')
const busy = ref(false)
const outdated = computed(() => serverState.state === 'outdated')
const outdatedKind = computed(() => ({ old_version: 'old', no_sdcpp: 'forge' })[serverState.reason] || 'incompatible')
const FEATURE_KEYS = { sdcpp: 'server.feature_sdcpp', download_stream: 'server.feature_download_stream', llamacpp: 'server.feature_llm', sd_jobs: 'server.feature_sd_jobs' }
const missingText = computed(() => [...new Set(serverState.missing.map(f => FEATURE_KEYS[f] ? t(FEATURE_KEYS[f]) : f))].join(', '))

async function search() {
  searching.value = true
  searched.value = false
  found.value = []
  try {
    found.value = (await api.discoverServers()) || []
  } catch (e) {
    console.warn('discoverServers', e)
  } finally {
    searching.value = false
    searched.value = true
  }
}

async function apply(url) {
  busy.value = true
  try {
    if (await useServer(url) === 'ok') emit('ready')
  } catch (e) {
    serverState.error = String(e)
  } finally {
    busy.value = false
  }
}

async function retry() {
  busy.value = true
  try {
    if (await checkServer() === 'ok') emit('ready')
  } finally {
    busy.value = false
  }
}

function openInstall() {
  BrowserOpenURL(SERVER_INSTALL_URL[locale.value] || SERVER_INSTALL_URL.en)
}

const close = () => emit('close')
function onKey(e) { if (e.key === 'Escape') close() }
onMounted(() => {
  window.addEventListener('keydown', onKey)
  if (!outdated.value) search()
})
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="modal-overlay">
    <div class="modal server-check-modal" role="dialog" aria-modal="true">
      <div class="modal-header">
        <h2 class="server-check-title">{{ outdated ? t(`server.outdated_${outdatedKind}_title`) : t('server.not_found_title') }}</h2>
        <button class="modal-close" :title="t('welcome.close')" @click="close"><X :size="16" /></button>
      </div>

      <template v-if="outdated">
        <p class="server-check-lead">{{ t(`server.outdated_${outdatedKind}_lead`, { url: serverState.url, version: serverState.version || '?', missing: missingText }) }}</p>
        <p v-if="outdatedKind === 'old'" class="server-check-lead dim">{{ t('server.outdated_models') }}</p>
      </template>
      <template v-else>
        <p class="server-check-lead">{{ serverState.url ? t('server.not_found_lead', { url: serverState.url }) : t('server.not_configured_lead') }}</p>

        <div class="server-check-block">
          <div class="server-check-row">
            <span class="server-check-label">{{ t('server.found_in_network') }}</span>
            <button class="btn btn-secondary btn-sm" :disabled="searching || busy" @click="search">{{ searching ? t('settings.btn_discovering') : t('settings.btn_discover') }}</button>
          </div>
          <button v-for="srv in found" :key="srv.ip_address + ':' + srv.port" class="server-check-found" :disabled="busy"
            @click="apply(`http://${srv.ip_address}:${srv.port}`)">
            <span class="server-check-name">{{ srv.name || 'Imagecaster Server' }}</span>
            <span class="server-check-addr">{{ srv.ip_address }}:{{ srv.port }}</span>
          </button>
          <p v-if="searched && !found.length && !searching" class="server-check-lead dim">{{ t('server.nothing_found') }}</p>
        </div>

        <div class="server-check-block">
          <span class="server-check-label">{{ t('server.enter_address') }}</span>
          <div class="server-check-row">
            <input v-model="address" class="form-input" placeholder="http://192.168.1.100:8080" @keydown.enter="apply(address)" />
            <button class="btn btn-primary btn-sm" :disabled="busy || !address.trim()" @click="apply(address)">{{ t('server.connect') }}</button>
          </div>
        </div>
      </template>

      <p v-if="serverState.error && !outdated" class="server-check-lead err">{{ serverState.error }}</p>

      <div class="server-check-actions">
        <button class="btn btn-secondary" @click="openInstall">{{ !outdated ? t('server.how_to_install') : outdatedKind === 'forge' ? t('server.server_guide') : t('server.how_to_update') }}</button>
        <span class="server-check-spacer"></span>
        <button class="btn btn-secondary" :disabled="busy || serverState.checking" @click="retry">{{ t('server.retry') }}</button>
        <button class="btn btn-secondary" @click="close">{{ t('server.close_limited') }}</button>
      </div>
    </div>
  </div>
</template>

<style>
.server-check-modal { width: min(560px, 92vw); max-width: 560px; display: flex; flex-direction: column; gap: 12px; }
.server-check-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--text-bright); }
.server-check-lead { margin: 0; line-height: 1.55; }
.server-check-lead.dim { color: var(--text-dim); font-size: 13px; }
.server-check-lead.err { color: var(--danger); font-size: 12px; word-break: break-word; }
.server-check-block { display: flex; flex-direction: column; gap: 6px; padding-top: 10px; border-top: 1px solid var(--border); }
.server-check-row { display: flex; align-items: center; gap: 8px; }
.server-check-row .form-input { flex: 1; min-width: 0; }
.server-check-label { flex: 1; font-size: 13px; color: var(--text); }
.server-check-found { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 8px 12px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 6px; cursor: pointer; text-align: left; color: inherit; font: inherit; }
.server-check-found:hover { border-color: var(--accent); }
.server-check-name { color: var(--text-bright); font-size: 13px; font-weight: 500; }
.server-check-addr { color: var(--text-dim); font-size: 12px; }
.server-check-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.server-check-spacer { flex: 1; }
.server-banner { display: flex; align-items: center; gap: 10px; padding: 6px 14px; background: rgba(248, 113, 113, 0.12); border-bottom: 1px solid var(--border); color: var(--text-bright); font-size: 13px; }
.server-banner .btn { margin-left: auto; }
</style>
