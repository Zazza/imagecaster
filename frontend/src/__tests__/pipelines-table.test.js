import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, SyncPresets: vi.fn() }
})

import CompoundPresetsPage from '../components/CompoundPresetsPage.vue'
import {
  ListPresets, ListPresetTypes, SyncPresets, ListCompoundPresets,
  DeleteCompoundPreset, PrepareCompoundPresetsExport, CreateCompoundPreset, OpenImportCompoundFile,
} from '../wailsjs/go/main/App.js'
import { t, en, ru } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const TYPES = [{ id: 1, name: 'general' }]

const STYLES = [
  { id: 1, name: 'Portrait Base', prompt: 'p', model_name: 'flux/flux1-dev.safetensors', type_id: 1, preset_type: 'general' },
  { id: 2, name: 'Anime Pass', prompt: 'p', model_name: 'sdxl/animagine.gguf', type_id: 1, preset_type: 'general' },
  { id: 3, name: 'Detail Up', prompt: 'p', model_name: 'flux1-dev.safetensors', type_id: 1, preset_type: 'general' },
  { id: 4, name: 'Free Style', prompt: 'p', model_name: '', type_id: 1, preset_type: 'general' },
  { id: 5, name: 'Lost Style', prompt: 'p', model_name: 'klein/klein-4b.gguf', type_id: 1, preset_type: 'general' },
]

let stepId = 1000
const step = (preset_id, denoising_strength, order) => ({ id: ++stepId, step_order: order, preset_id, denoising_strength, loras: null })

const PIPES = () => [
  { id: 101, name: 'Beta Flow', description: 'soft portrait', steps: [step(1, 0.85, 1), step(3, 0.35, 2)] },
  { id: 102, name: 'alpha Chain', description: 'neon look', steps: [step(2, 1, 1), step(1, 0.6, 2)] },
  { id: 103, name: 'Gamma Solo', description: '', steps: [step(4, 1, 1)] },
  { id: 104, name: 'delta Broken', description: '', steps: [step(2, 1, 1), step(999, 0.5, 2)] },
]
const PIPE_NAMES = ['Beta Flow', 'alpha Chain', 'Gamma Solo', 'delta Broken', 'Epsilon Hidden']

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  ListPresetTypes.mockResolvedValue(TYPES.map(x => ({ ...x })))
  ListPresets.mockResolvedValue(STYLES.map(p => ({ ...p })))
  SyncPresets.mockResolvedValue({ statuses: [], added: 0 })
  ListCompoundPresets.mockResolvedValue(PIPES())
  PrepareCompoundPresetsExport.mockResolvedValue('')
  window.alert = vi.fn()
})

afterEach(() => {
  vi.restoreAllMocks()
})

async function mountPage() {
  const wrapper = mount(CompoundPresetsPage, { attachTo: document.body })
  await settle()
  return wrapper
}

const rows = (w) => w.findAll('[data-testid="pipeline-row"]')
const rowName = (row) => PIPE_NAMES.find(n => row.text().includes(n))
const rowNames = (w) => rows(w).map(rowName)
const rowOf = (w, name) => rows(w).find(r => rowName(r) === name)
const shown = (w, sel) => {
  const el = w.find(sel)
  return el.exists() && el.isVisible()
}
const chip = (w, label) => w.findAll('[data-testid="filter-model"]').find(c => c.attributes('data-model') === label)
const modelBadges = (row) => row.findAll('[data-testid="badge-model"]').map(b => b.text().trim())
const enterSelect = async (w) => {
  await w.find('[data-testid="select-mode"]').trigger('click')
  await settle()
}
const checked = (w) => rows(w).filter(r => r.find('input[type="checkbox"]').element.checked).map(rowName)
const deletedIds = () => DeleteCompoundPreset.mock.calls.map(c => c[0]).sort((a, b) => a - b)
const exportedIds = () => [...PrepareCompoundPresetsExport.mock.calls.at(-1)[0]].sort((a, b) => a - b)
const openConfirm = async (w) => {
  await w.find('[data-testid="delete-selected"]').trigger('click')
  await settle()
}
const confirmDelete = async (w) => {
  await w.find('[data-testid="confirm-delete"]').trigger('click')
  await settle()
}

