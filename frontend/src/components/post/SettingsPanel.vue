<script setup>
import { onMounted } from 'vue'
import { X } from 'lucide-vue-next'
import { t } from '../../i18n/index.js'
import { usePostEditor, MAX_EDITOR_LORAS, DEFAULT_EDITOR_WEIGHT, MIN_EDITOR_WEIGHT, MAX_EDITOR_WEIGHT, EDITOR_WEIGHT_STEP } from '../../composables/usePostEditor.js'
import { useServerLoras } from '../../composables/useServerLoras.js'
const { model, rows, options, selected, ready, known, loaded, error } = usePostEditor()
defineProps({
  idPrefix: { type: String, default: 'post-editor' },
  scopeKey: { type: String, default: 'post_editor.scope' },
})
const { loras, error: loraError, load } = useServerLoras()
onMounted(load)
function addLora() {
  if (rows.value.length < MAX_EDITOR_LORAS) rows.value.push({ name: '', weight: DEFAULT_EDITOR_WEIGHT })
}
</script>

<template>
  <div class="form-group">
    <label class="form-label" :for="`${idPrefix}-model`">{{ t('post_editor.model') }}</label>
    <select :id="`${idPrefix}-model`" :data-testid="`${idPrefix}-model`" class="form-select" v-model="model">
      <option v-if="!model" value="">{{ t('post_editor.none') }}</option>
      <option v-for="option in options" :key="option.value" :value="option.value" :disabled="!option.available">{{ option.label }}{{ option.available ? '' : ` — ${t('post_editor.unavailable')}` }}</option>
    </select>
    <div class="form-hint">{{ t(scopeKey) }}</div>
    <div v-if="!loaded" class="form-hint">{{ t('post_editor.loading') }}</div>
    <div v-else-if="!known" class="form-hint">{{ t('post_editor.inventory_error') }}</div>
    <div v-else-if="!ready" :data-testid="`${idPrefix}-unavailable`" class="form-hint">{{ t('post_editor.missing') }} {{ selected?.missing?.join(', ') }}</div>
  </div>
  <div class="form-group">
    <label class="form-label">{{ t('edit.loras') }}</label>
    <div v-for="(row, index) in rows" :key="index" class="editor-lora-row">
      <input :data-testid="`${idPrefix}-lora-name`" v-model="row.name" class="form-input" :list="`${idPrefix}-loras`" :aria-label="t('edit.loras_name')" />
      <input :data-testid="`${idPrefix}-lora-weight`" v-model.number="row.weight" class="form-input editor-weight" type="number" :min="MIN_EDITOR_WEIGHT" :max="MAX_EDITOR_WEIGHT" :step="EDITOR_WEIGHT_STEP" :aria-label="t('edit.loras_weight')" />
      <button class="btn btn-sm btn-secondary" @click="rows.splice(index, 1)" :aria-label="t('edit.loras_remove')"><X :size="14" /></button>
    </div>
    <button :data-testid="`${idPrefix}-lora-add`" class="btn btn-sm btn-secondary" :disabled="!model || rows.length >= MAX_EDITOR_LORAS" @click="addLora">{{ t('edit.loras_add') }}</button>
    <datalist :id="`${idPrefix}-loras`"><option v-for="lora in loras" :key="lora.name" :value="lora.name" /></datalist>
    <div v-if="loraError" class="form-hint">{{ t('overrides.load_error') }}</div>
  </div>
  <div v-if="error" role="alert" class="form-hint">{{ error }}</div>
</template>

<style scoped>
.editor-lora-row { display: flex; gap: 8px; margin-bottom: 8px; }
.editor-weight { width: 80px; flex: none; }
</style>
