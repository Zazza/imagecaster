<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { Image, Trash2, X, Plus, Pencil, Check } from 'lucide-vue-next'
import ImageViewer from '../ImageViewer.vue'
import { useRemixQueue } from '../../remixQueue.js'
import { registerHistoryViewer } from '../../historyViewer.js'

const props = defineProps({
  open: { type: Boolean, default: false },
})
const emit = defineEmits(['navigate', 'count', 'error'])

const sessions = ref([])
const activeSessionId = ref(0)
const items = ref([])
const brokenThumbs = ref({})
const itemContainer = ref(null)

const { setPostQueue } = useRemixQueue()
const selectMode = ref(false)
const selectedIds = ref([])
const selectedCount = computed(() => items.value.filter(i => selectedIds.value.includes(i.id)).length)
const showMoveList = ref(false)
const moveListStyle = ref({})
const moving = ref(false)
let moveErrorShown = false
const moveTargets = computed(() => (sessions.value || []).filter(s => s.id !== activeSessionId.value))

const newSessionName = ref('')
const showNewSession = ref(false)
const renamingId = ref(0)
const renameName = ref('')
const confirmDeleteSession = ref(false)
const confirmDeleteViewer = ref(false)
let confirmDeleteTimer = null

const viewerIndex = ref(-1)
let eventOffs = []

const activeSessions = computed(() => (sessions.value || []).find(s => s.id === activeSessionId.value))
const itemCount = computed(() => {
  const s = activeSessions.value
  return s ? s.item_count : 0
})
watch(itemCount, n => emit('count', n), { immediate: true })

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
  if (item.id > 0) lines.push(`#${item.id}`)
  return lines.join('\n')
}

function formatTime(ts) {
  if (!ts) return ''
  return ts.replace(/^.*?(\d{2}:\d{2}).*/, '$1')
}

function scrollToEnd() {
  nextTick(() => {
    if (props.open && itemContainer.value) {
      itemContainer.value.scrollTop = itemContainer.value.scrollHeight
    }
  })
}

function toggleSelectItem(item) {
  const idx = selectedIds.value.indexOf(item.id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(item.id)
}

function exitSelectMode() {
  moveErrorShown = false
  selectMode.value = false
  selectedIds.value = []
  showMoveList.value = false
}

function toggleMoveList(e) {
  if (showMoveList.value) {
    showMoveList.value = false
    return
  }
  const r = e.currentTarget.getBoundingClientRect()
  const below = window.innerHeight - r.bottom
  moveListStyle.value = below >= 200 || below >= r.top
    ? { left: r.left + 'px', top: (r.bottom + 4) + 'px', maxHeight: Math.max(below - 12, 80) + 'px' }
    : { left: r.left + 'px', bottom: (window.innerHeight - r.top + 4) + 'px', maxHeight: Math.max(r.top - 12, 80) + 'px' }
  showMoveList.value = true
}

function closeMoveListOutside(e) {
  if (showMoveList.value && !e.target.closest?.('.session-move')) showMoveList.value = false
}

function closeMoveList() {
  showMoveList.value = false
}

async function moveSelected(sessionId) {
  const ids = items.value.filter(i => selectedIds.value.includes(i.id)).map(i => i.id)
  showMoveList.value = false
  if (!ids.length || moving.value) return
  moving.value = true
  moveErrorShown = false
  try {
    await api.moveSessionItems(ids, sessionId)
    exitSelectMode()
    await loadItems()
    await loadSessions()
  } catch (e) {
    await loadItems()
    await loadSessions()
    selectedIds.value = selectedIds.value.filter(id => items.value.some(i => i.id === id))
    moveErrorShown = true
    emit('error', t('footer.move_failed', { error: String(e?.message || e) }))
  } finally {
    moving.value = false
  }
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

async function loadSessions() {
  try {
    const result = await api.listSessions()
    sessions.value = result || []
    const active = sessions.value.find(s => s.is_active) || sessions.value[0]
    activeSessionId.value = active ? active.id : 0
    if (!moveErrorShown) emit('error', '')
  } catch (e) {
    emit('error', 'loadSessions: ' + String(e))
    sessions.value = []
  }
}

async function loadItems() {
  try {
    const result = await api.getSessionItems()
    items.value = result || []
  } catch (e) {
    emit('error', 'loadItems: ' + String(e))
    items.value = []
  }
}

async function selectSession(id) {
  if (id === activeSessionId.value) return
  try {
    await api.switchSession(id)
    await loadSessions()
    await loadItems()
  } catch (e) {
    emit('error', String(e))
  }
}

async function doCreateSession() {
  const name = newSessionName.value.trim()
  if (!name) return
  try {
    await api.createSession(name)
    newSessionName.value = ''
    showNewSession.value = false
    await loadSessions()
    await loadItems()
  } catch (e) {
    emit('error', String(e))
  }
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
  } catch (e) {
    emit('error', String(e))
  }
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
    await loadItems()
  } catch (e) {
    emit('error', String(e))
  }
}

