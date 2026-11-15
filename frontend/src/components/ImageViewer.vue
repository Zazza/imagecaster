<script setup>
import { computed, ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { t } from '../i18n/index.js'
import { b64Url } from '../imageDataUrl.js'
import { parseLoras } from '../loraOverride.js'
import { ChevronLeft, ChevronRight, Info, X } from 'lucide-vue-next'
import { useZoomPan } from '../composables/useZoomPan.js'
import ZoomControls from './ZoomControls.vue'

const props = defineProps({
  imageBase64: { type: String, required: true },
  hasPrev: { type: Boolean, default: false },
  hasNext: { type: Boolean, default: false },
  item: { type: Object, default: null },
  showActions: { type: Boolean, default: false },
  confirmDelete: { type: Boolean, default: false },
  canDelete: { type: Boolean, default: true },
  position: { type: Number, default: 0 },
  total: { type: Number, default: 0 },
  title: { type: String, default: '' },
})
const emit = defineEmits(['close', 'prev', 'next', 'export', 'delete'])

const infoExpanded = ref(false)

const imgSrc = computed(() => b64Url(props.imageBase64))

const chain = computed(() => {
  const c = props.item?.meta?.chain
  return Array.isArray(c) ? c.filter(s => s && typeof s === 'object') : []
})
const chainLabel = computed(() => props.item?.meta?.label || '')
const metaModel = computed(() => props.item?.meta?.model || '')

const chainHeader = computed(() => {
  if (!chain.value.length) return ''
  return chainLabel.value
    ? t('viewer.chain_title') + ': ' + chainLabel.value
    : t('viewer.chain_custom', { count: chain.value.length })
})

const metaOnlyLine = computed(() => {
  if (chain.value.length) return ''
  return [chainLabel.value, metaModel.value].filter(Boolean).join(' · ')
})

function opLabel(op) {
  if (op === 'base') return t('viewer.chain_op_base')
  if (op === 'upscale') return t('viewer.chain_op_upscale')
  if (op === 'redenoise') return t('viewer.chain_op_redenoise')
  return op || ''
}

function formatDenoise(v) {
  if (v === null || v === undefined || v === '') return ''
  const n = Number(v)
  if (!Number.isFinite(n) || n < 0 || n > 1) return ''
  return n.toFixed(2)
}

function loraSummary(json) {
  const parts = parseLoras(json).map(r => `${r.name} (${r.weight})`)
  if (!parts.length) return null
  const full = parts.join(', ')
  if (full.length <= 80) return { text: full, full }
  const kept = []
  let len = 0
  for (const p of parts) {
    if (kept.length && len + p.length > 60) break
    kept.push(p)
    len += p.length + 2
  }
  return { text: kept.join(', ') + ', …', full }
}

const chainRows = computed(() => chain.value.map((s, i) => {
  const denoise = formatDenoise(s.denoise)
  const loras = loraSummary(s.loras)
  return {
    num: s.step || i + 1,
    name: s.preset || opLabel(s.op),
    model: s.model || t('viewer.chain_default_model'),
    denoise,
    loraText: loras ? loras.text : '',
    loraFull: loras ? loras.full : '',
    hasLoras: !!loras,
  }
}))

const stageRef = ref(null)
const showCounter = computed(() => props.total > 1 && props.position > 0)

function stageSize(el) {
  if (!el.clientWidth || !el.clientHeight) return null
  const cs = window.getComputedStyle(el)
  return {
    w: el.clientWidth - parseFloat(cs.paddingLeft || 0) - parseFloat(cs.paddingRight || 0),
    h: el.clientHeight - parseFloat(cs.paddingTop || 0) - parseFloat(cs.paddingBottom || 0),
  }
}

const zoom = useZoomPan(stageRef, { measure: stageSize, initialSize: { w: window.innerWidth, h: window.innerHeight } })
const { naturalW, isFit, panEnabled, panning, zoomMenuOpen, renderedW, panX, panY } = zoom

const imgStyle = computed(() => {
  if (isFit.value || !naturalW.value) return null
  return {
    width: Math.round(renderedW.value) + 'px',
    transform: `translate(${Math.round(panX.value)}px, ${Math.round(panY.value)}px)`,
  }
})

watch(() => props.imageBase64, () => zoom.resetImage())

watch(infoExpanded, () => nextTick(zoom.measure))

function onKeydown(e) {
  if (e.key === 'Escape') {
    if (zoomMenuOpen.value) zoomMenuOpen.value = false
    else emit('close')
    return
  }
  if (e.key === 'ArrowLeft' && props.hasPrev) emit('prev')
  if (e.key === 'ArrowRight' && props.hasNext) emit('next')
  if (e.key === 'Delete' && props.showActions && props.canDelete) emit('delete')
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', zoom.measure)
  zoom.measure()
})
onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', zoom.measure)
})
</script>

