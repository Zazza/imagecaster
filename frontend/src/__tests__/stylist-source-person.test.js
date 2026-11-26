import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { api } from '../api.js'
import { people } from '../peopleStore.js'
import { sourceSubmitting } from '../composables/useStylistPlan.js'
import { usePostEditor } from '../composables/usePostEditor.js'
import { settle } from './helpers/async.js'
import SourceVariantsPanel from '../components/stylist/SourceVariantsPanel.vue'

vi.mock('../api.js', () => ({ api: { stylistSourcePlan: vi.fn(), stylistSourceShoot: vi.fn() } }))
vi.mock('../imageFile.js', () => ({ readImageFile: vi.fn(async () => 'source-photo') }))
vi.mock('../peopleStore.js', async () => ({ people: (await vi.importActual('vue')).ref([]) }))
vi.mock('../composables/usePostEditor.js', () => ({ usePostEditor: vi.fn() }))
vi.mock('../composables/useStylistPlan.js', async () => ({ adoptSourceFrames: vi.fn(), sourceSubmitting: (await vi.importActual('vue')).ref(false) }))
vi.mock('../components/post/SettingsPanel.vue', () => ({ default: { template: '<div />' } }))

enableAutoUnmount(afterEach)
const tid = id => `[data-testid="${id}"]`
const LORA_WEIGHT = 0.75
let editor
beforeEach(() => {
  vi.clearAllMocks()
  sourceSubmitting.value = false
  people.value = [
    { name: 'anna_z', display_name: 'Anna', lora: 'anna_z.safetensors', base: 'Z-Image', identity: true },
    { name: 'anna_flux', display_name: 'Anna', lora: 'anna_flux.safetensors', base: 'Flux.1', identity: true },
    { name: 'olga_x', display_name: 'Olga', lora: 'olga_x.safetensors', base: 'SDXL', identity: false },
    { name: 'untrained', display_name: 'Pending', base: 'Z-Image', identity: true },
    { name: 'old', display_name: 'Unsupported', lora: 'old.safetensors', base: 'SD1.5' },
  ]
  editor = { model: 'pose-editor.gguf', loras: [{ name: 'Pose style', weight: LORA_WEIGHT }] }
  usePostEditor.mockReturnValue({ ready: ref(true), snapshot: () => editor })
  api.stylistSourcePlan.mockResolvedValue(['Turn left.', 'Turn right.', 'Relax arms.'])
  api.stylistSourceShoot.mockResolvedValue({ job_ids: [41] })
})

async function panel() {
  const wrapper = mount(SourceVariantsPanel, { props: { faceSwapOn: true } })
  const input = wrapper.get(tid('source-photo-input'))
  Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['photo'], 'source.png', { type: 'image/png' })] })
  await input.trigger('change')
  await settle()
  await wrapper.get(tid('source-plan')).trigger('click')
  await settle()
  return wrapper
}
const options = select => select.findAll('option').map(option => option.element.value)

describe('optional identity for source photo variants', () => {
  it('offers trained people and only their compatible models independently of the editor', async () => {
    const wrapper = await panel()
    expect(options(wrapper.get(tid('source-person')))).toEqual(['', 'Anna', 'Olga'])
    expect(wrapper.find(tid('source-face-model')).exists()).toBe(false)
    await wrapper.get(tid('source-person')).setValue('Anna')
    expect(options(wrapper.get(tid('source-face-model')))).toEqual(['', 'Z-Image', 'Flux.1'])
    expect(wrapper.get(tid('source-face-model')).element.value).toBe('')
    await wrapper.get(tid('source-face-model')).setValue('Flux.1')
    await wrapper.get(tid('source-run')).trigger('click')
    expect(api.stylistSourceShoot.mock.calls[0][0]).toMatchObject({ person: 'Anna', base: 'Flux.1', face_swap: false, editor })
  })

  it('supports automatic face model selection and optional face swap', async () => {
    const wrapper = await panel()
    await wrapper.get(tid('source-person')).setValue('Anna')
    await wrapper.get(tid('source-face-swap')).setValue(true)
    await wrapper.get(tid('source-run')).trigger('click')
    expect(api.stylistSourceShoot.mock.calls[0][0]).toMatchObject({ person: 'Anna', base: '', face_swap: true })
  })

  it('resets stale manual model and swap on person change', async () => {
    const wrapper = await panel()
    await wrapper.get(tid('source-person')).setValue('Anna')
    await wrapper.get(tid('source-face-model')).setValue('Flux.1')
    await wrapper.get(tid('source-face-swap')).setValue(true)
    await wrapper.get(tid('source-person')).setValue('Olga')
    expect(options(wrapper.get(tid('source-face-model')))).toEqual(['', 'SDXL'])
    expect(wrapper.get(tid('source-face-model')).element.value).toBe('')
    expect(wrapper.find(tid('source-face-swap')).exists()).toBe(false)
    await wrapper.get(tid('source-run')).trigger('click')
    expect(api.stylistSourceShoot.mock.calls[0][0]).toMatchObject({ person: 'Olga', base: '', face_swap: false })
  })

  it('sends no identity fields after clearing the optional person', async () => {
    const wrapper = await panel()
    await wrapper.get(tid('source-person')).setValue('Anna')
    await wrapper.get(tid('source-face-model')).setValue('Flux.1')
    await wrapper.get(tid('source-face-swap')).setValue(true)
    await wrapper.get(tid('source-person')).setValue('')
    await wrapper.get(tid('source-run')).trigger('click')
    const request = api.stylistSourceShoot.mock.calls[0][0]
    for (const key of ['person', 'base', 'face_swap']) expect(request).not.toHaveProperty(key)
    expect(request.editor).toEqual(editor)
  })

  it('freezes identity and editor at launch while the request is pending', async () => {
    let resolve
    api.stylistSourceShoot.mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = await panel()
    await wrapper.get(tid('source-person')).setValue('Anna')
    await wrapper.get(tid('source-face-model')).setValue('Flux.1')
    await wrapper.get(tid('source-face-swap')).setValue(true)
    await wrapper.get(tid('source-run')).trigger('click')
    const request = api.stylistSourceShoot.mock.calls[0][0]
    expect(wrapper.get(tid('source-person')).element.disabled).toBe(true)
    expect(wrapper.get(tid('source-face-model')).element.disabled).toBe(true)
    editor.model = 'changed-editor.gguf'
    editor.loras[0].name = 'Changed style'
    people.value = []
    await settle()
    expect(request).toMatchObject({ person: 'Anna', base: 'Flux.1', face_swap: true, editor: { model: 'pose-editor.gguf', loras: [{ name: 'Pose style', weight: LORA_WEIGHT }] } })
    resolve({ job_ids: [41] })
    await settle()
  })
})
