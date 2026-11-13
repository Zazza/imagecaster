import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base, SyncPresets: vi.fn() }
})

import PresetsPage from '../components/PresetsPage.vue'
import {
  ListPresets, ListPresetTypes, SyncPresets, GetPresetsInstallStatus, ListCompoundPresets,
  DeletePreset, CreatePreset, InstallPresetDeps,
} from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { effectTypeLabel } from '../effectTypes.js'

enableAutoUnmount(afterEach)

const TYPES = [
  { id: 1, name: 'general' },
  { id: 2, name: 'anime' },
  { id: 3, name: 'post_light' },
]

const STYLES = [
  { id: 11, name: 'Kappa Bundled', prompt: 'misty forest scene', is_bundled: true, model_name: 'flux/flux1-dev.safetensors', type_id: 1, preset_type: 'general', steps: 20, cfg_scale: 1 },
  { id: 12, name: 'alpha Custom', prompt: 'neon city ANIME', is_bundled: false, model_name: 'sdxl/animagine.gguf', type_id: 2, preset_type: 'anime', steps: 25, cfg_scale: 6 },
  { id: 13, name: 'Mu Anymodel', prompt: 'quiet lake', is_bundled: true, model_name: '', type_id: 1, preset_type: 'general', steps: 30, cfg_scale: 4 },
  { id: 14, name: 'Lambda Mine', prompt: 'desert dunes', is_bundled: false, model_name: 'flux1-dev.safetensors', type_id: 2, preset_type: 'anime', steps: 10, cfg_scale: 2 },
]
const EFFECTS = [
  { id: 21, name: 'Glow Effect', prompt: 'golden hour', is_bundled: true, model_name: 'klein/flux-2-klein-4b.gguf', type_id: 3, preset_type: 'post_light', steps: 4, cfg_scale: 1 },
]
const ALL_NAMES = [...STYLES, ...EFFECTS].map(p => p.name)

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
  ListPresets.mockResolvedValue([...STYLES, ...EFFECTS].map(p => ({ ...p })))
  SyncPresets.mockResolvedValue({ statuses: [], added: 0 })
  GetPresetsInstallStatus.mockResolvedValue([])
  ListCompoundPresets.mockResolvedValue([])
})

afterEach(() => {
  vi.restoreAllMocks()
})

async function mountPage(mode = 'styles') {
  const wrapper = mount(PresetsPage, { props: { mode }, attachTo: document.body })
  await settle()
  return wrapper
}

const rows = (w) => w.findAll('[data-testid="style-row"]')
const rowName = (row) => ALL_NAMES.find(n => row.text().includes(n))
const rowNames = (w) => rows(w).map(rowName)
const rowOf = (w, name) => rows(w).find(r => rowName(r) === name)
const shown = (w, sel) => {
  const el = w.find(sel)
  return el.exists() && el.isVisible()
}
const buttonIn = (row, text) => row.findAll('button').find(b => [b.text().trim(), b.attributes('title'), b.attributes('aria-label')].includes(text))

describe('styles table: rows and badges', () => {
  it('one row per style, sorted by name, effects excluded', async () => {
    const w = await mountPage('styles')
    expect(rowNames(w)).toEqual(['alpha Custom', 'Kappa Bundled', 'Lambda Mine', 'Mu Anymodel'])
  })

  it('effects mode shows only effects as rows', async () => {
    const w = await mountPage('effects')
    expect(rowNames(w)).toEqual(['Glow Effect'])
    const row = rowOf(w, 'Glow Effect')
    expect(row.find('[data-testid="badge-group"]').text().trim()).toBe(effectTypeLabel('post_light', t))
    expect(row.find('[data-testid="badge-model"]').text().trim()).toBe('flux-2-klein-4b')
  })

  it('source badge shows bundled/custom label', async () => {
    const w = await mountPage()
    expect(rowOf(w, 'Kappa Bundled').find('[data-testid="badge-source"]').text().trim()).toBe(t('presets.source_bundled'))
    expect(rowOf(w, 'alpha Custom').find('[data-testid="badge-source"]').text().trim()).toBe(t('presets.source_custom'))
  })

  it('bundled and custom source badges look different', async () => {
    const w = await mountPage()
    const a = rowOf(w, 'Kappa Bundled').find('[data-testid="badge-source"]')
    const b = rowOf(w, 'alpha Custom').find('[data-testid="badge-source"]')
    expect(a.classes().join(' ') + (a.attributes('style') || '')).not.toBe(b.classes().join(' ') + (b.attributes('style') || ''))
  })

  it('model badge shows model name without directory and extension', async () => {
    const w = await mountPage()
    expect(rowOf(w, 'Kappa Bundled').find('[data-testid="badge-model"]').text().trim()).toBe('flux1-dev')
    expect(rowOf(w, 'alpha Custom').find('[data-testid="badge-model"]').text().trim()).toBe('animagine')
  })

  it('empty model shows "any model" badge', async () => {
    const w = await mountPage()
    expect(rowOf(w, 'Mu Anymodel').find('[data-testid="badge-model"]').text().trim()).toBe(t('presets.model_any'))
  })

  it('group badge shows group label', async () => {
    const w = await mountPage()
    expect(rowOf(w, 'alpha Custom').find('[data-testid="badge-group"]').text().trim()).toBe(effectTypeLabel('anime', t))
  })

  it('new i18n keys are translated', () => {
    for (const key of ['presets.source_bundled', 'presets.source_custom', 'presets.model_any']) {
      expect(t(key)).not.toBe(key)
      expect(t(key)).toBeTruthy()
    }
  })
})

