import { describe, it, expect, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'

import SearchableSelect from '../components/SearchableSelect.vue'

enableAutoUnmount(afterEach)

const ITEMS = [
  { value: 1, label: 'Anime Classic' },
  { value: 2, label: 'Photo Real', hint: 'flux' },
  { value: 3, label: 'Anime Neon' },
  { value: 4, label: 'Watercolor' },
]

function mountSelect(props = {}) {
  const w = mount(SearchableSelect, {
    attachTo: document.body,
    props: {
      items: ITEMS,
      modelValue: null,
      emptyText: 'nothing',
      'onUpdate:modelValue': v => w.setProps({ modelValue: v }),
      ...props,
    },
  })
  return w
}

const input = w => w.find('.ss-input')
const options = w => w.findAll('.ss-option')
const labels = w => options(w).map(o => o.find('.ss-option-label').text())
const activeLabel = w => w.find('.ss-option-active .ss-option-label').text()
const emitted = w => (w.emitted('update:modelValue') || []).map(e => e[0])
const key = (w, k) => input(w).trigger('keydown', { key: k })

describe('searchable select', () => {
  it('opens with all items on focus', async () => {
    const w = mountSelect()
    expect(w.find('.ss-list').exists()).toBe(false)
    await input(w).trigger('focus')
    expect(labels(w)).toEqual(ITEMS.map(i => i.label))
  })

  it.each([
    ['anime', ['Anime Classic', 'Anime Neon']],
    ['  NEON ', ['Anime Neon']],
    ['flux', ['Photo Real']],
    ['', ITEMS.map(i => i.label)],
  ])('typing %j filters by label or hint', async (q, want) => {
    const w = mountSelect()
    await input(w).trigger('focus')
    await input(w).setValue(q)
    expect(labels(w)).toEqual(want)
  })

  it('shows the empty text when nothing matches', async () => {
    const w = mountSelect()
    await input(w).trigger('focus')
    await input(w).setValue('zzz')
    expect(options(w)).toHaveLength(0)
    expect(w.find('.ss-empty').text()).toBe('nothing')
  })

  it('clicking an option emits its value and closes the list showing the label', async () => {
    const w = mountSelect()
    await input(w).trigger('focus')
    await options(w)[3].trigger('click')
    expect(emitted(w)).toEqual([4])
    expect(w.find('.ss-list').exists()).toBe(false)
    expect(input(w).element.value).toBe('Watercolor')
  })

  it('arrow keys move the highlight with wrap-around, enter picks it', async () => {
    const w = mountSelect()
    await input(w).trigger('focus')
    expect(activeLabel(w)).toBe('Anime Classic')
    await key(w, 'ArrowDown')
    await key(w, 'ArrowDown')
    expect(activeLabel(w)).toBe('Anime Neon')
    await key(w, 'ArrowUp')
    expect(activeLabel(w)).toBe('Photo Real')
    await key(w, 'ArrowUp')
    await key(w, 'ArrowUp')
    expect(activeLabel(w)).toBe('Watercolor')
    await key(w, 'ArrowDown')
    expect(activeLabel(w)).toBe('Anime Classic')
    await key(w, 'Enter')
    expect(emitted(w)).toEqual([1])
    expect(w.find('.ss-list').exists()).toBe(false)
  })

  it('enter picks among filtered items only', async () => {
    const w = mountSelect()
    await input(w).trigger('focus')
    await input(w).setValue('anime')
    await key(w, 'ArrowDown')
    await key(w, 'Enter')
    expect(emitted(w)).toEqual([3])
  })

  it('arrow down on a closed list opens it without moving', async () => {
    const w = mountSelect({ modelValue: 2 })
    await key(w, 'ArrowDown')
    expect(w.find('.ss-list').exists()).toBe(true)
    expect(activeLabel(w)).toBe('Photo Real')
  })

  it('escape closes without emitting and restores the selected label', async () => {
    const w = mountSelect({ modelValue: 2 })
    await input(w).trigger('focus')
    await input(w).setValue('wat')
    await key(w, 'Escape')
    expect(w.find('.ss-list').exists()).toBe(false)
    expect(emitted(w)).toEqual([])
    expect(input(w).element.value).toBe('Photo Real')
  })

  it('enter on a closed list emits nothing', async () => {
    const w = mountSelect()
    await key(w, 'Enter')
    expect(emitted(w)).toEqual([])
  })

  it('clear button emits null and appears only with a value', async () => {
    const w = mountSelect({ modelValue: 3 })
    expect(input(w).element.value).toBe('Anime Neon')
    await w.find('.ss-clear').trigger('click')
    expect(emitted(w)).toEqual([null])
    expect(w.find('.ss-clear').exists()).toBe(false)
    expect(input(w).element.value).toBe('')
  })
})
