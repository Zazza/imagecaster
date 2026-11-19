<script setup>
import { ref, computed } from 'vue'
import { t } from '../../i18n/index.js'
import { PRESET_NAME_MAX, PLACE_ANY } from '../../composables/useStylistPlan.js'

const FRAMINGS = ['close', 'waist', 'knee', 'full']

const props = defineProps({
  preset: { type: Object, required: true },
  places: { type: Array, default: () => [] },
})
const emit = defineEmits(['save', 'cancel'])

const name = ref('')
const place = ref(props.places.includes(props.preset.place) ? props.preset.place : '')
const framing = ref(FRAMINGS.includes(props.preset.framing) ? props.preset.framing : 'full')
const chosen = ref([...(props.preset.places || [])])

const isPose = computed(() => props.preset.aspect === 'pose')
const isBackground = computed(() => props.preset.aspect === 'background')
const placeChips = computed(() => [...props.places, PLACE_ANY])
const canSave = computed(() => {
  if (!name.value.trim()) return false
  if (isBackground.value && !place.value) return false
  if (isPose.value && !chosen.value.length) return false
  return true
})

function togglePlace(p) {
  chosen.value = chosen.value.includes(p) ? chosen.value.filter(x => x !== p) : [...chosen.value, p]
}

function save() {
  if (!canSave.value) return
  const out = { ...props.preset, name: name.value.trim() }
  if (isBackground.value) out.place = place.value
  if (isPose.value) Object.assign(out, { framing: framing.value, places: [...chosen.value] })
  emit('save', out)
}
</script>

<template>
  <div class="save-preset" data-testid="save-preset-form">
    <input class="form-input" type="text" v-model="name" :maxlength="PRESET_NAME_MAX" :placeholder="t('stylist.preset_name')" data-testid="preset-name" @keydown.enter="save" @keydown.esc="emit('cancel')" />
    <select v-if="isBackground" class="form-select" v-model="place" :title="t('stylist.preset_place')" data-testid="preset-place">
      <option value="" disabled>{{ t('stylist.preset_place') }}</option>
      <option v-for="p in places" :key="p" :value="p">{{ t('stylist.place_' + p) }}</option>
    </select>
    <template v-if="isPose">
      <select class="form-select" v-model="framing" :title="t('stylist.preset_framing')" data-testid="preset-framing">
        <option v-for="f in FRAMINGS" :key="f" :value="f">{{ t('stylist.framing_' + f) }}</option>
      </select>
      <div class="save-preset-places">
        <span class="text-dim">{{ t('stylist.preset_places') }}</span>
        <button v-for="p in placeChips" :key="p" type="button" class="styles-chip" :class="{ active: chosen.includes(p) }" :data-testid="'preset-place-' + p" @click="togglePlace(p)">{{ t('stylist.place_' + p) }}</button>
      </div>
    </template>
    <div class="save-preset-actions">
      <button class="btn btn-primary btn-sm" :disabled="!canSave" data-testid="preset-save" @click="save">{{ t('stylist.preset_save') }}</button>
      <button class="btn btn-secondary btn-sm" data-testid="preset-cancel" @click="emit('cancel')">{{ t('stylist.preset_cancel') }}</button>
    </div>
  </div>
</template>

<style scoped>
.save-preset { grid-column: 1 / -1; display: flex; flex-wrap: wrap; gap: 6px; align-items: center; padding: 6px; border: 1px dashed var(--border); border-radius: var(--radius-sm); }
.save-preset .form-input { flex: 1 1 200px; }
.save-preset .form-select { flex: 0 0 auto; width: auto; }
.save-preset-places { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; font-size: var(--fs-sm); }
.save-preset-actions { display: flex; gap: 6px; }
</style>
