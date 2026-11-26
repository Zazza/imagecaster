<script setup>
import SourceVariantsPanel from './stylist/SourceVariantsPanel.vue'
import ResolutionUpscaleOverlay from './ResolutionUpscaleOverlay.vue'
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { TriangleAlert, Shirt, RotateCcw, Maximize2, SlidersHorizontal, Pencil } from 'lucide-vue-next'
import { openInHistoryViewer } from '../historyViewer.js'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { api } from '../api.js'
import { locale, t } from '../i18n/index.js'
import { people } from '../peopleStore.js'
import { canFaceSwap, trainBaseLabel, peopleGroups, groupBases, groupLoraForBase, pipelinePerson, pipelineOverride } from '../people.js'
import { usePresets } from '../composables/usePresets.js'
import { b64Url } from '../imageDataUrl.js'
import { readImageFile } from '../imageFile.js'
import { useGenerationProgress } from '../composables/useGenerationProgress.js'
import { providePostEditor } from '../composables/usePostEditor.js'
import EditorSettings from './post/SettingsPanel.vue'
import { useStylistPlan, sourceSubmitting, restoreLastShoot, restorePlan, savePlanSoon, reshootFrame, editFrame, restoreDetailsFrame, EDIT_INSTRUCTION_MAX, FACE_RESTORE_MIN, FACE_RESTORE_MAX, FACE_RESTORE_STEP, garmentOutfitShown, FRAME_SIZES, stylistPeople, ASPECTS, FRAMES_MAX, clampFrames, latestDone, dropChoiceId } from '../composables/useStylistPlan.js'
import StatusMessage from './StatusMessage.vue'
import FloatingGenerationPanel from './FloatingGenerationPanel.vue'
import AspectTab from './stylist/AspectTab.vue'
import PlanLibrary from './stylist/PlanLibrary.vue'
import SeriesCard from './stylist/SeriesCard.vue'
import DetailRestorePanel from './stylist/DetailRestorePanel.vue'
import PresetOverridesPanel from './PresetOverridesPanel.vue'
import { modelBaseFor } from '../fastMode.js'
import { isEffectType } from '../effectTypes.js'
import { parseLoras } from '../loraOverride.js'

const {
  form, series, warnings, usedLLM, planning, regenerating, shooting, translating, error, jobIds, frames, view,
  appliedSelectionCurrent, invalidateSelection,
  frameCount, frameCountMismatch, outfitLocked, makePlan, regenerate, deleteSeries, deleteShot, updateSeries, shoot, refreshFrames, savePreset, planEpoch,
} = useStylistPlan()
const { sdProgress, preview } = useGenerationProgress({ acceptJob: id => frames.value.some(frame => frame.jobId === id) })
const emit = defineEmits(['navigate'])
const sourceMode = ref(false)
const sourcePhoto = ref('')
const sourceBusy = ref(false)
const modeLocked = computed(() => sourceSubmitting.value || sourceBusy.value || shooting.value || planning.value || frames.value.some(f => ['pending', 'running'].includes(f.status)))
function sourceQueued(result) {
  pickedJobId.value = 0
  upscaled.value = null
  error.value = result.error || ''
}
const toPeople = () => emit('navigate', { page: 'people' })

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
const pickedJobId = ref(0)
const upscaled = ref(null)
const reshooting = ref(false)
const upscaling = ref(false)
const bigImage = ref('')
const editOpen = ref(false)
const editInstruction = ref('')
const editEditor = providePostEditor({ settingsPrefix: 'stylist_edit_editor', restoreLegacy: false })
const editing = ref(false)
const autoPickEditId = ref(0)
let selectionEpoch = 0
const canEdit = computed(() => !sourceSubmitting.value && selectedFrame.value?.status === 'completed' && !!selectedFrame.value?.itemId)
const canSubmitEdit = computed(() => canEdit.value && editEditor.ready.value && !!editInstruction.value.trim() && [...editInstruction.value.trim()].length <= EDIT_INSTRUCTION_MAX && !editing.value && !shooting.value)


