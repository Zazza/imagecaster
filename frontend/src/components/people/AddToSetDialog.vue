<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { loadPeople, people } from '../../peopleStore.js'
import { personKey } from '../../people.js'

const props = defineProps({ paths: { type: Array, required: true } })
const emit = defineEmits(['close', 'added'])

const NEW = '__new__'
const keys = ref([])
const picked = ref('')
const newKey = ref('')
const busy = ref(false)
const index = ref(0)
const done = ref([])
const pending = ref(null)
const error = ref('')
const total = ref(null)
const MIN_TRAIN = 10

const norm = (k) => String(k || '').trim().replace(/\s+/g, ' ')
const key = computed(() => {
  if (picked.value !== NEW) return picked.value
  const typed = norm(newKey.value)
  return keys.value.find(k => k.toLowerCase() === typed.toLowerCase()) || typed
})
const fileName = (p) => p.split(/[\\/]/).pop()
const finished = computed(() => !busy.value && !pending.value && done.value.length === props.paths.length)
const addedCount = computed(() => done.value.filter(d => d.ok).length)

onMounted(async () => {
  const names = new Set()
  try {
    for (const s of (await api.personSetSummary()) || []) names.add(s.key)
  } catch (e) {
    error.value = String(e?.message || e)
  }
  try {
    await loadPeople()
  } catch (e) {
    console.warn('loadPeople:', e)
  }
  for (const p of people.value || []) names.add(personKey(p))
  keys.value = [...names].filter(Boolean).sort((a, b) => a.localeCompare(b))
  picked.value = keys.value[0] || NEW
})

function errText(e) {
  return String(e?.message || e).replace(/^Error:\s*/, '')
}

async function addOne(extra = {}) {
  const path = props.paths[index.value]
  try {
    const res = await api.personSetAdd({ path, key: key.value, ...extra })
    if (res?.pick_face || res?.confirm) {
      pending.value = { path, res, extra }
      return false
    }
    done.value.push({ path, ok: true, warnings: res?.warnings || [] })
  } catch (e) {
    done.value.push({ path, ok: false, reason: errText(e) })
  }
  index.value++
  return true
}

async function run() {
  busy.value = true
  error.value = ''
  try {
    while (index.value < props.paths.length) {
      if (!(await addOne())) return
    }
    if (addedCount.value) emit('added', key.value)
    await loadTotal()
  } finally {
    busy.value = false
  }
}

async function loadTotal() {
  try {
    const set = ((await api.personSetSummary()) || []).find(x => x.key.toLowerCase() === key.value.toLowerCase())
    total.value = set ? set.count : 0
  } catch (e) {
    console.warn('personSetSummary:', e)
  }
}

async function start() {
  if (!key.value || busy.value) return
  index.value = 0
  done.value = []
  await run()
}

async function resolve(extra) {
  const p = pending.value
  pending.value = null
  busy.value = true
  try {
    if (!(await addOne({ ...p.extra, ...extra }))) return
  } finally {
    busy.value = false
  }
  await run()
}

function skip() {
  done.value.push({ path: pending.value.path, ok: false, reason: t('people.set_skipped') })
  pending.value = null
  index.value++
  run()
}

const boxStyle = (b) => ({ left: b[0] * 100 + '%', top: b[1] * 100 + '%', width: (b[2] - b[0]) * 100 + '%', height: (b[3] - b[1]) * 100 + '%' })
</script>

