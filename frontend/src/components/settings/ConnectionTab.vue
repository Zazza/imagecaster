<script setup>
import { ref, reactive, computed, inject, watch, onUnmounted } from 'vue'
import { ChevronRight } from 'lucide-vue-next'
import StatusMessage from '../StatusMessage.vue'
import { api } from '../../api.js'
import { serverURL, serverModels, serverModelsLoading, loadServerModels } from '../../serverModels.js'
import { t } from '../../i18n/index.js'
import { visibleProcesses } from './processFilter.js'

const props = defineProps({
  active: { type: Boolean, default: true },
})

const {
  discoveredServers, discovering, serverStatus, utilitiesExpanded, generateModel, analyzeModel,
  generateParams, analyzeParams, refreshServerStatus, pollServerStatus, processActionLoading,
} = inject('settingsState')

const connectionSaved = ref(false)
const connectionError = ref('')
const llmParamsSaved = ref(false)
const llmParamsError = ref('')
const visibleLogs = reactive({})
const processLogs = reactive({})
const logTimers = {}

const serviceRows = computed(() => visibleProcesses(serverStatus.value?.processes, 'service'))
const utilityRows = computed(() => visibleProcesses(serverStatus.value?.processes, 'utility'))

async function discoverServers() {
  discovering.value = true
  discoveredServers.value = []
  try {
    discoveredServers.value = await api.discoverServers() || []
  } catch (e) {
    console.error('discoverServers:', e)
  } finally {
    discovering.value = false
  }
}

async function selectServer(server) {
  serverURL.value = `http://${server.ip_address}:${server.port}`
  try {
    await api.updateSettings({ server_url: serverURL.value })
  } catch (e) {
    connectionError.value = String(e)
    return
  }
  await refreshServerStatus()
}

async function runProcessAction(name, action, errKey) {
  processActionLoading[name] = true
  try {
    await action(name)
    pollServerStatus()
  } catch (e) {
    alert(t(errKey, { error: String(e) }))
    await refreshServerStatus()
  } finally {
    delete processActionLoading[name]
  }
}

const startProcess = name => runProcessAction(name, api.startServerProcess, 'settings.err_start')
const stopProcess = name => runProcessAction(name, api.stopServerProcess, 'settings.err_stop')
const restartProcess = name => runProcessAction(name, api.restartServerProcess, 'settings.err_restart')

async function refreshProcessLogs(name) {
  try {
    processLogs[name] = await api.getServerProcessLogs(name, 50) || []
  } catch (e) {
    processLogs[name] = [t('settings.logs_error', { error: e?.message || String(e) })]
  }
}

function hideProcessLogs(name) {
  visibleLogs[name] = false
  clearInterval(logTimers[name])
  delete logTimers[name]
}

function hideAllProcessLogs() {
  for (const name of Object.keys(visibleLogs)) hideProcessLogs(name)
}

watch(() => props.active, (on) => { if (!on) hideAllProcessLogs() })
onUnmounted(hideAllProcessLogs)

async function showProcessLogs(name) {
  if (visibleLogs[name]) {
    hideProcessLogs(name)
    return
  }
  visibleLogs[name] = true
  await refreshProcessLogs(name)
  if (visibleLogs[name] && !logTimers[name]) {
    logTimers[name] = setInterval(() => refreshProcessLogs(name), 2000)
  }
}

async function saveServerConnection() {
  connectionSaved.value = false
  connectionError.value = ''
  try {
    await api.updateSettings({
      server_url: serverURL.value,
      llm_generate_model: generateModel.value,
      llm_analyze_model: analyzeModel.value,
    })
    connectionSaved.value = true
    await refreshServerStatus()
  } catch (e) {
    connectionError.value = String(e)
  }
}

async function saveLLMParams() {
  llmParamsSaved.value = false
  llmParamsError.value = ''
  try {
    await api.updateSettings({
      llm_generate_num_ctx: String(generateParams.num_ctx),
      llm_generate_top_p: String(generateParams.top_p),
      llm_generate_num_thread: String(generateParams.num_thread),
      llm_analyze_num_ctx: String(analyzeParams.num_ctx),
      llm_analyze_top_p: String(analyzeParams.top_p),
      llm_analyze_num_thread: String(analyzeParams.num_thread),
    })
    llmParamsSaved.value = true
  } catch (e) {
    llmParamsError.value = String(e)
  }
}
</script>

