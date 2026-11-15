<script setup>
import { ref, inject } from 'vue'
import ToggleSwitch from '../ToggleSwitch.vue'
import PinModal from '../PinModal.vue'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'

const { kidsMode, kidsCategories, loadKidsCategories } = inject('settingsState')

const showPinModal = ref(false)
const pinMode = ref('set')
const pinError = ref('')
const kidsCatPinModal = ref(false)
const kidsCatPinError = ref('')
const kidsCatPending = ref(null)

function onKidsToggle(val) {
  pinError.value = ''
  pinMode.value = val ? 'set' : 'verify'
  showPinModal.value = true
}

async function onPinConfirm(pin) {
  pinError.value = ''
  const enabled = pinMode.value === 'set'
  try {
    await api.setKidsMode(enabled, pin)
    kidsMode.value = enabled
    showPinModal.value = false
    if (enabled) loadKidsCategories()
  } catch (e) {
    pinError.value = String(e) || 'Error'
  }
}

function onPinCancel() {
  showPinModal.value = false
  pinError.value = ''
}

function onKidsCatToggle(cat) {
  if (cat.alwaysOn || !kidsMode.value) return
  kidsCatPending.value = { name: cat.name, enabled: !cat.enabled }
  kidsCatPinError.value = ''
  kidsCatPinModal.value = true
}

async function onKidsCatPinConfirm(pin) {
  if (!kidsCatPending.value) return
  kidsCatPinError.value = ''
  try {
    await api.setKidsCategory(kidsCatPending.value.name, kidsCatPending.value.enabled, pin)
    const cat = kidsCategories.value.find(c => c.name === kidsCatPending.value.name)
    if (cat) cat.enabled = kidsCatPending.value.enabled
    kidsCatPinModal.value = false
    kidsCatPending.value = null
  } catch (e) {
    kidsCatPinError.value = String(e) || 'Error'
  }
}

function onKidsCatPinCancel() {
  kidsCatPinModal.value = false
  kidsCatPending.value = null
  kidsCatPinError.value = ''
}
</script>

<template>
  <div class="card">
    <h3 style="color: var(--text-bright); margin-bottom: 16px;">{{ t('settings.section_kids') }}</h3>
    <div style="display: flex; align-items: center; gap: 16px; margin-bottom: 16px;">
      <ToggleSwitch :modelValue="kidsMode" @update:modelValue="onKidsToggle" />
      <div>
        <div style="color: var(--text-bright); font-weight: 500;">{{ kidsMode ? t('settings.enabled') : t('settings.disabled') }}</div>
        <div style="color: var(--text-dim); font-size: 12px; margin-top: 2px;">
          {{ t('settings.kids_description') }}
        </div>
      </div>
    </div>

    <div v-if="kidsMode && kidsCategories.length > 0" style="margin-bottom: 16px; padding: 12px; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-sm);">
      <div style="color: var(--text-bright); font-weight: 500; margin-bottom: 10px;">{{ t('settings.filter_categories') }}</div>
      <div style="display: flex; flex-direction: column; gap: 8px;">
        <div v-for="cat in kidsCategories" :key="cat.name" style="display: flex; align-items: center; justify-content: space-between; padding: 4px 0;">
          <div>
            <span style="color: var(--text-bright); font-size: 13px;">{{ cat.label }}</span>
            <span v-if="cat.alwaysOn" style="color: var(--text-dim); font-size: 11px; margin-left: 6px;">{{ t('settings.always_on') }}</span>
          </div>
          <ToggleSwitch :modelValue="cat.enabled" @update:modelValue="onKidsCatToggle(cat)" :disabled="cat.alwaysOn" />
        </div>
      </div>
      <div style="color: var(--text-dim); font-size: 11px; margin-top: 8px;">{{ t('settings.pin_required_categories') }}</div>
    </div>

    <div style="color: var(--text-dim); font-size: 13px; line-height: 1.6;">
      {{ t('settings.kids_layers') }}
      <ul style="margin: 8px 0 0 16px; padding: 0;">
        <li>{{ t('settings.kids_filter_input') }}</li>
        <li>{{ t('settings.kids_filter_llm') }}</li>
        <li>{{ t('settings.kids_filter_output') }}</li>
        <li>{{ t('settings.kids_forces_negative') }}</li>
      </ul>
      <div style="margin-top: 8px;">{{ t('settings.kids_pin_protected') }}</div>
    </div>
  </div>

  <PinModal v-if="showPinModal" :mode="pinMode" :error="pinError" @confirm="onPinConfirm" @cancel="onPinCancel" />
  <PinModal v-if="kidsCatPinModal" mode="verify" :error="kidsCatPinError" @confirm="onKidsCatPinConfirm" @cancel="onKidsCatPinCancel" />
</template>
