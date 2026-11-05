<script setup>
import { X } from 'lucide-vue-next'
import { t } from '../i18n/index.js'

defineProps({
  text: { type: String, default: '' },
  kind: { type: String, default: 'error' },
})
defineEmits(['close'])
</script>

<template>
  <div class="status status-dismissible" :class="'status-' + kind">
    <span class="status-text"><slot>{{ text }}</slot></span>
    <button type="button" class="status-close" :title="t('common.close')" :aria-label="t('common.close')" @click="$emit('close')">
      <X :size="14" />
    </button>
  </div>
</template>

<style scoped>
.status-dismissible {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.status-text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.status-close {
  flex-shrink: 0;
  background: none;
  border: none;
  padding: 0;
  color: inherit;
  cursor: pointer;
  opacity: 0.7;
  line-height: 0;
}
.status-close:hover {
  opacity: 1;
}
</style>
