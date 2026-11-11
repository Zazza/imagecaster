import { reactive } from 'vue'
import { api } from './api.js'

export const defaultDownloads = reactive({
  plan: null,
  running: false,
  progress: '',
  failed: [],
  finished: false,
})

export async function loadDefaultPlan() {
  try {
    defaultDownloads.plan = await api.getDefaultDownloadPlan()
  } catch {
    defaultDownloads.plan = null
  }
  return defaultDownloads.plan
}

export async function runDefaultPlan(serverURL) {
  const plan = defaultDownloads.plan
  if (defaultDownloads.running || !plan || !plan.items || !plan.items.length) return []
  defaultDownloads.running = true
  defaultDownloads.finished = false
  defaultDownloads.failed = []
  try {
    for (const [i, f] of plan.items.entries()) {
      const head = `${i + 1}/${plan.items.length} ${f.filename}`
      defaultDownloads.progress = head
      try {
        if (serverURL) {
          await api.downloadServerModelStream(serverURL, f.type, f.url, f.filename, (line) => {
            defaultDownloads.progress = head + ': ' + line
          })
        } else {
          await api.downloadServerModel(f.type, f.url, f.filename)
        }
      } catch (e) {
        defaultDownloads.failed.push(`${f.filename}: ${String(e)}`)
      }
    }
  } finally {
    defaultDownloads.running = false
    defaultDownloads.progress = ''
    defaultDownloads.finished = true
    await loadDefaultPlan()
  }
  return defaultDownloads.failed
}
