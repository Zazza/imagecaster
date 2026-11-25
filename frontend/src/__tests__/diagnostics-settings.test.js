import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount, enableAutoUnmount } from '@vue/test-utils'
import DiagnosticsTab from '../components/settings/DiagnosticsTab.vue'
import { api } from '../api.js'
import { settle } from './helpers/async.js'

enableAutoUnmount(afterEach)
beforeEach(() => {
 vi.spyOn(api, 'getSettings').mockResolvedValue({})
 vi.spyOn(api, 'updateSettings').mockResolvedValue(undefined)
 vi.spyOn(api, 'diagnosticsStorage').mockResolvedValue({path:'/tmp/test-diagnostics',exists:false,bytes:0,frames:0,active:0})
 vi.spyOn(api, 'clearDiagnostics').mockResolvedValue(undefined)
 vi.spyOn(api, 'openDiagnosticsFolder').mockResolvedValue(undefined)
})
afterEach(() => vi.restoreAllMocks())
describe('diagnostics settings', () => {
 it('shows conservative defaults and requires explicit confirmation before clearing', async () => {
  const w=mount(DiagnosticsTab); await settle()
  expect(w.get('[data-testid="diagnostics-verbose"]').element.checked).toBe(false)
  expect(w.get('[data-testid="diagnostics-images"]').element.checked).toBe(false)
  expect(w.get('[data-testid="diagnostics-originals"]').element.checked).toBe(true)
  expect(w.get('[data-testid="diagnostics-retention"]').element.value).toBe('10')
  expect(w.get('[data-testid="diagnostics-clear"]').element.disabled).toBe(true)
  await w.get('[data-testid="diagnostics-clear-confirm"]').setValue(true)
  await w.get('[data-testid="diagnostics-clear"]').trigger('click'); await settle()
  expect(api.clearDiagnostics).toHaveBeenCalledWith(true)
 })
 it('persists switches and exposes storage failures', async () => {
  const w=mount(DiagnosticsTab);await settle()
  await w.get('[data-testid="diagnostics-verbose"]').setValue(true);await settle()
  expect(api.updateSettings).toHaveBeenCalledWith({diagnostics_verbose_logs:'true'})
  api.diagnosticsStorage.mockRejectedValueOnce(new Error('storage unavailable'))
  await w.get('[data-testid="diagnostics-refresh"]').trigger('click');await settle()
  expect(w.get('[data-testid="diagnostics-error"]').text()).toContain('storage unavailable')
 })
 it('blocks clearing while a recorder is active and opens the dedicated folder', async () => {
  api.diagnosticsStorage.mockResolvedValue({path:'/tmp/test-diagnostics',exists:true,bytes:128,frames:2,active:1})
  const w=mount(DiagnosticsTab);await settle()
  expect(w.text()).toContain('/tmp/test-diagnostics')
  await w.get('[data-testid="diagnostics-clear-confirm"]').setValue(true)
  expect(w.get('[data-testid="diagnostics-clear"]').element.disabled).toBe(true)
  await w.get('[data-testid="diagnostics-open"]').trigger('click');await settle()
  expect(api.openDiagnosticsFolder).toHaveBeenCalledOnce()
  expect(api.clearDiagnostics).not.toHaveBeenCalled()
 })
 it('rejects out-of-range retention without persisting it', async () => {
  const w=mount(DiagnosticsTab);await settle()
  await w.get('[data-testid="diagnostics-retention"]').setValue('51');await settle()
  expect(api.updateSettings).not.toHaveBeenCalled()
  expect(w.get('[data-testid="diagnostics-error"]').text()).not.toBe('')
 })

 it('rolls back a failed switch save and leaves other preferences usable', async () => {
  api.getSettings.mockResolvedValue({diagnostics_verbose_logs:'false',diagnostics_intermediate_images:'true',stylist_keep_originals:'false',diagnostics_retention:'7'})
  api.updateSettings.mockRejectedValueOnce(new Error('save unavailable'))
  const w=mount(DiagnosticsTab);await settle()
  await w.get('[data-testid="diagnostics-verbose"]').setValue(true);await settle()
  expect(w.get('[data-testid="diagnostics-verbose"]').element.checked).toBe(false)
  expect(w.get('[data-testid="diagnostics-images"]').element.checked).toBe(true)
  expect(w.get('[data-testid="diagnostics-originals"]').element.checked).toBe(false)
  expect(w.get('[data-testid="diagnostics-retention"]').element.value).toBe('7')
  expect(w.get('[data-testid="diagnostics-error"]').text()).toContain('save unavailable')
  for (const name of ['verbose','images','originals','retention']) expect(w.get(`[data-testid="diagnostics-${name}"]`).element.disabled).toBe(false)
 })

})
