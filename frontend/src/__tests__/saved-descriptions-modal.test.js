import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount, DOMWrapper } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => (await import('./mocks/wails-app')).default)

import SavedDescriptionsModal from '../components/SavedDescriptionsModal.vue'

enableAutoUnmount(afterEach)

const DESCS = [
  { id: 1, name: 'Alpha forest', text: 'a quiet forest', type: '', negative_prompt: '', created_at: '2026-01-01T10:00:00Z' },
  { id: 2, name: 'Bravo city', text: 'neon city at night', type: '', negative_prompt: 'blur', created_at: '2026-03-01T10:00:00Z' },
  { id: 3, name: 'Charlie forest', text: 'misty forest lake', type: '', negative_prompt: '', created_at: '2026-02-01T10:00:00Z' },
]
const NEWEST_FIRST_IDS = [2, 3, 1]

function mountModal(props = {}, listeners = {}) {
  return mount(SavedDescriptionsModal, {
    props: {
      visible: true,
      descriptions: DESCS.map(d => ({ ...d })),
      batchTarget: 'Style: Moss Portrait',
      batchRunning: false,
      batchProgress: { done: 0, total: 0, errors: [] },
      batchDisabled: '',
      ...props,
    },
    attrs: listeners,
  })
}

const tid = (w, id) => w.find(`[data-testid="${id}"]`)
const tidAll = (w, id) => w.findAll(`[data-testid="${id}"]`)

function cardOf(checkEl) {
  let el = checkEl.element.parentElement
  while (el && !el.querySelector('[data-testid="sdm-use"]')) el = el.parentElement
  return el ? new DOMWrapper(el) : null
}

function cards(w) {
  return tidAll(w, 'sdm-check').map(cardOf)
}

function cardByName(w, name) {
  return cards(w).find(c => c.text().includes(name))
}

function selectedCount(w) {
  const m = tid(w, 'sdm-selected-count').text().match(/\d+/)
  return m ? Number(m[0]) : NaN
}

function isChecked(checkWrapper) {
  const el = checkWrapper.element
  if (el.tagName === 'INPUT') return el.checked
  const inner = el.querySelector('input[type="checkbox"]')
  if (inner) return inner.checked
  return el.getAttribute('aria-checked') === 'true'
}

async function browserClick(wrapper) {
  await wrapper.trigger('click')
  const el = wrapper.element
  if (el.tagName === 'INPUT' && (el.type === 'checkbox' || el.type === 'radio')) {
    await wrapper.trigger('input')
    await wrapper.trigger('change')
  }
  await flushPromises()
}

async function selectAll(w) {
  await browserClick(tid(w, 'sdm-select-all'))
}

async function clickCard(w, name) {
  const c = cardByName(w, name)
  expect(c, 'card for ' + name).toBeTruthy()
  await c.trigger('click')
  await flushPromises()
}

async function selectIdea(w, name) {
  const check = tidAll(w, 'sdm-check').find(c => cardOf(c).text().includes(name))
  expect(check, 'checkbox for ' + name).toBeTruthy()
  await browserClick(check)
}

async function search(w, q) {
  const input = w.find('.sdm-toolbar input.form-input')
  await input.setValue(q)
  await flushPromises()
}

async function setPerIdea(w, n) {
  await tid(w, 'sdm-per-idea').setValue(String(n))
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
})

describe('У1: closes only via the close button', () => {
  it('click on the overlay background does not emit close', async () => {
    const w = mountModal()
    await w.find('.modal-overlay').trigger('click')
    await flushPromises()
    expect(w.emitted('close')).toBeUndefined()
  })

  it('mousedown inside the window and release on the overlay does not emit close', async () => {
    const w = mountModal()
    const inside = w.find('.sdm-toolbar')
    const overlay = w.find('.modal-overlay')
    await inside.trigger('mousedown')
    await overlay.trigger('mouseup')
    await overlay.trigger('click')
    await flushPromises()
    expect(w.emitted('close')).toBeUndefined()
  })

  it('close button emits close', async () => {
    const w = mountModal()
    await w.find('.modal-close').trigger('click')
    expect(w.emitted('close')).toHaveLength(1)
  })
})

