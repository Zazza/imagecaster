<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { loadPeople, people } from '../peopleStore.js'

const status = ref('')
const available = computed(() => status.value === 'ok')
const jobs = ref([])
const folder = ref('')
const name = ref('')
const gender = ref('man')
const bases = ref(['Z-Image'])
const base = ref('Z-Image')
const busy = ref(false)
const error = ref('')
let timer = null
let closed = false

const ACTIVE = ['queued', 'running']
const activeJobs = computed(() => jobs.value.filter(j => ACTIVE.includes(j.status)))
const finishedJobs = computed(() => jobs.value.filter(j => !ACTIVE.includes(j.status)).slice(-5).reverse())
const canTrain = computed(() => available.value && folder.value && /^[A-Za-z0-9_ -]{2,32}$/.test(name.value.trim()) && !busy.value)

function percent(j) {
  const s = j.progress?.steps || 0
  return s ? Math.min(100, Math.round((j.progress.step / s) * 100)) : 0
}

function statusText(j) {
  if (j.status === 'running') return t('people.status_running', { step: j.progress?.step || 0, steps: j.progress?.steps || 0 })
  return t('people.status_' + j.status)
}

async function refresh() {
  try {
    jobs.value = (await api.listTrainingJobs()) || []
  } catch (e) {
    console.error('listTrainingJobs:', e)
  }
  if (closed) return
  schedule()
  if (activeJobs.value.length === 0) await loadPeople()
}

function schedule() {
  clearTimeout(timer)
  timer = null
  if (!closed && activeJobs.value.length > 0) timer = setTimeout(refresh, 3000)
}

async function init() {
  error.value = ''
  try {
    status.value = await api.trainingStatus()
  } catch (e) {
    status.value = 'unreachable'
  }
  await loadBases()
  await loadPeople()
  await refresh()
}

async function loadBases() {
  try {
    const list = await api.trainingBases()
    if (Array.isArray(list) && list.length) bases.value = list
  } catch (e) {
    console.error('trainingBases:', e)
  }
  if (!bases.value.includes(base.value)) base.value = bases.value[0]
}

async function pickFolder() {
  try {
    const dir = await api.pickPeopleFolder()
    if (dir) folder.value = dir
  } catch (e) {
    error.value = String(e)
  }
}

async function train() {
  busy.value = true
  error.value = ''
  try {
    await api.trainPerson(folder.value, name.value.trim(), gender.value, base.value)
    folder.value = ''
    name.value = ''
    await refresh()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function cancel(j) {
  try {
    await api.cancelTraining(j.id)
    await refresh()
  } catch (e) {
    error.value = String(e)
  }
}

async function remove(p) {
  if (!window.confirm(t('people.delete_confirm', { name: p.name }))) return
  try {
    await api.deletePerson(p.name)
    await loadPeople()
  } catch (e) {
    error.value = String(e)
  }
}

onMounted(init)
onUnmounted(() => {
  closed = true
  clearTimeout(timer)
})
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ t('app.nav_people') }}</h1>
    </div>
    <div v-if="status && status !== 'ok'" class="card people-note" data-testid="people-unavailable">{{ t('people.unavailable_' + status) }}</div>
    <div v-if="error" class="card people-error" data-testid="people-error">{{ error }}</div>

    <div class="card">
      <div class="card-title">{{ t('people.train_title') }}</div>
      <p class="text-dim people-hint">{{ t('people.train_hint') }}</p>
      <div class="people-form">
        <label class="form-label">{{ t('people.folder') }}</label>
        <div class="people-row">
          <input class="form-input" type="text" :value="folder" readonly :placeholder="t('people.folder_none')" data-testid="people-folder" />
          <button class="btn btn-secondary" :disabled="!available || busy" @click="pickFolder">{{ t('people.folder_pick') }}</button>
        </div>
        <label class="form-label">{{ t('people.name') }}</label>
        <input class="form-input" type="text" v-model="name" maxlength="32" :placeholder="t('people.name_hint')" data-testid="people-name" />
        <label class="form-label">{{ t('people.gender') }}</label>
        <select class="form-select" v-model="gender" data-testid="people-gender">
          <option value="man">{{ t('people.gender_man') }}</option>
          <option value="woman">{{ t('people.gender_woman') }}</option>
          <option value="person">{{ t('people.gender_person') }}</option>
        </select>
        <label class="form-label">{{ t('people.base') }}</label>
        <select class="form-select" v-model="base" data-testid="people-base">
          <option v-for="b in bases" :key="b" :value="b">{{ b }}</option>
        </select>
        <div class="form-hint">{{ t('people.base_hint') }}</div>
        <div>
          <button class="btn btn-primary" :disabled="!canTrain" data-testid="people-train" @click="train">{{ busy ? t('people.uploading') : t('people.train') }}</button>
        </div>
      </div>
    </div>

    <div v-for="j in activeJobs" :key="j.id" class="card" data-testid="people-job">
      <div class="people-row people-job-head">
        <span><b>{{ j.name }}</b> — {{ statusText(j) }}</span>
        <button class="btn btn-sm btn-secondary" @click="cancel(j)">{{ t('people.cancel') }}</button>
      </div>
      <div class="people-bar"><div class="people-bar-fill" :style="{ width: percent(j) + '%' }"></div></div>
      <p class="text-dim people-hint">{{ t('people.gpu_busy') }}</p>
    </div>

    <div class="card">
      <div class="card-title">{{ t('people.list_title') }}</div>
      <div v-if="people.length === 0" class="text-dim">{{ t('people.empty') }}</div>
      <div v-for="p in people" :key="p.name" class="people-row people-item" data-testid="people-item">
        <span><b>{{ p.name }}</b> <span class="text-dim">{{ p.base || 'Z-Image' }}</span> <span class="mono text-dim">{{ p.trigger }}</span></span>
        <button class="btn btn-sm btn-secondary" @click="remove(p)">{{ t('people.delete') }}</button>
      </div>
    </div>

    <div v-if="finishedJobs.length" class="card">
      <div class="card-title">{{ t('people.recent') }}</div>
      <div v-for="j in finishedJobs" :key="j.id" class="people-row">
        <span>{{ j.name }} — {{ statusText(j) }}<span v-if="j.error" class="text-dim"> ({{ j.error.message }})</span></span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.card { margin-bottom: 12px; }
.people-form { display: flex; flex-direction: column; gap: 6px; max-width: 520px; }
.people-row { display: flex; align-items: center; gap: 8px; justify-content: space-between; }
.people-row .form-input { flex: 1; }
.people-item { padding: 6px 0; border-bottom: 1px solid var(--border); }
.people-hint { margin: 4px 0 8px; font-size: 12px; }
.people-error { color: var(--danger, #e5484d); }
.people-bar { height: 6px; background: var(--border); border-radius: 3px; overflow: hidden; margin-top: 8px; }
.people-bar-fill { height: 100%; background: var(--accent); transition: width 0.3s ease; }
</style>
