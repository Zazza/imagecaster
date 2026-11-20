<script setup>
import { ref, computed, watch } from 'vue'
import { BookmarkPlus } from 'lucide-vue-next'
import { t, locale } from '../../i18n/index.js'
import { SERIES_FIELDS, setRuField, setEnField, setShotRu, setShotEn, presetFromField, presetFromShot } from '../../composables/useStylistPlan.js'
import SavePresetForm from './SavePresetForm.vue'

const props = defineProps({
  series: { type: Object, required: true },
  index: { type: Number, required: true },
  outfitLocked: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
  locked: { type: Boolean, default: false },
  places: { type: Array, default: () => [] },
  epoch: { type: Number, default: 0 },
})
const emit = defineEmits(['update', 'regenerate', 'delete', 'delete-shot', 'save-preset'])

const showPrompt = ref(false)
const saving = ref(null)
const ruMode = computed(() => locale.value === 'ru')
const hasDirty = computed(() => (props.series.ru_dirty?.length || 0) > 0 || props.series.shots.some(sh => sh.ru_dirty))

function showsRu(text) {
  return ruMode.value && !showPrompt.value && !!text
}

function fieldValue(f) {
  return showsRu(props.series[f + '_ru']) ? props.series[f + '_ru'] : props.series[f]
}

function setField(f, value) {
  const ru = showsRu(props.series[f + '_ru'])
  emit('update', ru ? setRuField(props.series, f, value) : setEnField(props.series, f, value))
}

const savingPreset = computed(() => {
  const k = saving.value
  if (!k) return null
  if (k.shot >= 0) {
    const sh = props.series.shots[k.shot]
    return sh ? presetFromShot(props.series, k.shot, ruMode.value && !!sh.prompt_ru) : null
  }
  return presetFromField(props.series, k.field, ruMode.value && !!props.series[k.field + '_ru'])
})

watch(() => props.epoch, () => {
  saving.value = null
})

function openSave(key, field, shot) {
  saving.value = saving.value?.key === key ? null : { key, field, shot }
}

function saveField(f) {
  openSave(f, f, -1)
}

function saveShot(j) {
  openSave('shot-' + j, '', j)
}

function onSave(payload) {
  saving.value = null
  emit('save-preset', payload)
}

function setShot(j, value) {
  const ru = showsRu(props.series.shots[j].prompt_ru)
  emit('update', ru ? setShotRu(props.series, j, value) : setShotEn(props.series, j, value))
}
</script>

<template>
  <div class="card series-card" data-testid="series-card">
    <div class="series-head">
      <div class="card-title">{{ t('stylist.series_title', { n: index + 1 }) }}</div>
      <div class="series-actions">
        <button v-if="ruMode" class="btn btn-sm" :class="showPrompt ? 'btn-primary' : 'btn-secondary'" :aria-pressed="showPrompt" :title="t('stylist.prompt_toggle_hint')" data-testid="series-prompt-toggle" @click="showPrompt = !showPrompt">{{ t('stylist.prompt_toggle') }}</button>
        <button class="btn btn-secondary btn-sm" :disabled="busy || locked" data-testid="series-regenerate" @click="emit('regenerate', index)">{{ busy ? t('stylist.regenerating') : t('stylist.regenerate') }}</button>
        <button class="btn btn-danger-quiet btn-sm" :disabled="locked" data-testid="series-delete" @click="emit('delete', index)">{{ t('stylist.delete_series') }}</button>
      </div>
    </div>
    <div v-if="ruMode && hasDirty" class="form-hint" data-testid="series-dirty">{{ t('stylist.ru_dirty_hint') }}</div>
    <div class="series-fields">
      <template v-for="f in SERIES_FIELDS" :key="f">
        <label class="form-label">{{ t('stylist.tab_' + f) }}<span v-if="f === 'background' && series.place" class="text-dim"> · {{ t('stylist.place_' + series.place) }}</span><span v-if="series.ru_ai?.includes(f) && showsRu(series[f + '_ru']) && !(f === 'outfit' && outfitLocked)" class="text-dim series-ai" :data-testid="'series-ai-' + f"> · {{ t('stylist.ru_ai') }}</span></label>
        <div v-if="f === 'outfit' && outfitLocked" class="form-hint">{{ t('stylist.outfit_from_photo') }}</div>
        <div v-else class="series-field">
          <input class="form-input" type="text" :value="fieldValue(f)" :data-lang="showsRu(series[f + '_ru']) ? 'ru' : 'en'" :placeholder="t('stylist.field_empty')" :data-testid="'series-' + f" @change="setField(f, $event.target.value)" />
          <button class="btn btn-secondary btn-xs" :disabled="!series[f]" :title="t('stylist.save_preset')" :aria-label="t('stylist.save_preset')" :data-testid="'save-preset-' + f" @click="saveField(f)"><BookmarkPlus :size="14" /> {{ t('stylist.save_mine') }}</button>
        </div>
        <SavePresetForm v-if="saving?.key === f && savingPreset" :key="'form-' + f" :preset="savingPreset" :places="places" @save="onSave" @cancel="saving = null" />
      </template>
    </div>
    <div class="form-label">{{ t('stylist.shots', { n: series.shots.length }) }}</div>
    <div v-for="(sh, j) in series.shots" :key="j" class="series-shot" data-testid="series-shot">
      <span class="series-shot-name">{{ sh.pose || t('stylist.shot_no_pose') }}<span v-if="sh.landscape" class="badge badge-muted">{{ t('stylist.landscape') }}</span></span>
      <input class="form-input" type="text" :value="showsRu(sh.prompt_ru) ? sh.prompt_ru : sh.prompt" :data-lang="showsRu(sh.prompt_ru) ? 'ru' : 'en'" data-testid="shot-prompt" @change="setShot(j, $event.target.value)" />
      <button class="btn btn-secondary btn-xs" :disabled="!sh.prompt" :title="t('stylist.save_preset')" :aria-label="t('stylist.save_preset')" data-testid="shot-save" @click="saveShot(j)"><BookmarkPlus :size="14" /> {{ t('stylist.save_mine') }}</button>
      <button class="btn btn-danger-quiet btn-xs" :disabled="locked" :title="t('stylist.delete_shot')" data-testid="shot-delete" @click="emit('delete-shot', index, j)">×</button>
      <SavePresetForm v-if="saving?.key === 'shot-' + j && savingPreset" :preset="savingPreset" :places="places" @save="onSave" @cancel="saving = null" />
    </div>
  </div>
</template>

<style scoped>
.series-card { display: flex; flex-direction: column; gap: 6px; }
.series-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.series-actions { display: flex; gap: 6px; }
.series-fields { display: grid; grid-template-columns: 160px 1fr; gap: 6px 10px; align-items: center; }
.series-shot { display: grid; grid-template-columns: 200px 1fr auto auto; gap: 8px; align-items: center; }
.series-field { display: flex; gap: 6px; align-items: center; }
.series-field .form-input { flex: 1 1 auto; }
.series-ai { font-size: var(--fs-xs); }
.series-shot-name { font-size: var(--fs-sm); display: inline-flex; gap: 6px; align-items: center; }
</style>
