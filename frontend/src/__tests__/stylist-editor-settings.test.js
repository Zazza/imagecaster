import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { api } from '../api.js'
import { providePostEditor } from '../composables/usePostEditor.js'
import SettingsPanel from '../components/post/SettingsPanel.vue'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
const KLEIN = 'flux-2-klein-4b-Q8_0.gguf'
const FLUX3 = 'flux-3-klein-Q8_0.gguf'
const QWEN = 'qwen_image_2.1-Q8_0.gguf'
const WEIGHT = 0.75
const ROW = { name: 'StylistPortrait', weight: WEIGHT }
const MAX_ROWS = 8
const MAX_WEIGHT = 2
const CATALOG = [
  { name: 'flux-2-klein-4b', filename: KLEIN, modules: [{ type: 'text_encoder', filename: 'klein-te' }, { type: 'vae', filename: 'klein-vae' }] },
  { name: 'flux-3-klein', filename: FLUX3, modules: [{ type: 'text_encoder', filename: 'flux3-te' }, { type: 'vae', filename: 'flux3-vae' }] },
  { name: 'qwen_image_2.1', filename: QWEN, modules: [{ type: 'text_encoder', filename: 'qwen-te' }, { type: 'text_encoder', filename: 'qwen-mmproj' }, { type: 'vae', filename: 'qwen-vae' }] },
]
let settings, files
beforeEach(() => {
  vi.clearAllMocks()
  settings = {}
  files = { sd: [KLEIN, FLUX3, QWEN], text_encoder: ['klein-te', 'flux3-te', 'qwen-te', 'qwen-mmproj'], vae: ['klein-vae', 'flux3-vae', 'qwen-vae'] }
  vi.spyOn(api, 'getSettings').mockImplementation(async () => settings)
  vi.spyOn(api, 'updateSettings').mockImplementation(async values => { Object.assign(settings, values) })
  vi.spyOn(api, 'getModelCatalog').mockResolvedValue({ sd_models: CATALOG })
  vi.spyOn(api, 'getServerModels').mockImplementation(async type => (files[type] || []).map(name => ({ name })))
})
afterEach(() => vi.restoreAllMocks())
async function mountEditor(stylist = true) {
  let editor
  const Host = defineComponent({ setup() {
    editor = providePostEditor(stylist ? { settingsPrefix: 'stylist_edit_editor', restoreLegacy: false } : undefined)
    return () => h(SettingsPanel, stylist ? { scopeKey: 'stylist-editor', idPrefix: 'stylist-editor' } : {})
  } })
  const wrapper = mount(Host)
  await settle()
  return { wrapper, editor }
}
const select = async (w, model) => { await w.find('[data-testid="stylist-editor-model"]').setValue(model); await settle() }
const names = w => w.findAll('[data-testid="stylist-editor-lora-name"]').map(n => n.element.value)

describe('Stylist editor has independent persisted choices', () => {
  it('does not inherit Postprocess, legacy edit or person LoRAs', async () => {
    settings = { post_editor_model: QWEN, post_editor_loras: JSON.stringify({ klein: [ROW] }), edit_loras_klein: JSON.stringify([ROW]), edit_loras: JSON.stringify([ROW]), person_lora: 'WrongPerson' }
    const { editor } = await mountEditor()
    expect(editor.snapshot()).toEqual({ model: KLEIN, loras: [] })
  })
  it('recalls each family and remounts without changing Postprocess settings', async () => {
    const postSaved = JSON.stringify({ qwen: [{ name: 'PostOnly', weight: WEIGHT }] })
    settings = { post_editor_model: QWEN, post_editor_loras: postSaved, stylist_edit_editor_model: KLEIN, stylist_edit_editor_loras: JSON.stringify({ klein: [ROW], qwen: [{ name: 'QwenOnly', weight: WEIGHT }] }) }
    const { wrapper, editor } = await mountEditor()
    expect(names(wrapper)).toEqual([ROW.name])
    await select(wrapper, QWEN)
    expect(names(wrapper)).toEqual(['QwenOnly'])
    await select(wrapper, FLUX3)
    await wrapper.find('[data-testid="stylist-editor-lora-add"]').trigger('click')
    await wrapper.find('[data-testid="stylist-editor-lora-name"]').setValue('Flux3Only')
    await wrapper.find('[data-testid="stylist-editor-lora-weight"]').setValue(String(WEIGHT))
    const snapshot = editor.snapshot()
    wrapper.unmount()
    await settle()
    expect(settings.post_editor_model).toBe(QWEN)
    expect(settings.post_editor_loras).toBe(postSaved)
    const next = await mountEditor()
    expect(next.editor.snapshot()).toEqual(snapshot)
    next.wrapper.unmount()
    const post = await mountEditor(false)
    post.wrapper.unmount()
    await settle()
    expect(settings.stylist_edit_editor_model).toBe(FLUX3)
    expect(JSON.parse(settings.stylist_edit_editor_loras).flux3).toEqual(snapshot.loras)
  })
  it('keeps an unavailable checkpoint visible and blocks its snapshot', async () => {
    const missing = 'flux-3-klein-removed.gguf'
    settings = { stylist_edit_editor_model: missing }
    const { wrapper, editor } = await mountEditor()
    expect(wrapper.find('[data-testid="stylist-editor-model"]').element.value).toBe(missing)
    expect(editor.ready.value).toBe(false)
    expect(() => editor.snapshot()).toThrow()
  })
  it.each(CATALOG.map(c => [c.filename, c.modules]))('%s requires every installed module', async (model, modules) => {
    settings = { stylist_edit_editor_model: model }
    const full = await mountEditor()
    expect(full.editor.ready.value).toBe(true)
    full.wrapper.unmount()
    const missingModule = modules.at(-1)
    files[missingModule.type] = files[missingModule.type].filter(name => name !== missingModule.filename)
    const missing = await mountEditor()
    expect(missing.editor.ready.value).toBe(false)
    expect(missing.wrapper.text()).toContain(missingModule.filename)
  })
  it('bounds saved LoRA count and weights and snapshots deeply', async () => {
    settings = { stylist_edit_editor_loras: JSON.stringify({ klein: Array.from({ length: MAX_ROWS + 1 }, (_, index) => ({ name: `Style${index}`, weight: index ? MAX_WEIGHT + 1 : 0 })) }) }
    const { wrapper, editor } = await mountEditor()
    expect(names(wrapper)).toHaveLength(MAX_ROWS)
    expect(wrapper.find('[data-testid="stylist-editor-lora-add"]').element.disabled).toBe(true)
    const snapshot = editor.snapshot()
    expect(snapshot.loras[0].weight).toBe(0)
    expect(snapshot.loras.slice(1).every(row => row.weight === MAX_WEIGHT)).toBe(true)
    await wrapper.find('[data-testid="stylist-editor-lora-weight"]').setValue(String(WEIGHT))
    expect(snapshot.loras[0].weight).toBe(0)
  })
})
