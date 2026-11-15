<script setup>
import { ref, reactive, watch, provide, onMounted, onUnmounted } from 'vue'
import { fastGeneration, applyFastSetting } from '../fastMode.js'
import { api } from '../api.js'
import { checkServer } from '../serverState.js'
import { serverURL, loadServerModels } from '../serverModels.js'
import { t } from '../i18n/index.js'
import ConnectionTab from './settings/ConnectionTab.vue'
import PromptsTab from './settings/PromptsTab.vue'
import SafetyTab from './settings/SafetyTab.vue'
import AdvancedTab from './settings/AdvancedTab.vue'

const props = defineProps({
  initialTab: { type: String, default: 'connection' },
  active: { type: Boolean, default: true }
})

const TABS = [
  ['connection', 'settings.tab_connection'],
  ['prompts', 'settings.tab_prompts'],
  ['safety', 'settings.tab_safety'],
  ['advanced', 'settings.tab_advanced'],
]

const activeTab = ref(props.initialTab || 'connection')
watch(() => props.initialTab, (tab) => { if (tab) activeTab.value = tab })

const state = {
  discoveredServers: ref([]),
  discovering: ref(false),
  serverStatus: ref(null),
  utilitiesExpanded: ref(false),
  generateModel: ref(''),
  analyzeModel: ref(''),
  generateParams: reactive({ num_ctx: 4096, top_p: 0.9, num_thread: 0 }),
  analyzeParams: reactive({ num_ctx: 4096, top_p: 0.9, num_thread: 0 }),
  kidsMode: ref(false),
  kidsCategories: ref([]),
  memoryForm: reactive({ llm_auto_unload: true }),
  fastGeneration,
  processActionLoading: reactive({}),
  serverModelDownload: reactive({ type: 'sd', url: '', filename: '' }),
  llmPullName: ref(''),
  promptInstruction: ref(''),
  defaultPromptInstruction: ref(''),
  promptInstructionProse: ref(''),
  defaultPromptInstructionProse: ref(''),
  analyzeSystemPrompt: ref(''),
  analyzeSinglePrompt: ref(''),
  analyzeDescribePrompt: ref(''),
  editGarmentPrompt: ref(''),
  defaultAnalyzePrompts: ref(null),
  defaultGarmentPrompt: ref(''),
  refreshServerStatus,
  pollServerStatus,
  loadKidsCategories,
}
provide('settingsState', state)

async function refreshServerStatus() {
  if (!serverURL.value) return
  checkServer()
  try {
    state.serverStatus.value = await api.getServerStatus()
    loadServerModels()
  } catch (e) {
    console.error('refreshServerStatus:', e)
  }
}

let pollTimer = null

function stopServerPoll() {
  clearInterval(pollTimer)
  pollTimer = null
}

function pollServerStatus(interval = 2000, count = 10) {
  stopServerPoll()
  let i = 0
  pollTimer = setInterval(() => {
    i++
    if (i >= count) stopServerPoll()
    refreshServerStatus()
  }, interval)
}

onUnmounted(stopServerPoll)

async function loadKidsCategories() {
  try {
    state.kidsCategories.value = await api.getKidsCategories()
  } catch (e) {
    console.warn('SettingsPage.loadKidsCategories:', e)
  }
}

async function loadDefault(target, getter, label) {
  try {
    target.value = await getter()
  } catch (e) {
    console.error(label + ':', e)
  }
}

function fillEmpty(target, value) {
  if (!target.value && value) target.value = value
}

async function loadSettings() {
  const s = state
  try {
    const settings = await api.getSettings()
    s.generateModel.value = settings.llm_generate_model || settings.sd_prompt_model || ''
    s.analyzeModel.value = settings.llm_analyze_model || settings.vision_model || ''
    s.generateParams.num_ctx = parseInt(settings.llm_generate_num_ctx) || 4096
    s.generateParams.top_p = parseFloat(settings.llm_generate_top_p) || 0.9
    s.generateParams.num_thread = parseInt(settings.llm_generate_num_thread) || 0
    s.analyzeParams.num_ctx = parseInt(settings.llm_analyze_num_ctx) || 4096
    s.analyzeParams.top_p = parseFloat(settings.llm_analyze_top_p) || 0.9
    s.analyzeParams.num_thread = parseInt(settings.llm_analyze_num_thread) || 0
    s.kidsMode.value = await api.isKidsModeActive()
    loadKidsCategories()
    s.memoryForm.llm_auto_unload = settings.llm_auto_unload !== 'false'
    applyFastSetting(settings)
    s.promptInstruction.value = settings.sd_prompt_instruction || ''
    s.promptInstructionProse.value = settings.sd_prompt_instruction_prose || ''
    serverURL.value = settings.server_url || ''
    if (serverURL.value) {
      refreshServerStatus()
    }
  } catch (e) {
    console.error('loadSettings:', e)
  }

  await loadDefault(s.defaultPromptInstruction, api.getDefaultPromptInstruction, 'getDefaultPromptInstruction')
  fillEmpty(s.promptInstruction, s.defaultPromptInstruction.value)

  await loadDefault(s.defaultPromptInstructionProse, api.getDefaultPromptInstructionProse, 'getDefaultPromptInstructionProse')
  fillEmpty(s.promptInstructionProse, s.defaultPromptInstructionProse.value)

  try {
    const settings = await api.getSettings()
    s.analyzeSystemPrompt.value = settings.analyze_system_prompt || ''
    s.analyzeSinglePrompt.value = settings.analyze_prompt || ''
    s.analyzeDescribePrompt.value = settings.analyze_describe_prompt || ''
    s.editGarmentPrompt.value = settings.edit_garment_prompt || ''
  } catch (e) {
    console.error('loadAnalyzeSettings:', e)
  }

  await loadDefault(s.defaultAnalyzePrompts, api.getDefaultAnalyzePrompts, 'getDefaultAnalyzePrompts')
  const defs = s.defaultAnalyzePrompts.value
  if (defs) {
    fillEmpty(s.analyzeSystemPrompt, defs.system_prompt)
    fillEmpty(s.analyzeSinglePrompt, defs.single_prompt)
    if (!s.analyzeDescribePrompt.value) s.analyzeDescribePrompt.value = defs.describe_prompt || ''
  }

  await loadDefault(s.defaultGarmentPrompt, api.getDefaultEditGarmentPrompt, 'getDefaultEditGarmentPrompt')
  fillEmpty(s.editGarmentPrompt, s.defaultGarmentPrompt.value)
}

onMounted(loadSettings)
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ t('settings.title') }}</h1>
    </div>

    <div class="tabs">
      <button v-for="[tab, key] in TABS" :key="tab" class="tab" :class="{ active: activeTab === tab }" @click="activeTab = tab">{{ t(key) }}</button>
    </div>

    <ConnectionTab v-if="activeTab === 'connection'" :active="active" />
    <PromptsTab v-if="activeTab === 'prompts'" />
    <SafetyTab v-if="activeTab === 'safety'" />
    <AdvancedTab v-if="activeTab === 'advanced'" />
  </div>
</template>
