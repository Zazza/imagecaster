import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import PresetOverridesPanel from '../components/PresetOverridesPanel.vue'
import { GetSDModels, GetSDSamplers, GetSDSchedulers, GetSDLoRAs } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const FRONTEND_OVERRIDE_KEYS = ['model_name', 'sampler', 'schedule_type', 'steps', 'cfg_scale', 'clip_skip', 'loras']

beforeEach(() => {
  GetSDModels.mockImplementation(() => Promise.resolve([{ title: 'z-image-turbo-fp8-aio' }]))
  GetSDSamplers.mockImplementation(() => Promise.resolve([{ name: 'Euler' }]))
  GetSDSchedulers.mockImplementation(() => Promise.resolve([{ name: 'Simple' }]))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([{ name: 'detail' }]))
})

async function mountPanel(presetLoras = []) {
  let w = null
  w = mount(PresetOverridesPanel, {
    props: {
      modelValue: null,
      presetLoras,
      'onUpdate:modelValue': v => w?.setProps({ modelValue: v }),
    },
  })
  await w.find('.fold-head').trigger('click')
  await settle()
  return w
}

function sent(w) {
  return w.props('modelValue')
}

function numberInput(w, labelKey) {
  const group = w.findAll('.form-group').find(g => g.find('.form-label').text() === t(labelKey))
  return group.find('input')
}

function selectFor(w, labelKey) {
  const group = w.findAll('.form-group').find(g => g.find('.form-label').text() === t(labelKey))
  return group.find('select')
}

describe('preset overrides payload', () => {
  it('nothing changed -> no overrides at all', async () => {
    const w = await mountPanel([{ name: 'detail', weight: 0.6 }])
    expect(sent(w)).toBeNull()
    expect((w.emitted('update:modelValue') || []).every(e => e[0] === null)).toBe(true)
  })

  it.each([
    ['overrides.steps', '500', 'steps', 150],
    ['overrides.steps', '149.6', 'steps', 150],
    ['overrides.steps', '20', 'steps', 20],
    ['overrides.cfg', '99', 'cfg_scale', 30],
    ['overrides.cfg', '4.5', 'cfg_scale', 4.5],
    ['overrides.clip_skip', '40', 'clip_skip', 12],
    ['overrides.clip_skip', '2', 'clip_skip', 2],
  ])('%s = %s sends %s=%j', async (labelKey, typed, field, want) => {
    const w = await mountPanel()
    await numberInput(w, labelKey).setValue(typed)
    expect(sent(w)).toEqual({ [field]: want })
  })

  it.each([
    ['overrides.steps', '0'],
    ['overrides.steps', '-5'],
    ['overrides.cfg', '0'],
    ['overrides.clip_skip', '-1'],
  ])('%s = %s is not sent', async (labelKey, typed) => {
    const w = await mountPanel()
    await numberInput(w, labelKey).setValue(typed)
    expect(sent(w)).toBeNull()
  })

  it('model, sampler and scheduler use the keys the Go side parses', async () => {
    const w = await mountPanel()
    await selectFor(w, 'overrides.model').setValue('z-image-turbo-fp8-aio')
    await selectFor(w, 'overrides.sampler').setValue('Euler')
    await selectFor(w, 'overrides.scheduler').setValue('Simple')
    await numberInput(w, 'overrides.steps').setValue('8')
    await numberInput(w, 'overrides.cfg').setValue('1.5')
    await numberInput(w, 'overrides.clip_skip').setValue('2')
    const ov = sent(w)
    expect(ov).toEqual({
      model_name: 'z-image-turbo-fp8-aio',
      sampler: 'Euler',
      schedule_type: 'Simple',
      steps: 8,
      cfg_scale: 1.5,
      clip_skip: 2,
    })
    expect(Object.keys(ov).every(k => FRONTEND_OVERRIDE_KEYS.includes(k))).toBe(true)
  })

  it('ignore LoRAs sends loras as an empty JSON list string', async () => {
    const w = await mountPanel([{ name: 'detail', weight: 0.6 }])
    await w.find('.overrides-check input').setValue(true)
    expect(sent(w)).toEqual({ loras: '[]' })
  })

  it('edited LoRA weight sends the whole list as a JSON string', async () => {
    const w = await mountPanel([{ name: 'detail', weight: 0.6 }])
    await w.find('.lora-row input[type="number"]').setValue('1.1')
    const ov = sent(w)
    expect(typeof ov.loras).toBe('string')
    expect(JSON.parse(ov.loras)).toEqual([{ name: 'detail', weight: 1.1 }])
  })

  it('added LoRA row with a name is appended; weight out of range is sanitized', async () => {
    const w = await mountPanel([{ name: 'detail', weight: 0.6 }])
    await w.findAll('button').find(b => b.text() === t('overrides.loras_add')).trigger('click')
    const rows = w.findAll('.lora-row')
    await rows[1].find('input[type="text"]').setValue('  extra  ')
    await rows[1].find('input[type="number"]').setValue('7')
    expect(JSON.parse(sent(w).loras)).toEqual([{ name: 'detail', weight: 0.6 }, { name: 'extra', weight: 2 }])
  })

  it('removing the only style LoRA sends an empty list', async () => {
    const w = await mountPanel([{ name: 'detail', weight: 0.6 }])
    await w.find(`button[aria-label="${t('overrides.loras_remove')}"]`).trigger('click')
    expect(sent(w)).toEqual({ loras: '[]' })
  })

  it('a parent reset to null clears the fields', async () => {
    const w = await mountPanel()
    await numberInput(w, 'overrides.steps').setValue('30')
    expect(sent(w)).toEqual({ steps: 30 })
    await w.setProps({ modelValue: null })
    await settle()
    expect(numberInput(w, 'overrides.steps').element.value).toBe('')
    expect(sent(w)).toBeNull()
  })
})
