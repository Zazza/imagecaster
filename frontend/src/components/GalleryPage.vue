<script setup>
import { Images, X } from 'lucide-vue-next'
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { api } from '../api.js'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { t, locale } from '../i18n/index.js'
import { modelLabel } from '../presetFilters.js'
import { periodRange, groupByDay, dayTitle, parseCreatedAt } from '../galleryFilters.js'
import ImageViewer from './ImageViewer.vue'
import StatusMessage from './StatusMessage.vue'

const emit = defineEmits(['navigate'])

const PAGE = 120
const SELECT_ALL_PAGE = 500
const CHIPS_COLLAPSED = 12
const KINDS = ['generate', 'from_image', 'pipeline', 'edit', 'stylist', 'editor', 'other']
const SHAPES = ['portrait', 'landscape', 'square']

const items = ref([])
const total = ref(0)
const facets = ref({ sessions: [], models: [], labels: [], kinds: [], shapes: [], periods: [] })
const loading = ref(false)
const loadingMore = ref(false)
const error = ref('')
const everLoaded = ref(false)

const search = ref('')
const period = ref('all')
const customFrom = ref('')
const customTo = ref('')
const sessionIds = ref([])
const models = ref([])
const labels = ref([])
const kinds = ref([])
const shapes = ref([])
const expanded = ref({})

const selectMode = ref(false)
const selectedIds = ref(new Set())
const showDeleteConfirm = ref(false)
const deleting = ref(false)

const viewerId = ref(null)
const confirmDeleteViewer = ref(false)
const pendingDeleteViewerId = ref(null)
let confirmViewerTimer = null

const sentinel = ref(null)
let observer = null
let requestSeq = 0
let searchTimer = null
let refreshTimer = null
const offEvents = []

const filtersActive = computed(() => Boolean(
  search.value.trim() || period.value !== 'all' || sessionIds.value.length || models.value.length ||
  labels.value.length || kinds.value.length || shapes.value.length))

const groups = computed(() => groupByDay(items.value))
const hasMore = computed(() => items.value.length < total.value)
const selectedCount = computed(() => selectedIds.value.size)
const allSelected = computed(() => total.value > 0 && selectedIds.value.size >= total.value &&
  items.value.every(i => selectedIds.value.has(i.id)))
const viewerIndex = computed(() => viewerId.value == null ? -1 : items.value.findIndex(i => i.id === viewerId.value))
const viewerItem = computed(() => viewerIndex.value >= 0 ? items.value[viewerIndex.value] : null)
const viewerImage = computed(() => viewerItem.value ? api.sessionImageUrl(viewerItem.value.id) : '')
const viewerTitle = computed(() => {
  const it = viewerItem.value
  if (!it) return ''
  const d = parseCreatedAt(it.created_at)
  const when = d ? d.toLocaleString(locale.value === 'ru' ? 'ru-RU' : 'en-US', { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : ''
  return [when, it.session_name, it.label, modelText(it.model_name), it.width && it.height ? `${it.width}×${it.height}` : '']
    .filter(Boolean).join(' · ')
})

function periodStarts() {
  const out = {}
  for (const p of ['today', 'week', 'month']) out[p] = periodRange(p).date_from
  return out
}

async function fetchRange(want, start = 0) {
  let got = []
  let page = null
  while (got.length < want) {
    page = await api.listHistory(buildFilter(start + got.length, Math.min(SELECT_ALL_PAGE, want - got.length)))
    const batch = page?.items || []
    got = got.concat(batch)
    if (batch.length === 0 || start + got.length >= (page?.total || 0)) break
  }
  return { items: got, total: page?.total || 0, facets: page?.facets }
}

function buildFilter(offset, limit) {
  return {
    session_ids: sessionIds.value.map(Number),
    models: models.value,
    labels: labels.value,
    kinds: kinds.value,
    shapes: shapes.value,
    search: search.value.trim(),
    ...periodRange(period.value, { from: customFrom.value, to: customTo.value }),
    period_starts: periodStarts(),
    offset,
    limit,
  }
}

async function load({ keep = false } = {}) {
  const seq = ++requestSeq
  const want = keep ? Math.max(items.value.length, PAGE) : PAGE
  loading.value = !keep
  try {
    const oldTotal = total.value
    const page = await fetchRange(want)
    if (seq !== requestSeq) return
    const grown = page.total - oldTotal
    if (keep && grown > 0 && page.items.length < page.total) {
      const extra = await fetchRange(grown, page.items.length)
      if (seq !== requestSeq) return
      const known = new Set(page.items.map(i => i.id))
      page.items = page.items.concat(extra.items.filter(i => !known.has(i.id)))
    }
    items.value = page.items
    total.value = page.total
    facets.value = page.facets || facets.value
    error.value = ''
    everLoaded.value = true
    const ids = new Set(items.value.map(i => i.id))
    if (!hasMore.value) {
      const kept = [...selectedIds.value].filter(id => ids.has(id))
      if (kept.length !== selectedIds.value.size) selectedIds.value = new Set(kept)
    }
    if (viewerId.value != null && !ids.has(viewerId.value)) closeViewer()
  } catch (e) {
    if (seq !== requestSeq) return
    console.error('listHistory', e)
    error.value = String(e)
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

async function loadMore(limit = PAGE) {
  if (loadingMore.value || loading.value || !hasMore.value) return
  const seq = requestSeq
  loadingMore.value = true
  try {
    const page = await api.listHistory(buildFilter(items.value.length, limit))
    if (seq !== requestSeq) return
    const known = new Set(items.value.map(i => i.id))
    items.value = [...items.value, ...(page?.items || []).filter(i => !known.has(i.id))]
    total.value = page?.total ?? total.value
  } catch (e) {
    console.error('listHistory more', e)
    if (seq === requestSeq) error.value = String(e)
  } finally {
    loadingMore.value = false
  }
}

function scheduleRefresh() {
  clearTimeout(refreshTimer)
  refreshTimer = setTimeout(() => load({ keep: true }), 400)
}

watch([period, customFrom, customTo, sessionIds, models, labels, kinds, shapes], () => {
  selectedIds.value = new Set()
  load()
}, { deep: true })

watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    selectedIds.value = new Set()
    load()
  }, 250)
})

