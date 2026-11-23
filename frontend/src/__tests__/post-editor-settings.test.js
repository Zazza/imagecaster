import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount, enableAutoUnmount, flushPromises } from '@vue/test-utils'
import { api } from '../api.js'
import { providePostEditor } from '../composables/usePostEditor.js'
import SettingsPanel from '../components/post/SettingsPanel.vue'
import EditSection from '../components/EditSection.vue'
import PostProcessPage from '../components/PostProcessPage.vue'
import { EventsEmit, clearEventMocks } from './mocks/runtime'
import { useRemixQueue } from '../remixQueue.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { settle } from './helpers/async.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)
useFakeImage()
useFakeCanvas()
const SOURCE = 'c291cmNl'
const RESULT = 'cmVzdWx0'
const KLEIN = 'flux-2-klein-4b-Q8_0.gguf'
const FLUX3 = 'flux-3-klein-Q8_0.gguf'
const QWEN = 'qwen_image_2.1-Q8_0.gguf'
const WEIGHT = 0.75
const ROW = { name: 'Portrait', weight: WEIGHT }
const CATALOG = [
  { name: 'flux-2-klein-4b', filename: KLEIN, modules: [{ type: 'text_encoder', filename: 'klein-te' }, { type: 'vae', filename: 'klein-vae' }] },
  { name: 'flux-3-klein', filename: FLUX3, modules: [{ type: 'text_encoder', filename: 'flux3-te' }, { type: 'vae', filename: 'flux3-vae' }] },
  { name: 'qwen_image_2.1', filename: QWEN, modules: [{ type: 'text_encoder', filename: 'qwen-te' }, { type: 'text_encoder', filename: 'qwen-mmproj' }, { type: 'vae', filename: 'qwen-vae' }] },
]
const EFFECT = { id: 10, name: 'Soft Focus', preset_type: 'post', type_id: 1, model_name: KLEIN, loras: '[{"name":"HiddenStyle","weight":1}]' }
let files, settings, jobs
beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  localStorage.clear()
  useRemixQueue().clearPostQueue()
  settings = {}
  jobs = []
  files = { sd: [KLEIN, FLUX3, QWEN], text_encoder: ['klein-te', 'flux3-te', 'qwen-te', 'qwen-mmproj'], vae: ['klein-vae', 'flux3-vae', 'qwen-vae'] }
  vi.spyOn(api, 'getSettings').mockImplementation(async () => settings)
  vi.spyOn(api, 'updateSettings').mockImplementation(async values => { Object.assign(settings, values) })
  vi.spyOn(api, 'getModelCatalog').mockImplementation(async () => ({ sd_models: CATALOG }))
  vi.spyOn(api, 'getServerModels').mockImplementation(async type => (files[type] || []).map(name => ({ name })))
  vi.spyOn(api, 'getActiveSessionItem').mockResolvedValue({ id: 3 })
  vi.spyOn(api, 'getSessionImage').mockImplementation(async id => id === 3 ? SOURCE : RESULT)
  vi.spyOn(api, 'listPresets').mockResolvedValue([EFFECT])
  vi.spyOn(api, 'listPresetTypes').mockResolvedValue([{ id: 1, name: 'post' }])
  vi.spyOn(api, 'listHiresProfiles').mockResolvedValue([])
  vi.spyOn(api, 'editImage').mockResolvedValue({ image: RESULT })
  vi.spyOn(api, 'getQueue').mockImplementation(async () => jobs)
  vi.spyOn(api, 'enqueueFromImage').mockImplementation(async () => {
    const id = jobs.length + 1
    jobs.push({ id, status: 'pending', type: 'from_image' })
    return id
  })
  vi.spyOn(api, 'listPostRecipes').mockResolvedValue([{ id: 7, name: 'Two effects', steps: JSON.stringify([{ preset_id: EFFECT.id }, { preset_id: EFFECT.id }]), adjust: '{}' }])
})
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })
async function mountEditor(extra = null) {
  let editor
  const Host = defineComponent({ setup() { editor = providePostEditor(); return () => h('div', [h(SettingsPanel), extra && h(extra, { source: SOURCE })]) } })
  const wrapper = mount(Host)
  await settle()
  return { wrapper, editor }
}
const names = w => w.findAll('[data-testid="post-editor-lora-name"]').map(n => n.element.value)
const select = async (w, model) => { await w.find('[data-testid="post-editor-model"]').setValue(model); await settle() }
const button = (w, label) => w.findAll('button').find(b => b.text().includes(label))
async function mountPage(section) {
  const w = mount(PostProcessPage, { props: { initialSection: section }, attachTo: document.body })
  await settle()
  return w
}
describe('shared editor settings migration and isolation', () => {
  it.each([
    ['new explicit empty wins', { post_editor_loras: '{"klein":[]}', edit_loras_klein: JSON.stringify([ROW]) }, []],
    ['edit empty wins over effect', { edit_loras_klein: '[]', effect_loras_klein: JSON.stringify([ROW]) }, []],
    ['edit wins over effect', { edit_loras_klein: JSON.stringify([ROW]), effect_loras_klein: '[{"name":"Effect","weight":1}]' }, [ROW]],
    ['effect legacy fallback', { effect_loras_klein: JSON.stringify([ROW]) }, [ROW]],
    ['obsolete Moss ignored', { edit_loras: JSON.stringify([ROW]), effect_loras_moss: JSON.stringify([ROW]) }, []],
  ])('%s', async (_, saved, expected) => {
    settings = saved
    const { editor, wrapper } = await mountEditor()
    expect(editor.snapshot()).toEqual({ model: KLEIN, loras: expected })
    wrapper.unmount()
    await settle()
    expect(JSON.parse(settings.post_editor_loras).klein).toEqual(expected)
  })
  it('each family recalls its own ordered rows and persists after remount', async () => {
    settings = { edit_loras_klein: JSON.stringify([ROW]), edit_loras_qwen: '[{"name":"Qwen","weight":0.5}]' }
    const { wrapper, editor } = await mountEditor()
    await select(wrapper, QWEN)
    expect(names(wrapper)).toEqual(['Qwen'])
    await select(wrapper, FLUX3)
    expect(names(wrapper)).toEqual([])
    await wrapper.find('[data-testid="post-editor-lora-add"]').trigger('click')
    await wrapper.find('[data-testid="post-editor-lora-name"]').setValue('Flux3')
    await wrapper.find('[data-testid="post-editor-lora-weight"]').setValue(String(WEIGHT))
    const captured = editor.snapshot()
    await select(wrapper, KLEIN)
    expect(names(wrapper)).toEqual(['Portrait'])
    await wrapper.find('[data-testid="post-editor-lora-weight"]').setValue('1')
    expect(captured).toEqual({ model: FLUX3, loras: [{ name: 'Flux3', weight: WEIGHT }] })
    wrapper.unmount()
    await settle()
    const next = await mountEditor()
    await select(next.wrapper, FLUX3)
    expect(next.editor.snapshot()).toEqual(captured)
    await select(next.wrapper, QWEN)
    expect(names(next.wrapper)).toEqual(['Qwen'])
  })
  it('limits shared LoRA rows and normalizes saved weights before requesting', async () => {
    const MAX_ROWS = 8
    const MAX_WEIGHT = 2
    settings = { post_editor_loras: JSON.stringify({ klein: Array.from({ length: MAX_ROWS + 1 }, (_, index) => ({ name: `Lora${index}`, weight: index === 0 ? 0 : MAX_WEIGHT + 1 })) }) }
    const { wrapper, editor } = await mountEditor()
    expect(names(wrapper)).toHaveLength(MAX_ROWS)
    expect(wrapper.find('[data-testid="post-editor-lora-add"]').element.disabled).toBe(true)
    const rows = editor.snapshot().loras
    expect(rows[0].weight).toBe(0)
    expect(rows.slice(1).every(row => row.weight === MAX_WEIGHT)).toBe(true)
  })
  it('malformed saved rows show a load error and do not leak obsolete LoRAs', async () => {
    settings = { post_editor_loras: '{bad json', edit_loras: JSON.stringify([ROW]) }
    const { wrapper, editor } = await mountEditor()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(editor.snapshot().loras).toEqual([])
  })
  it('disposed page cannot persist a stale load after a newer page saved its choice', async () => {
    let resolveSettings, resolveCatalog
    const oldSettings = { post_editor_model: KLEIN, post_editor_loras: JSON.stringify({ klein: [ROW] }) }
    api.getSettings.mockImplementationOnce(() => new Promise(resolve => { resolveSettings = resolve }))
    api.getModelCatalog.mockImplementationOnce(() => new Promise(resolve => { resolveCatalog = resolve }))
    const oldPage = await mountEditor()
    oldPage.wrapper.unmount()
    settings = { post_editor_model: QWEN, post_editor_loras: JSON.stringify({ qwen: [{ name: 'NewPage', weight: WEIGHT }] }) }
    const newPage = await mountEditor()
    newPage.editor.dispose()
    await settle()
    expect(settings.post_editor_model).toBe(QWEN)
    const writesAfterNewPage = api.updateSettings.mock.calls.length
    vi.useFakeTimers()
    resolveSettings(oldSettings)
    resolveCatalog({ sd_models: CATALOG })
    await flushPromises()
    await vi.runAllTimersAsync()
    expect(api.updateSettings).toHaveBeenCalledTimes(writesAfterNewPage)
    expect(settings.post_editor_model).toBe(QWEN)
    expect(JSON.parse(settings.post_editor_loras).qwen).toEqual([{ name: 'NewPage', weight: WEIGHT }])
  })
  it('settings load and save errors are visible', async () => {
    api.getSettings.mockRejectedValueOnce(new Error('settings offline'))
    const first = await mountEditor()
    expect(first.wrapper.text()).toContain('settings offline')
    first.wrapper.unmount()
    api.updateSettings.mockRejectedValueOnce(new Error('save denied'))
    const second = await mountEditor()
    second.editor.dispose()
    await settle()
    expect(second.wrapper.text()).toContain('save denied')
  })
})
describe('downloaded editor availability', () => {
  it.each(CATALOG.map(c => [c.filename, c.modules]))('%s requires all catalog modules', async (model, modules) => {
    settings = { post_editor_model: model }
    const full = await mountEditor()
    expect(full.editor.ready.value).toBe(true)
    expect(full.editor.snapshot().model).toBe(model)
    full.wrapper.unmount()
    files[modules[0].type] = files[modules[0].type].filter(name => name !== modules[0].filename)
    const missing = await mountEditor()
    expect(missing.editor.ready.value).toBe(false)
    expect(missing.wrapper.text()).toContain(modules[0].filename)
    expect(() => missing.editor.snapshot()).toThrow()
  })
  it('missing Qwen vision module blocks execution', async () => {
    settings = { post_editor_model: QWEN }
    files.text_encoder = files.text_encoder.filter(name => name !== 'qwen-mmproj')
    const { wrapper, editor } = await mountEditor(EditSection)
    await wrapper.find('[data-testid="edit-instruction"]').setValue('change hair')
    expect(editor.ready.value).toBe(false)
    expect(wrapper.text()).toContain('qwen-mmproj')
    expect(button(wrapper, t('edit.btn_generate')).element.disabled).toBe(true)
    expect(api.editImage).not.toHaveBeenCalled()
  })
  it('retains a missing saved checkpoint without choosing another installed editor', async () => {
    const missing = 'flux-3-klein-removed.gguf'
    settings = { post_editor_model: missing }
    const { wrapper, editor } = await mountEditor()
    expect(wrapper.find('[data-testid="post-editor-model"]').element.value).toBe(missing)
    expect(editor.ready.value).toBe(false)
    expect(wrapper.find('[data-testid="post-editor-unavailable"]').exists()).toBe(true)
  })
  it('failed SD inventory finishes loading with a visible failure and never saves an empty default', async () => {
    api.getServerModels.mockImplementation(async type => {
      if (type === 'sd') throw new Error('inventory offline')
      return (files[type] || []).map(name => ({ name }))
    })
    const { wrapper, editor } = await mountEditor()
    expect(editor.loaded.value).toBe(true)
    expect(editor.ready.value).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain(t('post_editor.loading'))
    vi.useFakeTimers()
    await vi.runAllTimersAsync()
    editor.dispose()
    await flushPromises()
    expect(api.updateSettings).not.toHaveBeenCalled()
  })
  it('unknown server inventory blocks dependent tools', async () => {
    api.getServerModels.mockRejectedValue(new Error('inventory offline'))
    const { editor } = await mountEditor()
    expect(editor.ready.value).toBe(false)
    expect(() => editor.snapshot()).toThrow()
  })
})
describe('actual tool requests use the page selection', () => {
  beforeEach(() => { settings = { post_editor_model: FLUX3, post_editor_loras: JSON.stringify({ flux3: [ROW] }) } })
  it('settings is the twelfth accordion card and its header names the active editor and LoRA count', async () => {
    const w = await mountPage('settings')
    const headers = w.findAll('.card-header.card-toggle').filter(header => header.find('.card-num').exists())
    expect(headers.map(header => header.find('.card-num').text())).toEqual(['01', '02', '03', '04', '05', '06', '07', '08', '09', '10', '11', '12'])
    expect(headers.at(-1).text()).toContain(t('post_editor.title'))
    expect(headers.at(-1).text()).toContain(FLUX3)
    expect(headers.at(-1).text()).toContain(t('model.lora_count', { n: 1 }))
  })
  it('Edit sends selected checkpoint and LoRA while preserving quality settings', async () => {
    settings.edit_quality_mode = 'true'
    settings.edit_resolution_id = '5'
    const { wrapper } = await mountEditor(EditSection)
    await wrapper.find('[data-testid="edit-instruction"]').setValue('change hair')
    await button(wrapper, t('edit.btn_generate')).trigger('click')
    await settle()
    expect(api.editImage).toHaveBeenCalledWith(expect.objectContaining({ editor: { model: FLUX3, loras: [ROW] }, output_mode: 'quality', resolution_id: 5 }))
    expect(wrapper.find('[data-testid="edit-model-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="edit-lora-add"]').exists()).toBe(false)
  })
  it('Effect uses shared selection instead of its preset model and hidden LoRA', async () => {
    const w = await mountPage('effect')
    expect(w.findAll('[data-testid="post-editor-model"]')).toHaveLength(1)
    expect(w.find('.fx-model').exists()).toBe(false)
    await w.findAll('.fx-card').find(c => c.attributes('title') === EFFECT.name).trigger('click')
    await button(w, t('post.btn_apply_effect')).trigger('click')
    await settle()
    expect(api.enqueueFromImage).toHaveBeenCalledWith(expect.objectContaining({ preset_id: EFFECT.id, denoising_strength: 1, editor: { model: FLUX3, loras: [ROW] } }))
  })
  it('Effect strength slider supports the full editor strength', async () => {
    const w = await mountPage('effect')
    await w.findAll('.fx-card').find(c => c.attributes('title') === EFFECT.name).trigger('click')
    const slider = w.find('input[type="range"][step="0.01"]')
    expect(Number(slider.attributes('max'))).toBe(1)
    await slider.setValue('1')
    await button(w, t('post.btn_apply_effect')).trigger('click')
    await settle()
    expect(api.enqueueFromImage).toHaveBeenCalledWith(expect.objectContaining({ denoising_strength: 1 }))
  })
  it.each([[0.65, 0.65], [1.2, 1]])('Effect honors preset strength %s within the editor range', async (saved, expected) => {
    api.listPresets.mockResolvedValue([{ ...EFFECT, denoising_strength: saved }])
    const w = await mountPage('effect')
    await w.findAll('.fx-card').find(c => c.attributes('title') === EFFECT.name).trigger('click')
    await button(w, t('post.btn_apply_effect')).trigger('click')
    await settle()
    expect(api.enqueueFromImage).toHaveBeenCalledWith(expect.objectContaining({ denoising_strength: expected }))
  })
  it('a batch captures once before awaiting its first enqueue', async () => {
    useRemixQueue().setPostQueue([{ path: '/a.png', name: 'a.png' }, { path: '/b.png', name: 'b.png' }])
    vi.spyOn(api, 'readFileAsBase64').mockResolvedValue(SOURCE)
    let release
    const barrier = new Promise(resolve => { release = resolve })
    api.enqueueFromImage.mockImplementationOnce(async () => { await barrier; return 1 })
    const w = await mountPage('effect')
    await w.findAll('.fx-card').find(c => c.attributes('title') === EFFECT.name).trigger('click')
    await button(w, t('post.btn_apply_effect_all', { n: 2 })).trigger('click')
    await settle()
    expect(api.enqueueFromImage).toHaveBeenCalledTimes(1)
    await w.find('[data-testid="post-editor-lora-weight"]').setValue('1')
    await select(w, QWEN)
    release()
    await settle()
    expect(api.enqueueFromImage).toHaveBeenCalledTimes(2)
    for (const [params] of api.enqueueFromImage.mock.calls) expect(params.editor).toEqual({ model: FLUX3, loras: [ROW] })
    EventsEmit('queue:completed', { job_id: 1, result: { image_base64: RESULT } })
    EventsEmit('queue:completed', { job_id: 2, result: { image_base64: RESULT } })
    await settle()
  })
  it('all neural recipe steps retain the initial deep snapshot after settings change', async () => {
    const w = await mountPage('recipes')
    await w.findAll('.preset-item').find(i => i.text().includes('Two effects')).trigger('click')
    await settle()
    expect(api.enqueueFromImage).toHaveBeenCalledTimes(1)
    await select(w, QWEN)
    EventsEmit('queue:completed', { job_id: 1, result: { session_item_id: 99 } })
    await settle()
    expect(api.enqueueFromImage).toHaveBeenCalledTimes(2)
    for (const [params] of api.enqueueFromImage.mock.calls) expect(params.editor).toEqual({ model: FLUX3, loras: [ROW] })
    EventsEmit('queue:completed', { job_id: 2, result: { session_item_id: 99 } })
    await settle()
  })
})
