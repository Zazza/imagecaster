import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'

export const FOOTER_MIN_VH = 15
export const FOOTER_MAX_VH = 85

function pointerY(e) {
  return e.clientY || (e.touches && e.touches[0].clientY) || 0
}

export function useFooterResize(defaultHeight = 40) {
  const panelHeight = ref(defaultHeight)
  let dragging = false
  let dragStartY = 0
  let dragStartHeight = 0
  let saveTimer = null

  function save() {
    api.saveWindowLayout(panelHeight.value).catch(e => console.warn('saveWindowLayout:', e))
  }

  function scheduleSave() {
    clearTimeout(saveTimer)
    saveTimer = setTimeout(save, 500)
  }

  function onDragMove(e) {
    if (!dragging) return
    e.preventDefault()
    const delta = dragStartY - pointerY(e)
    const vh = window.innerHeight
    const newVh = Math.round((dragStartHeight * vh / 100 + delta) / vh * 100)
    panelHeight.value = Math.max(FOOTER_MIN_VH, Math.min(FOOTER_MAX_VH, newVh))
  }

  function onDragEnd() {
    dragging = false
    document.removeEventListener('mousemove', onDragMove)
    document.removeEventListener('mouseup', onDragEnd)
    document.removeEventListener('touchmove', onDragMove)
    document.removeEventListener('touchend', onDragEnd)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    scheduleSave()
  }

  function onDragStart(e) {
    dragging = true
    dragStartY = pointerY(e)
    dragStartHeight = panelHeight.value
    document.addEventListener('mousemove', onDragMove)
    document.addEventListener('mouseup', onDragEnd)
    document.addEventListener('touchmove', onDragMove, { passive: false })
    document.addEventListener('touchend', onDragEnd)
    document.body.style.cursor = 'ns-resize'
    document.body.style.userSelect = 'none'
  }

  onMounted(async () => {
    try {
      const h = await api.getFooterHeight()
      if (h > 0) panelHeight.value = h
    } catch (e) {
      console.warn('getFooterHeight:', e)
    }
    window.addEventListener('beforeunload', save)
  })

  onUnmounted(() => {
    window.removeEventListener('beforeunload', save)
    clearTimeout(saveTimer)
  })

  return { panelHeight, onDragStart }
}
