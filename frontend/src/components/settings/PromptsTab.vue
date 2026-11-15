<script setup>
import { ref, inject } from 'vue'
import StatusMessage from '../StatusMessage.vue'
import FoldBlock from '../FoldBlock.vue'
import { api } from '../../api.js'
import { t } from '../../i18n/index.js'

const {
  promptInstruction, defaultPromptInstruction, promptInstructionProse, defaultPromptInstructionProse,
  analyzeSystemPrompt, analyzeSinglePrompt, analyzeDescribePrompt, editGarmentPrompt,
  defaultAnalyzePrompts, defaultGarmentPrompt,
} = inject('settingsState')

const promptInstructionSaved = ref(false)
const promptInstructionError = ref('')
const promptInstructionProseSaved = ref(false)
const promptInstructionProseError = ref('')
const analyzeSaved = ref(false)
const analyzeError = ref('')

async function saveSetting(key, value, saved, error) {
  saved.value = false
  error.value = ''
  try {
    await api.updateSettings({ [key]: value })
    saved.value = true
  } catch (e) {
    error.value = String(e)
  }
}

const savePromptInstruction = () => saveSetting('sd_prompt_instruction', promptInstruction.value, promptInstructionSaved, promptInstructionError)
const savePromptInstructionProse = () => saveSetting('sd_prompt_instruction_prose', promptInstructionProse.value, promptInstructionProseSaved, promptInstructionProseError)

function storedPrompt(value, def) {
  return def && value.trim() === def.trim() ? '' : value
}

function promptState(value, def) {
  return value && def && value.trim() !== def.trim() ? t('settings.prompt_custom') : t('settings.prompt_default')
}

async function saveAnalyzePrompts() {
  analyzeSaved.value = false
  analyzeError.value = ''
  try {
    const defs = defaultAnalyzePrompts.value
    await api.updateSettings({
      analyze_system_prompt: storedPrompt(analyzeSystemPrompt.value, defs?.system_prompt),
      analyze_prompt: storedPrompt(analyzeSinglePrompt.value, defs?.single_prompt),
      analyze_describe_prompt: storedPrompt(analyzeDescribePrompt.value, defs?.describe_prompt),
      edit_garment_prompt: storedPrompt(editGarmentPrompt.value, defaultGarmentPrompt.value),
    })
    analyzeSaved.value = true
  } catch (e) {
    analyzeError.value = String(e)
  }
}

function resetAnalyzePrompts() {
  const defs = defaultAnalyzePrompts.value
  if (!defs) return
  analyzeSystemPrompt.value = defs.system_prompt
  analyzeSinglePrompt.value = defs.single_prompt
  analyzeDescribePrompt.value = defs.describe_prompt || ''
  if (defaultGarmentPrompt.value) editGarmentPrompt.value = defaultGarmentPrompt.value
}
</script>

<template>
  <div class="prompts-tab">
    <div class="card">
      <h3 class="prompts-title">{{ t('settings.group_image_prompt') }}</h3>
      <div class="prompts-sub">{{ t('settings.group_image_prompt_sub') }}</div>

      <FoldBlock :title="t('settings.section_sd_prompt')" :hint="t('settings.prompt_instruction_description')" :summary="promptState(promptInstruction, defaultPromptInstruction)">
        <StatusMessage v-if="promptInstructionSaved" kind="success" :timeout="8000" class="mb-3" @close="promptInstructionSaved = false">{{ t('settings.instruction_saved') }}</StatusMessage>
        <StatusMessage v-if="promptInstructionError" class="mb-3" @close="promptInstructionError = ''">{{ promptInstructionError }}</StatusMessage>
        <textarea class="form-textarea prompt-area" v-model="promptInstruction" rows="16"></textarea>
        <div class="prompt-actions">
          <button class="btn btn-primary" @click="savePromptInstruction">{{ t('settings.btn_save_instruction') }}</button>
          <button class="btn btn-secondary" @click="promptInstruction = defaultPromptInstruction">{{ t('settings.btn_reset_default') }}</button>
        </div>
      </FoldBlock>

      <FoldBlock :title="t('settings.section_sd_prompt_prose')" :hint="t('settings.prompt_instruction_prose_description')" :summary="promptState(promptInstructionProse, defaultPromptInstructionProse)">
        <StatusMessage v-if="promptInstructionProseSaved" kind="success" :timeout="8000" class="mb-3" @close="promptInstructionProseSaved = false">{{ t('settings.instruction_saved') }}</StatusMessage>
        <StatusMessage v-if="promptInstructionProseError" class="mb-3" @close="promptInstructionProseError = ''">{{ promptInstructionProseError }}</StatusMessage>
        <textarea class="form-textarea prompt-area" v-model="promptInstructionProse" rows="16"></textarea>
        <div class="prompt-actions">
          <button class="btn btn-primary" @click="savePromptInstructionProse">{{ t('settings.btn_save_instruction') }}</button>
          <button class="btn btn-secondary" @click="promptInstructionProse = defaultPromptInstructionProse">{{ t('settings.btn_reset_default') }}</button>
        </div>
      </FoldBlock>
    </div>

    <div class="card mt-4">
      <h3 class="prompts-title">{{ t('settings.group_describe_photo') }}</h3>
      <div class="prompts-sub">{{ t('settings.group_describe_photo_sub') }}</div>

      <FoldBlock :title="t('settings.label_system_prompt')" :hint="t('settings.hint_system_prompt')" :summary="promptState(analyzeSystemPrompt, defaultAnalyzePrompts?.system_prompt)">
        <textarea class="form-textarea prompt-area" v-model="analyzeSystemPrompt" rows="3"></textarea>
      </FoldBlock>
      <FoldBlock :title="t('settings.label_single_analysis')" :hint="t('settings.hint_single_analysis')" :summary="promptState(analyzeSinglePrompt, defaultAnalyzePrompts?.single_prompt)">
        <textarea class="form-textarea prompt-area" v-model="analyzeSinglePrompt" rows="10"></textarea>
      </FoldBlock>
      <FoldBlock :title="t('settings.label_describe_analysis')" :hint="t('settings.hint_describe_analysis')" :summary="promptState(analyzeDescribePrompt, defaultAnalyzePrompts?.describe_prompt)">
        <textarea class="form-textarea prompt-area" v-model="analyzeDescribePrompt" rows="6"></textarea>
      </FoldBlock>
      <FoldBlock :title="t('settings.edit_garment_prompt')" :hint="t('settings.edit_garment_prompt_hint')" :summary="promptState(editGarmentPrompt, defaultGarmentPrompt)">
        <textarea class="form-textarea prompt-area" v-model="editGarmentPrompt" rows="8"></textarea>
      </FoldBlock>

      <StatusMessage v-if="analyzeSaved" kind="success" :timeout="8000" class="mb-3" @close="analyzeSaved = false">{{ t('settings.prompts_saved') }}</StatusMessage>
      <StatusMessage v-if="analyzeError" class="mb-3" @close="analyzeError = ''">{{ analyzeError }}</StatusMessage>
      <div class="prompt-actions">
        <button class="btn btn-primary" @click="saveAnalyzePrompts">{{ t('settings.btn_save_prompts') }}</button>
        <button class="btn btn-secondary" @click="resetAnalyzePrompts">{{ t('settings.btn_reset_default') }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.prompts-title {
  color: var(--text-bright);
  margin-bottom: 4px;
}
.prompts-sub {
  color: var(--text-dim);
  font-size: var(--fs-md);
  margin-bottom: 8px;
}
.prompt-area {
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  line-height: 1.5;
}
.prompt-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
</style>
