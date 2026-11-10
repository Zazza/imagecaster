import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base }
})

import UnifiedPresetsPage from '../components/UnifiedPresetsPage.vue'
import PresetsPage from '../components/PresetsPage.vue'
import PresetForm from '../components/PresetForm.vue'
import {
  ListPresets, ListPresetTypes, GetPresetsInstallStatus, ListCompoundPresets, PreparePresetsExport, DeletePreset,
  GetSDModels, GetSDSamplers, GetSDSchedulers, GetSDUpscalers, GetSDVAEs, GetSDLoRAs,
} from '../wailsjs/go/main/App.js'
import { t, en, ru } from '../i18n/index.js'
import { effectTypeLabel } from '../effectTypes.js'

enableAutoUnmount(afterEach)

const TYPES = [
  { id: 1, name: 'general' },
  { id: 2, name: 'post' },
  { id: 3, name: 'post_light' },
  { id: 4, name: 'post_film' },
]
const EFFECT_TYPES = TYPES.filter(x => x.name === 'post' || x.name.startsWith('post_'))
const STYLE_TYPES = TYPES.filter(x => !EFFECT_TYPES.includes(x))

const STYLES = [
  { id: 101, name: 'Plain Style Qx' },
  { id: 102, name: 'General Style Qx', type_id: 1, preset_type: 'general' },
]
const EFFECTS = [
  { id: 201, name: 'Base Effect Qx', type_id: 2, preset_type: 'post' },
  { id: 202, name: 'Light Effect Qx', type_id: 3, preset_type: 'post_light' },
  { id: 203, name: 'Film Effect Qx', type_id: 4, preset_type: 'post_film' },
]

async function settle() {
  for (let i = 0; i < 3; i++) {
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
  }
  await nextTick()
}

function givenPresets(list) {
  ListPresets.mockResolvedValue(list.map(p => ({ ...p })))
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  ListPresetTypes.mockResolvedValue(TYPES.map(x => ({ ...x })))
  GetPresetsInstallStatus.mockResolvedValue([])
  ListCompoundPresets.mockResolvedValue([])
  for (const fn of [GetSDModels, GetSDSamplers, GetSDSchedulers, GetSDUpscalers, GetSDVAEs, GetSDLoRAs]) {
    fn.mockResolvedValue([])
  }
  givenPresets([...STYLES, ...EFFECTS])
})

afterEach(() => {
  vi.restoreAllMocks()
})

async function mountList(mode) {
  const props = mode === undefined ? {} : { mode }
  const wrapper = mount(PresetsPage, { props, attachTo: document.body })
  await settle()
  return wrapper
}

function cardNames(wrapper) {
  return wrapper.findAll('.preset-card').map(c => c.find('.preset-name').text())
}

function buttonByText(wrapper, text) {
  return wrapper.findAll('button').find(b => b.text().trim() === text.trim())
}

function typeFilterOptions(wrapper) {
  const select = wrapper.find('select.form-select')
  expect(select.exists()).toBe(true)
  return select.findAll('option').slice(1).map(o => ({ value: o.attributes('value'), text: o.text().trim() }))
}

function optionMatchesType(opt, type) {
  const labels = [type.name, effectTypeLabel(type.name, t)]
  return opt.value === String(type.id) || opt.value === type.name || labels.includes(opt.text)
}

describe('Effects tab: which presets each mode shows', () => {
  const cases = [
    { name: 'styles (explicit)', mode: 'styles', expected: STYLES },
    { name: 'styles (default mode)', mode: undefined, expected: STYLES },
    { name: 'effects', mode: 'effects', expected: EFFECTS },
  ]
  for (const c of cases) {
    it(`T1: mode ${c.name} shows only its presets`, async () => {
      const wrapper = await mountList(c.mode)
      expect(cardNames(wrapper).sort()).toEqual(c.expected.map(p => p.name).sort())
    })
  }

  it('T1: effect is decided by the type from ListPresetTypes (type_id), not by preset_type text', async () => {
    givenPresets([
      { id: 301, name: 'Mislabeled Effect Qx', type_id: 3, preset_type: '' },
      { id: 302, name: 'Mislabeled Style Qx', type_id: 1, preset_type: 'post' },
    ])
    const effects = await mountList('effects')
    expect(cardNames(effects)).toEqual(['Mislabeled Effect Qx'])
    effects.unmount()
    const styles = await mountList('styles')
    expect(cardNames(styles)).toEqual(['Mislabeled Style Qx'])
  })
})

describe('Effects tab: type filter', () => {
  it('T2: effects filter offers effect groups and no style types', async () => {
    const wrapper = await mountList('effects')
    const opts = typeFilterOptions(wrapper)
    expect(opts.length).toBeGreaterThan(0)
    for (const st of STYLE_TYPES) {
      expect(opts.some(o => optionMatchesType(o, st))).toBe(false)
    }
    for (const o of opts) {
      expect(EFFECT_TYPES.some(et => optionMatchesType(o, et))).toBe(true)
    }
  })

  it('T2: styles filter offers no effect types', async () => {
    const wrapper = await mountList('styles')
    const opts = typeFilterOptions(wrapper)
    for (const et of EFFECT_TYPES) {
      expect(opts.some(o => optionMatchesType(o, et))).toBe(false)
    }
    expect(opts.some(o => o.text.toLowerCase().startsWith('post'))).toBe(false)
  })
})

