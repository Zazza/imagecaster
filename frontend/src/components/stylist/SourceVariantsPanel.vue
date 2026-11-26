<script setup>
import { ref, computed, watch, onUnmounted } from 'vue'
import { api } from '../../api.js'
import { t, locale } from '../../i18n/index.js'
import { adoptSourceFrames, sourceSubmitting } from '../../composables/useStylistPlan.js'
import { people } from '../../peopleStore.js'
import { peopleGroups, groupBases, groupLoraForBase, canFaceSwap, trainBaseLabel } from '../../people.js'
import { readImageFile } from '../../imageFile.js'
import { b64Url } from '../../imageDataUrl.js'
import { usePostEditor } from '../../composables/usePostEditor.js'
import EditorSettings from '../post/SettingsPanel.vue'
const props = defineProps({ locked: Boolean, faceSwapOn: Boolean })
const person = ref('')
const base = ref('')
const faceSwap = ref(false)
const groups = computed(() => peopleGroups(people.value.filter(value => value.lora)))
const group = computed(() => groups.value.find(value => value.key === person.value))
const bases = computed(() => groupBases(group.value))
const facePerson = computed(() => groupLoraForBase(group.value, base.value || bases.value[0]))
const swapAvailable = computed(() => canFaceSwap(facePerson.value, props.faceSwapOn))
watch(person, () => { base.value = ''; faceSwap.value = false })
watch(swapAvailable, value => { if (!value) faceSwap.value = false })
const emit = defineEmits(['source', 'queued', 'busy'])
const MAX_VARIANTS = 8
const MAX_INSTRUCTION = 2000
const DEFAULT_VARIANTS = 3
const image = ref('')
const wish = ref('')
const count = ref(DEFAULT_VARIANTS)
const instructions = ref([])
const plannedFor = ref('')
const planning = ref(false)
const submitting = sourceSubmitting
const reading = ref(false)
const error = ref('')
const editor = usePostEditor()
const photoInput = ref(null)
const openSection = ref('photo')
let revision = 0
let fileSequence = 0
let disposed = false
const fingerprint = computed(() => JSON.stringify([image.value, wish.value, count.value]))
const countValid = computed(() => Number.isInteger(count.value) && count.value >= 1 && count.value <= MAX_VARIANTS)
const busy = computed(() => planning.value || submitting.value || reading.value)
const validPlan = computed(() => plannedFor.value === fingerprint.value && countValid.value && instructions.value.length === count.value && instructions.value.every(v => v.trim() && [...v].length <= MAX_INSTRUCTION))
watch(fingerprint, () => { revision++; plannedFor.value = '' }, { flush: 'sync' })
watch(busy, value => emit('busy', value), { flush: 'sync' })
onUnmounted(() => { disposed = true; revision++; fileSequence++ })
async function upload(event) {
  if (props.locked || submitting.value) return
  const sequence = ++fileSequence
  revision++
  plannedFor.value = ''
  reading.value = true
  try {
    const value = await readImageFile(event.target.files?.[0])
    if (disposed || sequence !== fileSequence) return
    image.value = value
    instructions.value = []
    error.value = ''
    emit('source', value)
    openSection.value = 'task'
  } catch (e) {
    if (!disposed && sequence === fileSequence) error.value = String(e?.message || e)
  } finally {
    if (!disposed && sequence === fileSequence) reading.value = false
  }
}
async function plan() {
  if (busy.value || props.locked || !image.value || !countValid.value) return
  planning.value = true
  error.value = ''
  const version = revision
  const key = fingerprint.value
  try {
    const result = await api.stylistSourcePlan({ image_base64: image.value, wish: wish.value, count: count.value, locale: locale.value })
    if (disposed || revision !== version) return
    if (!Array.isArray(result) || result.length !== count.value || !result.every(v => typeof v === 'string' && v.trim() && [...v].length <= MAX_INSTRUCTION)) throw new Error(t('source.invalid_plan'))
    instructions.value = [...result]
    plannedFor.value = key
    openSection.value = 'poses'
  } catch (e) {
    if (!disposed && revision === version) error.value = String(e?.message || e)
  } finally { if (!disposed) planning.value = false }
}
function remove(index) {
  const wasCurrent = validPlan.value
  instructions.value.splice(index, 1)
  count.value = instructions.value.length
  if (wasCurrent) plannedFor.value = fingerprint.value
}
async function run() {
  if (busy.value || props.locked || !validPlan.value || !editor.ready.value) return
  submitting.value = true
  error.value = ''
  try {
    const selected = editor.snapshot()
    const snapshot = { model: selected.model, loras: (selected.loras || []).map(lora => ({ ...lora })) }
    const request = { image_base64: image.value, instructions: [...instructions.value], editor: snapshot }
    if (person.value) Object.assign(request, { person: person.value, base: base.value, face_swap: !!(swapAvailable.value && faceSwap.value) })
    const result = await api.stylistSourceShoot(request)
    if (result?.job_ids?.length) {
      adoptSourceFrames(result.job_ids)
      emit('queued', result)
    }
    if (!disposed) error.value = result?.error || ''
  } catch (e) { if (!disposed) error.value = String(e?.message || e) }
  finally { submitting.value = false }
}
</script>