describe('pipelines table: rows and columns', () => {
  it('one row per pipeline, sorted by name', async () => {
    const w = await mountPage()
    expect(rowNames(w)).toEqual(['alpha Chain', 'Beta Flow', 'delta Broken', 'Gamma Solo'])
  })

  it('row shows name and description', async () => {
    const w = await mountPage()
    const text = rowOf(w, 'Beta Flow').text()
    expect(text).toContain('Beta Flow')
    expect(text).toContain('soft portrait')
  })

  it('steps column is a chain of style badges in step order', async () => {
    const w = await mountPage()
    const badges = rowOf(w, 'Beta Flow').findAll('[data-testid="step-badge"]')
    expect(badges).toHaveLength(2)
    expect(badges[0].text()).toContain('Portrait Base')
    expect(badges[1].text()).toContain('Detail Up')
  })

  it('denoising strength shown from the second step only', async () => {
    const w = await mountPage()
    const badges = rowOf(w, 'Beta Flow').findAll('[data-testid="step-badge"]')
    expect(badges[0].text()).not.toMatch(/0[.,]85|85/)
    expect(badges[1].text()).toMatch(/0[.,]35|35/)
  })

  it('model badges list chain models without repeats', async () => {
    const w = await mountPage()
    expect(modelBadges(rowOf(w, 'Beta Flow'))).toEqual(['flux1-dev'])
    expect([...modelBadges(rowOf(w, 'alpha Chain'))].sort()).toEqual(['animagine', 'flux1-dev'])
  })

  it('each row has edit, duplicate and delete', async () => {
    const w = await mountPage()
    for (const row of rows(w)) {
      expect(row.find('[data-testid="row-edit"]').exists()).toBe(true)
      expect(row.find('[data-testid="row-duplicate"]').exists()).toBe(true)
      expect(row.find('[data-testid="row-delete"]').exists()).toBe(true)
    }
  })

  it('no pipelines shows no rows', async () => {
    ListCompoundPresets.mockResolvedValue([])
    const w = await mountPage()
    expect(rows(w)).toHaveLength(0)
  })
})

describe('pipelines table: filters', () => {
  it('search by name is instant and case-insensitive', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-search"]').setValue('GAMMA')
    expect(rowNames(w)).toEqual(['Gamma Solo'])
  })

  it('search by description', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-search"]').setValue('neon')
    expect(rowNames(w)).toEqual(['alpha Chain'])
  })

  it('search by step style name', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-search"]').setValue('detail up')
    expect(rowNames(w)).toEqual(['Beta Flow'])
    await w.find('[data-testid="filter-search"]').setValue('anime pass')
    expect(rowNames(w)).toEqual(['alpha Chain', 'delta Broken'])
  })

  it('model chips show counts and combine with OR', async () => {
    const w = await mountPage()
    expect(chip(w, 'flux1-dev').text()).toContain('2')
    expect(chip(w, '').text()).toContain('1')
    await chip(w, 'flux1-dev').trigger('click')
    expect(rowNames(w)).toEqual(['alpha Chain', 'Beta Flow'])
    await chip(w, '').trigger('click')
    expect(rowNames(w)).toEqual(['alpha Chain', 'Beta Flow', 'Gamma Solo'])
    await chip(w, 'flux1-dev').trigger('click')
    expect(rowNames(w)).toEqual(['Gamma Solo'])
  })

  it('selected model chip stays with zero after search hides its pipelines', async () => {
    const w = await mountPage()
    await chip(w, 'animagine').trigger('click')
    await w.find('[data-testid="filter-search"]').setValue('gamma')
    expect(rows(w)).toHaveLength(0)
    expect(chip(w, 'animagine')).toBeTruthy()
    expect(chip(w, 'animagine').text()).toContain('0')
  })

  it('clicking a model badge in a row turns on that model filter', async () => {
    const w = await mountPage()
    const badge = rowOf(w, 'Beta Flow').findAll('[data-testid="badge-model"]').find(b => b.text().trim() === 'flux1-dev')
    await badge.trigger('click')
    expect(rowNames(w)).toEqual(['alpha Chain', 'Beta Flow'])
  })

  it('reset hidden without filters, clears all filters', async () => {
    const w = await mountPage()
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(false)
    await w.find('[data-testid="filter-search"]').setValue('beta')
    await chip(w, 'flux1-dev').trigger('click')
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(true)
    await w.find('[data-testid="filter-reset"]').trigger('click')
    expect(rows(w)).toHaveLength(4)
    expect(w.find('[data-testid="filter-search"]').element.value).toBe('')
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(false)
  })

  it('nothing found shows message and reset', async () => {
    const w = await mountPage()
    expect(shown(w, '[data-testid="no-match"]')).toBe(false)
    await w.find('[data-testid="filter-search"]').setValue('zzz-no-such')
    expect(rows(w)).toHaveLength(0)
    expect(shown(w, '[data-testid="no-match"]')).toBe(true)
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(true)
  })
})

