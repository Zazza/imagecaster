import { ref, reactive, computed } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { b64Url } from '../imageDataUrl.js'
import { resultImageBase64 } from './useJobTracker.js'
import { createBatch } from '../postBatch.js'

const SESSION_PATH = 'session:'

export function readBatchImage(path) {
  if (path.startsWith(SESSION_PATH)) return api.getSessionImage(Number(path.slice(SESSION_PATH.length)))
  return api.readFileAsBase64(path)
}

function readBatchThumb(path) {
  if (path.startsWith(SESSION_PATH)) return api.getSessionThumb(Number(path.slice(SESSION_PATH.length)))
  return api.readThumbnail(path)
}

export async function jobImage(outcome) {
  if (outcome.status !== 'completed') return ''
  try {
    return await resultImageBase64(outcome.result)
  } catch (e) {
    console.error('job image load error:', e)
    return ''
  }
}

export function usePostBatch({ queueJobs, error, successMsg, onStart, onShow, onEmpty, onExit }) {
  const batch = ref(null)
  const batchActive = computed(() => !!batch.value)
  const batchIndex = ref(0)
  const batchBusy = ref(false)
  const batchExporting = ref(false)
  const batchProgress = ref({ done: 0, total: 0 })
  const batchThumbs = reactive({})
  const batchMsg = ref('')
  const chainJobIds = new Set()
  let batchCancelled = false
  let batchViewReq = 0

  function isCancelled() {
    return batchCancelled
  }

  async function waitForChainJob(jobId) {
    chainJobIds.add(jobId)
    const outcome = await queueJobs.track(jobId)
    chainJobIds.delete(jobId)
    return outcome
  }

  async function dropChainJobs(cancel) {
    for (const id of [...chainJobIds]) {
      chainJobIds.delete(id)
      queueJobs.untrack(id)
      if (!cancel) continue
      try {
        await api.cancelQueueJob(id)
      } catch (e) {
        console.error('cancel queue job error:', e)
      }
    }
  }

  async function trackBatchJob(enqueued) {
    const jobId = await enqueued
    if (!jobId) return null
    if (batchCancelled) {
      try {
        await api.cancelQueueJob(jobId)
      } catch (e) {
        console.error('cancel queue job error:', e)
      }
      return null
    }
    return { wait: waitForChainJob(jobId).then(jobImage) }
  }

  async function startBatch(items) {
    if (batch.value) exitBatch(false)
    try { await api.batchReset() } catch (e) { console.warn('PostProcessPage.startBatch:', e) }
    batch.value = createBatch(items, {
      readImage: readBatchImage,
      saveImage: (b64) => api.batchSaveImage(b64),
    })
    batchMsg.value = ''
    onStart()
    loadBatchThumbs()
    await showBatchItem(0)
  }

  async function showBatchItem(i) {
    const b = batch.value
    if (!b || !b.items[i]) return
    batchIndex.value = i
    const req = ++batchViewReq
    const item = b.items[i]
    try {
      const orig = await readBatchImage(item.path)
      const curPath = b.currentPath(i)
      const cur = curPath === item.path ? orig : await readBatchImage(curPath)
      if (req !== batchViewReq || batch.value !== b) return
      onShow(orig, cur, (item.name || 'image').replace(/\.[^.]+$/, '') || 'image')
    } catch (e) {
      if (req === batchViewReq) error.value = String(e)
    }
  }

  function loadBatchThumbs() {
    const b = batch.value
    if (!b) return
    b.items.forEach((_, i) => {
      const path = b.currentPath(i)
      if (batchThumbs[path]) return
      readBatchThumb(path).then(t => {
        if (t) batchThumbs[path] = t
      }).catch(e => console.warn('PostProcessPage.loadBatchThumbs:', e))
    })
  }

  function batchThumbSrc(i) {
    const b = batch.value
    return b ? b64Url(batchThumbs[b.currentPath(i)] || '') : ''
  }

  async function runBatchStep(op, onDone) {
    const b = batch.value
    if (!b || batchBusy.value || batchExporting.value) return
    batchBusy.value = true
    batchCancelled = false
    batchMsg.value = ''
    error.value = ''
    successMsg.value = ''
    batchProgress.value = { done: 0, total: b.items.length }
    try {
      const res = await b.applyStep(op, {
        isCancelled: () => batchCancelled,
        onProgress: (done, total) => {
          batchProgress.value = { done, total }
          loadBatchThumbs()
        },
      })
      if (res.failed > 0 && !res.cancelled) error.value = t('post.batch_step_failed', { n: res.failed })
      if (res.cancelled) batchMsg.value = t('post.pipeline_interrupted')
      if (res.changed > 0 && onDone) onDone(res)
    } finally {
      batchBusy.value = false
      if (batch.value === b) {
        loadBatchThumbs()
        await showBatchItem(batchIndex.value)
      }
    }
  }

  function stopBatch() {
    if (batchBusy.value) interruptBatch()
    else batchCancelled = true
  }

  async function interruptBatch() {
    batchCancelled = true
    await dropChainJobs(true)
  }

  async function removeBatchItem(i) {
    const b = batch.value
    if (!b || batchBusy.value || batchExporting.value) return
    if (!b.removeItem(i)) return
    if (b.items.length === 0) {
      exitBatch(false)
      onEmpty()
      return
    }
    if (i < batchIndex.value) {
      batchIndex.value--
      return
    }
    if (i === batchIndex.value) await showBatchItem(Math.min(i, b.items.length - 1))
  }

  async function undoBatch() {
    const b = batch.value
    if (!b || batchBusy.value || batchExporting.value) return
    if (b.undo()) {
      batchMsg.value = ''
      await showBatchItem(batchIndex.value)
    }
  }

  async function resetBatch() {
    const b = batch.value
    if (!b || batchBusy.value || batchExporting.value) return
    while (b.undo()) {}
    await showBatchItem(batchIndex.value)
  }

  function exitBatch(keepCurrent) {
    if (!batch.value) return
    interruptBatch()
    batch.value = null
    batchMsg.value = ''
    for (const k of Object.keys(batchThumbs)) delete batchThumbs[k]
    api.batchReset().catch(e => console.warn('PostProcessPage.exitBatch:', e))
    if (keepCurrent) onExit()
  }

  async function exportBatch(resolveFolder, exporter) {
    const b = batch.value
    if (!b || batchBusy.value || batchExporting.value) return
    const folder = await resolveFolder()
    if (!folder) {
      error.value = t('post.pipeline_needs_folder')
      return
    }
    batchExporting.value = true
    batchCancelled = false
    error.value = ''
    successMsg.value = ''
    batchProgress.value = { done: 0, total: b.items.length }
    const exportOne = exporter(folder)
    try {
      const res = await b.exportAll(exportOne, { isCancelled: () => batchCancelled, onProgress: (done, total) => { batchProgress.value = { done, total } } })
      successMsg.value = t('post.batch_saved', { k: res.saved, m: res.skipped, path: folder })
      if (res.cancelled) successMsg.value += ' — ' + t('post.pipeline_interrupted')
    } finally {
      batchExporting.value = false
    }
  }

  function dispose() {
    if (batch.value) {
      interruptBatch()
      batch.value = null
      api.batchReset().catch(e => console.warn('PostProcessPage.onUnmounted:', e))
    }
    batchCancelled = true
    dropChainJobs(false)
  }

  return {
    batch, batchActive, batchIndex, batchBusy, batchExporting, batchProgress, batchMsg,
    isCancelled, waitForChainJob, dropChainJobs, trackBatchJob,
    startBatch, showBatchItem, batchThumbSrc, runBatchStep, stopBatch, interruptBatch,
    removeBatchItem, undoBatch, resetBatch, exitBatch, exportBatch, dispose,
  }
}
