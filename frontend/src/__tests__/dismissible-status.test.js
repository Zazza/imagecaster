import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import StatusMessage from '../components/StatusMessage.vue'
import ObjectSection from '../components/ObjectSection.vue'
import RepairSection from '../components/RepairSection.vue'
import { GetSettings, EditObject, RepairImage } from '../wailsjs/go/main/App.js'
import { t } from '../i18n/index.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'

const SOURCE = 'c291cmNl'

useFakeImage()
useFakeCanvas()

async function paintMask(wrapper) {
  wrapper.vm.beginStroke(10, 10, 5)
  wrapper.vm.strokeTo(12, 12, 5)
  wrapper.vm.endStroke()
  await settle()
}

const closeSelector = () => `button[aria-label="${t('common.close')}"]`

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
})

describe('StatusMessage', () => {
  it('default kind is error: classes status status-error, shows text', () => {
    const w = mount(StatusMessage, { props: { text: 'boom' } })
    const root = w.find('.status')
    expect(root.exists()).toBe(true)
    expect(root.classes()).toContain('status-error')
    expect(root.classes()).not.toContain('status-warning')
    expect(w.text()).toContain('boom')
  })

  it('kind=warning: classes status status-warning', () => {
    const w = mount(StatusMessage, { props: { text: 'stopped', kind: 'warning' } })
    const root = w.find('.status')
    expect(root.classes()).toContain('status-warning')
    expect(root.classes()).not.toContain('status-error')
    expect(w.text()).toContain('stopped')
  })

  it('has a close button labelled with common.close', () => {
    const w = mount(StatusMessage, { props: { text: 'boom' } })
    const btn = w.find(closeSelector())
    expect(btn.exists()).toBe(true)
    expect(btn.attributes('aria-label')).toBe(t('common.close'))
  })

  it('emits close on button click, once per click', async () => {
    const w = mount(StatusMessage, { props: { text: 'boom' } })
    expect(w.emitted('close')).toBeFalsy()
    await w.find(closeSelector()).trigger('click')
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('warning kind also emits close', async () => {
    const w = mount(StatusMessage, { props: { text: 'x', kind: 'warning' } })
    await w.find(closeSelector()).trigger('click')
    expect(w.emitted('close')).toHaveLength(1)
  })
})

describe('StatusMessage timeout', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('emits close by itself after timeout', () => {
    const w = mount(StatusMessage, { props: { text: 'queued', kind: 'success', timeout: 8000 } })
    vi.advanceTimersByTime(7999)
    expect(w.emitted('close')).toBeFalsy()
    vi.advanceTimersByTime(1)
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('without timeout never closes by itself', () => {
    const w = mount(StatusMessage, { props: { text: 'boom' } })
    vi.advanceTimersByTime(600000)
    expect(w.emitted('close')).toBeFalsy()
  })

  it('new text restarts the timer', async () => {
    const w = mount(StatusMessage, { props: { text: 'a', timeout: 8000 } })
    vi.advanceTimersByTime(5000)
    await w.setProps({ text: 'b' })
    vi.advanceTimersByTime(5000)
    expect(w.emitted('close')).toBeFalsy()
    vi.advanceTimersByTime(3000)
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('click closes once, timer does not fire again', async () => {
    const w = mount(StatusMessage, { props: { text: 'a', timeout: 8000 } })
    await w.find(closeSelector()).trigger('click')
    vi.advanceTimersByTime(10000)
    expect(w.emitted('close')).toHaveLength(1)
  })
})

const sections = [
  {
    name: 'ObjectSection',
    component: ObjectSection,
    runTestId: 'object-run',
    api: () => EditObject,
    message: 'server exploded',
  },
  {
    name: 'RepairSection',
    component: RepairSection,
    runTestId: 'repair-run',
    api: () => RepairImage,
    message: 'sd exploded',
  },
]

describe.each(sections)('$name: error message is dismissible', ({ component, runTestId, api, message }) => {
  async function mountWithError() {
    GetSettings.mockReturnValue(Promise.resolve({}))
    api().mockRejectedValue(message)
    const wrapper = mount(component, { props: { source: SOURCE, active: true }, attachTo: document.body })
    await settle()
    await paintMask(wrapper)
    await wrapper.find(`[data-testid="${runTestId}"]`).trigger('click')
    await settle()
    return wrapper
  }

  it('no error message before a failure', async () => {
    GetSettings.mockReturnValue(Promise.resolve({}))
    const wrapper = mount(component, { props: { source: SOURCE, active: true }, attachTo: document.body })
    await settle()
    expect(wrapper.find('.status-error').exists()).toBe(false)
  })

  it('failure shows .status.status-error with text and a close button', async () => {
    const wrapper = await mountWithError()
    const err = wrapper.find('.status-error')
    expect(err.exists()).toBe(true)
    expect(err.isVisible()).toBe(true)
    expect(err.classes()).toContain('status')
    expect(err.text()).toContain(message)
    expect(err.find(closeSelector()).exists()).toBe(true)
  })

  it('clicking close hides the error message', async () => {
    const wrapper = await mountWithError()
    await wrapper.find('.status-error').find(closeSelector()).trigger('click')
    await settle()
    expect(wrapper.find('.status-error').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(message)
  })

  it('after closing, a new failure shows the message again', async () => {
    const wrapper = await mountWithError()
    await wrapper.find('.status-error').find(closeSelector()).trigger('click')
    await settle()
    expect(wrapper.find('.status-error').exists()).toBe(false)
    await wrapper.find(`[data-testid="${runTestId}"]`).trigger('click')
    await settle()
    expect(wrapper.find('.status-error').exists()).toBe(true)
    expect(wrapper.text()).toContain(message)
  })
})
