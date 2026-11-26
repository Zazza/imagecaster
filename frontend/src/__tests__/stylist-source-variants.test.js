import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { api } from '../api.js'
import { readImageFile } from '../imageFile.js'
import { usePostEditor } from '../composables/usePostEditor.js'
import { adoptSourceFrames, sourceSubmitting } from '../composables/useStylistPlan.js'
import { setLocale } from '../i18n/index.js'
import { settle } from './helpers/async.js'
import SourceVariantsPanel from '../components/stylist/SourceVariantsPanel.vue'

vi.mock('../api.js', () => ({ api: { stylistSourcePlan: vi.fn(), stylistSourceShoot: vi.fn() } }))
vi.mock('../imageFile.js', () => ({ readImageFile: vi.fn() }))
vi.mock('../composables/usePostEditor.js', () => ({ usePostEditor: vi.fn() }))
vi.mock('../composables/useStylistPlan.js', async () => {
  const { ref } = await vi.importActual('vue')
  return { adoptSourceFrames: vi.fn(), sourceSubmitting: ref(false) }
})
vi.mock('../components/post/SettingsPanel.vue', () => ({ default: { template: '<div />' } }))

enableAutoUnmount(afterEach)

const DEFAULT_COUNT = 3
const MAX_COUNT = 8
const LORA_WEIGHT = 0.75
const FIRST_JOB_ID = 41
const SECOND_JOB_ID = 42
const ORIGINAL = 'SYNTHETIC_ORIGINAL_PHOTO'
const NEXT_ORIGINAL = 'SYNTHETIC_NEXT_PHOTO'
const PLAN = ['Turn slightly left, keeping the setting.', 'Look over the shoulder.', 'Relax the arms.']
const tid = id => `[data-testid="${id}"]`
const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

let editor
beforeEach(() => {
  vi.clearAllMocks()
  sourceSubmitting.value = false
  setLocale('ru')
  editor = {
    ready: ref(true),
    snapshot: vi.fn(() => ({ model: 'selected-editor.gguf', loras: [{ name: 'ChosenStyle', weight: LORA_WEIGHT }] })),
  }
  usePostEditor.mockReturnValue(editor)
  readImageFile.mockResolvedValue(ORIGINAL)
  api.stylistSourcePlan.mockResolvedValue([...PLAN])
  api.stylistSourceShoot.mockResolvedValue({ job_ids: [FIRST_JOB_ID, SECOND_JOB_ID], error: '' })
})

async function upload(wrapper, filename = 'source.png') {
  const input = wrapper.get(tid('source-photo-input'))
  Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['synthetic'], filename, { type: 'image/png' })] })
  await input.trigger('change')
  await settle()
}

async function plannedPanel() {
  const wrapper = mount(SourceVariantsPanel)
  await upload(wrapper)
  await wrapper.get(tid('source-plan')).trigger('click')
  await settle()
  return wrapper
}

const instructions = wrapper => wrapper.findAll(tid('source-instruction'))
const runDisabled = wrapper => wrapper.get(tid('source-run')).element.disabled

