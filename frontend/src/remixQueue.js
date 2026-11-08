import { ref } from 'vue'

const postQueue = ref([])

export function useRemixQueue() {
  function setPostQueue(items) {
    postQueue.value = items || []
  }

  function clearPostQueue() {
    postQueue.value = []
  }

  return { postQueue, setPostQueue, clearPostQueue }
}
