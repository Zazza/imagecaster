<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { TriangleAlert, Shirt } from 'lucide-vue-next'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { people } from '../peopleStore.js'
import { canFaceSwap, trainBaseLabel } from '../people.js'
import { b64Url } from '../imageDataUrl.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { useStylistPlan, stylistPeople, ASPECTS, FRAMES_MAX, clampFrames, latestDone, dropChoiceId } from '../composables/useStylistPlan.js'
import StatusMessage from './StatusMessage.vue'
import GenProgress from './GenProgress.vue'
import ImageViewer from './ImageViewer.vue'
import AspectTab from './stylist/AspectTab.vue'
import SeriesCard from './stylist/SeriesCard.vue'

const {
  form, series, warnings, usedLLM, planning, regenerating, shooting, translating, error, jobIds, frames, view,
  frameCount, outfitLocked, makePlan, regenerate, deleteSeries, deleteShot, updateSeries, shoot, refreshFrames, savePreset, planEpoch,
} = useStylistPlan()
const { sdProgress } = useGenerationProgress()

const QUEUE_EVENTS = ['queue:changed', 'queue:started', 'queue:completed', 'queue:failed']

const base = ref(null)
const baseError = ref('')
const baseLoading = ref(true)
const peopleLoaded = ref(false)
const peopleError = ref('')
const tab = ref('outfit')
const openSection = ref('person')
const faceSwapStatus = ref('')
const faceSwapOn = computed(() => faceSwapStatus.value === 'ok')
const thumbs = ref({})
const shotNotice = ref('')
const pickedItemId = ref(0)
const bigImage = ref('')
const showViewer = ref(false)

const persons = computed(() => stylistPeople(people.value))
const person = computed(() => persons.value.find(p => p.name === form.person) || null)
const swapAvailable = computed(() => canFaceSwap(person.value, faceSwapOn.value))
const faceSwapNote = computed(() => {
  if (swapAvailable.value) return t('stylist.face_swap_on')
  if (!person.value?.identity) return t('stylist.face_swap_no_face')
  return faceSwapStatus.value === 'not_installed' ? t('stylist.face_swap_no_server') : t('stylist.face_swap_unknown')
})
const appearanceHint = computed(() => person.value?.gender === 'man' ? 'a man' : person.value?.gender === 'person' ? 'a person' : 'a woman')
const canPlan = computed(() => !!person.value && !!base.value && !planning.value && !shooting.value)
const planBlocked = computed(() => {
  if (baseLoading.value) return t('stylist.base_loading')
  if (!base.value) return t('stylist.base_failed')
  if (!person.value) return t('stylist.need_person')
  return ''
})
const garmentUnnamed = computed(() => !!form.garment && !(form.garmentOutfit || '').trim())
const canShoot = computed(() => !!person.value && frameCount.value > 0 && frameCount.value <= FRAMES_MAX && !shooting.value && !planning.value && regenerating.value < 0 && !garmentUnnamed.value)
const garmentMissed = (f) => !!f.garment && f.garment !== 'done'
const garmentReason = (f) => f.garment.replace(/^skipped:\s*/, '')
const runningIndex = computed(() => frames.value.findIndex(f => f.status === 'running'))
const runningFrame = computed(() => runningIndex.value >= 0 ? frames.value[runningIndex.value] : null)
const bigItemId = computed(() => {
  if (pickedItemId.value && frames.value.some(f => f.itemId === pickedItemId.value)) return pickedItemId.value
  return latestDone(frames.value)?.itemId || 0
})
const sectionSub = computed(() => ({
  person: person.value ? person.value.name : t('stylist.need_person'),
  task: t('stylist.sub_task', { n: form.frames }),
  look: outfitLocked.value ? t('stylist.sub_look_garment') : t('stylist.sub_look'),
}))

function setOpenSection(name) {
  openSection.value = name
}

function setView(v) {
  view.value = v
}

function dropWarning(i) {
  warnings.value = warnings.value.filter((_, j) => j !== i)
}

function clearError() {
  error.value = ''
}

