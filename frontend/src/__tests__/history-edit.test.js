import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { EventsEmit, clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import EditPage from '../components/EditPage.vue'
import { GetActiveSessionItem, GetSessionImage } from '../wailsjs/go/main/App.js'

const sessionItem = { id: 42, info: 'null', is_preview: false }
const sessionImageB64 = 'c2Vzc2lvbi1pbWFnZQ=='

function sourceImg(wrapper) {
  return wrapper.find('img.source-img')
}

enableAutoUnmount(afterEach)

describe('History → Edit (active session item)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearEventMocks()
  })

  it('loads active item image on mount', async () => {
    GetActiveSessionItem.mockReturnValue(Promise.resolve(sessionItem))
    GetSessionImage.mockReturnValue(Promise.resolve(sessionImageB64))

    const wrapper = mount(EditPage)
    await flushPromises()

    expect(sourceImg(wrapper).attributes('src')).toBe('data:image/png;base64,' + sessionImageB64)
  })

  it('reloads image when user picks another item from History while page is already open', async () => {
    GetActiveSessionItem.mockReturnValueOnce(Promise.resolve(null))
    GetActiveSessionItem.mockReturnValue(Promise.resolve(sessionItem))
    GetSessionImage.mockReturnValue(Promise.resolve(sessionImageB64))

    const wrapper = mount(EditPage)
    await flushPromises()
    expect(sourceImg(wrapper).exists()).toBe(false)

    EventsEmit('session:selected', { id: sessionItem.id })
    await flushPromises()

    expect(sourceImg(wrapper).attributes('src')).toBe('data:image/png;base64,' + sessionImageB64)
  })

  it('does not reload on session:added / session:active (generation must not clobber the edit source)', async () => {
    GetActiveSessionItem.mockReturnValueOnce(Promise.resolve(null))
    GetActiveSessionItem.mockReturnValue(Promise.resolve(sessionItem))
    GetSessionImage.mockReturnValue(Promise.resolve(sessionImageB64))

    const wrapper = mount(EditPage)
    await flushPromises()
    expect(GetActiveSessionItem).toHaveBeenCalledTimes(1)
    expect(sourceImg(wrapper).exists()).toBe(false)

    EventsEmit('session:added', { id: sessionItem.id })
    EventsEmit('session:active', { id: sessionItem.id })
    await flushPromises()

    expect(GetActiveSessionItem).toHaveBeenCalledTimes(1)
    expect(sourceImg(wrapper).exists()).toBe(false)
  })
})
