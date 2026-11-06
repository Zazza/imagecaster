<script setup>
import { ref, reactive, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { Image, Trash2, X, Plus, Pencil, Check } from 'lucide-vue-next'
import QueuePanel from './QueuePanel.vue'
import ImageViewer from './ImageViewer.vue'
import { useRemixQueue } from '../remixQueue.js'

const emit = defineEmits(['navigate'])

const expanded = ref(false)
const panel = ref('session')
const panelHeight = ref(40)
const footerRef = ref(null)
let dragging = false
let dragStartY = 0
let dragStartHeight = 0
let saveTimer = null
const logs = reactive([])
const logContainer = ref(null)
const filterLevel = ref('all')

const sessions = ref([])
const activeSessionId = ref(0)
const items = ref([])
const thumbnails = ref({})
const brokenThumbs = ref({})

const itemContainer = ref(null)

const { setPostQueue } = useRemixQueue()
const selectMode = ref(false)
const selectedIds = ref([])
const selectedCount = computed(() => items.value.filter(i => selectedIds.value.includes(i.id)).length)

function toggleSelectItem(item) {
  const idx = selectedIds.value.indexOf(item.id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(item.id)
}

function exitSelectMode() {
  selectMode.value = false
  selectedIds.value = []
}

function onSessionCardClick(item) {
  if (selectMode.value) toggleSelectItem(item)
  else openViewer(item)
}

function sendSelectedToPostProcess() {
  const picked = items.value
    .filter(i => selectedIds.value.includes(i.id))
    .map(i => ({ path: 'session:' + i.id, name: 'history_' + i.id }))
  if (!picked.length) return
  setPostQueue(picked)
  exitSelectMode()
  emit('navigate', { page: 'postprocess' })
}

watch(activeSessionId, exitSelectMode)

const newSessionName = ref('')
const showNewSession = ref(false)
const renamingId = ref(0)
const renameName = ref('')
const confirmDeleteSession = ref(false)
const confirmDeleteViewer = ref(false)
let confirmDeleteTimer = null

const appVersion = ref('')

const viewerIndex = ref(-1)

const queuePendingCount = ref(0)
let queueSubscribed = false

let observer = null

const services = reactive({
  llm: { available: false, label: 'footer.ai_assistant', model: '', visionModel: '', tab: 'connection' },
  sd: { available: false, label: 'footer.image_engine', model: '', tab: 'connection' },
})

const activeSessions = computed(() => (sessions.value || []).find(s => s.id === activeSessionId.value))
const itemCount = computed(() => {
  const s = activeSessions.value
  return s ? s.item_count : 0
})
const sourceLabelKeys = {
  'generate': 'footer.source_generate',
  'from-image': 'footer.source_from_image',
  'compound': 'footer.source_compound',
  'compound-from-image': 'footer.source_compound_from_image',
  'batch': 'footer.source_batch',
  'test': 'footer.source_test',
  'scene': 'footer.source_scene',
  'upscale': 'footer.source_upscale',
  'upscale-preview': 'footer.source_upscale_preview',
  'remove': 'footer.source_remove',
  'file-browser': 'footer.source_file_browser',
  'edit': 'footer.source_edit',
  'object': 'footer.source_object',
  'look': 'footer.source_look',
  'compare': 'footer.source_compare',
}

function sourceLabel(source) {
  const key = sourceLabelKeys[source]
  return key ? t(key) : (source || '')
}

function serviceTitle(key, svc) {
  let main
  if (key === 'llm') {
    main = svc.model || svc.visionModel
      ? t('footer.status_models', { models: (svc.model || '') + (svc.visionModel ? ' / ' + svc.visionModel : '') })
      : t('footer.status_no_model')
  } else {
    main = svc.model || t('footer.status_no_model')
  }
  return main + '\n' + t('footer.status_click_settings')
}

function chainSteps(item) {
  const chain = item?.meta?.chain
  return Array.isArray(chain) ? chain.length : 0
}

function itemTitle(item) {
  const meta = item.meta
  const count = chainSteps(item)
  const lines = []
  if (meta) {
    let label = meta.label || ''
    if (!label && count > 0) label = t('footer.custom_mode')
    if (!label) label = sourceLabel(item.source)
    if (label) lines.push(label)
    if (meta.model) lines.push(meta.model)
  } else {
    if (item.sampler) lines.push(item.sampler)
    if (!lines.length) lines.push(sourceLabel(item.source))
  }
  if (item.seed != null) lines.push(t('footer.tooltip_seed', { seed: item.seed }))
  if (count > 1) lines.push(t('footer.tooltip_steps', { count }))
  return lines.join('\n')
}

function addLog(raw) {
  try {
    const entry = JSON.parse(raw)
    logs.push(entry)
    if (logs.length > 500) {
      logs.splice(0, logs.length - 500)
    }
    if (expanded.value && panel.value === 'log') {
      nextTick(() => {
        if (logContainer.value) {
          logContainer.value.scrollTop = logContainer.value.scrollHeight
        }
      })
    }
  } catch {}
}

function togglePanel(p) {
  if (expanded.value && panel.value === p) {
    expanded.value = false
    return
  }
  panel.value = p
  expanded.value = true
  if (p === 'log') {
    nextTick(() => {
      if (logContainer.value) {
        logContainer.value.scrollTop = logContainer.value.scrollHeight
      }
    })
  }
  if (p === 'session') {
    loadSessions().then(() => loadItems()).then(() => {
      nextTick(() => {
        observeItems()
        if (itemContainer.value) {
          itemContainer.value.scrollTop = itemContainer.value.scrollHeight
        }
      })
    }).catch(() => {})
  }
}

function levelClass(level) {
  switch (level) {
    case 'error': return 'log-error'
    case 'warn': return 'log-warn'
    case 'info': return 'log-info'
    case 'debug': return 'log-debug'
    default: return ''
  }
}

const filteredLogs = () => {
  if (filterLevel.value === 'all') return logs
  return logs.filter(l => l.level === filterLevel.value)
}

function goToService(key) {
  const svc = services[key]
  if (!svc) return
  emit('navigate', { page: 'settings', tab: svc.tab || 'connection' })
}

async function checkServices() {
  try {
    const settings = await api.getSettings()
    if (settings.connection_mode === 'server') {
      await checkServicesViaServer(settings)
    } else {
      await checkServicesDirect()
    }
  } catch {
    await checkServicesDirect()
  }
}

async function checkServicesViaServer(settings) {
  try {
    const status = await api.getServerStatus()
    const health = status.health || {}
    const models = status.models || {}
    services.llm.available = health.ollama?.healthy || false
    const genModel = settings.llm_generate_model || settings.sd_prompt_model || settings.llm_model || ''
    services.llm.model = genModel && genModel !== 'default' ? genModel : ''
    const visModel = settings.llm_analyze_model || settings.vision_model || ''
    services.llm.visionModel = visModel && visModel !== 'default' ? visModel : ''
    services.sd.available = health.sd?.healthy || false
    services.sd.model = models.sd_checkpoint || ''
  } catch {
    services.llm.available = false
    services.sd.available = false
  }
}

async function checkServicesDirect() {
  try {
    const status = await api.checkServices()
    services.llm.available = status.llm?.available || false
    services.llm.model = status.llm?.model || ''
    services.llm.visionModel = status.llm?.vision_model || ''
    services.sd.available = status.sd?.available || false
    services.sd.model = status.sd?.model || ''
  } catch {
    services.llm.available = false
    services.sd.available = false
  }
}

function setupObserver() {
  observer = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      if (entry.isIntersecting) {
        const id = Number(entry.target.dataset.itemId)
        if (id) {
          loadThumb(id)
        }
        observer.unobserve(entry.target)
      }
    }
  }, { rootMargin: '100px' })
}