describe('У2: card click toggles selection, "use" button', () => {
  it('renders a checkbox and a use button per idea', () => {
    const w = mountModal()
    expect(tidAll(w, 'sdm-check')).toHaveLength(3)
    expect(tidAll(w, 'sdm-use')).toHaveLength(3)
  })

  it('clicking a card selects it, clicking again deselects it', async () => {
    const w = mountModal()
    expect(selectedCount(w)).toBe(0)
    await clickCard(w, 'Bravo city')
    expect(selectedCount(w)).toBe(1)
    const check = tidAll(w, 'sdm-check').find(c => cardOf(c).text().includes('Bravo city'))
    expect(isChecked(check)).toBe(true)
    await clickCard(w, 'Bravo city')
    expect(selectedCount(w)).toBe(0)
    expect(isChecked(check)).toBe(false)
  })

  it('clicking the idea text inside a card toggles its selection', async () => {
    const w = mountModal()
    const card = cardByName(w, 'Bravo city')
    const textEl = card.findAll('*').filter(e => e.element.children.length === 0 && e.text() === 'neon city at night')[0]
    expect(textEl, 'leaf element with idea text').toBeTruthy()
    await textEl.trigger('click')
    await flushPromises()
    expect(selectedCount(w)).toBe(1)
    await textEl.trigger('click')
    await flushPromises()
    expect(selectedCount(w)).toBe(0)
  })

  it('clicking the checkbox toggles selection exactly once', async () => {
    const w = mountModal()
    const check = tidAll(w, 'sdm-check')[0]
    await browserClick(check)
    expect(selectedCount(w)).toBe(1)
    expect(isChecked(check)).toBe(true)
    await browserClick(check)
    expect(selectedCount(w)).toBe(0)
    expect(isChecked(check)).toBe(false)
  })

  it('use button emits use with the idea and then close, without toggling selection', async () => {
    const log = []
    const w = mountModal({}, {
      onUse: (d) => log.push(['use', d]),
      onClose: () => log.push(['close']),
    })
    const card = cardByName(w, 'Charlie forest')
    await card.find('[data-testid="sdm-use"]').trigger('click')
    await flushPromises()
    expect(log.map(e => e[0])).toEqual(['use', 'close'])
    expect(log[0][1]).toEqual(expect.objectContaining({ id: 3, name: 'Charlie forest' }))
    expect(selectedCount(w)).toBe(0)
  })
})

describe('У3: select all, counter, deleted ideas', () => {
  it('select all selects every idea, second press clears', async () => {
    const w = mountModal()
    await selectAll(w)
    expect(selectedCount(w)).toBe(3)
    tidAll(w, 'sdm-check').forEach(c => expect(isChecked(c)).toBe(true))
    await selectAll(w)
    expect(selectedCount(w)).toBe(0)
  })

  it('select all when only some are selected selects all visible', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    expect(selectedCount(w)).toBe(1)
    await selectAll(w)
    expect(selectedCount(w)).toBe(3)
  })

  it('select all with search filter selects only visible ideas', async () => {
    const batch = vi.fn()
    const w = mountModal({}, { onBatch: batch })
    await search(w, 'forest')
    expect(tidAll(w, 'sdm-check')).toHaveLength(2)
    await selectAll(w)
    expect(selectedCount(w)).toBe(2)
    await search(w, '')
    expect(tidAll(w, 'sdm-check')).toHaveLength(3)
    expect(selectedCount(w)).toBe(2)
    await tid(w, 'sdm-batch-run').trigger('click')
    await flushPromises()
    expect(batch).toHaveBeenCalledTimes(1)
    expect([...batch.mock.calls[0][0].ids].sort()).toEqual([1, 3])
  })

  it('select all when all visible are selected deselects only the visible ones', async () => {
    const w = mountModal()
    await selectIdea(w, 'Bravo city')
    await search(w, 'forest')
    await selectAll(w)
    expect(selectedCount(w)).toBe(3)
    await selectAll(w)
    expect(selectedCount(w)).toBe(1)
  })

  it('an idea removed from descriptions drops out of the selection', async () => {
    const batch = vi.fn()
    const w = mountModal({}, { onBatch: batch })
    await selectAll(w)
    expect(selectedCount(w)).toBe(3)
    await w.setProps({ descriptions: DESCS.filter(d => d.id !== 3).map(d => ({ ...d })) })
    await flushPromises()
    expect(selectedCount(w)).toBe(2)
    await tid(w, 'sdm-batch-run').trigger('click')
    await flushPromises()
    expect(batch.mock.calls[0][0].ids).not.toContain(3)
    expect(batch.mock.calls[0][0].ids).toHaveLength(2)
  })
})