const presetNotice = ref('')
const places = computed(() => [...new Set((base.value?.background || []).map(bg => bg.place).filter(Boolean))].sort())

async function reloadBase() {
  try {
    base.value = await api.stylistPresets()
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

async function onSavePreset(payload) {
  presetNotice.value = ''
  const id = await savePreset(payload)
  if (!id) return
  await reloadBase()
  presetNotice.value = t('stylist.preset_saved')
}

async function onRenamePreset(id, name) {
  try {
    await api.stylistRenamePreset(id, name)
  } catch (e) {
    error.value = String(e?.message || e)
    return
  }
  await reloadBase()
}

async function onDeletePreset(id) {
  try {
    await api.stylistDeletePreset(id)
  } catch (e) {
    error.value = String(e?.message || e)
    return
  }
  form.aspects = dropChoiceId(form.aspects, id)
  await reloadBase()
}

function itemsFor(aspect) {
  if (!base.value) return []
  return base.value[aspect] || []
}

async function loadAppearance(name) {
  form.appearance = ''
  if (!name) return
  try {
    const text = (await api.stylistAppearance(name)) || ''
    if (form.person === name) form.appearance = text
  } catch (e) {
    console.warn('stylistAppearance:', e)
  }
}

async function saveAppearance() {
  if (!form.person) return
  try {
    await api.setStylistAppearance(form.person, form.appearance)
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

watch(() => form.person, (name, old) => {
  if (name === old) return
  const p = persons.value.find(x => x.name === name)
  form.gender = p?.gender || ''
  loadAppearance(name)
})

function toggleTheme(id) {
  form.themes = form.themes.includes(id) ? form.themes.filter(x => x !== id) : [...form.themes, id]
}

const describing = ref(false)
const garmentNote = ref('')

async function describeGarment() {
  const photo = form.garment
  if (!photo || describing.value) return
  describing.value = true
  garmentNote.value = ''
  try {
    const names = await api.stylistDescribeGarment(photo)
    if (form.garment === photo && names) form.garmentOutfit = names
  } catch (e) {
    if (form.garment === photo) garmentNote.value = String(e?.message || e)
  } finally {
    describing.value = false
  }
}

async function pickGarment() {
  try {
    const b64 = await api.stylistPickGarment()
    if (!b64) return
    form.garment = b64
    form.garmentOutfit = ''
    await describeGarment()
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

async function onShoot() {
  shotNotice.value = ''
  const ids = await shoot(swapAvailable.value)
  if (ids.length) {
    pickedItemId.value = 0
    shotNotice.value = t('stylist.shot_queued', { n: ids.length })
  }
}

async function cancelRunning() {
  const f = runningFrame.value
  if (!f) return
  try {
    await api.cancelQueueJob(f.jobId)
  } catch (e) {
    error.value = String(e?.message || e)
  }
  refreshFrames()
}

function pickFrame(f) {
  if (f.status === 'completed' && f.itemId) pickedItemId.value = f.itemId
}

watch(frames, async (list) => {
  for (const f of list) {
    if (!f.itemId || thumbs.value[f.itemId]) continue
    try {
      const b64 = await api.getSessionThumb(f.itemId)
      if (b64) thumbs.value = { ...thumbs.value, [f.itemId]: b64Url(b64) }
    } catch (e) {
      console.warn('stylist thumb:', e)
    }
  }
}, { immediate: true })

watch(bigItemId, async (id) => {
  showViewer.value = false
  if (!id) {
    bigImage.value = ''
    return
  }
  try {
    const b64 = await api.getSessionImage(id)
    if (bigItemId.value === id) bigImage.value = b64 || ''
  } catch (e) {
    console.warn('stylist frame image:', e)
  }
}, { immediate: true })

let offQueue = []
onMounted(async () => {
  offQueue = QUEUE_EVENTS.map(ev => EventsOn(ev, refreshFrames))
  refreshFrames()
  try {
    base.value = await api.stylistPresets()
  } catch (e) {
    baseError.value = String(e?.message || e)
  } finally {
    baseLoading.value = false
  }
  try {
    people.value = (await api.listPeople()) || []
  } catch (e) {
    peopleError.value = t('stylist.people_error', { error: String(e?.message || e) })
  } finally {
    peopleLoaded.value = true
  }
  if (!form.person && persons.value.length) form.person = persons.value[0].name
  else if (form.person) loadAppearance(form.person)
  try {
    faceSwapStatus.value = (await api.faceSwapStatus()) || ''
  } catch (e) {
    console.warn('faceSwapStatus:', e)
  }
})

onUnmounted(() => {
  offQueue.forEach(off => off?.())
  offQueue = []
})
</script>

<template>
  <div class="page-enter gen-page" data-testid="stylist-page">
    <StatusMessage v-if="baseError" :text="baseError" @close="baseError = ''" />
    <StatusMessage v-if="error" data-testid="stylist-error" @close="clearError"><TriangleAlert :size="14" class="inline-icon" /> {{ error }}</StatusMessage>

    <div class="gen-workspace">
      <div class="gen-preview-col">
        <div class="stylist-switch">
          <button class="btn btn-sm" :class="view === 'plan' ? 'btn-primary' : 'btn-secondary'" :aria-pressed="view === 'plan'" data-testid="stylist-view-plan" @click="setView('plan')">{{ t('stylist.view_plan') }}<span v-if="series.length" class="stylist-count">{{ frameCount }}</span></button>
          <button class="btn btn-sm" :class="view === 'frames' ? 'btn-primary' : 'btn-secondary'" :aria-pressed="view === 'frames'" data-testid="stylist-view-frames" @click="setView('frames')">{{ t('stylist.view_frames') }}<span v-if="frames.length" class="stylist-count">{{ frames.length }}</span></button>
        </div>

        <template v-if="view === 'plan'">
          <StatusMessage v-for="(w, i) in warnings" :key="i" kind="warning" data-testid="stylist-warning" @close="dropWarning(i)">{{ w }}</StatusMessage>
          <StatusMessage v-if="presetNotice" kind="success" :timeout="8000" data-testid="stylist-preset-notice" @close="presetNotice = ''">{{ presetNotice }}</StatusMessage>
          <div v-if="series.length && usedLLM" class="form-hint">{{ t('stylist.used_llm') }}</div>
          <SeriesCard
            v-for="(s, i) in series" :key="i" :series="s" :index="i" :outfit-locked="outfitLocked" :busy="regenerating === i" :locked="shooting" :places="places" :epoch="planEpoch"
            @update="updateSeries(i, $event)" @regenerate="regenerate" @delete="deleteSeries" @delete-shot="deleteShot" @save-preset="onSavePreset"
          />
          <div v-if="!series.length" class="generate-image-area empty" data-testid="stylist-plan-empty">
            <div class="generate-placeholder">
              <div class="generate-placeholder-icon"><Shirt :size="56" :stroke-width="1.25" /></div>
              <div class="generate-placeholder-title">{{ t('stylist.empty_plan_title') }}</div>
              <div class="generate-placeholder-hint">{{ t('stylist.empty_plan_hint') }}</div>
            </div>
          </div>
        </template>

        <template v-else>
          <div v-if="runningFrame" class="card stylist-progress" data-testid="stylist-progress">
            <GenProgress :label="t('stylist.frame_of', { i: runningIndex + 1, n: frames.length })" :progress="sdProgress?.progress || 0" :steps="sdProgress?.steps || 0" :eta="sdProgress?.etaRelative || 0" stats @interrupt="cancelRunning" />
          </div>
          <div class="generate-image-area" :class="{ empty: !bigImage, loading: !bigImage && !!runningFrame }">
            <img v-if="bigImage" :src="b64Url(bigImage)" alt="" class="img-fade-in stylist-big" data-testid="stylist-big" @click="showViewer = true" />
            <div v-else class="generate-placeholder" data-testid="stylist-frames-empty">
              <div class="generate-placeholder-icon"><Shirt :size="56" :stroke-width="1.25" /></div>
              <div class="generate-placeholder-title">{{ frames.length ? t('stylist.empty_frames_wait') : t('stylist.empty_frames_title') }}</div>
              <div class="generate-placeholder-hint">{{ frames.length ? t('stylist.results_hint') : t('stylist.empty_frames_hint') }}</div>
            </div>
          </div>
          <div v-if="frames.length" class="card stylist-strip-card">
            <div class="card-title">{{ t('stylist.results') }}</div>
            <div class="form-hint">{{ t('stylist.results_hint') }}</div>
            <div class="stylist-strip" data-testid="stylist-results">
              <button
                v-for="(f, i) in frames" :key="f.jobId" type="button" class="stylist-thumb"
                :class="['is-' + f.status, { active: f.itemId && f.itemId === bigItemId, 'no-garment': garmentMissed(f) }]"
                :title="f.error || (garmentMissed(f) ? t('stylist.frame_no_garment_title', { reason: garmentReason(f) }) : f.shot)" data-testid="stylist-frame" @click="pickFrame(f)"
              >
                <img v-if="f.itemId && thumbs[f.itemId]" :src="thumbs[f.itemId]" alt="" />
                <span v-if="garmentMissed(f)" class="stylist-thumb-badge" data-testid="stylist-frame-no-garment">{{ t('stylist.frame_no_garment') }}</span>
                <span class="stylist-thumb-text" :class="{ 'on-image': f.itemId && thumbs[f.itemId] }" data-testid="stylist-frame-caption">{{ i + 1 }} · {{ t('stylist.frame_' + f.status) }}</span>
              </button>
            </div>
          </div>
        </template>
      </div>

      <div class="gen-controls">
        <div class="gen-sections accordion">
          <div class="card" :class="{ collapsed: openSection !== 'person' }" data-testid="stylist-section-person">
            <div class="card-header card-toggle" @click="setOpenSection('person')">
              <span class="card-heading"><span class="card-num mono">01</span><span class="card-heading-text"><span class="card-title">{{ t('stylist.section_person') }}</span><span class="card-subtitle">{{ sectionSub.person }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'person' }">
              <div class="card-body">
                <div v-if="!peopleLoaded" class="form-hint" data-testid="stylist-people-loading">{{ t('stylist.people_loading') }}</div>
                <div v-else-if="peopleError" class="status status-error" data-testid="stylist-people-error">{{ peopleError }}</div>
                <div v-else-if="!persons.length" class="form-hint" data-testid="stylist-no-people">{{ t('stylist.no_people') }}</div>
                <div class="form-group">
                  <label class="form-label">{{ t('stylist.person') }}</label>
                  <select class="form-select" v-model="form.person" data-testid="stylist-person">
                    <option v-for="p in persons" :key="p.name" :value="p.name">{{ p.name }} · {{ trainBaseLabel(p.base) }}</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">{{ t('stylist.appearance') }}</label>
                  <input class="form-input" type="text" v-model="form.appearance" maxlength="500" :placeholder="appearanceHint" data-testid="stylist-appearance" @change="saveAppearance" />
                  <div class="form-hint">{{ t('stylist.appearance_hint') }}</div>
                </div>
                <div v-if="person" class="form-hint" :class="{ 'stylist-face-off': !swapAvailable }" data-testid="stylist-face-swap">{{ faceSwapNote }}</div>
                <details class="stylist-more">
                  <summary>{{ t('stylist.more') }}</summary>
                  <label class="form-label">{{ t('stylist.seed') }}</label>
                  <input class="form-input stylist-narrow" type="text" v-model="form.seed" :placeholder="t('stylist.seed_hint')" />
                </details>
              </div>
            </div>
          </div>

          <div class="card" :class="{ collapsed: openSection !== 'task' }" data-testid="stylist-section-task">
            <div class="card-header card-toggle" @click="setOpenSection('task')">
              <span class="card-heading"><span class="card-num mono">02</span><span class="card-heading-text"><span class="card-title">{{ t('stylist.section_task') }}</span><span class="card-subtitle">{{ sectionSub.task }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'task' }">
              <div class="card-body">
                <div class="form-group">
                  <label class="form-label">{{ t('stylist.wish') }}</label>
                  <textarea class="form-textarea" v-model="form.wish" rows="3" maxlength="1000" :placeholder="t('stylist.wish_hint')" data-testid="stylist-wish"></textarea>
                  <div class="form-hint" data-testid="stylist-wish-help">{{ t('stylist.wish_help') }}</div>
                </div>
                <div class="form-group">
                  <label class="form-label">{{ t('stylist.themes') }}</label>
                  <div class="styles-chips" data-testid="stylist-themes">
                    <button v-for="th in base?.themes || []" :key="th.id" type="button" class="styles-chip" :class="{ active: form.themes.includes(th.id) }" :data-testid="'theme-' + th.id" @click="toggleTheme(th.id)">{{ th.name }}</button>
                  </div>
                </div>
                <div class="form-group">
                  <label class="form-label">{{ t('stylist.frames') }}</label>
                  <input class="form-input stylist-narrow" type="number" min="1" :max="FRAMES_MAX" :value="form.frames" data-testid="stylist-frames" @change="form.frames = clampFrames($event.target.value); $event.target.value = form.frames" />
                </div>
                <div class="form-group">
                  <label class="form-label">{{ t('stylist.garment') }}</label>
                  <div class="stylist-garment">
                    <img v-if="form.garment" :src="b64Url(form.garment)" class="stylist-garment-img" alt="" />
                    <button class="btn btn-secondary btn-sm" data-testid="stylist-garment-pick" @click="pickGarment">{{ form.garment ? t('stylist.garment_change') : t('stylist.garment_pick') }}</button>
                    <button v-if="form.garment" class="btn btn-danger-quiet btn-sm" data-testid="stylist-garment-clear" @click="form.garment = ''">{{ t('stylist.garment_clear') }}</button>
                  </div>
                  <div class="form-hint">{{ t('stylist.garment_hint') }}</div>
                </div>
                <div v-if="form.garment" class="form-group">
                  <label class="form-label">{{ t('stylist.garment_outfit') }}</label>
                  <div class="stylist-garment">
                    <input class="form-input" :class="{ 'is-invalid': garmentUnnamed && !describing }" type="text" v-model="form.garmentOutfit" maxlength="300" :disabled="describing" :placeholder="describing ? t('stylist.garment_describing') : t('stylist.garment_outfit_placeholder')" data-testid="stylist-garment-outfit" />
                    <button class="btn btn-secondary btn-sm" :disabled="describing" data-testid="stylist-garment-describe" @click="describeGarment">{{ describing ? t('stylist.garment_describing') : t('stylist.garment_describe') }}</button>
                  </div>
                  <div v-if="garmentNote" class="form-hint stylist-face-off" data-testid="stylist-garment-note">{{ garmentNote }}</div>
                  <div class="form-hint">{{ t('stylist.garment_outfit_hint') }}</div>
                </div>
              </div>
            </div>
          </div>

          <div class="card" :class="{ collapsed: openSection !== 'look' }" data-testid="stylist-section-look">
            <div class="card-header card-toggle" @click="setOpenSection('look')">
              <span class="card-heading"><span class="card-num mono">03</span><span class="card-heading-text"><span class="card-title">{{ t('stylist.section_look') }}</span><span class="card-subtitle">{{ sectionSub.look }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'look' }">
              <div class="card-body">
                <div class="tabs stylist-tabs">
                  <button v-for="a in ASPECTS" :key="a" class="tab" :class="{ active: tab === a }" :data-testid="'tab-' + a" @click="tab = a">{{ t('stylist.tab_' + a) }}</button>
                </div>
                <div v-if="tab === 'outfit' && outfitLocked" class="form-hint" data-testid="garment-outfit-tab">{{ t('stylist.garment_outfit_moved') }}</div>
                <AspectTab v-else :aspect="tab" :choice="form.aspects[tab]" :items="itemsFor(tab)" :themes="base?.themes || []" @update:choice="form.aspects[tab] = $event" @rename="onRenamePreset" @delete="onDeletePreset" />
              </div>
            </div>
          </div>
        </div>

        <div class="gen-actions">
          <div class="gen-actions-row">
            <button class="btn btn-primary btn-generate stylist-action" :disabled="!canPlan" data-testid="stylist-plan" @click="makePlan">{{ planning ? t('stylist.planning') : t('stylist.plan') }}</button>
            <button class="btn btn-primary btn-generate stylist-action" :disabled="!canShoot" data-testid="stylist-shoot" @click="onShoot">{{ translating ? t('stylist.translating') : shooting ? t('stylist.shooting') : t('stylist.shoot', { n: frameCount }) }}</button>
          </div>
          <div v-if="planBlocked" class="form-hint stylist-blocked" data-testid="stylist-plan-blocked">{{ planBlocked }}</div>
          <div v-else-if="frameCount > FRAMES_MAX" class="form-hint stylist-blocked">{{ t('stylist.too_many', { n: FRAMES_MAX }) }}</div>
          <div v-else-if="!series.length" class="form-hint stylist-blocked" data-testid="stylist-shoot-blocked">{{ t('stylist.need_plan') }}</div>
          <div v-else-if="garmentUnnamed" class="form-hint stylist-blocked" data-testid="stylist-garment-unnamed">{{ describing ? t('stylist.garment_describing') : t('stylist.need_garment_outfit') }}</div>
          <div v-if="shotNotice" class="form-hint stylist-blocked" data-testid="stylist-shot-notice">{{ shotNotice }}</div>
        </div>
      </div>
    </div>

    <ImageViewer v-if="showViewer && bigImage" :image-base64="bigImage" @close="showViewer = false" />
  </div>
</template>

<style scoped>
.stylist-switch { display: flex; gap: 8px; flex-shrink: 0; }
.stylist-count { margin-left: 6px; opacity: 0.7; }
.stylist-narrow { max-width: 120px; }
.stylist-garment { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.stylist-garment ~ .form-hint + .form-group { margin-top: 8px; }
.form-input.is-invalid { border-color: var(--warning, var(--danger)); }
.stylist-garment-img { width: 48px; height: 48px; object-fit: cover; border-radius: var(--radius-sm); }
.stylist-more { margin-top: 8px; }
.stylist-face-off { color: var(--warning, var(--text-dim)); }
.stylist-more summary { cursor: pointer; color: var(--text-dim); }
.stylist-tabs { margin-bottom: 8px; }
.stylist-action { flex: 1 1 0; }
.stylist-blocked { margin-top: 6px; }
.stylist-progress { flex-shrink: 0; }
.stylist-big { cursor: zoom-in; margin: auto; align-self: center; min-height: 0; }
.stylist-strip-card { flex-shrink: 0; display: flex; flex-direction: column; gap: 6px; }
.stylist-strip { display: flex; gap: 8px; overflow-x: auto; padding-bottom: 4px; }
.stylist-thumb { flex: 0 0 auto; width: 72px; height: 96px; padding: 0; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface-2); display: flex; align-items: center; justify-content: center; overflow: hidden; cursor: default; }
.stylist-thumb.is-completed { cursor: pointer; }
.stylist-thumb.is-running { border-color: var(--accent); }
.stylist-thumb.is-failed { border-color: var(--danger); }
.stylist-thumb.active { outline: 2px solid var(--accent); outline-offset: 1px; }
.stylist-thumb { position: relative; }
.stylist-thumb.no-garment { border-color: var(--warning, var(--danger)); }
.stylist-thumb-badge { position: absolute; left: 2px; right: 2px; top: 2px; padding: 1px 2px; border-radius: var(--radius-sm); background: rgba(0, 0, 0, 0.7); color: var(--warning, #f0b429); font-size: 10px; line-height: 1.3; text-align: center; }
.stylist-thumb-text.on-image { position: absolute; left: 0; right: 0; bottom: 0; padding: 1px 3px; background: rgba(0, 0, 0, 0.55); color: #fff; font-size: 10px; line-height: 1.4; text-align: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.stylist-thumb img { width: 100%; height: 100%; object-fit: cover; }
.stylist-thumb-text { font-size: var(--fs-xs); color: var(--text-dim); text-align: center; padding: 4px; }
</style>
