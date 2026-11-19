<script setup>
import { computed } from 'vue'
import { t } from '../../i18n/index.js'

const props = defineProps({
  post: { type: Object, required: true },
  source: { type: String, default: '' },
})

const { body, bodyState, bodyActive, bodyWidthMax, bodyHeightMax } = props.post

const note = computed(() => {
  if (!bodyActive.value || !props.source) return ''
  if (bodyState.status === 'loading') return t('post.body_loading')
  if (bodyState.status !== 'skip') return ''
  if (bodyState.skip === 'no_body') return t('post.body_none')
  if (bodyState.skip === 'not_local') return t('post.body_not_local')
  return t('post.body_unavailable')
})

const canRetry = computed(() => bodyActive.value && bodyState.status === 'skip' && bodyState.skip === 'unavailable')
</script>

<template>
  <div class="form-hint">{{ t('post.body_hint') }}</div>
  <div class="form-group" @dblclick="body.width = 0">
    <label class="form-label">{{ t('post.body_width', { value: body.width }) }}</label>
    <input type="range" class="form-range" :min="-bodyWidthMax" :max="bodyWidthMax" step="1" v-model.number="body.width" data-testid="body-width" />
  </div>
  <div class="form-group" @dblclick="body.height = 0">
    <label class="form-label">{{ t('post.body_height', { value: body.height }) }}</label>
    <input type="range" class="form-range" :min="-bodyHeightMax" :max="bodyHeightMax" step="1" v-model.number="body.height" data-testid="body-height" />
  </div>
  <div v-if="note" class="form-hint body-note" data-testid="body-note" :title="bodyState.error || null">{{ note }}</div>
  <div class="adj-actions">
    <button v-if="canRetry" class="btn btn-sm btn-secondary" data-testid="body-retry" @click="post.retryBody">{{ t('post.body_retry') }}</button>
    <button v-if="bodyActive" class="btn btn-sm btn-secondary" data-testid="body-reset" @click="post.resetBody">{{ t('post.body_reset') }}</button>
  </div>
</template>

<style scoped src="./post-shared.css"></style>

<style scoped>
.body-note {
  color: var(--warning, var(--text-dim));
}
</style>