<template>
  <div class="card">
    <StatusMessage v-if="connectionSaved" kind="success" :timeout="8000" style="margin-bottom: 16px;" @close="connectionSaved = false">{{ t('settings.saved_immediate') }}</StatusMessage>
    <StatusMessage v-if="connectionError" style="margin-bottom: 16px;" @close="connectionError = ''">{{ connectionError }}</StatusMessage>

      <div class="form-group">
        <label class="form-label">{{ t('settings.label_server_url') }}</label>
        <div style="display: flex; gap: 8px;">
          <input class="form-input" v-model="serverURL" placeholder="http://192.168.1.100:8080" style="flex: 1;" />
          <button class="btn btn-secondary btn-sm" @click="discoverServers" :disabled="discovering">
            {{ discovering ? t('settings.btn_discovering') : t('settings.btn_discover') }}
          </button>
        </div>
      </div>

      <div v-if="discoveredServers.length > 0" style="margin-bottom: 16px;">
        <div v-for="srv in discoveredServers" :key="srv.ip_address + ':' + srv.port"
          @click="selectServer(srv)"
          style="padding: 10px 12px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 6px; margin-bottom: 6px; cursor: pointer; transition: border-color 0.15s;"
          @mouseenter="$event.target.style.borderColor = 'var(--accent)'"
          @mouseleave="$event.target.style.borderColor = 'var(--border)'"
        >
          <div style="color: var(--text-bright); font-size: 13px; font-weight: 500;">{{ srv.name || 'Imagecaster Server' }}</div>
          <div style="color: var(--text-dim); font-size: 12px;">{{ srv.ip_address }}:{{ srv.port }}</div>
        </div>
      </div>
      <div v-else-if="discovering" style="color: var(--text-dim); font-size: 13px; margin-bottom: 16px; text-align: center;">
        <span class="spinner"></span> {{ t('settings.btn_discovering') }}
      </div>

      <div v-if="serverStatus" style="margin-bottom: 16px; padding: 12px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 6px;">
        <div style="color: var(--text-bright); font-weight: 500; margin-bottom: 8px;">{{ t('settings.section_server_status') }}</div>
        <div v-for="[name, proc] in serviceRows" :key="name" :data-testid="'process-row-' + name" style="padding: 6px 0; border-bottom: 1px solid var(--border);">
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <div>
              <span style="color: var(--text-bright); font-size: 13px;">{{ proc.name || name }}</span>
              <span data-testid="process-status" :style="{ color: proc.status === 'running' ? 'var(--success)' : proc.status === 'starting' ? 'var(--warning)' : 'var(--danger)', marginLeft: '8px', fontSize: '12px' }">
                {{ proc.status === 'running' ? t('settings.process_running') : proc.status === 'starting' ? t('settings.process_starting') : proc.status === 'crashed' ? t('settings.process_crashed') : proc.status === 'unhealthy' ? t('settings.process_unhealthy') : t('settings.process_stopped') }}
              </span>
              <span v-if="proc.pid" style="color: var(--text-dim); font-size: 11px; margin-left: 6px;">PID {{ proc.pid }}</span>
              <span v-if="proc.uptime" style="color: var(--text-dim); font-size: 11px; margin-left: 6px;">{{ proc.uptime }}</span>
              <span v-if="proc.restarts > 0" style="color: var(--warning); font-size: 11px; margin-left: 6px;">{{ t('settings.process_restarts', { count: proc.restarts }) }}</span>
            </div>
            <div style="display: flex; gap: 6px;">
              <button v-if="proc.status !== 'running' && proc.status !== 'starting' && proc.status !== 'unhealthy'" data-testid="process-start" class="btn btn-primary btn-sm" @click="startProcess(name)" :disabled="processActionLoading[name]">{{ t('settings.btn_start') }}</button>
              <button v-if="proc.status === 'running' || proc.status === 'unhealthy'" data-testid="process-stop" class="btn btn-secondary btn-sm" @click="stopProcess(name)" :disabled="processActionLoading[name]">{{ t('settings.btn_stop') }}</button>
              <button v-if="proc.status === 'running' || proc.status === 'unhealthy'" data-testid="process-restart" class="btn btn-secondary btn-sm" @click="restartProcess(name)" :disabled="processActionLoading[name]">{{ t('settings.btn_restart') }}</button>
              <button class="btn btn-secondary btn-sm" @click="showProcessLogs(name)">{{ t('settings.btn_logs') }}</button>
            </div>
          </div>
          <div v-if="visibleLogs[name]" style="margin-top: 6px; padding: 8px; background: var(--bg-dark); border-radius: 4px; max-height: 200px; overflow-y: auto; font-family: monospace; font-size: 11px; color: var(--text-dim);">
            <div v-if="processLogs[name] && processLogs[name].length > 0">
              <div v-for="(line, i) in processLogs[name]" :key="i">{{ line }}</div>
            </div>
            <div v-else style="color: var(--text-dim);">{{ t('settings.no_logs') }}</div>
          </div>
        </div>
        <div v-if="serverStatus.gpu && serverStatus.gpu.available" style="margin-top: 8px; font-size: 12px; color: var(--text-dim);">
          {{ t('settings.gpu_vram', { name: serverStatus.gpu.name, used: serverStatus.gpu.memory_used_mb, total: serverStatus.gpu.memory_total_mb }) }}
        </div>
        <div v-if="utilityRows.length" style="margin-top: 12px; border-top: 1px solid var(--border); padding-top: 8px;">
          <div @click="utilitiesExpanded = !utilitiesExpanded" style="cursor: pointer; color: var(--text-dim); font-size: 12px; font-weight: 500; display: flex; align-items: center; gap: 4px;">
            <span :style="{ transform: utilitiesExpanded ? 'rotate(90deg)' : 'rotate(0deg)', transition: 'transform 0.15s', display: 'inline-flex' }"><ChevronRight :size="12" /></span>
            {{ t('settings.section_utilities') }}
          </div>
          <div v-if="utilitiesExpanded">
            <div v-for="[name, proc] in utilityRows" :key="'u-'+name" style="padding: 6px 0; border-bottom: 1px solid var(--border);">
              <div>
                <span style="color: var(--text-bright); font-size: 13px;">{{ proc.name || name }}</span>
                <span v-if="serverStatus.installs && serverStatus.installs[name]" :style="{ color: serverStatus.installs[name].installed ? 'var(--success)' : 'var(--danger)', marginLeft: '8px', fontSize: '11px', padding: '1px 6px', borderRadius: '3px', background: serverStatus.installs[name].installed ? 'rgba(74,222,128,0.1)' : 'rgba(248,113,113,0.1)' }">
                  {{ serverStatus.installs[name].installed ? t('settings.installed') : t('settings.not_installed') }}
                </span>
                <span v-if="serverStatus.installs && serverStatus.installs[name] && serverStatus.installs[name].version" style="color: var(--text-dim); font-size: 11px; margin-left: 6px;">v{{ serverStatus.installs[name].version }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="form-group" v-if="serverModels.llm.length > 0" style="margin-top: 16px;">
        <label class="form-label">{{ t('settings.label_model_generate') }}</label>
        <div style="display: flex; gap: 8px;">
          <select class="form-select" v-model="generateModel" style="flex: 1;">
            <option value="" disabled>{{ t('llm_setup.choose') }}</option>
            <option v-for="m in serverModels.llm" :key="m.name" :value="m.name">{{ m.name }}</option>
          </select>
          <button class="btn btn-secondary btn-sm" @click="loadServerModels" :disabled="serverModelsLoading.llm">
            {{ serverModelsLoading.llm ? '...' : t('settings.btn_refresh') }}
          </button>
        </div>
      </div>

      <div class="form-group" v-if="serverModels.llm.length > 0">
        <label class="form-label">{{ t('settings.label_model_analyze') }}</label>
        <select class="form-select" v-model="analyzeModel">
          <option value="">{{ t('settings.same_as_generate') }}</option>
          <option v-for="m in serverModels.llm" :key="m.name" :value="m.name">{{ m.name }}</option>
        </select>
      </div>


      <button class="btn btn-primary" @click="saveServerConnection">{{ t('settings.btn_save_server') }}</button>

    <details class="settings-advanced">
      <summary>{{ t('settings.section_llm_params') }}</summary>
      <StatusMessage v-if="llmParamsSaved" kind="success" :timeout="8000" style="margin-bottom: 16px;" @close="llmParamsSaved = false">{{ t('settings.params_saved') }}</StatusMessage>
      <StatusMessage v-if="llmParamsError" style="margin-bottom: 16px;" @close="llmParamsError = ''">{{ llmParamsError }}</StatusMessage>

      <h4 style="color: var(--text-bright); margin: 16px 0 8px; font-size: 14px;">{{ t('settings.generate_params') }}</h4>
      <div class="form-row-2">
        <div class="form-group">
          <label class="form-label">{{ t('settings.label_context_size') }}</label>
          <input class="form-input" type="number" v-model.number="generateParams.num_ctx" step="512" min="512" max="32768" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('settings.label_top_p') }}</label>
          <input class="form-input" type="number" v-model.number="generateParams.top_p" step="0.05" min="0" max="1" />
        </div>
      </div>
      <div class="form-row-2">
        <div class="form-group">
          <label class="form-label">{{ t('settings.label_threads') }}</label>
          <input class="form-input" type="number" v-model.number="generateParams.num_thread" min="0" max="64" />
        </div>
      </div>

      <h4 style="color: var(--text-bright); margin: 16px 0 8px; font-size: 14px;">{{ t('settings.analyze_params') }}</h4>
      <div class="form-row-2">
        <div class="form-group">
          <label class="form-label">{{ t('settings.label_context_size') }}</label>
          <input class="form-input" type="number" v-model.number="analyzeParams.num_ctx" step="512" min="512" max="32768" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('settings.label_top_p') }}</label>
          <input class="form-input" type="number" v-model.number="analyzeParams.top_p" step="0.05" min="0" max="1" />
        </div>
      </div>
      <div class="form-row-2">
        <div class="form-group">
          <label class="form-label">{{ t('settings.label_threads') }}</label>
          <input class="form-input" type="number" v-model.number="analyzeParams.num_thread" min="0" max="64" />
        </div>
      </div>

      <button class="btn btn-primary" @click="saveLLMParams" style="margin-top: 8px;">{{ t('settings.btn_save_params') }}</button>
    </details>
  </div>
</template>