const { presets, presetTypes, compoundPresets, loadPresets } = usePresets()
const modelCatalog = ref(null)
const persons = computed(() => stylistPeople(people.value))
const groups = computed(() => peopleGroups(persons.value))
const group = computed(() => groups.value.find(g => g.key === form.person) || null)
const stepModels = (c) => (c?.steps || []).map(s => presets.value.find(p => p.id === s.preset_id)?.model_name || '')
const TARGET_KINDS = ['base', 'preset', 'pipe']
const presetBase = (p) => modelBaseFor(modelCatalog.value, p?.model_name || '')
const targetsByKind = computed(() => {
  const out = { base: [], preset: [], pipe: [] }
  if (!group.value) return out
  const bases = groupBases(group.value)
  out.base = bases.map(b => ({ id: 'base:' + b, label: trainBaseLabel(b) }))
  if (Array.isArray(modelCatalog.value?.sd_models)) {
    for (const p of presets.value) {
      if (!isEffectType(p.preset_type) && bases.includes(presetBase(p))) out.preset.push({ id: 'preset:' + p.id, label: p.name, typeId: p.type_id || null })
    }
    for (const c of compoundPresets.value) {
      if (pipelinePerson(modelCatalog.value, stepModels(c), group.value).available) out.pipe.push({ id: 'pipe:' + c.id, label: c.name })
    }
  }
  return out
})
const targetKinds = computed(() => TARGET_KINDS.filter(k => targetsByKind.value[k].length))
const showTargetControls = computed(() => form.planningMode === 'manual' || form.targetLocked)
function onPlanningModeChange(event) {
  const nextMode = event.target.value
  if (form.planningMode !== nextMode) invalidateSelection()
  if (form.planningMode === 'manual' && nextMode !== 'manual') form.targetLocked = false
  form.planningMode = nextMode
}
const conceptText = (key) => locale.value === 'ru' ? form.shootConcept?.[key + '_ru'] || form.shootConcept?.[key] : form.shootConcept?.[key]
const presetGroups = computed(() => {
  const used = new Set(targetsByKind.value.preset.map(o => o.typeId).filter(Boolean))
  return presetTypes.value.filter(pt => !isEffectType(pt.name) && used.has(pt.id))
})
const targets = computed(() => {
  const list = targetsByKind.value[form.targetKind] || []
  if (form.targetKind !== 'preset' || !form.presetGroup) return list
  return list.filter(o => o.typeId === form.presetGroup)
})
const listsReady = ref(false)
watch([presetGroups, listsReady], ([list, ready]) => {
  if (ready && form.presetGroup && !list.some(g => g.id === form.presetGroup)) form.presetGroup = null
})
form.overrides = null
watch([targetKinds, listsReady], ([kinds, ready]) => {
  if (form.planningMode !== 'manual' && !form.targetLocked) return
  if (ready && kinds.length && !kinds.includes(form.targetKind)) form.targetKind = kinds[0]
})
const targetPreset = computed(() => (form.target.startsWith('preset:') ? presets.value.find(p => 'preset:' + p.id === form.target) || null : null))
const targetPresetLoras = computed(() => (targetPreset.value ? parseLoras(targetPreset.value.loras) : []))
const overridesPanel = ref(null)
watch(() => form.target, (now, before) => {
  if (now === before) return
  form.overrides = null
  overridesPanel.value?.reset()
})
const targetPipeline = computed(() => (form.target.startsWith('pipe:') ? compoundPresets.value.find(c => 'pipe:' + c.id === form.target) || null : null))
const person = computed(() => {
  if (!group.value) return null
  if (targetPipeline.value) {
    const ov = pipelineOverride(modelCatalog.value, stepModels(targetPipeline.value), group.value, true)
    return group.value.loras.find(p => p.name === (ov?.face_swap_name || ov?.name)) || group.value.loras[0]
  }
  if (targetPreset.value) return groupLoraForBase(group.value, presetBase(targetPreset.value)) || group.value.loras[0]
  return groupLoraForBase(group.value, form.target.replace(/^base:/, '')) || group.value.loras[0]
})
const pipelineMissing = computed(() => (targetPipeline.value && group.value ? pipelinePerson(modelCatalog.value, stepModels(targetPipeline.value), group.value).missing : []))
watch([targets, listsReady], ([list, ready]) => {
  if (form.planningMode !== 'manual' && !form.targetLocked) return
  if (ready && !list.some(x => x.id === form.target)) form.target = list[0]?.id || ''
})
const swapAvailable = computed(() => canFaceSwap(person.value, faceSwapOn.value))
const faceSwapNote = computed(() => {
  if (swapAvailable.value) return form.faceSwap ? t('stylist.face_swap_on') : t('stylist.face_swap_off')
  if (!person.value?.identity) return t('stylist.face_swap_no_face')
  return faceSwapStatus.value === 'not_installed' ? t('stylist.face_swap_no_server') : t('stylist.face_swap_unknown')
})
const appearanceHint = computed(() => person.value?.gender === 'man' ? 'a man' : person.value?.gender === 'person' ? 'a person' : 'a woman')
const canPlan = computed(() => !sourceSubmitting.value && !!person.value && !!base.value && !planning.value && !shooting.value)
const planBlocked = computed(() => {
  if (baseLoading.value) return t('stylist.base_loading')
  if (!base.value) return t('stylist.base_failed')
  if (!person.value) return t('stylist.need_person')
  return ''
})
const garmentUnnamed = computed(() => !!form.garment && !(form.garmentOutfit || '').trim())
const canShoot = computed(() => !sourceSubmitting.value && !!person.value && frameCount.value > 0 && frameCount.value <= FRAMES_MAX && !frameCountMismatch.value && appliedSelectionCurrent.value && !shooting.value && !planning.value && regenerating.value < 0 && !garmentUnnamed.value)
const garmentMissed = (f) => !!f.garment && f.garment !== 'done'
const garmentReason = (f) => f.garment.replace(/^skipped:\s*/, '')
const anatomyWarningCodes = new Set(['base_defect', 'garment_defect', 'uncertain', 'unavailable'])
const anatomyWarningText = (f) => t(`stylist.anatomy_${anatomyWarningCodes.has(f?.anatomyWarning) ? f.anatomyWarning : 'unavailable'}`)
const runningIndex = computed(() => frames.value.findIndex(f => f.status === 'running'))
const runningFrame = computed(() => runningIndex.value >= 0 ? frames.value[runningIndex.value] : null)
const liveImage = computed(() => {
  const frame = runningFrame.value
  if (!frame || sdProgress.value?.queue_job_id !== frame.jobId) return ''
  if (pickedJobId.value && pickedJobId.value !== frame.jobId) return ''
  return preview.value
})
const FINISHED = ['completed', 'failed', 'cancelled']
const selectedFrame = computed(() => {
  const f = pickedJobId.value ? frames.value.find(x => x.jobId === pickedJobId.value) : null
  if (f && FINISHED.includes(f.status)) return f
  return latestDone(frames.value)
})
const bigItemId = computed(() => {
  const f = selectedFrame.value
  if (!f || f.status !== 'completed') return 0
  if (upscaled.value && upscaled.value.fromJobId === f.jobId) return upscaled.value.itemId
  return f.itemId || 0
})
function onResolutionUpscaled(result) {
  if (result?.session_item_id && selectedFrame.value) upscaled.value = { fromJobId: selectedFrame.value.jobId, itemId: result.session_item_id }
}
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

