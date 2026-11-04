<script setup>
import { reactive, ref, onMounted, onUnmounted, provide } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import GeneratePage from './GeneratePage.vue'

const props = defineProps({
  resetting: { type: Boolean, default: false },
})

const emit = defineEmits(['navigate'])

const shared = reactive({
  description: '',
  negative: '',
  selectedPresetId: null,
  selectedCompoundPresetId: null,
  genMode: 'preset',
})

provide('sharedGenState', shared)

const isDragOver = ref(false)

function onDragOver(e) {
  if (e.dataTransfer?.types?.includes('Files')) {
    e.preventDefault()
    isDragOver.value = true
  }
}

function onDragLeave(e) {
  if (!e.currentTarget?.contains(e.relatedTarget)) {
    isDragOver.value = false
  }
}

function onDrop(e) {
  e.preventDefault()
  isDragOver.value = false
  const files = e.dataTransfer?.files
  if (files?.length) {
    for (const file of files) {
      if (file.type.startsWith('image/')) {
        const reader = new FileReader()
        reader.onload = () => {
          const base64 = reader.result.split(',')[1]
          if (base64) {
            emit('navigate', { page: 'postprocess', section: 'remix', image: base64 })
          }
        }
        reader.readAsDataURL(file)
        return
      }
    }
  }
}

onMounted(async () => {
  try {
    const s = await api.getSettings()
    if (s.gen_preset_id) shared.selectedPresetId = Number(s.gen_preset_id)
    if (s.gen_compound_preset_id) shared.selectedCompoundPresetId = Number(s.gen_compound_preset_id)
    if (s.gen_mode) shared.genMode = s.gen_mode
    if (s.gen_description) shared.description = s.gen_description
    if (s.gen_negative) shared.negative = s.gen_negative
  } catch {}
})

onUnmounted(() => {
  if (props.resetting) return
  api.updateSettings({
    gen_preset_id: String(shared.selectedPresetId || ''),
    gen_compound_preset_id: String(shared.selectedCompoundPresetId || ''),
    gen_mode: shared.genMode,
    gen_description: shared.description || '',
    gen_negative: shared.negative || '',
  }).catch(() => {})
})
</script>

<template>
  <div class="page-enter" @dragover="onDragOver" @dragleave="onDragLeave" @drop="onDrop">
    <div v-if="isDragOver" class="drop-zone-overlay">
      {{ t('gen.drop_postprocess') }}
    </div>
    <div class="unified-tab-content">
      <GeneratePage :resetting="resetting" @navigate="emit('navigate', $event)" />
    </div>
  </div>
</template>
