<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { useRemixQueue } from '../remixQueue.js'
import { sortEntries, normalizeSort } from '../fileSort.js'
import ImageViewer from './ImageViewer.vue'
import { FolderUp, Folder, FolderOpen, Image, Check, ArrowUp, ArrowDown } from 'lucide-vue-next'

const emit = defineEmits(['navigate'])

const currentPath = ref('')
const entries = ref([])
const loading = ref(false)
const error = ref('')

const thumbnails = ref({})
const loadedSet = new Set()
let loadGeneration = 0

const viewerIndex = ref(-1)
const selectMode = ref(false)
const selectedPaths = ref([])
const { setPostQueue } = useRemixQueue()
const SORT_STORAGE_KEY = 'fb_sort'

function loadSort() {
  try {
    const saved = JSON.parse(localStorage.getItem(SORT_STORAGE_KEY) || 'null')
    return normalizeSort(saved?.key, saved?.dir)
  } catch {
    return normalizeSort()
  }
}

const initialSort = loadSort()
const sortKey = ref(initialSort.key)
const sortDir = ref(initialSort.dir)

function saveSort() {
  try {
    localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify({ key: sortKey.value, dir: sortDir.value }))
  } catch {}
}

function setSortKey(key) {
  sortKey.value = key
  saveSort()
}

function toggleSortDir() {
  sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  saveSort()
}

const sortedEntries = computed(() => sortEntries(entries.value, sortKey.value, sortDir.value))
const imageEntries = computed(() => sortedEntries.value.filter(e => !e.is_dir))
const imageIndexMap = computed(() => {
  const map = new Map()
  imageEntries.value.forEach((e, i) => map.set(e.path, i))
  return map
})

const fullImages = ref({})
const fullLoading = ref(new Set())

const viewerImage = computed(() => {
  if (viewerIndex.value < 0 || viewerIndex.value >= imageEntries.value.length) return ''
  const entry = imageEntries.value[viewerIndex.value]
  return fullImages.value[entry.path] || thumbnails.value[entry.path] || ''
})

let observer = null

function setupObserver() {
  observer = new IntersectionObserver((items) => {
    for (const item of items) {
      if (item.isIntersecting) {
        const path = item.target.dataset.path
        if (path && !loadedSet.has(path)) {
          loadedSet.add(path)
          loadSingleThumbnail(path)
        }
        observer.unobserve(item.target)
      }
    }
  }, { rootMargin: '200px' })
}

function observeCards() {
  if (!observer) return
  const cards = document.querySelectorAll('.fb-card-img[data-path]')
  cards.forEach(el => observer.observe(el))
}

async function loadSingleThumbnail(path) {
  try {
    const b64 = await api.readThumbnail(path)
    if (b64) thumbnails.value[path] = b64
  } catch {}
}

async function loadPath() {
  if (!currentPath.value) return
  loading.value = true
  error.value = ''
  thumbnails.value = {}
  fullImages.value = {}
  loadedSet.clear()
  viewerIndex.value = -1
  exitSelectMode()
  loadGeneration++
  const gen = loadGeneration
  try {
    entries.value = (await api.browseDirectory(currentPath.value)) || []
    if (gen !== loadGeneration) return
    requestAnimationFrame(() => observeCards())
  } catch (e) {
    error.value = String(e)
    entries.value = []
  } finally {
    loading.value = false
    api.updateSettings({ file_browser_path: currentPath.value }).catch(() => {})
  }
}

async function browseFolder() {
  try {
    const path = await api.selectBrowserFolder()
    if (path) {
      currentPath.value = path
      await loadPath()
    }
  } catch (e) {
    error.value = String(e)
  }
}

function goUp() {
  if (!currentPath.value) return
  const p = currentPath.value.replace(/[/\\]+$/, '')
  const sep = p.includes('\\') ? '\\' : '/'
  const idx = p.lastIndexOf(sep)
  if (idx <= 0) return
  currentPath.value = p.substring(0, idx) || sep
  loadPath()
}

function openDir(entry) {
  currentPath.value = entry.path
  loadPath()
}

function openViewer(index) {
  viewerIndex.value = index
  loadFullImage(imageEntries.value[index])
}

function viewerPrev() {
  if (viewerIndex.value > 0) {
    viewerIndex.value--
    loadFullImage(imageEntries.value[viewerIndex.value])
  }
}

function viewerNext() {
  if (viewerIndex.value < imageEntries.value.length - 1) {
    viewerIndex.value++
    loadFullImage(imageEntries.value[viewerIndex.value])
  }
}

