import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { settle } from './helpers/index.js'
import { clearEventMocks } from './mocks/runtime'
import { ListPresets, ListPresetTypes, ListCompoundPresets, GetModelCatalog, StylistPresets, ListPeople, FaceSwapStatus, StylistAppearance, GetQueue, StylistFrames, StylistLastShoot } from '../wailsjs/go/main/App.js'
import StylistPage from '../components/StylistPage.vue'

enableAutoUnmount(afterEach)
import { api } from '../api.js'
import { setLocale } from '../i18n/index.js'
import { applyPlanSnapshot, planSnapshot, resetStylistPlan, useStylistPlan } from '../composables/useStylistPlan.js'

const target = 'preset:12'
const proposal = () => ({
  concept: 'Night in the city', light: 'Side light', focus: 'Person and architecture', target,
  series: [{ outfit: 'jacket', hair: 'bun', makeup: 'neutral', background: 'night street', direction: 'Hard side light', shots: [{ prompt: 'standing', framing: 'full' }] }],
  warnings: [],
})

let state
let suggest
let shoot
beforeEach(() => {
  vi.restoreAllMocks()
  resetStylistPlan()
  state = useStylistPlan()
  Object.assign(state.form, { person: 'Example', frames: 1, planningMode: 'auto', wish: 'night portrait', target, targetKind: 'preset' })
  suggest = vi.spyOn(api, 'stylistSuggest').mockResolvedValue(proposal())
  shoot = vi.spyOn(api, 'stylistShoot').mockResolvedValue([])
})

it('immediately replaces the old plan with the selected shooting plan without shooting', async () => {
  const old = [{ outfit: 'existing', shots: [{ prompt: 'existing pose' }] }]
  state.series.value = old
  await state.suggestPlan()
  expect(state.series.value).toEqual(proposal().series)
  expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(state.series.value).toEqual(proposal().series)
  expect(state.form.shootConcept).toMatchObject({ concept: 'Night in the city', light: 'Side light', focus: 'Person and architecture' })
  expect(shoot).not.toHaveBeenCalled()
})

it('sends selected constraints in mixed mode and preserves them after applying', async () => {
  const aspects = {
    outfit: { mode: 'pick', ids: ['o01'], text: '' },
    hair: { mode: 'custom', ids: [], text: 'neat bun' },
    makeup: { mode: 'keep', ids: [], text: '' },
    background: { mode: 'auto', ids: [], text: '' },
    pose: { mode: 'pick', ids: ['p01'], text: '' },
  }
  Object.assign(state.form, { planningMode: 'mixed', targetLocked: true, aspects })
  await state.suggestPlan()
  expect(suggest).toHaveBeenCalledOnce()
  expect(suggest.mock.calls[0][0]).toMatchObject({ target, plan: { wish: 'night portrait', aspects } })
  expect(state.form.aspects).toEqual(aspects)
  expect(state.form.target).toBe(target)
})

it('allows auto selection of technical target only when unlocked', async () => {
  state.form.targetLocked = false
  await state.suggestPlan()
  expect(suggest.mock.calls[0][0].target || '').toBe('')
  expect(state.form.target).toBe(target)
  expect(state.form.targetKind).toBe('preset')
})

it('does not send garment image data to planning and preserves the garment', async () => {
  Object.assign(state.form, { garment: 'SYNTHETIC_PRIVATE_IMAGE', garmentOutfit: 'red wool coat' })
  await state.suggestPlan()
  const req = suggest.mock.calls[0][0]
  expect(req.garment_outfit).toBe('red wool coat')
  expect(req.plan.garment).toBe(true)
  expect(JSON.stringify(req)).not.toContain('SYNTHETIC_PRIVATE_IMAGE')
  expect(state.form.garment).toBe('SYNTHETIC_PRIVATE_IMAGE')
})

it('keeps the current plan and exposes an error when planning fails', async () => {
  state.series.value = [{ outfit: 'existing', shots: [] }]
  suggest.mockRejectedValue(new Error('planner is unavailable'))
  await state.suggestPlan()
  expect(state.series.value).toEqual([{ outfit: 'existing', shots: [] }])
  expect(state.error.value).toContain('planner is unavailable')
  expect(state.planning.value).toBe(false)
})

it('rejects responses for a changed form and prevents overlapping requests', async () => {
  let resolve
  suggest.mockImplementation(() => new Promise(done => { resolve = done }))
  const pending = state.suggestPlan()
  await Promise.resolve()
  await state.suggestPlan()
  expect(suggest).toHaveBeenCalledOnce()
  state.form.person = 'Another person'
  resolve(proposal())
  await pending
  expect(state.appliedSelectionCurrent.value).toBe(false)
  expect(state.series.value).toEqual([])
  expect(shoot).not.toHaveBeenCalled()
})

it('keeps manual edits in the installed plan and prevents an empty shoot', async () => {
  await state.suggestPlan()
  state.series.value = [{ outfit: 'manually edited', shots: [] }]
  await state.shoot(false)
  expect(shoot).not.toHaveBeenCalled()
  expect(state.series.value).toEqual([{ outfit: 'manually edited', shots: [] }])
})

