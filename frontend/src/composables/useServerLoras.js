import { ref } from 'vue'
import { api } from '../api.js'

let pending = null

export function fetchServerLoras() {
  if (!pending) {
    pending = Promise.resolve()
      .then(() => api.getLoRAs())
      .then(list => list || [])
      .finally(() => { pending = null })
  }
  return pending
}

export function useServerLoras() {
  const loras = ref([])
  const loaded = ref(false)
  const error = ref(false)

  async function load() {
    if (loaded.value) return loras.value
    error.value = false
    try {
      loras.value = await fetchServerLoras()
      loaded.value = true
    } catch (e) {
      console.error('server loras load error:', e)
      error.value = true
    }
    return loras.value
  }

  return { loras, loaded, error, load }
}
