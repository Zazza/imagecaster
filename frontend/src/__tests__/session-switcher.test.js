import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import AppFooter from '../components/AppFooter.vue'
import { t } from '../i18n/index.js'
import { ListSessions, GetActiveSessionItem, GetSessionItems } from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)

const sessions = [
  { id: 11, name: 'Первая', item_count: 0, created_at: '', updated_at: '', is_active: false },
  { id: 22, name: 'Вторая', item_count: 0, created_at: '', updated_at: '', is_active: true },
]

async function openHistory(wrapper) {
  const label = t('footer.session')
  const btn = wrapper.findAll('button').find(b => b.text().startsWith(label))
  expect(btn, `button "${label}"`).toBeTruthy()
  await btn.trigger('click')
  await flushPromises()
}

describe('AppFooter session switcher', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
  })

  it('selects the session marked is_active when there is no active item', async () => {
    ListSessions.mockImplementation(() => Promise.resolve(sessions.map(s => ({ ...s }))))
    GetActiveSessionItem.mockImplementation(() => Promise.resolve(null))
    GetSessionItems.mockImplementation(() => Promise.resolve([]))

    const wrapper = mount(AppFooter)
    await flushPromises()
    await openHistory(wrapper)

    const select = wrapper.find('select')
    expect(select.exists()).toBe(true)
    expect(select.element.value).toBe('22')
  })
})