it('persists planning choices and restores old plans with safe manual defaults', async () => {
  await state.suggestPlan()
  const snapshot = planSnapshot()
  resetStylistPlan()
  expect(applyPlanSnapshot(snapshot)).toBe(true)
  expect(state.form.planningMode).toBe('auto')
  expect(state.form.shootConcept).toMatchObject({ concept: 'Night in the city' })
  expect(state.series.value[0].direction).toBe('Hard side light')
  expect(applyPlanSnapshot(JSON.stringify({ v: 1, form: { person: 'Legacy', frames: 1 }, series: [] }))).toBe(true)
  expect(state.form.planningMode).toBe('manual')
  expect(state.form.targetLocked).toBe(false)
  expect(state.form.shootConcept).toBeNull()
})


it('installs auto selection through the page without a separate apply button or generation', async () => {
  clearEventMocks()
  resetStylistPlan()
  state.form.frames = 1
  StylistPresets.mockResolvedValue({ version: 1, themes: [], outfit: [], hair: [], makeup: [], background: [], pose: [] })
  ListPeople.mockResolvedValue([{ name: 'Example', base: 'Z-Image', lora: 'example_z', gender: 'woman', identity: true }])
  FaceSwapStatus.mockResolvedValue('ok')
  StylistAppearance.mockResolvedValue('')
  GetQueue.mockResolvedValue([])
  StylistFrames.mockResolvedValue([])
  StylistLastShoot.mockResolvedValue([])
  suggest.mockResolvedValue({ ...proposal(), target: 'base:Z-Image' })
  const page = mount(StylistPage)
  await settle()
  await page.get('[data-testid="stylist-person"]').setValue('Example')
  await page.get('[data-testid="stylist-planning-mode"]').setValue('auto')
  await page.get('[data-testid="stylist-plan"]').trigger('click')
  await settle()
  expect(suggest).toHaveBeenCalledOnce()
  expect(page.find('[data-testid="stylist-auto-proposal"]').exists()).toBe(false)
  expect(page.findAll('[data-testid="series-card"]')).toHaveLength(1)
  expect(state.series.value).toEqual(proposal().series)
  expect(page.find('[data-testid="stylist-auto-apply"]').exists()).toBe(false)
  expect(page.get('[data-testid="stylist-shoot"]').element.disabled).toBe(false)
  expect(page.get('[data-testid="stylist-applied-concept"]').text()).toContain('Night in the city')
  expect(state.series.value).toEqual(proposal().series)
  expect(shoot).not.toHaveBeenCalled()
})


it('sends raw Russian constraints only through the protected planning API without translation first', async () => {
  setLocale('ru')
  const translate = vi.spyOn(api, 'stylistTranslate').mockImplementation(texts => Promise.resolve(texts.map(() => 'translated private text')))
  Object.assign(state.form, {
    planningMode: 'mixed',
    garment: 'SYNTHETIC_PRIVATE_IMAGE',
    garmentOutfit: 'красное шерстяное пальто',
    aspects: {
      ...state.form.aspects,
      hair: { mode: 'custom', ids: [], text: 'аккуратный пучок' },
    },
  })
  suggest.mockRejectedValue(new Error('planning requires a local server'))
  await state.suggestPlan()
  expect(translate).not.toHaveBeenCalled()
  expect(suggest).toHaveBeenCalledOnce()
  const request = suggest.mock.calls[0][0]
  expect(request.plan.aspects.hair.text).toBe('аккуратный пучок')
  expect(request.garment_outfit).toBe('красное шерстяное пальто')
  expect(JSON.stringify(request)).not.toContain('SYNTHETIC_PRIVATE_IMAGE')
  expect(state.error.value).toContain('planning requires a local server')
})

