import { watch, inject } from 'vue'
import { api } from '../api.js'
import { restoreCustomSteps, persistCustomSteps } from '../customSteps.js'

const GEN_MODES = ['preset', 'compound', 'custom']

export function useGenStatePersistence({
  selectedTypeId, selectedPresetId, selectedCompoundPresetId, genMode, description, negative,
  extraPrompt, extraNegativePrompt, customSteps, selectedResolutionId, selectedHiresProfileId, genCount,
  presets, compoundPresets, genTypes, descs,
}) {
  const shared = inject('sharedGenState', null)

  watch([selectedResolutionId, selectedHiresProfileId], () => {
    api.updateSettings({
      gen_resolution_id: String(selectedResolutionId.value || ''),
      gen_hires_profile_id: String(selectedHiresProfileId.value || ''),
    }).catch(e => console.warn('GeneratePage.watch:', e))
  })

  function saveGenState() {
    api.updateSettings({
      gen_preset_id: String(selectedPresetId.value || ''),
      gen_type_id: String(selectedTypeId.value || ''),
      gen_description: description.value || '',
      gen_negative: negative.value || '',
      gen_desc_id: String(descs.getCurrentDescId() || ''),
      gen_extra_prompt: extraPrompt.value || '',
      gen_extra_negative: extraNegativePrompt.value || '',
      gen_mode: genMode.value,
      gen_compound_preset_id: String(selectedCompoundPresetId.value || ''),
      gen_custom_steps: JSON.stringify(persistCustomSteps(customSteps.value, presets.value)),
      gen_resolution_id: String(selectedResolutionId.value || ''),
      gen_hires_profile_id: String(selectedHiresProfileId.value || ''),
      gen_count: String(genCount.value || 1),
    }).catch(e => console.warn('GeneratePage.saveGenState:', e))
  }

  function selectPreset(id) {
    const pid = Number(id)
    if (presets.value.find(p => p.id === pid)) selectedPresetId.value = pid
  }

  function selectCompoundPreset(id) {
    const cpid = Number(id)
    if (compoundPresets.value.find(c => c.id === cpid)) selectedCompoundPresetId.value = cpid
  }

  function restoreFromSettings(s) {
    let resolutionLoaded = false
    if (s.gen_type_id) {
      const tid = Number(s.gen_type_id) || null
      if (tid && genTypes.value.find(pt => pt.id === tid)) selectedTypeId.value = tid
    }
    if (s.gen_extra_prompt) extraPrompt.value = s.gen_extra_prompt
    if (s.gen_extra_negative) extraNegativePrompt.value = s.gen_extra_negative
    if (s.gen_preset_id) selectPreset(s.gen_preset_id)
    if (s.gen_description) description.value = s.gen_description
    const rid = Number(s.gen_desc_id) || null
    const bindingRestored = descs.restoreBinding(rid)
    if (s.gen_negative && (bindingRestored || !rid)) negative.value = s.gen_negative
    if (GEN_MODES.includes(s.gen_mode)) genMode.value = s.gen_mode
    if (s.gen_compound_preset_id) selectCompoundPreset(s.gen_compound_preset_id)
    if (s.gen_custom_steps) {
      try {
        customSteps.value = restoreCustomSteps(JSON.parse(s.gen_custom_steps), presets.value)
      } catch (e) {
        console.warn('GeneratePage.onMounted:', e)
      }
    }
    if (s.gen_resolution_id) {
      selectedResolutionId.value = Number(s.gen_resolution_id)
      resolutionLoaded = true
    }
    if (s.gen_hires_profile_id) selectedHiresProfileId.value = Number(s.gen_hires_profile_id)
    if (s.gen_count) genCount.value = Math.max(1, Math.min(100, Number(s.gen_count) || 1))
    return resolutionLoaded
  }

  function restoreFromShared() {
    let resolutionLoaded = false
    if (shared.selectedPresetId) selectPreset(shared.selectedPresetId)
    if (shared.selectedCompoundPresetId) selectCompoundPreset(shared.selectedCompoundPresetId)
    if (GEN_MODES.includes(shared.genMode)) genMode.value = shared.genMode
    if (shared.description) description.value = shared.description
    if (shared.negative) negative.value = shared.negative
    if (shared.selectedResolutionId) {
      selectedResolutionId.value = shared.selectedResolutionId
      resolutionLoaded = true
    }
    if (shared.selectedHiresProfileId !== undefined) selectedHiresProfileId.value = shared.selectedHiresProfileId
    return resolutionLoaded
  }

  async function restoreGenState() {
    let resolutionLoaded = false
    try {
      const s = await api.getSettings()
      resolutionLoaded = restoreFromSettings(s)
    } catch (e) {
      console.warn('GeneratePage.onMounted:', e)
    }
    if (shared && restoreFromShared()) resolutionLoaded = true
    if (!resolutionLoaded) {
      try {
        const resolutions = await api.listResolutions()
        if (resolutions && resolutions.length > 0) selectedResolutionId.value = resolutions[0].id
      } catch (e) {
        console.warn('GeneratePage.onMounted:', e)
      }
    }
  }

  function persistGenState() {
    saveGenState()
    if (!shared) return
    shared.selectedPresetId = selectedPresetId.value
    shared.selectedCompoundPresetId = selectedCompoundPresetId.value
    shared.genMode = genMode.value === 'custom' ? 'preset' : genMode.value
    shared.description = description.value
    shared.negative = negative.value
    shared.selectedResolutionId = selectedResolutionId.value
    shared.selectedHiresProfileId = selectedHiresProfileId.value
  }

  return { saveGenState, restoreGenState, persistGenState }
}
