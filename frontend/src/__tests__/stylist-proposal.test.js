import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { settle } from './helpers/index.js'
import { clearEventMocks } from './mocks/runtime'
import { StylistPresets, ListPeople, FaceSwapStatus, StylistAppearance, GetQueue, StylistFrames, StylistLastShoot } from '../wailsjs/go/main/App.js'
import StylistPage from '../components/StylistPage.vue'
import { api } from '../api.js'
import { setLocale } from '../i18n/index.js'
import { applyPlanSnapshot, planSnapshot, resetStylistPlan, useStylistPlan } from '../composables/useStylistPlan.js'

enableAutoUnmount(afterEach)
const tid = id => `[data-testid="${id}"]`
const proposal = () => ({
 concept: 'Night in the city', concept_ru: 'Ночь в городе', light: 'Side light', light_ru: 'Боковой свет', focus: 'Person and architecture', focus_ru: 'Человек и архитектура', target: 'base:Z-Image', warnings: [],
 series: [{ outfit: 'wool coat', outfit_ru: 'Шерстяное пальто', hair: 'bun', hair_ru: 'Пучок', makeup: 'neutral', makeup_ru: 'Нейтральный макияж', background: 'night street', background_ru: 'Ночная улица', direction: 'Hard side light', direction_ru: 'Жёсткий боковой свет', shots: [{ prompt: 'standing by a window', prompt_ru: 'Стоит у окна', framing: 'full', landscape: false }] }],
})
let state, shoot
beforeEach(() => {
 vi.restoreAllMocks(); clearEventMocks(); setLocale('ru'); resetStylistPlan(); state = useStylistPlan(); state.form.frames = 1
 StylistPresets.mockResolvedValue({ version: 1, themes: [], outfit: [], hair: [], makeup: [], background: [], pose: [] })
 ListPeople.mockResolvedValue([{ name: 'Example', base: 'Z-Image', lora: 'example_z', gender: 'woman', identity: true }])
 FaceSwapStatus.mockResolvedValue('ok'); StylistAppearance.mockResolvedValue(''); GetQueue.mockResolvedValue([]); StylistFrames.mockResolvedValue([]); StylistLastShoot.mockResolvedValue([])
 vi.spyOn(api, 'stylistSuggest').mockResolvedValue(proposal()); shoot = vi.spyOn(api, 'stylistShoot').mockResolvedValue([])
})
async function pageWithProposal(locale = 'ru') {
 setLocale(locale); const page = mount(StylistPage); await settle()
 await page.get(tid('stylist-person')).setValue('Example'); await page.get(tid('stylist-planning-mode')).setValue('auto')
 await page.get(tid('stylist-plan')).trigger('click'); await settle(); return page
}
it('Russian completed plan shows its translated concept and editable series immediately', async () => {
 const page = await pageWithProposal()
 const text = page.get(tid('stylist-applied-concept')).text()
 for (const value of ['Ночь в городе', 'Боковой свет', 'Человек и архитектура']) expect(text).toContain(value)
 expect(page.get(tid('series-outfit')).element.value).toBe('Шерстяное пальто')
 expect(page.get(tid('shot-prompt')).element.value).toBe('Стоит у окна')
 expect(page.find(tid('stylist-plan-empty')).exists()).toBe(false)
 expect(page.find(tid('stylist-auto-proposal')).exists()).toBe(false)
 expect(page.find(tid('stylist-auto-apply')).exists()).toBe(false)
 expect(page.get(tid('stylist-shoot')).element.disabled).toBe(false)
 expect(shoot).not.toHaveBeenCalled()
})
it('English completed plan uses original generation texts', async () => {
 const page = await pageWithProposal('en')
 expect(page.get(tid('stylist-applied-concept')).text()).toContain('Night in the city')
 expect(page.get(tid('series-outfit')).element.value).toBe('wool coat')
 expect(page.get(tid('shot-prompt')).element.value).toBe('standing by a window')
})
it('new planning defaults leave automatic model choice unlocked', () => { expect(state.form.targetLocked).toBe(false) })
it('user manual to auto unlocks selection while restored explicitly pinned auto stays pinned', async () => {
 const page = mount(StylistPage); await settle(); state.form.targetLocked = true
 await page.get(tid('stylist-planning-mode')).setValue('auto'); await settle(); expect(state.form.targetLocked).toBe(false)
 state.form.targetLocked = true; const saved = planSnapshot(); resetStylistPlan(); expect(applyPlanSnapshot(saved)).toBe(true); await settle()
 expect(state.form.planningMode).toBe('auto'); expect(state.form.targetLocked).toBe(true)
})
it('garment proposal explains the photo garment rather than a default outfit', async () => {
 const page = mount(StylistPage); await settle()
 await page.get(tid('stylist-person')).setValue('Example')
 Object.assign(state.form, { garment: 'SYNTHETIC_PRIVATE_IMAGE', garmentOutfit: 'Красное шерстяное пальто' })
 await page.get(tid('stylist-planning-mode')).setValue('auto')
 await page.get(tid('stylist-plan')).trigger('click'); await settle()
 const outfit = page.get(tid('series-card')).text()
 expect(outfit).toMatch(/с фото|из фото|по фото/i)
 expect(state.form.garmentOutfit).toBe('Красное шерстяное пальто')
 expect(page.find(tid('series-outfit')).exists()).toBe(false)
 expect(shoot).not.toHaveBeenCalled()
})
for (const pinned of [false, true]) {
 it(`${pinned ? 'pinned' : 'unlocked'} auto application ${pinned ? 'preserves' : 'clears'} overrides even for the same model`, async () => {
  const overrides = { steps: 37, cfg: 8, sampler: 'manual sampler' }
  Object.assign(state.form, { person: 'Example', planningMode: 'auto', target: 'base:Z-Image', targetKind: 'base', targetLocked: pinned, overrides })
  await state.suggestPlan()
  expect(state.form.target).toBe('base:Z-Image')
  expect(state.form.overrides).toEqual(pinned ? overrides : null)
  expect(shoot).not.toHaveBeenCalled()
 })
}
for (const mode of ['auto', 'mixed']) {
 it(`${mode} hides technical model controls until explicitly pinned`, async () => {
  const page = mount(StylistPage); await settle()
  await page.get(tid('stylist-planning-mode')).setValue(mode); await settle()
  expect(page.find(tid('stylist-target')).exists()).toBe(false)
  expect(page.find(tid('stylist-kind-base')).exists()).toBe(false)
  await page.get(tid('stylist-target-locked')).setValue(true); await settle()
  expect(page.find(tid('stylist-target')).exists()).toBe(true)
  expect(page.find(tid('stylist-kind-base')).exists()).toBe(true)
  await page.get(tid('stylist-target-locked')).setValue(false); await settle()
  expect(page.find(tid('stylist-target')).exists()).toBe(false)
  await page.get(tid('stylist-planning-mode')).setValue('manual'); await settle()
  expect(page.find(tid('stylist-target')).exists()).toBe(true)
  expect(page.find(tid('stylist-kind-base')).exists()).toBe(true)
 })
}
it('legacy snapshot with no model pin restores manual controls visibly', async () => {
 const page = mount(StylistPage); await settle()
 expect(applyPlanSnapshot(JSON.stringify({ v: 1, form: { person: 'Example', frames: 1 }, series: [] }))).toBe(true)
 await settle()
 expect(state.form.planningMode).toBe('manual')
 expect(state.form.targetLocked).toBe(false)
 expect(page.find(tid('stylist-target')).exists()).toBe(true)
 expect(page.find(tid('stylist-kind-base')).exists()).toBe(true)
})
