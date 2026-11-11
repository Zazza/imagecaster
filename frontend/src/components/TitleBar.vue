<script setup>
import { Minus, Square, Copy, X } from 'lucide-vue-next'
import { WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime/runtime.js'
import { maximised, installWindowFrame } from '../composables/useWindowFrame.js'
import { t } from '../i18n/index.js'

installWindowFrame()
</script>

<template>
  <div class="titlebar" @dblclick.self="WindowToggleMaximise">
    <span class="titlebar-text mono" @dblclick="WindowToggleMaximise">{{ t('app.title') }}</span>
    <button type="button" class="titlebar-btn" :title="t('window.minimise')" :aria-label="t('window.minimise')" @click="WindowMinimise"><Minus :size="14" /></button>
    <button type="button" class="titlebar-btn" :title="maximised ? t('window.restore') : t('window.maximise')" :aria-label="maximised ? t('window.restore') : t('window.maximise')" @click="WindowToggleMaximise">
      <Copy v-if="maximised" :size="12" />
      <Square v-else :size="12" />
    </button>
    <button type="button" class="titlebar-btn close" :title="t('window.close')" :aria-label="t('window.close')" @click="Quit"><X :size="14" /></button>
  </div>
</template>

<style>
#app {
  height: 100%;
  box-sizing: border-box;
  padding: 0 6px 6px;
  background: linear-gradient(var(--bg), var(--bg)) content-box, var(--frame);
}
.win-maximised #app { padding: 0; }
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
  height: 32px;
  margin: 0 -6px;
  background: var(--frame);
  color: var(--text-dim);
  user-select: none;
}
.win-maximised .titlebar { margin: 0; }
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
