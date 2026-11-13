import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import AppFooter from '../components/AppFooter.vue'
import { t, setLocale } from '../i18n/index.js'
import { GetSettings, GetServerStatus } from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  GetSettings.mockImplementation(() => Promise.resolve({}))
})

function llmDot(w) {
  const dot = w.findAll('.status-dot').find(d => d.text().includes(t('footer.ai_assistant')))
  expect(dot).toBeTruthy()
  return dot
}

describe('footer LLM availability uses health.llm only', () => {
  it.each([
    ['llm healthy', { llm: { healthy: true } }, true],
    ['only ollama healthy', { ollama: { healthy: true } }, false],
    ['llm and ollama healthy', { llm: { healthy: true }, ollama: { healthy: true } }, true],
    ['llm down, ollama healthy', { llm: { healthy: false }, ollama: { healthy: true } }, false],
    ['llm healthy, ollama down', { llm: { healthy: true }, ollama: { healthy: false } }, true],
    ['both down', { llm: { healthy: false }, ollama: { healthy: false } }, false],
    ['no health info', {}, false],
  ])('%s', async (_, health, online) => {
    GetServerStatus.mockImplementation(() => Promise.resolve({ health, models: {} }))
    const w = mount(AppFooter)
    await flushPromises()
    expect(llmDot(w).classes('online')).toBe(online)
  })
})
