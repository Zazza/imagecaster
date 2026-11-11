<script setup>
import { ref } from 'vue'
import { ChevronDown } from 'lucide-vue-next'

defineProps({
  title: { type: String, required: true },
  summary: { type: String, default: '' },
})
const emit = defineEmits(['toggle'])

const open = ref(false)
function toggle() {
  open.value = !open.value
  emit('toggle', open.value)
}
</script>

<template>
  <div class="fold" :class="{ 'fold-open': open }">
    <button type="button" class="fold-head" :aria-expanded="open ? 'true' : 'false'" @click="toggle">
      <ChevronDown :size="14" class="fold-chevron" :class="{ closed: !open }" />
      <span class="fold-title">{{ title }}</span>
      <span v-if="summary" class="fold-summary">{{ summary }}</span>
    </button>
    <div v-show="open" class="fold-body">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.fold {
  margin: 12px 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.fold-head {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 8px 12px;
  background: none;
  border: none;
  color: var(--text-dim);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  user-select: none;
}
.fold-head:hover,
.fold-open .fold-head {
  color: var(--text);
}
.fold-chevron {
  flex-shrink: 0;
  transition: transform 0.15s;
}
.fold-chevron.closed {
  transform: rotate(-90deg);
}
.fold-title {
  flex-shrink: 0;
}
.fold-summary {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--text-dim);
}
.fold-summary::before {
  content: '·';
  margin-right: 6px;
}
.fold-body {
  padding: 4px 12px 12px;
}
</style>
