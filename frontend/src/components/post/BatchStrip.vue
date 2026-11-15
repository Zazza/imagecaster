<script setup>
import { X } from 'lucide-vue-next'
import { t } from '../../i18n/index.js'

const props = defineProps({
  batch: { type: Object, required: true },
})

const { batch: current, batchIndex, batchBusy, batchExporting, batchProgress, batchMsg } = props.batch
</script>

<template>
  <div class="batch-panel">
    <div class="batch-bar">
      <span class="batch-title">{{ t('post.batch_title', { n: current.items.length }) }}</span>
      <span class="batch-steps">{{ t('post.batch_steps', { n: current.steps }) }}</span>
      <template v-if="batchBusy || batchExporting">
        <span class="spinner spinner-sm"></span>
        <span>{{ t('post.batch_progress', { done: batchProgress.done, total: batchProgress.total }) }}</span>
        <button class="btn btn-sm btn-secondary" @click="batch.stopBatch">{{ t('progress.btn_interrupt') }}</button>
      </template>
      <span v-else-if="batchMsg" class="batch-msg">{{ batchMsg }}</span>
      <button class="btn btn-sm btn-secondary batch-exit" :disabled="batchBusy || batchExporting" @click="batch.exitBatch(true)">{{ t('post.batch_exit') }}</button>
    </div>
    <div class="batch-strip">
      <div
        v-for="(item, i) in current.items"
        :key="item.path"
        class="batch-thumb"
        :class="{ active: i === batchIndex, failed: item.failed }"
        role="button"
        tabindex="0"
        :title="item.failed ? item.name + ' — ' + t('post.batch_item_failed') : item.name"
        @click="batch.showBatchItem(i)"
        @keydown.enter.self="batch.showBatchItem(i)"
        @keydown.space.self.prevent="batch.showBatchItem(i)"
      >
        <img v-if="batch.batchThumbSrc(i)" :src="batch.batchThumbSrc(i)" :alt="item.name" />
        <span v-else class="batch-thumb-name">{{ item.name }}</span>
        <button
          class="batch-thumb-remove"
          :title="t('post.batch_remove')"
          :aria-label="t('post.batch_remove')"
          :disabled="batchBusy || batchExporting"
          @click.stop="batch.removeBatchItem(i)"
        ><X :size="10" /></button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.batch-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 16px 16px;
}

.batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: var(--fs-sm);
  color: var(--text-dim);
}

.batch-title {
  color: var(--text);
  font-weight: 600;
}

.batch-exit {
  margin-left: auto;
}

.batch-strip {
  display: flex;
  gap: 10px;
  overflow-x: auto;
  padding: 8px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.batch-thumb {
  position: relative;
  flex: 0 0 auto;
  width: 64px;
  height: 64px;
  padding: 0;
  border: 2px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  overflow: hidden;
  cursor: pointer;
}

.batch-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.batch-thumb-remove {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 16px;
  height: 16px;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s;
}

.batch-thumb:hover .batch-thumb-remove,
.batch-thumb:focus-within .batch-thumb-remove {
  opacity: 1;
}

.batch-thumb-remove:disabled {
  cursor: default;
  opacity: 0.3;
}

.batch-thumb.active {
  border-color: var(--accent);
}

.batch-thumb.failed {
  border-color: var(--danger);
}

.batch-thumb-name {
  font-size: 9px;
  color: var(--text-dim);
  word-break: break-all;
}
</style>