function observeItems() {
  if (!observer || !itemContainer.value) return
  const els = itemContainer.value.querySelectorAll('.si-card[data-item-id]')
  els.forEach(el => observer.observe(el))
}

function loadThumb(id) {
  if (thumbnails.value[id]) return
  thumbnails.value[id] = api.sessionThumbUrl(id)
}

const footerError = ref('')

async function loadSessions() {
  try {
    const result = await api.listSessions()
    sessions.value = result || []
    const active = sessions.value.find(s => s.is_active) || sessions.value[0]
    activeSessionId.value = active ? active.id : 0
    footerError.value = ''
  } catch (e) {
    footerError.value = 'loadSessions: ' + String(e)
    sessions.value = []
  }
}

async function loadItems() {
  try {
    const result = await api.getSessionItems()
    items.value = result || []
    nextTick(() => observeItems())
  } catch (e) {
    footerError.value = 'loadItems: ' + String(e)
    items.value = []
  }
}

async function selectSession(id) {
  if (id === activeSessionId.value) return
  try {
    await api.switchSession(id)
    thumbnails.value = {}
    await loadSessions()
    await loadItems()
  } catch {}
}

async function doCreateSession() {
  const name = newSessionName.value.trim()
  if (!name) return
  try {
    const si = await api.createSession(name)
    newSessionName.value = ''
    showNewSession.value = false
    await loadSessions()
    thumbnails.value = {}
    await loadItems()
  } catch {}
}