async function onAddPreset(payload) {
  presetNotice.value = ''
  const id = await savePreset(payload)
  if (!id) return
  await reloadBase()
  const choice = form.aspects[payload.aspect]
  if (choice) form.aspects[payload.aspect] = { ...choice, mode: 'pick', ids: [...(choice.ids || []).filter(x => x !== id), id] }
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
    let text = (await api.stylistAppearance(name)) || ''
    for (const p of groups.value.find(g => g.key === name)?.loras || []) {
      if (text || p.name === name) continue
      text = (await api.stylistAppearance(p.name)) || ''
    }
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
  const g = groups.value.find(x => x.key === name)
  form.gender = g?.loras[0]?.gender || ''
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
    if (form.garment !== photo || !names) return
    let shown = names
    try {
      shown = await garmentOutfitShown(names)
    } catch (e) {
      garmentNote.value = t('stylist.garment_translate_ru_failed', { error: String(e?.message || e) })
    }
    if (form.garment === photo) form.garmentOutfit = shown
  } catch (e) {
    if (form.garment === photo) garmentNote.value = String(e?.message || e)
  } finally {
    describing.value = false
  }
}

async function setGarment(b64) {
  if (!b64) return
  form.garment = b64
  form.garmentOutfit = ''
  await describeGarment()
}

