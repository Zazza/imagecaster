<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { useKidsMode } from '../composables/useKidsMode.js'
import { describeErrorText, describeSource } from '../describeToGenerate.js'

const props = defineProps({
  imageBase64: { type: String, default: '' },
  loader: { type: Function, default: null },
  sourceKey: { type: [String, Number], default: '' },
})
const emit = defineEmits(['to-generate'])

const { kidsModeActive, loadKidsMode } = useKidsMode()
const mode = ref('describe')
const text = ref('')
const busy = ref(false)
const error = ref('')
const copied = ref(false)
let token = 0
let debounce = null

async function describe(fresh = false) {
  if (!props.imageBase64) return
  const my = ++token
  busy.value = true
  error.value = ''
  copied.value = false
  try {
    const image = props.loader ? await props.loader() : await describeSource(props.imageBase64)
    if (my !== token) return
    const result = await api.describePhoto(image, mode.value, fresh)
    if (my === token) text.value = result || ''
  } catch (e) {
    if (my === token) error.value = describeErrorText(e, t)
  } finally {
    if (my === token) busy.value = false
  }
}

function setMode(m) {
  if (mode.value === m) return
  mode.value = m
  text.value = ''
  describe()
}

async function copy() {
  try {
    await navigator.clipboard.writeText(text.value)
    copied.value = true
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

watch(() => props.sourceKey || props.imageBase64, () => {
  token++
  busy.value = false
  text.value = ''
  error.value = ''
  if (debounce) clearTimeout(debounce)
  debounce = setTimeout(() => { debounce = null; describe() }, 400)
})

onUnmounted(() => {
  token++
  if (debounce) clearTimeout(debounce)
})

onMounted(() => {
  loadKidsMode()
  describe()
})
</script>

<template>
  <div class="describe-panel" data-testid="describe-panel">
    <div class="describe-head">
      <span class="describe-title">{{ t('describe.title') }}</span>
      <div class="describe-modes">
        <button data-testid="describe-mode-describe" class="btn btn-sm btn-xs" :class="mode === 'describe' ? 'btn-primary' : 'btn-secondary'" @click="setMode('describe')">{{ t('describe.mode_text') }}</button>
        <button data-testid="describe-mode-quick" class="btn btn-sm btn-xs" :class="mode === 'quick' ? 'btn-primary' : 'btn-secondary'" @click="setMode('quick')">{{ t('describe.mode_tags') }}</button>
      </div>
    </div>
    <div v-if="busy" class="describe-busy" data-testid="describe-busy">{{ t('describe.busy') }}</div>
    <div v-if="error" class="describe-error" data-testid="describe-error">{{ error }}</div>
    <textarea data-testid="describe-text" class="form-textarea describe-text" v-model="text" rows="8" :readonly="kidsModeActive" :placeholder="t('describe.placeholder')"></textarea>
    <div class="describe-actions">
      <button data-testid="describe-again" class="btn btn-sm btn-secondary" :disabled="busy || !imageBase64" @click="describe(true)">{{ t('describe.again') }}</button>
      <button data-testid="describe-copy" class="btn btn-sm btn-secondary" :disabled="!text" @click="copy">{{ copied ? t('describe.copied') : t('describe.copy') }}</button>
      <button data-testid="describe-to-generate" class="btn btn-sm btn-primary" :disabled="!text || busy" @click="emit('to-generate', text)">{{ t('describe.to_generate') }}</button>
    </div>
  </div>
</template>

<style scoped>
.describe-panel { display: flex; flex-direction: column; gap: 8px; }
.describe-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.describe-title { font-weight: 600; }
.describe-modes { display: flex; gap: 6px; }
.describe-busy { font-size: var(--fs-sm, 12px); opacity: 0.8; }
.describe-error { font-size: var(--fs-sm, 12px); color: var(--color-danger, #e5484d); }
.describe-text { width: 100%; resize: vertical; }
.describe-actions { display: flex; flex-wrap: wrap; gap: 6px; }
</style>