describe('styles table: instant filters', () => {
  it('typing in search filters rows immediately, case-insensitive, by name and prompt', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-search"]').setValue('KAPPA')
    expect(rowNames(w)).toEqual(['Kappa Bundled'])
    await w.find('[data-testid="filter-search"]').setValue('anime')
    expect(rowNames(w)).toEqual(['alpha Custom'])
  })

  it('source chips filter rows', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-source-custom"]').trigger('click')
    expect(rowNames(w)).toEqual(['alpha Custom', 'Lambda Mine'])
    await w.find('[data-testid="filter-source-bundled"]').trigger('click')
    expect(rowNames(w)).toEqual(['Kappa Bundled', 'Mu Anymodel'])
    await w.find('[data-testid="filter-source-all"]').trigger('click')
    expect(rows(w)).toHaveLength(4)
  })

  it('model chips show counts and allow multiple selection', async () => {
    const w = await mountPage()
    const chip = (label) => w.findAll('[data-testid="filter-model"]').find(c => c.attributes('data-model') === label)
    expect(chip('flux1-dev').text()).toContain('2')
    expect(chip('animagine').text()).toContain('1')
    await chip('animagine').trigger('click')
    expect(rowNames(w)).toEqual(['alpha Custom'])
    await chip('').trigger('click')
    expect(rowNames(w)).toEqual(['alpha Custom', 'Mu Anymodel'])
    await chip('animagine').trigger('click')
    expect(rowNames(w)).toEqual(['Mu Anymodel'])
  })

  it('selected model chip stays with zero count after other filters', async () => {
    const w = await mountPage()
    const chip = (label) => w.findAll('[data-testid="filter-model"]').find(c => c.attributes('data-model') === label)
    await chip('animagine').trigger('click')
    await w.find('[data-testid="filter-source-bundled"]').trigger('click')
    expect(rows(w)).toHaveLength(0)
    expect(chip('animagine')).toBeTruthy()
    expect(chip('animagine').text()).toContain('0')
  })

  it('group select filters rows', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-group"]').setValue('anime')
    expect(rowNames(w)).toEqual(['alpha Custom', 'Lambda Mine'])
  })

  it('filters combine with AND', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-group"]').setValue('general')
    await w.find('[data-testid="filter-source-bundled"]').trigger('click')
    await w.find('[data-testid="filter-search"]').setValue('lake')
    expect(rowNames(w)).toEqual(['Mu Anymodel'])
  })
})

describe('styles table: badge click filters', () => {
  it('clicking model badge filters to that model', async () => {
    const w = await mountPage()
    await rowOf(w, 'Kappa Bundled').find('[data-testid="badge-model"]').trigger('click')
    expect(rowNames(w)).toEqual(['Kappa Bundled', 'Lambda Mine'])
  })

  it('clicking "any model" badge filters to presets without model', async () => {
    const w = await mountPage()
    await rowOf(w, 'Mu Anymodel').find('[data-testid="badge-model"]').trigger('click')
    expect(rowNames(w)).toEqual(['Mu Anymodel'])
  })

  it('clicking source badge filters by source', async () => {
    const w = await mountPage()
    await rowOf(w, 'alpha Custom').find('[data-testid="badge-source"]').trigger('click')
    expect(rowNames(w)).toEqual(['alpha Custom', 'Lambda Mine'])
  })

  it('clicking group badge filters by group', async () => {
    const w = await mountPage()
    await rowOf(w, 'Kappa Bundled').find('[data-testid="badge-group"]').trigger('click')
    expect(rowNames(w)).toEqual(['Kappa Bundled', 'Mu Anymodel'])
  })
})