function startRename(s) {
  renamingId.value = s.id
  renameName.value = s.name
}

async function doRename() {
  if (!renameName.value.trim()) return
  try {
    await api.renameSession(renamingId.value, renameName.value.trim())
    renamingId.value = 0
    await loadSessions()
  } catch {}
}

async function doDeleteSession() {
  if (!confirmDeleteSession.value) {
    confirmDeleteSession.value = true
    confirmDeleteTimer = setTimeout(() => { confirmDeleteSession.value = false }, 3000)
    return
  }
  clearTimeout(confirmDeleteTimer)
  confirmDeleteSession.value = false
  if (!window.confirm(t('footer.confirm_delete_dialog'))) return
  try {
    await api.deleteSession(activeSessionId.value)
    await loadSessions()
    thumbnails.value = {}
    await loadItems()
  } catch {}
}

async function doClearSession() {
  try {
    await api.clearSession()
    items.value = []
    thumbnails.value = {}
    await loadSessions()
  } catch {}
}

async function activateItem(item, target) {
  try {
    await api.setActiveSessionItem(item.id)
    items.value.forEach(i => { i.is_active = i.id === item.id })
    emit('navigate', target)
  } catch {}
}

async function deleteItem(id) {
  try {
    await api.deleteSessionItem(id)
    items.value = items.value.filter(i => i.id !== id)
    delete thumbnails.value[id]
    await loadSessions()
  } catch {}
}

const viewerItem = computed(() => viewerIndex.value >= 0 ? items.value[viewerIndex.value] : null)
const viewerImage = computed(() => viewerItem.value ? api.sessionImageUrl(viewerItem.value.id) : '')

function openViewer(item) {
  const idx = items.value.findIndex(i => i.id === item.id)
  if (idx >= 0) viewerIndex.value = idx
}

function closeViewer() {
  viewerIndex.value = -1
}

async function viewerRemix() {
  const item = viewerItem.value
  if (!item) return
  closeViewer()
  await activateItem(item, { page: 'postprocess', section: 'remix' })
}

async function viewerEdit() {
  const item = viewerItem.value
  if (!item) return
  closeViewer()
  await activateItem(item, { page: 'postprocess', section: 'edit' })
}

async function viewerExport() {
  const item = viewerItem.value
  if (!item) return
  closeViewer()
  await activateItem(item, { page: 'postprocess' })
}

function viewerPrev() {
  if (viewerIndex.value > 0) viewerIndex.value--
}

function viewerNext() {
  if (viewerIndex.value < items.value.length - 1) viewerIndex.value++
}

async function viewerDelete() {
  const item = viewerItem.value
  if (!item) return
  if (!confirmDeleteViewer.value) {
    confirmDeleteViewer.value = true
    setTimeout(() => { confirmDeleteViewer.value = false }, 3000)
    return
  }
  confirmDeleteViewer.value = false
  await deleteItem(item.id)
  if (items.value.length === 0) {
    closeViewer()
  } else if (viewerIndex.value >= items.value.length) {
    viewerIndex.value = items.value.length - 1
  }
}

function formatTime(ts) {
  if (!ts) return ''
  return ts.replace(/^.*?(\d{2}:\d{2}).*/, '$1')
}

async function refreshQueueCount() {
  try {
    const jobs = await api.getQueue()
    queuePendingCount.value = (jobs || []).filter(j => j.status === 'pending' || j.status === 'running').length
  } catch {}
}

