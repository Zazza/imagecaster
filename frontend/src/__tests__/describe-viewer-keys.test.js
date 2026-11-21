import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import ImageViewer from '../components/ImageViewer.vue'
import GeneratePage from '../components/GeneratePage.vue'
import {
  DescribePhoto, IsKidsModeActive, GetSettings, UpdateSettings, ListPresets, ListCompoundPresets,
  GetModelCatalog, GetSDModels, GetSDLoRAs, GetSDSamplers, GetSDSchedulers,
} from '../wailsjs/go/main/App.js'
import { describeToGenerate } from '../describeToGenerate.js'
import { setLocale, t } from '../i18n/index.js'
import { settle } from './helpers/index.js'

enableAutoUnmount(afterEach)

const IMG = 'QUFBQQ=='
let stored

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
  stored = {}
  IsKidsModeActive.mockImplementation(() => Promise.resolve(false))
  DescribePhoto.mockImplementation(() => Promise.resolve('описание'))
  GetSettings.mockImplementation(() => Promise.resolve({ ...stored }))
  UpdateSettings.mockImplementation(d => { Object.assign(stored, d); return Promise.resolve(undefined) })
  ListPresets.mockImplementation(() => Promise.resolve([]))
  ListCompoundPresets.mockImplementation(() => Promise.resolve([]))
  GetModelCatalog.mockImplementation(() => Promise.resolve({ llm_generate: [], llm_vision: [], lora: [], sd_models: [] }))
  GetSDModels.mockImplementation(() => Promise.resolve([]))
  GetSDLoRAs.mockImplementation(() => Promise.resolve([]))
  GetSDSamplers.mockImplementation(() => Promise.resolve([]))
  GetSDSchedulers.mockImplementation(() => Promise.resolve([]))
})

afterEach(() => vi.unstubAllGlobals())

function key(target, k) {
  target.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true }))
}

describe('клавиши просмотра при наборе описания', () => {
  it('Delete and arrows typed in the description field do not delete or switch the picture', async () => {
    const w = mount(ImageViewer, {
      props: { imageBase64: IMG, showActions: true, canDelete: true, hasPrev: true, hasNext: true },
      attachTo: document.body,
    })
    await settle()
    document.querySelector('[data-testid="viewer-describe"]').click()
    await settle()
    const field = document.querySelector('[data-testid="describe-text"]')
    for (const k of ['Delete', 'Delete', 'ArrowLeft', 'ArrowRight']) key(field, k)
    await settle()
    expect(w.emitted('delete')).toBeFalsy()
    expect(w.emitted('prev')).toBeFalsy()
    expect(w.emitted('next')).toBeFalsy()
    key(document.body, 'Delete')
    key(document.body, 'ArrowRight')
    expect(w.emitted('delete')).toBeTruthy()
    expect(w.emitted('next')).toBeTruthy()
  })
})

describe('«В Генерацию» при открытой «Генерации»', () => {
  it('text lands in the Generate description field without writing gen_description', async () => {
    const w = mount(GeneratePage, { attachTo: document.body })
    await settle()
    UpdateSettings.mockClear()
    await describeToGenerate('рыжий кот', vi.fn())
    await settle()
    const field = w.findAll('textarea').find(a => a.attributes('placeholder') === t('generate.placeholder_description'))
    expect(field.element.value).toBe('рыжий кот')
    expect(UpdateSettings.mock.calls.some(c => 'gen_description' in c[0] && c[0].gen_description !== 'рыжий кот')).toBe(false)
  })
})
