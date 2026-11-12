import { reactive } from 'vue'
import { api } from './api.js'
import { setServerGate } from './serverGate.js'

export const SERVER_INSTALL_URL = {
  ru: 'https://github.com/Zazza/imagecaster-server/blob/main/README-ru.md',
  en: 'https://github.com/Zazza/imagecaster-server#readme',
}

export const serverState = reactive({
  state: '',
  url: '',
  version: '',
  missing: [],
  error: '',
  checking: false,
  dismissed: false,
})

export function serverReady() {
  return serverState.state === '' || serverState.state === 'ok'
}

setServerGate(serverReady)

export async function checkServer() {
  serverState.checking = true
  try {
    const r = await api.checkServer()
    serverState.state = r?.state || 'not_found'
    serverState.url = r?.url || ''
    serverState.version = r?.version || ''
    serverState.missing = r?.missing || []
    serverState.error = r?.error || ''
  } catch (e) {
    serverState.state = 'not_found'
    serverState.error = String(e)
  } finally {
    serverState.checking = false
  }
  if (serverState.state === 'ok') serverState.dismissed = false
  return serverState.state
}

export async function useServer(url) {
  const clean = String(url || '').trim().replace(/\/+$/, '')
  if (!clean) return serverState.state
  await api.updateSettings({ server_url: /^https?:\/\//.test(clean) ? clean : `http://${clean}` })
  return checkServer()
}
