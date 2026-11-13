import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'

const openURL = vi.hoisted(() => vi.fn())

vi.mock('../wailsjs/runtime/runtime', async () => ({ ...(await import('./mocks/runtime')), BrowserOpenURL: openURL }))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  const extra = {}
  for (const n of [
    'GenerateSDPrompt', 'GenerateImage', 'UpscaleImage', 'AnalyzeImage', 'GenerateCompoundImage',
    'EditImage', 'EditObject', 'RepairImage', 'DescribeForStyle', 'ApplyLookDirect',
    'EnqueueTxt2Img', 'EnqueueFromImage', 'EnqueueLook', 'EnqueueEdit', 'EnqueueCompound', 'RetryQueueJob',
    'ListSessions',
  ]) extra[n] = vi.fn(() => Promise.resolve('called'))
  return {
    ...base,
    ...extra,
    GetSettings: vi.fn(() => Promise.resolve({ ok: 1 })),
    UpdateSettings: vi.fn(() => Promise.resolve(undefined)),
    DiscoverServers: vi.fn(() => Promise.resolve([])),
    CheckServer: vi.fn(() => Promise.resolve({ state: 'ok', url: 'http://s:1', version: '2', missing: [], error: '' })),
  }
})

enableAutoUnmount(afterEach)

const GEN = {
  generateSdPrompt: 'GenerateSDPrompt', generateImage: 'GenerateImage', upscaleImage: 'UpscaleImage',
  analyzeImage: 'AnalyzeImage', generateCompoundImage: 'GenerateCompoundImage', editImage: 'EditImage',
  editObject: 'EditObject', repairImage: 'RepairImage', describeForStyle: 'DescribeForStyle',
  applyLookDirect: 'ApplyLookDirect', enqueueTxt2Img: 'EnqueueTxt2Img', enqueueFromImage: 'EnqueueFromImage',
  enqueueLook: 'EnqueueLook', enqueueEdit: 'EnqueueEdit', enqueueCompound: 'EnqueueCompound',
  retryQueueJob: 'RetryQueueJob',
}

async function load() {
  vi.resetModules()
  const App = await import('../wailsjs/go/main/App.js')
  const { api } = await import('../api.js')
  const ss = await import('../serverState.js')
  const i18n = await import('../i18n/index.js')
  i18n.setLocale('ru')
  return { App, api, ss, i18n }
}

function resp(state, extra = {}) {
  return { state, url: 'http://srv:7860', version: '1.0', missing: [], error: '', ...extra }
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
})

