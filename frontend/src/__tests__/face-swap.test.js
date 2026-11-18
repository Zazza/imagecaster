import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'

import GeneratePage from '../components/GeneratePage.vue'
import PeoplePage from '../components/PeoplePage.vue'
import CatalogPage from '../components/CatalogPage.vue'
import {
  GetSettings, UpdateSettings, ListPresets, EnqueueTxt2Img, GetModelCatalog, ListPeople, FaceSwapStatus,
  TrainingStatus, ListTrainingJobs, TrainingBases, PickPeopleFolder, AddPersonIdentity,
  GetServerStatus, GetServerModels, GetServerLLMModels,
} from '../wailsjs/go/main/App.js'
import { t, setLocale } from '../i18n/index.js'
import { settle } from './helpers/index.js'
import { loadPeople } from '../peopleStore.js'
import { personOverride, canFaceSwap } from '../people.js'
import { serverURL, serverModels, serverModelsKnown, modelActionLoading, resetActiveDownload } from '../serverModels.js'
import { api } from '../api.js'
import { ru } from '../i18n/ru.js'
import { en } from '../i18n/en.js'

enableAutoUnmount(afterEach)

const PRESETS = [{ id: 1, name: 'Photo', preset_type: 'general', loras: '', model_name: 'z-image-turbo-fp8-aio.gguf' }]
const CATALOG = {
  sd_models: [{ name: 'z-image-turbo-fp8-aio', base: 'Z-Image', filename: 'z-image-turbo-fp8-aio.gguf' }],
  llm_generate: [],
  llm_vision: [],
  lora: [],
  train: [],
  face_swap: {
    note: 'Non-commercial use only',
    note_ru: 'Только некоммерческое использование',
    files: [
      { type: 'face', filename: 'det_10g.onnx', url: 'https://h/det_10g.onnx', size_gb: 0.02, sha256: 'a'.repeat(64) },
      { type: 'face', filename: 'inswapper_128.onnx', url: 'https://h/inswapper_128.onnx', size_gb: 0.56, sha256: 'b'.repeat(64) },
    ],
  },
}
const WITH_FACE = { name: 'anna', base: 'Z-Image', lora: 'anna', identity: true }
const NO_FACE = { name: 'bob', base: 'Z-Image', lora: 'bob', identity: false }
const tid = (id) => `[data-testid="${id}"]`

beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  setLocale('ru')
  ListPresets.mockImplementation(() => Promise.resolve(PRESETS.map(p => ({ ...p }))))
  GetModelCatalog.mockImplementation(() => Promise.resolve(structuredClone(CATALOG)))
  ListPeople.mockImplementation(() => Promise.resolve([{ ...WITH_FACE }, { ...NO_FACE }]))
  EnqueueTxt2Img.mockImplementation(() => Promise.resolve(101))
  FaceSwapStatus.mockImplementation(() => Promise.resolve('ok'))
  TrainingStatus.mockImplementation(() => Promise.resolve('ok'))
  ListTrainingJobs.mockImplementation(() => Promise.resolve([]))
  TrainingBases.mockImplementation(() => Promise.resolve(['Z-Image']))
})

describe('people.js: face swap override', () => {
  const catalog = { sd_models: CATALOG.sd_models }
  const preset = PRESETS[0]
  const people = [WITH_FACE, NO_FACE]

  it.each([
    ['person with face, flag on', 'anna', true, { name: 'anna', face_swap: true }],
    ['person with face, flag off', 'anna', false, { name: 'anna' }],
    ['person without face, flag on', 'bob', true, { name: 'bob' }],
  ])('%s', (_n, name, flag, want) => {
    expect(personOverride(catalog, preset, {}, people, name, flag)).toEqual(want)
  })

  it.each([
    [WITH_FACE, true, true],
    [WITH_FACE, false, false],
    [NO_FACE, true, false],
    [null, true, false],
  ])('canFaceSwap(%o, %s) = %s', (p, feature, want) => {
    expect(canFaceSwap(p, feature)).toBe(want)
  })
})

async function mountGenerate() {
  await loadPeople()
  GetSettings.mockImplementation(() => Promise.resolve({ gen_mode: 'preset', gen_preset_id: '1' }))
  const w = mount(GeneratePage, { attachTo: document.body })
  await settle()
  return w
}

async function generate(w) {
  const ta = w.findAll('textarea').find(x => x.attributes('placeholder') === t('generate.placeholder_description'))
  await ta.setValue('a woman in a cafe')
  await w.find('.btn-generate').trigger('click')
  await settle()
}

describe('generate: face swap checkbox', () => {
  it('hidden without a picked person', async () => {
    const w = await mountGenerate()
    expect(w.find(tid('gen-face-swap')).exists()).toBe(false)
  })

  it('hidden for a person without a face', async () => {
    const w = await mountGenerate()
    await w.find(tid('gen-person-select')).setValue('bob')
    await settle()
    expect(w.find(tid('gen-face-swap')).exists()).toBe(false)
  })

  it('hidden when the server has no face_swap feature', async () => {
    FaceSwapStatus.mockImplementation(() => Promise.resolve('not_installed'))
    const w = await mountGenerate()
    await w.find(tid('gen-person-select')).setValue('anna')
    await settle()
    expect(w.find(tid('gen-face-swap')).exists()).toBe(false)
  })

  it('shown off by default; job has no face_swap', async () => {
    const w = await mountGenerate()
    await w.find(tid('gen-person-select')).setValue('anna')
    await settle()
    const box = w.find(tid('gen-face-swap-input'))
    expect(box.exists()).toBe(true)
    expect(box.element.checked).toBe(false)
    await generate(w)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides.person).toEqual({ name: 'anna' })
  })

  it('checked: job carries face_swap', async () => {
    const w = await mountGenerate()
    await w.find(tid('gen-person-select')).setValue('anna')
    await settle()
    await w.find(tid('gen-face-swap-input')).setValue(true)
    await generate(w)
    expect(EnqueueTxt2Img.mock.calls[0][0].overrides.person).toEqual({ name: 'anna', face_swap: true })
  })

  it('changing the person turns the flag off', async () => {
    const w = await mountGenerate()
    await w.find(tid('gen-person-select')).setValue('anna')
    await settle()
    await w.find(tid('gen-face-swap-input')).setValue(true)
    await w.find(tid('gen-person-select')).setValue('')
    await settle()
    await w.find(tid('gen-person-select')).setValue('anna')
    await settle()
    expect(w.find(tid('gen-face-swap-input')).element.checked).toBe(false)
  })
})

