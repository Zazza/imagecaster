import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import PresetsPage from '../components/PresetsPage.vue'
import { ListPresets, ListPresetTypes, SyncPresets, GetSettings } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const PRESETS = [
  { id: 1, name: 'Ready Style', preset_type: 'general', type_id: 1 },
  { id: 2, name: 'Missing Model Style', preset_type: 'general', type_id: 1 },
  { id: 3, name: 'Missing Lora Style', preset_type: 'general', type_id: 1 },
]
const STATUSES = [
  { id: 1, installed: true, missing_sd: [], missing_lora: [] },
  { id: 2, installed: false, missing_sd: ['flux1-dev'], missing_lora: [] },
  { id: 3, installed: false, missing_sd: [], missing_lora: ['ink'] },
]
const ALL = PRESETS.map(p => p.name)

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  ListPresetTypes.mockImplementation(() => Promise.resolve([{ id: 1, name: 'general' }]))
  GetSettings.mockImplementation(() => Promise.resolve({}))
})

afterEach(() => vi.restoreAllMocks())

function givenSync(kind) {
  if (kind === 'statuses') SyncPresets.mockImplementation(() => Promise.resolve({ statuses: STATUSES, added: 0 }))
  else SyncPresets.mockImplementation(() => Promise.reject(new Error('server unreachable')))
}

async function generateStyleOptions() {
  const w = mount(GeneratePage, { attachTo: document.body })
  await settle()
  const select = w.findAll('select').find(s => s.find('option').text() === t('generate.select_style'))
  return select.findAll('option').slice(1).map(o => o.text())
}

async function mountStylesList() {
  const w = mount(PresetsPage, { attachTo: document.body })
  await settle()
  return w
}

const rowNames = w => w.findAll('[data-testid="style-row"]').map(r => r.find('.styles-name').text())

describe('style availability on the generate page', () => {
  it('styles the server marks as not installed are not offered', async () => {
    givenSync('statuses')
    expect(await generateStyleOptions()).toEqual(['Ready Style'])
  })

  it('sync failure hides nothing', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    givenSync('error')
    expect(await generateStyleOptions()).toEqual(ALL)
  })
})

describe('style availability on the styles page', () => {
  it('unavailable styles are hidden until "show unavailable" is ticked', async () => {
    givenSync('statuses')
    const w = await mountStylesList()
    expect(rowNames(w)).toEqual(['Ready Style'])
    await w.find('.styles-unavailable input').setValue(true)
    expect(rowNames(w).sort()).toEqual([...ALL].sort())
  })

  it('sync failure hides nothing', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    givenSync('error')
    const w = await mountStylesList()
    expect(rowNames(w).sort()).toEqual([...ALL].sort())
  })
})
