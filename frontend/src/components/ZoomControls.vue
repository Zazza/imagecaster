<script setup>
import { t } from '../i18n/index.js'
import { ZOOM_MIN, ZOOM_MAX, ZOOM_PRESETS } from '../composables/useZoomPan.js'

const props = defineProps({
  zoom: { type: Object, required: true },
  prefix: { type: String, default: 'split-zoom' },
  limitMax: { type: Boolean, default: true },
})

const { isFit, zoomPct, zoomLabel, zoomMenuOpen } = props.zoom
</script>

<template>
  <div :class="prefix">
    <slot />
    <button
      :class="prefix + '-btn'"
      :title="t('viewer.zoom_out')"
      :aria-label="t('viewer.zoom_out')"
      :disabled="isFit && zoom.fitPercent() <= ZOOM_MIN"
      @click="zoom.zoomBy(-10)"
    >&minus;</button>
    <button
      :class="[prefix + '-btn', prefix + '-label']"
      :title="t('viewer.zoom_presets')"
      :aria-expanded="zoomMenuOpen"
      aria-haspopup="menu"
      @click="zoom.onZoomIndicatorClick"
    >{{ zoomLabel }}</button>
    <button
      :class="prefix + '-btn'"
      :title="t('viewer.zoom_in')"
      :aria-label="t('viewer.zoom_in')"
      :disabled="limitMax && !isFit && zoomPct >= ZOOM_MAX"
      @click="zoom.zoomBy(10)"
    >&plus;</button>
    <div v-if="zoomMenuOpen" :class="prefix + '-menu'" role="menu">
      <button :class="{ active: isFit }" @click="zoom.setFit">{{ t('viewer.zoom_fit') }}</button>
      <button
        v-for="p in ZOOM_PRESETS"
        :key="p"
        :class="{ active: !isFit && zoomPct === p }"
        @click="zoom.enterZoom(p)"
      >{{ p }}%</button>
    </div>
  </div>
</template>