<template>
  <div class="image-viewer-overlay" role="dialog" aria-modal="true">
    <div class="iv-toolbar">
      <div class="iv-toolbar-side">
        <span v-if="showCounter" class="iv-counter">{{ t('viewer.counter', { n: position, total }) }}</span>
        <span v-if="title" class="iv-title" :title="title">{{ title }}</span>
      </div>
      <ZoomControls :zoom="zoom" prefix="image-viewer-zoom" :limit-max="false" />
      <div class="iv-toolbar-side iv-toolbar-right">
        <button
          v-if="item"
          class="image-viewer-tool-btn iv-info-toggle"
          :class="{ active: infoExpanded }"
          :aria-expanded="infoExpanded"
          @click="infoExpanded = !infoExpanded"
        ><Info :size="14" /> {{ t('viewer.info_toggle') }}</button>
        <slot name="actions" />
        <button v-if="showActions" class="image-viewer-tool-btn" @click="$emit('export')">{{ t('viewer.postprocess') }}</button>
        <template v-if="showActions && canDelete">
          <span class="iv-sep" aria-hidden="true"></span>
          <button
            class="image-viewer-tool-btn tool-danger iv-delete"
            :class="{ confirm: confirmDelete }"
            :data-alt="confirmDelete ? t('viewer.delete') : t('viewer.confirm_delete')"
            @click="$emit('delete')"
          ><span>{{ confirmDelete ? t('viewer.confirm_delete') : t('viewer.delete') }}</span></button>
        </template>
        <button class="image-viewer-close" :title="t('viewer.close')" :aria-label="t('viewer.close')" @click="$emit('close')"><X :size="18" /></button>
      </div>
    </div>
    <div class="iv-body">
      <div class="iv-main">
        <div ref="stageRef" class="image-viewer-stage" @wheel="zoom.onWheel" @pointerdown="zoomMenuOpen = false">
          <img
            :src="imgSrc"
            :alt="t('viewer.full_size')"
            :class="{ zoomed: !isFit && naturalW > 0, pannable: panEnabled, panning }"
            :style="imgStyle"
            @dblclick="zoom.toggleFit100"
            @dragstart.prevent
            @load="zoom.onImgLoad"
            @pointerdown="zoom.onPointerDown"
            @pointermove="zoom.onPointerMove"
            @pointerup="zoom.onPointerUp"
            @pointercancel="zoom.onPointerUp"
          />
        </div>
        <button v-if="hasPrev" class="image-viewer-nav image-viewer-prev" :aria-label="t('viewer.prev')" @click="$emit('prev')"><ChevronLeft :size="32" :stroke-width="1.5" /></button>
        <button v-if="hasNext" class="image-viewer-nav image-viewer-next" :aria-label="t('viewer.next')" @click="$emit('next')"><ChevronRight :size="32" :stroke-width="1.5" /></button>
      </div>
      <aside v-if="item && infoExpanded" class="iv-info">
        <div v-if="item.prompt" class="viewer-prompt">{{ item.prompt }}</div>
        <div class="viewer-params">
          <span v-if="item.sampler">{{ item.sampler }}</span>
          <span v-if="item.steps">{{ t('viewer.info_steps') }} {{ item.steps }}</span>
          <span v-if="item.cfg_scale">{{ t('viewer.info_cfg') }} {{ item.cfg_scale }}</span>
          <span v-if="item.seed != null">{{ t('viewer.info_seed') }} {{ item.seed }}</span>
          <span v-if="item.denoising">D: {{ item.denoising }}</span>
          <span v-if="item.width && item.height">{{ item.width }}&times;{{ item.height }}</span>
        </div>
        <div v-if="chainRows.length" class="viewer-chain">
          <div class="viewer-chain-title">{{ chainHeader }}</div>
          <div v-for="row in chainRows" :key="row.num" class="viewer-chain-row">
            <span class="viewer-chain-num">{{ t('viewer.chain_step', { num: row.num }) }}</span>
            <span v-if="row.name" class="viewer-chain-name">{{ row.name }}</span>
            <span v-if="row.model">{{ row.model }}</span>
            <span v-if="row.denoise">{{ t('viewer.chain_denoise', { value: row.denoise }) }}</span>
            <span v-if="row.hasLoras" :title="row.loraFull">{{ t('viewer.chain_loras') }}: {{ row.loraText }}</span>
          </div>
        </div>
        <div v-else-if="metaOnlyLine" class="viewer-params">
          <span>{{ metaOnlyLine }}</span>
        </div>
      </aside>
    </div>
  </div>
</template>
