import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base }
})

import AppFooter from '../components/AppFooter.vue'
import FileBrowserPage from '../components/FileBrowserPage.vue'
import ImageViewer from '../components/ImageViewer.vue'
import * as App from '../wailsjs/go/main/App.js'
import {
  ListSessions, GetActiveSessionItem, GetSessionItems, SetActiveSessionItem, DeleteSessionItem,
  GetSettings, BrowseDirectory, ReadFileAsBase64, ReadThumbnail, SetLastImage,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const OLD_REMIX = 'Ремикс'
const OLD_EDIT = 'Правка'
const OLD_CARD_POST = 'В постобработку'
const OLD_CARD_REMIX = 'В ремикс'

async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

function viewer(wrapper) {
  return wrapper.findComponent(ImageViewer)
}

function viewerButtons(wrapper, text) {
  return viewer(wrapper).findAll('button').filter(b => b.text().trim() === text)
}

function viewerLabels(wrapper) {
  return viewer(wrapper).findAll('button').map(b => b.text().trim())
}

function pressKey(key) {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

function navigatedToPost(wrapper) {
  return (wrapper.emitted('navigate') || []).filter(e => e[0] && e[0].page === 'postprocess')
}

function deleteMocksCalled() {
  return Object.entries(App)
    .filter(([name, fn]) => /^(Delete|Remove)/.test(name) && fn && fn.mock)
    .filter(([, fn]) => fn.mock.calls.length > 0)
    .map(([name]) => name)
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  vi.stubGlobal('IntersectionObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('VA1: history viewer actions', () => {
  const ITEMS = [
    { id: 301, source: 'generate', created_at: '2026-11-07T10:00:00Z', is_active: false },
    { id: 302, source: 'generate', created_at: '2026-11-07T10:01:00Z', is_active: false },
  ]

  beforeEach(() => {
    ListSessions.mockImplementation(() => Promise.resolve([
      { id: 7, name: 'Сессия', item_count: ITEMS.length, created_at: '', updated_at: '', is_active: true },
    ]))
    GetActiveSessionItem.mockImplementation(() => Promise.resolve(null))
    GetSessionItems.mockImplementation(() => Promise.resolve(ITEMS.map(i => ({ ...i }))))
  })

  async function openViewer(cardIndex = 0) {
    const wrapper = mount(AppFooter)
    await settle()
    const label = t('footer.session')
    const btn = wrapper.findAll('button').find(b => b.text().startsWith(label))
    expect(btn, `button "${label}"`).toBeTruthy()
    await btn.trigger('click')
    await settle()
    const cards = wrapper.findAll('.si-card')
    expect(cards.length).toBe(ITEMS.length)
    const id = Number(cards[cardIndex].attributes('data-item-id'))
    await cards[cardIndex].trigger('click')
    await settle()
    expect(viewer(wrapper).exists()).toBe(true)
    return { wrapper, id }
  }

  it('shows only post-processing and delete actions, no remix/edit', async () => {
    const { wrapper } = await openViewer()
    const labels = viewerLabels(wrapper)
    expect(labels).not.toContain(OLD_REMIX)
    expect(labels).not.toContain(OLD_EDIT)
    expect(viewerButtons(wrapper, t('viewer.postprocess')).length).toBeGreaterThan(0)
    expect(viewerButtons(wrapper, t('viewer.delete')).length).toBeGreaterThan(0)
  })

  it('every post-processing button opens this photo in post-processing', async () => {
    const { wrapper: probe } = await openViewer(1)
    const count = viewerButtons(probe, t('viewer.postprocess')).length
    probe.unmount()
    expect(count).toBeGreaterThan(0)

    for (let i = 0; i < count; i++) {
      vi.clearAllMocks()
      const { wrapper, id } = await openViewer(1)
      await viewerButtons(wrapper, t('viewer.postprocess'))[i].trigger('click')
      await settle()
      expect(SetActiveSessionItem).toHaveBeenCalledWith(id)
      const nav = navigatedToPost(wrapper)
      expect(nav).toHaveLength(1)
      expect(nav[0][0].section).toBeUndefined()
      expect(DeleteSessionItem).not.toHaveBeenCalled()
      wrapper.unmount()
    }
  })

  it('delete button needs a second press to confirm', async () => {
    const { wrapper, id } = await openViewer()
    await viewerButtons(wrapper, t('viewer.delete'))[0].trigger('click')
    await settle()
    expect(DeleteSessionItem).not.toHaveBeenCalled()
    const confirm = viewerButtons(wrapper, t('viewer.confirm_delete'))
    expect(confirm.length).toBeGreaterThan(0)
    await confirm[0].trigger('click')
    await settle()
    expect(DeleteSessionItem).toHaveBeenCalledTimes(1)
    expect(DeleteSessionItem).toHaveBeenCalledWith(id)
  })

  it('Delete key deletes with confirmation as before', async () => {
    const { wrapper, id } = await openViewer()
    pressKey('Delete')
    await settle()
    expect(DeleteSessionItem).not.toHaveBeenCalled()
    expect(viewerButtons(wrapper, t('viewer.confirm_delete')).length).toBeGreaterThan(0)
    pressKey('Delete')
    await settle()
    expect(DeleteSessionItem).toHaveBeenCalledWith(id)
  })
})

describe('VA2/VA3: My Images', () => {
  const DIR = '/pics'
  const ENTRIES = [
    { name: 'one.png', path: '/pics/one.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
    { name: 'two.png', path: '/pics/two.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
    { name: 'three.png', path: '/pics/three.png', is_dir: false, size: 100, mod_time: '2026-11-07 10:00' },
  ]
  const B64 = {
    '/pics/one.png': 'T05FX0lNRw==',
    '/pics/two.png': 'VFdPX0lNRw==',
    '/pics/three.png': 'VEhSRUVfSU1H',
  }

  beforeEach(() => {
    GetSettings.mockImplementation(() => Promise.resolve({ file_browser_path: DIR }))
    BrowseDirectory.mockImplementation(() => Promise.resolve(ENTRIES.map(e => ({ ...e }))))
    ReadFileAsBase64.mockImplementation(p => Promise.resolve(B64[p]))
    ReadThumbnail.mockImplementation(p => Promise.resolve(B64[p]))
  })

  async function mountPage() {
    const wrapper = mount(FileBrowserPage)
    await settle()
    expect(imageCards(wrapper)).toHaveLength(ENTRIES.length)
    return wrapper
  }

  function imageCards(wrapper) {
    return wrapper.findAll('.fb-card').filter(c => !c.classes().includes('fb-card-dir'))
  }

  function cardPath(card) {
    const name = card.find('.fb-card-name').text().trim()
    return ENTRIES.find(e => e.name === name).path
  }

  async function openAt(wrapper, index) {
    await imageCards(wrapper)[index].trigger('click')
    await settle()
    expect(viewer(wrapper).exists()).toBe(true)
  }

  async function next(wrapper) {
    const btn = viewer(wrapper).find('.image-viewer-next')
    expect(btn.exists(), 'next arrow').toBe(true)
    await btn.trigger('click')
    await settle()
  }

  it('VA2: post-processing after paging right sends the second photo', async () => {
    const wrapper = await mountPage()
    const second = cardPath(imageCards(wrapper)[1])
    await openAt(wrapper, 0)
    await next(wrapper)

    const post = viewerButtons(wrapper, t('viewer.postprocess'))
    expect(post.length).toBeGreaterThan(0)
    SetLastImage.mockClear()
    await post[0].trigger('click')
    await settle()

    expect(SetLastImage).toHaveBeenCalledTimes(1)
    expect(SetLastImage.mock.calls[0][0]).toBe(B64[second])
    const nav = navigatedToPost(wrapper)
    expect(nav).toHaveLength(1)
    expect(nav[0][0].section).toBeUndefined()
  })

  it('VA2: post-processing without paging sends the opened photo', async () => {
    const wrapper = await mountPage()
    const third = cardPath(imageCards(wrapper)[2])
    await openAt(wrapper, 2)
    SetLastImage.mockClear()
    await viewerButtons(wrapper, t('viewer.postprocess'))[0].trigger('click')
    await settle()
    expect(SetLastImage).toHaveBeenCalledTimes(1)
    expect(SetLastImage.mock.calls[0][0]).toBe(B64[third])
    expect(navigatedToPost(wrapper)).toHaveLength(1)
  })

  it('VA2: viewer has no delete button and no remix/edit', async () => {
    const wrapper = await mountPage()
    await openAt(wrapper, 0)
    const labels = viewerLabels(wrapper)
    expect(labels).not.toContain(t('viewer.delete'))
    expect(labels).not.toContain(t('viewer.confirm_delete'))
    expect(labels).not.toContain(OLD_REMIX)
    expect(labels).not.toContain(OLD_EDIT)
  })

  it('VA2: Delete key does nothing', async () => {
    const wrapper = await mountPage()
    await openAt(wrapper, 0)
    const srcBefore = viewer(wrapper).find('img').attributes('src')
    pressKey('Delete')
    await settle()
    pressKey('Delete')
    await settle()
    expect(deleteMocksCalled()).toEqual([])
    expect(viewer(wrapper).exists()).toBe(true)
    expect(viewer(wrapper).find('img').attributes('src')).toBe(srcBefore)
    expect(viewerLabels(wrapper)).not.toContain(t('viewer.confirm_delete'))
    expect(imageCards(wrapper)).toHaveLength(ENTRIES.length)
    expect(wrapper.emitted('navigate')).toBeFalsy()
  })

  it('VA3/U1: thumbnails have no action buttons (no "to post-processing", no "to remix")', async () => {
    const wrapper = await mountPage()
    for (const card of imageCards(wrapper)) {
      expect(card.findAll('button')).toHaveLength(0)
      const titles = card.findAll('[title]').map(el => el.attributes('title'))
      expect(titles).not.toContain(OLD_CARD_POST)
      expect(titles).not.toContain(OLD_CARD_REMIX)
      expect(card.text()).not.toContain(OLD_CARD_POST)
      expect(card.text()).not.toContain(OLD_CARD_REMIX)
    }
    expect(wrapper.find('.fb-card-actions').exists()).toBe(false)
  })

  it('VA3: clicking a thumbnail opens the viewer and does not navigate', async () => {
    const wrapper = await mountPage()
    expect(viewer(wrapper).exists()).toBe(false)
    await openAt(wrapper, 1)
    expect(wrapper.emitted('navigate')).toBeFalsy()
    expect(SetLastImage).not.toHaveBeenCalled()
  })

  it('VA3: select mode works as before — marks instead of opening, sends selection to post-processing', async () => {
    const wrapper = await mountPage()
    const selectBtn = wrapper.findAll('button').find(b => b.text().trim() === t('browser.select_mode'))
    expect(selectBtn, 'select mode button').toBeTruthy()
    await selectBtn.trigger('click')
    await settle()

    await imageCards(wrapper)[0].trigger('click')
    await settle()
    await imageCards(wrapper)[2].trigger('click')
    await settle()
    expect(viewer(wrapper).exists()).toBe(false)
    expect(wrapper.text()).toContain(t('browser.selected_count', { n: 2 }))

    const barButtons = wrapper.find('.fb-select-bar').findAll('button').map(b => b.text().trim())
    expect(barButtons).toEqual([t('browser.send_to_postprocess_multi', { n: 2 }), t('browser.cancel_select')])
    const postMulti = wrapper.findAll('button').find(b => b.text().includes(t('browser.send_to_postprocess_multi', { n: 2 })))
    expect(postMulti).toBeTruthy()
    await postMulti.trigger('click')
    await settle()
    expect(navigatedToPost(wrapper)).toHaveLength(1)
  })

  it('U2/VA4: removed i18n keys are gone', async () => {
    const ru = (await import('../i18n/ru.js')).ru
    const en = (await import('../i18n/en.js')).en
    for (const dict of [ru, en]) {
      for (const key of ['viewer.remix', 'viewer.edit', 'browser.send_to_postprocess', 'browser.send_to_remix', 'browser.send_to_remix_multi']) {
        expect(dict[key], key).toBeUndefined()
      }
    }
  })
})