onMounted(async () => {
  EventsOn('log:entry', addLog)
  checkServices()
  const interval = setInterval(checkServices, 30000)
  setupObserver()

  EventsOn('session:added', () => {
    loadSessions()
    loadItems().then(() => {
      nextTick(() => {
        if (expanded.value && panel.value === 'session' && itemContainer.value) {
          itemContainer.value.scrollTop = itemContainer.value.scrollHeight
        }
      })
    })
  })
  EventsOn('session:removed', () => { loadItems(); loadSessions() })
  EventsOn('session:cleared', () => { loadItems(); loadSessions() })
  EventsOn('session:switched', () => {
    thumbnails.value = {}
    loadItems()
    loadSessions()
  })
  EventsOn('session:active', (data) => {
    const id = data?.id
    if (id) items.value.forEach(i => { i.is_active = i.id === id })
  })
  EventsOn('session:deleted', () => { loadSessions(); loadItems() })
  EventsOn('session:created', () => { loadSessions(); loadItems() })

  if (!queueSubscribed) {
    queueSubscribed = true
    EventsOn('queue:changed', refreshQueueCount)
    EventsOn('queue:started', refreshQueueCount)
    EventsOn('queue:completed', refreshQueueCount)
    EventsOn('queue:failed', refreshQueueCount)
    refreshQueueCount()
  }

  await loadSessions()
  api.version().then(v => { appVersion.value = v }).catch(() => {})
  await loadItems()

  onUnmounted(() => {
    EventsOff('log:entry')
    clearInterval(interval)
    EventsOff('session:added')
    EventsOff('session:removed')
    EventsOff('session:cleared')
    EventsOff('session:switched')
    EventsOff('session:active')
    EventsOff('session:deleted')
    EventsOff('session:created')
    if (observer) observer.disconnect()
    thumbnails.value = {}
  })
})

function onDragStart(e) {
  dragging = true
  dragStartY = e.clientY || (e.touches && e.touches[0].clientY) || 0
  dragStartHeight = panelHeight.value
  document.addEventListener('mousemove', onDragMove)
  document.addEventListener('mouseup', onDragEnd)
  document.addEventListener('touchmove', onDragMove, { passive: false })
  document.addEventListener('touchend', onDragEnd)
  document.body.style.cursor = 'ns-resize'
  document.body.style.userSelect = 'none'
}

function onDragMove(e) {
  if (!dragging) return
  e.preventDefault()
  const clientY = e.clientY || (e.touches && e.touches[0].clientY) || 0
  const delta = dragStartY - clientY
  const vh = window.innerHeight
  let newVh = Math.round((dragStartHeight * vh / 100 + delta) / vh * 100)
  newVh = Math.max(15, Math.min(85, newVh))
  panelHeight.value = newVh
}

function onDragEnd() {
  dragging = false
  document.removeEventListener('mousemove', onDragMove)
  document.removeEventListener('mouseup', onDragEnd)
  document.removeEventListener('touchmove', onDragMove)
  document.removeEventListener('touchend', onDragEnd)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  scheduleSave()
}

function scheduleSave() {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    api.saveWindowLayout(panelHeight.value).catch(() => {})
  }, 500)
}

function saveBeforeClose() {
  api.saveWindowLayout(panelHeight.value).catch(() => {})
}

onMounted(async () => {
  try {
    const h = await api.getFooterHeight()
    if (h > 0) panelHeight.value = h
  } catch {}
  window.addEventListener('beforeunload', saveBeforeClose)
})

onUnmounted(() => {
  window.removeEventListener('beforeunload', saveBeforeClose)
  clearTimeout(saveTimer)
})
</script>

