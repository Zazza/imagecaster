import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'

vi.mock('../wailsjs/runtime/runtime', () => import('./mocks/runtime'))
vi.mock('../wailsjs/go/main/App.js', async () => {
  const base = (await import('./mocks/wails-app')).default
  return { ...base }
})

import ImageViewer from '../components/ImageViewer.vue'
import { t, setLocale } from '../i18n/index.js'

enableAutoUnmount(afterEach)

const ITEM = { prompt: 'a cat', sampler: 'Euler', steps: 20, seed: 1 }

function make(props = {}) {
  return mount(ImageViewer, { props: { imageBase64: 'AAAA', ...props }, attachTo: document.body })
}

function buttons(w, label) {
  return w.findAll('button').filter(b => b.text().trim().includes(label))
}

function pressKey(key) {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

beforeEach(() => setLocale('ru'))

describe('ImageViewer layout', () => {
  it('клик по фону и по картинке не закрывает', async () => {
    const w = make({ item: ITEM })
    await w.find('.image-viewer-overlay').trigger('click')
    await w.find('img').trigger('click')
    expect(w.emitted('close')).toBeUndefined()
  })

  it('кнопка «Закрыть» шлёт close', async () => {
    const w = make()
    await w.find('.image-viewer-close').trigger('click')
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('Esc шлёт close', async () => {
    const w = make()
    pressKey('Escape')
    await nextTick()
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('стрелки ←/→ шлют prev/next', async () => {
    const w = make({ hasPrev: true, hasNext: true })
    pressKey('ArrowLeft')
    pressKey('ArrowRight')
    await nextTick()
    expect(w.emitted('prev')).toHaveLength(1)
    expect(w.emitted('next')).toHaveLength(1)
  })

  it('счётчик «5 из 12» в полосе', () => {
    const w = make({ position: 5, total: 12 })
    expect(t('viewer.counter', { n: 5, total: 12 })).toBe('5 из 12')
    const c = w.find('.iv-toolbar .iv-counter')
    expect(c.exists()).toBe(true)
    expect(c.text().trim()).toBe('5 из 12')
  })

  it('total=1 — счётчика нет', () => {
    expect(make({ position: 1, total: 1 }).find('.iv-counter').exists()).toBe(false)
  })

  it('без position/total — счётчика нет', () => {
    expect(make().find('.iv-counter').exists()).toBe(false)
  })

  it('title виден', () => {
    const w = make({ title: 'photo_01.png' })
    expect(w.find('.iv-title').text()).toContain('photo_01.png')
  })

  it('«Постобработка» и «Удалить» по одной, в полосе, при закрытых и открытых сведениях', async () => {
    const w = make({ item: ITEM, showActions: true })
    const check = () => {
      const post = buttons(w, t('viewer.postprocess'))
      const del = buttons(w, t('viewer.delete'))
      expect(post).toHaveLength(1)
      expect(del).toHaveLength(1)
      expect(w.find('.iv-toolbar').element.contains(post[0].element)).toBe(true)
      expect(w.find('.iv-toolbar').element.contains(del[0].element)).toBe(true)
    }
    check()
    await w.find('.iv-info-toggle').trigger('click')
    expect(w.find('.iv-info').exists()).toBe(true)
    check()
  })

  it('canDelete=false — «Удалить» нет', () => {
    const w = make({ item: ITEM, showActions: true, canDelete: false })
    expect(buttons(w, t('viewer.delete'))).toHaveLength(0)
    expect(buttons(w, t('viewer.confirm_delete'))).toHaveLength(0)
  })

  it('item=null — кнопки «Информация» нет', () => {
    expect(make({ item: null }).find('.iv-info-toggle').exists()).toBe(false)
  })

  it('кнопка «Информация» открывает и прячет панель с промптом', async () => {
    const w = make({ item: ITEM })
    expect(w.find('.iv-info').exists() && w.find('.iv-info').isVisible()).toBe(false)
    await w.find('.iv-info-toggle').trigger('click')
    expect(w.find('.iv-info').isVisible()).toBe(true)
    expect(w.find('.iv-info').text()).toContain('a cat')
    await w.find('.iv-info-toggle').trigger('click')
    expect(w.find('.iv-info').exists() && w.find('.iv-info').isVisible()).toBe(false)
  })

  it('«Постобработка» шлёт export', async () => {
    const w = make({ item: ITEM, showActions: true })
    await buttons(w, t('viewer.postprocess'))[0].trigger('click')
    expect(w.emitted('export')).toHaveLength(1)
  })

  it('«Удалить» шлёт delete', async () => {
    const w = make({ item: ITEM, showActions: true })
    await buttons(w, t('viewer.delete'))[0].trigger('click')
    expect(w.emitted('delete')).toHaveLength(1)
  })

  it('prev/next кнопки по hasPrev/hasNext и шлют события', async () => {
    const none = make({ hasPrev: false, hasNext: false })
    expect(none.find('.image-viewer-prev').exists()).toBe(false)
    expect(none.find('.image-viewer-next').exists()).toBe(false)
    none.unmount()
    const w = make({ hasPrev: true, hasNext: true })
    await w.find('.image-viewer-prev').trigger('click')
    await w.find('.image-viewer-next').trigger('click')
    expect(w.emitted('prev')).toHaveLength(1)
    expect(w.emitted('next')).toHaveLength(1)
  })
})

describe('zoom controls', () => {
  const label = w => w.find('.image-viewer-zoom-label').text().trim()

  it('plus zooms in, label click returns to fit', async () => {
    const w = make()
    expect(label(w)).toBe(t('viewer.zoom_fit'))
    await w.find(`[aria-label="${t('viewer.zoom_in')}"]`).trigger('click')
    expect(label(w)).toBe('110%')
    await w.find('.image-viewer-zoom-label').trigger('click')
    expect(label(w)).toBe(t('viewer.zoom_fit'))
  })

  it('preset menu sets zoom and double click toggles fit / 100%', async () => {
    const w = make()
    await w.find('.image-viewer-zoom-label').trigger('click')
    const p50 = w.find('.image-viewer-zoom-menu').findAll('button').find(b => b.text() === '50%')
    await p50.trigger('click')
    expect(label(w)).toBe('50%')
    expect(w.find('.image-viewer-zoom-menu').exists()).toBe(false)
    await w.find('.image-viewer-zoom-label').trigger('click')
    await w.find('img').trigger('dblclick')
    expect(label(w)).toBe('100%')
    await w.find('img').trigger('dblclick')
    expect(label(w)).toBe(t('viewer.zoom_fit'))
  })

  it('Escape closes the zoom menu first, then the viewer', async () => {
    const w = make()
    await w.find('.image-viewer-zoom-label').trigger('click')
    expect(w.find('.image-viewer-zoom-menu').exists()).toBe(true)
    pressKey('Escape')
    await nextTick()
    expect(w.find('.image-viewer-zoom-menu').exists()).toBe(false)
    expect(w.emitted('close')).toBeFalsy()
    pressKey('Escape')
    expect(w.emitted('close')).toHaveLength(1)
  })
})
