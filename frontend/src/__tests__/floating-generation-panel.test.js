import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import FloatingGenerationPanel from '../components/FloatingGenerationPanel.vue'
import GenProgress from '../components/GenProgress.vue'

const PARENT_WIDTH = 800
const PARENT_HEIGHT = 600
const PANEL_WIDTH = 320
const PANEL_HEIGHT = 180
const STRIP_HEIGHT = 36
const SCREEN_LEFT = 100
const SCREEN_TOP = 50
const EDGE_GAP = 12
const DRAG_X = -200
const DRAG_Y = -120
const RESIZED_WIDTH = 400
const RESIZED_HEIGHT = 220
const tid = name => `[data-testid="${name}"]`
let parentWidth, parentHeight, observers

enableAutoUnmount(afterEach)
beforeEach(() => {
  parentWidth = PARENT_WIDTH
  parentHeight = PARENT_HEIGHT
  observers = []
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback) { this.callback = callback; observers.push(this) }
    observe() {}
    disconnect() {}
  })
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
    if (this.dataset.testid === 'panel-parent') return rect(SCREEN_LEFT, SCREEN_TOP, parentWidth, parentHeight)
    const panel = this.dataset.testid === 'progress-panel' ? this : this.closest(tid('progress-panel'))
    if (!panel) return rect(0, 0, 0, 0)
    const collapsed = panel.querySelector(tid('progress-panel-toggle'))?.getAttribute('aria-expanded') === 'false'
    const height = collapsed ? STRIP_HEIGHT : PANEL_HEIGHT
    const left = panel.style.left ? parseFloat(panel.style.left) : parentWidth - PANEL_WIDTH - EDGE_GAP
    const top = panel.style.top ? parseFloat(panel.style.top) : parentHeight - height - EDGE_GAP
    return rect(SCREEN_LEFT + left, SCREEN_TOP + top, PANEL_WIDTH, height)
  })
  for (const [key, dimension] of [['clientWidth', 'width'], ['offsetWidth', 'width'], ['clientHeight', 'height'], ['offsetHeight', 'height']]) {
    vi.spyOn(HTMLElement.prototype, key, 'get').mockImplementation(function () { return this.getBoundingClientRect()[dimension] })
  }
  vi.spyOn(HTMLElement.prototype, 'setPointerCapture').mockImplementation(() => {})
  vi.spyOn(HTMLElement.prototype, 'releasePointerCapture').mockImplementation(() => {})
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
function rect(x, y, width, height) { return { x, y, left: x, top: y, width, height, right: x + width, bottom: y + height, toJSON() {} } }
function fixture(props = {}) {
  const wrapper = mount(defineComponent({
    props: ['active', 'label', 'progress', 'steps', 'eta', 'previewAvailable'],
    setup(values) { return () => h('div', { 'data-testid': 'panel-parent' }, [h(FloatingGenerationPanel, { ...values })]) },
  }), { attachTo: document.body, props: { active: true, label: '2 / 87 — Генерация изображения', progress: 0.5, steps: 30, eta: 4, ...props } })
  return { wrapper, panel: wrapper.findComponent(FloatingGenerationPanel), root: () => wrapper.get(tid('progress-panel')), handle: () => wrapper.get(tid('progress-panel-handle')) }
}
function position(view) {
  const panel = view.root().element.getBoundingClientRect()
  return { left: panel.left - SCREEN_LEFT, top: panel.top - SCREEN_TOP }
}
async function drag(view, dx = DRAG_X, dy = DRAG_Y) {
  const handle = view.handle()
  const box = handle.element.getBoundingClientRect()
  await handle.trigger('pointerdown', { button: 0, pointerId: 1, clientX: box.left, clientY: box.top, isPrimary: true })
  await handle.trigger('pointermove', { pointerId: 1, clientX: box.left + dx, clientY: box.top + dy, buttons: 1 })
  await handle.trigger('pointerup', { pointerId: 1 })
}

