<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ChevronDown, ChevronUp, GripVertical, Square } from 'lucide-vue-next'
import GenProgress from './GenProgress.vue'
import { t } from '../i18n/index.js'

const props = defineProps({
  active: { type: Boolean, default: true },
  label: { type: String, default: '' },
  progress: { type: Number, default: 0 },
  steps: { type: Number, default: 0 },
  eta: { type: Number, default: 0 },
  previewAvailable: { type: Boolean, default: undefined },
})
const emit = defineEmits(['interrupt'])
const PANEL_EDGE_GAP = 12
const KEYBOARD_MOVE_STEP = 10
const panel = ref(null)
const collapsed = ref(false)
const position = ref(null)
const percent = computed(() => Math.round(Math.min(1, Math.max(0, props.progress)) * 100))
const panelStyle = computed(() => ({
  '--panel-edge-gap': `${PANEL_EDGE_GAP}px`,
  ...(position.value ? { left: `${position.value.x}px`, top: `${position.value.y}px`, right: 'auto', bottom: 'auto' } : {}),
}))
let drag = null
let observer

function bounds() {
  const element = panel.value
  const parent = element?.parentElement
  if (!parent || !parent.clientWidth || !parent.clientHeight) return null
  const rect = element.getBoundingClientRect()
  const parentRect = parent.getBoundingClientRect()
  return {
    x: rect.left - parentRect.left - parent.clientLeft,
    y: rect.top - parentRect.top - parent.clientTop,
    maxX: Math.max(0, parent.clientWidth - rect.width),
    maxY: Math.max(0, parent.clientHeight - rect.height),
  }
}
function moveTo(x, y) {
  const area = bounds()
  if (!area) return
  position.value = { x: Math.min(area.maxX, Math.max(0, x)), y: Math.min(area.maxY, Math.max(0, y)) }
}
function clampPosition() {
  if (position.value) moveTo(position.value.x, position.value.y)
}
function startDrag(event) {
  if (event.button !== 0 || event.target.closest('button')) return
  const area = bounds()
  if (!area) return
  drag = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, left: area.x, top: area.y }
  event.currentTarget.setPointerCapture?.(event.pointerId)
  event.preventDefault()
}
function moveDrag(event) {
  if (!drag || drag.pointerId !== event.pointerId) return
  moveTo(drag.left + event.clientX - drag.x, drag.top + event.clientY - drag.y)
}
function endDrag() { drag = null }
function moveWithKeyboard(event) {
  if (event.target !== event.currentTarget) return
  const directions = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }
  const direction = directions[event.key]
  const area = bounds()
  if (!direction || !area) return
  event.preventDefault()
  moveTo(area.x + direction[0] * KEYBOARD_MOVE_STEP, area.y + direction[1] * KEYBOARD_MOVE_STEP)
}
async function toggle() {
  collapsed.value = !collapsed.value
  await nextTick()
  clampPosition()
}
watch(() => props.active, async active => {
  if (!active) endDrag()
  await nextTick()
  clampPosition()
})
onMounted(() => {
  observer = new ResizeObserver(clampPosition)
  observer.observe(panel.value.parentElement)
  observer.observe(panel.value)
})
onUnmounted(() => { observer?.disconnect(); endDrag() })
</script>

<template>
  <section ref="panel" v-show="active" class="floating-generation-panel" :class="{ collapsed }" :style="panelStyle" data-testid="progress-panel" @pointerdown.stop @click.stop @dblclick.stop @wheel.stop>
    <div class="fgp-header" data-testid="progress-panel-handle" tabindex="0" :aria-label="t('progress.panel_move')" :title="t('progress.panel_move')" @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="endDrag" @pointercancel="endDrag" @lostpointercapture="endDrag" @keydown="moveWithKeyboard">
      <GripVertical :size="14" class="fgp-grip" aria-hidden="true" />
      <span class="fgp-label" :title="label">{{ label }}</span>
      <span v-if="collapsed" class="fgp-percent">{{ percent }}%</span>
      <button v-if="collapsed" type="button" class="fgp-button gp-interrupt" :title="t('progress.btn_interrupt')" :aria-label="t('progress.btn_interrupt')" @pointerdown.stop @click="emit('interrupt')"><Square :size="12" /></button>
      <button type="button" class="fgp-button" data-testid="progress-panel-toggle" :aria-expanded="!collapsed" :aria-label="t(collapsed ? 'progress.panel_expand' : 'progress.panel_collapse')" :title="t(collapsed ? 'progress.panel_expand' : 'progress.panel_collapse')" @pointerdown.stop @click="toggle"><ChevronUp v-if="collapsed" :size="14" /><ChevronDown v-else :size="14" /></button>
    </div>
    <div v-if="collapsed" class="fgp-mini-bar"><div :style="{ width: percent + '%' }"></div></div>
    <GenProgress v-else class="fgp-details" :progress="progress" :steps="steps" :eta="eta" :preview-available="previewAvailable" stats @interrupt="emit('interrupt')" />
  </section>
</template>

<style scoped>
.floating-generation-panel { position: absolute; z-index: 3; right: var(--panel-edge-gap); bottom: var(--panel-edge-gap); width: min(320px, calc(100% - 2 * var(--panel-edge-gap))); max-height: calc(100% - 2 * var(--panel-edge-gap)); overflow: auto; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); box-shadow: 0 4px 16px #0003; text-align: left; }
.fgp-header { display: flex; align-items: center; gap: 6px; padding: 6px 8px; cursor: grab; touch-action: none; user-select: none; }
.fgp-header:active { cursor: grabbing; }
.fgp-header:focus-visible, .fgp-button:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
.fgp-grip { flex-shrink: 0; color: var(--text-dim); }
.fgp-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-sm); color: var(--text); }
.fgp-percent { font-size: var(--fs-xs); color: var(--text-dim); }
.fgp-button { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; border: 0; border-radius: var(--radius-sm); padding: 4px; background: transparent; color: var(--text-dim); cursor: pointer; margin: 0; }
.fgp-button:hover { background: var(--surface-2); color: var(--text); }
.fgp-details { padding: 0 10px 10px; }
.fgp-mini-bar { height: 3px; background: var(--surface-2); }
.fgp-mini-bar > div { height: 100%; background: var(--accent); transition: width .3s; }
</style>
