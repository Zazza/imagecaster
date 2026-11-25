import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { settle } from './helpers/index.js'
import { clearEventMocks } from './mocks/runtime'
import { StylistPresets, ListPeople, FaceSwapStatus, StylistAppearance, GetQueue, StylistFrames, StylistLastShoot } from '../wailsjs/go/main/App.js'
import StylistPage from '../components/StylistPage.vue'
import { setLocale } from '../i18n/index.js'
import { resetStylistPlan, useStylistPlan } from '../composables/useStylistPlan.js'

enableAutoUnmount(afterEach)
const tid = id => `[data-testid="${id}"]`
const disabled = control => control.element.disabled || !!control.element.closest('fieldset[disabled]')
const lookControls = page => page.get(tid('stylist-section-look')).findAll('button, input, select, textarea')
let state
beforeEach(() => {
  vi.restoreAllMocks(); clearEventMocks(); setLocale('ru'); resetStylistPlan(); state = useStylistPlan()
  StylistPresets.mockResolvedValue({
    version: 1, themes: [{ id: 'city', name: 'Город' }],
    outfit: [{ id: 'coat', name: 'Пальто', prompt: 'coat' }],
    hair: [], makeup: [], background: [{ id: 'street', name: 'Улица', prompt: 'street', place: 'outside' }], pose: [],
  })
  ListPeople.mockResolvedValue([{ name: 'Example', base: 'Z-Image', lora: 'example_z', gender: 'woman', identity: true }])
  FaceSwapStatus.mockResolvedValue('ok'); StylistAppearance.mockResolvedValue(''); GetQueue.mockResolvedValue([]); StylistFrames.mockResolvedValue([]); StylistLastShoot.mockResolvedValue([])
})
async function pageReady() {
  const page = mount(StylistPage); await settle()
  await page.get(tid('stylist-section-look')).get('.card-header').trigger('click')
  return page
}
async function mode(page, value) {
  await page.get(tid('stylist-planning-mode')).setValue(value); await settle()
}
function expectControlsDisabled(page, expected) {
  const controls = lookControls(page)
  expect(controls.length).toBeGreaterThan(0)
  for (const control of controls) expect(disabled(control), control.attributes('data-testid')).toBe(expected)
}

it('auto disables tabs and choices, then mixed and manual restore availability', async () => {
  const page = await pageReady()
  await page.get(tid('mode-pick')).trigger('click')
  expect(page.find(tid('pick-coat')).exists()).toBe(true)
  expectControlsDisabled(page, false)
  await mode(page, 'auto')
  expectControlsDisabled(page, true)
  expect(page.get(tid('stylist-section-look')).text()).toMatch(/смешанн/i)
  await mode(page, 'mixed')
  expectControlsDisabled(page, false)
  await mode(page, 'manual')
  expectControlsDisabled(page, false)
})

it('keeps custom text and selected IDs through auto and restores editable choices', async () => {
  const page = await pageReady()
  await page.get(tid('mode-pick')).trigger('click')
  await page.get(tid('pick-coat')).trigger('click')
  await page.get(tid('tab-hair')).trigger('click')
  await page.get(tid('mode-custom')).trigger('click')
  await page.get(tid('custom-text')).setValue('Loose curls')
  const choices = JSON.parse(JSON.stringify(state.form.aspects))
  await mode(page, 'auto')
  expectControlsDisabled(page, true)
  expect(state.form.aspects).toEqual(choices)
  for (const nextMode of ['mixed', 'manual']) {
    await mode(page, nextMode)
    expectControlsDisabled(page, false)
    expect(page.get(tid('custom-text')).element.value).toBe('Loose curls')
    expect(state.form.aspects).toEqual(choices)
  }
  await page.get(tid('custom-text')).setValue('Braids')
  expect(state.form.aspects.hair.text).toBe('Braids')
  await page.get(tid('tab-outfit')).trigger('click')
  expect(page.get(tid('pick-coat')).classes()).toContain('active')
})

it('auto leaves themes and garment controls available outside Look', async () => {
  const page = await pageReady()
  await mode(page, 'auto')
  for (const id of ['theme-city', 'stylist-garment-pick', 'stylist-garment-paste']) expect(disabled(page.get(tid(id)))).toBe(false)
  await page.get(tid('theme-city')).trigger('click')
  expect(state.form.themes).toContain('city')
  state.form.garment = 'SYNTHETIC_PRIVATE_IMAGE'; await settle()
  for (const id of ['stylist-garment-clear', 'stylist-garment-outfit', 'stylist-garment-describe']) expect(disabled(page.get(tid(id)))).toBe(false)
  await page.get(tid('stylist-garment-outfit')).setValue('Red coat')
  expect(state.form.garmentOutfit).toBe('Red coat')
  await page.get(tid('stylist-garment-clear')).trigger('click')
  expect(state.form.garment).toBe('')
})