<template>
  <div class="modal-overlay set-overlay">
    <div class="modal-content set-dialog" data-testid="set-dialog">
      <div class="card-title">{{ t('people.set_add_title', { n: paths.length }) }}</div>
      <p class="text-dim">{{ t('people.set_add_hint') }}</p>

      <template v-if="!done.length && !pending && !busy">
        <label class="form-label">{{ t('people.display') }}</label>
        <select class="form-select" v-model="picked" data-testid="set-key">
          <option v-for="k in keys" :key="k" :value="k">{{ k }}</option>
          <option :value="NEW">{{ t('people.set_new_person') }}</option>
        </select>
        <input v-if="picked === NEW" class="form-input set-new" type="text" v-model="newKey" maxlength="40" :placeholder="t('people.display_hint')" data-testid="set-new-key" />
        <div class="set-actions">
          <button class="btn btn-primary" :disabled="!key" data-testid="set-add" @click="start">{{ t('people.set_add_btn') }}</button>
          <button class="btn btn-secondary" @click="emit('close')">{{ t('people.set_cancel') }}</button>
        </div>
      </template>

      <div v-if="busy && !pending" class="text-dim" data-testid="set-progress">{{ t('people.set_checking', { i: Math.min(index + 1, paths.length), n: paths.length }) }}</div>

      <div v-if="pending" class="set-pending" data-testid="set-pending">
        <div class="set-question">{{ fileName(pending.path) }}: {{ pending.res.pick_face ? t('people.set_pick_face') : pending.res.confirm }}</div>
        <div class="set-preview">
          <img :src="'data:image/jpeg;base64,' + pending.res.preview" alt="" />
          <button
            v-for="(f, i) in pending.res.faces" :key="i" class="set-face" :style="boxStyle(f.box)"
            :class="{ 'set-face-pick': pending.res.pick_face }" :disabled="!pending.res.pick_face"
            :data-testid="'set-face-' + i" :title="t('people.set_this_face')" @click="resolve({ face_index: i })"
          />
        </div>
        <div class="set-actions">
          <button v-if="pending.res.confirm" class="btn btn-primary" data-testid="set-confirm" @click="resolve({ confirm: true })">{{ t('people.set_yes') }}</button>
          <button class="btn btn-secondary" data-testid="set-skip" @click="skip">{{ t('people.set_skip') }}</button>
        </div>
      </div>

      <template v-if="finished">
        <div class="set-summary" data-testid="set-summary">{{ t('people.set_result', { ok: addedCount, n: paths.length, key }) }}</div>
        <div v-if="total !== null" class="set-total" data-testid="set-total">
          {{ t('people.set_total', { key, n: total }) }}<template v-if="total < MIN_TRAIN"> — {{ t('people.set_missing', { n: MIN_TRAIN - total }).toLowerCase() }}</template>
        </div>
        <ul class="set-list">
          <li v-for="d in done.filter(x => !x.ok || x.warnings.length)" :key="d.path" :class="{ 'set-bad': !d.ok }" data-testid="set-item">
            {{ fileName(d.path) }} — {{ d.ok ? d.warnings.join('; ') : d.reason }}
          </li>
        </ul>
        <div class="set-actions">
          <button class="btn btn-primary" data-testid="set-close" @click="emit('close')">{{ t('people.set_close') }}</button>
        </div>
      </template>

      <div v-if="error" class="form-hint set-bad">{{ error }}</div>
    </div>
  </div>
</template>

<style scoped>
.set-overlay { z-index: 300; }
.set-dialog { max-width: 520px; display: flex; flex-direction: column; gap: 10px; }
.set-new { margin-top: 6px; }
.set-actions { display: flex; gap: 8px; margin-top: 4px; }
.set-preview { position: relative; display: inline-block; max-width: 100%; }
.set-preview img { display: block; max-width: 100%; max-height: 50vh; border-radius: var(--radius-sm); }
.set-face { position: absolute; border: 2px solid var(--warning, #e0a030); background: transparent; border-radius: 4px; padding: 0; }
.set-face-pick { cursor: pointer; border-color: var(--accent, #4a9eff); }
.set-face-pick:hover { background: rgba(74, 158, 255, 0.2); }
.set-list { margin: 0; padding-left: 18px; font-size: var(--fs-sm); }
.set-bad { color: var(--danger, #e05050); }
.set-question { font-weight: 500; }
</style>
