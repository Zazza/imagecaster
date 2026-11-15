import { ref, watch } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'

export function useSavedDescriptions({ description, negative, error }) {
  const savedDescs = ref([])
  const showSavedDescs = ref(false)
  let currentDescId = null
  let lastUsedDescText = ''
  let lastUsedDescNegative = ''

  function bind(id, text, neg) {
    currentDescId = id
    lastUsedDescText = text
    lastUsedDescNegative = neg
  }

  function unbind() {
    bind(null, '', '')
  }

  watch(description, (val) => {
    if (currentDescId !== null && val !== lastUsedDescText) {
      if (negative.value === lastUsedDescNegative) negative.value = ''
      unbind()
    }
  })

  function getCurrentDescId() {
    return currentDescId
  }

  function restoreBinding(id) {
    const desc = id && savedDescs.value.find(d => d.id === id)
    if (!desc || desc.text !== description.value) return false
    bind(desc.id, desc.text, desc.negative_prompt || '')
    return true
  }

  async function loadSavedDescs() {
    try {
      savedDescs.value = await api.listDescriptions() || []
    } catch (e) {
      console.error(e)
    }
  }

  async function saveDescription() {
    if (!description.value.trim()) return
    try {
      const created = await api.createDescriptionFull({
        text: description.value.trim(),
        name: '',
        negative_prompt: negative.value || '',
        type: '',
      })
      if (created && created.id) bind(created.id, description.value.trim(), negative.value || '')
      await loadSavedDescs()
    } catch (e) {
      error.value = t('generate.error_save_desc', { error: String(e) })
    }
  }

  async function deleteDescription(id) {
    try {
      await api.deleteDescription(id)
      if (currentDescId === id) unbind()
      await loadSavedDescs()
    } catch (e) {
      error.value = t('generate.error_delete_desc', { error: String(e) })
    }
  }

  function useDescription(desc) {
    bind(desc.id, desc.text, desc.negative_prompt || '')
    description.value = desc.text
    if (desc.negative_prompt) negative.value = desc.negative_prompt
    showSavedDescs.value = false
  }

  async function handleCreateDesc(data) {
    try {
      await api.createDescriptionFull(data)
      await loadSavedDescs()
    } catch (e) {
      error.value = t('generate.error_create', { error: String(e) })
    }
  }

  async function handleUpdateDesc(data) {
    try {
      await api.updateDescription(data)
      await loadSavedDescs()
      if (currentDescId === data.id) {
        const negLoaded = negative.value === lastUsedDescNegative
        bind(currentDescId, data.text, data.negative_prompt || '')
        description.value = data.text
        if (negLoaded) negative.value = data.negative_prompt || ''
      }
    } catch (e) {
      error.value = t('generate.error_update', { error: String(e) })
    }
  }

  return {
    savedDescs,
    showSavedDescs,
    getCurrentDescId,
    restoreBinding,
    loadSavedDescs,
    saveDescription,
    deleteDescription,
    useDescription,
    handleCreateDesc,
    handleUpdateDesc,
  }
}
