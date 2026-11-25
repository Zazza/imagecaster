import { it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import DetailRestorePanel from '../components/stylist/DetailRestorePanel.vue'
import { api } from '../api.js'
import PostViewer from '../components/post/PostViewer.vue'
import { settle } from './helpers/index.js'

it('keeps restoration disabled until a source, painted mask and alignment confirmation exist', async () => {
  const restoreFunction = vi.fn()
  const w = mount(DetailRestorePanel, { props: { frame: { jobId: 101, itemId: 201, status: 'completed' }, target: 'TARGET', restore: restoreFunction } })
  await settle()
  expect(w.find('[data-testid="detail-apply"]').element.disabled).toBe(true)
  expect(w.find('[data-testid="detail-aligned"]').element.checked).toBe(false)
  expect(restoreFunction).not.toHaveBeenCalled()
  w.unmount()
})

let images
beforeEach(() => {
 images=[]
 vi.stubGlobal('Image',class {
  constructor(){images.push(this)}
  load(width,height){this.naturalWidth=width;this.naturalHeight=height;this.onload?.()}
 })
 api.stylistDetailSource=vi.fn()
})
afterEach(()=>vi.unstubAllGlobals())
const props=()=>({frame:{jobId:101,itemId:201,status:'completed'},target:'TARGET',restore:vi.fn()})

it('rejects source dimensions that do not match the displayed target',async()=>{
 api.stylistDetailSource.mockResolvedValue('SOURCE')
 const w=mount(DetailRestorePanel,{props:props()})
 await w.find('[data-testid="detail-load-source"]').trigger('click')
 await settle()
 images[0].load(10,10)
 images[1].load(20,10)
 await settle()
 expect(w.find('[data-testid="detail-error"]').exists()).toBe(true)
 expect(w.find('[data-testid="detail-apply"]').element.disabled).toBe(true)
 expect(w.findComponent(PostViewer).exists()).toBe(false)
 w.unmount()
})

it('discards a late source response after switching the selected frame',async()=>{
 let resolve
 api.stylistDetailSource.mockReturnValue(new Promise(r=>{resolve=r}))
 const w=mount(DetailRestorePanel,{props:props()})
 await w.find('[data-testid="detail-load-source"]').trigger('click')
 await w.setProps({frame:{jobId:102,itemId:202,status:'completed'}})
 resolve('STALE')
 await settle()
 expect(w.findComponent(PostViewer).exists()).toBe(false)
 expect(w.find('[data-testid="detail-aligned"]').element.checked).toBe(false)
 expect(w.find('[data-testid="detail-apply"]').element.disabled).toBe(true)
 w.unmount()
})

it('requires painted mask and confirmation, then sends an immutable frame and stage snapshot',async()=>{
 vi.spyOn(HTMLCanvasElement.prototype,'getContext').mockImplementation(function(){
  return {getImageData:()=>({data:new Uint8ClampedArray([255,255,255,255])}),createImageData:()=>({data:new Uint8ClampedArray(4)}),putImageData(){},clearRect(){},beginPath(){},arc(){},fill(){}}
 })
 vi.spyOn(HTMLCanvasElement.prototype,'toDataURL').mockReturnValue('data:image/png;base64,MASK')
 api.stylistDetailSource.mockResolvedValue('SOURCE')
 const p=props()
 p.restore.mockResolvedValue(999)
 const w=mount(DetailRestorePanel,{props:p,global:{stubs:{PostViewer:true}}})
 await w.find('[data-testid="detail-load-source"]').trigger('click')
 await settle()
 images[0].load(10,10)
 images[1].load(10,10)
 await settle()
 images[2].load(10,10)
 await settle()
 await w.find('[data-testid="detail-aligned"]').setValue(true)
 expect(w.find('[data-testid="detail-apply"]').element.disabled).toBe(true)
 await w.find('[data-testid="detail-paint-toggle"]').trigger('click')
 const mask=w.findComponent(PostViewer).props('maskComp')
 mask.beginStroke(2,2,1)
 mask.endStroke()
 await settle()
 expect(w.find('[data-testid="detail-apply"]').element.disabled).toBe(false)
 await w.find('[data-testid="detail-apply"]').trigger('click')
 await settle()
 expect(p.restore).toHaveBeenCalledWith({job_id:101,stage:'base',mask_base64:'MASK',aligned:true})
 expect(w.emitted('queued')).toEqual([[999]])
 expect(w.find('[data-testid="detail-aligned"]').element.checked).toBe(false)
 expect(w.find('[data-testid="detail-apply"]').element.disabled).toBe(true)
 await w.setProps({frame:{jobId:102,itemId:202,status:'completed'}})
 expect(p.restore.mock.calls[0][0].job_id).toBe(101)
 w.unmount()
 vi.restoreAllMocks()
})