async function loadFullImage(entry) {
  if (!entry || fullImages.value[entry.path] || fullLoading.value.has(entry.path)) return
  fullLoading.value.add(entry.path)
  try {
    const b64 = await api.readFileAsBase64(entry.path)
    if (b64) fullImages.value[entry.path] = b64
  } catch {}
  fullLoading.value.delete(entry.path)
}

async function sendToPostProcess(entry) {
  try {
    const b64 = await api.readFileAsBase64(entry.path)
    if (!b64) return
    await api.setLastImage(b64)
    emit('navigate', { page: 'postprocess' })
  } catch (e) {
    error.value = String(e)
  }
}

function formatSize(bytes) {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function mimeForEntry(entry) {
  const ext = (entry.name.split('.').pop() || '').toLowerCase()
  if (ext === 'jpg' || ext === 'jpeg') return 'image/jpeg'
  return 'image/png'
}

function thumbSrc(entry) {
  const b64 = thumbnails.value[entry.path]
  if (!b64) return ''
  return `data:${mimeForEntry(entry)};base64,` + b64
}

function getEntryIndex(entry) {
  return imageIndexMap.value.get(entry.path) ?? -1
}

function handleCardClick(entry) {
  if (entry.is_dir) {
    openDir(entry)
  } else if (selectMode.value) {
    toggleSelect(entry)
  } else {
    openViewer(getEntryIndex(entry))
  }
}

function toggleSelect(entry) {
  const idx = selectedPaths.value.indexOf(entry.path)
  if (idx >= 0) {
    selectedPaths.value.splice(idx, 1)
  } else {
    selectedPaths.value.push(entry.path)
  }
}

function isSelected(entry) {
  return selectedPaths.value.includes(entry.path)
}

function exitSelectMode() {
  selectMode.value = false
  selectedPaths.value = []
}

function sendSelectedToPostProcess() {
  const items = imageEntries.value
    .filter(e => selectedPaths.value.includes(e.path))
    .map(e => ({ path: e.path, name: e.name }))
  if (!items.length) return
  setPostQueue(items)
  exitSelectMode()
  emit('navigate', { page: 'postprocess' })
}

onMounted(async () => {
  setupObserver()
  try {
    const settings = await api.getSettings()
    if (settings?.file_browser_path) {
      currentPath.value = settings.file_browser_path
      await loadPath()
    }
  } catch {}
})

onUnmounted(() => {
  if (observer) observer.disconnect()
  loadGeneration++
})
</script>

<template>
  <div class="file-browser">
    <h1 class="page-title">{{ t('app.nav_my_images') }}</h1>
    <div class="fb-path-bar">
      <input
        v-model="currentPath"
        class="fb-path-input"
        :placeholder="t('browser.placeholder_path')"
        @keydown.enter="loadPath"
      />
      <button class="btn btn-secondary" @click="browseFolder">
        <FolderOpen :size="14" /> {{ t('browser.btn_browse') }}
      </button>
      <button class="btn btn-secondary" @click="goUp" :disabled="!currentPath" :title="t('browser.btn_up')" :aria-label="t('browser.btn_up')">
        <FolderUp :size="14" />
      </button>
      <button class="btn btn-primary" @click="loadPath" :disabled="!currentPath || loading">
        {{ t('browser.btn_go') }}
      </button>
      <button v-if="!selectMode && imageEntries.length" class="btn btn-secondary" @click="selectMode = true">
        {{ t('browser.select_mode') }}
      </button>
    </div>

    <div v-if="selectMode" class="fb-select-bar">
      <span class="fb-select-count">{{ t('browser.selected_count', { n: selectedPaths.length }) }}</span>
      <button class="btn btn-primary" :disabled="!selectedPaths.length" @click="sendSelectedToPostProcess">
        {{ t('browser.send_to_postprocess_multi', { n: selectedPaths.length }) }}
      </button>
      <button class="btn btn-secondary" @click="exitSelectMode">
        {{ t('browser.cancel_select') }}
      </button>
    </div>

    <div v-if="error" class="fb-error">{{ error }}</div>

    <div v-if="!loading && entries.length" class="fb-sort-bar">
      <span class="fb-sort-label">{{ t('browser.sort_by') }}</span>
      <div class="fb-sort-group">
        <button class="fb-sort-btn" :class="{ active: sortKey === 'date' }" @click="setSortKey('date')">{{ t('browser.sort_date') }}</button>
        <button class="fb-sort-btn" :class="{ active: sortKey === 'name' }" @click="setSortKey('name')">{{ t('browser.sort_name') }}</button>
      </div>
      <button class="fb-sort-btn fb-sort-dir" :title="sortDir === 'asc' ? t('browser.sort_asc') : t('browser.sort_desc')" :aria-label="sortDir === 'asc' ? t('browser.sort_asc') : t('browser.sort_desc')" @click="toggleSortDir">
        <ArrowUp v-if="sortDir === 'asc'" :size="14" />
        <ArrowDown v-else :size="14" />
        {{ sortDir === 'asc' ? t('browser.sort_asc') : t('browser.sort_desc') }}
      </button>
    </div>

    <div v-if="loading" class="fb-loading">{{ t('browser.loading') }}</div>

    <div v-else-if="entries.length" class="fb-grid">
      <div
        v-for="entry in sortedEntries"
        :key="entry.path"
        class="fb-card"
        :class="{ 'fb-card-dir': entry.is_dir, 'fb-card-selected': !entry.is_dir && selectMode && isSelected(entry) }"
        @click="handleCardClick(entry)"
      >
        <template v-if="entry.is_dir">
          <div class="fb-thumb fb-thumb-dir">
            <Folder :size="56" :stroke-width="1.5" />
          </div>
          <div class="fb-card-info">
            <div class="fb-card-name">{{ entry.name }}</div>
            <div class="fb-card-meta">{{ t('browser.folder') }}</div>
          </div>
        </template>
        <template v-else>
          <div class="fb-thumb fb-card-img" :data-path="entry.path">
            <img v-if="thumbnails[entry.path]" :src="thumbSrc(entry)" :alt="entry.name" />
            <Image v-else :size="32" class="fb-thumb-placeholder" />
          </div>
          <div class="fb-card-info">
            <div class="fb-card-name">{{ entry.name }}</div>
            <div class="fb-card-meta">{{ formatSize(entry.size) }}</div>
          </div>
          <div v-if="selectMode && isSelected(entry)" class="fb-check">
            <Check :size="14" />
          </div>
        </template>
      </div>
    </div>

    <div v-else-if="currentPath && !loading" class="fb-empty">
      {{ t('browser.no_images') }}
    </div>

    <div v-else-if="!currentPath" class="fb-empty">
      {{ t('browser.select_folder') }}
    </div>

    <ImageViewer
      v-if="viewerIndex >= 0 && viewerImage"
      :imageBase64="viewerImage"
      :hasPrev="viewerIndex > 0"
      :hasNext="viewerIndex < imageEntries.length - 1"
      :showActions="true"
      :canDelete="false"
      @close="viewerIndex = -1"
      @prev="viewerPrev"
      @next="viewerNext"
      @export="sendToPostProcess(imageEntries[viewerIndex])"
    />
  </div>
</template>

<style scoped>
.file-browser {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.fb-path-bar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.fb-path-input {
  flex: 1;
  padding: 8px 12px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  font-size: 13px;
}

.fb-path-input:focus {
  outline: none;
  border-color: var(--accent);
}

.fb-select-bar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.fb-select-count {
  font-size: 13px;
  color: var(--text);
}

.fb-card-selected {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.fb-check {
  position: absolute;
  top: 6px;
  left: 6px;
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent);
  color: var(--on-accent);
  border-radius: var(--radius-sm);
}

.fb-error {
  color: var(--danger);
  font-size: 13px;
  padding: 8px;
}

.fb-loading, .fb-empty {
  text-align: center;
  padding: 40px 16px;
  color: var(--text-dim);
  font-size: 14px;
}

.fb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 12px;
}

.fb-card {
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--surface-1);
  cursor: pointer;
  transition: border-color 0.2s;
  position: relative;
}

.fb-card:hover {
  border-color: var(--accent);
}

.fb-card-dir:hover {
  border-color: var(--text-dim);
}

.fb-thumb {
  width: 100%;
  aspect-ratio: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface-2);
  overflow: hidden;
}

.fb-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.fb-thumb-dir {
  color: var(--accent);
}

.fb-thumb-dir svg {
  fill: color-mix(in srgb, var(--accent) 18%, transparent);
}

.fb-sort-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 13px;
}

.fb-sort-label {
  color: var(--text-dim);
}

.fb-sort-group {
  display: flex;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.fb-sort-group .fb-sort-btn {
  border: none;
  border-radius: 0;
}

.fb-sort-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  background: var(--surface-1);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 13px;
  cursor: pointer;
}

.fb-sort-btn:hover {
  border-color: var(--accent);
}

.fb-sort-btn.active {
  background: var(--accent);
  color: var(--on-accent);
}

.fb-thumb-placeholder {
  color: var(--text-dim);
}

.fb-card-info {
  padding: 8px 10px;
}

.fb-card-name {
  font-size: 12px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fb-card-meta {
  font-size: 11px;
  color: var(--text-dim);
  margin-top: 2px;
}

</style>