<template>
  <div class="app-footer" :class="{ expanded }" ref="footerRef" :style="expanded ? { flex: '0 0 ' + panelHeight + 'vh' } : {}">
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
        <span v-if="footerError" style="color: var(--danger); font-size: 11px; margin-right: 8px;">{{ footerError }}</span>
        <button class="footer-tab-btn" :class="{ active: expanded && panel === 'session' }" @click="togglePanel('session')">
          {{ t('footer.session') }} {{ itemCount > 0 ? `(${itemCount})` : '' }}
        </button>
        <button class="footer-tab-btn" :class="{ active: expanded && panel === 'log' }" @click="togglePanel('log')">
          {{ t('footer.log') }}
        </button>
        <button class="footer-tab-btn" :class="{ active: expanded && panel === 'queue' }" @click="togglePanel('queue')">
          {{ t('footer.processing') }} {{ queuePendingCount > 0 ? `(${queuePendingCount})` : '' }}
        </button>
      </div>
    </div>

    <div v-if="expanded && panel === 'session'" class="footer-session">
      <div class="session-toolbar">
        <div class="session-selector">
          <select class="session-select" :value="activeSessionId" :aria-label="t('footer.session_label')" @change="selectSession(Number($event.target.value))">
            <option v-for="s in sessions" :key="s.id" :value="s.id">
              {{ s.name }} ({{ s.item_count }})
            </option>
          </select>
          <button class="session-tool-btn" :title="t('footer.title_new_session')" @click="showNewSession = true">
            <Plus :size="12" />
          </button>
          <template v-if="renamingId">
            <input class="session-rename-input" v-model="renameName" @keydown.enter="doRename" @keydown.escape="renamingId = 0" />
            <button class="session-tool-btn" @click="doRename"><Check :size="12" /></button>
            <button class="session-tool-btn" @click="renamingId = 0"><X :size="12" /></button>
          </template>
          <template v-else>
            <button class="session-tool-btn" :title="t('footer.title_rename')" @click="activeSessions && startRename(activeSessions)">
              <Pencil :size="12" />
            </button>
          </template>
        </div>
        <div class="session-actions">
          <button v-if="items.length && !selectMode" class="session-tool-btn text-btn" @click="selectMode = true">{{ t('footer.select_mode') }}</button>
          <button class="session-tool-btn text-btn" @click="doClearSession" :disabled="!items.length">{{ t('footer.btn_clear') }}</button>
          <button class="session-delete-btn" :class="{ confirm: confirmDeleteSession }" @click="doDeleteSession" :disabled="sessions.length <= 1">
            <Trash2 :size="12" /> {{ confirmDeleteSession ? t('footer.confirm_delete') : t('footer.btn_delete') }}
          </button>
          <button class="session-tool-btn" @click="expanded = false"><X :size="12" /></button>
        </div>
      </div>

      <div v-if="showNewSession" class="session-new">
        <input class="session-rename-input" v-model="newSessionName" :placeholder="t('footer.placeholder_session_name')" @keydown.enter="doCreateSession" @keydown.escape="showNewSession = false" />
        <button class="session-tool-btn" @click="doCreateSession"><Check :size="12" /></button>
        <button class="session-tool-btn" @click="showNewSession = false"><X :size="12" /></button>
      </div>

      <div v-if="selectMode" class="session-select-bar">
        <span>{{ t('footer.selected_count', { n: selectedCount }) }}</span>
        <button class="session-tool-btn text-btn" :disabled="!selectedCount" @click="sendSelectedToPostProcess">{{ t('footer.send_to_postprocess', { n: selectedCount }) }}</button>
        <button class="session-tool-btn text-btn" @click="exitSelectMode">{{ t('footer.cancel_select') }}</button>
      </div>

      <div v-if="items.length" class="si-grid" ref="itemContainer">
        <div
          v-for="item in items"
          :key="item.id"
          class="si-card"
          :class="{ 'si-active': item.is_active, 'si-selected': selectMode && selectedIds.includes(item.id) }"
          :data-item-id="item.id"
          :title="itemTitle(item)"
          @click="onSessionCardClick(item)"
        >
          <div v-if="selectMode && selectedIds.includes(item.id)" class="si-check"><Check :size="12" /></div>
          <span v-if="chainSteps(item) > 1" class="si-steps-badge" aria-hidden="true">{{ chainSteps(item) }}</span>
          <div class="si-thumb">
            <img v-if="thumbnails[item.id] && !brokenThumbs[item.id]" :src="thumbnails[item.id]" alt="" @error="brokenThumbs[item.id] = true" />
            <Image v-else :size="24" class="si-placeholder" />
          </div>
          <div class="si-meta">
            <span class="si-time">{{ formatTime(item.created_at) }}</span>
            <span class="si-source">{{ sourceLabel(item.source) }}</span>
          </div>
          <div class="si-actions" @click.stop>
            <button class="si-del-btn" @click="deleteItem(item.id)">
              <X :size="10" />
            </button>
          </div>
          <div v-if="item.is_active" class="si-active-badge"></div>
        </div>
      </div>
      <div v-else class="si-empty">{{ t('footer.no_images') }}</div>
    </div>

    <div v-if="expanded && panel === 'queue'" class="footer-queue">
      <QueuePanel />
    </div>

    <div v-if="expanded && panel === 'log'" class="footer-log">
      <div class="log-toolbar">
        <div class="log-filters">
          <button class="filter-btn" :class="{ active: filterLevel === 'all' }" @click.stop="filterLevel = 'all'">{{ t('footer.filter_all') }}</button>
          <button class="filter-btn" :class="{ active: filterLevel === 'error' }" @click.stop="filterLevel = 'error'">{{ t('footer.filter_errors') }}</button>
          <button class="filter-btn" :class="{ active: filterLevel === 'warn' }" @click.stop="filterLevel = 'warn'">{{ t('footer.filter_warnings') }}</button>
          <button class="filter-btn" :class="{ active: filterLevel === 'info' }" @click.stop="filterLevel = 'info'">{{ t('footer.filter_info') }}</button>
          <button class="filter-btn" :class="{ active: filterLevel === 'debug' }" @click.stop="filterLevel = 'debug'">{{ t('footer.filter_debug') }}</button>
        </div>
        <button class="filter-btn" @click.stop="logs.splice(0, logs.length)">{{ t('footer.btn_clear_logs') }}</button>
      </div>
      <div class="log-entries" ref="logContainer">
        <div v-for="(entry, i) in filteredLogs()" :key="i" class="log-entry" :class="levelClass(entry.level)">
          <span class="log-time">{{ entry.timestamp }}</span>
          <span class="log-level">{{ entry.level.toUpperCase() }}</span>
          <span class="log-msg">{{ entry.message }}</span>
        </div>
        <div v-if="logs.length === 0" class="log-empty">{{ t('footer.no_logs') }}</div>
      </div>
    </div>

    <ImageViewer
      v-if="viewerIndex >= 0"
      :imageBase64="viewerImage"
      :hasPrev="viewerIndex > 0"
      :hasNext="viewerIndex < items.length - 1"
      :item="viewerItem"
      :showActions="true"
      :confirmDelete="confirmDeleteViewer"
      @close="closeViewer"
      @prev="viewerPrev"
      @next="viewerNext"
      @remix="viewerRemix"
      @edit="viewerEdit"
      @export="viewerExport"
      @delete="viewerDelete"
    />
  </div>