describe('serverState.checkServer (condition 1)', () => {
  it.each(['ok', 'not_configured', 'not_found', 'outdated'])('fills serverState and returns state %s', async (state) => {
    const { App, ss } = await load()
    App.CheckServer.mockResolvedValue(resp(state, { version: '3.1', missing: ['sdcpp'], error: 'e1' }))
    const got = await ss.checkServer()
    expect(got).toBe(state)
    expect(App.CheckServer).toHaveBeenCalledTimes(1)
    expect(ss.serverState.state).toBe(state)
    expect(ss.serverState.url).toBe('http://srv:7860')
    expect(ss.serverState.version).toBe('3.1')
    expect(ss.serverState.missing).toEqual(['sdcpp'])
    expect(ss.serverState.error).toBe('e1')
    expect(ss.serverState.checking).toBe(false)
  })

  it('api.checkServer returns the binding result', async () => {
    const { App, api } = await load()
    App.CheckServer.mockResolvedValue(resp('outdated'))
    expect(await api.checkServer()).toEqual(resp('outdated'))
  })

  it('a thrown error becomes not_found', async () => {
    const { App, ss } = await load()
    App.CheckServer.mockRejectedValue(new Error('boom'))
    expect(await ss.checkServer()).toBe('not_found')
    expect(ss.serverState.state).toBe('not_found')
    expect(ss.serverState.checking).toBe(false)
  })

  it('serverState has all documented fields', async () => {
    const { ss } = await load()
    for (const f of ['state', 'url', 'version', 'missing', 'error', 'checking', 'dismissed']) {
      expect(ss.serverState, f).toHaveProperty(f)
    }
  })

  it('serverReady is true before the first check, true on ok, false otherwise', async () => {
    const { App, ss } = await load()
    expect(ss.serverReady()).toBe(true)
    App.CheckServer.mockResolvedValue(resp('ok'))
    await ss.checkServer()
    expect(ss.serverReady()).toBe(true)
    for (const s of ['not_found', 'outdated', 'not_configured']) {
      App.CheckServer.mockResolvedValue(resp(s))
      await ss.checkServer()
      expect(ss.serverReady(), s).toBe(false)
    }
  })

  it('SERVER_INSTALL_URL has ru and en links', async () => {
    const { ss } = await load()
    expect(ss.SERVER_INSTALL_URL.ru).toMatch(/^https?:\/\//)
    expect(ss.SERVER_INSTALL_URL.en).toMatch(/^https?:\/\//)
  })
})

describe('serverState.useServer (condition 1)', () => {
  it.each([
    ['192.168.1.5:7860', 'http://192.168.1.5:7860'],
    ['http://h:1/', 'http://h:1'],
    ['https://h:1///', 'https://h:1'],
    ['h:2', 'http://h:2'],
  ])('saves %s as %s and re-checks', async (input, saved) => {
    const { App, ss } = await load()
    App.CheckServer.mockResolvedValue(resp('ok'))
    await ss.useServer(input)
    expect(App.UpdateSettings).toHaveBeenCalledTimes(1)
    expect(App.UpdateSettings.mock.calls[0][0]).toMatchObject({ server_url: saved })
    expect(App.CheckServer).toHaveBeenCalled()
    expect(App.UpdateSettings.mock.invocationCallOrder[0]).toBeLessThan(App.CheckServer.mock.invocationCallOrder.at(-1))
    expect(ss.serverState.state).toBe('ok')
  })

  it.each(['', '   '])('ignores empty input %j', async (input) => {
    const { App, ss } = await load()
    await ss.useServer(input)
    expect(App.UpdateSettings).not.toHaveBeenCalled()
  })
})

describe('limited mode gate (condition 2)', () => {
  it('before any check generation calls go through', async () => {
    const { App, api } = await load()
    for (const [m, b] of Object.entries(GEN)) {
      await expect(api[m]({}), m).resolves.toBe('called')
      expect(App[b], b).toHaveBeenCalled()
    }
  })

  it('when state is ok generation calls go through', async () => {
    const { App, api, ss } = await load()
    App.CheckServer.mockResolvedValue(resp('ok'))
    await ss.checkServer()
    for (const [m, b] of Object.entries(GEN)) {
      await expect(api[m]({}), m).resolves.toBe('called')
      expect(App[b], b).toHaveBeenCalled()
    }
  })

  it.each(['not_found', 'outdated', 'not_configured'])('state %s: generation calls reject with server.need_server and skip the binding', async (state) => {
    const { App, api, ss, i18n } = await load()
    App.CheckServer.mockResolvedValue(resp(state))
    await ss.checkServer()
    for (const [m, b] of Object.entries(GEN)) {
      let err
      try { await api[m]({}) } catch (e) { err = e }
      expect(err, m).toBeInstanceOf(Error)
      expect(err.message, m).toBe(i18n.t('server.need_server'))
      expect(App[b], b).not.toHaveBeenCalled()
    }
  })

  it('a thrown check (not_found) also blocks', async () => {
    const { App, api, ss } = await load()
    App.CheckServer.mockRejectedValue(new Error('x'))
    await ss.checkServer()
    await expect(api.generateImage(1)).rejects.toThrow()
    expect(App.GenerateImage).not.toHaveBeenCalled()
  })

  it('becomes unblocked again once the server is ok', async () => {
    const { App, api, ss } = await load()
    App.CheckServer.mockResolvedValue(resp('not_found'))
    await ss.checkServer()
    await expect(api.enqueueTxt2Img({})).rejects.toThrow()
    App.CheckServer.mockResolvedValue(resp('ok'))
    await ss.checkServer()
    await expect(api.enqueueTxt2Img({})).resolves.toBe('called')
  })

  it.each(['not_found', 'outdated', 'not_configured'])('non-generation calls are not blocked in %s', async (state) => {
    const { App, api, ss } = await load()
    App.CheckServer.mockResolvedValue(resp(state))
    await ss.checkServer()
    await expect(api.getSettings()).resolves.toEqual({ ok: 1 })
    await expect(api.listSessions()).resolves.toBe('called')
    expect(App.GetSettings).toHaveBeenCalled()
    expect(App.ListSessions).toHaveBeenCalled()
  })
})

async function mountModal(state, { url = 'http://srv:7860', found = [], locale = 'ru', extra = {} } = {}) {
  const env = await load()
  env.i18n.setLocale(locale)
  env.App.DiscoverServers.mockResolvedValue(found)
  env.App.CheckServer.mockResolvedValue(resp(state, { url, ...extra }))
  await env.ss.checkServer()
  env.App.CheckServer.mockClear()
  const Modal = (await import('../components/ServerCheckModal.vue')).default
  const w = mount(Modal, { attachTo: document.body })
  await flushPromises()
  await nextTick()
  return { ...env, w }
}

function btn(w, key, t) {
  const b = w.findAll('button').find(x => x.text().includes(t(key)))
  expect(b, `button ${key}`).toBeTruthy()
  return b
}

describe('ServerCheckModal (condition 3)', () => {
  it.each(['not_found', 'not_configured'])('state %s searches the network on open and lists servers', async (state) => {
    const { App, w } = await mountModal(state, { url: state === 'not_configured' ? '' : 'http://srv:7860', found: [{ name: 'Box', ip_address: '10.0.0.7', port: 7861 }] })
    expect(App.DiscoverServers).toHaveBeenCalled()
    expect(w.text()).toContain('10.0.0.7:7861')
  })

  it('shows nothing_found when the search finds nothing', async () => {
    const { i18n, w } = await mountModal('not_found', { found: [] })
    expect(w.text()).toContain(i18n.t('server.nothing_found'))
  })

  it('clicking a found server saves http://ip:port and emits ready when re-check is ok', async () => {
    const { App, w } = await mountModal('not_found', { found: [{ name: 'Box', ip_address: '10.0.0.7', port: 7861 }] })
    App.CheckServer.mockResolvedValue(resp('ok'))
    const item = w.findAll('button').find(b => b.text().includes('10.0.0.7:7861'))
    await item.trigger('click')
    await flushPromises()
    expect(App.UpdateSettings.mock.calls[0][0]).toMatchObject({ server_url: 'http://10.0.0.7:7861' })
    expect(App.CheckServer).toHaveBeenCalled()
    expect(w.emitted('ready')).toBeTruthy()
  })

  it('clicking a found server does not emit ready if re-check is not ok', async () => {
    const { App, w } = await mountModal('not_found', { found: [{ name: 'Box', ip_address: '10.0.0.7', port: 7861 }] })
    App.CheckServer.mockResolvedValue(resp('outdated'))
    await w.findAll('button').find(b => b.text().includes('10.0.0.7:7861')).trigger('click')
    await flushPromises()
    expect(App.UpdateSettings).toHaveBeenCalled()
    expect(w.emitted('ready')).toBeFalsy()
  })

  it('typed address + Connect saves it and emits ready on ok', async () => {
    const { App, i18n, w } = await mountModal('not_configured', { url: '' })
    App.CheckServer.mockResolvedValue(resp('ok'))
    await w.find('input').setValue('192.168.0.9:7860/')
    await btn(w, 'server.connect', i18n.t).trigger('click')
    await flushPromises()
    expect(App.UpdateSettings.mock.calls[0][0]).toMatchObject({ server_url: 'http://192.168.0.9:7860' })
    expect(w.emitted('ready')).toBeTruthy()
  })

  it('typed address + Connect with failing re-check does not emit ready', async () => {
    const { App, i18n, w } = await mountModal('not_found')
    App.CheckServer.mockResolvedValue(resp('not_found'))
    await w.find('input').setValue('h:1')
    await btn(w, 'server.connect', i18n.t).trigger('click')
    await flushPromises()
    expect(App.UpdateSettings).toHaveBeenCalled()
    expect(w.emitted('ready')).toBeFalsy()
  })

  it('Check again re-checks and emits ready on ok', async () => {
    const { App, i18n, w } = await mountModal('not_found')
    App.CheckServer.mockResolvedValue(resp('ok'))
    await btn(w, 'server.retry', i18n.t).trigger('click')
    await flushPromises()
    expect(App.CheckServer).toHaveBeenCalled()
    expect(w.emitted('ready')).toBeTruthy()
  })

  it('Check again does not emit ready when still not found', async () => {
    const { App, i18n, w } = await mountModal('not_found')
    App.CheckServer.mockResolvedValue(resp('not_found'))
    await btn(w, 'server.retry', i18n.t).trigger('click')
    await flushPromises()
    expect(App.CheckServer).toHaveBeenCalled()
    expect(w.emitted('ready')).toBeFalsy()
  })

  it.each(['ru', 'en'])('install guide opens SERVER_INSTALL_URL for locale %s', async (locale) => {
    const { i18n, ss, w } = await mountModal('not_found', { locale })
    await btn(w, 'server.how_to_install', i18n.t).trigger('click')
    expect(openURL).toHaveBeenCalledWith(ss.SERVER_INSTALL_URL[locale])
  })

  it('Close emits close', async () => {
    const { i18n, w } = await mountModal('not_found')
    await btn(w, 'server.close_limited', i18n.t).trigger('click')
    expect(w.emitted('close')).toBeTruthy()
  })

  it('Escape emits close', async () => {
    const { w } = await mountModal('not_found')
    w.element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(w.emitted('close')).toBeTruthy()
  })

  it('outdated does not search the network and shows no not_found text', async () => {
    const { App, i18n, w } = await mountModal('outdated', { extra: { reason: 'old_version' } })
    expect(w.text()).toContain(i18n.t('server.outdated_old_title'))
    expect(w.text()).not.toContain(i18n.t('server.not_found_title'))
    expect(App.DiscoverServers).not.toHaveBeenCalled()
  })
})

const FEATURE_KEY = { sdcpp: 'server.feature_sdcpp', download_stream: 'server.feature_download_stream', llamacpp: 'server.feature_llm' }

function featureText(t, missing) {
  return [...new Set(missing.map(n => FEATURE_KEY[n] ? t(FEATURE_KEY[n]) : n))].join(', ')
}

const OUTDATED_TITLES = ['server.outdated_old_title', 'server.outdated_forge_title', 'server.outdated_incompatible_title']

describe('ServerCheckModal outdated reasons', () => {
  const cases = [
    ['old_version', ['sdcpp', 'download_stream', 'llamacpp'], 'server.outdated_old_title', 'server.outdated_old_lead', true],
    ['no_sdcpp', ['sdcpp'], 'server.outdated_forge_title', 'server.outdated_forge_lead', false],
    ['missing_features', ['download_stream', 'llamacpp'], 'server.outdated_incompatible_title', 'server.outdated_incompatible_lead', false],
    ['', ['download_stream', 'llamacpp'], 'server.outdated_incompatible_title', 'server.outdated_incompatible_lead', false],
    ['something_new', ['download_stream', 'llamacpp'], 'server.outdated_incompatible_title', 'server.outdated_incompatible_lead', false],
    [undefined, ['download_stream', 'llamacpp'], 'server.outdated_incompatible_title', 'server.outdated_incompatible_lead', false],
  ]
  for (const locale of ['ru', 'en']) {
    it.each(cases)(`${locale}: reason %j shows its title, lead and models note`, async (reason, missing, title, lead, models) => {
      const extra = { version: '1.5', missing }
      if (reason !== undefined) extra.reason = reason
      const { i18n, w } = await mountModal('outdated', { locale, extra })
      const text = w.text()
      expect(text).toContain(i18n.t(title))
      for (const other of OUTDATED_TITLES.filter(k => k !== title)) expect(text).not.toContain(i18n.t(other))
      expect(text).toContain(i18n.t(lead, { url: 'http://srv:7860', version: '1.5', missing: featureText(i18n.t, missing) }))
      if (models) expect(text).toContain(i18n.t('server.outdated_models'))
      else expect(text).not.toContain(i18n.t('server.outdated_models'))
    })
  }

  it.each(['ru', 'en'])('%s: missing_features lead shows feature texts, not raw names, joined by comma', async (locale) => {
    const { i18n, w } = await mountModal('outdated', { locale, extra: { reason: 'missing_features', missing: ['sdcpp', 'download_stream', 'llamacpp'] } })
    const text = w.text()
    expect(text).toContain([i18n.t('server.feature_sdcpp'), i18n.t('server.feature_download_stream'), i18n.t('server.feature_llm')].join(', '))
    expect(text).not.toContain('download_stream')
    expect(text).not.toContain('llamacpp')
  })

  it.each([
    ['ru', 'движок текста (llama.cpp)'],
    ['en', 'a text engine (llama.cpp)'],
  ])('%s: missing llamacpp is shown as the llama.cpp text engine without Ollama', async (locale, text) => {
    const { i18n, w } = await mountModal('outdated', { locale, extra: { reason: 'missing_features', missing: ['llamacpp'] } })
    expect(i18n.t('server.feature_llm')).toBe(text)
    expect(w.text()).toContain(text)
    expect(w.text().toLowerCase()).not.toContain('ollama')
  })

  it('an unknown feature name is shown as is', async () => {
    const { i18n, w } = await mountModal('outdated', { locale: 'en', extra: { reason: 'missing_features', missing: ['download_stream', 'magic_x'] } })
    expect(w.text()).toContain(`${i18n.t('server.feature_download_stream')}, magic_x`)
  })

  it.each([
    ['outdated', 'no_sdcpp', 'server.server_guide'],
    ['outdated', 'old_version', 'server.how_to_update'],
    ['outdated', 'missing_features', 'server.how_to_update'],
    ['outdated', '', 'server.how_to_update'],
    ['outdated', 'something_new', 'server.how_to_update'],
    ['not_found', '', 'server.how_to_install'],
    ['not_configured', '', 'server.how_to_install'],
  ])('%s with reason %j: guide button says %s', async (state, reason, key) => {
    for (const locale of ['ru', 'en']) {
      const { i18n, w } = await mountModal(state, { locale, url: state === 'not_configured' ? '' : 'http://srv:7860', extra: { reason } })
      const labels = w.findAll('button').map(b => b.text())
      expect(labels.some(l => l.includes(i18n.t(key))), `${locale} ${key}`).toBe(true)
      for (const other of ['server.server_guide', 'server.how_to_update', 'server.how_to_install'].filter(k => k !== key)) {
        expect(labels.some(l => l.includes(i18n.t(other))), `${locale} ${other}`).toBe(false)
      }
      w.unmount()
    }
  })
})

describe('serverState.reason', () => {
  it.each(['old_version', 'no_sdcpp', 'missing_features'])('is filled from the check result: %s', async (reason) => {
    const { App, ss } = await load()
    App.CheckServer.mockResolvedValue(resp('outdated', { reason }))
    await ss.checkServer()
    expect(ss.serverState.reason).toBe(reason)
  })

  it('is empty when the result has no reason, also after a previous reason', async () => {
    const { App, ss } = await load()
    expect(ss.serverState.reason).toBe('')
    App.CheckServer.mockResolvedValue(resp('outdated', { reason: 'no_sdcpp' }))
    await ss.checkServer()
    App.CheckServer.mockResolvedValue(resp('ok'))
    await ss.checkServer()
    expect(ss.serverState.reason).toBe('')
  })
})