describe('Stylist source photo variants', () => {
  it('requires an uploaded photo, starts with three variants and needs no person', async () => {
    const wrapper = mount(SourceVariantsPanel)
    expect(Number(wrapper.get(tid('source-count')).element.value)).toBe(DEFAULT_COUNT)
    expect(wrapper.get(tid('source-plan')).element.disabled).toBe(true)
    await wrapper.get(tid('source-plan')).trigger('click')
    expect(api.stylistSourcePlan).not.toHaveBeenCalled()
    await upload(wrapper)
    expect(wrapper.emitted('source').at(-1)).toEqual([ORIGINAL])
    expect(wrapper.get(tid('source-plan')).element.disabled).toBe(false)
    await wrapper.get(tid('source-wish')).setValue('More relaxed poses')
    await wrapper.get(tid('source-plan')).trigger('click')
    await settle()
    expect(api.stylistSourcePlan).toHaveBeenCalledWith({ image_base64: ORIGINAL, wish: 'More relaxed poses', count: DEFAULT_COUNT, locale: 'ru' })
    expect(instructions(wrapper).map(input => input.element.value)).toEqual(PLAN)
    expect(runDisabled(wrapper)).toBe(false)
  })

  it('sends the selected language and requested count to planning', async () => {
    setLocale('en')
    api.stylistSourcePlan.mockResolvedValue(PLAN.slice(0, 2))
    const wrapper = mount(SourceVariantsPanel)
    await upload(wrapper)
    await wrapper.get(tid('source-count')).setValue('2')
    await wrapper.get(tid('source-wish')).setValue('standing poses')
    await wrapper.get(tid('source-plan')).trigger('click')
    await settle()
    expect(api.stylistSourcePlan).toHaveBeenCalledWith({ image_base64: ORIGINAL, wish: 'standing poses', count: 2, locale: 'en' })
  })

  it('does not fabricate a plan when image reading or planning fails', async () => {
    readImageFile.mockRejectedValueOnce(new Error('Unsupported image'))
    const wrapper = mount(SourceVariantsPanel)
    await upload(wrapper)
    expect(wrapper.get(tid('source-error')).text()).toContain('Unsupported image')
    expect(wrapper.get(tid('source-plan')).element.disabled).toBe(true)
    await upload(wrapper)
    api.stylistSourcePlan.mockRejectedValueOnce(new Error('Vision unavailable'))
    await wrapper.get(tid('source-plan')).trigger('click')
    await settle()
    expect(wrapper.get(tid('source-error')).text()).toContain('Vision unavailable')
    expect(instructions(wrapper)).toHaveLength(0)
    expect(api.stylistSourceShoot).not.toHaveBeenCalled()
  })

  it.each([
    ['empty', []],
    ['blank instruction', [PLAN[0], '  ', PLAN[2]]],
    ['wrong count', PLAN.slice(0, 2)],
  ])('blocks enqueue for a %s planning response', async (_, response) => {
    api.stylistSourcePlan.mockResolvedValue(response)
    const wrapper = await plannedPanel()
    expect(runDisabled(wrapper)).toBe(true)
    await wrapper.get(tid('source-run')).trigger('click')
    expect(api.stylistSourceShoot).not.toHaveBeenCalled()
  })

  it.each(['0', String(MAX_COUNT + 1), '2.5'])('rejects invalid variant count %s before requesting a plan', async count => {
    const wrapper = mount(SourceVariantsPanel)
    await upload(wrapper)
    await wrapper.get(tid('source-count')).setValue(count)
    expect(wrapper.get(tid('source-plan')).element.disabled).toBe(true)
    await wrapper.get(tid('source-plan')).trigger('click')
    expect(api.stylistSourcePlan).not.toHaveBeenCalled()
  })

  it.each(['wish', 'count', 'photo'])('discards a pending plan after the %s changes', async field => {
    const pending = deferred()
    api.stylistSourcePlan.mockReturnValue(pending.promise)
    const wrapper = mount(SourceVariantsPanel)
    await upload(wrapper)
    await wrapper.get(tid('source-plan')).trigger('click')
    await settle()
    if (field === 'wish') await wrapper.get(tid('source-wish')).setValue('Different wish')
    if (field === 'count') await wrapper.get(tid('source-count')).setValue('2')
    if (field === 'photo') {
      readImageFile.mockResolvedValueOnce(NEXT_ORIGINAL)
      await upload(wrapper, 'next.png')
    }
    pending.resolve([...PLAN])
    await settle()
    expect(instructions(wrapper)).toHaveLength(0)
    expect(runDisabled(wrapper)).toBe(true)
  })

  it('does not let an earlier file read replace the newest selected photo', async () => {
    const firstRead = deferred()
    const secondRead = deferred()
    readImageFile.mockReturnValueOnce(firstRead.promise).mockReturnValueOnce(secondRead.promise)
    const wrapper = mount(SourceVariantsPanel)
    await upload(wrapper, 'first.png')
    await upload(wrapper, 'second.png')
    secondRead.resolve(NEXT_ORIGINAL)
    await settle()
    firstRead.resolve(ORIGINAL)
    await settle()
    await wrapper.get(tid('source-plan')).trigger('click')
    await settle()
    expect(api.stylistSourcePlan.mock.calls[0][0].image_base64).toBe(NEXT_ORIGINAL)
    expect(wrapper.emitted('source').at(-1)).toEqual([NEXT_ORIGINAL])
  })

  it('blocks duplicate plan clicks and discards answers after unmount', async () => {
    const pending = deferred()
    api.stylistSourcePlan.mockReturnValue(pending.promise)
    const wrapper = mount(SourceVariantsPanel)
    await upload(wrapper)
    await wrapper.get(tid('source-plan')).trigger('click')
    await wrapper.get(tid('source-plan')).trigger('click')
    expect(api.stylistSourcePlan).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    pending.resolve([...PLAN])
    await settle()
    expect(api.stylistSourceShoot).not.toHaveBeenCalled()
    expect(wrapper.emitted('queued')).toBeUndefined()
  })

  it('allows editing and deleting variants while preserving the remaining plan', async () => {
    const wrapper = await plannedPanel()
    const changed = 'Cross the arms, keeping the original clothes and setting.'
    await instructions(wrapper)[0].setValue(changed)
    await wrapper.findAll(tid('source-remove'))[1].trigger('click')
    expect(Number(wrapper.get(tid('source-count')).element.value)).toBe(DEFAULT_COUNT - 1)
    expect(instructions(wrapper).map(input => input.element.value)).toEqual([changed, PLAN[2]])
    expect(runDisabled(wrapper)).toBe(false)
    await wrapper.get(tid('source-run')).trigger('click')
    await settle()
    expect(api.stylistSourceShoot.mock.calls[0][0]).toMatchObject({ image_base64: ORIGINAL, instructions: [changed, PLAN[2]] })
  })

  it('disables enqueue for blank instructions and a changed plan count', async () => {
    const wrapper = await plannedPanel()
    await instructions(wrapper)[0].setValue('   ')
    expect(runDisabled(wrapper)).toBe(true)
    await instructions(wrapper)[0].setValue(PLAN[0])
    expect(runDisabled(wrapper)).toBe(false)
    await wrapper.get(tid('source-count')).setValue('2')
    expect(runDisabled(wrapper)).toBe(true)
    await wrapper.get(tid('source-run')).trigger('click')
    expect(api.stylistSourceShoot).not.toHaveBeenCalled()
  })

  it('captures the selected editor and LoRAs before enqueue and prevents duplicate jobs', async () => {
    const chosen = { model: 'selected-editor.gguf', loras: [{ name: 'ChosenStyle', weight: LORA_WEIGHT }] }
    editor.snapshot.mockReturnValue(chosen)
    const pending = deferred()
    api.stylistSourceShoot.mockReturnValue(pending.promise)
    const wrapper = await plannedPanel()
    await wrapper.get(tid('source-run')).trigger('click')
    expect(runDisabled(wrapper)).toBe(true)
    expect(wrapper.emitted('busy').at(-1)).toEqual([true])
    chosen.model = 'later-editor.gguf'
    chosen.loras[0].name = 'LaterStyle'
    await wrapper.get(tid('source-run')).trigger('click')
    expect(api.stylistSourceShoot).toHaveBeenCalledTimes(1)
    expect(api.stylistSourceShoot.mock.calls[0][0]).toEqual({
      image_base64: ORIGINAL,
      instructions: PLAN,
      editor: { model: 'selected-editor.gguf', loras: [{ name: 'ChosenStyle', weight: LORA_WEIGHT }] },
    })
    pending.resolve({ job_ids: [FIRST_JOB_ID, SECOND_JOB_ID], error: '' })
    await settle()
    expect(wrapper.emitted('queued').at(-1)).toEqual([{ job_ids: [FIRST_JOB_ID, SECOND_JOB_ID], error: '' }])
    expect(adoptSourceFrames).toHaveBeenCalledWith([FIRST_JOB_ID, SECOND_JOB_ID])
    expect(wrapper.emitted('busy').at(-1)).toEqual([false])
  })

  it('keeps accepted IDs trackable when enqueue only partially succeeds', async () => {
    const result = { job_ids: [FIRST_JOB_ID], error: 'Second variant could not be queued' }
    api.stylistSourceShoot.mockResolvedValue(result)
    const wrapper = await plannedPanel()
    await wrapper.get(tid('source-run')).trigger('click')
    await settle()
    expect(wrapper.emitted('queued').at(-1)).toEqual([result])
    expect(adoptSourceFrames).toHaveBeenCalledWith(result.job_ids)
    expect(wrapper.get(tid('source-error')).text()).toContain(result.error)
    expect(instructions(wrapper).map(input => input.element.value)).toEqual(PLAN)
  })

  it('keeps enqueue failures visible and releases its busy state', async () => {
    api.stylistSourceShoot.mockRejectedValue(new Error('Queue unavailable'))
    const wrapper = await plannedPanel()
    await wrapper.get(tid('source-run')).trigger('click')
    await settle()
    expect(wrapper.get(tid('source-error')).text()).toContain('Queue unavailable')
    expect(wrapper.emitted('busy').at(-1)).toEqual([false])
    expect(wrapper.emitted('queued')).toBeUndefined()
    expect(adoptSourceFrames).not.toHaveBeenCalled()
    expect(instructions(wrapper).map(input => input.element.value)).toEqual(PLAN)
  })

  it.each(['', 'Some variants were not queued'])('preserves accepted job IDs after navigation with error %j', async error => {
    const pending = deferred()
    api.stylistSourceShoot.mockReturnValue(pending.promise)
    const wrapper = await plannedPanel()
    await wrapper.get(tid('source-run')).trigger('click')
    expect(adoptSourceFrames).not.toHaveBeenCalled()
    wrapper.unmount()
    pending.resolve({ job_ids: [FIRST_JOB_ID, SECOND_JOB_ID], error })
    await settle()
    expect(adoptSourceFrames).toHaveBeenCalledExactlyOnceWith([FIRST_JOB_ID, SECOND_JOB_ID])
  })

  it('does not replace the current shoot when the backend accepts no jobs', async () => {
    api.stylistSourceShoot.mockResolvedValue({ job_ids: [], error: 'No variants accepted' })
    const wrapper = await plannedPanel()
    await wrapper.get(tid('source-run')).trigger('click')
    await settle()
    expect(adoptSourceFrames).not.toHaveBeenCalled()
    expect(wrapper.get(tid('source-error')).text()).toContain('No variants accepted')
  })

  it.each(['accepted', 'rejected'])('blocks a second enqueue after remount until the first request is %s', async outcome => {
    const pending = deferred()
    api.stylistSourceShoot.mockReturnValueOnce(pending.promise)
    const first = await plannedPanel()
    await first.get(tid('source-run')).trigger('click')
    expect(sourceSubmitting.value).toBe(true)
    first.unmount()
    const second = mount(SourceVariantsPanel)
    await settle()
    expect(second.get(tid('source-plan')).element.disabled).toBe(true)
    expect(runDisabled(second)).toBe(true)
    await second.get(tid('source-plan')).trigger('click')
    await second.get(tid('source-run')).trigger('click')
    expect(api.stylistSourcePlan).toHaveBeenCalledTimes(1)
    expect(api.stylistSourceShoot).toHaveBeenCalledTimes(1)
    if (outcome === 'accepted') pending.resolve({ job_ids: [FIRST_JOB_ID], error: '' })
    else pending.reject(new Error('Queue unavailable after navigation'))
    await settle()
    expect(sourceSubmitting.value).toBe(false)
    await upload(second)
    expect(second.get(tid('source-plan')).element.disabled).toBe(false)
    await second.get(tid('source-plan')).trigger('click')
    await settle()
    await second.get(tid('source-run')).trigger('click')
    await settle()
    expect(api.stylistSourceShoot).toHaveBeenCalledTimes(2)
  })

  it('blocks enqueue while the editor is unavailable or the parent locks the panel', async () => {
    const wrapper = await plannedPanel()
    editor.ready.value = false
    await settle()
    expect(runDisabled(wrapper)).toBe(true)
    editor.ready.value = true
    await wrapper.setProps({ locked: true })
    expect(runDisabled(wrapper)).toBe(true)
    expect(wrapper.get(tid('source-plan')).element.disabled).toBe(true)
    await wrapper.get(tid('source-run')).trigger('click')
    expect(api.stylistSourceShoot).not.toHaveBeenCalled()
  })
})
