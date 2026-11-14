import { vi, beforeEach } from 'vitest'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

beforeEach(async () => {
  const [{ resetQueueState }, { resetPresetsState }] = await Promise.all([
    import('../composables/useQueue.js'),
    import('../composables/usePresets.js'),
  ])
  resetQueueState()
  resetPresetsState()
})