<template>
  <div class="source-variants" data-testid="source-variants">
    <div class="gen-sections accordion" data-testid="source-sections">
      <div class="card" :class="{ collapsed: openSection !== 'photo' }">
        <div class="card-header card-toggle" role="button" tabindex="0" :aria-expanded="openSection === 'photo'" @click="openSection = openSection === 'photo' ? '' : 'photo'" @keydown.enter="openSection = 'photo'" @keydown.space.prevent="openSection = 'photo'">
          <span class="card-heading"><span class="card-num mono">01</span><span class="card-title">{{ t('source.photo') }}</span></span>
        </div>
        <div class="card-collapse" :class="{ collapsed: openSection !== 'photo' }">
          <div class="card-body">
            <div class="form-group source-photo-row">
              <input id="source-photo" ref="photoInput" type="file" accept="image/*" hidden data-testid="source-photo-input" :disabled="locked || submitting" @change="upload" />
              <img v-if="image" :src="b64Url(image)" :alt="t('source.photo')" class="source-thumb" />
              <button type="button" class="btn btn-secondary btn-sm" :disabled="locked || submitting" data-testid="source-photo-pick" @click="photoInput?.click()">{{ t(image ? 'source.change_photo' : 'source.choose_photo') }}</button>
            </div>
            <div class="form-group">
              <label class="form-label" for="source-person">{{ t('stylist.person') }}</label>
              <select id="source-person" v-model="person" class="form-select" :disabled="locked || submitting" data-testid="source-person">
                <option value="">{{ t('people.pick_none') }}</option>
                <option v-for="value in groups" :key="value.key" :value="value.key">{{ value.key }}</option>
              </select>
            </div>
            <div v-if="person" class="form-group">
              <label class="form-label" for="source-face-model">{{ t('source.face_model') }}</label>
              <select id="source-face-model" v-model="base" class="form-select" :disabled="locked || submitting" data-testid="source-face-model">
                <option value="">{{ t('preset.automatic') }}</option>
                <option v-for="value in bases" :key="value" :value="value">{{ trainBaseLabel(value) }}</option>
              </select>
              <div class="form-hint">{{ t('stylist.face_pass_hint') }}</div>
              <label v-if="swapAvailable" class="form-label"><input v-model="faceSwap" type="checkbox" :disabled="locked || submitting" data-testid="source-face-swap" /> {{ t('people.face_swap') }}</label>
            </div>
            <div class="form-hint">{{ t('source.preserve') }}</div>
            <div class="form-hint">{{ t('source.limitations') }}</div>
          </div>
        </div>
      </div>
      <div class="card" :class="{ collapsed: openSection !== 'task' }">
        <div class="card-header card-toggle" role="button" tabindex="0" :aria-expanded="openSection === 'task'" @click="openSection = openSection === 'task' ? '' : 'task'" @keydown.enter="openSection = 'task'" @keydown.space.prevent="openSection = 'task'">
          <span class="card-heading"><span class="card-num mono">02</span><span class="card-title">{{ t('source.task') }}</span></span>
        </div>
        <div class="card-collapse" :class="{ collapsed: openSection !== 'task' }">
          <div class="card-body">
            <div class="form-group">
              <label class="form-label" for="source-wish">{{ t('source.wish') }}</label>
              <textarea id="source-wish" v-model="wish" rows="3" class="form-textarea" :maxlength="MAX_INSTRUCTION" :disabled="locked || submitting" data-testid="source-wish" :placeholder="t('source.placeholder')"></textarea>
            </div>
            <div class="form-group">
              <label class="form-label" for="source-count">{{ t('source.count') }}</label>
              <input id="source-count" v-model.number="count" type="number" :min="1" :max="MAX_VARIANTS" class="form-input" :disabled="locked || submitting" data-testid="source-count" />
            </div>
            <button type="button" class="btn btn-secondary" :disabled="busy || locked || !image || !countValid" data-testid="source-plan" @click="plan">{{ planning ? t('stylist.planning') : t('source.plan') }}</button>
          </div>
        </div>
      </div>
      <div class="card" :class="{ collapsed: openSection !== 'poses' }">
        <div class="card-header card-toggle" role="button" tabindex="0" :aria-expanded="openSection === 'poses'" @click="openSection = openSection === 'poses' ? '' : 'poses'" @keydown.enter="openSection = 'poses'" @keydown.space.prevent="openSection = 'poses'">
          <span class="card-heading"><span class="card-num mono">03</span><span class="card-title">{{ t('source.poses') }}<span v-if="instructions.length" class="source-count">{{ instructions.length }}</span></span></span>
        </div>
        <div class="card-collapse" :class="{ collapsed: openSection !== 'poses' }">
          <div class="card-body">
            <div v-if="!instructions.length" class="form-hint">{{ t('source.plan_first') }}</div>
            <div v-for="(_, index) in instructions" :key="index" class="form-group">
              <label class="form-label" :for="`source-pose-${index}`">{{ t('source.variant', { n: index + 1 }) }}</label>
              <textarea :id="`source-pose-${index}`" v-model="instructions[index]" rows="3" class="form-textarea" :maxlength="MAX_INSTRUCTION" :disabled="busy || locked" data-testid="source-instruction"></textarea>
              <button type="button" class="btn btn-sm btn-danger-quiet" :disabled="busy || locked" data-testid="source-remove" @click="remove(index)">{{ t('source.remove') }}</button>
            </div>
          </div>
        </div>
      </div>
      <div class="card" :class="{ collapsed: openSection !== 'editor' }">
        <div class="card-header card-toggle" role="button" tabindex="0" :aria-expanded="openSection === 'editor'" @click="openSection = openSection === 'editor' ? '' : 'editor'" @keydown.enter="openSection = 'editor'" @keydown.space.prevent="openSection = 'editor'">
          <span class="card-heading"><span class="card-num mono">04</span><span class="card-title">{{ t('source.editor') }}</span></span>
        </div>
        <div class="card-collapse" :class="{ collapsed: openSection !== 'editor' }"><div class="card-body"><EditorSettings id-prefix="source-editor" scope-key="stylist.edit_editor_scope" /></div></div>
      </div>
    </div>
    <div class="gen-actions" data-testid="source-actions">
      <div v-if="instructions.length && !validPlan" class="form-hint">{{ t('source.stale') }}</div>
      <div v-if="error" role="alert" class="status status-error" data-testid="source-error">{{ error }}</div>
      <div class="gen-actions-row">
        <button type="button" class="btn btn-primary btn-generate source-submit" :disabled="busy || locked || !validPlan || !editor.ready.value" data-testid="source-run" @click="run">{{ t('source.run') }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.source-variants { display: flex; flex-direction: column; flex: 1; min-height: 0; overflow: hidden; }
.source-photo-row { display: flex; align-items: center; flex-wrap: wrap; gap: var(--sp-2); }
.source-thumb { width: 48px; height: 48px; object-fit: cover; border-radius: var(--radius-sm); }
.source-count { margin-left: var(--sp-2); color: var(--text-dim); }
.source-submit { width: 100%; }
</style>
