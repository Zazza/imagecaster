import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import DownloadProgress from '../components/DownloadProgress.vue'
import { api } from '../api.js'
import { defaultDownloads, runDefaultPlan } from '../modelDownloads.js'

enableAutoUnmount(afterEach)

describe('DownloadProgress', () => {
  it('shows percent, sizes and bar width', () => {
    const w = mount(DownloadProgress, { props: { percent: 42.7, text: '1.0 GB / 2.4 GB', label: 'File 2 of 3: a.gguf' } })
    expect(w.find('.dl-progress-pct').text()).toBe('42%')
    expect(w.find('.dl-progress-text').text()).toBe('1.0 GB / 2.4 GB')
    expect(w.find('.dl-progress-label').text()).toBe('File 2 of 3: a.gguf')
    expect(w.find('.dl-progress-bar').attributes('style')).toContain('width: 42.7%')
  })

  it('runs without a percent', () => {
    const w = mount(DownloadProgress, { props: { percent: null, text: 'pulling manifest' } })
    expect(w.find('.dl-progress-pct').text()).toBe('…')
    expect(w.find('.dl-progress-bar.running').exists()).toBe(true)
    expect(w.find('.dl-progress-label').exists()).toBe(false)
  })
})

describe('runDefaultPlan progress', () => {
  afterEach(() => vi.restoreAllMocks())

  it('tracks file step and percent, then resets', async () => {
    const seen = []
    vi.spyOn(api, 'getDefaultDownloadPlan').mockResolvedValue({ items: [], total_gb: 0 })
    vi.spyOn(api, 'downloadServerModelStream').mockImplementation(async (_u, _t, _url, filename, onProgress) => {
      onProgress('1.0 GB / 4.0 GB', 25)
      seen.push({ step: defaultDownloads.step, steps: defaultDownloads.steps, file: defaultDownloads.file, percent: defaultDownloads.percent, text: defaultDownloads.progress, filename })
    })
    defaultDownloads.plan = { items: [{ type: 'sd', url: 'u1', filename: 'a.gguf' }, { type: 'vae', url: 'u2', filename: 'b.safetensors' }], total_gb: 5 }

    await runDefaultPlan('http://srv')

    expect(seen).toEqual([
      { step: 1, steps: 2, file: 'a.gguf', percent: 25, text: '1.0 GB / 4.0 GB', filename: 'a.gguf' },
      { step: 2, steps: 2, file: 'b.safetensors', percent: 25, text: '1.0 GB / 4.0 GB', filename: 'b.safetensors' },
    ])
    expect(defaultDownloads.running).toBe(false)
    expect(defaultDownloads.percent).toBe(null)
    expect(defaultDownloads.steps).toBe(0)
  })
})
