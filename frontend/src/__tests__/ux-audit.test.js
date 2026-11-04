import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import FileBrowserPage from '../components/FileBrowserPage.vue'
import SettingsPage from '../components/SettingsPage.vue'
import ImportModal from '../components/ImportModal.vue'
import PipelineImportModal from '../components/PipelineImportModal.vue'
import PinModal from '../components/PinModal.vue'
import PresetsPage from '../components/PresetsPage.vue'
import { t } from '../i18n/index.js'
import {
  BrowseDirectory,
  GetSettings,
  GetServerStatus,
  ListPresets,
  ListPresetTypes,
  GetPresetsInstallStatus,
} from '../wailsjs/go/main/App.js'

enableAutoUnmount(afterEach)

let renderErrors = []

function mountPage(component, options = {}) {
  return mount(component, {
    ...options,
    global: {
      ...(options.global || {}),
      config: {
        errorHandler: (err) => { renderErrors.push(err) },
      },
    },
  })
}

async function settle() {
  for (let i = 0; i < 5; i++) {
    await flushPromises()
  }
}

async function clickBackdrop(wrapper) {
  const overlay = wrapper.find('.modal-overlay')
  expect(overlay.exists()).toBe(true)
  await overlay.trigger('mousedown')
  await overlay.trigger('mouseup')
  await overlay.trigger('click')
  return overlay
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  renderErrors = []
  BrowseDirectory.mockImplementation(() => Promise.resolve([]))
  GetSettings.mockImplementation(() => Promise.resolve({}))
  GetServerStatus.mockImplementation(() => Promise.resolve(undefined))
  ListPresets.mockImplementation(() => Promise.resolve([]))
  ListPresetTypes.mockImplementation(() => Promise.resolve([]))
  GetPresetsInstallStatus.mockImplementation(() => Promise.resolve([]))
  vi.stubGlobal('IntersectionObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('U1: My images page tolerates null directory listing', () => {
  it('renders the empty state without errors when browseDirectory resolves null', async () => {
    GetSettings.mockImplementation(() => Promise.resolve({ file_browser_path: '/some/empty/folder' }))
    BrowseDirectory.mockImplementation(() => Promise.resolve(null))

    const wrapper = mountPage(FileBrowserPage)
    await settle()

    expect(BrowseDirectory).toHaveBeenCalledWith('/some/empty/folder')
    expect(renderErrors).toEqual([])
    expect(wrapper.find('.file-browser').exists()).toBe(true)
    expect(wrapper.text()).toContain(t('browser.no_images'))
  })

  it('control: renders the empty state for an empty array', async () => {
    GetSettings.mockImplementation(() => Promise.resolve({ file_browser_path: '/some/empty/folder' }))
    BrowseDirectory.mockImplementation(() => Promise.resolve([]))

    const wrapper = mountPage(FileBrowserPage)
    await settle()

    expect(renderErrors).toEqual([])
    expect(wrapper.text()).toContain(t('browser.no_images'))
  })
})

describe('U2: Settings server status tolerates missing processes', () => {
  const serverSettings = { connection_mode: 'server', server_url: 'http://127.0.0.1:9999' }

  it.each([
    ['processes is null', { processes: null }],
    ['processes is absent', {}],
  ])('renders the server status block without errors when %s', async (_label, status) => {
    GetSettings.mockImplementation(() => Promise.resolve(serverSettings))
    GetServerStatus.mockImplementation(() => Promise.resolve(status))

    const wrapper = mountPage(SettingsPage, { props: { initialTab: 'connection' } })
    await settle()

    expect(GetServerStatus).toHaveBeenCalled()
    expect(renderErrors).toEqual([])
    expect(wrapper.text()).toContain(t('settings.section_server_status'))
  })

  it('control: renders the server status block with a populated processes map', async () => {
    GetSettings.mockImplementation(() => Promise.resolve(serverSettings))
    GetServerStatus.mockImplementation(() => Promise.resolve({
      processes: { sd: { name: 'sd', category: 'service', status: 'running' } },
    }))

    const wrapper = mountPage(SettingsPage, { props: { initialTab: 'connection' } })
    await settle()

    expect(renderErrors).toEqual([])
    expect(wrapper.text()).toContain(t('settings.section_server_status'))
  })
})