</template>

<script>
export default { name: 'AppFooter' }
</script>

<style scoped>
.app-footer {
  border-top: 1px solid var(--border);
  background: var(--surface);
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

.status-dot {
  font-size: 12px;
  font-weight: 500;
  padding: 1px 6px;
  border-radius: 3px;
  cursor: pointer;
  transition: background 0.15s;
}

.status-dot:hover {
  background: var(--surface-2);
}

.status-dot {
  color: var(--text);
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

/* Session panel */
.footer-session {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-top: 1px solid var(--border);
}

.session-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 12px;
  background: var(--surface-2);
  border-bottom: 1px solid var(--border);
  gap: 8px;
}

.session-selector {
  display: flex;
  align-items: center;
  gap: 4px;
}

.session-select {
  appearance: none;
  background-color: var(--surface-2);
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 12 12'%3E%3Cpath fill='%237a7a8e' d='M6 8L1 3h10z'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 8px center;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  padding: 4px 26px 4px 10px;
  font-size: 12px;
  max-width: 220px;
  cursor: pointer;
  outline: none;
  transition: border-color 0.15s;
}

.session-select:hover,
.session-select:focus {
  border-color: var(--accent);
}

.session-select option {
  background: var(--surface);
  color: var(--text);
}

.session-actions {
  display: flex;
  gap: 4px;
  align-items: center;
}

.session-tool-btn {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 1px solid var(--border);
  border-radius: 3px;
  color: var(--text-dim);
  cursor: pointer;
  padding: 0;
}

.session-tool-btn:hover:not(:disabled) {
  color: var(--text);
  border-color: var(--text-dim);
}

.session-tool-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.session-tool-btn.text-btn {
  width: auto;
  padding: 2px 8px;
  font-size: 11px;
}

.session-delete-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  background: transparent;
  border: 1px solid var(--border);
  border-radius: 3px;
  color: var(--text-dim);
  cursor: pointer;
  font-size: 11px;
  padding: 2px 8px;
  height: 24px;
  min-width: 136px;
  white-space: nowrap;
}