describe('pipelines table: missing step styles', () => {
  it('deleted step style gets a warning badge and the row a warning', async () => {
    const w = await mountPage()
    const row = rowOf(w, 'delta Broken')
    const badges = row.findAll('[data-testid="step-badge"]')
    expect(badges[0].classes()).not.toContain('badge-warning')
    expect(badges[1].classes()).toContain('badge-warning')
    expect(row.find('[data-testid="pipeline-missing"]').exists()).toBe(true)
  })

  it('pipeline with all styles present has no warning', async () => {
    const w = await mountPage()
    const row = rowOf(w, 'Beta Flow')
    expect(row.find('[data-testid="pipeline-missing"]').exists()).toBe(false)
    for (const b of row.findAll('[data-testid="step-badge"]')) expect(b.classes()).not.toContain('badge-warning')
  })

  it('unavailable step style (missing model on server) gets a warning badge', async () => {
    ListCompoundPresets.mockResolvedValue([...PIPES(), { id: 105, name: 'Epsilon Hidden', description: '', steps: [step(1, 1, 1), step(5, 0.5, 2)] }])
    SyncPresets.mockResolvedValue({ statuses: [{ id: 5, installed: false, missing_sd: ['klein-4b'], missing_lora: [] }], added: 0 })
    const w = await mountPage()
    const row = rowOf(w, 'Epsilon Hidden')
    const badges = row.findAll('[data-testid="step-badge"]')
    expect(badges[0].classes()).not.toContain('badge-warning')
    expect(badges[1].classes()).toContain('badge-warning')
    expect(row.find('[data-testid="pipeline-missing"]').exists()).toBe(true)
  })
})

describe('pipelines table: single delete', () => {
  it('first click does not delete, second click deletes', async () => {
    const w = await mountPage()
    await rowOf(w, 'Gamma Solo').find('[data-testid="row-delete"]').trigger('click')
    await settle()
    expect(DeleteCompoundPreset).not.toHaveBeenCalled()
    expect(rowOf(w, 'Gamma Solo').text()).toContain(t('presets.btn_sure'))
    await rowOf(w, 'Gamma Solo').find('[data-testid="row-delete"]').trigger('click')
    await settle()
    expect(DeleteCompoundPreset).toHaveBeenCalledTimes(1)
    expect(DeleteCompoundPreset).toHaveBeenCalledWith(103)
  })
})

describe('pipelines table: other actions', () => {
  it('duplicate creates a copy with the same steps', async () => {
    const w = await mountPage()
    await rowOf(w, 'Beta Flow').find('[data-testid="row-duplicate"]').trigger('click')
    await settle()
    expect(CreateCompoundPreset).toHaveBeenCalledTimes(1)
    const arg = CreateCompoundPreset.mock.calls[0][0]
    expect(arg.name).toBe('Beta Flow (copy)')
    expect(arg.steps.map(s => s.preset_id)).toEqual([1, 3])
    expect(arg.steps.map(s => s.denoising_strength)).toEqual([0.85, 0.35])
  })

  it('edit opens the form with pipeline name', async () => {
    const w = await mountPage()
    await rowOf(w, 'Beta Flow').find('[data-testid="row-edit"]').trigger('click')
    await settle()
    expect(w.findAll('input').some(i => i.element.value === 'Beta Flow')).toBe(true)
  })

  it('import button opens the import file dialog', async () => {
    const w = await mountPage()
    const btn = w.findAll('button').find(b => b.text().trim() === t('compound.btn_import'))
    expect(btn).toBeTruthy()
    await btn.trigger('click')
    await settle()
    expect(OpenImportCompoundFile).toHaveBeenCalledTimes(1)
  })

  it('compound i18n keys are in parity between ru and en', () => {
    const keys = (d) => Object.keys(d).filter(k => k.startsWith('compound.')).sort()
    expect(keys(ru)).toEqual(keys(en))
  })
})

