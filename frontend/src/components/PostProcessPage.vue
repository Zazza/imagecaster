<script setup>
import StatusMessage from './StatusMessage.vue'
import { ref, reactive, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { b64Url } from '../imageDataUrl.js'
import { useRemixQueue } from '../remixQueue.js'
import { usePresets } from '../composables/usePresets.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { useJobTracker } from '../composables/useJobTracker.js'
import { useImageInput } from '../composables/useImageInput.js'
import { usePostBatch, readBatchImage } from '../composables/usePostBatch.js'
import { usePostAdjust } from '../composables/usePostAdjust.js'
import EditSection from './EditSection.vue'
import RemixSection from './RemixSection.vue'
import ObjectSection from './ObjectSection.vue'
import RepairSection from './RepairSection.vue'
import PostViewer from './post/PostViewer.vue'
import BatchStrip from './post/BatchStrip.vue'
import EffectPanel from './post/EffectPanel.vue'
import AdjustPanel from './post/AdjustPanel.vue'
import FinishPanel from './post/FinishPanel.vue'
import ExportPanel from './post/ExportPanel.vue'
import RecipesPanel from './post/RecipesPanel.vue'

const props = defineProps({
  initialSection: { type: String, default: 'effect' },
  initialImage: { type: String, default: '' },
})

const emit = defineEmits(['clear-initial-image'])

const openSection = ref('effect')
const editSectionRef = ref(null)
const remixSectionRef = ref(null)
const objectSectionRef = ref(null)
const objectCompRef = ref(null)
const repairSectionRef = ref(null)
const repairCompRef = ref(null)
const effectSectionRef = ref(null)
const effectCompRef = ref(null)
const adjustSectionRef = ref(null)
const exportSectionRef = ref(null)
const exportCompRef = ref(null)
const finishSectionRef = ref(null)
const recipesSectionRef = ref(null)
const recipesCompRef = ref(null)

function sectionEl(name) {
  if (name === 'edit') return editSectionRef.value
  if (name === 'remix') return remixSectionRef.value
  if (name === 'object') return objectSectionRef.value
  if (name === 'repair') return repairSectionRef.value
  if (name === 'adjust') return adjustSectionRef.value
  if (name === 'export') return exportSectionRef.value
  if (name === 'finish') return finishSectionRef.value
  if (name === 'recipes') return recipesSectionRef.value
  return effectSectionRef.value
}

function setOpenSection(name) {
  if (openSection.value === name) return
  openSection.value = name
  nextTick(() => {
    sectionEl(name)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  })
}

watch(() => props.initialSection, (val) => {
  if (val) setOpenSection(val)
})

const sourceB64 = ref('')
const currentB64 = ref('')
const sourceName = ref('image')
const imageInfo = ref({ width: 0, height: 0 })
const error = ref('')
const successMsg = ref('')
const status = { error, successMsg }

const appliedEffects = ref([])
const applyingEffect = ref(false)
const recipeRunning = ref(false)
const lookBusy = ref(false)
const lookResultPrompt = ref('')

const post = usePostAdjust({ currentB64, error })
const queueJobs = useJobTracker()

const batch = usePostBatch({
  queueJobs,
  error,
  successMsg,
  onStart() {
    post.resetAdjustAll()
    layerUndo.value = []
    layerRedo.value = []
    appliedEffects.value = []
    setOpenSection('effect')
  },
  onShow(orig, cur, name) {
    sourceB64.value = orig
    currentB64.value = cur
    sourceName.value = name
    loadImageDimensions(cur)
    updateFilename()
  },
  onEmpty() {
    sourceB64.value = ''
    currentB64.value = ''
  },
  onExit() {
    const cur = currentB64.value
    if (cur) setSource(cur, sourceName.value)
  },
})
const { batch: batchState, batchActive, batchBusy, batchExporting } = batch
const { previewB64, previewing, finishActive } = post

const input = useImageInput({ setSource, error })

const maskComp = computed(() => openSection.value === 'object' ? objectCompRef.value : openSection.value === 'repair' ? repairCompRef.value : openSection.value === 'effect' ? effectCompRef.value : null)
const maskMode = computed(() => !!maskComp.value?.maskActive)

const { loadPresets } = usePresets()

function onEffectApplied(b64, entry) {
  appliedEffects.value.push(entry)
  onToolResult(b64)
}

function applyRecipe(r) {
  return recipesCompRef.value?.applyRecipe(r)
}

function onEditApplyRecipe(payload) {
  applyRecipe({ steps: JSON.stringify(payload.steps || []), adjust: JSON.stringify(payload.adjust || {}) })
}

async function onEditApplyLook({ text, strength }) {
  if (lookBusy.value || applyingEffect.value || batchBusy.value || recipeRunning.value) return
  if (!currentB64.value) {
    error.value = t('export.error_no_image')
    return
  }
  if (!text) return
  if (batchActive.value) {
    await batch.runBatchStep(async (b64) => {
      const job = await batch.trackBatchJob(api.enqueueLook({ image_base64: b64, text, strength, seed: null }))
      return job ? () => job.wait : ''
    })
    return
  }
  lookBusy.value = true
  lookResultPrompt.value = ''
  error.value = ''
  successMsg.value = ''
  try {
    const res = await api.applyLookDirect(currentB64.value, text, strength, null)
    if (res?.image) {
      appliedEffects.value.push({ prompt: res.effective_prompt || '', strength })
      onToolResult(res.image)
      lookResultPrompt.value = res.effective_prompt || ''
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    lookBusy.value = false
  }
}

async function onRemixApplyBatch(params) {
  if (!batchActive.value || lookBusy.value || applyingEffect.value || batchBusy.value || recipeRunning.value) return
  await batch.runBatchStep(async (b64) => {
    const job = await batch.trackBatchJob(api.enqueueFromImage({ ...params, image_base64: b64 }))
    return job ? () => job.wait : ''
  })
}

async function onEditApplyBatch(params) {
  if (!batchActive.value || lookBusy.value || applyingEffect.value || batchBusy.value || recipeRunning.value) return
  await batch.runBatchStep(async (b64) => {
    const job = await batch.trackBatchJob(api.enqueueEdit({ ...params, image_base64: b64 }))
    return job ? () => job.wait : ''
  })
}

async function onEditSaveRecipe(payload) {
  try {
    await api.savePostRecipe({
      name: payload.name,
      steps: JSON.stringify(payload.steps || []),
      adjust: JSON.stringify(payload.adjust || {}),
    })
    await recipesCompRef.value?.load()
  } catch (e) {
    error.value = String(e)
  }
}

const LAYER_HISTORY_MAX = 15
const layerUndo = ref([])
const layerRedo = ref([])
const canUndoLayer = computed(() => layerUndo.value.length > 0)
const canRedoLayer = computed(() => layerRedo.value.length > 0)
const canUndoView = computed(() => batchActive.value ? batchState.value.canUndo && !batchBusy.value && !batchExporting.value : maskMode.value ? !!maskComp.value?.canUndoMask : canUndoLayer.value)
const canRedoView = computed(() => batchActive.value ? false : maskMode.value ? !!maskComp.value?.canRedoMask : canRedoLayer.value)

function undoView() {
  if (batchActive.value) batch.undoBatch()
  else if (maskMode.value) maskComp.value?.undoMask()
  else undoLayer()
}

function redoView() {
  if (batchActive.value) return
  if (maskMode.value) maskComp.value?.redoMask()
  else redoLayer()
}

function pushLayerHistory() {
  if (!currentB64.value) return
  layerUndo.value.push(currentB64.value)
  if (layerUndo.value.length > LAYER_HISTORY_MAX) layerUndo.value.shift()
  layerRedo.value = []
}

function undoLayer() {
  if (layerUndo.value.length === 0) return
  layerRedo.value.push(currentB64.value)
  currentB64.value = layerUndo.value.pop()
  loadImageDimensions(currentB64.value)
}

function redoLayer() {
  if (layerRedo.value.length === 0) return
  layerUndo.value.push(currentB64.value)
  if (layerUndo.value.length > LAYER_HISTORY_MAX) layerUndo.value.shift()
  currentB64.value = layerRedo.value.pop()
  loadImageDimensions(currentB64.value)
}

function resetLayer() {
  if (batchActive.value) {
    batch.resetBatch()
    return
  }
  pushLayerHistory()
  currentB64.value = sourceB64.value
  appliedEffects.value = []
}

function onToolResult(b64) {
  if (!b64) return
  pushLayerHistory()
  currentB64.value = b64
  loadImageDimensions(b64)
}

function onRemixSource(b64) {
  if (!b64) return
  pushLayerHistory()
  setSource(b64, 'remix_image', true)
}

function consumeInitialImage() {
  if (!props.initialImage) return
  setSource(props.initialImage, 'dropped_image')
  emit('clear-initial-image')
}

watch(() => props.initialImage, () => consumeInitialImage())

function updateFilename() {
  exportCompRef.value?.updateFilename(sourceName.value)
}

function loadImageDimensions(base64) {
  const img = new Image()
  img.onload = () => {
    imageInfo.value = { width: img.naturalWidth, height: img.naturalHeight }
  }
  img.src = b64Url(base64)
}

function setSource(b64, name, keepLayerHistory = false) {
  if (batchActive.value) batch.exitBatch(false)
  sourceB64.value = b64
  effectCompRef.value?.onNewSource(b64, currentB64.value === b64)
  currentB64.value = b64
  sourceName.value = (name || 'image').replace(/\.[^.]+$/, '') || 'image'
  post.resetAdjustAll()
  if (!keepLayerHistory) {
    layerUndo.value = []
    layerRedo.value = []
    appliedEffects.value = []
  }
  error.value = ''
  successMsg.value = ''
  loadImageDimensions(b64)
  updateFilename()
}

function onPageKeydown(e) {
  if (!(e.ctrlKey || e.metaKey)) return
  const tag = e.target?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || e.target?.isContentEditable) return
  const isZ = e.code === 'KeyZ' || e.key.toLowerCase() === 'z'
  const isY = e.code === 'KeyY' || e.key.toLowerCase() === 'y'
  if (!isZ && !isY) return
  e.preventDefault()
  const redo = isY || e.shiftKey
  if (batchActive.value) {
    if (!redo) batch.undoBatch()
    return
  }
  if (maskMode.value) {
    if (redo) maskComp.value?.redoMask()
    else maskComp.value?.undoMask()
    return
  }
  if (redo) redoLayer()
  else undoLayer()
}

const { postQueue, clearPostQueue } = useRemixQueue()

const { sdProgress: genProgress, preview: genPreview } = useGenerationProgress()
const sectionBusy = reactive({ edit: false, remix: false, object: false, repair: false })
const genBusy = computed(() => sectionBusy.edit || sectionBusy.remix || sectionBusy.object || sectionBusy.repair || applyingEffect.value || lookBusy.value || recipeRunning.value || batchBusy.value)
const genActive = computed(() => !!genProgress.value && genProgress.value.progress > 0 && genProgress.value.progress < 1)
const livePreview = computed(() => (genBusy.value && genActive.value && genPreview.value) || '')
watch(genBusy, (busy) => {
  if (busy) {
    genProgress.value = null
    genPreview.value = ''
  }
})
async function consumePostQueue() {
  if (!postQueue.value.length) return
  const items = postQueue.value.slice()
  clearPostQueue()
  if (items.length > 1) {
    await batch.startBatch(items)
    return
  }
  try {
    const b64 = await readBatchImage(items[0].path)
    if (b64) setSource(b64, items[0].name)
  } catch (e) {
    error.value = String(e)
  }
}

let pageMounted = false

watch(() => postQueue.value.length, (n) => {
  if (n > 0 && pageMounted) consumePostQueue()
})

let offSessionSelected = () => {}

onMounted(async () => {
  openSection.value = props.initialSection || 'effect'
  document.addEventListener('keydown', onPageKeydown)
  offSessionSelected = EventsOn('session:selected', () => { input.loadLastImage() })
  await loadPresets()
  await effectCompRef.value?.load()
  await exportCompRef.value?.load()
  await recipesCompRef.value?.load()
  consumeInitialImage()
  await consumePostQueue()
  pageMounted = true
  if (!sourceB64.value) {
    await input.loadLastImage()
  }
})

onUnmounted(() => {
  batch.dispose()
  post.stopPreview()
  document.removeEventListener('keydown', onPageKeydown)
  offSessionSelected()
})
</script>

<template>
  <div class="page-enter post-page">
    <StatusMessage v-if="error" :text="error" @close="error = ''" />
    <StatusMessage v-if="successMsg" kind="success" :timeout="8000" :text="successMsg" @close="successMsg = ''" />

    <div class="post-workspace">
      <div class="post-preview-col">
        <div class="card post-preview-card">
          <div class="card-header">
            <span class="card-title">{{ t('post.source') }}</span>
            <div class="card-actions">
              <button class="btn btn-sm btn-secondary" @click="input.loadLastImage">{{ t('export.btn_load_last') }}</button>
              <button class="btn btn-sm btn-secondary" @click="input.uploadImage">{{ t('export.btn_upload') }}</button>
              <button class="btn btn-sm btn-secondary" @click="input.pasteFromClipboard">{{ t('fi.btn_paste') }}</button>
            </div>
          </div>
          <PostViewer
            :source="sourceB64"
            :after="previewB64 || currentB64"
            :previewing="previewing"
            :finish-active="finishActive"
            :mask-comp="maskComp"
            :gen-busy="genBusy"
            :gen-active="genActive"
            :gen-progress="genProgress"
            :live-preview="livePreview"
            :can-undo="canUndoView"
            :can-redo="canRedoView"
            @undo="undoView"
            @redo="redoView"
            @upload="input.uploadImage()"
            @drop="input.onDrop"
          />
          <div v-if="sourceB64" class="split-meta">
            <span v-if="imageInfo.width">{{ imageInfo.width }} x {{ imageInfo.height }}</span>
            <button v-if="batchActive ? batchState.canUndo && !batchBusy && !batchExporting : currentB64 !== sourceB64" class="btn btn-sm btn-secondary" @click="resetLayer">{{ t('post.btn_reset_layer') }}</button>
          </div>
          <BatchStrip v-if="batchActive" :batch="batch" />
        </div>
      </div>

      <div class="post-controls accordion">
        <div ref="editSectionRef" class="card" :class="{ collapsed: openSection !== 'edit' }">
          <div class="card-header card-toggle" @click="setOpenSection('edit')">
            <span class="card-heading"><span class="card-num mono">01</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_edit') }}</span><span class="card-subtitle">{{ t('post.sub_edit') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'edit' }">
            <div class="card-body">
              <EditSection :source="currentB64" :batch="batchActive" :look-busy="lookBusy || batchBusy" :look-result-prompt="lookResultPrompt" @result="onToolResult" @busy="v => sectionBusy.edit = v" @apply-look="onEditApplyLook" @apply-edit="onEditApplyBatch" @apply-recipe="onEditApplyRecipe" @save-recipe="onEditSaveRecipe" />
            </div>
          </div>
        </div>

        <div ref="remixSectionRef" class="card" :class="{ collapsed: openSection !== 'remix' }">
          <div class="card-header card-toggle" @click="setOpenSection('remix')">
            <span class="card-heading"><span class="card-num mono">02</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_remix') }}</span><span class="card-subtitle">{{ t('post.sub_remix') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'remix' }">
            <div class="card-body">
              <RemixSection :source="currentB64" :batch="batchActive" :batch-busy="batchBusy || batchExporting || recipeRunning || lookBusy || applyingEffect" @apply-batch="onRemixApplyBatch" :active="openSection === 'remix'" @result="onToolResult" @busy="v => sectionBusy.remix = v" @source="onRemixSource" />
            </div>
          </div>
        </div>

        <div ref="objectSectionRef" class="card" :class="{ collapsed: openSection !== 'object' }">
          <div class="card-header card-toggle" @click="setOpenSection('object')">
            <span class="card-heading"><span class="card-num mono">03</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_object') }}</span><span class="card-subtitle">{{ t('post.sub_object') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'object' }">
            <div class="card-body">
              <div v-if="batchActive" class="form-hint">{{ t('post.batch_unavailable') }}</div>
              <ObjectSection v-else ref="objectCompRef" :source="currentB64" :active="openSection === 'object'" @result="onToolResult" @busy="v => sectionBusy.object = v" />
            </div>
          </div>
        </div>

        <div ref="repairSectionRef" class="card" :class="{ collapsed: openSection !== 'repair' }">
          <div class="card-header card-toggle" @click="setOpenSection('repair')">
            <span class="card-heading"><span class="card-num mono">04</span><span class="card-heading-text"><span class="card-title">{{ t('post.section_repair') }}</span><span class="card-subtitle">{{ t('post.sub_repair') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'repair' }">
            <div class="card-body">
              <div v-if="batchActive" class="form-hint">{{ t('post.batch_unavailable') }}</div>
              <RepairSection v-else ref="repairCompRef" :source="currentB64" :active="openSection === 'repair'" @result="onToolResult" @busy="v => sectionBusy.repair = v" />
            </div>
          </div>
        </div>

        <div ref="effectSectionRef" class="card" :class="{ collapsed: openSection !== 'effect' }">
          <div class="card-header card-toggle" @click="setOpenSection('effect')">
            <span class="card-heading"><span class="card-num mono">05</span><span class="card-heading-text"><span class="card-title">{{ t('post.effect_title') }}</span><span class="card-subtitle">{{ t('post.sub_effect') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'effect' }">
            <div class="card-body">
              <EffectPanel
                ref="effectCompRef"
                :source="currentB64"
                :batch="batch"
                :queue-jobs="queueJobs"
                :status="status"
                :blocked="recipeRunning || lookBusy"
                :applied-count="appliedEffects.length"
                @busy="v => applyingEffect = v"
                @applied="onEffectApplied"
                @apply-recipe="applyRecipe"
                @clear-log="appliedEffects = []"
              />
            </div>
          </div>
        </div>

        <div ref="adjustSectionRef" class="card" :class="{ collapsed: openSection !== 'adjust' }">
          <div class="card-header card-toggle" @click="setOpenSection('adjust')">
            <span class="card-heading"><span class="card-num mono">06</span><span class="card-heading-text"><span class="card-title">{{ t('post.adjust_title') }}</span><span class="card-subtitle">{{ t('post.sub_adjust') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'adjust' }">
            <div class="card-body">
              <AdjustPanel :post="post" :source="currentB64" />
            </div>
          </div>
        </div>

        <div ref="finishSectionRef" class="card" :class="{ collapsed: openSection !== 'finish' }">
          <div class="card-header card-toggle" @click="setOpenSection('finish')">
            <span class="card-heading"><span class="card-num mono">07</span><span class="card-heading-text"><span class="card-title">{{ t('post.finish_title') }}</span><span class="card-subtitle">{{ t('post.sub_finish') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'finish' }">
            <div class="card-body">
              <FinishPanel :post="post" />
            </div>
          </div>
        </div>

        <div ref="exportSectionRef" class="card" :class="{ collapsed: openSection !== 'export' }">
          <div class="card-header card-toggle" @click="setOpenSection('export')">
            <span class="card-heading"><span class="card-num mono">08</span><span class="card-heading-text"><span class="card-title">{{ t('export.export_settings') }}</span><span class="card-subtitle">{{ t('post.sub_export') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'export' }">
            <div class="card-body">
              <ExportPanel
                ref="exportCompRef"
                :source="currentB64"
                :source-name="sourceName"
                :image-info="imageInfo"
                :batch="batch"
                :post="post"
                :status="status"
              />
            </div>
          </div>
        </div>

        <div ref="recipesSectionRef" class="card" :class="{ collapsed: openSection !== 'recipes' }">
          <div class="card-header card-toggle" @click="setOpenSection('recipes')">
            <span class="card-heading"><span class="card-num mono">09</span><span class="card-heading-text"><span class="card-title">{{ t('post.recipes_title') }}</span><span class="card-subtitle">{{ t('post.sub_recipes') }}</span></span></span>
          </div>
          <div class="card-collapse" :class="{ collapsed: openSection !== 'recipes' }">
            <div class="card-body">
              <RecipesPanel
                ref="recipesCompRef"
                :source="currentB64"
                :batch="batch"
                :post="post"
                :applied-effects="appliedEffects"
                :status="status"
                :blocked="applyingEffect || lookBusy"
                @busy="v => recipeRunning = v"
                @result="onEffectApplied"
                @open="setOpenSection('recipes')"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.post-page {
  padding: 12px;
  height: 100%;
  overflow-y: auto;
}

.post-workspace {
  display: flex;
  gap: 10px;
  height: calc(100vh - 124px - var(--titlebar-h, 32px) - var(--frame-pad, 6px));
  min-height: 480px;
}

.post-preview-col {
  flex: 1;
  min-width: 0;
  display: flex;
}

.post-preview-card {
  flex: 1;
  padding: 0;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

.post-controls {
  width: 360px;
  flex-shrink: 0;
  align-self: flex-start;
  max-height: 100%;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  min-height: 0;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}









.card.collapsed .card-header {
  border-bottom: none;
}









.card-collapse > .card-body {
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  overflow: clip;
  transition: padding 0.2s ease;
}


.card-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-bright);
}

.card-actions {
  display: flex;
  gap: 6px;
}

.card-actions .btn {
  white-space: nowrap;
}

.post-preview-card .card-header {
  flex-wrap: wrap;
  gap: 8px;
}

.card-body {
  padding: 12px;
}

.split-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 16px 12px;
  font-size: 12px;
  color: var(--text-dim);
}

</style>
