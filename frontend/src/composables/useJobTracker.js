import { ref, onMounted, onUnmounted } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'

function parseInfo(info) {
  if (!info || typeof info !== 'string') return info || null
  try {
    return JSON.parse(info)
  } catch (e) {
    console.warn('queue result info is not JSON:', e)
    return info
  }
}

export function parseQueueResult(raw) {
  if (!raw) return null
  let r = raw
  if (typeof raw === 'string') {
    try {
      r = JSON.parse(raw)
    } catch (e) {
      console.error('queue result parse error:', e)
      return null
    }
  }
  if (!r || typeof r !== 'object') return null
  const sessionItemId = r.session_item_id || null
  const imageBase64 = r.image_base64 || ''
  return {
    sessionItemId,
    imageBase64,
    hasImage: !!(sessionItemId || imageBase64),
    info: parseInfo(r.info),
    qualityUpscaleSkipped: !!r.quality_upscale_skipped,
    qualityRedenoiseSkipped: !!r.quality_redenoise_skipped,
    ideaPrompt: r.idea_prompt || '',
    ideaNegativePrompt: r.idea_negative_prompt || '',
  }
}

export function resultImageUrl(result) {
  if (!result) return ''
  if (result.sessionItemId) return api.sessionImageUrl(result.sessionItemId)
  return result.imageBase64 || ''
}

export async function resultImageBase64(result) {
  if (!result) return ''
  if (result.imageBase64) return result.imageBase64
  if (!result.sessionItemId) return ''
  return (await api.getSessionImage(result.sessionItemId)) || ''
}

export function isFinalPause(data) {
  return (data?.retry_count || 0) >= (data?.max_retries || 0)
}

export function useJobTracker(handlers = {}) {
  const pending = ref(new Set())
  const done = ref(0)
  const waiters = new Map()
  let offs = []

  function resolveWaiter(id, outcome) {
    const resolve = waiters.get(id)
    waiters.delete(id)
    if (resolve) resolve(outcome)
  }

  function track(id) {
    if (!id) return Promise.resolve({ status: 'failed', error: '' })
    pending.value.add(id)
    return new Promise(resolve => {
      const prev = waiters.get(id)
      waiters.set(id, prev ? (o) => { prev(o); resolve(o) } : resolve)
    })
  }

  function settle(id, outcome) {
    if (!id || !pending.value.has(id)) return false
    pending.value.delete(id)
    done.value++
    resolveWaiter(id, outcome)
    return true
  }

  function untrack(id) {
    if (!pending.value.has(id)) return
    pending.value.delete(id)
    resolveWaiter(id, { status: 'cancelled', error: '' })
  }

  function clear() {
    for (const id of [...pending.value]) untrack(id)
  }

  function reset() {
    clear()
    done.value = 0
  }

  function afterSettle(tracked) {
    if (tracked && pending.value.size === 0) handlers.onIdle?.()
  }

  function onCompleted(data) {
    if (!data) return
    const result = parseQueueResult(data.result)
    const tracked = settle(data.job_id, { status: 'completed', result, data, error: '' })
    handlers.onCompleted?.(data, result, tracked)
    afterSettle(tracked)
  }

  function onFailed(data, paused) {
    if (!data) return
    const tracked = settle(data.job_id, { status: 'failed', data, error: data.error || '' })
    handlers.onFailed?.(data, { tracked, paused })
    afterSettle(tracked)
  }

  function onPaused(data) {
    if (!data?.job_id || !isFinalPause(data)) return
    onFailed(data, true)
  }

  async function onChanged() {
    if (pending.value.size === 0) return
    const ids = [...pending.value]
    let jobs
    try {
      jobs = (await api.getQueue()) || []
    } catch (e) {
      console.error('job tracker queue refresh error:', e)
      return
    }
    const byId = new Map(jobs.map(j => [j.id, j]))
    let settledAny = false
    for (const id of ids) {
      const job = byId.get(id)
      if (job && job.status !== 'cancelled') continue
      if (settle(id, { status: 'cancelled', error: '' })) {
        settledAny = true
        handlers.onCancelled?.(id)
      }
    }
    afterSettle(settledAny)
  }

  onMounted(() => {
    offs = [
      EventsOn('queue:completed', onCompleted),
      EventsOn('queue:failed', (d) => onFailed(d, false)),
      EventsOn('queue:paused', onPaused),
      EventsOn('queue:changed', onChanged),
    ]
  })

  onUnmounted(() => {
    offs.forEach(off => off?.())
    offs = []
    clear()
  })

  return { pending, done, track, untrack, clear, reset }
}
