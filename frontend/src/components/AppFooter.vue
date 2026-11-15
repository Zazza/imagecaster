<script setup>
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { X } from 'lucide-vue-next'
import QueuePanel from './QueuePanel.vue'
import SessionPanel from './footer/SessionPanel.vue'
import LogPanel from './footer/LogPanel.vue'
import { useServiceStatus } from '../composables/useServiceStatus.js'
import { useFooterResize } from '../composables/useFooterResize.js'

const emit = defineEmits(['navigate'])

const MAX_LOGS = 500

const expanded = ref(false)
const panel = ref('session')
const footerError = ref('')
const appVersion = ref('')
const itemCount = ref(0)
const queuePendingCount = ref(0)
const logs = reactive([])
const filterLevel = ref('all')
const logPanel = ref(null)
const seenLogIds = new Set()
let eventOffs = []

const { services, serviceTitle } = useServiceStatus()
const { panelHeight, onDragStart } = useFooterResize()

function pushLog(entry) {
  if (entry.id) {
    if (seenLogIds.has(entry.id)) return false
    seenLogIds.add(entry.id)
  }
  logs.push(entry)
  return true
}

function trimLogs() {
  if (logs.length > MAX_LOGS) logs.splice(0, logs.length - MAX_LOGS)
}

async function loadLogHistory() {
  let history
  try {
    history = await api.getLogEntries()
  } catch (e) {
    console.warn('getLogEntries', e)
    return
  }
  const live = logs.splice(0, logs.length)
  seenLogIds.clear()
  for (const entry of [...(history || []), ...live]) pushLog(entry)
  logs.sort((a, b) => (a.id || 0) - (b.id || 0))
  trimLogs()
}

function addLog(raw) {
  try {
    const entry = JSON.parse(raw)
    if (!pushLog(entry)) return
    trimLogs()
    if (expanded.value && panel.value === 'log') logPanel.value?.scrollToBottom()
  } catch (e) {
    console.warn('log entry parse error:', e)
  }
}

function clearLogs() {
  logs.splice(0, logs.length)
}

function togglePanel(p) {
  if (expanded.value && panel.value === p) {
    expanded.value = false
    return
  }
  panel.value = p
  expanded.value = true
}

function goToService(key) {
  const svc = services[key]
  if (!svc) return
  emit('navigate', { page: 'settings', tab: svc.tab || 'connection' })
}

async function refreshQueueCount() {
  try {
    queuePendingCount.value = (await api.getQueueActiveCount()) || 0
  } catch (e) {
    console.error('queue active count error:', e)
  }
}

onMounted(() => {
  eventOffs.push(
    EventsOn('log:entry', addLog),
    EventsOn('queue:changed', refreshQueueCount),
    EventsOn('queue:started', refreshQueueCount),
    EventsOn('queue:completed', refreshQueueCount),
    EventsOn('queue:failed', refreshQueueCount),
  )
  loadLogHistory()
  refreshQueueCount()
  api.version().then(v => { appVersion.value = v }).catch(e => console.warn('version:', e))
})

onUnmounted(() => {
  eventOffs.forEach(off => off?.())
  eventOffs = []
})
</script>

<template>
  <div class="app-footer" :class="{ expanded }" :style="expanded ? { flex: '0 0 ' + panelHeight + 'vh' } : {}">
    <div v-if="expanded" class="footer-resize-handle" @mousedown="onDragStart" @touchstart="onDragStart"></div>
    <div class="footer-bar">
      <div class="footer-status">
        <span v-for="(svc, key) in services" :key="key"
              class="status-dot"
              :class="{ online: svc.available, offline: !svc.available }"
              @click="goToService(key)" :title="serviceTitle(key, svc)">
          <span class="dot-indicator" :class="{ online: svc.available }"></span>
          {{ t(svc.label) }}
        </span>
      </div>
      <div class="footer-actions">
        <span v-if="appVersion" class="footer-version">v{{ appVersion }}</span>
        <span v-if="footerError" class="footer-error">{{ footerError }}</span>
        <button class="footer-tab-btn" :class="{ active: expanded && panel === 'session' }" @click="togglePanel('session')">
          {{ t('footer.session') }} {{ itemCount > 0 ? `(${itemCount})` : '' }}
        </button>
        <button class="footer-tab-btn" :class="{ active: expanded && panel === 'log' }" @click="togglePanel('log')">
          {{ t('footer.log') }}
        </button>
        <button class="footer-tab-btn" :class="{ active: expanded && panel === 'queue' }" @click="togglePanel('queue')">
          {{ t('footer.processing') }} {{ queuePendingCount > 0 ? `(${queuePendingCount})` : '' }}
        </button>
        <button v-if="expanded" class="footer-tab-btn footer-panel-close" :aria-label="t('common.close')" :title="t('common.close')" @click="expanded = false">
          <X :size="12" />
        </button>
      </div>
    </div>

    <SessionPanel :open="expanded && panel === 'session'" @navigate="emit('navigate', $event)" @count="itemCount = $event" @error="footerError = $event" />

    <div v-if="expanded && panel === 'queue'" class="footer-queue">
      <QueuePanel />
    </div>

    <LogPanel v-if="expanded && panel === 'log'" ref="logPanel" v-model:filter="filterLevel" :logs="logs" @clear="clearLogs" />
  </div>
</template>

<script>
export default { name: 'AppFooter' }
</script>

<style scoped>
.footer-error {
  margin-right: var(--sp-2);
  color: var(--danger);
  font-size: var(--fs-xs);
}

.app-footer {
  background: var(--frame);
  flex-shrink: 0;
  min-height: 30px;
  position: relative;
  z-index: 100;
}

.app-footer.expanded {
  display: flex;
  flex-direction: column;
}

.footer-resize-handle {
  height: 5px;
  cursor: ns-resize;
  background: transparent;
  position: relative;
  flex-shrink: 0;
}

.footer-resize-handle:hover,
.footer-resize-handle:active {
  background: var(--accent-bg);
}

.footer-resize-handle::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 2px;
  transform: translateX(-50%);
  width: 32px;
  height: 3px;
  border-radius: 2px;
  background: var(--border);
}

.footer-resize-handle:hover::after {
  background: var(--accent);
}

.footer-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  justify-content: space-between;
  align-items: center;
  padding: 6px 16px;
  cursor: default;
  user-select: none;
}

.footer-bar:hover {
  background: var(--surface-2);
}

.footer-status {
  display: flex;
  gap: 12px;
}

.footer-actions {
  display: flex;
  gap: 4px;
  align-items: center;
}

.footer-version {
  font-size: 11px;
  color: var(--text-dim);
  margin-right: 4px;
  opacity: 0.7;
}

.footer-tab-btn {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-dim);
  padding: 2px 10px;
  border-radius: 3px;
  font-size: 11px;
  cursor: pointer;
}

.footer-tab-btn:hover {
  border-color: var(--border);
}

.footer-tab-btn.active {
  background: var(--accent-bg);
  border-color: var(--accent);
  color: var(--text-bright);
}

.footer-panel-close {
  display: inline-flex;
  align-items: center;
  padding: 2px 6px;
}

.status-dot {
  font-size: 12px;
  font-weight: 500;
  padding: 1px 6px;
  border-radius: 3px;
  cursor: pointer;
  transition: background 0.15s;
  color: var(--text);
}

.status-dot:hover {
  background: var(--surface-2);
}

.dot-indicator {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--danger);
  margin-right: 4px;
  vertical-align: middle;
}

.dot-indicator.online { background: var(--success); }

.footer-queue {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-top: 1px solid var(--border);
}
</style>
