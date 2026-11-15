<script setup>
import { computed } from 'vue'
import { t } from '../i18n/index.js'

const props = defineProps({
  progress: { type: Number, default: 0 },
  steps: { type: Number, default: 0 },
  eta: { type: Number, default: 0 },
  stats: { type: Boolean, default: false },
  showStep: { type: Boolean, default: true },
  label: { type: String, default: '' },
  small: { type: Boolean, default: false },
  block: { type: Boolean, default: false },
})
defineEmits(['interrupt'])

const percent = computed(() => Math.round(props.progress * 100))
const step = computed(() => Math.round(props.progress * props.steps))
const etaSec = computed(() => Math.ceil(props.eta))
</script>

<template>
  <div class="gen-progress" :class="{ small, 'has-stats': stats }">
    <div v-if="label" class="gp-label">{{ label }}</div>
    <div v-if="stats" class="gp-stats">
      <span>{{ percent }}%</span>
      <span v-if="showStep">{{ t('progress.sd_step', { current: step, total: steps }) }}<span v-if="eta > 0"> — ~{{ etaSec }}s</span></span>
      <span v-else><span v-if="eta > 0">~{{ etaSec }}s</span></span>
    </div>
    <div class="gp-bar"><div :style="{ width: (progress * 100) + '%' }"></div></div>
    <button class="btn btn-sm btn-secondary gp-interrupt" :class="{ 'gp-block': block }" @click="$emit('interrupt')">{{ t('progress.btn_interrupt') }}</button>
  </div>
</template>

<style scoped>
.gp-label {
  font-size: var(--fs-sm);
  color: var(--text-dim);
}
.gp-stats {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  margin-bottom: 4px;
}
.gp-stats span {
  color: var(--text-dim);
  font-size: var(--fs-sm);
}
.small .gp-stats span {
  font-size: var(--fs-xs);
}
.gp-bar {
  background: var(--surface-2);
  border-radius: 4px;
  overflow: hidden;
  height: 6px;
}
.small .gp-bar {
  height: 4px;
}
.gen-progress:not(.has-stats) .gp-bar {
  margin-top: 6px;
}
.gp-bar div {
  background: var(--accent);
  height: 100%;
  transition: width 0.3s;
}
.gp-interrupt {
  margin-top: 8px;
  font-size: var(--fs-xs);
}
.gp-block {
  width: 100%;
}
</style>
