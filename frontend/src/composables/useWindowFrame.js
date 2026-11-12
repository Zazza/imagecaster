import { ref, watch } from 'vue'
import { WindowIsMaximised, WindowIsFullscreen, Environment } from '../wailsjs/runtime/runtime.js'
import { frameEdge } from '../windowEdge.js'

export const maximised = ref(false)
export const platform = ref('')

async function syncMaximised() {
  try {
    maximised.value = await WindowIsMaximised()
  } catch {
    // outside Wails there is no window to ask
  }
}

async function syncFullscreen() {
  try {
    document.documentElement.classList.toggle('win-fullscreen', !!(await WindowIsFullscreen()))
  } catch {
    // outside Wails there is no window to ask
  }
}

function onWindowResize() {
  syncMaximised()
  syncFullscreen()
}

function setInactive(v) {
  document.documentElement.classList.toggle('win-inactive', v)
}

async function syncPlatform() {
  try {
    const env = await Environment()
    platform.value = env.platform || ''
    document.documentElement.classList.toggle('win-rounded', env.platform === 'linux')
    document.documentElement.classList.toggle('win-mac', env.platform === 'darwin')
  } catch {
    // outside Wails there is no window to ask
  }
}

let installed = false

export function installWindowFrame() {
  if (installed) return
  installed = true
  watch(maximised, (v) => document.documentElement.classList.toggle('win-maximised', v))
  onWindowResize()
  syncPlatform()
  window.addEventListener('resize', onWindowResize)
  window.addEventListener('blur', () => setInactive(true))
  window.addEventListener('focus', () => setInactive(false))
  window.addEventListener('mousemove', () => {
    const flags = window.wails?.flags
    if (!flags?.enableResize) return
    const edge = frameEdge(flags.resizeEdge, maximised.value)
    if (edge === flags.resizeEdge) return
    flags.resizeEdge = edge
    document.documentElement.style.cursor = edge || flags.defaultCursor || ''
  })
}