const filterLists = { sessionIds, models, labels, kinds, shapes }

function toggle(name, value) {
  const list = filterLists[name]
  list.value = list.value.includes(value) ? list.value.filter(v => v !== value) : [...list.value, value]
}

function resetFilters() {
  search.value = ''
  period.value = 'all'
  customFrom.value = ''
  customTo.value = ''
  sessionIds.value = []
  models.value = []
  labels.value = []
  kinds.value = []
  shapes.value = []
}

function visibleChips(key, list) {
  if (expanded.value[key] || list.length <= CHIPS_COLLAPSED) return list
  return list.slice(0, CHIPS_COLLAPSED)
}

function toggleExpanded(key) {
  expanded.value = { ...expanded.value, [key]: !expanded.value[key] }
}

function facetCount(list, value) {
  const f = (list || []).find(x => x.value === value)
  return f ? f.count : 0
}

function modelText(raw) {
  return raw ? modelLabel(raw) : t('gallery.model_unknown')
}

function sessionText(f) {
  return f.name || t('gallery.session_deleted')
}

function dayHeading(key) {
  return dayTitle(key, { today: t('gallery.today'), yesterday: t('gallery.yesterday'), locale: locale.value })
}

function onItemClick(item) {
  if (selectMode.value) {
    toggleSelect(item.id)
    return
  }
  viewerId.value = item.id
}

function toggleSelect(id) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

function toggleSelectMode() {
  selectMode.value = !selectMode.value
  if (!selectMode.value) selectedIds.value = new Set()
}

async function selectAll() {
  if (allSelected.value) {
    selectedIds.value = new Set()
    return
  }
  const seq = requestSeq
  try {
    const loaded = items.value.map(i => i.id)
    const rest = hasMore.value ? await fetchRange(total.value - loaded.length, loaded.length) : { items: [] }
    if (seq !== requestSeq) return
    selectedIds.value = new Set([...loaded, ...rest.items.map(i => i.id)])
  } catch (e) {
    console.error('select all history', e)
    error.value = String(e)
  }
}

async function confirmBatchDelete() {
  showDeleteConfirm.value = false
  const ids = [...selectedIds.value]
  if (!ids.length) return
  deleting.value = true
  try {
    await api.deleteHistoryItems(ids)
  } catch (e) {
    console.error('deleteHistoryItems', e)
    error.value = String(e)
  } finally {
    deleting.value = false
    selectedIds.value = new Set()
    selectMode.value = false
    await load({ keep: true })
  }
}

function closeViewer() {
  viewerId.value = null
  confirmDeleteViewer.value = false
  pendingDeleteViewerId.value = null
}

function showAt(idx) {
  const it = items.value[idx]
  if (!it) return
  viewerId.value = it.id
  confirmDeleteViewer.value = false
  pendingDeleteViewerId.value = null
}

function viewerPrev() {
  if (viewerIndex.value > 0) showAt(viewerIndex.value - 1)
}

