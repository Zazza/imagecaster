import { ref, computed, watch } from 'vue'
import { api } from '../api.js'
import { t } from '../i18n/index.js'
import { customStepsPayload } from '../customSteps.js'
import { runIdeasBatch } from '../ideasBatch.js'

function ideaDisplayName(desc) {
  if (desc.name) return desc.name
  const text = desc.text || ''
  return text.length > 40 ? text.slice(0, 40) + '…' : text
}

export function useIdeasBatch({
  savedDescs, showSavedDescs, genMode, presets, compoundPresets, selectedPresetId, selectedCompoundPresetId,
  presetOverrides, customSteps, customValid, selectedResolutionId, selectedHiresProfileId,
  jobs, batchTotal, generatingImage, generationStage, error, successMsg, resetProgress, saveGenState,
}) {
  const ideasBatchRunning = ref(false)
  const ideasBatchProgress = ref(null)
  let ideasBatchStopped = false

  const ideasBatchTarget = computed(() => {
    if (genMode.value === 'preset') {
      const p = presets.value.find(x => x.id === selectedPresetId.value)
      return p ? t('descriptions.batch_target_preset', { name: p.name }) : ''
    }
    if (genMode.value === 'custom') return customValid.value ? t('descriptions.batch_target_custom', { n: customSteps.value.length }) : ''
    const c = compoundPresets.value.find(x => x.id === selectedCompoundPresetId.value)
    return c ? t('descriptions.batch_target_workflow', { name: c.name }) : ''
  })

  const ideasBatchDisabled = computed(() => {
    if (genMode.value === 'preset') return selectedPresetId.value ? '' : t('descriptions.batch_no_style')
    if (genMode.value === 'custom') return customValid.value ? '' : t('descriptions.batch_bad_custom')
    return selectedCompoundPresetId.value ? '' : t('descriptions.batch_no_workflow')
  })

  async function runIdeasBatchFromModal({ ids, perIdea }) {
    if (ideasBatchRunning.value || ideasBatchDisabled.value) return
    const byId = new Map(savedDescs.value.map(d => [d.id, d]))
    const ideas = (ids || []).map(id => byId.get(id)).filter(Boolean)
    if (!ideas.length) return

    const mode = genMode.value
    const presetId = selectedPresetId.value
    const compoundId = selectedCompoundPresetId.value
    const ov = presetOverrides.value
    const customPayload = customStepsPayload(customSteps.value, presets.value)
    const resId = selectedResolutionId.value
    const hiresId = selectedHiresProfileId.value

    saveGenState()
    if (jobs.pending.value.size === 0) {
      resetProgress()
      batchTotal.value = 0
      jobs.done.value = 0
    }
    ideasBatchStopped = false
    ideasBatchRunning.value = true
    ideasBatchProgress.value = { done: 0, total: ideas.length, errors: [] }
    error.value = ''
    successMsg.value = ''

    try {
      const res = await runIdeasBatch({
        ideas,
        perIdea,
        isStopped: () => ideasBatchStopped,
        generatePrompt: (idea) => ({ prompt: idea.text || '', negative_prompt: idea.negative_prompt || '' }),
        enqueue: async ({ prompt, negative_prompt }) => {
          const common = { extra_prompt: '', extra_negative_prompt: '', idea: prompt, idea_negative: negative_prompt, resolution_id: resId, hires_profile_id: hiresId }
          let jobId
          if (mode === 'compound') jobId = await api.enqueueCompound({ compound_preset_id: compoundId, ...common })
          else if (mode === 'custom') jobId = await api.enqueueCompound({ steps: customPayload, ...common, seed: null })
          else jobId = await api.enqueueTxt2Img({ preset_id: presetId, ...common, overrides: ov, seed: null })
          if (jobId) {
            jobs.track(jobId)
            batchTotal.value++
            generatingImage.value = true
            generationStage.value = 'image'
          }
          return jobId
        },
        onProgress: ({ done, total }) => {
          ideasBatchProgress.value = { ...ideasBatchProgress.value, done, total }
        },
      })
      ideasBatchProgress.value = {
        ...ideasBatchProgress.value,
        errors: res.errors.map(e => ({ name: ideaDisplayName(e.idea), error: e.error === 'empty prompt' ? t('descriptions.batch_empty_idea') : e.error })),
      }
      if (res.jobIds.length) {
        successMsg.value = t('descriptions.batch_done', { n: res.jobIds.length })
        ideasBatchProgress.value = { ...ideasBatchProgress.value, message: successMsg.value }
      }
    } catch (e) {
      error.value = String(e)
    } finally {
      ideasBatchRunning.value = false
      if (jobs.pending.value.size === 0) {
        generatingImage.value = false
        generationStage.value = ''
      }
    }
  }

  watch(showSavedDescs, (open) => {
    if (open && !ideasBatchRunning.value) ideasBatchProgress.value = null
  })

  function stopIdeasBatch() {
    ideasBatchStopped = true
  }

  return {
    ideasBatchRunning,
    ideasBatchProgress,
    ideasBatchTarget,
    ideasBatchDisabled,
    runIdeasBatchFromModal,
    stopIdeasBatch,
  }
}
