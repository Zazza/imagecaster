import { reactive } from 'vue'
import { api } from './api.js'

export const defaultDownloads = reactive({
  plan: null,
  running: false,
  progress: '',
  percent: null,
  file: '',
  step: 0,
  steps: 0,
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
    defaultDownloads.steps = plan.items.length
    for (const [i, f] of plan.items.entries()) {
      defaultDownloads.step = i + 1
      defaultDownloads.file = f.filename
      defaultDownloads.progress = ''
      defaultDownloads.percent = null
      try {
        if (serverURL) {
          await api.downloadServerModelStream(serverURL, f.type, f.url, f.filename, (text, percent) => {
            defaultDownloads.progress = text
            defaultDownloads.percent = percent
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
    defaultDownloads.percent = null
    defaultDownloads.file = ''
    defaultDownloads.step = 0
    defaultDownloads.steps = 0
    defaultDownloads.finished = true
    await loadDefaultPlan()
  }
  return defaultDownloads.failed
}
