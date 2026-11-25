import { beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api.js'
import { applyPlanSnapshot, applyTemplate, templateSnapshot, restorePlan, planSnapshot, resetStylistPlan, useStylistPlan } from '../composables/useStylistPlan.js'

const presetID = 42
const frames = 5
const shots = () => [{ outfit: 'wool coat', shots: Array.from({ length: frames }, () => ({ prompt: 'standing by window', framing: 'full' })) }]
const proposal = () => ({ target: `preset:${presetID}`, target_name: 'Natural skin', target_model: 'Flux.1', target_reason: 'Natural skin and wool texture', target_reason_ru: 'Естественная кожа и шерсть', concept: 'Window portrait', light: 'Soft window', focus: 'Skin and material', series: shots(), warnings: [] })
let state, shoot
beforeEach(() => {
 vi.restoreAllMocks(); resetStylistPlan(); state = useStylistPlan()
 Object.assign(state.form, { person: 'Synthetic', frames, target: 'base:Z-Image', targetKind: 'base', planningMode: 'manual', wish: 'natural portrait' })
 vi.spyOn(api, 'stylistSuggest').mockResolvedValue(proposal())
 shoot = vi.spyOn(api, 'stylistShoot').mockResolvedValue([])
})

it('blocks the previous manual Z-Image plan after switching to auto with the same frame count', async () => {
 state.series.value = shots(); state.form.planningMode = 'auto'
 await state.shoot(false)
 expect(shoot).not.toHaveBeenCalled()
 expect(state.error.value).toBeTruthy()
})

it('applies the selected Flux preset and queues all five shots without the old base or overrides', async () => {
 Object.assign(state.form, { planningMode: 'auto', overrides: { steps: 37 } })
 await state.suggestPlan(); expect(state.appliedSelectionCurrent.value).toBe(true)
 expect(shoot).not.toHaveBeenCalled()
 await state.shoot(false)
 expect(shoot).toHaveBeenCalledOnce()
 const request = shoot.mock.calls[0][0]
 expect(request.preset).toBe(presetID)
 expect(request).not.toHaveProperty('base'); expect(request).not.toHaveProperty('overrides')
 expect(request.series.flatMap(s => s.shots)).toHaveLength(frames)
})

it('restores proven selection but blocks a legacy auto plan with the same target and count', async () => {
 state.form.planningMode = 'auto'; await state.suggestPlan()
 const saved = planSnapshot(); resetStylistPlan(); expect(applyPlanSnapshot(saved)).toBe(true)
 await state.shoot(false); expect(shoot).toHaveBeenCalledOnce(); shoot.mockClear()
 const legacy = { v: 1, form: { person: 'Synthetic', frames, target: `preset:${presetID}`, targetKind: 'preset', planningMode: 'auto', wish: 'natural portrait' }, series: shots() }
 expect(applyPlanSnapshot(JSON.stringify(legacy))).toBe(true)
 await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
})

it('honors an explicitly pinned target and its overrides', async () => {
 Object.assign(state.form, { planningMode: 'auto', targetLocked: true, overrides: { steps: 37 } })
 api.stylistSuggest.mockResolvedValue({ ...proposal(), target: 'base:Z-Image', target_model: 'Z-Image' })
 await state.suggestPlan(); expect(api.stylistSuggest.mock.calls[0][0].target).toBe('base:Z-Image')
 await state.shoot(false)
 expect(shoot.mock.calls[0][0]).toMatchObject({ base: 'Z-Image', overrides: { steps: 37 } })
})

it('rechecks intent after asynchronous translation before sending a job', async () => {
 state.form.planningMode = 'auto'; await state.suggestPlan()
 state.series.value[0].shots[0] = { prompt_ru: 'у окна', ru_dirty: true }
 let finish
 vi.spyOn(api, 'stylistTranslate').mockImplementation(() => new Promise(resolve => { finish = resolve }))
 const pending = state.shoot(false)
 expect(finish).toBeTypeOf('function')
 state.form.wish = 'dramatic noir'
 finish(['by a window']); await pending
 expect(shoot).not.toHaveBeenCalled()
})

for (const change of [s => { s.form.wish = 'noir hard light' }, s => { s.form.themes = ['noir'] }, s => { s.form.person = 'Other synthetic' }, s => { s.form.garmentOutfit = 'silk dress' }]) {
 it('requires a fresh applied choice after changing photographic intent', async () => {
  state.form.planningMode = 'auto'; await state.suggestPlan()
  change(state); await state.shoot(false)
  expect(shoot).not.toHaveBeenCalled()
 })
}

it('preserves applied choice when changing seed, size and face settings or deleting a shot deliberately', async () => {
 state.form.planningMode = 'auto'; await state.suggestPlan()
 Object.assign(state.form, { seed: '123', frameSize: 'large', faceSwap: false }); state.deleteShot(0, 0)
 await state.shoot(false)
 expect(shoot).toHaveBeenCalledOnce()
 expect(shoot.mock.calls[0][0].series.flatMap(s => s.shots)).toHaveLength(frames - 1)
})

it('restores applied garment choice when persisted image bytes were compressed', async () => {
 Object.assign(state.form, { planningMode: 'auto', garment: 'SYNTHETIC_ORIGINAL_IMAGE_BYTES', garmentOutfit: 'wool coat' })
 await state.suggestPlan(); expect(state.appliedSelectionCurrent.value).toBe(true)
 const saved = planSnapshot()
 vi.spyOn(api, 'stylistLoadPlan').mockResolvedValue(saved)
 vi.spyOn(api, 'stylistLoadGarment').mockResolvedValue('SYNTHETIC_COMPRESSED_SAME_IMAGE_BYTES')
 resetStylistPlan(); await restorePlan()
 expect(state.form.garment).toBe('SYNTHETIC_COMPRESSED_SAME_IMAGE_BYTES')
 await state.shoot(false); expect(shoot).toHaveBeenCalledOnce()
 shoot.mockClear(); state.form.garment = 'SYNTHETIC_DIFFERENT_GARMENT_IMAGE'
 await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
})

for (const kind of ['template', 'snapshot']) {
it(`${kind} cannot validate its old applied choice against a different current garment photo`, async () => {
 Object.assign(state.form, { planningMode: 'auto', garment: 'SYNTHETIC_FIRST_GARMENT', garmentOutfit: 'wool coat' })
 await state.suggestPlan(); expect(state.appliedSelectionCurrent.value).toBe(true)
 const saved = kind === 'template' ? templateSnapshot() : planSnapshot()
 state.form.garment = 'SYNTHETIC_SECOND_GARMENT'
 expect(kind === 'template' ? applyTemplate(saved, 'Saved portrait') : applyPlanSnapshot(saved)).toBe(true)
 expect(state.form.garment).toBe('SYNTHETIC_SECOND_GARMENT')
 await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
})
}

it('exports reusable templates without private applied-selection inputs while session snapshots retain proof', async () => {
 const privatePerson = 'PRIVATE_PERSON_57bb58ba_9234'
 const privateGarmentDescription = 'PRIVATE_GARMENT_DESCRIPTION_134d9c2a'
 const privateImage = 'PRIVATE_SYNTHETIC_IMAGE_2a581951'
 Object.assign(state.form, { planningMode: 'auto', person: privatePerson, garment: privateImage, garmentOutfit: privateGarmentDescription })
 await state.suggestPlan(); expect(state.appliedSelectionCurrent.value).toBe(true)
 const session = planSnapshot()
 const template = templateSnapshot()
 for (const privateValue of [privatePerson, privateGarmentDescription, privateImage]) expect(template).not.toContain(privateValue)
 const exportedForm = JSON.parse(template).form
 expect(exportedForm).not.toHaveProperty('garmentIdentity')
 expect(exportedForm.shootConcept).not.toHaveProperty('selection_signature')
 expect(applyPlanSnapshot(session)).toBe(true)
 await state.shoot(false); expect(shoot).toHaveBeenCalledOnce(); shoot.mockClear()
 expect(applyTemplate(template, 'Reusable style')).toBe(true)
 await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
 expect(applyTemplate(session, 'Legacy exported style with nested proof')).toBe(true)
 await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
 await state.suggestPlan(); expect(state.appliedSelectionCurrent.value).toBe(true)
 await state.shoot(false); expect(shoot).toHaveBeenCalledOnce()
})

it('a manual template remains usable without automatic selection proof', async () => {
 state.series.value = shots()
 const saved = templateSnapshot()
 expect(applyTemplate(saved, 'Manual portrait')).toBe(true)
 await state.shoot(false)
 expect(shoot).toHaveBeenCalledOnce()
})

it('replaces a completed plan on another successful selection and preserves it on failure', async () => {
 state.form.planningMode = 'auto'
 await state.suggestPlan()
 const replacement = proposal()
 replacement.target = 'preset:81'
 replacement.series[0].outfit = 'silk coat'
 api.stylistSuggest.mockResolvedValueOnce(replacement)
 await state.suggestPlan()
 expect(state.form.target).toBe('preset:81')
 expect(state.series.value).toEqual(replacement.series)
 expect(state.appliedSelectionCurrent.value).toBe(true)
 api.stylistSuggest.mockRejectedValueOnce(new Error('planner unavailable'))
 await state.suggestPlan()
 expect(state.series.value).toEqual(replacement.series)
 expect(state.appliedSelectionCurrent.value).toBe(true)
 await state.shoot(false)
 expect(shoot.mock.calls[0][0].preset).toBe(81)
})

it('refuses a stale replacement and keeps old series without reusing incompatible proof', async () => {
 state.form.planningMode = 'auto'
 await state.suggestPlan()
 const previous = JSON.parse(JSON.stringify(state.series.value))
 let finish
 api.stylistSuggest.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
 const pending = state.suggestPlan()
 state.form.wish = 'different lighting and place'
 finish(proposal())
 await pending
 expect(state.series.value).toEqual(previous)
 expect(state.appliedSelectionCurrent.value).toBe(false)
 await state.shoot(false)
 expect(shoot).not.toHaveBeenCalled()
})
