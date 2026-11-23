import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref } from 'vue'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import EditSection from '../components/EditSection.vue'
import PostProcessPage from '../components/PostProcessPage.vue'
import { POST_EDITOR_KEY } from '../composables/usePostEditor.js'
import { GetSettings, ListPresets, ListPresetTypes, GetActiveSessionItem, GetSessionImage, ListHiresProfiles, ReadFileAsBase64, EnqueueLook, ApplyPostLook } from '../wailsjs/go/main/App.js'
import { mockInstalledEditor } from './helpers/post-editor.js'
import { useFakeImage, useFakeCanvas } from './helpers/dom.js'
import { useRemixQueue } from '../remixQueue.js'
import { settle } from './helpers/async.js'
import { t } from '../i18n/index.js'

const REQUEST = 'hand in hair, head turned left, cold blue light'
const PARTIAL = 0.5
const SOURCE_A = 'QUFBQQ=='
const SOURCE_B = 'QkJCQg=='
const MODEL = 'flux-2-klein-4b.safetensors'
enableAutoUnmount(afterEach)
useFakeImage()
useFakeCanvas()
beforeEach(() => {
 vi.clearAllMocks()
 mockInstalledEditor()
 useRemixQueue().clearPostQueue()
 GetSettings.mockResolvedValue({ edit_look_mode: 'free' })
 ListPresets.mockResolvedValue([])
 ListPresetTypes.mockResolvedValue([])
 ListHiresProfiles.mockResolvedValue([])
 GetActiveSessionItem.mockResolvedValue(null)
 GetSessionImage.mockResolvedValue('')
 ReadFileAsBase64.mockImplementation(path => Promise.resolve(path.includes('a.png') ? SOURCE_A : SOURCE_B))
})
async function click(w, label) {
 const button = w.findAll('button').find(b => b.text() === label || b.text().includes(label))
 expect(button).toBeTruthy()
 await button.trigger('click')
 await settle()
}
async function free(w) {
 await click(w, t('edit.mode_look'))
 if (!w.find('[data-testid="edit-look-text"]').exists()) await click(w, t('edit.look_mode_free'))
 await w.find('[data-testid="edit-look-text"]').setValue(REQUEST)
}
describe('opt-in pose and scene look', () => {
 it.each([false, true])('defaults off, emits opted-in flag, and restores legacy payload (batch=%s)', async batch => {
  const editor = { model: ref(MODEL), family: ref('klein'), ready: ref(true), snapshot: () => ({ model: MODEL, loras: [] }) }
  const w = mount(EditSection, { props: { source: SOURCE_A, batch }, global: { provide: { [POST_EDITOR_KEY]: editor } } })
  await settle()
  expect(w.find('[data-testid="edit-look-change-scene"]').exists()).toBe(false)
  await free(w)
  const checkbox = w.get('[data-testid="edit-look-change-scene"]')
  expect(checkbox.element.checked).toBe(false)
  await w.get('[data-testid="edit-look-strength"]').setValue(String(PARTIAL))
  await click(w, t('edit.look_apply'))
  expect(w.emitted('apply-look').at(-1)[0]).toEqual({ text: REQUEST, strength: PARTIAL })
  await checkbox.setValue(true)
  await click(w, t('edit.look_apply'))
  expect(w.emitted('apply-look').at(-1)[0]).toEqual({ text: REQUEST, strength: PARTIAL, change_scene: true })
  await checkbox.setValue(false)
  await click(w, t('edit.look_apply'))
  expect(w.emitted('apply-look').at(-1)[0]).toEqual({ text: REQUEST, strength: PARTIAL })
  await click(w, t('edit.look_mode_effects'))
  expect(w.find('[data-testid="edit-look-change-scene"]').exists()).toBe(false)
 })
 it('passes opt-in through the direct typed API with the selected editor', async () => {
  GetActiveSessionItem.mockResolvedValue({ id: 1, info: 'null', is_preview: false })
  GetSessionImage.mockResolvedValue(SOURCE_A)
  ApplyPostLook.mockResolvedValue({ image_base64: SOURCE_B })
  const w = mount(PostProcessPage, { props: { initialSection: 'edit' } })
  await settle()
  await free(w)
  await w.get('[data-testid="edit-look-change-scene"]').setValue(true)
  await click(w, t('edit.look_apply'))
  expect(ApplyPostLook).toHaveBeenCalledTimes(1)
  const p = ApplyPostLook.mock.calls[0][0]
  expect(p.change_scene).toBe(true)
  expect(p.text).toBe(REQUEST)
  expect(p.editor.model).toBe(MODEL)
  expect(p.image_base64.replace(/^data:[^,]*,/, '')).toBe(SOURCE_A)
  expect(EnqueueLook).not.toHaveBeenCalled()
 })
 it('snapshots opt-in for every batch photo before waiting for the first enqueue', async () => {
  let resolveFirst
  EnqueueLook.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve })).mockResolvedValue('scene-second')
  useRemixQueue().setPostQueue([{ path: '/photos/a.png', name: 'a.png' }, { path: '/photos/b.png', name: 'b.png' }])
  const w = mount(PostProcessPage, { props: { initialSection: 'edit' } })
  await settle()
  await free(w)
  await w.get('[data-testid="edit-look-change-scene"]').setValue(true)
  await click(w, t('edit.look_apply'))
  expect(EnqueueLook).toHaveBeenCalledTimes(1)
  await w.get('[data-testid="edit-look-change-scene"]').setValue(false)
  resolveFirst('scene-first')
  await settle()
  expect(EnqueueLook).toHaveBeenCalledTimes(2)
  const params = EnqueueLook.mock.calls.map(call => call[0])
  expect(params.map(p => p.image_base64.replace(/^data:[^,]*,/, ''))).toEqual([SOURCE_A, SOURCE_B])
  for (const p of params) {
   expect(p.change_scene).toBe(true)
   expect(p.text).toBe(REQUEST)
   expect(p.editor.model).toBe(MODEL)
  }
  expect(ApplyPostLook).not.toHaveBeenCalled()
 })
})
