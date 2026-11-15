import { onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { readImageFile } from '../imageFile.js'

export function useImageInput({ setSource, error }) {
  async function loadLastImage() {
    try {
      const item = await api.getActiveSessionItem()
      if (item) {
        const image = await api.getSessionImage(item.id)
        if (image) {
          setSource(image, item.file_name || 'generated_image')
          return
        }
      }
    } catch (e) {
      console.warn('PostProcessPage.loadLastImage:', e)
    }
  }

  async function uploadImage() {
    try {
      const result = await api.readImageFile()
      if (result) setSource(result, 'uploaded_image')
    } catch (e) {
      error.value = String(e)
    }
  }

  async function pasteFromClipboard() {
    try {
      const base64 = await api.readClipboardImage()
      if (base64) setSource(base64, 'pasted_image')
    } catch (e) {
      error.value = String(e)
    }
  }

  function handlePaste(e) {
    const items = e.clipboardData?.items
    if (!items) return
    for (const item of items) {
      if (item.type.startsWith('image/')) {
        e.preventDefault()
        const file = item.getAsFile()
        if (!file) continue
        readImageFile(file)
          .then(base64 => setSource(base64, 'pasted_image'))
          .catch(err => { error.value = err.message })
        return
      }
    }
  }

  function onDrop(e) {
    const file = e.dataTransfer?.files?.[0]
    if (!file) return
    readImageFile(file)
      .then(base64 => setSource(base64, file.name))
      .catch(err => { error.value = err.message })
  }

  onMounted(() => document.addEventListener('paste', handlePaste))
  onUnmounted(() => document.removeEventListener('paste', handlePaste))

  return { loadLastImage, uploadImage, pasteFromClipboard, onDrop }
}
