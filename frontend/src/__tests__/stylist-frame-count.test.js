import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { settle } from './helpers/index.js'
import { clearEventMocks } from './mocks/runtime'
import { StylistPresets, ListPeople, FaceSwapStatus, StylistAppearance, GetQueue, StylistFrames, StylistLastShoot } from '../wailsjs/go/main/App.js'
import StylistPage from '../components/StylistPage.vue'
import { api } from '../api.js'
import { setLocale } from '../i18n/index.js'
import { applyPlanSnapshot, resetStylistPlan, useStylistPlan } from '../composables/useStylistPlan.js'
enableAutoUnmount(afterEach)
const tid = id => `[data-testid="${id}"]`
const seriesOf = counts => counts.map((count, i) => ({ outfit: 'coat', shots: Array.from({ length: count }, (_, j) => ({ prompt: `pose ${i}-${j}`, framing: 'full' })) }))
const proposal = counts => ({ target: 'base:Z-Image', concept: 'City', light: 'Daylight', focus: 'Person', warnings: [], series: seriesOf(counts) })
let state, suggest, shoot
beforeEach(() => {
  vi.restoreAllMocks(); clearEventMocks(); setLocale('ru'); resetStylistPlan(); state = useStylistPlan()
  Object.assign(state.form, { person: 'Example', frames: 5, target: 'base:Z-Image', targetKind: 'base' })
  StylistPresets.mockResolvedValue({ version: 1, themes: [], outfit: [], hair: [], makeup: [], background: [], pose: [] })
  ListPeople.mockResolvedValue([{ name: 'Example', base: 'Z-Image', lora: 'example_z', gender: 'woman', identity: true }])
  FaceSwapStatus.mockResolvedValue('ok'); StylistAppearance.mockResolvedValue(''); GetQueue.mockResolvedValue([]); StylistFrames.mockResolvedValue([]); StylistLastShoot.mockResolvedValue([])
  suggest = vi.spyOn(api, 'stylistSuggest').mockResolvedValue(proposal([4, 1]))
  vi.spyOn(api, 'stylistPlan').mockResolvedValue(proposal([4, 1]))
  shoot = vi.spyOn(api, 'stylistShoot').mockResolvedValue([])
})
for (const mode of ['manual', 'auto', 'mixed']) {
  it(`${mode}: restored three-shot plan cannot shoot when five are requested`, async () => {
    expect(applyPlanSnapshot(JSON.stringify({ v: 1, form: { ...state.form, planningMode: mode }, series: seriesOf([3]) }))).toBe(true)
    const page = mount(StylistPage); await settle()
    expect(page.get(tid('stylist-shoot')).attributes('disabled')).toBeDefined()
    const warning = page.get(tid('stylist-frame-count-mismatch')).text()
    expect(warning).toContain('5'); expect(warning).toContain('3')
    expect(warning).toMatch(/план.*кадр|кадр.*план/i)
    await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
  })
  it(`${mode}: malformed three-shot API result preserves the old plan`, async () => {
    state.form.planningMode = mode
    const old = seriesOf([2]); state.series.value = old
    suggest.mockResolvedValue(proposal([3])); api.stylistPlan.mockResolvedValue(proposal([3]))
    await state.makePlan()
    expect(state.series.value).toEqual(old)
    expect(state.error.value).toBeTruthy(); expect(shoot).not.toHaveBeenCalled()
  })
}
it('valid five-shot proposal applies both series and sends all five shots', async () => {
  state.form.planningMode = 'auto'; await state.suggestPlan()
  expect(state.appliedSelectionCurrent.value).toBe(true); expect(state.appliedSelectionCurrent.value).toBe(true)
  expect(shoot).not.toHaveBeenCalled(); await state.shoot(false)
  expect(shoot).toHaveBeenCalledOnce()
  expect(shoot.mock.calls[0][0].series.map(s => s.shots.length)).toEqual([4, 1])
})
it('raw number input invalidates proposal before blur and next request asks for five', async () => {
  Object.assign(state.form, { planningMode: 'auto', frames: 3 }); suggest.mockResolvedValue(proposal([3]))
  const page = mount(StylistPage); await settle(); await state.suggestPlan(); await settle()
  expect(state.appliedSelectionCurrent.value).toBe(true)
  const input = page.get(tid('stylist-frames')); input.element.value = '5'; await input.trigger('input')
  expect(state.form.frames).toBe(5)
  expect(page.get(tid('stylist-shoot')).element.disabled).toBe(true)
  await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
  suggest.mockResolvedValue(proposal([4, 1])); await state.suggestPlan()
  expect(suggest.mock.calls.at(-1)[0].plan.frames).toBe(5)
})
for (const deletion of ['shot', 'series']) {
  it(`intentional ${deletion} deletion updates requested count and preserves shooting`, async () => {
    state.series.value = seriesOf([4, 1])
    if (deletion === 'shot') state.deleteShot(0, 0)
    else state.deleteSeries(1)
    expect(state.form.frames).toBe(4); await state.shoot(false)
    expect(shoot).toHaveBeenCalledOnce(); expect(shoot.mock.calls[0][0].series.flatMap(s => s.shots)).toHaveLength(4)
    state.deleteSeries(0); if (state.series.value.length) state.deleteSeries(0)
    shoot.mockClear(); await state.shoot(false); expect(shoot).not.toHaveBeenCalled()
  })
}
for (const mode of ['manual', 'auto', 'mixed']) {
  it(`${mode}: direct shoot rejects three shots when five requested`, async () => {
    state.form.planningMode = mode; state.series.value = seriesOf([3])
    await state.shoot(false)
    expect(shoot).not.toHaveBeenCalled()
    expect(state.error.value).toBeTruthy()
  })
}
it('count changed while translating prevents a previously valid shoot from reaching API', async () => {
  state.series.value = seriesOf([5])
  state.series.value[0].shots[0] = { prompt_ru: 'руки у лица', ru_dirty: true }
  let finishTranslation
  const translate = vi.spyOn(api, 'stylistTranslate').mockImplementation(() => new Promise(resolve => { finishTranslation = resolve }))
  const pending = state.shoot(false)
  expect(translate).toHaveBeenCalledOnce()
  state.form.frames = 3
  finishTranslation(['hands by face'])
  await pending
  expect(shoot).not.toHaveBeenCalled()
  expect(state.error.value).toBeTruthy()
})