describe('U3: modals do not close on backdrop click', () => {
  it('ImportModal does not emit close on overlay mousedown+click', async () => {
    const wrapper = mountPage(ImportModal, {
      props: { presets: [{ name: 'Moss', source_file: 'moss.json' }] },
    })
    await settle()

    await clickBackdrop(wrapper)

    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('done')).toBeUndefined()
  })

  it('ImportModal still closes via its close button', async () => {
    const wrapper = mountPage(ImportModal, {
      props: { presets: [{ name: 'Moss', source_file: 'moss.json' }] },
    })
    await settle()

    await wrapper.find('.modal-close').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('PipelineImportModal does not emit close on overlay mousedown+click', async () => {
    const wrapper = mountPage(PipelineImportModal, {
      props: { pipelines: [{ name: 'Pipe', description: 'desc', steps: [] }] },
    })
    await settle()

    await clickBackdrop(wrapper)

    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('done')).toBeUndefined()
  })

  it('PipelineImportModal still closes via its close button', async () => {
    const wrapper = mountPage(PipelineImportModal, {
      props: { pipelines: [{ name: 'Pipe', description: 'desc', steps: [] }] },
    })
    await settle()

    await wrapper.find('.modal-close').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it.each(['set', 'enter'])('PinModal (%s) does not emit cancel on overlay mousedown+click', async (mode) => {
    const wrapper = mountPage(PinModal, { props: { mode } })
    await settle()

    await clickBackdrop(wrapper)

    expect(wrapper.emitted('cancel')).toBeUndefined()
    expect(wrapper.emitted('confirm')).toBeUndefined()
  })

  it('PinModal still cancels via its close button', async () => {
    const wrapper = mountPage(PinModal, { props: { mode: 'enter' } })
    await settle()

    await wrapper.find('.modal-close').trigger('click')

    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })

  describe('PresetsPage delete confirmation', () => {
    const PRESETS = [
      { id: 11, name: 'Alpha style', prompt: 'a', sampler: 'Euler', steps: 20, cfg_scale: 7, preset_type: 'general' },
      { id: 12, name: 'Bravo style', prompt: 'b', sampler: 'Euler', steps: 20, cfg_scale: 7, preset_type: 'general' },
    ]

    function findButton(wrapper, label) {
      return wrapper.findAll('button').find(b => b.text().trim() === label)
    }

    async function openDeleteConfirm() {
      ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
      const wrapper = mountPage(PresetsPage, { attachTo: document.body })
      await settle()

      const selectBtn = findButton(wrapper, t('presets.btn_select'))
      expect(selectBtn).toBeTruthy()
      await selectBtn.trigger('click')

      const checkbox = wrapper.find('.preset-card input[type="checkbox"]')
      expect(checkbox.exists()).toBe(true)
      await checkbox.setValue(true)

      const deleteSelected = findButton(wrapper, t('presets.btn_delete_selected', { count: 1 }))
      expect(deleteSelected).toBeTruthy()
      await deleteSelected.trigger('click')
      await settle()

      const title = wrapper.findAll('.modal-title').find(el => el.text() === t('presets.delete_title'))
      expect(title).toBeTruthy()
      return wrapper
    }

    function confirmOverlay(wrapper) {
      return wrapper.findAll('.modal-overlay').find(o => o.text().includes(t('presets.delete_title')))
    }

    it('stays open after overlay mousedown+click', async () => {
      const wrapper = await openDeleteConfirm()

      const overlay = confirmOverlay(wrapper)
      expect(overlay).toBeTruthy()
      await overlay.trigger('mousedown')
      await overlay.trigger('mouseup')
      await overlay.trigger('click')
      await settle()

      expect(confirmOverlay(wrapper)).toBeTruthy()
      expect(renderErrors).toEqual([])
    })

    it('still closes via its cancel button', async () => {
      const wrapper = await openDeleteConfirm()

      const overlay = confirmOverlay(wrapper)
      const cancel = overlay.findAll('button').find(b => b.text().trim() === t('presets.btn_cancel'))
      expect(cancel).toBeTruthy()
      await cancel.trigger('click')
      await settle()

      expect(confirmOverlay(wrapper)).toBeFalsy()
    })
  })
})
