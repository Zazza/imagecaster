<script setup>
import { ref, computed } from 'vue'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'

const MIN_TRAIN = 10
const MIN_FACE = 5

const props = defineProps({
  set: { type: Object, required: true },
  loras: { type: Array, default: () => [] },
  canFace: { type: Boolean, default: false },
  available: { type: Boolean, default: false },
})
const emit = defineEmits(['train', 'changed', 'error'])

const open = ref(false)
const photos = ref([])
const thumbs = ref({})
const busy = ref(false)

const missing = computed(() => Math.max(0, MIN_TRAIN - props.set.count))
const oneAngle = computed(() => props.set.count >= MIN_FACE && props.set.half + props.set.profile === 0)

async function toggle() {
  open.value = !open.value
  if (open.value) await load()
}

async function load() {
  try {
    photos.value = (await api.personSetList(props.set.key)) || []
  } catch (e) {
    emit('error', e)
    return
  }
  for (const p of photos.value) {
    if (thumbs.value[p.id]) continue
    api.personSetThumb(p.id).then(b64 => { thumbs.value = { ...thumbs.value, [p.id]: b64 } }).catch(e => console.warn('personSetThumb:', e))
  }
}

async function remove(p) {
  try {
    await api.personSetRemove(p.id)
    photos.value = photos.value.filter(x => x.id !== p.id)
    emit('changed')
  } catch (e) {
    emit('error', e)
  }
}

async function faceFromSet() {
  busy.value = true
  try {
    for (const l of props.loras) await api.addPersonIdentityFromSet(props.set.key, l.name)
    emit('changed')
  } catch (e) {
    emit('error', e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="set-card" data-testid="set-card">
    <div class="set-head">
      <div class="set-info">
        <div><b>{{ set.key }}</b> <span class="text-dim" data-testid="set-count">— {{ t('people.set_count', { n: set.count }) }}</span></div>
        <div class="text-dim set-angles" data-testid="set-angles">{{ t('people.set_angles', { front: set.front, half: set.half, profile: set.profile }) }}</div>
      </div>
      <div class="set-buttons">
        <button class="btn btn-sm btn-secondary" data-testid="set-toggle" @click="toggle">{{ open ? t('people.set_hide') : t('people.set_show') }}</button>
        <button v-if="loras.length && set.count >= MIN_FACE" class="btn btn-sm btn-secondary" :disabled="!available || !canFace || busy" data-testid="set-face" @click="faceFromSet">{{ busy ? t('people.uploading') : t('people.set_face') }}</button>
        <button class="btn btn-sm btn-primary" :disabled="!available || missing > 0" data-testid="set-train" @click="emit('train', set.key)">{{ t('people.set_train') }}</button>
      </div>
    </div>
    <div v-if="missing > 0" class="form-hint" data-testid="set-missing">{{ t('people.set_missing', { n: missing }) }}</div>
    <div v-if="oneAngle" class="form-hint" data-testid="set-one-angle">{{ t('people.set_one_angle') }}</div>
    <div v-if="open" class="set-grid" data-testid="set-grid">
      <div v-for="p in photos" :key="p.id" class="set-thumb" data-testid="set-photo">
        <img v-if="thumbs[p.id]" :src="'data:image/jpeg;base64,' + thumbs[p.id]" :alt="p.source_name" :title="p.source_name" />
        <button class="set-remove" :title="t('people.set_remove')" :aria-label="t('people.set_remove')" data-testid="set-remove" @click="remove(p)">×</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.set-card { padding: 8px 0; border-bottom: 1px solid var(--border); }
.set-card:last-child { border-bottom: none; }
.set-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.set-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.set-angles { font-size: 12px; }
.set-buttons { display: flex; gap: 6px; flex-shrink: 0; }
.set-card .form-hint { margin-top: 4px; }
.set-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(84px, 1fr)); gap: 6px; margin-top: 8px; }
.set-thumb { position: relative; aspect-ratio: 1; background: var(--surface-2); border-radius: var(--radius-sm); overflow: hidden; }
.set-thumb img { width: 100%; height: 100%; object-fit: cover; }
.set-remove { position: absolute; top: 2px; right: 2px; width: 20px; height: 20px; line-height: 18px; padding: 0; border: none; border-radius: 50%; background: rgba(0, 0, 0, 0.6); color: #fff; cursor: pointer; }
</style>