async function pickGarment() {
  try {
    await setGarment(await api.stylistPickGarment())
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

async function pasteGarment() {
  try {
    await setGarment(await api.readClipboardImage())
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

const garmentBox = ref(null)
const garmentDragOver = ref(false)

function garmentFromFile(file) {
  readImageFile(file)
    .then(setGarment)
    .catch(err => { error.value = String(err?.message || err) })
}

function onGarmentDrop(e) {
  garmentDragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  if (file) garmentFromFile(file)
}

function onGarmentPaste(e) {
  if (!garmentBox.value) return
  for (const item of e.clipboardData?.items || []) {
    if (!item.type?.startsWith('image/')) continue
    const file = item.getAsFile()
    if (!file) continue
    e.preventDefault()
    garmentFromFile(file)
    return
  }
}

async function onShoot() {
  selectionEpoch++
  autoPickEditId.value = 0
  shotNotice.value = ''
  const ids = await shoot(swapAvailable.value && form.faceSwap, person.value?.name)
  if (ids.length) {
    pickedJobId.value = 0
    upscaled.value = null
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
  if (f.status !== 'running' && !FINISHED.includes(f.status)) return
  selectionEpoch++
  autoPickEditId.value = 0
  pickedJobId.value = f.jobId
  upscaled.value = null
  editOpen.value = false
}

function toggleEdit() {
  if (!canEdit.value) return
  upscaled.value = null
  editOpen.value = !editOpen.value
}

async function onEdit() {
  const source = selectedFrame.value
  if (!canSubmitEdit.value || !source) return
  const epoch = selectionEpoch
  const instruction = editInstruction.value
  editing.value = true
  error.value = ''
  try {
    const id = await editFrame(source.jobId, instruction, editEditor.snapshot())
    if (id && epoch === selectionEpoch) {
      autoPickEditId.value = id
      editInstruction.value = ''
      shotNotice.value = t('stylist.edit_queued')
    }
  } catch (e) {
    error.value = String(e?.message || e)
  } finally {
    editing.value = false
  }
}

watch(() => selectedFrame.value?.jobId, () => {
  editInstruction.value = ''
  editOpen.value = false
})

watch([frames, autoPickEditId], ([list, id]) => {
  if (!id) return
  const frame = list.find(f => f.jobId === id)
  if (!frame || !FINISHED.includes(frame.status)) return
  pickedJobId.value = id
  upscaled.value = null
  autoPickEditId.value = 0
})

async function onReshoot() {
  const f = selectedFrame.value
  if (!f || reshooting.value) return
  selectionEpoch++
  autoPickEditId.value = 0
  reshooting.value = true
  error.value = ''
  try {
    await reshootFrame(f.jobId)
    pickedJobId.value = 0
    upscaled.value = null
  } catch (e) {
    error.value = String(e?.message || e)
  } finally {
    reshooting.value = false
  }
}

async function onUpscale() {
  const f = selectedFrame.value
  const itemId = bigItemId.value
  if (!f || !itemId || upscaling.value) return
  selectionEpoch++
  autoPickEditId.value = 0
  upscaling.value = true
  error.value = ''
  try {
    const res = await api.upscaleSessionItem(itemId, 0)
    if (!res?.image || !res.session_item_id) {
      error.value = t('generate.error_upscale_x2')
      return
    }
    upscaled.value = { fromJobId: f.jobId, itemId: res.session_item_id }
  } catch (e) {
    error.value = String(e?.message || e)
  } finally {
    upscaling.value = false
  }
}

async function toPostProcess() {
  if (!bigItemId.value) return
  try {
    await api.openInHistory(bigItemId.value)
    emit('navigate', { page: 'postprocess' })
  } catch (e) {
    error.value = String(e?.message || e)
  }
}

async function openBig() {
  await openInHistoryViewer(bigItemId.value)
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
  bigImage.value = ''
  if (!id) {
    return
  }
  try {
    const b64 = await api.getSessionImage(id)
    if (bigItemId.value === id) bigImage.value = b64 || ''
  } catch (e) {
    console.warn('stylist frame image:', e)
  }
}, { immediate: true })

watch([form, series], () => savePlanSoon(), { deep: true })

let offQueue = []
onMounted(async () => {
  document.addEventListener('paste', onGarmentPaste)
  offQueue = [
    ...QUEUE_EVENTS.map(ev => EventsOn(ev, refreshFrames)),
    EventsOn('session:removed', (d) => { if (!d?.moved_to) refreshFrames() }),
    EventsOn('session:deleted', refreshFrames),
    EventsOn('session:cleared', refreshFrames),
  ]
  await restorePlan()
  if (jobIds.value.length) refreshFrames()
  else restoreLastShoot()
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
  if (form.person && !groups.value.some(g => g.key === form.person)) form.person = persons.value.find(p => p.name === form.person) ? groups.value.find(g => g.loras.some(p => p.name === form.person))?.key || '' : ''
  if (!form.person && groups.value.length) form.person = groups.value[0].key
  else if (form.person) loadAppearance(form.person)
  const presetsLoaded = loadPresets()
  try {
    modelCatalog.value = await api.getModelCatalog()
  } catch (e) {
    console.warn('getModelCatalog:', e)
  }
  await presetsLoaded
  listsReady.value = true
  try {
    faceSwapStatus.value = (await api.faceSwapStatus()) || ''
  } catch (e) {
    console.warn('faceSwapStatus:', e)
  }
})

onUnmounted(() => {
  document.removeEventListener('paste', onGarmentPaste)
  offQueue.forEach(off => off?.())
  offQueue = []
})
</script>

<template>
  <div class="page-enter gen-page stylist-page" data-testid="stylist-page">
    <StatusMessage v-if="baseError" :text="baseError" @close="baseError = ''" />
    <StatusMessage v-if="error" data-testid="stylist-error" @close="clearError"><TriangleAlert :size="14" class="inline-icon" /> {{ error }}</StatusMessage>

    <div class="gen-workspace">
      <div class="gen-preview-col">
        <div class="stylist-switch">
          <button class="btn btn-sm" :class="view === 'plan' ? 'btn-primary' : 'btn-secondary'" :aria-pressed="view === 'plan'" data-testid="stylist-view-plan" @click="setView('plan')">{{ t('stylist.view_plan') }}<span v-if="series.length && !sourceMode" class="stylist-count">{{ frameCount }}</span></button>
          <button class="btn btn-sm" :class="view === 'frames' ? 'btn-primary' : 'btn-secondary'" :aria-pressed="view === 'frames'" data-testid="stylist-view-frames" @click="setView('frames')">{{ t('stylist.view_frames') }}<span v-if="frames.length" class="stylist-count">{{ frames.length }}</span></button>
          <PlanLibrary v-if="view === 'plan' && !sourceMode" :themes="base?.themes || []" :selected-themes="form.themes" :can-save="series.length > 0" :locked="shooting || planning || regenerating >= 0" />
        </div>

        <div v-if="view === 'plan' && sourceMode" class="generate-image-area" :class="{ empty: !sourcePhoto }">
          <img v-if="sourcePhoto" :src="b64Url(sourcePhoto)" :alt="t('source.photo')" class="stylist-big" />
          <p v-else class="form-hint">{{ t('source.upload_hint') }}</p>
        </div>
        <template v-else-if="view === 'plan'">
          <StatusMessage v-for="(w, i) in warnings" :key="i" kind="warning" data-testid="stylist-warning" @close="dropWarning(i)">{{ w }}</StatusMessage>
          <StatusMessage v-if="presetNotice" kind="success" :timeout="8000" data-testid="stylist-preset-notice" @close="presetNotice = ''">{{ presetNotice }}</StatusMessage>
          <div v-if="form.shootConcept && series.length" class="card" data-testid="stylist-applied-concept"><div class="card-body"><strong>{{ conceptText('concept') }}</strong><p>{{ conceptText('light') }} · {{ conceptText('focus') }}</p><p>{{ form.shootConcept.target_name || form.shootConcept.target }}<span v-if="form.shootConcept.target_model"> · {{ form.shootConcept.target_model }}</span></p><p v-if="conceptText('target_reason')">{{ conceptText('target_reason') }}</p><div class="form-hint">{{ t('stylist.auto_regenerate_hint') }}</div></div></div>
          <div v-if="series.length && usedLLM" class="form-hint">{{ t('stylist.used_llm') }}</div>
          <SeriesCard
            v-for="(s, i) in series" :key="i" :series="s" :index="i" :outfit-locked="outfitLocked" :busy="regenerating === i" :locked="shooting" :can-regenerate="!s.direction" :places="places" :epoch="planEpoch"
            @update="updateSeries(i, $event)" @regenerate="regenerate" @delete="deleteSeries" @delete-shot="deleteShot" @save-preset="onSavePreset"
          />
          <div v-if="series.length" class="stylist-shoot-bar">
            <button class="btn btn-primary btn-generate" :disabled="!canShoot" data-testid="stylist-shoot" @click="onShoot">{{ translating ? t('stylist.translating') : shooting ? t('stylist.shooting') : t('stylist.shoot', { n: frameCount }) }}</button>
            <div v-if="!appliedSelectionCurrent" class="form-hint stylist-blocked" data-testid="stylist-auto-apply-required">{{ t('stylist.auto_apply_required') }}</div>
            <div v-if="frameCountMismatch" class="form-hint stylist-blocked" data-testid="stylist-frame-count-mismatch">{{ t('stylist.frame_count_mismatch', { got: frameCount, want: clampFrames(form.frames) }) }}</div>
            <div v-if="frameCount > FRAMES_MAX" class="form-hint stylist-blocked">{{ t('stylist.too_many', { n: FRAMES_MAX }) }}</div>
            <div v-else-if="garmentUnnamed" class="form-hint stylist-blocked" data-testid="stylist-garment-unnamed">{{ describing ? t('stylist.garment_describing') : t('stylist.need_garment_outfit') }}</div>
          </div>
          <div v-if="!series.length" class="generate-image-area empty" data-testid="stylist-plan-empty">
            <div class="generate-placeholder">
              <div class="generate-placeholder-icon"><Shirt :size="56" :stroke-width="1.25" /></div>
              <div class="generate-placeholder-title">{{ t('stylist.empty_plan_title') }}</div>
              <div class="generate-placeholder-hint">{{ t('stylist.empty_plan_hint') }}</div>
            </div>
          </div>
        </template>

        <template v-else>
          <div class="generate-image-area" style="position: relative" :class="{ empty: !bigImage && !liveImage, loading: !!runningFrame }">
            <img v-if="liveImage" :src="liveImage" :alt="t('progress.preview')" class="img-fade-in stylist-big" data-testid="stylist-big" data-live="true" />
            <img v-else-if="bigImage && bigItemId" :src="b64Url(bigImage)" alt="" class="img-fade-in stylist-big" data-testid="stylist-big" @click="openBig" />
            <div v-else-if="selectedFrame && selectedFrame.status !== 'completed'" class="generate-placeholder" data-testid="stylist-frame-failed">
              <div class="generate-placeholder-icon"><TriangleAlert :size="56" :stroke-width="1.25" /></div>
              <div class="generate-placeholder-title">{{ t('stylist.frame_not_made') }}</div>
              <div class="generate-placeholder-hint">{{ selectedFrame.error || t('stylist.frame_' + selectedFrame.status) }}</div>
            </div>
            <div v-else class="generate-placeholder" data-testid="stylist-frames-empty">
              <div class="generate-placeholder-icon"><Shirt :size="56" :stroke-width="1.25" /></div>
              <div class="generate-placeholder-title">{{ frames.length ? t('stylist.empty_frames_wait') : t('stylist.empty_frames_title') }}</div>
              <div class="generate-placeholder-hint">{{ frames.length ? t('stylist.results_hint') : t('stylist.empty_frames_hint') }}</div>
            </div>
            <FloatingGenerationPanel
              :active="!!runningFrame" data-testid="stylist-progress"
              :label="t('stylist.frame_of', { i: runningIndex + 1, n: frames.length })"
              :progress="sdProgress?.progress || 0" :steps="sdProgress?.steps || 0" :eta="sdProgress?.etaRelative || 0"
              :preview-available="sdProgress?.preview_available" @interrupt="cancelRunning"
            />
            <ResolutionUpscaleOverlay v-if="!liveImage" :image="bigImage" :item-id="bigItemId" @completed="onResolutionUpscaled" />
          </div>
          <div v-if="selectedFrame && !liveImage" class="action-row gen-result-actions stylist-actions" data-testid="stylist-actions">
            <button class="btn btn-sm btn-secondary" :disabled="reshooting || sourceSubmitting" data-testid="stylist-reshoot" @click="onReshoot"><RotateCcw :size="14" /> {{ t('stylist.reshoot') }}</button>
            <button v-if="bigItemId" class="btn btn-sm btn-secondary" :disabled="upscaling" data-testid="stylist-upscale" @click="onUpscale">
              <span v-if="upscaling" class="spinner gen-action-spinner"></span>
              <Maximize2 v-else :size="14" />
              {{ t('generate.btn_upscale_x2') }}
            </button>
            <button v-if="canEdit" class="btn btn-sm btn-secondary" :aria-expanded="editOpen" aria-controls="stylist-edit-panel" data-testid="stylist-edit-toggle" @click="toggleEdit"><Pencil :size="14" /> {{ t('stylist.edit_frame') }}</button>
            <button v-if="bigItemId" class="btn btn-sm btn-primary" data-testid="stylist-to-post" @click="toPostProcess"><SlidersHorizontal :size="14" /> {{ t('post.btn_send') }}</button>
          </div>
          <div v-if="editOpen && canEdit" id="stylist-edit-panel" class="card stylist-edit-panel" data-testid="stylist-edit-panel">
            <EditorSettings id-prefix="stylist-editor" scope-key="stylist.edit_editor_scope" />
            <label class="form-label" for="stylist-edit-instruction">{{ t('stylist.edit_question') }}</label>
            <textarea id="stylist-edit-instruction" v-model="editInstruction" class="form-textarea" rows="3" :maxlength="EDIT_INSTRUCTION_MAX" :disabled="editing" :placeholder="t('stylist.edit_placeholder')" data-testid="stylist-edit-instruction"></textarea>
            <div class="form-hint">{{ t('stylist.edit_hint') }}</div>
            <button class="btn btn-primary" :disabled="!canSubmitEdit" data-testid="stylist-edit-submit" @click="onEdit">{{ editing ? t('stylist.edit_submitting') : t('stylist.edit_create') }}</button>
          </div>
          <div v-if="selectedFrame?.faceSwap?.startsWith('skipped:')" class="form-hint" data-testid="stylist-face-swap-skipped">{{ t('stylist.edit_face_skipped', { reason: selectedFrame.faceSwap.slice('skipped:'.length) }) }}</div>
          <div v-if="selectedFrame?.facePass?.startsWith('skipped:')" class="form-hint" data-testid="stylist-face-pass-skipped">{{ t('stylist.face_pass_skipped', { reason: selectedFrame.facePass.slice('skipped:'.length) }) }}</div>
          <div v-if="selectedFrame?.anatomyWarning" class="form-hint" data-testid="stylist-anatomy-warning">{{ anatomyWarningText(selectedFrame) }}</div>
          <details v-if="canEdit && bigImage && !liveImage" class="card stylist-more" data-testid="stylist-detail-panel">
            <summary>{{ t('stylist.detail_title') }}</summary>
            <DetailRestorePanel :frame="selectedFrame" :target="bigImage" :restore="restoreDetailsFrame" />
          </details>
          <div v-if="frames.length" class="card stylist-strip-card">
            <div class="card-title">{{ t('stylist.results') }}</div>
            <div class="form-hint">{{ t('stylist.results_hint') }}</div>
            <div class="stylist-strip" data-testid="stylist-results">
              <button
                v-for="(f, i) in frames" :key="f.jobId" type="button" class="stylist-thumb"
                :class="['is-' + f.status, { active: f.jobId === (pickedJobId || (liveImage ? runningFrame?.jobId : selectedFrame?.jobId)), 'no-garment': garmentMissed(f) }]"
                :title="f.error || (garmentMissed(f) ? t('stylist.frame_no_garment_title', { reason: garmentReason(f) }) : f.shot)" data-testid="stylist-frame" @click="pickFrame(f)"
              >
                <img v-if="f.itemId && thumbs[f.itemId]" :src="thumbs[f.itemId]" alt="" />
                <span v-if="garmentMissed(f)" class="stylist-thumb-badge" data-testid="stylist-frame-no-garment">{{ t('stylist.frame_no_garment') }}</span>
                <span v-if="f.anatomyWarning" class="stylist-thumb-badge stylist-anatomy-badge" :title="anatomyWarningText(f)" data-testid="stylist-anatomy-badge">{{ t('stylist.anatomy_badge') }}</span>
                <span class="stylist-thumb-text" :class="{ 'on-image': f.itemId && thumbs[f.itemId] }" data-testid="stylist-frame-caption">{{ i + 1 }} · {{ t('stylist.frame_' + f.status) }}</span>
              </button>
            </div>
          </div>
        </template>
      </div>

      <div class="gen-controls">
        <div class="stylist-switch stylist-mode" data-testid="stylist-work-mode">
          <button class="btn btn-sm" :class="!sourceMode ? 'btn-primary' : 'btn-secondary'" :disabled="modeLocked" :aria-pressed="!sourceMode" @click="sourceMode = false">{{ t('source.new_shoot') }}</button>
          <button class="btn btn-sm" :class="sourceMode ? 'btn-primary' : 'btn-secondary'" :disabled="modeLocked" :aria-pressed="sourceMode" data-testid="stylist-source-mode" @click="sourceMode = true">{{ t('source.mode') }}</button>
        </div>
        <p v-if="sourceSubmitting" class="form-hint" role="status">{{ t('source.submitting') }}</p>
        <SourceVariantsPanel v-show="sourceMode" :face-swap-on="faceSwapOn" :locked="shooting || frames.some(f => ['pending', 'running'].includes(f.status))" @source="sourcePhoto = $event" @busy="sourceBusy = $event" @queued="sourceQueued" />
        <div v-show="!sourceMode" class="gen-sections accordion">
          <div class="card" :class="{ collapsed: openSection !== 'person' }" data-testid="stylist-section-person">
            <div class="card-header card-toggle" @click="setOpenSection('person')">
              <span class="card-heading"><span class="card-num mono">01</span><span class="card-heading-text"><span class="card-title">{{ t('stylist.section_person') }}</span><span class="card-subtitle">{{ sectionSub.person }}</span></span></span>
            </div>
            <div class="card-collapse" :class="{ collapsed: openSection !== 'person' }">
              <div class="card-body">
                <div v-if="!peopleLoaded" class="form-hint" data-testid="stylist-people-loading">{{ t('stylist.people_loading') }}</div>
                <div v-else-if="peopleError" class="status status-error" data-testid="stylist-people-error">{{ peopleError }}</div>
                <div v-else-if="!persons.length" class="status status-warning stylist-no-people" data-testid="stylist-no-people">
                  <span>{{ t('stylist.no_people') }}</span>
                  <button type="button" class="btn btn-sm btn-primary" data-testid="stylist-no-people-go" @click="toPeople">{{ t('stylist.go_people') }}</button>
                </div>
                <div v-if="persons.length" class="form-group">
                  <div class="stylist-label-row">
                    <label class="form-label">{{ t('stylist.person') }}</label>
                    <button type="button" class="btn btn-secondary btn-xs" data-testid="stylist-add-person" @click="toPeople">{{ t('stylist.add_person') }}</button>
                  </div>
                  <select class="form-select" v-model="form.person" data-testid="stylist-person">
                    <option v-for="g in groups" :key="g.key" :value="g.key">{{ g.key }}</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">{{ t('stylist.appearance') }}</label>
                  <input class="form-input" type="text" v-model="form.appearance" maxlength="500" :placeholder="appearanceHint" data-testid="stylist-appearance" @change="saveAppearance" />
                  <div class="form-hint">{{ t('stylist.appearance_hint') }}</div>
                </div>
                <label v-if="person && swapAvailable" class="stylist-face-toggle">
                  <input type="checkbox" v-model="form.faceSwap" data-testid="stylist-face-swap-toggle" />
                  {{ t('people.face_swap') }}
                </label>
                <div v-if="person" class="form-hint" :class="{ 'stylist-face-off': !swapAvailable || !form.faceSwap }" data-testid="stylist-face-swap">{{ faceSwapNote }}</div>
                <details class="stylist-more">
                  <summary>{{ t('stylist.more') }}</summary>
                  <label class="stylist-face-toggle">
                    <input type="checkbox" v-model="form.facePass" :disabled="form.targetKind === 'pipe'" data-testid="stylist-face-pass-toggle" />
                    {{ t('stylist.face_pass_option') }}
                  </label>
                  <div class="form-hint">{{ t(form.targetKind === 'pipe' ? 'stylist.face_pass_pipeline' : 'stylist.face_pass_hint') }}</div>
                  <label class="form-label">{{ t('stylist.face_restore_label') }} · {{ form.faceRestoreBlend }}</label>
                  <input type="range" v-model.number="form.faceRestoreBlend" :min="FACE_RESTORE_MIN" :max="FACE_RESTORE_MAX" :step="FACE_RESTORE_STEP" :disabled="!swapAvailable || !form.faceSwap" data-testid="stylist-face-restore-blend" />
                  <div class="form-hint">{{ t('stylist.face_restore_hint') }}</div>
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
                  <label class="form-label" for="stylist-planning-mode">{{ t('stylist.planning_mode') }}</label>
                  <select id="stylist-planning-mode" class="form-select" :value="form.planningMode" @change="onPlanningModeChange" :disabled="planning || shooting" data-testid="stylist-planning-mode">
                    <option value="manual">{{ t('stylist.planning_manual') }}</option>
                    <option value="auto">{{ t('stylist.planning_auto') }}</option>
                    <option value="mixed">{{ t('stylist.planning_mixed') }}</option>
                  </select>
                  <div class="form-hint">{{ t('stylist.planning_' + form.planningMode + '_hint') }}</div>
                  <label v-if="form.planningMode !== 'manual'" class="form-label"><input type="checkbox" v-model="form.targetLocked" data-testid="stylist-target-locked" /> {{ t('stylist.auto_lock_target') }}</label>
                </div>
                <div v-if="showTargetControls && targetKinds.length" class="form-group">
                  <label class="form-label">{{ t('stylist.target') }}</label>
                  <div class="stylist-target-kinds" role="group">
                    <button v-for="k in TARGET_KINDS" :key="k" type="button" class="btn btn-sm" :class="form.targetKind === k ? 'btn-primary' : 'btn-secondary'" :aria-pressed="form.targetKind === k ? 'true' : 'false'" :disabled="!targetsByKind[k].length" :title="targetsByKind[k].length ? '' : t('stylist.kind_none_' + k)" :data-testid="'stylist-kind-' + k" @click="form.targetKind = k">{{ t('stylist.kind_' + k) }}</button>
                  </div>
                  <select v-if="form.targetKind === 'preset' && presetGroups.length" class="form-select stylist-preset-group" v-model="form.presetGroup" data-testid="stylist-preset-group">
                    <option :value="null">{{ t('generate.all_types') }}</option>
                    <option v-for="g in presetGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
                  </select>
                  <select v-if="targets.length" class="form-select" v-model="form.target" data-testid="stylist-target">
                    <option v-for="o in targets" :key="o.id" :value="o.id">{{ o.label }}</option>
                  </select>
                  <div v-if="pipelineMissing.length" class="form-hint" data-testid="stylist-target-missing">{{ t('people.pipeline_missing', { steps: pipelineMissing.join(', ') }) }}</div>
                </div>
                <div v-if="!showTargetControls" class="form-hint" data-testid="stylist-auto-target-hint">{{ t('stylist.auto_target_hint') }}</div>
                <details class="stylist-more">
                  <summary>{{ t('stylist.more') }}</summary>
                  <label class="form-label">{{ t('stylist.frame_size') }}</label>
                  <select class="form-select stylist-narrow" v-model="form.frameSize" data-testid="stylist-frame-size">
                    <option v-for="s in FRAME_SIZES" :key="s" :value="s">{{ t('stylist.frame_size_' + s) }}</option>
                  </select>
                  <div class="form-hint">{{ t('stylist.frame_size_hint') }}</div>
                  <label class="form-label">{{ t('stylist.seed') }}</label>
                  <input class="form-input stylist-narrow" type="text" v-model="form.seed" :placeholder="t('stylist.seed_hint')" />
                  <PresetOverridesPanel v-if="showTargetControls && form.targetKind !== 'pipe'" ref="overridesPanel" v-model="form.overrides" :preset-loras="targetPresetLoras" hide-model :title="t('stylist.overrides')" data-testid="stylist-overrides" />
                  <div v-else-if="showTargetControls" class="form-hint" data-testid="stylist-overrides-pipe">{{ t('stylist.overrides_pipe') }}</div>
                </details>
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
                  <input class="form-input stylist-narrow" type="number" min="1" :max="FRAMES_MAX" v-model.number="form.frames" data-testid="stylist-frames" @change="form.frames = clampFrames($event.target.value); $event.target.value = form.frames" />
                </div>
                <div ref="garmentBox" class="form-group stylist-garment-drop" :class="{ 'drop-zone-active': garmentDragOver }" data-testid="stylist-garment-drop" @dragover.prevent="garmentDragOver = true" @dragleave="garmentDragOver = false" @drop.prevent="onGarmentDrop">
                  <label class="form-label">{{ t('stylist.garment') }}</label>
                  <div class="stylist-garment">
                    <img v-if="form.garment" :src="b64Url(form.garment)" class="stylist-garment-img" alt="" />
                    <button class="btn btn-secondary btn-sm" data-testid="stylist-garment-pick" @click="pickGarment">{{ form.garment ? t('stylist.garment_change') : t('stylist.garment_pick') }}</button>
                    <button class="btn btn-secondary btn-sm" data-testid="stylist-garment-paste" @click="pasteGarment">{{ t('stylist.garment_paste') }}</button>
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
                <div v-if="form.planningMode === 'auto'" id="stylist-auto-look-hint" class="form-hint" data-testid="stylist-auto-look-hint">{{ t('stylist.auto_look_disabled') }}</div>
                <fieldset class="stylist-look-controls" :disabled="form.planningMode === 'auto'" :aria-label="t('stylist.section_look')" :aria-describedby="form.planningMode === 'auto' ? 'stylist-auto-look-hint' : undefined" data-testid="stylist-look-controls">
                  <div class="tabs stylist-tabs">
                    <button v-for="a in ASPECTS" :key="a" class="tab" :class="{ active: tab === a }" :data-testid="'tab-' + a" @click="tab = a">{{ t('stylist.tab_' + a) }}</button>
                  </div>
                  <div v-if="tab === 'outfit' && outfitLocked" class="form-hint" data-testid="garment-outfit-tab">{{ t('stylist.garment_outfit_moved') }}</div>
                  <AspectTab v-else :aspect="tab" :choice="form.aspects[tab]" :items="itemsFor(tab)" :themes="base?.themes || []" @update:choice="form.aspects[tab] = $event" @rename="onRenamePreset" @delete="onDeletePreset" :places="places" @save-preset="onAddPreset" />
                </fieldset>
              </div>
            </div>
          </div>
        </div>

        <div v-if="!sourceMode" class="gen-actions">
          <div class="gen-actions-row">
            <button class="btn btn-primary btn-generate stylist-action" :disabled="!canPlan" data-testid="stylist-plan" @click="makePlan">{{ planning ? t('stylist.planning') : form.planningMode === 'manual' ? t('stylist.plan') : t('stylist.auto_suggest') }}</button>
          </div>
          <div v-if="planBlocked" class="form-hint stylist-blocked" data-testid="stylist-plan-blocked">{{ planBlocked }}</div>
          <div v-if="shotNotice" class="form-hint stylist-blocked" data-testid="stylist-shot-notice">{{ shotNotice }}</div>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
.stylist-page { display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
.stylist-page > .gen-workspace { flex: 1; height: auto; min-height: 0; }
.stylist-page .gen-preview-col .generate-image-area { min-height: 12rem; }
@media (min-width: 900px) {
  .stylist-page > .gen-workspace { flex-direction: row; }
  .stylist-page .gen-controls { width: 360px; }
}
@media (max-width: 899px) {
  .stylist-page { overflow-y: auto; }
  .stylist-page > .gen-workspace { flex: none; min-height: 100%; }
  .stylist-page .gen-preview-col { flex: none; height: 40vh; min-height: 16rem; }
  .stylist-page .gen-controls { flex: none; height: 70vh; min-height: 20rem; }
}

.stylist-switch { display: flex; gap: 8px; flex-shrink: 0; align-items: center; }
.stylist-mode { padding: var(--sp-2) var(--sp-3); border-bottom: 1px solid var(--border); flex-wrap: wrap; }
.stylist-shoot-bar { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
.stylist-count { margin-left: 6px; opacity: 0.7; }
.stylist-narrow { max-width: 120px; }
.stylist-garment-drop { border: 1px dashed transparent; border-radius: var(--radius-sm); }
.stylist-garment { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.stylist-garment ~ .form-hint + .form-group { margin-top: 8px; }
.form-input.is-invalid { border-color: var(--warning, var(--danger)); }
.stylist-garment-img { width: 48px; height: 48px; object-fit: cover; border-radius: var(--radius-sm); }
.stylist-more { margin-top: 8px; }
.stylist-no-people { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
.stylist-label-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 4px; }
.stylist-label-row .form-label { margin-bottom: 0; }
.stylist-preset-group { margin-bottom: 6px; }
.stylist-face-toggle { display: flex; align-items: center; gap: 6px; margin-top: 8px; cursor: pointer; }
.stylist-target-kinds { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 6px; }
.stylist-face-off { color: var(--warning, var(--text-dim)); }
.stylist-more summary { cursor: pointer; color: var(--text-dim); }
.stylist-tabs { margin-bottom: 8px; }
.stylist-action { flex: 1 1 0; }
.stylist-blocked { margin-top: 6px; }
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
.stylist-look-controls { border: 0; padding: 0; margin: 0; min-width: 0; }
.stylist-look-controls:disabled { opacity: 0.5; }
.stylist-anatomy-badge { top: auto; bottom: 24px; }
.stylist-thumb-text.on-image { position: absolute; left: 0; right: 0; bottom: 0; padding: 1px 3px; background: rgba(0, 0, 0, 0.55); color: #fff; font-size: 10px; line-height: 1.4; text-align: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.stylist-thumb img { width: 100%; height: 100%; object-fit: cover; }
.stylist-thumb-text { font-size: var(--fs-xs); color: var(--text-dim); text-align: center; padding: 4px; }

.stylist-edit-panel { display: grid; gap: 10px; }
.stylist-edit-panel .btn { justify-self: start; }
</style>
