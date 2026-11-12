<script setup>
import { Minus, Square, Copy, X } from 'lucide-vue-next'
import { WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime/runtime.js'
import { maximised, platform, installWindowFrame } from '../composables/useWindowFrame.js'
import { t } from '../i18n/index.js'

installWindowFrame()
</script>

<template>
  <div class="titlebar" @dblclick.self="WindowToggleMaximise">
    <span class="titlebar-text mono" @dblclick="WindowToggleMaximise">{{ t('app.title') }}</span>
    <template v-if="platform !== 'darwin'">
      <button type="button" class="titlebar-btn" :title="t('window.minimise')" :aria-label="t('window.minimise')" @click="WindowMinimise"><Minus :size="14" /></button>
      <button type="button" class="titlebar-btn" :title="maximised ? t('window.restore') : t('window.maximise')" :aria-label="maximised ? t('window.restore') : t('window.maximise')" @click="WindowToggleMaximise">
        <Copy v-if="maximised" :size="12" />
        <Square v-else :size="12" />
      </button>
      <button type="button" class="titlebar-btn close" :title="t('window.close')" :aria-label="t('window.close')" @click="Quit"><X :size="14" /></button>
    </template>
  </div>
</template>

<style>
:root { --titlebar-h: 32px; --frame-pad: 6px; }
.win-mac { --titlebar-h: 28px; --frame-pad: 0px; }
#app {
  height: 100%;
  box-sizing: border-box;
  padding: 0 6px 6px;
  background: linear-gradient(var(--bg), var(--bg)) content-box, var(--frame);
}
.win-maximised #app, .win-mac #app { padding: 0; }
.win-rounded, .win-rounded body { background: transparent; }
.win-rounded #app { border-radius: 8px; overflow: hidden; contain: paint; }
.win-rounded.win-maximised #app { border-radius: 0; }
.titlebar {
  --wails-draggable: drag;
  position: relative;
  z-index: 100;
  flex: none;
  display: flex;
  align-items: center;
  height: var(--titlebar-h);
  margin: 0 -6px;
  background: var(--frame);
  color: var(--text-dim);
  user-select: none;
}
.win-maximised .titlebar, .win-mac .titlebar { margin: 0; }
.win-mac .titlebar { padding-left: 78px; }
.win-mac.win-fullscreen .titlebar { padding-left: 0; }
.titlebar-text, .titlebar-btn { transition: opacity 0.15s; }
.win-inactive .titlebar-text, .win-inactive .titlebar-btn { opacity: 0.5; }
.win-inactive .titlebar-btn:hover { opacity: 1; }
.titlebar-text {
  position: absolute;
  left: 140px;
  right: 140px;
  text-align: center;
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--accent-mark);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.titlebar-btn:first-of-type { margin-left: auto; }
.titlebar-btn {
  --wails-draggable: no-drag;
  flex: none;
  width: 44px;
  height: 100%;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: none;
  color: var(--text-dim);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.titlebar-btn:hover { background: var(--surface-3); color: var(--text); }
.titlebar-btn.close:hover { background: var(--danger); color: #fff; }
</style>