describe('styles table: reset and empty result', () => {
  it('reset hidden without active filters', async () => {
    const w = await mountPage()
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(false)
  })

  it('reset appears when a filter is active and clears all filters', async () => {
    const w = await mountPage()
    await w.find('[data-testid="filter-search"]').setValue('kappa')
    await w.find('[data-testid="filter-source-bundled"]').trigger('click')
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(true)
    await w.find('[data-testid="filter-reset"]').trigger('click')
    expect(rows(w)).toHaveLength(4)
    expect(w.find('[data-testid="filter-search"]').element.value).toBe('')
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(false)
  })

  it('no rows after filters shows "nothing found" with reset', async () => {
    const w = await mountPage()
    expect(shown(w, '[data-testid="no-match"]')).toBe(false)
    await w.find('[data-testid="filter-search"]').setValue('zzz-no-such-style')
    expect(rows(w)).toHaveLength(0)
    expect(shown(w, '[data-testid="no-match"]')).toBe(true)
    expect(shown(w, '[data-testid="filter-reset"]')).toBe(true)
  })
})

describe('styles table: existing actions per row', () => {
  it('each row has edit, duplicate and delete', async () => {
    const w = await mountPage()
    for (const row of rows(w)) {
      expect(buttonIn(row, t('presets.btn_edit'))).toBeTruthy()
      expect(buttonIn(row, t('presets.btn_duplicate'))).toBeTruthy()
      expect(buttonIn(row, t('presets.btn_delete'))).toBeTruthy()
    }
  })

  it('delete needs a second click to confirm', async () => {
    const w = await mountPage()
    await buttonIn(rowOf(w, 'Lambda Mine'), t('presets.btn_delete')).trigger('click')
    expect(DeletePreset).not.toHaveBeenCalled()
    await buttonIn(rowOf(w, 'Lambda Mine'), t('presets.btn_sure')).trigger('click')
    await settle()
    expect(DeletePreset).toHaveBeenCalledTimes(1)
    expect(DeletePreset).toHaveBeenCalledWith(14)
  })

  it('duplicate creates a copy', async () => {
    const w = await mountPage()
    await buttonIn(rowOf(w, 'alpha Custom'), t('presets.btn_duplicate')).trigger('click')
    await settle()
    expect(CreatePreset).toHaveBeenCalledTimes(1)
    expect(CreatePreset.mock.calls[0][0].name).toBe('alpha Custom (copy)')
  })

  it('unavailable style is hidden by default and its install status is used', async () => {
    SyncPresets.mockResolvedValue({ statuses: [{ id: 14, installed: false, missing_sd: ['flux1-dev'], missing_lora: [] }], added: 0 })
    const w = await mountPage()
    expect(rowNames(w)).not.toContain('Lambda Mine')
    expect(rows(w)).toHaveLength(3)
  })
})

describe('styles table: unavailable styles, install and select mode', () => {
  const MISSING = { statuses: [{ id: 14, installed: false, missing_sd: ['flux1-dev'], missing_lora: ['detail-lora'] }], added: 0 }
  const showUnavailable = async (w) => {
    const label = w.findAll('label').find(l => l.text().includes(t('presets.show_unavailable')))
    expect(label).toBeTruthy()
    await label.find('input[type="checkbox"]').setValue(true)
    await settle()
  }

  it('style with missing model or LoRA is hidden by default', async () => {
    SyncPresets.mockResolvedValue(MISSING)
    const w = await mountPage()
    expect(rowNames(w)).toEqual(['alpha Custom', 'Kappa Bundled', 'Mu Anymodel'])
  })

  it('show unavailable reveals the row with missing status, names and install button', async () => {
    SyncPresets.mockResolvedValue(MISSING)
    const w = await mountPage()
    await showUnavailable(w)
    const row = rowOf(w, 'Lambda Mine')
    expect(row).toBeTruthy()
    const text = row.text()
    const at = text.indexOf(t('presets.status_missing'))
    expect(at).toBeGreaterThanOrEqual(0)
    expect(text.indexOf('flux1-dev', at)).toBeGreaterThan(at)
    expect(text.indexOf('detail-lora', at)).toBeGreaterThan(at)
    expect(buttonIn(row, t('presets.btn_install'))).toBeTruthy()
    expect(buttonIn(rowOf(w, 'alpha Custom'), t('presets.btn_install'))).toBeFalsy()
  })

  it('install button calls install API with the preset id', async () => {
    SyncPresets.mockResolvedValue(MISSING)
    const w = await mountPage()
    await showUnavailable(w)
    await buttonIn(rowOf(w, 'Lambda Mine'), t('presets.btn_install')).trigger('click')
    await settle()
    expect(InstallPresetDeps).toHaveBeenCalledTimes(1)
    expect(InstallPresetDeps).toHaveBeenCalledWith(14)
  })

  it('select mode shows a checkbox in every row', async () => {
    const w = await mountPage()
    for (const row of rows(w)) expect(row.find('input[type="checkbox"]').exists()).toBe(false)
    const btn = w.findAll('button').find(b => b.text().trim() === t('presets.btn_select'))
    expect(btn).toBeTruthy()
    await btn.trigger('click')
    await settle()
    expect(rows(w)).toHaveLength(4)
    for (const row of rows(w)) expect(row.find('input[type="checkbox"]').exists()).toBe(true)
  })
})
