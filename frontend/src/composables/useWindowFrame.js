import { ref, watch } from 'vue'
import { WindowIsMaximised, Environment } from '../wailsjs/runtime/runtime.js'
import { frameEdge } from '../windowEdge.js'

export const maximised = ref(false)

async function syncMaximised() {
  try {
    maximised.value = await WindowIsMaximised()
  } catch {
    // outside Wails there is no window to ask
  }
}

async function syncPlatform() {
  try {
    const env = await Environment()
    document.documentElement.classList.toggle('win-rounded', env.platform === 'linux')
  } catch {
    // outside Wails there is no window to ask
  }
}

let installed = false

export function installWindowFrame() {
  if (installed) return
  installed = true
  watch(maximised, (v) => document.documentElement.classList.toggle('win-maximised', v))
  syncMaximised()
  syncPlatform()
  window.addEventListener('resize', syncMaximised)
  window.addEventListener('mousemove', () => {
    const flags = window.wails?.flags
    if (!flags?.enableResize) return
    const edge = frameEdge(flags.resizeEdge, maximised.value)
    if (edge === flags.resizeEdge) return
    flags.resizeEdge = edge
    document.documentElement.style.cursor = edge || flags.defaultCursor || ''
  })
}
