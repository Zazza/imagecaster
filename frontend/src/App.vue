<script setup>
import TitleBar from './components/TitleBar.vue'
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { WindowSetSystemDefaultTheme } from './wailsjs/runtime/runtime'
import { Sparkles, Sliders, SlidersHorizontal, Settings, RotateCcw, FolderOpen, Sun, Moon } from 'lucide-vue-next'
import { api } from './api.js'
import { t, setLocale, locale } from './i18n/index.js'
import UnifiedPresetsPage from './components/UnifiedPresetsPage.vue'
import UnifiedGeneratePage from './components/UnifiedGeneratePage.vue'
import PostProcessPage from './components/PostProcessPage.vue'
import SettingsPage from './components/SettingsPage.vue'
import AppFooter from './components/AppFooter.vue'
import FileBrowserPage from './components/FileBrowserPage.vue'
import WelcomeModal from './components/WelcomeModal.vue'
import ServerCheckModal from './components/ServerCheckModal.vue'
import { serverState, checkServer } from './serverState.js'
import { WELCOME_KEY, welcomeClosed, isFreshInstall, startupTab } from './welcome.js'
import { RELEASE_NOTES } from './releaseNotes.js'
import { loadDefaultPlan } from './modelDownloads.js'

const page = ref('generate')
const APP_VERSION = RELEASE_NOTES[0]?.version || ''
const welcome = ref(null)
const showServerCheck = ref(false)

async function startup() {
  if (await checkServer() === 'ok') {
    initWelcome()
    return
  }
  showServerCheck.value = true
}

function onServerReady() {
  showServerCheck.value = false
  initWelcome()
}

function onServerCheckClose() {
  showServerCheck.value = false
  serverState.dismissed = true
  initWelcome()
}

function readWelcome() {
  try { return JSON.parse(localStorage.getItem(WELCOME_KEY) || 'null') } catch { return null }
}

function closeWelcome(dontShow) {
  try {
    localStorage.setItem(WELCOME_KEY, JSON.stringify(welcomeClosed(readWelcome(), APP_VERSION, dontShow)))
  } catch (e) {
    console.warn('welcome state not saved', e)
  }
  welcome.value = null
}

function welcomeToSettings(dontShow) {
  closeWelcome(dontShow)
  onNavigate({ page: 'settings', tab: 'connection' })
}

async function checkFreshInstall() {
  try {
    const models = await api.getServerModels('sd')
    if ((models || []).length !== 0) return false
  } catch {
    return false
  }
  return isFreshInstall(await loadDefaultPlan(), 0)
}

async function initWelcome() {
  const state = readWelcome()
  const tab = startupTab(state, APP_VERSION, false)
  if (tab) {
    welcome.value = tab
    return
  }
  if (await checkFreshInstall()) welcome.value = startupTab(state, APP_VERSION, true)
}
const resetKey = ref(0)
const confirmReset = ref(false)
const resetting = ref(false)
const isResetting = ref(false)
let resetTimer = null

const settingsTab = ref('')
const settingsKey = ref(0)
const postprocessSection = ref('')
const initialImage = ref('')
const theme = ref('dark')

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', theme.value)
  api.updateSettings({ theme: theme.value }).catch(() => {})
}

async function resetAll() {
  if (resetting.value) return
  if (!confirmReset.value) {
    confirmReset.value = true
    resetTimer = setTimeout(() => { confirmReset.value = false }, 3000)
    return
  }
  clearTimeout(resetTimer)
  confirmReset.value = false
  resetting.value = true
  isResetting.value = true
  try {
    await api.updateSettings({
      gen_preset_id: '',
      gen_type_id: '',
      gen_description: '',
      gen_negative: '',
      gen_extra_prompt: '',
      gen_extra_negative: '',
      gen_mode: 'preset',
      gen_compound_preset_id: '',
      gen_resolution_id: '',
      gen_hires_profile_id: '',
      gen_count: '',
      fi_preset_id: '',
      fi_compound_preset_id: '',
      fi_gen_mode: 'preset',
      fi_denoising: '0.5',
      fi_extra_negative: '',
      fi_analyze_mode: 'quick',
      object_op: 'remove',
      object_brush: '',
      fi_quality_mode: 'false',
      fi_preserve_identity: 'true',
      fi_count: '',
      fi_hires_profile_id: '',
      edit_instruction: '',
      edit_quality_mode: 'false',
      edit_resolution_id: '',
      edit_style_preset_id: '',
      edit_style_tags: 'false',
      edit_look_mode: 'effects',
      test_prompt: '',
      test_negative: '',
      test_mode: 'preset',
      test_sampler: '',
      test_schedule_type: '',
      test_steps: '',
      test_cfg_scale: '',
      test_width: '',
      test_height: '',
      test_resolution_id: '',
      test_hires_profile_id: '',
    })
    await api.clearLastImage()
    resetKey.value++
    await nextTick()
  } catch (e) {
    console.error('Reset failed:', e)
  } finally {
    isResetting.value = false
    resetting.value = false
  }
}

function onNavigate(target) {
  if (target.page === 'settings' && target.tab) {
    settingsTab.value = target.tab
  } else if (target.page === 'postprocess') {
    if (target.section) postprocessSection.value = target.section
    if (target.image) initialImage.value = target.image
  }
  page.value = target.page
  if (target.page === 'settings') {
    setTimeout(() => { settingsTab.value = '' }, 100)
  } else if (target.page === 'postprocess') {
    setTimeout(() => { postprocessSection.value = '' }, 100)
  }
}

const currentPage = computed(() => {
  switch (page.value) {
    case 'generate': return UnifiedGeneratePage
    case 'postprocess': return PostProcessPage
    case 'presets': return UnifiedPresetsPage
    case 'settings': return SettingsPage
    case 'browser': return FileBrowserPage
    default: return UnifiedGeneratePage
  }
})