describe('pipelines table: select mode', () => {
  it('select mode adds checkboxes, cancel leaves it', async () => {
    const w = await mountPage()
    for (const row of rows(w)) expect(row.find('input[type="checkbox"]').exists()).toBe(false)
    await enterSelect(w)
    for (const row of rows(w)) expect(row.find('input[type="checkbox"]').exists()).toBe(true)
    const cancel = w.findAll('button').filter(b => !b.element.closest('.modal')).find(b => b.text().trim() === t('presets.btn_cancel'))
    expect(cancel).toBeTruthy()
    await cancel.trigger('click')
    await settle()
    for (const row of rows(w)) expect(row.find('input[type="checkbox"]').exists()).toBe(false)
  })

  it('select all selects only visible rows after filter and exports only them', async () => {
    const w = await mountPage()
    await enterSelect(w)
    await chip(w, 'animagine').trigger('click')
    await w.find('[data-testid="select-all"]').trigger('click')
    await settle()
    expect(checked(w)).toEqual(['alpha Chain', 'delta Broken'])
    expect(w.find('[data-testid="export-selected"]').text()).toContain('2')
    await w.find('[data-testid="export-selected"]').trigger('click')
    await settle()
    expect(PrepareCompoundPresetsExport).toHaveBeenCalledTimes(1)
    expect(exportedIds()).toEqual([102, 104])
  })

  it('second click on select all deselects', async () => {
    const w = await mountPage()
    await enterSelect(w)
    await w.find('[data-testid="select-all"]').trigger('click')
    await settle()
    expect(checked(w)).toHaveLength(4)
    await w.find('[data-testid="select-all"]').trigger('click')
    await settle()
    expect(checked(w)).toEqual([])
  })

  it('header checkbox selects only visible rows and toggles off', async () => {
    const w = await mountPage()
    await enterSelect(w)
    await w.find('[data-testid="filter-search"]').setValue('anime pass')
    await w.find('[data-testid="select-all-check"]').setValue(true)
    await settle()
    expect(checked(w)).toEqual(['alpha Chain', 'delta Broken'])
    await w.find('[data-testid="select-all-check"]').setValue(false)
    await settle()
    expect(checked(w)).toEqual([])
  })

  it('changing filter drops hidden pipelines from selection', async () => {
    const w = await mountPage()
    await enterSelect(w)
    await w.find('[data-testid="select-all"]').trigger('click')
    await settle()
    await w.find('[data-testid="filter-search"]').setValue('beta')
    await settle()
    expect(w.find('[data-testid="export-selected"]').text()).toContain('1')
    expect(w.find('[data-testid="delete-selected"]').text()).toContain('1')
    await w.find('[data-testid="filter-search"]').setValue('')
    await settle()
    expect(checked(w)).toEqual(['Beta Flow'])
    await w.find('[data-testid="export-selected"]').trigger('click')
    await settle()
    expect(exportedIds()).toEqual([101])
  })

  it('delete selected asks for confirmation with names, deletes after confirm', async () => {
    const w = await mountPage()
    await enterSelect(w)
    await chip(w, 'flux1-dev').trigger('click')
    await w.find('[data-testid="select-all"]').trigger('click')
    await settle()
    await openConfirm(w)
    expect(DeleteCompoundPreset).not.toHaveBeenCalled()
    const text = w.find('[data-testid="delete-names"]').text()
    expect(text).toContain('alpha Chain')
    expect(text).toContain('Beta Flow')
    expect(text).not.toContain('Gamma Solo')
    await confirmDelete(w)
    expect(deletedIds()).toEqual([101, 102])
  })

  it('cancel in confirmation deletes nothing and closes it', async () => {
    const w = await mountPage()
    await enterSelect(w)
    await w.find('[data-testid="select-all"]').trigger('click')
    await settle()
    await openConfirm(w)
    const cancel = w.findAll('.modal button').find(b => b.text().trim() === t('presets.btn_cancel'))
    expect(cancel).toBeTruthy()
    await cancel.trigger('click')
    await settle()
    expect(DeleteCompoundPreset).not.toHaveBeenCalled()
    expect(shown(w, '[data-testid="confirm-delete"]')).toBe(false)
  })

  it('confirmation shows at most 10 names and a "more" line', async () => {
    const many = Array.from({ length: 13 }, (_, i) => ({
      id: 200 + i, name: `Bulk ${String(i).padStart(2, '0')}`, description: '', steps: [step(1, 1, 1)],
    }))
    ListCompoundPresets.mockResolvedValue(many)
    const w = await mountPage()
    await enterSelect(w)
    await w.find('[data-testid="select-all"]').trigger('click')
    await settle()
    await openConfirm(w)
    const text = w.find('[data-testid="delete-names"]').text()
    expect(many.filter(p => text.includes(p.name))).toHaveLength(10)
    expect(text).toContain(t('presets.delete_more', { count: 3 }))
    expect(DeleteCompoundPreset).not.toHaveBeenCalled()
    await confirmDelete(w)
    expect(deletedIds()).toEqual(many.map(p => p.id))
  })
})
