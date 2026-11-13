import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import LLMSetupModal from '../components/LLMSetupModal.vue'
import { GetSettings } from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

async function settle() {
  for (let i = 0; i < 6; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

const SETUP = {
  needed: true,
  generate: [
    { name: 'gen-inst:3b', installed: true, recommended: false },
    { name: 'gen-rec-inst:8b', installed: true, recommended: true },
    { name: 'gen-new:4b', installed: false, recommended: false, size_gb: 2.5 },
    { name: 'gen-rec-new:9b', installed: false, recommended: true, size_gb: 5.2 },
  ],
  vision: [
    { name: 'vis-inst:3b', installed: true, recommended: false },
    { name: 'vis-rec-new:8b', installed: false, recommended: true, size_gb: 6.2 },
  ],
  generate_selected: '',
  vision_selected: '',
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  vi.stubGlobal('confirm', () => false)
  vi.stubGlobal('alert', () => {})
  GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

async function mountModal() {
  const w = mount(LLMSetupModal, { props: { setup: JSON.parse(JSON.stringify(SETUP)) }, attachTo: document.body })
  await settle()
  return w
}

function optionText(w, selectIndex, name) {
  const sel = w.findAll('select')[selectIndex]
  expect(sel, `select ${selectIndex}`).toBeTruthy()
  const opt = sel.findAll('option').find(o => o.text().includes(name))
  expect(opt, `option ${name}`).toBeTruthy()
  return opt.text().replace(/\s+/g, ' ').trim()
}

describe.each(['ru', 'en'])('LLMSetupModal option texts (condition 3, %s)', (locale) => {
  beforeEach(() => setLocale(locale))

  it('installed model reads "<name> — installed"', async () => {
    const w = await mountModal()
    expect(optionText(w, 0, 'gen-inst:3b')).toBe(`gen-inst:3b — ${t('llm_setup.installed')}`)
    expect(optionText(w, 1, 'vis-inst:3b')).toBe(`vis-inst:3b — ${t('llm_setup.installed')}`)
  })

  it('not installed model reads "<name> — download, size"', async () => {
    const w = await mountModal()
    expect(optionText(w, 0, 'gen-new:4b')).toBe(`gen-new:4b — ${t('llm_setup.download', { size: 2.5 })}`)
  })

  it('recommended model gets ", recommended" appended', async () => {
    const w = await mountModal()
    const rec = t('llm_setup.recommended')
    expect(optionText(w, 0, 'gen-rec-inst:8b')).toBe(`gen-rec-inst:8b — ${t('llm_setup.installed')}, ${rec}`)
    expect(optionText(w, 0, 'gen-rec-new:9b')).toBe(`gen-rec-new:9b — ${t('llm_setup.download', { size: 5.2 })}, ${rec}`)
    expect(optionText(w, 1, 'vis-rec-new:8b')).toBe(`vis-rec-new:8b — ${t('llm_setup.download', { size: 6.2 })}, ${rec}`)
  })

  it('not recommended model has no recommended mark', async () => {
    const w = await mountModal()
    expect(optionText(w, 0, 'gen-inst:3b')).not.toContain(t('llm_setup.recommended'))
    expect(optionText(w, 0, 'gen-new:4b')).not.toContain(t('llm_setup.recommended'))
  })
})