watch(locale, (l) => {
  Promise.resolve(api.updateSettings({ ui_locale: l })).catch((e) => console.warn('save ui_locale:', e))
}, { immediate: true })

onMounted(async () => {
  WindowSetSystemDefaultTheme()
  try {
    const s = await api.getSettings()
    if (s?.theme) {
      theme.value = s.theme
      document.documentElement.setAttribute('data-theme', s.theme)
    }
  } catch {}
  startup()
  window.addEventListener('beforeunload', () => {
    api.saveWindowLayout(0).catch(() => {})
  })
})

onUnmounted(() => {
  clearTimeout(resetTimer)
})
</script>

<template>
  <div class="app">
    <TitleBar />
    <div class="app-body">
    <aside class="sidebar">
      <div class="sidebar-logo" :title="t('welcome.tab_about')" @click="welcome = 'about'">
        <img src="./assets/logo.png" alt="logo" class="logo-img" /> <span class="sidebar-label">{{ t('app.title') }}</span>
      </div>
      <nav class="sidebar-nav">
        <div class="sidebar-group">
          <div class="sidebar-group-label">{{ t('app.group_create') }}</div>
          <a class="sidebar-link" :class="{ active: page === 'generate' }" @click="page = 'generate'" :title="t('app.nav_generate')">
            <Sparkles :size="16" class="icon" /> <span class="sidebar-label">{{ t('app.nav_generate') }}</span>
          </a>
        </div>
        <div class="sidebar-group">
          <div class="sidebar-group-label">{{ t('app.group_refine') }}</div>
          <a class="sidebar-link" :class="{ active: page === 'postprocess' }" @click="page = 'postprocess'" :title="t('app.nav_postprocess')">
            <SlidersHorizontal :size="16" class="icon" /> <span class="sidebar-label">{{ t('app.nav_postprocess') }}</span>
          </a>
        </div>
        <div class="sidebar-group">
          <div class="sidebar-group-label">{{ t('app.group_view') }}</div>
          <a class="sidebar-link" :class="{ active: page === 'browser' }" @click="page = 'browser'" :title="t('app.nav_my_images')">
            <FolderOpen :size="16" class="icon" /> <span class="sidebar-label">{{ t('app.nav_my_images') }}</span>
          </a>
        </div>
        <div class="sidebar-group">
          <div class="sidebar-group-label">{{ t('app.group_library') }}</div>
          <a class="sidebar-link" :class="{ active: page === 'presets' }" @click="page = 'presets'" :title="t('app.nav_styles')">
            <Sliders :size="16" class="icon" /> <span class="sidebar-label">{{ t('app.nav_styles') }}</span>
          </a>
        </div>
        <div class="sidebar-group">
          <div class="sidebar-group-label">{{ t('app.group_setup') }}</div>
          <a class="sidebar-link" :class="{ active: page === 'settings' }" @click="page = 'settings'" :title="t('app.nav_settings')">
            <Settings :size="16" class="icon" /> <span class="sidebar-label">{{ t('app.nav_settings') }}</span>
          </a>
        </div>
      </nav>
      <div class="sidebar-footer">
        <button class="sidebar-theme-btn" @click="toggleTheme" :title="theme === 'dark' ? t('app.theme_light') : t('app.theme_dark')">
          <Sun v-if="theme === 'dark'" :size="14" class="icon" />
          <Moon v-else :size="14" class="icon" />
        </button>
        <button class="sidebar-theme-btn" @click="setLocale(locale === 'ru' ? 'en' : 'ru')" :title="locale === 'ru' ? 'English' : 'Русский'">
          {{ locale === 'ru' ? 'EN' : 'RU' }}
        </button>
        <button class="sidebar-reset-btn" :class="{ confirm: confirmReset }" :title="confirmReset ? t('app.confirm') : t('app.reset_all')" @click="resetAll">
          <RotateCcw :size="14" class="icon" /> <span class="sidebar-label">{{ confirmReset ? t('app.confirm') : t('app.reset_all') }}</span>
        </button>
      </div>
    </aside>
    <div class="app-col">
    <div v-if="serverState.state && serverState.state !== 'ok' && !showServerCheck" class="server-banner">
      <span>{{ serverState.state === 'outdated' ? t({ old_version: 'server.banner_outdated', no_sdcpp: 'server.banner_forge' }[serverState.reason] || 'server.banner_incompatible') : t('server.banner_offline') }}</span>
      <button class="btn btn-secondary btn-sm" @click="showServerCheck = true">{{ t('server.banner_fix') }}</button>
    </div>
    <main class="main">
      <SettingsPage v-show="page === 'settings'" :active="page === 'settings'" :initial-tab="settingsTab" @navigate="onNavigate" />
      <template v-if="page !== 'settings'">
        <UnifiedGeneratePage v-if="page === 'generate'" :key="resetKey" :resetting="isResetting" @navigate="onNavigate" />
        <PostProcessPage v-else-if="page === 'postprocess'" :initial-section="postprocessSection" :initial-image="initialImage" @clear-initial-image="initialImage = ''" />
        <FileBrowserPage v-else-if="page === 'browser'" @navigate="onNavigate" />
        <component v-else :is="currentPage" />
      </template>
    </main>
    <AppFooter @navigate="onNavigate" />
    </div>
    </div>
    <ServerCheckModal v-if="showServerCheck" @ready="onServerReady" @close="onServerCheckClose" />
    <WelcomeModal v-if="welcome && !showServerCheck" :tab="welcome" @close="closeWelcome" @settings="welcomeToSettings" />
  </div>
</template>
