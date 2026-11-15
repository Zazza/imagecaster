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
    <h3 class="text-bright mb-4">{{ t('settings.section_kids') }}</h3>
    <div class="kids-toggle">
      <ToggleSwitch :modelValue="kidsMode" @update:modelValue="onKidsToggle" />
      <div>
        <div class="kids-state">{{ kidsMode ? t('settings.enabled') : t('settings.disabled') }}</div>
        <div class="kids-desc">
          {{ t('settings.kids_description') }}
        </div>
      </div>
    </div>

    <div v-if="kidsMode && kidsCategories.length > 0" class="inset-box">
      <div class="inset-box-title kids-cat-title">{{ t('settings.filter_categories') }}</div>
      <div class="kids-cats">
        <div v-for="cat in kidsCategories" :key="cat.name" class="kids-cat flex-between">
          <div>
            <span class="text-bright fs-md">{{ cat.label }}</span>
            <span v-if="cat.alwaysOn" class="kids-cat-note">{{ t('settings.always_on') }}</span>
          </div>
          <ToggleSwitch :modelValue="cat.enabled" @update:modelValue="onKidsCatToggle(cat)" :disabled="cat.alwaysOn" />
        </div>
      </div>
      <div class="text-dim fs-xs mt-2">{{ t('settings.pin_required_categories') }}</div>
    </div>

    <div class="kids-layers">
      {{ t('settings.kids_layers') }}
      <ul class="kids-layers-list">
        <li>{{ t('settings.kids_filter_input') }}</li>
        <li>{{ t('settings.kids_filter_llm') }}</li>
        <li>{{ t('settings.kids_filter_output') }}</li>
        <li>{{ t('settings.kids_forces_negative') }}</li>
      </ul>
      <div class="mt-2">{{ t('settings.kids_pin_protected') }}</div>
    </div>
  </div>

  <PinModal v-if="showPinModal" :mode="pinMode" :error="pinError" @confirm="onPinConfirm" @cancel="onPinCancel" />
  <PinModal v-if="kidsCatPinModal" mode="verify" :error="kidsCatPinError" @confirm="onKidsCatPinConfirm" @cancel="onKidsCatPinCancel" />
</template>

<style scoped>
.kids-toggle {
  display: flex;
  align-items: center;
  gap: var(--sp-4);
  margin-bottom: var(--sp-4);
}

.kids-state {
  color: var(--text-bright);
  font-weight: 500;
}

.kids-desc {
  margin-top: 2px;
  color: var(--text-dim);
  font-size: var(--fs-sm);
}

.kids-cat-title {
  margin-bottom: 10px;
}

.kids-cats {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}

.kids-cat {
  padding: var(--sp-1) 0;
}

.kids-cat-note {
  margin-left: 6px;
  color: var(--text-dim);
  font-size: var(--fs-xs);
}

.kids-layers {
  color: var(--text-dim);
  font-size: var(--fs-md);
  line-height: 1.6;
}

.kids-layers-list {
  margin: var(--sp-2) 0 0 var(--sp-4);
  padding: 0;
}
</style>
