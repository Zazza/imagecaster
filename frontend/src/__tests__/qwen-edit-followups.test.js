import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import EditSection from '../components/EditSection.vue'
import { POST_EDITOR_KEY } from '../composables/usePostEditor.js'
import { GetSettings, ListPresets, EditImage } from '../wailsjs/go/main/App.js'
import { settle } from './helpers/async.js'
import { t } from '../i18n/index.js'

enableAutoUnmount(afterEach)
const QWEN = 'qwen_image_2.1-Q8_0.gguf'
const ROW = { name: 'QwenPortrait', weight: 0.8 }
const STYLE = { id: 4, name: 'Qwen Film', model_name: QWEN, prompt: 'film', loras: '[{"name":"StyleOnly","weight":1}]' }
beforeEach(() => {
  vi.clearAllMocks()
  GetSettings.mockResolvedValue({})
  ListPresets.mockResolvedValue([STYLE, { id: 2, name: 'Klein Film', model_name: 'flux-2-klein-4b' }])
  EditImage.mockResolvedValue({ image: 'cmVzdWx0' })
})
describe('Qwen edit with the shared editor', () => {
  it('filters styles by family and sends explicit editor independently of style LoRA', async () => {
    const editor = { model: ref(QWEN), family: ref('qwen'), ready: ref(true), snapshot: () => ({ model: QWEN, loras: [{ ...ROW }] }) }
    const w = mount(EditSection, { props: { source: 'c291cmNl' }, global: { provide: { [POST_EDITOR_KEY]: editor } } })
    await settle()
    const styles = w.find('[data-testid="edit-style-select"]')
    expect(styles.findAll('option').map(o => o.text())).toContain(STYLE.name)
    expect(styles.findAll('option').map(o => o.text())).not.toContain('Klein Film')
    await styles.setValue(String(STYLE.id))
    await w.find('[data-testid="edit-instruction"]').setValue('change hair')
    await w.findAll('button').find(b => b.text().includes(t('edit.btn_generate'))).trigger('click')
    await settle()
    expect(EditImage).toHaveBeenCalledWith(expect.objectContaining({ editor: { model: QWEN, loras: [ROW] }, style_preset_id: STYLE.id }))
    expect(w.find('[data-testid="edit-model-select"]').exists()).toBe(false)
    expect(w.find('[data-testid="edit-lora-add"]').exists()).toBe(false)
  })
})
