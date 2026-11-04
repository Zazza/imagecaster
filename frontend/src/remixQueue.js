import { ref } from 'vue'

const queue = ref([])
const postQueue = ref([])

export function useRemixQueue() {
  function setQueue(items) {
    queue.value = items || []
  }

  function clearQueue() {
    queue.value = []
  }

  function setPostQueue(items) {
    postQueue.value = items || []
  }

  function clearPostQueue() {
    postQueue.value = []
  }

  return { queue, setQueue, clearQueue, postQueue, setPostQueue, clearPostQueue }
}