describe('shared floating generation panel', () => {
  it('shows label, steps, percent and ETA and emits interrupt', async () => {
    const view = fixture()
    await nextTick()
    expect(view.root().text()).toContain('2 / 87')
    expect(view.root().text()).toContain('50%')
    expect(view.root().text()).toContain('15')
    expect(view.root().text()).toContain('30')
    expect(view.root().text()).toContain('~4s')
    await view.root().get('.gp-interrupt').trigger('click')
    expect(view.panel.emitted('interrupt')).toHaveLength(1)
  })
  it('collapses to a strip while retaining status, percent, progress and stop, then restores details', async () => {
    const view = fixture()
    const toggle = view.root().get(tid('progress-panel-toggle'))
    expect(toggle.attributes('aria-expanded')).toBe('true')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(view.root().text()).toContain('2 / 87')
    expect(view.root().text()).toContain('50%')
    expect(view.root().findComponent(GenProgress).exists() && view.root().findComponent(GenProgress).isVisible()).toBe(false)
    expect(view.root().findAll('[style]').some(el => el.element.style.width === '50%')).toBe(true)
    await view.root().get('.gp-interrupt').trigger('click')
    expect(view.panel.emitted('interrupt')).toHaveLength(1)
    await toggle.trigger('click')
    expect(view.root().findComponent(GenProgress).isVisible()).toBe(true)
    expect(view.root().text()).toContain('~4s')
  })
  it('drags by its header and clamps within the parent, including after resize', async () => {
    const view = fixture()
    await nextTick()
    const before = position(view)
    await drag(view)
    expect(position(view).left).toBeLessThan(before.left)
    expect(position(view).top).toBeLessThan(before.top)
    await drag(view, -PARENT_WIDTH, -PARENT_HEIGHT)
    expect(position(view).left).toBeGreaterThanOrEqual(0)
    expect(position(view).top).toBeGreaterThanOrEqual(0)
    await drag(view, PARENT_WIDTH, PARENT_HEIGHT)
    expect(position(view).left).toBeLessThanOrEqual(parentWidth - PANEL_WIDTH)
    expect(position(view).top).toBeLessThanOrEqual(parentHeight - PANEL_HEIGHT)
    parentWidth = RESIZED_WIDTH
    parentHeight = RESIZED_HEIGHT
    observers.forEach(observer => observer.callback([]))
    await nextTick()
    expect(position(view).left).toBeLessThanOrEqual(parentWidth - PANEL_WIDTH)
    expect(position(view).top).toBeLessThanOrEqual(parentHeight - PANEL_HEIGHT)
  })
  it('stops moving after pointer cancellation', async () => {
    const view = fixture()
    await nextTick()
    const handle = view.handle()
    const box = handle.element.getBoundingClientRect()
    await handle.trigger('pointerdown', { button: 0, pointerId: 1, clientX: box.left, clientY: box.top })
    await handle.trigger('pointermove', { pointerId: 1, clientX: box.left + DRAG_X, clientY: box.top + DRAG_Y, buttons: 1 })
    const stopped = position(view)
    await handle.trigger('pointercancel', { pointerId: 1 })
    await handle.trigger('pointermove', { pointerId: 1, clientX: 0, clientY: 0, buttons: 1 })
    expect(position(view)).toEqual(stopped)
  })

  it('does not start dragging from header buttons or a non-primary mouse button', async () => {
    const view = fixture()
    await nextTick()
    const before = position(view)
    const toggle = view.root().get(tid('progress-panel-toggle'))
    await toggle.trigger('pointerdown', { button: 0, pointerId: 1, clientX: 0, clientY: 0 })
    await view.handle().trigger('pointermove', { pointerId: 1, clientX: DRAG_X, clientY: DRAG_Y, buttons: 1 })
    expect(position(view)).toEqual(before)
    const box = view.handle().element.getBoundingClientRect()
    await view.handle().trigger('pointerdown', { button: 2, pointerId: 1, clientX: box.left, clientY: box.top })
    await view.handle().trigger('pointermove', { pointerId: 1, clientX: box.left + DRAG_X, clientY: box.top + DRAG_Y, buttons: 2 })
    expect(position(view)).toEqual(before)
  })
  it('retains collapse and position across stage updates and keeps an expanded panel in bounds', async () => {
    const view = fixture()
    await nextTick()
    await drag(view)
    await view.root().get(tid('progress-panel-toggle')).trigger('click')
    const before = position(view)
    await view.wrapper.setProps({ label: '3 / 87 — Смена одежды', progress: 0.1, steps: 20, eta: 10 })
    expect(view.root().get(tid('progress-panel-toggle')).attributes('aria-expanded')).toBe('false')
    expect(position(view)).toEqual(before)
    expect(view.root().text()).toContain('3 / 87')
    expect(view.root().text()).toContain('10%')
    await drag(view, PARENT_WIDTH, PARENT_HEIGHT)
    await view.root().get(tid('progress-panel-toggle')).trigger('click')
    await nextTick()
    expect(position(view).top).toBeLessThanOrEqual(parentHeight - PANEL_HEIGHT)
  })
  it('hides when inactive and becomes available again without losing collapse', async () => {
    const view = fixture()
    await view.root().get(tid('progress-panel-toggle')).trigger('click')
    await view.wrapper.setProps({ active: false })
    expect(view.root().isVisible()).toBe(false)
    await view.wrapper.setProps({ active: true })
    expect(view.root().isVisible()).toBe(true)
    expect(view.root().get(tid('progress-panel-toggle')).attributes('aria-expanded')).toBe('false')
  })
})