describe('people page: face for swap', () => {
  it('badge shows yes/no per person', async () => {
    const w = mount(PeoplePage)
    await settle()
    const badges = w.findAll(tid('people-identity')).map(b => b.text())
    expect(badges).toEqual([t('people.identity_yes'), t('people.identity_no')])
  })

  it('Add face sends the picked folder and person name', async () => {
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/anna-face'))
    AddPersonIdentity.mockImplementation(() => Promise.resolve({ ...WITH_FACE }))
    const w = mount(PeoplePage)
    await settle()
    await w.findAll(tid('people-identity-add'))[1].trigger('click')
    await settle()
    expect(AddPersonIdentity).toHaveBeenCalledWith('/photos/anna-face', 'bob')
  })

  it('badge turns to yes after Add face succeeds', async () => {
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/bob-face'))
    AddPersonIdentity.mockImplementation(() => {
      ListPeople.mockImplementation(() => Promise.resolve([{ ...WITH_FACE }, { ...NO_FACE, identity: true }]))
      return Promise.resolve({ ...NO_FACE, identity: true })
    })
    const w = mount(PeoplePage)
    await settle()
    await w.findAll(tid('people-identity-add'))[1].trigger('click')
    await settle()
    expect(w.findAll(tid('people-identity')).map(b => b.text())).toEqual([t('people.identity_yes'), t('people.identity_yes')])
  })

  it('Add face disabled without face swap on the server', async () => {
    FaceSwapStatus.mockImplementation(() => Promise.resolve('not_installed'))
    const w = mount(PeoplePage)
    await settle()
    expect(w.find(tid('people-identity-add')).attributes('disabled')).toBeDefined()
  })

  it('server error is shown', async () => {
    PickPeopleFolder.mockImplementation(() => Promise.resolve('/photos/x'))
    AddPersonIdentity.mockImplementation(() => Promise.reject(new Error('на фото мало чётко видных лиц')))
    const w = mount(PeoplePage)
    await settle()
    await w.find(tid('people-identity-add')).trigger('click')
    await settle()
    expect(w.find(tid('people-error')).text()).toContain('мало чётко видных лиц')
  })
})

describe('catalog: face swap section', () => {
  let downloads
  beforeEach(() => {
    serverURL.value = ''
    for (const k of Object.keys(serverModels)) serverModels[k] = []
    for (const k of Object.keys(serverModelsKnown)) delete serverModelsKnown[k]
    for (const k of Object.keys(modelActionLoading)) delete modelActionLoading[k]
    resetActiveDownload()
    downloads = []
    vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} })
    vi.stubGlobal('alert', () => {})
    GetSettings.mockImplementation(() => Promise.resolve({ connection_mode: 'server', server_url: 'http://srv:1' }))
    UpdateSettings.mockImplementation(() => Promise.resolve(undefined))
    GetServerStatus.mockImplementation(() => Promise.resolve({}))
    GetServerModels.mockImplementation((type) => Promise.resolve(type === 'face' ? [{ name: 'det_10g.onnx', size: 1 }] : []))
    GetServerLLMModels.mockImplementation(() => Promise.resolve([]))
    vi.spyOn(api, 'downloadServerModelStream').mockImplementation((_s, type, url, filename) => {
      downloads.push({ type, url, filename })
      return Promise.resolve()
    })
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('shows the licence note and downloads only missing face files', async () => {
    serverURL.value = 'http://srv:1'
    const w = mount(CatalogPage, { props: { active: false }, attachTo: document.body })
    await settle()
    await w.setProps({ active: true })
    await settle()
    await w.find(tid('catalog-tab-face')).trigger('click')
    await settle()
    expect(w.find(tid('catalog-face-licence')).text()).toBe('Только некоммерческое использование')
    expect(w.findAll(tid('catalog-face-file'))).toHaveLength(2)
    await w.find(tid('catalog-face-download')).trigger('click')
    await settle()
    expect(downloads).toEqual([{ type: 'face', url: 'https://h/inswapper_128.onnx', filename: 'inswapper_128.onnx' }])
  })
})

describe('i18n: face swap keys', () => {
  const keys = [
    'people.identity_yes', 'people.identity_no', 'people.identity_add', 'people.identity_hint', 'people.identity_unavailable',
    'people.face_swap', 'people.face_swap_hint', 'settings.catalog_tab_face', 'settings.catalog_face_hint', 'settings.catalog_face_name',
  ]
  it.each(keys)('%s exists in ru and en', (k) => {
    expect(ru[k]).toBeTruthy()
    expect(en[k]).toBeTruthy()
  })
})
