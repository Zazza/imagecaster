<script setup>
import { ref, computed } from 'vue'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { trainBaseLabel, trainMinutes, loraState, newPhotosSince } from '../../people.js'

const MIN_TRAIN = 10
const MIN_FACE = 5

const props = defineProps({
  set: { type: Object, required: true },
  loras: { type: Array, default: () => [] },
  canFace: { type: Boolean, default: false },
  available: { type: Boolean, default: false },
  busyNames: { type: Array, default: () => [] },
})
const emit = defineEmits(['train', 'changed', 'error', 'started'])

const open = ref(false)
const photos = ref([])
const thumbs = ref({})
const busy = ref(false)
const acting = ref('')

const loraRows = computed(() => props.loras.map(l => ({
  lora: l,
  state: loraState(l),
  fresh: newPhotosSince(props.set.added, l.photos_at || l.created_at),
  busy: props.busyNames.includes(l.name),
})))

function stateText(st) {
  if (st.kind === 'continued') return t('people.lora_continued', { n: st.n })
  return t(st.kind === 'quick' ? 'people.lora_quick' : 'people.lora_full')
}

function minutesText(base, mode) {
  const m = trainMinutes(base, mode)
  return m ? t('people.train_minutes', { m }) : ''
}

async function trainLora(row, mode) {
  const l = row.lora
  const key = mode === 'continue' ? 'people.continue_confirm' : 'people.retrain_confirm'
  if (!window.confirm(t(key, { base: trainBaseLabel(l.base || 'Z-Image'), n: row.fresh, total: props.set.count, time: minutesText(l.base, mode) }))) return
  acting.value = l.name
  try {
    await api.trainPersonFromSet(props.set.key, l.name, l.gender || 'person', l.base || 'Z-Image', mode, false, false)
    emit('started')
  } catch (e) {
    emit('error', e)
  } finally {
    acting.value = ''
  }
}

async function rollback(row) {
  if (!window.confirm(t('people.rollback_confirm', { base: trainBaseLabel(row.lora.base || 'Z-Image') }))) return
  acting.value = row.lora.name
  try {
    await api.rollbackPerson(row.lora.name)
    emit('changed')
  } catch (e) {
    emit('error', e)
  } finally {
    acting.value = ''
  }
}

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
    <div v-for="row in loraRows" :key="row.lora.name" class="set-lora" data-testid="set-lora">
      <div class="set-lora-info">
        <span>{{ trainBaseLabel(row.lora.base || 'Z-Image') }}</span>
        <span class="text-dim" data-testid="set-lora-state">{{ stateText(row.state) }}</span>
        <span class="text-dim" data-testid="set-lora-new">{{ row.fresh ? t('people.lora_new_photos', { n: row.fresh }) : t('people.lora_no_new') }}</span>
        <span v-if="row.busy" class="text-dim" data-testid="set-lora-busy">{{ t('people.lora_training') }}</span>
      </div>
      <div class="set-buttons">
        <button class="btn btn-sm btn-primary" :disabled="!available || row.busy || !!acting || set.count < MIN_TRAIN" :title="minutesText(row.lora.base, 'continue')" data-testid="set-continue" @click="trainLora(row, 'continue')">{{ t('people.lora_continue') }}</button>
        <button class="btn btn-sm btn-secondary" :disabled="!available || row.busy || !!acting || set.count < MIN_TRAIN" :title="minutesText(row.lora.base, 'retrain')" data-testid="set-retrain" @click="trainLora(row, 'retrain')">{{ t('people.lora_retrain') }}</button>
        <button v-if="row.lora.previous" class="btn btn-sm btn-secondary" :disabled="!available || row.busy || !!acting" data-testid="set-rollback" @click="rollback(row)">{{ t('people.lora_rollback') }}</button>
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
.set-lora { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-top: 6px; padding-left: 12px; border-left: 2px solid var(--border); }
.set-lora-info { display: flex; gap: 10px; flex-wrap: wrap; font-size: 13px; }
.set-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(84px, 1fr)); gap: 6px; margin-top: 8px; }
.set-thumb { position: relative; aspect-ratio: 1; background: var(--surface-2); border-radius: var(--radius-sm); overflow: hidden; }
.set-thumb img { width: 100%; height: 100%; object-fit: cover; }
.set-remove { position: absolute; top: 2px; right: 2px; width: 20px; height: 20px; line-height: 18px; padding: 0; border: none; border-radius: 50%; background: rgba(0, 0, 0, 0.6); color: #fff; cursor: pointer; }
</style>
