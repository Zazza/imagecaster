<script setup>
import { computed } from 'vue'
import { t } from '../../i18n/index.js'

const props = defineProps({
  aspect: { type: String, required: true },
  choice: { type: Object, required: true },
  items: { type: Array, default: () => [] },
  themes: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:choice'])

const MODES = ['auto', 'keep', 'pick']

const groups = computed(() => {
  const out = new Map()
  const themeName = Object.fromEntries(props.themes.map(th => [th.id, th.name]))
  for (const it of props.items) {
    let key = ''
    if (props.aspect === 'pose') key = t('stylist.framing_' + it.framing)
    else if (it.theme) key = themeName[it.theme] || it.theme
    if (!out.has(key)) out.set(key, [])
    out.get(key).push(it)
  }
  return [...out.entries()].map(([name, items]) => ({ name, items }))
})

function setMode(mode) {
  emit('update:choice', { mode, ids: props.choice.ids || [] })
}

function toggle(id) {
  const ids = props.choice.ids || []
  const next = ids.includes(id) ? ids.filter(x => x !== id) : [...ids, id]
  emit('update:choice', { mode: 'pick', ids: next })
}
</script>

<template>
  <div class="aspect-tab" :data-testid="'aspect-' + aspect">
      <div class="aspect-modes">
        <label v-for="m in MODES" :key="m" class="check-label">
          <input type="radio" :name="'mode-' + aspect" :value="m" :checked="choice.mode === m" :data-testid="'mode-' + m" @change="setMode(m)" />
          {{ t('stylist.mode_' + m) }}
        </label>
      </div>
      <div class="form-hint">{{ t('stylist.mode_hint_' + (aspect === 'pose' ? 'pose_' : '') + choice.mode) }}</div>
      <div v-if="choice.mode === 'pick'" class="aspect-groups">
        <div v-for="g in groups" :key="g.name" class="aspect-group">
          <div v-if="g.name" class="styles-filter-label">{{ g.name }}</div>
          <div class="styles-chips">
            <button v-for="it in g.items" :key="it.id" type="button" class="styles-chip" :class="{ active: (choice.ids || []).includes(it.id) }" :title="it.prompt" :data-testid="'pick-' + it.id" @click="toggle(it.id)">{{ it.name }}</button>
          </div>
        </div>
      </div>
  </div>
</template>

<style scoped>
.aspect-tab { display: flex; flex-direction: column; gap: 8px; }
.aspect-modes { display: flex; gap: 16px; flex-wrap: wrap; }
.aspect-groups { display: flex; flex-direction: column; gap: 8px; }
.aspect-group { display: flex; flex-direction: column; gap: 4px; }
</style>
