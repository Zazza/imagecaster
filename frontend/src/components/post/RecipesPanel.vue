<script setup>
import { ref, watch, onUnmounted } from 'vue'
import { X } from 'lucide-vue-next'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'
import { adjustObjectActive, retouchObjectActive, splitRecipeAdjust } from '../../composables/usePostAdjust.js'
import { jobImage } from '../../composables/usePostBatch.js'

const props = defineProps({
  source: { type: String, default: '' },
  batch: { type: Object, required: true },
  post: { type: Object, required: true },
  appliedEffects: { type: Array, required: true },
  status: { type: Object, required: true },
  blocked: { type: Boolean, default: false },
})

const emit = defineEmits(['busy', 'result', 'open'])

const { batchActive, batchBusy } = props.batch
const { error, successMsg } = props.status

const recipes = ref([])
const recipeRunning = ref(false)
const recipeStepIdx = ref(0)
const recipeStepTotal = ref(0)
const recipeCurrentId = ref(null)
const showRecipeForm = ref(false)
const recipeName = ref('')
const pendingDeleteRecipeId = ref(null)
let recipeDeleteTimer = null
let recipeCancelled = false

function setRecipeRunning(v) {
  recipeRunning.value = v
  emit('busy', v)
}

function recipeSteps(r) {
  try {
    const steps = JSON.parse(r.steps || '[]')
    return Array.isArray(steps) ? steps : []
  } catch (e) {
    console.warn('PostProcessPage.recipeSteps:', e)
    return []
  }
}

function recipeAdjustActive(r) {
  let a = null
  try { a = JSON.parse(r.adjust || '{}') } catch (e) { console.warn('PostProcessPage.recipeAdjustActive:', e); return false }
  return adjustObjectActive(a) || retouchObjectActive(a?.retouch) || !!a?.upscale_4k
}

function recipeMeta(r) {
  const n = recipeSteps(r).length
  if (n === 0) return t('post.recipe_steps_only_adjust')
  return t('post.recipe_steps', { n }) + (recipeAdjustActive(r) ? ' ' + t('post.recipe_steps_adjust') : '')
}

async function loadRecipes() {
  try {
    recipes.value = await api.listPostRecipes() || []
  } catch (e) {
    console.warn('PostProcessPage.loadRecipes:', e)
    recipes.value = []
  }
}

function openRecipeForm() {
  recipeName.value = ''
  showRecipeForm.value = true
  emit('open')
}

async function saveRecipe() {
  try {
    await api.savePostRecipe({
      name: recipeName.value,
      steps: JSON.stringify(props.appliedEffects.map(e => e.preset_id
        ? { preset_id: e.preset_id, strength: e.strength }
        : { prompt: e.prompt || '', strength: e.strength })),
      adjust: JSON.stringify({ ...props.post.adjustParams(), ...props.post.recipeExtras() }),
    })
    recipeName.value = ''
    showRecipeForm.value = false
    await loadRecipes()
  } catch (e) {
    error.value = String(e)
  }
}

async function deleteRecipe(id) {
  if (recipeRunning.value) return
  if (pendingDeleteRecipeId.value === id) {
    clearTimeout(recipeDeleteTimer)
    pendingDeleteRecipeId.value = null
    try {
      await api.deletePostRecipe(id)
      await loadRecipes()
    } catch (e) {
      error.value = String(e)
    }
  } else {
    clearTimeout(recipeDeleteTimer)
    pendingDeleteRecipeId.value = id
    recipeDeleteTimer = setTimeout(() => { pendingDeleteRecipeId.value = null }, 3000)
  }
}

function recipeJobParams(presetId, strength, b64) {
  return {
    image_base64: b64,
    mode: 'img2img',
    gen_mode: 'preset',
    preset_id: presetId,
    compound_preset_id: 0,
    denoising_strength: strength,
    tags: '',
    extra_negative_prompt: '',
    resolution_id: null,
    hires_profile_id: null,
    overrides: null,
    preserve_identity: true,
    seed: null,
  }
}

function enqueueRecipeStep(step, img) {
  if (step.prompt && !step.preset_id) {
    const strength = typeof step.strength === 'number' ? step.strength : 0.5
    return api.enqueueLook({ image_base64: img, text: step.prompt, strength, seed: null })
  }
  return api.enqueueFromImage(recipeJobParams(step.preset_id, step.strength, img))
}

async function runRecipeChain(img, steps) {
  const first = await props.batch.trackBatchJob(enqueueRecipeStep(steps[0], img))
  if (!first) return ''
  return async () => {
    let cur = await first.wait
    for (let i = 1; i < steps.length && cur; i++) {
      if (props.batch.isCancelled()) return ''
      const job = await props.batch.trackBatchJob(enqueueRecipeStep(steps[i], cur))
      if (!job) return ''
      cur = await job.wait
    }
    return cur || ''
  }
}

