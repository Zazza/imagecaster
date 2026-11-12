<script setup>
import { computed } from 'vue'

const props = defineProps({
  percent: { type: Number, default: null },
  text: { type: String, default: '' },
  label: { type: String, default: '' },
  wide: { type: Boolean, default: false },
})

const known = computed(() => typeof props.percent === 'number' && Number.isFinite(props.percent))
const shown = computed(() => (known.value ? Math.max(0, Math.min(100, props.percent)) : 0))
</script>

<template>
  <div class="dl-progress" :class="{ wide }" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="known ? Math.round(shown) : undefined">
    <div class="dl-progress-head">
      <span class="dl-progress-pct">{{ known ? Math.floor(shown) + '%' : '…' }}</span>
      <span class="dl-progress-text" :title="text">{{ text }}</span>
    </div>
    <div class="dl-progress-track">
      <div v-if="known" class="dl-progress-bar" :style="{ width: shown + '%' }"></div>
      <div v-else class="dl-progress-bar running"></div>
    </div>
    <div v-if="label" class="dl-progress-label" :title="label">{{ label }}</div>
  </div>
</template>

<style scoped>
.dl-progress { width: 240px; flex-shrink: 0; display: flex; flex-direction: column; gap: 3px; font-size: 11px; color: var(--text-dim); }
.dl-progress.wide { width: 100%; }
.dl-progress-head { display: flex; align-items: baseline; gap: 6px; min-width: 0; }
.dl-progress-pct { color: var(--text-bright); font-weight: 600; font-variant-numeric: tabular-nums; flex-shrink: 0; }
.dl-progress-text, .dl-progress-label { font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.dl-progress-track { position: relative; height: 6px; border-radius: 3px; background: var(--border); overflow: hidden; }
.dl-progress-bar { height: 100%; background: var(--accent); border-radius: 3px; transition: width 0.3s ease; }
.dl-progress-bar.running { position: absolute; width: 30%; animation: dl-progress-run 1.2s ease-in-out infinite; }
@keyframes dl-progress-run {
  from { left: -30%; }
  to { left: 100%; }
}
</style>
