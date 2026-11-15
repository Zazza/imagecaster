import { ref, onMounted, onUnmounted } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { b64Url } from '../imageDataUrl.js'

export function useGenerationProgress() {
  const llmStatus = ref('')
  const sdProgress = ref(null)
  const preview = ref('')

  function onLLMStatus(data) {
    llmStatus.value = data?.status || ''
  }

  function onSDProgress(data) {
    sdProgress.value = data ? { ...data, etaRelative: data.eta_relative ?? data.etaRelative } : data
    if (data?.preview) {
      preview.value = b64Url(data.preview)
    }
  }

  async function interrupt() {
    try {
      await api.interruptGeneration()
    } catch (e) {
      console.warn('useGenerationProgress.interrupt:', e)
    }
  }

  function reset() {
    llmStatus.value = ''
    sdProgress.value = null
    preview.value = ''
  }

  const isGenerating = () => llmStatus.value === 'thinking' || (sdProgress.value && sdProgress.value.progress > 0)

  let offLLM, offSD
  onMounted(() => {
    offLLM = EventsOn('llm:status', onLLMStatus)
    offSD = EventsOn('sd:progress', onSDProgress)
  })

  onUnmounted(() => {
    offLLM?.()
    offSD?.()
  })

  return { llmStatus, sdProgress, preview, interrupt, reset, isGenerating }
}