async function applyRecipe(r) {
  if (recipeRunning.value || props.blocked || batchBusy.value) return
  const steps = recipeSteps(r)
  let parsed = {}
  try {
    parsed = JSON.parse(r.adjust || '{}') || {}
  } catch (e) {
    console.warn('PostProcessPage.applyRecipe:', e)
  }
  const { adjust: adjustObj, extras } = splitRecipeAdjust(parsed)
  const adjust = props.post.adjust
  const applyAdjust = () => {
    Object.assign(adjust, adjustObj)
    props.post.applyRecipeExtras(extras)
  }
  if (steps.length === 0) {
    applyAdjust()
    return
  }
  if (batchActive.value) {
    await props.batch.runBatchStep(b64 => runRecipeChain(b64, steps), (res) => {
      if (!res.cancelled) applyAdjust()
    })
    return
  }
  if (!props.source) {
    error.value = t('export.error_no_image')
    return
  }
  setRecipeRunning(true)
  recipeCancelled = false
  recipeCurrentId.value = r.id
  recipeStepTotal.value = steps.length
  recipeStepIdx.value = 0
  error.value = ''
  successMsg.value = ''
  let img = props.source
  let aborted = 0
  let abortReason = ''
  try {
    for (let i = 0; i < steps.length; i++) {
      if (recipeCancelled) break
      recipeStepIdx.value = i + 1
      const step = steps[i]
      if (step.prompt && !step.preset_id) {
        const stepStrength = typeof step.strength === 'number' ? step.strength : 0.5
        let res = null
        try {
          res = await api.applyLookDirect(img, step.prompt, stepStrength, null)
        } catch (e) {
          abortReason = String(e)
        }
        if (!res?.image) {
          aborted = i + 1
          break
        }
        emit('result', res.image, { prompt: res.effective_prompt || '', strength: stepStrength })
        img = res.image
        continue
      }
      let jobId = 0
      try {
        jobId = await api.enqueueFromImage(recipeJobParams(step.preset_id, step.strength, img))
      } catch (e) {
        abortReason = String(e)
      }
      if (!jobId) {
        aborted = i + 1
        break
      }
      if (recipeCancelled) {
        try {
          await api.cancelQueueJob(jobId)
        } catch (e) {
          console.error('cancel queue job error:', e)
        }
        break
      }
      const outcome = await props.batch.waitForChainJob(jobId)
      if (recipeCancelled) break
      const res = await jobImage(outcome)
      if (!res) {
        aborted = i + 1
        abortReason = outcome.error || t(outcome.status === 'cancelled' ? 'post.recipe_reason_cancelled' : 'post.recipe_reason_no_image')
        break
      }
      emit('result', res, { preset_id: step.preset_id, strength: step.strength })
      img = res
    }
  } finally {
    if (aborted) error.value = t('post.recipe_aborted', { i: aborted, reason: abortReason || t('post.recipe_reason_no_image') })
    else if (!recipeCancelled) applyAdjust()
    setRecipeRunning(false)
    recipeCurrentId.value = null
  }
}

async function interruptRecipe() {
  recipeCancelled = true
  await props.batch.dropChainJobs(true)
}

onUnmounted(() => {
  recipeCancelled = true
})

defineExpose({ applyRecipe, load: loadRecipes })
</script>

<template>
  <div class="adj-actions">
    <button class="btn btn-sm btn-secondary" :disabled="recipeRunning" @click="openRecipeForm">{{ t('post.recipe_save') }}</button>
  </div>
  <div class="preset-list">
    <div v-if="!recipes.length" class="preset-empty">{{ t('post.recipes_empty') }}</div>
    <div
      v-for="r in recipes"
      :key="r.id"
      class="preset-item"
      :class="{ active: recipeCurrentId === r.id }"
      @click="applyRecipe(r)"
    >
      <div class="preset-info">
        <span class="preset-name">{{ r.name }}</span>
        <span class="preset-meta">{{ recipeMeta(r) }}</span>
      </div>
      <div class="preset-actions">
        <button
          class="btn-icon btn-icon-danger"
          :class="{ 'btn-confirm': pendingDeleteRecipeId === r.id }"
          :title="pendingDeleteRecipeId === r.id ? t('post.recipe_delete_confirm') : t('post.recipe_delete')"
          @click.stop="deleteRecipe(r.id)"
        ><X :size="14" /></button>
      </div>
    </div>
  </div>

  <div class="pipeline-progress" v-if="recipeRunning">
    <span class="spinner spinner-sm"></span>
    {{ t('post.recipe_progress', { i: recipeStepIdx, n: recipeStepTotal }) }}
  </div>
  <button v-if="recipeRunning" class="btn btn-secondary btn-block" @click="interruptRecipe">
    {{ t('progress.btn_interrupt') }}
  </button>

  <div class="preset-form" v-if="showRecipeForm && !recipeRunning">
    <div class="form-divider"></div>
    <div class="form-group">
      <input class="form-control" type="text" v-model="recipeName" :placeholder="t('post.recipe_name')" />
    </div>
    <div class="preset-form-actions">
      <button class="btn btn-sm btn-primary" @click="saveRecipe" :disabled="!recipeName">
        {{ t('export.btn_save') }}
      </button>
      <button class="btn btn-sm btn-secondary" @click="showRecipeForm = false">{{ t('export.btn_cancel') }}</button>
    </div>
  </div>
</template>

<style scoped src="./post-shared.css"></style>

<style scoped>
.pipeline-progress {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: var(--fs-md);
  color: var(--text);
  margin-bottom: 6px;
}
</style>
