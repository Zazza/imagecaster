import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import EditSection from '../components/EditSection.vue'
import { POST_EDITOR_KEY } from '../composables/usePostEditor.js'
import { GetSettings, ListPresets } from '../wailsjs/go/main/App.js'
import { mockInstalledEditor } from './helpers/post-editor.js'
import { settle } from './helpers/async.js'
import { t } from '../i18n/index.js'

const FULL = 1
const PARTIAL = 0.5
const MINIMUM = 0.35
const STEP = 0.01
const REQUEST = 'cold blue cinematic lighting'
enableAutoUnmount(afterEach)
beforeEach(() => {
 vi.clearAllMocks()
 mockInstalledEditor()
 GetSettings.mockResolvedValue({ edit_look_mode: 'free' })
 ListPresets.mockResolvedValue([])
})
const click = async (w, label) => {
 const button = w.findAll('button').find(b => b.text().includes(label))
 expect(button).toBeTruthy()
 await button.trigger('click')
 await settle()
}
async function mountFree(batch = false) {
 const editor = { model: ref('flux-2-klein-4b.safetensors'), family: ref('klein'), ready: ref(true), snapshot: vi.fn(() => ({ model: 'flux-2-klein-4b.safetensors', loras: [] })) }
 const w = mount(EditSection, { props: { source: 'c291cmNl', batch }, global: { provide: { [POST_EDITOR_KEY]: editor } } })
 await settle()
 await click(w, t('edit.mode_look'))
 await w.find('[data-testid="edit-look-text"]').setValue(REQUEST)
 return { w, editor }
}
describe('free look reference strength', () => {
 it('offers full strength by default with a precise adjustable range', async () => {
  const { w } = await mountFree()
  const range = w.find('[data-testid="edit-look-strength"]')
  expect(Number(range.attributes('min'))).toBe(MINIMUM)
  expect(Number(range.attributes('max'))).toBe(FULL)
  expect(Number(range.attributes('step'))).toBe(STEP)
  expect(Number(range.element.value)).toBe(FULL)
 })
 it.each([false, true])('emits the chosen strength without resetting the shared editor (batch=%s)', async batch => {
  const { w, editor } = await mountFree(batch)
  for (const strength of [PARTIAL, FULL]) {
   await w.find('[data-testid="edit-look-strength"]').setValue(String(strength))
   await click(w, t('edit.look_apply'))
   expect(w.emitted('apply-look').at(-1)[0]).toEqual({ text: REQUEST, strength })
   expect(editor.model.value).toBe('flux-2-klein-4b.safetensors')
   expect(editor.ready.value).toBe(true)
  }
 })
})