describe('Effects tab: select all works on the current tab only', () => {
  const cases = [
    { mode: 'effects', count: EFFECTS.length },
    { mode: 'styles', count: STYLES.length },
  ]
  for (const c of cases) {
    it(`T3: select all on ${c.mode} counts ${c.count}`, async () => {
      const wrapper = await mountList(c.mode)
      const selectBtn = buttonByText(wrapper, t('presets.btn_select'))
      expect(selectBtn).toBeTruthy()
      await selectBtn.trigger('click')
      await settle()
      const allBtn = buttonByText(wrapper, t('presets.btn_select_all'))
      expect(allBtn).toBeTruthy()
      await allBtn.trigger('click')
      await settle()
      expect(buttonByText(wrapper, t('presets.btn_export', { count: c.count }))).toBeTruthy()
      const total = STYLES.length + EFFECTS.length
      expect(buttonByText(wrapper, t('presets.btn_export', { count: total }))).toBeFalsy()
    })
  }
})

describe('Effects tab: export and delete act on the current tab only', () => {
  const cases = [
    { mode: 'styles', ids: STYLES.map(p => p.id) },
    { mode: 'effects', ids: EFFECTS.map(p => p.id) },
  ]

  async function selectAll(wrapper) {
    await buttonByText(wrapper, t('presets.btn_select')).trigger('click')
    await settle()
    await buttonByText(wrapper, t('presets.btn_select_all')).trigger('click')
    await settle()
  }

  for (const c of cases) {
    it(`T3: export on ${c.mode} sends only its presets`, async () => {
      const wrapper = await mountList(c.mode)
      await selectAll(wrapper)
      await buttonByText(wrapper, t('presets.btn_export', { count: c.ids.length })).trigger('click')
      await settle()
      expect(PreparePresetsExport).toHaveBeenCalledTimes(1)
      expect([...PreparePresetsExport.mock.calls[0][0]].sort()).toEqual([...c.ids].sort())
    })

    it(`T3: delete selected on ${c.mode} deletes only its presets`, async () => {
      const wrapper = await mountList(c.mode)
      await selectAll(wrapper)
      await buttonByText(wrapper, t('presets.btn_delete_selected', { count: c.ids.length })).trigger('click')
      await settle()
      const confirm = wrapper.find('.modal').findAll('button').find(b => b.text() === t('presets.btn_delete'))
      expect(confirm).toBeTruthy()
      await confirm.trigger('click')
      await settle()
      expect(DeletePreset.mock.calls.map(a => a[0]).sort()).toEqual([...c.ids].sort())
    })
  }
})

describe('Effects tab: new preset defaults', () => {
  async function openNew(mode) {
    const wrapper = await mountList(mode)
    const btn = buttonByText(wrapper, t('presets.btn_new'))
    expect(btn).toBeTruthy()
    await btn.trigger('click')
    await settle()
    const form = wrapper.findComponent(PresetForm)
    expect(form.exists()).toBe(true)
    return form
  }

  it('T4: new on effects opens the form with the base effect type', async () => {
    const form = await openNew('effects')
    const defaults = form.props('defaults')
    expect(defaults).toBeTruthy()
    expect(defaults.type_id).toBe(2)
    expect(defaults.preset_type).toBe('post')
  })

  it('T4: new on styles opens the form without a type', async () => {
    const form = await openNew('styles')
    const defaults = form.props('defaults') || {}
    expect(defaults.type_id).toBeUndefined()
    expect(defaults.preset_type).toBeUndefined()
  })
})

describe('Effects tab: empty state', () => {
  it('T5: only styles exist: effects tab shows empty state and no cards', async () => {
    givenPresets(STYLES)
    const wrapper = await mountList('effects')
    expect(wrapper.find('.empty-state').exists()).toBe(true)
    expect(wrapper.findAll('.preset-card').length).toBe(0)
    expect(wrapper.find('.empty-state').text()).toContain('Эффектов пока нет')
    expect(wrapper.find('.page-title').text()).toBe('Эффекты')
  })

  it('T5: only effects exist: styles tab shows empty state and no cards', async () => {
    givenPresets(EFFECTS)
    const wrapper = await mountList('styles')
    expect(wrapper.find('.empty-state').exists()).toBe(true)
    expect(wrapper.findAll('.preset-card').length).toBe(0)
    expect(wrapper.find('.empty-state').text()).toContain('Стилей пока нет')
    expect(wrapper.find('.page-title').text()).toBe('Стили')
  })
})

describe('Effects tab: UnifiedPresetsPage tabs', () => {
  it('T6: i18n has unified.tab_effects in ru and en', () => {
    expect(ru['unified.tab_effects']).toBe('Эффекты')
    expect(en['unified.tab_effects']).toBe('Effects')
  })

  it('T6: three tabs in order Styles, Effects, Workflows; second tab renders effects list', async () => {
    const wrapper = mount(UnifiedPresetsPage, { attachTo: document.body })
    await settle()
    const tabs = wrapper.findAll('.tabs .tab').map(b => b.text().trim())
    expect(tabs).toEqual([t('unified.tab_presets'), t('unified.tab_effects'), t('unified.tab_pipelines')])

    const first = wrapper.findComponent(PresetsPage)
    expect(first.exists()).toBe(true)
    expect(first.props('mode') ?? 'styles').toBe('styles')

    await wrapper.findAll('.tabs .tab')[1].trigger('click')
    await settle()
    const page = wrapper.findComponent(PresetsPage)
    expect(page.exists()).toBe(true)
    expect(page.props('mode')).toBe('effects')
    expect(cardNames(wrapper).sort()).toEqual(EFFECTS.map(p => p.name).sort())
  })
})
