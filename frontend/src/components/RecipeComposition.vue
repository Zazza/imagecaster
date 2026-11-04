<script setup>
import { computed } from 'vue'
import { t } from '../i18n/index.js'

const props = defineProps({
  vision: { type: String, default: '' },
  steps: { type: Array, default: () => [] },
  adjust: { type: Object, default: () => ({}) },
})

const adjustDefaults = {
  brightness: 0,
  contrast: 0,
  black_point: 0,
  white_point: 255,
  gamma: 1,
  saturation: 0,
  temperature: 0,
  balance_r: 0,
  balance_g: 0,
  balance_b: 0,
  sharpen: 0,
  vignette: 0,
}

function isNeutral(key, v) {
  const d = adjustDefaults[key]
  if (key === 'white_point' || key === 'gamma') return v === d || v === 0
  return v === d
}

const activeAdjust = computed(() =>
  Object.keys(adjustDefaults)
    .filter(k => props.adjust[k] !== undefined && !isNeutral(k, props.adjust[k]))
    .map(k => ({ key: k, value: props.adjust[k] }))
)

function fmtAdjust(key, v) {
  return key === 'gamma' ? Number(v).toFixed(2) : String(v)
}

function fmtStrength(v) {
  return Number(v).toFixed(2)
}

function isPromptStep(s) {
  return !!s.prompt && !s.preset_id
}

function stepLabel(s) {
  if (!isPromptStep(s)) return s.name
  const p = s.prompt || ''
  return t('post.recipe_prompt_step') + ': ' + (p.length > 40 ? p.slice(0, 40) + '…' : p)
}

function stepTitle(s) {
  return isPromptStep(s) ? (s.prompt || '') : (s.name || '')
}
</script>

<template>
  <div class="recipe-composition" data-testid="recipe-composition">
    <div class="comp-title">{{ t('post.composition') }}</div>
    <div v-if="vision" class="comp-vision">
      <span class="comp-vision-label">{{ t('post.autostyle_vision') }}</span>
      <span class="comp-vision-text">{{ vision }}</span>
    </div>
    <div v-if="steps.length" class="comp-chips">
      <span v-for="(s, i) in steps" :key="i" class="comp-chip" :title="stepTitle(s)">{{ stepLabel(s) }} {{ fmtStrength(s.strength) }}</span>
    </div>
    <div v-if="activeAdjust.length" class="comp-chips">
      <span v-for="a in activeAdjust" :key="a.key" class="comp-chip comp-chip-adjust">{{ t('post.' + a.key) }} {{ fmtAdjust(a.key, a.value) }}</span>
    </div>
    <div class="comp-actions"><slot></slot></div>
  </div>
</template>

<style scoped>
.recipe-composition {
  margin-bottom: 12px;
  padding: 10px;
  background: var(--surface-2);
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
}

.comp-title {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-dim);
  margin-bottom: 6px;
}

.comp-vision {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
  font-size: 11px;
  line-height: 1.35;
}

.comp-vision-label {
  flex-shrink: 0;
  color: var(--accent);
}

.comp-vision-text {
  color: var(--text-dim);
  word-break: break-word;
}

.comp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 4px;
}

.comp-chip {
  padding: 2px 8px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 11px;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}

.comp-chip-adjust {
  color: var(--text-dim);
}

.comp-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
  align-items: center;
}
</style>
