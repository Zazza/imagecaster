<script setup>
import { computed } from 'vue'
import { t } from '../../i18n/index.js'

const props = defineProps({
  post: { type: Object, required: true },
  source: { type: String, default: '' },
})

const { retouch, face, retouchActive, retouchLocal, eyeColors, faceWidthMax } = props.post

const faceNote = computed(() => {
  if (!retouchLocal.value || !props.source) return ''
  if (face.status === 'loading') return t('post.retouch_face_loading')
  if (face.status !== 'skip') return ''
  if (face.skip === 'no_face') return t('post.retouch_face_none')
  if (face.skip === 'not_local') return t('post.retouch_face_not_local')
  return t('post.retouch_face_unavailable')
})

const canRetry = computed(() => retouchLocal.value && face.status === 'skip' && face.skip === 'unavailable')

const checks = ['under_eyes', 'teeth', 'eye_whites', 'light']
</script>

<template>
  <div class="form-hint">{{ t('post.retouch_hint') }}</div>
  <label v-for="key in checks" :key="key" class="form-label retouch-check">
    <input type="checkbox" v-model="retouch[key]" :data-testid="'retouch-' + key" /> {{ t('post.retouch_' + key) }}
  </label>
  <div class="form-group">
    <label class="form-label">{{ t('post.retouch_eye_color') }}</label>
    <select class="form-select" v-model="retouch.eye_color" data-testid="retouch-eye_color">
      <option value="">{{ t('post.retouch_eye_as_is') }}</option>
      <option v-for="c in eyeColors" :key="c" :value="c">{{ t('post.retouch_eye_' + c.replace(' ', '_')) }}</option>
    </select>
  </div>
  <div class="form-group" @dblclick="retouch.face_width = 0">
    <label class="form-label">{{ t('post.retouch_face_width', { value: retouch.face_width }) }}</label>
    <input type="range" class="form-range" :min="-faceWidthMax" :max="faceWidthMax" step="1" v-model.number="retouch.face_width" data-testid="retouch-face_width" />
  </div>
  <div v-if="faceNote" class="form-hint retouch-note" data-testid="retouch-face-note" :title="face.error || null">{{ faceNote }}</div>
  <div class="adj-actions">
    <button v-if="canRetry" class="btn btn-sm btn-secondary" data-testid="retouch-retry" @click="post.retryFace">{{ t('post.retouch_retry') }}</button>
    <button v-if="retouchActive" class="btn btn-sm btn-secondary" data-testid="retouch-reset" @click="post.resetRetouch">{{ t('post.retouch_reset') }}</button>
  </div>
</template>

<style scoped src="./post-shared.css"></style>

<style scoped>
.retouch-check {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  cursor: pointer;
}

.retouch-note {
  color: var(--warning, var(--text-dim));
}
</style>
