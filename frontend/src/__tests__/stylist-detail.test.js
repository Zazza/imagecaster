import { it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import { clearEventMocks } from './mocks/runtime'
import { StylistPresets, StylistPlan, StylistShoot, StylistAppearance, ListPeople, FaceSwapStatus, GetQueue, StylistFrames, StylistLastShoot, GetSessionImage, GetSessionThumb } from '../wailsjs/go/main/App.js'
import { settle } from './helpers/index.js'
import { useStylistPlan, resetStylistPlan, planSnapshot } from '../composables/useStylistPlan.js'
import StylistPage from '../components/StylistPage.vue'

enableAutoUnmount(afterEach)
const tid = id => `[data-testid="${id}"]`
beforeEach(() => {
  vi.clearAllMocks()
  clearEventMocks()
  resetStylistPlan()
  useStylistPlan().form.frames = 1
  StylistPresets.mockResolvedValue({ version: 1, themes: [], outfit: [], hair: [], makeup: [], background: [], pose: [] })
  ListPeople.mockResolvedValue([{ name: 'anna', base: 'Z-Image', lora: 'anna_z', gender: 'woman', identity: true }])
  FaceSwapStatus.mockResolvedValue('ok')
  StylistAppearance.mockResolvedValue('')
  StylistPlan.mockResolvedValue({ series: [{ outfit: 'dress', shots: [{ pose: 'Front', prompt: 'front' }] }], warnings: [], used_llm: true })
  GetQueue.mockResolvedValue([])
  StylistFrames.mockResolvedValue([])
  StylistLastShoot.mockResolvedValue([])
  StylistShoot.mockResolvedValue([101])
})

it('keeps default face pass off and restoration at the legacy strength', async () => {
  const w = mount(StylistPage)
  await settle()
  expect(w.find(tid('stylist-face-pass-toggle')).element.checked).toBe(false)
  expect(Number(w.find(tid('stylist-face-restore-blend')).element.value)).toBe(0.6)
})

it('persists chosen face detail options and snapshots them into a shoot', async () => {
  const w = mount(StylistPage)
  await settle()
  await w.find(tid('stylist-face-pass-toggle')).setValue(true)
  await w.find(tid('stylist-face-restore-blend')).setValue('0')
  await settle()
  expect(JSON.parse(planSnapshot()).form).toMatchObject({ facePass: true, faceRestoreBlend: 0 })
  await w.find(tid('stylist-plan')).trigger('click')
  await settle()
  await w.find(tid('stylist-shoot')).trigger('click')
  await settle()
  expect(StylistShoot).toHaveBeenCalled()
  expect(StylistShoot.mock.calls[0][0]).toMatchObject({ face_pass: true, face_restore_blend: 0 })
})

it('removes the previous frame from detail restoration while the selected image loads or fails', async () => {
  StylistLastShoot.mockResolvedValue([7, 8])
  StylistFrames.mockResolvedValue([
    { job_id: 7, status: 'completed', item_id: 107, shot: 'Front' },
    { job_id: 8, status: 'completed', item_id: 108, shot: 'Side' },
  ])
  GetSessionThumb.mockResolvedValue('SYNTHETIC_THUMB')
  let rejectNext
  GetSessionImage.mockImplementation(id => id === 107 ? Promise.resolve('SYNTHETIC_IMAGE_A') : new Promise((_, reject) => { rejectNext = reject }))
  const w = mount(StylistPage)
  await settle()
  const frames = w.findAll(tid('stylist-frame'))
  await frames[0].trigger('click')
  await settle()
  expect(w.find(tid('stylist-big')).attributes('src')).toContain('SYNTHETIC_IMAGE_A')
  expect(w.find(tid('stylist-detail-panel')).exists()).toBe(true)
  await frames[1].trigger('click')
  await settle()
  const pendingImage = w.find(tid('stylist-big')).exists()
  const pendingDetails = w.find(tid('stylist-detail-panel')).exists()
  rejectNext(new Error('synthetic missing image'))
  await settle()
  expect({ pendingImage, pendingDetails }).toEqual({ pendingImage: false, pendingDetails: false })
  expect(w.find(tid('stylist-big')).exists()).toBe(false)
  expect(w.find(tid('stylist-detail-panel')).exists()).toBe(false)
})