for (const presentInCatalog of [true, false]) {
 it(`keeps applied ordinary preset and enables shooting; present in UI catalog: ${presentInCatalog}`, async () => {
  clearEventMocks()
  resetStylistPlan()
  state.form.frames = 1
  const presetID = 42
  const typeID = 10
  const model = 'synthetic-moss.safetensors'
  GetModelCatalog.mockResolvedValue({ sd_models: [{ name: 'synthetic-moss', filename: model, base: 'Moss-Image' }] })
  const preset = { id: presetID, type_id: typeID, name: 'Moss Boudoir Moody', preset_type: 'general', model_name: model, loras: '' }
  ListPresets.mockResolvedValue(presentInCatalog ? [preset] : [{ ...preset, id: presetID + 1 }])
  ListPresetTypes.mockResolvedValue([{ id: typeID, name: 'Portrait' }])
  ListCompoundPresets.mockResolvedValue([])
  StylistPresets.mockResolvedValue({ version: 1, themes: [], outfit: [], hair: [], makeup: [], background: [], pose: [] })
  ListPeople.mockResolvedValue([{ name: 'Synthetic', base: 'Moss-Image', lora: 'synthetic_moss', gender: 'woman' }])
  FaceSwapStatus.mockResolvedValue('not_installed')
  let finishAppearance
  StylistAppearance.mockImplementation(() => new Promise(resolve => { finishAppearance = resolve }))
  GetQueue.mockResolvedValue([])
  StylistFrames.mockResolvedValue([])
  StylistLastShoot.mockResolvedValue([])
  suggest.mockResolvedValue({ ...proposal(), target: `preset:${presetID}`, target_name: 'Moss Boudoir Moody' })
  const page = mount(StylistPage)
  await settle()
  await page.get('[data-testid="stylist-planning-mode"]').setValue('auto')
  await page.get('[data-testid="stylist-plan"]').trigger('click')
  await settle()
  expect(shoot).not.toHaveBeenCalled()
  finishAppearance('Synthetic background appearance')
  await settle()
  expect(page.find('[data-testid="stylist-auto-apply"]').exists()).toBe(false)
  expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(shoot).not.toHaveBeenCalled()
  expect(state.form.shootConcept.target).toBe(`preset:${presetID}`)
  expect(state.form.target).toBe(`preset:${presetID}`)
  expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(page.get('[data-testid="stylist-shoot"]').element.disabled).toBe(false)
  expect(shoot).not.toHaveBeenCalled()
  await page.get('[data-testid="stylist-shoot"]').trigger('click')
  await settle()
  expect(shoot).toHaveBeenCalledOnce()
  expect(shoot.mock.calls[0][0]).toMatchObject({ preset: presetID, appearance: 'Synthetic background appearance' })
  expect(shoot.mock.calls[0][0].series.flatMap(series => series.shots)).toHaveLength(state.form.frames)
 })
}

it('keeps the immediately installed plan valid when background appearance metadata arrives', async () => {
  clearEventMocks()
  resetStylistPlan()
  state.form.frames = 1
  StylistPresets.mockResolvedValue({ version: 1, themes: [], outfit: [], hair: [], makeup: [], background: [], pose: [] })
  ListPeople.mockResolvedValue([{ name: 'Synthetic', base: 'Z-Image', lora: 'synthetic_z', gender: 'woman' }])
  let finishAppearance
  StylistAppearance.mockImplementation(() => new Promise(resolve => { finishAppearance = resolve }))
  GetQueue.mockResolvedValue([])
  StylistFrames.mockResolvedValue([])
  StylistLastShoot.mockResolvedValue([])
  suggest.mockResolvedValue({ ...proposal(), target: 'base:Z-Image' })
  const page = mount(StylistPage)
  await settle()
  await page.get('[data-testid="stylist-planning-mode"]').setValue('auto')
  await page.get('[data-testid="stylist-plan"]').trigger('click')
  await settle()
  expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(page.find('[data-testid="stylist-auto-apply"]').exists()).toBe(false)
  expect(state.appliedSelectionCurrent.value).toBe(true)
  finishAppearance('Synthetic appearance metadata loaded in background')
  await settle()
  expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(page.find('[data-testid="stylist-auto-apply"]').exists()).toBe(false)
  expect(state.appliedSelectionCurrent.value).toBe(true)
})

it('keeps a ready proposal current after changing generation-only controls', async () => {
  await state.suggestPlan()
  Object.assign(state.form, { appearance: 'background metadata', seed: '123', frameSize: 'large', faceSwap: false, facePass: true, overrides: { steps: 37 } })
  expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(state.appliedSelectionCurrent.value).toBe(true)
})

for (const change of [
  s => { s.form.wish = 'another photographic intent' },
  s => { s.form.themes = ['night'] },
  s => { s.form.person = 'Another synthetic person' },
  s => { s.form.garment = 'SYNTHETIC_NEW_GARMENT' },
  s => { s.form.targetLocked = true },
]) {
  it('invalidates a ready proposal when actual planning inputs change', async () => {
    await state.suggestPlan()
    change(state)
    expect(state.appliedSelectionCurrent.value).toBe(false)
  })
}

it('tracks mixed constraints but ignores inactive auto constraints and an unlocked manual target', async () => {
  await state.suggestPlan()
  state.form.aspects.hair = { mode: 'custom', ids: [], text: 'synthetic bun' }
  expect(state.appliedSelectionCurrent.value).toBe(true)
  state.form.target = 'base:Another cached target'
  expect(state.appliedSelectionCurrent.value).toBe(false)
  state.form.planningMode = 'mixed'
  await state.suggestPlan()
  state.form.aspects.hair.text = 'synthetic loose hair'
  expect(state.appliedSelectionCurrent.value).toBe(false)
})

it('invalidates a ready proposal after changing the selected garment description or pinned target', async () => {
  state.form.garment = 'SYNTHETIC_GARMENT'
  await state.suggestPlan()
  state.form.garmentOutfit = 'synthetic wool coat'
  expect(state.appliedSelectionCurrent.value).toBe(false)
  state.form.targetLocked = true
  await state.suggestPlan()
  state.form.target = 'base:Another pinned target'
  expect(state.appliedSelectionCurrent.value).toBe(false)
})
