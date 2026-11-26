import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import SessionPanel from '../components/footer/SessionPanel.vue'
import ImageViewer from '../components/ImageViewer.vue'
import { openInHistoryViewer } from '../historyViewer.js'
import { ListSessions, GetSessionItems, OpenInHistory } from '../wailsjs/go/main/App.js'
import { clearEventMocks } from './mocks/runtime'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

let activeSession
const groups = {
  11: [{ id: 101, prompt: 'First frame' }],
  22: [{ id: 201, prompt: 'Original photo' }, { id: 202, prompt: 'Variant' }, { id: 203, prompt: 'Next variant' }],
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  activeSession = 11
  ListSessions.mockImplementation(async () => Object.keys(groups).map(id => ({ id: Number(id), is_active: Number(id) === activeSession })))
  GetSessionItems.mockImplementation(async () => groups[activeSession])
  OpenInHistory.mockImplementation(async id => {
    activeSession = Number(Object.keys(groups).find(group => groups[group].some(item => item.id === id)))
  })
})

describe('Shared history viewer', () => {
  it('opens an item in the selected group without changing the active image', async () => {
    const wrapper = mount(SessionPanel, { props: { open: true } })
    await settle()
    expect(await openInHistoryViewer(101)).toBe(true)
    await settle()
    expect(OpenInHistory).not.toHaveBeenCalled()
    const viewer = wrapper.getComponent(ImageViewer)
    expect(viewer.props('item').id).toBe(101)
    expect(viewer.props('showActions')).toBe(true)
  })

  it('loads the owning group and opens the full viewer with metadata, navigation and actions', async () => {
    const wrapper = mount(SessionPanel, { props: { open: true } })
    await settle()
    expect(await openInHistoryViewer(202)).toBe(true)
    await settle()
    expect(OpenInHistory).toHaveBeenCalledWith(202)
    expect(wrapper.get('select').element.value).toBe('22')
    const viewer = wrapper.getComponent(ImageViewer)
    expect(viewer.props()).toMatchObject({ item: { id: 202, prompt: 'Variant' }, position: 2, total: 3, showActions: true, hasPrev: true, hasNext: true })
    await wrapper.get('.image-viewer-prev').trigger('click')
    expect(viewer.props('item').id).toBe(201)
    expect(viewer.props('hasPrev')).toBe(false)
    await wrapper.get('.image-viewer-next').trigger('click')
    await wrapper.get('.image-viewer-next').trigger('click')
    expect(viewer.props('item').id).toBe(203)
    expect(viewer.props('hasNext')).toBe(false)
    expect(wrapper.find('.iv-delete').exists()).toBe(true)
    expect(wrapper.find('.iv-info-toggle').exists()).toBe(true)
  })
})
