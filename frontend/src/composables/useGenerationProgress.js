import { ref, onMounted, onUnmounted } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { b64Url } from '../imageDataUrl.js'

export function useGenerationProgress({ acceptJob = () => true } = {}) {
  const llmStatus = ref('')
  const sdProgress = ref(null)
  const preview = ref('')
  let activeQueueJob = 0
  let owner = ''
  let frame = 0
  let stage = 0
  let remoteJob = ''

  function onLLMStatus(data) {
    llmStatus.value = data?.status || ''
  }

  function clearSD() {
    sdProgress.value = null
    preview.value = ''
    owner = ''
    frame = 0
    stage = 0
    remoteJob = ''
  }

  function onSDProgress(data) {
    if (!data) {
      clearSD()
      return
    }
    if (data.queue_job_id && (!acceptJob(data.queue_job_id) || (activeQueueJob && activeQueueJob !== data.queue_job_id))) return
    const nextOwner = data.queue_job_id ? `queue:${data.queue_job_id}` : `direct:${data.job || ''}`
    if (owner !== nextOwner) clearSD()
    if (data.stage && stage && data.stage < stage) return
    if (remoteJob === data.job && data.preview_frame && data.preview_frame < frame) return
    if (remoteJob !== data.job) frame = 0
    owner = nextOwner
    stage = data.stage || stage
    remoteJob = data.job || ''
    // Before sampling, false is also the server's initial, unchecked value.
    const sampling = data.steps > 0 && data.progress > 0 && data.progress < 1
    const previewAvailable = data.preview_disabled || (data.preview_available === false && !sampling)
      ? undefined : data.preview_available
    sdProgress.value = { ...data, preview_available: previewAvailable, etaRelative: data.eta_relative ?? data.etaRelative }
    // Keep the last image of this owner through decoding, passes and SD stages.
    if (data.preview && (!data.preview_frame || data.preview_frame > frame)) {
      preview.value = b64Url(data.preview)
      frame = data.preview_frame || 0
    }
  }

  async function interrupt() {
    try {
      const id = sdProgress.value?.queue_job_id
      if (id) await api.cancelQueueJob(id)
      else await api.interruptGeneration()
    } catch (e) {
      console.warn('useGenerationProgress.interrupt:', e)
    }
  }

  function reset() {
    llmStatus.value = ''
    clearSD()
  }

  const isGenerating = () => llmStatus.value === 'thinking' || (sdProgress.value && sdProgress.value.progress > 0)

  let subscriptions = []
  onMounted(() => {
    subscriptions = [
      EventsOn('llm:status', onLLMStatus),
      EventsOn('sd:progress', onSDProgress),
      EventsOn('sd:progress:start', clearSD),
      EventsOn('sd:progress:stop', clearSD),
      EventsOn('queue:started', data => {
        activeQueueJob = data?.job_id || 0
        clearSD()
      }),
    ]
  })

  onUnmounted(() => subscriptions.forEach(off => off?.()))

  return { llmStatus, sdProgress, preview, interrupt, reset, isGenerating }
}