async function viewerNext() {
  if (viewerIndex.value >= items.value.length - 1 && hasMore.value) await loadMore()
  if (viewerIndex.value >= 0 && viewerIndex.value < items.value.length - 1) showAt(viewerIndex.value + 1)
}

async function viewerDelete() {
  const item = viewerItem.value
  if (!item) return
  if (!confirmDeleteViewer.value || pendingDeleteViewerId.value !== item.id) {
    confirmDeleteViewer.value = true
    pendingDeleteViewerId.value = item.id
    clearTimeout(confirmViewerTimer)
    confirmViewerTimer = setTimeout(() => { confirmDeleteViewer.value = false; pendingDeleteViewerId.value = null }, 3000)
    return
  }
  const id = pendingDeleteViewerId.value
  confirmDeleteViewer.value = false
  pendingDeleteViewerId.value = null
  try {
    await api.deleteHistoryItems([id])
  } catch (e) {
    console.error('delete history item', e)
    error.value = String(e)
    return
  }
  const idx = items.value.findIndex(i => i.id === id)
  items.value = items.value.filter(i => i.id !== id)
  total.value = Math.max(0, total.value - 1)
  if (items.value.length === 0) closeViewer()
  else showAt(Math.min(Math.max(idx, 0), items.value.length - 1))
}

async function openInHistory(target) {
  const item = viewerItem.value
  if (!item) return
  try {
    await api.openInHistory(item.id)
    closeViewer()
    emit('navigate', target)
  } catch (e) {
    console.error('openInHistory', e)
    error.value = String(e)
  }
}

async function saveViewerImage() {
  const item = viewerItem.value
  if (!item) return
  try {
    const b64 = await api.getSessionImage(item.id)
    await api.saveImage(b64, item.file_name || `history_${item.id}.png`)
  } catch (e) {
    console.error('save history image', e)
    error.value = String(e)
  }
}

function setupObserver() {
  if (observer || typeof IntersectionObserver === 'undefined') return
  observer = new IntersectionObserver(entries => {
    if (entries.some(e => e.isIntersecting)) loadMore()
  }, { rootMargin: '600px' })
  if (sentinel.value) observer.observe(sentinel.value)
}

watch(sentinel, (el, old) => {
  if (!observer) return
  if (old) observer.unobserve(old)
  if (el) observer.observe(el)
})

onMounted(async () => {
  for (const name of ['session:added', 'session:removed', 'session:cleared', 'session:deleted', 'session:created']) {
    const off = EventsOn(name, scheduleRefresh)
    if (typeof off === 'function') offEvents.push(off)
  }
  await load()
  await nextTick()
  setupObserver()
})

onUnmounted(() => {
  for (const off of offEvents) off()
  clearTimeout(searchTimer)
  clearTimeout(refreshTimer)
  clearTimeout(confirmViewerTimer)
  if (observer) observer.disconnect()
})
</script>

