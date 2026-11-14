import { describe, it, expect, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import CustomPipelineEditor from '../components/CustomPipelineEditor.vue'
import { newCustomStep, customStepsPayload } from '../customSteps.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const PRESETS = [
  { id: 1, name: 'Plain', loras: '' },
  { id: 2, name: 'Detail', loras: '[{"name":"detail","weight":0.6}]' },
  { id: 3, name: 'Ink', loras: '' },
]

function step(id, strength = 0.5) {
  return { ...newCustomStep(), preset_id: id, denoising_strength: strength }
}

function mountEditor(steps) {
  const w = mount(CustomPipelineEditor, {
    props: {
      modelValue: steps,
      presets: PRESETS,
      'onUpdate:modelValue': v => w.setProps({ modelValue: v }),
    },
  })
  return w
}

function lastValue(w) {
  const ev = w.emitted('update:modelValue')
  return ev ? ev[ev.length - 1][0] : undefined
}

function stepEls(w) {
  return w.findAll('.custom-step')
}

function stepButton(w, idx, labelKey) {
  return stepEls(w)[idx].find(`button[aria-label="${t(labelKey)}"]`)
}

function addButton(w) {
  return w.findAll('button').find(b => b.text() === t('custom.btn_add_step'))
}

describe('custom pipeline editor', () => {
  it('add appends an empty step to the emitted list', async () => {
    const w = mountEditor([step(1)])
    await addButton(w).trigger('click')
    expect(lastValue(w)).toEqual([step(1), newCustomStep()])
    expect(stepEls(w)).toHaveLength(2)
  })

  it('add is disabled at 8 steps and emits nothing', async () => {
    const eight = Array.from({ length: 8 }, (_, i) => step((i % 3) + 1))
    const w = mountEditor(eight)
    expect(addButton(w).attributes('disabled')).toBeDefined()
    expect(w.text()).toContain(t('custom.max_steps_hint', { count: 8 }))
    await addButton(w).trigger('click')
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })

  it('remove drops that step; the only step cannot be removed', async () => {
    const w = mountEditor([step(1), step(2), step(3)])
    await stepButton(w, 1, 'custom.step_remove').trigger('click')
    expect(lastValue(w).map(s => s.preset_id)).toEqual([1, 3])

    const single = mountEditor([step(1)])
    expect(stepButton(single, 0, 'custom.step_remove').attributes('disabled')).toBeDefined()
  })

  it('move up / move down swap neighbours; edges are disabled', async () => {
    const w = mountEditor([step(1), step(2, 0.3), step(3, 0.7)])
    expect(stepButton(w, 0, 'custom.step_move_up').attributes('disabled')).toBeDefined()
    expect(stepButton(w, 2, 'custom.step_move_down').attributes('disabled')).toBeDefined()

    await stepButton(w, 2, 'custom.step_move_up').trigger('click')
    expect(lastValue(w).map(s => s.preset_id)).toEqual([1, 3, 2])

    await stepButton(w, 0, 'custom.step_move_down').trigger('click')
    expect(lastValue(w).map(s => [s.preset_id, s.denoising_strength])).toEqual([[3, 0.7], [1, 0.5], [2, 0.3]])
  })

  it('first step is txt2img and has no strength field; later steps have one', () => {
    const w = mountEditor([step(1), step(2)])
    const [first, second] = stepEls(w)
    expect(first.text()).toContain(t('custom.step_label', { num: 1, type: t('custom.step_txt2img') }))
    expect(second.text()).toContain(t('custom.step_label', { num: 2, type: t('custom.step_img2img') }))
    expect(first.find('.custom-denoise').exists()).toBe(false)
    expect(second.find('.custom-denoise input').exists()).toBe(true)
  })

  it.each([
    ['0', 0.05],
    ['3', 1],
    ['0.35', 0.35],
    ['', 0.5],
  ])('strength %j on a later step becomes %j', async (typed, want) => {
    const steps = [step(1), step(2)]
    const w = mountEditor(steps)
    const input = stepEls(w)[1].find('.custom-denoise input')
    await input.setValue(typed)
    await input.trigger('change')
    expect(steps[1].denoising_strength).toBe(want)
  })

  it('choosing a style for an untouched step takes the style LoRAs, payload sends none', async () => {
    const steps = [newCustomStep()]
    const w = mountEditor(steps)
    const input = stepEls(w)[0].find('.ss-input')
    await input.trigger('focus')
    await input.setValue('Detail')
    await w.findAll('.ss-option').find(o => o.text().includes('Detail')).trigger('click')
    expect(steps[0].preset_id).toBe(2)
    expect(steps[0].loraRows).toEqual([{ name: 'detail', weight: 0.6 }])
    expect(customStepsPayload(steps, PRESETS)).toEqual([{ preset_id: 2, denoising_strength: 0.5 }])
  })

  it('LoRA controls are hidden in kids mode', () => {
    const w = mount(CustomPipelineEditor, { props: { modelValue: [step(2)], presets: PRESETS, disableLoras: true } })
    expect(w.find('.custom-loras').exists()).toBe(false)
    expect(w.find('.custom-lora-badge').exists()).toBe(false)
  })
})
