<script setup>
import { RotateCcw } from 'lucide-vue-next'
import { t } from '../../i18n/index.js'

const props = defineProps({
  post: { type: Object, required: true },
  source: { type: String, default: '' },
})

const { adjust, sliderMeta, adjustGroups, autoAdjusting } = props.post
</script>

<template>
  <div class="adj-actions">
    <button class="btn btn-sm btn-secondary" :disabled="!source || autoAdjusting" @click="post.applyAutoAdjust('levels')">{{ t('post.auto_levels') }}</button>
    <button class="btn btn-sm btn-secondary" :disabled="!source || autoAdjusting" @click="post.applyAutoAdjust('wb')">{{ t('post.auto_wb') }}</button>
    <button class="btn btn-sm btn-secondary" @click="post.resetAdjustAll">{{ t('post.btn_reset_all') }}</button>
  </div>
  <div class="adj-groups">
    <div v-for="g in adjustGroups" :key="g.labelKey" class="adj-group">
      <div class="adj-group-title">{{ t(g.labelKey) }}</div>
      <div
        v-for="key in g.keys"
        :key="key"
        class="adj-row"
        :title="t('post.dblclick_reset')"
        @dblclick="post.resetAdjustKey(key)"
      >
        <div class="adj-row-head">
          <span class="adj-label">{{ t('post.' + key) }}</span>
          <span class="adj-value-wrap">
            <button
              v-if="adjust[key] !== sliderMeta[key].def"
              type="button"
              class="adj-reset"
              :data-testid="'adj-reset-' + key"
              :title="t('post.adjust_reset_one')"
              :aria-label="t('post.adjust_reset_one') + ': ' + t('post.' + key)"
              @click="post.resetAdjustKey(key)"
            ><RotateCcw :size="11" /></button>
            <span class="adj-value">{{ post.formatAdjust(key) }}</span>
          </span>
        </div>
        <input
          type="range"
          class="form-range"
          :min="sliderMeta[key].min"
          :max="sliderMeta[key].max"
          :step="sliderMeta[key].step"
          v-model.number="adjust[key]"
        />
      </div>
    </div>
  </div>
</template>

<style scoped src="./post-shared.css"></style>

<style scoped>
.adj-groups {
  display: grid;
  grid-template-columns: 1fr;
  gap: 12px;
}

.adj-group {
  min-width: 0;
}

.adj-group-title {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-dim);
  margin-bottom: 8px;
}

.adj-row {
  margin-bottom: 10px;
  cursor: default;
}

.adj-row-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 2px;
}

.adj-label {
  font-size: 12px;
  color: var(--text);
}

.adj-value-wrap {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.adj-reset {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--text-dim);
  cursor: pointer;
}

.adj-reset:hover {
  color: var(--accent);
  background: var(--surface-2);
}

.adj-value {
  font-size: 11px;
  color: var(--text-dim);
  font-variant-numeric: tabular-nums;
}
</style>