describe('У4: batch panel', () => {
  it('panel hidden when nothing is selected and no batch is running', () => {
    const w = mountModal()
    expect(tid(w, 'sdm-per-idea').exists()).toBe(false)
    expect(tid(w, 'sdm-batch-run').exists()).toBe(false)
  })

  it('panel shown after selecting one idea, with target label and default perIdea=1', async () => {
    const w = mountModal({ batchTarget: 'Style: Moss Portrait' })
    await selectIdea(w, 'Alpha forest')
    expect(tid(w, 'sdm-per-idea').exists()).toBe(true)
    expect(Number(tid(w, 'sdm-per-idea').element.value)).toBe(1)
    expect(tid(w, 'sdm-batch-total').text()).toMatch(/(^|\D)1(\D|$)/)
    expect(w.text()).toContain('Style: Moss Portrait')
  })

  it('total = perIdea × selected', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await selectIdea(w, 'Bravo city')
    await setPerIdea(w, 3)
    expect(tid(w, 'sdm-batch-total').text()).toMatch(/(^|\D)6(\D|$)/)
  })

  it('run emits batch with ids in display order (newest first) and perIdea', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await selectIdea(w, 'Bravo city')
    await selectIdea(w, 'Charlie forest')
    await setPerIdea(w, 4)
    await tid(w, 'sdm-batch-run').trigger('click')
    await flushPromises()
    expect(w.emitted('batch')).toHaveLength(1)
    expect(w.emitted('batch')[0][0]).toEqual({ ids: NEWEST_FIRST_IDS, perIdea: 4 })
  })

  it('ids order follows display order, not click order', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await selectIdea(w, 'Bravo city')
    await tid(w, 'sdm-batch-run').trigger('click')
    await flushPromises()
    expect(w.emitted('batch')[0][0].ids).toEqual([2, 1])
  })

  it('perIdea above 20 is clamped to 20 in the emitted batch', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await setPerIdea(w, 25)
    await tid(w, 'sdm-batch-run').trigger('click')
    await flushPromises()
    expect(w.emitted('batch')[0][0].perIdea).toBe(20)
  })

  it('perIdea 0 is clamped to 1 in the emitted batch', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await setPerIdea(w, 0)
    await tid(w, 'sdm-batch-run').trigger('click')
    await flushPromises()
    expect(w.emitted('batch')[0][0].perIdea).toBe(1)
  })

  it('run button disabled with batchDisabled reason shown, no emit', async () => {
    const w = mountModal({ batchDisabled: 'No style selected' })
    await selectIdea(w, 'Alpha forest')
    const run = tid(w, 'sdm-batch-run')
    expect(run.attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('No style selected')
    await run.trigger('click')
    expect(w.emitted('batch')).toBeUndefined()
  })

  it('run button disabled while batchRunning', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await w.setProps({ batchRunning: true, batchProgress: { done: 0, total: 1, errors: [] } })
    await flushPromises()
    const run = tid(w, 'sdm-batch-run')
    if (run.exists()) {
      expect(run.attributes('disabled')).toBeDefined()
      await run.trigger('click')
    }
    expect(w.emitted('batch')).toBeUndefined()
  })

  it('panel visible while batchRunning even with nothing selected; run disabled', async () => {
    const w = mountModal({ batchRunning: true, batchProgress: { done: 1, total: 3, errors: [] } })
    await flushPromises()
    expect(tid(w, 'sdm-batch-progress').exists()).toBe(true)
    const run = tid(w, 'sdm-batch-run')
    if (run.exists()) expect(run.attributes('disabled')).toBeDefined()
  })
})

describe('У5: progress, stop, errors', () => {
  it('shows done and total while running', async () => {
    const w = mountModal({ batchRunning: true, batchProgress: { done: 2, total: 5, errors: [] } })
    await flushPromises()
    const text = tid(w, 'sdm-batch-progress').text()
    expect(text).toMatch(/(^|\D)2(\D|$)/)
    expect(text).toMatch(/(^|\D)5(\D|$)/)
  })

  it('progress updates when batchProgress prop changes', async () => {
    const w = mountModal({ batchRunning: true, batchProgress: { done: 0, total: 7, errors: [] } })
    await w.setProps({ batchProgress: { done: 4, total: 7, errors: [] } })
    await flushPromises()
    expect(tid(w, 'sdm-batch-progress').text()).toMatch(/(^|\D)4(\D|$)/)
  })

  it('stop button emits batch-stop', async () => {
    const w = mountModal({ batchRunning: true, batchProgress: { done: 0, total: 2, errors: [] } })
    await flushPromises()
    await tid(w, 'sdm-batch-stop').trigger('click')
    expect(w.emitted('batch-stop')).toHaveLength(1)
  })

  it('stop button not shown when no batch is running', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    expect(tid(w, 'sdm-batch-stop').exists()).toBe(false)
  })

  it('after the batch, errors are listed with idea names', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await selectIdea(w, 'Bravo city')
    await tid(w, 'sdm-batch-run').trigger('click')
    await w.setProps({ batchRunning: true, batchProgress: { done: 0, total: 2, errors: [] } })
    await flushPromises()
    await w.setProps({
      batchRunning: false,
      batchProgress: { done: 2, total: 2, errors: [{ name: 'Bravo city', error: 'LLM timeout' }] },
    })
    await flushPromises()
    const errs = tid(w, 'sdm-batch-errors')
    expect(errs.exists()).toBe(true)
    expect(errs.text()).toContain('Bravo city')
  })

  it('no errors list when the batch finished without errors', async () => {
    const w = mountModal()
    await selectIdea(w, 'Alpha forest')
    await w.setProps({ batchRunning: false, batchProgress: { done: 1, total: 1, errors: [] } })
    await flushPromises()
    expect(tid(w, 'sdm-batch-errors').exists()).toBe(false)
  })
})