.session-delete-btn:hover:not(:disabled) {
  color: var(--danger, #e55);
  border-color: var(--danger, #e55);
}

.session-delete-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.session-delete-btn.confirm {
  color: #fff;
  background: var(--danger, #e55);
  border-color: var(--danger, #e55);
}

.session-rename-input {
  width: 120px;
  padding: 2px 6px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 3px;
  color: var(--text);
  font-size: 12px;
}

.session-new {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  background: var(--surface-1);
  border-bottom: 1px solid var(--border);
}

/* Session item grid */
.si-grid {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-wrap: wrap;
  align-content: flex-start;
  gap: 8px;
  padding: 8px 12px;
}

.session-select-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  font-size: 12px;
  color: var(--text-dim);
}

.si-selected {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.si-check {
  position: absolute;
  top: 4px;
  left: 4px;
  z-index: 2;
  width: 18px;
  height: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent);
  color: #fff;
  border-radius: var(--radius-sm);
}

.si-card {
  width: 80px;
  border: 2px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  position: relative;
  background: var(--surface-1);
  transition: border-color 0.15s;
}

.si-card:hover {
  border-color: var(--text-dim);
}

.si-card.si-active {
  border-color: var(--accent);
}

.si-thumb {
  width: 100%;
  aspect-ratio: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface-2);
  overflow: hidden;
}

.si-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.si-placeholder {
  color: var(--text-dim);
}

.si-meta {
  padding: 2px 4px;
  display: flex;
  justify-content: space-between;
  gap: 4px;
  min-width: 0;
  font-size: 10px;
  line-height: 1.3;
}

.si-time {
  color: var(--text-dim);
}

.si-time {
  flex-shrink: 0;
}

.si-source {
  color: var(--text-dim);
  font-weight: 500;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.si-steps-badge {
  position: absolute;
  top: 2px;
  left: 2px;
  z-index: 1;
  min-width: 14px;
  height: 14px;
  padding: 0 3px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 3px;
  color: var(--accent);
  font-size: 9px;
  font-weight: 600;
  line-height: 1;
  pointer-events: none;
}

.si-actions {
  position: absolute;
  top: 2px;
  right: 2px;
  opacity: 0;
  transition: opacity 0.15s;
}

.si-card:hover .si-actions {
  opacity: 1;
}

.si-del-btn {
  width: 18px;
  height: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 3px;
  color: var(--text-dim);
  cursor: pointer;
  padding: 0;
}

.si-del-btn:hover {
  color: var(--danger, #e55);
  border-color: var(--danger, #e55);
}

.si-active-badge {
  position: absolute;
  bottom: 2px;
  right: 2px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
}

.si-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-dim);
  font-size: 13px;
}

/* Log panel */
.footer-log {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-top: 1px solid var(--border);
}

/* Queue panel */
.footer-queue {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-top: 1px solid var(--border);
}

.log-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 12px;
  background: var(--surface-2);
  border-bottom: 1px solid var(--border);
}

.log-filters {
  display: flex;
  gap: 4px;
}

.filter-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 11px;
  cursor: pointer;
}

.filter-btn.active {
  background: var(--accent-bg);
  border-color: var(--accent);
  color: var(--text-bright);
}

.filter-btn:hover { border-color: var(--text-dim); }

.log-entries {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0;
  font-family: 'Menlo', 'Consolas', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
}

.log-entry {
  display: flex;
  gap: 8px;
  padding: 2px 12px;
}

.log-entry:hover { background: var(--surface-2); }

.log-time { color: var(--text-dim); flex-shrink: 0; width: 60px; }
.log-level { flex-shrink: 0; width: 44px; font-weight: 600; font-size: 11px; }

.log-error .log-level { color: var(--danger); }
.log-error { background: rgba(224, 85, 85, 0.06); }
.log-warn .log-level { color: var(--warning); }
.log-info .log-level { color: var(--accent); }
.log-debug .log-level { color: var(--text-dim); }
.log-debug { color: var(--text-dim); }

.log-msg { word-break: break-word; min-width: 0; }

.log-empty {
  text-align: center;
  color: var(--text-dim);
  padding: 20px;
  font-family: inherit;
  font-size: 13px;
}
</style>