<template>
  <div class="page-enter">
    <div class="page-header">
      <h1 class="page-title">{{ t('gallery.title') }}</h1>
      <div class="header-actions">
        <template v-if="selectMode">
          <button class="btn btn-secondary btn-sm" data-testid="select-all" :disabled="total === 0" @click="selectAll">
            {{ allSelected ? t('presets.btn_deselect_all') : t('gallery.select_all', { count: total }) }}
          </button>
          <button class="btn btn-danger-quiet btn-sm" data-testid="delete-selected" :disabled="selectedCount === 0 || deleting" @click="showDeleteConfirm = true">
            {{ t('presets.btn_delete_selected', { count: selectedCount }) }}
          </button>
          <button class="btn btn-secondary btn-sm" @click="toggleSelectMode">{{ t('presets.btn_cancel') }}</button>
        </template>
        <button v-else class="btn btn-secondary" data-testid="select-mode" :disabled="total === 0" @click="toggleSelectMode">{{ t('presets.btn_select') }}</button>
      </div>
    </div>

    <StatusMessage v-if="error" @close="error = ''">{{ error }}</StatusMessage>

    <div v-if="everLoaded && !filtersActive && total === 0 && !loading" class="empty-state" data-testid="gallery-empty">
      <div class="empty-state-icon"><Images :size="40" :stroke-width="1.5" /></div>
      <p>{{ t('gallery.empty') }}</p>
    </div>

    <template v-else>
      <div class="styles-filters">
        <div class="styles-filter-row">
          <input class="form-input styles-search" data-testid="filter-search" v-model="search" :placeholder="t('gallery.search')" />
          <div class="styles-chips">
            <button
              v-for="p in ['all', 'today', 'week', 'month', 'custom']"
              :key="p"
              class="styles-chip"
              data-testid="filter-period"
              :data-period="p"
              :class="{ active: period === p }"
              @click="period = p"
            >{{ t('gallery.period_' + p) }}<span v-if="p !== 'custom'" class="styles-chip-count"> {{ facetCount(facets.periods, p) }}</span></button>
          </div>
          <template v-if="period === 'custom'">
            <input class="form-input gal-date" type="date" data-testid="filter-date-from" v-model="customFrom" :aria-label="t('gallery.date_from')" />
            <span class="styles-filter-label">—</span>
            <input class="form-input gal-date" type="date" data-testid="filter-date-to" v-model="customTo" :aria-label="t('gallery.date_to')" />
          </template>
          <div class="styles-chips">
            <button
              v-for="s in SHAPES"
              :key="s"
              class="styles-chip"
              data-testid="filter-shape"
              :data-shape="s"
              :class="{ active: shapes.includes(s) }"
              @click="toggle('shapes', s)"
            >{{ t('gallery.shape_' + s) }} <span class="styles-chip-count">{{ facetCount(facets.shapes, s) }}</span></button>
          </div>
          <button v-if="filtersActive" class="btn btn-secondary btn-sm" data-testid="filter-reset" @click="resetFilters">{{ t('presets.filter_reset') }}</button>
        </div>

        <div class="styles-chips">
          <span class="styles-filter-label">{{ t('gallery.filter_kind') }}</span>
          <template v-for="k in KINDS" :key="k">
            <button
              v-if="facetCount(facets.kinds, k) > 0 || kinds.includes(k)"
              class="styles-chip"
              data-testid="filter-kind"
              :data-kind="k"
              :class="{ active: kinds.includes(k) }"
              @click="toggle('kinds', k)"
            >{{ t('gallery.kind_' + k) }} <span class="styles-chip-count">{{ facetCount(facets.kinds, k) }}</span></button>
          </template>
        </div>

        <div v-if="facets.sessions.length" class="styles-chips">
          <span class="styles-filter-label">{{ t('gallery.filter_session') }}</span>
          <button
            v-for="f in visibleChips('sessions', facets.sessions)"
            :key="f.value"
            class="styles-chip"
            data-testid="filter-session"
            :data-session="f.value"
            :class="{ active: sessionIds.includes(f.value) }"
            @click="toggle('sessionIds', f.value)"
          >{{ sessionText(f) }} <span class="styles-chip-count">{{ f.count }}</span></button>
          <button v-if="facets.sessions.length > CHIPS_COLLAPSED" class="gal-more" @click="toggleExpanded('sessions')">{{ expanded.sessions ? t('gallery.less') : t('gallery.more', { count: facets.sessions.length - CHIPS_COLLAPSED }) }}</button>
        </div>

        <div v-if="facets.models.length" class="styles-chips">
          <span class="styles-filter-label">{{ t('presets.filter_model') }}</span>
          <button
            v-for="f in visibleChips('models', facets.models)"
            :key="f.value"
            class="styles-chip"
            data-testid="filter-model"
            :data-model="f.value"
            :title="f.value"
            :class="{ active: models.includes(f.value) }"
            @click="toggle('models', f.value)"
          >{{ modelText(f.value) }} <span class="styles-chip-count">{{ f.count }}</span></button>
          <button v-if="facets.models.length > CHIPS_COLLAPSED" class="gal-more" @click="toggleExpanded('models')">{{ expanded.models ? t('gallery.less') : t('gallery.more', { count: facets.models.length - CHIPS_COLLAPSED }) }}</button>
        </div>

        <div v-if="facets.labels.length" class="styles-chips">
          <span class="styles-filter-label">{{ t('gallery.filter_label') }}</span>
          <button
            v-for="f in visibleChips('labels', facets.labels)"
            :key="f.value"
            class="styles-chip"
            data-testid="filter-label"
            :data-label="f.value"
            :class="{ active: labels.includes(f.value), 'gal-pipeline': f.pipeline }"
            :title="f.pipeline ? t('gallery.kind_pipeline') : ''"
            @click="toggle('labels', f.value)"
          >{{ f.value }} <span class="styles-chip-count">{{ f.count }}</span></button>
          <button v-if="facets.labels.length > CHIPS_COLLAPSED" class="gal-more" @click="toggleExpanded('labels')">{{ expanded.labels ? t('gallery.less') : t('gallery.more', { count: facets.labels.length - CHIPS_COLLAPSED }) }}</button>
        </div>
      </div>

      <div class="gal-summary" data-testid="gallery-total">{{ t('gallery.total', { count: total }) }}</div>

      <div v-if="loading && !items.length" style="padding: 40px; text-align: center;"><span class="spinner"></span></div>

      <div v-else-if="total === 0" class="styles-empty" data-testid="no-match">
        <p>{{ t('gallery.no_match') }}</p>
        <button class="btn btn-secondary btn-sm" @click="resetFilters">{{ t('presets.filter_reset') }}</button>
      </div>

      <div v-else>
        <section v-for="g in groups" :key="g.key" class="gal-day" data-testid="gallery-day">
          <h3 class="gal-day-title">{{ dayHeading(g.key) }} <span class="gal-day-count">{{ g.items.length }}</span></h3>
          <div class="gal-grid">
            <button
              v-for="it in g.items"
              :key="it.id"
              class="gal-tile"
              data-testid="gallery-item"
              :data-id="it.id"
              :class="{ selected: selectedIds.has(it.id) }"
              :title="[it.label, modelText(it.model_name)].filter(Boolean).join(' · ')"
              @click="onItemClick(it)"
            >
              <img :src="api.sessionThumbUrl(it.id)" loading="lazy" decoding="async" alt="" />
              <input v-if="selectMode" type="checkbox" class="gal-check" :checked="selectedIds.has(it.id)" tabindex="-1" @click.stop="toggleSelect(it.id)" />
            </button>
          </div>
        </section>
        <div ref="sentinel" class="gal-sentinel">
          <span v-if="loadingMore" class="spinner"></span>
          <button v-else-if="hasMore" class="btn btn-secondary btn-sm" data-testid="load-more" @click="loadMore()">{{ t('gallery.load_more') }}</button>
        </div>
      </div>
    </template>

    <div v-if="showDeleteConfirm" class="modal-overlay">
      <div class="modal">
        <div class="modal-header">
          <h2 class="modal-title">{{ t('gallery.delete_title') }}</h2>
          <button class="modal-close" @click="showDeleteConfirm = false"><X :size="16" /></button>
        </div>
        <p>{{ t('gallery.delete_confirm_n', { count: selectedCount }) }}</p>
        <p style="color: var(--text-dim); font-size: 0.9em;">{{ t('gallery.delete_hint') }}</p>
        <div style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 16px;">
          <button class="btn btn-secondary" @click="showDeleteConfirm = false">{{ t('presets.btn_cancel') }}</button>
          <button class="btn btn-danger" data-testid="confirm-delete" @click="confirmBatchDelete">{{ t('presets.btn_delete') }}</button>
        </div>
      </div>
    </div>

    <ImageViewer
      v-if="viewerItem"
      :imageBase64="viewerImage"
      :hasPrev="viewerIndex > 0"
      :hasNext="viewerIndex < items.length - 1 || hasMore"
      :position="viewerIndex + 1"
      :total="total"
      :item="viewerItem"
      :title="viewerTitle"
      :showActions="true"
      :confirmDelete="confirmDeleteViewer"
      @close="closeViewer"
      @prev="viewerPrev"
      @next="viewerNext"
      @export="openInHistory({ page: 'postprocess' })"
      @delete="viewerDelete"
    >
      <template #actions>
        <button class="image-viewer-tool-btn" data-testid="viewer-save" @click="saveViewerImage">{{ t('gallery.save') }}</button>
        <button class="image-viewer-tool-btn" data-testid="viewer-open-history" @click="openInHistory({ page: 'generate' })">{{ t('gallery.open_in_history') }}</button>
      </template>
    </ImageViewer>
  </div>
</template>

<style scoped>
.gal-date { width: auto; }
.gal-more { border: none; background: transparent; color: var(--accent); font-size: 12px; cursor: pointer; padding: 0 4px; }
.gal-pipeline { border-style: dashed; }
.gal-summary { font-size: 12px; color: var(--text-dim); margin-bottom: 8px; }
.gal-day { margin-bottom: 16px; }
.gal-day-title { font-size: 13px; font-weight: 600; color: var(--text-bright); margin: 0 0 8px; }
.gal-day-count { font-size: 11px; color: var(--text-dim); font-weight: 400; margin-left: 4px; }
.gal-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 8px; }
.gal-tile {
  position: relative; aspect-ratio: 1; padding: 0; border: 2px solid transparent; border-radius: var(--radius-sm);
  background: var(--surface-2); overflow: hidden; cursor: pointer;
}
.gal-tile img { width: 100%; height: 100%; object-fit: cover; display: block; }
.gal-tile:hover { border-color: var(--border); }
.gal-tile.selected { border-color: var(--accent); }
.gal-check { position: absolute; top: 6px; left: 6px; }
.gal-sentinel { display: flex; justify-content: center; padding: 16px; min-height: 40px; }
</style>