async function doClearSession() {
  try {
    await api.clearSession()
    items.value = []
    await loadSessions()
  } catch (e) {
    emit('error', String(e))
  }
}

async function activateItem(item, target) {
  try {
    await api.setActiveSessionItem(item.id)
    items.value.forEach(i => { i.is_active = i.id === item.id })
    emit('navigate', target)
  } catch (e) {
    emit('error', String(e))
  }
}

async function deleteItem(id) {
  try {
    await api.deleteSessionItem(id)
    items.value = items.value.filter(i => i.id !== id)
    await loadSessions()
  } catch (e) {
    emit('error', String(e))
  }
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

async function openViewerById(id) {
  let idx = items.value.findIndex(i => i.id === id)
  if (idx < 0) {
    await loadItems()
    idx = items.value.findIndex(i => i.id === id)
  }
  if (idx < 0) return false
  viewerIndex.value = idx
  return true
}

const unregisterHistoryViewer = registerHistoryViewer(openViewerById)
onUnmounted(unregisterHistoryViewer)

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

watch(() => props.open, (on) => {
  if (!on) return
  loadSessions().then(() => loadItems()).then(scrollToEnd).catch(e => emit('error', String(e)))
})

onMounted(async () => {
  eventOffs.push(
    EventsOn('session:added', (data) => {
      loadSessions()
      if (data?.session_id && activeSessionId.value && data.session_id !== activeSessionId.value) return
      loadItems().then(scrollToEnd)
    }),
    EventsOn('session:removed', () => { loadItems(); loadSessions() }),
    EventsOn('session:cleared', () => { loadItems(); loadSessions() }),
    EventsOn('session:switched', () => { loadItems(); loadSessions() }),
    EventsOn('session:active', (data) => {
      const id = data?.id
      if (id && items.value.some(i => i.id === id)) items.value.forEach(i => { i.is_active = i.id === id })
    }),
    EventsOn('session:deleted', () => { loadSessions(); loadItems() }),
    EventsOn('session:created', () => { loadSessions(); loadItems() }),
  )
  document.addEventListener('mousedown', closeMoveListOutside)
  window.addEventListener('resize', closeMoveList)
  await loadSessions()
  await loadItems()
})

onUnmounted(() => {
  document.removeEventListener('mousedown', closeMoveListOutside)
  window.removeEventListener('resize', closeMoveList)
  eventOffs.forEach(off => off?.())
  eventOffs = []
})
</script>

<template>
  <div v-if="open" class="footer-session">
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
      <div class="session-move">
        <button class="session-tool-btn text-btn" :disabled="!selectedCount || !moveTargets.length || moving" :title="moveTargets.length ? '' : t('footer.move_no_targets')" @click="toggleMoveList">{{ t('footer.move_to', { n: selectedCount }) }}</button>
        <div v-if="showMoveList" class="session-move-list" role="menu" :style="moveListStyle">
          <button v-for="s in moveTargets" :key="s.id" class="session-move-item" role="menuitem" @click="moveSelected(s.id)">{{ s.name }}</button>
        </div>
      </div>
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
          <img v-if="!brokenThumbs[item.id]" :src="api.sessionThumbUrl(item.id)" alt="" loading="lazy" @error="brokenThumbs[item.id] = true" />
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
  <ImageViewer
    v-if="viewerIndex >= 0"
    :imageBase64="viewerImage"
    :hasPrev="viewerIndex > 0"
    :hasNext="viewerIndex < items.length - 1"
    :position="viewerIndex + 1"
    :total="items.length"
    :item="viewerItem"
    :itemNumber="viewerItem?.id || 0"
    :showActions="true"
    :confirmDelete="confirmDeleteViewer"
    @close="closeViewer"
    @prev="viewerPrev"
    @next="viewerNext"
    @export="viewerExport"
    @delete="viewerDelete"
  />
</template>

<style scoped>
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
  font-size: var(--fs-sm);
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
  font-size: var(--fs-xs);
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
  font-size: var(--fs-xs);
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
  font-size: var(--fs-sm);
}

.session-new {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  background: var(--surface-1);
  border-bottom: 1px solid var(--border);
}

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
  font-size: var(--fs-sm);
  color: var(--text-dim);
}

.session-move {
  position: relative;
}

.session-move-list {
  position: fixed;
  z-index: 1000;
  display: flex;
  flex-direction: column;
  min-width: 160px;
  overflow-y: auto;
  padding: 4px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.35);
}

.session-move-item {
  padding: 4px 8px;
  text-align: left;
  font-size: var(--fs-sm);
  color: var(--text);
  background: none;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.session-move-item:hover {
  background: var(--surface-2);
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
  color: var(--on-accent);
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
  font-size: var(--fs-2xs);
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
  font-size: var(--fs-md);
}</style>
